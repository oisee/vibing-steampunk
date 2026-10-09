package vsp

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strings"

	"github.com/oisee/vibing-steampunk/pkg/adt"
	"github.com/oisee/vibing-steampunk/pkg/graph"
	"github.com/spf13/cobra"
)

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
	// the healthScanCap deterministic and diffing reports meaningful.
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
	var missed []adt.Unsearched

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
			var missed []adt.Unsearched
			source, missed, err = client.GetFunctionGroupAllSources(ctx, obj.objName)
			// A sub-source that failed to load yields no dependencies, which
			// downstream is indistinguishable from having none.
			for _, m := range missed {
				fmt.Fprintf(os.Stderr, "    WARN: %s %s: %s: %s\n", obj.objType, obj.objName, m.Object, m.Reason)
			}
		} else {
			source, err = client.GetSource(ctx, obj.objType, obj.objName, nil)
		}
		if err != nil {
			fmt.Fprintf(os.Stderr, "    WARN: %s %s: %v\n", obj.objType, obj.objName, err)
			// The node stays in the graph with no edges, so the boundary totals
			// below count it as an object that crosses nothing rather than as
			// one nobody could read.
			missed = append(missed, adt.Unsearched{Object: obj.objType + " " + obj.objName, Reason: err.Error()})
			continue
		}

		g.AddSourceDeps(nodeID, source)
		count++
	}

	// Resolve packages
	missed = append(missed, resolvePackagesCLI(ctx, client, g)...)

	if truncated {
		fmt.Fprintf(os.Stderr,
			"  WARN: analysis truncated at %d objects (CR has %d source-bearing objects). Deps for the remaining %d are not counted — boundary totals are lower bounds, not complete.\n",
			maxObjects, len(sortedObjs), len(sortedObjs)-maxObjects)
	}
	// Same reasoning as `vsp boundaries`: the report itself goes to stdout in
	// whatever format was asked for, so the gap is stated beside the WARN lines
	// that produced it.
	if note := adt.UnsearchedNote(missed, len(sortedObjs), "object"); note != "" {
		fmt.Fprintln(os.Stderr, note)
	}

	return graph.AnalyzeTransportBoundaries(g, scope), nil
}

// transportBoundaryStatus is the verdict, including the one the two-way
// version could not express.
//
// SELF-CONSISTENT means "this transport carries everything it depends on",
// and it was reported for a transport holding no objects at all — which is
// trivially true and says nothing. A transport number that does not exist
// answered with it, and a reader looking for reassurance found some.
//
// So a report over nothing gets its own word. It is not a failure — the query
// worked, the transport is simply empty or absent — but it is not a pass
// either, and those are different things.
func transportBoundaryStatus(report *graph.TransportBoundaryReport) string {
	if report.ObjectCount == 0 {
		return "EMPTY"
	}
	if !report.Summary.SelfConsistent {
		return "INCOMPLETE"
	}
	return "SELF-CONSISTENT"
}

// transportBoundaryNote explains an EMPTY verdict, which otherwise reads as a
// tool that failed silently.
func transportBoundaryNote(report *graph.TransportBoundaryReport) string {
	if report.ObjectCount != 0 {
		return ""
	}
	return "This transport holds no objects — it is empty, or the number does not exist. " +
		"Nothing was analysed, so neither verdict applies."
}

func printTRBoundariesText(report *graph.TransportBoundaryReport, details bool) {
	status := transportBoundaryStatus(report)
	fmt.Printf("Transport Boundaries: %s\n", report.Scope)
	fmt.Printf("Status: %s\n", status)
	if note := transportBoundaryNote(report); note != "" {
		fmt.Printf("%s\n", note)
	}
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
	status := transportBoundaryStatus(report)
	statusClass := "PASS"
	switch status {
	case "INCOMPLETE":
		statusClass = "FAIL"
	case "EMPTY":
		// Not a failure and not a pass; styling it as either would put a
		// colour on a verdict that was never reached.
		statusClass = "UNKNOWN"
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
	status := transportBoundaryStatus(report)
	fmt.Printf("# Transport Boundaries: %s\n\n", report.Scope)
	fmt.Printf("**Status:** %s\n\n", status)
	if note := transportBoundaryNote(report); note != "" {
		fmt.Printf("%s\n\n", note)
	}

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

func init() {
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
}
