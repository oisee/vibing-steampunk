package dap

import (
	"context"
	"fmt"
	"net/url"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/oisee/vibing-steampunk/pkg/saprfc"
)

// fakeADT is SAP's debugger as the ADT resources present it, enough of it to
// carry a whole DAP conversation: the breakpoint set, the blocking listener,
// attach, stack, the variable tree, frame moves, steps and the teardown. Its
// documents are shaped after what A4H answered (pkg/saprfc's a4h-step
// cassette), and it runs ZVSP_DEBUG_DEMO from pkg/saprfc's fixtures.
//
// The program runs when the test says so: run() is "somebody executed the
// report in SAP GUI", and it stops if a breakpoint is registered on its path.
type fakeADT struct {
	mu sync.Mutex

	bps       map[int]bool // registered lines in ZVSP_DEBUG_DEMO
	bpPosts   int
	lastBPSet int // size of the last posted set
	runs      chan struct{}

	listening bool
	attached  bool
	line      int  // where the debuggee is
	inForm    bool // inside FORM double
	cursor    string
	counter   string

	listenerDeleted bool

	// Faults for the teardown tests.
	failStep    string        // a step method that fails with a real error
	hangBPClear bool          // posting the empty set hangs until its context ends
	slowRelease time.Duration // deleting the listener takes this long
	detachTried bool
	log         []string
}

func newFakeADT() *fakeADT {
	return &fakeADT{bps: map[int]bool{}, runs: make(chan struct{}, 4), counter: "7"}
}

// run starts the report, as a user in SAP GUI would.
func (f *fakeADT) run() { f.runs <- struct{}{} }

func (f *fakeADT) calls() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]string(nil), f.log...)
}

const demoURI = "/sap/bc/adt/programs/programs/zvsp_debug_demo"

var bpLine = regexp.MustCompile(`adtcore:uri="([^"#]+)#start=(\d+)"`)
var parentID = regexp.MustCompile(`<PARENT_ID>([^<]*)</PARENT_ID>`)
var varID = regexp.MustCompile(`<ID>([^<]*)</ID>`)

func ok(body string) *saprfc.ADTResponse {
	return &saprfc.ADTResponse{Status: 200, ReasonPhrase: "OK", Body: []byte(body)}
}

func exception(status int, reason, subType, msg string) *saprfc.ADTResponse {
	return &saprfc.ADTResponse{Status: status, ReasonPhrase: reason, Body: []byte(
		`<?xml version="1.0" encoding="utf-8"?><exc:exception xmlns:exc="http://www.sap.com/abapxml/types/communicationframework">` +
			`<namespace id="com.sap.adt"/><type id="AdiFailed"/><message lang="EN">` + msg + `</message>` +
			`<properties><entry key="com.sap.adt.communicationFramework.subType">` + subType + `</entry></properties></exc:exception>`)}
}

func (f *fakeADT) Do(ctx context.Context, req saprfc.ADTRequest) (*saprfc.ADTResponse, error) {
	u, err := url.Parse(req.URI)
	if err != nil {
		return nil, err
	}
	q := u.Query()
	method := q.Get("method")

	f.mu.Lock()
	entry := req.Method + " " + u.Path
	if method != "" {
		entry += "?method=" + method
	}
	f.log = append(f.log, entry)
	f.mu.Unlock()

	switch {
	case req.Method == "GET" && u.Path == "/sap/bc/adt/repository/informationsystem/search":
		name := strings.ToLower(q.Get("query"))
		if name != "zvsp_debug_demo" {
			return ok(`<?xml version="1.0" encoding="utf-8"?><adtcore:objectReferences xmlns:adtcore="http://www.sap.com/adt/core"/>`), nil
		}
		return ok(`<?xml version="1.0" encoding="utf-8"?><adtcore:objectReferences xmlns:adtcore="http://www.sap.com/adt/core">` +
			`<adtcore:objectReference adtcore:uri="` + demoURI + `" adtcore:type="PROG/P" adtcore:name="ZVSP_DEBUG_DEMO" adtcore:packageName="$TMP"/></adtcore:objectReferences>`), nil

	case u.Path == "/sap/bc/adt/debugger/breakpoints" && req.Method == "GET":
		return ok(""), nil

	case u.Path == "/sap/bc/adt/debugger/breakpoints" && req.Method == "POST":
		f.mu.Lock()
		if f.hangBPClear && !bpLine.Match(req.Body) {
			f.mu.Unlock()
			<-ctx.Done()
			return nil, ctx.Err()
		}
		defer f.mu.Unlock()
		f.bpPosts++
		f.bps = map[int]bool{}
		var sb strings.Builder
		sb.WriteString(`<?xml version="1.0" encoding="utf-8"?><dbg:breakpoints xmlns:dbg="http://www.sap.com/adt/debugger">`)
		matches := bpLine.FindAllStringSubmatch(string(req.Body), -1)
		f.lastBPSet = len(matches)
		for _, m := range matches {
			uri, line := m[1], m[2]
			if line == "1" {
				// REPORT carries no statement to stop at.
				fmt.Fprintf(&sb, `<breakpoint kind="line" errorMessage="Cannot create a breakpoint at this position" adtcore:uri="%s#start=%s" xmlns:adtcore="http://www.sap.com/adt/core"/>`, uri, line)
				continue
			}
			var n int
			fmt.Sscan(line, &n)
			if strings.EqualFold(uri, demoURI+"/source/main") {
				f.bps[n] = true
			}
			fmt.Fprintf(&sb, `<breakpoint kind="line" id="KIND=0.MAIN_PROGRAM=ZVSP_DEBUG_DEMO.LINE_NR=%s" adtcore:uri="%s#start=%s" adtcore:type="PROG/P" adtcore:name="ZVSP_DEBUG_DEMO" xmlns:adtcore="http://www.sap.com/adt/core"/>`, line, uri, line)
		}
		sb.WriteString(`</dbg:breakpoints>`)
		return ok(sb.String()), nil

	case u.Path == "/sap/bc/adt/debugger/listeners" && req.Method == "POST":
		f.mu.Lock()
		f.listening = true
		f.mu.Unlock()
		for {
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-f.runs:
			}
			f.mu.Lock()
			if !f.bps[26] {
				f.mu.Unlock()
				continue // the report ran through: no breakpoint on its path
			}
			f.line, f.inForm, f.cursor = 26, false, ""
			f.mu.Unlock()
			return ok(`<?xml version="1.0" encoding="utf-8"?><asx:abap version="1.0" xmlns:asx="http://www.sap.com/abapxml"><asx:values><DATA><STPDA_DEBUGGEE>` +
				`<CLIENT>001</CLIENT><DEBUGGEE_ID>DBGEE1</DEBUGGEE_ID><DEBUGGEE_USER>TESTUSER</DEBUGGEE_USER><PRG_CURR>ZVSP_DEBUG_DEMO</PRG_CURR>` +
				`<INCL_CURR>ZVSP_DEBUG_DEMO</INCL_CURR><LINE_CURR>26</LINE_CURR><DBGEE_KIND>DEBUGGEE</DBGEE_KIND><IS_ATTACH_IMPOSSIBLE>false</IS_ATTACH_IMPOSSIBLE>` +
				`<URI>` + demoURI + `/source/main#start=26</URI></STPDA_DEBUGGEE></DATA></asx:values></asx:abap>`), nil
		}

	case u.Path == "/sap/bc/adt/debugger/listeners" && req.Method == "DELETE":
		f.mu.Lock()
		slow := f.slowRelease
		f.mu.Unlock()
		if slow > 0 {
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(slow):
			}
		}
		f.mu.Lock()
		f.listenerDeleted = true
		f.listening = false
		f.mu.Unlock()
		return ok(""), nil

	case u.Path == "/sap/bc/adt/debugger" && method == "attach":
		f.mu.Lock()
		f.attached = true
		f.mu.Unlock()
		return ok(`<?xml version="1.0" encoding="utf-8"?><dbg:attach isRfc="false" isSameSystem="true" debugSessionId="S1" isSteppingPossible="true" xmlns:dbg="http://www.sap.com/adt/debugger">` +
			`<dbg:reachedBreakpoints><dbg:breakpoint id="KIND=0.MAIN_PROGRAM=ZVSP_DEBUG_DEMO.LINE_NR=26" kind="line"/></dbg:reachedBreakpoints></dbg:attach>`), nil

	case u.Path == "/sap/bc/adt/debugger/stack":
		f.mu.Lock()
		defer f.mu.Unlock()
		if !f.attached {
			return exception(400, "Bad Request", "noSessionAttached", "No session attached"), nil
		}
		return ok(f.stackXML()), nil

	case req.Method == "PUT" && strings.HasPrefix(u.Path, "/sap/bc/adt/debugger/stack/type/"):
		f.mu.Lock()
		f.cursor = u.Path
		f.mu.Unlock()
		return ok(""), nil

	case u.Path == "/sap/bc/adt/debugger" && method == "getChildVariables":
		f.mu.Lock()
		defer f.mu.Unlock()
		return ok(f.children(parentID.FindAllStringSubmatch(string(req.Body), -1))), nil

	case u.Path == "/sap/bc/adt/debugger" && method == "getVariables":
		f.mu.Lock()
		defer f.mu.Unlock()
		var vars []string
		for _, m := range varID.FindAllStringSubmatch(string(req.Body), -1) {
			switch m[1] {
			case "LT_ROWS":
				vars = append(vars, variable("LT_ROWS", "LT_ROWS", "TY_ROWS", "table", "", 2))
			case "LV_COUNTER":
				vars = append(vars, variable("LV_COUNTER", "LV_COUNTER", "I", "simple", f.counter+" ", 0))
			}
		}
		return ok(abapXML(`<DATA>` + strings.Join(vars, "") + `</DATA>`)), nil

	case u.Path == "/sap/bc/adt/debugger" && method == "setVariableValue":
		f.mu.Lock()
		if q.Get("variableName") == "LV_COUNTER" {
			f.counter = string(req.Body)
		}
		f.mu.Unlock()
		return ok(string(req.Body)), nil

	case u.Path == "/sap/bc/adt/debugger" && strings.HasPrefix(method, "step"):
		f.mu.Lock()
		defer f.mu.Unlock()
		if !f.attached {
			return exception(400, "Bad Request", "noSessionAttached", "No session attached"), nil
		}
		f.cursor = ""
		switch {
		case method == f.failStep:
			return exception(500, "Internal Server Error", "kernelError", "The work process was cancelled"), nil
		case method == "stepContinue":
			f.attached = false
			return exception(500, "Internal Server Error", "debuggeeEnded", "An exception was raised"), nil
		case method == "stepInto" && f.line == 26 && !f.inForm:
			f.line, f.inForm = 9, true
		case method == "stepReturn" && f.inForm, method == "stepOver" && f.line == 26:
			f.line, f.inForm = 28, false
		case method == "stepOver" && f.line == 28:
			f.attached = false
			return exception(500, "Internal Server Error", "debuggeeEnded", "An exception was raised"), nil
		default:
			f.line++
		}
		return ok(`<?xml version="1.0" encoding="utf-8"?><dbg:step isRfc="false" isSameSystem="true" debugSessionId="S1" isSteppingPossible="true" isDebuggeeChanged="false" xmlns:dbg="http://www.sap.com/adt/debugger"><dbg:reachedBreakpoints/></dbg:step>`), nil

	case u.Path == "/sap/bc/adt/debugger" && method == "detach":
		f.mu.Lock()
		f.detachTried = true
		f.mu.Unlock()
		// What A4H answers: it does not know the method, and the client
		// falls back to letting the debuggee run on.
		return exception(400, "Bad Request", "", "Unknown method ''"), nil

	case u.Path == "/sap/bc/adt/debugger" && method == "terminateDebuggee":
		f.mu.Lock()
		f.attached = false
		f.mu.Unlock()
		return ok(""), nil

	case req.Method == "GET" && u.Path == demoURI+"/source/main":
		return ok("REPORT zvsp_debug_demo.\n"), nil
	}
	return exception(404, "Not Found", "", "no fake for "+req.Method+" "+req.URI), nil
}

func (f *fakeADT) stackXML() string {
	entry := func(pos int, line int, eventType, eventName string, active bool) string {
		return fmt.Sprintf(`<stackEntry stackPosition="%d" stackType="ABAP" stackUri="/sap/bc/adt/debugger/stack/type/ABAP/position/%d" programName="ZVSP_DEBUG_DEMO" includeName="ZVSP_DEBUG_DEMO" line="%d" eventType="%s" eventName="%s" sourceType="ABAP" systemProgram="false" isVit="false" isActive="%v" adtcore:uri="%s/source/main#start=%d,0" xmlns:adtcore="http://www.sap.com/adt/core"/>`,
			pos, pos, line, eventType, eventName, active, demoURI, line)
	}
	dynp := `<stackEntry stackPosition="1" stackType="DYNP" stackUri="/sap/bc/adt/debugger/stack/type/DYNP/position/1" programName="SAPMSSY0" includeName="1000" line="6" eventType="PAI SCREEN" eventName="1000" sourceType="DYNP" systemProgram="true" isVit="true" isActive="false" adtcore:uri="/sap/bc/adt/vit/wb/object_type/progps/object_name/SAPMSSY0#start=6" xmlns:adtcore="http://www.sap.com/adt/core"/>`
	var entries string
	if f.inForm {
		entries = entry(2, f.line, "FORM", "DOUBLE", true) + entry(1, 26, "EVENT", "START-OF-SELECTION", false) + dynp
	} else {
		entries = entry(1, f.line, "EVENT", "START-OF-SELECTION", true) + dynp
	}
	return `<?xml version="1.0" encoding="utf-8"?><dbg:stack isRfc="false" debugCursorStackIndex="0" isSameSystem="true" xmlns:dbg="http://www.sap.com/adt/debugger">` + entries + `</dbg:stack>`
}

func abapXML(data string) string {
	return `<?xml version="1.0" encoding="utf-8"?><asx:abap version="1.0" xmlns:asx="http://www.sap.com/abapxml"><asx:values>` + data + `</asx:values></asx:abap>`
}

func variable(id, name, typ, meta, value string, lines int) string {
	return fmt.Sprintf(`<STPDA_ADT_VARIABLE><ID>%s</ID><NAME>%s</NAME><DECLARED_TYPE_NAME>%s</DECLARED_TYPE_NAME><META_TYPE>%s</META_TYPE><VALUE>%s</VALUE><TABLE_LINES>%d</TABLE_LINES></STPDA_ADT_VARIABLE>`,
		id, name, typ, meta, value, lines)
}

func hierarchy(parent, child, name string) string {
	return fmt.Sprintf(`<STPDA_ADT_VARIABLE_HIERARCHY><PARENT_ID>%s</PARENT_ID><CHILD_ID>%s</CHILD_ID><CHILD_NAME>%s</CHILD_NAME></STPDA_ADT_VARIABLE_HIERARCHY>`, parent, child, name)
}

// children answers getChildVariables the way A4H does: @ROOT names the
// synthetic roots, a root names its variables, a structure its components,
// and a table nothing at all — its rows are asked for by subscript.
func (f *fakeADT) children(parents [][]string) string {
	var hier, vars []string
	for _, m := range parents {
		switch p := m[1]; {
		case p == "@ROOT":
			hier = append(hier, hierarchy("@ROOT", "@GLOBALS", "Globals"))
			if f.inForm {
				hier = append(hier, hierarchy("@ROOT", "@PARAMETERS", "Parameters"))
			}
		case p == "@GLOBALS":
			vars = append(vars,
				variable("LV_COUNTER", "LV_COUNTER", "I", "simple", f.counter+" ", 0),
				variable("LS_ROW", "LS_ROW", "TY_ROW", "structure", "", 0),
				variable("LT_ROWS", "LT_ROWS", "TY_ROWS", "table", "", 2))
		case p == "@PARAMETERS":
			vars = append(vars, variable("IV_IN", "IV_IN", "I", "simple", f.counter+" ", 0))
		case p == "LS_ROW":
			vars = append(vars,
				variable("LS_ROW-ID", "ID", "I", "simple", "2 ", 0),
				variable("LS_ROW-NAME", "NAME", "STRING", "string", "second", 0))
		case strings.HasPrefix(p, "LT_ROWS["):
			vars = append(vars, variable(p, p, "TY_ROW", "structure", "", 0))
		case p == "LT_ROWS":
			return "" // SAP's answer for a table: an empty body
		}
	}
	return abapXML(`<DATA><VARIABLES>` + strings.Join(vars, "") + `</VARIABLES><HIERARCHIES>` + strings.Join(hier, "") + `</HIERARCHIES></DATA>`)
}
