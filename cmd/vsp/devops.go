package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"sort"
	"strings"
	"time"

	embedded "github.com/oisee/vibing-steampunk/embedded/abap"
	"github.com/oisee/vibing-steampunk/embedded/deps"
	"github.com/oisee/vibing-steampunk/pkg/adt"
	"github.com/oisee/vibing-steampunk/pkg/ctxcomp"
	"github.com/oisee/vibing-steampunk/pkg/graph"
	"github.com/spf13/cobra"
)

// --- source subcommands ---

var sourceReadCmd = &cobra.Command{
	Use:   "read <type> <name>",
	Short: "Read ABAP source code",
	Long: `Read source code for an ABAP object (same as 'vsp source <type> <name>').

Examples:
  vsp source read CLAS ZCL_MY_CLASS
  vsp source read PROG ZTEST_PROGRAM`,
	Args: cobra.ExactArgs(2),
	RunE: runSource, // reuse existing handler
}

var sourceWriteCmd = &cobra.Command{
	Use:   "write <type> <name>",
	Short: "Write ABAP source code from stdin",
	Long: `Write source code to an ABAP object. Reads source from stdin.

Examples:
  cat myclass.abap | vsp source write CLAS ZCL_MY_CLASS
  echo "REPORT ztest." | vsp source write PROG ZTEST
  vsp source write CLAS ZCL_FOO --transport A4HK900001 < source.abap`,
	Args: cobra.ExactArgs(2),
	RunE: runSourceWrite,
}

var sourceEditCmd = &cobra.Command{
	Use:   "edit <type> <name>",
	Short: "Edit ABAP source code (string replacement)",
	Long: `Perform surgical string replacement on ABAP source code.

Examples:
  vsp source edit CLAS ZCL_FOO --old "rv_result = 1." --new "rv_result = 42."
  vsp source edit PROG ZTEST --old "old code" --new "new code" --replace-all`,
	Args: cobra.ExactArgs(2),
	RunE: runSourceEdit,
}

var sourceContextCmd = &cobra.Command{
	Use:   "context <type> <name>",
	Short: "Get source with compressed dependency contracts",
	Long: `Retrieve source code with auto-appended dependency contracts.
Dependencies are extracted from the source and their public APIs are compressed.

Examples:
  vsp source context CLAS ZCL_MY_CLASS
  vsp source context CLAS ZCL_FOO --max-deps 30`,
	Args: cobra.ExactArgs(2),
	RunE: runSourceContext,
}

// --- context top-level shortcut ---

var contextCmd = &cobra.Command{
	Use:   "context <type> <name>",
	Short: "Get source with compressed dependency contracts",
	Long: `Retrieve source code with auto-appended dependency contracts (shortcut for 'vsp source context').
Use --depth 2 or 3 to expand transitive dependencies (deps of deps).

Examples:
  vsp context CLAS ZCL_MY_CLASS
  vsp context CLAS ZCL_FOO --max-deps 30
  vsp context CLAS ZCL_DEEP --depth 2   # deps of deps`,
	Args: cobra.ExactArgs(2),
	RunE: runSourceContext,
}

// --- test command ---

var testCmd = &cobra.Command{
	Use:   "test [type] [name]",
	Short: "Run ABAP Unit tests",
	Long: `Run ABAP Unit tests for an object or package.

Examples:
  vsp test CLAS ZCL_MY_CLASS
  vsp test PROG ZTEST_PROGRAM
  vsp test --package '$TMP'
  vsp test --package '$ZADT'`,
	RunE: runTest,
}

// --- atc command ---

var atcCmd = &cobra.Command{
	Use:   "atc <type> <name>",
	Short: "Run ATC checks",
	Long: `Run ABAP Test Cockpit (ATC) checks on an object.

Examples:
  vsp atc CLAS ZCL_MY_CLASS
  vsp atc PROG ZTEST_REPORT
  vsp atc CLAS ZCL_FOO --variant MY_VARIANT`,
	Args: cobra.ExactArgs(2),
	RunE: runATC,
}

// --- health command ---

var healthCmd = &cobra.Command{
	Use:   "health [type] [name]",
	Short: "Show a compact health snapshot for a package or object",
	Long: `Show a compact health snapshot composed from existing signals:
unit tests, ATC findings, boundary analysis, and staleness.

Examples:
  vsp health --package '$ZDEV'
  vsp health --package '$ZDEV' --fast
  vsp health CLAS ZCL_ORDER_SERVICE
  vsp health --package '$ZDEV' --format json`,
	RunE: runHealth,
}

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
	for _, obj := range objects {
		if !IsSourceBearing(obj.Type) {
			continue
		}
		fmt.Fprintf(os.Stderr, "\r  [%d] %s %-40s", count+1, obj.Type, obj.Name)
		source, err := client.GetSource(ctx, obj.Type, obj.Name, nil)
		if err != nil {
			fmt.Fprintf(os.Stderr, "\n  WARN: %s %s: %v\n", obj.Type, obj.Name, err)
			continue
		}
		if source == "" {
			continue
		}
		nodeID := graph.NodeID(obj.Type, obj.Name)
		g.AddNode(&graph.Node{ID: nodeID, Name: obj.Name, Type: obj.Type, Package: obj.Package})
		edges := graph.ExtractDepsFromSource(source, nodeID)
		dynEdges := graph.ExtractDynamicCalls(source, nodeID)
		for _, e := range append(edges, dynEdges...) {
			g.AddEdge(e)
			parts := strings.SplitN(e.To, ":", 2)
			if len(parts) == 2 {
				g.AddNode(&graph.Node{ID: e.To, Name: parts[1], Type: parts[0]})
			}
		}
		count++
	}
	if count > 0 {
		fmt.Fprintf(os.Stderr, "\n")
	}
	fmt.Fprintf(os.Stderr, "Resolving target packages...\n")
	resolvePackagesCLI(ctx, client, g)

	// Analyze crossings
	crossReport := graph.AnalyzeCrossings(g, scope, nil)

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
	fmt.Printf("Boundaries: %s (%d packages, %d objects scanned)\n\n",
		report.RootPackage, report.PackagesScanned, report.ObjectsScanned)

	if len(report.Entries) == 0 {
		fmt.Println("No crossings found.")
		return
	}

	dirOrder := []graph.CrossingDirection{
		graph.CrossSibling, graph.CrossDownward, graph.CrossCommonDown,
		graph.CrossExternal, graph.CrossUpward, graph.CrossUpwardSkip, graph.CrossCommon,
	}
	for _, dir := range dirOrder {
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

// --- deploy command ---

var deployCmd = &cobra.Command{
	Use:   "deploy <file> <package>",
	Short: "Deploy ABAP source file to SAP",
	Long: `Deploy an ABAP source file to a SAP package.

Supports abapGit-compatible file extensions:
  .clas.abap, .prog.abap, .intf.abap, .ddls.asddls, etc.

Examples:
  vsp deploy zcl_test.clas.abap '$TMP'
  vsp deploy zreport.prog.abap '$TMP' --transport A4HK900001`,
	Args: cobra.ExactArgs(2),
	RunE: runDeploy,
}

// --- transport command ---

var transportCmd = &cobra.Command{
	Use:   "transport",
	Short: "Transport management",
	Long: `Manage CTS transport requests.

Examples:
  vsp transport list
  vsp transport get A4HK900094`,
}

var transportListCmd = &cobra.Command{
	Use:   "list",
	Short: "List transport requests",
	Long: `List transport requests for the current user.

Examples:
  vsp transport list
  vsp transport list --user DEVELOPER`,
	RunE: runTransportList,
}

var transportGetCmd = &cobra.Command{
	Use:   "get <number>",
	Short: "Get transport details",
	Long: `Get detailed information about a transport request.

Examples:
  vsp transport get A4HK900094`,
	Args: cobra.ExactArgs(1),
	RunE: runTransportGet,
}

// --- install command ---

var installCmd = &cobra.Command{
	Use:   "install",
	Short: "Install components to SAP system",
	Long: `Install software components to a SAP system.

Subcommands:
  zadt-vsp    Install ZADT_VSP WebSocket handler (9 ABAP objects)
  abapgit     Install abapGit standalone or full edition
  list        List available installable components

Examples:
  vsp -s a4h install zadt-vsp
  vsp -s a4h install abapgit
  vsp -s a4h install abapgit --edition full
  vsp -s a4h install list
  vsp -s a4h install zadt-vsp --dry-run`,
}

var installZadtVspCmd = &cobra.Command{
	Use:   "zadt-vsp",
	Short: "Install ZADT_VSP WebSocket handler",
	Long: `Install the ZADT_VSP WebSocket handler to enable advanced features.

Deploys 9 ABAP objects (1 interface, 8 classes) in dependency order:
  ZIF_VSP_SERVICE, ZCL_VSP_UTILS, ZADT_CL_TADIR_MOVE, ZCL_VSP_RFC_SERVICE,
  ZCL_VSP_DEBUG_SERVICE, ZCL_VSP_AMDP_SERVICE, ZCL_VSP_GIT_SERVICE,
  ZCL_VSP_REPORT_SERVICE, ZCL_VSP_APC_HANDLER

Features unlocked after install:
  - WebSocket debugging (TPDAPI)
  - RFC/BAPI execution
  - AMDP debugging (experimental)
  - abapGit export (158 object types, requires abapGit)

Examples:
  vsp -s a4h install zadt-vsp
  vsp -s a4h install zadt-vsp --package '$ZADT_CUSTOM'
  vsp -s a4h install zadt-vsp --dry-run`,
	RunE: runInstallZadtVsp,
}

var installAbapGitCmd = &cobra.Command{
	Use:   "abapgit",
	Short: "Install abapGit from embedded ZIP",
	Long: `Install abapGit to a SAP system from the embedded ZIP archive.

Editions:
  standalone  Single program ZABAPGIT (default)
  full        Full $ZGIT + $ZGIT_DEV packages (576 objects)

Examples:
  vsp -s a4h install abapgit
  vsp -s a4h install abapgit --edition full
  vsp -s a4h install abapgit --edition full --package '$ZGIT_CUSTOM'
  vsp -s a4h install abapgit --dry-run`,
	RunE: runInstallAbapGit,
}

var installListCmd = &cobra.Command{
	Use:   "list",
	Short: "List available installable components",
	Long: `List all components that can be installed to a SAP system.

Shows embedded dependencies, their availability status, and target packages.`,
	RunE: runInstallList,
}

func init() {
	// Source subcommands
	sourceCmd.AddCommand(sourceReadCmd)
	sourceCmd.AddCommand(sourceWriteCmd)
	sourceCmd.AddCommand(sourceEditCmd)
	sourceCmd.AddCommand(sourceContextCmd)

	// Source write flags
	sourceWriteCmd.Flags().String("transport", "", "Transport request number")

	// Source edit flags
	sourceEditCmd.Flags().String("old", "", "String to find (required)")
	sourceEditCmd.Flags().String("new", "", "Replacement string (required)")
	sourceEditCmd.Flags().Bool("replace-all", false, "Replace all occurrences")
	sourceEditCmd.Flags().String("transport", "", "Transport request number")
	_ = sourceEditCmd.MarkFlagRequired("old")
	_ = sourceEditCmd.MarkFlagRequired("new")

	// Source context and context shortcut flags
	sourceContextCmd.Flags().Int("max-deps", 20, "Maximum number of dependencies to resolve")
	sourceContextCmd.Flags().Int("depth", 1, "Dependency expansion depth (1-3)")
	contextCmd.Flags().Int("max-deps", 20, "Maximum number of dependencies to resolve")
	contextCmd.Flags().Int("depth", 1, "Dependency expansion depth (1-3)")

	// Test flags
	testCmd.Flags().String("package", "", "Run tests for entire package")

	// Health flags
	healthCmd.Flags().String("package", "", "Analyze an entire package")
	healthCmd.Flags().Bool("fast", false, "Faster package snapshot: skip expensive checks like tests and boundary scan")
	healthCmd.Flags().Bool("details", false, "Show full details: failing test methods, ATC findings")
	healthCmd.Flags().String("format", "text", "Output format: text, json, md, or html")
	healthCmd.Flags().String("report", "", "Generate report file: md or html (writes to <package>.<ext>)")

	// ATC flags
	atcCmd.Flags().String("variant", "", "ATC check variant (empty for system default)")
	atcCmd.Flags().Int("max-findings", 100, "Maximum number of findings")

	// Deploy flags
	deployCmd.Flags().String("transport", "", "Transport request number")

	// Transport list flags
	transportListCmd.Flags().String("user", "", "Filter by user (default: current user)")

	// Install flags
	installZadtVspCmd.Flags().String("package", "$ZADT_VSP", "Target package for ZADT_VSP objects")
	installZadtVspCmd.Flags().Bool("dry-run", false, "Show what would be deployed without deploying")
	installZadtVspCmd.Flags().Bool("skip-git-service", false, "Skip ZCL_VSP_GIT_SERVICE even if abapGit is detected")

	installAbapGitCmd.Flags().String("edition", "standalone", "abapGit edition: standalone or full")
	installAbapGitCmd.Flags().String("package", "", "Target package (default: $ABAPGIT for standalone, $ZGIT for full)")
	installAbapGitCmd.Flags().Bool("dry-run", false, "Show what would be deployed without deploying")

	// Install subcommands
	installCmd.AddCommand(installZadtVspCmd)
	installCmd.AddCommand(installAbapGitCmd)
	installCmd.AddCommand(installListCmd)

	// Register top-level commands
	rootCmd.AddCommand(testCmd)
	rootCmd.AddCommand(atcCmd)
	rootCmd.AddCommand(healthCmd)
	boundariesCmd.Flags().String("format", "text", "Output format: text, json, md, mermaid, html, dot, plantuml, graphml")
	boundariesCmd.Flags().String("report", "", "Save report to file: md or filename.md")
	boundariesCmd.Flags().Bool("exact", false, "Check only the exact package, no subpackages")
	rootCmd.AddCommand(boundariesCmd)

	trBoundariesCmd.Flags().String("format", "text", "Output format: text, json, md, or html")
	trBoundariesCmd.Flags().String("report", "", "Generate report file: html, md, json, or filename.{html,md,json}")
	trBoundariesCmd.Flags().Bool("details", false, "Show cross-package dependencies within the transport scope")
	rootCmd.AddCommand(trBoundariesCmd)

	crBoundariesCmd.Flags().String("format", "text", "Output format: text, json, md, or html")
	crBoundariesCmd.Flags().String("report", "", "Generate report file: html, md, json, or filename.{html,md,json}")
	crBoundariesCmd.Flags().Bool("details", false, "Show cross-package dependencies within the CR scope")
	rootCmd.AddCommand(crBoundariesCmd)

	crHistoryCmd.Flags().String("format", "text", "Output format: text or json")
	rootCmd.AddCommand(crHistoryCmd)
	rootCmd.AddCommand(deployCmd)
	rootCmd.AddCommand(transportCmd)
	rootCmd.AddCommand(contextCmd)
	rootCmd.AddCommand(installCmd)
	rootCmd.AddCommand(whatPackageCmd)

	// Transport subcommands
	transportCmd.AddCommand(transportListCmd)
	transportCmd.AddCommand(transportGetCmd)
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

var trBoundariesCmd = &cobra.Command{
	Use:   "tr-boundaries <transport> [transport...]",
	Short: "Check transport self-consistency (are all dependencies included?)",
	Long: `Analyze whether a transport (or set of transports) carries all the objects
it depends on. Reports missing custom dependencies, standard SAP references,
and dynamic calls.

Examples:
  vsp tr-boundaries A4HK900001
  vsp tr-boundaries A4HK900001 A4HK900002
  vsp tr-boundaries A4HK900001 --format json`,
	Args: cobra.MinimumNArgs(1),
	RunE: runTRBoundaries,
}

var crBoundariesCmd = &cobra.Command{
	Use:   "cr-boundaries <cr-id>",
	Short: "Check change request self-consistency via E070A attribute",
	Long: `Resolve all transports for a change request (via E070A transport attribute),
then check if they collectively carry all required dependencies.

Requires transport_attribute to be configured (.vsp.json or SAP_TRANSPORT_ATTRIBUTE env).

Examples:
  vsp cr-boundaries JIRA-123
  vsp cr-boundaries JIRA-123 --format json`,
	Args: cobra.ExactArgs(1),
	RunE: runCRBoundaries,
}

var crHistoryCmd = &cobra.Command{
	Use:   "cr-history <type> <name>",
	Short: "List all CRs where an object was touched",
	Long: `Find all change requests that touched an object, derived from transport
history (E071) and transport attributes (E070A). Includes both R3TR and LIMU entries.

Requires transport_attribute to be configured for CR grouping.

Examples:
  vsp cr-history CLAS ZCL_MY_CLASS
  vsp cr-history PROG ZTEST_PROGRAM
  vsp cr-history CLAS ZCL_MY_CLASS --format json`,
	Args: cobra.ExactArgs(2),
	RunE: runCRHistory,
}

// --- handler implementations ---

func runTRBoundaries(cmd *cobra.Command, args []string) error {
	params, err := resolveSystemParams(cmd)
	if err != nil {
		return err
	}
	client, err := getClient(params)
	if err != nil {
		return err
	}
	trList := make([]string, len(args))
	for i, a := range args {
		trList[i] = strings.ToUpper(strings.TrimSpace(a))
	}

	report, err := analyzeTRBoundariesCLI(context.Background(), client, trList)
	if err != nil {
		return err
	}

	return outputTRBoundaries(cmd, report)
}

func runCRBoundaries(cmd *cobra.Command, args []string) error {
	params, err := resolveSystemParams(cmd)
	if err != nil {
		return err
	}
	client, err := getClient(params)
	if err != nil {
		return err
	}

	crID := strings.TrimSpace(args[0])
	attr := params.TransportAttribute
	if attr == "" {
		return fmt.Errorf("transport_attribute not configured. Set SAP_TRANSPORT_ATTRIBUTE or transport_attribute in .vsp.json")
	}

	// Resolve transports from CR
	fmt.Fprintf(os.Stderr, "Resolving transports for CR %s (attribute: %s)...\n", crID, attr)
	attrQuery := fmt.Sprintf(
		"SELECT TRKORR FROM E070A WHERE ATTRIBUTE = '%s' AND REFERENCE = '%s'",
		attr, crID)
	attrResult, err := client.RunQuery(context.Background(), attrQuery, 500)
	if err != nil {
		return fmt.Errorf("E070A query failed: %v", err)
	}

	var trList []string
	for _, row := range attrResult.Rows {
		tr := strings.TrimSpace(fmt.Sprintf("%v", row["TRKORR"]))
		if tr != "" {
			trList = append(trList, tr)
		}
	}
	if len(trList) == 0 {
		fmt.Printf("No transports found for CR %s\n", crID)
		return nil
	}

	// Also get child tasks
	reqQuoted := make([]string, len(trList))
	for i, tr := range trList {
		reqQuoted[i] = "'" + tr + "'"
	}
	taskQuery := fmt.Sprintf("SELECT TRKORR FROM E070 WHERE STRKORR IN (%s)", strings.Join(reqQuoted, ","))
	taskResult, err := client.RunQuery(context.Background(), taskQuery, 500)
	if err == nil && taskResult != nil {
		for _, row := range taskResult.Rows {
			tr := strings.TrimSpace(fmt.Sprintf("%v", row["TRKORR"]))
			if tr != "" {
				trList = append(trList, tr)
			}
		}
	}

	fmt.Fprintf(os.Stderr, "Found %d transports for CR %s\n", len(trList), crID)

	report, err := analyzeTRBoundariesCLI(context.Background(), client, trList)
	if err != nil {
		return err
	}
	report.Scope = fmt.Sprintf("CR:%s (%s)", crID, attr)

	return outputTRBoundaries(cmd, report)
}

func runCRHistory(cmd *cobra.Command, args []string) error {
	params, err := resolveSystemParams(cmd)
	if err != nil {
		return err
	}
	client, err := getClient(params)
	if err != nil {
		return err
	}
	format, _ := cmd.Flags().GetString("format")

	objType := strings.ToUpper(args[0])
	objName := strings.ToUpper(args[1])
	attr := params.TransportAttribute

	// Query E071 for R3TR (exact match) and LIMU (prefix match) separately
	// SAP freestyle query API doesn't support complex OR clauses well
	trSet := make(map[string]bool)

	e071R3TR := fmt.Sprintf(
		"SELECT TRKORR FROM E071 WHERE PGMID = 'R3TR' AND OBJECT = '%s' AND OBJ_NAME = '%s'",
		objType, objName)
	r3trResult, err := client.RunQuery(context.Background(), e071R3TR, 500)
	if err != nil {
		return fmt.Errorf("E071 R3TR query failed: %v", err)
	}
	for _, row := range r3trResult.Rows {
		tr := strings.TrimSpace(fmt.Sprintf("%v", row["TRKORR"]))
		if tr != "" {
			trSet[tr] = true
		}
	}

	e071LIMU := fmt.Sprintf(
		"SELECT TRKORR FROM E071 WHERE PGMID = 'LIMU' AND OBJ_NAME LIKE '%s%%'",
		objName)
	limuResult, err := client.RunQuery(context.Background(), e071LIMU, 500)
	if err != nil {
		fmt.Fprintf(os.Stderr, "WARN: E071 LIMU query failed (continuing with R3TR only): %v\n", err)
	} else {
		for _, row := range limuResult.Rows {
			tr := strings.TrimSpace(fmt.Sprintf("%v", row["TRKORR"]))
			if tr != "" {
				trSet[tr] = true
			}
		}
	}

	if len(trSet) == 0 {
		fmt.Printf("No transports found for %s %s\n", objType, objName)
		return nil
	}

	// Resolve task→request
	trQuoted := make([]string, 0, len(trSet))
	for tr := range trSet {
		trQuoted = append(trQuoted, "'"+tr+"'")
	}
	e070Query := fmt.Sprintf("SELECT TRKORR, STRKORR, AS4USER, AS4DATE FROM E070 WHERE TRKORR IN (%s)", strings.Join(trQuoted, ","))
	e070Result, _ := client.RunQuery(context.Background(), e070Query, 500)

	requestSet := make(map[string]bool)
	trMeta := make(map[string]struct{ user, date string })
	if e070Result != nil {
		for _, row := range e070Result.Rows {
			tr := strings.TrimSpace(fmt.Sprintf("%v", row["TRKORR"]))
			parent := strings.TrimSpace(fmt.Sprintf("%v", row["STRKORR"]))
			user := strings.TrimSpace(fmt.Sprintf("%v", row["AS4USER"]))
			date := strings.TrimSpace(fmt.Sprintf("%v", row["AS4DATE"]))
			trMeta[tr] = struct{ user, date string }{user, date}
			if parent != "" {
				requestSet[parent] = true
			} else {
				requestSet[tr] = true
			}
		}
	}

	type crEntry struct {
		crID       string
		transports []string
		users      map[string]bool
		dates      map[string]bool
	}
	var crEntries []crEntry

	// Look up CRs via E070A if attribute configured
	if attr != "" && len(requestSet) > 0 {
		reqList := make([]string, 0, len(requestSet))
		for r := range requestSet {
			reqList = append(reqList, "'"+r+"'")
		}
		attrQuery := fmt.Sprintf("SELECT TRKORR, REFERENCE FROM E070A WHERE ATTRIBUTE = '%s' AND TRKORR IN (%s)", attr, strings.Join(reqList, ","))
		attrResult, err := client.RunQuery(context.Background(), attrQuery, 500)
		if err == nil && attrResult != nil {
			crMap := make(map[string]*crEntry)
			for _, row := range attrResult.Rows {
				tr := strings.TrimSpace(fmt.Sprintf("%v", row["TRKORR"]))
				ref := strings.TrimSpace(fmt.Sprintf("%v", row["REFERENCE"]))
				if ref == "" {
					continue
				}
				e, ok := crMap[ref]
				if !ok {
					e = &crEntry{crID: ref, users: make(map[string]bool), dates: make(map[string]bool)}
					crMap[ref] = e
				}
				e.transports = append(e.transports, tr)
				if meta, ok := trMeta[tr]; ok {
					e.users[meta.user] = true
					e.dates[meta.date] = true
				}
			}
			for _, e := range crMap {
				crEntries = append(crEntries, *e)
			}
		}
	}

	switch format {
	case "json":
		result := map[string]any{
			"object_type": objType,
			"object_name": objName,
			"attribute":   attr,
			"transports":  trSet,
			"crs":         crEntries,
		}
		data, _ := json.MarshalIndent(result, "", "  ")
		fmt.Println(string(data))
	default:
		fmt.Printf("CR History: %s %s\n", objType, objName)
		fmt.Printf("Transports: %d found\n", len(trSet))
		for tr := range trSet {
			meta := trMeta[tr]
			fmt.Printf("  %s  %s  %s\n", tr, meta.user, meta.date)
		}
		if len(crEntries) > 0 {
			fmt.Printf("\nChange Requests (attribute: %s):\n", attr)
			for _, e := range crEntries {
				users := make([]string, 0, len(e.users))
				for u := range e.users {
					users = append(users, u)
				}
				fmt.Printf("  %s  transports: %s  users: %s\n", e.crID, strings.Join(e.transports, ","), strings.Join(users, ","))
			}
		} else if attr != "" {
			fmt.Printf("\nNo CRs found for attribute %s\n", attr)
		} else {
			fmt.Printf("\nNo transport_attribute configured — set SAP_TRANSPORT_ATTRIBUTE for CR grouping\n")
		}
	}
	return nil
}

func analyzeTRBoundariesCLI(ctx context.Context, client *adt.Client, trList []string) (*graph.TransportBoundaryReport, error) {
	// Step 1: resolve parent dev objects for the TR list. The shared helper
	// handles R3TR and LIMU uniformly, collapses LIMU subcomponents to their
	// parent, and TADIR-filters deleted entries (so source-fetch does not
	// 404 later on stale class references).
	liveObjs, deletedRefs, err := collectCRDevObjects(ctx, client, trList)
	if err != nil {
		return nil, err
	}
	if len(deletedRefs) > 0 {
		fmt.Fprintf(os.Stderr, "  (dropping %d deleted/stale object refs — see report)\n", len(deletedRefs))
	}

	trSet := make(map[string]bool)
	for _, tr := range trList {
		trSet[strings.ToUpper(tr)] = true
	}

	type objKey struct{ objType, objName string }
	objectSet := make(map[objKey]bool)
	scopeObjects := make(map[string]bool)

	for _, o := range liveObjs {
		objectSet[objKey{o.ObjectType, o.ObjectName}] = true
		scopeObjects[graph.NodeID(o.ObjectType, o.ObjectName)] = true
	}

	if len(objectSet) == 0 {
		return &graph.TransportBoundaryReport{
			Scope:       strings.Join(trList, ","),
			ObjectCount: 0,
			Summary:     graph.TransportBoundarySummary{SelfConsistent: true},
		}, nil
	}

	scope := &graph.TransportScope{
		Label:      strings.Join(trList, ","),
		Transports: trSet,
		Objects:    scopeObjects,
	}

	// Step 2: Build dependency graph. Sort the object list so the analysis
	// (and its stderr "Analyzing X..." trace) is stable across runs — makes
	// the maxObjects=50 cap deterministic and diffing reports meaningful.
	sortedObjs := make([]objKey, 0, len(objectSet))
	for k := range objectSet {
		sortedObjs = append(sortedObjs, k)
	}
	sort.Slice(sortedObjs, func(i, j int) bool {
		if sortedObjs[i].objType != sortedObjs[j].objType {
			return sortedObjs[i].objType < sortedObjs[j].objType
		}
		return sortedObjs[i].objName < sortedObjs[j].objName
	})

	g := graph.New()
	maxObjects := 50
	count := 0
	truncated := false

	for _, obj := range sortedObjs {
		if count >= maxObjects {
			truncated = true
			break
		}
		nodeID := graph.NodeID(obj.objType, obj.objName)
		g.AddNode(&graph.Node{ID: nodeID, Name: obj.objName, Type: obj.objType})

		if obj.objType != "CLAS" && obj.objType != "PROG" && obj.objType != "FUGR" && obj.objType != "INTF" {
			continue
		}

		fmt.Fprintf(os.Stderr, "  Analyzing %s %s...\n", obj.objType, obj.objName)
		var source string
		var err error
		if obj.objType == "FUGR" {
			// GetSource for FUGR returns JSON metadata (function module list), which has
			// no ABAP statements and yields zero dependencies. For accurate graph analysis
			// we need the actual source: TOP include + all fmodules + all sub-includes.
			source, err = client.GetFunctionGroupAllSources(ctx, obj.objName)
		} else {
			source, err = client.GetSource(ctx, obj.objType, obj.objName, nil)
		}
		if err != nil {
			fmt.Fprintf(os.Stderr, "    WARN: %s %s: %v\n", obj.objType, obj.objName, err)
			continue
		}

		edges := graph.ExtractDepsFromSource(source, nodeID)
		dynEdges := graph.ExtractDynamicCalls(source, nodeID)
		for _, e := range append(edges, dynEdges...) {
			g.AddEdge(e)
			g.AddNode(&graph.Node{
				ID:   e.To,
				Name: strings.SplitN(e.To, ":", 2)[1],
				Type: strings.SplitN(e.To, ":", 2)[0],
			})
		}
		count++
	}

	// Resolve packages
	resolvePackagesCLI(ctx, client, g)

	if truncated {
		fmt.Fprintf(os.Stderr,
			"  WARN: analysis truncated at %d objects (CR has %d source-bearing objects). Deps for the remaining %d are not counted — boundary totals are lower bounds, not complete.\n",
			maxObjects, len(sortedObjs), len(sortedObjs)-maxObjects)
	}

	return graph.AnalyzeTransportBoundaries(g, scope), nil
}

func printTRBoundariesText(report *graph.TransportBoundaryReport, details bool) {
	status := "SELF-CONSISTENT"
	if !report.Summary.SelfConsistent {
		status = "INCOMPLETE"
	}
	fmt.Printf("Transport Boundaries: %s\n", report.Scope)
	fmt.Printf("Status: %s\n", status)
	fmt.Printf("Objects: %d | Deps: %d (in-scope: %d [same-pkg: %d, cross-pkg: %d], missing: %d, standard: %d, dynamic: %d)\n\n",
		report.ObjectCount, report.Summary.TotalDeps,
		report.Summary.InScope, report.Summary.InScopeSamePkg, report.Summary.InScopeCrossPkg,
		report.Summary.Missing, report.Summary.Standard, report.Summary.Dynamic)

	if len(report.Missing) > 0 {
		fmt.Println("MISSING (custom objects not in transport):")
		fmt.Println("  Source                → Target                  Edge        Package")
		fmt.Println("  ────────────────────  ──────────────────────── ─────────── ────────────")
		for _, e := range report.Missing {
			fmt.Printf("  %-4s %-16s → %-4s %-18s %-11s %s\n",
				e.SourceType, e.SourceName, e.TargetType, e.TargetName, e.EdgeKind, e.TargetPackage)
		}
		fmt.Println()
	}

	if len(report.Dynamic) > 0 {
		fmt.Println("DYNAMIC (unresolved calls):")
		for _, e := range report.Dynamic {
			fmt.Printf("  %-4s %-16s → %s\n", e.SourceType, e.SourceName, e.RefDetail)
		}
		fmt.Println()
	}

	if details && len(report.CrossPackage) > 0 {
		fmt.Println("CROSS-PACKAGE (in scope but different package):")
		currentPkg := ""
		for _, e := range report.CrossPackage {
			if e.TargetPackage != currentPkg {
				currentPkg = e.TargetPackage
				fmt.Printf("\n  → %s\n", currentPkg)
			}
			fmt.Printf("    %-4s %-20s (%-12s) → %-4s %-20s  %s\n",
				e.SourceType, e.SourceName, e.SourcePackage, e.TargetType, e.TargetName, e.EdgeKind)
		}
		fmt.Println()
	}
}

func outputTRBoundaries(cmd *cobra.Command, report *graph.TransportBoundaryReport) error {
	format, _ := cmd.Flags().GetString("format")
	reportFlag, _ := cmd.Flags().GetString("report")
	details, _ := cmd.Flags().GetBool("details")

	// --report: resolve format and filename. Accepts three shapes:
	//   - explicit format name ("html" / "md" / "json") — filename is
	//     auto-generated from the scope identifier
	//   - filename ending in a known extension — format inferred
	//   - anything else → explicit error listing the accepted forms
	if reportFlag != "" {
		switch {
		case strings.HasSuffix(reportFlag, ".html"):
			format = "html"
		case strings.HasSuffix(reportFlag, ".md"):
			format = "md"
		case strings.HasSuffix(reportFlag, ".json"):
			format = "json"
		case reportFlag == "html" || reportFlag == "md" || reportFlag == "json":
			format = reportFlag
			reportFlag = sanitizeScopeFilename(report.Scope) + "." + format
		default:
			return fmt.Errorf("unsupported --report value %q (want html, md, json, or filename.{html,md,json})", reportFlag)
		}
		f, err := os.Create(reportFlag)
		if err != nil {
			return fmt.Errorf("creating report file: %w", err)
		}
		defer f.Close()
		origStdout := os.Stdout
		os.Stdout = f
		defer func() { os.Stdout = origStdout }()
	}

	switch format {
	case "json":
		data, _ := json.MarshalIndent(report, "", "  ")
		fmt.Println(string(data))
	case "html":
		printTRBoundariesHTML(report, details)
	case "md":
		printTRBoundariesMarkdown(report, details)
	default:
		printTRBoundariesText(report, details)
	}

	if reportFlag != "" {
		fmt.Fprintf(os.Stderr, "Report saved to %s\n", reportFlag)
	}
	return nil
}

func printTRBoundariesHTML(report *graph.TransportBoundaryReport, details bool) {
	status := "SELF-CONSISTENT"
	statusClass := "PASS"
	if !report.Summary.SelfConsistent {
		status = "INCOMPLETE"
		statusClass = "FAIL"
	}

	fmt.Printf(`<!DOCTYPE html>
<html><head><meta charset="UTF-8">
<title>Transport Boundaries: %s</title>
<style>
  body { font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', sans-serif; max-width: 1100px; margin: 2em auto; padding: 0 1em; color: #333; }
  h1 { border-bottom: 2px solid #ddd; padding-bottom: 0.3em; }
  h2 { margin-top: 1.5em; color: #555; }
  table { border-collapse: collapse; width: 100%%; margin: 1em 0; }
  th, td { border: 1px solid #ddd; padding: 6px 10px; text-align: left; font-size: 0.9em; }
  th { background: #f5f5f5; }
  .PASS { color: #2e7d32; } .FAIL { color: #c62828; } .WARN { color: #ef6c00; }
  .summary { display: flex; gap: 2em; margin: 1em 0; }
  .summary .box { background: #f9f9f9; border: 1px solid #ddd; border-radius: 6px; padding: 0.8em 1.2em; }
  .summary .num { font-size: 1.5em; font-weight: bold; }
  nav.toc { background: #f9f9f9; border: 1px solid #ddd; border-radius: 6px; padding: 0.8em 1.2em; margin-bottom: 1.5em; }
  nav.toc summary { font-weight: bold; cursor: pointer; }
  nav.toc ul { margin: 0.5em 0 0; padding-left: 1.5em; }
  nav.toc li { margin: 0.2em 0; }
  nav.toc a { text-decoration: none; color: #1565c0; }
</style>
</head><body>
`, report.Scope)

	fmt.Printf("<h1>Transport Boundaries: %s</h1>\n", report.Scope)
	fmt.Printf("<p><strong class=%q>%s</strong></p>\n", statusClass, status)

	// TOC
	fmt.Println(`<nav class="toc"><details open><summary>Contents</summary><ul>`)
	fmt.Println(`<li><a href="#summary">Summary</a></li>`)
	if len(report.Missing) > 0 {
		fmt.Println(`<li><a href="#missing">Missing Dependencies</a></li>`)
	}
	if len(report.Standard) > 0 {
		fmt.Println(`<li><a href="#standard">Standard SAP References</a></li>`)
	}
	if len(report.Dynamic) > 0 {
		fmt.Println(`<li><a href="#dynamic">Dynamic Calls</a></li>`)
	}
	if details && len(report.CrossPackage) > 0 {
		fmt.Println(`<li><a href="#crosspackage">Cross-Package within Scope</a></li>`)
	}
	fmt.Println(`</ul></details></nav>`)

	// Summary
	fmt.Println(`<h2 id="summary">Summary</h2>`)
	fmt.Println(`<div class="summary">`)
	fmt.Printf("<div class=\"box\"><div class=\"num\">%d</div>Objects</div>\n", report.ObjectCount)
	fmt.Printf("<div class=\"box\"><div class=\"num\">%d</div>In-Scope</div>\n", report.Summary.InScope)
	fmt.Printf("<div class=\"box\"><div class=\"num %s\">%d</div>Missing</div>\n",
		map[bool]string{true: "PASS", false: "FAIL"}[report.Summary.Missing == 0], report.Summary.Missing)
	fmt.Printf("<div class=\"box\"><div class=\"num\">%d</div>Standard</div>\n", report.Summary.Standard)
	fmt.Printf("<div class=\"box\"><div class=\"num\">%d</div>Dynamic</div>\n", report.Summary.Dynamic)
	fmt.Println(`</div>`)

	// Missing
	if len(report.Missing) > 0 {
		fmt.Printf("<h2 id=\"missing\" class=\"FAIL\">Missing Dependencies (%d)</h2>\n", len(report.Missing))
		fmt.Println("<table><tr><th>Source</th><th>Target</th><th>Edge</th><th>Target Package</th></tr>")
		for _, e := range report.Missing {
			fmt.Printf("<tr><td>%s %s</td><td>%s %s</td><td>%s</td><td>%s</td></tr>\n",
				e.SourceType, e.SourceName, e.TargetType, e.TargetName, e.EdgeKind, e.TargetPackage)
		}
		fmt.Println("</table>")
	}

	// Standard
	if len(report.Standard) > 0 {
		fmt.Printf("<h2 id=\"standard\">Standard SAP References (%d)</h2>\n", len(report.Standard))
		fmt.Println("<table><tr><th>Source</th><th>Target</th><th>Edge</th></tr>")
		for _, e := range report.Standard {
			fmt.Printf("<tr><td>%s %s</td><td>%s %s</td><td>%s</td></tr>\n",
				e.SourceType, e.SourceName, e.TargetType, e.TargetName, e.EdgeKind)
		}
		fmt.Println("</table>")
	}

	// Dynamic
	if len(report.Dynamic) > 0 {
		fmt.Printf("<h2 id=\"dynamic\" class=\"WARN\">Dynamic Calls (%d)</h2>\n", len(report.Dynamic))
		fmt.Println("<table><tr><th>Source</th><th>Detail</th></tr>")
		for _, e := range report.Dynamic {
			fmt.Printf("<tr><td>%s %s</td><td>%s</td></tr>\n", e.SourceType, e.SourceName, e.RefDetail)
		}
		fmt.Println("</table>")
	}

	// Cross-package: in-scope deps that cross package boundaries (--details)
	if details && len(report.CrossPackage) > 0 {
		fmt.Printf("<h2 id=\"crosspackage\">Cross-Package within Scope (%d)</h2>\n", len(report.CrossPackage))
		currentPkg := ""
		for _, e := range report.CrossPackage {
			if e.TargetPackage != currentPkg {
				if currentPkg != "" {
					fmt.Println("</table>")
				}
				currentPkg = e.TargetPackage
				fmt.Printf("<h3>%s</h3>\n", currentPkg)
				fmt.Println("<table><tr><th>Source Pkg</th><th>Source</th><th>Target</th><th>Edge</th></tr>")
			}
			fmt.Printf("<tr><td>%s</td><td>%s %s</td><td>%s %s</td><td>%s</td></tr>\n",
				e.SourcePackage, e.SourceType, e.SourceName, e.TargetType, e.TargetName, e.EdgeKind)
		}
		if currentPkg != "" {
			fmt.Println("</table>")
		}
	}

	fmt.Println("</body></html>")
}

// sanitizeScopeFilename turns a boundary-report scope label like
// "CR:CR-EXAMPLE (Z_CR_ATTR)" into a filesystem-safe base name:
// keep only [A-Za-z0-9_-], collapse everything else into underscores,
// and trim trailing underscores so we do not emit ugly `report_.md`.
func sanitizeScopeFilename(scope string) string {
	var b strings.Builder
	b.Grow(len(scope))
	for _, r := range scope {
		switch {
		case r >= 'A' && r <= 'Z', r >= 'a' && r <= 'z', r >= '0' && r <= '9', r == '_', r == '-':
			b.WriteRune(r)
		default:
			b.WriteByte('_')
		}
	}
	out := strings.Trim(b.String(), "_")
	if out == "" {
		return "report"
	}
	// Collapse any run of underscores into a single one so multi-
	// character punctuation sequences don't expand into long dashes.
	for strings.Contains(out, "__") {
		out = strings.ReplaceAll(out, "__", "_")
	}
	return out
}

// printTRBoundariesMarkdown renders a cr-boundaries / tr-boundaries
// report as GitHub-flavoured Markdown. Same three sections as the
// text output (Summary → Missing → Dynamic) plus an optional Cross-
// Package section under --details. Emoji-free on purpose so grep and
// automated log parsers stay happy.
func printTRBoundariesMarkdown(report *graph.TransportBoundaryReport, details bool) {
	status := "SELF-CONSISTENT"
	if !report.Summary.SelfConsistent {
		status = "INCOMPLETE"
	}
	fmt.Printf("# Transport Boundaries: %s\n\n", report.Scope)
	fmt.Printf("**Status:** %s\n\n", status)

	fmt.Printf("## Summary\n\n")
	fmt.Println("| Metric | Value |")
	fmt.Println("|---|---|")
	fmt.Printf("| Objects in scope | %d |\n", report.ObjectCount)
	fmt.Printf("| Total dependencies | %d |\n", report.Summary.TotalDeps)
	fmt.Printf("| In-scope (same package) | %d |\n", report.Summary.InScopeSamePkg)
	fmt.Printf("| In-scope (cross-package) | %d |\n", report.Summary.InScopeCrossPkg)
	fmt.Printf("| Missing | %d |\n", report.Summary.Missing)
	fmt.Printf("| Standard SAP refs | %d |\n", report.Summary.Standard)
	fmt.Printf("| Dynamic (unresolved) | %d |\n", report.Summary.Dynamic)
	fmt.Println()

	if len(report.Missing) > 0 {
		fmt.Printf("## MISSING — custom objects not in transport\n\n")
		fmt.Println("| Source | → | Target | Edge | Package |")
		fmt.Println("|---|---|---|---|---|")
		for _, e := range report.Missing {
			fmt.Printf("| `%s %s` | → | `%s %s` | %s | `%s` |\n",
				e.SourceType, e.SourceName,
				e.TargetType, e.TargetName,
				e.EdgeKind, e.TargetPackage)
		}
		fmt.Println()
	}

	if len(report.Dynamic) > 0 {
		fmt.Printf("## DYNAMIC — unresolved calls\n\n")
		for _, e := range report.Dynamic {
			fmt.Printf("- `%s %s` → %s\n", e.SourceType, e.SourceName, e.RefDetail)
		}
		fmt.Println()
	}

	if details && len(report.CrossPackage) > 0 {
		fmt.Printf("## CROSS-PACKAGE — in-scope, different package\n\n")
		currentPkg := ""
		for _, e := range report.CrossPackage {
			if e.TargetPackage != currentPkg {
				currentPkg = e.TargetPackage
				fmt.Printf("\n### `%s`\n\n", currentPkg)
			}
			fmt.Printf("- `%s %s` (`%s`) → `%s %s` via %s\n",
				e.SourceType, e.SourceName, e.SourcePackage,
				e.TargetType, e.TargetName, e.EdgeKind)
		}
		fmt.Println()
	}
}

func runSourceWrite(cmd *cobra.Command, args []string) error {
	params, err := resolveSystemParams(cmd)
	if err != nil {
		return err
	}

	client, err := getClient(params)
	if err != nil {
		return err
	}

	objType := strings.ToUpper(args[0])
	name := strings.ToUpper(args[1])
	transport, _ := cmd.Flags().GetString("transport")

	// Read source from stdin
	source, err := io.ReadAll(os.Stdin)
	if err != nil {
		return fmt.Errorf("failed to read stdin: %w", err)
	}
	if len(source) == 0 {
		return fmt.Errorf("no source provided on stdin")
	}

	ctx := context.Background()
	result, err := client.WriteSource(ctx, objType, name, string(source), &adt.WriteSourceOptions{
		Transport: transport,
	})
	if err != nil {
		return fmt.Errorf("write failed: %w", err)
	}

	if result.Success {
		fmt.Fprintf(os.Stderr, "%s %s %s\n", result.Mode, result.ObjectType, result.ObjectName)
		if result.ObjectURL != "" {
			fmt.Fprintf(os.Stderr, "URL: %s\n", result.ObjectURL)
		}
	} else {
		fmt.Fprintf(os.Stderr, "Write failed for %s %s\n", objType, name)
		if result.Message != "" {
			fmt.Fprintf(os.Stderr, "%s\n", result.Message)
		}
		if len(result.SyntaxErrors) > 0 {
			fmt.Fprintf(os.Stderr, "Syntax errors:\n")
			for _, se := range result.SyntaxErrors {
				fmt.Fprintf(os.Stderr, "  Line %d: %s\n", se.Line, se.Text)
			}
		}
		return fmt.Errorf("write failed")
	}

	return nil
}

func runSourceEdit(cmd *cobra.Command, args []string) error {
	params, err := resolveSystemParams(cmd)
	if err != nil {
		return err
	}

	client, err := getClient(params)
	if err != nil {
		return err
	}

	objType := strings.ToUpper(args[0])
	name := strings.ToUpper(args[1])
	oldStr, _ := cmd.Flags().GetString("old")
	newStr, _ := cmd.Flags().GetString("new")
	replaceAll, _ := cmd.Flags().GetBool("replace-all")
	transport, _ := cmd.Flags().GetString("transport")

	// Build object URL from type + name
	objectURL := buildObjectURL(objType, name)
	if objectURL == "" {
		return fmt.Errorf("unsupported object type: %s (supported: CLAS, PROG, INTF)", objType)
	}

	ctx := context.Background()
	result, err := client.EditSourceWithOptions(ctx, objectURL, oldStr, newStr, &adt.EditSourceOptions{
		ReplaceAll:  replaceAll,
		SyntaxCheck: true,
		Transport:   transport,
	})
	if err != nil {
		return fmt.Errorf("edit failed: %w", err)
	}

	if result.Success {
		fmt.Fprintf(os.Stderr, "Edited %s (%d replacement(s))\n", result.ObjectName, result.MatchCount)
		if result.Activation != nil && result.Activation.Success {
			fmt.Fprintf(os.Stderr, "Activated successfully\n")
		}
	} else {
		fmt.Fprintf(os.Stderr, "Edit failed for %s\n", result.ObjectName)
		if result.Message != "" {
			fmt.Fprintf(os.Stderr, "%s\n", result.Message)
		}
		if len(result.SyntaxErrors) > 0 {
			fmt.Fprintf(os.Stderr, "Syntax errors:\n")
			for _, se := range result.SyntaxErrors {
				fmt.Fprintf(os.Stderr, "  %s\n", se)
			}
		}
		return fmt.Errorf("edit failed")
	}

	return nil
}

// cliSourceAdapter adapts adt.Client to ctxcomp.ADTSourceFetcher interface.
type cliSourceAdapter struct {
	client *adt.Client
}

func (a *cliSourceAdapter) GetSource(ctx context.Context, objectType, name string, opts interface{}) (string, error) {
	return a.client.GetSource(ctx, objectType, name, nil)
}

func runSourceContext(cmd *cobra.Command, args []string) error {
	params, err := resolveSystemParams(cmd)
	if err != nil {
		return err
	}

	client, err := getClient(params)
	if err != nil {
		return err
	}

	objType := strings.ToUpper(args[0])
	name := strings.ToUpper(args[1])
	maxDeps, _ := cmd.Flags().GetInt("max-deps")

	ctx := context.Background()

	// First get the source
	source, err := client.GetSource(ctx, objType, name, nil)
	if err != nil {
		return fmt.Errorf("failed to get source: %w", err)
	}

	depth, _ := cmd.Flags().GetInt("depth")

	// Create adapter and compress
	provider := ctxcomp.NewMultiSourceProvider("", &cliSourceAdapter{client: client})
	compressor := ctxcomp.NewCompressor(provider, maxDeps).WithDepth(depth)
	result, err := compressor.Compress(ctx, source, name, objType)
	if err != nil {
		return fmt.Errorf("context compression failed: %w", err)
	}

	// Output: source with prologue
	if result.Prologue != "" {
		fmt.Print(result.Prologue)
		fmt.Println()
	}
	fmt.Print(source)

	// Stats to stderr
	fmt.Fprintf(os.Stderr, "\n--- Context: %d deps found, %d resolved, %d failed, %d prologue lines ---\n",
		result.Stats.DepsFound, result.Stats.DepsResolved, result.Stats.DepsFailed, result.Stats.TotalLines)

	return nil
}

func runTest(cmd *cobra.Command, args []string) error {
	params, err := resolveSystemParams(cmd)
	if err != nil {
		return err
	}

	client, err := getClient(params)
	if err != nil {
		return err
	}

	packageName, _ := cmd.Flags().GetString("package")

	var objectURL string
	if packageName != "" {
		// Package-level test
		objectURL = fmt.Sprintf("/sap/bc/adt/packages/%s", strings.ToUpper(packageName))
	} else {
		// Object-level test
		if len(args) != 2 {
			return fmt.Errorf("usage: vsp test <type> <name> or vsp test --package <package>")
		}
		objType := strings.ToUpper(args[0])
		name := strings.ToUpper(args[1])
		objectURL = buildObjectURL(objType, name)
		if objectURL == "" {
			return fmt.Errorf("unsupported object type: %s (supported: CLAS, PROG, INTF)", objType)
		}
	}

	ctx := context.Background()
	result, err := client.RunUnitTests(ctx, objectURL, nil)
	if err != nil {
		return fmt.Errorf("test run failed: %w", err)
	}

	// Format output
	if len(result.Classes) == 0 {
		fmt.Println("No test classes found.")
		return nil
	}

	totalPassed := 0
	totalFailed := 0

	for _, class := range result.Classes {
		fmt.Printf("Test Class: %s\n", class.Name)
		for _, method := range class.TestMethods {
			status := "PASS"
			if len(method.Alerts) > 0 {
				hasFailure := false
				for _, alert := range method.Alerts {
					if alert.Kind == "failedAssertion" || alert.Kind == "exception" {
						hasFailure = true
						break
					}
				}
				if hasFailure {
					status = "FAIL"
					totalFailed++
				} else {
					totalPassed++
				}
			} else {
				totalPassed++
			}
			fmt.Printf("  %s  %s (%.3fs)\n", status, method.Name, method.ExecutionTime)
			for _, alert := range method.Alerts {
				fmt.Printf("         %s: %s\n", alert.Kind, alert.Title)
				for _, detail := range alert.Details {
					fmt.Printf("           %s\n", detail)
				}
			}
		}
		// Class-level alerts
		for _, alert := range class.Alerts {
			fmt.Printf("  %s: %s\n", alert.Kind, alert.Title)
			totalFailed++
		}
	}

	fmt.Printf("\nTotal: %d passed, %d failed\n", totalPassed, totalFailed)
	if totalFailed > 0 {
		return fmt.Errorf("%d test(s) failed", totalFailed)
	}
	return nil
}

func runATC(cmd *cobra.Command, args []string) error {
	params, err := resolveSystemParams(cmd)
	if err != nil {
		return err
	}

	client, err := getClient(params)
	if err != nil {
		return err
	}

	objType := strings.ToUpper(args[0])
	name := strings.ToUpper(args[1])
	variant, _ := cmd.Flags().GetString("variant")
	maxFindings, _ := cmd.Flags().GetInt("max-findings")

	objectURL := buildObjectURL(objType, name)
	if objectURL == "" {
		return fmt.Errorf("unsupported object type: %s (supported: CLAS, PROG, INTF)", objType)
	}

	ctx := context.Background()
	worklist, err := client.RunATCCheck(ctx, objectURL, variant, maxFindings)
	if err != nil {
		return fmt.Errorf("ATC check failed: %w", err)
	}

	// Format output
	totalFindings := 0
	for _, obj := range worklist.Objects {
		if len(obj.Findings) == 0 {
			continue
		}
		fmt.Printf("%s %s (%s)\n", obj.Type, obj.Name, obj.PackageName)
		for _, f := range obj.Findings {
			priority := "INFO"
			switch f.Priority {
			case 1:
				priority = "ERROR"
			case 2:
				priority = "WARN"
			}
			location := ""
			if f.Line > 0 {
				location = fmt.Sprintf(" [line %d]", f.Line)
			}
			fmt.Printf("  %s%s %s — %s\n", priority, location, f.CheckTitle, f.MessageTitle)
			totalFindings++
		}
	}

	if totalFindings == 0 {
		fmt.Println("No findings.")
	} else {
		fmt.Printf("\nTotal: %d finding(s)\n", totalFindings)
	}
	return nil
}

type cliHealthScope struct {
	Kind       string `json:"kind"`
	Package    string `json:"package,omitempty"`
	ObjectType string `json:"object_type,omitempty"`
	ObjectName string `json:"object_name,omitempty"`
}

type cliHealthSummary struct {
	Status   string `json:"status"`
	Headline string `json:"headline"`
}

type cliHealthSignal struct {
	Status  string         `json:"status"`
	Details map[string]any `json:"details,omitempty"`
}

type cliHealthResult struct {
	Scope            cliHealthScope              `json:"scope"`
	Summary          cliHealthSummary            `json:"summary"`
	Signals          map[string]cliHealthSignal  `json:"signals"`
	TestDetails      *adt.UnitTestResult         `json:"testDetails,omitempty"`
	ATCDetails       *adt.ATCWorklist            `json:"atcDetails,omitempty"`
	CrossingDetails  *graph.CrossingReport       `json:"crossingDetails,omitempty"`
}

func runHealth(cmd *cobra.Command, args []string) error {
	params, err := resolveSystemParams(cmd)
	if err != nil {
		return err
	}
	client, err := getClient(params)
	if err != nil {
		return err
	}

	packageName, _ := cmd.Flags().GetString("package")
	fast, _ := cmd.Flags().GetBool("fast")
	details, _ := cmd.Flags().GetBool("details")
	format, _ := cmd.Flags().GetString("format")
	report, _ := cmd.Flags().GetString("report")
	packageName = strings.ToUpper(strings.TrimSpace(packageName))

	// --report: either bare format ("md"/"html") or a filename ("report.md"/"out.html")
	var reportFile string
	if report != "" {
		if strings.HasSuffix(report, ".md") {
			format = "md"
			reportFile = report
		} else if strings.HasSuffix(report, ".html") {
			format = "html"
			reportFile = report
		} else if report == "md" || report == "html" {
			format = report
		} else {
			return fmt.Errorf("unsupported report format %q (want md, html, or filename ending in .md/.html)", report)
		}
	}

	result := &cliHealthResult{Signals: make(map[string]cliHealthSignal)}

	if packageName != "" {
		result.Scope = cliHealthScope{Kind: "package", Package: packageName}
		populatePackageHealthCLI(context.Background(), client, packageName, fast, result)
	} else {
		if len(args) != 2 {
			return fmt.Errorf("usage: vsp health <type> <name> or vsp health --package <package>")
		}
		objType := strings.ToUpper(args[0])
		objName := strings.ToUpper(args[1])
		result.Scope = cliHealthScope{Kind: "object", ObjectType: objType, ObjectName: objName}
		populateObjectHealthCLI(context.Background(), client, objType, objName, result)
	}

	result.Summary = summarizeCLIHealth(result.Signals)

	// --report: redirect output to file
	if report != "" {
		fileName := reportFile
		if fileName == "" {
			scopeName := packageName
			if scopeName == "" {
				scopeName = strings.ToUpper(args[0]) + "_" + strings.ToUpper(args[1])
			}
			fileName = strings.ReplaceAll(scopeName, "$", "_") + "." + format
		}
		f, err := os.Create(fileName)
		if err != nil {
			return fmt.Errorf("creating report file: %w", err)
		}
		defer f.Close()
		// Redirect stdout to the file for the print functions
		origStdout := os.Stdout
		os.Stdout = f
		defer func() { os.Stdout = origStdout }()

		switch format {
		case "md":
			printCLIHealthMD(result)
		case "html":
			printCLIHealthHTML(result, details)
		}

		os.Stdout = origStdout
		fmt.Fprintf(os.Stderr, "Report saved to %s\n", fileName)
		return nil
	}

	switch format {
	case "json":
		data, err := json.MarshalIndent(result, "", "  ")
		if err != nil {
			return err
		}
		fmt.Println(string(data))
	case "md":
		printCLIHealthMD(result)
	case "html":
		printCLIHealthHTML(result, details)
	default:
		printCLIHealth(result, details)
	}
	return nil
}

func populatePackageHealthCLI(ctx context.Context, client *adt.Client, pkg string, fast bool, result *cliHealthResult) {
	// Verify the package exists before running expensive checks
	pkgContent, err := client.GetPackage(ctx, strings.ToUpper(pkg))
	if err != nil || pkgContent == nil || (len(pkgContent.Objects) == 0 && len(pkgContent.SubPackages) == 0) {
		errMsg := fmt.Sprintf("Package %s not found on this system", pkg)
		if err != nil {
			errMsg = fmt.Sprintf("Package %s: %v", pkg, err)
		}
		result.Signals["tests"] = cliHealthSignal{Status: "ERROR", Details: map[string]any{"message": errMsg}}
		result.Signals["atc"] = cliHealthSignal{Status: "ERROR", Details: map[string]any{"message": errMsg}}
		result.Signals["boundaries"] = cliHealthSignal{Status: "ERROR", Details: map[string]any{"message": errMsg}}
		result.Signals["staleness"] = cliHealthSignal{Status: "ERROR", Details: map[string]any{"message": errMsg}}
		return
	}

	if fast {
		result.Signals["tests"] = cliHealthSignal{Status: "SKIPPED", Details: map[string]any{"reason": "fast mode"}}
		result.Signals["boundaries"] = cliHealthSignal{Status: "SKIPPED", Details: map[string]any{"reason": "fast mode"}}
	} else {
		fmt.Fprintf(os.Stderr, "  [1/4] Running tests...\n")
		testSignal, testDetails := collectPackageTestsWithDetails(ctx, client, pkg)
		result.Signals["tests"] = testSignal
		result.TestDetails = testDetails
		fmt.Fprintf(os.Stderr, "  [2/4] Checking boundaries...\n")
		boundarySignal, crossingReport := collectPackageBoundariesWithDetails(ctx, client, pkg)
		result.Signals["boundaries"] = boundarySignal
		result.CrossingDetails = crossingReport
	}
	step := 3
	if fast {
		step = 1
	}
	fmt.Fprintf(os.Stderr, "  [%d/%d] Running ATC...\n", step, step+1)
	atcSignal, atcDetails := collectPackageATCWithDetails(ctx, client, pkg)
	result.Signals["atc"] = atcSignal
	result.ATCDetails = atcDetails
	fmt.Fprintf(os.Stderr, "  [%d/%d] Checking staleness...\n", step+1, step+1)
	result.Signals["staleness"] = collectPackageStalenessCLI(ctx, client, pkg)
}

func populateObjectHealthCLI(ctx context.Context, client *adt.Client, objType, objName string, result *cliHealthResult) {
	// Verify the object exists before running expensive checks
	_, err := client.GetSource(ctx, objType, objName, nil)
	if err != nil {
		errMsg := fmt.Sprintf("%s %s not found on this system: %v", objType, objName, err)
		result.Signals["tests"] = cliHealthSignal{Status: "ERROR", Details: map[string]any{"message": errMsg}}
		result.Signals["atc"] = cliHealthSignal{Status: "ERROR", Details: map[string]any{"message": errMsg}}
		result.Signals["boundaries"] = cliHealthSignal{Status: "ERROR", Details: map[string]any{"message": errMsg}}
		result.Signals["staleness"] = cliHealthSignal{Status: "ERROR", Details: map[string]any{"message": errMsg}}
		return
	}

	result.Signals["tests"] = collectObjectTestsCLI(ctx, client, objType, objName)
	result.Signals["atc"] = collectObjectATCCLI(ctx, client, objType, objName)
	result.Signals["boundaries"] = collectObjectBoundariesCLI(ctx, client, objType, objName)
	result.Signals["staleness"] = collectObjectStalenessCLI(ctx, client, objType, objName)
}

func collectObjectTestsCLI(ctx context.Context, client *adt.Client, objType, objName string) cliHealthSignal {
	objectURL := buildObjectURL(objType, objName)
	if objectURL == "" {
		return cliHealthSignal{Status: "UNKNOWN"}
	}
	result, err := client.RunUnitTests(ctx, objectURL, nil)
	if err != nil {
		return cliHealthSignal{Status: "ERROR", Details: map[string]any{"message": err.Error()}}
	}
	classes, methods, alerts := summarizeUnitTestsCLI(result)
	status := "PASS"
	if classes == 0 {
		status = "NONE"
	}
	if alerts > 0 {
		status = "FAIL"
	}
	return cliHealthSignal{Status: status, Details: map[string]any{"classes": classes, "methods": methods, "alerts": alerts}}
}

func collectPackageTestsWithDetails(ctx context.Context, client *adt.Client, pkg string) (cliHealthSignal, *adt.UnitTestResult) {
	// Resolve full package hierarchy (TDEVC + prefix fallback) — same as slim/changelog.
	// SAP's test runner only covers the exact package, not subpackages.
	scope, err := AcquirePackageScope(ctx, client, pkg, true)
	if err != nil {
		return cliHealthSignal{Status: "ERROR", Details: map[string]any{"message": err.Error()}}, nil
	}

	packages := scope.Packages
	if len(packages) == 0 {
		packages = []string{strings.ToUpper(pkg)}
	}

	combined := &adt.UnitTestResult{}
	totalClasses, totalMethods, totalAlerts := 0, 0, 0
	for _, p := range packages {
		objectURL := fmt.Sprintf("/sap/bc/adt/packages/%s", p)
		result, err := client.RunUnitTests(ctx, objectURL, nil)
		if err != nil {
			continue
		}
		combined.Classes = append(combined.Classes, result.Classes...)
		c, m, a := summarizeUnitTestsCLI(result)
		totalClasses += c
		totalMethods += m
		totalAlerts += a
	}

	status := "PASS"
	if totalClasses == 0 {
		status = "NONE"
	}
	if totalAlerts > 0 {
		status = "FAIL"
	}
	return cliHealthSignal{Status: status, Details: map[string]any{
		"packages_scanned": len(packages),
		"classes":          totalClasses,
		"methods":          totalMethods,
		"alerts":           totalAlerts,
	}}, combined
}

func summarizeUnitTestsCLI(result *adt.UnitTestResult) (classCount, methodCount, alertCount int) {
	if result == nil {
		return 0, 0, 0
	}
	classCount = len(result.Classes)
	for _, c := range result.Classes {
		methodCount += len(c.TestMethods)
		alertCount += len(c.Alerts)
		for _, m := range c.TestMethods {
			alertCount += len(m.Alerts)
		}
	}
	return classCount, methodCount, alertCount
}

func collectObjectATCCLI(ctx context.Context, client *adt.Client, objType, objName string) cliHealthSignal {
	objectURL := buildObjectURL(objType, objName)
	if objectURL == "" {
		return cliHealthSignal{Status: "UNKNOWN"}
	}
	result, err := client.RunATCCheck(ctx, objectURL, "", 100)
	if err != nil {
		return cliHealthSignal{Status: "ERROR", Details: map[string]any{"message": err.Error()}}
	}
	total, errors, warnings, infos := summarizeATCCLI(result)
	status := "CLEAN"
	if total > 0 {
		status = "FINDINGS"
	}
	return cliHealthSignal{Status: status, Details: map[string]any{"findings": total, "errors": errors, "warnings": warnings, "infos": infos}}
}

func collectPackageATCWithDetails(ctx context.Context, client *adt.Client, pkg string) (cliHealthSignal, *adt.ATCWorklist) {
	objectURL := fmt.Sprintf("/sap/bc/adt/packages/%s", strings.ToUpper(pkg))
	result, err := client.RunATCCheck(ctx, objectURL, "", 200)
	if err != nil {
		return cliHealthSignal{Status: "ERROR", Details: map[string]any{"message": err.Error()}}, nil
	}
	total, errors, warnings, infos := summarizeATCCLI(result)
	status := "CLEAN"
	if total > 0 {
		status = "FINDINGS"
	}
	return cliHealthSignal{Status: status, Details: map[string]any{"findings": total, "errors": errors, "warnings": warnings, "infos": infos}}, result
}

func summarizeATCCLI(result *adt.ATCWorklist) (total, errors, warnings, infos int) {
	if result == nil {
		return 0, 0, 0, 0
	}
	for _, obj := range result.Objects {
		total += len(obj.Findings)
		for _, f := range obj.Findings {
			switch f.Priority {
			case 1:
				errors++
			case 2:
				warnings++
			default:
				infos++
			}
		}
	}
	return total, errors, warnings, infos
}

func collectObjectBoundariesCLI(ctx context.Context, client *adt.Client, objType, objName string) cliHealthSignal {
	if objType != "CLAS" && objType != "PROG" && objType != "INTF" {
		return cliHealthSignal{Status: "UNKNOWN"}
	}
	source, err := client.GetSource(ctx, objType, objName, nil)
	if err != nil || source == "" {
		return cliHealthSignal{Status: "ERROR", Details: map[string]any{"message": "failed to read source"}}
	}
	g := graph.New()
	nodeID := graph.NodeID(objType, objName)
	g.AddNode(&graph.Node{ID: nodeID, Name: objName, Type: objType})
	edges := graph.ExtractDepsFromSource(source, nodeID)
	dynEdges := graph.ExtractDynamicCalls(source, nodeID)
	for _, e := range append(edges, dynEdges...) {
		g.AddEdge(e)
		parts := strings.SplitN(e.To, ":", 2)
		if len(parts) == 2 {
			g.AddNode(&graph.Node{ID: e.To, Name: parts[1], Type: parts[0]})
		}
	}
	resolvePackagesCLI(ctx, client, g)
	n := g.GetNode(nodeID)
	if n == nil || n.Package == "" {
		return cliHealthSignal{Status: "UNKNOWN"}
	}
	report := g.CheckBoundaries(n.Package, &graph.BoundaryOptions{IncludeDynamic: true})
	status := "CLEAN"
	if report.Violations > 0 {
		status = "VIOLATIONS"
	}
	return cliHealthSignal{Status: status, Details: map[string]any{"violations": report.Violations, "crossed_packages": report.CrossedPackages, "dynamic": report.Dynamic}}
}

func collectPackageBoundariesWithDetails(ctx context.Context, client *adt.Client, pkg string) (cliHealthSignal, *graph.CrossingReport) {
	// Resolve full package hierarchy
	scope, err := AcquirePackageScope(ctx, client, pkg, true)
	if err != nil {
		return cliHealthSignal{Status: "ERROR", Details: map[string]any{"message": err.Error()}}, nil
	}

	// Collect objects from all packages in scope
	objects, err := AcquirePackageObjects(ctx, client, ScopeToWhere(scope))
	if err != nil {
		return cliHealthSignal{Status: "ERROR", Details: map[string]any{"message": err.Error()}}, nil
	}

	g := graph.New()
	count := 0
	for _, obj := range objects {
		if !IsSourceBearing(obj.Type) {
			continue
		}
		if count >= 50 {
			break
		}
		fmt.Fprintf(os.Stderr, "\r    [%d] %s %-40s", count+1, obj.Type, obj.Name)
		source, err := client.GetSource(ctx, obj.Type, obj.Name, nil)
		if err != nil {
			fmt.Fprintf(os.Stderr, "\n    WARN: %s %s: %v\n", obj.Type, obj.Name, err)
			continue
		}
		if source == "" {
			continue
		}
		nodeID := graph.NodeID(obj.Type, obj.Name)
		g.AddNode(&graph.Node{ID: nodeID, Name: obj.Name, Type: obj.Type, Package: obj.Package})
		edges := graph.ExtractDepsFromSource(source, nodeID)
		dynEdges := graph.ExtractDynamicCalls(source, nodeID)
		for _, e := range append(edges, dynEdges...) {
			g.AddEdge(e)
			parts := strings.SplitN(e.To, ":", 2)
			if len(parts) == 2 {
				g.AddNode(&graph.Node{ID: e.To, Name: parts[1], Type: parts[0]})
			}
		}
		count++
	}
	if count > 0 {
		fmt.Fprintf(os.Stderr, "\r")
	}

	// Resolve packages for target nodes
	resolvePackagesCLI(ctx, client, g)

	// Directional crossing analysis
	report := graph.AnalyzeCrossings(g, scope, nil)

	status := "CLEAN"
	if report.Sibling > 0 || report.Downward > 0 || report.CommonDown > 0 || len(report.Circular) > 0 {
		status = "VIOLATIONS"
	}

	details := map[string]any{
		"packages_scanned": report.PackagesScanned,
		"objects_scanned":  count,
	}
	if report.Upward > 0 {
		details["upward"] = report.Upward
	}
	if report.Common > 0 {
		details["common"] = report.Common
	}
	if report.Sibling > 0 {
		details["sibling"] = report.Sibling
	}
	if report.Downward > 0 {
		details["downward"] = report.Downward
	}
	if report.CommonDown > 0 {
		details["common_down"] = report.CommonDown
	}
	if report.External > 0 {
		details["external"] = report.External
	}
	if report.Dynamic > 0 {
		details["dynamic"] = report.Dynamic
	}
	if len(report.Circular) > 0 {
		details["circular"] = report.Circular
	}
	return cliHealthSignal{Status: status, Details: details}, report
}

func collectObjectStalenessCLI(ctx context.Context, client *adt.Client, objType, objName string) cliHealthSignal {
	revs, err := client.GetRevisions(ctx, objType, objName, nil)
	if err != nil {
		return cliHealthSignal{Status: "ERROR", Details: map[string]any{"message": err.Error()}}
	}
	return stalenessCLIFromRevisions(revs)
}

func collectPackageStalenessCLI(ctx context.Context, client *adt.Client, pkg string) cliHealthSignal {
	content, err := client.GetPackage(ctx, pkg)
	if err != nil {
		return cliHealthSignal{Status: "ERROR", Details: map[string]any{"message": err.Error()}}
	}
	var newest time.Time
	checked := 0
	for _, obj := range content.Objects {
		objType := strings.ToUpper(obj.Type)
		if objType != "CLAS" && objType != "PROG" && objType != "INTF" {
			continue
		}
		if checked >= 10 {
			break
		}
		revs, err := client.GetRevisions(ctx, objType, obj.Name, nil)
		if err != nil || len(revs) == 0 {
			continue
		}
		tm, err := time.Parse(time.RFC3339, revs[0].Date)
		if err != nil {
			continue
		}
		if tm.After(newest) {
			newest = tm
		}
		checked++
	}
	if newest.IsZero() {
		// Fallback: query latest transport date from E070/E071
		transportQuery := fmt.Sprintf(
			"SELECT MAX( AS4DATE ) AS LAST_DATE FROM E070 WHERE TRKORR IN ( SELECT TRKORR FROM E071 WHERE OBJ_NAME IN ( SELECT OBJ_NAME FROM TADIR WHERE DEVCLASS LIKE '%s%%' ) )", pkg)
		tResult, tErr := client.RunQuery(ctx, transportQuery, 1)
		if tErr == nil && tResult != nil && len(tResult.Rows) > 0 {
			dateStr := strings.TrimSpace(fmt.Sprintf("%v", tResult.Rows[0]["LAST_DATE"]))
			if dateStr != "" && dateStr != "00000000" && len(dateStr) == 8 {
				tm, err := time.Parse("20060102", dateStr)
				if err == nil {
					return stalenessCLIFromTime(tm, 0)
				}
			}
		}
		return cliHealthSignal{Status: "UNKNOWN"}
	}
	return stalenessCLIFromTime(newest, checked)
}

func stalenessCLIFromRevisions(revs []adt.Revision) cliHealthSignal {
	if len(revs) == 0 {
		return cliHealthSignal{Status: "UNKNOWN"}
	}
	tm, err := time.Parse(time.RFC3339, revs[0].Date)
	if err != nil {
		return cliHealthSignal{Status: "ERROR", Details: map[string]any{"message": err.Error()}}
	}
	return stalenessCLIFromTime(tm, 1)
}

func stalenessCLIFromTime(tm time.Time, checked int) cliHealthSignal {
	ageDays := int(time.Since(tm).Hours() / 24)
	status := "ACTIVE"
	switch {
	case ageDays > 365:
		status = "STALE"
	case ageDays > 90:
		status = "AGING"
	}
	return cliHealthSignal{Status: status, Details: map[string]any{"last_changed": tm.Format(time.RFC3339), "age_days": ageDays, "checked": checked}}
}

func summarizeCLIHealth(signals map[string]cliHealthSignal) cliHealthSummary {
	// Check for errors first
	errorCount := 0
	for _, sig := range signals {
		if sig.Status == "ERROR" {
			errorCount++
		}
	}
	if errorCount > 0 {
		return cliHealthSummary{Status: "ERROR", Headline: fmt.Sprintf("%d signal(s) failed — check connection/auth", errorCount)}
	}

	// Detect suspiciously empty results (likely wrong system or auth issue)
	testClasses, _ := signals["tests"].Details["classes"].(int)
	testNone := signals["tests"].Status == "NONE" || testClasses == 0
	boundaryObjs, _ := signals["boundaries"].Details["objects_scanned"].(int)
	boundaryNone := boundaryObjs == 0 && signals["boundaries"].Status != "SKIPPED" && signals["boundaries"].Status != "ERROR"
	if testNone && boundaryNone && signals["staleness"].Status == "UNKNOWN" {
		return cliHealthSummary{Status: "WARN", Headline: "All signals empty — verify correct system and package name"}
	}

	if signals["tests"].Status == "FAIL" {
		return cliHealthSummary{Status: "BAD", Headline: "Unit tests are failing"}
	}
	if signals["boundaries"].Status == "VIOLATIONS" {
		return cliHealthSummary{Status: "WARN", Headline: "Boundary violations detected"}
	}
	if signals["atc"].Status == "FINDINGS" {
		return cliHealthSummary{Status: "WARN", Headline: "ATC findings detected"}
	}
	if signals["staleness"].Status == "STALE" {
		return cliHealthSummary{Status: "WARN", Headline: "Object or package appears stale"}
	}
	return cliHealthSummary{Status: "GOOD", Headline: "No major health issues detected"}
}

func printCLIHealth(result *cliHealthResult, details bool) {
	switch result.Scope.Kind {
	case "package":
		fmt.Printf("Health: package %s\n", result.Scope.Package)
	default:
		fmt.Printf("Health: %s %s\n", result.Scope.ObjectType, result.Scope.ObjectName)
	}
	fmt.Printf("Summary: %s — %s\n\n", result.Summary.Status, result.Summary.Headline)
	for _, key := range []string{"tests", "atc", "boundaries", "staleness"} {
		sig, ok := result.Signals[key]
		if !ok {
			continue
		}
		fmt.Printf("%-11s %s", key+":", sig.Status)
		if len(sig.Details) > 0 {
			data, _ := json.Marshal(sig.Details)
			fmt.Printf(" %s", string(data))
		}
		fmt.Println()
	}

	if !details {
		return
	}

	// Detailed test results — grouped by parent object
	if result.TestDetails != nil && len(result.TestDetails.Classes) > 0 {
		fmt.Printf("\n--- Test Details ---\n\n")
		groups := groupTestsByParent(result.TestDetails.Classes)
		for _, g := range groups {
			fmt.Printf("  %s\n", g.label)
			for _, class := range g.classes {
				className := class.Name
				if className == "" {
					className = "(anonymous)"
				}
				fmt.Printf("    %s\n", className)
				for _, method := range class.TestMethods {
					status := "PASS"
					if len(method.Alerts) > 0 {
						status = "FAIL"
					}
					fmt.Printf("      %-4s  %s (%.3fs)\n", status, method.Name, method.ExecutionTime)
					for _, alert := range method.Alerts {
						fmt.Printf("            %s: %s\n", alert.Kind, alert.Title)
						for _, d := range alert.Details {
							fmt.Printf("              %s\n", d)
						}
					}
				}
				for _, alert := range class.Alerts {
					fmt.Printf("      ALERT %s: %s\n", alert.Kind, alert.Title)
				}
			}
			fmt.Println()
		}
	}

	// Detailed ATC findings
	if result.ATCDetails != nil && len(result.ATCDetails.Objects) > 0 {
		fmt.Printf("\n--- ATC Findings ---\n\n")
		for _, obj := range result.ATCDetails.Objects {
			if len(obj.Findings) == 0 {
				continue
			}
			fmt.Printf("  %s %s (%d findings)\n", obj.Type, obj.Name, len(obj.Findings))
			for _, f := range obj.Findings {
				prio := "INFO"
				switch f.Priority {
				case 1:
					prio = "ERROR"
				case 2:
					prio = "WARN"
				}
				loc := ""
				if f.Location != "" {
					loc = " @ " + f.Location
				}
				fmt.Printf("    %-5s  %s — %s%s\n", prio, f.CheckTitle, f.MessageTitle, loc)
			}
		}
	}

	// Detailed crossing entries
	if result.CrossingDetails != nil && len(result.CrossingDetails.Entries) > 0 {
		fmt.Printf("\n--- Boundary Crossings ---\n\n")

		// Group by direction, show violations first
		dirOrder := []graph.CrossingDirection{
			graph.CrossSibling, graph.CrossDownward, graph.CrossCommonDown,
			graph.CrossExternal, graph.CrossUpward, graph.CrossUpwardSkip, graph.CrossCommon,
		}
		for _, dir := range dirOrder {
			var entries []graph.CrossingEntry
			for _, e := range result.CrossingDetails.Entries {
				if e.Direction == dir {
					entries = append(entries, e)
				}
			}
			if len(entries) == 0 {
				continue
			}
			marker := "OK"
			if dir == graph.CrossSibling || dir == graph.CrossDownward || dir == graph.CrossCommonDown {
				marker = "BAD"
			}
			if dir == graph.CrossExternal {
				marker = "WARN"
			}
			fmt.Printf("  %s  %s (%d)\n", marker, dir, len(entries))
			for _, e := range entries {
				ref := e.EdgeKind
				if e.RefDetail != "" {
					ref += " " + e.RefDetail
				}
				fmt.Printf("    %s → %s  %s %s → %s %s  [%s]\n",
					e.SourcePackage, e.TargetPackage, e.SourceType, e.SourceObject, e.TargetType, e.TargetObject, ref)
			}
		}

		if len(result.CrossingDetails.Circular) > 0 {
			fmt.Printf("\n  CIRCULAR dependencies:\n")
			for _, c := range result.CrossingDetails.Circular {
				fmt.Printf("    %s\n", c)
			}
		}
	}
}

type testGroup struct {
	label   string
	classes []adt.UnitTestClass
}

func groupTestsByParent(classes []adt.UnitTestClass) []testGroup {
	order := []string{}
	groups := map[string]*testGroup{}
	for _, c := range classes {
		key := c.ParentName
		if key == "" {
			key = "(unknown)"
		}
		g, ok := groups[key]
		if !ok {
			label := key
			if c.ParentType != "" {
				label = c.ParentType + " " + key
			}
			g = &testGroup{label: label}
			groups[key] = g
			order = append(order, key)
		}
		g.classes = append(g.classes, c)
	}
	result := make([]testGroup, 0, len(order))
	for _, k := range order {
		result = append(result, *groups[k])
	}
	return result
}

func printCLIHealthMD(result *cliHealthResult) {
	scope := result.Scope.Package
	if scope == "" {
		scope = result.Scope.ObjectType + " " + result.Scope.ObjectName
	}
	fmt.Printf("# Health Report: %s\n\n", scope)
	fmt.Printf("**%s** — %s\n\n", result.Summary.Status, result.Summary.Headline)

	fmt.Print("## Signals\n\n")
	fmt.Println("| Signal | Status | Details |")
	fmt.Println("|--------|--------|---------|")
	for _, key := range []string{"tests", "atc", "boundaries", "staleness"} {
		sig, ok := result.Signals[key]
		if !ok {
			continue
		}
		detailStr := ""
		if len(sig.Details) > 0 {
			parts := make([]string, 0, len(sig.Details))
			for k, v := range sig.Details {
				parts = append(parts, fmt.Sprintf("%s: %v", k, v))
			}
			sort.Strings(parts)
			detailStr = strings.Join(parts, ", ")
		}
		fmt.Printf("| %s | %s | %s |\n", key, sig.Status, detailStr)
	}

	if result.TestDetails != nil && len(result.TestDetails.Classes) > 0 {
		fmt.Print("\n## Test Details\n\n")
		groups := groupTestsByParent(result.TestDetails.Classes)
		for _, g := range groups {
			fmt.Printf("### %s\n\n", g.label)
			for _, class := range g.classes {
				className := class.Name
				if className == "" {
					className = "(anonymous)"
				}
				fmt.Printf("#### %s\n\n", className)
				fmt.Println("| Method | Status | Time | Details |")
				fmt.Println("|--------|--------|------|---------|")
				for _, method := range class.TestMethods {
					status := "PASS"
					detail := ""
					if len(method.Alerts) > 0 {
						status = "FAIL"
						parts := make([]string, 0)
						for _, a := range method.Alerts {
							parts = append(parts, fmt.Sprintf("%s: %s", a.Kind, a.Title))
							parts = append(parts, a.Details...)
						}
						detail = strings.Join(parts, "; ")
					}
					fmt.Printf("| %s | %s | %.3fs | %s |\n", method.Name, status, method.ExecutionTime, detail)
				}
				fmt.Println()
			}
		}
	}

	if result.ATCDetails != nil && len(result.ATCDetails.Objects) > 0 {
		fmt.Print("\n## ATC Findings\n\n")
		for _, obj := range result.ATCDetails.Objects {
			if len(obj.Findings) == 0 {
				continue
			}
			fmt.Printf("### %s %s\n\n", obj.Type, obj.Name)
			fmt.Println("| Priority | Check | Message | Location |")
			fmt.Println("|----------|-------|---------|----------|")
			for _, f := range obj.Findings {
				prio := "Info"
				switch f.Priority {
				case 1:
					prio = "Error"
				case 2:
					prio = "Warning"
				}
				fmt.Printf("| %s | %s | %s | %s |\n", prio, f.CheckTitle, f.MessageTitle, f.Location)
			}
			fmt.Println()
		}
	}

	printCrossingsMD(result.CrossingDetails)
}

func printCrossingsMD(report *graph.CrossingReport) {
	if report == nil || len(report.Entries) == 0 {
		return
	}
	fmt.Print("\n## Boundary Crossings\n\n")

	dirOrder := []graph.CrossingDirection{
		graph.CrossSibling, graph.CrossDownward, graph.CrossCommonDown,
		graph.CrossExternal, graph.CrossUpward, graph.CrossUpwardSkip, graph.CrossCommon,
	}
	for _, dir := range dirOrder {
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

func printCLIHealthHTML(result *cliHealthResult, details bool) {
	scope := result.Scope.Package
	if scope == "" {
		scope = result.Scope.ObjectType + " " + result.Scope.ObjectName
	}

	fmt.Println(`<!DOCTYPE html>
<html><head><meta charset="UTF-8">
<title>Health Report</title>
<style>
  body { font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', sans-serif; max-width: 960px; margin: 2em auto; padding: 0 1em; color: #333; }
  h1 { border-bottom: 2px solid #ddd; padding-bottom: 0.3em; }
  h2 { margin-top: 1.5em; color: #555; }
  h3 { color: #666; }
  table { border-collapse: collapse; width: 100%; margin: 1em 0; }
  th, td { border: 1px solid #ddd; padding: 8px; text-align: left; }
  th { background: #f5f5f5; }
  .PASS, .CLEAN, .GOOD { color: #2e7d32; }
  .FAIL, .BAD, .ERROR { color: #c62828; }
  .WARN, .FINDINGS { color: #ef6c00; }
  .NONE, .UNKNOWN, .SKIPPED { color: #757575; }
  .prio-error { color: #c62828; font-weight: bold; }
  .prio-warn { color: #ef6c00; }
  .prio-info { color: #1565c0; }
  nav.toc { background: #f9f9f9; border: 1px solid #ddd; border-radius: 6px; padding: 0.8em 1.2em; margin-bottom: 1.5em; }
  nav.toc summary { font-weight: bold; cursor: pointer; }
  nav.toc ul { margin: 0.5em 0 0; padding-left: 1.5em; }
  nav.toc li { margin: 0.2em 0; }
  nav.toc a { text-decoration: none; color: #1565c0; }
  nav.toc a:hover { text-decoration: underline; }
</style>
</head><body>`)

	fmt.Printf("<h1>Health Report: %s</h1>\n", scope)
	fmt.Printf("<p><strong class=%q>%s</strong> — %s</p>\n", result.Summary.Status, result.Summary.Status, result.Summary.Headline)

	// Table of contents
	fmt.Println(`<nav class="toc"><details open><summary>Contents</summary><ul>`)
	fmt.Println(`<li><a href="#signals">Signals</a></li>`)
	if result.TestDetails != nil && len(result.TestDetails.Classes) > 0 {
		fmt.Println(`<li><a href="#tests">Test Details</a></li>`)
	}
	if result.ATCDetails != nil && len(result.ATCDetails.Objects) > 0 {
		fmt.Println(`<li><a href="#atc">ATC Findings</a></li>`)
	}
	if result.CrossingDetails != nil && len(result.CrossingDetails.Entries) > 0 {
		fmt.Println(`<li><a href="#boundaries">Boundary Crossings</a></li>`)
	}
	fmt.Println(`</ul></details></nav>`)

	fmt.Println(`<h2 id="signals">Signals</h2>`)
	fmt.Println("<table><tr><th>Signal</th><th>Status</th><th>Details</th></tr>")
	for _, key := range []string{"tests", "atc", "boundaries", "staleness"} {
		sig, ok := result.Signals[key]
		if !ok {
			continue
		}
		detailStr := ""
		if len(sig.Details) > 0 {
			parts := make([]string, 0, len(sig.Details))
			for k, v := range sig.Details {
				parts = append(parts, fmt.Sprintf("%s: %v", k, v))
			}
			sort.Strings(parts)
			detailStr = strings.Join(parts, ", ")
		}
		fmt.Printf("<tr><td>%s</td><td class=%q>%s</td><td>%s</td></tr>\n", key, sig.Status, sig.Status, detailStr)
	}
	fmt.Println("</table>")

	if result.TestDetails != nil && len(result.TestDetails.Classes) > 0 {
		if details {
			fmt.Println(`<h2 id="tests">Test Details</h2>`)
		} else {
			fmt.Println(`<h2 id="tests">Failing Tests</h2>`)
		}
		groups := groupTestsByParent(result.TestDetails.Classes)
		for _, g := range groups {
			// Without --details, skip groups that have no failures
			if !details {
				hasFailures := false
				for _, class := range g.classes {
					for _, method := range class.TestMethods {
						if len(method.Alerts) > 0 {
							hasFailures = true
							break
						}
					}
					if hasFailures {
						break
					}
				}
				if !hasFailures {
					continue
				}
			}
			fmt.Printf("<h3>%s</h3>\n", g.label)
			for _, class := range g.classes {
				// Without --details, skip classes with no failures
				if !details {
					hasClassFailures := false
					for _, method := range class.TestMethods {
						if len(method.Alerts) > 0 {
							hasClassFailures = true
							break
						}
					}
					if !hasClassFailures {
						continue
					}
				}
				className := class.Name
				if className == "" {
					className = "(anonymous)"
				}
				fmt.Printf("<h4>%s</h4>\n", className)
				fmt.Println("<table><tr><th>Method</th><th>Status</th><th>Time</th><th>Details</th></tr>")
				for _, method := range class.TestMethods {
					status := "PASS"
					detail := ""
					if len(method.Alerts) > 0 {
						status = "FAIL"
						parts := make([]string, 0)
						for _, a := range method.Alerts {
							parts = append(parts, fmt.Sprintf("<strong>%s:</strong> %s", a.Kind, a.Title))
							for _, d := range a.Details {
								parts = append(parts, d)
							}
						}
						detail = strings.Join(parts, "<br>")
					}
					// Without --details, only show failing methods
					if !details && status == "PASS" {
						continue
					}
					fmt.Printf("<tr><td>%s</td><td class=%q>%s</td><td>%.3fs</td><td>%s</td></tr>\n", method.Name, status, status, method.ExecutionTime, detail)
				}
				fmt.Println("</table>")
			}
		}
	}

	if result.ATCDetails != nil && len(result.ATCDetails.Objects) > 0 {
		fmt.Println(`<h2 id="atc">ATC Findings</h2>`)
		for _, obj := range result.ATCDetails.Objects {
			if len(obj.Findings) == 0 {
				continue
			}
			fmt.Printf("<h3>%s %s (%d findings)</h3>\n", obj.Type, obj.Name, len(obj.Findings))
			fmt.Println("<table><tr><th>Priority</th><th>Check</th><th>Message</th><th>Location</th></tr>")
			for _, f := range obj.Findings {
				prio := "Info"
				prioClass := "prio-info"
				switch f.Priority {
				case 1:
					prio = "Error"
					prioClass = "prio-error"
				case 2:
					prio = "Warning"
					prioClass = "prio-warn"
				}
				fmt.Printf("<tr><td class=%q>%s</td><td>%s</td><td>%s</td><td>%s</td></tr>\n", prioClass, prio, f.CheckTitle, f.MessageTitle, f.Location)
			}
			fmt.Println("</table>")
		}
	}

	// Crossing details
	if result.CrossingDetails != nil && len(result.CrossingDetails.Entries) > 0 {
		fmt.Println(`<h2 id="boundaries">Boundary Crossings</h2>`)
		dirOrder := []graph.CrossingDirection{
			graph.CrossSibling, graph.CrossDownward, graph.CrossCommonDown,
			graph.CrossExternal, graph.CrossUpward, graph.CrossUpwardSkip, graph.CrossCommon,
		}
		for _, dir := range dirOrder {
			var entries []graph.CrossingEntry
			for _, e := range result.CrossingDetails.Entries {
				if e.Direction == dir {
					entries = append(entries, e)
				}
			}
			if len(entries) == 0 {
				continue
			}
			cssClass := "PASS"
			if dir == graph.CrossSibling || dir == graph.CrossDownward || dir == graph.CrossCommonDown {
				cssClass = "FAIL"
			}
			if dir == graph.CrossExternal {
				cssClass = "WARN"
			}
			fmt.Printf("<h3 class=%q>%s (%d)</h3>\n", cssClass, dir, len(entries))
			fmt.Println("<table><tr><th>From Pkg</th><th>Source</th><th>To Pkg</th><th>Target</th><th>Edge</th><th>Detail</th></tr>")
			for _, e := range entries {
				fmt.Printf("<tr><td>%s</td><td>%s %s</td><td>%s</td><td>%s %s</td><td>%s</td><td>%s</td></tr>\n",
					e.SourcePackage, e.SourceType, e.SourceObject,
					e.TargetPackage, e.TargetType, e.TargetObject,
					e.EdgeKind, e.RefDetail)
			}
			fmt.Println("</table>")
		}
		if len(result.CrossingDetails.Circular) > 0 {
			fmt.Println("<h3 class=\"FAIL\">Circular Dependencies</h3><ul>")
			for _, c := range result.CrossingDetails.Circular {
				fmt.Printf("<li>%s</li>\n", c)
			}
			fmt.Println("</ul>")
		}
	}

	fmt.Println("</body></html>")
}

// resolvePackagesCLI queries TADIR to fill in missing package info and correct
// object types. Two-pass: TADIR for R3TR objects, then TFDIR→TADIR for FMs.
func resolvePackagesCLI(ctx context.Context, client *adt.Client, g *graph.Graph) {
	var names []string
	nodesByName := make(map[string][]*graph.Node)
	for _, n := range g.Nodes() {
		if n.Package == "" && !graph.IsStandardObject(n.Name) && !strings.HasPrefix(n.ID, "DYNAMIC:") {
			names = append(names, n.Name)
			nodesByName[strings.ToUpper(n.Name)] = append(nodesByName[strings.ToUpper(n.Name)], n)
		}
	}
	if len(names) == 0 {
		return
	}

	// Pass 1: TADIR batch lookup
	resolveTADIRcli(ctx, client, names, nodesByName)

	// Pass 2: TFDIR fallback for unresolved nodes (function modules)
	var unresolved []string
	for _, n := range names {
		if nodes, ok := nodesByName[strings.ToUpper(n)]; ok {
			for _, node := range nodes {
				if node.Package == "" {
					unresolved = append(unresolved, strings.ToUpper(n))
					break
				}
			}
		}
	}
	if len(unresolved) > 0 {
		resolveFMviaTFDIRcli(ctx, client, unresolved, nodesByName)
	}
}

func resolveTADIRcli(ctx context.Context, client *adt.Client, names []string, nodesByName map[string][]*graph.Node) {
	// Batch size 5: SAP freestyle query has a ~255 char literal limit for IN clauses
	for start := 0; start < len(names); start += 5 {
		end := start + 5
		if end > len(names) {
			end = len(names)
		}
		chunk := names[start:end]
		quoted := make([]string, len(chunk))
		for i, n := range chunk {
			quoted[i] = "'" + strings.ToUpper(n) + "'"
		}
		query := fmt.Sprintf("SELECT object, obj_name, devclass FROM tadir WHERE pgmid = 'R3TR' AND obj_name IN (%s)", strings.Join(quoted, ","))
		result, err := client.RunQuery(ctx, query, len(chunk)*3)
		if err != nil {
			fmt.Fprintf(os.Stderr, "    WARN: TADIR resolve batch failed: %v\n", err)
			continue
		}
		if result == nil {
			continue
		}
		for _, row := range result.Rows {
			objType := strings.ToUpper(strings.TrimSpace(fmt.Sprintf("%v", row["OBJECT"])))
			objName := strings.ToUpper(strings.TrimSpace(fmt.Sprintf("%v", row["OBJ_NAME"])))
			devclass := strings.ToUpper(strings.TrimSpace(fmt.Sprintf("%v", row["DEVCLASS"])))
			if nodes, ok := nodesByName[objName]; ok {
				for _, n := range nodes {
					n.Package = devclass
					if objType != "" && n.Type != objType {
						n.Type = objType
					}
				}
			}
		}
	}
}

func resolveFMviaTFDIRcli(ctx context.Context, client *adt.Client, fmNames []string, nodesByName map[string][]*graph.Node) {
	fugrSet := make(map[string]bool)
	fmToFugr := make(map[string]string)

	// Batch TFDIR queries (SAP 255-char IN clause limit)
	for start := 0; start < len(fmNames); start += 5 {
		end := start + 5
		if end > len(fmNames) {
			end = len(fmNames)
		}
		batch := fmNames[start:end]
		quoted := make([]string, len(batch))
		for i, n := range batch {
			quoted[i] = "'" + n + "'"
		}
		query := fmt.Sprintf("SELECT FUNCNAME, PNAME FROM TFDIR WHERE FUNCNAME IN (%s)", strings.Join(quoted, ","))
		result, err := client.RunQuery(ctx, query, len(batch)*2)
		if err != nil {
			fmt.Fprintf(os.Stderr, "    WARN: TFDIR resolve batch failed: %v\n", err)
			continue
		}
		if result == nil {
			continue
		}
		for _, row := range result.Rows {
			funcName := strings.ToUpper(strings.TrimSpace(fmt.Sprintf("%v", row["FUNCNAME"])))
			pname := strings.ToUpper(strings.TrimSpace(fmt.Sprintf("%v", row["PNAME"])))
			fugrName := ""
			if strings.HasPrefix(pname, "SAPL") {
				fugrName = pname[4:]
			} else if pname != "" {
				fugrName = pname
			}
			if fugrName != "" {
				fmToFugr[funcName] = fugrName
				fugrSet[fugrName] = true
			}
		}
	}
	if len(fugrSet) == 0 {
		return
	}

	fugrQuoted := make([]string, 0, len(fugrSet))
	for fg := range fugrSet {
		fugrQuoted = append(fugrQuoted, "'"+fg+"'")
	}
	fugrQuery := fmt.Sprintf("SELECT obj_name, devclass FROM tadir WHERE pgmid = 'R3TR' AND object = 'FUGR' AND obj_name IN (%s)", strings.Join(fugrQuoted, ","))
	fugrResult, err := client.RunQuery(ctx, fugrQuery, len(fugrSet)*2)
	if err != nil {
		fmt.Fprintf(os.Stderr, "    WARN: FUGR TADIR resolve failed: %v\n", err)
		return
	}
	if fugrResult == nil {
		return
	}

	fugrPkg := make(map[string]string)
	for _, row := range fugrResult.Rows {
		objName := strings.ToUpper(strings.TrimSpace(fmt.Sprintf("%v", row["OBJ_NAME"])))
		devclass := strings.ToUpper(strings.TrimSpace(fmt.Sprintf("%v", row["DEVCLASS"])))
		fugrPkg[objName] = devclass
	}

	for fmName, fugrName := range fmToFugr {
		if devclass, ok := fugrPkg[fugrName]; ok {
			if nodes, ok := nodesByName[fmName]; ok {
				for _, n := range nodes {
					n.Package = devclass
					n.Type = "FUNC"
				}
			}
		}
	}
}

func runDeploy(cmd *cobra.Command, args []string) error {
	params, err := resolveSystemParams(cmd)
	if err != nil {
		return err
	}

	client, err := getClient(params)
	if err != nil {
		return err
	}

	filePath := args[0]
	packageName := strings.ToUpper(args[1])
	transport, _ := cmd.Flags().GetString("transport")

	ctx := context.Background()
	result, err := client.DeployFromFile(ctx, filePath, packageName, transport)
	if err != nil {
		return fmt.Errorf("deploy failed: %w", err)
	}

	if result.Success {
		action := "Updated"
		if result.Created {
			action = "Created"
		}
		fmt.Fprintf(os.Stderr, "%s %s %s\n", action, result.ObjectType, result.ObjectName)
		if result.ObjectURL != "" {
			fmt.Fprintf(os.Stderr, "URL: %s\n", result.ObjectURL)
		}
		if result.Message != "" {
			fmt.Fprintf(os.Stderr, "%s\n", result.Message)
		}
	} else {
		fmt.Fprintf(os.Stderr, "Deploy failed for %s\n", filePath)
		if result.Message != "" {
			fmt.Fprintf(os.Stderr, "%s\n", result.Message)
		}
		for _, e := range result.Errors {
			fmt.Fprintf(os.Stderr, "  %s\n", e)
		}
		if len(result.SyntaxErrors) > 0 {
			fmt.Fprintf(os.Stderr, "Syntax errors:\n")
			for _, se := range result.SyntaxErrors {
				fmt.Fprintf(os.Stderr, "  %s\n", se)
			}
		}
		return fmt.Errorf("deploy failed")
	}

	return nil
}

func runTransportList(cmd *cobra.Command, args []string) error {
	params, err := resolveSystemParams(cmd)
	if err != nil {
		return err
	}

	client, err := getClient(params)
	if err != nil {
		return err
	}

	user, _ := cmd.Flags().GetString("user")

	ctx := context.Background()
	transports, err := client.ListTransports(ctx, user)
	if err != nil {
		return fmt.Errorf("listing transports failed: %w", err)
	}

	if len(transports) == 0 {
		fmt.Println("No transport requests found.")
		return nil
	}

	fmt.Printf("%-12s %-12s %-8s %-10s %s\n", "NUMBER", "OWNER", "STATUS", "TYPE", "DESCRIPTION")
	fmt.Println(strings.Repeat("-", 80))
	for _, t := range transports {
		status := t.Status
		if t.StatusText != "" {
			status = t.StatusText
		}
		fmt.Printf("%-12s %-12s %-8s %-10s %s\n", t.Number, t.Owner, status, t.Type, t.Description)
	}
	fmt.Printf("\n%d transport(s)\n", len(transports))
	return nil
}

func runTransportGet(cmd *cobra.Command, args []string) error {
	params, err := resolveSystemParams(cmd)
	if err != nil {
		return err
	}

	client, err := getClient(params)
	if err != nil {
		return err
	}

	number := strings.ToUpper(args[0])

	ctx := context.Background()
	details, err := client.GetTransport(ctx, number)
	if err != nil {
		return fmt.Errorf("getting transport failed: %w", err)
	}

	fmt.Printf("Transport: %s\n", details.Number)
	fmt.Printf("Owner:     %s\n", details.Owner)
	fmt.Printf("Type:      %s\n", details.Type)
	fmt.Printf("Status:    %s\n", details.StatusText)
	if details.Target != "" {
		fmt.Printf("Target:    %s\n", details.Target)
	}
	fmt.Printf("Desc:      %s\n", details.Description)

	if len(details.Tasks) > 0 {
		fmt.Printf("\nTasks (%d):\n", len(details.Tasks))
		for _, task := range details.Tasks {
			fmt.Printf("  %s  %-12s  %-8s  %s\n", task.Number, task.Owner, task.StatusText, task.Description)
			for _, obj := range task.Objects {
				fmt.Printf("    %s %s %s\n", obj.PgmID, obj.Type, obj.Name)
			}
		}
	}

	if len(details.Objects) > 0 {
		fmt.Printf("\nObjects (%d):\n", len(details.Objects))
		for _, obj := range details.Objects {
			fmt.Printf("  %s %s %s\n", obj.PgmID, obj.Type, obj.Name)
		}
	}

	return nil
}

// --- install handler implementations ---

func runInstallZadtVsp(cmd *cobra.Command, args []string) error {
	params, err := resolveSystemParams(cmd)
	if err != nil {
		return err
	}

	client, err := getClient(params)
	if err != nil {
		return err
	}

	packageName, _ := cmd.Flags().GetString("package")
	packageName = strings.ToUpper(packageName)
	dryRun, _ := cmd.Flags().GetBool("dry-run")
	skipGitService, _ := cmd.Flags().GetBool("skip-git-service")

	// Validate package name
	if !strings.HasPrefix(packageName, "$") {
		return fmt.Errorf("package name must start with $ (local package): %s", packageName)
	}

	ctx := context.Background()

	fmt.Fprintf(os.Stderr, "ZADT_VSP Installation\n")
	fmt.Fprintf(os.Stderr, "=====================\n\n")

	// Phase 1: Check prerequisites
	fmt.Fprintf(os.Stderr, "Checking prerequisites...\n")

	// Check if package exists.
	// GetPackage reads the nodestructure API and cannot distinguish
	// "package does not exist" from "package exists but has no children",
	// so we use the direct PackageExists probe here. If the probe itself
	// errors (5xx, network), we fall through to the create path and let
	// SAP's own error surface there.
	packageExists, err := client.PackageExists(ctx, packageName)
	if err != nil {
		fmt.Fprintf(os.Stderr, "  Package %s existence check failed: %v — will attempt create\n", packageName, err)
		packageExists = false
	}
	if packageExists {
		fmt.Fprintf(os.Stderr, "  Package %s exists\n", packageName)
	} else {
		fmt.Fprintf(os.Stderr, "  Package %s will be created\n", packageName)
	}

	// Check for abapGit (for Git service)
	hasAbapGit := false
	if !skipGitService {
		results, err := client.SearchObject(ctx, "ZCL_ABAPGIT_OBJECTS", 1)
		if err == nil && len(results) > 0 {
			hasAbapGit = true
			fmt.Fprintf(os.Stderr, "  abapGit detected -> Git service will be deployed\n")
		} else {
			fmt.Fprintf(os.Stderr, "  abapGit not detected -> Git service will be skipped\n")
			skipGitService = true
		}
	} else {
		fmt.Fprintf(os.Stderr, "  Git service skipped (--skip-git-service)\n")
	}

	// Get objects to deploy
	objects := embedded.GetObjects()

	// Check existing objects
	existingObjects := []string{}
	for _, obj := range objects {
		results, err := client.SearchObject(ctx, obj.Name, 1)
		if err == nil && len(results) > 0 && results[0].Name == obj.Name {
			existingObjects = append(existingObjects, obj.Name)
		}
	}
	if len(existingObjects) > 0 {
		fmt.Fprintf(os.Stderr, "  Existing objects will be updated: %s\n", strings.Join(existingObjects, ", "))
	}

	fmt.Fprintf(os.Stderr, "\n")

	// Show deployment plan
	fmt.Fprintf(os.Stderr, "Deployment Plan (%d objects):\n", len(objects))
	fmt.Fprintf(os.Stderr, "%s\n", strings.Repeat("-", 60))
	for i, obj := range objects {
		if obj.Name == "ZCL_VSP_GIT_SERVICE" && skipGitService {
			fmt.Fprintf(os.Stderr, "  [%d/%d] %-30s SKIP (no abapGit)\n", i+1, len(objects), obj.Name)
		} else {
			action := "CREATE"
			for _, existing := range existingObjects {
				if existing == obj.Name {
					action = "UPDATE"
					break
				}
			}
			fmt.Fprintf(os.Stderr, "  [%d/%d] %-30s %s - %s\n", i+1, len(objects), obj.Name, action, obj.Description)
		}
	}
	fmt.Fprintf(os.Stderr, "%s\n\n", strings.Repeat("-", 60))

	if dryRun {
		fmt.Fprintf(os.Stderr, "Dry run - no changes made.\n")
		return nil
	}

	// Phase 2: Create package if needed
	if !packageExists {
		fmt.Fprintf(os.Stderr, "Creating package %s...\n", packageName)
		err := client.CreateObject(ctx, adt.CreateObjectOptions{
			ObjectType:  adt.ObjectTypePackage,
			Name:        packageName,
			Description: "VSP WebSocket Handler",
		})
		if err != nil {
			return fmt.Errorf("failed to create package %s: %w", packageName, err)
		}
		fmt.Fprintf(os.Stderr, "  Package created\n\n")
	} else {
		fmt.Fprintf(os.Stderr, "Using existing package %s\n\n", packageName)
	}

	// Phase 3: Deploy objects
	fmt.Fprintf(os.Stderr, "Deploying ABAP objects...\n")

	deployed := 0
	skipped := 0
	failed := 0

	for i, obj := range objects {
		// Skip Git service if no abapGit
		if obj.Name == "ZCL_VSP_GIT_SERVICE" && skipGitService {
			fmt.Fprintf(os.Stderr, "  [%d/%d] %s ... SKIPPED (no abapGit)\n", i+1, len(objects), obj.Name)
			skipped++
			continue
		}

		fmt.Fprintf(os.Stderr, "  [%d/%d] %s ... ", i+1, len(objects), obj.Name)

		opts := &adt.WriteSourceOptions{
			Package:     packageName,
			Description: obj.Description,
			Mode:        adt.WriteModeUpsert,
		}
		res, err := client.WriteSource(ctx, obj.Type, obj.Name, obj.Source, opts)
		switch {
		case err != nil:
			fmt.Fprintf(os.Stderr, "FAILED: %v\n", err)
			failed++
		case res == nil || !res.Success:
			msg := "unknown failure"
			if res != nil && res.Message != "" {
				msg = res.Message
			}
			fmt.Fprintf(os.Stderr, "FAILED: %s\n", msg)
			failed++
		default:
			fmt.Fprintf(os.Stderr, "OK\n")
			deployed++
		}
	}

	fmt.Fprintf(os.Stderr, "\n")

	// Summary
	if failed > 0 {
		fmt.Fprintf(os.Stderr, "DEPLOYMENT PARTIALLY FAILED\n")
		fmt.Fprintf(os.Stderr, "Deployed: %d, Skipped: %d, Failed: %d\n\n", deployed, skipped, failed)
	} else {
		fmt.Fprintf(os.Stderr, "DEPLOYMENT COMPLETE\n")
		fmt.Fprintf(os.Stderr, "Deployed: %d, Skipped: %d\n\n", deployed, skipped)
	}

	// Post-deployment instructions
	fmt.Fprint(os.Stderr, embedded.PostDeploymentInstructions())

	// Features summary
	fmt.Fprintf(os.Stderr, "\nFeatures unlocked:\n")
	fmt.Fprintf(os.Stderr, "  WebSocket debugging (TPDAPI)\n")
	fmt.Fprintf(os.Stderr, "  RFC/BAPI execution\n")
	fmt.Fprintf(os.Stderr, "  AMDP debugging (experimental)\n")
	if hasAbapGit && !skipGitService {
		fmt.Fprintf(os.Stderr, "  abapGit export (158 object types)\n")
	} else {
		fmt.Fprintf(os.Stderr, "  abapGit export NOT available (install abapGit first)\n")
	}

	if failed > 0 {
		return fmt.Errorf("%d object(s) failed to deploy", failed)
	}
	return nil
}

func runInstallAbapGit(cmd *cobra.Command, args []string) error {
	params, err := resolveSystemParams(cmd)
	if err != nil {
		return err
	}

	client, err := getClient(params)
	if err != nil {
		return err
	}

	edition, _ := cmd.Flags().GetString("edition")
	edition = strings.ToLower(edition)
	packageName, _ := cmd.Flags().GetString("package")
	dryRun, _ := cmd.Flags().GetBool("dry-run")

	// Validate edition
	if edition != "standalone" && edition != "full" {
		return fmt.Errorf("invalid edition '%s'. Use 'standalone' or 'full'", edition)
	}

	// Map edition to dependency name
	depName := "abapgit-standalone"
	if edition == "full" {
		depName = "abapgit-full"
	}

	// Set default package based on edition
	if packageName == "" {
		if edition == "standalone" {
			packageName = "$ABAPGIT"
		} else {
			packageName = "$ZGIT"
		}
	}
	packageName = strings.ToUpper(packageName)

	// Validate package name
	if !strings.HasPrefix(packageName, "$") {
		return fmt.Errorf("package name must start with $ (local package): %s", packageName)
	}

	fmt.Fprintf(os.Stderr, "Install abapGit (%s edition)\n", edition)
	fmt.Fprintf(os.Stderr, "============================\n\n")

	// Get ZIP data
	zipData := deps.GetDependencyZIP(depName)
	if zipData == nil || len(zipData) == 0 {
		fmt.Fprintf(os.Stderr, "ZIP not embedded for edition '%s'\n\n", edition)
		fmt.Fprintf(os.Stderr, "To embed abapGit:\n")
		fmt.Fprintf(os.Stderr, "1. On a system with abapGit installed, run:\n")
		if edition == "standalone" {
			fmt.Fprintf(os.Stderr, "   vsp export '$ABAPGIT' -o abapgit-standalone.zip\n")
		} else {
			fmt.Fprintf(os.Stderr, "   vsp export '$ZGIT' -o abapgit-full.zip\n")
		}
		fmt.Fprintf(os.Stderr, "\n2. Place ZIP in embedded/deps/\n")
		fmt.Fprintf(os.Stderr, "3. Update embedded/deps/embed.go with go:embed directive\n")
		fmt.Fprintf(os.Stderr, "4. Rebuild vsp\n\n")
		fmt.Fprintf(os.Stderr, "Alternative: Download from https://github.com/abapGit/abapGit\n")
		return fmt.Errorf("embedded ZIP not available for edition '%s'", edition)
	}

	fmt.Fprintf(os.Stderr, "Source: %s (embedded, %d bytes)\n", depName, len(zipData))
	fmt.Fprintf(os.Stderr, "Target: %s\n\n", packageName)

	// Parse ZIP
	files, err := deps.UnzipInMemory(zipData)
	if err != nil {
		return fmt.Errorf("failed to parse ZIP: %w", err)
	}

	// Create deployment plan
	plan := deps.CreateDeploymentPlan(depName, packageName, files)
	fmt.Fprintf(os.Stderr, "Found %d objects in %d files\n\n", plan.TotalObjects, plan.TotalFiles)

	// Show deployment plan
	fmt.Fprintf(os.Stderr, "Deployment Plan (%d objects):\n", plan.TotalObjects)
	fmt.Fprintf(os.Stderr, "%s\n", strings.Repeat("-", 60))
	for i, obj := range plan.Objects {
		includeInfo := ""
		if len(obj.Includes) > 0 {
			var incTypes []string
			for t := range obj.Includes {
				incTypes = append(incTypes, t)
			}
			includeInfo = fmt.Sprintf(" [+%s]", strings.Join(incTypes, ","))
		}
		fmt.Fprintf(os.Stderr, "  [%d/%d] %-6s %-40s%s\n", i+1, plan.TotalObjects, obj.Type, obj.Name, includeInfo)
	}
	fmt.Fprintf(os.Stderr, "%s\n\n", strings.Repeat("-", 60))

	if dryRun {
		fmt.Fprintf(os.Stderr, "Dry run - no changes made.\n")
		return nil
	}

	ctx := context.Background()

	// Ensure package exists. PackageExists probes /packages/{name} directly;
	// a GetPackage-based check cannot distinguish "absent" from "present but
	// empty" because nodestructure returns an empty tree in both cases.
	fmt.Fprintf(os.Stderr, "Checking package %s...\n", packageName)
	exists, pkgErr := client.PackageExists(ctx, packageName)
	if pkgErr != nil {
		fmt.Fprintf(os.Stderr, "  Package existence check failed: %v — will attempt create\n", pkgErr)
		exists = false
	}
	if !exists {
		fmt.Fprintf(os.Stderr, "Creating package %s...\n", packageName)
		err = client.CreateObject(ctx, adt.CreateObjectOptions{
			ObjectType:  adt.ObjectTypePackage,
			Name:        packageName,
			Description: fmt.Sprintf("abapGit %s edition", edition),
		})
		if err != nil {
			return fmt.Errorf("failed to create package: %w", err)
		}
		fmt.Fprintf(os.Stderr, "  Package created\n")
	} else {
		fmt.Fprintf(os.Stderr, "  Package exists\n")
	}

	// Deploy objects
	fmt.Fprintf(os.Stderr, "\nDeploying objects...\n")
	success := 0
	failCount := 0

	for i, obj := range plan.Objects {
		if obj.MainSource == "" {
			continue // skip XML-only entries
		}

		fmt.Fprintf(os.Stderr, "  [%d/%d] %s %s ... ", i+1, plan.TotalObjects, obj.Type, obj.Name)

		desc := obj.Description
		if desc == "" {
			desc = fmt.Sprintf("Deployed: %s", obj.Name)
		}

		wopts := &adt.WriteSourceOptions{
			Package:     packageName,
			Description: desc,
			Mode:        adt.WriteModeUpsert,
		}
		_, err := client.WriteSource(ctx, obj.Type, obj.Name, obj.MainSource, wopts)
		if err != nil {
			fmt.Fprintf(os.Stderr, "FAILED: %v\n", err)
			failCount++
		} else {
			fmt.Fprintf(os.Stderr, "OK\n")
			success++
		}
	}

	fmt.Fprintf(os.Stderr, "\nDeployment complete: %d success, %d failed\n", success, failCount)

	if failCount > 0 {
		return fmt.Errorf("%d object(s) failed to deploy", failCount)
	}
	return nil
}

func runInstallList(_ *cobra.Command, _ []string) error {
	fmt.Println("Available Installable Components")
	fmt.Println("================================")
	fmt.Println()

	// ZADT_VSP
	objects := embedded.GetObjects()
	fmt.Printf("1. zadt-vsp\n")
	fmt.Printf("   Description: ZADT_VSP WebSocket handler for advanced features\n")
	fmt.Printf("   Default package: $ZADT_VSP\n")
	fmt.Printf("   Objects: %d (1 interface, %d classes)\n", len(objects), len(objects)-1)
	fmt.Printf("   Status: Embedded (always available)\n")
	fmt.Printf("   Install: vsp install zadt-vsp\n")
	fmt.Println()

	// Embedded dependencies (abapGit editions)
	dependencies := deps.GetAvailableDependencies()
	for i, dep := range dependencies {
		status := "Not embedded (placeholder)"
		if dep.Available {
			status = "Available"
		}
		fmt.Printf("%d. %s\n", i+2, dep.Name)
		fmt.Printf("   Description: %s\n", dep.Description)
		fmt.Printf("   Default package: %s\n", dep.Package)
		fmt.Printf("   Status: %s\n", status)
		if dep.Available {
			edition := "standalone"
			if strings.Contains(dep.Name, "full") {
				edition = "full"
			}
			fmt.Printf("   Install: vsp install abapgit --edition %s\n", edition)
		}
		fmt.Println()
	}

	return nil
}

// buildObjectURL constructs an ADT object URL from type and name.
func buildObjectURL(objType, name string) string {
	name = strings.ToLower(name)
	switch objType {
	case "CLAS":
		return fmt.Sprintf("/sap/bc/adt/oo/classes/%s", name)
	case "PROG":
		return fmt.Sprintf("/sap/bc/adt/programs/programs/%s", name)
	case "INTF":
		return fmt.Sprintf("/sap/bc/adt/oo/interfaces/%s", name)
	case "FUGR":
		return fmt.Sprintf("/sap/bc/adt/functions/groups/%s", name)
	case "DDLS":
		return fmt.Sprintf("/sap/bc/adt/ddic/ddl/sources/%s", name)
	default:
		return ""
	}
}
