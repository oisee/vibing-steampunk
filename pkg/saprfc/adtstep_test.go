package saprfc

import (
	"context"
	"net/url"
	"strings"
	"testing"
)

// recordingTransport answers every ADT request with 200 and keeps the URIs it
// was sent, so a test can read exactly what went on the wire.
type recordingTransport struct{ uris []string }

func (r *recordingTransport) Do(_ context.Context, req ADTRequest) (*ADTResponse, error) {
	r.uris = append(r.uris, req.URI)
	return &ADTResponse{Status: 200, Body: []byte("<step/>")}, nil
}

func stepQuery(t *testing.T, uri string) url.Values {
	t.Helper()
	path, raw, ok := strings.Cut(uri, "?")
	if !ok || path != "/sap/bc/adt/debugger" {
		t.Fatalf("step went to %q, want /sap/bc/adt/debugger?…", uri)
	}
	q, err := url.ParseQuery(raw)
	if err != nil {
		t.Fatalf("step query %q: %v", raw, err)
	}
	return q
}

// stepRunToLine and stepJumpToLine are refused by SAP without the target, so
// the target has to reach the query string, fragment and all.
func TestADTStepTo_SendsTheTargetURI(t *testing.T) {
	tr := &recordingTransport{}
	d := NewADTDebugger(tr, "TESTUSER")
	target := "/sap/bc/adt/oo/classes/zcl_demo/source/main#start=42"

	if _, err := d.ADTStepTo(context.Background(), "stepRunToLine", target); err != nil {
		t.Fatalf("ADTStepTo: %v", err)
	}
	q := stepQuery(t, tr.uris[0])
	if got := q.Get("method"); got != "stepRunToLine" {
		t.Errorf("method = %q, want stepRunToLine", got)
	}
	if got := q.Get("uri"); got != target {
		t.Errorf("uri = %q, want %q", got, target)
	}
}

// Every other step carries no target, and must not grow an empty uri=.
func TestADTStep_SendsNoURI(t *testing.T) {
	tr := &recordingTransport{}
	d := NewADTDebugger(tr, "TESTUSER")

	if _, err := d.ADTStep(context.Background(), "stepOver"); err != nil {
		t.Fatalf("ADTStep: %v", err)
	}
	q := stepQuery(t, tr.uris[0])
	if got := q.Get("method"); got != "stepOver" {
		t.Errorf("method = %q, want stepOver", got)
	}
	if _, present := q["uri"]; present {
		t.Errorf("stepOver sent a uri parameter: %q", tr.uris[0])
	}
}
