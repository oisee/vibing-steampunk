package adt

import (
	"testing"
)

func TestDebugVariable_IsComplexType(t *testing.T) {
	tests := []struct {
		metaType DebugMetaType
		expected bool
	}{
		{DebugMetaTypeSimple, false},
		{DebugMetaTypeString, false},
		{DebugMetaTypeStructure, true},
		{DebugMetaTypeTable, true},
		{DebugMetaTypeDataRef, true},
		{DebugMetaTypeObjectRef, true},
		{DebugMetaTypeClass, true},
		{DebugMetaTypeObject, true},
		{DebugMetaTypeBoxRef, true},
		{DebugMetaTypeBoxedComp, false},
		{DebugMetaTypeAnonymComp, false},
		{DebugMetaTypeUnknown, false},
	}

	for _, tt := range tests {
		v := DebugVariable{MetaType: tt.metaType}
		result := v.IsComplexType()
		if result != tt.expected {
			t.Errorf("IsComplexType() for %s: expected %v, got %v", tt.metaType, tt.expected, result)
		}
	}
}

func TestDebugStepTypes(t *testing.T) {
	// Verify step type constants
	tests := []struct {
		stepType DebugStepType
		expected string
	}{
		{DebugStepInto, "stepInto"},
		{DebugStepOver, "stepOver"},
		{DebugStepReturn, "stepReturn"},
		{DebugStepContinue, "stepContinue"},
		{DebugStepRunToLine, "stepRunToLine"},
		{DebugStepJumpToLine, "stepJumpToLine"},
		{DebugTerminate, "terminateDebuggee"},
	}

	for _, tt := range tests {
		if string(tt.stepType) != tt.expected {
			t.Errorf("step type %v: expected '%s', got '%s'", tt.stepType, tt.expected, string(tt.stepType))
		}
	}
}
