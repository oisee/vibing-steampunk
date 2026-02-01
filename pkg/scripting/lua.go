// Package scripting provides Lua scripting support for vsp.
// It enables TAS-style debugging with scriptable breakpoints,
// variable recording, and automated debug workflows.
package scripting

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/oisee/vibing-steampunk/pkg/adt"
	lua "github.com/yuin/gopher-lua"
)

// LuaEngine wraps a Lua VM with vsp bindings.
type LuaEngine struct {
	L      *lua.LState
	client *adt.Client
	ctx    context.Context
	output io.Writer

	// Checkpoints for state capture
	checkpoints map[string]map[string]any

	// Execution recording for replay
	recorder       *adt.ExecutionRecorder
	historyManager *adt.HistoryManager
	isRecording    bool
}

// NewLuaEngine creates a new Lua engine with ADT client bindings.
func NewLuaEngine(client *adt.Client) *LuaEngine {
	L := lua.NewState(lua.Options{
		CallStackSize:       120,
		RegistrySize:        1024 * 20,
		SkipOpenLibs:        false,
		IncludeGoStackTrace: true,
	})

	engine := &LuaEngine{
		L:           L,
		client:      client,
		ctx:         context.Background(),
		output:      os.Stdout,
		checkpoints: make(map[string]map[string]any),
	}

	engine.registerBuiltins()
	engine.registerADTBindings()

	return engine
}

// SetContext sets the context for ADT operations.
func (e *LuaEngine) SetContext(ctx context.Context) {
	e.ctx = ctx
}

// SetOutput sets the output writer for print statements.
func (e *LuaEngine) SetOutput(w io.Writer) {
	e.output = w
}

// Close closes the Lua state.
func (e *LuaEngine) Close() {
	e.L.Close()
}

// Execute runs a Lua script string.
func (e *LuaEngine) Execute(script string) error {
	return e.L.DoString(script)
}

// ExecuteFile runs a Lua script file.
func (e *LuaEngine) ExecuteFile(path string) error {
	return e.L.DoFile(path)
}

// REPL runs an interactive Lua Read-Eval-Print Loop.
func (e *LuaEngine) REPL() {
	reader := bufio.NewReader(os.Stdin)
	fmt.Fprintln(e.output, "vsp Lua REPL v0.1.0")
	fmt.Fprintln(e.output, "Type 'exit' to quit, 'help' for commands.")
	fmt.Fprintln(e.output, "")

	for {
		fmt.Fprint(e.output, "lua> ")
		line, err := reader.ReadString('\n')
		if err != nil {
			if err == io.EOF {
				fmt.Fprintln(e.output, "\nGoodbye!")
				return
			}
			fmt.Fprintf(e.output, "Error reading input: %v\n", err)
			continue
		}

		line = strings.TrimSpace(line)

		if line == "" {
			continue
		}

		if line == "exit" || line == "quit" {
			fmt.Fprintln(e.output, "Goodbye!")
			return
		}

		if line == "help" {
			e.printHelp()
			continue
		}

		if err := e.Execute(line); err != nil {
			fmt.Fprintf(e.output, "Error: %v\n", err)
		}
	}
}

func (e *LuaEngine) printHelp() {
	help := `
vsp Lua Commands:
─────────────────────────────────────────────────────────
  exit, quit     Exit the REPL

Search & Source:
  searchObject(query, [type])     Search for ABAP objects
  grepObjects(pattern, [type])    Grep in object sources
  getSource(type, name)           Get source code
  writeSource(type, name, src)    Write source code
  editSource(type, name, old, new) Edit source code

Debugging - Breakpoints:
  setBreakpoint(prog, line)       Set line breakpoint
  setStatementBP(statement)       Set statement breakpoint (e.g., "COMMIT")
  setExceptionBP(exception)       Set exception breakpoint (e.g., "CX_SY_ZERODIVIDE")
  setMessageBP(class, num, [type]) Set message breakpoint (e.g., "00", "001", "E")
  setBadiBP(badiName)             Set BAdi breakpoint
  setEnhancementBP(spot, [impl])  Set enhancement point breakpoint
  setWatchpoint(var, [condition]) Set watchpoint (condition: change/read/any)
  setMethodBP(class, method)      Set method entry breakpoint
  getBreakpoints()                List active breakpoints
  deleteBreakpoint(id)            Delete a breakpoint

  listen([timeout])               Wait for debuggee
  attach(debuggeeId)              Attach to debuggee
  detach()                        Detach from debuggee

  stepOver()                      Step over
  stepInto()                      Step into
  stepReturn()                    Step out
  continue_()                     Continue execution

  getStack()                      Get call stack
  getVariables([scope])           Get variables
  setVariable(name, value)        Set variable value (FORCE REPLAY!)

Recording & Checkpoints:
  saveCheckpoint(name)            Save current state
  getCheckpoint(name)             Get saved state
  listCheckpoints()               List all checkpoints
  injectCheckpoint(name)          Inject saved state

Execution Recording:
  startRecording([session], [program])  Start recording execution
  stopRecording()                       Stop and get statistics
  getRecording()                        Get current recording info
  saveRecording([path])                 Save recording to disk

History Navigation:
  getStateAtStep(stepNumber)      Get variable state at step N
  findWhenChanged(var, value)     Find when variable became value
  findChanges(varName)            Find all steps where var changed
  listRecordings([path])          List saved recordings
  loadRecording(id, [path])       Load a saved recording
  compareRecordings(id1, id2)     Compare two recordings

Force Replay:
  setVariable(name, value)        Modify variable in live session
  injectCheckpoint(name)          Inject all vars from checkpoint
  forceReplay(recordingId, [step]) Inject state from saved recording
  replayFromStep(stepNumber)      Inject state from current recording

Utilities:
  print(...)                      Print values
  sleep(seconds)                  Sleep for N seconds
  json.encode(value)              Encode to JSON
  json.decode(str)                Decode from JSON
─────────────────────────────────────────────────────────
`
	fmt.Fprintln(e.output, help)
}

// registerBuiltins registers built-in Lua functions.
func (e *LuaEngine) registerBuiltins() {
	// Override print to use our output writer
	e.L.SetGlobal("print", e.L.NewFunction(e.luaPrint))

	// Sleep function
	e.L.SetGlobal("sleep", e.L.NewFunction(e.luaSleep))

	// JSON module
	jsonMod := e.L.NewTable()
	e.L.SetField(jsonMod, "encode", e.L.NewFunction(e.luaJSONEncode))
	e.L.SetField(jsonMod, "decode", e.L.NewFunction(e.luaJSONDecode))
	e.L.SetGlobal("json", jsonMod)
}

// luaPrint implements print() for Lua.
func (e *LuaEngine) luaPrint(L *lua.LState) int {
	top := L.GetTop()
	parts := make([]string, top)
	for i := 1; i <= top; i++ {
		parts[i-1] = L.ToStringMeta(L.Get(i)).String()
	}
	fmt.Fprintln(e.output, strings.Join(parts, "\t"))
	return 0
}

// luaSleep implements sleep(seconds) for Lua.
func (e *LuaEngine) luaSleep(L *lua.LState) int {
	seconds := L.ToNumber(1)
	if seconds <= 0 {
		seconds = 1
	}

	select {
	case <-e.ctx.Done():
		L.RaiseError("context cancelled")
	case <-contextWithTimeout(e.ctx, seconds):
	}

	return 0
}

// luaJSONEncode implements json.encode(value) for Lua.
func (e *LuaEngine) luaJSONEncode(L *lua.LState) int {
	val := L.Get(1)
	goVal := luaToGo(val)

	jsonBytes, err := jsonMarshal(goVal)
	if err != nil {
		L.Push(lua.LNil)
		L.Push(lua.LString(err.Error()))
		return 2
	}

	L.Push(lua.LString(string(jsonBytes)))
	return 1
}

// luaJSONDecode implements json.decode(str) for Lua.
func (e *LuaEngine) luaJSONDecode(L *lua.LState) int {
	str := L.ToString(1)

	var goVal any
	if err := jsonUnmarshal([]byte(str), &goVal); err != nil {
		L.Push(lua.LNil)
		L.Push(lua.LString(err.Error()))
		return 2
	}

	L.Push(goToLua(L, goVal))
	return 1
}
