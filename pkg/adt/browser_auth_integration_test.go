//go:build integration

package adt

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/chromedp/chromedp"
)

// TestBrowserAuth_SAMLRedirectChain simulates a SAML-like redirect chain
// with an httptest server and verifies that extractSAPCookies correctly
// captures session cookies after multi-hop redirects.
//
// Run with: go test -tags=integration -run TestBrowserAuth_SAMLRedirectChain -v ./pkg/adt/
//
// Requires a Chromium-based browser installed (Edge, Chrome, Chromium).
func TestBrowserAuth_SAMLRedirectChain(t *testing.T) {
	// Create a test server simulating SAML SSO:
	// GET /sap/bc/adt/ → 302 to /saml/idp (simulates SAP→IAS redirect)
	// GET /saml/idp → 302 to /saml/callback?SAMLResponse=mock (simulates IAS→SAP)
	// GET /saml/callback → sets MYSAPSSO2 + SAP_SESSIONID cookies, returns 200
	mux := http.NewServeMux()

	mux.HandleFunc("/sap/bc/adt/", func(w http.ResponseWriter, r *http.Request) {
		// Step 1: SAP redirects to IAS for SAML authentication
		http.Redirect(w, r, "/saml/idp", http.StatusFound)
	})

	mux.HandleFunc("/saml/idp", func(w http.ResponseWriter, r *http.Request) {
		// Step 2: IAS "authenticates" and redirects back with SAMLResponse
		http.Redirect(w, r, "/saml/callback?SAMLResponse=mock_assertion", http.StatusFound)
	})

	mux.HandleFunc("/saml/callback", func(w http.ResponseWriter, r *http.Request) {
		// Step 3: SAP processes SAMLResponse and sets session cookies
		http.SetCookie(w, &http.Cookie{
			Name:  "MYSAPSSO2",
			Value: "test_sso_token_abc123",
			Path:  "/",
		})
		http.SetCookie(w, &http.Cookie{
			Name:  "SAP_SESSIONID_TST_001",
			Value: "test_session_xyz789",
			Path:  "/sap/bc/",
		})
		http.SetCookie(w, &http.Cookie{
			Name:  "sap-usercontext",
			Value: "sap-client=001",
			Path:  "/sap/",
		})
		w.WriteHeader(http.StatusOK)
		fmt.Fprint(w, "<html><body>ADT Welcome Page</body></html>")
	})

	ts := httptest.NewServer(mux)
	defer ts.Close()

	// Create headless browser context
	opts := append(chromedp.DefaultExecAllocatorOptions[:],
		chromedp.Flag("headless", true),
		chromedp.Flag("disable-gpu", true),
		chromedp.Flag("no-sandbox", true),
	)

	// Use auto-detected browser
	if found, _ := FindBrowser(); found != "" {
		opts = append(opts, chromedp.ExecPath(found))
	}

	allocCtx, allocCancel := chromedp.NewExecAllocator(context.Background(), opts...)
	defer allocCancel()

	browserCtx, browserCancel := chromedp.NewContext(allocCtx)
	defer browserCancel()

	ctx, cancel := context.WithTimeout(browserCtx, 30*time.Second)
	defer cancel()

	// Navigate to the SAP ADT endpoint (triggers redirect chain)
	targetURL := ts.URL + "/sap/bc/adt/"
	if err := chromedp.Run(ctx, chromedp.Navigate(targetURL)); err != nil {
		t.Fatalf("navigation failed: %v", err)
	}

	// Wait briefly for cookies to be set after redirect chain
	time.Sleep(500 * time.Millisecond)

	// Extract cookies using our function
	cookies, hasAuth, err := extractSAPCookies(ctx, ts.URL)
	if err != nil {
		t.Fatalf("extractSAPCookies failed: %v", err)
	}

	// Verify auth cookie detection
	if !hasAuth {
		t.Error("expected hasAuth=true after SAML redirect chain, got false")
		t.Logf("cookies found: %v", cookieNames(cookies))
	}

	// Verify specific cookies
	if _, ok := cookies["MYSAPSSO2"]; !ok {
		t.Error("expected MYSAPSSO2 cookie after SAML authentication")
	}

	// Verify path-scoped cookie is also captured (this is the T1.1 fix)
	if _, ok := cookies["SAP_SESSIONID_TST_001"]; !ok {
		t.Error("expected SAP_SESSIONID_TST_001 cookie (path-scoped to /sap/bc/) — cookieURLsForSAP fix required")
	}

	// Verify weak cookie is present but doesn't affect auth detection
	if _, ok := cookies["sap-usercontext"]; !ok {
		t.Error("expected sap-usercontext cookie to be captured")
	}

	// Verify cookie classification
	for name := range cookies {
		if matchesSAPAuthCookie(name) {
			t.Logf("strong auth cookie: %s", name)
		} else if matchesSAPWeakCookie(name) {
			t.Logf("weak cookie: %s", name)
		} else {
			t.Logf("other cookie: %s", name)
		}
	}
}

// TestBrowserAuth_PollDetectsCookies verifies that pollForSAPCookies
// correctly detects cookies that appear after a delay (simulating slow
// SAML redirect chains).
func TestBrowserAuth_PollDetectsCookies(t *testing.T) {
	// Server that sets auth cookies only on the second request to /check
	requestCount := 0
	mux := http.NewServeMux()

	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		requestCount++
		if requestCount >= 3 {
			// After a few requests, set auth cookies (simulates delayed SAML completion)
			http.SetCookie(w, &http.Cookie{
				Name:  "MYSAPSSO2",
				Value: "delayed_sso_token",
				Path:  "/",
			})
		}
		w.WriteHeader(http.StatusOK)
		fmt.Fprint(w, "<html><body>Loading...</body></html>")
	})

	ts := httptest.NewServer(mux)
	defer ts.Close()

	opts := append(chromedp.DefaultExecAllocatorOptions[:],
		chromedp.Flag("headless", true),
		chromedp.Flag("disable-gpu", true),
		chromedp.Flag("no-sandbox", true),
	)

	if found, _ := FindBrowser(); found != "" {
		opts = append(opts, chromedp.ExecPath(found))
	}

	allocCtx, allocCancel := chromedp.NewExecAllocator(context.Background(), opts...)
	defer allocCancel()

	browserCtx, browserCancel := chromedp.NewContext(allocCtx)
	defer browserCancel()

	ctx, cancel := context.WithTimeout(browserCtx, 15*time.Second)
	defer cancel()

	// Navigate to the test server
	if err := chromedp.Run(ctx, chromedp.Navigate(ts.URL)); err != nil {
		t.Fatalf("navigation failed: %v", err)
	}

	// Poll for cookies — should eventually find MYSAPSSO2
	cookies, err := pollForSAPCookies(ctx, ts.URL, true)
	if err != nil {
		t.Fatalf("pollForSAPCookies failed: %v", err)
	}

	if _, ok := cookies["MYSAPSSO2"]; !ok {
		t.Error("expected MYSAPSSO2 cookie to be found by polling")
	}
}

func cookieNames(cookies map[string]string) []string {
	names := make([]string, 0, len(cookies))
	for name := range cookies {
		names = append(names, name)
	}
	return names
}

