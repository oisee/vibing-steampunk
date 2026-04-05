package main

import (
	"context"
	"fmt"
	"net/url"
	"os"
	"strconv"
	"strings"

	"encoding/json"

	"github.com/oisee/vibing-steampunk/pkg/abaplint"
	"github.com/oisee/vibing-steampunk/pkg/adt"
	"github.com/oisee/vibing-steampunk/pkg/graph"
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

Note: If ExecuteABAP is blocked by safety settings, you'll see
a clear message explaining what's needed.`,
	RunE: runExecute,
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
  vsp graph CLAS ZCL_MY_CLASS --direction callees --depth 2`,
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

func init() {
	// Graph flags
	graphCmd.Flags().String("direction", "callees", "Direction: callees, callers, or both")
	graphCmd.Flags().Int("depth", 1, "Maximum traversal depth")
	rootCmd.AddCommand(graphCmd)

	// Graph co-change subcommand
	graphCoChangeCmd.Flags().Int("top", 20, "Maximum results (0=all)")
	graphCoChangeCmd.Flags().String("format", "text", "Output format: text or json")
	graphCmd.AddCommand(graphCoChangeCmd)

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

	maxRows := 100
	if top > 0 {
		maxRows = top + skip
	}

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

	ctx := context.Background()
	result, err := client.ExecuteABAP(ctx, code, nil)
	if err != nil {
		errStr := err.Error()
		if strings.Contains(errStr, "safety") || strings.Contains(errStr, "blocked") {
			return fmt.Errorf("%w\n\nExecuteABAP requires write permissions.\nCheck --read-only and --allowed-ops settings", err)
		}
		return fmt.Errorf("execute failed: %w\n\nNote: ExecuteABAP wraps code in a unit test class.\nFor advanced execution, use ZADT_VSP WebSocket (vsp install zadt-vsp)", err)
	}

	if len(result.Output) > 0 {
		for _, line := range result.Output {
			fmt.Println(line)
		}
	}
	if result.Message != "" {
		fmt.Fprintf(os.Stderr, "%s\n", result.Message)
	}
	return nil
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
	depth, _ := cmd.Flags().GetInt("depth")

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

	// Try ADT call graph first, fallback to WBCROSSGT
	adtFailed := false

	switch direction {
	case "callers":
		node, err := client.GetCallersOf(ctx, objURI, depth)
		if err != nil {
			adtFailed = true
		} else {
			printGraphNode(node, 0)
		}
	case "both":
		fmt.Println("=== CALLEES (uses) ===")
		callees, err := client.GetCalleesOf(ctx, objURI, depth)
		if err != nil {
			adtFailed = true
		} else {
			printGraphNode(callees, 0)
		}
		fmt.Println("\n=== CALLERS (used by) ===")
		callers, err := client.GetCallersOf(ctx, objURI, depth)
		if err != nil {
			adtFailed = true
		} else {
			printGraphNode(callers, 0)
		}
	default: // callees
		node, err := client.GetCalleesOf(ctx, objURI, depth)
		if err != nil {
			adtFailed = true
		} else {
			printGraphNode(node, 0)
		}
	}

	if !adtFailed {
		return nil
	}

	// Fallback: use WBCROSSGT table
	fmt.Fprintf(os.Stderr, "ADT call graph not available, using WBCROSSGT table fallback\n\n")

	switch direction {
	case "callers":
		return graphFromCross(ctx, client, name, objType, "callers")
	case "both":
		fmt.Println("=== USES (callees from WBCROSSGT) ===")
		graphFromCross(ctx, client, name, objType, "callees")
		fmt.Println("\n=== USED BY (callers from WBCROSSGT) ===")
		return graphFromCross(ctx, client, name, objType, "callers")
	default:
		return graphFromCross(ctx, client, name, objType, "callees")
	}
}

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

	// Execute all queries and merge
	seen := map[string]bool{}
	for _, sql := range queries {
		result, err := client.RunQuery(ctx, sql, 200)
		if err != nil {
			continue // skip failed queries silently
		}
		if result == nil {
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
				fmt.Printf("  %s\n", key)
			}
		}
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

func printGraphNode(node *adt.CallGraphNode, indent int) {
	if node == nil {
		return
	}
	prefix := strings.Repeat("  ", indent)
	label := node.Name
	if node.Type != "" {
		label = node.Type + " " + label
	}
	if node.Description != "" {
		label += " — " + node.Description
	}
	fmt.Printf("%s%s\n", prefix, label)
	for i := range node.Children {
		printGraphNode(&node.Children[i], indent+1)
	}
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
	if format == "json" {
		data, err := json.MarshalIndent(result, "", "  ")
		if err != nil {
			return err
		}
		fmt.Println(string(data))
		return nil
	}

	// Text output
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
