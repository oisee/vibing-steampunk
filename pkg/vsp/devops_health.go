package vsp

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/oisee/vibing-steampunk/pkg/adt"
	"github.com/oisee/vibing-steampunk/pkg/graph"
	"github.com/oisee/vibing-steampunk/pkg/graph/adtsource"
	"github.com/spf13/cobra"
)

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
	// Unsearched names what this signal could not look at. A health report is
	// read as a verdict, and a signal that reached nine packages of ten and
	// answered "NONE" reports better health than the truth — which is the one
	// direction a health report must never be wrong in.
	Unsearched []adt.Unsearched `json:"unsearched,omitempty"`
}

type cliHealthResult struct {
	Scope           cliHealthScope             `json:"scope"`
	Summary         cliHealthSummary           `json:"summary"`
	Signals         map[string]cliHealthSignal `json:"signals"`
	TestDetails     *adt.UnitTestResult        `json:"testDetails,omitempty"`
	ATCDetails      *adt.ATCWorklist           `json:"atcDetails,omitempty"`
	CrossingDetails *graph.CrossingReport      `json:"crossingDetails,omitempty"`
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
	status, details := adtsource.UnitTestVerdict(result)
	return cliHealthSignal{Status: status, Details: details}
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
	var missed []adt.Unsearched
	for _, p := range packages {
		objectURL := fmt.Sprintf("/sap/bc/adt/packages/%s", p)
		result, err := client.RunUnitTests(ctx, objectURL, nil)
		if err != nil {
			// packages_scanned used to count this one, so a run that reached
			// three of ten packages, found nothing, and said "NONE across 10
			// packages" was the whole lie in one line.
			missed = append(missed, adt.Unsearched{Object: p, Reason: err.Error()})
			continue
		}
		combined.Classes = append(combined.Classes, result.Classes...)
		c, m, a := adtsource.UnitTestCounts(result)
		totalClasses += c
		totalMethods += m
		totalAlerts += a
	}

	scanned := len(packages) - len(missed)
	status := "PASS"
	if totalClasses == 0 {
		status = "NONE"
	}
	// "This package has no tests" and "nobody could run tests here" are
	// different answers, and only one of them is reassuring.
	if scanned == 0 {
		status = "UNKNOWN"
	}
	if totalAlerts > 0 {
		status = "FAIL"
	}
	return cliHealthSignal{Status: status, Unsearched: missed, Details: map[string]any{
		"packages_scanned": scanned,
		"classes":          totalClasses,
		"methods":          totalMethods,
		"alerts":           totalAlerts,
	}}, combined
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
	status, details := adtsource.ATCVerdict(result)
	return cliHealthSignal{Status: status, Details: details}
}

func collectPackageATCWithDetails(ctx context.Context, client *adt.Client, pkg string) (cliHealthSignal, *adt.ATCWorklist) {
	objectURL := fmt.Sprintf("/sap/bc/adt/packages/%s", strings.ToUpper(pkg))
	result, err := client.RunATCCheck(ctx, objectURL, "", 200)
	if err != nil {
		return cliHealthSignal{Status: "ERROR", Details: map[string]any{"message": err.Error()}}, nil
	}
	status, details := adtsource.ATCVerdict(result)
	return cliHealthSignal{Status: status, Details: details}, result
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
	g.AddSourceDeps(nodeID, source)
	missed := resolvePackagesCLI(ctx, client, g)
	n := g.GetNode(nodeID)
	if n == nil || n.Package == "" {
		return cliHealthSignal{Status: "UNKNOWN", Unsearched: missed}
	}
	report := g.CheckBoundaries(n.Package, &graph.BoundaryOptions{IncludeDynamic: true})
	status := "CLEAN"
	if report.Violations > 0 {
		status = "VIOLATIONS"
	}
	return cliHealthSignal{Status: status, Unsearched: missed, Details: map[string]any{"violations": report.Violations, "crossed_packages": report.CrossedPackages, "dynamic": report.Dynamic}}
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
	// Two ways this signal stops short of the package: an object that will not
	// load, and the cap. Both make the crossing counts lower bounds, and a
	// lower bound presented as a count is what turns a partial read into
	// "CLEAN".
	var missed []adt.Unsearched
	sourceBearing, attempted := 0, 0

	// The cap used to be 50, and it was there because reading was serial: fifty
	// objects at a round trip each is about six seconds, and a health signal
	// nobody waits for is a health signal nobody runs. Reading six at a time
	// took a 222-object package to 1.6 seconds, so the cap can rise to where it
	// stops shaping the answer.
	//
	// It does not go away. A cap that is never reached costs nothing and is the
	// only thing standing between this command and a customer package of four
	// thousand objects — and when it *is* reached, the number is stated below
	// rather than the sweep ending quietly.
	var toRead []PackageObject
	var refs []sourceRef
	for _, obj := range objects {
		if !IsSourceBearing(obj.Type) {
			continue
		}
		sourceBearing++
		if len(refs) >= healthScanCap {
			continue
		}
		toRead = append(toRead, obj)
		refs = append(refs, sourceRef{Type: obj.Type, Name: obj.Name})
	}
	attempted = len(refs)

	for i, r := range fetchSources(ctx, client, refs, "  ") {
		if r.Err != nil {
			fmt.Fprintf(os.Stderr, "    WARN: %s %s: %v\n", r.Ref.Type, r.Ref.Name, r.Err)
			missed = append(missed, adt.Unsearched{Object: r.Ref.Type + " " + r.Ref.Name, Reason: r.Err.Error()})
			continue
		}
		if r.Source == "" {
			continue
		}
		obj := toRead[i]
		nodeID := graph.NodeID(obj.Type, obj.Name)
		g.AddNode(&graph.Node{ID: nodeID, Name: obj.Name, Type: obj.Type, Package: obj.Package})
		g.AddSourceDeps(nodeID, r.Source)
		count++
	}
	if count > 0 {
		fmt.Fprintf(os.Stderr, "\r")
	}

	// Resolve packages for target nodes
	missed = append(missed, resolvePackagesCLI(ctx, client, g)...)

	// The cap is not a failure but it is a gap, and a reader deciding whether
	// CLEAN means anything needs it stated the same way. An object read as
	// empty source was read, so it is neither.
	if capped := sourceBearing - attempted; capped > 0 {
		missed = append(missed, adt.Unsearched{
			Object: fmt.Sprintf("%d further object(s) in %s", capped, pkg),
			Reason: fmt.Sprintf("not read: this signal stops at %d objects", healthScanCap),
		})
	}

	// Directional crossing analysis
	report := graph.AnalyzeCrossings(g, scope, nil)

	status := "CLEAN"
	if report.Sibling > 0 || report.Downward > 0 || report.CommonDown > 0 || len(report.Circular) > 0 {
		status = "VIOLATIONS"
	}
	// Nothing was read, so nothing crossed anything. That is not CLEAN.
	if count == 0 && sourceBearing > 0 {
		status = "UNKNOWN"
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
	return cliHealthSignal{Status: status, Details: details, Unsearched: missed}, report
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
	// Staleness is the newest change anyone made, so a revision list that did
	// not arrive can only make the package look older than it is — and STALE is
	// the answer someone acts on by going looking for the owner.
	var missed []adt.Unsearched
	for _, obj := range content.Objects {
		objType := adtsource.SourceKind(obj.Type)
		if objType != "CLAS" && objType != "PROG" && objType != "INTF" {
			continue
		}
		if checked >= 10 {
			break
		}
		revs, err := client.GetRevisions(ctx, objType, obj.Name, nil)
		if err != nil {
			// An object with no version history is an answer; an object whose
			// history could not be fetched is not, and one `continue` used to
			// serve both.
			missed = append(missed, adt.Unsearched{Object: objType + " " + obj.Name, Reason: err.Error()})
			continue
		}
		if len(revs) == 0 {
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
		return cliHealthSignal{Status: "UNKNOWN", Unsearched: missed}
	}
	sig := stalenessCLIFromTime(newest, checked)
	sig.Unsearched = missed
	return sig
}

// stalenessCLIFromRevisions and stalenessCLIFromTime wrap the shared verdicts
// in the CLI's signal type.
func stalenessCLIFromRevisions(revs []adt.Revision) cliHealthSignal {
	status, details := adtsource.RevisionsVerdict(revs)
	return cliHealthSignal{Status: status, Details: details}
}

func stalenessCLIFromTime(tm time.Time, checked int) cliHealthSignal {
	status, details := adtsource.StalenessVerdict(tm, checked)
	return cliHealthSignal{Status: status, Details: details}
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

	// Last, and only when nothing worse was found: a report that could not look
	// everywhere has not earned GOOD. A bad finding still outranks this — the
	// gap does not make a failing test less true — but "no major health issues
	// detected" said over a partial sweep is the sentence someone closes the
	// tab on.
	if gaps := totalUnsearched(signals); gaps > 0 {
		return cliHealthSummary{
			Status:   "PARTIAL",
			Headline: fmt.Sprintf("Nothing bad found, but %d thing(s) could not be checked — this is not a clean bill of health", gaps),
		}
	}
	return cliHealthSummary{Status: "GOOD", Headline: "No major health issues detected"}
}

// totalUnsearched counts what the signals could not look at.
func totalUnsearched(signals map[string]cliHealthSignal) int {
	n := 0
	for _, sig := range signals {
		n += len(sig.Unsearched)
	}
	return n
}

// searchedCount recovers how many things a signal did reach, so the caveat can
// say "3 of 12" rather than only "3". Each signal counts a different noun, and
// the ones that count nothing fall back to naming the gaps alone.
func searchedCount(sig cliHealthSignal) int {
	for _, key := range []string{"packages_scanned", "objects_scanned", "checked"} {
		if v, ok := sig.Details[key].(int); ok {
			return v
		}
	}
	return 0
}

func init() {
	// Health flags
	healthCmd.Flags().String("package", "", "Analyze an entire package")
	healthCmd.Flags().Bool("fast", false, "Faster package snapshot: skip expensive checks like tests and boundary scan")
	healthCmd.Flags().Bool("details", false, "Show full details: failing test methods, ATC findings")
	healthCmd.Flags().String("format", "text", "Output format: text, json, md, or html")
	healthCmd.Flags().String("report", "", "Generate report file: md or html (writes to <package>.<ext>)")

	rootCmd.AddCommand(healthCmd)
}
