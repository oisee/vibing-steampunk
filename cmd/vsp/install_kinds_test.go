package main

import (
	"strings"
	"testing"

	embedded "github.com/oisee/vibing-steampunk/embedded/abap"
)

// The install list counted "1 interface, N-1 classes" whatever the objects
// were; with a program among them it was wrong.
func TestObjectKindSummaryMatchesTheObjects(t *testing.T) {
	got := objectKindSummary(embedded.GetObjects())
	if got != "1 interface, 9 classes, 3 programs" {
		t.Errorf("got %q", got)
	}
}

// The buffer command reads the buffer file; its help must not send people
// looking for TMS_TP_SHOW_BUFFER.
func TestTransportBufferHelpNamesTheFile(t *testing.T) {
	if strings.Contains(transportBufferCmd.Long, "TMS_TP_SHOW_BUFFER") || !strings.Contains(transportBufferCmd.Long, "DIR_TRANS/buffer/<SID>") {
		t.Errorf("help: %s", transportBufferCmd.Long)
	}
}
