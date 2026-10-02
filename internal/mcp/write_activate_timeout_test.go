package mcp

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/oisee/vibing-steampunk/pkg/adt"
)

// slowWriteSAP stands in for SAP over WriteSource and Activate. Everything is
// answered at once except the two requests that take long on a real system:
// the PUT of the source and the activation, which answer after delay.
func slowWriteSAP(t *testing.T, delay time.Duration) (*httptest.Server, *writeCounts) {
	t.Helper()
	return writeSAP(t, delay, `<?xml version="1.0" encoding="utf-8"?><chkl:messages xmlns:chkl="http://www.sap.com/abapxml/checklist">`+
		`<chkl:properties checkExecuted="true" activationExecuted="true" generationExecuted="true"/></chkl:messages>`)
}

// writeSAP is slowWriteSAP answering the activation with activationXML.
func writeSAP(t *testing.T, delay time.Duration, activationXML string) (*httptest.Server, *writeCounts) {
	t.Helper()
	counts := &writeCounts{}
	puts, unlocks := &counts.puts, &counts.unlocks
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Read the body, so the server notices a client that gives up.
		_, _ = io.Copy(io.Discard, r.Body)
		w.Header().Set("X-CSRF-Token", "t")
		slow := func() bool {
			select {
			case <-time.After(delay):
				return true
			case <-r.Context().Done():
				return false
			}
		}
		switch {
		case r.Method == http.MethodPut && strings.HasSuffix(r.URL.Path, "/source/main"):
			if !slow() {
				return
			}
			puts.Add(1)
			w.WriteHeader(http.StatusOK)
		case strings.Contains(r.URL.Path, "/activation"):
			if !slow() {
				return
			}
			w.Header().Set("Content-Type", "application/xml")
			_, _ = w.Write([]byte(activationXML))
		case strings.Contains(r.URL.Path, "/checkruns"):
			w.Header().Set("Content-Type", "application/vnd.sap.adt.checkmessages+xml")
			_, _ = w.Write([]byte(`<?xml version="1.0" encoding="UTF-8"?><chkl:messages xmlns:chkl="http://www.sap.com/adt/checklist"/>`))
		case r.Method == http.MethodPost && r.URL.Query().Get("_action") == "UNLOCK":
			unlocks.Add(1)
			w.WriteHeader(http.StatusOK)
		case r.Method == http.MethodPost && r.URL.Query().Get("_action") == "LOCK":
			w.Header().Set("Content-Type", "application/vnd.sap.as+xml")
			_, _ = w.Write([]byte(`<?xml version="1.0" encoding="UTF-8"?>
<asx:abap xmlns:asx="http://www.sap.com/abapxml" version="1.0"><asx:values><DATA>
<LOCK_HANDLE>HANDLE-1</LOCK_HANDLE><MODIFICATION_SUPPORT>Modification</MODIFICATION_SUPPORT>
</DATA></asx:values></asx:abap>`))
		case strings.Contains(r.URL.Path, "informationsystem/search"):
			_, _ = w.Write([]byte(`<?xml version="1.0" encoding="UTF-8"?>
<adtcore:objectReferences xmlns:adtcore="http://www.sap.com/adt/core">
  <adtcore:objectReference adtcore:uri="/sap/bc/adt/programs/programs/zdemo_slow" adtcore:type="PROG/P" adtcore:name="ZDEMO_SLOW" adtcore:packageName="$TMP"/>
</adtcore:objectReferences>`))
		default:
			w.WriteHeader(http.StatusOK)
		}
	}))
	t.Cleanup(srv.Close)
	return srv, counts
}

// writeCounts is what the fake saw complete.
type writeCounts struct {
	puts    atomic.Int32
	unlocks atomic.Int32
}

// The PUT of a source and its activation used to be cut at the client's
// per-request timeout (60s; here shortened to keep the test fast) whatever
// --call-timeout said, because only ExecuteABAP, ABAP Unit and the deploys
// ran under the call's budget. Under a budget they must outlast that timeout;
// without one the per-request limit must stay as it was.
func TestWriteAndActivateHonourCallTimeout(t *testing.T) {
	const perRequest = 150 * time.Millisecond
	const delay = 600 * time.Millisecond

	newServer := func(url string, callTimeout time.Duration) *Server {
		return &Server{
			config:    &Config{CallTimeout: callTimeout},
			adtClient: adt.NewClient(url, "u", "p", adt.WithTimeout(perRequest)),
		}
	}

	t.Run("WriteSource under the server budget", func(t *testing.T) {
		srv, counts := slowWriteSAP(t, delay)
		s := newServer(srv.URL, 30*time.Second)
		res, err := s.handleWriteSource(context.Background(), newRequest(map[string]any{
			"object_type": "PROG", "name": "ZDEMO_SLOW", "source": "REPORT zdemo_slow.", "mode": "update",
		}))
		if err != nil {
			t.Fatal(err)
		}
		if text := resultText(res); res.IsError || strings.Contains(text, "Timeout") || strings.Contains(text, "timed out") {
			t.Fatalf("the write was cut at the per-request timeout despite its budget: %s", text)
		}
		if counts.puts.Load() != 1 {
			t.Fatalf("the source PUT completed %d times, want 1", counts.puts.Load())
		}
	})

	t.Run("Activate under the server budget", func(t *testing.T) {
		srv, _ := slowWriteSAP(t, delay)
		s := newServer(srv.URL, 30*time.Second)
		res, err := s.handleActivate(context.Background(), newRequest(map[string]any{
			"object_url": "/sap/bc/adt/programs/programs/zdemo_slow", "object_name": "ZDEMO_SLOW",
		}))
		if err != nil {
			t.Fatal(err)
		}
		if res.IsError {
			t.Fatalf("the activation was cut at the per-request timeout despite its budget: %s", resultText(res))
		}
	})

	t.Run("Activate under params.timeout", func(t *testing.T) {
		srv, _ := slowWriteSAP(t, delay)
		s := newServer(srv.URL, 0)
		res, err := s.handleActivateMultiple(context.Background(), newRequest(map[string]any{
			"objects": []any{map[string]any{"url": "/sap/bc/adt/programs/programs/zdemo_slow", "name": "ZDEMO_SLOW"}},
			"timeout": float64(30),
		}))
		if err != nil {
			t.Fatal(err)
		}
		if res.IsError {
			t.Fatalf("the activation was cut at the per-request timeout despite params.timeout: %s", resultText(res))
		}
	})

	t.Run("without a budget the per-request limit stays", func(t *testing.T) {
		srv, _ := slowWriteSAP(t, delay)
		s := newServer(srv.URL, 0)
		res, err := s.handleActivate(context.Background(), newRequest(map[string]any{
			"object_url": "/sap/bc/adt/programs/programs/zdemo_slow", "object_name": "ZDEMO_SLOW",
		}))
		if err != nil {
			t.Fatal(err)
		}
		// longCall words it ("per-request limit"); the bare client error says
		// Client.Timeout. Either way the 60s default (here 150ms) still holds.
		if text := resultText(res); !res.IsError || !(strings.Contains(text, "per-request limit") || strings.Contains(text, "Client.Timeout")) {
			t.Fatalf("want the per-request timeout to end an activation without a budget, got %s", text)
		}
	})
}

// A budget that runs out inside the lock window must not strand the lock.
// LOCK is answered at once, the PUT outlasts the call's budget, and the
// deferred UNLOCK used to run on the call's ended context: it failed before a
// byte was sent and the ENQUEUE stayed on the object (#166). It must still
// reach SAP.
func TestWriteSourceReleasesTheLockWhenTheBudgetRunsOut(t *testing.T) {
	// One case per workflow branch: PROG update (WriteProgram), INTF update,
	// CLAS update (WriteClass) and DDLS update each take their own lock.
	for _, c := range []struct{ typ, name, source string }{
		{"PROG", "ZDEMO_SLOW", "REPORT zdemo_slow."},
		{"INTF", "ZIF_DEMO_SLOW", "INTERFACE zif_demo_slow PUBLIC.\nENDINTERFACE."},
		{"CLAS", "ZCL_DEMO_SLOW", "CLASS zcl_demo_slow DEFINITION PUBLIC.\nENDCLASS.\nCLASS zcl_demo_slow IMPLEMENTATION.\nENDCLASS."},
		{"DDLS", "ZDEMO_SLOW_V", "define view entity ZDEMO_SLOW_V as select from t000 { mandt }"},
	} {
		t.Run(c.typ, func(t *testing.T) {
			srv, counts := slowWriteSAP(t, 5*time.Second)
			s := &Server{config: &Config{}, adtClient: adt.NewClient(srv.URL, "u", "p")}
			res, err := s.handleWriteSource(context.Background(), newRequest(map[string]any{
				"object_type": c.typ, "name": c.name, "source": c.source, "mode": "update",
				"timeout": 0.5,
			}))
			if err != nil {
				t.Fatal(err)
			}
			if text := resultText(res); !res.IsError || !strings.Contains(text, "WriteSource timed out") {
				t.Fatalf("want a timeout, got %s", text)
			}
			if counts.puts.Load() != 0 {
				t.Fatal("the PUT was meant to be cut off by the budget")
			}
			if counts.unlocks.Load() == 0 {
				t.Fatal("the lock taken before the PUT was never released: no UNLOCK reached SAP")
			}
		})
	}
}

// The MCP WriteSource failure hands over the structured result as JSON, which
// holds every activation message. Its first line is the verdict only: the
// messages must not come twice.
func TestWriteSourceFailureSendsActivationMessagesOnce(t *testing.T) {
	const msg = "Type ZIF_NOWHERE is unknown."
	srv, _ := writeSAP(t, 0, `<?xml version="1.0" encoding="utf-8"?>`+
		`<chkl:messages xmlns:chkl="http://www.sap.com/abapxml/checklist">`+
		`<chkl:properties checkExecuted="true" activationExecuted="false" generationExecuted="false"/>`+
		`<msg objDescr="Program ZDEMO_SLOW" type="E" line="3" href=""><shortText><txt>`+msg+`</txt></shortText></msg>`+
		`</chkl:messages>`)
	s := &Server{config: &Config{}, adtClient: adt.NewClient(srv.URL, "u", "p")}
	res, err := s.handleWriteSource(context.Background(), newRequest(map[string]any{
		"object_type": "PROG", "name": "ZDEMO_SLOW", "source": "REPORT zdemo_slow.", "mode": "update",
	}))
	if err != nil {
		t.Fatal(err)
	}
	text := resultText(res)
	if !res.IsError || !strings.HasPrefix(text, "WriteSource failed: Activation failed") {
		t.Fatalf("want the activation verdict, got %s", text)
	}
	if n := strings.Count(text, msg); n != 1 {
		t.Fatalf("the activation message appears %d times, want once (in the JSON):\n%s", n, text)
	}
}
