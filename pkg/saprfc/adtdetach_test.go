package saprfc

import (
	"context"
	"net/url"
	"reflect"
	"strings"
	"testing"
)

// detachTransport answers by method + path + ?method=, and records the
// requests in that form.
type detachTransport struct {
	answers map[string]*ADTResponse
	asked   []string
}

func (s *detachTransport) Do(_ context.Context, req ADTRequest) (*ADTResponse, error) {
	path, query, _ := strings.Cut(req.URI, "?")
	key := req.Method + " " + path
	if q, _ := url.ParseQuery(query); q.Get("method") != "" {
		key += "?method=" + q.Get("method")
	}
	s.asked = append(s.asked, key)
	if res, ok := s.answers[key]; ok {
		return res, nil
	}
	return &ADTResponse{Status: 404, ReasonPhrase: "Not Found"}, nil
}

const (
	detachKey   = "POST /sap/bc/adt/debugger?method=detach"
	continueKey = "POST /sap/bc/adt/debugger?method=stepContinue"
	listenerDel = "DELETE /sap/bc/adt/debugger/listeners"
)

func ended(subType string) *ADTResponse {
	return &ADTResponse{Status: 500, ReasonPhrase: "Internal Server Error", Body: []byte(
		`<exc:exception><properties><entry key="com.sap.adt.communicationFramework.subType">` + subType + `</entry></properties></exc:exception>`)}
}

// engagedDebugger is a debugger that listened, so a detach has work to do.
func engagedDebugger(t *testing.T, answers map[string]*ADTResponse) (*Debugger, *detachTransport) {
	t.Helper()
	tr := &detachTransport{answers: answers}
	d := NewADTDebugger(tr, "TESTUSER")
	d.engaged, d.listenUser = true, "TESTUSER"
	return d, tr
}

// ADTDetach's requests, pinned: the MCP tools, vsp adt debug and vsp dap all
// release through it, and splitting out the debuggee half must not change
// what it sends or in which order.
func TestADTDetachSequenceIsUnchanged(t *testing.T) {
	cases := map[string]struct {
		answers map[string]*ADTResponse
		want    []string
	}{
		"detach accepted": {
			answers: map[string]*ADTResponse{detachKey: {Status: 200}, listenerDel: {Status: 200}},
			want:    []string{detachKey, listenerDel},
		},
		"detach refused, as on A4H": {
			answers: map[string]*ADTResponse{detachKey: {Status: 400}, continueKey: ended("debuggeeEnded"), listenerDel: {Status: 200}},
			want:    []string{detachKey, continueKey, listenerDel},
		},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			d, tr := engagedDebugger(t, tc.answers)
			if err := d.ADTDetach(context.Background()); err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(tr.asked, tc.want) {
				t.Errorf("asked %v, want %v", tr.asked, tc.want)
			}
			// Nothing left to release: a second detach sends nothing.
			tr.asked = nil
			if err := d.ADTDetach(context.Background()); err != nil || len(tr.asked) != 0 {
				t.Errorf("second detach: %v, asked %v", err, tr.asked)
			}
		})
	}
}

// DetachDebuggee releases only the debuggee, and is nil only when SAP
// confirmed that nothing is attached any more.
func TestDetachDebuggee(t *testing.T) {
	cases := map[string]struct {
		answers   map[string]*ADTResponse
		want      []string
		confirmed bool
	}{
		"detach accepted": {
			answers: map[string]*ADTResponse{detachKey: {Status: 200}},
			want:    []string{detachKey}, confirmed: true,
		},
		"detach refused, the debuggee ran to its end": {
			answers: map[string]*ADTResponse{detachKey: {Status: 400}, continueKey: ended("debuggeeEnded")},
			want:    []string{detachKey, continueKey}, confirmed: true,
		},
		"detach refused, nothing was attached": {
			answers: map[string]*ADTResponse{detachKey: {Status: 400}, continueKey: ended("noSessionAttached")},
			want:    []string{detachKey, continueKey}, confirmed: true,
		},
		"detach refused, the program stopped again": {
			answers: map[string]*ADTResponse{detachKey: {Status: 400}, continueKey: {Status: 200}},
			want:    []string{detachKey, continueKey}, confirmed: false,
		},
		"detach refused, the fallback failed": {
			answers: map[string]*ADTResponse{detachKey: {Status: 400}, continueKey: ended("kernelError")},
			want:    []string{detachKey, continueKey}, confirmed: false,
		},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			d, tr := engagedDebugger(t, tc.answers)
			err := d.DetachDebuggee(context.Background())
			if (err == nil) != tc.confirmed {
				t.Errorf("confirmed=%v, err %v", err == nil, err)
			}
			if !reflect.DeepEqual(tr.asked, tc.want) {
				t.Errorf("asked %v, want %v (the listener must be left alone)", tr.asked, tc.want)
			}
		})
	}
}

// uriRecorder records each request's method and full URI, and answers 200.
type uriRecorder struct{ sent []string }

func (r *uriRecorder) Do(_ context.Context, req ADTRequest) (*ADTResponse, error) {
	r.sent = append(r.sent, req.Method+" "+req.URI)
	return &ADTResponse{Status: 200}, nil
}

// The listener stopped from a side connection is the very DELETE ADTDetach
// sends, and once SAP confirmed it, the detach does not send it again: it
// releases the debuggee alone. A new listen makes the deletion owed again.
func TestADTStopListenerViaSendsTheDetachDelete(t *testing.T) {
	session := &uriRecorder{}
	d := NewADTDebugger(session, "TESTUSER")
	d.engaged, d.listenUser = true, "TESTUSER"
	if err := d.ADTDetach(context.Background()); err != nil {
		t.Fatal(err)
	}
	want := session.sent[len(session.sent)-1]

	session.sent = nil
	d.engaged = true
	side := &uriRecorder{}
	if err := d.ADTStopListenerVia(context.Background(), side); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(side.sent, []string{want}) {
		t.Errorf("side connection sent %v, want %v", side.sent, []string{want})
	}
	if len(session.sent) != 0 {
		t.Errorf("the session was used: %v", session.sent)
	}
	if err := d.ADTDetach(context.Background()); err != nil {
		t.Fatal(err)
	}
	for _, s := range session.sent {
		if strings.HasPrefix(s, "DELETE ") {
			t.Errorf("the detach deleted the listener again: %v", session.sent)
		}
	}

	// A refusal is not a removal: the detach still owes the DELETE.
	d.engaged, d.listenerStopped = true, false
	refused := &detachTransport{answers: map[string]*ADTResponse{listenerDel: {Status: 500, ReasonPhrase: "Internal Server Error"}}}
	if err := d.ADTStopListenerVia(context.Background(), refused); err == nil {
		t.Error("a refused deletion reported success")
	}
	if d.listenerStopped {
		t.Error("a refused deletion was recorded as done")
	}
}
