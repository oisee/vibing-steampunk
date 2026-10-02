package adt

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"
)

const (
	testEnhoxhhURL = "/sap/bc/adt/enhancements/enhoxhh/zenh_demo"
	testEnhoxhbURL = "/sap/bc/adt/enhancements/enhoxhb/zenh_demo"
)

// testLockWithCorrNrXML is a LOCK answer for an object in a transportable
// package: the lock names the request the object is recorded in.
const testLockWithCorrNrXML = `<?xml version="1.0" encoding="UTF-8"?>
<asx:abap xmlns:asx="http://www.sap.com/abapxml" version="1.0"><asx:values><DATA>
<LOCK_HANDLE>HANDLE-1</LOCK_HANDLE><CORRNR>TR-EXAMPLE-2</CORRNR>
</DATA></asx:values></asx:abap>`

func testPluginOptions(pkg, transport, source string) SourceCodePluginOptions {
	return SourceCodePluginOptions{
		Name: "zenh_demo", Description: "Demo", Package: pkg, Transport: transport,
		ObjectURL: "/sap/bc/adt/functions/groups/zdemo", Option: `\FU:Z_DEMO\SE:BEGIN\EI`,
		Source: source,
	}
}

func testBadiOptions(pkg, transport string) BadiImplementationOptions {
	return BadiImplementationOptions{
		Name: "zenh_demo", Description: "Demo", Package: pkg, Transport: transport,
		Spot: "badi_x", ImplementingClass: "zcl_x",
	}
}

// --read-only refuses both creates before a single request leaves.
func TestEnhancementCreates_ReadOnlyRefusedBeforeAnyRequest(t *testing.T) {
	for name, create := range map[string]func(*Client) error{
		"source code plug-in": func(c *Client) error {
			_, err := c.CreateSourceCodePlugin(context.Background(), testPluginOptions("$TMP", "", ""))
			return err
		},
		"source code plug-in with code": func(c *Client) error {
			_, err := c.CreateSourceCodePlugin(context.Background(), testPluginOptions("$TMP", "", "WRITE 'x'."))
			return err
		},
		"BAdI implementation": func(c *Client) error {
			_, err := c.CreateBadiImplementation(context.Background(), testBadiOptions("$TMP", ""))
			return err
		},
	} {
		rec := &adtRecorder{}
		c := newStubbedClient(t, rec, func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
		}, WithReadOnly())
		if err := create(c); err == nil {
			t.Errorf("%s: created under --read-only", name)
		}
		if calls := rec.snapshot(); len(calls) > 0 {
			t.Errorf("%s: %d request(s) reached SAP before the refusal", name, len(calls))
			dumpCalls(t, calls)
		}
	}
}

// With --allowed-packages the code goes in without a package lookup between
// the LOCK and the PUT: the creation checked the package, and a stateless
// search inside the window retires the session the lock lives in (#91).
func TestCreateSourceCodePlugin_NoStatelessRequestBetweenLockAndPut(t *testing.T) {
	rec := &adtRecorder{}
	client := newStubbedClient(t, rec, func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.Contains(r.URL.Path, "informationsystem/search"):
			_, _ = io.WriteString(w, searchXMLFor(testEnhoxhhURL, "ZENH_DEMO", "ZPKG"))
		case strings.Contains(r.URL.Path, "/checkruns"):
			w.Header().Set("Content-Type", "application/vnd.sap.adt.checkmessages+xml")
			_, _ = io.WriteString(w, testEmptyCheckXML)
		case r.Method == http.MethodPost && r.URL.Query().Get("_action") == "LOCK":
			w.Header().Set("Content-Type", "application/vnd.sap.as+xml")
			_, _ = io.WriteString(w, testLockXML)
		case r.Method == http.MethodPost && r.URL.Path == enhoxhhCollection:
			w.WriteHeader(http.StatusCreated)
		default:
			w.WriteHeader(http.StatusOK)
		}
	}, WithAllowedPackages("Z*"), WithAllowTransportableEdits())

	if _, err := client.CreateSourceCodePlugin(context.Background(),
		testPluginOptions("ZPKG", "TR-EXAMPLE-1", "WRITE 'x'.")); err != nil {
		t.Fatalf("CreateSourceCodePlugin: %v", err)
	}

	calls := rec.snapshot()
	lockAt := indexOfCall(calls, isLock)
	putAt := indexOfCall(calls, isSourcePut)
	if lockAt < 0 || putAt < 0 || putAt < lockAt {
		t.Fatalf("expected a LOCK followed by a source PUT; trace:\n%v", calls)
	}
	assertWindowStateful(t, calls, lockAt, putAt)
}

// With no request named or chosen, the code goes with the request the lock
// names, as every other write under a lock does.
func TestCreateSourceCodePlugin_WriteCarriesTheLockTransport(t *testing.T) {
	rec := &adtRecorder{}
	client := newStubbedClient(t, rec, func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.Contains(r.URL.Path, "/checkruns"):
			w.Header().Set("Content-Type", "application/vnd.sap.adt.checkmessages+xml")
			_, _ = io.WriteString(w, testEmptyCheckXML)
		case r.Method == http.MethodPost && r.URL.Query().Get("_action") == "LOCK":
			w.Header().Set("Content-Type", "application/vnd.sap.as+xml")
			_, _ = io.WriteString(w, testLockWithCorrNrXML)
		case r.Method == http.MethodPost && r.URL.Path == enhoxhhCollection:
			w.WriteHeader(http.StatusCreated)
		default:
			w.WriteHeader(http.StatusOK)
		}
	}, WithTransportChoice("off"), WithAllowTransportableEdits())

	if _, err := client.CreateSourceCodePlugin(context.Background(),
		testPluginOptions("ZPKG", "", "WRITE 'x'.")); err != nil {
		t.Fatalf("CreateSourceCodePlugin: %v", err)
	}
	calls := rec.snapshot()
	putAt := indexOfCall(calls, isSourcePut)
	if putAt < 0 {
		t.Fatalf("no source PUT; trace:\n%v", calls)
	}
	if got := calls[putAt].query.Get("corrNr"); got != "TR-EXAMPLE-2" {
		t.Errorf("source PUT corrNr = %q, want the lock's TR-EXAMPLE-2", got)
		dumpCalls(t, calls)
	}
}

func TestCreateBadiImplementation_WriteCarriesTheLockTransport(t *testing.T) {
	rec := &adtRecorder{}
	client := newStubbedClient(t, rec, func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && r.URL.Query().Get("_action") == "LOCK":
			w.Header().Set("Content-Type", "application/vnd.sap.as+xml")
			_, _ = io.WriteString(w, testLockWithCorrNrXML)
		case r.Method == http.MethodPost && r.URL.Path == enhoxhbCollection:
			w.WriteHeader(http.StatusCreated)
		default:
			w.WriteHeader(http.StatusOK)
		}
	}, WithTransportChoice("off"), WithAllowTransportableEdits())

	if _, err := client.CreateBadiImplementation(context.Background(), testBadiOptions("ZPKG", "")); err != nil {
		t.Fatalf("CreateBadiImplementation: %v", err)
	}
	calls := rec.snapshot()
	putAt := indexOfCall(calls, func(c wireCall) bool {
		return c.method == http.MethodPut && c.path == testEnhoxhbURL
	})
	if putAt < 0 {
		t.Fatalf("no PUT of the implementation; trace:\n%v", calls)
	}
	if got := calls[putAt].query.Get("corrNr"); got != "TR-EXAMPLE-2" {
		t.Errorf("implementation PUT corrNr = %q, want the lock's TR-EXAMPLE-2", got)
		dumpCalls(t, calls)
	}
}

// The lock's request is still subject to the transportable-edit policy: when
// it is refused, nothing is written and the lock is released.
func TestCreateSourceCodePlugin_LockTransportRefusedReleasesTheLock(t *testing.T) {
	rec := &adtRecorder{}
	client := newStubbedClient(t, rec, func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.Contains(r.URL.Path, "/checkruns"):
			w.Header().Set("Content-Type", "application/vnd.sap.adt.checkmessages+xml")
			_, _ = io.WriteString(w, testEmptyCheckXML)
		case r.Method == http.MethodPost && r.URL.Query().Get("_action") == "LOCK":
			w.Header().Set("Content-Type", "application/vnd.sap.as+xml")
			_, _ = io.WriteString(w, testLockWithCorrNrXML)
		case r.Method == http.MethodPost && r.URL.Path == enhoxhhCollection:
			w.WriteHeader(http.StatusCreated)
		default:
			w.WriteHeader(http.StatusOK)
		}
	}, WithTransportChoice("off"))

	u, err := client.CreateSourceCodePlugin(context.Background(), testPluginOptions("ZPKG", "", "WRITE 'x'."))
	if err == nil || !strings.Contains(err.Error(), "TR-EXAMPLE-2") {
		t.Fatalf("expected the lock's request to be refused, got %v", err)
	}
	if u != testEnhoxhhURL {
		t.Errorf("the created ENHO is not reported: %q", u)
	}
	calls := rec.snapshot()
	if indexOfCall(calls, isSourcePut) >= 0 {
		t.Error("the code was written with a refused request")
	}
	if indexOfCall(calls, func(c wireCall) bool { return c.query.Get("_action") == "UNLOCK" }) < 0 {
		t.Error("the lock was not released")
		dumpCalls(t, calls)
	}
}

// A request named at creation goes out on the LOCK as well (#256), so the
// lock, the write and the creation all name the same request.
func TestEnhancementCreates_LockCarriesTheCreationTransport(t *testing.T) {
	for name, tc := range map[string]struct {
		lockPath string
		create   func(*Client) error
	}{
		"source code plug-in": {testEnhoxhhURL, func(c *Client) error {
			_, err := c.CreateSourceCodePlugin(context.Background(), testPluginOptions("ZPKG", "TR-EXAMPLE-1", "WRITE 'x'."))
			return err
		}},
		"BAdI implementation": {testEnhoxhbURL, func(c *Client) error {
			_, err := c.CreateBadiImplementation(context.Background(), testBadiOptions("ZPKG", "TR-EXAMPLE-1"))
			return err
		}},
	} {
		rec := &adtRecorder{}
		client := newStubbedClient(t, rec, func(w http.ResponseWriter, r *http.Request) {
			switch {
			case strings.Contains(r.URL.Path, "/checkruns"):
				w.Header().Set("Content-Type", "application/vnd.sap.adt.checkmessages+xml")
				_, _ = io.WriteString(w, testEmptyCheckXML)
			case r.Method == http.MethodPost && r.URL.Query().Get("_action") == "LOCK":
				w.Header().Set("Content-Type", "application/vnd.sap.as+xml")
				_, _ = io.WriteString(w, testLockWithCorrNrXML)
			case r.Method == http.MethodPost && (r.URL.Path == enhoxhhCollection || r.URL.Path == enhoxhbCollection):
				w.WriteHeader(http.StatusCreated)
			default:
				w.WriteHeader(http.StatusOK)
			}
		}, WithTransportChoice("off"), WithAllowTransportableEdits())

		if err := tc.create(client); err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		calls := rec.snapshot()
		lockAt := indexOfCall(calls, func(c wireCall) bool {
			return c.path == tc.lockPath && c.query.Get("_action") == "LOCK"
		})
		if lockAt < 0 {
			t.Fatalf("%s: no LOCK; trace:\n%v", name, calls)
		}
		if got := calls[lockAt].query.Get("corrNr"); got != "TR-EXAMPLE-1" {
			t.Errorf("%s: LOCK corrNr = %q, want the creation's TR-EXAMPLE-1", name, got)
			dumpCalls(t, calls)
		}
	}
}

// A transportable package with no request and transportable edits off is
// refused before the POST: no container is created to be stranded.
func TestCreateBadiImplementation_TransportablePackageRefusedBeforeThePOST(t *testing.T) {
	rec := &adtRecorder{}
	client := newStubbedClient(t, rec, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}, WithTransportChoice("off"))

	_, err := client.CreateBadiImplementation(context.Background(), testBadiOptions("ZPKG", ""))
	if err == nil || !strings.Contains(err.Error(), "transportable") {
		t.Fatalf("err = %v, want a transportable-edit refusal", err)
	}
	for _, c := range rec.snapshot() {
		if c.method == http.MethodPost && c.path == enhoxhbCollection {
			t.Fatalf("the container was POSTed before the refusal")
		}
	}
}

// The request the lock names is refused by the transport whitelist after the
// container exists. It is not deleted -- a DELETE writes to that same request
// -- and comes back with what to do by hand.
func TestCreateBadiImplementation_LockTransportRefusedKeepsTheContainer(t *testing.T) {
	rec := &adtRecorder{}
	client := newStubbedClient(t, rec, func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && r.URL.Query().Get("_action") == "LOCK":
			w.Header().Set("Content-Type", "application/vnd.sap.as+xml")
			_, _ = io.WriteString(w, testLockWithCorrNrXML)
		case r.Method == http.MethodPost && r.URL.Path == enhoxhbCollection:
			w.WriteHeader(http.StatusCreated)
		default:
			w.WriteHeader(http.StatusOK)
		}
	}, WithTransportChoice("off"), WithAllowTransportableEdits(), WithAllowedTransports("OTHER*"))

	u, err := client.CreateBadiImplementation(context.Background(), testBadiOptions("ZPKG", ""))
	if err == nil || !strings.Contains(err.Error(), "TR-EXAMPLE-2") || !strings.Contains(err.Error(), "empty ENHO") {
		t.Fatalf("err = %v, want the lock's request refused and the empty ENHO named", err)
	}
	if u != testEnhoxhbURL {
		t.Errorf("url = %q, want the container's", u)
	}
	for _, c := range rec.snapshot() {
		if c.method == http.MethodDelete || c.method == http.MethodPut {
			t.Errorf("%s %s after the refusal", c.method, c.path)
		}
	}
}
