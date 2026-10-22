package mcp

// The read-only invariant, as one test.
//
// --read-only has been extended path by path (#280, #283), each time after a
// write or an execution was found that it did not reach. This test walks the
// whole surface instead: every tool an expert-mode server registers, every
// SAP(action=...) the routers dispatch, and every analyze/i18n/revisions type
// in their tables, called with synthetic arguments against a fake SAP that
// answers everything and records what it is sent. It then asks:
//
//   - did anything write? No PUT, DELETE or PATCH, and no POST outside the
//     readPOSTs allowlist, each entry of which says why it only reads;
//   - did everything classified MUTATE or EXECUTE refuse, naming read-only or
//     the safety configuration;
//   - did a MUTATE or EXECUTE call dial the ZADT_VSP WebSocket or the RFC
//     gateway at all;
//   - is everything classified? A new tool, or a new case in a router, fails
//     with "classify me" until it is placed in readOnlyClasses.
//
// What it does not inspect: ZADT_VSP WebSocket and RFC traffic is checked as
// dials only. A MUTATE or EXECUTE call must not dial at all, but a READ call
// may, and what it then sends over the WebSocket or the RFC connection is not
// looked at: the fake WebSocket endpoint refuses the upgrade and the fake
// gateway hangs up, so nothing is sent there. A READ classification is
// therefore trusted for those channels.
//
// Known gaps are named in readOnlyClasses with their reason, reported and not
// failed. VSP_READONLY_TRACE=1 logs every call and what it sent.
//
// The classification tables (readOnlyClasses, actionCases, toolArgOverrides,
// packageGated) live in readonly_classes_test.go; this file is the harness.

import (
	"context"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/mark3labs/mcp-go/mcp"

	"github.com/oisee/vibing-steampunk/pkg/mcpext"
)

// --- the fake SAP -----------------------------------------------------------

// sentRequest is what the fake SAP saw.
type sentRequest struct {
	Method, Path, Query string
	CSRF, Upgrade       string
	SessionType         string
}

func (r sentRequest) String() string {
	s := r.Method + " " + r.Path
	if r.Query != "" {
		s += "?" + r.Query
	}
	if r.Upgrade != "" {
		s += " [upgrade " + r.Upgrade + "]"
	}
	return s
}

// fakeSAP answers every request plausibly — 200, a CSRF token when asked, a
// lock handle for a lock, source for a source path, a search hit in $TMP for a
// search — and records method, path and the headers that matter.
type fakeSAP struct {
	mu   sync.Mutex
	reqs []sentRequest
	srv  *httptest.Server
	// absent makes every object read a 404 and every repository search
	// empty, so a handler that checks for an object before writing — by
	// reading it or by searching for it — takes its create path. The run is
	// made in both worlds: a write can hide behind either answer.
	absent atomic.Bool
}

func newFakeSAP(t *testing.T) *fakeSAP {
	t.Helper()
	f := &fakeSAP{}
	f.srv = httptest.NewServer(http.HandlerFunc(f.serve))
	t.Cleanup(f.srv.Close)
	return f
}

const (
	fakeLockResult = `<?xml version="1.0" encoding="utf-8"?>` +
		`<asx:abap xmlns:asx="http://www.sap.com/abapxml" version="1.0"><asx:values><DATA>` +
		`<LOCK_HANDLE>FAKELOCKHANDLE</LOCK_HANDLE><CORRNR></CORRNR><IS_LOCAL>X</IS_LOCAL>` +
		`</DATA></asx:values></asx:abap>`
	fakeEmptyXML = `<?xml version="1.0" encoding="utf-8"?><root/>`
	fakeSource   = "REPORT zdemo_report.\nWRITE 'demo'.\n"
)

func (f *fakeSAP) serve(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	f.reqs = append(f.reqs, sentRequest{
		Method:      r.Method,
		Path:        r.URL.Path,
		Query:       r.URL.RawQuery,
		CSRF:        r.Header.Get("X-CSRF-Token"),
		Upgrade:     r.Header.Get("Upgrade"),
		SessionType: r.Header.Get("X-sap-adt-sessiontype"),
	})
	f.mu.Unlock()

	fetch := strings.EqualFold(r.Header.Get("X-CSRF-Token"), "fetch")
	if fetch {
		w.Header().Set("X-CSRF-Token", "FAKE-CSRF-TOKEN")
	}
	q := r.URL.Query()
	if f.absent.Load() && !fetch && (r.Method == http.MethodGet || r.Method == http.MethodHead) &&
		!strings.Contains(r.URL.Path, "/informationsystem/search") && !strings.Contains(r.URL.Path, "/discovery") {
		w.Header().Set("Content-Type", "application/xml")
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`<?xml version="1.0" encoding="utf-8"?><exc:exception xmlns:exc="http://www.sap.com/abapxml/types/communicationframework">` +
			`<type id="ExceptionResourceNotFound"/><message lang="EN">Resource does not exist</message></exc:exception>`))
		return
	}
	switch {
	case strings.EqualFold(q.Get("_action"), "LOCK"):
		w.Header().Set("Content-Type", "application/vnd.sap.as+xml")
		_, _ = w.Write([]byte(fakeLockResult))
	case strings.Contains(r.URL.Path, "/informationsystem/search"):
		w.Header().Set("Content-Type", "application/xml")
		if f.absent.Load() {
			_, _ = w.Write([]byte(`<?xml version="1.0" encoding="utf-8"?><adtcore:objectReferences xmlns:adtcore="http://www.sap.com/adt/core"/>`))
			return
		}
		_, _ = w.Write([]byte(fakeSearchHits(q.Get("query"))))
	case strings.Contains(r.URL.Path, "/source/"):
		w.Header().Set("Content-Type", "text/plain")
		_, _ = w.Write([]byte(fakeSource))
	case strings.Contains(r.Header.Get("Accept"), "json"):
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte("{}"))
	default:
		w.Header().Set("Content-Type", "application/xml")
		_, _ = w.Write([]byte(fakeEmptyXML))
	}
}

// fakeSearchKinds are the repository collections a search hit is offered in:
// whatever name is searched for exists as each of them, in $TMP, so that a
// package lookup by object URL resolves for any object a probe names.
var fakeSearchKinds = []struct{ collection, typ string }{
	{"programs/programs", "PROG/P"},
	{"programs/includes", "PROG/I"},
	{"oo/classes", "CLAS/OC"},
	{"oo/interfaces", "INTF/OI"},
	{"functions/groups", "FUGR/F"},
	{"ddic/dataelements", "DTEL/DE"},
	{"ddic/domains", "DOMA/DD"},
	{"ddic/tables", "TABL/DT"},
	{"ddic/ddl/sources", "DDLS/DF"},
	{"messageclass", "MSAG/N"},
}

// fakeSearchHits answers a repository search for query with one hit per
// fakeSearchKinds entry, all in $TMP.
func fakeSearchHits(query string) string {
	name := strings.ToUpper(strings.Trim(strings.TrimSpace(query), "*"))
	if name == "" {
		name = "ZDEMO_REPORT"
	}
	var b strings.Builder
	b.WriteString(`<?xml version="1.0" encoding="utf-8"?><adtcore:objectReferences xmlns:adtcore="http://www.sap.com/adt/core">`)
	for _, k := range fakeSearchKinds {
		fmt.Fprintf(&b, `<adtcore:objectReference adtcore:uri="/sap/bc/adt/%s/%s" adtcore:type="%s" adtcore:name="%s" adtcore:packageName="$TMP" adtcore:description="Demo"/>`,
			k.collection, strings.ToLower(name), k.typ, name)
	}
	b.WriteString(`</adtcore:objectReferences>`)
	return b.String()
}

func (f *fakeSAP) take() []sentRequest {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := f.reqs
	f.reqs = nil
	return out
}

// fakeRFCGateway accepts TCP connections, counts them and hangs up: it shows
// whether a call got as far as dialling the gateway, nothing more.
type fakeRFCGateway struct {
	port  int
	dials atomic.Int64
}

func newFakeRFCGateway(t *testing.T) *fakeRFCGateway {
	t.Helper()
	ln, err := (&net.ListenConfig{}).Listen(context.Background(), "tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	t.Cleanup(func() { _ = ln.Close() })
	g := &fakeRFCGateway{port: ln.Addr().(*net.TCPAddr).Port}
	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			g.dials.Add(1)
			_ = conn.Close()
		}
	}()
	return g
}

// --- POST allowlist ---------------------------------------------------------

// readPOST is a POST that reads: ADT puts a request body on a number of reads,
// so a POST alone is not a write. Every entry says why it changes nothing.
type readPOST struct {
	re  *regexp.Regexp
	why string
}

var readPOSTs = []readPOST{
	{regexp.MustCompile(`^/sap/bc/adt/repository/informationsystem/usageReferences`), "where-used list: the scope travels in the body; nothing is stored"},
	{regexp.MustCompile(`^/sap/bc/adt/repository/nodestructure`), "package tree listing; ADT takes its filter as a POST"},
	{regexp.MustCompile(`^/sap/bc/adt/datapreview/(freestyle|ddic)`), "data preview: SELECT only, the statement in the body; --block-free-sql governs free SQL"},
	{regexp.MustCompile(`^/sap/bc/adt/checkruns`), "syntax check of the source in the body; reports diagnostics, stores nothing"},
	{regexp.MustCompile(`^/sap/bc/adt/abapunit/testruns`), "ABAP Unit run; under --read-only only harmless tests run, dangerous/critical ones are refused in the client (#283)"},
	{regexp.MustCompile(`^/sap/bc/adt/atc/(runs|worklists)`), "ATC run: checks objects and records its findings as a worklist; changes no object"},
	{regexp.MustCompile(`^/sap/bc/adt/navigation/target`), "find definition for a position in the source in the body"},
	{regexp.MustCompile(`^/sap/bc/adt/abapsource/codecompletion/`), "code completion proposals for the source in the body"},
	{regexp.MustCompile(`^/sap/bc/adt/abapsource/prettyprinter(\?|$)`), "formats the source in the body and returns it; stores nothing (settings are a separate PUT, refused)"},
	{regexp.MustCompile(`^/sap/bc/adt/abapsource/typehierarchy`), "type hierarchy for a position in the source in the body"},
	{regexp.MustCompile(`^/sap/bc/adt/cts/transportchecks`), "transport check: asks which request an object would go to, creates none"},
	{regexp.MustCompile(`^/sap/bc/adt/[^?]*\?(.*&)?_action=UNLOCK(&|$)`), "releases a lock this session holds; READ locks stay allowed under --read-only, so their release must too"},
	{regexp.MustCompile(`^/sap/bc/adt/debugger/listeners`), "waits for a debuggee of this user; registers no breakpoint and changes no program"},
	{regexp.MustCompile(`^/sap/bc/adt/debugger\?(.*&)?method=(attach|detach|getStack|getVariables|getChildVariables)(&|$)`), "debugger protocol verbs that take, read or release a stopped debuggee; stepping and variable writes are not on this list"},
}

func isReadPOST(r sentRequest) (string, bool) {
	target := r.Path
	if r.Query != "" {
		target += "?" + r.Query
	}
	for _, p := range readPOSTs {
		if p.re.MatchString(target) {
			return p.why, true
		}
	}
	return "", false
}

// isWrite reports whether a request could change the system.
func isWrite(r sentRequest) bool {
	switch r.Method {
	case http.MethodGet, http.MethodHead, http.MethodOptions:
		return false
	case http.MethodPost:
		_, ok := isReadPOST(r)
		return !ok
	}
	return true // PUT, DELETE, PATCH and anything unusual
}

// --- synthetic arguments ----------------------------------------------------

const (
	synthObjectURL = "/sap/bc/adt/programs/programs/zdemo_report"
	synthClassURL  = "/sap/bc/adt/oo/classes/zcl_demo"
)

// synthValue gives a plausible value for a parameter by its name, in the
// repository's sanitised vocabulary: $TMP, ZDEMO_*, TR-EXAMPLE. ok=false
// leaves the parameter out.
func synthValue(name, typ, dir string) (any, bool) {
	switch name {
	// Left out: they steer a call to a narrower path (a method, an include)
	// or route it elsewhere, and the plain call is the one worth making.
	case "type", "op", "method", "include", "variant", "condition", "target", "secondary_id", "expected_source_hash":
		return nil, false
	case "object_url", "object_uri", "uri", "test_object_uri", "config_uri", "object":
		return synthObjectURL, true
	case "class_url":
		return synthClassURL, true
	case "source_url":
		return synthObjectURL + "/source/main", true
	case "version_uri", "version1_uri", "version2_uri":
		return synthObjectURL + "/source/main/versions/00001/content", true
	case "source", "class_source", "code", "content":
		return fakeSource, true
	case "test_source":
		return "CLASS ltc_demo DEFINITION FOR TESTING. ENDCLASS.\nCLASS ltc_demo IMPLEMENTATION. ENDCLASS.", true
	case "package", "package_name", "packageName", "dev_class", "new_package", "context_package":
		return "$TMP", true
	case "transport":
		return "TR-EXAMPLE", true
	case "mode":
		return "upsert", true
	case "name", "object_name", "objectName", "name1", "name2", "source_name", "oldName",
		"program_name", "program", "report", "main_program":
		return "ZDEMO_REPORT", true
	case "newName", "target_name":
		return "ZDEMO_REPORT2", true
	case "object_type", "objType", "type1", "type2":
		return "PROG", true
	case "class_name":
		return "ZCL_DEMO", true
	case "interface_name":
		return "ZIF_DEMO", true
	case "function_name", "function":
		return "Z_DEMO_FM", true
	case "function_group", "parent", "parent1", "parent2", "parent_name":
		return "ZDEMO_FG", true
	case "include_name":
		return "ZDEMO_INCL", true
	case "table_name", "structure_name":
		return "ZDEMO_TAB", true
	case "view_name", "ddls_name":
		return "ZDEMO_DDLS", true
	case "message_class":
		return "ZDEMO_MSG", true
	case "transaction_name":
		return "ZDEMO_TCODE", true
	case "type_name":
		return "ZDEMO_TYPE", true
	case "service_definition", "service_name":
		return "ZDEMO_SRV", true
	case "app_name":
		return "ZDEMO_APP", true
	case "iam_app", "iam_apps":
		return "ZDEMO_IAM", true
	case "business_catalog":
		return "ZDEMO_BC", true
	case "include_type":
		return "testclasses", true
	case "language", "source_language":
		return "EN", true
	case "target_language":
		return "DE", true
	case "description", "short", "medium", "long", "heading":
		return "Demo", true
	case "query", "name_filter":
		return "ZDEMO*", true
	case "pattern":
		return "WRITE", true
	case "sql_query":
		return "SELECT * FROM T000", true
	case "user", "trace_user", "user_name":
		return "TESTUSER", true
	case "access_mode":
		return "MODIFY", true
	case "lock_handle":
		return "FAKELOCKHANDLE", true
	case "text_symbols", "selection_texts", "heading_texts":
		return `{"001":"Demo"}`, true
	case "params":
		return "{}", true
	case "old_string":
		return "demo", true
	case "new_string":
		return "demo2", true
	case "since", "released_from":
		return "20260101", true
	case "until", "released_to":
		return "20261231", true
	case "risk_level":
		return "harmless", true
	case "step_type":
		return "stepOver", true
	case "file_path":
		return filepath.Join(dir, "zdemo_report.prog.abap"), true
	case "output_dir", "outputPath":
		return dir, true
	case "kind":
		return "line", true
	case "direction":
		return "callers", true
	}
	switch typ {
	case "number", "integer":
		return 1.0, true
	case "boolean":
		return false, true
	case "array":
		return []any{"ZDEMO_REPORT"}, true
	case "object":
		return map[string]any{}, true
	}
	return "ZDEMO_OBJ", true
}

// synthArgs fills every parameter a tool declares.
func synthArgs(tool mcp.Tool, dir string) map[string]any {
	args := map[string]any{}
	for name, raw := range tool.InputSchema.Properties {
		m, _ := raw.(map[string]any)
		typ, _ := m["type"].(string)
		if v, ok := synthValue(name, typ, dir); ok {
			args[name] = v
		}
	}
	for k, v := range toolArgOverrides[tool.Name] {
		args[k] = v
	}
	return args
}

// synthBag is every synthetic parameter at once, for a handler that is not a
// tool and so declares no schema.
func synthBag(dir string) map[string]any {
	names := []string{
		"object_url", "class_url", "source", "test_source", "package", "package_name", "transport",
		"name", "object_name", "object_type", "class_name", "program_name", "program", "report",
		"function_name", "function_group", "table_name", "message_class", "language",
		"target_language", "description", "user", "file_path", "text_symbols", "dump_id",
		"trace_id", "keyword", "sql_query", "pattern", "query", "source_language",
	}
	bag := map[string]any{}
	for _, n := range names {
		if v, ok := synthValue(n, "string", dir); ok {
			bag[n] = v
		}
	}
	bag["transports"] = "TR-EXAMPLE"
	bag["objects"] = []any{map[string]any{"object_type": "PROG", "object_name": "ZDEMO_REPORT", "pgmid": "R3TR", "type": "PROG", "name": "ZDEMO_REPORT"}}
	bag["texts"] = []any{map[string]any{"key": "001", "text": "Demo"}}
	return bag
}

func mergeArgs(base map[string]any, over map[string]any) map[string]any {
	out := map[string]any{}
	for k, v := range base {
		out[k] = v
	}
	for k, v := range over {
		out[k] = v
	}
	return out
}

// --- the SAP() action surface ----------------------------------------------

// actionCase is one SAP(action, target, params) call. Like names a tool whose
// synthetic arguments the params start from; Params are laid over them, and
// Exact uses Params alone (for routes that branch on which parameter is set).
type actionCase struct {
	Name           string
	Action, Target string
	Like           string
	Params         map[string]any
	Exact          bool
}

func (c actionCase) params(tools map[string]mcp.Tool, dir string) map[string]any {
	if c.Exact {
		return mergeArgs(nil, c.Params)
	}
	base := synthBag(dir)
	if c.Like != "" {
		if tool, ok := tools[c.Like]; ok {
			base = synthArgs(tool, dir)
		}
	}
	return mergeArgs(base, c.Params)
}

func kv(pairs ...any) map[string]any {
	m := map[string]any{}
	for i := 0; i+1 < len(pairs); i += 2 {
		m[pairs[i].(string)] = pairs[i+1]
	}
	return m
}

// tableCases are the SAP() actions routed through tables, enumerated from the
// tables themselves.
func (s *Server) tableCases() []actionCase {
	analyze := s.AnalyzeTypes()
	out := make([]actionCase, 0, len(analyze)+len(s.i18nTypes())+len(s.revisionTypes())+len(s.lintTypes()))
	for _, t := range analyze {
		out = append(out, actionCase{Name: "SAP analyze type=" + t, Action: "analyze", Params: kv("type", t)})
	}
	i18nExtra := map[string]map[string]any{
		"texts_set":           {"texts": map[string]any{"001": "Demo"}},
		"write_text_pool":     {"texts": map[string]any{"001": "Demo"}},
		"write_message_texts": {"texts": []any{map[string]any{"number": "001", "text": "Demo"}}},
		"write_labels":        {"lock_handle": "FAKELOCKHANDLE", "short": "Demo", "medium": "Demo", "long": "Demo", "heading": "Demo"},
	}
	for op := range s.i18nTypes() {
		out = append(out, actionCase{Name: "SAP i18n op=" + op, Action: "i18n", Target: "PROG ZDEMO_REPORT",
			Params: mergeArgs(kv("op", op), i18nExtra[op])})
	}
	for op := range s.revisionTypes() {
		out = append(out, actionCase{Name: "SAP revisions op=" + op, Action: "revisions", Target: "PROG ZDEMO_REPORT", Params: kv("op", op)})
	}
	for a := range s.lintTypes() {
		out = append(out, actionCase{Name: "SAP " + a, Action: a})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

// routedLiterals parses the SAP() route functions and returns, per function,
// every string literal it matches an action, target type, type or op against,
// together with the order handleUniversalTool tries the route functions in.
func routedLiterals(t *testing.T) (lits map[string]map[string]bool, order []string) {
	t.Helper()
	fset := token.NewFileSet()
	files, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatal(err)
	}
	lits = map[string]map[string]bool{}
	routeFn := regexp.MustCompile(`^route[A-Za-z0-9]*Action$`)
	for _, file := range files {
		if strings.HasSuffix(file, "_test.go") {
			continue
		}
		f, err := parser.ParseFile(fset, file, nil, 0)
		if err != nil {
			t.Fatal(err)
		}
		for _, decl := range f.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if !ok || fn.Body == nil || (!routeFn.MatchString(fn.Name.Name) && fn.Name.Name != "handleUniversalTool") {
				continue
			}
			name := fn.Name.Name
			add := func(e ast.Expr) {
				lit, ok := e.(*ast.BasicLit)
				if !ok || lit.Kind != token.STRING {
					return
				}
				v, err := strconv.Unquote(lit.Value)
				if err != nil || v == "" {
					return
				}
				if lits[name] == nil {
					lits[name] = map[string]bool{}
				}
				lits[name][v] = true
			}
			ast.Inspect(fn.Body, func(n ast.Node) bool {
				switch n := n.(type) {
				case *ast.CaseClause:
					for _, e := range n.List {
						add(e)
					}
				case *ast.BinaryExpr:
					if n.Op == token.EQL {
						add(n.X)
						add(n.Y)
					}
				case *ast.CompositeLit:
					// routes := []routeFunc{s.routeSourceAction, ...}
					if name != "handleUniversalTool" {
						return true
					}
					if typ, ok := n.Type.(*ast.ArrayType); !ok || fmt.Sprint(typ.Elt) != "routeFunc" {
						return true
					}
					for _, e := range n.Elts {
						if sel, ok := e.(*ast.SelectorExpr); ok {
							order = append(order, sel.Sel.Name)
						}
					}
				}
				return true
			})
		}
	}
	if len(order) == 0 {
		t.Fatal("found no route list in handleUniversalTool")
	}
	return lits, order
}

type routeFunc = func(ctx context.Context, action, objectType, objectName string, params map[string]any) (*mcp.CallToolResult, bool, error)

// routeTable names every route function, so that the router a case reaches
// can be found by trying them in handleUniversalTool's own order. A router
// added there and not here fails the test.
func routeTable(s *Server) map[string]routeFunc {
	return map[string]routeFunc{
		"routeSourceAction":         s.routeSourceAction,
		"routeReadAction":           s.routeReadAction,
		"routeSearchAction":         s.routeSearchAction,
		"routeGrepAction":           s.routeGrepAction,
		"routeCodeIntelAction":      s.routeCodeIntelAction,
		"routeDevToolsAction":       s.routeDevToolsAction,
		"routeATCAction":            s.routeATCAction,
		"routeCRUDAction":           s.routeCRUDAction,
		"routeClassIncludeAction":   s.routeClassIncludeAction,
		"routeWorkflowAction":       s.routeWorkflowAction,
		"routeFileIOAction":         s.routeFileIOAction,
		"routeDebuggerAction":       s.routeDebuggerAction,
		"routeDebuggerLegacyAction": s.routeDebuggerLegacyAction,
		"routeAMDPADTAction":        s.routeAMDPADTAction,
		"routeAMDPAction":           s.routeAMDPAction,
		"routeUI5Action":            s.routeUI5Action,
		"routeTransportAction":      s.routeTransportAction,
		"routeGitAction":            s.routeGitAction,
		"routeReportAction":         s.routeReportAction,
		"routeInstallAction":        s.routeInstallAction,
		"routeSystemAction":         s.routeSystemAction,
		"routeRFCAction":            s.routeRFCAction,
		"routeDumpsAction":          s.routeDumpsAction,
		"routeTracesAction":         s.routeTracesAction,
		"routeSQLTraceAction":       s.routeSQLTraceAction,
		"routeLintAction":           s.routeLintAction,
		"routeAnalysisAction":       s.routeAnalysisAction,
		"routeContextAction":        s.routeContextAction,
		"routeServiceBindingAction": s.routeServiceBindingAction,
		"routeI18nAction":           s.routeI18nAction,
		"routeRevisionsAction":      s.routeRevisionsAction,
		"routeExtensionAction":      s.routeExtensionAction,
	}
}

// shadowedRouteLiterals are literals a router matches that no call can reach,
// because an earlier router claims every call that carries them. Each is
// checked to still be in its router, and still to be claimed by the earlier
// one, so the list cannot go stale in either direction.
var shadowedRouteLiterals = map[string]string{
	"routeReadAction PROG": shadowedBySource,
	"routeReadAction CLAS": shadowedBySource,
	"routeReadAction INTF": shadowedBySource,
	"routeReadAction FUNC": shadowedBySource,
	"routeReadAction FUGR": shadowedBySource,
	"routeReadAction INCL": shadowedBySource,
	"routeReadAction MSAG": shadowedBySource,
}

const shadowedBySource = "routeSourceAction claims action=read for this type first; routeReadAction's case for it is dead code"

// attributeCases finds, for each case, the route function that claims it: the
// first in handleUniversalTool's order to answer handled. It calls the
// handlers, so it runs against the fake SAP like everything else, and what
// it sends is discarded.
func (env *invariantEnv) attributeCases(t *testing.T, order []string, cases []actionCase, tools map[string]mcp.Tool) map[string]string {
	t.Helper()
	cfg := env.cfg()
	cfg.Mode = "hyperfocused"
	s := NewServer(cfg)
	table := routeTable(s)
	for _, name := range order {
		if table[name] == nil {
			t.Errorf("handleUniversalTool tries %s, which routeTable does not know: add it there so its cases can be attributed", name)
		}
	}
	if len(table) != len(order) {
		t.Errorf("routeTable has %d routers and handleUniversalTool tries %d", len(table), len(order))
	}
	defer func() {
		s.closeDebugSession(context.Background())
		s.dropSharedRFC(context.Background())
		env.settle(s)
		env.sap.take()
	}()
	out := map[string]string{}
	for _, c := range cases {
		action := strings.ToLower(strings.TrimSpace(c.Action))
		if action == "" || action == "info" || action == "help" {
			out[c.Name] = "handleUniversalTool"
			continue
		}
		objectType, objectName := parseTarget(c.Target)
		params := c.params(tools, env.dir)
		for _, name := range order {
			fn := table[name]
			if fn == nil {
				continue
			}
			ctx, cancel := context.WithTimeout(context.Background(), probeTimeout)
			_, handled, _ := fn(ctx, action, objectType, objectName, mergeArgs(nil, params))
			cancel()
			if handled {
				out[c.Name] = name
				break
			}
		}
	}
	return out
}

// uncoveredRouteLiterals returns every (router, literal) pair no case that
// reaches that router carries, as action, target type, type or op.
func uncoveredRouteLiterals(t *testing.T, lits map[string]map[string]bool, order []string, cases []actionCase, claimedBy map[string]string) []string {
	t.Helper()
	covered := map[string]map[string]bool{}
	carries := map[string][]string{} // literal -> names of cases carrying it
	for _, c := range cases {
		fn := claimedBy[c.Name]
		if fn == "" {
			continue
		}
		if covered[fn] == nil {
			covered[fn] = map[string]bool{}
		}
		typ, _ := parseTarget(c.Target)
		keys := []string{c.Action, typ}
		for _, k := range []string{"type", "op"} {
			if v, ok := c.Params[k].(string); ok {
				keys = append(keys, v)
			}
		}
		for _, k := range keys {
			covered[fn][strings.ToLower(k)] = true
			carries[strings.ToLower(k)] = append(carries[strings.ToLower(k)], c.Name)
		}
	}
	rank := map[string]int{"handleUniversalTool": -1}
	for i, n := range order {
		rank[n] = i
	}
	var out []string
	seenShadow := map[string]bool{}
	for fn, set := range lits {
		for lit := range set {
			if covered[fn][strings.ToLower(lit)] {
				continue
			}
			key := fn + " " + lit
			if why, ok := shadowedRouteLiterals[key]; ok {
				seenShadow[key] = true
				// Still shadowed: some case carrying it is claimed earlier.
				earlier := false
				for _, name := range carries[strings.ToLower(lit)] {
					if r, ok := rank[claimedBy[name]]; ok && r < rank[fn] {
						earlier = true
					}
				}
				if !earlier {
					t.Errorf("shadowedRouteLiterals says %s is shadowed (%s), but no case carrying it is claimed by an earlier router", key, why)
				}
				continue
			}
			out = append(out, fmt.Sprintf("%q in %s", lit, fn))
		}
	}
	for key := range shadowedRouteLiterals {
		if !seenShadow[key] {
			t.Errorf("shadowedRouteLiterals lists %s, which is no longer an unreached literal of that router: remove it", key)
		}
	}
	sort.Strings(out)
	return out
}

// --- running one probe ------------------------------------------------------

type probeOutcome struct {
	Name     string
	World    string
	Class    surfaceClass
	Known    bool
	Text     string
	IsErr    bool
	Requests []sentRequest
	WSDials  int
	RFCDials int64
	// Late is what arrived after the handler returned, while the test
	// waited for the server to go quiet.
	Late         []sentRequest
	LateWSDials  int
	LateRFCDials int64
	Took         time.Duration
}

func (o probeOutcome) writes() []sentRequest { return writesIn(o.Requests) }

func writesIn(reqs []sentRequest) []sentRequest {
	var w []sentRequest
	for _, r := range reqs {
		if r.Upgrade == "" && isWrite(r) {
			w = append(w, r)
		}
	}
	return w
}

var refusalWords = regexp.MustCompile(`(?i)read-only|read only|readonly|safety configuration|blocked by safety`)

func (o probeOutcome) refused() bool { return refusalWords.MatchString(o.Text) }

type invariantEnv struct {
	sap     *fakeSAP
	gateway *fakeRFCGateway
	dir     string
	cfg     func() *Config
}

func newInvariantEnv(t *testing.T, cfg func(base string) *Config) *invariantEnv {
	t.Helper()
	env := &invariantEnv{sap: newFakeSAP(t), gateway: newFakeRFCGateway(t), dir: t.TempDir()}
	// The RFC destination of this server comes from a .vsp.json for its own URL
	// and client, pointed at the counting gateway. Keep a developer's own
	// configuration out of it.
	t.Setenv("HOME", t.TempDir())
	t.Chdir(env.dir)
	for _, k := range requestShapingEnv {
		t.Setenv(k, "")
	}
	conf := fmt.Sprintf(`{"systems": {"own": {"url": %q, "client": "001",
	  "rfc_host": "127.0.0.1", "rfc_sysnr": "00", "rfc_port": %d}}}`, env.sap.srv.URL, env.gateway.port)
	if err := os.WriteFile(filepath.Join(env.dir, ".vsp.json"), []byte(conf), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(env.dir, "zdemo_report.prog.abap"), []byte(fakeSource), 0o600); err != nil {
		t.Fatal(err)
	}
	env.cfg = func() *Config { return cfg(env.sap.srv.URL) }
	return env
}

// requestShapingEnv are the environment variables the server or its client
// read that change which requests are sent, or how: a response cache that
// answers a GET without sending it, credentials and system settings taken
// from the environment, per-system overrides of this test's .vsp.json entry
// ("own"), and debug switches. All are cleared, so that a developer's shell
// cannot change what the test sees.
var requestShapingEnv = []string{
	"VSP_CACHE", "VSP_CACHE_TTL", "VSP_CACHE_PATH",
	"SAP_URL", "SAP_CLIENT", "SAP_USER", "SAP_PASSWORD", "SAP_LANGUAGE", "SAP_INSECURE",
	"SAP_PROXY_CONTEXTID_GUARD", "VSP_TRANSPORT_ATTRIBUTE",
	"VSP_OWN_PASSWORD", "VSP_OWN_RFC_PASSWORD", "VSP_OWN_TRANSPORT_ATTRIBUTE", "VSP_OWN_CACHE",
	"VSP_DEBUG", "VSP_DEBUG_XML", "VSP_HTTP_TRACE", "VSP_TRACE_LOG", "DEBUG_LINE", "DEBUG_TARGET",
}

const probeTimeout = 2 * time.Second

// Settling: after a call returns, the server is given until it has been
// quiet for settleQuiet, and none of its async tasks is still running, up to
// settleMax.
const (
	settleQuiet = 3 * time.Millisecond
	settleMax   = time.Second
)

// settle waits for traffic a handler left running behind it: tasks the
// server tracks as running (RunReportAsync and the like), and any request or
// gateway dial still arriving.
func (env *invariantEnv) settle(s *Server) {
	deadline := time.Now().Add(settleMax)
	count := func() (int, int64) {
		env.sap.mu.Lock()
		defer env.sap.mu.Unlock()
		return len(env.sap.reqs), env.gateway.dials.Load()
	}
	running := func() bool {
		s.asyncTasksMu.RLock()
		defer s.asyncTasksMu.RUnlock()
		for _, task := range s.asyncTasks {
			if task.Status == "running" {
				return true
			}
		}
		return false
	}
	n, d := count()
	for time.Now().Before(deadline) {
		time.Sleep(settleQuiet)
		n2, d2 := count()
		if n2 == n && d2 == d && !running() {
			return
		}
		n, d = n2, d2
	}
}

func (env *invariantEnv) run(name string, call func(ctx context.Context, s *Server) (*mcp.CallToolResult, error), mode string) probeOutcome {
	cfg := env.cfg()
	cfg.Mode = mode
	s := NewServer(cfg)
	env.sap.take()
	before := env.gateway.dials.Load()
	ctx, cancel := context.WithTimeout(context.Background(), probeTimeout)
	start := time.Now()
	res, err := call(ctx, s)
	took := time.Since(start)
	cancel()
	requests := env.sap.take()
	// The gateway counts a connection before it hangs up, and a client only
	// fails its logon once the hang-up arrives, so a dial that happened has
	// been counted by the time the call returns.
	during := env.gateway.dials.Load()

	// What arrives after the handler returned — a goroutine it started, such
	// as RunReportAsync's — is the call's too. Wait for it, then keep it.
	env.settle(s)
	late := env.sap.take()
	afterLate := env.gateway.dials.Load()

	// The server's own shutdown releases a debug session it opened; that is
	// the test tearing the server down, not the call under test, and what it
	// sends is not attributed to it.
	s.closeDebugSession(context.Background())
	s.dropSharedRFC(context.Background())
	env.sap.take()

	o := probeOutcome{Name: name, Took: took}
	if err != nil {
		o.Text, o.IsErr = err.Error(), true
	} else if res != nil {
		o.Text, o.IsErr = resultText(res), res.IsError
	}
	o.Requests = requests
	for _, r := range o.Requests {
		if r.Upgrade != "" {
			o.WSDials++
		}
	}
	o.RFCDials = during - before
	o.Late = late
	for _, r := range late {
		if r.Upgrade != "" {
			o.LateWSDials++
		}
	}
	o.LateRFCDials = afterLate - during
	o.Class, o.Known = readOnlyClasses[name]
	if !o.Known {
		o.Class, o.Known = invariantExtensionClasses[name]
	}
	return o
}

// invariantExtensions are registered on every server the invariant builds, so
// that the extensions' actions are walked like the built-in ones -- classified
// by the class each declares -- and a combination of extensions is checked
// the same way as one.
var (
	invariantExtensions                                = []mcpext.Extension{newFakeExtension()}
	invariantExtensionCases, invariantExtensionClasses = extensionCases(invariantExtensions)
)

func readOnlyConfig(base string) *Config {
	return &Config{
		BaseURL:  base,
		Username: "TESTUSER",
		Password: "unused",
		Client:   "001",
		Language: "EN",
		ReadOnly: true,
		// Transports are enabled so that --read-only, and nothing else, is
		// what stands between a transport write and SAP.
		EnableTransports: true,
		Extensions:       invariantExtensions,
	}
}

// runAll calls every registered tool and every SAP() action once.
func (env *invariantEnv) runAll(expert *Server, tools map[string]mcp.Tool, cases []actionCase) []probeOutcome {
	var out []probeOutcome
	for _, name := range expert.RegisteredTools() {
		name := name
		args := synthArgs(tools[name], env.dir)
		out = append(out, env.run("tool "+name, func(ctx context.Context, s *Server) (*mcp.CallToolResult, error) {
			st, ok := s.mcpServer.ListTools()[name]
			if !ok {
				return nil, fmt.Errorf("tool %s not registered", name)
			}
			return st.Handler(ctx, newRequest(mergeArgs(nil, args)))
		}, "expert"))
	}
	for _, c := range cases {
		c := c
		params := c.params(tools, env.dir)
		out = append(out, env.run(c.Name, func(ctx context.Context, s *Server) (*mcp.CallToolResult, error) {
			return s.handleUniversalTool(ctx, newRequest(map[string]any{
				"action": c.Action, "target": c.Target, "params": mergeArgs(nil, params),
			}))
		}, "hyperfocused"))
	}
	return out
}

// --- the test ---------------------------------------------------------------

// TestReadOnlyInvariant runs the whole surface twice, against a SAP where
// every object exists and one where none does, so that a write cannot hide
// behind either answer, and prints the summary line CI lifts.
func TestReadOnlyInvariant(t *testing.T) {
	// Parsed before the environment moves the working directory.
	lits, order := routedLiterals(t)
	env := newInvariantEnv(t, readOnlyConfig)

	expert := NewServer(&Config{BaseURL: env.sap.srv.URL, Mode: "expert", ReadOnly: true})
	tools := map[string]mcp.Tool{}
	handlers := expert.mcpServer.ListTools()
	for name, st := range handlers {
		tools[name] = st.Tool
	}

	cases := append(actionCases(), expert.tableCases()...)
	cases = append(cases, invariantExtensionCases...)
	outcomes := make([]probeOutcome, 0, 2*(len(tools)+len(cases)))
	for _, world := range []string{"present", "absent"} {
		env.sap.absent.Store(world == "absent")
		for _, o := range env.runAll(expert, tools, cases) {
			o.World = world
			outcomes = append(outcomes, o)
		}
	}
	env.sap.absent.Store(false)

	// Every literal a router matches on must be exercised by some case that
	// reaches that router, not merely by some case somewhere: a new case in
	// one router reusing another router's literal is still unclassified.
	unrouted := uncoveredRouteLiterals(t, lits, order, cases, env.attributeCases(t, order, cases, tools))

	assertReadOnlyInvariant(t, outcomes, unrouted)

	t.Run("allowed-packages Z* on a $TMP object", testPackageGate)
}

// assertReadOnlyInvariant checks the outcomes and prints the one-line summary
// CI lifts into the PR report.
func assertReadOnlyInvariant(t *testing.T, outcomes []probeOutcome, unrouted []string) {
	t.Helper()
	trace := os.Getenv("VSP_READONLY_TRACE") != ""
	reached := map[string]bool{}
	unclassified := map[string]bool{}
	leaked := map[string]bool{}
	gaps := map[string]string{}
	gapLeaks := map[string][]string{}
	kinds := map[surfaceKind]map[string]bool{}

	for _, o := range outcomes {
		reached[o.Name] = true
		if trace {
			var sent []string
			for _, r := range o.Requests {
				if r.Method != http.MethodGet && r.Method != http.MethodHead {
					sent = append(sent, r.String())
				}
			}
			t.Logf("%-7s %-50s refused=%-5v ws=%d rfc=%d %v | %s", o.World, o.Name, o.refused(), o.WSDials, o.RFCDials, sent, firstLine(o.Text))
		}
		if !o.Known {
			unclassified[o.Name] = true
			continue
		}
		if kinds[o.Class.Kind] == nil {
			kinds[o.Class.Kind] = map[string]bool{}
		}
		kinds[o.Class.Kind][o.Name] = true

		var bad []string
		for _, r := range o.writes() {
			bad = append(bad, r.String())
		}
		if o.Class.Kind != kindRead {
			if o.WSDials > 0 {
				bad = append(bad, fmt.Sprintf("%d ZADT_VSP WebSocket dial(s)", o.WSDials))
			}
			if o.RFCDials > 0 {
				bad = append(bad, fmt.Sprintf("%d RFC gateway dial(s)", o.RFCDials))
			}
		}

		// After the call returned, nothing may write or dial, whatever the
		// class: a READ has no business leaving a writer behind either.
		for _, r := range writesIn(o.Late) {
			bad = append(bad, "after the call returned: "+r.String())
		}
		if o.LateWSDials > 0 {
			bad = append(bad, fmt.Sprintf("after the call returned: %d ZADT_VSP WebSocket dial(s)", o.LateWSDials))
		}
		if o.LateRFCDials > 0 {
			bad = append(bad, fmt.Sprintf("after the call returned: %d RFC gateway dial(s)", o.LateRFCDials))
		}

		if o.Class.KnownGap != "" {
			gaps[o.Name] = o.Class.KnownGap
			if len(bad) > 0 {
				gapLeaks[o.Name] = bad
			}
			continue
		}
		if len(bad) > 0 {
			leaked[o.Name] = true
			t.Errorf("%s (%s, %s world): reached SAP under --read-only:\n    %s\n  answer: %s",
				o.Name, o.Class.Kind, o.World, strings.Join(bad, "\n    "), firstLine(o.Text))
		}
		// The refusal is asked for where every write path is live: in the
		// absent world a handler may rightly find nothing to change (a
		// recovery with nothing to recover), and there only the leak check
		// above applies.
		if o.World == "present" && o.Class.Kind != kindRead && !o.refused() {
			t.Errorf("%s (%s, %s world): not refused under --read-only; the answer does not name read-only or the safety configuration:\n  %s",
				o.Name, o.Class.Kind, o.World, firstLine(o.Text))
		}
	}

	names := make([]string, 0, len(unclassified))
	for n := range unclassified {
		names = append(names, n)
	}
	sort.Strings(names)
	for _, n := range names {
		t.Errorf("%s: classify me — add it to readOnlyClasses as READ, MUTATE or EXECUTE", n)
	}
	for _, u := range unrouted {
		t.Errorf("the router literal %s is exercised by no actionCase that reaches that router: add a case for it to actionCases and classify me", u)
	}
	for n := range readOnlyClasses {
		if !reached[n] {
			t.Errorf("readOnlyClasses lists %q, which no registered tool or routed action reaches any more: remove it", n)
		}
	}

	gapNames := make([]string, 0, len(gaps))
	for n := range gaps {
		gapNames = append(gapNames, n)
	}
	sort.Strings(gapNames)
	for _, n := range gapNames {
		if sent := gapLeaks[n]; len(sent) > 0 {
			t.Logf("known gap %s: %s\n    sent: %s", n, gaps[n], strings.Join(sent, "; "))
		}
	}
	t.Logf("classified: %d READ, %d MUTATE, %d EXECUTE", len(kinds[kindRead]), len(kinds[kindMutate]), len(kinds[kindExecute]))

	summary := fmt.Sprintf("readonly-invariant: %d tools/actions, %d writes leaked, %d known gaps, %d unclassified",
		len(reached), len(leaked), len(gaps), len(unclassified)+len(unrouted))
	fmt.Println(summary)
	t.Log(summary)
}

// --- the package gate -------------------------------------------------------

// packageRefusal is the package gate's own wording: CheckPackage's refusal
// of $TMP, or the UI5 surface's fail-closed refusal (no app→package
// resolution yet). A lookup error that merely mentions a package is not it.
// packageGateArgs overrides the synthetic arguments of a packageGated entry
// named "tool <Tool> <variant>".
var packageGateArgs = map[string]map[string]any{
	"tool MoveObject out of $TMP": {"object_type": "PROG", "object_name": "ZDEMO_REPORT", "new_package": "ZDEMO_PKG"},
}

var packageRefusal = regexp.MustCompile(`operations on package '\$TMP' are blocked by safety configuration|` +
	`on UI5 surface is blocked: UI5 app→package resolution not yet implemented`)

func testPackageGate(t *testing.T) {
	env := newInvariantEnv(t, func(base string) *Config {
		return &Config{
			BaseURL: base, Username: "TESTUSER", Password: "unused", Client: "001", Language: "EN",
			AllowedPackages: []string{"Z*"},
		}
	})
	expert := NewServer(&Config{BaseURL: env.sap.srv.URL, Mode: "expert"})
	tools := map[string]mcp.Tool{}
	for name, st := range expert.mcpServer.ListTools() {
		tools[name] = st.Tool
	}
	cases := map[string]actionCase{}
	for _, c := range actionCases() {
		cases[c.Name] = c
	}
	for _, c := range expert.tableCases() {
		cases[c.Name] = c
	}

	names := make([]string, 0, len(packageGated))
	for n := range packageGated {
		names = append(names, n)
	}
	sort.Strings(names)
	held, gaps := 0, 0
	for _, name := range names {
		var call func(ctx context.Context, s *Server) (*mcp.CallToolResult, error)
		mode := "expert"
		if rest, ok := strings.CutPrefix(name, "tool "); ok {
			tool := strings.Fields(rest)[0]
			if _, registered := tools[tool]; !registered {
				t.Errorf("%s is listed in packageGated and is not a registered tool", name)
				continue
			}
			// No transport: the transportable-edit check would refuse first,
			// and it is the package gate under test.
			args := mergeArgs(synthArgs(tools[tool], env.dir), packageGateArgs[name])
			delete(args, "transport")
			call = func(ctx context.Context, s *Server) (*mcp.CallToolResult, error) {
				return s.mcpServer.ListTools()[tool].Handler(ctx, newRequest(mergeArgs(nil, args)))
			}
		} else {
			c, ok := cases[name]
			if !ok {
				t.Errorf("%s is listed in packageGated and is no SAP() action case", name)
				continue
			}
			params := c.params(tools, env.dir)
			delete(params, "transport")
			mode = "hyperfocused"
			call = func(ctx context.Context, s *Server) (*mcp.CallToolResult, error) {
				return s.handleUniversalTool(ctx, newRequest(map[string]any{"action": c.Action, "target": c.Target, "params": mergeArgs(nil, params)}))
			}
		}
		o := env.run(name, call, mode)

		var bad []string
		for _, r := range o.writes() {
			bad = append(bad, r.String())
		}
		if o.WSDials > 0 {
			bad = append(bad, fmt.Sprintf("%d ZADT_VSP WebSocket dial(s)", o.WSDials))
		}
		if o.RFCDials > 0 {
			bad = append(bad, fmt.Sprintf("%d RFC gateway dial(s)", o.RFCDials))
		}
		for _, r := range writesIn(o.Late) {
			bad = append(bad, "after the call returned: "+r.String())
		}
		if o.LateWSDials+int(o.LateRFCDials) > 0 {
			bad = append(bad, fmt.Sprintf("after the call returned: %d WebSocket and %d RFC dial(s)", o.LateWSDials, o.LateRFCDials))
		}
		if why := packageGated[name]; why != "" {
			gaps++
			t.Logf("known gap %s: %s\n    sent: %v", name, why, bad)
			continue
		}
		held++
		if len(bad) > 0 {
			t.Errorf("%s: a $TMP object reached SAP under --allowed-packages Z*:\n    %s\n  answer: %s", name, strings.Join(bad, "\n    "), firstLine(o.Text))
		}
		if !packageRefusal.MatchString(o.Text) {
			t.Errorf("%s: not refused by the package gate under --allowed-packages Z*; answer: %s", name, firstLine(o.Text))
		}
	}
	t.Logf("package gate: %d held, %d known gaps", held, gaps)
}
