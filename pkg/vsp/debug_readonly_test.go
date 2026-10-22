package vsp

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"testing/synctest"
	"time"

	"github.com/gorilla/websocket"
	"github.com/oisee/vibing-steampunk/pkg/adt"
)

// roundTripperFunc serves a request in-process.
type roundTripperFunc func(*http.Request) (*http.Response, error)

func (f roundTripperFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

// debugTestSAP is a SAP that only counts requests and answers 404.
func debugTestSAP(hits *atomic.Int64) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		http.Error(w, "no", http.StatusNotFound)
	})
}

// debugTestSession is a vsp debug session whose SAP only counts requests.
// The ADT client reaches it in-process, so the session can live in a synctest
// bubble and synctest.Wait can tell when every goroutine it started -- the
// REPL's debugger listener -- has done all it is going to. The WebSocket is
// never connected; its URL is a real httptest server outside the bubble,
// which counts the same way.
func debugTestSession(t *testing.T, readOnly bool, wsURL string, sap http.Handler) *debugSession {
	t.Helper()
	cfg := adt.NewConfig("http://sap.invalid", "TESTUSER", "secret")
	hc := cfg.NewHTTPClient()
	hc.Transport = roundTripperFunc(func(r *http.Request) (*http.Response, error) {
		rec := httptest.NewRecorder()
		sap.ServeHTTP(rec, r)
		resp := rec.Result()
		resp.Request = r
		return resp, nil
	})
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	return &debugSession{
		client:   adt.NewClientWithTransport(cfg, adt.NewTransportWithClient(cfg, hc)),
		wsClient: adt.NewDebugWebSocketClient(wsURL, "001", "TESTUSER", "secret", false),
		user:     "TESTUSER",
		readOnly: readOnly,
		ctx:      ctx,
		cancel:   cancel,
	}
}

// The REPL's "run" submits a report and "call" runs a function module: both
// execute code on the system, so a read-only system refuses them before the
// debugger listener starts or anything is sent.
func TestDebugREPL_ReadOnlyRefusesRunAndCall(t *testing.T) {
	cases := map[string]func(*debugSession) error{
		"run":  func(s *debugSession) error { return s.runProgram([]string{"ZDEMO_REPORT"}) },
		"call": func(s *debugSession) error { return s.callRFC([]string{"Z_DOUBLE", "N=21"}) },
	}
	for name, do := range cases {
		t.Run(name, func(t *testing.T) {
			var hits atomic.Int64
			sap := debugTestSAP(&hits)
			srv := httptest.NewServer(sap)
			t.Cleanup(srv.Close)
			synctest.Test(t, func(t *testing.T) {
				s := debugTestSession(t, true, srv.URL, sap)
				err := do(s)
				if err == nil || !strings.Contains(err.Error(), "blocked by safety configuration") {
					t.Fatalf("want a safety refusal, got %v", err)
				}
				// A listener, had one started, has now sent its request: every
				// goroutine in the bubble has run until it blocked or exited.
				synctest.Wait()
				if n := hits.Load(); n != 0 {
					t.Errorf("a refused %s still reached SAP %d time(s)", name, n)
				}
			})
		})
	}
}

// fakeZADTVSP is a ZADT_VSP WebSocket endpoint: it greets, then records every
// message a client sends. Everything else on the server answers 404.
func fakeZADTVSP(t *testing.T) (url string, sent <-chan map[string]any) {
	t.Helper()
	ch := make(chan map[string]any, 16)
	up := websocket.Upgrader{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/sap/bc/apc/sap/zadt_vsp" {
			http.Error(w, "no", http.StatusNotFound)
			return
		}
		conn, err := up.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer conn.Close()
		_ = conn.WriteMessage(websocket.TextMessage, []byte(`{"id":"welcome","success":true,"data":{"session":"S1"}}`))
		for {
			_, msg, err := conn.ReadMessage()
			if err != nil {
				return
			}
			var m map[string]any
			if json.Unmarshal(msg, &m) == nil {
				ch <- m
			}
		}
	}))
	t.Cleanup(srv.Close)
	return srv.URL, ch
}

// On a writable system "run" goes out: the report submit reaches ZADT_VSP.
func TestDebugREPL_WritableRunReachesTheWebSocket(t *testing.T) {
	url, sent := fakeZADTVSP(t)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	ws := adt.NewDebugWebSocketClient(url, "001", "TESTUSER", "secret", false)
	if err := ws.Connect(ctx); err != nil {
		t.Fatalf("connect to the fake ZADT_VSP: %v", err)
	}
	t.Cleanup(func() { _ = ws.Close() })
	s := &debugSession{
		client:   adt.NewClient(url, "TESTUSER", "secret"),
		wsClient: ws,
		user:     "TESTUSER",
		ctx:      ctx,
		cancel:   cancel,
	}

	err := s.runProgram([]string{"ZDEMO_REPORT"})
	if err != nil && strings.Contains(err.Error(), "blocked") {
		t.Fatalf("run refused on a writable system: %v", err)
	}
	select {
	case m := <-sent:
		params, _ := m["params"].(map[string]any)
		if m["action"] != "runReport" || params["report"] != "ZDEMO_REPORT" {
			t.Errorf("the WebSocket got %v, want a runReport of ZDEMO_REPORT", m)
		}
	case <-time.After(2 * time.Second):
		t.Fatalf("the report submit never reached the WebSocket (run returned %v)", err)
	}
}

// cliReadOnly is what vsp debug and vsp rfc read: read_only or SAP_READ_ONLY.
func TestCLIReadOnly_ConfigOrEnvironment(t *testing.T) {
	t.Setenv("SAP_READ_ONLY", "")
	if cliReadOnly(&systemParams{}) {
		t.Error("read-only with neither set")
	}
	if !cliReadOnly(&systemParams{ReadOnly: true}) {
		t.Error("read_only ignored")
	}
	t.Setenv("SAP_READ_ONLY", "true")
	if !cliReadOnly(&systemParams{}) {
		t.Error("SAP_READ_ONLY ignored")
	}
}
