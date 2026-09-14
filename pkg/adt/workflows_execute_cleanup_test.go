package adt

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
)

const successfulExecuteRunResult = `<?xml version="1.0" encoding="utf-8"?>
<aunit:runResult xmlns:aunit="http://www.sap.com/adt/aunit">
  <program name="ZTEMP_EXEC_TEST">
    <testClasses>
      <testClass name="LTC_EXECUTOR">
        <testMethods><testMethod name="EXECUTE_PAYLOAD" executionTime="0.01"><alerts/></testMethod></testMethods>
      </testClass>
    </testClasses>
  </program>
</aunit:runResult>`

// executeCleanupServer is a wire-level SAP stand-in for the ExecuteABAP path.
// Its independently configurable PUT, UNLOCK and DELETE responses let these
// tests prove which cleanup requests reach the transport after a failure.
type executeCleanupServer struct {
	putStatus    int
	unlockStatus int
	deleteStatus int
	cancelOnPut  context.CancelFunc

	mu    sync.Mutex
	calls map[string]int
}

func (s *executeCleanupServer) start(t *testing.T) *Client {
	t.Helper()
	s.calls = make(map[string]int)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.Contains(r.URL.Path, "/discovery"):
			s.record("discovery")
			w.Header().Set("X-CSRF-Token", "TOKEN")
			w.WriteHeader(http.StatusOK)

		case strings.Contains(r.URL.Path, "/activation"):
			s.record("activate")
			w.Header().Set("Content-Type", "application/xml")
			w.WriteHeader(http.StatusOK)

		case strings.Contains(r.URL.Path, "/abapunit/testruns"):
			s.record("testrun")
			w.Header().Set("Content-Type", "application/xml")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(successfulExecuteRunResult))

		case r.Method == http.MethodPost && r.URL.Query().Get("_action") == "LOCK":
			s.record("lock")
			w.Header().Set("Content-Type", "application/vnd.sap.as+xml")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`<?xml version="1.0" encoding="UTF-8"?>
<asx:abap xmlns:asx="http://www.sap.com/abapxml"><asx:values><DATA>
<LOCK_HANDLE>HANDLE-1</LOCK_HANDLE><MODIFICATION_SUPPORT>Modification</MODIFICATION_SUPPORT>
</DATA></asx:values></asx:abap>`))

		case r.Method == http.MethodPost && r.URL.Query().Get("_action") == "UNLOCK":
			s.record("unlock")
			w.WriteHeader(s.statusOrOK(s.unlockStatus))

		case r.Method == http.MethodPut && strings.HasSuffix(r.URL.Path, "/source/main"):
			s.record("put")
			if s.cancelOnPut != nil {
				s.cancelOnPut()
			}
			w.WriteHeader(s.statusOrOK(s.putStatus))

		case r.Method == http.MethodDelete:
			s.record("delete")
			w.WriteHeader(s.statusOrOK(s.deleteStatus))

		default:
			s.record(r.Method + " " + r.URL.Path)
			w.WriteHeader(http.StatusOK)
		}
	}))
	t.Cleanup(srv.Close)

	cfg := NewConfig(srv.URL, "TESTUSER", "secret")
	return NewClientWithTransport(cfg, NewTransport(cfg))
}

func (s *executeCleanupServer) statusOrOK(status int) int {
	if status == 0 {
		return http.StatusOK
	}
	return status
}

func (s *executeCleanupServer) record(call string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.calls[call]++
}

func (s *executeCleanupServer) count(call string) int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.calls[call]
}

func TestExecuteABAPCleanupSurvivesCallerCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	srv := &executeCleanupServer{putStatus: http.StatusInternalServerError, cancelOnPut: cancel}
	result, err := srv.start(t).ExecuteABAP(ctx, "lv_result = 'ok'.", nil)
	if err != nil {
		t.Fatalf("ExecuteABAP: %v", err)
	}
	if ctx.Err() == nil {
		t.Fatal("test setup did not cancel the caller context during PUT")
	}
	if !result.CleanedUp {
		t.Fatalf("CleanedUp = false after successful detached cleanup: %s", result.Message)
	}
	if got := srv.count("unlock"); got != 1 {
		t.Fatalf("UNLOCK requests = %d, want 1 after the cancelled PUT", got)
	}
	if got := srv.count("delete"); got != 1 {
		t.Fatalf("DELETE requests = %d, want 1 after caller cancellation", got)
	}
}

func TestExecuteABAPPreservesPutFailureWhenUnlockFails(t *testing.T) {
	srv := &executeCleanupServer{
		putStatus:    http.StatusInternalServerError,
		unlockStatus: http.StatusInternalServerError,
	}
	result, err := srv.start(t).ExecuteABAP(context.Background(), "lv_result = 'ok'.", nil)
	if err != nil {
		t.Fatalf("ExecuteABAP: %v", err)
	}
	if !strings.Contains(result.Message, "Failed to update source") {
		t.Fatalf("primary PUT failure was lost: %q", result.Message)
	}
	if !strings.Contains(result.Message, "was left LOCKED") || !strings.Contains(result.Message, "500") {
		t.Fatalf("UNLOCK failure and operator advice are not visible: %q", result.Message)
	}
	if got := srv.count("unlock"); got != 1 {
		t.Fatalf("UNLOCK requests = %d, want 1", got)
	}
}

func TestExecuteABAPDoesNotClaimCleanupAfterUnknownDelete(t *testing.T) {
	srv := &executeCleanupServer{deleteStatus: http.StatusInternalServerError}
	result, err := srv.start(t).ExecuteABAP(context.Background(), "lv_result = 'ok'.", nil)
	if err != nil {
		t.Fatalf("ExecuteABAP: %v", err)
	}
	if result.CleanedUp {
		t.Fatalf("CleanedUp = true after failed DELETE: %s", result.Message)
	}
	if got := srv.count("delete"); got != 1 {
		t.Fatalf("DELETE requests = %d, want exactly one for an unknown result", got)
	}
	if !strings.Contains(result.Message, "outcome is unknown and was not retried") {
		t.Fatalf("unknown DELETE result was not reported: %q", result.Message)
	}
}

func TestExecuteABAPCleanupAndKeepProgramRemainDistinct(t *testing.T) {
	t.Run("cleanup after normal execution", func(t *testing.T) {
		srv := &executeCleanupServer{}
		result, err := srv.start(t).ExecuteABAP(context.Background(), "lv_result = 'ok'.", nil)
		if err != nil {
			t.Fatalf("ExecuteABAP: %v", err)
		}
		if !result.Success || !result.CleanedUp {
			t.Fatalf("normal execution result = %+v, want success with successful DELETE", result)
		}
		if got := srv.count("delete"); got != 1 {
			t.Fatalf("DELETE requests = %d, want 1", got)
		}
	})

	t.Run("KeepProgram", func(t *testing.T) {
		srv := &executeCleanupServer{}
		result, err := srv.start(t).ExecuteABAP(context.Background(), "lv_result = 'ok'.", &ExecuteABAPOptions{KeepProgram: true})
		if err != nil {
			t.Fatalf("ExecuteABAP: %v", err)
		}
		if !result.Success || result.CleanedUp {
			t.Fatalf("KeepProgram result = %+v, want success without cleanup", result)
		}
		if got := srv.count("delete"); got != 0 {
			t.Fatalf("DELETE requests = %d, want none when KeepProgram is set", got)
		}
	})
}
