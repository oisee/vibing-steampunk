package vsp

import (
	"context"
	"fmt"
	"net"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/spf13/cobra"
)

// cliFakeGateway accepts TCP connections on loopback, counts them and hangs up.
// It only shows whether a command got as far as dialling the gateway.
//
// dials is a barrier, not a wait: it dials a probe of its own and returns, once
// the listener has accepted the probe, how many other connections came before
// it. The kernel hands out connections in the order they were established, so
// a dial made before dials was called has been counted by the time it returns;
// there is no window to guess.
func cliFakeGateway(t *testing.T) (port int, dials func() int64) {
	t.Helper()
	ln, err := (&net.ListenConfig{}).Listen(context.Background(), "tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	done := make(chan struct{})
	t.Cleanup(func() { close(done); _ = ln.Close() })
	accepted := make(chan string)
	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			from := conn.RemoteAddr().String()
			_ = conn.Close()
			select {
			case accepted <- from:
			case <-done:
				return
			}
		}
	}()
	var n int64
	return ln.Addr().(*net.TCPAddr).Port, func() int64 {
		t.Helper()
		probe, err := (&net.Dialer{}).DialContext(context.Background(), "tcp", ln.Addr().String())
		if err != nil {
			t.Fatalf("probe dial: %v", err)
		}
		defer probe.Close()
		self := probe.LocalAddr().String()
		timeout := time.After(5 * time.Second)
		for {
			select {
			case from := <-accepted:
				if from == self {
					return n
				}
				n++
			case <-timeout:
				t.Fatal("the gateway never accepted the probe")
			}
		}
	}
}

// rfcCLITestEnv puts the CLI in an empty directory and HOME with a clean SAP_*
// environment, and writes a .vsp.json whose default system's gateway is the
// fake one, read_only as given.
func rfcCLITestEnv(t *testing.T, readOnly bool) func() int64 {
	t.Helper()
	dir := t.TempDir()
	t.Chdir(dir)
	t.Setenv("HOME", dir)
	for _, name := range envOnlyCLIVars {
		t.Setenv(name, "")
	}
	saved := systemName
	systemName = ""
	t.Cleanup(func() { systemName = saved })

	port, dials := cliFakeGateway(t)
	cfg := fmt.Sprintf(`{"default":"devsys","systems":{"devsys":{"url":"http://127.0.0.1:1","client":"001",
	  "user":"TESTUSER","password":"secret","read_only":%t,
	  "rfc_host":"127.0.0.1","rfc_sysnr":"00","rfc_port":%d,"rfc_user":"TESTUSER","rfc_password":"secret"}}}`, readOnly, port)
	if err := os.WriteFile(".vsp.json", []byte(cfg), 0o600); err != nil {
		t.Fatal(err)
	}
	return dials
}

type rfcCLICase struct {
	cmd  *cobra.Command
	args []string
}

func rfcWriteCommands() map[string]rfcCLICase {
	return map[string]rfcCLICase{
		"call":     {rfcCallCmd, []string{"Z_DOUBLE", `{"N":21}`}},
		"run":      {rfcRunCmd, []string{"ZDEMO_REPORT"}},
		"adt POST": {rfcADTCmd, []string{"POST", "/sap/bc/adt/activation"}},
		"adt put":  {rfcADTCmd, []string{"put", "/sap/bc/adt/programs/programs/zdemo/source/main"}},
	}
}

func assertRefusedBeforeGateway(t *testing.T, c rfcCLICase, dials func() int64) {
	t.Helper()
	err := c.cmd.RunE(c.cmd, c.args)
	if err == nil || !strings.Contains(err.Error(), "blocked by safety configuration") {
		t.Fatalf("want a safety refusal, got %v", err)
	}
	if n := dials(); n != 0 {
		t.Errorf("a refused command still dialled the gateway %d time(s)", n)
	}
}

func TestRFCCLI_ReadOnlySystemRefusesWrites(t *testing.T) {
	for name, c := range rfcWriteCommands() {
		t.Run(name, func(t *testing.T) {
			dials := rfcCLITestEnv(t, true)
			assertRefusedBeforeGateway(t, c, dials)
		})
	}
}

// SAP_READ_ONLY makes a system read-only even when .vsp.json does not.
func TestRFCCLI_SAPReadOnlyRefusesWrites(t *testing.T) {
	for name, c := range rfcWriteCommands() {
		t.Run(name, func(t *testing.T) {
			dials := rfcCLITestEnv(t, false)
			t.Setenv("SAP_READ_ONLY", "true")
			assertRefusedBeforeGateway(t, c, dials)
		})
	}
}

// Without .vsp.json, from SAP_* variables alone.
func TestRFCCLI_EnvOnlyReadOnlyRefusesCall(t *testing.T) {
	dials := rfcCLITestEnv(t, false)
	_ = os.Remove(".vsp.json")
	t.Setenv("SAP_URL", "http://127.0.0.1:1")
	t.Setenv("SAP_USER", "TESTUSER")
	t.Setenv("SAP_PASSWORD", "secret")
	t.Setenv("SAP_READ_ONLY", "true")
	assertRefusedBeforeGateway(t, rfcWriteCommands()["call"], dials)
}

func TestRFCCLI_ReadsAndWritableWritesReachTheGateway(t *testing.T) {
	cases := map[string]struct {
		readOnly bool
		c        rfcCLICase
	}{
		"read-only adt GET": {true, rfcCLICase{rfcADTCmd, []string{"GET", "/sap/bc/adt/discovery"}}},
		"read-only ping":    {true, rfcCLICase{rfcPingCmd, nil}},
		"writable call":     {false, rfcWriteCommands()["call"]},
		"writable run":      {false, rfcWriteCommands()["run"]},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			dials := rfcCLITestEnv(t, tc.readOnly)
			err := tc.c.cmd.RunE(tc.c.cmd, tc.c.args)
			if err != nil && strings.Contains(err.Error(), "blocked") {
				t.Fatalf("refused: %v", err)
			}
			if dials() == 0 {
				t.Errorf("never reached the gateway (err %v)", err)
			}
		})
	}
}

// rfcCLIFreeSQLEnv is rfcCLITestEnv with block_free_sql set as given.
func rfcCLIFreeSQLEnv(t *testing.T, block bool) func() int64 {
	t.Helper()
	dials := rfcCLITestEnv(t, false)
	raw, err := os.ReadFile(".vsp.json")
	if err != nil {
		t.Fatal(err)
	}
	cfg := strings.Replace(string(raw), `"read_only":false`, fmt.Sprintf(`"read_only":false,"block_free_sql":%t`, block), 1)
	if err := os.WriteFile(".vsp.json", []byte(cfg), 0o600); err != nil {
		t.Fatal(err)
	}
	return dials
}

func runReadTable(t *testing.T, where string) error {
	t.Helper()
	if err := rfcReadTableCmd.Flags().Set("where", where); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = rfcReadTableCmd.Flags().Set("where", "") })
	return rfcReadTableCmd.RunE(rfcReadTableCmd, []string{"USR02"})
}

// A caller's --where is a free query; a system that blocks free SQL refuses
// it before the gateway is dialled.
func TestRFCCLI_ReadTableWhereRefusedWhenFreeSQLBlocked(t *testing.T) {
	setups := map[string]func(t *testing.T) func() int64{
		"block_free_sql": func(t *testing.T) func() int64 { return rfcCLIFreeSQLEnv(t, true) },
		"SAP_BLOCK_FREE_SQL": func(t *testing.T) func() int64 {
			d := rfcCLIFreeSQLEnv(t, false)
			t.Setenv("SAP_BLOCK_FREE_SQL", "true")
			return d
		},
		"env only": func(t *testing.T) func() int64 {
			d := rfcCLITestEnv(t, false)
			_ = os.Remove(".vsp.json")
			t.Setenv("SAP_URL", "http://127.0.0.1:1")
			t.Setenv("SAP_USER", "TESTUSER")
			t.Setenv("SAP_PASSWORD", "secret")
			t.Setenv("SAP_BLOCK_FREE_SQL", "true")
			return d
		},
	}
	for name, setup := range setups {
		t.Run(name, func(t *testing.T) {
			dials := setup(t)
			err := runReadTable(t, "BNAME = 'TESTUSER'")
			if err == nil || !strings.Contains(err.Error(), "blocked by safety configuration") || !strings.Contains(err.Error(), "type F") {
				t.Fatalf("want a free-SQL refusal, got %v", err)
			}
			if n := dials(); n != 0 {
				t.Errorf("a refused read-table still dialled the gateway %d time(s)", n)
			}
		})
	}
}

func TestRFCCLI_ReadTableOtherwiseReachesTheGateway(t *testing.T) {
	cases := map[string]struct {
		block bool
		where string
	}{
		"blocked, no where": {true, ""},
		"blocked, blank":    {true, "   "},
		"unblocked, where":  {false, "BNAME = 'TESTUSER'"},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			dials := rfcCLIFreeSQLEnv(t, tc.block)
			err := runReadTable(t, tc.where)
			if err != nil && strings.Contains(err.Error(), "blocked") {
				t.Fatalf("refused: %v", err)
			}
			if dials() == 0 {
				t.Errorf("never reached the gateway (err %v)", err)
			}
		})
	}
}
