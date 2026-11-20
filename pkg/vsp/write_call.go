package vsp

import (
	"context"
	"fmt"
	"io"
	"time"

	"github.com/spf13/cobra"

	"github.com/oisee/vibing-steampunk/pkg/adt"
)

// callTimeoutFlagUsage documents --call-timeout on the CLI commands that write
// and activate source.
const callTimeoutFlagUsage = "Seconds one object's write and activation may take in all (default: SAP_CALL_TIMEOUT); at most 3600. 0 = none: each request to SAP is limited to 60s"

// addCallTimeoutFlag gives the commands that write and activate source the
// --call-timeout the MCP server has. Without it every request, the PUT of a
// large source and its activation among them, is cut off at the client's 60s
// per-request timeout, however long the operator is willing to wait.
func addCallTimeoutFlag(cmds ...*cobra.Command) {
	for _, c := range cmds {
		c.Flags().Int("call-timeout", 0, callTimeoutFlagUsage)
	}
}

// withWriteBudget bounds one write and activation by budget instead of the
// client's per-request timeout, the way a long MCP call is bounded. A zero
// budget leaves ctx as it is: each request keeps its 60s limit.
func withWriteBudget(ctx context.Context, budget time.Duration) (context.Context, context.CancelFunc) {
	if budget <= 0 {
		return ctx, func() {}
	}
	ctx, cancel := context.WithTimeout(ctx, budget)
	return adt.WithCallDeadline(ctx), cancel
}

// printActivationMessages writes what SAP said when it refused an activation,
// at most adt.ActivationMessageLimit messages, so that "activation failed" is
// never the whole story.
func printActivationMessages(w io.Writer, activation *adt.ActivationResult) {
	lines := activation.MessageLines(adt.ActivationMessageLimit)
	if len(lines) == 0 {
		return
	}
	fmt.Fprintf(w, "Activation messages:\n")
	for _, l := range lines {
		fmt.Fprintf(w, "  %s\n", l)
	}
}
