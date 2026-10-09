package vsp

// `vsp loads` — what a compiled unit pulls in, and what pulls it in.
//
// The MCP half is analyze type=loads. This is the command half, written because
// a capability reachable from one surface and not the other looks from outside
// exactly like a capability that does not exist — which is the same gap the
// mode-parity fix closed for the whole analyze surface.

import (
	"fmt"
	"os"
	"sort"
	"strings"

	"github.com/oisee/vibing-steampunk/pkg/adt"
	"github.com/oisee/vibing-steampunk/pkg/graph"
	"github.com/oisee/vibing-steampunk/pkg/graph/adtsource"
	"github.com/spf13/cobra"
)

func init() {
	loadsCmd.Flags().String("direction", "loads", "loads (what this pulls in), loaded-by (what pulls this in), or both")
	loadsCmd.Flags().Bool("json", false, "Emit JSON")
	rootCmd.AddCommand(loadsCmd)
}

var loadsCmd = &cobra.Command{
	Use:   "loads <object>",
	Short: "The compile-time load graph: what must be present for this to run",
	Long: `Report what a compiled unit loads, and what loads it, from D010INC.

This is a different relation from every other dependency this tool reports, and
the difference is the reason it is worth asking. The cross-reference tables
record what code *names*. D010INC records what has to be *loaded* for it to run.

The gap between the two is not academic:

  Nothing references an include — an include is included. So an include that
  nothing loads is dead in a way no where-used list will ever show, and one
  that nothing references can still be load-critical.

Three kinds of row live in that table and only one is a dependency between
objects: a unit loading its own parts (a class and its methods) is containment,
<SYSINI> and friends are kernel machinery, and the rest — one object's pool
loaded by another — is what this reports.

  vsp -s dev loads ZCL_DEMO_ORDER
  vsp -s dev loads ZDEMO_GROUP --direction loaded-by
  vsp -s dev loads ZCL_DEMO_ORDER --direction both --json`,
	Args: cobra.ExactArgs(1),
	RunE: runLoads,
}

func runLoads(cmd *cobra.Command, args []string) error {
	asJSON, _ := cmd.Flags().GetBool("json")
	direction := strings.ToLower(strings.TrimSpace(mustFlagString(cmd, "direction")))
	switch direction {
	case "loads", "loaded-by", "both":
	default:
		return fmt.Errorf("direction %q is not one this can answer: \"loads\", \"loaded-by\" or \"both\"", direction)
	}

	params, err := resolveSystemParams(cmd)
	if err != nil {
		return err
	}
	client, err := getClient(params)
	if err != nil {
		return err
	}
	name := strings.ToUpper(args[0])

	report := map[string]any{
		"object":    name,
		"direction": direction,
		"source": "D010INC, the compile-time load table. These are loads, not calls: " +
			"what must be present for this to run, which is not the same as what it names.",
	}
	var gaps []adt.Unsearched

	side := func(rows []adt.LoadRow, g []adt.Unsearched, key string) []loadEdge {
		gaps = append(gaps, g...)
		edges := loadEdgesOf(rows)
		report[key] = edges
		report[key+"_total"] = len(edges)
		if len(rows) > 0 && len(edges) == 0 {
			report[key+"_note"] = "rows exist and none is a dependency between objects: a unit loading " +
				"its own includes is containment, and <SYSINI> is machinery."
		}
		return edges
	}

	var down, up []loadEdge
	if direction == "loads" || direction == "both" {
		rows, g, err := client.Loads(cmd.Context(), name)
		if err != nil {
			return err
		}
		down = side(rows, g, "loads")
	}
	if direction == "loaded-by" || direction == "both" {
		rows, g, err := client.LoadedBy(cmd.Context(), name)
		if err != nil {
			return err
		}
		up = side(rows, g, "loaded_by")
	}

	if len(gaps) > 0 {
		report["unsearched"] = gaps
		report["gap"] = adt.UnsearchedNote(gaps, len(gaps), "table")
	}

	if asJSON {
		return emitJSON(report)
	}

	// The gap goes before the lists, not after: a reader who has seen a
	// plausible list has already drawn the conclusion by the time a footnote
	// tells them it was partial.
	if note := adt.UnsearchedNote(gaps, len(gaps), "table"); note != "" {
		fmt.Fprintf(os.Stderr, "%s\n\n", note)
	}
	fmt.Print(loadsText(name, direction, down, up, report))
	return nil
}

// loadEdge is one load between two objects, with the pool suffix kept.
type loadEdge struct {
	From    string `json:"from"`
	To      string `json:"to"`
	Include string `json:"include"`
	Why     string `json:"why"`
}

// loadEdgesOf runs the rows through the graph builder, which is what decides
// which of the three kinds of row is a dependency.
func loadEdgesOf(rows []adt.LoadRow) []loadEdge {
	converted := adtsource.D010INCRows(rows)

	out := make([]loadEdge, 0)
	for _, e := range graph.BuildD010INCGraph(converted).Edges() {
		include := strings.TrimPrefix(e.RefDetail, "LOADS:")
		out = append(out, loadEdge{
			From:    e.From,
			To:      e.To,
			Include: include,
			Why:     poolKindOf(include),
		})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].To < out[j].To })
	return out
}

// poolKindOf says why a load exists, from the suffix SAP pads the include with.
//
// The meanings are quoted from SAP's own source rather than inferred.
// CL_OO_CLASSNAME_SERVICE carries them as a comment beside the include names it
// builds:
//
//	" CU = public section
//	" CO = protected section
//	" CI = private section
//	" CP = class pool
//	" CT = used internally to store compiler optimizations??
//
// The two question marks are SAP's. So CT is reported as SAP describes it and
// not as something tidier: the first draft of this function called it "a class's
// type declarations", which was a guess dressed as a fact about a suffix whose
// own vendor is unsure. Anything not on that list is passed through as the raw
// suffix, because a reader can look up a code and cannot unlearn a wrong name.
func poolKindOf(include string) string {
	i := strings.LastIndex(include, "=")
	if i < 0 || i+1 >= len(include) {
		return "an include of this unit"
	}
	suffix := include[i+1:]
	switch {
	case suffix == "CP":
		return "the class pool"
	case suffix == "CU":
		return "a class's public section"
	case suffix == "CO":
		return "a class's protected section"
	case suffix == "CI":
		return "a class's private section"
	case suffix == "CT":
		return "internal — SAP's own note says compiler optimizations, with a question mark"
	case strings.HasPrefix(suffix, "CM"):
		return "one method of a class"
	default:
		// IT, IU, IP and the rest: real and undocumented here. Naming them
		// would be inventing.
		return "pool suffix " + suffix
	}
}

func loadsText(name, direction string, down, up []loadEdge, report map[string]any) string {
	var b strings.Builder
	fmt.Fprintf(&b, "Load graph for %s — from D010INC, which records what must be present, not what is named.\n", name)

	// Which end of the edge to name depends on the question. Asking what this
	// loads, the answer is the other end; asking what loads this, the answer is
	// the near end. Printing the same end for both showed the object as its own
	// loader — a wrong name, not merely a useless one.
	section := func(title, key string, edges []loadEdge, near bool) {
		if len(edges) == 0 {
			if note, ok := report[key+"_note"].(string); ok {
				fmt.Fprintf(&b, "\n%s: none.\n  %s\n", title, note)
				return
			}
			fmt.Fprintf(&b, "\n%s: none recorded.\n", title)
			return
		}
		fmt.Fprintf(&b, "\n%s (%d):\n", title, len(edges))
		for _, e := range edges {
			end := e.To
			if near {
				end = e.From
			}
			if i := strings.Index(end, ":"); i > 0 {
				end = end[i+1:]
			}
			fmt.Fprintf(&b, "  %-40s %s\n", end, e.Why)
		}
	}

	if direction == "loads" || direction == "both" {
		section("Loads", "loads", down, false)
	}
	if direction == "loaded-by" || direction == "both" {
		section("Loaded by", "loaded_by", up, true)
	}
	return b.String()
}

// mustFlagString reads a string flag, treating a missing one as empty rather
// than as an error the caller has to handle at every call.
func mustFlagString(cmd *cobra.Command, name string) string {
	v, _ := cmd.Flags().GetString(name)
	return v
}
