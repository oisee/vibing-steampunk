package mcp

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
)

// --- DeleteObject's self-lock window (issue #238) ---
//
// With --allowed-packages set, DeleteObject's own gate resolves the package
// through a stateless search. The handler used to take its lock first, so that
// search landed inside the lock window, retired the session the handle belongs
// to, and the DELETE came back 423 ExceptionResourceInvalidLockHandle. The
// handler now gates before it locks, as UpdateSource's deploy path does.

type deleteCall struct {
	method, path, action, sessionType, lockHandle string
}

func (c deleteCall) String() string {
	return fmt.Sprintf("%-6s %s action=%q sessiontype=%q", c.method, c.path, c.action, c.sessionType)
}

func newDeleteTestServer(t *testing.T, pkg string) (*Server, func() []deleteCall) {
	t.Helper()
	return newDeleteTestServerWith(t, pkg, nil)
}

// newDeleteTestServerWith is newDeleteTestServer with an override: when
// override answers true it has written the response itself.
func newDeleteTestServerWith(t *testing.T, pkg string, override func(w http.ResponseWriter, r *http.Request) bool) (*Server, func() []deleteCall) {
	t.Helper()
	var mu sync.Mutex
	var calls []deleteCall

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		calls = append(calls, deleteCall{
			method:      r.Method,
			path:        r.URL.Path,
			action:      r.URL.Query().Get("_action"),
			sessionType: r.Header.Get("X-sap-adt-sessiontype"),
			lockHandle:  r.URL.Query().Get("lockHandle"),
		})
		mu.Unlock()

		w.Header().Set("X-CSRF-Token", "TOKEN")
		if override != nil && override(w, r) {
			return
		}
		switch {
		case strings.Contains(r.URL.Path, "informationsystem/search"):
			_, _ = io.WriteString(w, `<?xml version="1.0" encoding="UTF-8"?>
<adtcore:objectReferences xmlns:adtcore="http://www.sap.com/adt/core">
  <adtcore:objectReference adtcore:uri="/sap/bc/adt/programs/programs/zdemo_del" adtcore:type="PROG/P" adtcore:name="ZDEMO_DEL" adtcore:packageName="`+pkg+`"/>
</adtcore:objectReferences>`)
		case r.Method == http.MethodPost && r.URL.Query().Get("_action") == "LOCK":
			w.Header().Set("Content-Type", "application/vnd.sap.as+xml")
			_, _ = io.WriteString(w, `<?xml version="1.0" encoding="UTF-8"?>
<asx:abap xmlns:asx="http://www.sap.com/abapxml" version="1.0"><asx:values><DATA>
<LOCK_HANDLE>HANDLE-1</LOCK_HANDLE><IS_LOCAL>X</IS_LOCAL>
</DATA></asx:values></asx:abap>`)
		default:
			w.WriteHeader(http.StatusOK)
		}
	}))
	t.Cleanup(ts.Close)

	server := NewServer(&Config{
		BaseURL:            ts.URL,
		Username:           "u",
		Password:           "p",
		Client:             "001",
		Language:           "EN",
		InsecureSkipVerify: true,
		AllowedPackages:    []string{"$TMP"},
	})
	if server == nil {
		t.Fatal("NewServer returned nil")
	}
	return server, func() []deleteCall {
		mu.Lock()
		defer mu.Unlock()
		return append([]deleteCall(nil), calls...)
	}
}

func dumpDeleteCalls(t *testing.T, calls []deleteCall) {
	t.Helper()
	for i, c := range calls {
		t.Logf("  [%d] %s", i, c)
	}
}

func TestHandleDeleteObject_SelfLockChecksPackageBeforeLock(t *testing.T) {
	server, trace := newDeleteTestServer(t, "$TMP")

	res, err := server.handleDeleteObject(context.Background(), newRequest(map[string]any{
		"object_url": "/sap/bc/adt/programs/programs/ZDEMO_DEL",
	}))
	if err != nil {
		t.Fatalf("handleDeleteObject: %v", err)
	}

	calls := trace()
	if res.IsError {
		dumpDeleteCalls(t, calls)
		t.Fatalf("delete failed: %+v", res.Content)
	}

	lockAt, delAt, searchAt := -1, -1, -1
	for i, c := range calls {
		switch {
		case c.method == http.MethodPost && c.action == "LOCK":
			lockAt = i
		case c.method == http.MethodDelete:
			delAt = i
		case strings.Contains(c.path, "informationsystem/search"):
			searchAt = i
		}
	}
	if lockAt < 0 || delAt < lockAt {
		dumpDeleteCalls(t, calls)
		t.Fatal("expected a LOCK followed by a DELETE")
	}
	if searchAt < 0 || searchAt > lockAt {
		t.Errorf("package lookup at %d, LOCK at %d; want the lookup above the lock", searchAt, lockAt)
	}
	for _, c := range calls[lockAt+1 : delAt] {
		if c.sessionType != "stateful" {
			t.Errorf("request inside the lock window is not stateful: %s — it retires the "+
				"session the lock handle lives in, and the DELETE returns 423 (issue #238)", c)
		}
	}
	if t.Failed() {
		dumpDeleteCalls(t, calls)
	}
}

func TestHandleDeleteObject_ForeignPackageRefusedBeforeLock(t *testing.T) {
	server, trace := newDeleteTestServer(t, "ZSOMEONE_ELSE")

	res, err := server.handleDeleteObject(context.Background(), newRequest(map[string]any{
		"object_url": "/sap/bc/adt/programs/programs/ZDEMO_DEL",
	}))
	if err != nil {
		t.Fatalf("handleDeleteObject: %v", err)
	}
	if !res.IsError {
		t.Fatal("a delete outside the allowlist succeeded")
	}
	for i, c := range trace() {
		if c.action == "LOCK" || c.method == http.MethodDelete {
			t.Errorf("an object outside the allowlist was touched: [%d] %s", i, c)
		}
	}
}

// --- delete <TYPE> <NAME> in hyperfocused mode (issue #240) ---

func universalDelete(t *testing.T, s *Server, target string, params map[string]any) (string, bool) {
	t.Helper()
	args := map[string]any{"action": "delete", "target": target}
	if params != nil {
		args["params"] = params
	}
	res, err := s.handleUniversalTool(context.Background(), newRequest(args))
	if err != nil {
		t.Fatalf("handleUniversalTool: %v", err)
	}
	return resultText(res), res.IsError
}

// TestUniversalDeleteByTypeAndName pins #240: SAP(action="delete",
// target="PROG X") is routed, gates before it locks, locks, deletes and
// releases the lock after the DELETE -- without a lock_handle from the caller.
func TestUniversalDeleteByTypeAndName(t *testing.T) {
	server, trace := newDeleteTestServer(t, "$TMP")

	text, isErr := universalDelete(t, server, "PROG ZDEMO_DEL", nil)
	calls := trace()
	if isErr {
		dumpDeleteCalls(t, calls)
		t.Fatalf("delete PROG ZDEMO_DEL failed: %s", text)
	}

	lockAt, delAt, unlockAt, searchAt := -1, -1, -1, -1
	for i, c := range calls {
		switch {
		case c.method == http.MethodPost && c.action == "LOCK":
			lockAt = i
		case c.method == http.MethodPost && c.action == "UNLOCK":
			unlockAt = i
		case c.method == http.MethodDelete:
			delAt = i
			if want := "/sap/bc/adt/programs/programs/zdemo_del"; c.path != want {
				t.Errorf("DELETE went to %s, want %s", c.path, want)
			}
		case strings.Contains(c.path, "informationsystem/search"):
			if lockAt < 0 {
				searchAt = i
			}
		}
	}
	if searchAt < 0 || lockAt < 0 || searchAt > lockAt {
		t.Errorf("package lookup at %d, LOCK at %d; want the allowed-packages check above the lock", searchAt, lockAt)
	}
	if delAt < lockAt {
		t.Errorf("DELETE at %d, LOCK at %d; want LOCK then DELETE", delAt, lockAt)
	}
	if unlockAt < delAt {
		t.Errorf("UNLOCK at %d, DELETE at %d; want the lock released after the DELETE, or its ENQUEUE stays in SM12", unlockAt, delAt)
	}
	if t.Failed() {
		dumpDeleteCalls(t, calls)
	}
}

// TestUniversalDeleteByNameRespectsAllowedPackages: an object outside
// --allowed-packages is refused before it is locked.
func TestUniversalDeleteByNameRespectsAllowedPackages(t *testing.T) {
	server, trace := newDeleteTestServer(t, "ZSOMEONE_ELSE")

	text, isErr := universalDelete(t, server, "PROG ZDEMO_DEL", nil)
	if !isErr {
		t.Fatalf("a delete outside the allowlist succeeded: %s", text)
	}
	for i, c := range trace() {
		if c.action == "LOCK" || c.method == http.MethodDelete {
			t.Errorf("an object outside the allowlist was touched: [%d] %s", i, c)
		}
	}
}

// TestUniversalDeleteByNameRefusedReadOnly: --read-only refuses it before
// anything is locked or deleted.
func TestUniversalDeleteByNameRefusedReadOnly(t *testing.T) {
	fake, trace := newDeleteTestServer(t, "$TMP")
	server := NewServer(&Config{
		BaseURL: fake.config.BaseURL, Username: "u", Password: "p", Client: "001", Language: "EN",
		AllowedPackages: []string{"$TMP"}, ReadOnly: true,
	})

	text, isErr := universalDelete(t, server, "CLAS ZCL_DEMO_DEL", nil)
	low := strings.ToLower(text)
	if !isErr || !(strings.Contains(low, "read-only") || strings.Contains(low, "safety configuration")) {
		t.Errorf("delete under --read-only: want a refusal naming read-only or the safety configuration, got: %s", text)
	}
	for i, c := range trace() {
		if c.action == "LOCK" || c.method == http.MethodDelete {
			t.Errorf("--read-only let a delete through: [%d] %s", i, c)
		}
	}
}

// TestUniversalDeleteUnknownTypeSaysWhatWorks: a type with no delete here is
// not routed, and the answer lists the targets that are.
func TestUniversalDeleteUnknownTypeSaysWhatWorks(t *testing.T) {
	server, _ := newDeleteTestServer(t, "$TMP")
	text, isErr := universalDelete(t, server, "NOPE ZDEMO", nil)
	if !isErr || !strings.Contains(text, "<TYPE> <NAME>") || !strings.Contains(text, "PROG") {
		t.Errorf("unknown delete type: want the supported targets listed, got: %s", text)
	}
}

func TestDeleteURLByName(t *testing.T) {
	for _, tc := range []struct{ typ, name, parent, want string }{
		{"PROG", "ZDEMO", "", "/sap/bc/adt/programs/programs/zdemo"},
		{"CLAS", "ZCL_DEMO", "", "/sap/bc/adt/oo/classes/zcl_demo"},
		{"INCL", "ZDEMO_TOP", "", "/sap/bc/adt/programs/includes/ZDEMO_TOP"},
		{"FUNC", "Z_DEMO_FM", "ZDEMO_FG", "/sap/bc/adt/functions/groups/ZDEMO_FG/fmodules/Z_DEMO_FM"},
		{"STRUCT", "ZDEMO_S", "", "/sap/bc/adt/ddic/structures/zdemo_s"},
	} {
		got, ok := deleteURLByName(tc.typ, tc.name, tc.parent)
		if !ok || got != tc.want {
			t.Errorf("deleteURLByName(%s, %s, %s) = %q, %v; want %q", tc.typ, tc.name, tc.parent, got, ok, tc.want)
		}
	}
	for _, typ := range deleteNameTypes {
		parent := ""
		if typ == "FUNC" {
			parent = "ZDEMO_FG"
		}
		if got, ok := deleteURLByName(typ, "ZDEMO", parent); !ok || !strings.HasPrefix(got, "/sap/bc/adt/") {
			t.Errorf("%s is listed as deletable by name but has no URL: %q", typ, got)
		}
	}
}

// The expert DeleteObject tool with a lock_handle from an earlier call: the
// caller holds that lock, so the handler sends the DELETE with it and takes
// no lock of its own and releases none.
func TestHandleDeleteObject_SuppliedHandleNoLockNoUnlock(t *testing.T) {
	server, trace := newDeleteTestServer(t, "$TMP")
	res, err := server.handleDeleteObject(context.Background(), newRequest(map[string]any{
		"object_url":  "/sap/bc/adt/programs/programs/ZDEMO_DEL",
		"lock_handle": "CALLER-HANDLE",
	}))
	if err != nil {
		t.Fatal(err)
	}
	calls := trace()
	if res.IsError {
		dumpDeleteCalls(t, calls)
		t.Fatalf("delete failed: %s", resultText(res))
	}
	deletes := 0
	for i, c := range calls {
		switch {
		case c.action == "LOCK" || c.action == "UNLOCK":
			t.Errorf("a supplied handle must not be locked or released by the handler: [%d] %s", i, c)
		case c.method == http.MethodDelete:
			deletes++
			if c.lockHandle != "CALLER-HANDLE" {
				t.Errorf("DELETE carried lockHandle %q, want the caller's", c.lockHandle)
			}
		}
	}
	if deletes != 1 {
		dumpDeleteCalls(t, calls)
		t.Errorf("%d DELETEs, want 1", deletes)
	}
}

// When the UNLOCK after a successful DELETE fails, the answer says the object
// is deleted and the lock may stay, once each.
func TestHandleDeleteObject_FailedUnlockAfterDeleteSaidOnce(t *testing.T) {
	server, _ := newDeleteTestServerWith(t, "$TMP", func(w http.ResponseWriter, r *http.Request) bool {
		if r.Method == http.MethodPost && r.URL.Query().Get("_action") == "UNLOCK" {
			w.WriteHeader(http.StatusInternalServerError)
			return true
		}
		return false
	})
	text, isErr := universalDelete(t, server, "PROG ZDEMO_DEL", nil)
	if isErr {
		t.Fatalf("the DELETE succeeded; a failed UNLOCK after it is a note, not an error: %s", text)
	}
	if !strings.Contains(text, "may stay in SM12") {
		t.Errorf("want the stranded-lock note, got: %s", text)
	}
	if strings.Count(strings.ToLower(text), "deleted") != 1 {
		t.Errorf("\"deleted\" said more than once: %s", text)
	}
}

// delete FUNC <name> without a group looks the group up; when the search
// finds no such module the answer says to pass parent, and nothing is
// locked or deleted.
func TestUniversalDeleteFUNCLookupFails(t *testing.T) {
	server, trace := newDeleteTestServer(t, "$TMP") // the search answers a PROG, never a FUGR/FF
	text, isErr := universalDelete(t, server, "FUNC Z_DEMO_FM", nil)
	if !isErr || !strings.Contains(text, `"parent"`) {
		t.Errorf("want a refusal that names params.parent, got: %s", text)
	}
	for i, c := range trace() {
		if c.action == "LOCK" || c.method == http.MethodDelete {
			t.Errorf("a module whose group was not found was touched: [%d] %s", i, c)
		}
	}
}

// Names that are not object names are refused before a URL is built from
// them: a dot segment or a space would address something else.
func TestUniversalDeleteRefusesBadNames(t *testing.T) {
	server, trace := newDeleteTestServer(t, "$TMP")
	for _, target := range []string{"PROG .", "PROG ..", "CLAS ZCL_A ZCL_B", "PROG Z?X", "PROG ZA%2FB", "PROG ../ZX", "PROG //"} {
		text, isErr := universalDelete(t, server, target, nil)
		if !isErr || !strings.Contains(text, "not an object name") {
			t.Errorf("%s: want a refusal of the name, got: %s", target, text)
		}
	}
	for i, c := range trace() {
		if c.action == "LOCK" || c.method == http.MethodDelete || strings.Contains(c.path, "search") {
			t.Errorf("a refused name reached SAP: [%d] %s", i, c)
		}
	}
	for _, name := range []string{"ZDEMO", "/NS/CL_X", "$ZPKG_1", "Z_DEMO_FM"} {
		if !validDeleteName(name) {
			t.Errorf("validDeleteName(%q) = false; it is an object name", name)
		}
	}
}

// DEVC is not deleted by name: which package --allowed-packages checks for a
// package (itself or its parent) is not pinned, so the by-name form is not
// offered for it.
func TestUniversalDeleteDEVCNotByName(t *testing.T) {
	if deletableByName("DEVC") {
		t.Fatal("DEVC is deletable by name")
	}
}

// LockObject, then DeleteObject with that handle. With a handle supplied,
// DeleteObject's own gate resolves the package between the LOCK and the
// DELETE. This test pins the choice to send that lookup in the lock's
// stateful session while this client holds a lock. It asserts the session
// header, not that the handle survives: the fake does not model the
// stateless-request isolation that already protects the lock's context on
// SAP_BASIS 758. A report from 816 suggests that isolation alone may not be
// enough there, so the lookup does not rely on it.
func TestHandleDeleteObject_SuppliedHandleKeepsLookupInLockSession(t *testing.T) {
	server, trace := newDeleteTestServer(t, "$TMP")
	url := "/sap/bc/adt/programs/programs/ZDEMO_DEL"

	lockRes, err := server.handleLockObject(context.Background(), newRequest(map[string]any{"object_url": url}))
	if err != nil || lockRes.IsError {
		t.Fatalf("LockObject: %v %+v", err, lockRes)
	}
	res, err := server.handleDeleteObject(context.Background(), newRequest(map[string]any{
		"object_url":  url,
		"lock_handle": "HANDLE-1",
	}))
	if err != nil || res.IsError {
		t.Fatalf("DeleteObject: %v %+v", err, res)
	}

	calls := trace()
	lockAt, delAt := -1, -1
	for i, c := range calls {
		if c.action == "LOCK" {
			lockAt = i
		}
		if c.method == http.MethodDelete {
			delAt = i
		}
	}
	if lockAt < 0 || delAt < lockAt {
		dumpDeleteCalls(t, calls)
		t.Fatal("expected a LOCK followed by a DELETE")
	}
	between := calls[lockAt+1 : delAt]
	if len(between) == 0 {
		dumpDeleteCalls(t, calls)
		t.Fatal("expected the package lookup between LOCK and DELETE")
	}
	for _, c := range between {
		if c.sessionType != "stateful" {
			t.Errorf("request between LOCK and DELETE is not stateful: %s", c)
		}
	}
	if t.Failed() {
		dumpDeleteCalls(t, calls)
	}
}
