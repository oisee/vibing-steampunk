package vsp

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync"
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
	return runSourceReadAt(t, sap.URL, flags)
}

// runSourceReadAt is runSourceRead against the fake SAP at sapURL. The flags
// are reset when it returns, so one test can run it several times.
func runSourceReadAt(t *testing.T, sapURL string, flags map[string]string) string {
	t.Helper()
	t.Setenv("SAP_URL", sapURL)
	t.Setenv("SAP_USER", "TESTUSER")
	t.Setenv("SAP_PASSWORD", "secret")
	oldSystemName := systemName
	systemName = ""
	t.Cleanup(func() { systemName = oldSystemName })

	cmd := sourceReadCmd
	defer func() {
		for k := range flags {
			f := cmd.Flags().Lookup(k)
			if f != nil {
				_ = f.Value.Set(f.DefValue)
				f.Changed = false
			}
		}
		cmd.SetOut(nil)
	}()
	for k, v := range flags {
		if err := cmd.Flags().Set(k, v); err != nil {
			t.Fatalf("--%s: %v", k, err)
		}
	}
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

// With the response cache on (VSP_CACHE=true, kept on disk between CLI runs),
// --if-none-match must still read SAP: a cached v1 would say "unchanged"
// about an object another client changed to v2.
func TestSourceReadIfNoneMatchBypassesResponseCache(t *testing.T) {
	const v2 = "REPORT zdemo_sum.\r\nWRITE 'changed elsewhere'.\r\n"
	var mu sync.Mutex
	body := cliSummarySource
	sap := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-CSRF-Token", "t")
		if r.URL.Path == "/sap/bc/adt/programs/programs/ZDEMO_SUM/source/main" {
			mu.Lock()
			b := body
			mu.Unlock()
			_, _ = w.Write([]byte(b))
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	t.Cleanup(sap.Close)
	t.Setenv("VSP_CACHE", "true")
	t.Setenv("VSP_CACHE_PATH", filepath.Join(t.TempDir(), "cache.db"))

	if out := runSourceReadAt(t, sap.URL, nil); out != cliSummarySource {
		t.Fatalf("first read: %q", out)
	}
	mu.Lock()
	body = v2
	mu.Unlock()
	// The control: the cache is on, and a plain read still gets v1 from it.
	if out := runSourceReadAt(t, sap.URL, nil); out != cliSummarySource {
		t.Fatalf("the response cache is not on in this test (plain read got %q)", out)
	}
	if out := runSourceReadAt(t, sap.URL, map[string]string{"if-none-match": cliSummarySHA256}); out != v2 {
		t.Fatalf("v1's sha256 against a source now v2: want the v2 body, got %q", out)
	}
}
