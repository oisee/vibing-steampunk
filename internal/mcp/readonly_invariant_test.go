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

// --- classification ---------------------------------------------------------

type surfaceKind string

const (
	kindRead    surfaceKind = "READ"
	kindMutate  surfaceKind = "MUTATE"
	kindExecute surfaceKind = "EXECUTE"
)

// surfaceClass says what a tool or action does to the system. KnownGap, when
// set, names a write or execution --read-only does not refuse yet, and why it
// is recorded instead of fixed; such an entry is reported, not failed.
type surfaceClass struct {
	Kind     surfaceKind
	KnownGap string
}

var (
	clsRead    = surfaceClass{Kind: kindRead}
	clsMutate  = surfaceClass{Kind: kindMutate}
	clsExecute = surfaceClass{Kind: kindExecute}
)

func knownGap(k surfaceKind, why string) surfaceClass { return surfaceClass{Kind: k, KnownGap: why} }

// readOnlyClasses classifies every tool ("tool X") and every SAP() action
// ("SAP <action> ..."). A name the test reaches that is not here fails with
// "classify me": a new tool or route has to be placed in one of the three
// kinds before it ships.
var readOnlyClasses = map[string]surfaceClass{
	// --- tools: reads ---
	"tool AMDPGetBreakpoints":       clsRead,
	"tool AMDPGetVariables":         clsRead,
	"tool AMDPDebuggerStop":         clsRead, // ends this server's own AMDP session
	"tool AnalyzeABAPCode":          clsRead,
	"tool AnalyzeCallGraph":         clsRead,
	"tool CheckBoundaries":          clsRead,
	"tool CodeCompletion":           clsRead,
	"tool CompareCallGraphs":        clsRead,
	"tool CompareLanguages":         clsRead,
	"tool CompareSource":            clsRead,
	"tool CompareVersions":          clsRead,
	"tool DebuggerAttach":           clsRead, // takes a stopped debuggee of this user; changes nothing in it
	"tool DebuggerDetach":           clsRead,
	"tool DebuggerGetStack":         clsRead,
	"tool DebuggerGetVariables":     clsRead,
	"tool DebuggerListen":           clsRead,
	"tool ExportToFile":             clsRead, // writes a local file, not SAP
	"tool FindDefinition":           clsRead,
	"tool FindReferences":           clsRead,
	"tool GetAPIReleaseState":       clsRead,
	"tool GetATCCustomizing":        clsRead,
	"tool GetAbapHelp":              clsRead,
	"tool GetAsyncResult":           clsRead,
	"tool GetBreakpoints":           clsRead,
	"tool GetCDSDependencies":       clsRead,
	"tool GetCDSElementInfo":        clsRead,
	"tool GetCDSImpactAnalysis":     clsRead,
	"tool GetCallGraph":             clsRead,
	"tool GetCalleesOf":             clsRead,
	"tool GetCallersOf":             clsRead,
	"tool GetCheckRunResults":       clsRead,
	"tool GetClass":                 clsRead,
	"tool GetClassComponents":       clsRead,
	"tool GetClassInclude":          clsRead,
	"tool GetClassInfo":             clsRead,
	"tool GetCodeCoverage":          clsRead, // harmless ABAP Unit only; dangerous runs are refused (#283)
	"tool GetConnectionInfo":        clsRead,
	"tool GetContext":               clsRead,
	"tool GetDataElementLabels":     clsRead,
	"tool GetDump":                  clsRead,
	"tool GetFeatures":              clsRead,
	"tool GetFunction":              clsRead,
	"tool GetFunctionGroup":         clsRead,
	"tool GetInactiveObjects":       clsRead,
	"tool GetInclude":               clsRead,
	"tool GetInstalledComponents":   clsRead,
	"tool GetInterface":             clsRead,
	"tool GetMessageClassTexts":     clsRead,
	"tool GetMessages":              clsRead,
	"tool GetObjectStructure":       clsRead,
	"tool GetObjectTextsInLanguage": clsRead,
	"tool GetPackage":               clsRead,
	"tool GetPrettyPrinterSettings": clsRead,
	"tool GetProgram":               clsRead,
	"tool GetRevisionSource":        clsRead,
	"tool GetRevisions":             clsRead,
	"tool GetSQLTraceState":         clsRead,
	"tool GetSource":                clsRead,
	"tool GetStructure":             clsRead,
	"tool GetSystemInfo":            clsRead,
	"tool GetTable":                 clsRead,
	"tool GetTableContents":         clsRead,
	"tool GetTextElements":          clsRead,
	"tool GetTextPool":              clsRead,
	"tool GetTrace":                 clsRead,
	"tool GetTransaction":           clsRead,
	"tool GetTransport":             clsRead,
	"tool GetTransportInfo":         clsRead,
	"tool GetTypeHierarchy":         clsRead,
	"tool GetTypeInfo":              clsRead,
	"tool GetUserTransports":        clsRead,
	"tool GetVariants":              clsRead,
	"tool GitExport":                clsRead,
	"tool GitTypes":                 clsRead,
	"tool GraphStats":               clsRead,
	"tool GrepObject":               clsRead,
	"tool GrepObjects":              clsRead,
	"tool GrepPackage":              clsRead,
	"tool GrepPackages":             clsRead,
	"tool ListDependencies":         clsRead,
	"tool ListDumps":                clsRead,
	"tool ListSQLTraces":            clsRead,
	"tool ListTraces":               clsRead,
	"tool ListTransports":           clsRead,
	"tool PrettyPrint":              clsRead, // formats the source it is given; stores nothing
	"tool RunATCCheck":              clsRead,
	"tool RunQuery":                 clsRead, // free SQL is a read; --block-free-sql governs it, not --read-only
	"tool RunUnitTests":             clsRead, // harmless ABAP Unit only; dangerous runs are refused (#283)
	"tool SAP":                      clsRead, // the dispatcher; its actions are classified below
	"tool SaveToFile":               clsRead, // writes a local file, not SAP
	"tool SearchObject":             clsRead,
	"tool SyntaxCheck":              clsRead,
	"tool TraceExecution":           clsRead,
	"tool UI5GetApp":                clsRead,
	"tool UI5GetFileContent":        clsRead,
	"tool UI5ListApps":              clsRead,
	"tool UnlockObject":             clsRead, // releases a lock; READ locks stay allowed under --read-only

	// --- tools: writes ---
	"tool Activate":                 clsMutate,
	"tool ActivateMultiple":         clsMutate,
	"tool ActivatePackage":          clsMutate,
	"tool AssignIAMAppToCatalog":    clsMutate,
	"tool CloneObject":              clsMutate,
	"tool CreateAndActivateProgram": clsMutate,
	"tool CreateBusinessCatalog":    clsMutate,
	"tool CreateClassWithTests":     clsMutate,
	"tool CreateIAMApp":             clsMutate,
	"tool CreateObject":             clsMutate,
	"tool CreatePackage":            clsMutate,
	"tool CreateTable":              clsMutate,
	"tool CreateTestInclude":        clsMutate,
	"tool CreateTransport":          clsMutate,
	"tool DeleteObject":             clsMutate,
	"tool DeleteTransport":          clsMutate,
	"tool DeployFromFile":           clsMutate,
	"tool DeployZip":                clsMutate,
	"tool EditSource":               clsMutate,
	"tool ImportFromFile":           clsMutate,
	"tool InstallAbapGit":           clsMutate,
	"tool InstallDummyTest":         clsMutate,
	"tool InstallZADTVSP":           clsMutate,
	"tool LockObject":               clsMutate, // a MODIFY lock; READ locks are allowed
	"tool MoveObject":               clsMutate,
	"tool PublishServiceBinding":    clsMutate,
	"tool RecoverFailedCreate":      clsMutate,
	"tool ReleaseTransport":         clsMutate,
	"tool RenameObject":             clsMutate,
	"tool SetPrettyPrinterSettings": clsMutate,
	"tool SetTextElements":          clsMutate,
	"tool UI5CreateApp":             clsMutate,
	"tool UI5DeleteApp":             clsMutate,
	"tool UI5DeleteFile":            clsMutate,
	"tool UI5UploadFile":            clsMutate,
	"tool UnpublishServiceBinding":  clsMutate,
	"tool UpdateClassInclude":       clsMutate,
	"tool UpdateSource":             clsMutate,
	"tool WriteClass":               clsMutate,
	"tool WriteDataElementLabels":   clsMutate,
	"tool WriteMessageClassTexts":   clsMutate,
	"tool WriteMetadataExtension":   clsMutate,
	"tool WriteProgram":             clsMutate,
	"tool WriteSource":              clsMutate,
	"tool SetBreakpoint":            knownGap(kindMutate, gapBreakpoints),
	"tool DeleteBreakpoint":         knownGap(kindMutate, gapBreakpoints),
	"tool AMDPSetBreakpoint":        knownGap(kindMutate, gapAMDP),

	// --- tools: execution ---
	"tool CallRFC":            clsExecute,
	"tool ExecuteABAP":        clsExecute,
	"tool RunReport":          clsExecute,
	"tool RunReportAsync":     clsExecute,
	"tool DebuggerStep":       knownGap(kindExecute, gapStepping),
	"tool AMDPDebuggerStart":  knownGap(kindExecute, gapAMDP),
	"tool AMDPDebuggerResume": knownGap(kindExecute, gapAMDP),
	"tool AMDPDebuggerStep":   knownGap(kindExecute, gapAMDP),

	// --- SAP(): entry, help ---
	"SAP info":        clsRead,
	"SAP (no action)": clsRead,
	"SAP help":        clsRead,

	// --- SAP(): clsRead, query, search, grep ---
	"SAP read PROG": clsRead, "SAP read CLAS": clsRead, "SAP read INTF": clsRead, "SAP read FUNC": clsRead,
	"SAP read FUGR": clsRead, "SAP read INCL": clsRead, "SAP read DDLS": clsRead, "SAP read BDEF": clsRead,
	"SAP read SRVD": clsRead, "SAP read SRVB": clsRead, "SAP read MSAG": clsRead, "SAP read VIEW": clsRead,
	"SAP read ENHO": clsRead, "SAP read TABL": clsRead, "SAP read DEVC": clsRead, "SAP read IDOC": clsRead,
	"SAP read TRAN": clsRead, "SAP read TYPE_INFO": clsRead, "SAP read STRUCT": clsRead,
	"SAP read CDS_DEPS": clsRead, "SAP read CDS_IMPACT": clsRead, "SAP read CDS_ELEMENTS": clsRead,
	"SAP read TABL_CONTENTS": clsRead, "SAP read CHECK_RUN": clsRead, "SAP read API_STATE": clsRead,
	"SAP read CLASS_INFO": clsRead, "SAP read UI5_APP": clsRead, "SAP read ENHANCEMENT_OPTIONS": clsRead,
	"SAP read CLAS_INCLUDE": clsRead, "SAP read COVERAGE": clsRead, "SAP read UI5_LIST": clsRead,
	"SAP read UI5_FILE":                   clsRead,
	"SAP read COVERAGE include_dangerous": clsExecute,
	"SAP query TABL_CONTENTS":             clsRead, "SAP query SQL": clsRead, "SAP query SQL table": clsRead,
	"SAP query (no params)": clsRead,
	"SAP search":            clsRead,
	"SAP grep package":      clsRead, "SAP grep packages": clsRead, "SAP grep object": clsRead,
	"SAP grep objects": clsRead, "SAP grep (no target)": clsRead,

	// --- SAP(): edit ---
	"SAP edit PROG": clsMutate, "SAP edit CLAS": clsMutate, "SAP edit INTF": clsMutate, "SAP edit FUNC": clsMutate,
	"SAP edit INCL": clsMutate, "SAP edit DDLS": clsMutate, "SAP edit BDEF": clsMutate, "SAP edit SRVD": clsMutate,
	"SAP edit MSAG": clsMutate, "SAP edit TABL": clsMutate,
	"SAP edit EDITSOURCE":              clsMutate,
	"SAP edit LOCK":                    clsMutate,
	"SAP edit UNLOCK":                  clsRead,
	"SAP edit UPDATE_SOURCE":           clsMutate,
	"SAP edit MOVE":                    clsMutate,
	"SAP edit COMPARE_SOURCE":          clsRead,
	"SAP edit RECOVER_FAILED_CREATE":   clsMutate,
	"SAP edit ACTIVATE":                clsMutate,
	"SAP edit ACTIVATE_MULTI":          clsMutate,
	"SAP edit ACTIVATE_PACKAGE":        clsMutate,
	"SAP edit CLAS_INCLUDE":            clsMutate,
	"SAP edit PUBLISH_SERVICE":         clsMutate,
	"SAP edit UNPUBLISH_SERVICE":       clsMutate,
	"SAP edit type=publish_service":    clsMutate,
	"SAP edit type=unpublish_service":  clsMutate,
	"SAP edit type=write_program":      clsMutate,
	"SAP edit type=write_class":        clsMutate,
	"SAP edit type=set_description":    clsMutate,
	"SAP edit type=description":        clsMutate,
	"SAP edit type=deploy_from_file":   clsMutate,
	"SAP edit type=save_to_file":       clsRead, // writes a local file, not SAP
	"SAP edit type=rename":             clsMutate,
	"SAP system type=deploy_from_file": clsMutate,
	"SAP system type=save_to_file":     clsRead,
	"SAP system type=rename":           clsMutate,
	"SAP edit UI5_UPLOAD":              clsMutate,

	// --- SAP(): create, delete ---
	"SAP create OBJECT": clsMutate, "SAP create DEVC": clsMutate, "SAP create TABL": clsMutate,
	"SAP create CLONE": clsMutate, "SAP create ENHO": clsMutate, "SAP create BADI_IMPL": clsMutate,
	"SAP create DOMA": clsMutate, "SAP create DTEL": clsMutate, "SAP create STRUCT": clsMutate,
	"SAP create APPEND": clsMutate, "SAP create CLAS_TEST_INCLUDE": clsMutate, "SAP create PROGRAM": clsMutate,
	"SAP create CLASS_WITH_TESTS": clsMutate, "SAP create UI5_APP": clsMutate,
	"SAP delete OBJECT": clsMutate, "SAP delete (no target)": clsMutate,
	"SAP delete UI5_FILE": clsMutate, "SAP delete UI5_APP": clsMutate,

	// --- SAP(): test ---
	"SAP test unit":                        clsRead, // harmless ABAP Unit only (#283)
	"SAP test type=unit include_dangerous": clsExecute,
	"SAP test type=atc":                    clsRead,
	"SAP test type=atc_customizing":        clsRead,

	// --- SAP(): analyze, switch-routed ---
	"SAP analyze type=definition":                  clsRead,
	"SAP analyze type=references":                  clsRead,
	"SAP analyze type=completion":                  clsRead,
	"SAP analyze type=pretty_print":                clsRead,
	"SAP analyze type=get_pretty_printer_settings": clsRead,
	"SAP analyze type=set_pretty_printer_settings": clsMutate,
	"SAP analyze type=type_hierarchy":              clsRead,
	"SAP analyze type=class_components":            clsRead,
	"SAP analyze type=inactive_objects":            clsRead,
	"SAP analyze type=abap_help":                   clsRead,
	"SAP analyze type=syntax_check":                clsRead,
	"SAP analyze type=execute_abap":                clsExecute,

	// --- SAP(): analyze, table-routed (AnalyzeTypes) ---
	"SAP analyze type=analyze_call_graph":  clsRead,
	"SAP analyze type=analyze_deps":        clsRead,
	"SAP analyze type=application_log":     clsRead,
	"SAP analyze type=call_graph":          clsRead,
	"SAP analyze type=callees":             clsRead,
	"SAP analyze type=callers":             clsRead,
	"SAP analyze type=check_boundaries":    clsRead,
	"SAP analyze type=cluster_read":        clsRead,
	"SAP analyze type=co_change":           clsRead,
	"SAP analyze type=compare_call_graphs": clsRead,
	"SAP analyze type=context":             clsRead,
	"SAP analyze type=cr_boundaries":       clsRead,
	"SAP analyze type=cr_history":          clsRead,
	"SAP analyze type=documentation":       clsRead,
	"SAP analyze type=dump_impact":         clsRead,
	"SAP analyze type=effects":             clsRead,
	"SAP analyze type=explain_dump":        clsRead,
	"SAP analyze type=fm_test_data":        clsRead,
	"SAP analyze type=get_dump":            clsRead,
	"SAP analyze type=get_trace":           clsRead,
	"SAP analyze type=graph_stats":         clsRead,
	"SAP analyze type=group_dumps":         clsRead,
	"SAP analyze type=health":              clsRead,
	"SAP analyze type=img_activity":        clsRead,
	"SAP analyze type=img_search":          clsRead,
	"SAP analyze type=impact":              clsRead,
	"SAP analyze type=job_list":            clsRead,
	"SAP analyze type=job_log":             clsRead,
	"SAP analyze type=lint":                clsRead,
	"SAP analyze type=list_dumps":          clsRead,
	"SAP analyze type=list_sql_traces":     clsRead,
	"SAP analyze type=list_traces":         clsRead,
	"SAP analyze type=loads":               clsRead,
	"SAP analyze type=object_structure":    clsRead,
	"SAP analyze type=parse_abap":          clsRead,
	"SAP analyze type=similar_dumps":       clsRead,
	"SAP analyze type=spool_list":          clsRead,
	"SAP analyze type=spool_read":          clsRead,
	"SAP analyze type=sql_trace_state":     clsRead,
	"SAP analyze type=tr_boundaries":       clsRead,
	"SAP analyze type=trace_execution":     clsRead,
	"SAP analyze type=usage_examples":      clsRead,
	"SAP analyze type=variants":            clsRead,
	"SAP analyze type=where_used_config":   clsRead,
	"SAP lint":                             clsRead,

	// --- SAP(): debug ---
	"SAP debug AMDP_ADT_START":       knownGap(kindExecute, gapAMDP),
	"SAP debug AMDP_ADT_BREAKPOINT":  knownGap(kindMutate, gapAMDP),
	"SAP debug AMDP_ADT_AWAIT":       clsRead,
	"SAP debug AMDP_ADT_STOP":        clsRead,
	"SAP debug AMDP_START":           knownGap(kindExecute, gapAMDP),
	"SAP debug AMDP_RESUME":          knownGap(kindExecute, gapAMDP),
	"SAP debug AMDP_STOP":            clsRead,
	"SAP debug AMDP_STEP":            knownGap(kindExecute, gapAMDP),
	"SAP debug AMDP_GET_VARIABLES":   clsRead,
	"SAP debug AMDP_SET_BREAKPOINT":  knownGap(kindMutate, gapAMDP),
	"SAP debug AMDP_GET_BREAKPOINTS": clsRead,
	"SAP debug SET_BREAKPOINT":       knownGap(kindMutate, gapBreakpoints),
	"SAP debug GET_BREAKPOINTS":      clsRead,
	"SAP debug DELETE_BREAKPOINT":    knownGap(kindMutate, gapBreakpoints),
	"SAP debug CALL_RFC":             clsExecute,
	"SAP debug MOVE":                 clsMutate,
	"SAP debug LISTEN":               clsRead,
	"SAP debug ATTACH":               clsRead,
	"SAP debug DETACH":               clsRead,
	"SAP debug STEP":                 knownGap(kindExecute, gapStepping),
	"SAP debug GET_STACK":            clsRead,
	"SAP debug GET_VARIABLES":        clsRead,
	"SAP debug RUN_REPORT":           clsExecute,
	"SAP debug RUN_REPORT_ASYNC":     clsExecute,
	"SAP debug GET_ASYNC_RESULT":     clsRead,
	"SAP debug GET_VARIANTS":         clsRead,
	"SAP debug GET_TEXT_ELEMENTS":    clsRead,
	"SAP debug SET_TEXT_ELEMENTS":    clsMutate,

	// --- SAP(): system ---
	"SAP system INFO":                         clsRead,
	"SAP system COMPONENTS":                   clsRead,
	"SAP system CONNECTION":                   clsRead,
	"SAP system FEATURES":                     clsRead,
	"SAP system type=git_types":               clsRead,
	"SAP system type=git_export":              clsRead,
	"SAP system type=install_zadt_vsp":        clsMutate,
	"SAP system type=install_abapgit":         clsMutate,
	"SAP system type=install_dummy_test":      clsMutate,
	"SAP system type=list_dependencies":       clsRead,
	"SAP system type=deploy_zip":              clsMutate,
	"SAP system type=list_transports":         clsRead,
	"SAP system type=get_transport":           clsRead,
	"SAP system type=create_transport":        clsMutate,
	"SAP system type=release_transport":       clsMutate,
	"SAP system type=delete_transport":        clsMutate,
	"SAP system type=get_user_transports":     clsRead,
	"SAP system type=get_transport_info":      clsRead,
	"SAP system type=execute_abap":            clsExecute,
	"SAP system type=merge_transports":        clsMutate,
	"SAP system type=move_transport_object":   clsMutate,
	"SAP system type=move_object":             clsMutate,
	"SAP system type=copy_to_toc":             clsMutate,
	"SAP system type=transport_of_copies":     clsMutate,
	"SAP system type=add_transport_object":    clsMutate,
	"SAP system type=add_to_transport":        clsMutate,
	"SAP system type=remove_transport_object": clsMutate,
	"SAP system type=remove_from_transport":   clsMutate,
	"SAP system type=ui5_list_apps":           clsRead,
	"SAP system type=ui5_get_app":             clsRead,
	"SAP system type=ui5_get_file":            clsRead,
	"SAP system type=ui5_upload_file":         clsMutate,
	"SAP system type=ui5_delete_file":         clsMutate,
	"SAP system type=ui5_create_app":          clsMutate,
	"SAP system type=ui5_delete_app":          clsMutate,

	// --- SAP(): rfc ---
	"SAP rfc op=info":             clsRead,
	"SAP rfc (no op, no target)":  clsRead,
	"SAP rfc op=ping":             clsRead,
	"SAP rfc op=probe":            clsRead,
	"SAP rfc op=describe":         clsRead,
	"SAP rfc (no op, target)":     clsRead,
	"SAP rfc op=call":             clsExecute,
	"SAP rfc (args imply call)":   clsExecute,
	"SAP rfc op=search":           clsRead,
	"SAP rfc op=read_table":       clsRead,
	"SAP rfc op=read-table":       clsRead,
	"SAP rfc op=table":            clsRead,
	"SAP rfc op=read_table where": clsRead,    // free SQL; --block-free-sql governs it (#283)
	"SAP rfc op=run":              clsExecute, // schedules the report as an XBP background job (#261)
	"SAP rfc op=job":              clsRead,    // TBTCO status, job log, spool list of an existing job (#261)

	// --- SAP(): i18n, revisions ---
	"SAP i18n op=texts":               clsRead,
	"SAP i18n op=data_element_labels": clsRead,
	"SAP i18n op=message_class_texts": clsRead,
	"SAP i18n op=text_pool":           clsRead,
	"SAP i18n op=texts_get":           clsRead,
	"SAP i18n op=compare_languages":   clsRead,
	"SAP i18n op=texts_set":           clsMutate,
	"SAP i18n op=write_labels":        clsMutate,
	"SAP i18n op=write_message_texts": clsMutate,
	"SAP i18n op=write_text_pool":     clsMutate,
	"SAP revisions op=list":           clsRead,
	"SAP revisions op=source":         clsRead,
	"SAP revisions op=compare":        clsRead,
	"SAP history":                     clsRead,
}

// The known gaps, each already named in the README's read-only notes.
const (
	gapBreakpoints = "setting and deleting breakpoints is not gated by --read-only (README, known gaps): " +
		"a breakpoint changes no object or data, and refusing it would make the read-only debugger useless"
	gapStepping = "debugger stepping is not gated by --read-only (README, known gaps): it advances a program " +
		"somebody else started and stopped; whether that is execution under read-only is still open"
	gapAMDP = "starting and driving an AMDP debug session is not gated by --read-only (README, known gaps); " +
		"the session runs the procedure the user triggers, and its breakpoints are session state"
)

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

// toolArgOverrides are the few tools whose parameters have a shape the name
// alone does not give away. Without them the call fails validation before it
// reaches the gate, and the probe would prove nothing.
var toolArgOverrides = map[string]map[string]any{
	"CreateObject":           {"object_type": "PROG/P"},
	"RenameObject":           {"objType": "PROG/P"},
	"ActivateMultiple":       {"objects": []any{"PROG ZDEMO_REPORT"}},
	"CreateTable":            {"fields": `[{"name":"MANDT","type":"CLNT","key":true},{"name":"ID","type":"CHAR","length":10,"key":true}]`},
	"WriteMessageClassTexts": {"texts": []any{map[string]any{"number": "001", "text": "Demo"}}},
	"InstallAbapGit":         {"edition": "standalone"},
	"DeployZip":              {"source": "abapgit-standalone"},
	"CompareCallGraphs":      {"trace_data": "{}"},
	"TraceExecution":         {"run_tests": true},
	"UnlockObject":           {"lock_handle": "FAKELOCKHANDLE"},
	"RecoverFailedCreate":    {"object_type": "PROG/P"},
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

// actionCases are the SAP() actions routed by switch statements, which cannot
// be enumerated at run time. TestReadOnlyInvariant parses the route functions
// and fails on any routed literal no case here covers, so a new case in a
// router cannot go unclassified either.
func actionCases() []actionCase {
	obj := synthObjectURL
	srcEdit := func(typ, name string) actionCase {
		return actionCase{Name: "SAP edit " + typ, Action: "edit", Target: typ + " " + name,
			Exact: true, Params: kv("source", fakeSource, "package", "$TMP", "transport", "TR-EXAMPLE", "description", "Demo")}
	}
	srcRead := func(typ, name string) actionCase {
		return actionCase{Name: "SAP read " + typ, Action: "read", Target: typ + " " + name, Exact: true, Params: kv("parent", "ZDEMO_FG")}
	}
	sys := func(typ, like string, extra ...any) actionCase {
		return actionCase{Name: "SAP system type=" + typ, Action: "system", Like: like, Params: mergeArgs(kv("type", typ), kv(extra...))}
	}
	dbg := func(target, like string, extra ...any) actionCase {
		return actionCase{Name: "SAP debug " + target, Action: "debug", Target: target, Like: like, Params: kv(extra...)}
	}
	an := func(typ, like string, extra ...any) actionCase {
		return actionCase{Name: "SAP analyze type=" + typ, Action: "analyze", Like: like, Params: mergeArgs(kv("type", typ), kv(extra...))}
	}
	rfc := func(name, target string, extra ...any) actionCase {
		return actionCase{Name: "SAP rfc " + name, Action: "rfc", Target: target, Exact: true, Params: kv(extra...)}
	}
	cases := []actionCase{
		{Name: "SAP info", Action: "info", Exact: true},
		{Name: "SAP (no action)", Action: "", Exact: true},
		{Name: "SAP help", Action: "help", Exact: true},

		// read
		srcRead("PROG", "ZDEMO_REPORT"), srcRead("CLAS", "ZCL_DEMO"), srcRead("INTF", "ZIF_DEMO"),
		srcRead("FUNC", "Z_DEMO_FM"), srcRead("FUGR", "ZDEMO_FG"), srcRead("INCL", "ZDEMO_INCL"),
		srcRead("DDLS", "ZDEMO_DDLS"), srcRead("BDEF", "ZDEMO_BDEF"), srcRead("SRVD", "ZDEMO_SRV"),
		srcRead("SRVB", "ZDEMO_SRVB"), srcRead("MSAG", "ZDEMO_MSG"), srcRead("VIEW", "ZDEMO_VIEW"),
		srcRead("ENHO", "ZDEMO_ENHO"), srcRead("TABL", "ZDEMO_TAB"), srcRead("DEVC", "$TMP"),
		srcRead("IDOC", "0000000000000001"), srcRead("TRAN", "ZDEMO_TCODE"), srcRead("TYPE_INFO", "ZDEMO_TYPE"),
		srcRead("STRUCT", "ZDEMO_TAB"), srcRead("CDS_DEPS", "ZDEMO_DDLS"), srcRead("CDS_IMPACT", "ZDEMO_DDLS"),
		srcRead("CDS_ELEMENTS", "ZDEMO_DDLS"), srcRead("TABL_CONTENTS", "T000"), srcRead("CHECK_RUN", "ZDEMO_RUN"),
		srcRead("API_STATE", obj), srcRead("CLASS_INFO", "ZCL_DEMO"), srcRead("UI5_APP", "ZDEMO_APP"),
		{Name: "SAP read ENHANCEMENT_OPTIONS", Action: "read", Target: "ENHANCEMENT_OPTIONS ZDEMO_REPORT", Params: kv("object_type", "PROG")},
		{Name: "SAP read CLAS_INCLUDE", Action: "read", Target: "CLAS_INCLUDE ZCL_DEMO", Exact: true, Params: kv("include_type", "testclasses")},
		{Name: "SAP read COVERAGE", Action: "read", Target: "COVERAGE " + obj, Exact: true},
		{Name: "SAP read COVERAGE include_dangerous", Action: "read", Target: "COVERAGE " + obj, Exact: true, Params: kv("include_dangerous", true)},
		{Name: "SAP read UI5_LIST", Action: "read", Target: "UI5_LIST", Like: "UI5ListApps"},
		{Name: "SAP read UI5_FILE", Action: "read", Target: "UI5_FILE", Like: "UI5GetFileContent"},

		// query
		{Name: "SAP query TABL_CONTENTS", Action: "query", Target: "TABL_CONTENTS T000", Exact: true, Params: kv("max_rows", 1.0)},
		{Name: "SAP query SQL", Action: "query", Target: "SQL", Exact: true, Params: kv("sql", "SELECT * FROM T000")},
		{Name: "SAP query SQL table", Action: "query", Target: "SQL T000", Exact: true},
		{Name: "SAP query (no params)", Action: "query", Exact: true},

		// search and grep
		{Name: "SAP search", Action: "search", Target: "ZDEMO*", Exact: true},
		{Name: "SAP grep package", Action: "grep", Exact: true, Params: kv("pattern", "WRITE", "package", "$TMP")},
		{Name: "SAP grep packages", Action: "grep", Exact: true, Params: kv("pattern", "WRITE", "packages", []any{"$TMP"})},
		{Name: "SAP grep object", Action: "grep", Exact: true, Params: kv("pattern", "WRITE", "object_url", obj)},
		{Name: "SAP grep objects", Action: "grep", Exact: true, Params: kv("pattern", "WRITE", "object_urls", []any{obj})},
		{Name: "SAP grep (no target)", Action: "grep", Exact: true, Params: kv("pattern", "WRITE")},

		// edit
		srcEdit("PROG", "ZDEMO_REPORT"), srcEdit("CLAS", "ZCL_DEMO"), srcEdit("INTF", "ZIF_DEMO"),
		srcEdit("FUNC", "Z_DEMO_FM"), srcEdit("INCL", "ZDEMO_INCL"), srcEdit("DDLS", "ZDEMO_DDLS"),
		srcEdit("BDEF", "ZDEMO_BDEF"), srcEdit("SRVD", "ZDEMO_SRV"), srcEdit("MSAG", "ZDEMO_MSG"),
		srcEdit("TABL", "ZDEMO_TAB"),
		{Name: "SAP edit EDITSOURCE", Action: "edit", Target: "EDITSOURCE", Like: "EditSource"},
		{Name: "SAP edit LOCK", Action: "edit", Target: "LOCK", Like: "LockObject"},
		{Name: "SAP edit UNLOCK", Action: "edit", Target: "UNLOCK", Like: "UnlockObject"},
		{Name: "SAP edit UPDATE_SOURCE", Action: "edit", Target: "UPDATE_SOURCE", Like: "UpdateSource"},
		{Name: "SAP edit MOVE", Action: "edit", Target: "MOVE", Like: "MoveObject"},
		{Name: "SAP edit COMPARE_SOURCE", Action: "edit", Target: "COMPARE_SOURCE", Like: "CompareSource"},
		{Name: "SAP edit RECOVER_FAILED_CREATE", Action: "edit", Target: "RECOVER_FAILED_CREATE", Like: "RecoverFailedCreate"},
		{Name: "SAP edit ACTIVATE", Action: "edit", Target: "ACTIVATE", Like: "Activate"},
		{Name: "SAP edit ACTIVATE_MULTI", Action: "edit", Target: "ACTIVATE_MULTI", Like: "ActivateMultiple"},
		{Name: "SAP edit ACTIVATE_PACKAGE", Action: "edit", Target: "ACTIVATE_PACKAGE", Like: "ActivatePackage"},
		{Name: "SAP edit CLAS_INCLUDE", Action: "edit", Target: "CLAS_INCLUDE", Like: "UpdateClassInclude"},
		{Name: "SAP edit PUBLISH_SERVICE", Action: "edit", Target: "PUBLISH_SERVICE", Like: "PublishServiceBinding"},
		{Name: "SAP edit UNPUBLISH_SERVICE", Action: "edit", Target: "UNPUBLISH_SERVICE", Like: "UnpublishServiceBinding"},
		{Name: "SAP edit type=publish_service", Action: "edit", Like: "PublishServiceBinding", Params: kv("type", "publish_service")},
		{Name: "SAP edit type=unpublish_service", Action: "edit", Like: "UnpublishServiceBinding", Params: kv("type", "unpublish_service")},
		{Name: "SAP edit type=write_program", Action: "edit", Like: "WriteProgram", Params: kv("type", "write_program")},
		{Name: "SAP edit type=write_class", Action: "edit", Like: "WriteClass", Params: kv("type", "write_class")},
		// Exact: with a source parameter, routeSourceAction would claim the
		// call as a WriteSource before routeWorkflowAction saw it.
		{Name: "SAP edit type=set_description", Action: "edit", Target: "PROG ZDEMO_REPORT", Exact: true, Params: kv("type", "set_description", "description", "Demo")},
		{Name: "SAP edit type=description", Action: "edit", Target: "PROG ZDEMO_REPORT", Exact: true, Params: kv("type", "description", "description", "Demo")},
		{Name: "SAP edit type=deploy_from_file", Action: "edit", Like: "DeployFromFile", Params: kv("type", "deploy_from_file")},
		{Name: "SAP edit type=save_to_file", Action: "edit", Like: "SaveToFile", Params: kv("type", "save_to_file")},
		{Name: "SAP edit type=rename", Action: "edit", Like: "RenameObject", Params: kv("type", "rename")},
		{Name: "SAP system type=deploy_from_file", Action: "system", Like: "DeployFromFile", Params: kv("type", "deploy_from_file")},
		{Name: "SAP system type=save_to_file", Action: "system", Like: "SaveToFile", Params: kv("type", "save_to_file")},
		{Name: "SAP system type=rename", Action: "system", Like: "RenameObject", Params: kv("type", "rename")},
		{Name: "SAP edit UI5_UPLOAD", Action: "edit", Target: "UI5_UPLOAD", Like: "UI5UploadFile"},

		// create
		{Name: "SAP create OBJECT", Action: "create", Target: "OBJECT", Like: "CreateObject"},
		{Name: "SAP create DEVC", Action: "create", Target: "DEVC", Like: "CreatePackage"},
		{Name: "SAP create TABL", Action: "create", Target: "TABL", Like: "CreateTable"},
		{Name: "SAP create CLONE", Action: "create", Target: "CLONE", Like: "CloneObject"},
		{Name: "SAP create ENHO", Action: "create", Target: "ENHO", Exact: true, Params: kv("name", "ZDEMO_ENHO", "spot", "ZDEMO_SPOT",
			"option", "ZDEMO_OPTION", "program", "ZDEMO_REPORT", "source", fakeSource, "package", "$TMP", "description", "Demo")},
		{Name: "SAP create BADI_IMPL", Action: "create", Target: "BADI_IMPL", Exact: true, Params: kv("name", "ZDEMO_BADI_IMPL", "spot", "ZDEMO_SPOT",
			"badi", "ZDEMO_BADI", "implementation", "ZDEMO_IMPL", "class", "ZCL_DEMO", "package", "$TMP", "description", "Demo")},
		{Name: "SAP create DOMA", Action: "create", Target: "DOMA ZDEMO_DOMA", Exact: true, Params: kv("description", "Demo", "package", "$TMP", "data_type", "CHAR", "length", 10.0)},
		{Name: "SAP create DTEL", Action: "create", Target: "DTEL ZDEMO_DTEL", Exact: true, Params: kv("description", "Demo", "package", "$TMP", "domain", "ZDEMO_DOMA")},
		{Name: "SAP create STRUCT", Action: "create", Target: "STRUCT ZDEMO_STRUCT", Exact: true, Params: kv("description", "Demo", "package", "$TMP", "source", "@EndUserText.label : 'Demo'\ndefine structure zdemo_struct { field : abap.char(10); }")},
		{Name: "SAP create APPEND", Action: "create", Target: "APPEND ZDEMO_APPEND", Exact: true, Params: kv("description", "Demo", "package", "$TMP", "source", "@EndUserText.label : 'Demo'\nextend type zdemo_tab with zdemo_append { zzfield : abap.char(10); }")},
		{Name: "SAP create CLAS_TEST_INCLUDE", Action: "create", Target: "CLAS_TEST_INCLUDE", Like: "CreateTestInclude"},
		{Name: "SAP create PROGRAM", Action: "create", Target: "PROGRAM", Like: "CreateAndActivateProgram"},
		{Name: "SAP create CLASS_WITH_TESTS", Action: "create", Target: "CLASS_WITH_TESTS", Like: "CreateClassWithTests"},
		{Name: "SAP create UI5_APP", Action: "create", Target: "UI5_APP", Like: "UI5CreateApp"},

		// delete
		{Name: "SAP delete OBJECT", Action: "delete", Target: "OBJECT", Like: "DeleteObject"},
		{Name: "SAP delete (no target)", Action: "delete", Like: "DeleteObject"},
		{Name: "SAP delete UI5_FILE", Action: "delete", Target: "UI5_FILE", Like: "UI5DeleteFile"},
		{Name: "SAP delete UI5_APP", Action: "delete", Target: "UI5_APP", Like: "UI5DeleteApp"},

		// test
		{Name: "SAP test unit", Action: "test", Exact: true, Params: kv("object_url", obj)},
		{Name: "SAP test type=unit include_dangerous", Action: "test", Exact: true, Params: kv("type", "unit", "object_url", obj, "include_dangerous", true)},
		{Name: "SAP test type=atc", Action: "test", Like: "RunATCCheck", Params: kv("type", "atc")},
		{Name: "SAP test type=atc_customizing", Action: "test", Like: "GetATCCustomizing", Params: kv("type", "atc_customizing")},

		// analyze types routed by switch
		an("definition", "FindDefinition"), an("references", "FindReferences"), an("completion", "CodeCompletion"),
		an("pretty_print", "PrettyPrint"), an("get_pretty_printer_settings", "GetPrettyPrinterSettings"),
		an("set_pretty_printer_settings", "SetPrettyPrinterSettings"), an("type_hierarchy", "GetTypeHierarchy"),
		an("class_components", "GetClassComponents"), an("inactive_objects", "GetInactiveObjects"),
		an("abap_help", "GetAbapHelp"), an("syntax_check", "SyntaxCheck"), an("execute_abap", "ExecuteABAP"),

		// debug
		dbg("AMDP_ADT_START", "AMDPDebuggerStart"), dbg("AMDP_ADT_BREAKPOINT", "AMDPSetBreakpoint"),
		dbg("AMDP_ADT_AWAIT", "AMDPDebuggerResume"), dbg("AMDP_ADT_STOP", "AMDPDebuggerStop"),
		dbg("AMDP_START", "AMDPDebuggerStart"), dbg("AMDP_RESUME", "AMDPDebuggerResume"),
		dbg("AMDP_STOP", "AMDPDebuggerStop"), dbg("AMDP_STEP", "AMDPDebuggerStep"),
		dbg("AMDP_GET_VARIABLES", "AMDPGetVariables"), dbg("AMDP_SET_BREAKPOINT", "AMDPSetBreakpoint"),
		dbg("AMDP_GET_BREAKPOINTS", "AMDPGetBreakpoints"),
		dbg("SET_BREAKPOINT", "SetBreakpoint"), dbg("GET_BREAKPOINTS", "GetBreakpoints"),
		dbg("DELETE_BREAKPOINT", "DeleteBreakpoint"), dbg("CALL_RFC", "CallRFC"), dbg("MOVE", "MoveObject"),
		dbg("LISTEN", "DebuggerListen"), dbg("ATTACH", "DebuggerAttach"), dbg("DETACH", "DebuggerDetach"),
		dbg("STEP", "DebuggerStep"), dbg("GET_STACK", "DebuggerGetStack"), dbg("GET_VARIABLES", "DebuggerGetVariables"),
		dbg("RUN_REPORT", "RunReport"), dbg("RUN_REPORT_ASYNC", "RunReportAsync"), dbg("GET_ASYNC_RESULT", "GetAsyncResult"),
		dbg("GET_VARIANTS", "GetVariants"), dbg("GET_TEXT_ELEMENTS", "GetTextElements"), dbg("SET_TEXT_ELEMENTS", "SetTextElements"),

		// system
		{Name: "SAP system INFO", Action: "system", Target: "INFO", Exact: true},
		{Name: "SAP system COMPONENTS", Action: "system", Target: "COMPONENTS", Exact: true},
		{Name: "SAP system CONNECTION", Action: "system", Target: "CONNECTION", Exact: true},
		{Name: "SAP system FEATURES", Action: "system", Target: "FEATURES", Exact: true},
		sys("git_types", "GitTypes"), sys("git_export", "GitExport"),
		sys("install_zadt_vsp", "InstallZADTVSP"), sys("install_abapgit", "InstallAbapGit"),
		sys("install_dummy_test", "InstallDummyTest"), sys("list_dependencies", "ListDependencies"),
		sys("deploy_zip", "DeployZip"),
		sys("list_transports", "ListTransports"), sys("get_transport", "GetTransport"),
		sys("create_transport", "CreateTransport"), sys("release_transport", "ReleaseTransport"),
		sys("delete_transport", "DeleteTransport"), sys("get_user_transports", "GetUserTransports"),
		sys("get_transport_info", "GetTransportInfo"), sys("execute_abap", "ExecuteABAP"),
		{Name: "SAP system type=merge_transports", Action: "system", Exact: true, Params: kv("type", "merge_transports", "source", "TR-EXAMPLE", "target", "TR-EXAMPLE2")},
		{Name: "SAP system type=move_transport_object", Action: "system", Exact: true, Params: kv("type", "move_transport_object", "object", "R3TR PROG ZDEMO_REPORT", "from", "TR-EXAMPLE", "to", "TR-EXAMPLE2")},
		{Name: "SAP system type=move_object", Action: "system", Exact: true, Params: kv("type", "move_object", "object", "R3TR PROG ZDEMO_REPORT", "from", "TR-EXAMPLE", "to", "TR-EXAMPLE2")},
		{Name: "SAP system type=copy_to_toc", Action: "system", Exact: true, Params: kv("type", "copy_to_toc", "transport", "TR-EXAMPLE", "target", "QAS")},
		{Name: "SAP system type=transport_of_copies", Action: "system", Exact: true, Params: kv("type", "transport_of_copies", "transport", "TR-EXAMPLE", "target", "QAS")},
		{Name: "SAP system type=add_transport_object", Action: "system", Exact: true, Params: kv("type", "add_transport_object", "object", "R3TR PROG ZDEMO_REPORT", "transport", "TR-EXAMPLE")},
		{Name: "SAP system type=add_to_transport", Action: "system", Exact: true, Params: kv("type", "add_to_transport", "object", "R3TR PROG ZDEMO_REPORT", "transport", "TR-EXAMPLE")},
		{Name: "SAP system type=remove_transport_object", Action: "system", Exact: true, Params: kv("type", "remove_transport_object", "object", "R3TR PROG ZDEMO_REPORT", "transport", "TR-EXAMPLE")},
		{Name: "SAP system type=remove_from_transport", Action: "system", Exact: true, Params: kv("type", "remove_from_transport", "object", "R3TR PROG ZDEMO_REPORT", "transport", "TR-EXAMPLE")},
		sys("ui5_list_apps", "UI5ListApps"), sys("ui5_get_app", "UI5GetApp"), sys("ui5_get_file", "UI5GetFileContent"),
		sys("ui5_upload_file", "UI5UploadFile"), sys("ui5_delete_file", "UI5DeleteFile"),
		sys("ui5_create_app", "UI5CreateApp"), sys("ui5_delete_app", "UI5DeleteApp"),

		// rfc
		rfc("op=info", "", "op", "info"), rfc("(no op, no target)", ""),
		rfc("op=ping", "", "op", "ping"), rfc("op=probe", "", "op", "probe"),
		rfc("op=describe", "STFC_CONNECTION", "op", "describe"), rfc("(no op, target)", "STFC_CONNECTION"),
		rfc("op=call", "Z_DEMO_FM", "op", "call", "args", map[string]any{"N": 21.0}),
		rfc("(args imply call)", "Z_DEMO_FM", "args", map[string]any{"N": 21.0}),
		rfc("op=search", "ZDEMO*", "op", "search"),
		rfc("op=read_table", "T000", "op", "read_table", "top", 1.0),
		rfc("op=read-table", "T000", "op", "read-table", "top", 1.0),
		rfc("op=table", "T000", "op", "table", "top", 1.0),
		rfc("op=read_table where", "T000", "op", "read_table", "top", 1.0, "where", "MANDT = '001'"),
		rfc("op=run", "ZDEMO_REPORT", "op", "run", "wait", 0.0),
		rfc("op=job", "VSP_ZDEMO_REPORT", "op", "job", "job_count", "12345678"),

		{Name: "SAP history", Action: "history", Target: "PROG ZDEMO_REPORT", Exact: true},
	}
	return cases
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
	return o
}

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

// packageGated are the object-scoped mutations a server started with
// --allowed-packages "Z*" (and without --read-only) must refuse for an object
// in $TMP, before any write. An empty value means the package gate must hold;
// a non-empty one is a known gap and its reason.
var packageGated = map[string]string{
	"tool CloneObject":              "",
	"tool CreateAndActivateProgram": "",
	"tool CreateClassWithTests":     "",
	"tool CreateObject":             "",
	"tool CreateTable":              "",
	"tool CreateTestInclude":        "",
	"tool DeleteObject":             "",
	"tool DeployFromFile":           "",
	"tool EditSource":               "",
	"tool ExecuteABAP":              "", // its temporary program goes to $TMP
	"tool ImportFromFile":           "",
	"tool MoveObject":               "", // into $TMP: refused for the target package
	"tool MoveObject out of $TMP":   "", // into ZDEMO_PKG: refused for the package the object is in now
	"tool RecoverFailedCreate":      "",
	"tool RenameObject":             "",
	"tool UI5CreateApp":             "",
	"tool UI5UploadFile":            "", // fails closed: UI5 app→package resolution is not implemented
	"tool UpdateClassInclude":       "",
	"tool UpdateSource":             "",
	"tool WriteClass":               "",
	"tool WriteDataElementLabels":   "",
	"tool WriteMessageClassTexts":   "",
	"tool WriteProgram":             "",
	"tool WriteSource":              "",
	"SAP edit PROG":                 "",
	"SAP edit type=set_description": "",
	"SAP create DOMA":               "",
	"SAP create DTEL":               "",
	"SAP create STRUCT":             "",
	"SAP create BADI_IMPL":          "",
	"SAP i18n op=texts_set":         "",

	"tool Activate":                gapActivationPackage,
	"tool ActivateMultiple":        gapActivationPackage,
	"tool CreateIAMApp":            gapActivationPackage + "; an IAM object that already exists goes straight to activation",
	"tool LockObject":              "a MODIFY lock is checked as an operation but not against --allowed-packages; the write it precedes is",
	"tool PublishServiceBinding":   "publishing a service binding is checked as an update (#283) but not against --allowed-packages: the binding's package needs a lookup",
	"tool UnpublishServiceBinding": "unpublishing a service binding is checked as an update (#283) but not against --allowed-packages: the binding's package needs a lookup",
	"tool SetTextElements":         "writes a program's texts through ZADT_VSP; refused under --read-only (#283), not checked against --allowed-packages: the program's package needs a lookup",
}

const gapActivationPackage = "activation is checked as an operation (A) but not against --allowed-packages: resolving each " +
	"object's package would add a lookup to every write workflow's activation, and fail closed for objects the quick search cannot place"

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
