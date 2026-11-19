package vsp

import (
	"context"
	"fmt"
	"net/url"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	"encoding/json"

	"github.com/oisee/vibing-steampunk/pkg/abaplint"
	"github.com/oisee/vibing-steampunk/pkg/adt"
	"github.com/oisee/vibing-steampunk/pkg/graph"
	"github.com/oisee/vibing-steampunk/pkg/graph/adtsource"
	"github.com/spf13/cobra"
)

// --- query command ---

var queryCmd = &cobra.Command{
	Use:   "query <table> [--where ...] [--top N] [--skip N]",
	Short: "Query SAP table contents",
	Long: `Query SAP database table contents via ADT.
Works with standard ADT — no ZADT_VSP required.

Examples:
  vsp query T000
  vsp query T000 --top 5
  vsp query USR02 --where "BNAME = 'DEVELOPER'" --top 10
  vsp query DD03L --where "TABNAME = 'MARA'" --fields "FIELDNAME,DATATYPE,LENG"
  vsp -s a4h query TADIR --where "DEVCLASS = '$TMP'" --top 20`,
	Args: cobra.ExactArgs(1),
	RunE: runQuery,
}

// --- grep command ---

var grepCmd = &cobra.Command{
	Use:   "grep <pattern> --package <package>",
	Short: "Search source code in packages",
	Long: `Search for patterns in ABAP source code across packages.
Works with standard ADT — no ZADT_VSP required.

Examples:
  vsp grep "SELECT.*FROM.*mara" --package '$TMP'
  vsp grep "TYPE REF TO" --package 'ZFINANCE' -i
  vsp -s a4h grep "cl_abap_unit" --package '$ZADT' --type CLAS`,
	Args: cobra.ExactArgs(1),
	RunE: runGrep,
}

// --- system command ---

var systemInfoCmd = &cobra.Command{
	Use:   "info",
	Short: "Show SAP system information",
	Long: `Display SAP system information including version, components, and kernel.
Works with standard ADT — no ZADT_VSP required.

Examples:
  vsp system info
  vsp -s a4h system info`,
	RunE: runSystemInfo,
}

var systemCmd = &cobra.Command{
	Use:   "system",
	Short: "System information and management",
}

// --- lint command ---

var lintCmd = &cobra.Command{
	Use:   "lint <type> <name>",
	Short: "Run ABAP lint checks",
	Long: `Run local ABAP lint checks on source code.
Fully offline — no SAP connection required for local files.
When connected to SAP, fetches source automatically.

Rules: line_length, empty_statement, obsolete_statement,
  max_one_statement, preferred_compare_operator, naming conventions.

Examples:
  vsp lint CLAS ZCL_MY_CLASS
  vsp lint PROG ZTEST_REPORT
  vsp lint --file myclass.clas.abap
  vsp lint --stdin < source.abap`,
	RunE: runLint,
}

// --- execute command ---

var executeCmd = &cobra.Command{
	Use:   "execute [code|file]",
	Short: "Execute ABAP code on SAP",
	Long: `Execute arbitrary ABAP code via ExecuteABAP (unit test wrapper).
Requires write permissions. Code can be inline or from a file.

Examples:
  vsp execute "WRITE 'Hello from CLI'."
  vsp execute --file script.abap
  echo "WRITE sy-datum." | vsp execute --stdin

Code that dies is reported as a failure and exits non-zero. Two different
things have to be looked at for that, because a failing run leaves its
evidence in one of two places:

  in the response   ABAP Unit catches what the code raises — a zero divide,
                    an uncaught exception — and says so in the test result.
                    Nothing reaches ST22 in that case.
  in ST22           what the code starts elsewhere is not caught: SUBMIT of a
                    report that writes a list has no screen to write to in a
                    background session, and dumps in SAPMSSY0 a moment later,
                    long after this request was answered 200.

The second is found by reading the runtime error feed before and after the
run and reporting what appeared in between, which makes it circumstantial:
--no-dump-check turns it off, --dump-wait changes how long to keep looking
(2s by default, since the dump is written after the request was answered;
--dump-wait 0 looks once and moves on).

Note: If ExecuteABAP is blocked by safety settings, you'll see
a clear message explaining what's needed.`,
	// The failure report is printed in full before the error is returned, and
	// a usage screen after it helps nobody.
	SilenceUsage:  true,
	SilenceErrors: true,
	RunE:          runExecute,
}

// --- graph command ---

var graphCmd = &cobra.Command{
	Use:   "graph <type> <name>",
	Short: "Call graph & dependency analysis",
	Long: `Show call graph or run dependency analysis for ABAP objects.
Works with standard ADT — no ZADT_VSP required.

Subcommands:
  vsp graph co-change CLAS ZCL_FOO         — what changes together (transport-based)

Direct usage (call graph):
  vsp graph CLAS ZCL_MY_CLASS
  vsp graph CLAS ZCL_MY_CLASS --direction callers
  vsp graph FUNC Z_MY_FM --direction callers
  vsp graph CLAS ZCL_MY_CLASS --direction callees

--direction callers reads the where-used list SE84 uses, so it reports the
package and the object type rather than a bare list of include names.

--direction callees reads the CROSS and WBCROSSGT cross-reference tables, which
SAP fills at activation. They record references and not calls: a dynamic
CALL METHOD (name) is in no row, and a reference in dead code is in every one.
Rows are marked call or reference so the difference is visible. This direction
needs free SQL; --block-free-sql turns it off and it says so.

Both directions are one hop. --depth is accepted and ignored.`,
	Args: cobra.ExactArgs(2),
	RunE: runGraph,
}

var graphCoChangeCmd = &cobra.Command{
	Use:   "co-change <type> <name>",
	Short: "Find objects that change together (transport-based co-change analysis)",
	Long: `Analyze transport history to find objects that frequently change alongside
the given object. Uses E070/E071 tables via ADT SQL.

Examples:
  vsp graph co-change CLAS ZCL_PRICING
  vsp graph co-change CLAS ZCL_PRICING --top 10
  vsp graph co-change PROG ZREPORT --format json`,
	Args: cobra.ExactArgs(2),
	RunE: runGraphCoChange,
}

var graphWhereUsedConfigCmd = &cobra.Command{
	Use:   "where-used-config <variable>",
	Short: "Find programs that read a TVARVC/STVARV variable (heuristic)",
	Long: `Find ABAP programs/classes that read a specific TVARVC variable.
Uses CROSS table to find candidates, then greps source to confirm.

Results show confidence: HIGH = grep-confirmed, MEDIUM = CROSS-only candidate.

Examples:
  vsp graph where-used-config ZKEKEKE
  vsp graph where-used-config ZKEKEKE --no-grep
  vsp graph where-used-config ZKEKEKE --format json`,
	Args: cobra.ExactArgs(1),
	RunE: runGraphWhereUsedConfig,
}

var methodSigCmd = &cobra.Command{
	Use:   "method-signature <class_name> <method_name>",
	Short: "Show method signature (parameters, types, visibility)",
	Long: `Read a method signature from SAP without fetching the whole class source.
Shows parameters by direction, types, OPTIONAL/DEFAULT, RAISING, and visibility.

Examples:
  vsp method-signature ZCL_TRAVEL GET_DATA
  vsp method-signature ZCL_TRAVEL FACTORY --format json`,
	Args: cobra.ExactArgs(2),
	RunE: runMethodSignature,
}

var classSectionsCmd = &cobra.Command{
	Use:   "class-sections <class_name>",
	Short: "Show class structure organized by PUBLIC/PROTECTED/PRIVATE sections",
	Long: `Read a class structure from SAP and display methods, attributes, types,
and events organized by visibility section.

Examples:
  vsp class-sections ZCL_TRAVEL
  vsp class-sections ZCL_TRAVEL --format json`,
	Args: cobra.ExactArgs(1),
	RunE: runClassSections,
}

var renamePreviewCmd = &cobra.Command{
	Use:   "rename-preview <type> <old_name> <new_name>",
	Short: "Preview what references would be affected by renaming an object (read-only)",
	Long: `Show all static references that would need updating if an object is renamed.
No changes are made — this is a preview/risk assessment tool.

Examples:
  vsp rename-preview CLAS ZCL_OLD_HELPER ZCL_NEW_HELPER
  vsp rename-preview FUNC Z_OLD_FM Z_NEW_FM
  vsp rename-preview PROG ZOLD_REPORT ZNEW_REPORT
  vsp rename-preview CLAS ZCL_FOO ZCL_BAR --format json`,
	Args: cobra.ExactArgs(3),
	RunE: runRenamePreview,
}

var slimCmd = &cobra.Command{
	Use:   "slim <package>",
	Short: "Find dead code in a package (read-only analysis)",
	Long: `Scan a package for dead-code candidates based on static references.

V1 reports custom objects with zero static incoming references in CROSS/WBCROSSGT.
Method-level slimming is planned, but not exposed as reliable v1 output yet.
This is read-only cleanup intelligence, not deletion advice. Dynamic/framework entry
points may still exist and are not fully visible to static indexes yet.

This is read-only: no objects are deleted. Use the report to decide what to clean up.

Examples:
  vsp slim '$ZDEV'
  vsp slim '$ZDEV' --exact-package
  vsp slim '$ZDEV' --format json`,
	Args: cobra.ExactArgs(1),
	RunE: runSlim,
}

var examplesCmd = &cobra.Command{
	Use:   "examples <type> <name>",
	Short: "Find real usage examples of an ABAP object (FM, method, SUBMIT, FORM)",
	Long: `Find concrete usage examples by scanning caller source code.
Shows ranked snippets of how the target is actually called in the codebase.

Supported targets:
  FUNC <fm_name>                         — CALL FUNCTION examples
  CLAS <class> --method <method>         — class method call examples
  INTF <intf> --method <method>          — interface method call examples
  PROG <program> --submit               — SUBMIT program examples
  PROG <program> --form <form_name>     — PERFORM form IN PROGRAM examples

Examples:
  vsp examples FUNC Z_CALCULATE_TAX
  vsp examples CLAS ZCL_TRAVEL --method GET_DATA
  vsp examples INTF ZIF_API --method EXECUTE
  vsp examples PROG ZREPORT --submit
  vsp examples PROG ZPRICING --form CALC_TAX
  vsp examples CLAS ZCL_FOO --method BAR --top 5 --format json`,
	Args: cobra.ExactArgs(2),
	RunE: runExamples,
}

func init() {
	// Examples command (top-level)
	examplesCmd.Flags().String("method", "", "Method name (for CLAS/INTF targets)")
	examplesCmd.Flags().String("form", "", "FORM name (for PROG targets)")
	examplesCmd.Flags().Bool("submit", false, "Find SUBMIT examples (for PROG targets)")
	examplesCmd.Flags().Int("top", 10, "Maximum examples to show")
	examplesCmd.Flags().String("format", "text", "Output format: text or json")
	rootCmd.AddCommand(examplesCmd)

	// Rename preview command (top-level)
	renamePreviewCmd.Flags().String("format", "text", "Output format: text or json")
	rootCmd.AddCommand(renamePreviewCmd)

	// Method signature command (top-level)
	methodSigCmd.Flags().String("format", "text", "Output format: text or json")
	rootCmd.AddCommand(methodSigCmd)

	// Class sections command (top-level)
	classSectionsCmd.Flags().String("format", "text", "Output format: text or json")
	rootCmd.AddCommand(classSectionsCmd)

	// Slim command (top-level)
	slimCmd.Flags().Bool("include-subpackages", true, "Include subpackages")
	slimCmd.Flags().Bool("exact-package", false, "Analyze only the exact package, excluding subpackages")
	slimCmd.Flags().String("format", "text", "Output format: text or json")
	slimCmd.Flags().String("level", "objects", "Analysis depth: objects (fastest), methods (objects + dead methods), full (+ attributes)")
	rootCmd.AddCommand(slimCmd)

	// Graph flags
	graphCmd.Flags().String("direction", "callees", "Direction: callees, callers, or both")
	// --depth is kept so a scripted invocation does not start failing, and it
	// is marked for what it is. Neither source is recursive: the where-used
	// list is one hop by nature and the cross-reference tables are keyed by
	// include, so there was never a traversal for this number to bound.
	graphCmd.Flags().Int("depth", 1, "Accepted and ignored: both directions are one hop")
	_ = graphCmd.Flags().MarkDeprecated("depth", "both directions are one hop; the flag does nothing")
	rootCmd.AddCommand(graphCmd)

	// Graph co-change subcommand
	graphCoChangeCmd.Flags().Int("top", 20, "Maximum results (0=all)")
	graphCoChangeCmd.Flags().String("format", "text", "Output format: text, json, mermaid, or html")
	graphCmd.AddCommand(graphCoChangeCmd)

	// Graph where-used-config subcommand
	graphWhereUsedConfigCmd.Flags().String("format", "text", "Output format: text, json, mermaid, or html")
	graphWhereUsedConfigCmd.Flags().Bool("no-grep", false, "Skip source grep (faster, MEDIUM confidence only)")
	graphCmd.AddCommand(graphWhereUsedConfigCmd)

	// Query flags
	queryCmd.Flags().Int("top", 0, "Maximum number of rows (0=all)")
	queryCmd.Flags().Int("skip", 0, "Skip first N rows")
	queryCmd.Flags().String("where", "", "WHERE clause (e.g. \"BNAME = 'X'\")")
	queryCmd.Flags().String("fields", "", "Comma-separated field list")
	queryCmd.Flags().String("order", "", "ORDER BY clause")

	// Grep flags
	grepCmd.Flags().String("package", "", "Package to search in (required)")
	grepCmd.Flags().BoolP("ignore-case", "i", false, "Case-insensitive search")
	grepCmd.Flags().String("type", "", "Filter by object type (CLAS, PROG, etc.)")
	grepCmd.Flags().Int("max", 100, "Maximum results")
	_ = grepCmd.MarkFlagRequired("package")

	// Lint flags
	lintCmd.Flags().String("file", "", "Lint a local file instead of fetching from SAP")
	lintCmd.Flags().Bool("stdin", false, "Read source from stdin")
	lintCmd.Flags().Int("max-length", 120, "Maximum line length")

	// Execute flags
	executeCmd.Flags().String("file", "", "Read ABAP code from file")
	executeCmd.Flags().Bool("stdin", false, "Read ABAP code from stdin")
	executeCmd.Flags().Bool("json", false, "Print the result as JSON, the same object the execute_abap MCP tool answers (result_text, output, failure, ...)")
	executeCmd.Flags().Bool("no-dump-check", false, "Do not look in ST22 for a runtime error this run may have caused")
	executeCmd.Flags().Duration("dump-wait", 2*time.Second, "How long to keep looking for that runtime error after the code returns")

	// System subcommands
	systemCmd.AddCommand(systemInfoCmd)

	// Register commands
	rootCmd.AddCommand(queryCmd)
	rootCmd.AddCommand(grepCmd)
	rootCmd.AddCommand(systemCmd)
	rootCmd.AddCommand(lintCmd)
	rootCmd.AddCommand(executeCmd)
}

// --- handlers ---

func runQuery(cmd *cobra.Command, args []string) error {
	params, err := resolveSystemParams(cmd)
	if err != nil {
		return err
	}

	client, err := getClient(params)
	if err != nil {
		return err
	}

	table := args[0]
	top, _ := cmd.Flags().GetInt("top")
	skip, _ := cmd.Flags().GetInt("skip")
	topExplicit := cmd.Flags().Changed("top")
	where, _ := cmd.Flags().GetString("where")
	fields, _ := cmd.Flags().GetString("fields")
	orderBy, _ := cmd.Flags().GetString("order")

	// Build SQL query
	fieldList := "*"
	if fields != "" {
		fieldList = fields
	}
	sql := fmt.Sprintf("SELECT %s FROM %s", fieldList, table)
	if where != "" {
		sql += " WHERE " + where
	}
	if orderBy != "" {
		sql += " ORDER BY " + orderBy
	}

	maxRows := adt.ResolveRowLimit(topExplicit && top == 0, top) + skip

	ctx := context.Background()
	result, err := client.RunQuery(ctx, sql, maxRows)
	if err != nil {
		return fmt.Errorf("query failed: %w\n\nNote: RunQuery uses standard ADT API (no ZADT_VSP required)", err)
	}

	// Print results as table
	if result == nil || len(result.Columns) == 0 {
		fmt.Println("No results")
		return nil
	}

	// Skip rows
	startRow := skip
	if startRow >= len(result.Rows) {
		fmt.Println("No results after skip")
		return nil
	}

	// Header — column names
	colNames := make([]string, len(result.Columns))
	for i, c := range result.Columns {
		colNames[i] = c.Name
	}
	fmt.Println(strings.Join(colNames, "\t"))
	fmt.Println(strings.Repeat("-", 80))

	// Rows
	count := 0
	for i := startRow; i < len(result.Rows); i++ {
		if top > 0 && count >= top {
			break
		}
		row := result.Rows[i]
		vals := make([]string, len(colNames))
		for j, col := range colNames {
			vals[j] = fmt.Sprintf("%v", row[col])
		}
		fmt.Println(strings.Join(vals, "\t"))
		count++
	}
	fmt.Fprintf(os.Stderr, "\n%d rows\n", count)
	return nil
}

func runGrep(cmd *cobra.Command, args []string) error {
	params, err := resolveSystemParams(cmd)
	if err != nil {
		return err
	}

	client, err := getClient(params)
	if err != nil {
		return err
	}

	pattern := args[0]
	pkg, _ := cmd.Flags().GetString("package")
	ignoreCase, _ := cmd.Flags().GetBool("ignore-case")
	objType, _ := cmd.Flags().GetString("type")
	max, _ := cmd.Flags().GetInt("max")

	var types []string
	if objType != "" {
		types = strings.Split(objType, ",")
	}

	ctx := context.Background()
	result, err := client.GrepPackage(ctx, pkg, pattern, ignoreCase, types, max)
	if err != nil {
		return fmt.Errorf("grep failed: %w\n\nNote: GrepPackage uses standard ADT search — no ZADT_VSP required", err)
	}

	if result == nil || len(result.Objects) == 0 {
		fmt.Println("No matches")
		return nil
	}

	totalMatches := 0
	for _, obj := range result.Objects {
		for _, m := range obj.Matches {
			fmt.Printf("%s:%d: %s\n", obj.ObjectURL, m.LineNumber, strings.TrimSpace(m.MatchedLine))
			totalMatches++
		}
	}
	fmt.Fprintf(os.Stderr, "\n%d matches in %d objects\n", totalMatches, len(result.Objects))
	return nil
}

func runSystemInfo(cmd *cobra.Command, args []string) error {
	params, err := resolveSystemParams(cmd)
	if err != nil {
		return err
	}

	client, err := getClient(params)
	if err != nil {
		return err
	}

	ctx := context.Background()
	info, err := client.GetSystemInfo(ctx)
	if err != nil {
		return fmt.Errorf("failed to get system info: %w", err)
	}

	fmt.Printf("System:    %s\n", info.SystemID)
	fmt.Printf("Host:      %s\n", info.HostName)
	fmt.Printf("Client:    %s\n", info.Client)
	fmt.Printf("SAP:       %s\n", info.SAPRelease)
	fmt.Printf("ABAP:      %s\n", info.ABAPRelease)
	fmt.Printf("Kernel:    %s\n", info.KernelRelease)
	fmt.Printf("Database:  %s %s\n", info.DatabaseSystem, info.DatabaseRelease)

	// Check ZADT_VSP availability
	_, searchErr := client.SearchObject(ctx, "ZCL_VSP_APC_HANDLER", 1)
	if searchErr != nil {
		fmt.Printf("\nZADT_VSP:  not installed")
		fmt.Printf("\n           Install with: vsp install zadt-vsp\n")
	} else {
		fmt.Printf("\nZADT_VSP:  installed\n")
	}

	return nil
}

func runLint(cmd *cobra.Command, args []string) error {
	file, _ := cmd.Flags().GetString("file")
	stdin, _ := cmd.Flags().GetBool("stdin")
	maxLen, _ := cmd.Flags().GetInt("max-length")

	var source string
	var filename string

	if stdin {
		// Read from stdin
		data, err := readStdin()
		if err != nil {
			return fmt.Errorf("failed to read stdin: %w", err)
		}
		source = string(data)
		filename = "stdin.abap"
	} else if file != "" {
		// Read from file
		data, err := os.ReadFile(file)
		if err != nil {
			return fmt.Errorf("failed to read file: %w", err)
		}
		source = string(data)
		filename = file
	} else {
		// Fetch from SAP
		if len(args) < 2 {
			return fmt.Errorf("usage: vsp lint <type> <name>, or use --file/--stdin")
		}
		params, err := resolveSystemParams(cmd)
		if err != nil {
			return err
		}
		client, err := getClient(params)
		if err != nil {
			return err
		}
		ctx := context.Background()
		src, err := client.GetSource(ctx, args[0], args[1], nil)
		if err != nil {
			return fmt.Errorf("failed to fetch source: %w", err)
		}
		source = src
		filename = args[1]
	}

	// Run linter
	linter := &abaplint.Linter{Rules: []abaplint.Rule{
		&abaplint.LineLengthRule{MaxLength: maxLen},
		&abaplint.EmptyStatementRule{},
		&abaplint.ObsoleteStatementRule{
			Compute: true, Add: true, Subtract: true,
			Multiply: true, Divide: true, Move: true, Refresh: true,
		},
		&abaplint.MaxOneStatementRule{},
		&abaplint.PreferredCompareOperatorRule{
			BadOperators: []string{"EQ", "><", "NE", "GE", "GT", "LT", "LE"},
		},
		&abaplint.ColonMissingSpaceRule{},
		&abaplint.LocalVariableNamesRule{
			ExpectedData:     `^[Ll][VvSsTtRrCc]_\w+$`,
			ExpectedConstant: `^[Ll][Cc]_\w+$`,
			ExpectedFS:       `^<[Ll][VvSsTtRr]_\w+>$`,
		},
	}}

	issues := linter.Run(filename, source)

	if len(issues) == 0 {
		fmt.Fprintf(os.Stderr, "No issues found in %s\n", filename)
		return nil
	}

	for _, iss := range issues {
		severity := "W"
		if iss.Severity == "Error" {
			severity = "E"
		}
		fmt.Printf("%s:%d:%d: %s [%s] %s\n", filename, iss.Row, iss.Col, severity, iss.Key, iss.Message)
	}
	fmt.Fprintf(os.Stderr, "\n%d issues found\n", len(issues))

	// Return error if there are Error-level issues
	for _, iss := range issues {
		if iss.Severity == "Error" {
			return fmt.Errorf("%d issues found", len(issues))
		}
	}
	return nil
}

func runExecute(cmd *cobra.Command, args []string) error {
	file, _ := cmd.Flags().GetString("file")
	stdin, _ := cmd.Flags().GetBool("stdin")

	var code string

	if stdin {
		data, err := readStdin()
		if err != nil {
			return fmt.Errorf("failed to read stdin: %w", err)
		}
		code = string(data)
	} else if file != "" {
		data, err := os.ReadFile(file)
		if err != nil {
			return fmt.Errorf("failed to read file: %w", err)
		}
		code = string(data)
	} else if len(args) > 0 {
		code = args[0]
	} else {
		return fmt.Errorf("usage: vsp execute <code>, or use --file/--stdin")
	}

	params, err := resolveSystemParams(cmd)
	if err != nil {
		return err
	}

	client, err := getClient(params)
	if err != nil {
		return err
	}

	// Ctrl-C cancels the run rather than killing the process, so ExecuteABAP
	// gets to run its deferred delete of the temporary program.
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	// What ST22 already held before any of this ran. It has to be read first —
	// afterwards there is no way to tell an old dump from a new one — and it is
	// only ever advisory, so a system that will not answer for its runtime
	// errors still gets to run the code.
	var watch *adt.DumpWatch
	if skip, _ := cmd.Flags().GetBool("no-dump-check"); !skip {
		w, werr := client.WatchDumps(ctx, strings.ToUpper(strings.TrimSpace(params.User)))
		if werr != nil {
			fmt.Fprintf(os.Stderr, "! ST22 will not be checked for runtime errors: %v\n", werr)
		} else {
			watch = w
		}
	}

	result, err := client.ExecuteABAP(ctx, code, nil)
	if err != nil {
		errStr := err.Error()
		if strings.Contains(errStr, "safety") || strings.Contains(errStr, "blocked") {
			return fmt.Errorf("%w\n\nExecuteABAP requires write permissions.\nCheck --read-only and --allowed-ops settings", err)
		}
		return fmt.Errorf("execute failed: %w\n\nNote: ExecuteABAP wraps code in a unit test class.\nFor advanced execution, use ZADT_VSP WebSocket (vsp install zadt-vsp)", err)
	}

	// Every value, whole and in order, one per line: a value is never cut, and
	// a script reading stdout gets exactly what the code returned. --json gives
	// the MCP tool's object instead, for a caller that needs to tell one value
	// with a line break in it from two.
	if asJSON, _ := cmd.Flags().GetBool("json"); asJSON {
		out, jerr := adt.IndentJSON(result.Lean())
		if jerr != nil {
			return fmt.Errorf("could not encode the result: %w", jerr)
		}
		fmt.Println(string(out))
	} else {
		for _, line := range result.Output {
			fmt.Println(line)
		}
	}
	// ST22 is consulted before anything is said about the failure, because what
	// is said depends on the answer: "ABAP Unit caught this and there is no
	// dump" is a useful thing to know and a wrong thing to claim when a dump is
	// sitting there.
	var dumped []adt.Dump
	st22Read := false
	if watch != nil {
		wait, _ := cmd.Flags().GetDuration("dump-wait")
		fresh, derr := client.AwaitDumps(ctx, watch, wait)
		if derr != nil {
			fmt.Fprintf(os.Stderr, "! ST22 could not be read again: %v\n", derr)
		} else {
			st22Read = true
		}
		dumped = fresh
	}

	// The failure report says all of this and says it better, so the one-line
	// summary would only be the same sentence twice.
	if result.Message != "" && result.Failure == nil {
		fmt.Fprintf(os.Stderr, "%s\n", result.Message)
	}
	if result.Failure != nil {
		reportExecuteFailure(result.Failure, st22Read && len(dumped) == 0)
	}
	if watch != nil {
		reportDumpsAfterRun(dumped, result.ProgramName, watch.User)
	}

	return executeExitError(result, dumped)
}

// executeExitError is what `vsp execute` exits with. The exit code is the part
// scripts read, so every kind of failure has to reach it. The detail is on
// stderr already; this only has to be short and true.
func executeExitError(result *adt.ExecuteABAPResult, dumped []adt.Dump) error {
	switch {
	case result.Failure != nil && result.Failure.Kind == adt.ExecuteFailureSyntax:
		// Said apart from "did not finish", because it did not start. That
		// distinction is the whole of this fix, and the exit line is where a
		// script reads it.
		return fmt.Errorf("the code did not compile, so none of it ran")
	case result.Failure != nil && result.Failure.Kind == adt.ExecuteFailureNotRun:
		return fmt.Errorf("the code cannot be shown to have run")
	case result.Failure != nil && len(dumped) > 0:
		return fmt.Errorf("the code did not finish, and a runtime error appeared while it ran")
	case result.Failure != nil:
		return fmt.Errorf("the code did not finish")
	case len(dumped) > 0:
		return fmt.Errorf("a runtime error appeared while this ran")
	case result.Success && !result.CleanedUp:
		// The code ran, but the temporary program is still in $TMP (the CLI
		// never asks to keep it); the warning naming it is on stderr.
		return fmt.Errorf("the code ran, but its temporary program %s was not deleted", result.ProgramName)
	case !result.Success:
		// No failure from the run, because there was no run: the temporary
		// program could not be created, locked, written or activated. Its
		// message is already on stderr; exiting 0 would call that a success.
		return fmt.Errorf("the code did not run")
	}
	return nil
}

// reportExecuteFailure prints what ABAP Unit caught. This is the certain half:
// the failure was raised inside the code we submitted, and SAP said so in the
// same response.
//
// st22Clear says the runtime error feed was read afterwards and had nothing
// new in it, which is worth passing on — for most of these failures ABAP Unit
// is the only witness and going to look in ST22 finds nothing.
func reportExecuteFailure(failure *adt.ExecuteFailure, st22Clear bool) {
	fmt.Fprintln(os.Stderr)
	title := failure.Title
	where := ""
	if failure.Line > 0 {
		// A syntax message from SAP is a finished sentence and ends in a full
		// stop, which reads as a typo once a clause is bolted onto it.
		title = strings.TrimRight(title, ". ")
		where = fmt.Sprintf(" at line %d of the code you gave me", failure.Line)
	}
	fmt.Fprintf(os.Stderr, "%s%s\n", title, where)
	for _, detail := range failure.Details {
		fmt.Fprintf(os.Stderr, "  %s\n", detail)
	}
	switch failure.Kind {
	case adt.ExecuteFailureSyntax:
		// The sentence below would be a lie here — ABAP Unit caught nothing,
		// because ABAP Unit was never reached. SAP refused the program at
		// activation, so not one statement of the payload was executed, and
		// saying so is what stops the reader hunting for a runtime cause.
		fmt.Fprintln(os.Stderr, "\nSAP refused to activate the generated program, so none of this code ran.")
	case adt.ExecuteFailureNotRun:
		fmt.Fprintln(os.Stderr, "\nThe program activated but no test was reported, so this is not a result — it is a gap.")
	default:
		if st22Clear {
			// Worth saying, because the obvious next move — go and look in ST22
			// — finds nothing at all for this kind of failure: ABAP Unit caught
			// the exception, so no work process ever terminated.
			fmt.Fprintln(os.Stderr, "\nABAP Unit caught this and nothing new reached ST22, so there is no dump to look up.")
		}
	}
}

// reportDumpsAfterRun prints runtime errors that were not in ST22 before the
// run and were after it.
//
// The wording is deliberately weaker than the one above. Nothing here proves
// causation: the evidence is that a dump by this user appeared in the window
// this ran in, which on a busy system is a coincidence waiting to happen. The
// one exception is a dump that names the throwaway program we generated —
// nobody else could have run that — and that one is stated as fact.
func reportDumpsAfterRun(dumps []adt.Dump, programName, user string) {
	if len(dumps) == 0 {
		return
	}
	fmt.Fprintln(os.Stderr)
	ours := false
	for _, d := range dumps {
		if strings.EqualFold(d.Program, programName) {
			ours = true
		}
	}
	if ours {
		fmt.Fprintln(os.Stderr, "This code dumped:")
	} else {
		fmt.Fprintln(os.Stderr, "A runtime error appeared while this ran:")
	}
	for _, d := range dumps {
		fmt.Fprintf(os.Stderr, "  %s  %s in %s (user %s)\n", stamp(d.At), d.ErrorType, d.Program, d.User)
		if d.Message != "" {
			fmt.Fprintf(os.Stderr, "      %s\n", d.Message)
		}
	}
	if !ours {
		who := "the same user"
		if user == "" {
			who = "some user"
		}
		fmt.Fprintf(os.Stderr, "\nThat is a dump by %s inside the window this ran in, which is an\nargument, not a proof — this code may only have started whatever died.\nSee it whole with: vsp dumps --explain latest\n", who)
	}
}

func runGraph(cmd *cobra.Command, args []string) error {
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
	direction, _ := cmd.Flags().GetString("direction")

	// Build object URI (url.PathEscape handles namespaced objects like /UI5/CL_REPOSITORY)
	encodedName := url.PathEscape(strings.ToLower(name))
	var objURI string
	switch objType {
	case "CLAS":
		objURI = "/sap/bc/adt/oo/classes/" + encodedName
	case "PROG":
		objURI = "/sap/bc/adt/programs/programs/" + encodedName
	case "INTF":
		objURI = "/sap/bc/adt/oo/interfaces/" + encodedName
	case "FUGR":
		objURI = "/sap/bc/adt/functions/groups/" + encodedName
	default:
		objURI = "/sap/bc/adt/oo/classes/" + encodedName
	}

	ctx := context.Background()

	// FUNC used to fall into the default and be asked for as a class, which is
	// a 404 that then looks like a call graph the system declined to give. A
	// module is addressed under its group, and only TFDIR knows which group
	// that is.
	if objType == "FUNC" {
		res, qerr := client.RunQuery(ctx,
			fmt.Sprintf("SELECT PNAME FROM TFDIR WHERE FUNCNAME = '%s'", name), 1)
		if qerr != nil || res == nil || len(res.Rows) == 0 {
			return fmt.Errorf("function module %s is not in TFDIR, so its group is unknown", name)
		}
		pool := strings.TrimSpace(fmt.Sprintf("%v", res.Rows[0]["PNAME"]))
		group := strings.TrimPrefix(pool, "SAPL")
		objURI = "/sap/bc/adt/functions/groups/" + url.PathEscape(strings.ToLower(group)) +
			"/fmodules/" + encodedName
	}

	// For transactions: resolve TCODE → program name first
	if objType == "TRAN" || objType == "TCODE" {
		result, err := client.RunQuery(ctx,
			fmt.Sprintf("SELECT PGMNA FROM TSTC WHERE TCODE = '%s'", name), 1)
		if err == nil && result != nil && len(result.Rows) > 0 {
			pgm := fmt.Sprintf("%v", result.Rows[0]["PGMNA"])
			fmt.Fprintf(os.Stderr, "Transaction %s → Program %s\n\n", name, pgm)
			name = strings.TrimSpace(pgm)
			objType = "PROG"
			objURI = "/sap/bc/adt/programs/programs/" + url.PathEscape(strings.ToLower(name))
		} else {
			return fmt.Errorf("transaction %s not found in TSTC", name)
		}
	}

	// Both directions now read a source that exists. The ADT call-graph
	// resource this command used to try first — /sap/bc/adt/cai/callgraph —
	// is advertised on none of 7.50, 7.57 and 7.58 and answers 404, so every
	// run paid for two doomed requests before reaching the tables. What is
	// left is one query per direction and a fallback that only fires on a real
	// failure.
	//
	// An answer of zero is an answer and is not retried. Falling through on an
	// empty list would send more requests to say the same thing under a banner
	// reading "not available" — which would describe a query that had in fact
	// succeeded.
	switch direction {
	case "callers":
		callers, unresolved, err := whereUsedCallers(ctx, client, objURI)
		if err != nil {
			fmt.Fprintf(os.Stderr, "The where-used list could not be read (%v); falling back to the cross-reference tables.\n\n", err)
			return graphFromCross(ctx, client, name, objType, "callers")
		}
		printWhereUsedCallers(callers, unresolved)
		return nil
	case "both":
		fmt.Println("=== CALLEES (what this uses) ===")
		if err := printCalleesOf(ctx, client, objURI, name, objType); err != nil {
			return err
		}
		fmt.Println("\n=== CALLERS (what uses this) ===")
		callers, unresolved, err := whereUsedCallers(ctx, client, objURI)
		if err != nil {
			fmt.Fprintf(os.Stderr, "The where-used list could not be read (%v); falling back to the cross-reference tables.\n\n", err)
			return graphFromCross(ctx, client, name, objType, "callers")
		}
		printWhereUsedCallers(callers, unresolved)
		return nil
	default: // callees
		return printCalleesOf(ctx, client, objURI, name, objType)
	}
}

// printCalleesOf prints what an object reaches, from the cross-reference
// tables. The --depth flag does not reach here on purpose: the tables are keyed
// by include and a second hop means a fresh pair of queries per child, for an
// answer that gets less useful the wider it grows.
func printCalleesOf(ctx context.Context, client *adt.Client, objURI, name, objType string) error {
	callees, gaps, err := client.Callees(ctx, objURI)
	if err != nil {
		// No fallback here, and the reason is worth stating because the code
		// that used to be here looked like resilience.
		//
		// A fallback is only a fallback if it takes a different route. The
		// callee reader is free SQL over CROSS and WBCROSSGT; the scan it fell
		// back to is free SQL over CROSS and WBCROSSGT. It cannot succeed where
		// the first failed — measured: with free SQL blocked, the first call
		// returns a message naming both tables and the reason, and then the
		// scan runs and fails identically with a worse one. Six object types in
		// both directions never reached it at all.
		//
		// The caller path keeps its fallback and should: there the first route
		// is the ADT where-used resource, so the tables really are a second
		// opinion.
		return err
	}
	if note := adt.UnsearchedNote(gaps, 2, "cross-reference table"); note != "" {
		// Printed before the rows, not after: a reader who sees a plausible
		// table first has already drawn the conclusion by the time a footnote
		// tells them it was partial.
		fmt.Printf("  %s\n\n", note)
	}
	if len(callees) == 0 {
		if n := client.InactiveReferenceCount(ctx, objURI); n > 0 {
			// One of the three readings below, decided rather than listed.
			fmt.Printf("  No row for this object's includes — but %d are recorded "+
				"against an inactive version of it, in the index SAP keeps for "+
				"objects with unactivated changes. So this object does reference "+
				"things; the version that was activated here does not.\n", n)
			return nil
		}
		// Empty here is often true. The tables record global types, classes,
		// methods and external calls; a class doing only local work has no
		// row, and neither has a routine that calls nothing outside its own
		// program. The other readings are "never activated on this system" and
		// "no such object", and the sentence names all three because the empty
		// list looks the same in every case.
		fmt.Println("  No row for this object's includes. It may genuinely reference nothing global — " +
			"the tables hold global types, classes, methods and external calls, nothing local. " +
			"It may also never have been activated here, or not exist. Ask about a neighbour that " +
			"should have references to tell which.")
		return nil
	}
	rows := make([][]string, 0, len(callees))
	for _, c := range callees {
		what := "reference"
		if c.Calls {
			what = "call"
		}
		rows = append(rows, []string{c.Name, c.Kind, what, c.Component, c.Source})
	}
	fmt.Print(formatTable([]string{"Object", "Kind", "Call?", "Component", "Table"}, rows))
	fmt.Fprintf(os.Stderr, "\n%d references, recorded at activation — not observed calls.\n", len(callees))
	return nil
}

// graphFromCross is the fallback, reached only when the good reader has already
// failed. Two things are worth knowing before adding anything to it.
//
// It answers at object level. Upward it could do better: WBCROSSGTX.LONG_NAME
// accepts LIKE despite being a STRG column — checked live on 7.58, and the
// table exists on 7.50 too — so a search there yields the hashes under which
// long names are stored in WBCROSSGT, and those rows are otherwise unreachable
// by name. Their includes then decode to methods through DecodeMethodIncludes.
// That was written and deleted rather than shipped, and the reason is the
// second thing.
//
// Nobody knows how often this branch is taken. What is known is what happened
// to it: usage_examples for function modules, SUBMIT and programs came through
// here and returned nothing for its entire existence, because a CROSS query
// asked for 'FU' in a C(1) column and the 400 was read as "no callers found".
// A branch entered rarely rots unwatched. So enriching it puts more code where
// nothing checks it — and if it ever seems worth doing, measure how often
// execution actually lands here first, rather than assuming as this codebase
// has now done three times in a day.
func graphFromCross(ctx context.Context, client *adt.Client, name, objType, direction string) error {
	// Build queries for BOTH cross-reference tables
	// WBCROSSGT: OO references (classes, interfaces, methods, types)
	// CROSS:     procedural references (FORMs, function modules, programs)
	var queries []string

	if direction == "callers" {
		// Who references this object?
		queries = append(queries,
			fmt.Sprintf("SELECT INCLUDE, OTYPE, NAME FROM WBCROSSGT WHERE NAME LIKE '%s%%'", name))
		// Also check CROSS for procedural callers
		queries = append(queries,
			fmt.Sprintf("SELECT INCLUDE, TYPE AS OTYPE, NAME FROM CROSS WHERE NAME LIKE '%s%%'", name))
	} else {
		// What does this object reference? Pattern depends on object type
		switch objType {
		case "CLAS":
			// Class includes: CLASSNAME===========CM001, etc.
			queries = append(queries,
				fmt.Sprintf("SELECT INCLUDE, OTYPE, NAME FROM WBCROSSGT WHERE INCLUDE LIKE '%s%%'", name))
		case "PROG":
			// Programs: direct include name
			queries = append(queries,
				fmt.Sprintf("SELECT INCLUDE, OTYPE, NAME FROM WBCROSSGT WHERE INCLUDE = '%s'", name))
			queries = append(queries,
				fmt.Sprintf("SELECT INCLUDE, TYPE AS OTYPE, NAME FROM CROSS WHERE INCLUDE = '%s'", name))
		case "FUGR":
			// Function group: L<name>* includes
			queries = append(queries,
				fmt.Sprintf("SELECT INCLUDE, OTYPE, NAME FROM WBCROSSGT WHERE INCLUDE LIKE 'L%s%%'", name))
			queries = append(queries,
				fmt.Sprintf("SELECT INCLUDE, TYPE AS OTYPE, NAME FROM CROSS WHERE INCLUDE LIKE 'L%s%%'", name))
		default:
			queries = append(queries,
				fmt.Sprintf("SELECT INCLUDE, OTYPE, NAME FROM WBCROSSGT WHERE INCLUDE LIKE '%s%%'", name))
		}
	}

	// Execute all queries and merge.
	//
	// The rows are collected before anything is printed, so a query that failed
	// can be reported ahead of them. This used to skip failures silently, and
	// this is the *fallback* path — reached when the good reader has already
	// failed — so a second silent failure here left the object looking as if it
	// referenced nothing at all.
	seen := map[string]bool{}
	var found []string
	var gaps []adt.Unsearched
	for _, sql := range queries {
		result, err := client.RunQuery(ctx, sql, 200)
		if err != nil || result == nil {
			table := "cross-reference table"
			for _, t := range []string{"WBCROSSGT", "CROSS"} {
				if strings.Contains(sql, " FROM "+t+" ") {
					table = t
				}
			}
			reason := "the query returned nothing at all"
			if err != nil {
				reason = err.Error()
			}
			gaps = append(gaps, adt.Unsearched{Object: table, Reason: reason})
			continue
		}
		for _, row := range result.Rows {
			var key string
			if direction == "callers" {
				inc := fmt.Sprintf("%v", row["INCLUDE"])
				parts := strings.Split(inc, "=")
				key = parts[0]
			} else {
				ot := fmt.Sprintf("%v", row["OTYPE"])
				nm := fmt.Sprintf("%v", row["NAME"])
				if strings.Contains(nm, "\\") {
					continue
				}
				key = fmt.Sprintf("%-4s %s", crossToADTType(ot), nm)
			}
			if key != "" && key != name && !seen[key] {
				seen[key] = true
				found = append(found, key)
			}
		}
	}

	if note := adt.UnsearchedNote(gaps, len(queries), "query"); note != "" {
		fmt.Printf("  %s\n\n", note)
	}
	for _, key := range found {
		fmt.Printf("  %s\n", key)
	}

	if len(seen) == 0 {
		fmt.Println("  (no references found)")
	} else {
		fmt.Fprintf(os.Stderr, "\n%d unique references\n", len(seen))
	}
	return nil
}

func crossOType(adtType string) string {
	switch adtType {
	case "CLAS":
		return "CL"
	case "INTF":
		return "IF"
	case "PROG":
		return "PR"
	case "FUGR":
		return "FU"
	default:
		return "CL"
	}
}

func crossToADTType(crossType string) string {
	switch strings.TrimSpace(crossType) {
	case "CL":
		return "CLAS"
	case "IF":
		return "INTF"
	case "TY":
		return "TYPE"
	case "DA":
		return "DATA"
	case "ME":
		return "METH"
	case "PR":
		return "PROG"
	case "FU":
		return "FUNC"
	default:
		return crossType
	}
}

// whereUsedCallers asks the SE84 where-used list who calls an object.
func whereUsedCallers(ctx context.Context, client *adt.Client, objURI string) ([]adt.ExposedCaller, []adt.Unsearched, error) {
	return client.WhereUsed(ctx, objURI)
}

func printWhereUsedCallers(callers []adt.ExposedCaller, unresolved []adt.Unsearched) {
	// Printed ahead of the table, as the callee gap is: an include standing in
	// for its program is still a caller, but the list is not the whole answer.
	if note := adt.UnresolvedIncludesNote(unresolved); note != "" {
		fmt.Printf("%s\n\n", note)
	}
	if len(callers) == 0 {
		// Checked live: a name that does not exist gets the same 200 and the
		// same empty list as a real object nobody calls. The list cannot tell
		// them apart, so neither should the sentence printed about it.
		fmt.Println("The where-used list answered and it is empty: nobody calls this — or the name does not exist, which reads identically here.")
		return
	}
	rows := make([][]string, 0, len(callers))
	for _, c := range callers {
		kind := "code"
		if c.IsTest {
			kind = "test"
		}
		rows = append(rows, []string{c.Name, c.Type, c.Package, kind, c.Component})
	}
	fmt.Print(formatTable([]string{"Object", "Type", "Package", "Kind", "References in"}, rows))
	fmt.Fprintf(os.Stderr, "\n%d callers, from the where-used list.\n", len(callers))
}

func readStdin() ([]byte, error) {
	return os.ReadFile("/dev/stdin")
}

// --- graph co-change handler ---

func runGraphCoChange(cmd *cobra.Command, args []string) error {
	params, err := resolveSystemParams(cmd)
	if err != nil {
		return err
	}
	client, err := getClient(params)
	if err != nil {
		return err
	}

	objType := strings.ToUpper(args[0])
	objName := strings.ToUpper(args[1])
	topN, _ := cmd.Flags().GetInt("top")
	format, _ := cmd.Flags().GetString("format")
	ctx := context.Background()

	targetNodeID := graph.NodeID(objType, objName)

	// Step 1: Find transports containing this object (E071)
	fmt.Fprintf(os.Stderr, "Querying E071 for %s %s...\n", objType, objName)
	e071Query := fmt.Sprintf(
		"SELECT TRKORR, PGMID, OBJECT, OBJ_NAME FROM E071 WHERE PGMID = 'R3TR' AND OBJECT = '%s' AND OBJ_NAME = '%s'",
		objType, objName)
	e071Result, err := client.RunQuery(ctx, e071Query, 200)
	if err != nil {
		return fmt.Errorf("E071 query failed: %w", err)
	}
	if e071Result == nil || len(e071Result.Rows) == 0 {
		fmt.Println("No transports found for this object.")
		return nil
	}

	// Collect transport numbers (may be tasks or requests)
	trNums := make(map[string]bool)
	for _, row := range e071Result.Rows {
		tr := strings.TrimSpace(fmt.Sprintf("%v", row["TRKORR"]))
		if tr != "" {
			trNums[tr] = true
		}
	}
	fmt.Fprintf(os.Stderr, "Found %d transport entries.\n", len(trNums))

	// Step 2: Resolve E070 headers to get request/task hierarchy
	trList := make([]string, 0, len(trNums))
	for tr := range trNums {
		trList = append(trList, "'"+tr+"'")
	}
	e070Query := fmt.Sprintf(
		"SELECT TRKORR, STRKORR, TRFUNCTION, TRSTATUS, AS4USER, AS4DATE FROM E070 WHERE TRKORR IN (%s)",
		strings.Join(trList, ","))
	e070Result, err := client.RunQuery(ctx, e070Query, 500)
	if err != nil {
		return fmt.Errorf("E070 query failed: %w", err)
	}

	// Parse headers, collect parent request numbers
	var headers []graph.TransportHeader
	requestNums := make(map[string]bool)
	for _, row := range e070Result.Rows {
		h := graph.TransportHeader{
			TRKORR:     strings.TrimSpace(fmt.Sprintf("%v", row["TRKORR"])),
			STRKORR:    strings.TrimSpace(fmt.Sprintf("%v", row["STRKORR"])),
			TRFUNCTION: strings.TrimSpace(fmt.Sprintf("%v", row["TRFUNCTION"])),
			TRSTATUS:   strings.TrimSpace(fmt.Sprintf("%v", row["TRSTATUS"])),
			AS4USER:    strings.TrimSpace(fmt.Sprintf("%v", row["AS4USER"])),
			AS4DATE:    strings.TrimSpace(fmt.Sprintf("%v", row["AS4DATE"])),
		}
		headers = append(headers, h)
		if h.IsRequest() {
			requestNums[h.TRKORR] = true
		} else if h.STRKORR != "" {
			requestNums[h.STRKORR] = true
		}
	}

	// Fetch parent request headers if we only had tasks
	var missingRequests []string
	for rn := range requestNums {
		if !trNums[rn] {
			missingRequests = append(missingRequests, "'"+rn+"'")
		}
	}
	if len(missingRequests) > 0 {
		parentQuery := fmt.Sprintf(
			"SELECT TRKORR, STRKORR, TRFUNCTION, TRSTATUS, AS4USER, AS4DATE FROM E070 WHERE TRKORR IN (%s)",
			strings.Join(missingRequests, ","))
		parentResult, err := client.RunQuery(ctx, parentQuery, 100)
		if err == nil && parentResult != nil {
			for _, row := range parentResult.Rows {
				h := graph.TransportHeader{
					TRKORR:     strings.TrimSpace(fmt.Sprintf("%v", row["TRKORR"])),
					STRKORR:    strings.TrimSpace(fmt.Sprintf("%v", row["STRKORR"])),
					TRFUNCTION: strings.TrimSpace(fmt.Sprintf("%v", row["TRFUNCTION"])),
					TRSTATUS:   strings.TrimSpace(fmt.Sprintf("%v", row["TRSTATUS"])),
					AS4USER:    strings.TrimSpace(fmt.Sprintf("%v", row["AS4USER"])),
					AS4DATE:    strings.TrimSpace(fmt.Sprintf("%v", row["AS4DATE"])),
				}
				headers = append(headers, h)
				requestNums[h.TRKORR] = true
			}
		}
	}

	// Step 2b: Fetch ALL child tasks of resolved parent requests.
	// Without this, sibling objects in other tasks of the same request are missed.
	if len(requestNums) > 0 {
		parentList := make([]string, 0, len(requestNums))
		for rn := range requestNums {
			parentList = append(parentList, "'"+rn+"'")
		}
		childTaskQuery := fmt.Sprintf(
			"SELECT TRKORR, STRKORR, TRFUNCTION, TRSTATUS, AS4USER, AS4DATE FROM E070 WHERE STRKORR IN (%s)",
			strings.Join(parentList, ","))
		childResult, err := client.RunQuery(ctx, childTaskQuery, 500)
		if err == nil && childResult != nil {
			for _, row := range childResult.Rows {
				h := graph.TransportHeader{
					TRKORR:     strings.TrimSpace(fmt.Sprintf("%v", row["TRKORR"])),
					STRKORR:    strings.TrimSpace(fmt.Sprintf("%v", row["STRKORR"])),
					TRFUNCTION: strings.TrimSpace(fmt.Sprintf("%v", row["TRFUNCTION"])),
					TRSTATUS:   strings.TrimSpace(fmt.Sprintf("%v", row["TRSTATUS"])),
					AS4USER:    strings.TrimSpace(fmt.Sprintf("%v", row["AS4USER"])),
					AS4DATE:    strings.TrimSpace(fmt.Sprintf("%v", row["AS4DATE"])),
				}
				// Only add if not already known
				found := false
				for _, existing := range headers {
					if existing.TRKORR == h.TRKORR {
						found = true
						break
					}
				}
				if !found {
					headers = append(headers, h)
				}
			}
		}
	}

	// Collect all request + task numbers for sibling E071 fetch
	allTRNums := make(map[string]bool)
	for _, h := range headers {
		allTRNums[h.TRKORR] = true
	}
	for rn := range requestNums {
		allTRNums[rn] = true
	}

	// Step 3: Fetch all E071 objects for these transports (sibling objects)
	allTRList := make([]string, 0, len(allTRNums))
	for tr := range allTRNums {
		allTRList = append(allTRList, "'"+tr+"'")
	}
	fmt.Fprintf(os.Stderr, "Fetching sibling objects from %d transports...\n", len(allTRList))
	siblingQuery := fmt.Sprintf(
		"SELECT TRKORR, PGMID, OBJECT, OBJ_NAME FROM E071 WHERE TRKORR IN (%s) AND PGMID = 'R3TR'",
		strings.Join(allTRList, ","))
	siblingResult, err := client.RunQuery(ctx, siblingQuery, 2000)
	if err != nil {
		return fmt.Errorf("sibling E071 query failed: %w", err)
	}

	// Parse objects
	var objects []graph.TransportObject
	if siblingResult != nil {
		for _, row := range siblingResult.Rows {
			objects = append(objects, graph.TransportObject{
				TRKORR:  strings.TrimSpace(fmt.Sprintf("%v", row["TRKORR"])),
				PGMID:   strings.TrimSpace(fmt.Sprintf("%v", row["PGMID"])),
				Object:  strings.TrimSpace(fmt.Sprintf("%v", row["OBJECT"])),
				ObjName: strings.TrimSpace(fmt.Sprintf("%v", row["OBJ_NAME"])),
			})
		}
	}

	// Step 4: Build graph and run query
	g := graph.BuildTransportGraph(headers, objects)
	result := graph.WhatChangesWith(g, targetNodeID, topN)

	// Output
	switch format {
	case "json":
		data, err := json.MarshalIndent(result, "", "  ")
		if err != nil {
			return err
		}
		fmt.Println(string(data))
		return nil
	case "mermaid":
		fmt.Println(graph.CoChangeToMermaid(result))
		return nil
	case "html":
		mmd := graph.CoChangeToMermaid(result)
		title := fmt.Sprintf("Co-change: %s (%d transports)", targetNodeID, result.TotalTransports)
		fmt.Println(graph.WrapMermaidHTML(title, mmd))
		return nil
	}

	// Text output (default)
	fmt.Printf("Co-change analysis: %s\n", targetNodeID)
	fmt.Printf("Transports: %d\n\n", result.TotalTransports)

	if len(result.CoChanges) == 0 {
		fmt.Println("No co-changing objects found.")
		return nil
	}

	tableRows := make([][]string, 0, len(result.CoChanges))
	for _, e := range result.CoChanges {
		trIDs := strings.Join(e.Transports, ", ")
		if len(trIDs) > 60 {
			trIDs = trIDs[:57] + "..."
		}
		tableRows = append(tableRows, []string{
			fmt.Sprintf("%d", e.Count),
			e.Type,
			e.Name,
			trIDs,
		})
	}

	fmt.Print(formatTable(
		[]string{"Count", "Type", "Name", "Shared Transports"},
		tableRows,
	))
	fmt.Fprintf(os.Stderr, "\n%d co-changing objects\n", len(result.CoChanges))
	return nil
}

// --- method-signature handler ---

func runMethodSignature(cmd *cobra.Command, args []string) error {
	params, err := resolveSystemParams(cmd)
	if err != nil {
		return err
	}
	client, err := getClient(params)
	if err != nil {
		return err
	}

	className := strings.ToUpper(strings.TrimSpace(args[0]))
	methodName := strings.ToUpper(strings.TrimSpace(args[1]))
	format, _ := cmd.Flags().GetString("format")
	ctx := context.Background()

	fmt.Fprintf(os.Stderr, "Reading definition of %s=>%s...\n", className, methodName)

	// Fetch class definition source (not implementation)
	source, err := client.GetSource(ctx, "CLAS", className, nil)
	if err != nil {
		return fmt.Errorf("failed to read class source: %w", err)
	}

	sig := graph.ExtractMethodSignature(className, methodName, source)

	if sig.RawDef == "" {
		return fmt.Errorf("method %s not found in class %s definition", methodName, className)
	}

	// Output
	if format == "json" {
		data, err := json.MarshalIndent(sig, "", "  ")
		if err != nil {
			return err
		}
		fmt.Println(string(data))
		return nil
	}

	fmt.Print(graph.FormatMethodSignature(sig))
	return nil
}

// --- class-sections handler ---

func runClassSections(cmd *cobra.Command, args []string) error {
	params, err := resolveSystemParams(cmd)
	if err != nil {
		return err
	}
	client, err := getClient(params)
	if err != nil {
		return err
	}

	className := strings.ToUpper(strings.TrimSpace(args[0]))
	format, _ := cmd.Flags().GetString("format")
	ctx := context.Background()

	fmt.Fprintf(os.Stderr, "Reading structure of %s...\n", className)

	// Fetch class structure via GetClassStructureElements (ADT objectstructure)
	elements, err := fetchClassStructureElements(ctx, client, className)
	if err != nil {
		return fmt.Errorf("failed to read class structure: %w", err)
	}

	result := graph.ClassifySections(className, elements)

	// Output
	if format == "json" {
		data, err := json.MarshalIndent(result, "", "  ")
		if err != nil {
			return err
		}
		fmt.Println(string(data))
		return nil
	}

	fmt.Print(graph.FormatClassSections(result))
	return nil
}

// fetchClassStructureElements fetches the full class structure from ADT
// and returns all elements (methods, attributes, types, events).
func fetchClassStructureElements(ctx context.Context, client *adt.Client, className string) ([]graph.ClassStructureElement, error) {
	structure, err := client.GetClassObjectStructure(ctx, className)
	if err != nil {
		return nil, err
	}

	var elements []graph.ClassStructureElement
	for _, elem := range structure.Elements {
		elements = append(elements, graph.ClassStructureElement{
			Name:       elem.Name,
			ADTType:    elem.Type,
			Visibility: elem.Visibility,
			Level:      elem.Level,
		})
	}
	return elements, nil
}

// --- rename-preview handler ---

func runRenamePreview(cmd *cobra.Command, args []string) error {
	params, err := resolveSystemParams(cmd)
	if err != nil {
		return err
	}
	client, err := getClient(params)
	if err != nil {
		return err
	}

	objType := strings.ToUpper(args[0])
	oldName := strings.ToUpper(args[1])
	newName := strings.ToUpper(args[2])
	format, _ := cmd.Flags().GetString("format")
	ctx := context.Background()

	fmt.Fprintf(os.Stderr, "Scanning references to %s %s...\n", objType, oldName)
	oldNameUpper := strings.ToUpper(strings.TrimSpace(oldName))

	// Who references this object name.
	//
	// This is not a report, it is the thing somebody reads before renaming, so
	// every way it can be wrong is a way somebody breaks a system. The version
	// this replaces could be wrong in three:
	//
	//   Both queries were guarded by `err == nil`, so two failed reads produced
	//   an empty impact list — "nothing references this" — in front of a
	//   destructive operation.
	//
	//   NAME LIKE 'ZCL_ORDER%' also matches ZCL_ORDER_ITEM, so a sibling's
	//   callers were listed as this object's. That overstates, which is the
	//   safer direction and still wrong.
	//
	//   A reference whose name does not fit WBCROSSGT's CHAR(120) is stored
	//   under a SHA-1, so it matches no prefix of the real name and was absent
	//   entirely. That understates, before a rename, which is the direction
	//   that breaks things.
	var allRefs []graph.RenameRefRow
	var unread []adt.Unsearched

	addRows := func(rows []map[string]interface{}, typeCol, source string) {
		for _, row := range rows {
			name := strings.ToUpper(strings.TrimSpace(fmt.Sprintf("%v", row["NAME"])))
			// Exactly this object, or this object followed by the backslash
			// that introduces a component. Never merely starting with it.
			if name != oldNameUpper && !strings.HasPrefix(name, oldNameUpper+`\`) {
				continue
			}
			allRefs = append(allRefs, graph.RenameRefRow{
				CallerInclude: strings.TrimSpace(fmt.Sprintf("%v", row["INCLUDE"])),
				TargetName:    name,
				RefType:       strings.TrimSpace(fmt.Sprintf("%v", row[typeCol])),
				Source:        source,
			})
		}
	}

	wbResult, wbErr := client.RunQuery(ctx,
		fmt.Sprintf("SELECT INCLUDE, OTYPE, NAME FROM WBCROSSGT WHERE NAME LIKE '%s%%'", oldName), 2000)
	if wbErr != nil {
		unread = append(unread, adt.Unsearched{Object: "WBCROSSGT", Reason: wbErr.Error()})
	} else if wbResult != nil {
		// Decode before matching: a hashed name matches no prefix, so without
		// this the rows exist and the filter above drops every one of them.
		_ = client.ResolveLongNames(ctx, wbResult.Rows)
		addRows(wbResult.Rows, "OTYPE", "WBCROSSGT")
	}

	crossResult, crossErr := client.RunQuery(ctx,
		fmt.Sprintf("SELECT INCLUDE, TYPE, NAME FROM CROSS WHERE NAME LIKE '%s%%'", oldName), 2000)
	if crossErr != nil {
		unread = append(unread, adt.Unsearched{Object: "CROSS", Reason: crossErr.Error()})
	} else if crossResult != nil {
		addRows(crossResult.Rows, "TYPE", "CROSS")
	}

	if len(unread) == 2 {
		return fmt.Errorf("neither cross-reference table could be read, so this is not an impact "+
			"assessment and must not be used as one: %s; %s", unread[0].Reason, unread[1].Reason)
	}
	if note := adt.UnsearchedNote(unread, 2, "cross-reference table"); note != "" {
		fmt.Fprintf(os.Stderr, "%s\nReferences recorded only there are missing from the list below.\n\n", note)
	}

	// Compute preview
	result := graph.ComputeRenamePreview(objType, oldName, newName, allRefs)

	// Output
	if format == "json" {
		data, err := json.MarshalIndent(result, "", "  ")
		if err != nil {
			return err
		}
		fmt.Println(string(data))
		return nil
	}

	// Text output
	fmt.Printf("Rename Preview: %s %s → %s\n", result.ObjectType, result.OldName, result.NewName)
	fmt.Printf("Affected objects: %d (%d total references)\n\n", result.AffectedCount, result.TotalRefs)

	if result.AffectedCount > 0 {
		tableRows := make([][]string, 0, len(result.Refs))
		for _, r := range result.Refs {
			tableRows = append(tableRows, []string{
				r.Confidence,
				fmt.Sprintf("%d", r.RefCount),
				r.CallerType,
				r.CallerName,
				r.Source,
			})
		}
		fmt.Print(formatTable(
			[]string{"Confidence", "Refs", "Type", "Object", "Source"},
			tableRows,
		))
		fmt.Println()
	} else {
		fmt.Println("No static references found.")
		fmt.Println()
	}

	// Risks
	if len(result.Risks) > 0 {
		fmt.Println("⚠️  Risks (not detectable by static analysis):")
		for _, r := range result.Risks {
			fmt.Printf("  [%s] %s\n", r.Kind, r.Description)
		}
		fmt.Println()
	}

	return nil
}

// --- slim handler ---

func runSlim(cmd *cobra.Command, args []string) error {
	params, err := resolveSystemParams(cmd)
	if err != nil {
		return err
	}
	client, err := getClient(params)
	if err != nil {
		return err
	}

	pkg := strings.ToUpper(args[0])
	inclSub, _ := cmd.Flags().GetBool("include-subpackages")
	exactPkg, _ := cmd.Flags().GetBool("exact-package")
	format, _ := cmd.Flags().GetString("format")
	level, _ := cmd.Flags().GetString("level")
	ctx := context.Background()
	if exactPkg {
		inclSub = false
	}

	// Step 1: Resolve package scope via shared helper (TDEVC hierarchy + fallback)
	fmt.Fprintf(os.Stderr, "Resolving package scope for %s...\n", pkg)
	scope, err := AcquirePackageScope(ctx, client, pkg, inclSub)
	if err != nil {
		return fmt.Errorf("scope resolution failed: %w", err)
	}
	fmt.Fprintf(os.Stderr, "Scope: %d packages (%s)\n", len(scope.Packages), scope.Method)

	// Step 1b: Fetch objects via shared helper
	fmt.Fprintf(os.Stderr, "Loading objects...\n")
	pkgObjects, err := AcquirePackageObjects(ctx, client, ScopeToWhere(scope))
	if err != nil {
		return fmt.Errorf("object fetch failed: %w", err)
	}
	if len(pkgObjects) == 0 {
		return fmt.Errorf("package %s is empty or not found", pkg)
	}

	// Convert to SlimObjectInfo + build name set
	var objects []graph.SlimObjectInfo
	objNames := make(map[string]bool)
	nameList := make([]string, 0, len(pkgObjects))
	for _, obj := range pkgObjects {
		objects = append(objects, graph.SlimObjectInfo{
			Name: obj.Name, Type: obj.Type, Package: obj.Package,
		})
		objNames[obj.Name] = true
		nameList = append(nameList, obj.Name)
	}
	fmt.Fprintf(os.Stderr, "Found %d objects.\n", len(objects))

	// Step 2: Query reverse references via shared helper
	fmt.Fprintf(os.Stderr, "Querying reverse references...\n")
	allRefs := AcquireReverseRefs(ctx, client, nameList, true)
	fmt.Fprintf(os.Stderr, "Collected %d reverse references.\n", len(allRefs))

	// Step 2.5: Collect method info for non-dead classes (if --level methods or full)
	if level == "methods" || level == "full" {
		// Build set of class names
		var classNames []string
		for _, obj := range objects {
			if obj.Type == "CLAS" {
				classNames = append(classNames, obj.Name)
			}
		}
		var uninspected []adt.Unsearched
		if len(classNames) > 0 {
			fmt.Fprintf(os.Stderr, "Fetching class structures (%d classes)...\n", len(classNames))
			for i, cls := range classNames {
				fmt.Fprintf(os.Stderr, "\r  [%d/%d] %-40s", i+1, len(classNames), cls)
				structure, err := client.GetClassObjectStructure(ctx, cls)
				if err != nil {
					// A class whose structure will not load keeps its entry and
					// gets no methods, which is indistinguishable from a class
					// that has none — and this report is about what is unused.
					uninspected = append(uninspected, adt.Unsearched{Object: "CLAS " + cls, Reason: err.Error()})
					continue
				}
				methods := structure.GetMethods()
				var methodNames []string
				for _, m := range methods {
					if m.Name != "" {
						methodNames = append(methodNames, m.Name)
					}
				}
				// Update the object with method info
				for j := range objects {
					if strings.EqualFold(objects[j].Name, cls) {
						objects[j].Methods = methodNames
						break
					}
				}
			}
			fmt.Fprintf(os.Stderr, "\r\n")
		}
		if note := adt.UnsearchedNote(uninspected, len(classNames), "class"); note != "" {
			fmt.Fprintf(os.Stderr, "%s\n", note)
		}
	}

	// Step 3: Compute slim report
	result := graph.ComputeSlim(objects, allRefs, nil, objNames)
	result.Scope = pkg

	// Output
	if format == "json" {
		data, err := json.MarshalIndent(result, "", "  ")
		if err != nil {
			return err
		}
		fmt.Println(string(data))
		return nil
	}

	// Text output
	fmt.Printf("Slim Report: %s (%d objects: %d live, %d dead, %d internal-only)\n\n",
		result.Scope, result.TotalObjects, result.LiveObjectCount, result.DeadObjectCount, result.InternalOnlyCount)

	if result.DeadObjectCount == 0 && result.InternalOnlyCount == 0 && result.DeadMethodCount == 0 {
		fmt.Println("No dead code found. Package is clean.")
		return nil
	}

	if result.DeadObjectCount > 0 {
		fmt.Printf("Dead objects (%d) — zero references anywhere [HIGH confidence]:\n", result.DeadObjectCount)
		for _, d := range result.DeadObjects {
			pkg := d.Package
			if pkg == "" {
				pkg = "-"
			}
			fmt.Printf("  %s %s %s [%s]\n", "❌", d.Type, d.Name, pkg)
		}
		fmt.Println()
	}

	if result.InternalOnlyCount > 0 {
		fmt.Printf("Internal-only objects (%d) — referenced only within scope (review needed):\n", result.InternalOnlyCount)
		for _, d := range result.InternalOnly {
			pkg := d.Package
			if pkg == "" {
				pkg = "-"
			}
			fmt.Printf("  %s %s %s [%s] — %d internal refs\n", "ℹ️", d.Type, d.Name, pkg, d.InternalRefs)
		}
		fmt.Println()
	}

	if result.DeadMethodCount > 0 {
		fmt.Printf("Dead methods (%d) — in non-dead classes, no external callers [MEDIUM]:\n", result.DeadMethodCount)
		for _, d := range result.DeadMethods {
			fmt.Printf("  %s %s=>%s\n", "⚠️", d.Name, d.Method)
		}
		fmt.Println()
	}

	fmt.Fprintf(os.Stderr, "Summary: %d dead, %d internal-only, %d dead methods, %d live\n",
		result.DeadObjectCount, result.InternalOnlyCount, result.DeadMethodCount, result.LiveObjectCount)
	return nil
}

// --- examples handler ---

func runExamples(cmd *cobra.Command, args []string) error {
	params, err := resolveSystemParams(cmd)
	if err != nil {
		return err
	}
	client, err := getClient(params)
	if err != nil {
		return err
	}

	objType := strings.ToUpper(args[0])
	objName := strings.ToUpper(args[1])
	method, _ := cmd.Flags().GetString("method")
	form, _ := cmd.Flags().GetString("form")
	isSubmit, _ := cmd.Flags().GetBool("submit")
	topN, _ := cmd.Flags().GetInt("top")
	format, _ := cmd.Flags().GetString("format")
	ctx := context.Background()

	method = strings.ToUpper(strings.TrimSpace(method))
	form = strings.ToUpper(strings.TrimSpace(form))

	// Build target
	target := graph.UsageTarget{
		ObjectType: objType,
		ObjectName: objName,
		Method:     method,
		Form:       form,
	}

	// Validate target type
	switch objType {
	case "FUNC":
		// OK as-is
	case "CLAS", "INTF":
		if method == "" {
			return fmt.Errorf("--method is required for %s targets. Example: vsp examples %s %s --method METHOD_NAME", objType, objType, objName)
		}
	case "PROG":
		if !isSubmit && form == "" {
			return fmt.Errorf("--submit or --form required for PROG targets. Example: vsp examples PROG %s --submit", objName)
		}
		if isSubmit {
			target.ObjectType = "SUBMIT"
		}
	default:
		return fmt.Errorf("unsupported type %q. Supported: FUNC, CLAS, INTF, PROG", objType)
	}

	// Step 1: Find reverse callers
	fmt.Fprintf(os.Stderr, "Finding callers of %s %s", objType, objName)
	if method != "" {
		fmt.Fprintf(os.Stderr, " method %s", method)
	}
	if form != "" {
		fmt.Fprintf(os.Stderr, " form %s", form)
	}
	fmt.Fprintln(os.Stderr, "...")

	// Query WBCROSSGT + CROSS for who references this object
	// The section travels with the name because a caller found through a class's
	// test include is not in that class's main source, and reading main is
	// reading the wrong half of the object. Every one of the 186 callers of
	// CL_ABAP_UNIT_ASSERT=>ASSERT_EQUALS is a CCAU row; the command returned
	// zero examples and said "0 of 15" as though that were an answer.
	var callerNames []struct {
		name    string
		objType string
		section string
	}
	seen := make(map[string]bool)

	var wbQuery string
	var crossQuery string
	switch target.ObjectType {
	case "FUNC":
		crossQuery = fmt.Sprintf("SELECT INCLUDE, TYPE, NAME FROM CROSS WHERE NAME = '%s' AND TYPE = '%s'", objName, adt.CrossTypeFunctionModule)
	case "SUBMIT":
		crossQuery = fmt.Sprintf("SELECT INCLUDE, TYPE, NAME FROM CROSS WHERE NAME = '%s' AND TYPE = '%s'", objName, adt.CrossTypeReport)
	case "PROG":
		if form != "" {
			crossQuery = fmt.Sprintf("SELECT INCLUDE, TYPE, NAME FROM CROSS WHERE NAME = '%s' AND TYPE = '%s'", form, adt.CrossTypeSubroutine)
		} else {
			crossQuery = fmt.Sprintf("SELECT INCLUDE, TYPE, NAME FROM CROSS WHERE NAME = '%s' AND TYPE = '%s'", objName, adt.CrossTypeReport)
		}
	case "CLAS", "INTF":
		wbQuery = fmt.Sprintf("SELECT INCLUDE, OTYPE, NAME FROM WBCROSSGT WHERE NAME LIKE '%s%%'", objName)
		crossQuery = fmt.Sprintf("SELECT INCLUDE, TYPE, NAME FROM CROSS WHERE NAME LIKE '%s%%'", objName)
	}

	// A failed query is recorded, not skipped: the caller count below would read
	// as an answer. `asked` is only the note's total — FUNC, SUBMIT and PROG ask
	// one table, CLAS and INTF ask two.
	var gaps []adt.Unsearched
	asked := 0

	if wbQuery != "" {
		asked++
		wbResult, err := client.RunQuery(ctx, wbQuery, 200)
		if err != nil {
			gaps = append(gaps, adt.Unsearched{Object: "WBCROSSGT", Reason: err.Error()})
		} else if wbResult != nil {
			for _, row := range wbResult.Rows {
				include := strings.TrimSpace(fmt.Sprintf("%v", row["INCLUDE"]))
				if include == "" || strings.Contains(include, "\\") {
					continue
				}
				_, cType, cName := graph.NormalizeInclude(include)
				if strings.EqualFold(cName, objName) {
					continue
				}
				// Keyed by the document that will be read, not by the object:
				// a class calling the target from both its main source and its
				// test include is two reads, and keeping only the first drops
				// half the answer.
				section := graph.SectionOfInclude(include)
				key := sourceRef{Type: cType, Name: cName, Section: section}.Address()
				if !seen[key] {
					seen[key] = true
					callerNames = append(callerNames, struct {
						name    string
						objType string
						section string
					}{cName, cType, section})
				}
			}
		}
	}

	if crossQuery != "" {
		asked++
		crossResult, err := client.RunQuery(ctx, crossQuery, 200)
		if err != nil {
			gaps = append(gaps, adt.Unsearched{Object: "CROSS", Reason: err.Error()})
		} else if crossResult != nil {
			for _, row := range crossResult.Rows {
				include := strings.TrimSpace(fmt.Sprintf("%v", row["INCLUDE"]))
				if include == "" {
					continue
				}
				_, cType, cName := graph.NormalizeInclude(include)
				if cType == "FUGR" || strings.EqualFold(cName, objName) {
					continue
				}
				// Keyed by the document that will be read, not by the object:
				// a class calling the target from both its main source and its
				// test include is two reads, and keeping only the first drops
				// half the answer.
				section := graph.SectionOfInclude(include)
				key := sourceRef{Type: cType, Name: cName, Section: section}.Address()
				if !seen[key] {
					seen[key] = true
					callerNames = append(callerNames, struct {
						name    string
						objType string
						section string
					}{cName, cType, section})
				}
			}
		}
	}

	// A failed query is not an empty answer: no caller count is printed unless
	// every table asked answered. UnsearchedNote caps each reason at one line —
	// SAP's refusal is a 10 KB HTML page.
	if note := adt.UnsearchedNote(gaps, asked, "cross-reference table"); note != "" {
		return fmt.Errorf("a cross-reference table could not be read, so this is not a list of "+
			"callers and must not be used as one:\n%s", note)
	}

	fmt.Fprintf(os.Stderr, "Found %d callers.\n", len(callerNames))

	if len(callerNames) == 0 {
		fmt.Println("No callers found.")
		return nil
	}

	// Cap callers for source fetch
	maxFetch := topN + 5 // fetch a few extra in case some don't match
	if maxFetch > 30 {
		maxFetch = 30
	}
	if len(callerNames) > maxFetch {
		callerNames = callerNames[:maxFetch]
	}

	// Step 2: Fetch source for each caller
	fmt.Fprintf(os.Stderr, "Fetching source for %d callers...\n", len(callerNames))
	var callers []graph.CallerSource
	var unread []adt.Unsearched
	refs := make([]sourceRef, 0, len(callerNames))
	for _, c := range callerNames {
		refs = append(refs, sourceRef{Type: c.objType, Name: c.name, Section: c.section})
	}
	for i, r := range fetchSources(ctx, client, refs, "") {
		c := callerNames[i]
		source, err := r.Source, r.Err
		if err != nil || source == "" {
			// It still called the target; only the snippet is missing. Dropping
			// it silently makes the example list look like the whole of the
			// usage rather than the part that could be shown.
			reason := "the source came back empty"
			if err != nil {
				reason = err.Error()
			}
			unread = append(unread, adt.Unsearched{Object: c.objType + " " + c.name, Reason: reason})
			continue
		}
		isTest := graph.IsTestCaller(c.name, "")
		callers = append(callers, graph.CallerSource{
			NodeID:  graph.NodeID(c.objType, c.name),
			Name:    c.name,
			Type:    c.objType,
			Package: "", // resolved later if needed
			IsTest:  isTest,
			Source:  source,
		})
	}

	if note := adt.UnsearchedNote(unread, len(callerNames), "caller"); note != "" {
		fmt.Fprintf(os.Stderr, "%s\n", note)
	}

	// Step 3: Extract examples
	result := graph.FindUsageExamples(target, callers, topN)

	// Output
	if format == "json" {
		data, err := json.MarshalIndent(result, "", "  ")
		if err != nil {
			return err
		}
		fmt.Println(string(data))
		return nil
	}

	// Text output
	targetDesc := objType + " " + objName
	if method != "" {
		targetDesc += "=>" + method
	}
	if form != "" {
		targetDesc += " FORM " + form
	}
	fmt.Printf("Usage examples: %s (%d of %d callers)\n\n", targetDesc, len(result.Examples), result.TotalCallers)

	if len(result.Examples) == 0 {
		fmt.Println("No usage examples found.")
		return nil
	}

	for i, ex := range result.Examples {
		testLabel := ""
		if ex.IsTest {
			testLabel = " (test)"
		}
		fmt.Printf("%d. %s %s%s — %s [%s]\n", i+1, ex.CallerType, ex.CallerName, testLabel, ex.Confidence, ex.MatchType)
		fmt.Print(ex.Snippet)
		fmt.Println()
	}

	fmt.Fprintf(os.Stderr, "%d examples shown\n", len(result.Examples))
	return nil
}

// --- graph where-used-config handler ---

func runGraphWhereUsedConfig(cmd *cobra.Command, args []string) error {
	params, err := resolveSystemParams(cmd)
	if err != nil {
		return err
	}
	client, err := getClient(params)
	if err != nil {
		return err
	}

	variable := strings.ToUpper(strings.TrimSpace(args[0]))
	format, _ := cmd.Flags().GetString("format")
	noGrep, _ := cmd.Flags().GetBool("no-grep")
	doGrep := !noGrep
	ctx := context.Background()

	// Step 1: Find the objects whose code touches the TVARVC table, in the two
	// cross-reference tables (see adtsource.TVARVCReaders for why both).
	fmt.Fprintf(os.Stderr, "Querying WBCROSSGT and CROSS for TVARVC references...\n")
	candidates, wbErr, crossErr := adtsource.TVARVCReaders(ctx, client)
	if wbErr != nil && crossErr != nil {
		return fmt.Errorf("neither cross-reference table could be read, so this is not an answer: %v; %v", wbErr, crossErr)
	}
	// Half an answer is worth printing, but not worth printing silently.
	if wbErr != nil {
		fmt.Fprintf(os.Stderr, "WARN: object-oriented callers were not searched: %v\n", wbErr)
	}
	if crossErr != nil {
		fmt.Fprintf(os.Stderr, "WARN: classic procedural callers were not searched: %v\n", crossErr)
	}
	// JSON goes on to the envelope even with nothing found: "no readers" with
	// a table gap beside it is a different answer from "no readers", and a
	// JSON consumer reads nothing but the document.
	if len(candidates) == 0 && format != "json" {
		fmt.Println("No programs reference the TVARVC table.")
		return nil
	}
	fmt.Fprintf(os.Stderr, "Found %d candidate programs. ", len(candidates))

	// Step 3: Grep each candidate for the variable name
	var refs []graph.TVARVCReference
	grepCount := 0
	// Held until the count line is done, so each warning is a line of its own.
	var grepFailures []string
	// What could not be searched, for the JSON answer: the tables that could
	// not be asked, and the candidates that could not be read. A JSON consumer
	// sees no stderr, so the gaps ride in the same document as the readers.
	gaps := adtsource.TVARVCTableGaps(wbErr, crossErr)
	for _, c := range candidates {
		confirmed := false
		if doGrep {
			objURL := cliADTObjectURL(c.Type, c.Name)
			if objURL == "" {
				gaps = append(gaps, adt.Unsearched{
					Object: c.Type + " " + c.Name,
					Reason: "no ADT source URL for this object type, so it was listed unconfirmed rather than grepped",
				})
			} else {
				grepResult, err := client.GrepObject(ctx, objURL, variable, true, 0)
				// Unconfirmed already means "read it, the name is not there".
				// A grep that failed must not be filed under it — and
				// GrepObject reports a source it could not read in its
				// result, not in err.
				if reason, failed := adtsource.GrepFailure(grepResult, err); failed {
					grepFailures = append(grepFailures, fmt.Sprintf("WARN: %s %s could not be grepped: %s", c.Type, c.Name, reason))
					gaps = append(gaps, adt.Unsearched{Object: c.Type + " " + c.Name, Reason: reason})
				} else if len(grepResult.Matches) > 0 {
					confirmed = true
					grepCount++
				}
			}
		}
		refs = append(refs, graph.TVARVCReference{
			VariableName: variable,
			ObjectType:   c.Type,
			ObjectName:   c.Name,
			Confirmed:    confirmed,
		})
	}
	if doGrep {
		fmt.Fprintf(os.Stderr, "Grep confirmed %d.\n", grepCount)
		for _, w := range grepFailures {
			fmt.Fprintln(os.Stderr, w)
		}
		if len(grepFailures) > 0 {
			fmt.Fprintf(os.Stderr, "WARN: %d of %d candidates could not be grepped, so an unconfirmed row below may only mean unread.\n",
				len(grepFailures), len(candidates))
		}
	} else {
		fmt.Fprintf(os.Stderr, "Grep skipped.\n")
	}

	// Step 4: Build graph and run query
	g := graph.BuildConfigGraph(
		[]graph.TVARVCVariable{{Name: variable}},
		refs,
	)
	result := graph.WhereUsedConfig(g, variable)

	// Output
	switch format {
	case "json":
		// The same envelope MCP's where_used_config answers with.
		envelope := struct {
			*graph.ConfigUsageResult
			Unsearched []adt.Unsearched `json:"unsearched,omitempty"`
			Notes      []string         `json:"notes,omitempty"`
		}{ConfigUsageResult: result, Unsearched: gaps}
		if note := adtsource.ConfigGapNote(refs, gaps); note != "" {
			envelope.Notes = append(envelope.Notes, note)
		}
		data, err := json.MarshalIndent(envelope, "", "  ")
		if err != nil {
			return err
		}
		fmt.Println(string(data))
		return nil
	case "mermaid":
		fmt.Println(graph.ConfigUsageToMermaid(result))
		return nil
	case "html":
		mmd := graph.ConfigUsageToMermaid(result)
		title := fmt.Sprintf("Config readers: %s", variable)
		fmt.Println(graph.WrapMermaidHTML(title, mmd))
		return nil
	}

	// Text output (default)
	fmt.Printf("Where-used config: %s\n", variable)
	fmt.Printf("Found: %v\n\n", result.Found)

	if len(result.Readers) == 0 {
		fmt.Println("No programs found reading this variable.")
		return nil
	}

	tableRows := make([][]string, 0, len(result.Readers))
	for _, r := range result.Readers {
		pkg := r.Package
		if pkg == "" {
			pkg = "-"
		}
		tableRows = append(tableRows, []string{
			r.Confidence,
			r.Type,
			r.Name,
			pkg,
		})
	}

	fmt.Print(formatTable(
		[]string{"Confidence", "Type", "Name", "Package"},
		tableRows,
	))
	fmt.Fprintf(os.Stderr, "\n%d readers (%d HIGH, %d MEDIUM)\n",
		len(result.Readers),
		countConfidence(result.Readers, "HIGH"),
		countConfidence(result.Readers, "MEDIUM"),
	)
	return nil
}

func cliADTObjectURL(objType, objName string) string {
	name := strings.ToLower(objName)
	switch objType {
	case "CLAS":
		return "/sap/bc/adt/oo/classes/" + name
	case "PROG":
		return "/sap/bc/adt/programs/programs/" + name
	case "INTF":
		return "/sap/bc/adt/oo/interfaces/" + name
	case "FUGR":
		return "/sap/bc/adt/functions/groups/" + name
	default:
		return ""
	}
}

func countConfidence(readers []graph.ConfigReaderEntry, confidence string) int {
	n := 0
	for _, r := range readers {
		if r.Confidence == confidence {
			n++
		}
	}
	return n
}

// formatTable formats results as a simple table.
func formatTable(columns []string, rows [][]string) string {
	// Calculate column widths
	widths := make([]int, len(columns))
	for i, col := range columns {
		widths[i] = len(col)
	}
	for _, row := range rows {
		for i, cell := range row {
			if i < len(widths) && len(cell) > widths[i] {
				widths[i] = len(cell)
			}
		}
	}

	var sb strings.Builder
	// Header
	for i, col := range columns {
		if i > 0 {
			sb.WriteString("  ")
		}
		sb.WriteString(fmt.Sprintf("%-"+strconv.Itoa(widths[i])+"s", col))
	}
	sb.WriteByte('\n')
	// Separator
	for i, w := range widths {
		if i > 0 {
			sb.WriteString("  ")
		}
		sb.WriteString(strings.Repeat("-", w))
	}
	sb.WriteByte('\n')
	// Rows
	for _, row := range rows {
		for i, cell := range row {
			if i > 0 {
				sb.WriteString("  ")
			}
			if i < len(widths) {
				sb.WriteString(fmt.Sprintf("%-"+strconv.Itoa(widths[i])+"s", cell))
			}
		}
		sb.WriteByte('\n')
	}
	return sb.String()
}
