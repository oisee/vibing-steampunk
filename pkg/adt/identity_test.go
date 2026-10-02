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

// pinFake is a SAP system that knows who it is. It records every request it
// sees, so a test can say what was sent — and, more to the point, what was not.
type pinFake struct {
	*httptest.Server
	sid, client, user string
	noSysInfo         bool   // answer 404 for the system information resource
	logsys            string // T000-LOGSYS for the fallback

	mu   sync.Mutex
	seen []string
}

func newPinFake(t *testing.T, sid, client, user string) *pinFake {
	t.Helper()
	f := &pinFake{sid: sid, client: client, user: user}
	f.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		f.mu.Lock()
		f.seen = append(f.seen, r.Method+" "+r.URL.Path)
		f.mu.Unlock()
		w.Header().Set("x-csrf-token", "AbCdEfGhIjKlMnOpQrStUv==")
		switch r.URL.Path {
		case systemInformationPath:
			if f.noSysInfo {
				http.NotFound(w, r)
				return
			}
			w.Header().Set("Content-Type", "application/vnd.sap.adt.core.http.systeminformation.v1+json")
			fmt.Fprintf(w, `{"systemID":%q,"client":%q,"userName":%q,"userFullName":"Some One","language":"EN"}`, f.sid, f.client, f.user)
		case "/sap/bc/adt/datapreview/freestyle":
			_, _ = w.Write([]byte(previewXML([]string{"MANDT", "LOGSYS"}, []string{f.client, f.logsys})))
		case "/sap/bc/adt/cts/transportrequests":
			fmt.Fprintf(w, `<tm:root xmlns:tm="http://www.sap.com/cts/adt/tm" tm:name=%q/>`, f.user)
		default:
			_, _ = w.Write([]byte("ok"))
		}
	}))
	t.Cleanup(f.Close)
	return f
}

func (f *pinFake) requests() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]string(nil), f.seen...)
}

func mustPin(t *testing.T, s string) IdentityPin {
	t.Helper()
	p, err := ParseIdentityPin(s)
	if err != nil {
		t.Fatal(err)
	}
	return *p
}

const pinReadPath = "/sap/bc/adt/programs/programs/ZPIN/source/main"

func TestParseIdentityPin(t *testing.T) {
	good := map[string]IdentityPin{
		"A4H.001/TESTUSER": {"A4H", "001", "TESTUSER"},
		"a4h.001/testuser": {"A4H", "001", "TESTUSER"},
		"A4H.001":          {"A4H", "001", ""},
		"A4H":              {"A4H", "", ""},
		"A4H/DEVELOPER":    {"A4H", "", "DEVELOPER"},
		" s4d.100 ":        {"S4D", "100", ""},
	}
	for in, want := range good {
		got, err := ParseIdentityPin(in)
		if err != nil || *got != want {
			t.Errorf("%q: got %+v, %v; want %+v", in, got, err, want)
		}
	}
	for _, in := range []string{"", "A4", "A4HX", "4AH", "A4H.01", "A4H.0011", "A4H.abc", "A4H/", "A4H./X", "A4H.001/A/B"} {
		if _, err := ParseIdentityPin(in); err == nil {
			t.Errorf("%q: accepted", in)
		}
	}
	if s := mustPin(t, "a4h.001/testuser").String(); s != "A4H.001/TESTUSER" {
		t.Errorf("String() = %q", s)
	}
}

// A pin that matches costs one request, once, and everything proceeds.
func TestPinMatchLetsRequestsThrough(t *testing.T) {
	f := newPinFake(t, "A4H", "001", "DEVELOPER")
	c := NewClient(f.URL, "developer", "secret", WithClient("001"), WithExpect(mustPin(t, "a4h.001/developer")))

	for i := 0; i < 2; i++ {
		if _, err := c.transport.Request(context.Background(), pinReadPath, nil); err != nil {
			t.Fatalf("request %d: %v", i, err)
		}
	}
	want := []string{"GET " + systemInformationPath, "GET " + pinReadPath, "GET " + pinReadPath}
	if got := f.requests(); strings.Join(got, "|") != strings.Join(want, "|") {
		t.Errorf("requests:\n got %v\nwant %v", got, want)
	}
	if _, verified, refused := c.IdentityStatus(); verified == nil || refused != nil {
		t.Errorf("status: verified %v, refused %v", verified, refused)
	}
}

// Another system: the preflight is the only thing that ever reaches it. The
// read that triggered it, and a write after it, are refused unsent.
func TestPinSIDMismatchRefusesAndSendsNothingMore(t *testing.T) {
	f := newPinFake(t, "B4H", "001", "DEVELOPER")
	c := NewClient(f.URL, "DEVELOPER", "secret", WithClient("001"), WithExpect(mustPin(t, "A4H.001/DEVELOPER")))

	_, err := c.transport.Request(context.Background(), pinReadPath, nil)
	if !IsIdentityMismatch(err) {
		t.Fatalf("want an identity refusal, got %v", err)
	}
	if !strings.Contains(err.Error(), "connected to B4H.001 as DEVELOPER, expected A4H.001/DEVELOPER") {
		t.Errorf("refusal does not say where it is and where it should be: %v", err)
	}
	_, err = c.transport.Request(context.Background(), "/sap/bc/adt/activation", &RequestOptions{Method: http.MethodPost, Body: []byte("<x/>")})
	if !IsIdentityMismatch(err) {
		t.Fatalf("a write after the refusal: want an identity refusal, got %v", err)
	}
	if err := c.CheckSession(context.Background()); !IsIdentityMismatch(err) {
		t.Fatalf("the session check after the refusal: want an identity refusal, got %v", err)
	}
	if got := f.requests(); len(got) != 1 || got[0] != "GET "+systemInformationPath {
		t.Errorf("only the preflight may reach a mismatched system; it saw %v", got)
	}
}

// The configured user is not the pinned one: refused before a single request,
// so the wrong user's password never reaches SAP and never counts as a failed
// logon.
func TestPinUserMismatchFromConfigSendsNoRequestAtAll(t *testing.T) {
	f := newPinFake(t, "A4H", "001", "TESTUSER")
	c := NewClient(f.URL, "otheruser", "secret", WithClient("001"), WithExpect(mustPin(t, "A4H.001/TESTUSER")))

	_, err := c.transport.Request(context.Background(), pinReadPath, nil)
	if !IsIdentityMismatch(err) {
		t.Fatalf("want an identity refusal, got %v", err)
	}
	if !strings.Contains(err.Error(), "OTHERUSER") || !strings.Contains(err.Error(), "no logon was attempted") {
		t.Errorf("refusal does not name the configured user: %v", err)
	}
	if err := c.CheckSession(context.Background()); !IsIdentityMismatch(err) {
		t.Fatalf("session check: want an identity refusal, got %v", err)
	}
	if got := f.requests(); len(got) != 0 {
		t.Errorf("the fake must see nothing; it saw %v", got)
	}
}

// The system answers in another client than the pin's.
func TestPinClientMismatchIsRefused(t *testing.T) {
	f := newPinFake(t, "A4H", "100", "DEVELOPER")
	c := NewClient(f.URL, "DEVELOPER", "secret", WithClient("001"), WithExpect(mustPin(t, "A4H.001")))
	_, err := c.transport.Request(context.Background(), pinReadPath, nil)
	if !IsIdentityMismatch(err) || !strings.Contains(err.Error(), "connected to A4H.100") {
		t.Fatalf("want a client refusal, got %v", err)
	}
	if got := f.requests(); len(got) != 1 {
		t.Errorf("only the preflight may be sent; saw %v", got)
	}
}

// A configured client that differs from the pin is refused without asking.
func TestPinConfiguredClientMismatchSendsNothing(t *testing.T) {
	f := newPinFake(t, "A4H", "100", "DEVELOPER")
	c := NewClient(f.URL, "DEVELOPER", "secret", WithClient("100"), WithExpect(mustPin(t, "A4H.001")))
	if _, err := c.transport.Request(context.Background(), pinReadPath, nil); !IsIdentityMismatch(err) {
		t.Fatalf("want a refusal, got %v", err)
	}
	if got := f.requests(); len(got) != 0 {
		t.Errorf("the fake must see nothing; it saw %v", got)
	}
}

// No pin, no preflight: behaviour is exactly what it was.
func TestNoPinNoPreflight(t *testing.T) {
	f := newPinFake(t, "A4H", "001", "DEVELOPER")
	c := NewClient(f.URL, "DEVELOPER", "secret", WithClient("001"))
	if _, err := c.transport.Request(context.Background(), pinReadPath, nil); err != nil {
		t.Fatal(err)
	}
	if err := c.VerifyIdentity(context.Background()); err != nil {
		t.Fatal(err)
	}
	if got := f.requests(); len(got) != 1 || got[0] != "GET "+pinReadPath {
		t.Errorf("want just the read; saw %v", got)
	}
	if pin, _, _ := c.IdentityStatus(); pin != nil {
		t.Errorf("no pin configured, status reports %v", pin)
	}
}

// A cookie session has no user name until the system names it: the user is
// compared after the preflight, not before.
func TestPinCookieSessionComparesTheReportedUser(t *testing.T) {
	f := newPinFake(t, "A4H", "001", "SOMEONE_ELSE")
	c := NewClient(f.URL, "", "", WithClient("001"), WithCookies(map[string]string{"MYSAPSSO2": "x"}),
		WithExpect(mustPin(t, "A4H.001/TESTUSER")))
	_, err := c.transport.Request(context.Background(), pinReadPath, nil)
	if !IsIdentityMismatch(err) || !strings.Contains(err.Error(), "as SOMEONE_ELSE") {
		t.Fatalf("want a user refusal, got %v", err)
	}
	if got := f.requests(); len(got) != 1 {
		t.Errorf("only the preflight may be sent; saw %v", got)
	}
}

// A release without the system information resource: the SID comes from
// T000's logical system name and a cookie session's user from the transport
// organizer, as SAP(info) and the debugger already find them.
func TestPinFallsBackWhereSystemInformationIsMissing(t *testing.T) {
	f := newPinFake(t, "A4H", "001", "TESTUSER")
	f.noSysInfo, f.logsys = true, "A4HCLNT001"
	c := NewClient(f.URL, "", "", WithClient("001"), WithCookies(map[string]string{"MYSAPSSO2": "x"}),
		WithExpect(mustPin(t, "A4H.001/TESTUSER")))
	if _, err := c.transport.Request(context.Background(), pinReadPath, nil); err != nil {
		t.Fatalf("matching identity through the fallback: %v (requests %v)", err, f.requests())
	}

	g := newPinFake(t, "", "001", "TESTUSER")
	g.noSysInfo, g.logsys = true, "B4HCLNT001"
	c = NewClient(g.URL, "TESTUSER", "secret", WithClient("001"), WithExpect(mustPin(t, "A4H.001/TESTUSER")))
	if _, err := c.transport.Request(context.Background(), pinReadPath, nil); !IsIdentityMismatch(err) {
		t.Fatalf("want a SID refusal through the fallback, got %v", err)
	}

	// A logical system name that does not follow <SID>CLNT<client> proves
	// nothing, and an unproven pin is not kept.
	h := newPinFake(t, "", "001", "TESTUSER")
	h.noSysInfo, h.logsys = true, "ERPDEV"
	c = NewClient(h.URL, "TESTUSER", "secret", WithClient("001"), WithExpect(mustPin(t, "A4H")))
	if _, err := c.transport.Request(context.Background(), pinReadPath, nil); !IsIdentityMismatch(err) ||
		!strings.Contains(err.Error(), "could not confirm the system ID") {
		t.Fatalf("want an unconfirmed-SID refusal, got %v", err)
	}
}
