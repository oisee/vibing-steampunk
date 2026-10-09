package runtimeguard

import (
	"os"
	"path/filepath"
	"testing"
)

func TestRuntimeCannotWriteLogsAndCanBeCleaned(t *testing.T) {
	dir, cleanup, err := Prepare()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := cleanup(); err != nil {
			t.Error(err)
		}
	})
	if _, err := os.ReadFile(filepath.Join(dir, "sapnwrfc.ini")); err != nil {
		t.Fatal("SDK config cannot be read")
	}
	for _, name := range []string{"dev_rfc.log", "dev_rfc.trc", "rfc.trc"} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte("fixture"), 0600); err == nil {
			t.Fatal("SDK could persist a log")
		}
	}
}
