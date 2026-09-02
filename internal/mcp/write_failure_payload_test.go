package mcp

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/oisee/vibing-steampunk/pkg/adt"
)

// TestWriteSourceFailureKeepsTheDiagnosis guards the boundary added in #182.
// Failing closed is right — an HTTP 200 carrying success=false must not read as
// success. But the verdict alone is not actionable: the commonest failure is a
// syntax error, and the line, offset and text live in result.SyntaxErrors. An
// agent handed only "Source has syntax errors - not saved" has to guess.
func TestWriteSourceFailureKeepsTheDiagnosis(t *testing.T) {
	result := &adt.WriteSourceResult{
		Success:    false,
		ObjectType: "PROG",
		ObjectName: "ZDEMO_ONE",
		Message:    "Source has syntax errors - not saved",
		SyntaxErrors: []adt.SyntaxCheckResult{
			{Line: 42, Offset: 7, Severity: "E", Text: "Field LV_X is unknown"},
		},
	}

	err := adt.WriteSourceResultError(result)
	if err == nil {
		t.Fatal("success=false must become an error, or the caller reads a failure as a success")
	}

	payload, marshalErr := json.MarshalIndent(result, "", "  ")
	if marshalErr != nil {
		t.Fatalf("marshalling the result: %v", marshalErr)
	}
	text := fmt.Sprintf("%v\n\n%s", err, payload)

	for _, want := range []string{"Source has syntax errors", "Field LV_X is unknown", `"line": 42`} {
		if !strings.Contains(text, want) {
			t.Errorf("the failure text must carry %q so the caller can act on it; got:\n%s", want, text)
		}
	}
}
