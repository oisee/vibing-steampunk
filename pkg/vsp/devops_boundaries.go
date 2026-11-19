package vsp

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/oisee/vibing-steampunk/pkg/adt"
	"github.com/oisee/vibing-steampunk/pkg/graph"
	"github.com/oisee/vibing-steampunk/pkg/graph/adtsource"
	"github.com/spf13/cobra"
)

func runBoundaries(cmd *cobra.Command, args []string) error {
	params, err := resolveSystemParams(cmd)
	if err != nil {
		return err
	}
	client, err := getClient(params)
	if err != nil {
		return err
	}

	pkg := strings.ToUpper(strings.TrimSpace(args[0]))
	format, _ := cmd.Flags().GetString("format")
	report, _ := cmd.Flags().GetString("report")
	exact, _ := cmd.Flags().GetBool("exact")
	ctx := context.Background()

	// Resolve scope
	scope, err := AcquirePackageScope(ctx, client, pkg, !exact)
	if err != nil {
		return fmt.Errorf("scope resolution failed: %w", err)
	}
	fmt.Fprintf(os.Stderr, "Analyzing boundaries for %s (%d packages in scope)...\n", pkg, len(scope.Packages))

	// Collect objects
	objects, err := AcquirePackageObjects(ctx, client, ScopeToWhere(scope))
	if err != nil {
		return err
	}
	if len(objects) == 0 {
		return fmt.Errorf("package %s is empty or not found", pkg)
	}

	// Build graph
	g := graph.New()
	count := 0
	// An object whose source will not load contributes no edges, and an edge
	// that was never extracted cannot cross a boundary. "No crossings found"
	// is then said about a package that was read in part.
	var missed []adt.Unsearched
	sourceBearing := 0
	// One implementation of the concurrent read, in fetchsources.go, used by
	// every scan. See there for why results come back in input order.
	var toFetch []PackageObject
	var refs []sourceRef
	for _, obj := range objects {
		if IsSourceBearing(obj.Type) {
			toFetch = append(toFetch, obj)
			refs = append(refs, sourceRef{Type: obj.Type, Name: obj.Name})
		}
	}
	sourceBearing = len(refs)

	for i, r := range fetchSources(ctx, client, refs, "") {
		if r.Err != nil {
			fmt.Fprintf(os.Stderr, "  WARN: %s %s: %v\n", r.Ref.Type, r.Ref.Name, r.Err)
			missed = append(missed, adt.Unsearched{Object: r.Ref.Type + " " + r.Ref.Name, Reason: r.Err.Error()})
			continue
		}
		if r.Source == "" {
			continue
		}
		obj := toFetch[i]
		nodeID := graph.NodeID(obj.Type, obj.Name)
		g.AddNode(&graph.Node{ID: nodeID, Name: obj.Name, Type: obj.Type, Package: obj.Package})
		g.AddSourceDeps(nodeID, r.Source)
		count++
	}

	fmt.Fprintf(os.Stderr, "Resolving target packages...\n")
	missed = append(missed, resolvePackagesCLI(ctx, client, g)...)

	// Analyze crossings
	crossReport := graph.AnalyzeCrossings(g, scope, nil)

	// The gap goes into the report as data, not as a sentence appended to it.
	//
	// It used to go to stderr only, on the reasoning that this command writes
	// DOT, GraphML and JSON to stdout and prose in the middle of those is a
	// parse error rather than a caveat. That reasoning is right about the
	// machine formats and wrong about the text one, which is what a person
	// reads and what `> report.txt` captures: a package where 53 of 167 objects
	// answered 404 printed "No crossings found." with every failure on the
	// other stream.
	crossReport.SourceAttempted = sourceBearing
	crossReport.SourceRead = sourceBearing - len(missed)
	for _, m := range missed {
		crossReport.Unreadable = append(crossReport.Unreadable, m.Object+": "+m.Reason)
	}
	if note := adt.UnsearchedNote(missed, sourceBearing+len(missed), "object"); note != "" {
		fmt.Fprintln(os.Stderr, note)
	}

	// Handle --report flag
	if report != "" {
		baseName := strings.ReplaceAll(pkg, "$", "_") + "_boundaries"
		extMap := map[string]string{".md": "md", ".html": "html", ".dot": "dot", ".puml": "plantuml", ".graphml": "graphml"}
		bareMap := map[string]string{"md": ".md", "html": ".html", "dot": ".dot", "plantuml": ".puml", "graphml": ".graphml"}
		detected := false
		for ext, fmt := range extMap {
			if strings.HasSuffix(report, ext) {
				format = fmt
				detected = true
				break
			}
		}
		if !detected {
			if extSuffix, ok := bareMap[report]; ok {
				format = report
				report = baseName + extSuffix
			} else {
				return fmt.Errorf("unsupported report format %q (want md, html, dot, plantuml, graphml)", report)
			}
		}
		f, err := os.Create(report)
		if err != nil {
			return fmt.Errorf("creating report file: %w", err)
		}
		defer f.Close()
		origStdout := os.Stdout
		os.Stdout = f
		switch format {
		case "md":
			printCrossingsMD(crossReport)
		case "html":
			mmd := graph.CrossingToMermaid(crossReport, scope)
			title := fmt.Sprintf("Boundaries: %s", pkg)
			fmt.Println(graph.WrapMermaidHTML(title, mmd))
		case "dot":
			fmt.Println(graph.ToDOT(g, pkg))
		case "plantuml":
			fmt.Println(graph.ToPlantUML(g, pkg))
		case "graphml":
			fmt.Println(graph.ToGraphML(g))
		}
		os.Stdout = origStdout
		fmt.Fprintf(os.Stderr, "Report saved to %s\n", report)
		return nil
	}

	switch format {
	case "json":
		data, err := json.MarshalIndent(crossReport, "", "  ")
		if err != nil {
			return err
		}
		fmt.Println(string(data))
	case "md":
		printCrossingsMD(crossReport)
	case "mermaid":
		fmt.Println(graph.CrossingToMermaid(crossReport, scope))
	case "html":
		mmd := graph.CrossingToMermaid(crossReport, scope)
		title := fmt.Sprintf("Boundaries: %s", pkg)
		fmt.Println(graph.WrapMermaidHTML(title, mmd))
	case "dot":
		fmt.Println(graph.ToDOT(g, pkg))
	case "plantuml":
		fmt.Println(graph.ToPlantUML(g, pkg))
	case "graphml":
		fmt.Println(graph.ToGraphML(g))
	default:
		printCrossingsText(crossReport)
	}
	return nil
}

func printCrossingsText(report *graph.CrossingReport) {
	fmt.Printf("Boundaries: %s (%d packages, %d objects in the graph)\n\n",
		report.RootPackage, report.PackagesScanned, report.ObjectsScanned)

	// Above the verdict, not below it. A reader who stops at "No crossings
	// found" has to have seen this first, or they have read a clean bill over a
	// package that was read in part.
	if caveat := report.Caveat(); caveat != "" {
		fmt.Printf("%s\n\n", caveat)
	}

	if len(report.Entries) == 0 {
		if report.Complete() {
			fmt.Println("No crossings found.")
		} else {
			fmt.Println("No crossings found in what could be read.")
		}
		return
	}

	for _, dir := range graph.CrossingDirectionOrder {
		var entries []graph.CrossingEntry
		for _, e := range report.Entries {
			if e.Direction == dir {
				entries = append(entries, e)
			}
		}
		if len(entries) == 0 {
			continue
		}
		marker := "OK  "
		if dir == graph.CrossSibling || dir == graph.CrossDownward || dir == graph.CrossCommonDown {
			marker = "BAD "
		}
		if dir == graph.CrossExternal {
			marker = "WARN"
		}
		fmt.Printf("  %s  %-12s %d\n", marker, dir, len(entries))
		for _, e := range entries {
			ref := e.EdgeKind
			if e.RefDetail != "" {
				ref += " " + e.RefDetail
			}
			fmt.Printf("         %s → %s  %s %s → %s %s  [%s]\n",
				e.SourcePackage, e.TargetPackage, e.SourceType, e.SourceObject, e.TargetType, e.TargetObject, ref)
		}
		fmt.Println()
	}

	if len(report.Circular) > 0 {
		fmt.Println("  CIRCULAR:")
		for _, c := range report.Circular {
			fmt.Printf("    %s\n", c)
		}
		fmt.Println()
	}

	bad := report.Sibling + report.Downward + report.CommonDown
	if bad == 0 && len(report.Circular) == 0 {
		fmt.Println("CLEAN — no directional violations")
	} else {
		fmt.Printf("%d violations (sibling: %d, downward: %d, common_down: %d)\n",
			bad, report.Sibling, report.Downward, report.CommonDown)
	}
}

var boundariesCmd = &cobra.Command{
	Use:   "boundaries <package>",
	Short: "Analyze directional package boundary crossings",
	Long: `Analyze cross-package dependencies with directional classification.

Directions: UPWARD (ok), COMMON (ok), SIBLING (bad), DOWNWARD (bad),
COMMON_DOWN (bad), EXTERNAL (info). Detects circular sibling dependencies.

Examples:
  vsp boundaries '$ZDEV'
  vsp boundaries '$ZDEV' --format json
  vsp boundaries '$ZDEV' --report md
  vsp boundaries '$ZDEV' --exact`,
	Args: cobra.ExactArgs(1),
	RunE: runBoundaries,
}

var whatPackageCmd = &cobra.Command{
	Use:   "what-package <name> [name...]",
	Short: "Look up TADIR package assignment for objects",
	Long: `Query TADIR to find the canonical type and package (DEVCLASS) for one
or more ABAP objects. Useful for debugging boundary analysis results.

Examples:
  vsp what-package ZCL_MY_CLASS
  vsp what-package ZIF_LOGGER ZCX_S ZCL_BLOG
  vsp what-package ZDEMO_117_MIN_ALERT_CREATE_LOC`,
	Args: cobra.MinimumNArgs(1),
	RunE: runWhatPackage,
}

func runWhatPackage(cmd *cobra.Command, args []string) error {
	params, err := resolveSystemParams(cmd)
	if err != nil {
		return err
	}
	client, err := getClient(params)
	if err != nil {
		return err
	}

	names := make([]string, len(args))
	for i, a := range args {
		names[i] = strings.ToUpper(strings.TrimSpace(a))
	}

	// Query TADIR for R3TR entries
	quoted := make([]string, len(names))
	for i, n := range names {
		quoted[i] = "'" + n + "'"
	}
	inClause := strings.Join(quoted, ",")

	query := fmt.Sprintf("SELECT PGMID, OBJECT, OBJ_NAME, DEVCLASS FROM TADIR WHERE OBJ_NAME IN (%s) ORDER BY PGMID, OBJECT", inClause)
	result, err := client.RunQuery(context.Background(), query, len(names)*5)
	if err != nil {
		return fmt.Errorf("TADIR query failed: %v", err)
	}

	found := make(map[string]bool)
	if result != nil {
		for _, row := range result.Rows {
			pgmid := strings.TrimSpace(fmt.Sprintf("%v", row["PGMID"]))
			objType := strings.TrimSpace(fmt.Sprintf("%v", row["OBJECT"]))
			objName := strings.TrimSpace(fmt.Sprintf("%v", row["OBJ_NAME"]))
			devclass := strings.TrimSpace(fmt.Sprintf("%v", row["DEVCLASS"]))
			fmt.Printf("%-5s %-4s %-40s → %s\n", pgmid, objType, objName, devclass)
			found[strings.ToUpper(objName)] = true
		}
	}

	// Pass 2: TFDIR fallback for not-found names (function modules)
	var notFound []string
	for _, n := range names {
		if !found[n] {
			notFound = append(notFound, n)
		}
	}
	if len(notFound) > 0 {
		nfQuoted := make([]string, len(notFound))
		for i, n := range notFound {
			nfQuoted[i] = "'" + n + "'"
		}
		tfQuery := fmt.Sprintf("SELECT FUNCNAME, PNAME FROM TFDIR WHERE FUNCNAME IN (%s)", strings.Join(nfQuoted, ","))
		tfResult, err := client.RunQuery(context.Background(), tfQuery, len(notFound)*2)
		if err == nil && tfResult != nil {
			for _, row := range tfResult.Rows {
				funcName := strings.ToUpper(strings.TrimSpace(fmt.Sprintf("%v", row["FUNCNAME"])))
				pname := strings.ToUpper(strings.TrimSpace(fmt.Sprintf("%v", row["PNAME"])))
				fugrName := ""
				if strings.HasPrefix(pname, "SAPL") {
					fugrName = pname[4:]
				}
				// Look up FUGR in TADIR
				devclass := "?"
				if fugrName != "" {
					fugrQuery := fmt.Sprintf("SELECT DEVCLASS FROM TADIR WHERE PGMID = 'R3TR' AND OBJECT = 'FUGR' AND OBJ_NAME = '%s'", fugrName)
					fugrResult, err := client.RunQuery(context.Background(), fugrQuery, 1)
					if err == nil && fugrResult != nil && len(fugrResult.Rows) > 0 {
						devclass = strings.TrimSpace(fmt.Sprintf("%v", fugrResult.Rows[0]["DEVCLASS"]))
					}
				}
				fmt.Printf("%-5s %-4s %-40s → %s  (FUGR: %s, PNAME: %s)\n", "TFDIR", "FUNC", funcName, devclass, fugrName, pname)
				found[funcName] = true
			}
		}
	}

	// Report truly not found
	for _, n := range names {
		if !found[n] {
			fmt.Printf("%-5s %-4s %-40s → NOT FOUND\n", "?", "?", n)
		}
	}

	return nil
}

func printCrossingsMD(report *graph.CrossingReport) {
	if report == nil || len(report.Entries) == 0 {
		return
	}
	fmt.Print("\n## Boundary Crossings\n\n")

	for _, dir := range graph.CrossingDirectionOrder {
		var entries []graph.CrossingEntry
		for _, e := range report.Entries {
			if e.Direction == dir {
				entries = append(entries, e)
			}
		}
		if len(entries) == 0 {
			continue
		}
		verdict := "OK"
		if dir == graph.CrossSibling || dir == graph.CrossDownward || dir == graph.CrossCommonDown {
			verdict = "BAD"
		}
		if dir == graph.CrossExternal {
			verdict = "WARN"
		}
		fmt.Printf("### %s — %s (%d)\n\n", dir, verdict, len(entries))
		fmt.Println("| From Pkg | Source Object | To Pkg | Target Object | Edge | Detail |")
		fmt.Println("|----------|---------------|--------|---------------|------|--------|")
		for _, e := range entries {
			fmt.Printf("| %s | %s %s | %s | %s %s | %s | %s |\n",
				e.SourcePackage, e.SourceType, e.SourceObject,
				e.TargetPackage, e.TargetType, e.TargetObject,
				e.EdgeKind, e.RefDetail)
		}
		fmt.Println()
	}

	if len(report.Circular) > 0 {
		fmt.Print("### Circular Dependencies\n\n")
		for _, c := range report.Circular {
			fmt.Printf("- %s\n", c)
		}
		fmt.Println()
	}
}

// resolvePackagesCLI fills in missing packages and corrects object types from
// TADIR, then TFDIR→TADIR for function modules (see adtsource.ResolvePackages).
//
// It returns the objects whose package is unknown *because a query failed*, and
// that distinction is the whole point. AnalyzeCrossings drops an edge whose
// source package is empty and guesses at one whose target package is empty, so
// a resolve that never ran comes back as a boundary report with fewer crossings
// in it — clean because nothing could be looked at, which reads exactly like
// clean because there was nothing to find. An object the query did reach and
// did not find in TADIR is a genuine answer (a local class, a standard name)
// and is not reported here, or every run would carry a caveat.
func resolvePackagesCLI(ctx context.Context, client *adt.Client, g *graph.Graph) []adt.Unsearched {
	return adtsource.ResolvePackages(ctx, client, g, warnResolveFailure).Unplaced()
}

// warnResolveFailure says on stderr that a lookup failed, as it happens. A
// failed batch loses five objects' packages, and a boundary report is built
// out of packages; failing the whole run over it would be worse, so the run
// carries on — but silently is what turned a blocked query into a clean report.
func warnResolveFailure(f adtsource.Failure) {
	if f.Err == nil {
		return
	}
	switch f.Stage {
	case adtsource.StageTADIR:
		fmt.Fprintf(os.Stderr, "    WARN: TADIR resolve batch failed: %v\n", f.Err)
	case adtsource.StageTFDIR:
		fmt.Fprintf(os.Stderr, "    WARN: TFDIR resolve batch failed: %v\n", f.Err)
	case adtsource.StageFUGR:
		fmt.Fprintf(os.Stderr, "    WARN: FUGR TADIR resolve failed: %v\n", f.Err)
	}
}

func init() {
	boundariesCmd.Flags().String("format", "text", "Output format: text, json, md, mermaid, html, dot, plantuml, graphml")
	boundariesCmd.Flags().String("report", "", "Save report to file: md or filename.md")
	boundariesCmd.Flags().Bool("exact", false, "Check only the exact package, no subpackages")
	rootCmd.AddCommand(boundariesCmd)

	rootCmd.AddCommand(whatPackageCmd)
}
