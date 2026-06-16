package adt

import (
	"net/http"
	"testing"
	"time"
)

func TestNewConfig(t *testing.T) {
	tests := []struct {
		name     string
		baseURL  string
		username string
		password string
		opts     []Option
		want     *Config
	}{
		{
			name:     "default config",
			baseURL:  "https://sap.example.com:44300",
			username: "testuser",
			password: "testpass",
			opts:     nil,
			want: &Config{
				BaseURL:     "https://sap.example.com:44300",
				Username:    "testuser",
				Password:    "testpass",
				Client:      "001",
				Language:    "EN",
				SessionType: SessionStateless,
				Timeout:     60 * time.Second,
			},
		},
		{
			name:     "with custom client",
			baseURL:  "https://sap.example.com:44300",
			username: "testuser",
			password: "testpass",
			opts:     []Option{WithClient("100")},
			want: &Config{
				BaseURL:     "https://sap.example.com:44300",
				Username:    "testuser",
				Password:    "testpass",
				Client:      "100",
				Language:    "EN",
				SessionType: SessionStateless,
				Timeout:     60 * time.Second,
			},
		},
		{
			name:     "with custom language",
			baseURL:  "https://sap.example.com:44300",
			username: "testuser",
			password: "testpass",
			opts:     []Option{WithLanguage("DE")},
			want: &Config{
				BaseURL:     "https://sap.example.com:44300",
				Username:    "testuser",
				Password:    "testpass",
				Client:      "001",
				Language:    "DE",
				SessionType: SessionStateless,
				Timeout:     60 * time.Second,
			},
		},
		{
			name:     "with insecure skip verify",
			baseURL:  "https://sap.example.com:44300",
			username: "testuser",
			password: "testpass",
			opts:     []Option{WithInsecureSkipVerify()},
			want: &Config{
				BaseURL:            "https://sap.example.com:44300",
				Username:           "testuser",
				Password:           "testpass",
				Client:             "001",
				Language:           "EN",
				InsecureSkipVerify: true,
				SessionType:        SessionStateless,
				Timeout:            60 * time.Second,
			},
		},
		{
			name:     "with stateful session (explicit)",
			baseURL:  "https://sap.example.com:44300",
			username: "testuser",
			password: "testpass",
			opts:     []Option{WithSessionType(SessionStateful)},
			want: &Config{
				BaseURL:     "https://sap.example.com:44300",
				Username:    "testuser",
				Password:    "testpass",
				Client:      "001",
				Language:    "EN",
				SessionType: SessionStateful,
				Timeout:     60 * time.Second,
			},
		},
		{
			name:     "with custom timeout",
			baseURL:  "https://sap.example.com:44300",
			username: "testuser",
			password: "testpass",
			opts:     []Option{WithTimeout(60 * time.Second)},
			want: &Config{
				BaseURL:     "https://sap.example.com:44300",
				Username:    "testuser",
				Password:    "testpass",
				Client:      "001",
				Language:    "EN",
				SessionType: SessionStateless,
				Timeout:     60 * time.Second,
			},
		},
		{
			name:     "with multiple options",
			baseURL:  "https://sap.example.com:44300",
			username: "testuser",
			password: "testpass",
			opts: []Option{
				WithClient("200"),
				WithLanguage("FR"),
				WithInsecureSkipVerify(),
				WithTimeout(120 * time.Second),
			},
			want: &Config{
				BaseURL:            "https://sap.example.com:44300",
				Username:           "testuser",
				Password:           "testpass",
				Client:             "200",
				Language:           "FR",
				InsecureSkipVerify: true,
				SessionType:        SessionStateless,
				Timeout:            120 * time.Second,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := NewConfig(tt.baseURL, tt.username, tt.password, tt.opts...)

			if got.BaseURL != tt.want.BaseURL {
				t.Errorf("BaseURL = %v, want %v", got.BaseURL, tt.want.BaseURL)
			}
			if got.Username != tt.want.Username {
				t.Errorf("Username = %v, want %v", got.Username, tt.want.Username)
			}
			if got.Password != tt.want.Password {
				t.Errorf("Password = %v, want %v", got.Password, tt.want.Password)
			}
			if got.Client != tt.want.Client {
				t.Errorf("Client = %v, want %v", got.Client, tt.want.Client)
			}
			if got.Language != tt.want.Language {
				t.Errorf("Language = %v, want %v", got.Language, tt.want.Language)
			}
			if got.InsecureSkipVerify != tt.want.InsecureSkipVerify {
				t.Errorf("InsecureSkipVerify = %v, want %v", got.InsecureSkipVerify, tt.want.InsecureSkipVerify)
			}
			if got.SessionType != tt.want.SessionType {
				t.Errorf("SessionType = %v, want %v", got.SessionType, tt.want.SessionType)
			}
			if got.Timeout != tt.want.Timeout {
				t.Errorf("Timeout = %v, want %v", got.Timeout, tt.want.Timeout)
			}
		})
	}
}

func TestNewHTTPClient(t *testing.T) {
	cfg := NewConfig("https://sap.example.com:44300", "user", "pass")
	client := cfg.NewHTTPClient()

	if client == nil {
		t.Error("NewHTTPClient returned nil")
	}
	if client.Jar == nil {
		t.Error("HTTP client should have cookie jar")
	}
	if client.Timeout != cfg.Timeout {
		t.Errorf("HTTP client timeout = %v, want %v", client.Timeout, cfg.Timeout)
	}

	// Verify transport has proxy configured (fixes #13)
	transport, ok := client.Transport.(*http.Transport)
	if !ok {
		t.Error("HTTP client transport should be *http.Transport")
	} else if transport.Proxy == nil {
		t.Error("HTTP transport should have Proxy function set (for HTTP_PROXY/HTTPS_PROXY support)")
	}
}

func TestSessionTypes(t *testing.T) {
	if SessionStateful != "stateful" {
		t.Errorf("SessionStateful = %v, want stateful", SessionStateful)
	}
	if SessionStateless != "stateless" {
		t.Errorf("SessionStateless = %v, want stateless", SessionStateless)
	}
	if SessionKeep != "keep" {
		t.Errorf("SessionKeep = %v, want keep", SessionKeep)
	}
}

// TestNewHTTPClient_CheckRedirectPreservesADTHeaders verifies that the
// HTTP client's CheckRedirect callback re-sets headers that are
// load-bearing for the ADT lock→write→unlock sequence across redirects:
//   - Authorization: Go strips this by default on cross-origin redirects
//     (sensitive header per RFC 7235). Without restoration, SAML flows get
//     401 even when curl works (issue #90).
//   - X-CSRF-Token: mutation requests are rejected as CSRF-violating if
//     this disappears mid-sequence.
//   - X-sap-adt-sessiontype: the lock handle is bound to a stateful
//     session; if a redirect hop lands stateless, the subsequent PUT
//     can't find the lock and gets HTTP 423.
//
// Go's default forwards custom headers on same-origin redirects, but this
// test pins the behaviour explicitly so future refactors can't silently
// drop them.
func TestNewHTTPClient_CheckRedirectPreservesADTHeaders(t *testing.T) {
	cfg := NewConfig("https://sap.example.com:44300", "user", "pass")
	client := cfg.NewHTTPClient()

	if client.CheckRedirect == nil {
		t.Fatal("CheckRedirect must be set")
	}

	// Build the "via" chain: the original request that carries the ADT
	// headers we expect to be re-set on the redirect target.
	orig, err := http.NewRequest(http.MethodPost,
		"https://sap.example.com:44300/sap/bc/adt/oo/classes/ZFOO?_action=LOCK", nil)
	if err != nil {
		t.Fatalf("NewRequest(orig): %v", err)
	}
	orig.Header.Set("Authorization", "Basic dXNlcjpwYXNz")
	orig.Header.Set("X-CSRF-Token", "ABC123==")
	orig.Header.Set("X-sap-adt-sessiontype", "stateful")

	// The redirect target that Go would follow — initially without any
	// of the ADT headers (simulating Go having stripped them cross-origin).
	next, err := http.NewRequest(http.MethodPost,
		"https://sap.example.com:44300/sap/bc/adt/follow", nil)
	if err != nil {
		t.Fatalf("NewRequest(next): %v", err)
	}

	if err := client.CheckRedirect(next, []*http.Request{orig}); err != nil {
		t.Fatalf("CheckRedirect returned error: %v", err)
	}

	if got := next.Header.Get("Authorization"); got != "Basic dXNlcjpwYXNz" {
		t.Errorf("Authorization = %q, want preserved from initial request (issue #90)", got)
	}
	if got := next.Header.Get("X-CSRF-Token"); got != "ABC123==" {
		t.Errorf("X-CSRF-Token = %q, want preserved — mutation requests need it to survive redirect hops", got)
	}
	if got := next.Header.Get("X-sap-adt-sessiontype"); got != "stateful" {
		t.Errorf("X-sap-adt-sessiontype = %q, want preserved — lock handles are bound to a stateful session (issue #88)", got)
	}
}

// TestNewHTTPClient_CheckRedirectHonoursLimit guards the 10-redirect cap
// that CheckRedirect enforces to prevent infinite loops.
func TestNewHTTPClient_CheckRedirectHonoursLimit(t *testing.T) {
	cfg := NewConfig("https://sap.example.com:44300", "user", "pass")
	client := cfg.NewHTTPClient()

	if client.CheckRedirect == nil {
		t.Fatal("CheckRedirect must be set")
	}

	next, _ := http.NewRequest(http.MethodGet, "https://sap.example.com:44300/", nil)
	via := make([]*http.Request, 10)
	for i := range via {
		via[i], _ = http.NewRequest(http.MethodGet, "https://sap.example.com:44300/", nil)
	}

	if err := client.CheckRedirect(next, via); err == nil {
		t.Error("CheckRedirect must return an error on the 10th redirect")
	}
}
