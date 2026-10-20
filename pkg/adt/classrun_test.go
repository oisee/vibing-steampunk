package adt

import (
	"context"
	"io"
	"net/http"
	"strings"
	"sync"
	"testing"
)

// classrunRoute answers the package lookup for one class (in pkg) and the
// classrun POST with output and status.
func classrunRoute(uri, name, pkg, output string, status int) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.Contains(r.URL.Path, "informationsystem/search"):
			w.Header().Set("Content-Type", "application/xml")
			if pkg == "" {
				_, _ = io.WriteString(w, `<?xml version="1.0" encoding="UTF-8"?><adtcore:objectReferences xmlns:adtcore="http://www.sap.com/adt/core"/>`)
				return
			}
			_, _ = io.WriteString(w, searchXMLFor(uri, name, pkg))
		case r.Method == http.MethodPost && strings.HasPrefix(r.URL.Path, "/sap/bc/adt/oo/classrun/"):
			w.Header().Set("Content-Type", "text/plain")
			w.WriteHeader(status)
			_, _ = io.WriteString(w, output)
		default:
			w.WriteHeader(http.StatusOK)
		}
	}
}

func isClassrun(c wireCall) bool {
	return c.method == http.MethodPost && strings.HasPrefix(c.path, "/sap/bc/adt/oo/classrun/")
}

func countClassruns(calls []wireCall) int {
	n := 0
	for _, c := range calls {
		if isClassrun(c) {
			n++
		}
	}
	return n
}

const demoClassURI = "/sap/bc/adt/oo/classes/zcl_demo_run"

func TestRunClass_PostsAndReturnsOutput(t *testing.T) {
	rec := &adtRecorder{}
	client := newStubbedClient(t, rec, classrunRoute(demoClassURI, "ZCL_DEMO_RUN", "$TMP", "hello from main\n", http.StatusOK))

	out, err := client.RunClass(context.Background(), " zcl_demo_run ")
	if err != nil {
		t.Fatalf("RunClass: %v", err)
	}
	if out != "hello from main\n" {
		t.Errorf("output = %q, want the console text unchanged", out)
	}
	calls := rec.snapshot()
	if countClassruns(calls) != 1 {
		dumpCalls(t, calls)
		t.Fatal("want exactly one classrun POST")
	}
	for _, c := range calls {
		if isClassrun(c) && c.path != "/sap/bc/adt/oo/classrun/ZCL_DEMO_RUN" {
			t.Errorf("classrun path = %q, want the upper-case class name", c.path)
		}
	}
}

func TestRunClass_EmptyOutputIsNotAnError(t *testing.T) {
	rec := &adtRecorder{}
	client := newStubbedClient(t, rec, classrunRoute(demoClassURI, "ZCL_DEMO_RUN", "$TMP", "", http.StatusOK))

	out, err := client.RunClass(context.Background(), "ZCL_DEMO_RUN")
	if err != nil || out != "" {
		t.Fatalf("RunClass = %q, %v; want empty output and no error", out, err)
	}
}

func TestRunClass_ReadOnlyRefusesBeforeSending(t *testing.T) {
	rec := &adtRecorder{}
	client := newStubbedClient(t, rec, classrunRoute(demoClassURI, "ZCL_DEMO_RUN", "$TMP", "x", http.StatusOK), WithReadOnly())

	_, err := client.RunClass(context.Background(), "ZCL_DEMO_RUN")
	if err == nil || !strings.Contains(err.Error(), "safety configuration") {
		t.Fatalf("err = %v, want a refusal naming the safety configuration", err)
	}
	if calls := rec.snapshot(); len(calls) != 0 {
		dumpCalls(t, calls)
		t.Fatal("nothing may reach the server under --read-only")
	}
}

func TestRunClass_DisallowedWorkflowRefuses(t *testing.T) {
	rec := &adtRecorder{}
	client := newStubbedClient(t, rec, classrunRoute(demoClassURI, "ZCL_DEMO_RUN", "$TMP", "x", http.StatusOK),
		WithSafety(SafetyConfig{DisallowedOps: "W"}))

	if _, err := client.RunClass(context.Background(), "ZCL_DEMO_RUN"); err == nil {
		t.Fatal("--disallowed-ops W must refuse RunClass")
	}
	if calls := rec.snapshot(); len(calls) != 0 {
		dumpCalls(t, calls)
		t.Fatal("nothing may reach the server under --disallowed-ops W")
	}
}

func TestRunClass_AllowedPackageRuns(t *testing.T) {
	rec := &adtRecorder{}
	client := newStubbedClient(t, rec, classrunRoute(demoClassURI, "ZCL_DEMO_RUN", "$TMP", "ok", http.StatusOK),
		WithAllowedPackages("$TMP"))

	out, err := client.RunClass(context.Background(), "ZCL_DEMO_RUN")
	if err != nil || out != "ok" {
		t.Fatalf("RunClass = %q, %v; a class in $TMP must run with --allowed-packages $TMP", out, err)
	}
}

func TestRunClass_OtherPackageRefusedBeforeRun(t *testing.T) {
	rec := &adtRecorder{}
	client := newStubbedClient(t, rec, classrunRoute(demoClassURI, "ZCL_DEMO_RUN", "ZPROD", "x", http.StatusOK),
		WithAllowedPackages("$TMP"))

	if _, err := client.RunClass(context.Background(), "ZCL_DEMO_RUN"); err == nil {
		t.Fatal("a class outside the allowed packages must be refused")
	}
	if n := countClassruns(rec.snapshot()); n != 0 {
		t.Fatalf("%d classrun POST(s) for a class outside the allowed packages", n)
	}
}

func TestRunClass_UnknownPackageFailsClosed(t *testing.T) {
	rec := &adtRecorder{}
	client := newStubbedClient(t, rec, classrunRoute(demoClassURI, "ZCL_DEMO_RUN", "", "x", http.StatusOK),
		WithAllowedPackages("$TMP"))

	if _, err := client.RunClass(context.Background(), "ZCL_DEMO_RUN"); err == nil {
		t.Fatal("without package metadata RunClass must refuse (fail closed)")
	}
	if n := countClassruns(rec.snapshot()); n != 0 {
		t.Fatalf("%d classrun POST(s) although the package was unknown", n)
	}
}

func TestRunClass_InvalidNameSendsNothing(t *testing.T) {
	for _, name := range []string{"", "   ", "ZCL DEMO", "ZCL_DEMO;DROP", "ZCL_DEMO?x=1", "../ZCL"} {
		rec := &adtRecorder{}
		client := newStubbedClient(t, rec, classrunRoute(demoClassURI, "ZCL_DEMO_RUN", "$TMP", "x", http.StatusOK))
		if _, err := client.RunClass(context.Background(), name); err == nil {
			t.Errorf("RunClass(%q): want an error for an invalid class name", name)
		}
		if calls := rec.snapshot(); len(calls) != 0 {
			dumpCalls(t, calls)
			t.Errorf("RunClass(%q) sent requests for an invalid name", name)
		}
	}
}

func TestRunClass_NamespaceClassIsEscaped(t *testing.T) {
	// wireCall.path is the decoded path, so the escaped form is taken here
	// from the request as it arrived.
	var mu sync.Mutex
	var escaped []string
	inner := classrunRoute("/sap/bc/adt/oo/classes/%2fns%2fcl_x", "/NS/CL_X", "$TMP", "ns", http.StatusOK)
	route := func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost && strings.HasPrefix(r.URL.Path, "/sap/bc/adt/oo/classrun/") {
			mu.Lock()
			escaped = append(escaped, r.URL.EscapedPath())
			mu.Unlock()
		}
		inner(w, r)
	}
	rec := &adtRecorder{}
	client := newStubbedClient(t, rec, route)

	if _, err := client.RunClass(context.Background(), "/NS/CL_X"); err != nil {
		t.Fatalf("RunClass(/NS/CL_X): %v", err)
	}
	mu.Lock()
	defer mu.Unlock()
	if len(escaped) != 1 || escaped[0] != "/sap/bc/adt/oo/classrun/%2FNS%2FCL_X" {
		dumpCalls(t, rec.snapshot())
		t.Fatalf("classrun POST paths = %q, want one POST to /sap/bc/adt/oo/classrun/%%2FNS%%2FCL_X", escaped)
	}
}

func TestRunClass_SAPErrorIsReturned(t *testing.T) {
	for _, status := range []int{http.StatusNotFound, http.StatusInternalServerError} {
		rec := &adtRecorder{}
		client := newStubbedClient(t, rec, classrunRoute(demoClassURI, "ZCL_DEMO_RUN", "$TMP", "Class ZCL_DEMO_RUN does not exist", status))
		_, err := client.RunClass(context.Background(), "ZCL_DEMO_RUN")
		if err == nil {
			t.Errorf("status %d: want an error", status)
			continue
		}
		if !strings.Contains(err.Error(), "does not exist") {
			t.Errorf("status %d: error %q lacks the SAP text", status, err)
		}
	}
}
