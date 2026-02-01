// ABOUTME: Lua bindings for execution recording and history navigation.
// ABOUTME: Provides recording, history navigation, and force replay functions.

package scripting

import (
	"fmt"
	"strings"
	"time"

	"github.com/oisee/vibing-steampunk/pkg/adt"
	lua "github.com/yuin/gopher-lua"
)

// ensureHistoryManager initializes the history manager if needed.
func (e *LuaEngine) ensureHistoryManager(storePath string) error {
	if e.historyManager != nil {
		return nil
	}
	var err error
	e.historyManager, err = adt.NewHistoryManager(storePath)
	return err
}

// --- Recording functions ---

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
		return pushBoolError(L, fmt.Errorf("no active recording"))
	}

	storePath := getOptString(L, 1, ".vsp-recordings")

	if err := e.ensureHistoryManager(storePath); err != nil {
		return pushBoolError(L, err)
	}

	if err := e.historyManager.SaveRecording(e.recorder); err != nil {
		return pushBoolError(L, err)
	}

	L.Push(lua.LBool(true))
	L.Push(lua.LString(e.recorder.GetRecording().ID))
	return 2
}

// --- History Navigation functions ---

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

	if err := e.ensureHistoryManager(storePath); err != nil {
		return pushError(L, err)
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

	if err := e.ensureHistoryManager(storePath); err != nil {
		return pushError(L, err)
	}

	recording, err := e.historyManager.LoadRecording(recordingID)
	if err != nil {
		return pushError(L, err)
	}

	tbl := L.NewTable()
	L.SetField(tbl, "id", lua.LString(recording.ID))
	L.SetField(tbl, "session_id", lua.LString(recording.SessionID))
	L.SetField(tbl, "program", lua.LString(recording.Program))
	L.SetField(tbl, "total_steps", lua.LNumber(recording.TotalSteps))
	L.SetField(tbl, "is_complete", lua.LBool(recording.IsComplete))

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

	if err := e.ensureHistoryManager(storePath); err != nil {
		return pushError(L, err)
	}

	comparison, err := e.historyManager.CompareRecordings(id1, id2)
	if err != nil {
		return pushError(L, err)
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

// --- Force Replay functions ---

func (e *LuaEngine) luaForceReplay(L *lua.LState) int {
	recordingID := getString(L, 1)
	stepNumber := getOptInt(L, 2, -1)
	storePath := getOptString(L, 3, ".vsp-recordings")

	if err := e.ensureHistoryManager(storePath); err != nil {
		return pushBoolError(L, err)
	}

	recording, err := e.historyManager.LoadRecording(recordingID)
	if err != nil {
		L.Push(lua.LBool(false))
		L.Push(lua.LString("recording not found: " + err.Error()))
		return 2
	}

	if stepNumber == -1 {
		stepNumber = recording.TotalSteps
	}

	if stepNumber < 1 || stepNumber > recording.TotalSteps {
		L.Push(lua.LBool(false))
		L.Push(lua.LString(fmt.Sprintf("invalid step %d (recording has %d steps)", stepNumber, recording.TotalSteps)))
		return 2
	}

	frame := recording.Frames[stepNumber-1]
	vars := frame.Variables
	if vars == nil {
		vars = frame.VariableDelta
	}

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

func (e *LuaEngine) luaReplayFromStep(L *lua.LState) int {
	stepNumber := int(L.ToNumber(1))

	if e.recorder == nil {
		L.Push(lua.LBool(false))
		L.Push(lua.LString("no active recording"))
		return 2
	}

	vars := e.recorder.GetVariablesAtStep(stepNumber)
	if vars == nil {
		L.Push(lua.LBool(false))
		L.Push(lua.LString(fmt.Sprintf("step %d not found in current recording", stepNumber)))
		return 2
	}

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
