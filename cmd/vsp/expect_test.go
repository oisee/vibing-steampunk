package main

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/oisee/vibing-steampunk/internal/mcp"
	"github.com/oisee/vibing-steampunk/pkg/config"
)

func withExpectFlag(t *testing.T, v string) {
	t.Helper()
	old := expectFlag
	expectFlag = v
	t.Cleanup(func() { expectFlag = old })
}

// The server's pin: --expect, else SAP_EXPECT, else the system's "expect".
func TestServerPinPrecedence(t *testing.T) {
	systems := &config.SystemsConfig{Default: "a4h", Systems: map[string]config.SystemConfig{
		"a4h": {URL: "http://a4h", Expect: "A4H.001/FROMCONFIG"},
		"b4h": {URL: "http://b4h", Expect: "B4H"},
	}}
	withExpectFlag(t, "")
	t.Setenv("SAP_EXPECT", "")
	if got := resolveServerExpect("", systems); got.Raw != "A4H.001/FROMCONFIG" || !strings.Contains(got.Source, `"a4h"`) {
		t.Errorf("default system: %+v", got)
	}
	if got := resolveServerExpect("b4h", systems); got.Raw != "B4H" {
		t.Errorf("named system: %+v", got)
	}
	t.Setenv("SAP_EXPECT", "A4H/FROMENV")
	if got := resolveServerExpect("", systems); got.Raw != "A4H/FROMENV" {
		t.Errorf("env: %+v", got)
	}
	withExpectFlag(t, "A4H/FROMFLAG")
	if got := resolveServerExpect("", systems); got.Raw != "A4H/FROMFLAG" || got.Source != "--expect" {
		t.Errorf("flag: %+v", got)
	}
}

// The server refuses to start when its configured user is not the pinned one
// — the stale-shell case — and says where each came from.
func TestServerRefusesAWrongConfiguredUserAtStartup(t *testing.T) {
	withExpectFlag(t, "")
	t.Setenv("SAP_EXPECT", "A4H.001/TESTUSER")
	t.Setenv("SAP_USER", "testuser")
	c := &mcp.Config{BaseURL: "http://sap.invalid", Username: "otheruser", Password: "x", Client: "001"}
	err := applyServerExpect(c, nil, false, false)
	if err == nil {
		t.Fatal("a wrong configured user was accepted")
	}
	for _, want := range []string{"OTHERUSER", "TESTUSER", "no logon was attempted", "SAP_EXPECT"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("%q missing from %v", want, err)
		}
	}

	c.Username = "testuser"
	if err := applyServerExpect(c, nil, false, false); err != nil {
		t.Fatalf("matching user refused: %v", err)
	}
	if c.Expect == nil || c.Expect.String() != "A4H.001/TESTUSER" {
		t.Errorf("pin not applied: %+v", c.Expect)
	}

	var log bytes.Buffer
	logServerLogon(&log, c, nil, false)
	if !strings.Contains(log.String(), "user TESTUSER from SAP_USER") || !strings.Contains(log.String(), "pinned to A4H.001/TESTUSER") {
		t.Errorf("startup log does not say whose logon and from where:\n%s", log.String())
	}
}

// The startup log names a .vsp.json default system the server is not using.
func TestServerLogonLogNamesTheUnusedConfigUser(t *testing.T) {
	c := &mcp.Config{BaseURL: "http://sap.invalid", Username: "otheruser", Client: "001"}
	systems := &config.SystemsConfig{Default: "a4h", Systems: map[string]config.SystemConfig{"a4h": {User: "TESTUSER"}}}
	var log bytes.Buffer
	logServerLogon(&log, c, systems, true)
	out := log.String()
	if !strings.Contains(out, "from --user") || !strings.Contains(out, `.vsp.json system "a4h" (user TESTUSER) is not used`) {
		t.Errorf("startup log:\n%s", out)
	}
}

// A CLI command pinned to another user than its system's is refused while the
// client is built: the fake SAP sees nothing at all.
func TestCLIRefusesAWrongConfiguredUserWithoutARequest(t *testing.T) {
	var hits atomic.Int32
	sap := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { hits.Add(1) }))
	defer sap.Close()

	params := &systemParams{URL: sap.URL, User: "OTHERUSER", Password: "x", Client: "001", Language: "EN",
		Expect: "A4H.001/TESTUSER", ExpectSource: ".vsp.json system \"a4h\""}
	if _, err := buildClient(params); err == nil || !strings.Contains(err.Error(), "no logon was attempted") {
		t.Fatalf("want a refusal before logon, got %v", err)
	}
	if n := hits.Load(); n != 0 {
		t.Errorf("the fake saw %d requests", n)
	}

	params.User = "testuser"
	client, err := buildClient(params)
	if err != nil {
		t.Fatal(err)
	}
	if pin, _, _ := client.IdentityStatus(); pin == nil {
		t.Error("the CLI client carries no pin")
	}
}

// The CLI's pin: --expect, else the system's own, else SAP_EXPECT.
func TestCLIPinPrecedence(t *testing.T) {
	withExpectFlag(t, "")
	t.Setenv("SAP_EXPECT", "A4H/ENV")
	if v, _ := cliExpect("A4H/SYS"); v != "A4H/SYS" {
		t.Errorf("system pin should win over env, got %q", v)
	}
	if v, _ := cliExpect(""); v != "A4H/ENV" {
		t.Errorf("env pin: got %q", v)
	}
	withExpectFlag(t, "A4H/FLAG")
	if v, src := cliExpect("A4H/SYS"); v != "A4H/FLAG" || src != "--expect" {
		t.Errorf("flag: got %q from %q", v, src)
	}
}
