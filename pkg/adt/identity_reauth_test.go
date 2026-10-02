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

// One wrong password is one failed logon. The preflight's 401 comes back
// without a CSRF refresh or a retry, and a later request does not try again:
// the fake sees exactly one request however often vsp is asked.
func TestPinPreflight401IsOneFailedLogon(t *testing.T) {
	var mu sync.Mutex
	var seen []string
	sap := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		seen = append(seen, r.Method+" "+r.URL.Path)
		mu.Unlock()
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer sap.Close()

	c := NewClient(sap.URL, "TESTUSER", "wrong", WithClient("001"), WithExpect(mustPin(t, "A4H.001/TESTUSER")))
	for i := 0; i < 3; i++ {
		_, err := c.transport.Request(context.Background(), pinReadPath, nil)
		if err == nil || !strings.Contains(err.Error(), "401") {
			t.Fatalf("request %d: want the 401, got %v", i, err)
		}
	}
	if err := c.CheckSession(context.Background()); err == nil {
		t.Fatal("session check after a refused logon succeeded")
	}
	mu.Lock()
	defer mu.Unlock()
	if len(seen) != 1 {
		t.Errorf("one wrong password must cost one failed logon; the fake saw %d requests: %v", len(seen), seen)
	}
}

// A re-authentication can put another user's session behind the same client.
// The verdict does not survive it: the request retried after the refresh is
// held until the new session has been checked, and refused when it is not the
// pinned user's.
func TestPinIsCheckedAgainAfterReauthentication(t *testing.T) {
	var mu sync.Mutex
	var served []string // data requests answered, by session
	sap := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		session := ""
		if ck, err := r.Cookie("MYSAPSSO2"); err == nil {
			session = ck.Value
		}
		user := map[string]string{"first": "TESTUSER", "second": "OTHERUSER"}[session]
		w.Header().Set("x-csrf-token", "AbCdEfGhIjKlMnOpQrStUv==")
		switch r.URL.Path {
		case systemInformationPath:
			fmt.Fprintf(w, `{"systemID":"A4H","client":"001","userName":%q}`, user)
		case pinReadPath:
			mu.Lock()
			defer mu.Unlock()
			served = append(served, session)
			if session == "first" && len(served) > 1 {
				w.WriteHeader(http.StatusUnauthorized) // the first session expired
				return
			}
			_, _ = w.Write([]byte("ok"))
		default:
			_, _ = w.Write([]byte("ok"))
		}
	}))
	defer sap.Close()

	reauth := func(context.Context) (map[string]string, error) {
		return map[string]string{"MYSAPSSO2": "second"}, nil
	}
	c := NewClient(sap.URL, "", "", WithClient("001"), WithCookies(map[string]string{"MYSAPSSO2": "first"}),
		WithReauthFunc(reauth), WithExpect(mustPin(t, "A4H.001/TESTUSER")))

	if _, err := c.transport.Request(context.Background(), pinReadPath, nil); err != nil {
		t.Fatalf("first session: %v", err)
	}
	_, err := c.transport.Request(context.Background(), pinReadPath, nil)
	if !IsIdentityMismatch(err) || !strings.Contains(err.Error(), "as OTHERUSER") {
		t.Fatalf("the session after re-authentication is another user's; want a refusal, got %v", err)
	}
	mu.Lock()
	defer mu.Unlock()
	for _, s := range served {
		if s == "second" {
			t.Errorf("work was served on the unverified session: %v", served)
		}
	}
}

// Cookies set on the client from outside are a new session too.
func TestPinIsCheckedAgainAfterNewCookies(t *testing.T) {
	user := "TESTUSER"
	var mu sync.Mutex
	sap := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("x-csrf-token", "AbCdEfGhIjKlMnOpQrStUv==")
		if r.URL.Path == systemInformationPath {
			mu.Lock()
			fmt.Fprintf(w, `{"systemID":"A4H","client":"001","userName":%q}`, user)
			mu.Unlock()
			return
		}
		_, _ = w.Write([]byte("ok"))
	}))
	defer sap.Close()
	c := NewClient(sap.URL, "", "", WithClient("001"), WithCookies(map[string]string{"MYSAPSSO2": "a"}),
		WithExpect(mustPin(t, "A4H.001/TESTUSER")))
	if _, err := c.transport.Request(context.Background(), pinReadPath, nil); err != nil {
		t.Fatal(err)
	}
	mu.Lock()
	user = "OTHERUSER"
	mu.Unlock()
	c.transport.SetCookies(map[string]string{"MYSAPSSO2": "b"})
	if _, err := c.transport.Request(context.Background(), pinReadPath, nil); !IsIdentityMismatch(err) {
		t.Fatalf("new cookies of another user: want a refusal, got %v", err)
	}
}

// A ZADT_VSP WebSocket logs on by itself. Built from a pinned client, it is
// not opened until the pin holds: against another system nothing but the
// preflight is sent.
func TestPinGuardsTheWebSocket(t *testing.T) {
	f := newPinFake(t, "B4H", "001", "TESTUSER")
	c := NewClient(f.URL, "TESTUSER", "secret", WithClient("001"), WithExpect(mustPin(t, "A4H.001/TESTUSER")))
	for name, connect := range map[string]func(context.Context) error{
		"debug": c.NewDebugWebSocketClient().Connect,
		"amdp":  c.NewAMDPWebSocketClient().Connect,
	} {
		if err := connect(context.Background()); !IsIdentityMismatch(err) {
			t.Errorf("%s WebSocket: want an identity refusal, got %v", name, err)
		}
	}
	if got := f.requests(); len(got) != 1 || got[0] != "GET "+systemInformationPath {
		t.Errorf("only the preflight may reach a mismatched system; it saw %v", got)
	}
}
