package main

import (
	"bytes"
	"strings"
	"testing"
)

func TestSNCServeInvocation(t *testing.T) {
	for argv, want := range map[string]bool{
		"vsp snc-serve":        true,
		"vsp snc-serve-worker": true,
		"vsp":                  false,
		"vsp search snc-serve": false,
		"vsp --snc-serve":      false,
	} {
		if got := sncServeInvocation(strings.Fields(argv)); got != want {
			t.Errorf("%q: got %v", argv, got)
		}
	}
}

// The worker refuses to run unless the supervisor started it, and writes
// nothing to stdout, which carries frames only.
func TestSNCServeWorkerNeedsSupervisor(t *testing.T) {
	t.Setenv("VSP_SNC_WORKER", "")
	var out, errOut bytes.Buffer
	if code := runSNCServe([]string{"vsp", "snc-serve-worker"}, &bytes.Buffer{}, &out, &errOut); code != 2 || out.Len() != 0 {
		t.Fatalf("code %d, stdout %q", code, out.String())
	}
}
