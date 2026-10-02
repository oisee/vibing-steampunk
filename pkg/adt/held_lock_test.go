package adt

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
)

// A write that fails inside its lock window releases the lock on a context of
// its own and, when that release fails too, says the object was left locked.
// The deferred release used to run on the write's ctx with its error dropped.
func TestWriteReportsALockItCouldNotRelease(t *testing.T) {
	var unlocks atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-CSRF-Token", "t")
		switch {
		case r.URL.Query().Get("_action") == "UNLOCK":
			unlocks.Add(1)
			w.WriteHeader(http.StatusInternalServerError)
		case r.URL.Query().Get("_action") == "LOCK":
			w.Header().Set("Content-Type", "application/vnd.sap.as+xml")
			_, _ = w.Write([]byte(`<?xml version="1.0" encoding="UTF-8"?>
<asx:abap xmlns:asx="http://www.sap.com/abapxml" version="1.0"><asx:values><DATA>
<LOCK_HANDLE>HANDLE-1</LOCK_HANDLE></DATA></asx:values></asx:abap>`))
		case r.Method == http.MethodPut:
			w.WriteHeader(http.StatusInternalServerError)
		case strings.Contains(r.URL.Path, "/checkruns"):
			_, _ = w.Write([]byte(`<?xml version="1.0" encoding="UTF-8"?><chkl:messages xmlns:chkl="http://www.sap.com/adt/checklist"/>`))
		case strings.Contains(r.URL.Path, "informationsystem/search"):
			_, _ = w.Write([]byte(`<?xml version="1.0" encoding="UTF-8"?>
<adtcore:objectReferences xmlns:adtcore="http://www.sap.com/adt/core">
  <adtcore:objectReference adtcore:uri="/sap/bc/adt/oo/interfaces/zif_demo" adtcore:type="INTF/OI" adtcore:name="ZIF_DEMO" adtcore:packageName="$TMP"/>
</adtcore:objectReferences>`))
		default:
			w.WriteHeader(http.StatusOK)
		}
	}))
	defer srv.Close()

	client := NewClient(srv.URL, "u", "p")
	result, err := client.WriteSource(context.Background(), "INTF", "ZIF_DEMO", "INTERFACE zif_demo PUBLIC.\nENDINTERFACE.",
		&WriteSourceOptions{Mode: WriteModeUpdate})
	if err != nil {
		t.Fatal(err)
	}
	if result.Success {
		t.Fatal("a failed PUT reported success")
	}
	if unlocks.Load() == 0 {
		t.Fatal("no UNLOCK was attempted after the failed PUT")
	}
	if !strings.Contains(result.Message, "left LOCKED") {
		t.Fatalf("a lock that could not be released went unreported: %q", result.Message)
	}
}

// An UNLOCK that SAP carried out but whose answer was lost is not a stranded
// lock. The retry then meets "invalid lock handle", which is the first
// UNLOCK's success seen from the other side, and the result must not claim
// the object was left locked.
func TestLostUnlockAnswerIsNotAStrandedLock(t *testing.T) {
	var unlocks atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-CSRF-Token", "t")
		switch {
		case r.URL.Query().Get("_action") == "UNLOCK":
			if unlocks.Add(1) == 1 {
				// Processed, then the connection drops before any answer.
				conn, _, err := w.(http.Hijacker).Hijack()
				if err == nil {
					_ = conn.Close()
				}
				return
			}
			w.WriteHeader(http.StatusLocked)
			_, _ = w.Write([]byte(`<?xml version="1.0" encoding="utf-8"?><exc:exception xmlns:exc="http://www.sap.com/abapxml/types/communicationframework">` +
				`<type id="ExceptionResourceInvalidLockHandle"/><message lang="EN">Resource is not locked (invalid lock handle)</message></exc:exception>`))
		case r.URL.Query().Get("_action") == "LOCK":
			w.Header().Set("Content-Type", "application/vnd.sap.as+xml")
			_, _ = w.Write([]byte(`<?xml version="1.0" encoding="UTF-8"?>
<asx:abap xmlns:asx="http://www.sap.com/abapxml" version="1.0"><asx:values><DATA>
<LOCK_HANDLE>HANDLE-1</LOCK_HANDLE></DATA></asx:values></asx:abap>`))
		case strings.Contains(r.URL.Path, "/checkruns"):
			_, _ = w.Write([]byte(`<?xml version="1.0" encoding="UTF-8"?><chkl:messages xmlns:chkl="http://www.sap.com/adt/checklist"/>`))
		case strings.Contains(r.URL.Path, "informationsystem/search"):
			_, _ = w.Write([]byte(`<?xml version="1.0" encoding="UTF-8"?>
<adtcore:objectReferences xmlns:adtcore="http://www.sap.com/adt/core">
  <adtcore:objectReference adtcore:uri="/sap/bc/adt/oo/interfaces/zif_demo" adtcore:type="INTF/OI" adtcore:name="ZIF_DEMO" adtcore:packageName="$TMP"/>
</adtcore:objectReferences>`))
		default:
			w.WriteHeader(http.StatusOK)
		}
	}))
	defer srv.Close()

	client := NewClient(srv.URL, "u", "p")
	result, err := client.WriteSource(context.Background(), "INTF", "ZIF_DEMO", "INTERFACE zif_demo PUBLIC.\nENDINTERFACE.",
		&WriteSourceOptions{Mode: WriteModeUpdate})
	if err != nil {
		t.Fatal(err)
	}
	if n := unlocks.Load(); n != 2 {
		t.Fatalf("want the UNLOCK whose answer was lost and one retry, got %d UNLOCKs (%q)", n, result.Message)
	}
	if strings.Contains(result.Message, "left LOCKED") || strings.Contains(result.Message, "may still be locked") {
		t.Fatalf("a lock SAP had released was reported as held: %q", result.Message)
	}
}

// When no UNLOCK gets an answer, the lock is in doubt, and the advice says
// that rather than "left LOCKED".
func TestUnconfirmedReleaseIsWordedAsInDoubt(t *testing.T) {
	var unlocks atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-CSRF-Token", "t")
		if r.URL.Query().Get("_action") != "UNLOCK" {
			return // the CSRF fetch
		}
		unlocks.Add(1)
		if conn, _, err := w.(http.Hijacker).Hijack(); err == nil {
			_ = conn.Close()
		}
	}))
	defer srv.Close()

	held := NewClient(srv.URL, "u", "p").holdLock("/sap/bc/adt/oo/interfaces/zif_demo", "HANDLE-1")
	if held.unlock(context.Background()) == nil {
		t.Fatal("a dropped connection must fail the UNLOCK")
	}
	advice := held.release(context.Background())
	if unlocks.Load() != 2 {
		t.Fatalf("want both UNLOCKs to reach the server, got %d", unlocks.Load())
	}
	if !strings.Contains(advice, "may still be locked") || strings.Contains(advice, "left LOCKED") {
		t.Fatalf("got %q", advice)
	}
}
