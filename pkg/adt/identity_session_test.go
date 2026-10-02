package adt

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
)

// sessionFake names the user of a session by its MYSAPSSO2 cookie (the first
// one on the request, which is the explicitly configured one) and records what
// it served, and how.
type sessionFake struct {
	*httptest.Server
	users map[string]string // cookie value -> user
	// reissue, when set, is the cookie value the system information answer
	// sets, as SAP does when it opens a new session on a still-valid ticket.
	reissue string
	// reissueEvery sets a fresh session cookie on every answer, as an SSO
	// system that refreshes its session does; anyUser names the user of any
	// session not in users.
	reissueEvery bool
	anyUser      string
	issued       int

	mu       sync.Mutex
	served   []string // "path session"
	upgrades []string // the session or Authorization a WebSocket upgrade carried
	all      []string
}

func newSessionFake(t *testing.T, users map[string]string) *sessionFake {
	t.Helper()
	f := &sessionFake{users: users}
	f.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		session := ""
		if ck, err := r.Cookie("MYSAPSSO2"); err == nil {
			session = ck.Value
		}
		f.mu.Lock()
		defer f.mu.Unlock()
		f.all = append(f.all, r.Method+" "+r.URL.Path)
		w.Header().Set("x-csrf-token", "AbCdEfGhIjKlMnOpQrStUv==")
		if f.reissueEvery {
			f.issued++
			http.SetCookie(w, &http.Cookie{Name: "MYSAPSSO2", Value: fmt.Sprintf("s%d", f.issued), Path: "/"})
		}
		switch r.URL.Path {
		case systemInformationPath:
			user, ok := f.users[session]
			if !ok && f.anyUser != "" {
				user, ok = f.anyUser, true
			}
			if !ok && r.Header.Get("Authorization") != "" {
				user, ok = "TESTUSER", true
			}
			if !ok {
				w.WriteHeader(http.StatusUnauthorized)
				return
			}
			if f.reissue != "" {
				http.SetCookie(w, &http.Cookie{Name: "MYSAPSSO2", Value: f.reissue, Path: "/"})
			}
			fmt.Fprintf(w, `{"systemID":"A4H","client":"001","userName":%q}`, user)
		case "/sap/bc/apc/sap/zadt_vsp":
			if session != "" {
				f.upgrades = append(f.upgrades, "cookie "+session)
			} else {
				f.upgrades = append(f.upgrades, "basic")
			}
			w.WriteHeader(http.StatusUnauthorized)
		default:
			f.served = append(f.served, r.URL.Path+" "+session)
			_, _ = w.Write([]byte("ok"))
		}
	}))
	t.Cleanup(f.Close)
	return f
}

// Out of the threat model: the verified system reissues the session cookie in
// its preflight answer, even as another user's. The pin guards against
// operator misconfiguration, not a hostile server, and what the verified
// server issues is covered by its verdict — so the work proceeds, on the
// cookie the server handed out, without a second preflight.
func TestPinTrustsACookieTheVerifiedServerReissues(t *testing.T) {
	f := newSessionFake(t, map[string]string{"correct": "TESTUSER", "wrong": "OTHERUSER"})
	f.reissue = "wrong"
	c := NewClient(f.URL, "", "", WithClient("001"), WithCookies(map[string]string{"MYSAPSSO2": "correct"}),
		WithExpect(mustPin(t, "A4H.001/TESTUSER")))
	for i := 0; i < 2; i++ {
		if _, err := c.transport.Request(context.Background(), pinReadPath, nil); err != nil {
			t.Fatalf("request %d: %v", i, err)
		}
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	if n := count(f.all, "GET "+systemInformationPath); n != 1 {
		t.Errorf("want one preflight, got %d: %v", n, f.all)
	}
}

// An SSO system that refreshes its cookie on every answer costs one
// preflight, not one (or three) per request.
func TestPinOnePreflightWhenTheServerReissuesEveryAnswer(t *testing.T) {
	f := newSessionFake(t, map[string]string{"s0": "TESTUSER"})
	f.reissueEvery, f.anyUser = true, "TESTUSER"
	c := NewClient(f.URL, "", "", WithClient("001"), WithCookies(map[string]string{"MYSAPSSO2": "s0"}),
		WithExpect(mustPin(t, "A4H.001/TESTUSER")))
	const n = 5
	for i := 0; i < n; i++ {
		if _, err := c.transport.Request(context.Background(), pinReadPath, nil); err != nil {
			t.Fatalf("request %d: %v", i, err)
		}
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	if got := count(f.all, "GET "+systemInformationPath); got != 1 {
		t.Errorf("want exactly one preflight across %d requests, got %d: %v", n, got, f.all)
	}
	if got := count(f.all, "GET "+pinReadPath); got != n {
		t.Errorf("want %d reads, got %d", n, got)
	}
}

func count(list []string, want string) int {
	n := 0
	for _, s := range list {
		if s == want {
			n++
		}
	}
	return n
}

// The request that triggered the preflight was built with the cookie the
// preflight's answer then replaced. It goes out with the current one, not
// the one it was built with.
func TestPinRequestCarriesTheVerifiedCookie(t *testing.T) {
	f := newSessionFake(t, map[string]string{"first": "TESTUSER", "second": "TESTUSER"})
	f.reissue = "second"
	c := NewClient(f.URL, "", "", WithClient("001"), WithCookies(map[string]string{"MYSAPSSO2": "first"}),
		WithExpect(mustPin(t, "A4H.001/TESTUSER")))
	if _, err := c.transport.Request(context.Background(), pinReadPath, nil); err != nil {
		t.Fatal(err)
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	if len(f.served) != 1 || f.served[0] != pinReadPath+" second" {
		t.Errorf("the work must carry the verified session; served %v", f.served)
	}
}

// A WebSocket built from a pinned client logs on with the session that was
// verified when it connects, not the one copied when it was built.
func TestPinWebSocketDialsWithTheVerifiedSession(t *testing.T) {
	f := newSessionFake(t, map[string]string{"old": "TESTUSER", "new": "TESTUSER"})
	c := NewClient(f.URL, "", "", WithClient("001"), WithCookies(map[string]string{"MYSAPSSO2": "old"}),
		WithExpect(mustPin(t, "A4H.001/TESTUSER")))
	ws := c.NewDebugWebSocketClient() // copies "old"
	c.transport.SetCookies(map[string]string{"MYSAPSSO2": "new"})

	_ = ws.Connect(context.Background()) // the fake refuses the upgrade; what it carried is the point
	f.mu.Lock()
	defer f.mu.Unlock()
	if len(f.upgrades) != 1 || f.upgrades[0] != "cookie new" {
		t.Errorf("the upgrade must carry the verified session; it carried %v", f.upgrades)
	}
}

// Under a pin, a WebSocket upgrade answered 401 is final: the password is not
// sent a second time, so a password changed since the preflight costs one
// failed logon, not two.
func TestPinWebSocket401IsNotRetriedWithThePassword(t *testing.T) {
	f := newSessionFake(t, nil)
	c := NewClient(f.URL, "TESTUSER", "secret", WithClient("001"), WithExpect(mustPin(t, "A4H.001/TESTUSER")))
	err := c.NewAMDPWebSocketClient().Connect(context.Background())
	if err == nil || !strings.Contains(err.Error(), "401") {
		t.Fatalf("want the 401, got %v", err)
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	want := []string{"GET " + systemInformationPath, "GET /sap/bc/apc/sap/zadt_vsp"}
	if strings.Join(f.all, "|") != strings.Join(want, "|") {
		t.Errorf("requests:\n got %v\nwant %v (one logon with the password after the preflight)", f.all, want)
	}
}
