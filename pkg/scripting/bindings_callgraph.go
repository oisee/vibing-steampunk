// ABOUTME: Lua bindings for call graph analysis.
// ABOUTME: Provides getCallGraph, getCallersOf, getCalleesOf functions.

package scripting

import (
	"github.com/oisee/vibing-steampunk/pkg/adt"
	lua "github.com/yuin/gopher-lua"
)

func (e *LuaEngine) luaGetCallGraph(L *lua.LState) int {
	objectURI := getString(L, 1)
	direction := getOptString(L, 2, "callees")
	maxDepth := getOptInt(L, 3, 5)

	graph, err := e.client.GetCallGraph(e.ctx, objectURI, &adt.CallGraphOptions{
		Direction:  direction,
		MaxDepth:   maxDepth,
		MaxResults: 500,
	})
	if err != nil {
		return pushError(L, err)
	}

	L.Push(callGraphToLua(L, graph))
	return 1
}

func (e *LuaEngine) luaGetCallersOf(L *lua.LState) int {
	objectURI := getString(L, 1)
	maxDepth := getOptInt(L, 2, 5)

	graph, err := e.client.GetCallersOf(e.ctx, objectURI, maxDepth)
	if err != nil {
		return pushError(L, err)
	}

	L.Push(callGraphToLua(L, graph))
	return 1
}

func (e *LuaEngine) luaGetCalleesOf(L *lua.LState) int {
	objectURI := getString(L, 1)
	maxDepth := getOptInt(L, 2, 5)

	graph, err := e.client.GetCalleesOf(e.ctx, objectURI, maxDepth)
	if err != nil {
		return pushError(L, err)
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
