package vsp

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/oisee/vibing-steampunk/pkg/adt"
	"github.com/spf13/cobra"
)

func init() {
	compatCmd.Flags().Bool("full", false, "Ask every check, not only the ones that decide routing")
	compatCmd.Flags().Bool("json", false, "Emit the report as JSON, for diffing or for a record")
	compatCmd.Flags().Bool("routes", true, "Print the routing advice derived from the answers")
	compatCmd.Flags().String("group", "", "Function group to probe with (default: found automatically)")
	compatCmd.Flags().String("class", "", "Class to probe with (default: found automatically)")
	compatCmd.Flags().String("program", "", "Program to probe with (default: found automatically)")
	compatCmd.Flags().String("package", "$TMP", "Package to probe with")
	compatCmd.Flags().String("against", "", "Second system to compare with, by name from the config")
	rootCmd.AddCommand(compatCmd)
}

var compatCmd = &cobra.Command{
	Use:   "compat",
	Short: "Ask a system what it supports, and how to route each capability",
	Long: `Probe the ADT surface of a system and report what answers, what is absent,
and which content types it will accept.

Two SAP releases answer the same request differently in ways nothing documents:
a resource present on one is missing on the other, and a content type accepted
by one is refused by the other. Those differences decide which route a capability
has to take, and finding them by hand costs an afternoon each time.

  vsp -s dev compat                  # quick: what decides routing, in seconds
  vsp -s dev compat --full           # the whole surface
  vsp -s dev compat --against prod   # what the two systems disagree about
  vsp -s dev compat --full --json    # a record to keep or to diff later`,
	RunE: runCompat,
}

func runCompat(cmd *cobra.Command, args []string) error {
	full, _ := cmd.Flags().GetBool("full")
	asJSON, _ := cmd.Flags().GetBool("json")
	showRoutes, _ := cmd.Flags().GetBool("routes")
	against, _ := cmd.Flags().GetString("against")

	depth := adt.CompatQuick
	if full {
		depth = adt.CompatFull
	}

	params, err := resolveSystemParams(cmd)
	if err != nil {
		return err
	}
	report, err := probeSystem(cmd, params, depth)
	if err != nil {
		return err
	}

	if against == "" {
		if asJSON {
			return emitJSON(report)
		}
		fmt.Print(report.Text())
		if showRoutes {
			fmt.Printf("\nRouting\n\n%s", report.RoutingText())
		}
		return nil
	}

	// Comparing needs the other system resolved the same way this one was.
	otherParams, err := resolveNamedSystem(cmd, against)
	if err != nil {
		return err
	}
	otherReport, err := probeSystem(cmd, otherParams, depth)
	if err != nil {
		return err
	}

	if asJSON {
		return emitJSON(map[string]any{"a": report, "b": otherReport})
	}
	fmt.Printf("Differences\n\n%s", adt.DiffCompatReports(report, otherReport))
	if showRoutes {
		fmt.Printf("\nRouting\n\n%s", adt.DiffRoutes(report, otherReport))
	}
	return nil
}

// probeSystem runs the probe against one system, finding objects to probe with
// when the caller did not name any.
func probeSystem(cmd *cobra.Command, params *systemParams, depth adt.CompatDepth) (*adt.CompatReport, error) {
	client, err := getClient(params)
	if err != nil {
		return nil, err
	}

	targets := adt.CompatTargets{}
	targets.FunctionGroup, _ = cmd.Flags().GetString("group")
	targets.Class, _ = cmd.Flags().GetString("class")
	targets.Program, _ = cmd.Flags().GetString("program")
	targets.Package, _ = cmd.Flags().GetString("package")
	missed := fillMissingTargets(cmd, client, &targets)

	// The report will say "skipped: no object of the required kind was found to
	// probe with", which is a statement about the system. When the search is
	// what failed it is a statement about us, and the difference decides
	// whether a reader goes to basis or reruns with --class. On stderr because
	// the report itself may be JSON on stdout.
	if note := adt.UnsearchedNote(missed, 4, "object kind"); note != "" {
		fmt.Fprintln(os.Stderr, note)
	}

	fmt.Fprintf(os.Stderr, "probing %s...\n", params.Name)
	return client.RunCompatProbe(cmd.Context(), targets, depth), nil
}

// fillMissingTargets finds one object of each kind to probe with.
//
// The checks that need a target are the interesting ones, and a probe that
// skipped them because the caller did not know a class name on this system
// would be quiet about exactly what it exists to find out.
func fillMissingTargets(cmd *cobra.Command, client *adt.Client, targets *adt.CompatTargets) []adt.Unsearched {
	// Why a kind came back empty is the part the report cannot work out for
	// itself: a search that returned nothing means the system has none, and a
	// search that failed means nobody looked.
	var missed []adt.Unsearched
	// One search is not enough: a system with hundreds of Z objects returns the
	// first page alphabetically, and the kind being looked for may not be on
	// it. Widen the net before giving up, because a skipped check is a question
	// left unanswered.
	// The type has to match exactly. A prefix match on "PROG" also accepts
	// PROG/I, an include, which does not live under /programs/programs/ — so
	// the probe would read a 404 it caused itself as the system lacking the
	// resource, and say so in a report someone else will act on.
	find := func(objType string, patterns ...string) string {
		var lastErr error
		for _, pattern := range patterns {
			results, err := client.SearchObject(cmd.Context(), pattern, 100)
			if err != nil {
				// Trying the next pattern is right: one search that fails
				// should not decide the kind. Only a kind where every search
				// failed is a gap.
				lastErr = err
				continue
			}
			for _, r := range results {
				if strings.EqualFold(r.Type, objType) {
					return r.Name
				}
			}
			lastErr = nil
		}
		if lastErr != nil {
			missed = append(missed, adt.Unsearched{Object: objType, Reason: lastErr.Error()})
		}
		return ""
	}
	if targets.FunctionGroup == "" {
		targets.FunctionGroup = find("FUGR/F", "Z*", "*")
	}
	if targets.Class == "" {
		targets.Class = find("CLAS/OC", "Z*", "CL_*")
	}
	if targets.Program == "" {
		targets.Program = find("PROG/P", "Z*", "R*")
	}
	// A package the caller named is used as given; otherwise prefer one this
	// system actually has, so that a 404 means the resource is missing rather
	// than the package.
	if found := find("DEVC/K", "Z*", "*"); found != "" && !cmd.Flags().Changed("package") {
		targets.Package = found
	}
	return missed
}

// resolveNamedSystem resolves a second system by name, so --against can be given
// any system from the config rather than only the default.
func resolveNamedSystem(cmd *cobra.Command, name string) (*systemParams, error) {
	previous := systemName
	systemName = name
	defer func() { systemName = previous }()
	return resolveSystemParams(cmd)
}

func emitJSON(v any) error {
	blob, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	fmt.Println(string(blob))
	return nil
}
