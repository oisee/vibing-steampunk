// ABOUTME: Lua bindings for checkpoint operations.
// ABOUTME: Provides saveCheckpoint, getCheckpoint, listCheckpoints, injectCheckpoint.

package scripting

import (
	"fmt"
	"strings"
	"time"

	lua "github.com/yuin/gopher-lua"
)

func (e *LuaEngine) luaSaveCheckpoint(L *lua.LState) int {
	name := getString(L, 1)

	checkpoint := make(map[string]any)
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

func (e *LuaEngine) luaInjectCheckpoint(L *lua.LState) int {
	name := getString(L, 1)

	checkpoint, ok := e.checkpoints[name]
	if !ok {
		L.Push(lua.LBool(false))
		L.Push(lua.LString("checkpoint not found: " + name))
		return 2
	}

	injected := 0
	failed := 0
	var lastError string

	for varName, value := range checkpoint {
		if strings.HasPrefix(varName, "_") {
			continue
		}

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
