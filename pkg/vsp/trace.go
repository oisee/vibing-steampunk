package vsp

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/oisee/open-rfc-go/rfc"
	"github.com/oisee/vibing-steampunk/pkg/saprfc"
	"github.com/spf13/cobra"
)

// `vsp trace` reads SAT runtime traces: what a program actually called, as
// opposed to what a static graph predicts it might.
//
// Everything here goes over the classic-RFC tunnel by default, and the same
// operations work unchanged over HTTPS — the trace API is ADT REST, and a trace
// needs no session at all, so the pooled connection is enough.

var traceCmd = &cobra.Command{
	Use:   "trace",
	Short: "SAT runtime traces: the measured call tree",
	Long: `Read and arm ABAP runtime traces (SAT) through SAP's own ADT resources.

A static call graph cannot know what CALL FUNCTION lv_name resolved to, which
PERFORM (f) ran, or which destination an RFC went to. A trace is the evidence.

  vsp trace list                       traces this system holds
  vsp trace tree <ID>                  the measured call tree of one trace
  vsp trace run ZADT_DEBUG_LOOP        arm a trace for that object, wait, print it

Nothing is installed on the server for any of this.`,
}

var traceListCmd = &cobra.Command{
	Use:   "list",
	Short: "List the trace files this system holds",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		return withTracer(cmd, func(ctx context.Context, t *saprfc.Tracer) error {
			traces, err := t.Traces(ctx)
			if err != nil {
				return err
			}
			if asJSON(cmd) {
				return json.NewEncoder(os.Stdout).Encode(traces)
			}
			if len(traces) == 0 {
				fmt.Println("no traces on this system")
				return nil
			}
			fmt.Printf("%-34s %-18s %-9s %8s  %s\n", "ID", "OBJECT", "SHAPE", "RUNTIME", "TITLE")
			for _, f := range traces {
				shape := "tree"
				if f.Aggregated {
					shape = "aggregated"
				}
				object := f.ObjectName
				if object == "" {
					object = "-"
				}
				fmt.Printf("%-34s %-18s %-9s %7.1fms  %s\n",
					f.ID, object, shape, float64(f.Runtime)/1000, f.Title)
			}
			return nil
		})
	},
}

var traceTreeCmd = &cobra.Command{
	Use:   "tree <TRACE-ID>",
	Short: "Print the measured call tree of a trace",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		return withTracer(cmd, func(ctx context.Context, t *saprfc.Tracer) error {
			stmts, err := t.Tree(ctx, args[0])
			if err != nil {
				return err
			}
			return renderTree(cmd, stmts)
		})
	},
}

var traceRunCmd = &cobra.Command{
	Use:   "run <OBJECT>",
	Short: "Arm a trace for one object, wait for it to fire, and print the tree",
	Long: `Arm a SAT trace for one object and wait.

The workload has to run somewhere else — that is the point: a trace watches real
execution, it does not create it. Start this, then call the function module,
start the transaction, or let the job run. With --call, vsp fires an RFC-enabled
function module itself once the trace is armed, which makes the loop
self-contained for a test.

Naming the object matters. A trace armed for "any object" is armed for vsp's own
session too, and vsp is usually the next thing to talk to the system.`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		object := strings.ToUpper(args[0])
		objType, _ := cmd.Flags().GetString("type")
		procType, _ := cmd.Flags().GetString("process")
		wait, _ := cmd.Flags().GetInt("wait")
		call, _ := cmd.Flags().GetBool("call")
		keep, _ := cmd.Flags().GetBool("keep")

		// --call runs the object; arming and reading a trace do not.
		if call {
			if err := rfcWriteGate(cmd, "TraceCall"); err != nil {
				return err
			}
		}

		return withRFCDest(cmd, func(ctx context.Context, c *rfc.Client, dest saprfc.Params) error {
			t := saprfc.NewTracer(saprfc.RFCTunnel(c), dest.User, dest.Client)

			before, err := t.Traces(ctx)
			if err != nil {
				return err
			}
			seen := map[string]bool{}
			for _, f := range before {
				seen[f.ID] = true
			}

			params, err := t.NewParameters(ctx, saprfc.CallTreeParams("vsp "+object))
			if err != nil {
				return fmt.Errorf("registering trace parameters: %w", err)
			}
			reqID, err := t.Arm(ctx, saprfc.TraceRequest{
				Description:  "vsp " + object,
				ObjectType:   saprfc.TraceObjectType(objType),
				ObjectName:   object,
				ProcessType:  saprfc.TraceProcessType(procType),
				MaxRuns:      1,
				ParametersID: params,
			})
			if err != nil {
				return fmt.Errorf("arming the trace: %w", err)
			}
			fmt.Fprintf(os.Stderr, "armed for %s (%s, %s); waiting up to %ds\n", object, objType, procType, wait)

			var fireErr callOutcome
			if call {
				// A separate connection, because a session cannot trace itself
				// into existence: the request is consumed by the session that
				// runs the object.
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
						fmt.Fprintln(os.Stderr, "! --call failed:", ferr)
						fireErr.fail(ferr)
					}
				}()
			}

			deadline := time.Now().Add(time.Duration(wait) * time.Second)
			for {
				traces, terr := t.Traces(ctx)
				if terr != nil {
					return terr
				}
				for _, f := range traces {
					if seen[f.ID] {
						continue
					}
					fmt.Fprintf(os.Stderr, "trace %s — %s, %.1fms\n", f.ID, f.Title, float64(f.Runtime)/1000)
					stmts, serr := t.Tree(ctx, f.ID)
					if serr != nil {
						return serr
					}
					return renderTree(cmd, stmts)
				}
				if time.Now().After(deadline) {
					if !keep {
						_ = t.DeleteRequest(ctx, reqID)
					}
					// "Nothing ran it" is true and misleading when the thing
					// that did not run it was the --call we launched: the
					// reader concludes the object is dead code and stops
					// looking, having never been told the call errored.
					return fmt.Errorf("nothing ran %s within %ds — the trace request is %s%s",
						object, wait, disarmed(keep), fireErr.note())
				}
				time.Sleep(2 * time.Second)
			}
		})
	},
}

// callOutcome carries what became of the --call goroutine back to the code that
// decides what the wait meant.
//
// The goroutine fires the object on a second connection and cannot return an
// error to anyone, so its failures went to stderr and nowhere else. The command
// then ended by saying nothing ran the object — an empty result standing in for
// a failure it had already seen.
type callOutcome struct {
	mu  sync.Mutex
	err error
}

func (c *callOutcome) fail(err error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.err = err
}

// note is the clause to append to a "nothing happened" verdict, or nothing at
// all when the call did go out.
func (c *callOutcome) note() string {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.err == nil {
		return ""
	}
	return " — and --call never reached it: " + c.err.Error()
}

func disarmed(keep bool) string {
	if keep {
		return "still armed (--keep)"
	}
	return "disarmed again"
}

var traceRequestsCmd = &cobra.Command{
	Use:   "requests",
	Short: "List armed trace requests, or disarm one with --delete",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		return withTracer(cmd, func(ctx context.Context, t *saprfc.Tracer) error {
			if id, _ := cmd.Flags().GetString("delete"); id != "" {
				if err := t.DeleteRequest(ctx, id); err != nil {
					return err
				}
				fmt.Println("disarmed", id)
				return nil
			}
			ids, err := t.Requests(ctx)
			if err != nil {
				return err
			}
			if len(ids) == 0 {
				fmt.Println("no trace requests are armed")
				return nil
			}
			for _, id := range ids {
				fmt.Println(id)
			}
			return nil
		})
	},
}

var traceRmCmd = &cobra.Command{
	Use:   "rm <TRACE-ID>",
	Short: "Delete a trace file",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		return withTracer(cmd, func(ctx context.Context, t *saprfc.Tracer) error {
			if err := t.DeleteTrace(ctx, args[0]); err != nil {
				return err
			}
			fmt.Println("deleted", args[0])
			return nil
		})
	},
}

// withTracer opens a pooled RFC connection and hands over a tracer on it.
func withTracer(cmd *cobra.Command, fn func(context.Context, *saprfc.Tracer) error) error {
	return withRFCDest(cmd, func(ctx context.Context, c *rfc.Client, dest saprfc.Params) error {
		return fn(ctx, saprfc.NewTracer(saprfc.RFCTunnel(c), dest.User, dest.Client))
	})
}

func asJSON(cmd *cobra.Command) bool {
	v, _ := cmd.Flags().GetBool("json")
	return v
}

// renderTree prints the tree, or the JSONL stream the offline tools read.
func renderTree(cmd *cobra.Command, stmts []saprfc.TraceStatement) error {
	if asJSON(cmd) {
		enc := json.NewEncoder(os.Stdout)
		for _, s := range stmts {
			if err := enc.Encode(s); err != nil {
				return err
			}
		}
		return nil
	}
	callsOnly, _ := cmd.Flags().GetBool("calls")
	fmt.Print(saprfc.FormatTree(stmts, callsOnly))
	return nil
}

func init() {
	for _, c := range []*cobra.Command{traceListCmd, traceTreeCmd, traceRunCmd} {
		c.Flags().Bool("json", false, "Emit JSON (one object per statement for a tree)")
	}
	for _, c := range []*cobra.Command{traceTreeCmd, traceRunCmd} {
		c.Flags().Bool("calls", false, "Only the statements that hand control to another unit")
	}
	traceRunCmd.Flags().String("type", "functionmodule", "Object type: functionmodule, report, transaction, url, any")
	traceRunCmd.Flags().String("process", "any", "Process type: any, dialog, batch, rfc, http")
	traceRunCmd.Flags().Int("wait", 120, "Seconds to wait for the object to run")
	traceRunCmd.Flags().Bool("call", false, "Call the object over RFC once armed (function modules only)")
	traceRunCmd.Flags().Bool("keep", false, "Leave the request armed if nothing ran in time")
	traceRequestsCmd.Flags().String("delete", "", "Disarm this request id")

	traceCmd.AddCommand(traceListCmd, traceTreeCmd, traceRunCmd, traceRequestsCmd, traceRmCmd)
	rootCmd.AddCommand(traceCmd)
}
