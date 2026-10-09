package vsp

// `vsp effects` — what a unit does to the world, and to its caller's transaction.
//
// The analysis has existed since April and nothing could invoke it. This is the
// command half; the MCP half is analyze type=effects.

import (
	"fmt"
	"os"
	"strings"

	"github.com/oisee/vibing-steampunk/pkg/graph"
	"github.com/spf13/cobra"
)

func init() {
	effectsCmd.Flags().String("type", "CLAS", "Object type: CLAS, PROG, INTF, FUGR")
	effectsCmd.Flags().String("file", "", "Read the source from a file instead of from SAP")
	effectsCmd.Flags().Bool("json", false, "Emit JSON")
	rootCmd.AddCommand(effectsCmd)
}

var effectsCmd = &cobra.Command{
	Use:   "effects <object>",
	Short: "What a unit does to the world, and to its caller's transaction",
	Long: `Report the side effects of an ABAP unit, and classify its LUW behaviour.

The interesting effects in ABAP are not database writes. They are LUW effects,
because ABAP lets a unit defer a write so that it lands inside somebody else's
transaction: a method calling CALL FUNCTION ... IN UPDATE TASK has not written
anything yet, and whoever calls COMMIT WORK higher up triggers every deferred
write it queued. That is invisible coupling, and nothing in SAP's toolchain
reports it.

  safe         neither commits nor defers — leaves the caller's transaction intact
  participant  defers work that the caller's COMMIT will run
  owner        contains COMMIT WORK — ends its caller's transaction
  unsafe       both, so part of what it queues is committed by each

The analysis is local: it reads the unit's own source and nothing it calls, and
the answer says so rather than leaving it to be assumed.

  vsp -s dev effects ZCL_DEMO_ORDER
  vsp -s dev effects ZDEMO_REPORT --type PROG
  vsp effects --file ./local.abap        # no system needed`,
	Args: cobra.MaximumNArgs(1),
	RunE: runEffects,
}

func runEffects(cmd *cobra.Command, args []string) error {
	asJSON, _ := cmd.Flags().GetBool("json")
	file, _ := cmd.Flags().GetString("file")
	objType, _ := cmd.Flags().GetString("type")

	var source, name string
	switch {
	case file != "":
		blob, err := os.ReadFile(file)
		if err != nil {
			return err
		}
		source, name = string(blob), file
	case len(args) == 1:
		params, err := resolveSystemParams(cmd)
		if err != nil {
			return err
		}
		client, err := getClient(params)
		if err != nil {
			return err
		}
		name = strings.ToUpper(args[0])
		source, err = client.GetSource(cmd.Context(), strings.ToUpper(objType), name, nil)
		if err != nil {
			return fmt.Errorf("reading %s %s: %w", objType, name, err)
		}
	default:
		return fmt.Errorf("name an object, or pass --file")
	}

	e := graph.ExtractEffects(source)
	if asJSON {
		return emitJSON(effectsReport(name, source, e))
	}
	fmt.Print(effectsText(name, e))
	return nil
}

// effectsReport is the JSON shape, kept beside the text one so they cannot
// describe different things.
func effectsReport(name, source string, e *graph.EffectInfo) map[string]any {
	class := e.ClassifyLUW()
	return map[string]any{
		"object":          name,
		"lines":           strings.Count(source, "\n") + 1,
		"luw":             class,
		"consequence":     luwConsequenceText(class),
		"pure":            e.IsPure(),
		"readsTables":     e.ReadsDB,
		"writesTables":    e.WritesDB,
		"rfcDestinations": e.SyncRFC,
		"note":            "local analysis: effects inside anything this unit calls are not counted",
	}
}

func luwConsequenceText(class string) string {
	switch class {
	case "safe":
		return "neither commits nor registers deferred work — the caller's transaction is left intact"
	case "participant":
		return "registers work that runs when somebody else commits — its writes land in the caller's transaction"
	case "owner":
		return "contains COMMIT WORK — ends its caller's transaction, so every caller above loses atomicity"
	case "unsafe":
		return "both commits and registers deferred work — part of what it queues is committed by each"
	}
	return "unclassified"
}

func effectsText(name string, e *graph.EffectInfo) string {
	var b strings.Builder
	class := e.ClassifyLUW()
	fmt.Fprintf(&b, "%s\n\n", name)
	fmt.Fprintf(&b, "  LUW          %s\n", class)
	fmt.Fprintf(&b, "               %s\n", luwConsequenceText(class))
	if e.IsPure() {
		b.WriteString("  pure         no effect detected in this source\n")
	}
	writeList(&b, "reads", e.ReadsDB)
	writeList(&b, "writes", e.WritesDB)
	writeList(&b, "RFC dest.", e.SyncRFC)

	var flags []string
	for _, f := range []struct {
		on bool
		s  string
	}{
		{e.HasCommit, "COMMIT WORK"}, {e.HasRollback, "ROLLBACK WORK"},
		{e.UpdateTask, "IN UPDATE TASK"}, {e.BackgroundTask, "IN BACKGROUND TASK"},
		{e.UpdateTaskLocal, "SET UPDATE TASK LOCAL"}, {e.AsyncRFC, "STARTING NEW TASK"},
		{e.BackgroundJob, "SUBMIT VIA JOB"}, {e.SubmitAndReturn, "SUBMIT AND RETURN"},
		{e.HTTPCall, "HTTP client"}, {e.APCPush, "APC push"},
		{e.WritesState, "writes state"}, {e.RaisesExc, "raises exception"},
		{e.RaisesMessage, "MESSAGE E/A/X"}, {e.LeavesContext, "leaves context"},
	} {
		if f.on {
			flags = append(flags, f.s)
		}
	}
	writeList(&b, "effects", flags)

	// The limit travels with the answer. A reader who takes a local "safe" for
	// a transitive one has been misled by the report, not by the code.
	b.WriteString("\n  Local analysis: an effect inside anything this unit calls is not counted here.\n")
	return b.String()
}

func writeList(b *strings.Builder, label string, items []string) {
	if len(items) == 0 {
		return
	}
	fmt.Fprintf(b, "  %-12s %s\n", label, strings.Join(items, ", "))
}
