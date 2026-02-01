// Package adt provides ABAP Development Tools client functionality.
// recorder.go implements execution recording for time-travel debugging.
package adt

import (
	"encoding/json"
	"maps"
	"sync"
	"time"
)

// CodeLocation represents a position in ABAP source code.
type CodeLocation struct {
	Program   string `json:"program"`
	Include   string `json:"include,omitempty"`
	Line      int    `json:"line"`
	Statement string `json:"statement,omitempty"`
}

// VariableValue represents a captured variable value.
type VariableValue struct {
	Name      string `json:"name"`
	Type      string `json:"type"`
	Value     any    `json:"value"`
	IsChanged bool   `json:"is_changed,omitempty"` // Changed since last frame
}

// DBOperation represents a database operation captured during execution.
type DBOperation struct {
	Operation string         `json:"operation"` // SELECT, INSERT, UPDATE, DELETE
	Table     string         `json:"table"`
	Rows      int            `json:"rows,omitempty"`
	Duration  time.Duration  `json:"duration,omitempty"`
	Details   map[string]any `json:"details,omitempty"`
}

// RFCCall represents an RFC call captured during execution.
type RFCCall struct {
	Function  string         `json:"function"`
	System    string         `json:"system,omitempty"`
	Duration  time.Duration  `json:"duration,omitempty"`
	Inputs    map[string]any `json:"inputs,omitempty"`
	Outputs   map[string]any `json:"outputs,omitempty"`
	Exception string         `json:"exception,omitempty"`
}

// ExecutionFrame represents a single point in execution history.
type ExecutionFrame struct {
	StepNumber int                      `json:"step_number"`
	Timestamp  time.Time                `json:"timestamp"`
	Location   CodeLocation             `json:"location"`
	StepType   string                   `json:"step_type"` // entry, step_over, step_into, step_return, breakpoint
	Variables  map[string]VariableValue `json:"variables"`
	DBOps      []DBOperation            `json:"db_ops,omitempty"`
	RFCCalls   []RFCCall                `json:"rfc_calls,omitempty"`

	// Delta compression: only store changed variables after first frame
	VariableDelta map[string]VariableValue `json:"variable_delta,omitempty"`
	BaseFrameRef  int                      `json:"base_frame_ref,omitempty"` // Reference to full variable snapshot
}

// ExecutionRecording represents a complete debug session recording.
type ExecutionRecording struct {
	ID            string           `json:"id"`
	SessionID     string           `json:"session_id"`
	StartTime     time.Time        `json:"start_time"`
	EndTime       time.Time        `json:"end_time,omitempty"`
	Program       string           `json:"program"`
	Description   string           `json:"description,omitempty"`
	Frames        []ExecutionFrame `json:"frames"`
	Tags          []string         `json:"tags,omitempty"`
	TotalSteps    int              `json:"total_steps"`
	CurrentStep   int              `json:"current_step"`
	IsComplete    bool             `json:"is_complete"`
	Checkpoints   map[string]int   `json:"checkpoints,omitempty"` // name -> step number
	SnapshotEvery int              `json:"snapshot_every"`        // Full snapshot every N frames (for delta compression)
}

// ExecutionRecorder captures execution frames during debugging.
type ExecutionRecorder struct {
	mu            sync.RWMutex
	recording     *ExecutionRecording
	lastVariables map[string]VariableValue
	snapshotEvery int
}

// NewExecutionRecorder creates a new recorder for a debug session.
func NewExecutionRecorder(sessionID, program string) *ExecutionRecorder {
	return &ExecutionRecorder{
		recording: &ExecutionRecording{
			ID:            generateRecordingID(),
			SessionID:     sessionID,
			StartTime:     time.Now(),
			Program:       program,
			Frames:        make([]ExecutionFrame, 0, 100),
			Checkpoints:   make(map[string]int),
			SnapshotEvery: 10, // Full snapshot every 10 frames by default
		},
		lastVariables: make(map[string]VariableValue),
		snapshotEvery: 10,
	}
}

// generateRecordingID creates a unique recording ID.
func generateRecordingID() string {
	// Include nanoseconds for uniqueness in rapid succession
	return time.Now().Format("20060102-150405.000000000")
}

// RecordFrame adds a new execution frame to the recording.
func (r *ExecutionRecorder) RecordFrame(location CodeLocation, stepType string, variables map[string]VariableValue) {
	r.mu.Lock()
	defer r.mu.Unlock()

	stepNum := len(r.recording.Frames) + 1
	frame := ExecutionFrame{
		StepNumber: stepNum,
		Timestamp:  time.Now(),
		Location:   location,
		StepType:   stepType,
	}

	// Apply delta compression
	if stepNum == 1 || stepNum%r.snapshotEvery == 0 {
		// Full snapshot
		frame.Variables = variables
		frame.BaseFrameRef = 0 // Self-referencing means full snapshot
	} else {
		// Delta only - store only changed variables
		frame.VariableDelta = r.computeDelta(variables)
		// Find the last full snapshot
		for i := stepNum - 1; i >= 1; i-- {
			if i == 1 || i%r.snapshotEvery == 0 {
				frame.BaseFrameRef = i
				break
			}
		}
	}

	// Mark changed variables
	for name, val := range variables {
		if lastVal, exists := r.lastVariables[name]; exists {
			if !variablesEqual(lastVal, val) {
				val.IsChanged = true
				variables[name] = val
			}
		}
	}

	r.lastVariables = variables
	r.recording.Frames = append(r.recording.Frames, frame)
	r.recording.TotalSteps = stepNum
	r.recording.CurrentStep = stepNum
}

// computeDelta returns only variables that changed since last frame.
func (r *ExecutionRecorder) computeDelta(current map[string]VariableValue) map[string]VariableValue {
	delta := make(map[string]VariableValue)

	for name, val := range current {
		if lastVal, exists := r.lastVariables[name]; !exists || !variablesEqual(lastVal, val) {
			val.IsChanged = true
			delta[name] = val
		}
	}

	return delta
}

// variablesEqual compares two variable values for equality.
func variablesEqual(a, b VariableValue) bool {
	// Compare by JSON serialization for deep equality
	aJSON, _ := json.Marshal(a.Value)
	bJSON, _ := json.Marshal(b.Value)
	return string(aJSON) == string(bJSON)
}

// AddDBOperation adds a database operation to the current frame.
func (r *ExecutionRecorder) AddDBOperation(op DBOperation) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if len(r.recording.Frames) > 0 {
		lastFrame := &r.recording.Frames[len(r.recording.Frames)-1]
		lastFrame.DBOps = append(lastFrame.DBOps, op)
	}
}

// AddRFCCall adds an RFC call to the current frame.
func (r *ExecutionRecorder) AddRFCCall(call RFCCall) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if len(r.recording.Frames) > 0 {
		lastFrame := &r.recording.Frames[len(r.recording.Frames)-1]
		lastFrame.RFCCalls = append(lastFrame.RFCCalls, call)
	}
}

// AddCheckpoint marks a named checkpoint at the current step.
func (r *ExecutionRecorder) AddCheckpoint(name string) {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.recording.Checkpoints[name] = r.recording.CurrentStep
}

// GetFrame returns the frame at a specific step number.
func (r *ExecutionRecorder) GetFrame(stepNumber int) *ExecutionFrame {
	r.mu.RLock()
	defer r.mu.RUnlock()

	if stepNumber < 1 || stepNumber > len(r.recording.Frames) {
		return nil
	}

	return &r.recording.Frames[stepNumber-1]
}

// GetVariablesAtStep reconstructs full variable state at a specific step.
func (r *ExecutionRecorder) GetVariablesAtStep(stepNumber int) map[string]VariableValue {
	r.mu.RLock()
	defer r.mu.RUnlock()

	if stepNumber < 1 || stepNumber > len(r.recording.Frames) {
		return nil
	}

	frame := r.recording.Frames[stepNumber-1]

	// If this is a full snapshot, return it directly
	if len(frame.Variables) > 0 {
		return frame.Variables
	}

	// Otherwise, reconstruct from base frame + deltas
	baseStep := frame.BaseFrameRef
	if baseStep == 0 {
		baseStep = 1
	}

	// Start with base frame variables
	result := make(map[string]VariableValue)
	baseFrame := r.recording.Frames[baseStep-1]
	maps.Copy(result, baseFrame.Variables)

	// Apply deltas from base+1 to target step
	for i := baseStep; i < stepNumber; i++ {
		f := r.recording.Frames[i]
		maps.Copy(result, f.VariableDelta)
	}

	// Apply final delta
	maps.Copy(result, frame.VariableDelta)

	return result
}

// FindWhenChanged finds the first step where a variable changed to a specific value.
func (r *ExecutionRecorder) FindWhenChanged(variableName string, targetValue any) int {
	r.mu.RLock()
	defer r.mu.RUnlock()

	targetJSON, _ := json.Marshal(targetValue)

	for i, frame := range r.recording.Frames {
		var vars map[string]VariableValue
		if frame.Variables != nil {
			vars = frame.Variables
		} else {
			vars = frame.VariableDelta
		}

		if val, exists := vars[variableName]; exists {
			valJSON, _ := json.Marshal(val.Value)
			if string(valJSON) == string(targetJSON) {
				return i + 1
			}
		}
	}

	return -1
}

// FindChanges returns all steps where a variable changed.
func (r *ExecutionRecorder) FindChanges(variableName string) []int {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var changes []int

	for i, frame := range r.recording.Frames {
		var vars map[string]VariableValue
		if frame.Variables != nil {
			vars = frame.Variables
		} else {
			vars = frame.VariableDelta
		}

		if val, exists := vars[variableName]; exists && val.IsChanged {
			changes = append(changes, i+1)
		}
	}

	return changes
}

// GetRecording returns the complete recording.
func (r *ExecutionRecorder) GetRecording() *ExecutionRecording {
	r.mu.RLock()
	defer r.mu.RUnlock()

	return r.recording
}

// Complete marks the recording as finished.
func (r *ExecutionRecorder) Complete() {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.recording.EndTime = time.Now()
	r.recording.IsComplete = true
}

// ToJSON serializes the recording to JSON.
func (r *ExecutionRecorder) ToJSON() ([]byte, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	return json.Marshal(r.recording)
}

// FromJSON deserializes a recording from JSON.
func FromJSON(data []byte) (*ExecutionRecording, error) {
	var recording ExecutionRecording
	if err := json.Unmarshal(data, &recording); err != nil {
		return nil, err
	}
	return &recording, nil
}

// Stats returns statistics about the recording.
func (r *ExecutionRecorder) Stats() map[string]any {
	r.mu.RLock()
	defer r.mu.RUnlock()

	totalVars := 0
	totalDBOps := 0
	totalRFCCalls := 0
	uniqueLocations := make(map[string]bool)

	for _, frame := range r.recording.Frames {
		if frame.Variables != nil {
			totalVars += len(frame.Variables)
		}
		totalVars += len(frame.VariableDelta)
		totalDBOps += len(frame.DBOps)
		totalRFCCalls += len(frame.RFCCalls)

		locKey := frame.Location.Program + ":" + frame.Location.Include
		uniqueLocations[locKey] = true
	}

	return map[string]any{
		"total_steps":      r.recording.TotalSteps,
		"unique_locations": len(uniqueLocations),
		"total_variables":  totalVars,
		"total_db_ops":     totalDBOps,
		"total_rfc_calls":  totalRFCCalls,
		"checkpoints":      len(r.recording.Checkpoints),
		"is_complete":      r.recording.IsComplete,
		"duration_seconds": r.recording.EndTime.Sub(r.recording.StartTime).Seconds(),
	}
}
