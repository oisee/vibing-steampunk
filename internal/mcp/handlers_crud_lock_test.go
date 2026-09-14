package mcp

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"
)

// updateSourceWireCall records the HTTP-level sequence for the one handler
// whose automatic lock has to survive its own package policy check (#169).
// A mock of UpdateSource would miss the stateless SearchObject request that
// invalidates SAP's session-bound lock handle.
type updateSourceWireCall struct {
	method      string
	path        string
	query       url.Values
	sessionType string
}

type updateSourceWireRecorder struct {
	mu    sync.Mutex
	calls []updateSourceWireCall
}

func (r *updateSourceWireRecorder) record(req *http.Request) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.calls = append(r.calls, updateSourceWireCall{
		method:      req.Method,
		path:        req.URL.Path,
		query:       req.URL.Query(),
		sessionType: req.Header.Get("X-sap-adt-sessiontype"),
	})
}

func (r *updateSourceWireRecorder) snapshot() []updateSourceWireCall {
	r.mu.Lock()
	defer r.mu.Unlock()
	calls := make([]updateSourceWireCall, len(r.calls))
	copy(calls, r.calls)
	return calls
}

func (c updateSourceWireCall) isLock() bool {
	return c.method == http.MethodPost && c.query.Get("_action") == "LOCK"
}

func (c updateSourceWireCall) isUnlock() bool {
	return c.method == http.MethodPost && c.query.Get("_action") == "UNLOCK"
}

func (c updateSourceWireCall) isSourcePut() bool {
	return c.method == http.MethodPut && strings.HasSuffix(c.path, "/source/main")
}

func updateSourceSearchXML(uri, name, packageName string) string {
	return fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?>
<adtcore:objectReferences xmlns:adtcore="http://www.sap.com/adt/core">
  <adtcore:objectReference adtcore:uri="%s" adtcore:type="CLAS/OC" adtcore:name="%s" adtcore:packageName="%s"/>
</adtcore:objectReferences>`, uri, name, packageName)
}

const updateSourceLockXML = `<?xml version="1.0" encoding="UTF-8"?>
<asx:abap xmlns:asx="http://www.sap.com/abapxml" version="1.0"><asx:values><DATA>
<LOCK_HANDLE>HANDLE-1</LOCK_HANDLE><IS_LOCAL>X</IS_LOCAL>
</DATA></asx:values></asx:abap>`

func newUpdateSourceTestServer(t *testing.T, cfg *Config, objectURL, objectName, packageName string) (*Server, *updateSourceWireRecorder) {
	t.Helper()
	recorder := &updateSourceWireRecorder{}
	sap := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		recorder.record(req)
		w.Header().Set("X-CSRF-Token", "TOKEN")

		switch {
		case strings.Contains(req.URL.Path, "informationsystem/search"):
			w.Header().Set("Content-Type", "application/xml")
			_, _ = io.WriteString(w, updateSourceSearchXML(objectURL, objectName, packageName))
		case req.URL.Query().Get("_action") == "LOCK":
			w.Header().Set("Content-Type", "application/xml")
			_, _ = io.WriteString(w, updateSourceLockXML)
		default:
			w.WriteHeader(http.StatusOK)
		}
	}))
	t.Cleanup(sap.Close)

	cfg.BaseURL = sap.URL
	return NewServer(cfg), recorder
}

func updateSourceTestConfig() *Config {
	return &Config{
		Username: "TESTUSER",
		Password: "secret",
		Client:   "001",
		Language: "EN",
		Mode:     "expert",
	}
}

func updateSourceCall(t *testing.T, s *Server, args map[string]any) *httpResult {
	t.Helper()
	result, err := s.handleUpdateSource(context.Background(), newRequest(args))
	if err != nil {
		t.Fatalf("handleUpdateSource returned a transport error: %v", err)
	}
	return &httpResult{isError: result.IsError, text: toolResultText(t, result)}
}

// httpResult keeps the assertions focused on handler behaviour rather than the
// MCP result implementation details.
type httpResult struct {
	isError bool
	text    string
}

func indexUpdateSourceCall(calls []updateSourceWireCall, predicate func(updateSourceWireCall) bool) int {
	for i, call := range calls {
		if predicate(call) {
			return i
		}
	}
	return -1
}

func countUpdateSourceCalls(calls []updateSourceWireCall, predicate func(updateSourceWireCall) bool) int {
	count := 0
	for _, call := range calls {
		if predicate(call) {
			count++
		}
	}
	return count
}

func isPackageSearch(call updateSourceWireCall) bool {
	return strings.Contains(call.path, "informationsystem/search")
}

func TestHandleUpdateSource_AutoLockChecksPackageBeforeLock(t *testing.T) {
	const objectURL = "/sap/bc/adt/oo/classes/%2FZDEMO%2FCL_LOCK"
	cfg := updateSourceTestConfig()
	cfg.AllowedPackages = []string{"$TMP"}
	server, recorder := newUpdateSourceTestServer(t, cfg, objectURL, "/ZDEMO/CL_LOCK", "$TMP")

	result := updateSourceCall(t, server, map[string]any{
		"object_url": objectURL,
		"source":     "CLASS /ZDEMO/CL_LOCK DEFINITION PUBLIC. ENDCLASS.",
	})
	if result.isError {
		t.Fatalf("allowed package was rejected: %s", result.text)
	}

	calls := recorder.snapshot()
	search := indexUpdateSourceCall(calls, isPackageSearch)
	lock := indexUpdateSourceCall(calls, func(call updateSourceWireCall) bool { return call.isLock() })
	put := indexUpdateSourceCall(calls, func(call updateSourceWireCall) bool { return call.isSourcePut() })
	unlock := indexUpdateSourceCall(calls, func(call updateSourceWireCall) bool { return call.isUnlock() })
	if search < 0 || lock < 0 || put < 0 || unlock < 0 {
		t.Fatalf("expected package search, LOCK, PUT and UNLOCK; got %#v", calls)
	}
	if !(search < lock && lock < put && put < unlock) {
		t.Fatalf("package validation must finish before the lock and the write must remain one lock window; search=%d lock=%d put=%d unlock=%d calls=%#v", search, lock, put, unlock, calls)
	}
	if got := countUpdateSourceCalls(calls, func(call updateSourceWireCall) bool { return call.isSourcePut() }); got != 1 {
		t.Fatalf("PUT count = %d, want exactly one; calls=%#v", got, calls)
	}
	for _, call := range calls[lock+1 : unlock] {
		if call.sessionType != "stateful" {
			t.Errorf("request between LOCK and UNLOCK must stay stateful, got %+v", call)
		}
	}
}

func TestHandleUpdateSource_AutoLockRejectsBeforeLockOrPut(t *testing.T) {
	const objectURL = "/sap/bc/adt/programs/programs/ZDEMO_REJECT"

	tests := []struct {
		name         string
		configure    func(*Config)
		packageName  string
		args         map[string]any
		wantSearches int
	}{
		{
			name:        "package not allowed",
			configure:   func(cfg *Config) { cfg.AllowedPackages = []string{"$TMP"} },
			packageName: "ZOTHER",
			args: map[string]any{
				"object_url": objectURL,
				"source":     "REPORT zdemo_reject.",
			},
			wantSearches: 1,
		},
		{
			name: "read only",
			configure: func(cfg *Config) {
				cfg.ReadOnly = true
				cfg.AllowedPackages = []string{"$TMP"}
			},
			packageName: "$TMP",
			args: map[string]any{
				"object_url": objectURL,
				"source":     "REPORT zdemo_reject.",
			},
			wantSearches: 0,
		},
		{
			name: "update disabled",
			configure: func(cfg *Config) {
				cfg.DisallowedOps = "U"
				cfg.AllowedPackages = []string{"$TMP"}
			},
			packageName: "$TMP",
			args: map[string]any{
				"object_url": objectURL,
				"source":     "REPORT zdemo_reject.",
			},
			wantSearches: 0,
		},
		{
			name:        "transport policy",
			configure:   func(cfg *Config) { cfg.AllowedPackages = []string{"$TMP"} },
			packageName: "$TMP",
			args: map[string]any{
				"object_url": objectURL,
				"source":     "REPORT zdemo_reject.",
				"transport":  "TR-EXAMPLE",
			},
			wantSearches: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := updateSourceTestConfig()
			tt.configure(cfg)
			server, recorder := newUpdateSourceTestServer(t, cfg, objectURL, "ZDEMO_REJECT", tt.packageName)

			result := updateSourceCall(t, server, tt.args)
			if !result.isError {
				t.Fatalf("unsafe update succeeded: %s", result.text)
			}

			calls := recorder.snapshot()
			if got := countUpdateSourceCalls(calls, isPackageSearch); got != tt.wantSearches {
				t.Errorf("package-search count = %d, want %d; calls=%#v", got, tt.wantSearches, calls)
			}
			if got := countUpdateSourceCalls(calls, func(call updateSourceWireCall) bool { return call.isLock() }); got != 0 {
				t.Errorf("LOCK count = %d, want 0 after policy rejection; calls=%#v", got, calls)
			}
			if got := countUpdateSourceCalls(calls, func(call updateSourceWireCall) bool { return call.isSourcePut() }); got != 0 {
				t.Errorf("PUT count = %d, want 0 after policy rejection; calls=%#v", got, calls)
			}
		})
	}
}

func TestHandleUpdateSource_ManualLockHandleKeepsExistingPackageCheck(t *testing.T) {
	const objectURL = "/sap/bc/adt/programs/programs/ZDEMO_MANUAL"
	cfg := updateSourceTestConfig()
	cfg.AllowedPackages = []string{"$TMP"}
	server, recorder := newUpdateSourceTestServer(t, cfg, objectURL, "ZDEMO_MANUAL", "$TMP")

	result := updateSourceCall(t, server, map[string]any{
		"object_url":  objectURL,
		"source":      "REPORT zdemo_manual.",
		"lock_handle": "MANUAL-HANDLE",
	})
	if result.isError {
		t.Fatalf("manual-handle update was rejected: %s", result.text)
	}

	calls := recorder.snapshot()
	if got := countUpdateSourceCalls(calls, isPackageSearch); got != 1 {
		t.Errorf("manual handle path made %d package queries, want its existing one query only; calls=%#v", got, calls)
	}
	if got := countUpdateSourceCalls(calls, func(call updateSourceWireCall) bool { return call.isLock() || call.isUnlock() }); got != 0 {
		t.Errorf("manual handle path acquired or released a new lock %d time(s); calls=%#v", got, calls)
	}
}

func TestHandleUpdateSource_AutoLockWithoutPackageWhitelistKeepsExistingSuccessPath(t *testing.T) {
	const objectURL = "/sap/bc/adt/programs/programs/ZDEMO_UNRESTRICTED"
	server, recorder := newUpdateSourceTestServer(t, updateSourceTestConfig(), objectURL, "ZDEMO_UNRESTRICTED", "$TMP")

	result := updateSourceCall(t, server, map[string]any{
		"object_url": objectURL,
		"source":     "REPORT zdemo_unrestricted.",
	})
	if result.isError {
		t.Fatalf("unrestricted automatic update was rejected: %s", result.text)
	}

	calls := recorder.snapshot()
	if got := countUpdateSourceCalls(calls, isPackageSearch); got != 0 {
		t.Errorf("unrestricted update made %d package queries, want none; calls=%#v", got, calls)
	}
	if got := countUpdateSourceCalls(calls, func(call updateSourceWireCall) bool { return call.isLock() }); got != 1 {
		t.Errorf("LOCK count = %d, want 1; calls=%#v", got, calls)
	}
	if got := countUpdateSourceCalls(calls, func(call updateSourceWireCall) bool { return call.isSourcePut() }); got != 1 {
		t.Errorf("PUT count = %d, want 1; calls=%#v", got, calls)
	}
}
