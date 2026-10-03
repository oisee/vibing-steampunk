package vsp

import (
	"strings"
	"testing"
)

// Two ways of giving the snippet at once would check one and silently ignore
// the other, so the command refuses before it reads anything.
func TestCheckABAPRefusesMoreThanOneSource(t *testing.T) {
	cases := map[string]struct {
		flags map[string]string
		args  []string
	}{
		"argument and file":  {flags: map[string]string{"file": "x.abap"}, args: []string{"WRITE 1."}},
		"stdin and file":     {flags: map[string]string{"stdin": "true", "file": "x.abap"}},
		"stdin and argument": {flags: map[string]string{"stdin": "true"}, args: []string{"WRITE 1."}},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			restoreCommandFlags(t)
			for k, v := range tc.flags {
				if err := checkABAPCmd.Flags().Set(k, v); err != nil {
					t.Fatal(err)
				}
			}
			err := runCheckABAP(checkABAPCmd, tc.args)
			if err == nil || !strings.Contains(err.Error(), "one way only") {
				t.Fatalf("want a one-source refusal, got %v", err)
			}
		})
	}
}
