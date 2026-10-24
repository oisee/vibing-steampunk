package mcp

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/mark3labs/mcp-go/mcp"
	openrfc "github.com/oisee/open-rfc-go/rfc"

	"github.com/oisee/vibing-steampunk/pkg/saprfc"
)

// op "job" is the poller -- a job another process started, observed through the
// status letter alone. Nothing could reach its body: the op has no CLI verb, the
// RFC client is concrete, and the repo has no fake gateway. So the one line that
// turned the letter into "started" was the only part of #353 that no test ran,
// and CI's own proof gate called it proven anyway (the test did not compile on
// the base).
//
// The XBP reads are fields on Server, and rfcClientFor hands back a pre-set
// rfcShared without dialing, so the op runs end to end with no gateway. What is
// asserted is the op's own answer, not a helper beside it.
func jobOpServer(t *testing.T, status saprfc.PolledJobStatus) *Server {
	t.Helper()
	// dev.example.local / TESTUSER are the fixtures the sanitize policy names;
	// nothing here dials, so no real host is implied.
	cfg := Config{BaseURL: "https://dev.example.local:44300", Username: "TESTUSER", Password: "secret", Client: "100"}
	s := NewServer(&cfg)
	// A destination override would make rfcClientFor dial. A pre-set shared
	// client is returned as it is.
	s.rfcShared = &openrfc.Client{}
	s.jobStatusCall = func(context.Context, *openrfc.Client, string, string) (saprfc.PolledJobStatus, error) {
		return status, nil
	}
	return s
}

func TestJobOpReportsStartedFromTheStatusLetter(t *testing.T) {
	for _, tt := range []struct {
		letter string
		want   bool
	}{
		{"R", true},  // active
		{"F", true},  // finished
		{"A", true},  // cancelled
		{"P", false}, // scheduled
		{"S", false}, // released
		{"Y", false}, // ready
	} {
		t.Run(tt.letter, func(t *testing.T) {
			s := jobOpServer(t, saprfc.PolledJobStatus{Status: tt.letter})

			res, handled, err := s.routeRFCAction(context.Background(), "rfc", "ZDEMO_NIGHTLY", "", map[string]any{
				"op": "job", "job_count": "12345678", "joblog": false, "spool": false,
			})
			if !handled {
				t.Fatal("rfc action not handled")
			}
			if err != nil {
				t.Fatalf("op job: %v", err)
			}
			var run struct {
				Started bool   `json:"started"`
				Status  string `json:"status"`
			}
			decodeResult(t, res, &run)
			if run.Status != tt.letter {
				t.Errorf("status = %q, want %q", run.Status, tt.letter)
			}
			if run.Started != tt.want {
				t.Errorf("status %q: started = %t, want %t -- a poller has only the letter",
					tt.letter, run.Started, tt.want)
			}
		})
	}
}

// #353 also asked, in passing, for the start time. op "job" reads TBTCO's
// STRTDATE/STRTTIME alongside the letter and carries it as the system wrote it:
// a time.Time would be read as UTC and be wrong by the system's offset.
func TestJobOpReportsTheStartStamp(t *testing.T) {
	s := jobOpServer(t, saprfc.PolledJobStatus{Status: "R", StartStamp: "20261003 215908"})

	res, handled, err := s.routeRFCAction(context.Background(), "rfc", "ZDEMO_NIGHTLY", "", map[string]any{
		"op": "job", "job_count": "12345678", "joblog": false, "spool": false,
	})
	if !handled || err != nil {
		t.Fatalf("op job: handled=%t err=%v", handled, err)
	}
	var run struct {
		StartStamp string `json:"start_stamp"`
	}
	decodeResult(t, res, &run)
	if run.StartStamp != "20261003 215908" {
		t.Errorf("start_stamp = %q, want the TBTCO stamp 20261003 215908 as written", run.StartStamp)
	}
}

// What #354 asked for: a running job's log comes back from op "job", by default
// (no joblog parameter). The gate alone was tested before; this drives the read
// and asserts the entries reach the answer.
func TestJobOpFetchesTheLogOfARunningJob(t *testing.T) {
	s := jobOpServer(t, saprfc.PolledJobStatus{Status: "R"})
	called := false
	s.jobLogCall = func(_ context.Context, _ *openrfc.Client, name, count string) ([]saprfc.JobLogEntry, error) {
		called = true
		if name != "ZDEMO_NIGHTLY" || count != "12345678" {
			t.Errorf("log read for %q/%q, want ZDEMO_NIGHTLY/12345678", name, count)
		}
		return []saprfc.JobLogEntry{{Type: "S", Text: "tick 1"}, {Type: "S", Text: "tick 2"}}, nil
	}

	res, handled, err := s.routeRFCAction(context.Background(), "rfc", "ZDEMO_NIGHTLY", "", map[string]any{
		"op": "job", "job_count": "12345678",
	})
	if !handled || err != nil {
		t.Fatalf("op job: handled=%t err=%v", handled, err)
	}
	if !called {
		t.Fatal("no log read: a job that has begun must be asked for its log (#354)")
	}
	var run struct {
		JobLog []saprfc.JobLogEntry `json:"job_log"`
	}
	decodeResult(t, res, &run)
	if len(run.JobLog) != 2 || run.JobLog[0].Text != "tick 1" {
		t.Fatalf("job_log = %+v, want the two entries the read returned", run.JobLog)
	}
}

// A job the system does not know is refused. It must not come back as a job that
// merely has not begun, which is how "started": false used to be read.
func TestJobOpRefusesAJobTheSystemDoesNotKnow(t *testing.T) {
	s := jobOpServer(t, saprfc.PolledJobStatus{})

	_, handled, err := s.routeRFCAction(context.Background(), "rfc", "ZDEMO_NIGHTLY", "", map[string]any{
		"op": "job", "job_count": "12345678",
	})
	if !handled {
		t.Fatal("rfc action not handled")
	}
	if err == nil {
		t.Fatal("a job that is not there was reported as a job")
	}
}

// decodeResult pulls the op's JSON answer out of the MCP result.
func decodeResult(t *testing.T, res *mcp.CallToolResult, v any) {
	t.Helper()
	if res == nil || len(res.Content) == 0 {
		t.Fatal("empty result")
	}
	text, ok := res.Content[0].(mcp.TextContent)
	if !ok {
		t.Fatalf("result content is %T, want text", res.Content[0])
	}
	if err := json.Unmarshal([]byte(text.Text), v); err != nil {
		t.Fatalf("decode %q: %v", text.Text, err)
	}
}
