package mcp

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/mark3labs/mcp-go/mcp"
)

// A step that goes somewhere is refused before anything reaches SAP when it
// isn't told where. SAP would refuse it too, but as "Parameter uri could not be
// found", which reads like a server fault rather than a missing argument.
func TestDebuggerStep_TargetStepsNeedURI(t *testing.T) {
	for _, step := range []string{"stepRunToLine", "stepJumpToLine"} {
		t.Run(step, func(t *testing.T) {
			s, hits := reportTestServer(t, false)
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()

			req := mcp.CallToolRequest{}
			req.Params.Arguments = map[string]any{"step_type": step, "uri": "  "}
			res, err := s.handleDebuggerStep(ctx, req)
			if err != nil {
				t.Fatalf("handleDebuggerStep: %v", err)
			}
			if !res.IsError || !strings.Contains(toolResultText(t, res), "needs uri") {
				t.Fatalf("want a 'needs uri' refusal, got %q", toolResultText(t, res))
			}
			if n := hits(); n != 0 {
				t.Errorf("%s without a target still reached SAP %d time(s)", step, n)
			}
		})
	}
}

// After a continue lets the program finish, SAP's answer names why there is
// nothing left to step. Those two answers mean the step worked; anything else
// is still a failure.
func TestDebuggeeGone(t *testing.T) {
	for _, tc := range []struct {
		err  error
		want bool
	}{
		{nil, false},
		{errors.New(`stepContinue: ADT 500 Internal Server Error: Debuggee ended (debuggeeEnded)`), true},
		{errors.New(`ADT API error: status 500 at /sap/bc/adt/debugger: <exc:exception><properties><entry key="com.sap.adt.communicationFramework.subType">noSessionAttached</entry></properties></exc:exception>`), true},
		{errors.New(`stepContinue: ADT 500 Internal Server Error: An exception was raised (AdiFailed)`), false},
		{errors.New(`stepOver: ADT 403 Forbidden: no authorization`), false},
	} {
		if got := debuggeeGone(tc.err); got != tc.want {
			t.Errorf("debuggeeGone(%v) = %v, want %v", tc.err, got, tc.want)
		}
	}
}
