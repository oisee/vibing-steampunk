package vsp

import (
	"testing"

	"github.com/oisee/vibing-steampunk/pkg/adt"
)

// A setup step that failed (create, lock, write, activate) leaves no Failure,
// because nothing ran to fail; it must still not exit 0.
func TestExecuteExitErrorWhenNothingRan(t *testing.T) {
	cases := map[string]struct {
		result  *adt.ExecuteABAPResult
		wantErr bool
	}{
		"create failed":     {&adt.ExecuteABAPResult{Message: "Failed to create temp program: object left in place"}, true},
		"ran and passed":    {&adt.ExecuteABAPResult{Success: true, CleanedUp: true, Output: []string{"1"}}, false},
		"ran, no output":    {&adt.ExecuteABAPResult{Success: true, CleanedUp: true}, false},
		"ran, program left": {&adt.ExecuteABAPResult{Success: true, ProgramName: "ZTEMP_EXEC_12345678"}, true},
		"did not finish":    {&adt.ExecuteABAPResult{Failure: &adt.ExecuteFailure{Title: "zero divide"}}, true},
	}
	for name, tc := range cases {
		err := executeExitError(tc.result, nil)
		if (err != nil) != tc.wantErr {
			t.Errorf("%s: got %v, want error=%v", name, err, tc.wantErr)
		}
	}
}
