package vsp

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"time"

	"github.com/oisee/open-rfc-go/rfc"
	"github.com/oisee/vibing-steampunk/pkg/saprfc"
	"github.com/spf13/cobra"
)

// `vsp trace unit` is the whole recording in one command: anchor, catch, walk,
// write, clean up. The REPL can do the same by hand, but a recording is a thing
// you want to repeat identically, and a command is repeatable in a way a
// sequence of prompts is not.

var traceUnitCmd = &cobra.Command{
	Use:   "unit <OBJECT>",
	Short: "Record a unit statement by statement, with its variables",
	Long: `Walk one code unit with the debugger and record every stop.

This is the expensive half of "what really ran". 'vsp trace run' gives the call
tree for almost nothing and carries no data; this reads the variables at every
statement, at about one round trip each. It is a deliberate mode.

  vsp trace unit ZADT_DEBUG_LOOP --line 9 --wait 120

places an anchor breakpoint, waits for somebody to run the unit, then steps over
its statements — never descending into what it calls — and writes one JSON
object per stop. Values are redacted unless --values is given, because a capture
at a code boundary is business data by construction.

The workload has to come from elsewhere: a session cannot catch itself.`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		object := args[0]
		line, _ := cmd.Flags().GetInt("line")
		wait, _ := cmd.Flags().GetInt("wait")
		max, _ := cmd.Flags().GetInt("max-stops")
		values, _ := cmd.Flags().GetBool("values")
		out, _ := cmd.Flags().GetString("out")
		user, _ := cmd.Flags().GetString("user")
		call, _ := cmd.Flags().GetBool("call")

		// --call runs the object; waiting for someone else to run it does not.
		if call {
			if err := rfcWriteGate(cmd, "TraceCall"); err != nil {
				return err
			}
		}

		writer := os.Stdout
		if out != "" {
			f, err := os.Create(out)
			if err != nil {
				return err
			}
			defer func() { _ = f.Close() }()
			writer = f
		}

		return withRFCDestTimeout(cmd, time.Duration(wait+120)*time.Second,
			func(ctx context.Context, c *rfc.Client, dest saprfc.Params) error {
				who := user
				if who == "" {
					who = dest.User
				}
				dbg, err := saprfc.NewDebugger(ctx, c, who)
				if err != nil {
					return err
				}
				defer func() { _ = dbg.Close(ctx) }()

				bps, err := dbg.ADTAddBreakpoint(ctx, object, line, "")
				if err != nil {
					return fmt.Errorf("anchoring at %s:%d: %w", object, line, err)
				}
				for _, r := range dbg.Rejected() {
					fmt.Fprintf(os.Stderr, "! not placed: %s:%d — %s\n", object, line, r.ErrorMessage)
				}
				if len(bps) == 0 {
					return fmt.Errorf("no anchor was placed: %s:%d carries no statement to stop at", object, line)
				}
				fmt.Fprintf(os.Stderr, "anchored at %s:%d; waiting up to %ds for it to run\n", object, line, wait)

				var fireErr callOutcome
				if call {
					go func() {
						time.Sleep(2 * time.Second)
						fired, ferr := saprfc.Open(ctx, dest)
						if ferr != nil {
							fmt.Fprintln(os.Stderr, "! --call could not connect:", ferr)
							fireErr.fail(ferr)
							return
						}
						defer func() { _ = fired.Close(ctx) }()
						if _, ferr = fired.Call(ctx, object, rfc.Params{}); ferr != nil {
							fmt.Fprintln(os.Stderr, "! --call:", ferr)
							fireErr.fail(ferr)
						}
					}()
				}

				caught, _, err := dbg.ADTCatch(ctx, who, saprfc.IDEID, saprfc.TerminalID, wait)
				if err != nil {
					return err
				}
				if caught == nil {
					// Exit zero and "nobody ran it" is the recording saying it
					// found nothing to record. When our own --call is what
					// failed, that is not a finding, and a script reading the
					// exit code has no way to tell the two apart.
					if note := fireErr.note(); note != "" {
						return fmt.Errorf("no execution was caught%s", note)
					}
					fmt.Fprintln(os.Stderr, "nobody ran it")
					return nil
				}
				fmt.Fprintf(os.Stderr, "recording %s (%s) from %s/%s:%d\n",
					caught.ID, caught.User, caught.Program, caught.Include, caught.Line)

				enc := json.NewEncoder(writer)
				n, rerr := dbg.Record(ctx, saprfc.RecordOptions{MaxStops: max, Redact: !values},
					func(r saprfc.StopRecord) error { return enc.Encode(r) })
				fmt.Fprintf(os.Stderr, "%d stops recorded\n", n)
				return rerr
			})
	},
}

func init() {
	traceUnitCmd.Flags().Int("line", 1, "Line to anchor the breakpoint at")
	traceUnitCmd.Flags().Int("wait", 120, "Seconds to wait for the unit to run")
	traceUnitCmd.Flags().Int("max-stops", 2000, "Stop recording after this many stops")
	traceUnitCmd.Flags().Bool("values", false, "Record real values instead of «type:length» placeholders")
	traceUnitCmd.Flags().String("out", "", "Write the JSONL here instead of stdout")
	traceUnitCmd.Flags().String("user", "", "Whose execution to record (default: the logon user)")
	traceUnitCmd.Flags().Bool("call", false, "Call the object over RFC once anchored (function modules only)")
	traceCmd.AddCommand(traceUnitCmd)
}
