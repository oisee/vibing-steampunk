package adt

import (
	"context"
	"net/http"
	"strings"
	"testing"
)

// Like abapfs #293, an existing lock can supply the request even when the
// HTTP CTS service is unavailable. Exercise both FUNC writes, not just the
// transport resolver, and forbid any CTS request in the lock window.
func TestFunctionModuleReusesLockTransport(t *testing.T) {
	for _, tc := range []struct {
		name, supplied, locked, want string
	}{
		{"lock request", "", "DEVK900001", "DEVK900001"},
		{"explicit wins", "DEVK900002", "DEVK900001", "DEVK900002"},
		{"local", "", "", ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			puts, unlocks := 0, 0
			rec := &lockQueryRecorder{respond: func(r *http.Request) (int, string) {
				switch {
				case strings.Contains(r.URL.Path, "/cts/"):
					t.Error("FUNC write must not depend on HTTP CTS")
					return http.StatusOK, ""
				case r.URL.Query().Get("_action") == "LOCK":
					return http.StatusOK, strings.Replace(lockHandleXML, "</DATA>", "<CORRNR>"+tc.locked+"</CORRNR></DATA>", 1)
				case r.URL.Query().Get("_action") == "UNLOCK":
					unlocks++
				case r.Method == http.MethodPut:
					puts++
					if got := r.URL.Query().Get("corrNr"); got != tc.want {
						t.Errorf("PUT %s corrNr = %q, want %q", r.URL.Path, got, tc.want)
					}
					if r.URL.Query().Get("lockHandle") != "LH-1" || r.Header.Get("X-sap-adt-sessiontype") != "stateful" {
						t.Error("PUT lost its lock handle or stateful header")
					}
				}
				return http.StatusOK, `<fmodule:abapFunctionModule xmlns:fmodule="http://www.sap.com/adt/functions/fmodules" xmlns:adtcore="http://www.sap.com/adt/core" adtcore:name="ZFM" fmodule:processingType="normal"/>`
			}}
			if err := transportableEditClient(rec).writeFunctionModule(context.Background(), "ZFG", "ZFM", "rfc", "FUNCTION zfm.\nENDFUNCTION.", tc.supplied); err != nil {
				t.Fatal(err)
			}
			if puts != 2 || unlocks != 1 {
				t.Fatalf("PUTs=%d UNLOCKs=%d, want 2 and 1", puts, unlocks)
			}
		})
	}
}

func TestFunctionModuleLockTransportHonoursPolicy(t *testing.T) {
	for _, allow := range []bool{false, true} {
		t.Run(map[bool]string{false: "transportable edits disabled", true: "request outside allowlist"}[allow], func(t *testing.T) {
			puts, unlocks := 0, 0
			rec := &lockQueryRecorder{respond: func(r *http.Request) (int, string) {
				switch {
				case r.URL.Query().Get("_action") == "LOCK":
					return http.StatusOK, strings.Replace(lockHandleXML, "</DATA>", "<CORRNR>DEVK900001</CORRNR></DATA>", 1)
				case r.URL.Query().Get("_action") == "UNLOCK":
					unlocks++
				case r.Method == http.MethodPut:
					puts++
				}
				return http.StatusOK, `<fmodule:abapFunctionModule xmlns:fmodule="http://www.sap.com/adt/functions/fmodules"/>`
			}}
			c := transportableEditClient(rec)
			c.config.Safety.AllowTransportableEdits = allow
			c.config.Safety.AllowedTransports = []string{"DEVK999999"}
			err := c.writeFunctionModule(context.Background(), "ZFG", "ZFM", "rfc", "FUNCTION zfm.\nENDFUNCTION.", "")
			if err == nil || puts != 0 || unlocks != 1 {
				t.Fatalf("err=%v PUTs=%d UNLOCKs=%d, want refusal, 0 and 1", err, puts, unlocks)
			}
		})
	}
}

// A known transport cannot repair a lock discarded by the server. Keep that
// failure visible and release the handle; never retry a failed source PUT.
func TestFunctionModuleKnownTransportDoesNotHideInvalidLock(t *testing.T) {
	puts, unlocks := 0, 0
	rec := &lockQueryRecorder{respond: func(r *http.Request) (int, string) {
		switch {
		case r.URL.Query().Get("_action") == "LOCK":
			return http.StatusOK, strings.Replace(lockHandleXML, "</DATA>", "<CORRNR>DEVK900001</CORRNR></DATA>", 1)
		case r.URL.Query().Get("_action") == "UNLOCK":
			unlocks++
		case r.Method == http.MethodPut:
			puts++
			return http.StatusLocked, `<exc:exception xmlns:exc="http://www.sap.com/abapxml/types/communicationframework"><type>ExceptionResourceInvalidLockHandle</type><message>invalid lock handle</message></exc:exception>`
		}
		return http.StatusOK, `<fmodule:abapFunctionModule xmlns:fmodule="http://www.sap.com/adt/functions/fmodules"/>`
	}}
	err := transportableEditClient(rec).writeFunctionModule(context.Background(), "ZFG", "ZFM", "", "FUNCTION zfm.\nENDFUNCTION.", "DEVK900001")
	if err == nil || !strings.Contains(err.Error(), "423") || puts != 1 || unlocks != 1 {
		t.Fatalf("err=%v PUTs=%d UNLOCKs=%d, want 423, 1 and 1", err, puts, unlocks)
	}
}
