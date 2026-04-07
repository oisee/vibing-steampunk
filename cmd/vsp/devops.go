package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
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
	healthCmd.Flags().String("format", "text", "Output format: text or json")

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
	rootCmd.AddCommand(deployCmd)
	rootCmd.AddCommand(transportCmd)
	rootCmd.AddCommand(contextCmd)
	rootCmd.AddCommand(installCmd)

	// Transport subcommands
	transportCmd.AddCommand(transportListCmd)
	transportCmd.AddCommand(transportGetCmd)
}

// --- handler implementations ---

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
	Scope   cliHealthScope              `json:"scope"`
	Summary cliHealthSummary            `json:"summary"`
	Signals map[string]cliHealthSignal  `json:"signals"`
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
	format, _ := cmd.Flags().GetString("format")
	packageName = strings.ToUpper(strings.TrimSpace(packageName))

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

	if format == "json" {
		data, err := json.MarshalIndent(result, "", "  ")
		if err != nil {
			return err
		}
		fmt.Println(string(data))
		return nil
	}

	printCLIHealth(result)
	return nil
}

func populatePackageHealthCLI(ctx context.Context, client *adt.Client, pkg string, fast bool, result *cliHealthResult) {
	if fast {
		result.Signals["tests"] = cliHealthSignal{Status: "SKIPPED", Details: map[string]any{"reason": "fast mode"}}
		result.Signals["boundaries"] = cliHealthSignal{Status: "SKIPPED", Details: map[string]any{"reason": "fast mode"}}
	} else {
		fmt.Fprintf(os.Stderr, "  [1/4] Running tests...\n")
		result.Signals["tests"] = collectPackageTestsCLI(ctx, client, pkg)
		fmt.Fprintf(os.Stderr, "  [2/4] Checking boundaries...\n")
		result.Signals["boundaries"] = collectPackageBoundariesCLI(ctx, client, pkg)
	}
	step := 3
	if fast {
		step = 1
	}
	fmt.Fprintf(os.Stderr, "  [%d/%d] Running ATC...\n", step, step+1)
	result.Signals["atc"] = collectPackageATCCLI(ctx, client, pkg)
	fmt.Fprintf(os.Stderr, "  [%d/%d] Checking staleness...\n", step+1, step+1)
	result.Signals["staleness"] = collectPackageStalenessCLI(ctx, client, pkg)
}

func populateObjectHealthCLI(ctx context.Context, client *adt.Client, objType, objName string, result *cliHealthResult) {
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

func collectPackageTestsCLI(ctx context.Context, client *adt.Client, pkg string) cliHealthSignal {
	content, err := client.GetPackage(ctx, pkg)
	if err != nil {
		return cliHealthSignal{Status: "ERROR", Details: map[string]any{"message": err.Error()}}
	}
	var testClasses []adt.PackageObject
	for _, obj := range content.Objects {
		if strings.ToUpper(obj.Type) == "CLAS" && graph.IsTestCaller(obj.Name, "") {
			testClasses = append(testClasses, obj)
		}
	}
	if len(testClasses) == 0 {
		return cliHealthSignal{Status: "NONE"}
	}
	limit := 5
	if len(testClasses) < limit {
		limit = len(testClasses)
	}
	totalClasses, totalMethods, totalAlerts := 0, 0, 0
	for _, obj := range testClasses[:limit] {
		objectURL := buildObjectURL("CLAS", obj.Name)
		result, err := client.RunUnitTests(ctx, objectURL, nil)
		if err != nil {
			continue
		}
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
		"test_classes_found": len(testClasses),
		"test_classes_run":   limit,
		"classes":            totalClasses,
		"methods":            totalMethods,
		"alerts":             totalAlerts,
	}}
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

func collectPackageATCCLI(ctx context.Context, client *adt.Client, pkg string) cliHealthSignal {
	objectURL := fmt.Sprintf("/sap/bc/adt/packages/%s", strings.ToUpper(pkg))
	result, err := client.RunATCCheck(ctx, objectURL, "", 200)
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

func collectPackageBoundariesCLI(ctx context.Context, client *adt.Client, pkg string) cliHealthSignal {
	content, err := client.GetPackage(ctx, pkg)
	if err != nil {
		return cliHealthSignal{Status: "ERROR", Details: map[string]any{"message": err.Error()}}
	}
	g := graph.New()
	count := 0
	for _, obj := range content.Objects {
		objType := strings.ToUpper(obj.Type)
		if objType != "CLAS" && objType != "PROG" && objType != "INTF" {
			continue
		}
		if count >= 30 {
			break
		}
		source, err := client.GetSource(ctx, objType, obj.Name, nil)
		if err != nil || source == "" {
			continue
		}
		nodeID := graph.NodeID(objType, obj.Name)
		g.AddNode(&graph.Node{ID: nodeID, Name: obj.Name, Type: objType, Package: pkg})
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
	resolvePackagesCLI(ctx, client, g)
	report := g.CheckBoundaries(pkg, &graph.BoundaryOptions{IncludeDynamic: true})
	status := "CLEAN"
	if report.Violations > 0 {
		status = "VIOLATIONS"
	}
	return cliHealthSignal{Status: status, Details: map[string]any{"scanned_objects": count, "violations": report.Violations, "crossed_packages": report.CrossedPackages}}
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

func printCLIHealth(result *cliHealthResult) {
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
}

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
	if len(names) > 100 {
		names = names[:100]
	}
	quoted := make([]string, len(names))
	for i, n := range names {
		quoted[i] = "'" + strings.ToUpper(n) + "'"
	}
	query := fmt.Sprintf("SELECT obj_name, devclass FROM tadir WHERE pgmid = 'R3TR' AND obj_name IN (%s)", strings.Join(quoted, ","))
	result, err := client.RunQuery(ctx, query, 0)
	if err != nil || result == nil {
		return
	}
	for _, row := range result.Rows {
		objName := strings.ToUpper(strings.TrimSpace(fmt.Sprintf("%v", row["OBJ_NAME"])))
		devclass := strings.ToUpper(strings.TrimSpace(fmt.Sprintf("%v", row["DEVCLASS"])))
		if nodes, ok := nodesByName[objName]; ok {
			for _, n := range nodes {
				if n.Package == "" {
					n.Package = devclass
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

	// Check if package exists
	packageExists := false
	pkg, err := client.GetPackage(ctx, packageName)
	if err == nil && pkg.URI != "" {
		packageExists = true
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
			Package: packageName,
			Mode:    adt.WriteModeUpsert,
		}
		_, err := client.WriteSource(ctx, obj.Type, obj.Name, obj.Source, opts)
		if err != nil {
			fmt.Fprintf(os.Stderr, "FAILED: %v\n", err)
			failed++
		} else {
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

	// Ensure package exists
	fmt.Fprintf(os.Stderr, "Checking package %s...\n", packageName)
	pkg, pkgErr := client.GetPackage(ctx, packageName)
	if pkgErr != nil || pkg.URI == "" {
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
