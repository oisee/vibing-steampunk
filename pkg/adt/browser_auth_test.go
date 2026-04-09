package adt

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestSaveCookiesToFile(t *testing.T) {
	cookies := map[string]string{
		"MYSAPSSO2":              "abc123",
		"sap-usercontext":        "sap-client=001",
		"SAP_SESSIONID_NPL_001": "session456",
	}

	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "cookies.txt")

	err := SaveCookiesToFile(cookies, "https://sap.example.com:44300", path)
	if err != nil {
		t.Fatalf("SaveCookiesToFile failed: %v", err)
	}

	// Verify the file was created with restrictive permissions
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("cannot stat cookie file: %v", err)
	}
	if runtime.GOOS != "windows" && info.Mode().Perm() != 0600 {
		t.Errorf("expected permissions 0600, got %o", info.Mode().Perm())
	}

	// Verify roundtrip: saved cookies can be loaded back
	loaded, err := LoadCookiesFromFile(path)
	if err != nil {
		t.Fatalf("LoadCookiesFromFile failed: %v", err)
	}
	if len(loaded) != len(cookies) {
		t.Errorf("expected %d cookies, got %d", len(cookies), len(loaded))
	}
	for name, expected := range cookies {
		if loaded[name] != expected {
			t.Errorf("cookie %s: expected %q, got %q", name, expected, loaded[name])
		}
	}
}

func TestSaveCookiesToFile_HTTPS(t *testing.T) {
	cookies := map[string]string{"test": "value"}
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "cookies.txt")

	err := SaveCookiesToFile(cookies, "https://sap.example.com:44300", path)
	if err != nil {
		t.Fatalf("SaveCookiesToFile failed: %v", err)
	}

	data, _ := os.ReadFile(path)
	content := string(data)
	if !strings.Contains(content, "TRUE\t/\tTRUE") {
		t.Error("expected secure flag TRUE for https URL")
	}
}

func TestSaveCookiesToFile_HTTP(t *testing.T) {
	cookies := map[string]string{"test": "value"}
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "cookies.txt")

	err := SaveCookiesToFile(cookies, "http://sap.example.com:8000", path)
	if err != nil {
		t.Fatalf("SaveCookiesToFile failed: %v", err)
	}

	data, _ := os.ReadFile(path)
	content := string(data)
	if !strings.Contains(content, "TRUE\t/\tFALSE") {
		t.Error("expected secure flag FALSE for http URL")
	}
}

func TestSaveCookiesToFile_InvalidURL(t *testing.T) {
	err := SaveCookiesToFile(map[string]string{"a": "b"}, "://bad", "/tmp/vsp_test_invalid.txt")
	if err == nil {
		t.Error("expected error for invalid URL")
	}
}

func TestBrowserLogin_InvalidURL(t *testing.T) {
	ctx := context.TODO()
	_, err := BrowserLogin(ctx, "", false, 0, "", false)
	if err == nil {
		t.Error("expected error for empty URL")
	}

	_, err = BrowserLogin(ctx, "not-a-url", false, 0, "", false)
	if err == nil {
		t.Error("expected error for invalid URL")
	}
}

// --- T1.4: Cookie filtering unit tests ---

func TestCookieURLsForSAP(t *testing.T) {
	tests := []struct {
		name     string
		sapURL   string
		wantURLs []string
	}{
		{
			name:   "standard HTTPS URL",
			sapURL: "https://my413862.s4hana.cloud.sap",
			wantURLs: []string{
				"https://my413862.s4hana.cloud.sap",
				"https://my413862.s4hana.cloud.sap/sap/",
				"https://my413862.s4hana.cloud.sap/sap/bc/",
				"https://my413862.s4hana.cloud.sap/sap/bc/adt/",
			},
		},
		{
			name:   "URL with trailing slash",
			sapURL: "https://sap.example.com:44300/",
			wantURLs: []string{
				"https://sap.example.com:44300",
				"https://sap.example.com:44300/sap/",
				"https://sap.example.com:44300/sap/bc/",
				"https://sap.example.com:44300/sap/bc/adt/",
			},
		},
		{
			name:   "URL with port no trailing slash",
			sapURL: "https://sap.example.com:44300",
			wantURLs: []string{
				"https://sap.example.com:44300",
				"https://sap.example.com:44300/sap/",
				"https://sap.example.com:44300/sap/bc/",
				"https://sap.example.com:44300/sap/bc/adt/",
			},
		},
		{
			name:   "URL with query params (sap-client) stripped correctly",
			sapURL: "https://sap.example.com:44300?sap-client=100",
			wantURLs: []string{
				"https://sap.example.com:44300",
				"https://sap.example.com:44300/sap/",
				"https://sap.example.com:44300/sap/bc/",
				"https://sap.example.com:44300/sap/bc/adt/",
			},
		},
		{
			name:   "URL with path and fragment stripped",
			sapURL: "https://sap.example.com/some/path#section",
			wantURLs: []string{
				"https://sap.example.com",
				"https://sap.example.com/sap/",
				"https://sap.example.com/sap/bc/",
				"https://sap.example.com/sap/bc/adt/",
			},
		},
		{
			name:     "invalid URL returns input as-is",
			sapURL:   "not-a-url",
			wantURLs: []string{"not-a-url"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := cookieURLsForSAP(tt.sapURL)
			if len(got) != len(tt.wantURLs) {
				t.Fatalf("cookieURLsForSAP(%q) returned %d URLs, want %d\ngot: %v", tt.sapURL, len(got), len(tt.wantURLs), got)
			}
			for i, want := range tt.wantURLs {
				if got[i] != want {
					t.Errorf("cookieURLsForSAP(%q)[%d] = %q, want %q", tt.sapURL, i, got[i], want)
				}
			}
		})
	}
}

func TestMatchesSAPAuthCookie(t *testing.T) {
	tests := []struct {
		name string
		want bool
	}{
		// Strong auth cookies — should match
		{"MYSAPSSO2", true},
		{"SAP_SESSIONID_NPL_001", true},
		{"SAP_SESSIONID", true},
		{"JSESSIONID", true},
		{"JSESSIONID_abc123", true},

		// Weak cookies — should NOT match
		{"sap-usercontext", false},

		// Unrelated cookies — should NOT match
		{"_ga", false},
		{"PHPSESSID", false},
		{"", false},
		{"mysapsso2", false}, // case-sensitive: lowercase should not match
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := matchesSAPAuthCookie(tt.name); got != tt.want {
				t.Errorf("matchesSAPAuthCookie(%q) = %v, want %v", tt.name, got, tt.want)
			}
		})
	}
}

func TestMatchesSAPWeakCookie(t *testing.T) {
	tests := []struct {
		name string
		want bool
	}{
		{"sap-usercontext", true},
		{"sap-usercontext=sap-client=001", true}, // prefix match
		{"MYSAPSSO2", false},
		{"SAP_SESSIONID_NPL_001", false},
		{"", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := matchesSAPWeakCookie(tt.name); got != tt.want {
				t.Errorf("matchesSAPWeakCookie(%q) = %v, want %v", tt.name, got, tt.want)
			}
		})
	}
}

func TestSAPCookieClassification(t *testing.T) {
	// Verify that strong and weak cookie sets are disjoint and comprehensive
	// for the known SAP cookie names.
	knownStrong := []string{"MYSAPSSO2", "SAP_SESSIONID_NPL_001", "JSESSIONID"}
	knownWeak := []string{"sap-usercontext"}

	for _, name := range knownStrong {
		if !matchesSAPAuthCookie(name) {
			t.Errorf("expected %q to be a strong auth cookie", name)
		}
		if matchesSAPWeakCookie(name) {
			t.Errorf("strong cookie %q should not match as weak", name)
		}
	}

	for _, name := range knownWeak {
		if matchesSAPAuthCookie(name) {
			t.Errorf("weak cookie %q should not match as strong auth", name)
		}
		if !matchesSAPWeakCookie(name) {
			t.Errorf("expected %q to be a weak cookie", name)
		}
	}
}

func TestSanitizeURLForLog(t *testing.T) {
	tests := []struct {
		name    string
		rawURL  string
		want    string
	}{
		{
			name:   "strips SAMLResponse query param",
			rawURL: "https://sap.example.com/saml/callback?SAMLResponse=PHNhbWw%3D&RelayState=abc",
			want:   "https://sap.example.com/saml/callback",
		},
		{
			name:   "strips SAMLRequest query param",
			rawURL: "https://ias.example.com/saml2/idp/sso?SAMLRequest=base64data&SigAlg=rsa",
			want:   "https://ias.example.com/saml2/idp/sso",
		},
		{
			name:   "preserves clean URL without query",
			rawURL: "https://sap.example.com/sap/bc/adt/",
			want:   "https://sap.example.com/sap/bc/adt/",
		},
		{
			name:   "strips fragment too",
			rawURL: "https://sap.example.com/page#token=secret",
			want:   "https://sap.example.com/page",
		},
		{
			name:   "handles URL with port",
			rawURL: "https://sap.example.com:44300/sap/bc/?sap-client=001",
			want:   "https://sap.example.com:44300/sap/bc/",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := sanitizeURLForLog(tt.rawURL)
			if got != tt.want {
				t.Errorf("sanitizeURLForLog(%q) = %q, want %q", tt.rawURL, got, tt.want)
			}
		})
	}
}

func TestEmptyCookieJar(t *testing.T) {
	// Verify that unrelated cookie names don't trigger false positives
	unrelatedCookies := []string{
		"_ga", "PHPSESSID", "csrf_token", "__cfduid",
		"connect.sid", "laravel_session", "rack.session",
	}
	for _, name := range unrelatedCookies {
		if matchesSAPAuthCookie(name) {
			t.Errorf("unrelated cookie %q should not match as SAP auth cookie", name)
		}
		if matchesSAPWeakCookie(name) {
			t.Errorf("unrelated cookie %q should not match as SAP weak cookie", name)
		}
	}
}
