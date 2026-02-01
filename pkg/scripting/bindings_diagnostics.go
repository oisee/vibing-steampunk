// ABOUTME: Lua bindings for diagnostics operations.
// ABOUTME: Provides getDumps, getDump, getMessages, runUnitTests, syntaxCheck.

package scripting

import (
	"github.com/oisee/vibing-steampunk/pkg/adt"
	lua "github.com/yuin/gopher-lua"
)

func (e *LuaEngine) luaGetDumps(L *lua.LState) int {
	maxResults := getOptInt(L, 1, 20)

	dumps, err := e.client.GetDumps(e.ctx, &adt.DumpQueryOptions{
		MaxResults: maxResults,
	})
	if err != nil {
		return pushError(L, err)
	}

	tbl := L.NewTable()
	for i, dump := range dumps {
		row := L.NewTable()
		L.SetField(row, "id", lua.LString(dump.ID))
		L.SetField(row, "program", lua.LString(dump.Program))
		L.SetField(row, "exception", lua.LString(dump.ExceptionType))
		L.SetField(row, "user", lua.LString(dump.User))
		L.SetField(row, "time", lua.LString(dump.Timestamp))
		L.SetField(row, "title", lua.LString(dump.Title))
		tbl.RawSetInt(i+1, row)
	}

	L.Push(tbl)
	return 1
}

func (e *LuaEngine) luaGetDump(L *lua.LState) int {
	dumpID := getString(L, 1)

	dump, err := e.client.GetDump(e.ctx, dumpID)
	if err != nil {
		return pushError(L, err)
	}

	tbl := L.NewTable()
	L.SetField(tbl, "id", lua.LString(dump.ID))
	L.SetField(tbl, "program", lua.LString(dump.Program))
	L.SetField(tbl, "exception", lua.LString(dump.ExceptionType))
	L.SetField(tbl, "title", lua.LString(dump.Title))
	L.SetField(tbl, "user", lua.LString(dump.User))
	L.SetField(tbl, "line", lua.LNumber(dump.Line))
	L.SetField(tbl, "time", lua.LString(dump.Timestamp))

	if len(dump.StackTrace) > 0 {
		stack := L.NewTable()
		for i, frame := range dump.StackTrace {
			row := L.NewTable()
			L.SetField(row, "program", lua.LString(frame.Program))
			L.SetField(row, "include", lua.LString(frame.Include))
			L.SetField(row, "line", lua.LNumber(frame.Line))
			L.SetField(row, "event", lua.LString(frame.Event))
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
		return pushError(L, err)
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
		return pushError(L, err)
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
		return pushError(L, err)
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
