package adt

import (
	"bytes"
	"context"
	"crypto/tls"
	"fmt"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"os"
	"strings"
	"time"

	"golang.org/x/net/html"
)

// CredentialProvider returns fresh credentials for SAML authentication.
// Called on each auth attempt (initial + re-auth on 401).
// Caller zeroes returned byte slices after use.
type CredentialProvider func(ctx context.Context) (username, password []byte, err error)

// formData represents an extracted HTML form with its action URL and input fields.
type formData struct {
	Action string
	Method string
	Fields map[string]string
}

// maxSAMLHops limits the number of form-based POST redirects in the SAML chain.
const maxSAMLHops = 10

// SAMLLogin performs programmatic SAML SSO authentication against SAP S/4HANA via IAS.
//
// The 4-step dance:
//  1. GET SAP target URL → follow redirects → arrive at IdP (IAS) login page
//  2. Parse IAS login form, fill in credentials, POST to IAS
//  3. Parse SAMLResponse form from IAS response
//  4. Follow form POST chain (up to 10 hops) back to SAP → extract session cookies
//
// MFA is not supported — use --browser-auth for MFA-protected systems.
func SAMLLogin(ctx context.Context, sapURL string, credProvider CredentialProvider, insecure, verbose bool) (map[string]string, error) {
	username, password, err := credProvider(ctx)
	if err != nil {
		return nil, fmt.Errorf("credential provider: %w", err)
	}
	defer zeroBytes(username)
	defer zeroBytes(password)

	jar, _ := cookiejar.New(nil)
	client := &http.Client{
		Jar: jar,
		Transport: &http.Transport{
			Proxy: http.ProxyFromEnvironment,
			TLSClientConfig: &tls.Config{
				InsecureSkipVerify: insecure, //nolint:gosec // User-controlled via --insecure flag
			},
		},
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= maxSAMLHops {
				return fmt.Errorf("SAML redirect loop: exceeded %d hops", maxSAMLHops)
			}
			if verbose {
				fmt.Fprintf(os.Stderr, "[SAML-AUTH] Redirect → %s\n", sanitizeURLForLog(req.URL.String()))
			}
			return nil
		},
		Timeout: 60 * time.Second,
	}

	u, err := url.Parse(sapURL)
	if err != nil {
		return nil, fmt.Errorf("invalid SAP URL: %w", err)
	}
	if u.Scheme == "" || u.Host == "" {
		return nil, fmt.Errorf("invalid SAP URL (missing scheme or host): %s", sapURL)
	}

	// Target the ADT root — requires authentication, triggers SAML redirect.
	target := *u
	target.Path = "/sap/bc/adt/"
	target.RawQuery = ""
	target.Fragment = ""

	if verbose {
		fmt.Fprintf(os.Stderr, "[SAML-AUTH] Step 1: GET %s\n", target.String())
	}

	// Step 1: GET SAP target → HTTP client follows redirects → arrives at IdP login page.
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, target.String(), nil)
	if err != nil {
		return nil, fmt.Errorf("creating step 1 request: %w", err)
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("SAML step 1 (GET target): %w", err)
	}
	body, err := io.ReadAll(resp.Body)
	resp.Body.Close()
	if err != nil {
		return nil, fmt.Errorf("reading step 1 response: %w", err)
	}

	// Step 2: Parse IdP login form and fill in credentials.
	form, err := extractFormData(body, resp.Request.URL)
	if err != nil {
		return nil, fmt.Errorf("SAML step 1: no login form found in IdP response (status %d from %s): %w",
			resp.StatusCode, sanitizeURLForLog(resp.Request.URL.String()), err)
	}

	if verbose {
		fmt.Fprintf(os.Stderr, "[SAML-AUTH] Step 2: Found login form → %s (%d fields)\n",
			sanitizeURLForLog(form.Action), len(form.Fields))
	}

	// Validate that credentials are sent to the same host as the IdP page
	// to prevent exfiltration via a crafted form action.
	actionURL, err := url.Parse(form.Action)
	if err != nil {
		return nil, fmt.Errorf("invalid login form action URL: %w", err)
	}
	if actionURL.Host != "" && actionURL.Host != resp.Request.URL.Host {
		return nil, fmt.Errorf("refusing to send credentials to different host (%s vs %s)",
			sanitizeURLForLog(form.Action), sanitizeURLForLog(resp.Request.URL.String()))
	}
	if resp.Request.URL.Scheme == "https" && actionURL.Scheme == "http" {
		return nil, fmt.Errorf("refusing to send credentials over HTTP downgrade: %s",
			sanitizeURLForLog(form.Action))
	}

	// Build form values with credentials added directly — never store credentials
	// in form.Fields (Go strings are immutable and cannot be zeroed).
	credValues := url.Values{}
	for k, v := range form.Fields {
		credValues.Set(k, v)
	}
	credValues.Set("j_username", string(username))
	credValues.Set("j_password", string(password))

	resp, err = submitFormValues(ctx, client, form.Action, credValues)
	if err != nil {
		return nil, fmt.Errorf("SAML step 2 (POST credentials to IdP): %w", err)
	}
	body, err = io.ReadAll(resp.Body)
	resp.Body.Close()
	if err != nil {
		return nil, fmt.Errorf("reading step 2 response: %w", err)
	}

	// Steps 3-4: Follow SAMLResponse form chain back to SAP.
	for i := 0; i < maxSAMLHops; i++ {
		form, err = extractFormData(body, resp.Request.URL)
		if err != nil {
			// No more forms to submit — check cookies below.
			break
		}

		if verbose {
			fmt.Fprintf(os.Stderr, "[SAML-AUTH] Step %d: Following form → %s\n",
				i+3, sanitizeURLForLog(form.Action))
		}

		resp, err = submitForm(ctx, client, form)
		if err != nil {
			return nil, fmt.Errorf("SAML step %d (POST form): %w", i+3, err)
		}
		body, err = io.ReadAll(resp.Body)
		resp.Body.Close()
		if err != nil {
			return nil, fmt.Errorf("reading step %d response: %w", i+3, err)
		}
	}

	// Extract SAP cookies from the jar.
	sapCookies := extractSAPCookiesFromJar(jar, u)
	if len(sapCookies) == 0 {
		return nil, fmt.Errorf("SAML authentication completed but no SAP cookies received "+
			"(last status: %d from %s)", resp.StatusCode, sanitizeURLForLog(resp.Request.URL.String()))
	}

	hasAuth := false
	for name := range sapCookies {
		if matchesSAPAuthCookie(name) {
			hasAuth = true
			break
		}
	}
	if !hasAuth {
		return nil, fmt.Errorf("SAML authentication completed but no SAP auth cookies " +
			"(MYSAPSSO2/SAP_SESSIONID) found — check username/password")
	}

	if verbose {
		fmt.Fprintf(os.Stderr, "[SAML-AUTH] Authentication successful — %d cookies extracted\n", len(sapCookies))
		for name := range sapCookies {
			fmt.Fprintf(os.Stderr, "[SAML-AUTH]   cookie: %s\n", name)
		}
	}

	return sapCookies, nil
}

// submitForm submits an HTML form using the method specified in the form data.
func submitForm(ctx context.Context, client *http.Client, form *formData) (*http.Response, error) {
	values := url.Values{}
	for k, v := range form.Fields {
		values.Set(k, v)
	}
	return submitFormValues(ctx, client, form.Action, values)
}

// submitFormValues POSTs URL-encoded form values to the given action URL.
func submitFormValues(ctx context.Context, client *http.Client, action string, values url.Values) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, action, strings.NewReader(values.Encode()))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	return client.Do(req)
}

// extractFormData parses the first HTML <form> from body using the x/net/html tokenizer.
// Resolves relative action URLs against baseURL. Returns all hidden and text/password
// input fields; excludes submit/button/image inputs.
func extractFormData(body []byte, baseURL *url.URL) (*formData, error) {
	tokenizer := html.NewTokenizer(bytes.NewReader(body))

	var form *formData
	inForm := false

	for {
		tt := tokenizer.Next()
		switch tt {
		case html.ErrorToken:
			if form != nil {
				return form, nil
			}
			return nil, fmt.Errorf("no HTML form found")

		case html.StartTagToken, html.SelfClosingTagToken:
			tn, hasAttr := tokenizer.TagName()
			tagName := string(tn)

			if tagName == "form" && hasAttr && !inForm {
				form = &formData{
					Method: "POST",
					Fields: make(map[string]string),
				}
				inForm = true
				for {
					key, val, more := tokenizer.TagAttr()
					switch string(key) {
					case "action":
						action := string(val)
						if baseURL != nil {
							if resolved, err := baseURL.Parse(action); err == nil {
								action = resolved.String()
							}
						}
						form.Action = action
					case "method":
						form.Method = strings.ToUpper(string(val))
					}
					if !more {
						break
					}
				}
			}

			if inForm && tagName == "input" && hasAttr {
				var name, value, inputType string
				for {
					key, val, more := tokenizer.TagAttr()
					switch string(key) {
					case "name":
						name = string(val)
					case "value":
						value = string(val)
					case "type":
						inputType = strings.ToLower(string(val))
					}
					if !more {
						break
					}
				}
				if name != "" && inputType != "submit" && inputType != "button" && inputType != "image" {
					form.Fields[name] = value
				}
			}

		case html.EndTagToken:
			tn, _ := tokenizer.TagName()
			if string(tn) == "form" && inForm {
				return form, nil
			}
		}
	}
}

// extractSAPCookiesFromJar extracts all cookies for the SAP domain from the cookie jar.
// Queries multiple paths to catch path-scoped cookies (same approach as browser_auth.go).
func extractSAPCookiesFromJar(jar http.CookieJar, sapURL *url.URL) map[string]string {
	result := make(map[string]string)
	paths := []string{"", "/sap/", "/sap/bc/", "/sap/bc/adt/"}
	for _, p := range paths {
		u := *sapURL
		u.Path = p
		u.RawQuery = ""
		u.Fragment = ""
		for _, c := range jar.Cookies(&u) {
			result[c.Name] = c.Value
		}
	}
	return result
}

// zeroBytes overwrites a byte slice with zeros to prevent credential leakage.
func zeroBytes(b []byte) {
	for i := range b {
		b[i] = 0
	}
}
