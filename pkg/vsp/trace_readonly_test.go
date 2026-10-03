package vsp

import (
	"context"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/oisee/vibing-steampunk/pkg/saprfc"
	"github.com/spf13/cobra"
)

func setFlag(t *testing.T, cmd *cobra.Command, name, value string) {
	t.Helper()
	old := cmd.Flags().Lookup(name).Value.String()
	if err := cmd.Flags().Set(name, value); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = cmd.Flags().Set(name, old) })
}

// --call on vsp trace run and vsp trace unit runs the object; a read-only
// system refuses it before the gateway is dialled.
func TestTraceCall_RefusedOnAReadOnlySystem(t *testing.T) {
	for name, cmd := range map[string]*cobra.Command{"run": traceRunCmd, "unit": traceUnitCmd} {
		t.Run(name, func(t *testing.T) {
			dials := rfcCLITestEnv(t, true)
			setFlag(t, cmd, "call", "true")
			setFlag(t, cmd, "wait", "1")
			err := cmd.RunE(cmd, []string{"Z_DOUBLE"})
			if err == nil || !strings.Contains(err.Error(), "blocked by safety configuration") {
				t.Fatalf("want a safety refusal, got %v", err)
			}
			if n := dials(); n != 0 {
				t.Errorf("a refused --call still dialled the gateway %d time(s)", n)
			}
		})
	}
}

// Without --call nothing is executed, so a read-only system still traces.
func TestTraceWithoutCall_ReachesTheGatewayOnAReadOnlySystem(t *testing.T) {
	dials := rfcCLITestEnv(t, true)
	setFlag(t, traceRunCmd, "call", "false")
	setFlag(t, traceRunCmd, "wait", "1")
	err := traceRunCmd.RunE(traceRunCmd, []string{"Z_DOUBLE"})
	if err != nil && strings.Contains(err.Error(), "blocked") {
		t.Fatalf("refused: %v", err)
	}
	if dials() == 0 {
		t.Errorf("never reached the gateway (err %v)", err)
	}
}

// The debugger UI's Run calls the target; on a read-only system it is refused
// before a breakpoint is set or the call is made.
func TestDebugUIRun_RefusedOnAReadOnlySystem(t *testing.T) {
	port, dials := cliFakeGateway(t)
	fake := &recordingADT{}
	srv := &debugUIServer{
		dbg:      saprfc.NewADTDebugger(fake, "TESTUSER"),
		dest:     saprfc.Params{Host: "127.0.0.1", Port: port, Sysnr: "00", Client: "001", User: "TESTUSER", Password: "secret", Language: "E"},
		user:     "TESTUSER",
		target:   "Z_DOUBLE",
		line:     1,
		readOnly: true,
	}
	rec := httptest.NewRecorder()
	srv.handleRun(rec, httptest.NewRequestWithContext(context.Background(), "GET", "/api/run?seconds=1", nil))
	if !strings.Contains(rec.Body.String(), "blocked by safety configuration") {
		t.Fatalf("want a safety refusal in the page state, got %s", rec.Body.String())
	}
	if n := fake.count(); n != 0 {
		t.Errorf("a refused Run still sent %d ADT request(s)", n)
	}
	if n := dials(); n != 0 {
		t.Errorf("a refused Run still dialled the gateway %d time(s)", n)
	}
}
