package main

import (
	"reflect"
	"testing"
)

func TestSplitREPLArgs(t *testing.T) {
	got := splitREPLArgs(`call RPY_X HEADER={"A":"b c"} FLOW=[{"LINE":"PROCESS BEFORE OUTPUT."},{"LINE":"*"}] S=X`)
	want := []string{"call", "RPY_X", `HEADER={"A":"b c"}`, `FLOW=[{"LINE":"PROCESS BEFORE OUTPUT."},{"LINE":"*"}]`, "S=X"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %q", got)
	}
	if got := splitREPLArgs(`  a   b	c `); !reflect.DeepEqual(got, []string{"a", "b", "c"}) {
		t.Errorf("plain: %q", got)
	}
}
