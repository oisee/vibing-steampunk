// ABOUTME: Lua bindings for search and source code operations.
// ABOUTME: Provides searchObject, grepObjects, getSource, writeSource, editSource.

package scripting

import (
	"github.com/oisee/vibing-steampunk/pkg/adt"
	lua "github.com/yuin/gopher-lua"
)

func (e *LuaEngine) luaSearchObject(L *lua.LState) int {
	query := getString(L, 1)
	maxResults := getOptInt(L, 2, 100)

	results, err := e.client.SearchObject(e.ctx, query, maxResults)
	if err != nil {
		return pushError(L, err)
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
	pattern := getString(L, 1)
	objectQuery := getOptString(L, 2, "")
	contextLines := getOptInt(L, 3, 0)

	if objectQuery == "" {
		objectQuery = "*"
	}

	objects, err := e.client.SearchObject(e.ctx, objectQuery, 50)
	if err != nil {
		return pushError(L, err)
	}

	objectURLs := make([]string, len(objects))
	for i, obj := range objects {
		objectURLs[i] = obj.URI
	}

	if len(objectURLs) == 0 {
		L.Push(L.NewTable())
		return 1
	}

	result, err := e.client.GrepObjects(e.ctx, objectURLs, pattern, false, contextLines)
	if err != nil {
		return pushError(L, err)
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
		return pushError(L, err)
	}

	L.Push(lua.LString(source))
	return 1
}

func (e *LuaEngine) luaWriteSource(L *lua.LState) int {
	objType := getString(L, 1)
	name := getString(L, 2)
	source := getString(L, 3)

	result, err := e.client.WriteSource(e.ctx, objType, name, source, nil)
	if err != nil {
		return pushBoolError(L, err)
	}

	L.Push(lua.LBool(result.Success))
	return 1
}

func (e *LuaEngine) luaEditSource(L *lua.LState) int {
	objectURI := getString(L, 1)
	oldText := getString(L, 2)
	newText := getString(L, 3)
	replaceAll := getBool(L, 4)

	result, err := e.client.EditSource(e.ctx, objectURI, oldText, newText, replaceAll, true, false)
	if err != nil {
		return pushBoolError(L, err)
	}

	L.Push(lua.LBool(result.Success))
	return 1
}
