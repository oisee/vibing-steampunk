package adt

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

// --- Transport of copies ---

// fakeBridge stands in for ZADT_VSP's function bridge and records the calls.
type fakeBridge struct {
	mu    sync.Mutex
	calls []map[string]any
	subrc int
	// failFrom makes TR_COPY_COMM from these requests or tasks fail.
	failFrom map[string]bool
}

func (b *fakeBridge) CallRFC(_ context.Context, fn string, params map[string]any) (*RFCResult, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	rec := map[string]any{"_fn": fn}
	for k, v := range params {
		rec[k] = v
	}
	b.calls = append(b.calls, rec)
	if b.failFrom[fmt.Sprint(params["WI_TRKORR_FROM"])] {
		return &RFCResult{Subrc: 8, Message: "object list locked"}, nil
	}
	return &RFCResult{Subrc: b.subrc}, nil
}

// transportXML renders the organizer's answer for one request. Objects in a
// task appear in the task and, aggregated, in all_objects -- as ADT sends it.
func transportXML(number, desc, status string, tasks map[string][]string) string {
	var all, tk strings.Builder
	for task, objs := range tasks {
		fmt.Fprintf(&tk, `<tm:task tm:number="%s" tm:parent="%s" tm:owner="TESTUSER" tm:status="%s">`, task, number, status)
		for _, o := range objs {
			obj := fmt.Sprintf(`<tm:abap_object tm:pgmid="R3TR" tm:type="PROG" tm:name="%s"/>`, o)
			tk.WriteString(obj)
			all.WriteString(obj)
		}
		tk.WriteString(`</tm:task>`)
	}
	return fmt.Sprintf(`<?xml version="1.0" encoding="utf-8"?><tm:root xmlns:tm="http://www.sap.com/cts/adt/tm">`+
		`<tm:request tm:number="%s" tm:owner="TESTUSER" tm:desc="%s" tm:type="K" tm:status="%s">`+
		`<tm:all_objects>%s</tm:all_objects>%s</tm:request></tm:root>`, number, desc, status, all.String(), tk.String())
}

type tocServer struct {
	mu          sync.Mutex
	created     []string // create bodies
	released    []string // released numbers
	failRelease bool     // answer a release with a 500
	// objects answers the repository search: name -> {ADT type, package}.
	objects  map[string][2]string
	searches []string // names searched for
}

func newTocClient(t *testing.T, source string, opts ...Option) (*Client, *tocServer) {
	t.Helper()
	ts := &tocServer{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-CSRF-Token", "TOKEN")
		w.Header().Set("Content-Type", acceptTransportOrganizerV1)
		path := r.URL.Path
		switch {
		case r.Method == http.MethodPost && strings.HasSuffix(path, "/cts/transportrequests"):
			b, _ := io.ReadAll(r.Body)
			ts.mu.Lock()
			ts.created = append(ts.created, string(b))
			ts.mu.Unlock()
			_, _ = io.WriteString(w, `<?xml version="1.0" encoding="utf-8"?><tm:root xmlns:tm="http://www.sap.com/cts/adt/tm">`+
				`<tm:request tm:number="TR-TOC" tm:type="T"/></tm:root>`)
		case r.Method == http.MethodPost && (strings.HasSuffix(path, "/newreleasejobs") || strings.HasSuffix(path, "/relwithignlock")):
			ts.mu.Lock()
			fail := ts.failRelease
			if !fail {
				ts.released = append(ts.released, strings.TrimPrefix(path, "/sap/bc/adt/cts/transportrequests/"))
			}
			ts.mu.Unlock()
			if fail {
				http.Error(w, "release refused: target QAS not reachable", http.StatusInternalServerError)
				return
			}
			w.WriteHeader(http.StatusOK)
		case r.Method == http.MethodGet && strings.HasSuffix(path, "/informationsystem/search"):
			name := strings.ToUpper(r.URL.Query().Get("query"))
			ts.mu.Lock()
			ts.searches = append(ts.searches, name)
			hit, ok := ts.objects[name]
			ts.mu.Unlock()
			w.Header().Set("Content-Type", "application/xml")
			refs := ""
			if ok {
				refs = fmt.Sprintf(`<adtcore:objectReference adtcore:uri="/sap/bc/adt/x/%s" adtcore:type="%s" adtcore:name="%s" adtcore:packageName="%s"/>`,
					strings.ToLower(name), hit[0], name, hit[1])
			}
			_, _ = io.WriteString(w, `<?xml version="1.0" encoding="UTF-8"?><adtcore:objectReferences xmlns:adtcore="http://www.sap.com/adt/core">`+
				refs+`</adtcore:objectReferences>`)
		case r.Method == http.MethodGet && strings.HasSuffix(path, "/transportrequests/TR-TOC"):
			_, _ = io.WriteString(w, transportXML("TR-TOC", "ToC", "D", nil))
		case r.Method == http.MethodGet && strings.Contains(path, "/transportrequests/"):
			_, _ = io.WriteString(w, source)
		default:
			w.WriteHeader(http.StatusOK)
		}
	}))
	t.Cleanup(srv.Close)
	opts = append([]Option{WithEnableTransports()}, opts...)
	cfg := NewConfig(srv.URL, "TESTUSER", "secret", opts...)
	return NewClientWithTransport(cfg, NewTransport(cfg)), ts
}

func copiedFrom(b *fakeBridge) []string {
	b.mu.Lock()
	defer b.mu.Unlock()
	var out []string
	for _, c := range b.calls {
		if c["_fn"] == "TR_COPY_COMM" {
			out = append(out, fmt.Sprint(c["WI_TRKORR_FROM"]))
		}
	}
	return out
}

// A modifiable request keeps its objects in its tasks; TR_COPY_COMM on the
// request itself copies only the request's own entries (none), so each task
// with objects is copied, the way SE01's include-objects does it.
func TestCopyToTransportOfCopies_CopiesEachTaskOfAModifiableRequest(t *testing.T) {
	src := transportXML("TR-SRC", "[DEMO-1] Feature", "D", map[string][]string{
		"TR-TASK1": {"ZDEMO_A"},
		"TR-TASK2": {},
	})
	client, ts := newTocClient(t, src, WithCTSProject("PRJ_DEMO"))
	bridge := &fakeBridge{}

	res, err := client.copyToTransportOfCopies(context.Background(), bridge, "tr-src", TransportOfCopiesOptions{Target: "QAS"})
	if err != nil {
		t.Fatalf("copyToTransportOfCopies: %v", err)
	}
	if res.Transport != "TR-TOC" {
		t.Errorf("transport = %q, want TR-TOC", res.Transport)
	}
	if got := copiedFrom(bridge); len(got) != 1 || got[0] != "TR-TASK1" {
		t.Errorf("copied from %v, want only the task that holds objects [TR-TASK1]", got)
	}
	bridge.mu.Lock()
	call := bridge.calls[0]
	bridge.mu.Unlock()
	for k, want := range map[string]string{"WI_DIALOG": "", "WI_TRKORR_TO": "TR-TOC", "WI_WITHOUT_DOCUMENTATION": "X"} {
		if fmt.Sprint(call[k]) != want {
			t.Errorf("TR_COPY_COMM %s = %q, want %q", k, call[k], want)
		}
	}

	body := ts.created[0]
	for _, want := range []string{`tm:type="T"`, `tm:target="QAS"`, `tm:desc="ToC [DEMO-1] Feature"`, `tm:cts_project="PRJ_DEMO"`} {
		if !strings.Contains(body, want) {
			t.Errorf("create body lacks %s\nbody: %s", want, body)
		}
	}
	if strings.Contains(body, "tm:task") {
		t.Errorf("a transport of copies has no tasks, but the body asks for one: %s", body)
	}
	if len(ts.released) != 0 {
		t.Errorf("released %v without being asked", ts.released)
	}
}

// Release moves the tasks' entries into the request, so a released request
// is copied from the request alone -- copying its tasks too would double them.
func TestCopyToTransportOfCopies_CopiesAReleasedRequestFromTheRequest(t *testing.T) {
	src := transportXML("TR-SRC", "demo", "R", map[string][]string{"TR-TASK1": {"ZDEMO_A"}})
	client, _ := newTocClient(t, src)
	bridge := &fakeBridge{}

	if _, err := client.copyToTransportOfCopies(context.Background(), bridge, "TR-SRC", TransportOfCopiesOptions{Target: "QAS"}); err != nil {
		t.Fatalf("copyToTransportOfCopies: %v", err)
	}
	if got := copiedFrom(bridge); len(got) != 1 || got[0] != "TR-SRC" {
		t.Errorf("copied from %v, want [TR-SRC]", got)
	}
}

func TestCopyToTransportOfCopies_RefusesBeforeWriting(t *testing.T) {
	src := transportXML("TR-SRC", "demo", "D", map[string][]string{"TR-TASK1": {"ZDEMO_A"}})

	t.Run("no target", func(t *testing.T) {
		client, ts := newTocClient(t, src)
		if _, err := client.copyToTransportOfCopies(context.Background(), &fakeBridge{}, "TR-SRC", TransportOfCopiesOptions{}); err == nil {
			t.Fatal("accepted a transport of copies without a target")
		}
		if len(ts.created) != 0 {
			t.Error("created a request before refusing")
		}
	})
	t.Run("no bridge", func(t *testing.T) {
		client, ts := newTocClient(t, src)
		if _, err := client.CopyToTransportOfCopies(context.Background(), nil, "TR-SRC", TransportOfCopiesOptions{Target: "QAS"}); err == nil {
			t.Fatal("went ahead without the function bridge")
		}
		if len(ts.created) != 0 {
			t.Error("created a request it could not fill")
		}
	})
	t.Run("nothing to copy", func(t *testing.T) {
		empty := transportXML("TR-SRC", "demo", "D", map[string][]string{"TR-TASK1": {}})
		client, ts := newTocClient(t, empty)
		if _, err := client.copyToTransportOfCopies(context.Background(), &fakeBridge{}, "TR-SRC", TransportOfCopiesOptions{Target: "QAS"}); err == nil {
			t.Fatal("created an empty transport of copies")
		}
		if len(ts.created) != 0 {
			t.Error("created a request for nothing")
		}
	})
}

func TestCopyToTransportOfCopies_AFailedCopyNamesTheRequestItLeft(t *testing.T) {
	src := transportXML("TR-SRC", "demo", "D", map[string][]string{"TR-TASK1": {"ZDEMO_A"}})
	client, ts := newTocClient(t, src)

	res, err := client.copyToTransportOfCopies(context.Background(), &fakeBridge{subrc: 4}, "TR-SRC",
		TransportOfCopiesOptions{Target: "QAS", Release: true})
	if err == nil {
		t.Fatal("a failed TR_COPY_COMM was reported as success")
	}
	if res == nil || res.Transport != "TR-TOC" || !strings.Contains(err.Error(), "TR-TOC") {
		t.Errorf("the error must name the request that now exists: res=%+v err=%v", res, err)
	}
	if len(ts.released) != 0 {
		t.Error("released a transport of copies whose copy failed")
	}
}

func TestCopyToTransportOfCopies_ReleasesWhenAsked(t *testing.T) {
	src := transportXML("TR-SRC", "demo", "D", map[string][]string{"TR-TASK1": {"ZDEMO_A"}})
	client, ts := newTocClient(t, src)

	res, err := client.copyToTransportOfCopies(context.Background(), &fakeBridge{}, "TR-SRC",
		TransportOfCopiesOptions{Target: "QAS", Description: "own title", Release: true})
	if err != nil {
		t.Fatalf("copyToTransportOfCopies: %v", err)
	}
	// Its objects are locked in the original, so it is released ignoring
	// locks -- a plain release would come back as a question.
	if !res.Released || len(ts.released) != 1 || ts.released[0] != "TR-TOC/relwithignlock" {
		t.Errorf("released=%v calls=%v, want TR-TOC released with relwithignlock", res.Released, ts.released)
	}
	if !strings.Contains(ts.created[0], `tm:desc="own title"`) {
		t.Errorf("an explicit description was not used: %s", ts.created[0])
	}
}

func TestTransportOfCopiesDescription_FitsE07T(t *testing.T) {
	long := strings.Repeat("x", 80)
	if got := transportOfCopiesDescription(long); len([]rune(got)) != 60 || !strings.HasPrefix(got, "ToC ") {
		t.Errorf("description %q (%d runes), want 60 starting with \"ToC \"", got, len([]rune(got)))
	}
}

// One list that fails to copy does not stop the others, and the result is an
// error that names the transport of copies, what was copied, what was not,
// and that the release was not attempted -- never a success.
func TestCopyToTransportOfCopies_APartialCopyIsAnErrorThatSaysWhatWasDone(t *testing.T) {
	src := transportXML("TR-SRC", "demo", "D", map[string][]string{
		"TR-TASK1": {"ZDEMO_A"},
		"TR-TASK2": {"ZDEMO_B"},
	})
	client, ts := newTocClient(t, src)
	bridge := &fakeBridge{failFrom: map[string]bool{"TR-TASK2": true}}

	res, err := client.copyToTransportOfCopies(context.Background(), bridge, "TR-SRC",
		TransportOfCopiesOptions{Target: "QAS", Release: true})
	if err == nil {
		t.Fatal("a copy with a failed list was reported as success")
	}
	if got := copiedFrom(bridge); len(got) != 2 {
		t.Errorf("TR_COPY_COMM called from %v, want both tasks tried", got)
	}
	if res == nil || res.Transport != "TR-TOC" {
		t.Fatalf("result must name the transport of copies: %+v", res)
	}
	if len(res.CopiedFrom) != 1 || res.CopiedFrom[0] != "TR-TASK1" ||
		len(res.CopiedEntries) != 1 || res.CopiedEntries[0] != "R3TR PROG ZDEMO_A" {
		t.Errorf("copied from %v entries %v, want TR-TASK1 with R3TR PROG ZDEMO_A", res.CopiedFrom, res.CopiedEntries)
	}
	if len(res.Failed) != 1 || res.Failed[0].From != "TR-TASK2" ||
		len(res.Failed[0].Entries) != 1 || res.Failed[0].Entries[0] != "R3TR PROG ZDEMO_B" ||
		!strings.Contains(res.Failed[0].Error, "object list locked") {
		t.Errorf("failed = %+v, want TR-TASK2 with R3TR PROG ZDEMO_B and SAP's message", res.Failed)
	}
	if res.Released || !strings.HasPrefix(res.ReleaseStatus, "not attempted") {
		t.Errorf("released=%v status=%q, want not attempted", res.Released, res.ReleaseStatus)
	}
	if len(ts.released) != 0 {
		t.Errorf("released %v although the copy is incomplete", ts.released)
	}
	for _, want := range []string{"TR-TOC", "incomplete", "TR-TASK1", "R3TR PROG ZDEMO_A", "TR-TASK2", "R3TR PROG ZDEMO_B", "object list locked", "release: not attempted"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error lacks %q: %v", want, err)
		}
	}
}

// A release that fails after a complete copy is an error that says the
// transport of copies is filled and why it was not released.
func TestCopyToTransportOfCopies_AFailedReleaseIsAnError(t *testing.T) {
	src := transportXML("TR-SRC", "demo", "D", map[string][]string{"TR-TASK1": {"ZDEMO_A"}})
	client, ts := newTocClient(t, src)
	ts.mu.Lock()
	ts.failRelease = true
	ts.mu.Unlock()

	res, err := client.copyToTransportOfCopies(context.Background(), &fakeBridge{}, "TR-SRC",
		TransportOfCopiesOptions{Target: "QAS", Release: true})
	if err == nil {
		t.Fatal("a failed release was reported as success")
	}
	if res == nil || res.Released || !strings.HasPrefix(res.ReleaseStatus, "failed") {
		t.Fatalf("result = %+v, want release status failed", res)
	}
	if len(res.CopiedFrom) != 1 || len(res.Failed) != 0 {
		t.Errorf("copied %v failed %+v, want the copy itself complete", res.CopiedFrom, res.Failed)
	}
	for _, want := range []string{"TR-TOC", "R3TR PROG ZDEMO_A", "release: failed"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error lacks %q: %v", want, err)
		}
	}
}

// requestLevelXML is a modifiable request that holds entries itself, outside
// its task: an object and a CORR attribute. own lists them as the request's
// direct children, aggregate in all_objects -- ADT may send either or both.
func requestLevelXML(task []string, own, aggregate bool) string {
	var all, tk, req strings.Builder
	for _, o := range task {
		e := fmt.Sprintf(`<tm:abap_object tm:pgmid="R3TR" tm:type="PROG" tm:name="%s"/>`, o)
		tk.WriteString(e)
		all.WriteString(e)
	}
	for _, e := range []string{
		`<tm:abap_object tm:pgmid="R3TR" tm:type="TABU" tm:name="ZREQ_CONF"/>`,
		`<tm:abap_object tm:pgmid="CORR" tm:type="RELE" tm:name="TR-SRC"/>`,
	} {
		if aggregate {
			all.WriteString(e)
		}
		if own {
			req.WriteString(e)
		}
	}
	tasks := ""
	if len(task) > 0 {
		tasks = `<tm:task tm:number="TR-TASK1" tm:parent="TR-SRC" tm:owner="TESTUSER" tm:status="D">` + tk.String() + `</tm:task>`
	}
	return `<?xml version="1.0" encoding="utf-8"?><tm:root xmlns:tm="http://www.sap.com/cts/adt/tm">` +
		`<tm:request tm:number="TR-SRC" tm:owner="TESTUSER" tm:desc="demo" tm:type="K" tm:status="D">` +
		req.String() + `<tm:all_objects>` + all.String() + `</tm:all_objects>` + tasks + `</tm:request></tm:root>`
}

// A modifiable request's own entries are not copied -- its tasks' lists are
// -- and each is reported as skipped with the reason, never dropped silently.
func TestCopyToTransportOfCopies_RequestLevelEntriesAreSkippedWithANote(t *testing.T) {
	for _, c := range []struct{ own, aggregate bool }{{true, true}, {false, true}, {true, false}} {
		t.Run(fmt.Sprintf("apart=%v aggregated=%v", c.own, c.aggregate), func(t *testing.T) {
			client, _ := newTocClient(t, requestLevelXML([]string{"ZDEMO_A"}, c.own, c.aggregate))
			bridge := &fakeBridge{}
			res, err := client.copyToTransportOfCopies(context.Background(), bridge, "TR-SRC", TransportOfCopiesOptions{Target: "QAS"})
			if err != nil {
				t.Fatalf("copyToTransportOfCopies: %v", err)
			}
			if got := copiedFrom(bridge); len(got) != 1 || got[0] != "TR-TASK1" {
				t.Errorf("copied from %v, want only [TR-TASK1]", got)
			}
			skipped := map[string]string{}
			for _, s := range res.Skipped {
				skipped[s.Entry] = s.Reason
			}
			if len(skipped) != 2 {
				t.Fatalf("skipped = %+v, want the request's two own entries", res.Skipped)
			}
			if r := skipped["R3TR TABU ZREQ_CONF"]; !strings.Contains(r, "in no task") {
				t.Errorf("object entry skipped as %q, want the in-no-task note", r)
			}
			if r := skipped["CORR RELE TR-SRC"]; !strings.Contains(r, "request attribute") {
				t.Errorf("CORR entry skipped as %q, want the request-attribute note", r)
			}
			if _, ok := skipped["R3TR PROG ZDEMO_A"]; ok {
				t.Error("a task's entry was reported as skipped")
			}
		})
	}
}

// A modifiable request whose only entries are its own creates nothing and
// says which entries it did not copy, instead of "holds no objects".
func TestCopyToTransportOfCopies_OnlyRequestLevelEntriesRefusesNamingThem(t *testing.T) {
	client, ts := newTocClient(t, requestLevelXML(nil, true, true))
	_, err := client.copyToTransportOfCopies(context.Background(), &fakeBridge{}, "TR-SRC", TransportOfCopiesOptions{Target: "QAS"})
	if err == nil {
		t.Fatal("created a transport of copies with none of the request's entries")
	}
	for _, want := range []string{"R3TR TABU ZREQ_CONF", "CORR RELE TR-SRC"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error lacks %q: %v", want, err)
		}
	}
	if len(ts.created) != 0 {
		t.Error("created a request before refusing")
	}
}

// entriesXML is a modifiable request with one task holding the given entries
// ("PGMID TYPE NAME").
func entriesXML(entries ...string) string {
	var objs strings.Builder
	for _, e := range entries {
		f := strings.SplitN(e, " ", 3)
		fmt.Fprintf(&objs, `<tm:abap_object tm:pgmid="%s" tm:type="%s" tm:name="%s"/>`, f[0], f[1], f[2])
	}
	return `<?xml version="1.0" encoding="utf-8"?><tm:root xmlns:tm="http://www.sap.com/cts/adt/tm">` +
		`<tm:request tm:number="TR-SRC" tm:owner="TESTUSER" tm:desc="demo" tm:type="K" tm:status="D">` +
		`<tm:all_objects>` + objs.String() + `</tm:all_objects>` +
		`<tm:task tm:number="TR-TASK1" tm:parent="TR-SRC" tm:owner="TESTUSER" tm:status="D">` + objs.String() + `</tm:task>` +
		`</tm:request></tm:root>`
}

// Under --allowed-packages, a transport of copies carries no object from a
// package the server may not touch: the whole copy is refused before
// anything is created, naming every offending object.
func TestCopyToTransportOfCopies_RefusesObjectsOutsideTheAllowedPackages(t *testing.T) {
	src := entriesXML(
		"R3TR PROG ZDEMO_OK",
		"R3TR PROG ZSAP_OTHER",
		"LIMU METH ZCL_FOREIGN                   RUN",
		"LIMU FUNC Z_SOME_FM",
	)
	client, ts := newTocClient(t, src, WithAllowedPackages("ZDEMO*"))
	ts.objects = map[string][2]string{
		"ZDEMO_OK":    {"PROG/P", "ZDEMO_PKG"},
		"ZSAP_OTHER":  {"PROG/P", "ZOTHER_PKG"},
		"ZCL_FOREIGN": {"CLAS/OC", "$TMP"},
	}
	bridge := &fakeBridge{}

	_, err := client.copyToTransportOfCopies(context.Background(), bridge, "TR-SRC", TransportOfCopiesOptions{Target: "QAS"})
	if err == nil {
		t.Fatal("copied objects from packages outside the whitelist")
	}
	for _, want := range []string{"R3TR PROG ZSAP_OTHER", "ZOTHER_PKG", "LIMU METH ZCL_FOREIGN", "$TMP", "LIMU FUNC Z_SOME_FM"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("refusal does not name %q: %v", want, err)
		}
	}
	if strings.Contains(err.Error(), "R3TR PROG ZDEMO_OK") {
		t.Errorf("refusal names an allowed object: %v", err)
	}
	if len(ts.created) != 0 {
		t.Error("created the transport of copies before refusing")
	}
	if got := copiedFrom(bridge); len(got) != 0 {
		t.Errorf("TR_COPY_COMM called from %v after a refusal", got)
	}
}

func TestCopyToTransportOfCopies_AllowedPackagesPass(t *testing.T) {
	src := entriesXML("R3TR PROG ZDEMO_OK", "LIMU CINC ZCL_DEMO======================CCIMP", "LIMU METH ZCL_DEMO                      RUN")
	client, ts := newTocClient(t, src, WithAllowedPackages("ZDEMO*"))
	ts.objects = map[string][2]string{
		"ZDEMO_OK": {"PROG/P", "ZDEMO_PKG"},
		"ZCL_DEMO": {"CLAS/OC", "ZDEMO_PKG"},
	}
	if _, err := client.copyToTransportOfCopies(context.Background(), &fakeBridge{}, "TR-SRC", TransportOfCopiesOptions{Target: "QAS"}); err != nil {
		t.Fatalf("refused objects inside the whitelist: %v", err)
	}
	// The class is looked up once for its two LIMU entries.
	if len(ts.searches) != 2 {
		t.Errorf("searched %v, want ZDEMO_OK and ZCL_DEMO once each", ts.searches)
	}
}

// Table contents have no package: the rows of an SAP table that SE19 records
// for a PPF BAdI filter must not refuse the copy. They are copied without a
// lookup and named in the result; the repository objects beside them are
// still checked.
func TestCopyToTransportOfCopies_TableContentsAreNotPackageChecked(t *testing.T) {
	src := entriesXML("R3TR CLAS ZCL_DEMO", "R3TR TABU PPFTFLTCO1", "R3TR TDAT ZDEMO_V_CUST")
	client, ts := newTocClient(t, src, WithAllowedPackages("ZDEMO*"))
	ts.objects = map[string][2]string{"ZCL_DEMO": {"CLAS/OC", "ZDEMO_PKG"}}
	res, err := client.copyToTransportOfCopies(context.Background(), &fakeBridge{}, "TR-SRC", TransportOfCopiesOptions{Target: "QAS"})
	if err != nil {
		t.Fatalf("refused a copy holding table contents: %v", err)
	}
	if len(ts.searches) != 1 {
		t.Errorf("searched %v, want ZCL_DEMO only", ts.searches)
	}
	if got := strings.Join(res.PackageUnchecked, ", "); got != "R3TR TABU PPFTFLTCO1, R3TR TDAT ZDEMO_V_CUST" {
		t.Errorf("packageUnchecked = %q", got)
	}
}

// A repository object beside table contents is refused as before.
func TestCopyToTransportOfCopies_TableContentsDoNotHideAForeignObject(t *testing.T) {
	src := entriesXML("R3TR TABU PPFTFLTCO1", "R3TR PROG ZSAP_OTHER")
	client, ts := newTocClient(t, src, WithAllowedPackages("ZDEMO*"))
	ts.objects = map[string][2]string{"ZSAP_OTHER": {"PROG/P", "ZOTHER_PKG"}}
	_, err := client.copyToTransportOfCopies(context.Background(), &fakeBridge{}, "TR-SRC", TransportOfCopiesOptions{Target: "QAS"})
	if err == nil || !strings.Contains(err.Error(), "ZSAP_OTHER") || strings.Contains(err.Error(), "PPFTFLTCO1") {
		t.Errorf("err = %v, want a refusal naming ZSAP_OTHER only", err)
	}
	if len(ts.created) != 0 {
		t.Error("created the transport of copies before refusing")
	}
}

// Without a whitelist nothing changes: no package lookup is sent.
func TestCopyToTransportOfCopies_NoWhitelistSendsNoLookup(t *testing.T) {
	client, ts := newTocClient(t, entriesXML("R3TR PROG ZSAP_OTHER", "LIMU FUNC Z_SOME_FM"))
	if _, err := client.copyToTransportOfCopies(context.Background(), &fakeBridge{}, "TR-SRC", TransportOfCopiesOptions{Target: "QAS"}); err != nil {
		t.Fatalf("copyToTransportOfCopies: %v", err)
	}
	if len(ts.searches) != 0 {
		t.Errorf("sent package lookups %v without a whitelist", ts.searches)
	}
}
