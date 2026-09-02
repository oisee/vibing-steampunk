package scripting

import (
	"fmt"
	"strings"
	"time"

	"context"

	"github.com/oisee/vibing-steampunk/pkg/abaplint"
	"github.com/oisee/vibing-steampunk/pkg/adt"
	"github.com/oisee/vibing-steampunk/pkg/ctxcomp"
	lua "github.com/yuin/gopher-lua"
)

// registerADTBindings registers all ADT-related Lua functions.
func (e *LuaEngine) registerADTBindings() {
	// Search & Source
	e.L.SetGlobal("searchObject", e.L.NewFunction(e.luaSearchObject))
	e.L.SetGlobal("grepObjects", e.L.NewFunction(e.luaGrepObjects))
	e.L.SetGlobal("getSource", e.L.NewFunction(e.luaGetSource))
	e.L.SetGlobal("writeSource", e.L.NewFunction(e.luaWriteSource))
	e.L.SetGlobal("editSource", e.L.NewFunction(e.luaEditSource))

	// Debugging — every binding lives in debug_session.go, on the one session the
	// engine holds. registerSessionDebugBindings() puts them in place.
	e.registerSessionDebugBindings()

	// Call Graph
	e.L.SetGlobal("getCallGraph", e.L.NewFunction(e.luaGetCallGraph))
	e.L.SetGlobal("getCallersOf", e.L.NewFunction(e.luaGetCallersOf))
	e.L.SetGlobal("getCalleesOf", e.L.NewFunction(e.luaGetCalleesOf))

	// Checkpoints (for Force Replay)
	e.L.SetGlobal("saveCheckpoint", e.L.NewFunction(e.luaSaveCheckpoint))
	e.L.SetGlobal("getCheckpoint", e.L.NewFunction(e.luaGetCheckpoint))
	e.L.SetGlobal("listCheckpoints", e.L.NewFunction(e.luaListCheckpoints))
	e.L.SetGlobal("injectCheckpoint", e.L.NewFunction(e.luaInjectCheckpoint))

	// Execution Recording (Phase 5.2)
	e.L.SetGlobal("startRecording", e.L.NewFunction(e.luaStartRecording))
	e.L.SetGlobal("stopRecording", e.L.NewFunction(e.luaStopRecording))
	e.L.SetGlobal("getRecording", e.L.NewFunction(e.luaGetRecording))
	e.L.SetGlobal("saveRecording", e.L.NewFunction(e.luaSaveRecording))

	// History Navigation (Phase 5.2)
	e.L.SetGlobal("getStateAtStep", e.L.NewFunction(e.luaGetStateAtStep))
	e.L.SetGlobal("findWhenChanged", e.L.NewFunction(e.luaFindWhenChanged))
	e.L.SetGlobal("findChanges", e.L.NewFunction(e.luaFindChanges))
	e.L.SetGlobal("listRecordings", e.L.NewFunction(e.luaListRecordings))
	e.L.SetGlobal("loadRecording", e.L.NewFunction(e.luaLoadRecording))
	e.L.SetGlobal("compareRecordings", e.L.NewFunction(e.luaCompareRecordings))

	// Force Replay (Phase 5.5)
	e.L.SetGlobal("forceReplay", e.L.NewFunction(e.luaForceReplay))
	e.L.SetGlobal("replayFromStep", e.L.NewFunction(e.luaReplayFromStep))

	// Query & Analysis (new in v2.32)
	e.L.SetGlobal("query", e.L.NewFunction(e.luaQuery))
	e.L.SetGlobal("lint", e.L.NewFunction(e.luaLint))
	e.L.SetGlobal("parse", e.L.NewFunction(e.luaParse))
	e.L.SetGlobal("context", e.L.NewFunction(e.luaContext))
	e.L.SetGlobal("systemInfo", e.L.NewFunction(e.luaSystemInfo))

	// Diagnostics
	e.L.SetGlobal("listDumps", e.L.NewFunction(e.luaGetDumps)) // New canonical name
	e.L.SetGlobal("getDumps", e.L.NewFunction(e.luaGetDumps))  // Backwards compatibility
	e.L.SetGlobal("getDump", e.L.NewFunction(e.luaGetDump))
	e.L.SetGlobal("getMessages", e.L.NewFunction(e.luaGetMessages))
	e.L.SetGlobal("runUnitTests", e.L.NewFunction(e.luaRunUnitTests))
	e.L.SetGlobal("syntaxCheck", e.L.NewFunction(e.luaSyntaxCheck))
}

// --- Search & Source ---

func (e *LuaEngine) luaSearchObject(L *lua.LState) int {
	query := getString(L, 1)
	maxResults := getOptInt(L, 2, 100)

	results, err := e.client.SearchObject(e.ctx, query, maxResults)
	if err != nil {
		L.Push(lua.LNil)
		L.Push(lua.LString(err.Error()))
		return 2
	}

	tbl := L.NewTable()
	for i, obj := range results {
		row := L.NewTable()
		L.SetField(row, "name", lua.LString(obj.Name))
		L.SetField(row, "type", lua.LString(obj.Type))
		L.SetField(row, "uri", lua.LString(obj.URI))
		L.SetField(row, "package", lua.LString(obj.PackageName))
		tbl.RawSetInt(i+1, row)
	}

	L.Push(tbl)
	return 1
}

func (e *LuaEngine) luaGrepObjects(L *lua.LState) int {
	// grepObjects(pattern, objectQuery, [contextLines])
	// First searches for objects matching objectQuery, then greps for pattern
	pattern := getString(L, 1)
	objectQuery := getOptString(L, 2, "")
	contextLines := getOptInt(L, 3, 0)

	// If objectQuery is empty, use pattern as both search and grep pattern
	if objectQuery == "" {
		objectQuery = "*"
	}

	// Search for objects first
	objects, err := e.client.SearchObject(e.ctx, objectQuery, 50)
	if err != nil {
		L.Push(lua.LNil)
		L.Push(lua.LString(err.Error()))
		return 2
	}

	// Extract URIs
	objectURLs := make([]string, len(objects))
	for i, obj := range objects {
		objectURLs[i] = obj.URI
	}

	if len(objectURLs) == 0 {
		L.Push(L.NewTable()) // Return empty table
		return 1
	}

	// Grep in objects
	result, err := e.client.GrepObjects(e.ctx, objectURLs, pattern, false, contextLines)
	if err != nil {
		L.Push(lua.LNil)
		L.Push(lua.LString(err.Error()))
		return 2
	}

	tbl := L.NewTable()
	idx := 1
	for _, obj := range result.Objects {
		for _, match := range obj.Matches {
			row := L.NewTable()
			L.SetField(row, "uri", lua.LString(obj.ObjectURL))
			L.SetField(row, "name", lua.LString(obj.ObjectName))
			L.SetField(row, "line", lua.LNumber(match.LineNumber))
			L.SetField(row, "content", lua.LString(match.MatchedLine))
			tbl.RawSetInt(idx, row)
			idx++
		}
	}

	L.Push(tbl)
	return 1
}

func (e *LuaEngine) luaGetSource(L *lua.LState) int {
	objType := getString(L, 1)
	name := getString(L, 2)
	include := getOptString(L, 3, "")

	opts := &adt.GetSourceOptions{
		Include: include,
	}

	source, err := e.client.GetSource(e.ctx, objType, name, opts)
	if err != nil {
		L.Push(lua.LNil)
		L.Push(lua.LString(err.Error()))
		return 2
	}

	L.Push(lua.LString(source))
	return 1
}

func (e *LuaEngine) luaWriteSource(L *lua.LState) int {
	objType := getString(L, 1)
	name := getString(L, 2)
	source := getString(L, 3)
	var opts *adt.WriteSourceOptions
	if L.GetTop() >= 4 && L.Get(4) != lua.LNil {
		table, ok := L.Get(4).(*lua.LTable)
		if !ok {
			L.Push(lua.LBool(false))
			L.Push(lua.LString("writeSource options must be a table"))
			return 2
		}
		opts = &adt.WriteSourceOptions{
			Mode:        adt.WriteSourceMode(strings.ToLower(luaTableString(table, "mode"))),
			Description: luaTableString(table, "description"),
			Package:     luaTableString(table, "package"),
			TestSource:  luaTableString(table, "test_source"),
			Transport:   luaTableString(table, "transport"),
			Method:      luaTableString(table, "method"),
		}
	}

	if e.writeSource == nil {
		L.Push(lua.LBool(false))
		L.Push(lua.LString("writeSource is unavailable: ADT client is not configured"))
		return 2
	}

	result, err := e.writeSource(e.ctx, objType, name, source, opts)
	if err != nil {
		L.Push(lua.LBool(false))
		L.Push(lua.LString(err.Error()))
		return 2
	}
	if err := adt.WriteSourceResultError(result); err != nil {
		L.Push(lua.LBool(false))
		L.Push(lua.LString(err.Error()))
		return 2
	}

	L.Push(lua.LBool(true))
	return 1
}

func luaTableString(table *lua.LTable, key string) string {
	value := table.RawGetString(key)
	if value == lua.LNil {
		return ""
	}
	return lua.LVAsString(value)
}

func (e *LuaEngine) luaEditSource(L *lua.LState) int {
	// editSource(objectURI, oldText, newText, [replaceAll])
	objectURI := getString(L, 1)
	oldText := getString(L, 2)
	newText := getString(L, 3)
	replaceAll := getBool(L, 4)

	result, err := e.client.EditSource(e.ctx, objectURI, oldText, newText, replaceAll, true, false)
	if err != nil {
		L.Push(lua.LBool(false))
		L.Push(lua.LString(err.Error()))
		return 2
	}

	L.Push(lua.LBool(result.Success))
	return 1
}

// --- Debugging: Breakpoints ---
// Note: These are stubs. Full debugger integration requires more work.

// The debugger bindings used to live here, on the stateless ADT client. They
// could not work: a debug session lives in an ABAP roll area that a stateless
// client cannot get back to, so attach returned happily and every later call
// referred to nothing. They are in debug_session.go now, on a session the
// engine holds, and they are registered from there.

// --- Call Graph ---

func (e *LuaEngine) luaGetCallGraph(L *lua.LState) int {
	objectURI := getString(L, 1)
	direction := getOptString(L, 2, "callees")
	maxDepth := getOptInt(L, 3, 5)

	// maxDepth is still accepted so scripts keep parsing, and it is still
	// ignored: both sources behind CallGraph are one hop. See callees.go.
	_ = maxDepth
	graph, err := e.client.CallGraph(e.ctx, objectURI, &adt.CallGraphOptions{
		Direction:  direction,
		MaxResults: 500,
	})
	if err != nil {
		L.Push(lua.LNil)
		L.Push(lua.LString(err.Error()))
		return 2
	}

	L.Push(callGraphToLua(L, graph))
	return 1
}

func (e *LuaEngine) luaGetCallersOf(L *lua.LState) int {
	objectURI := getString(L, 1)
	maxDepth := getOptInt(L, 2, 5)

	_ = maxDepth
	graph, err := e.client.CallGraph(e.ctx, objectURI, &adt.CallGraphOptions{
		Direction:  "callers",
		MaxResults: 500,
	})
	if err != nil {
		L.Push(lua.LNil)
		L.Push(lua.LString(err.Error()))
		return 2
	}

	L.Push(callGraphToLua(L, graph))
	return 1
}

func (e *LuaEngine) luaGetCalleesOf(L *lua.LState) int {
	objectURI := getString(L, 1)
	maxDepth := getOptInt(L, 2, 5)

	_ = maxDepth
	graph, err := e.client.CallGraph(e.ctx, objectURI, &adt.CallGraphOptions{
		Direction:  "callees",
		MaxResults: 500,
	})
	if err != nil {
		L.Push(lua.LNil)
		L.Push(lua.LString(err.Error()))
		return 2
	}

	L.Push(callGraphToLua(L, graph))
	return 1
}

func callGraphToLua(L *lua.LState, node *adt.CallGraphNode) *lua.LTable {
	if node == nil {
		return L.NewTable()
	}

	tbl := L.NewTable()
	L.SetField(tbl, "uri", lua.LString(node.URI))
	L.SetField(tbl, "name", lua.LString(node.Name))
	L.SetField(tbl, "type", lua.LString(node.Type))

	if len(node.Children) > 0 {
		children := L.NewTable()
		for i, child := range node.Children {
			children.RawSetInt(i+1, callGraphToLua(L, &child))
		}
		L.SetField(tbl, "children", children)
	}

	return tbl
}

// --- Checkpoints (Force Replay) ---

func (e *LuaEngine) luaSaveCheckpoint(L *lua.LState) int {
	name := getString(L, 1)

	// Save timestamp as checkpoint placeholder
	// Full variable capture requires active debug session with variable IDs
	checkpoint := make(map[string]interface{})
	checkpoint["_timestamp"] = time.Now().Format(time.RFC3339)
	checkpoint["_note"] = "Checkpoint saved - variable capture requires active debug session"

	e.checkpoints[name] = checkpoint

	L.Push(lua.LBool(true))
	return 1
}

func (e *LuaEngine) luaGetCheckpoint(L *lua.LState) int {
	name := getString(L, 1)

	checkpoint, ok := e.checkpoints[name]
	if !ok {
		L.Push(lua.LNil)
		L.Push(lua.LString("checkpoint not found: " + name))
		return 2
	}

	L.Push(goToLua(L, checkpoint))
	return 1
}

func (e *LuaEngine) luaListCheckpoints(L *lua.LState) int {
	tbl := L.NewTable()
	i := 1
	for name, cp := range e.checkpoints {
		row := L.NewTable()
		L.SetField(row, "name", lua.LString(name))
		if ts, ok := cp["_timestamp"].(string); ok {
			L.SetField(row, "timestamp", lua.LString(ts))
		}
		tbl.RawSetInt(i, row)
		i++
	}

	L.Push(tbl)
	return 1
}

// injectCheckpoint(name) - Inject all variables from checkpoint into live debug session (FORCE REPLAY!)
func (e *LuaEngine) luaInjectCheckpoint(L *lua.LState) int {
	name := getString(L, 1)

	checkpoint, ok := e.checkpoints[name]
	if !ok {
		L.Push(lua.LBool(false))
		L.Push(lua.LString("checkpoint not found: " + name))
		return 2
	}

	// Inject each variable from checkpoint
	injected := 0
	failed := 0
	var lastError string

	for varName, value := range checkpoint {
		// Skip metadata fields
		if strings.HasPrefix(varName, "_") {
			continue
		}

		// Convert value to string
		var valueStr string
		switch v := value.(type) {
		case string:
			valueStr = v
		case float64:
			valueStr = fmt.Sprintf("%v", v)
		case bool:
			if v {
				valueStr = "X"
			} else {
				valueStr = " "
			}
		default:
			jsonBytes, _ := jsonMarshal(v)
			valueStr = string(jsonBytes)
		}

		_, err := e.client.DebuggerSetVariableValue(e.ctx, varName, valueStr)
		if err != nil {
			failed++
			lastError = fmt.Sprintf("%s: %v", varName, err)
			fmt.Fprintf(e.output, "Failed to inject %s: %v\n", varName, err)
		} else {
			injected++
			fmt.Fprintf(e.output, "Injected: %s = %s\n", varName, valueStr)
		}
	}

	if failed > 0 {
		L.Push(lua.LBool(false))
		L.Push(lua.LString(fmt.Sprintf("Injected %d, failed %d. Last error: %s", injected, failed, lastError)))
	} else {
		L.Push(lua.LBool(true))
		L.Push(lua.LNumber(injected))
	}
	return 2
}

// --- Diagnostics ---

// luaGetDumps lists runtime errors. The table keys are unchanged from the
// version that read the old feed parser, so existing scripts keep working —
// but program, exception, user and time were empty in every row that parser
// produced, and are now filled, because the categories the feed labels are read
// by label instead of by position.
func (e *LuaEngine) luaGetDumps(L *lua.LState) int {
	maxResults := getOptInt(L, 1, 20)

	dumps, err := e.client.Dumps(e.ctx, adt.DumpFilter{Limit: maxResults})
	if err != nil {
		L.Push(lua.LNil)
		L.Push(lua.LString(err.Error()))
		return 2
	}

	tbl := L.NewTable()
	for i, dump := range dumps {
		row := L.NewTable()
		L.SetField(row, "id", lua.LString(dump.ID))
		L.SetField(row, "program", lua.LString(dump.Program))
		L.SetField(row, "exception", lua.LString(dump.ErrorType))
		L.SetField(row, "user", lua.LString(dump.User))
		L.SetField(row, "time", lua.LString(dumpStamp(dump.At)))
		L.SetField(row, "title", lua.LString(dump.Message))
		tbl.RawSetInt(i+1, row)
	}

	L.Push(tbl)
	return 1
}

// dumpStamp renders a dump timestamp for Lua, which has no time type. A zero
// time prints as empty rather than as year one, because a script concatenating
// it should show a gap, not a date nobody wrote.
func dumpStamp(at time.Time) string {
	if at.IsZero() {
		return ""
	}
	return at.Format(time.RFC3339)
}

// luaGetDump reads one dump in detail. The old version returned the dump's
// HTML page with only its <title> extracted and an always-empty stack; this
// parses the formatted rendering, so the stack a script asks for is really
// there. The keys are the same ones, with "type" added to each frame — the
// frame kind (METHOD, FUNCTION, FORM) that "event" was always empty for.
func (e *LuaEngine) luaGetDump(L *lua.LState) int {
	dumpID := getString(L, 1)

	dump, err := e.client.DumpDetail(e.ctx, dumpID)
	if err != nil {
		L.Push(lua.LNil)
		L.Push(lua.LString(err.Error()))
		return 2
	}

	tbl := L.NewTable()
	L.SetField(tbl, "id", lua.LString(dump.ID))
	L.SetField(tbl, "program", lua.LString(dump.Program))
	L.SetField(tbl, "exception", lua.LString(dump.Exception))
	L.SetField(tbl, "title", lua.LString(dump.ErrorType))
	L.SetField(tbl, "include", lua.LString(dump.Include))
	L.SetField(tbl, "procedure", lua.LString(dump.Procedure))
	L.SetField(tbl, "component", lua.LString(dump.Component))
	L.SetField(tbl, "line", lua.LNumber(dump.Line))
	// The detail resource names no user and no timestamp — the listing does.
	// Both keys were always empty here anyway, and they stay present rather
	// than disappearing, because a script concatenating a nil field crashes
	// where one concatenating an empty string simply shows a gap.
	L.SetField(tbl, "user", lua.LString(""))
	L.SetField(tbl, "time", lua.LString(""))

	if len(dump.Stack) > 0 {
		stack := L.NewTable()
		for i, frame := range dump.Stack {
			row := L.NewTable()
			L.SetField(row, "program", lua.LString(frame.Program))
			L.SetField(row, "include", lua.LString(frame.Include))
			L.SetField(row, "line", lua.LNumber(frame.Line))
			L.SetField(row, "type", lua.LString(frame.Type))
			L.SetField(row, "event", lua.LString(frame.Type))
			L.SetField(row, "name", lua.LString(frame.Name))
			stack.RawSetInt(i+1, row)
		}
		L.SetField(tbl, "stack", stack)
	}

	L.Push(tbl)
	return 1
}

func (e *LuaEngine) luaGetMessages(L *lua.LState) int {
	msgClass := getString(L, 1)

	mc, err := e.client.GetMessageClass(e.ctx, msgClass)
	if err != nil {
		L.Push(lua.LNil)
		L.Push(lua.LString(err.Error()))
		return 2
	}

	tbl := L.NewTable()
	L.SetField(tbl, "name", lua.LString(mc.Name))
	L.SetField(tbl, "description", lua.LString(mc.Description))

	msgs := L.NewTable()
	for i, msg := range mc.Messages {
		row := L.NewTable()
		L.SetField(row, "number", lua.LString(msg.Number))
		L.SetField(row, "text", lua.LString(msg.Text))
		msgs.RawSetInt(i+1, row)
	}
	L.SetField(tbl, "messages", msgs)

	L.Push(tbl)
	return 1
}

func (e *LuaEngine) luaRunUnitTests(L *lua.LState) int {
	objectURI := getString(L, 1)

	result, err := e.client.RunUnitTests(e.ctx, objectURI, nil)
	if err != nil {
		L.Push(lua.LNil)
		L.Push(lua.LString(err.Error()))
		return 2
	}

	tbl := L.NewTable()

	classes := L.NewTable()
	for i, class := range result.Classes {
		row := L.NewTable()
		L.SetField(row, "name", lua.LString(class.Name))
		L.SetField(row, "uri", lua.LString(class.URI))

		methods := L.NewTable()
		for j, method := range class.TestMethods {
			m := L.NewTable()
			L.SetField(m, "name", lua.LString(method.Name))
			L.SetField(m, "type", lua.LString(method.Type))
			// Derive status from alerts
			status := "OK"
			if len(method.Alerts) > 0 {
				status = method.Alerts[0].Kind
			}
			L.SetField(m, "status", lua.LString(status))
			methods.RawSetInt(j+1, m)
		}
		L.SetField(row, "methods", methods)
		classes.RawSetInt(i+1, row)
	}
	L.SetField(tbl, "classes", classes)

	L.Push(tbl)
	return 1
}

func (e *LuaEngine) luaSyntaxCheck(L *lua.LState) int {
	objectType := getString(L, 1)
	name := getString(L, 2)

	errors, err := e.client.SyntaxCheck(e.ctx, objectType, name)
	if err != nil {
		L.Push(lua.LNil)
		L.Push(lua.LString(err.Error()))
		return 2
	}

	tbl := L.NewTable()
	for i, e := range errors {
		row := L.NewTable()
		L.SetField(row, "line", lua.LNumber(e.Line))
		L.SetField(row, "offset", lua.LNumber(e.Offset))
		L.SetField(row, "message", lua.LString(e.Text))
		L.SetField(row, "severity", lua.LString(e.Severity))
		tbl.RawSetInt(i+1, row)
	}

	L.Push(tbl)
	return 1
}

// --- Execution Recording (Phase 5.2) ---

func (e *LuaEngine) luaStartRecording(L *lua.LState) int {
	sessionID := getOptString(L, 1, "default")
	program := getOptString(L, 2, "")

	e.recorder = adt.NewExecutionRecorder(sessionID, program)
	e.isRecording = true

	L.Push(lua.LString(e.recorder.GetRecording().ID))
	return 1
}

func (e *LuaEngine) luaStopRecording(L *lua.LState) int {
	if e.recorder == nil {
		L.Push(lua.LNil)
		L.Push(lua.LString("no active recording"))
		return 2
	}

	e.recorder.Complete()
	e.isRecording = false

	stats := e.recorder.Stats()
	L.Push(goToLua(L, stats))
	return 1
}

func (e *LuaEngine) luaGetRecording(L *lua.LState) int {
	if e.recorder == nil {
		L.Push(lua.LNil)
		L.Push(lua.LString("no active recording"))
		return 2
	}

	recording := e.recorder.GetRecording()
	tbl := L.NewTable()
	L.SetField(tbl, "id", lua.LString(recording.ID))
	L.SetField(tbl, "session_id", lua.LString(recording.SessionID))
	L.SetField(tbl, "program", lua.LString(recording.Program))
	L.SetField(tbl, "total_steps", lua.LNumber(recording.TotalSteps))
	L.SetField(tbl, "current_step", lua.LNumber(recording.CurrentStep))
	L.SetField(tbl, "is_complete", lua.LBool(recording.IsComplete))

	// Checkpoints
	checkpoints := L.NewTable()
	for name, step := range recording.Checkpoints {
		L.SetField(checkpoints, name, lua.LNumber(step))
	}
	L.SetField(tbl, "checkpoints", checkpoints)

	L.Push(tbl)
	return 1
}

func (e *LuaEngine) luaSaveRecording(L *lua.LState) int {
	if e.recorder == nil {
		L.Push(lua.LBool(false))
		L.Push(lua.LString("no active recording"))
		return 2
	}

	storePath := getOptString(L, 1, ".vsp-recordings")

	// Initialize history manager if needed
	if e.historyManager == nil {
		var err error
		e.historyManager, err = adt.NewHistoryManager(storePath)
		if err != nil {
			L.Push(lua.LBool(false))
			L.Push(lua.LString(err.Error()))
			return 2
		}
	}

	if err := e.historyManager.SaveRecording(e.recorder); err != nil {
		L.Push(lua.LBool(false))
		L.Push(lua.LString(err.Error()))
		return 2
	}

	L.Push(lua.LBool(true))
	L.Push(lua.LString(e.recorder.GetRecording().ID))
	return 2
}

// --- History Navigation (Phase 5.2) ---

func (e *LuaEngine) luaGetStateAtStep(L *lua.LState) int {
	stepNumber := int(L.ToNumber(1))

	if e.recorder == nil {
		L.Push(lua.LNil)
		L.Push(lua.LString("no active recording"))
		return 2
	}

	vars := e.recorder.GetVariablesAtStep(stepNumber)
	if vars == nil {
		L.Push(lua.LNil)
		L.Push(lua.LString(fmt.Sprintf("step %d not found", stepNumber)))
		return 2
	}

	tbl := L.NewTable()
	for name, v := range vars {
		row := L.NewTable()
		L.SetField(row, "name", lua.LString(v.Name))
		L.SetField(row, "type", lua.LString(v.Type))
		L.SetField(row, "value", goToLua(L, v.Value))
		L.SetField(row, "is_changed", lua.LBool(v.IsChanged))
		tbl.RawSetString(name, row)
	}

	L.Push(tbl)
	return 1
}

func (e *LuaEngine) luaFindWhenChanged(L *lua.LState) int {
	variableName := getString(L, 1)
	targetValue := luaToGo(L.Get(2))

	if e.recorder == nil {
		L.Push(lua.LNumber(-1))
		L.Push(lua.LString("no active recording"))
		return 2
	}

	step := e.recorder.FindWhenChanged(variableName, targetValue)
	L.Push(lua.LNumber(step))
	return 1
}

func (e *LuaEngine) luaFindChanges(L *lua.LState) int {
	variableName := getString(L, 1)

	if e.recorder == nil {
		L.Push(lua.LNil)
		L.Push(lua.LString("no active recording"))
		return 2
	}

	changes := e.recorder.FindChanges(variableName)

	tbl := L.NewTable()
	for i, step := range changes {
		tbl.RawSetInt(i+1, lua.LNumber(step))
	}

	L.Push(tbl)
	return 1
}

func (e *LuaEngine) luaListRecordings(L *lua.LState) int {
	storePath := getOptString(L, 1, ".vsp-recordings")

	// Initialize history manager if needed
	if e.historyManager == nil {
		var err error
		e.historyManager, err = adt.NewHistoryManager(storePath)
		if err != nil {
			L.Push(lua.LNil)
			L.Push(lua.LString(err.Error()))
			return 2
		}
	}

	recordings := e.historyManager.ListRecordings(adt.RecordingFilter{
		Limit: getOptInt(L, 2, 20),
	})

	tbl := L.NewTable()
	for i, rec := range recordings {
		row := L.NewTable()
		L.SetField(row, "id", lua.LString(rec.ID))
		L.SetField(row, "session_id", lua.LString(rec.SessionID))
		L.SetField(row, "program", lua.LString(rec.Program))
		L.SetField(row, "total_steps", lua.LNumber(rec.TotalSteps))
		L.SetField(row, "is_complete", lua.LBool(rec.IsComplete))
		L.SetField(row, "start_time", lua.LString(rec.StartTime.Format(time.RFC3339)))
		tbl.RawSetInt(i+1, row)
	}

	L.Push(tbl)
	return 1
}

func (e *LuaEngine) luaLoadRecording(L *lua.LState) int {
	recordingID := getString(L, 1)
	storePath := getOptString(L, 2, ".vsp-recordings")

	// Initialize history manager if needed
	if e.historyManager == nil {
		var err error
		e.historyManager, err = adt.NewHistoryManager(storePath)
		if err != nil {
			L.Push(lua.LNil)
			L.Push(lua.LString(err.Error()))
			return 2
		}
	}

	recording, err := e.historyManager.LoadRecording(recordingID)
	if err != nil {
		L.Push(lua.LNil)
		L.Push(lua.LString(err.Error()))
		return 2
	}

	tbl := L.NewTable()
	L.SetField(tbl, "id", lua.LString(recording.ID))
	L.SetField(tbl, "session_id", lua.LString(recording.SessionID))
	L.SetField(tbl, "program", lua.LString(recording.Program))
	L.SetField(tbl, "total_steps", lua.LNumber(recording.TotalSteps))
	L.SetField(tbl, "is_complete", lua.LBool(recording.IsComplete))

	// Include frames summary
	frames := L.NewTable()
	for i, frame := range recording.Frames {
		row := L.NewTable()
		L.SetField(row, "step", lua.LNumber(frame.StepNumber))
		L.SetField(row, "program", lua.LString(frame.Location.Program))
		L.SetField(row, "line", lua.LNumber(frame.Location.Line))
		L.SetField(row, "type", lua.LString(frame.StepType))
		frames.RawSetInt(i+1, row)
	}
	L.SetField(tbl, "frames", frames)

	L.Push(tbl)
	return 1
}

func (e *LuaEngine) luaCompareRecordings(L *lua.LState) int {
	id1 := getString(L, 1)
	id2 := getString(L, 2)
	storePath := getOptString(L, 3, ".vsp-recordings")

	// Initialize history manager if needed
	if e.historyManager == nil {
		var err error
		e.historyManager, err = adt.NewHistoryManager(storePath)
		if err != nil {
			L.Push(lua.LNil)
			L.Push(lua.LString(err.Error()))
			return 2
		}
	}

	comparison, err := e.historyManager.CompareRecordings(id1, id2)
	if err != nil {
		L.Push(lua.LNil)
		L.Push(lua.LString(err.Error()))
		return 2
	}

	tbl := L.NewTable()
	L.SetField(tbl, "recording1_id", lua.LString(comparison.Recording1ID))
	L.SetField(tbl, "recording2_id", lua.LString(comparison.Recording2ID))
	L.SetField(tbl, "steps_compared", lua.LNumber(comparison.StepsCompared))
	L.SetField(tbl, "paths_match", lua.LBool(comparison.PathsMatch))

	diffs := L.NewTable()
	for i, diff := range comparison.Differences {
		row := L.NewTable()
		L.SetField(row, "type", lua.LString(diff.Type))
		L.SetField(row, "step", lua.LNumber(diff.StepNumber))
		L.SetField(row, "description", lua.LString(diff.Description))
		diffs.RawSetInt(i+1, row)
	}
	L.SetField(tbl, "differences", diffs)

	L.Push(tbl)
	return 1
}

// --- Force Replay (Phase 5.5) ---

// forceReplay(recordingId, [stepNumber]) - Inject state from recording into live debug session
// This is the killer feature: inject production state into dev session for debugging!
func (e *LuaEngine) luaForceReplay(L *lua.LState) int {
	recordingID := getString(L, 1)
	stepNumber := getOptInt(L, 2, -1) // -1 means last step
	storePath := getOptString(L, 3, ".vsp-recordings")

	// Initialize history manager if needed
	if e.historyManager == nil {
		var err error
		e.historyManager, err = adt.NewHistoryManager(storePath)
		if err != nil {
			L.Push(lua.LBool(false))
			L.Push(lua.LString(err.Error()))
			return 2
		}
	}

	// Load the recording
	recording, err := e.historyManager.LoadRecording(recordingID)
	if err != nil {
		L.Push(lua.LBool(false))
		L.Push(lua.LString("recording not found: " + err.Error()))
		return 2
	}

	// Default to last step
	if stepNumber == -1 {
		stepNumber = recording.TotalSteps
	}

	if stepNumber < 1 || stepNumber > recording.TotalSteps {
		L.Push(lua.LBool(false))
		L.Push(lua.LString(fmt.Sprintf("invalid step %d (recording has %d steps)", stepNumber, recording.TotalSteps)))
		return 2
	}

	// Get variables at the target step
	frame := recording.Frames[stepNumber-1]
	vars := frame.Variables
	if vars == nil {
		vars = frame.VariableDelta
	}

	// Inject each variable
	injected := 0
	failed := 0
	var lastError string

	fmt.Fprintf(e.output, "Force Replay: Injecting state from %s step %d\n", recordingID, stepNumber)
	fmt.Fprintf(e.output, "Location: %s:%d\n", frame.Location.Program, frame.Location.Line)

	for _, v := range vars {
		if v.Name == "" {
			continue
		}

		valueStr := fmt.Sprintf("%v", v.Value)
		_, err := e.client.DebuggerSetVariableValue(e.ctx, v.Name, valueStr)
		if err != nil {
			failed++
			lastError = fmt.Sprintf("%s: %v", v.Name, err)
			// Don't spam output for read-only vars
			if !strings.Contains(err.Error(), "read-only") {
				fmt.Fprintf(e.output, "  Failed: %s = %s (%v)\n", v.Name, valueStr, err)
			}
		} else {
			injected++
			fmt.Fprintf(e.output, "  Injected: %s = %s\n", v.Name, valueStr)
		}
	}

	fmt.Fprintf(e.output, "\nResult: %d injected, %d failed\n", injected, failed)

	if injected == 0 && failed > 0 {
		L.Push(lua.LBool(false))
		L.Push(lua.LString(lastError))
	} else {
		L.Push(lua.LBool(true))
		L.Push(lua.LNumber(injected))
	}
	return 2
}

// replayFromStep(stepNumber) - Inject state from current recording at specific step
func (e *LuaEngine) luaReplayFromStep(L *lua.LState) int {
	stepNumber := int(L.ToNumber(1))

	if e.recorder == nil {
		L.Push(lua.LBool(false))
		L.Push(lua.LString("no active recording"))
		return 2
	}

	// Get variables at the target step
	vars := e.recorder.GetVariablesAtStep(stepNumber)
	if vars == nil {
		L.Push(lua.LBool(false))
		L.Push(lua.LString(fmt.Sprintf("step %d not found in current recording", stepNumber)))
		return 2
	}

	// Inject each variable
	injected := 0
	failed := 0

	fmt.Fprintf(e.output, "Replay from step %d\n", stepNumber)

	for name, v := range vars {
		valueStr := fmt.Sprintf("%v", v.Value)
		_, err := e.client.DebuggerSetVariableValue(e.ctx, name, valueStr)
		if err != nil {
			failed++
		} else {
			injected++
			fmt.Fprintf(e.output, "  %s = %s\n", name, valueStr)
		}
	}

	fmt.Fprintf(e.output, "Result: %d injected, %d failed\n", injected, failed)

	L.Push(lua.LBool(true))
	L.Push(lua.LNumber(injected))
	return 2
}

// --- Query & Analysis (v2.32) ---

// query(sql, [maxRows]) → table of rows
func (e *LuaEngine) luaQuery(L *lua.LState) int {
	sql := L.ToString(1)
	maxRows := L.OptInt(2, 100)

	result, err := e.client.RunQuery(e.ctx, sql, maxRows)
	if err != nil {
		L.Push(lua.LNil)
		L.Push(lua.LString(err.Error()))
		return 2
	}

	tbl := L.NewTable()
	for i, row := range result.Rows {
		rowTbl := L.NewTable()
		for _, col := range result.Columns {
			val := fmt.Sprintf("%v", row[col.Name])
			L.SetField(rowTbl, col.Name, lua.LString(val))
		}
		L.RawSetInt(tbl, i+1, rowTbl)
	}

	L.Push(tbl)
	return 1
}

// lint(source) → table of issues
func (e *LuaEngine) luaLint(L *lua.LState) int {
	source := L.ToString(1)
	if source == "" {
		L.Push(lua.LNil)
		return 1
	}

	linter := abaplint.NewLinter()
	issues := linter.Run("lua", source)

	tbl := L.NewTable()
	for i, iss := range issues {
		row := L.NewTable()
		L.SetField(row, "key", lua.LString(iss.Key))
		L.SetField(row, "message", lua.LString(iss.Message))
		L.SetField(row, "row", lua.LNumber(iss.Row))
		L.SetField(row, "col", lua.LNumber(iss.Col))
		L.SetField(row, "severity", lua.LString(iss.Severity))
		L.RawSetInt(tbl, i+1, row)
	}

	L.Push(tbl)
	return 1
}

// parse(source) → table of statements
func (e *LuaEngine) luaParse(L *lua.LState) int {
	source := L.ToString(1)

	lex := &abaplint.Lexer{}
	tokens := lex.Run(source)
	parser := &abaplint.StatementParser{}
	stmts := parser.Parse(tokens)
	matcher := abaplint.NewStatementMatcher()
	matcher.ClassifyStatements(stmts)

	tbl := L.NewTable()
	for i, s := range stmts {
		row := L.NewTable()
		L.SetField(row, "type", lua.LString(s.Type))
		L.SetField(row, "text", lua.LString(s.ConcatTokens()))
		L.SetField(row, "tokens", lua.LNumber(len(s.Tokens)))
		L.RawSetInt(tbl, i+1, row)
	}

	L.Push(tbl)
	return 1
}

// context(objectType, name, [maxDeps]) → string (prologue + source)
func (e *LuaEngine) luaContext(L *lua.LState) int {
	objType := L.ToString(1)
	name := L.ToString(2)
	maxDeps := L.OptInt(3, 20)

	source, err := e.client.GetSource(e.ctx, objType, name, nil)
	if err != nil {
		L.Push(lua.LNil)
		L.Push(lua.LString(err.Error()))
		return 2
	}

	provider := ctxcomp.NewMultiSourceProvider("", &luaSourceAdapter{engine: e})
	compressor := ctxcomp.NewCompressor(provider, maxDeps)
	result, err := compressor.Compress(e.ctx, source, strings.ToUpper(name), strings.ToUpper(objType))
	if err != nil {
		L.Push(lua.LString(source))
		return 1
	}

	output := ""
	if result.Prologue != "" {
		output = result.Prologue + "\n"
	}
	output += source

	L.Push(lua.LString(output))
	return 1
}

// systemInfo() → table
func (e *LuaEngine) luaSystemInfo(L *lua.LState) int {
	info, err := e.client.GetSystemInfo(e.ctx)
	if err != nil {
		L.Push(lua.LNil)
		L.Push(lua.LString(err.Error()))
		return 2
	}

	tbl := L.NewTable()
	L.SetField(tbl, "systemId", lua.LString(info.SystemID))
	L.SetField(tbl, "client", lua.LString(info.Client))
	L.SetField(tbl, "sapRelease", lua.LString(info.SAPRelease))
	L.SetField(tbl, "kernelRelease", lua.LString(info.KernelRelease))
	L.SetField(tbl, "databaseSystem", lua.LString(info.DatabaseSystem))
	L.SetField(tbl, "hostName", lua.LString(info.HostName))

	L.Push(tbl)
	return 1
}

// luaSourceAdapter wraps the Lua engine's client for ctxcomp.ADTSourceFetcher.
type luaSourceAdapter struct {
	engine *LuaEngine
}

func (a *luaSourceAdapter) GetSource(ctx context.Context, objectType, name string, opts interface{}) (string, error) {
	var adtOpts *adt.GetSourceOptions
	if o, ok := opts.(*adt.GetSourceOptions); ok {
		adtOpts = o
	}
	return a.engine.client.GetSource(ctx, objectType, name, adtOpts)
}
