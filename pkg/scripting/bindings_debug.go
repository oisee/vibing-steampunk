// ABOUTME: Lua bindings for debugger operations.
// ABOUTME: Provides breakpoints, session control, execution stepping, and variable inspection.

package scripting

import (
	"fmt"

	"github.com/oisee/vibing-steampunk/pkg/adt"
	lua "github.com/yuin/gopher-lua"
)

// --- Breakpoint helper ---

// setBreakpoint creates a breakpoint and returns the ID or error.
func (e *LuaEngine) setBreakpoint(L *lua.LState, bp adt.Breakpoint) int {
	bp.Enabled = true
	req := &adt.BreakpointRequest{Breakpoints: []adt.Breakpoint{bp}}

	resp, err := e.client.SetExternalBreakpoint(e.ctx, req)
	if err != nil {
		return pushError(L, err)
	}

	if len(resp.Breakpoints) > 0 {
		L.Push(lua.LString(resp.Breakpoints[0].ID))
	} else {
		L.Push(lua.LString(""))
	}
	return 1
}

// --- Step helper ---

// doStep performs a debugger step and returns the result.
func (e *LuaEngine) doStep(L *lua.LState, stepType adt.DebugStepType) int {
	result, err := e.client.DebuggerStep(e.ctx, stepType, "")
	if err != nil {
		return pushError(L, err)
	}

	L.Push(goToLua(L, map[string]any{
		"session_id":  result.DebugSessionID,
		"stepping":    result.IsSteppingPossible,
		"termination": result.IsTerminationPossible,
	}))
	return 1
}

// --- Breakpoint functions ---

func (e *LuaEngine) luaSetBreakpoint(L *lua.LState) int {
	return e.setBreakpoint(L, adt.Breakpoint{
		Kind: adt.BreakpointKindLine,
		URI:  getString(L, 1),
		Line: getInt(L, 2),
	})
}

func (e *LuaEngine) luaSetStatementBreakpoint(L *lua.LState) int {
	return e.setBreakpoint(L, adt.Breakpoint{
		Kind:      adt.BreakpointKindStatement,
		Statement: getString(L, 1),
	})
}

func (e *LuaEngine) luaSetExceptionBreakpoint(L *lua.LState) int {
	return e.setBreakpoint(L, adt.Breakpoint{
		Kind:      adt.BreakpointKindException,
		Exception: getString(L, 1),
	})
}

func (e *LuaEngine) luaSetMessageBreakpoint(L *lua.LState) int {
	return e.setBreakpoint(L, adt.Breakpoint{
		Kind:        adt.BreakpointKindMessage,
		MessageArea: getString(L, 1),
		MessageID:   getString(L, 2),
		MessageType: getOptString(L, 3, ""),
	})
}

func (e *LuaEngine) luaSetBadiBreakpoint(L *lua.LState) int {
	return e.setBreakpoint(L, adt.Breakpoint{
		Kind:     adt.BreakpointKindBadi,
		BadiName: getString(L, 1),
	})
}

func (e *LuaEngine) luaSetEnhancementBreakpoint(L *lua.LState) int {
	return e.setBreakpoint(L, adt.Breakpoint{
		Kind:            adt.BreakpointKindEnhancement,
		EnhancementSpot: getString(L, 1),
		EnhancementImpl: getOptString(L, 2, ""),
	})
}

func (e *LuaEngine) luaSetWatchpoint(L *lua.LState) int {
	return e.setBreakpoint(L, adt.Breakpoint{
		Kind:           adt.BreakpointKindWatchpoint,
		Variable:       getString(L, 1),
		WatchCondition: getOptString(L, 2, "change"),
	})
}

func (e *LuaEngine) luaSetMethodBreakpoint(L *lua.LState) int {
	return e.setBreakpoint(L, adt.Breakpoint{
		Kind:       adt.BreakpointKindMethod,
		ClassName:  getString(L, 1),
		MethodName: getString(L, 2),
	})
}

func (e *LuaEngine) luaGetBreakpoints(L *lua.LState) int {
	user := getOptString(L, 1, "")

	resp, err := e.client.GetExternalBreakpoints(e.ctx, user)
	if err != nil {
		return pushError(L, err)
	}

	tbl := L.NewTable()
	for i, bp := range resp.Breakpoints {
		row := L.NewTable()
		L.SetField(row, "id", lua.LString(bp.ID))
		L.SetField(row, "kind", lua.LString(string(bp.Kind)))
		L.SetField(row, "uri", lua.LString(bp.URI))
		L.SetField(row, "line", lua.LNumber(bp.Line))
		L.SetField(row, "enabled", lua.LBool(bp.Enabled))
		tbl.RawSetInt(i+1, row)
	}

	L.Push(tbl)
	return 1
}

func (e *LuaEngine) luaDeleteBreakpoint(L *lua.LState) int {
	id := getString(L, 1)
	user := getOptString(L, 2, "")

	err := e.client.DeleteExternalBreakpoint(e.ctx, id, user)
	if err != nil {
		return pushBoolError(L, err)
	}

	L.Push(lua.LBool(true))
	return 1
}

// --- Session functions ---

func (e *LuaEngine) luaListen(L *lua.LState) int {
	timeout := getOptInt(L, 1, 30)

	opts := &adt.ListenOptions{
		TimeoutSeconds: timeout,
	}

	result, err := e.client.DebuggerListen(e.ctx, opts)
	if err != nil {
		return pushError(L, err)
	}

	if result == nil || result.Debuggee == nil {
		if result != nil && result.TimedOut {
			L.Push(lua.LNil)
			L.Push(lua.LString("timeout: no debuggee caught"))
		} else {
			L.Push(lua.LNil)
			L.Push(lua.LString("no debuggee caught"))
		}
		return 2
	}

	tbl := L.NewTable()
	L.SetField(tbl, "id", lua.LString(result.Debuggee.ID))
	L.SetField(tbl, "program", lua.LString(result.Debuggee.Program))
	L.SetField(tbl, "user", lua.LString(result.Debuggee.User))
	L.SetField(tbl, "line", lua.LNumber(result.Debuggee.Line))

	L.Push(tbl)
	return 1
}

func (e *LuaEngine) luaAttach(L *lua.LState) int {
	debuggeeID := getString(L, 1)
	user := getOptString(L, 2, "")

	result, err := e.client.DebuggerAttach(e.ctx, debuggeeID, user)
	if err != nil {
		return pushError(L, err)
	}

	tbl := L.NewTable()
	L.SetField(tbl, "session_id", lua.LString(result.DebugSessionID))
	L.SetField(tbl, "server", lua.LString(result.ServerName))
	L.SetField(tbl, "stepping_possible", lua.LBool(result.IsSteppingPossible))

	L.Push(tbl)
	return 1
}

func (e *LuaEngine) luaDetach(L *lua.LState) int {
	err := e.client.DebuggerDetach(e.ctx)
	if err != nil {
		return pushBoolError(L, err)
	}

	L.Push(lua.LBool(true))
	return 1
}

// --- Execution functions ---

func (e *LuaEngine) luaStepOver(L *lua.LState) int {
	return e.doStep(L, adt.DebugStepOver)
}

func (e *LuaEngine) luaStepInto(L *lua.LState) int {
	return e.doStep(L, adt.DebugStepInto)
}

func (e *LuaEngine) luaStepReturn(L *lua.LState) int {
	return e.doStep(L, adt.DebugStepReturn)
}

func (e *LuaEngine) luaContinue(L *lua.LState) int {
	return e.doStep(L, adt.DebugStepContinue)
}

// --- Inspection functions ---

func (e *LuaEngine) luaGetStack(L *lua.LState) int {
	stack, err := e.client.DebuggerGetStack(e.ctx, false)
	if err != nil {
		return pushError(L, err)
	}

	tbl := L.NewTable()
	for i, frame := range stack.Stack {
		row := L.NewTable()
		L.SetField(row, "program", lua.LString(frame.ProgramName))
		L.SetField(row, "include", lua.LString(frame.IncludeName))
		L.SetField(row, "line", lua.LNumber(frame.Line))
		L.SetField(row, "type", lua.LString(frame.StackType))
		L.SetField(row, "event", lua.LString(frame.EventName))
		tbl.RawSetInt(i+1, row)
	}

	L.Push(tbl)
	return 1
}

func (e *LuaEngine) luaGetVariables(L *lua.LState) int {
	varIDs := []string{}
	if L.GetTop() >= 1 {
		val := L.Get(1)
		if tbl, ok := val.(*lua.LTable); ok {
			tbl.ForEach(func(_, v lua.LValue) {
				if s, ok := v.(lua.LString); ok {
					varIDs = append(varIDs, string(s))
				}
			})
		} else if s, ok := val.(lua.LString); ok {
			varIDs = append(varIDs, string(s))
		}
	}

	if len(varIDs) == 0 {
		L.Push(lua.LNil)
		L.Push(lua.LString("variable IDs required"))
		return 2
	}

	vars, err := e.client.DebuggerGetVariables(e.ctx, varIDs)
	if err != nil {
		return pushError(L, err)
	}

	tbl := L.NewTable()
	for _, v := range vars {
		row := L.NewTable()
		L.SetField(row, "id", lua.LString(v.ID))
		L.SetField(row, "name", lua.LString(v.Name))
		L.SetField(row, "type", lua.LString(v.DeclaredTypeName))
		L.SetField(row, "value", lua.LString(v.Value))
		tbl.RawSetString(v.Name, row)
	}

	L.Push(tbl)
	return 1
}

func (e *LuaEngine) luaSetVariable(L *lua.LState) int {
	name := getString(L, 1)
	value := L.Get(2)

	var valueStr string
	switch v := value.(type) {
	case lua.LString:
		valueStr = string(v)
	case lua.LNumber:
		valueStr = fmt.Sprintf("%v", float64(v))
	case lua.LBool:
		if v {
			valueStr = "X" // ABAP true
		} else {
			valueStr = " " // ABAP false
		}
	default:
		goVal := luaToGo(value)
		jsonBytes, _ := jsonMarshal(goVal)
		valueStr = string(jsonBytes)
	}

	result, err := e.client.DebuggerSetVariableValue(e.ctx, name, valueStr)
	if err != nil {
		return pushBoolError(L, err)
	}

	L.Push(lua.LBool(true))
	L.Push(lua.LString(result))
	return 2
}
