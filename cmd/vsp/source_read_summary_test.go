package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// The fake program (CRLF, as ADT hands it out) and its digest, pinned by hand
// with printf ... | sha256sum and wc -c.
const (
	cliSummarySource = "REPORT zdemo_sum.\r\nWRITE 'hello'.\r\n"
	cliSummarySHA256 = "91531b9b6cd35eb4a8fc3943cf70b692809e538626fbb96500fabe33223e41cb"
)

// runSourceRead runs `vsp source read PROG ZDEMO_SUM` with flags against a
// fake SAP and returns what it printed.
func runSourceRead(t *testing.T, flags map[string]string) string {
	t.Helper()
	sap := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-CSRF-Token", "t")
		if r.URL.Path == "/sap/bc/adt/programs/programs/ZDEMO_SUM/source/main" {
			_, _ = w.Write([]byte(cliSummarySource))
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	t.Cleanup(sap.Close)
	t.Setenv("SAP_URL", sap.URL)
	t.Setenv("SAP_USER", "TESTUSER")
	t.Setenv("SAP_PASSWORD", "secret")
	oldSystemName := systemName
	systemName = ""
	t.Cleanup(func() { systemName = oldSystemName })

	cmd := sourceReadCmd
	for k, v := range flags {
		if err := cmd.Flags().Set(k, v); err != nil {
			t.Fatalf("--%s: %v", k, err)
		}
	}
	t.Cleanup(func() {
		for k := range flags {
			f := cmd.Flags().Lookup(k)
			_ = f.Value.Set(f.DefValue)
			f.Changed = false
		}
		cmd.SetOut(nil)
	})
	var out bytes.Buffer
	cmd.SetOut(&out)
	if err := runSource(cmd, []string{"PROG", "ZDEMO_SUM"}); err != nil {
		t.Fatalf("source read: %v", err)
	}
	return out.String()
}

func TestSourceReadSummaryFlag(t *testing.T) {
	out := runSourceRead(t, map[string]string{"summary": "true"})
	if strings.Contains(out, "REPORT") {
		t.Fatalf("--summary printed the source:\n%s", out)
	}
	var got struct {
		SHA256       string
		Lines, Bytes int
	}
	if err := json.Unmarshal([]byte(out), &got); err != nil {
		t.Fatalf("--summary is not JSON: %v\n%s", err, out)
	}
	if got.SHA256 != cliSummarySHA256 || got.Lines != 2 || got.Bytes != 35 {
		t.Fatalf("--summary = %+v", got)
	}
}

func TestSourceReadIfNoneMatchFlag(t *testing.T) {
	if out := runSourceRead(t, map[string]string{"if-none-match": cliSummarySHA256}); !strings.HasPrefix(out, "unchanged: source sha256 "+cliSummarySHA256+" ") {
		t.Fatalf("matching --if-none-match printed %q", out)
	}
	if out := runSourceRead(t, map[string]string{"if-none-match": strings.Repeat("a", 64)}); out != cliSummarySource {
		t.Fatalf("other --if-none-match printed %q, want the source", out)
	}
}
