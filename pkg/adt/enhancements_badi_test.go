package adt

import (
	"context"
	"encoding/xml"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
)

// The container the POST creates names the spot and nothing else -- the form
// the editor's wizard sends, and the only one the server accepts.
func TestBadiImplementationBody_TheContainer(t *testing.T) {
	body := badiImplementationBody(BadiImplementationOptions{
		Name: "ZENH_DEMO", Description: `Demo & "A"`, Package: "ZPKG", Spot: "BADI_X", ImplementingClass: "ZCL_X",
	}, "DE", false)
	var doc struct {
		Name   string `xml:"name,attr"`
		Type   string `xml:"type,attr"`
		Desc   string `xml:"description,attr"`
		Common struct {
			ToolType string `xml:"toolType,attr"`
			Usages   struct {
				Refs []struct {
					Usage string `xml:"element_usage,attr"`
					Obj   struct {
						Name string `xml:"name,attr"`
						Type string `xml:"type,attr"`
					} `xml:"objectReference"`
				} `xml:"referencedObject"`
			} `xml:"usages"`
		} `xml:"contentCommon"`
	}
	if err := xml.Unmarshal([]byte(body), &doc); err != nil {
		t.Fatalf("not XML: %v\n%s", err, body)
	}
	if doc.Name != "ZENH_DEMO" || doc.Type != "ENHO/XHB" || doc.Desc != `Demo & "A"` || doc.Common.ToolType != "BADI_IMPL" {
		t.Errorf("header = %+v", doc)
	}
	refs := doc.Common.Usages.Refs
	if len(refs) != 1 || refs[0].Usage != "EXTO" || refs[0].Obj.Type != "ENHS/XS" || refs[0].Obj.Name != "BADI_X" {
		t.Errorf("usages = %+v", refs)
	}
	if !strings.Contains(body, "<enho:badiImplementations/>") || strings.Contains(body, "implementingClass") {
		t.Errorf("the container must carry no implementation: %s", body)
	}
}

func TestBadiImplementationBody_TheImplementation(t *testing.T) {
	body := badiImplementationBody(BadiImplementationOptions{
		Name: "ZENH_DEMO", Description: "d", Package: "ZPKG", Spot: "BADI_X", BadiDefinition: "BADI_X_ONE",
		ImplementingClass: "ZCL_X", Implementation: "ZIMPL", Inactive: true,
	}, "", true)
	for _, want := range []string{
		`enho:name="ZIMPL"`, `enho:active="false"`,
		`<enho:enhancementSpot adtcore:type="ENHS/XSB" adtcore:name="BADI_X"/>`,
		`<enho:badiDefinition adtcore:type="ENHS/XB" adtcore:name="BADI_X_ONE"/>`,
		`<enho:implementingClass adtcore:type="CLAS/OC" adtcore:name="ZCL_X"/>`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("body lacks %s:\n%s", want, body)
		}
	}
}

// Creation is a POST of the container, then the implementation written in
// under a lock.
func TestCreateBadiImplementation_PostsThenWritesUnderALock(t *testing.T) {
	var mu sync.Mutex
	var calls []string
	var put string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("x-csrf-token", "TOKEN")
		b, _ := io.ReadAll(r.Body)
		mu.Lock()
		defer mu.Unlock()
		action := r.URL.Query().Get("_action")
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/sap/bc/adt/enhancements/enhoxhb":
			calls = append(calls, "POST")
			w.WriteHeader(http.StatusCreated)
		case r.URL.Path == "/sap/bc/adt/enhancements/enhoxhb/zenh_demo" && action == "LOCK":
			calls = append(calls, "LOCK")
			w.Header().Set("Content-Type", "application/xml")
			_, _ = io.WriteString(w, `<?xml version="1.0"?><asx:abap xmlns:asx="http://www.sap.com/abapxml"><asx:values><DATA><LOCK_HANDLE>H1</LOCK_HANDLE></DATA></asx:values></asx:abap>`)
		case r.URL.Path == "/sap/bc/adt/enhancements/enhoxhb/zenh_demo" && r.Method == http.MethodPut:
			calls = append(calls, "PUT:"+r.URL.Query().Get("lockHandle"))
			put = string(b)
		case r.URL.Path == "/sap/bc/adt/enhancements/enhoxhb/zenh_demo" && action == "UNLOCK":
			calls = append(calls, "UNLOCK")
		default:
			w.WriteHeader(http.StatusOK)
		}
	}))
	t.Cleanup(srv.Close)
	c := NewClient(srv.URL, "TESTUSER", "pw")

	u, err := c.CreateBadiImplementation(context.Background(), BadiImplementationOptions{
		Name: "zenh_demo", Description: "Demo", Package: "$TMP", Spot: "badi_x", ImplementingClass: "zcl_x",
	})
	if err != nil {
		t.Fatalf("CreateBadiImplementation: %v", err)
	}
	if u != "/sap/bc/adt/enhancements/enhoxhb/zenh_demo" {
		t.Errorf("url = %s", u)
	}
	mu.Lock()
	defer mu.Unlock()
	if strings.Join(calls, ",") != "POST,LOCK,PUT:H1,UNLOCK" {
		t.Errorf("calls = %v", calls)
	}
	for _, want := range []string{`enho:name="ZENH_DEMO"`, `adtcore:name="ZCL_X"`, `adtcore:name="BADI_X"`, `enho:active="true"`} {
		if !strings.Contains(put, want) {
			t.Errorf("PUT lacks %s", want)
		}
	}
}

func TestCreateBadiImplementation_RefusesIncompleteInput(t *testing.T) {
	c := NewClient("http://127.0.0.1:1", "TESTUSER", "pw")
	for name, opts := range map[string]BadiImplementationOptions{
		"no spot":        {Name: "Z", Description: "d", Package: "$TMP", ImplementingClass: "ZCL"},
		"no class":       {Name: "Z", Description: "d", Package: "$TMP", Spot: "S"},
		"no description": {Name: "Z", Package: "$TMP", Spot: "S", ImplementingClass: "ZCL"},
	} {
		if _, err := c.CreateBadiImplementation(context.Background(), opts); err == nil {
			t.Errorf("%s: accepted", name)
		}
	}
}

// Writing the implementation is an update: a configuration that allows
// creating but not updating refuses before the POST, leaving no empty ENHO.
func TestCreateBadiImplementation_UpdateRefusedBeforeThePOST(t *testing.T) {
	var posts int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("x-csrf-token", "TOKEN")
		if r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/enhoxhb") {
			posts++
		}
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(srv.Close)
	cfg := NewConfig(srv.URL, "TESTUSER", "pw")
	cfg.Safety.DisallowedOps = "U"
	c := NewClientWithTransport(cfg, NewTransport(cfg))

	_, err := c.CreateBadiImplementation(context.Background(), BadiImplementationOptions{
		Name: "zenh_demo", Description: "Demo", Package: "$TMP", Spot: "badi_x", ImplementingClass: "zcl_x",
	})
	if err == nil {
		t.Fatal("an update-refusing configuration created the implementation")
	}
	if posts != 0 {
		t.Errorf("the container was POSTed %d times before the refusal", posts)
	}
}

// badiFailServer answers the POST and the LOCKs, refuses the PUT the way SAP
// does when the server side tries to show a dialog, and records the calls.
// deleteStatus is what the DELETE of the container answers.
func badiFailServer(t *testing.T, deleteStatus int) (*Client, func() []string) {
	t.Helper()
	var mu sync.Mutex
	var calls []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("x-csrf-token", "TOKEN")
		mu.Lock()
		defer mu.Unlock()
		action := r.URL.Query().Get("_action")
		const obj = "/sap/bc/adt/enhancements/enhoxhb/zenh_demo"
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/sap/bc/adt/enhancements/enhoxhb":
			calls = append(calls, "POST")
			w.WriteHeader(http.StatusCreated)
		case r.URL.Path == obj && action == "LOCK":
			calls = append(calls, "LOCK")
			w.Header().Set("Content-Type", "application/xml")
			_, _ = io.WriteString(w, `<?xml version="1.0"?><asx:abap xmlns:asx="http://www.sap.com/abapxml"><asx:values><DATA><LOCK_HANDLE>H1</LOCK_HANDLE></DATA></asx:values></asx:abap>`)
		case r.URL.Path == obj && action == "UNLOCK":
			calls = append(calls, "UNLOCK")
		case r.URL.Path == obj && r.Method == http.MethodPut:
			calls = append(calls, "PUT")
			w.Header().Set("Content-Type", "application/xml")
			w.WriteHeader(http.StatusBadRequest)
			_, _ = io.WriteString(w, `<?xml version="1.0" encoding="utf-8"?><exc:exception xmlns:exc="http://www.sap.com/abapxml/types/communicationframework">`+
				`<type id="ExceptionResourceAlreadyExists"/><message lang="EN">I::000 Sending of dynpro SAPLSPO1 0500 not possible: No window system type specified</message></exc:exception>`)
		case r.URL.Path == obj && r.Method == http.MethodDelete:
			calls = append(calls, "DELETE")
			w.WriteHeader(deleteStatus)
		default:
			w.WriteHeader(http.StatusOK)
		}
	}))
	t.Cleanup(srv.Close)
	return NewClient(srv.URL, "TESTUSER", "pw"), func() []string {
		mu.Lock()
		defer mu.Unlock()
		return append([]string(nil), calls...)
	}
}

// A dialog on the server side fails the PUT. The empty container this call
// created is deleted again, and the error says why the PUT failed.
func TestCreateBadiImplementation_FailedPUTTakesTheContainerAway(t *testing.T) {
	c, calls := badiFailServer(t, http.StatusOK)
	u, err := c.CreateBadiImplementation(context.Background(), BadiImplementationOptions{
		Name: "zenh_demo", Description: "Demo", Package: "$TMP", Spot: "badi_x", ImplementingClass: "zcl_x",
	})
	if err == nil {
		t.Fatal("a refused PUT reported success")
	}
	var pce *PartialCreateError
	if !errors.As(err, &pce) || !pce.CleanupOK {
		t.Fatalf("err = %v, want a PartialCreateError with the cleanup done", err)
	}
	if u != "" {
		t.Errorf("url = %q, want none: nothing is left behind", u)
	}
	for _, want := range []string{"ZENH_DEMO", "adding the implementation failed", "SAPLSPO1", "SAP-internal", "SE19"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error lacks %q: %v", want, err)
		}
	}
	got := strings.Join(calls(), ",")
	if !strings.HasPrefix(got, "POST,LOCK,PUT,UNLOCK,") || !strings.HasSuffix(got, "DELETE") {
		t.Errorf("calls = %s, want the PUT's lock released, then the container deleted", got)
	}
}

// When the container cannot be deleted, its URL comes back with what is left
// to do by hand.
func TestCreateBadiImplementation_UndeletableContainerIsReported(t *testing.T) {
	c, calls := badiFailServer(t, http.StatusForbidden)
	u, err := c.CreateBadiImplementation(context.Background(), BadiImplementationOptions{
		Name: "zenh_demo", Description: "Demo", Package: "$TMP", Spot: "badi_x", ImplementingClass: "zcl_x",
	})
	var pce *PartialCreateError
	if !errors.As(err, &pce) || pce.CleanupOK {
		t.Fatalf("err = %v, want a PartialCreateError with the cleanup not done", err)
	}
	if u != "/sap/bc/adt/enhancements/enhoxhb/zenh_demo" {
		t.Errorf("url = %q, want the container left behind", u)
	}
	if len(pce.ManualSteps) == 0 || !strings.Contains(pce.ManualSteps[0], "empty ENHO") || !strings.Contains(pce.ManualSteps[0], "SE19") {
		t.Errorf("manual steps = %v", pce.ManualSteps)
	}
	if got := strings.Join(calls(), ","); !strings.Contains(got, "DELETE") {
		t.Errorf("calls = %s, want a DELETE attempted", got)
	}
}

func TestDialogHint(t *testing.T) {
	if dialogHint(errors.New("status 400: I::000 Sending of dynpro SAPLSPO1 0500 not possible")) == "" {
		t.Error("no hint for a dialog")
	}
	if dialogHint(errors.New("status 403: no authorization")) != "" || dialogHint(nil) != "" {
		t.Error("a hint for something else")
	}
}

// Deleted within an open request, the container leaves an entry there; the
// result says so.
func TestCreateBadiImplementation_UndoNamesTheRequestEntry(t *testing.T) {
	c, _ := badiFailServer(t, http.StatusOK)
	c.config.Safety.AllowTransportableEdits = true
	c.config.Safety.TransportChoice = "off"
	_, err := c.CreateBadiImplementation(context.Background(), BadiImplementationOptions{
		Name: "zenh_demo", Description: "Demo", Package: "ZPKG", Transport: "TR-EXAMPLE", Spot: "badi_x", ImplementingClass: "zcl_x",
	})
	var pce *PartialCreateError
	if !errors.As(err, &pce) || !pce.CleanupOK {
		t.Fatalf("err = %v, want the container deleted", err)
	}
	if len(pce.ManualSteps) != 1 || !strings.Contains(pce.ManualSteps[0], "TR-EXAMPLE keeps an entry R3TR ENHO ZENH_DEMO") {
		t.Errorf("manual steps = %v", pce.ManualSteps)
	}
}
