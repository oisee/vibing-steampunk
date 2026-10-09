package vsp

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/oisee/vibing-steampunk/pkg/adt"
	"github.com/oisee/vibing-steampunk/pkg/saprfc"
)

// A pinned RFC command whose RFC user contradicts the pin never dials the
// gateway: the system's rfc_user, and an --rfc-user override.
func TestRFCCLIPinRefusesBeforeTheGateway(t *testing.T) {
	t.Run("rfc_user", func(t *testing.T) {
		dials := rfcCLITestEnv(t, false) // rfc_user TESTUSER
		withExpectFlag(t, "")
		t.Setenv("SAP_EXPECT", "A4H.001/OTHERUSER")
		err := rfcInfoCmd.RunE(rfcInfoCmd, nil)
		if !adt.IsIdentityMismatch(err) {
			t.Fatalf("want an identity refusal, got %v", err)
		}
		if n := dials(); n != 0 {
			t.Errorf("the gateway was dialled %d time(s)", n)
		}
	})
	t.Run("--rfc-user", func(t *testing.T) {
		dials := rfcCLITestEnv(t, false)
		restoreCommandFlags(t)
		withExpectFlag(t, "A4H.001/TESTUSER")
		if err := rfcCmd.PersistentFlags().Set("rfc-user", "OTHERUSER"); err != nil {
			t.Fatal(err)
		}
		_ = rfcInfoCmd.InheritedFlags() // as Execute would: the parent's flags reach the command
		err := rfcInfoCmd.RunE(rfcInfoCmd, nil)
		if !adt.IsIdentityMismatch(err) || !strings.Contains(err.Error(), "OTHERUSER") {
			t.Fatalf("want an identity refusal naming the override, got %v", err)
		}
		if n := dials(); n != 0 {
			t.Errorf("the gateway was dialled %d time(s)", n)
		}
	})
}

// The debugger's own stateful ADT session carries the pin like every other
// client built from the same system: a wrong user is refused unsent, and a
// session on another system gets no further than the preflight.
func TestDebuggerTransportCarriesThePin(t *testing.T) {
	var mu sync.Mutex
	var seen []string
	sap := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		seen = append(seen, r.URL.Path)
		mu.Unlock()
		w.Header().Set("x-csrf-token", "AbCdEfGhIjKlMnOpQrStUv==")
		if r.URL.Path == "/sap/bc/adt/core/http/systeminformation" {
			fmt.Fprint(w, `{"systemID":"B4H","client":"001","userName":"TESTUSER"}`)
		}
	}))
	defer sap.Close()
	params := &systemParams{URL: sap.URL, User: "OTHERUSER", Password: "x", Client: "001", Language: "EN",
		Expect: "A4H.001/TESTUSER", ExpectSource: "--expect"}
	if _, err := statefulADTTransport(params, 0); !adt.IsIdentityMismatch(err) {
		t.Fatalf("wrong user: want an identity refusal, got %v", err)
	}

	params.User = "TESTUSER"
	transport, err := statefulADTTransport(params, 0)
	if err != nil {
		t.Fatal(err)
	}
	_, err = transport.Do(context.Background(), saprfc.ADTRequest{Method: "GET", URI: "/sap/bc/adt/debugger/listeners"})
	if !adt.IsIdentityMismatch(err) {
		t.Fatalf("another system: want an identity refusal, got %v", err)
	}
	mu.Lock()
	defer mu.Unlock()
	if len(seen) != 1 {
		t.Errorf("only the preflight may reach another system; it saw %v", seen)
	}
}

// The server checks the pin before its browser, SAML or SSO logon runs: a
// configured client that contradicts it is refused before that flow contacts
// anything.
func TestServerChecksThePinBeforeInteractiveLogon(t *testing.T) {
	var hits atomic.Int32
	sap := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { hits.Add(1) }))
	defer sap.Close()

	dir := t.TempDir()
	t.Chdir(dir)
	t.Setenv("HOME", dir)
	for _, name := range envOnlyCLIVars {
		t.Setenv(name, "")
	}
	t.Setenv("SAP_EXPECT", "")
	restoreCommandFlags(t)
	saved := *cfg
	t.Cleanup(func() { *cfg = saved; rootCmd.SetArgs(nil) })
	withExpectFlag(t, "")

	rootCmd.SetArgs([]string{"--url", sap.URL, "--saml-auth", "--saml-user", "someone@example.invalid",
		"--saml-password", "x", "--client", "100", "--expect", "A4H.001"})
	err := rootCmd.Execute()
	if !adt.IsIdentityMismatch(err) {
		t.Fatalf("want an identity refusal, got %v", err)
	}
	if n := hits.Load(); n != 0 {
		t.Errorf("the SAML logon contacted the system %d time(s) before the refusal", n)
	}
}
