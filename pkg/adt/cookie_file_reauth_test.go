package adt

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
)

func writeTestCookieFile(t *testing.T, path, value string) {
	t.Helper()
	if err := os.WriteFile(path, []byte("SAP_SESSION="+value+"\n"), 0600); err != nil {
		t.Fatalf("write cookie file: %v", err)
	}
}

func cookieFileTransport(t *testing.T, baseURL, cookieFile string, opts ...Option) *Transport {
	t.Helper()
	cookies, err := LoadCookiesFromFile(cookieFile)
	if err != nil {
		t.Fatalf("LoadCookiesFromFile: %v", err)
	}
	reauth, err := NewCookieFileReauthFunc(cookieFile)
	if err != nil {
		t.Fatalf("NewCookieFileReauthFunc: %v", err)
	}
	all := append([]Option{WithCookies(cookies), WithReauthFunc(reauth), WithReadOnlyReauth()}, opts...)
	return NewTransport(NewConfig(baseURL, "", "", all...))
}

func sessionCookie(r *http.Request) string {
	c, err := r.Cookie("SAP_SESSION")
	if err != nil {
		return ""
	}
	return c.Value
}

func TestCookieFileReauth_RecoversOnlySafeReadsAndInvalidatesCache(t *testing.T) {
	dir := t.TempDir()
	cookieFile := filepath.Join(dir, "cookies.txt")
	writeTestCookieFile(t, cookieFile, "A")

	var cachedCalls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/sap/bc/adt/core/discovery":
			if got := sessionCookie(r); got != "B" {
				http.Error(w, "expected refreshed cookie", http.StatusUnauthorized)
				return
			}
			w.Header().Set("X-CSRF-Token", "csrf-B")
			w.WriteHeader(http.StatusOK)
		case "/cached":
			cachedCalls.Add(1)
			_, _ = w.Write([]byte(sessionCookie(r)))
		case "/safe":
			if sessionCookie(r) != "B" {
				http.Error(w, "expired", http.StatusUnauthorized)
				return
			}
			_, _ = w.Write([]byte("recovered"))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	transport := cookieFileTransport(t, server.URL, cookieFile, WithCache(0))
	first, err := transport.Request(context.Background(), "/cached", nil)
	if err != nil || string(first.Body) != "A" {
		t.Fatalf("cached read before expiry = %q, %v", first.Body, err)
	}
	writeTestCookieFile(t, cookieFile, "B")

	resp, err := transport.Request(context.Background(), "/safe", nil)
	if err != nil {
		t.Fatalf("safe read did not recover: %v", err)
	}
	if got := string(resp.Body); got != "recovered" {
		t.Errorf("safe read = %q, want recovered", got)
	}
	if got := transport.getCSRFToken(); got != "csrf-B" {
		t.Errorf("CSRF token = %q, want csrf-B", got)
	}

	second, err := transport.Request(context.Background(), "/cached", nil)
	if err != nil || string(second.Body) != "B" {
		t.Fatalf("cached read after recovery = %q, %v", second.Body, err)
	}
	if got := cachedCalls.Load(); got != 2 {
		t.Errorf("cached endpoint calls = %d, want 2 after session cache invalidation", got)
	}
}

func TestCookieFileReauth_ConcurrentSafeReadsMergeOneReload(t *testing.T) {
	dir := t.TempDir()
	cookieFile := filepath.Join(dir, "cookies.txt")
	writeTestCookieFile(t, cookieFile, "B")

	const readers = 12
	var staleArrived atomic.Int32
	allStale := make(chan struct{})
	release := make(chan struct{})
	var discoveryCalls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/safe":
			if sessionCookie(r) == "B" {
				_, _ = w.Write([]byte("ok"))
				return
			}
			if staleArrived.Add(1) == readers {
				close(allStale)
			}
			<-release
			http.Error(w, "expired", http.StatusUnauthorized)
		case "/sap/bc/adt/core/discovery":
			discoveryCalls.Add(1)
			w.Header().Set("X-CSRF-Token", "csrf-B")
			w.WriteHeader(http.StatusOK)
		}
	}))
	defer server.Close()

	transport := cookieFileTransport(t, server.URL, cookieFile)
	transport.SetCookies(map[string]string{"SAP_SESSION": "A"})
	var wg sync.WaitGroup
	errs := make(chan error, readers)
	for range readers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			resp, err := transport.Request(context.Background(), "/safe", nil)
			if err == nil && string(resp.Body) != "ok" {
				err = fmt.Errorf("response body = %q", resp.Body)
			}
			errs <- err
		}()
	}
	<-allStale
	close(release)
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Errorf("safe read: %v", err)
		}
	}
	if got := discoveryCalls.Load(); got != 1 {
		t.Errorf("recovery CSRF probes = %d, want one merged reload", got)
	}
}

func TestCookieFileReauth_StopsAfterOneRetry(t *testing.T) {
	dir := t.TempDir()
	cookieFile := filepath.Join(dir, "cookies.txt")
	writeTestCookieFile(t, cookieFile, "B")

	var reads atomic.Int32
	var discovery atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/safe":
			reads.Add(1)
			http.Error(w, "still expired", http.StatusUnauthorized)
		case "/sap/bc/adt/core/discovery":
			discovery.Add(1)
			w.Header().Set("X-CSRF-Token", "csrf-B")
			w.WriteHeader(http.StatusOK)
		}
	}))
	defer server.Close()

	transport := cookieFileTransport(t, server.URL, cookieFile)
	transport.SetCookies(map[string]string{"SAP_SESSION": "A"})
	_, err := transport.Request(context.Background(), "/safe", nil)
	if err == nil {
		t.Fatal("read that remains expired must fail")
	}
	if got := reads.Load(); got != 2 {
		t.Errorf("read attempts = %d, want original plus one retry", got)
	}
	if got := discovery.Load(); got != 1 {
		t.Errorf("recovery probes = %d, want one", got)
	}
}

func TestCookieFileReauth_RejectsInvalidRefreshWithoutReplacingOldCookies(t *testing.T) {
	for _, tc := range []struct {
		name     string
		contents *string
		remove   bool
	}{
		{name: "empty", contents: ptr("")},
		{name: "malformed", contents: ptr("not a cookie\n")},
		{name: "partial Netscape record", contents: ptr("example\tFALSE\t/\tTRUE\t1\tSAP_SESSION\n")},
		{name: "missing", remove: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			cookieFile := filepath.Join(dir, "cookies.txt")
			writeTestCookieFile(t, cookieFile, "A")
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				http.Error(w, "expired", http.StatusUnauthorized)
			}))
			defer server.Close()

			transport := cookieFileTransport(t, server.URL, cookieFile)
			if tc.remove {
				if err := os.Remove(cookieFile); err != nil {
					t.Fatal(err)
				}
			} else if err := os.WriteFile(cookieFile, []byte(*tc.contents), 0600); err != nil {
				t.Fatal(err)
			}
			_, err := transport.Request(context.Background(), "/safe", nil)
			if err == nil || !strings.Contains(err.Error(), "re-authenticating after 401") {
				t.Fatalf("error = %v, want failed cookie-file recovery", err)
			}
			if got := transport.CurrentCookies(); got["SAP_SESSION"] != "A" {
				t.Errorf("cookies after rejected reload = %v, want old A session", got)
			}
		})
	}
}

func TestCookieFileReauth_DoesNotReplayWritesOrLockWindows(t *testing.T) {
	dir := t.TempDir()
	cookieFile := filepath.Join(dir, "cookies.txt")
	writeTestCookieFile(t, cookieFile, "B")

	var writes atomic.Int32
	var sawRefreshedCookie atomic.Bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if sessionCookie(r) == "B" {
			sawRefreshedCookie.Store(true)
		}
		switch r.URL.Path {
		case "/sap/bc/adt/core/discovery":
			w.Header().Set("X-CSRF-Token", "csrf-A")
			w.WriteHeader(http.StatusOK)
		case "/write":
			writes.Add(1)
			http.Error(w, "expired", http.StatusUnauthorized)
		case "/locked":
			http.Error(w, "expired", http.StatusUnauthorized)
		}
	}))
	defer server.Close()

	transport := cookieFileTransport(t, server.URL, cookieFile)
	transport.SetCookies(map[string]string{"SAP_SESSION": "A"})
	_, err := transport.Request(context.Background(), "/write", &RequestOptions{Method: http.MethodPost, Body: []byte("mutate")})
	if err == nil || !strings.Contains(err.Error(), "remote result is unknown") {
		t.Fatalf("write error = %v, want result-unknown refusal", err)
	}
	if got := writes.Load(); got != 1 {
		t.Errorf("write attempts = %d, want one", got)
	}
	_, err = transport.Request(context.Background(), "/locked", &RequestOptions{Method: http.MethodGet, Stateful: true})
	if err == nil || !strings.Contains(err.Error(), "remote result is unknown") {
		t.Fatalf("locked read error = %v, want result-unknown refusal", err)
	}
	if sawRefreshedCookie.Load() {
		t.Error("cookie-file refresh must not run for a write or lock window")
	}
}

func TestReadOnlyReauthDoesNotChangeOtherCredentialSourceReplayPolicy(t *testing.T) {
	var writes atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/sap/bc/adt/core/discovery":
			w.Header().Set("X-CSRF-Token", "csrf")
			w.WriteHeader(http.StatusOK)
		case "/write":
			if writes.Add(1) == 1 {
				http.Error(w, "expired", http.StatusUnauthorized)
				return
			}
			if sessionCookie(r) != "B" {
				http.Error(w, "missing normal reauth", http.StatusUnauthorized)
				return
			}
			w.WriteHeader(http.StatusOK)
		}
	}))
	defer server.Close()

	transport := NewTransport(NewConfig(server.URL, "", "",
		WithCookies(map[string]string{"SAP_SESSION": "A"}),
		WithReauthFunc(func(context.Context) (map[string]string, error) {
			return map[string]string{"SAP_SESSION": "B"}, nil
		}),
	))
	if _, err := transport.Request(context.Background(), "/write", &RequestOptions{Method: http.MethodPost}); err != nil {
		t.Fatalf("non-file credential source must preserve its existing retry policy: %v", err)
	}
	if got := writes.Load(); got != 2 {
		t.Errorf("write attempts = %d, want legacy retry", got)
	}
}

func ptr(s string) *string { return &s }
