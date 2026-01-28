// ABOUTME: Lua function registration for ADT bindings.
// ABOUTME: Registers all Lua global functions that expose ADT client functionality.

package scripting

// registerADTBindings registers all ADT-related Lua functions.
func (e *LuaEngine) registerADTBindings() {
	// Search & Source
	e.L.SetGlobal("searchObject", e.L.NewFunction(e.luaSearchObject))
	e.L.SetGlobal("grepObjects", e.L.NewFunction(e.luaGrepObjects))
	e.L.SetGlobal("getSource", e.L.NewFunction(e.luaGetSource))
	e.L.SetGlobal("writeSource", e.L.NewFunction(e.luaWriteSource))
	e.L.SetGlobal("editSource", e.L.NewFunction(e.luaEditSource))

	// Debugging - Breakpoints
	e.L.SetGlobal("setBreakpoint", e.L.NewFunction(e.luaSetBreakpoint))
	e.L.SetGlobal("setStatementBP", e.L.NewFunction(e.luaSetStatementBreakpoint))
	e.L.SetGlobal("setExceptionBP", e.L.NewFunction(e.luaSetExceptionBreakpoint))
	e.L.SetGlobal("setMessageBP", e.L.NewFunction(e.luaSetMessageBreakpoint))
	e.L.SetGlobal("setBadiBP", e.L.NewFunction(e.luaSetBadiBreakpoint))
	e.L.SetGlobal("setEnhancementBP", e.L.NewFunction(e.luaSetEnhancementBreakpoint))
	e.L.SetGlobal("setWatchpoint", e.L.NewFunction(e.luaSetWatchpoint))
	e.L.SetGlobal("setMethodBP", e.L.NewFunction(e.luaSetMethodBreakpoint))
	e.L.SetGlobal("getBreakpoints", e.L.NewFunction(e.luaGetBreakpoints))
	e.L.SetGlobal("deleteBreakpoint", e.L.NewFunction(e.luaDeleteBreakpoint))

	// Debugging - Session
	e.L.SetGlobal("listen", e.L.NewFunction(e.luaListen))
	e.L.SetGlobal("attach", e.L.NewFunction(e.luaAttach))
	e.L.SetGlobal("detach", e.L.NewFunction(e.luaDetach))

	// Debugging - Execution
	e.L.SetGlobal("stepOver", e.L.NewFunction(e.luaStepOver))
	e.L.SetGlobal("stepInto", e.L.NewFunction(e.luaStepInto))
	e.L.SetGlobal("stepReturn", e.L.NewFunction(e.luaStepReturn))
	e.L.SetGlobal("continue_", e.L.NewFunction(e.luaContinue))

	// Debugging - Inspection
	e.L.SetGlobal("getStack", e.L.NewFunction(e.luaGetStack))
	e.L.SetGlobal("getVariables", e.L.NewFunction(e.luaGetVariables))
	e.L.SetGlobal("setVariable", e.L.NewFunction(e.luaSetVariable))

	// Call Graph
	e.L.SetGlobal("getCallGraph", e.L.NewFunction(e.luaGetCallGraph))
	e.L.SetGlobal("getCallersOf", e.L.NewFunction(e.luaGetCallersOf))
	e.L.SetGlobal("getCalleesOf", e.L.NewFunction(e.luaGetCalleesOf))

	// Checkpoints
	e.L.SetGlobal("saveCheckpoint", e.L.NewFunction(e.luaSaveCheckpoint))
	e.L.SetGlobal("getCheckpoint", e.L.NewFunction(e.luaGetCheckpoint))
	e.L.SetGlobal("listCheckpoints", e.L.NewFunction(e.luaListCheckpoints))
	e.L.SetGlobal("injectCheckpoint", e.L.NewFunction(e.luaInjectCheckpoint))

	// Execution Recording
	e.L.SetGlobal("startRecording", e.L.NewFunction(e.luaStartRecording))
	e.L.SetGlobal("stopRecording", e.L.NewFunction(e.luaStopRecording))
	e.L.SetGlobal("getRecording", e.L.NewFunction(e.luaGetRecording))
	e.L.SetGlobal("saveRecording", e.L.NewFunction(e.luaSaveRecording))

	// History Navigation
	e.L.SetGlobal("getStateAtStep", e.L.NewFunction(e.luaGetStateAtStep))
	e.L.SetGlobal("findWhenChanged", e.L.NewFunction(e.luaFindWhenChanged))
	e.L.SetGlobal("findChanges", e.L.NewFunction(e.luaFindChanges))
	e.L.SetGlobal("listRecordings", e.L.NewFunction(e.luaListRecordings))
	e.L.SetGlobal("loadRecording", e.L.NewFunction(e.luaLoadRecording))
	e.L.SetGlobal("compareRecordings", e.L.NewFunction(e.luaCompareRecordings))

	// Force Replay
	e.L.SetGlobal("forceReplay", e.L.NewFunction(e.luaForceReplay))
	e.L.SetGlobal("replayFromStep", e.L.NewFunction(e.luaReplayFromStep))

	// Diagnostics
	e.L.SetGlobal("listDumps", e.L.NewFunction(e.luaGetDumps))
	e.L.SetGlobal("getDump", e.L.NewFunction(e.luaGetDump))
	e.L.SetGlobal("getMessages", e.L.NewFunction(e.luaGetMessages))
	e.L.SetGlobal("runUnitTests", e.L.NewFunction(e.luaRunUnitTests))
	e.L.SetGlobal("syntaxCheck", e.L.NewFunction(e.luaSyntaxCheck))
}
