package adt

import (
	"encoding/json"
	"testing"
	"time"
)

func TestNewExecutionRecorder(t *testing.T) {
	recorder := NewExecutionRecorder("test-session", "ZTEST_PROGRAM")

	if recorder == nil {
		t.Fatal("NewExecutionRecorder returned nil")
	}

	recording := recorder.GetRecording()
	if recording.SessionID != "test-session" {
		t.Errorf("expected session ID 'test-session', got '%s'", recording.SessionID)
	}
	if recording.Program != "ZTEST_PROGRAM" {
		t.Errorf("expected program 'ZTEST_PROGRAM', got '%s'", recording.Program)
	}
	if recording.TotalSteps != 0 {
		t.Errorf("expected 0 steps, got %d", recording.TotalSteps)
	}
	if recording.IsComplete {
		t.Error("expected IsComplete to be false")
	}
}

func TestRecordFrame(t *testing.T) {
	recorder := NewExecutionRecorder("test-session", "ZTEST")

	// Record first frame
	vars1 := map[string]VariableValue{
		"LV_COUNT": {Name: "LV_COUNT", Type: "I", Value: 0},
		"LV_NAME":  {Name: "LV_NAME", Type: "STRING", Value: "initial"},
	}
	recorder.RecordFrame(CodeLocation{Program: "ZTEST", Line: 10}, "entry", vars1)

	recording := recorder.GetRecording()
	if recording.TotalSteps != 1 {
		t.Errorf("expected 1 step, got %d", recording.TotalSteps)
	}

	// First frame should have full variables (not delta)
	frame := recorder.GetFrame(1)
	if frame == nil {
		t.Fatal("GetFrame(1) returned nil")
	}
	if len(frame.Variables) != 2 {
		t.Errorf("expected 2 variables, got %d", len(frame.Variables))
	}
	if frame.Location.Line != 10 {
		t.Errorf("expected line 10, got %d", frame.Location.Line)
	}

	// Record second frame with changes
	vars2 := map[string]VariableValue{
		"LV_COUNT": {Name: "LV_COUNT", Type: "I", Value: 1},
		"LV_NAME":  {Name: "LV_NAME", Type: "STRING", Value: "initial"}, // unchanged
	}
	recorder.RecordFrame(CodeLocation{Program: "ZTEST", Line: 11}, "step_over", vars2)

	if recording.TotalSteps != 2 {
		t.Errorf("expected 2 steps, got %d", recording.TotalSteps)
	}
}

func TestDeltaCompression(t *testing.T) {
	recorder := NewExecutionRecorder("test-session", "ZTEST")
	recorder.snapshotEvery = 5 // Full snapshot every 5 frames

	// Record 10 frames
	for i := 1; i <= 10; i++ {
		vars := map[string]VariableValue{
			"LV_COUNTER": {Name: "LV_COUNTER", Type: "I", Value: i},
		}
		recorder.RecordFrame(CodeLocation{Program: "ZTEST", Line: i * 10}, "step_over", vars)
	}

	recording := recorder.GetRecording()
	if recording.TotalSteps != 10 {
		t.Errorf("expected 10 steps, got %d", recording.TotalSteps)
	}

	// Frame 1 should have full variables
	frame1 := recorder.GetFrame(1)
	if len(frame1.Variables) == 0 {
		t.Error("frame 1 should have full variables")
	}

	// Frame 3 should have delta only (not at snapshot boundary)
	frame3 := recorder.GetFrame(3)
	if frame3.VariableDelta == nil {
		t.Error("frame 3 should have delta variables")
	}
	if frame3.BaseFrameRef != 1 {
		t.Errorf("frame 3 should reference base frame 1, got %d", frame3.BaseFrameRef)
	}

	// Frame 5 should have full variables (snapshot boundary)
	frame5 := recorder.GetFrame(5)
	if len(frame5.Variables) == 0 {
		t.Error("frame 5 should have full variables (snapshot)")
	}
}

func TestGetVariablesAtStep(t *testing.T) {
	recorder := NewExecutionRecorder("test-session", "ZTEST")
	recorder.snapshotEvery = 3

	// Record frames with changing values
	for i := 1; i <= 5; i++ {
		vars := map[string]VariableValue{
			"LV_VALUE": {Name: "LV_VALUE", Type: "I", Value: i * 10},
			"LV_CONST": {Name: "LV_CONST", Type: "STRING", Value: "constant"},
		}
		recorder.RecordFrame(CodeLocation{Program: "ZTEST", Line: i}, "step_over", vars)
	}

	// Get variables at step 4 (reconstructed from delta)
	vars := recorder.GetVariablesAtStep(4)
	if vars == nil {
		t.Fatal("GetVariablesAtStep(4) returned nil")
	}

	lvValue, ok := vars["LV_VALUE"]
	if !ok {
		t.Fatal("LV_VALUE not found")
	}
	if lvValue.Value != 40 {
		t.Errorf("expected LV_VALUE=40, got %v", lvValue.Value)
	}

	lvConst, ok := vars["LV_CONST"]
	if !ok {
		t.Fatal("LV_CONST not found")
	}
	if lvConst.Value != "constant" {
		t.Errorf("expected LV_CONST='constant', got %v", lvConst.Value)
	}
}

func TestFindWhenChanged(t *testing.T) {
	recorder := NewExecutionRecorder("test-session", "ZTEST")

	// Record frames where LV_STATUS changes at step 3
	statuses := []string{"INIT", "INIT", "PROCESSING", "PROCESSING", "DONE"}
	for i, status := range statuses {
		vars := map[string]VariableValue{
			"LV_STATUS": {Name: "LV_STATUS", Type: "STRING", Value: status},
		}
		recorder.RecordFrame(CodeLocation{Program: "ZTEST", Line: i + 1}, "step_over", vars)
	}

	// Find when status became "PROCESSING"
	step := recorder.FindWhenChanged("LV_STATUS", "PROCESSING")
	if step != 3 {
		t.Errorf("expected step 3, got %d", step)
	}

	// Find when status became "DONE"
	step = recorder.FindWhenChanged("LV_STATUS", "DONE")
	if step != 5 {
		t.Errorf("expected step 5, got %d", step)
	}

	// Find non-existent value
	step = recorder.FindWhenChanged("LV_STATUS", "ERROR")
	if step != -1 {
		t.Errorf("expected -1 for non-existent value, got %d", step)
	}
}

func TestFindChanges(t *testing.T) {
	recorder := NewExecutionRecorder("test-session", "ZTEST")

	// Record frames where LV_COUNTER changes
	for i := 1; i <= 5; i++ {
		vars := map[string]VariableValue{
			"LV_COUNTER": {Name: "LV_COUNTER", Type: "I", Value: i, IsChanged: i > 1},
		}
		recorder.RecordFrame(CodeLocation{Program: "ZTEST", Line: i}, "step_over", vars)
	}

	changes := recorder.FindChanges("LV_COUNTER")
	// All frames after first should show as changed
	if len(changes) < 1 {
		t.Errorf("expected at least 1 change, got %d", len(changes))
	}
}

func TestCheckpoints(t *testing.T) {
	recorder := NewExecutionRecorder("test-session", "ZTEST")

	// Record some frames
	for i := 1; i <= 5; i++ {
		vars := map[string]VariableValue{
			"LV_STEP": {Name: "LV_STEP", Type: "I", Value: i},
		}
		recorder.RecordFrame(CodeLocation{Program: "ZTEST", Line: i}, "step_over", vars)

		if i == 2 {
			recorder.AddCheckpoint("before_loop")
		}
		if i == 4 {
			recorder.AddCheckpoint("after_loop")
		}
	}

	recording := recorder.GetRecording()
	if len(recording.Checkpoints) != 2 {
		t.Errorf("expected 2 checkpoints, got %d", len(recording.Checkpoints))
	}

	if recording.Checkpoints["before_loop"] != 2 {
		t.Errorf("expected checkpoint 'before_loop' at step 2, got %d", recording.Checkpoints["before_loop"])
	}
	if recording.Checkpoints["after_loop"] != 4 {
		t.Errorf("expected checkpoint 'after_loop' at step 4, got %d", recording.Checkpoints["after_loop"])
	}
}

func TestRecordingComplete(t *testing.T) {
	recorder := NewExecutionRecorder("test-session", "ZTEST")

	vars := map[string]VariableValue{
		"LV_DONE": {Name: "LV_DONE", Type: "C", Value: "X"},
	}
	recorder.RecordFrame(CodeLocation{Program: "ZTEST", Line: 100}, "exit", vars)

	recorder.Complete()

	recording := recorder.GetRecording()
	if !recording.IsComplete {
		t.Error("expected IsComplete to be true")
	}
	if recording.EndTime.IsZero() {
		t.Error("expected EndTime to be set")
	}
}

func TestToJSON(t *testing.T) {
	recorder := NewExecutionRecorder("test-session", "ZTEST")

	vars := map[string]VariableValue{
		"LV_TEST": {Name: "LV_TEST", Type: "STRING", Value: "hello"},
	}
	recorder.RecordFrame(CodeLocation{Program: "ZTEST", Line: 1}, "entry", vars)
	recorder.Complete()

	data, err := recorder.ToJSON()
	if err != nil {
		t.Fatalf("ToJSON failed: %v", err)
	}

	// Verify it's valid JSON
	var parsed ExecutionRecording
	if err := json.Unmarshal(data, &parsed); err != nil {
		t.Fatalf("Failed to parse JSON: %v", err)
	}

	if parsed.SessionID != "test-session" {
		t.Errorf("expected session ID 'test-session', got '%s'", parsed.SessionID)
	}
	if parsed.TotalSteps != 1 {
		t.Errorf("expected 1 step, got %d", parsed.TotalSteps)
	}
}

func TestFromJSON(t *testing.T) {
	// Create a recording
	original := &ExecutionRecording{
		ID:         "test-123",
		SessionID:  "session-456",
		Program:    "ZTEST",
		StartTime:  time.Now(),
		TotalSteps: 3,
		IsComplete: true,
		Frames: []ExecutionFrame{
			{StepNumber: 1, Location: CodeLocation{Program: "ZTEST", Line: 10}},
			{StepNumber: 2, Location: CodeLocation{Program: "ZTEST", Line: 20}},
			{StepNumber: 3, Location: CodeLocation{Program: "ZTEST", Line: 30}},
		},
		Checkpoints: map[string]int{"start": 1, "end": 3},
	}

	data, _ := json.Marshal(original)

	parsed, err := FromJSON(data)
	if err != nil {
		t.Fatalf("FromJSON failed: %v", err)
	}

	if parsed.ID != original.ID {
		t.Errorf("expected ID '%s', got '%s'", original.ID, parsed.ID)
	}
	if parsed.TotalSteps != original.TotalSteps {
		t.Errorf("expected %d steps, got %d", original.TotalSteps, parsed.TotalSteps)
	}
	if len(parsed.Frames) != len(original.Frames) {
		t.Errorf("expected %d frames, got %d", len(original.Frames), len(parsed.Frames))
	}
}

func TestStats(t *testing.T) {
	recorder := NewExecutionRecorder("test-session", "ZTEST")

	// Record some frames with DB ops and RFC calls
	for i := 1; i <= 5; i++ {
		vars := map[string]VariableValue{
			"LV_I": {Name: "LV_I", Type: "I", Value: i},
		}
		recorder.RecordFrame(CodeLocation{Program: "ZTEST", Line: i * 10}, "step_over", vars)

		if i == 2 {
			recorder.AddDBOperation(DBOperation{Operation: "SELECT", Table: "MARA", Rows: 10})
		}
		if i == 3 {
			recorder.AddRFCCall(RFCCall{Function: "BAPI_USER_GET_DETAIL"})
		}
		if i == 4 {
			recorder.AddCheckpoint("checkpoint1")
		}
	}
	recorder.Complete()

	stats := recorder.Stats()

	if stats["total_steps"].(int) != 5 {
		t.Errorf("expected total_steps=5, got %v", stats["total_steps"])
	}
	if stats["total_db_ops"].(int) != 1 {
		t.Errorf("expected total_db_ops=1, got %v", stats["total_db_ops"])
	}
	if stats["total_rfc_calls"].(int) != 1 {
		t.Errorf("expected total_rfc_calls=1, got %v", stats["total_rfc_calls"])
	}
	if stats["checkpoints"].(int) != 1 {
		t.Errorf("expected checkpoints=1, got %v", stats["checkpoints"])
	}
	if !stats["is_complete"].(bool) {
		t.Error("expected is_complete=true")
	}
}

func TestVariablesEqual(t *testing.T) {
	tests := []struct {
		name     string
		a, b     VariableValue
		expected bool
	}{
		{
			name:     "equal strings",
			a:        VariableValue{Value: "hello"},
			b:        VariableValue{Value: "hello"},
			expected: true,
		},
		{
			name:     "different strings",
			a:        VariableValue{Value: "hello"},
			b:        VariableValue{Value: "world"},
			expected: false,
		},
		{
			name:     "equal numbers",
			a:        VariableValue{Value: 42},
			b:        VariableValue{Value: 42},
			expected: true,
		},
		{
			name:     "different numbers",
			a:        VariableValue{Value: 42},
			b:        VariableValue{Value: 43},
			expected: false,
		},
		{
			name:     "equal bools",
			a:        VariableValue{Value: true},
			b:        VariableValue{Value: true},
			expected: true,
		},
		{
			name:     "different types",
			a:        VariableValue{Value: "42"},
			b:        VariableValue{Value: 42},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := variablesEqual(tt.a, tt.b)
			if result != tt.expected {
				t.Errorf("variablesEqual(%v, %v) = %v, expected %v", tt.a.Value, tt.b.Value, result, tt.expected)
			}
		})
	}
}
