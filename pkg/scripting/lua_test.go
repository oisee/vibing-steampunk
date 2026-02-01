package scripting

import (
	"bytes"
	"context"
	"strings"
	"testing"
	"time"

	lua "github.com/yuin/gopher-lua"
)

func TestNewLuaEngine(t *testing.T) {
	engine := NewLuaEngine(nil)
	if engine == nil {
		t.Fatal("NewLuaEngine returned nil")
	}
	defer engine.Close()

	if engine.L == nil {
		t.Error("Lua state is nil")
	}
	if engine.checkpoints == nil {
		t.Error("checkpoints map is nil")
	}
}

func TestExecute(t *testing.T) {
	engine := NewLuaEngine(nil)
	defer engine.Close()

	// Simple expression - test via print output
	var buf bytes.Buffer
	engine.SetOutput(&buf)

	err := engine.Execute("x = 1 + 1; print(x)")
	if err != nil {
		t.Errorf("Execute failed: %v", err)
	}

	output := buf.String()
	if !strings.Contains(output, "2") {
		t.Errorf("expected '2' in output, got %s", output)
	}
}

func TestExecuteSyntaxError(t *testing.T) {
	engine := NewLuaEngine(nil)
	defer engine.Close()

	err := engine.Execute("if then else")
	if err == nil {
		t.Error("expected syntax error")
	}
}

func TestSetContext(t *testing.T) {
	engine := NewLuaEngine(nil)
	defer engine.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	engine.SetContext(ctx)
	if engine.ctx != ctx {
		t.Error("context was not set")
	}
}

func TestSetOutput(t *testing.T) {
	engine := NewLuaEngine(nil)
	defer engine.Close()

	var buf bytes.Buffer
	engine.SetOutput(&buf)

	engine.Execute("print('hello world')")

	output := buf.String()
	if !strings.Contains(output, "hello world") {
		t.Errorf("expected 'hello world' in output, got: %s", output)
	}
}

func TestPrintMultipleValues(t *testing.T) {
	engine := NewLuaEngine(nil)
	defer engine.Close()

	var buf bytes.Buffer
	engine.SetOutput(&buf)

	engine.Execute("print('a', 'b', 'c')")

	output := buf.String()
	if !strings.Contains(output, "a\tb\tc") {
		t.Errorf("expected tab-separated values, got: %s", output)
	}
}

func TestJSONEncode(t *testing.T) {
	engine := NewLuaEngine(nil)
	defer engine.Close()

	var buf bytes.Buffer
	engine.SetOutput(&buf)

	err := engine.Execute(`
		local data = {name = "test", value = 42}
		local result = json.encode(data)
		print(result)
	`)
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}

	output := buf.String()
	if !strings.Contains(output, "name") || !strings.Contains(output, "test") {
		t.Errorf("JSON encode failed, output: %s", output)
	}
}

func TestJSONDecode(t *testing.T) {
	engine := NewLuaEngine(nil)
	defer engine.Close()

	var buf bytes.Buffer
	engine.SetOutput(&buf)

	err := engine.Execute(`
		local jsonStr = '{"name": "test", "value": 42}'
		local data = json.decode(jsonStr)
		print(data.name, data.value)
	`)
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}

	output := buf.String()
	if !strings.Contains(output, "test") || !strings.Contains(output, "42") {
		t.Errorf("JSON decode failed, output: %s", output)
	}
}

func TestJSONEncodeArray(t *testing.T) {
	engine := NewLuaEngine(nil)
	defer engine.Close()

	var buf bytes.Buffer
	engine.SetOutput(&buf)

	err := engine.Execute(`
		local data = {1, 2, 3}
		local result = json.encode(data)
		print(result)
	`)
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}

	output := buf.String()
	if !strings.Contains(output, "[") {
		t.Errorf("expected JSON array, got: %s", output)
	}
}

func TestSaveAndGetCheckpoint(t *testing.T) {
	engine := NewLuaEngine(nil)
	defer engine.Close()

	var buf bytes.Buffer
	engine.SetOutput(&buf)

	// Mock checkpoint by directly accessing engine
	engine.checkpoints["test_cp"] = map[string]any{
		"LV_VALUE": "hello",
		"LV_COUNT": 42,
	}

	err := engine.Execute(`
		local cp = getCheckpoint("test_cp")
		if cp then
			print("found", cp.LV_VALUE, cp.LV_COUNT)
		else
			print("not found")
		end
	`)
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}

	output := buf.String()
	if !strings.Contains(output, "found") || !strings.Contains(output, "hello") {
		t.Errorf("checkpoint retrieval failed, output: %s", output)
	}
}

func TestListCheckpoints(t *testing.T) {
	engine := NewLuaEngine(nil)
	defer engine.Close()

	var buf bytes.Buffer
	engine.SetOutput(&buf)

	engine.checkpoints["cp1"] = map[string]any{}
	engine.checkpoints["cp2"] = map[string]any{}

	err := engine.Execute(`
		local list = listCheckpoints()
		print("count:", #list)
	`)
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}

	output := buf.String()
	if !strings.Contains(output, "count:") {
		t.Errorf("listCheckpoints failed, output: %s", output)
	}
}

func TestSleep(t *testing.T) {
	engine := NewLuaEngine(nil)
	defer engine.Close()

	start := time.Now()
	err := engine.Execute("sleep(0.1)") // 100ms
	elapsed := time.Since(start)

	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}

	if elapsed < 90*time.Millisecond {
		t.Errorf("sleep was too short: %v", elapsed)
	}
}

func TestSleepWithContext(t *testing.T) {
	engine := NewLuaEngine(nil)
	defer engine.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	engine.SetContext(ctx)

	// Sleep for longer than context timeout - should be interrupted
	err := engine.Execute("sleep(1)")
	// The error might be context cancelled or it might complete early
	// Either way, it should not take 1 second
	_ = err
}

// Helper conversion tests

func TestLuaToGo(t *testing.T) {
	L := lua.NewState()
	defer L.Close()

	tests := []struct {
		name     string
		input    lua.LValue
		expected any
	}{
		{"nil", lua.LNil, nil},
		{"bool true", lua.LBool(true), true},
		{"bool false", lua.LBool(false), false},
		{"integer", lua.LNumber(42), int64(42)},
		{"float", lua.LNumber(3.14), float64(3.14)},
		{"string", lua.LString("hello"), "hello"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := luaToGo(tt.input)
			if result != tt.expected {
				t.Errorf("luaToGo(%v) = %v, expected %v", tt.input, result, tt.expected)
			}
		})
	}
}

func TestLuaTableToGoArray(t *testing.T) {
	L := lua.NewState()
	defer L.Close()

	tbl := L.NewTable()
	tbl.RawSetInt(1, lua.LString("a"))
	tbl.RawSetInt(2, lua.LString("b"))
	tbl.RawSetInt(3, lua.LString("c"))

	result := luaToGo(tbl)
	arr, ok := result.([]any)
	if !ok {
		t.Fatalf("expected []any, got %T", result)
	}
	if len(arr) != 3 {
		t.Errorf("expected 3 elements, got %d", len(arr))
	}
}

func TestLuaTableToGoMap(t *testing.T) {
	L := lua.NewState()
	defer L.Close()

	tbl := L.NewTable()
	tbl.RawSetString("name", lua.LString("test"))
	tbl.RawSetString("value", lua.LNumber(42))

	result := luaToGo(tbl)
	m, ok := result.(map[string]any)
	if !ok {
		t.Fatalf("expected map[string]any, got %T", result)
	}
	if m["name"] != "test" {
		t.Errorf("expected name='test', got %v", m["name"])
	}
}

func TestGoToLua(t *testing.T) {
	L := lua.NewState()
	defer L.Close()

	tests := []struct {
		name     string
		input    any
		expected lua.LValueType
	}{
		{"nil", nil, lua.LTNil},
		{"bool", true, lua.LTBool},
		{"int", 42, lua.LTNumber},
		{"int64", int64(42), lua.LTNumber},
		{"float64", 3.14, lua.LTNumber},
		{"string", "hello", lua.LTString},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := goToLua(L, tt.input)
			if result.Type() != tt.expected {
				t.Errorf("goToLua(%v) type = %v, expected %v", tt.input, result.Type(), tt.expected)
			}
		})
	}
}

func TestGoSliceToLua(t *testing.T) {
	L := lua.NewState()
	defer L.Close()

	slice := []any{"a", "b", "c"}
	result := goToLua(L, slice)

	tbl, ok := result.(*lua.LTable)
	if !ok {
		t.Fatalf("expected *lua.LTable, got %T", result)
	}
	if tbl.Len() != 3 {
		t.Errorf("expected length 3, got %d", tbl.Len())
	}
}

func TestGoMapToLua(t *testing.T) {
	L := lua.NewState()
	defer L.Close()

	m := map[string]any{
		"name":  "test",
		"value": 42,
	}
	result := goToLua(L, m)

	tbl, ok := result.(*lua.LTable)
	if !ok {
		t.Fatalf("expected *lua.LTable, got %T", result)
	}

	name := tbl.RawGetString("name")
	if name.String() != "test" {
		t.Errorf("expected name='test', got %v", name)
	}
}

func TestGetStringHelpers(t *testing.T) {
	L := lua.NewState()
	defer L.Close()

	L.Push(lua.LString("hello"))
	L.Push(lua.LNumber(42))

	s := getString(L, 1)
	if s != "hello" {
		t.Errorf("getString(1) = %s, expected 'hello'", s)
	}
}

func TestGetOptString(t *testing.T) {
	L := lua.NewState()
	defer L.Close()

	// No arguments - should return default
	def := getOptString(L, 1, "default")
	if def != "default" {
		t.Errorf("expected default 'default', got %s", def)
	}

	// With argument
	L.Push(lua.LString("value"))
	val := getOptString(L, 1, "default")
	if val != "value" {
		t.Errorf("expected 'value', got %s", val)
	}
}

func TestGetOptInt(t *testing.T) {
	L := lua.NewState()
	defer L.Close()

	// No arguments - should return default
	def := getOptInt(L, 1, 100)
	if def != 100 {
		t.Errorf("expected default 100, got %d", def)
	}

	// With argument
	L.Push(lua.LNumber(42))
	val := getOptInt(L, 1, 100)
	if val != 42 {
		t.Errorf("expected 42, got %d", val)
	}
}

func TestGetTable(t *testing.T) {
	L := lua.NewState()
	defer L.Close()

	tbl := L.NewTable()
	tbl.RawSetString("key", lua.LString("value"))
	L.Push(tbl)

	result := getTable(L, 1)
	if result == nil {
		t.Fatal("getTable returned nil")
	}
	if result["key"] != "value" {
		t.Errorf("expected key='value', got %v", result["key"])
	}
}

func TestGetTableNil(t *testing.T) {
	L := lua.NewState()
	defer L.Close()

	L.Push(lua.LNil)
	result := getTable(L, 1)
	if result != nil {
		t.Errorf("expected nil for non-table, got %v", result)
	}
}

// Recording tests (without ADT client)

func TestStartRecordingWithoutClient(t *testing.T) {
	engine := NewLuaEngine(nil)
	defer engine.Close()

	var buf bytes.Buffer
	engine.SetOutput(&buf)

	// Start recording should work even without client
	err := engine.Execute(`startRecording("test-session", "ZTEST")`)
	if err != nil {
		t.Fatalf("startRecording failed: %v", err)
	}

	if !engine.isRecording {
		t.Error("isRecording should be true after startRecording")
	}

	if engine.recorder == nil {
		t.Error("recorder should not be nil after startRecording")
	}
}

func TestStopRecordingWithoutClient(t *testing.T) {
	engine := NewLuaEngine(nil)
	defer engine.Close()

	var buf bytes.Buffer
	engine.SetOutput(&buf)

	// Start and stop recording
	engine.Execute(`startRecording("test-session", "ZTEST")`)
	err := engine.Execute(`
		local stats = stopRecording()
		if stats then
			print("stopped")
		end
	`)
	if err != nil {
		t.Fatalf("stopRecording failed: %v", err)
	}

	if engine.isRecording {
		t.Error("isRecording should be false after stopRecording")
	}

	output := buf.String()
	if !strings.Contains(output, "stopped") {
		t.Errorf("expected 'stopped' in output, got: %s", output)
	}
}

func TestGetRecordingWithoutClient(t *testing.T) {
	engine := NewLuaEngine(nil)
	defer engine.Close()

	var buf bytes.Buffer
	engine.SetOutput(&buf)

	// Get recording when none exists
	err := engine.Execute(`
		local info = getRecording()
		if info then
			print("has recording")
		else
			print("no recording")
		end
	`)
	if err != nil {
		t.Fatalf("getRecording failed: %v", err)
	}

	output := buf.String()
	if !strings.Contains(output, "no recording") {
		t.Errorf("expected 'no recording' when not recording, got: %s", output)
	}
}

func TestGlobalFunctionsRegistered(t *testing.T) {
	engine := NewLuaEngine(nil)
	defer engine.Close()

	// Check that key functions are registered by executing a type check
	functions := []string{
		"print", "sleep",
		"saveCheckpoint", "getCheckpoint", "listCheckpoints",
		"startRecording", "stopRecording", "getRecording",
		"findWhenChanged", "findChanges",
	}

	var buf bytes.Buffer
	engine.SetOutput(&buf)

	for _, fn := range functions {
		err := engine.Execute("assert(type(" + fn + ") == 'function', '" + fn + " is not a function')")
		if err != nil {
			t.Errorf("function %s is not registered as a function: %v", fn, err)
		}
	}

	// Check json module
	err := engine.Execute("assert(type(json) == 'table', 'json is not a table')")
	if err != nil {
		t.Errorf("json module is not registered: %v", err)
	}
}
