package vsp

import (
	"context"
	"strings"
	"sync"
	"testing"

	"github.com/oisee/vibing-steampunk/pkg/saprfc"
)

// recordingADT is an ADT transport that answers 200 to everything and
// remembers what it was asked.
type recordingADT struct {
	mu   sync.Mutex
	reqs []saprfc.ADTRequest
}

func (r *recordingADT) Do(_ context.Context, req saprfc.ADTRequest) (*saprfc.ADTResponse, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.reqs = append(r.reqs, req)
	return &saprfc.ADTResponse{Status: 200, ReasonPhrase: "OK"}, nil
}

func (r *recordingADT) count() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return len(r.reqs)
}

func setDebugREPLReadOnly(t *testing.T, v bool) {
	t.Helper()
	saved := debugREPLReadOnly
	debugREPLReadOnly = v
	t.Cleanup(func() { debugREPLReadOnly = saved })
}

// eset overwrites a variable in a live program and adt with a writing method
// changes the system: on a read-only system the rfc debug and adt debug REPLs
// refuse both before anything is sent. Reading stays allowed.
func TestDebugREPL_ReadOnlyRefusesEsetAndWritingADT(t *testing.T) {
	ctx := context.Background()
	for _, line := range []string{
		"eset LV_COUNT 42",
		"adt POST /sap/bc/adt/activation",
		"adt delete /sap/bc/adt/programs/programs/zdemo",
	} {
		t.Run(line, func(t *testing.T) {
			setDebugREPLReadOnly(t, true)
			fake := &recordingADT{}
			err := runDebugCommand(ctx, saprfc.NewADTDebugger(fake, "TESTUSER"), line)
			if err == nil || !strings.Contains(err.Error(), "blocked by safety configuration") {
				t.Fatalf("want a safety refusal, got %v", err)
			}
			if n := fake.count(); n != 0 {
				t.Errorf("a refused command sent %d request(s)", n)
			}
		})
	}
	for name, tc := range map[string]struct {
		readOnly bool
		line     string
	}{
		"read-only adt GET": {true, "adt GET /sap/bc/adt/discovery"},
		"writable eset":     {false, "eset LV_COUNT 42"},
		"writable adt POST": {false, "adt POST /sap/bc/adt/activation"},
	} {
		t.Run(name, func(t *testing.T) {
			setDebugREPLReadOnly(t, tc.readOnly)
			fake := &recordingADT{}
			err := runDebugCommand(ctx, saprfc.NewADTDebugger(fake, "TESTUSER"), tc.line)
			if err != nil && strings.Contains(err.Error(), "blocked") {
				t.Fatalf("refused: %v", err)
			}
			if fake.count() == 0 {
				t.Error("never reached the transport")
			}
		})
	}
}

// The REPL commands take the flag from the selected system.
func TestADTDebug_TakesReadOnlyFromTheSystem(t *testing.T) {
	_ = rfcCLITestEnv(t, true) // .vsp.json with read_only true
	setDebugREPLReadOnly(t, false)
	if err := adtDebugCmd.Flags().Set("command", "adt POST /sap/bc/adt/activation"); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = adtDebugCmd.Flags().Set("command", "") })
	err := adtDebugCmd.RunE(adtDebugCmd, nil)
	if err == nil || !strings.Contains(err.Error(), "blocked by safety configuration") {
		t.Fatalf("want a safety refusal from vsp adt debug on a read_only system, got %v", err)
	}
}
