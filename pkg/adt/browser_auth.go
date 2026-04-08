package adt

import (
	"context"
	"fmt"
	"net/url"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"time"

	"github.com/chromedp/cdproto/network"
	"github.com/chromedp/cdproto/page"
	"github.com/chromedp/chromedp"
)

// sapCookieNames are cookie name prefixes that indicate successful SAP authentication.
// These are strong signals — their presence means the user is actually authenticated.
var sapAuthCookieNames = []string{
	"MYSAPSSO2",
	"SAP_SESSIONID",
	"JSESSIONID",
}

// sapWeakCookieNames are set before/during authentication and are not sufficient alone.
var sapWeakCookieNames = []string{
	"sap-usercontext",
}

// browserCandidates lists Chromium-based browsers to search for, in preference order.
// chromedp requires a Chromium-based browser (Chrome, Edge, Brave, Chromium).
var browserCandidates = map[string][]string{
	"windows": {
		`C:\Program Files\Microsoft\Edge\Application\msedge.exe`,
		`C:\Program Files (x86)\Microsoft\Edge\Application\msedge.exe`,
		`C:\Program Files\Google\Chrome\Application\chrome.exe`,
		`C:\Program Files (x86)\Google\Chrome\Application\chrome.exe`,
		`C:\Program Files\BraveSoftware\Brave-Browser\Application\brave.exe`,
	},
	"linux": {
		"microsoft-edge",
		"microsoft-edge-stable",
		"google-chrome",
		"google-chrome-stable",
		"chromium",
		"chromium-browser",
		"brave-browser",
	},
	"darwin": {
		"/Applications/Microsoft Edge.app/Contents/MacOS/Microsoft Edge",
		"/Applications/Google Chrome.app/Contents/MacOS/Google Chrome",
		"/Applications/Brave Browser.app/Contents/MacOS/Brave Browser",
		"/Applications/Chromium.app/Contents/MacOS/Chromium",
	},
}

// FindBrowser searches for an installed Chromium-based browser.
// Returns the executable path and a friendly name, or empty strings if none found.
func FindBrowser() (path string, name string) {
	candidates := browserCandidates[runtime.GOOS]
	for _, candidate := range candidates {
		if runtime.GOOS == "windows" {
			// On Windows, check absolute paths directly
			if _, err := os.Stat(candidate); err == nil {
				return candidate, friendlyBrowserName(candidate)
			}
		} else {
			// On Linux/macOS, try both absolute path and PATH lookup
			if _, err := os.Stat(candidate); err == nil {
				return candidate, friendlyBrowserName(candidate)
			}
			if p, err := exec.LookPath(candidate); err == nil {
				return p, friendlyBrowserName(candidate)
			}
		}
	}
	return "", ""
}

func friendlyBrowserName(path string) string {
	lower := strings.ToLower(path)
	switch {
	case strings.Contains(lower, "edge") || strings.Contains(lower, "msedge"):
		return "Microsoft Edge"
	case strings.Contains(lower, "brave"):
		return "Brave"
	case strings.Contains(lower, "chromium"):
		return "Chromium"
	default:
		return "Google Chrome"
	}
}

// BrowserLogin opens a headed browser window to the SAP system URL, waits for
// SSO authentication to complete (Kerberos/SPNEGO, Keycloak, SAML, etc.),
// and returns the session cookies.
//
// If execPath is empty, it auto-detects an installed Chromium-based browser
// (Edge, Chrome, Chromium, Brave). Set execPath to force a specific browser.
//
// The browser navigates to the ADT discovery endpoint which requires authentication,
// triggering the SSO redirect. Once SAP-specific cookies appear, they are extracted
// and the browser is closed.
func BrowserLogin(ctx context.Context, sapURL string, insecure bool, timeout time.Duration, execPath string, verbose bool) (map[string]string, error) {
	u, err := url.Parse(sapURL)
	if err != nil {
		return nil, fmt.Errorf("invalid SAP URL: %w", err)
	}
	if u.Scheme == "" || u.Host == "" {
		return nil, fmt.Errorf("invalid SAP URL (missing scheme or host): %s", sapURL)
	}

	// Target URL that requires authentication and renders HTML.
	// We use the ADT root which returns an HTML page after auth.
	// The /sap/bc/adt/core/discovery endpoint returns XML which browsers
	// try to download as a file, breaking the flow.
	// Build from parsed URL to handle sapURL with query/fragment correctly.
	adtURL := *u
	adtURL.Path = "/sap/bc/adt/"
	adtURL.RawQuery = ""
	adtURL.Fragment = ""
	targetURL := adtURL.String()

	// Create a headed (non-headless) browser context
	opts := append(chromedp.DefaultExecAllocatorOptions[:],
		chromedp.Flag("headless", false),
		chromedp.Flag("disable-gpu", false),
		chromedp.WindowSize(800, 700),
		// Set User-Agent to mimic Eclipse ADT. SAP ICM/ICF on many systems
		// (especially R3/ECC) only sends WWW-Authenticate: Negotiate (Kerberos)
		// for recognized ADT User-Agents. Without this, the system falls back
		// to form-based login instead of triggering SPNEGO.
		chromedp.UserAgent("Eclipse/4.39.0 (win32; x86_64) ADT/3.56.0 (devedition)"),
		// Enable Kerberos/SPNEGO: chromedp uses a temporary profile that doesn't
		// inherit the system's Integrated Windows Auth settings. These flags
		// explicitly allow Negotiate auth with the SAP server.
		chromedp.Flag("auth-server-whitelist", u.Host),
		chromedp.Flag("auth-negotiate-delegate-whitelist", u.Host),
	)

	// Determine which browser to use
	browserName := "browser"
	if execPath != "" {
		// User specified a browser explicitly
		if _, err := os.Stat(execPath); os.IsNotExist(err) {
			// Maybe it's a command name in PATH
			if resolved, lookErr := exec.LookPath(execPath); lookErr == nil {
				execPath = resolved
			} else {
				return nil, fmt.Errorf("browser executable not found: %s", execPath)
			}
		}
		opts = append(opts, chromedp.ExecPath(execPath))
		browserName = friendlyBrowserName(execPath)
	} else {
		// Auto-detect: find best available Chromium-based browser
		if found, name := FindBrowser(); found != "" {
			opts = append(opts, chromedp.ExecPath(found))
			browserName = name
		}
		// If not found, let chromedp try its own default detection
	}

	if insecure {
		opts = append(opts, chromedp.Flag("ignore-certificate-errors", true))
	}

	allocCtx, allocCancel := chromedp.NewExecAllocator(ctx, opts...)
	defer allocCancel()

	browserCtx, browserCancel := chromedp.NewContext(allocCtx)
	defer browserCancel()

	timeoutCtx, timeoutCancel := context.WithTimeout(browserCtx, timeout)
	defer timeoutCancel()

	fmt.Fprintf(os.Stderr, "[BROWSER-AUTH] Opening %s for SSO login: %s\n", browserName, targetURL)
	fmt.Fprintf(os.Stderr, "[BROWSER-AUTH] Complete login in the browser window. Timeout: %s\n", timeout)

	// In verbose mode, listen for navigation events to track SAML redirect chain.
	// This logs URL path + host for each redirect hop — never cookie values or SAML assertion bodies.
	// Query parameters are stripped to prevent leaking SAMLRequest/SAMLResponse in redirect-binding flows.
	if verbose {
		chromedp.ListenTarget(timeoutCtx, func(ev any) {
			switch e := ev.(type) {
			case *page.EventFrameNavigated:
				if e.Frame != nil && e.Frame.URL != "" {
					safeURL := sanitizeURLForLog(e.Frame.URL)
					fmt.Fprintf(os.Stderr, "[BROWSER-AUTH] Navigated → %s\n", safeURL)
				}
			case *network.EventResponseReceived:
				if e.Response != nil && e.Response.Status >= 300 && e.Response.Status < 400 {
					safeURL := sanitizeURLForLog(e.Response.URL)
					fmt.Fprintf(os.Stderr, "[BROWSER-AUTH] Redirect %d → %s\n", e.Response.Status, safeURL)
				}
			}
		})
	}

	// Navigate to the target URL (this triggers SSO redirect).
	// SSO flows (Kerberos 401, SAML redirect, etc.) often cause the initial
	// navigation to report ERR_ABORTED or similar — this is expected.
	// The browser stays open and the SSO handshake continues, so we ignore
	// page-load navigation errors and proceed to cookie polling.
	if err := chromedp.Run(timeoutCtx, chromedp.Navigate(targetURL)); err != nil {
		if timeoutCtx.Err() != nil {
			return nil, fmt.Errorf("browser auth timed out after %s — login was not completed", timeout)
		}
		errMsg := err.Error()
		if strings.Contains(errMsg, "executable file not found") ||
			strings.Contains(errMsg, "no such file") ||
			strings.Contains(errMsg, "cannot run") {
			return nil, fmt.Errorf("failed to launch browser: %w\nMake sure a Chromium-based browser is installed (Edge, Chrome, Chromium, Brave)", err)
		}
		// Navigation "error" is normal during SSO (ERR_ABORTED, etc.) — browser is still open
		fmt.Fprintf(os.Stderr, "[BROWSER-AUTH] SSO redirect in progress (this is normal)...\n")
	}

	// Poll for SAP cookies until they appear or timeout.
	// Pass the SAP base URL explicitly so GetCookies works even when
	// the browser navigation ended up as a download or redirect.
	cookies, err := pollForSAPCookies(timeoutCtx, sapURL, verbose)
	if err != nil {
		return nil, err
	}

	fmt.Fprintf(os.Stderr, "[BROWSER-AUTH] Authentication successful! Extracted %d cookies\n", len(cookies))
	if verbose {
		for name := range cookies {
			fmt.Fprintf(os.Stderr, "[BROWSER-AUTH]   cookie: %s\n", name)
		}
	}
	return cookies, nil
}

// samlPollInterval is the cookie polling interval.
// SAML SSO flows involve multi-hop redirects (SAP → IAS → SAP) that can take
// several seconds. A 500ms interval provides responsive detection without
// excessive CDP calls.
const samlPollInterval = 500 * time.Millisecond

// pollForSAPCookies polls the browser for SAP-specific cookies.
// It uses a faster poll interval (500ms) for responsive SAML cookie detection
// and logs each poll cycle in verbose mode for debugging redirect chains.
func pollForSAPCookies(ctx context.Context, sapURL string, verbose bool) (map[string]string, error) {
	ticker := time.NewTicker(samlPollInterval)
	defer ticker.Stop()

	start := time.Now()
	pollCount := 0
	lastCookieCount := -1

	for {
		select {
		case <-ctx.Done():
			elapsed := time.Since(start)
			return nil, fmt.Errorf("browser auth timed out after %s — login was not completed in time", elapsed.Truncate(time.Second))
		case <-ticker.C:
			pollCount++
			cookies, found, err := extractSAPCookies(ctx, sapURL)
			if err != nil {
				if ctx.Err() != nil {
					return nil, fmt.Errorf("browser was closed before authentication completed")
				}
				if verbose {
					fmt.Fprintf(os.Stderr, "[BROWSER-AUTH] Poll #%d (%.1fs): error reading cookies: %v\n",
						pollCount, time.Since(start).Seconds(), err)
				}
				continue
			}

			// Log in verbose mode, but only when cookie count changes or periodically
			if verbose && (len(cookies) != lastCookieCount || pollCount%10 == 0) {
				names := make([]string, 0, len(cookies))
				for name := range cookies {
					names = append(names, name)
				}
				fmt.Fprintf(os.Stderr, "[BROWSER-AUTH] Poll #%d (%.1fs): %d cookies [%s]\n",
					pollCount, time.Since(start).Seconds(), len(cookies), strings.Join(names, ", "))
				lastCookieCount = len(cookies)
			}

			if found {
				if verbose {
					fmt.Fprintf(os.Stderr, "[BROWSER-AUTH] Auth cookies detected after %d polls (%.1fs)\n",
						pollCount, time.Since(start).Seconds())
				}
				return cookies, nil
			}
		}
	}
}

// sanitizeURLForLog returns a URL safe for verbose logging.
// It strips query parameters to prevent leaking SAMLRequest/SAMLResponse
// values that may appear in redirect-binding flows. Returns "scheme://host/path".
func sanitizeURLForLog(rawURL string) string {
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return "(unparseable URL)"
	}
	// Reconstruct without query or fragment — only scheme + host + path
	safe := fmt.Sprintf("%s://%s%s", parsed.Scheme, parsed.Host, parsed.Path)
	return safe
}

// cookieURLsForSAP returns the set of URLs to query for cookies.
// SAML SSO flows often set cookies scoped to specific paths (e.g. /sap/bc/adt/)
// rather than the root domain. Querying multiple URL paths ensures we capture
// cookies regardless of their path scope.
// Uses proper URL parsing to handle sapURL with query/fragment correctly.
func cookieURLsForSAP(sapURL string) []string {
	u, err := url.Parse(sapURL)
	if err != nil || u.Scheme == "" || u.Host == "" {
		return []string{sapURL}
	}
	u.RawQuery = ""
	u.Fragment = ""
	base := *u

	paths := []string{"", "/sap/", "/sap/bc/", "/sap/bc/adt/"}
	urls := make([]string, 0, len(paths))
	for _, p := range paths {
		tmp := base
		tmp.Path = p
		urls = append(urls, tmp.String())
	}
	return urls
}

// extractSAPCookies retrieves all cookies from the browser and checks for SAP auth cookies.
func extractSAPCookies(ctx context.Context, sapURL string) (map[string]string, bool, error) {
	var browserCookies []*network.Cookie

	if err := chromedp.Run(ctx, chromedp.ActionFunc(func(ctx context.Context) error {
		var err error
		// Request cookies for multiple SAP URL paths explicitly.
		// SAML flows may set cookies scoped to /sap/bc/adt/ or /sap/bc/
		// rather than the root, so we query all relevant paths.
		browserCookies, err = network.GetCookies().WithURLs(cookieURLsForSAP(sapURL)).Do(ctx)
		return err
	})); err != nil {
		return nil, false, err
	}

	result := make(map[string]string)
	hasAuthCookie := false

	for _, c := range browserCookies {
		result[c.Name] = c.Value

		// Only strong auth cookies count (MYSAPSSO2, SAP_SESSIONID*).
		// sap-usercontext is set before login completes and is not sufficient.
		for _, prefix := range sapAuthCookieNames {
			if strings.HasPrefix(c.Name, prefix) {
				hasAuthCookie = true
				break
			}
		}
	}

	return result, hasAuthCookie, nil
}

// matchesSAPAuthCookie checks whether a cookie name matches any known SAP auth cookie prefix.
// Exported as a testable helper for unit tests.
func matchesSAPAuthCookie(name string) bool {
	for _, prefix := range sapAuthCookieNames {
		if strings.HasPrefix(name, prefix) {
			return true
		}
	}
	return false
}

// matchesSAPWeakCookie checks whether a cookie name matches any known SAP weak cookie prefix.
func matchesSAPWeakCookie(name string) bool {
	for _, prefix := range sapWeakCookieNames {
		if strings.HasPrefix(name, prefix) {
			return true
		}
	}
	return false
}

// SaveCookiesToFile writes cookies in Netscape cookie file format.
// This allows reuse via --cookie-file on subsequent runs.
func SaveCookiesToFile(cookies map[string]string, sapURL, filePath string) error {
	u, err := url.Parse(sapURL)
	if err != nil {
		return fmt.Errorf("invalid SAP URL: %w", err)
	}

	f, err := os.OpenFile(filePath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0600)
	if err != nil {
		return fmt.Errorf("failed to create cookie file: %w", err)
	}
	defer f.Close()

	fmt.Fprintln(f, "# Netscape HTTP Cookie File")
	fmt.Fprintln(f, "# Generated by vsp --browser-auth")
	fmt.Fprintf(f, "# %s\n", time.Now().Format(time.RFC3339))
	fmt.Fprintln(f)

	domain := u.Hostname()
	secure := "FALSE"
	if u.Scheme == "https" {
		secure = "TRUE"
	}

	expiry := time.Now().Add(24 * time.Hour).Unix()

	for name, value := range cookies {
		fmt.Fprintf(f, "%s\tTRUE\t/\t%s\t%d\t%s\t%s\n", domain, secure, expiry, name, value)
	}

	return nil
}
