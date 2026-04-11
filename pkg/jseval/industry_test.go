package jseval

import (
	"os"
	"strings"
	"testing"
)

func requireLocalJSFixture(t *testing.T, path string) {
	t.Helper()
	if _, err := os.Stat(path); err != nil {
		t.Skipf("local-only JS fixture not available: %s", path)
	}
}

func runFile(t *testing.T, path string) string {
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	out, err := Eval(string(data))
	if err != nil {
		t.Fatalf("eval: %v", err)
	}
	return strings.TrimSpace(out)
}

func TestTest262Basic(t *testing.T) {
	path := "/mnt/safe/@js2abap/test262_basic.js"
	requireLocalJSFixture(t, path)
	out := runFile(t, path)
	lines := strings.Split(out, "\n")
	last := lines[len(lines)-1]
	t.Logf("Result: %s", last)
	if !strings.HasPrefix(last, "PASS=") || strings.Contains(last, "FAIL=0") == false {
		t.Logf("Full output:\n%s", out)
	}
	if !strings.Contains(last, "FAIL=0") {
		t.Errorf("Some tests failed: %s", last)
	}
}

func TestSunspiderPartialSums(t *testing.T) {
	path := "/mnt/safe/@js2abap/sunspider_partial_sums.js"
	requireLocalJSFixture(t, path)
	out := runFile(t, path)
	t.Logf("Output:\n%s", out)
	if out == "" {
		t.Error("empty output")
	}
}

func TestRichardsMini(t *testing.T) {
	path := "/mnt/safe/@js2abap/richards_mini.js"
	requireLocalJSFixture(t, path)
	out := runFile(t, path)
	t.Logf("Output: %s", out)
	if out != "100500" {
		t.Errorf("got %q, want 100500", out)
	}
}
