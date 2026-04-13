package main

import (
	"context"
	"encoding/json"
	"fmt"
	"html"
	"os"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/oisee/vibing-steampunk/pkg/adt"
	"github.com/oisee/vibing-steampunk/pkg/graph"
	"github.com/spf13/cobra"
)

func nowNano() int64 { return time.Now().UnixNano() }

// stripColonName takes a "TYPE:NAME" identifier and returns just the
// name half. Used by the transitive-reach renderer so the displayed
// SQL-probe hint (`CROSS TYPE='F' NAME='FOO'`) shows the bare object
// name instead of the composite id.
func stripColonName(id string) string {
	if idx := strings.Index(id, ":"); idx >= 0 {
		return id[idx+1:]
	}
	return id
}

// renderFieldMap formats a map of field→value into a deterministic
// "field=value field=value" string for text-output rendering. Used by
// the value-level section so the same call site always prints the same
// way regardless of Go's randomised map iteration.
func renderFieldMap(m map[string]string) string {
	if len(m) == 0 {
		return ""
	}
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	parts := make([]string, 0, len(keys))
	for _, k := range keys {
		parts = append(parts, k+"="+m[k])
	}
	return strings.Join(parts, " ")
}

var crConfigAuditCmd = &cobra.Command{
	Use:   "cr-config-audit <cr-id>",
	Short: "Audit alignment between code and customizing data within a CR",
	Long: `Check that code objects in a Change Request's workbench transports line
up with the customizing table rows carried by the CR's customizing transports.

Reports three buckets:
  MISSING — code reads a table, no customizing row for it in any TR of the CR
  ORPHAN  — customizing row transported, no code in the CR references that table
  COVERED — both sides present

Uses CROSS / WBCROSSGT / E071K as the source of truth — never regex.

Requires transport_attribute to be configured (.vsp.json or SAP_TRANSPORT_ATTRIBUTE env).

Examples:
  vsp cr-config-audit JIRA-12345
  vsp cr-config-audit JIRA-12345 --details
  vsp cr-config-audit JIRA-12345 --format json`,
	Args: cobra.ExactArgs(1),
	RunE: runCRConfigAudit,
}

func init() {
	crConfigAuditCmd.Flags().Bool("details", false, "Show every code reference and every customizing row")
	crConfigAuditCmd.Flags().String("format", "text", "Output format: text, json, md, or html")
	crConfigAuditCmd.Flags().String("report", "", "Write report to file: html, md, json, or filename.{html,md,json}")
	crConfigAuditCmd.Flags().String("cache", "", "Path to sqlite cache file (default: /tmp/vsp-audit-cache-<system>.db)")
	crConfigAuditCmd.Flags().Bool("no-cache", false, "Disable the persistent DDIC cache for this run")
	crConfigAuditCmd.Flags().Bool("value-level", false, "Run the v2a-min value-level analyser (source fetch + literal extractor + TABKEY cross-match)")
	rootCmd.AddCommand(crConfigAuditCmd)
}

func runCRConfigAudit(cmd *cobra.Command, args []string) error {
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

	ctx := context.Background()

	// Open the DDIC cache unless the user disabled it. Failure to open is
	// non-fatal — we just run without a warm cache and log the reason.
	var cache *auditCache
	if noCache, _ := cmd.Flags().GetBool("no-cache"); !noCache {
		cachePath, _ := cmd.Flags().GetString("cache")
		c, err := openAuditCache(params.Name, cachePath)
		if err != nil {
			fmt.Fprintf(os.Stderr, "  [cache warn] %v — running without cache\n", err)
		} else {
			cache = c
			defer cache.Close()
		}
	}

	trList, err := resolveCRTransports(ctx, client, crID, attr)
	if err != nil {
		return err
	}
	if len(trList) == 0 {
		fmt.Printf("No transports found for CR %s\n", crID)
		return nil
	}
	fmt.Fprintf(os.Stderr, "Resolved %d transports for CR %s (attribute: %s)\n", len(trList), crID, attr)

	split, err := splitTransportsByFunction(ctx, client, trList)
	if err != nil {
		return err
	}
	split.CRID = crID
	fmt.Fprintf(os.Stderr, "Split: %d workbench, %d customizing, %d other\n",
		len(split.WorkbenchTRs), len(split.CustomizingTRs), len(split.OtherTRs))

	// Resolve parent dev objects for the WB transports once; feed the
	// result into both the table-scan (below) and the transitive reach
	// walker (after the report is built). The shared helper collapses
	// LIMU sub-components to their parent and TADIR-filters deleted
	// entries so every downstream consumer works with the same scope.
	liveScopeObjs, deletedRefs, err := collectCRDevObjects(ctx, client, split.WorkbenchTRs)
	if err != nil {
		return err
	}
	if len(deletedRefs) > 0 {
		fmt.Fprintf(os.Stderr, "Deleted/stale refs in transports: %d\n", len(deletedRefs))
	}

	// Build the full in-scope object set — every parent that TADIR
	// validated, regardless of whether it has a direct DDIC-table
	// reference. The transitive-reach walker needs this superset:
	// an FUGR that only calls helper FMs (and reads no tables itself)
	// still belongs to scope when reasoning about indirect coverage.
	fullScope := make(map[string]bool, len(liveScopeObjs))
	for _, o := range liveScopeObjs {
		fullScope[o.ObjectType+":"+o.ObjectName] = true
	}

	codeTables, err := collectCodeTablesFromScope(ctx, client, liveScopeObjs, cache)
	if err != nil {
		return err
	}
	fmt.Fprintf(os.Stderr, "Code-side: %d unique tables referenced\n", len(codeTables))

	// Customizing data can live in both Workbench and Customizing TRs —
	// the user explicitly noted this. Walk E071K across the whole CR.
	allTRs := append([]string{}, trList...)
	custTables, err := collectCustTables(ctx, client, allTRs)
	if err != nil {
		return err
	}
	fmt.Fprintf(os.Stderr, "Data-side: %d unique tables with transported rows\n", len(custTables))

	// v1.2a: DDIC metadata chain expansion. Walk every in-scope table through
	// DD03L→DD04L→DD01L→DD07L to collect every data element / domain / check
	// table / fixed-value set the code transitively depends on, then pull
	// the CR's own metadata objects from E071 and cross-match.
	allTablesInScope := make(map[string]bool)
	for t := range codeTables {
		allTablesInScope[t] = true
	}
	for t := range custTables {
		allTablesInScope[t] = true
	}
	reachable, err := walkDDICMetadata(ctx, client, allTablesInScope, cache)
	if err != nil {
		return err
	}
	fmt.Fprintf(os.Stderr, "DDIC metadata reachable from scope: %d objects\n", len(reachable))

	inCR, err := collectMetadataFromCR(ctx, client, allTRs)
	if err != nil {
		return err
	}
	fmt.Fprintf(os.Stderr, "DDIC metadata in CR transports: %d objects\n", len(inCR))

	report := &graph.CRConfigAuditReport{
		CRID:              crID,
		Transports:        *split,
		CodeTables:        codeTables,
		CustTables:        custTables,
		MetadataReachable: reachable,
		MetadataInCR:      inCR,
	}

	// v2a-min value-level analysis — opt-in behind --value-level because
	// it fetches per-object source and costs extra round-trips. When on,
	// we walk every in-scope code object's source through the literal
	// extractor, then cross-match against the CR's E071K TABKEYs.
	if valueLevel, _ := cmd.Flags().GetBool("value-level"); valueLevel {
		findings, err := runValueLevelAudit(ctx, client, codeTables, custTables, cache)
		if err != nil {
			fmt.Fprintf(os.Stderr, "  [value-level warn] %v\n", err)
		}
		report.ValueFindings = findings
	}

	graph.FinalizeCRConfigAuditReport(report)

	// v2a.3: annotate orphan tables with 1-hop transitive reach chains
	// so the user sees when an "orphan" is actually fed into the CR's
	// code path through a helper function. Runs after Finalize so the
	// Orphan slice is fully populated.
	runTransitiveReach(ctx, client, report, fullScope, cache)

	return outputCRConfigAudit(cmd, report)
}

// outputCRConfigAudit resolves --report / --format / --details flags and
// dispatches to the right renderer. When --report names a file (or a bare
// format like "html"), stdout is redirected to that file.
func outputCRConfigAudit(cmd *cobra.Command, report *graph.CRConfigAuditReport) error {
	format, _ := cmd.Flags().GetString("format")
	reportFlag, _ := cmd.Flags().GetString("report")
	details, _ := cmd.Flags().GetBool("details")

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
			reportFlag = "cr-config-audit-" + report.CRID + "." + format
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
	case "md":
		printCRConfigAuditMarkdown(report, details)
	case "html":
		printCRConfigAuditHTML(report, details)
	default:
		printCRConfigAuditText(report, details)
	}

	if reportFlag != "" {
		fmt.Fprintf(os.Stderr, "Report saved to %s\n", reportFlag)
	}
	return nil
}

// resolveCRTransports walks E070A by transport attribute, plus child tasks via
// E070.STRKORR, and returns the full de-duplicated list of TRKORRs for the CR.
func resolveCRTransports(ctx context.Context, client *adt.Client, crID, attr string) ([]string, error) {
	attrQuery := fmt.Sprintf(
		"SELECT TRKORR FROM E070A WHERE ATTRIBUTE = '%s' AND REFERENCE = '%s'",
		attr, crID)
	attrResult, err := client.RunQuery(ctx, attrQuery, 500)
	if err != nil {
		return nil, fmt.Errorf("E070A query failed: %w", err)
	}

	trSet := make(map[string]bool)
	var trList []string
	for _, row := range attrResult.Rows {
		tr := strings.TrimSpace(fmt.Sprintf("%v", row["TRKORR"]))
		if tr != "" && !trSet[tr] {
			trSet[tr] = true
			trList = append(trList, tr)
		}
	}
	if len(trList) == 0 {
		return nil, nil
	}

	// Child tasks. IN clause batched to 5 items (SAP freestyle 255-char limit).
	childRows, err := runChunkedINQuery(ctx, client,
		"SELECT TRKORR FROM E070 WHERE STRKORR IN (%s)", trList)
	if err != nil {
		// Non-fatal: continue with the parent requests we already have.
		fmt.Fprintf(os.Stderr, "WARN: child-task resolution failed: %v\n", err)
	}
	for _, row := range childRows {
		tr := strings.TrimSpace(fmt.Sprintf("%v", row["TRKORR"]))
		if tr != "" && !trSet[tr] {
			trSet[tr] = true
			trList = append(trList, tr)
		}
	}
	return trList, nil
}

// collectCodeTables walks every source-bearing dev object in the workbench TRs
// and queries CROSS / WBCROSSGT for its symbol references, then cross-checks
// each referenced name against DD02L to keep only actual DDIC tables and
// views. Result is a map table → list of call-site refs. No regex, no
// source-text scanning — CROSS and WBCROSSGT are the authoritative sources
// for "does this include touch this symbol?" and DD02L is the authoritative
// source for "is this symbol a DDIC table?".
//
// Note: WBCROSSGT OTYPE='TY' mixes table, class, interface, type element,
// and structure references — SAP does not distinguish them at the WBCROSSGT
// level. We collect them all then filter by DD02L presence. Likewise
// CROSS.TYPE='S' mixes tables and structures. DD02L with TABCLASS filter is
// the truth source for what's actually a table or view.
// collectCodeTablesFromScope is the inner half of the code-table scan.
// The outer half — resolving the CR's WB transports into a deduplicated
// set of dev objects via collectCRDevObjects — now runs one level up
// in runCRConfigAudit so the transitive-reach walker can share the
// full scope set without rebuilding it.
func collectCodeTablesFromScope(ctx context.Context, client *adt.Client, liveObjs []CRDevObject, cache *auditCache) (map[string][]graph.TableCodeRef, error) {
	result := make(map[string][]graph.TableCodeRef)
	if len(liveObjs) == 0 {
		return result, nil
	}

	objSet := map[devObj]bool{}
	for _, o := range liveObjs {
		// Only source-bearing object types go forward into the cross-ref scan.
		// DDLS joins the list because CDS sources read customizing tables.
		switch o.ObjectType {
		case "CLAS", "PROG", "INTF", "FUGR", "DDLS":
			objSet[devObj{o.ObjectType, o.ObjectName}] = true
		}
	}

	if len(objSet) == 0 {
		return result, nil
	}
	total := len(objSet)
	fmt.Fprintf(os.Stderr, "Scanning %d code objects for symbol references...\n", total)

	// Sort the object list so output order is stable across runs — makes
	// diffing two audits meaningful instead of "map randomised everything".
	objList := make([]devObj, 0, len(objSet))
	for o := range objSet {
		objList = append(objList, o)
	}
	sort.Slice(objList, func(i, j int) bool {
		if objList[i].objType != objList[j].objType {
			return objList[i].objType < objList[j].objType
		}
		return objList[i].objName < objList[j].objName
	})

	// Step 2: per-object symbol-ref queries run in parallel via a small
	// worker pool. An earlier attempt to batch 5 objects per query using
	// OR-LIKE failed on live d15 — SAP's freestyle query parser rejects
	// more than one LIKE per WHERE clause ("LIKE is not allowed here"),
	// so any OR-ed pattern list fell back to zero rows and silently
	// dropped every code-side table. Parallelism via goroutines gives the
	// same ~5x wall-clock win without tripping that parser limitation.
	type dedupKey struct{ symbol, include string }
	seen := map[dedupKey]bool{}
	var seenMu sync.Mutex

	rawRefs := make(map[string][]graph.TableCodeRef)
	var rawMu sync.Mutex
	prog := newProgress(total, 2)

	const refWorkers = 6
	jobCh := make(chan devObj)
	var wg sync.WaitGroup
	for w := 0; w < refWorkers; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for obj := range jobCh {
				fromObject := obj.objType + ":" + obj.objName
				refs := queryCodeRefsCached(ctx, client, obj.objName, obj.objType, cache)
				for _, r := range refs {
					if !plausibleTableName(r.Table) {
						continue
					}
					key := dedupKey{r.Table, r.FromInclude}
					seenMu.Lock()
					if seen[key] {
						seenMu.Unlock()
						continue
					}
					seen[key] = true
					seenMu.Unlock()
					r.FromObject = fromObject
					rawMu.Lock()
					rawRefs[r.Table] = append(rawRefs[r.Table], r)
					rawMu.Unlock()
				}
			}
		}()
	}

	// Dispatcher: feed objects to the worker pool, tick progress every
	// few items. Close the channel when we've handed out everything so
	// the workers drain and exit cleanly.
	go func() {
		for i, obj := range objList {
			prog.tick(i, obj.objName)
			jobCh <- obj
		}
		close(jobCh)
	}()
	wg.Wait()
	prog.done(len(objList))
	fmt.Fprintf(os.Stderr, "Collected %d unique symbols from cross-refs; filtering via DD02L...\n", len(rawRefs))

	// Step 3: cross-check unique symbols against DD02L to keep only DDIC
	// tables and views. TABCLASS filter:
	//   TRANSP / POOL / CLUSTER — physical tables that store customizing
	//   VIEW                     — database views that read customizing
	// INTTAB / APPEND are structure types — not storage, not customizing.
	symbols := make([]string, 0, len(rawRefs))
	for s := range rawRefs {
		symbols = append(symbols, s)
	}
	tableSet, err := filterDDICTables(ctx, client, symbols, cache)
	if err != nil {
		return nil, err
	}
	for t, refs := range rawRefs {
		if tableSet[t] {
			result[t] = refs
		}
	}
	return result, nil
}

// walkDDICMetadata expands the DDIC dependency chain for every table in
// scope: DD03L (fields → data element) → DD04L (data element → domain) →
// DD01L (domain → check table) → DD07L (domain → fixed values).
//
// Returns a map keyed by "KIND:NAME" (e.g. "DTEL:ZFOO_DTEL") to
// graph.MetadataRef. The key shape matches graph.FinalizeCRConfigAuditReport
// which pairs this map against the CR's transported metadata to produce
// Covered/Missing/Orphan buckets on the metadata plane.
//
// Query plan (no regex, everything through DDIC tables):
//   1. DD03L WHERE TABNAME IN (<batch>) → (TABNAME, FIELDNAME, ROLLNAME)
//   2. DD04L WHERE ROLLNAME IN (<batch>) → (ROLLNAME, DOMNAME)
//   3. DD01L WHERE DOMNAME IN (<batch>) → (DOMNAME, ENTITYTAB)   [check table]
//   4. DD07L WHERE DOMNAME IN (<batch>) → (DOMNAME, DOMVALUE_L)  [fixed values]
//
// Batched to 5 names per IN clause to respect SAP freestyle 255-char limit.
func walkDDICMetadata(ctx context.Context, client *adt.Client, tables map[string]bool, cache *auditCache) (map[string]graph.MetadataRef, error) {
	result := make(map[string]graph.MetadataRef)
	if len(tables) == 0 {
		return result, nil
	}

	tableList := make([]string, 0, len(tables))
	for t := range tables {
		tableList = append(tableList, t)
	}
	sort.Strings(tableList)

	// L2 cache: hash the sorted table set — a re-run of the same CR
	// (same scope tables) skips the entire DD03L/DD04L/DD01L/DD07L pass.
	cacheKey := "ddic-walk:" + hashStringList(tableList)
	if cache != nil {
		var cached map[string]graph.MetadataRef
		if cache.getJSON(cacheKey, &cached) {
			fmt.Fprintf(os.Stderr, "DDIC metadata: %d objects (from L2 sqlite, key %s)\n", len(cached), cacheKey[:24])
			return cached, nil
		}
	}

	fmt.Fprintf(os.Stderr, "Walking DDIC metadata for %d tables (DD03L→DD04L→DD01L→DD07L)...\n", len(tableList))
	prog := newProgress(4, 1)

	// --- Step 1: DD03L (table → fields → data element) ---
	prog.tick(0, "DD03L")
	type fieldEdge struct {
		table    string
		field    string
		rollname string
	}
	var fields []fieldEdge
	dd03Rows, err := runChunkedINQuery(ctx, client,
		"SELECT TABNAME, FIELDNAME, ROLLNAME FROM DD03L WHERE AS4LOCAL = 'A' AND TABNAME IN (%s)",
		tableList)
	if err != nil {
		return nil, fmt.Errorf("DD03L query failed: %w", err)
	}
	for _, row := range dd03Rows {
		tab := strings.ToUpper(strings.TrimSpace(fmt.Sprintf("%v", row["TABNAME"])))
		fld := strings.ToUpper(strings.TrimSpace(fmt.Sprintf("%v", row["FIELDNAME"])))
		rn := strings.ToUpper(strings.TrimSpace(fmt.Sprintf("%v", row["ROLLNAME"])))
		if rn == "" {
			continue
		}
		fields = append(fields, fieldEdge{tab, fld, rn})
	}

	// Unique ROLLNAMEs for the next step, keyed so we can remember the path.
	rollPaths := make(map[string]fieldEdge) // first seen path per DTEL wins — we keep one representative
	for _, f := range fields {
		if _, ok := rollPaths[f.rollname]; !ok {
			rollPaths[f.rollname] = f
		}
		// Record the DTEL node itself, pathed back to the first table that introduced it.
		key := "DTEL:" + f.rollname
		if _, ok := result[key]; !ok {
			result[key] = graph.MetadataRef{
				Kind:      "DTEL",
				Name:      f.rollname,
				FromTable: f.table,
				Path:      []string{"TABL:" + f.table, "FIELD:" + f.field, "DTEL:" + f.rollname},
			}
		}
	}

	// --- Step 2: DD04L (DTEL → domain) ---
	prog.tick(1, "DD04L")
	rollList := make([]string, 0, len(rollPaths))
	for r := range rollPaths {
		rollList = append(rollList, r)
	}
	sort.Strings(rollList)
	dd04Rows, err := runChunkedINQuery(ctx, client,
		"SELECT ROLLNAME, DOMNAME FROM DD04L WHERE AS4LOCAL = 'A' AND ROLLNAME IN (%s)",
		rollList)
	if err != nil {
		return nil, fmt.Errorf("DD04L query failed: %w", err)
	}
	domPaths := make(map[string]fieldEdge) // domain → its first-seen field edge
	for _, row := range dd04Rows {
		rn := strings.ToUpper(strings.TrimSpace(fmt.Sprintf("%v", row["ROLLNAME"])))
		dom := strings.ToUpper(strings.TrimSpace(fmt.Sprintf("%v", row["DOMNAME"])))
		if dom == "" {
			continue
		}
		origin := rollPaths[rn]
		if _, ok := domPaths[dom]; !ok {
			domPaths[dom] = origin
		}
		key := "DOMA:" + dom
		if _, ok := result[key]; !ok {
			result[key] = graph.MetadataRef{
				Kind:      "DOMA",
				Name:      dom,
				FromTable: origin.table,
				Path: []string{
					"TABL:" + origin.table,
					"FIELD:" + origin.field,
					"DTEL:" + rn,
					"DOMA:" + dom,
				},
			}
		}
	}

	// --- Step 3: DD01L (domain → check table) ---
	prog.tick(2, "DD01L")
	domList := make([]string, 0, len(domPaths))
	for d := range domPaths {
		domList = append(domList, d)
	}
	sort.Strings(domList)
	dd01Rows, err := runChunkedINQuery(ctx, client,
		"SELECT DOMNAME, ENTITYTAB FROM DD01L WHERE AS4LOCAL = 'A' AND DOMNAME IN (%s)",
		domList)
	if err != nil {
		return nil, fmt.Errorf("DD01L query failed: %w", err)
	}
	for _, row := range dd01Rows {
		dom := strings.ToUpper(strings.TrimSpace(fmt.Sprintf("%v", row["DOMNAME"])))
		chk := strings.ToUpper(strings.TrimSpace(fmt.Sprintf("%v", row["ENTITYTAB"])))
		if chk == "" {
			continue
		}
		origin := domPaths[dom]
		key := "CHKTAB:" + chk
		if _, ok := result[key]; !ok {
			result[key] = graph.MetadataRef{
				Kind:      "CHKTAB",
				Name:      chk,
				FromTable: origin.table,
				Path: []string{
					"TABL:" + origin.table,
					"DOMA:" + dom,
					"CHKTAB:" + chk,
				},
			}
		}
	}

	// --- Step 4: DD07L (domain → fixed values) ---
	// We record one FIXVAL entry per domain — the presence of a fixed-value
	// set is what matters for the audit. Individual values are part of the
	// v2a value-level pass, not here.
	prog.tick(3, "DD07L")
	dd07Rows, err := runChunkedINQuery(ctx, client,
		"SELECT DOMNAME FROM DD07L WHERE AS4LOCAL = 'A' AND DOMNAME IN (%s)",
		domList)
	if err != nil {
		return nil, fmt.Errorf("DD07L query failed: %w", err)
	}
	seenFixval := make(map[string]bool)
	for _, row := range dd07Rows {
		dom := strings.ToUpper(strings.TrimSpace(fmt.Sprintf("%v", row["DOMNAME"])))
		if dom == "" || seenFixval[dom] {
			continue
		}
		seenFixval[dom] = true
		origin := domPaths[dom]
		key := "FIXVAL:" + dom
		result[key] = graph.MetadataRef{
			Kind:      "FIXVAL",
			Name:      dom,
			FromTable: origin.table,
			Path: []string{
				"TABL:" + origin.table,
				"DOMA:" + dom,
				"FIXVAL:" + dom,
			},
		}
	}

	prog.done(4)
	if cache != nil {
		cache.putJSON(cacheKey, result)
	}
	return result, nil
}

// runValueLevelAudit is the v2a driver. It narrows the set of source
// objects we must fetch by intersecting two CROSS lookups with the
// already-validated in-scope set:
//
//   - CROSS TYPE='F' NAME IN (<known FMs>) — finds every include that
//     calls one of our registered customizing FMs (v2a-min path).
//   - CROSS TYPE='S' INCLUDE IN (<scope object prefixes>) — finds every
//     scope include that references any DDIC table directly (v2a.1
//     SELECT path). Tables we already saw in code-side cross-refs.
//
// Their union becomes the "interesting callers" set, and only those
// objects get source-fetched. On a typical 200+ object CR this drops
// the source-fetch set from ~200 to single-digit / low-double-digit.
//
// After source fetch the literal extractor runs on each, both
// CALL FUNCTION and SELECT shapes feed the same matcher, and the
// matcher cross-checks against transported TABKEYs.
func runValueLevelAudit(
	ctx context.Context,
	client *adt.Client,
	codeTables map[string][]graph.TableCodeRef,
	custTables map[string][]graph.TableCustRow,
	cache *auditCache,
) ([]graph.ValueLevelFinding, error) {
	// Collect in-scope object identifiers from codeTables. Each
	// TableCodeRef carries its source "CLAS:ZCL_FOO" — union them so
	// we know what cross-ref analysis already validated as in-scope.
	inScope := make(map[string]bool)
	for _, refs := range codeTables {
		for _, r := range refs {
			if r.FromObject != "" {
				inScope[r.FromObject] = true
			}
		}
	}
	if len(inScope) == 0 {
		return nil, nil
	}

	// Build a sorted list of (prefix, type, name) once. Longest
	// prefixes first so "LZFOO" wins over "ZFOO" for FUGR sub-includes.
	type prefixEntry struct {
		prefix  string
		objType string
		objName string
	}
	var prefixes []prefixEntry
	for id := range inScope {
		parts := strings.SplitN(id, ":", 2)
		if len(parts) != 2 {
			continue
		}
		objType, objName := parts[0], parts[1]
		prefixes = append(prefixes, prefixEntry{prefix: objName, objType: objType, objName: objName})
		if objType == "FUGR" {
			prefixes = append(prefixes, prefixEntry{prefix: "L" + objName, objType: objType, objName: objName})
		}
	}
	sort.Slice(prefixes, func(i, j int) bool {
		return len(prefixes[i].prefix) > len(prefixes[j].prefix)
	})

	type callSite struct {
		objType string
		objName string
	}
	callers := make(map[callSite]bool)

	matchInclude := func(inc string) {
		for _, p := range prefixes {
			if strings.HasPrefix(inc, p.prefix) {
				callers[callSite{p.objType, p.objName}] = true
				return
			}
		}
	}

	// Path A: callers of known customizing FMs (CROSS TYPE='F').
	if len(knownCustCalls) > 0 {
		fmNames := make([]string, 0, len(knownCustCalls))
		for name := range knownCustCalls {
			fmNames = append(fmNames, name)
		}
		sort.Strings(fmNames)
		for i := 0; i < len(fmNames); i += 5 {
			end := i + 5
			if end > len(fmNames) {
				end = len(fmNames)
			}
			batch := fmNames[i:end]
			quoted := make([]string, len(batch))
			for j, n := range batch {
				quoted[j] = "'" + n + "'"
			}
			q := fmt.Sprintf(
				"SELECT INCLUDE, NAME FROM CROSS WHERE TYPE = 'F' AND NAME IN (%s)",
				strings.Join(quoted, ","))
			res, err := client.RunQuery(ctx, q, 2000)
			if err != nil || res == nil {
				continue
			}
			for _, row := range res.Rows {
				inc := strings.ToUpper(strings.TrimSpace(fmt.Sprintf("%v", row["INCLUDE"])))
				if inc != "" {
					matchInclude(inc)
				}
			}
		}
	}

	// Path B: scope objects that already reference one of the CR's
	// transported tables. We rely on the data collectCodeTables already
	// produced — its codeTables map contains, for every Z/Y table the
	// scope code touches, the list of TableCodeRef with FromObject set
	// to "TYPE:NAME" of the calling code. Intersecting codeTables keys
	// with custTables keys yields exactly the set of objects whose
	// SELECT statements *might* match a transported row.
	//
	// This is significantly more reliable than a CROSS TYPE='S' query:
	// on S/4HANA the CROSS table is sparsely populated for OO code, so
	// the SQL-based pre-filter would miss every CLAS/INTF reference.
	// WBCROSSGT (which collectCodeTables already walked) is the right
	// source of truth, and we are simply re-using its output.
	for tab := range custTables {
		refs, ok := codeTables[tab]
		if !ok {
			continue
		}
		for _, r := range refs {
			if r.FromObject == "" {
				continue
			}
			parts := strings.SplitN(r.FromObject, ":", 2)
			if len(parts) != 2 {
				continue
			}
			callers[callSite{parts[0], parts[1]}] = true
		}
	}

	if len(callers) == 0 {
		fmt.Fprintf(os.Stderr, "Value-level: no in-scope callers found for registered FMs or transported tables.\n")
		return nil, nil
	}
	fmt.Fprintf(os.Stderr, "Value-level: fetching source for %d candidate callers (CALL FUNCTION + direct SELECT)...\n", len(callers))

	// Step 3: fetch source and run the extractor. Sort for stable output.
	callerList := make([]callSite, 0, len(callers))
	for c := range callers {
		callerList = append(callerList, c)
	}
	sort.Slice(callerList, func(i, j int) bool {
		if callerList[i].objType != callerList[j].objType {
			return callerList[i].objType < callerList[j].objType
		}
		return callerList[i].objName < callerList[j].objName
	})

	var allLiterals []CodeLiteralCall
	prog := newProgress(len(callerList), 2)
	for i, c := range callerList {
		prog.tick(i, c.objName)
		var source string
		var err error
		if c.objType == "FUGR" {
			source, err = client.GetFunctionGroupAllSources(ctx, c.objName)
		} else {
			source, err = client.GetSource(ctx, c.objType, c.objName, nil)
		}
		if err != nil || source == "" {
			continue
		}
		sourceID := c.objType + ":" + c.objName
		lits := extractCodeLiterals(sourceID, source)
		allLiterals = append(allLiterals, lits...)
	}
	prog.done(len(callerList))

	if len(allLiterals) == 0 {
		fmt.Fprintf(os.Stderr, "Value-level: extractor found 0 literal call sites.\n")
		return nil, nil
	}
	fmt.Fprintf(os.Stderr, "Value-level: extracted %d literal call sites; cross-matching against transported keys...\n", len(allLiterals))

	return matchValueLevelFindings(ctx, client, allLiterals, custTables, cache)
}

// collectMetadataFromCR scans E071 for DDIC metadata object types the CR is
// transporting (DTEL, DOMA, SHLP). Returns a map keyed by "KIND:NAME" so the
// caller can cross-match it against walkDDICMetadata's output. Path for these
// entries is just ["<KIND>:<NAME>"] — the CR itself is the trace.
func collectMetadataFromCR(ctx context.Context, client *adt.Client, trList []string) (map[string]graph.MetadataRef, error) {
	result := make(map[string]graph.MetadataRef)
	if len(trList) == 0 {
		return result, nil
	}
	rows, err := runChunkedINQuery(ctx, client,
		"SELECT OBJECT, OBJ_NAME FROM E071 WHERE PGMID = 'R3TR' AND OBJECT IN ('DTEL','DOMA','SHLP') AND TRKORR IN (%s)",
		trList)
	if err != nil {
		return nil, fmt.Errorf("E071 metadata query failed: %w", err)
	}
	for _, row := range rows {
		ot := strings.ToUpper(strings.TrimSpace(fmt.Sprintf("%v", row["OBJECT"])))
		nm := strings.ToUpper(strings.TrimSpace(fmt.Sprintf("%v", row["OBJ_NAME"])))
		if ot == "" || nm == "" {
			continue
		}
		kind := ot
		if ot == "SHLP" {
			kind = "SHLP"
		}
		key := kind + ":" + nm
		if _, ok := result[key]; !ok {
			result[key] = graph.MetadataRef{
				Kind: kind,
				Name: nm,
				Path: []string{kind + ":" + nm},
			}
		}
	}
	return result, nil
}

// collectCustTables walks E071K for every TR in the CR (both workbench and
// customizing — tables-as-data can travel in either) and returns a map from
// affected-table name to the list of transported rows. OBJNAME is used as
// the table identifier: for direct TABU entries it equals MASTERNAME, for
// view transports (CDAT/VDAT) it's the underlying base table, which is what
// application code actually SELECTs from.
func collectCustTables(ctx context.Context, client *adt.Client, allTRs []string) (map[string][]graph.TableCustRow, error) {
	result := make(map[string][]graph.TableCustRow)
	if len(allTRs) == 0 {
		return result, nil
	}
	rows, err := runChunkedINQuery(ctx, client,
		"SELECT TRKORR, OBJNAME, TABKEY, OBJFUNC FROM E071K WHERE OBJECT = 'TABU' AND TRKORR IN (%s)", allTRs)
	if err != nil {
		return nil, fmt.Errorf("E071K query failed: %w", err)
	}
	for _, row := range rows {
		tr := strings.TrimSpace(fmt.Sprintf("%v", row["TRKORR"]))
		obj := strings.ToUpper(strings.TrimSpace(fmt.Sprintf("%v", row["OBJNAME"])))
		key := fmt.Sprintf("%v", row["TABKEY"])
		fn := strings.ToUpper(strings.TrimSpace(fmt.Sprintf("%v", row["OBJFUNC"])))
		if obj == "" {
			continue
		}
		result[obj] = append(result[obj], graph.TableCustRow{
			Table:   obj,
			TRKORR:  tr,
			TabKey:  key,
			ObjFunc: fn,
		})
	}
	return result, nil
}

// ddicTableCache is a process-wide cache of "is this a DDIC table or view".
// Populated on first use via one bulk DD02L query; subsequent calls are O(1).
// DD02L is effectively static across a CLI invocation (the system doesn't
// ship tables while we run), so in-memory caching is always correct here.
// Persistent cross-invocation caching is the user's --cache-dd02l flag path
// (v1.1 TODO).
var ddicTableCache struct {
	loaded bool
	set    map[string]bool
}

// filterDDICTables keeps only names that correspond to real DDIC tables or
// views (TABCLASS in TRANSP/POOL/CLUSTER/VIEW). Internally it hydrates
// ddicTableCache once per process by pulling the full list of qualifying
// tables from DD02L in a single query — much cheaper than the previous
// per-name batched lookup (269 round-trips → 1).
//
// The bulk query is split by namespace prefix (Z/Y for custom, everything
// else for SAP standard) so each response stays comfortably within ADT
// transfer limits while the IN clause stays empty (literal length is zero).
func filterDDICTables(ctx context.Context, client *adt.Client, names []string, cache *auditCache) (map[string]bool, error) {
	out := make(map[string]bool)
	if len(names) == 0 {
		return out, nil
	}
	if !ddicTableCache.loaded {
		if err := hydrateDDICTableCache(ctx, client, cache); err != nil {
			return nil, err
		}
	}
	for _, n := range names {
		u := strings.ToUpper(n)
		if ddicTableCache.set[u] {
			out[u] = true
		}
	}
	return out, nil
}

// hydrateDDICTableCache bulk-fetches the DD02L table/view universe in a few
// queries. Custom-namespace tables (Z/Y) are typically < 10k rows total on
// a customer system; SAP-standard tables are much larger but we split the
// fetch by the DDIC name ranges SAP itself uses to keep each response small.
func hydrateDDICTableCache(ctx context.Context, client *adt.Client, cache *auditCache) error {
	if ddicTableCache.loaded {
		return nil
	}
	ddicTableCache.set = make(map[string]bool, 50000)

	// Try the L2 (sqlite) cache first — one TTL-protected blob holds the
	// entire DD02L snapshot. A warm hit skips every partition query below.
	if cache != nil {
		var names []string
		if cache.getJSON("dd02l:all", &names) {
			for _, n := range names {
				ddicTableCache.set[n] = true
			}
			ddicTableCache.loaded = true
			fmt.Fprintf(os.Stderr, "DD02L cache: %d tables/views (from L2 sqlite)\n", len(ddicTableCache.set))
			return nil
		}
	}

	// Partitioning: each slice is a DD02L WHERE-suffix. Custom Z/Y first
	// (cheap), then SAP standard split by initial letter to keep each
	// response bounded. A4LOCAL='A' means the active version; inactive
	// definitions would only add noise.
	//
	// Using per-partition queries with no IN clause dodges the 255-char
	// literal limit entirely while still letting us parallelise later if
	// needed. For now, sequential is fine.
	partitions := []string{
		"TABNAME LIKE 'Z%'",
		"TABNAME LIKE 'Y%'",
		"TABNAME LIKE '/%'", // namespaced: /BIC/, /NOV/, /BEV1/, …
		"TABNAME < 'E' AND TABNAME NOT LIKE 'Z%' AND TABNAME NOT LIKE 'Y%' AND TABNAME NOT LIKE '/%'",
		"TABNAME >= 'E' AND TABNAME < 'M'",
		"TABNAME >= 'M' AND TABNAME < 'S'",
		"TABNAME >= 'S' AND TABNAME < 'Z'",
	}

	fmt.Fprintf(os.Stderr, "Hydrating DD02L table cache (%d partitions)...\n", len(partitions))
	prog := newProgress(len(partitions), 1)
	for i, where := range partitions {
		prog.tick(i, "DD02L partition")
		query := fmt.Sprintf(
			"SELECT TABNAME FROM DD02L WHERE AS4LOCAL = 'A' AND TABCLASS IN ('TRANSP','VIEW','POOL','CLUSTER') AND %s",
			where)
		res, err := client.RunQuery(ctx, query, 200000)
		if err != nil {
			return fmt.Errorf("DD02L bulk query failed on partition %q: %w", where, err)
		}
		if res == nil {
			continue
		}
		for _, row := range res.Rows {
			tab := strings.ToUpper(strings.TrimSpace(fmt.Sprintf("%v", row["TABNAME"])))
			if tab != "" {
				ddicTableCache.set[tab] = true
			}
		}
	}
	prog.done(len(partitions))
	ddicTableCache.loaded = true
	fmt.Fprintf(os.Stderr, "DD02L cache: %d tables/views loaded\n", len(ddicTableCache.set))

	// Persist the universe to L2 sqlite so a warm re-run skips the seven
	// partition queries entirely.
	if cache != nil {
		names := make([]string, 0, len(ddicTableCache.set))
		for n := range ddicTableCache.set {
			names = append(names, n)
		}
		sort.Strings(names)
		cache.putJSON("dd02l:all", names)
	}
	return nil
}

func printCRConfigAuditMarkdown(r *graph.CRConfigAuditReport, details bool) {
	status := "ALIGNED"
	if !r.Summary.Aligned {
		status = "NOT ALIGNED"
	}
	fmt.Printf("# CR %s — Code ↔ Customizing Alignment Audit\n\n", r.CRID)
	fmt.Printf("**Status:** %s\n\n", status)
	fmt.Printf("## Transports\n\n")
	fmt.Printf("- Workbench TRs (%d): %s\n", len(r.Transports.WorkbenchTRs), strings.Join(r.Transports.WorkbenchTRs, ", "))
	fmt.Printf("- Customizing TRs (%d): %s\n", len(r.Transports.CustomizingTRs), strings.Join(r.Transports.CustomizingTRs, ", "))
	if len(r.Transports.OtherTRs) > 0 {
		fmt.Println("- Other TRs:")
		for _, o := range r.Transports.OtherTRs {
			fmt.Printf("  - `%s` [%s]\n", o.TR, o.Function)
		}
	}
	fmt.Println()

	fmt.Printf("## Summary\n\n")
	fmt.Println("| Metric | Value |")
	fmt.Println("|---|---|")
	fmt.Printf("| Tables read by code | %d (custom %d, SAP standard %d) |\n",
		r.Summary.TablesReadByCode, r.Summary.TablesCustomRead, r.Summary.TablesStandardRead)
	fmt.Printf("| Tables with transported data | %d |\n", r.Summary.TablesInCustTRs)
	fmt.Printf("| Covered | %d |\n", r.Summary.Covered)
	fmt.Printf("| Missing | %d |\n", r.Summary.Missing)
	fmt.Printf("| Orphan | %d |\n", r.Summary.Orphan)
	fmt.Println()

	if len(r.Missing) > 0 {
		fmt.Printf("## MISSING — custom tables read by code, not in any CR transport\n\n")
		for _, e := range r.Missing {
			fmt.Printf("### `%s`\n\n", e.Table)
			if details && len(e.CodeRefs) > 0 {
				for _, ref := range e.CodeRefs {
					fmt.Printf("- `%s` in `%s` (%s)\n", ref.FromObject, ref.FromInclude, ref.Source)
				}
				fmt.Println()
			}
		}
	}

	if len(r.Orphan) > 0 {
		fmt.Printf("## ORPHAN — transported rows not read by any code in CR\n\n")
		fmt.Println("| Table | Rows |")
		fmt.Println("|---|---|")
		for _, e := range r.Orphan {
			fmt.Printf("| `%s` | %d |\n", e.Table, len(e.CustRows))
		}
		fmt.Println()
		if details {
			for _, e := range r.Orphan {
				fmt.Printf("### `%s`\n\n", e.Table)
				for _, row := range e.CustRows {
					fmt.Printf("- `%s` `%s` `%s`\n", row.TRKORR, row.ObjFunc, row.TabKey)
				}
				fmt.Println()
			}
		}
	}

	if len(r.Covered) > 0 {
		fmt.Printf("## COVERED — tables read by code AND transported\n\n")
		fmt.Println("| Table | Rows | Code refs |")
		fmt.Println("|---|---|---|")
		for _, e := range r.Covered {
			fmt.Printf("| `%s` | %d | %d |\n", e.Table, len(e.CustRows), len(e.CodeRefs))
		}
		fmt.Println()
		if details {
			for _, e := range r.Covered {
				fmt.Printf("### `%s`\n\n", e.Table)
				fmt.Printf("**Transported rows:**\n\n")
				for _, row := range e.CustRows {
					fmt.Printf("- `%s` `%s` `%s`\n", row.TRKORR, row.ObjFunc, row.TabKey)
				}
				fmt.Printf("\n**Code references:**\n\n")
				for _, ref := range e.CodeRefs {
					fmt.Printf("- `%s` in `%s` (%s)\n", ref.FromObject, ref.FromInclude, ref.Source)
				}
				fmt.Println()
			}
		}
	}

	if details && len(r.StandardReads) > 0 {
		fmt.Printf("## SAP standard tables read (%d, informational)\n\n", len(r.StandardReads))
		var names []string
		for _, e := range r.StandardReads {
			names = append(names, "`"+e.Table+"`")
		}
		fmt.Println(strings.Join(names, ", "))
	}
}

func printCRConfigAuditHTML(r *graph.CRConfigAuditReport, details bool) {
	esc := html.EscapeString
	statusClass := "PASS"
	status := "ALIGNED"
	if !r.Summary.Aligned {
		status = "NOT ALIGNED"
		statusClass = "FAIL"
	}

	fmt.Printf(`<!DOCTYPE html>
<html><head><meta charset="UTF-8">
<title>CR %s — Code/Customizing Audit</title>
<style>
  body { font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', sans-serif; max-width: 1100px; margin: 2em auto; padding: 0 1em; color: #333; }
  h1 { border-bottom: 2px solid #ddd; padding-bottom: 0.3em; }
  h2 { margin-top: 2em; color: #555; border-bottom: 1px solid #eee; padding-bottom: 0.2em; }
  h3 { margin-top: 1.2em; color: #444; font-family: monospace; font-size: 1em; }
  table { border-collapse: collapse; width: 100%%; margin: 1em 0; }
  th, td { border: 1px solid #ddd; padding: 6px 10px; text-align: left; font-size: 0.9em; }
  th { background: #f5f5f5; }
  td.num, th.num { text-align: right; }
  .PASS { color: #2e7d32; font-weight: bold; }
  .FAIL { color: #c62828; font-weight: bold; }
  .summary { display: flex; gap: 1em; flex-wrap: wrap; margin: 1em 0; }
  .summary .box { background: #f9f9f9; border: 1px solid #ddd; border-radius: 6px; padding: 0.6em 1em; min-width: 7em; }
  .summary .num { font-size: 1.4em; font-weight: bold; display: block; }
  .summary .label { font-size: 0.8em; color: #666; }
  .summary .miss .num { color: #c62828; }
  .summary .orph .num { color: #ef6c00; }
  .summary .cov .num  { color: #2e7d32; }
  code, .mono { font-family: 'SFMono-Regular', Consolas, monospace; font-size: 0.88em; }
  .tr-list { color: #666; font-size: 0.85em; }
  details { margin: 0.5em 0; }
  details summary { cursor: pointer; font-weight: 500; }
  .standard-reads { color: #888; font-size: 0.85em; }
</style>
</head><body>
`, esc(r.CRID))

	fmt.Printf("<h1>CR %s — Code ↔ Customizing Alignment Audit</h1>\n", esc(r.CRID))
	fmt.Printf("<p>Status: <span class=\"%s\">%s</span></p>\n", statusClass, status)

	fmt.Println("<h2>Transports</h2>")
	if len(r.Transports.WorkbenchTRs) > 0 {
		fmt.Printf("<p><strong>Workbench (%d):</strong> <span class=\"tr-list mono\">%s</span></p>\n",
			len(r.Transports.WorkbenchTRs), esc(strings.Join(r.Transports.WorkbenchTRs, ", ")))
	}
	if len(r.Transports.CustomizingTRs) > 0 {
		fmt.Printf("<p><strong>Customizing (%d):</strong> <span class=\"tr-list mono\">%s</span></p>\n",
			len(r.Transports.CustomizingTRs), esc(strings.Join(r.Transports.CustomizingTRs, ", ")))
	}
	for _, o := range r.Transports.OtherTRs {
		fmt.Printf("<p><strong>Other [%s]:</strong> <code>%s</code></p>\n", esc(o.Function), esc(o.TR))
	}

	fmt.Println("<h2>Summary</h2>")
	fmt.Println(`<div class="summary">`)
	fmt.Printf(`<div class="box"><span class="num">%d</span><span class="label">Tables read by code</span></div>`, r.Summary.TablesReadByCode)
	fmt.Printf(`<div class="box"><span class="num">%d</span><span class="label">Custom reads</span></div>`, r.Summary.TablesCustomRead)
	fmt.Printf(`<div class="box"><span class="num">%d</span><span class="label">SAP-std reads</span></div>`, r.Summary.TablesStandardRead)
	fmt.Printf(`<div class="box"><span class="num">%d</span><span class="label">Tables transported</span></div>`, r.Summary.TablesInCustTRs)
	fmt.Printf(`<div class="box cov"><span class="num">%d</span><span class="label">Covered</span></div>`, r.Summary.Covered)
	fmt.Printf(`<div class="box miss"><span class="num">%d</span><span class="label">Missing</span></div>`, r.Summary.Missing)
	fmt.Printf(`<div class="box orph"><span class="num">%d</span><span class="label">Orphan</span></div>`, r.Summary.Orphan)
	fmt.Println(`</div>`)

	if len(r.Missing) > 0 {
		fmt.Println(`<h2 id="missing">MISSING — custom tables read by code, no transport</h2>`)
		fmt.Println(`<table><tr><th>Table</th><th class="num">Code refs</th></tr>`)
		for _, e := range r.Missing {
			fmt.Printf(`<tr><td><code>%s</code></td><td class="num">%d</td></tr>`, esc(e.Table), len(e.CodeRefs))
			fmt.Println()
		}
		fmt.Println(`</table>`)
		if details {
			for _, e := range r.Missing {
				fmt.Printf(`<details><summary><code>%s</code> — %d code refs</summary><ul>`, esc(e.Table), len(e.CodeRefs))
				for _, ref := range e.CodeRefs {
					fmt.Printf(`<li><code>%s</code> in <code>%s</code> <span class="tr-list">(%s)</span></li>`,
						esc(ref.FromObject), esc(ref.FromInclude), esc(ref.Source))
				}
				fmt.Println(`</ul></details>`)
			}
		}
	}

	if len(r.Orphan) > 0 {
		fmt.Println(`<h2 id="orphan">ORPHAN — transported rows not read by any code in CR</h2>`)
		fmt.Println(`<table><tr><th>Table</th><th class="num">Rows</th></tr>`)
		for _, e := range r.Orphan {
			fmt.Printf(`<tr><td><code>%s</code></td><td class="num">%d</td></tr>`, esc(e.Table), len(e.CustRows))
			fmt.Println()
		}
		fmt.Println(`</table>`)
		if details {
			for _, e := range r.Orphan {
				fmt.Printf(`<details><summary><code>%s</code> — %d rows</summary><table><tr><th>TR</th><th>Op</th><th>TABKEY</th></tr>`,
					esc(e.Table), len(e.CustRows))
				for _, row := range e.CustRows {
					fmt.Printf(`<tr><td><code>%s</code></td><td>%s</td><td><code>%s</code></td></tr>`,
						esc(row.TRKORR), esc(row.ObjFunc), esc(row.TabKey))
					fmt.Println()
				}
				fmt.Println(`</table></details>`)
			}
		}
	}

	if len(r.Covered) > 0 {
		fmt.Println(`<h2 id="covered">COVERED — tables both read and transported</h2>`)
		fmt.Println(`<table><tr><th>Table</th><th class="num">Rows</th><th class="num">Code refs</th></tr>`)
		for _, e := range r.Covered {
			fmt.Printf(`<tr><td><code>%s</code></td><td class="num">%d</td><td class="num">%d</td></tr>`,
				esc(e.Table), len(e.CustRows), len(e.CodeRefs))
			fmt.Println()
		}
		fmt.Println(`</table>`)
		if details {
			for _, e := range r.Covered {
				fmt.Printf(`<details><summary><code>%s</code> — %d rows, %d refs</summary>`,
					esc(e.Table), len(e.CustRows), len(e.CodeRefs))
				fmt.Println(`<h4>Transported rows</h4><table><tr><th>TR</th><th>Op</th><th>TABKEY</th></tr>`)
				for _, row := range e.CustRows {
					fmt.Printf(`<tr><td><code>%s</code></td><td>%s</td><td><code>%s</code></td></tr>`,
						esc(row.TRKORR), esc(row.ObjFunc), esc(row.TabKey))
					fmt.Println()
				}
				fmt.Println(`</table><h4>Code references</h4><ul>`)
				for _, ref := range e.CodeRefs {
					fmt.Printf(`<li><code>%s</code> in <code>%s</code> <span class="tr-list">(%s)</span></li>`,
						esc(ref.FromObject), esc(ref.FromInclude), esc(ref.Source))
				}
				fmt.Println(`</ul></details>`)
			}
		}
	}

	if details && len(r.StandardReads) > 0 {
		fmt.Printf(`<h2>SAP standard tables read (%d, informational)</h2><p class="standard-reads mono">`, len(r.StandardReads))
		names := make([]string, 0, len(r.StandardReads))
		for _, e := range r.StandardReads {
			names = append(names, esc(e.Table))
		}
		fmt.Printf("%s</p>\n", strings.Join(names, ", "))
	}

	fmt.Println(`</body></html>`)
}

// progress prints a rate-limited status line to stderr. Rate-limited so we
// do not flood the log on fast loops, but tight enough on slow loops that
// the user always sees something moving.
type progress struct {
	total       int
	lastPrintNS int64
	intervalNS  int64
}

func newProgress(total int, intervalSeconds int) *progress {
	return &progress{total: total, intervalNS: int64(intervalSeconds) * 1_000_000_000}
}

func (p *progress) tick(current int, label string) {
	if p.total <= 0 {
		return
	}
	nowNS := nowNano()
	if p.lastPrintNS != 0 && nowNS-p.lastPrintNS < p.intervalNS {
		return
	}
	p.lastPrintNS = nowNS
	pct := (current * 100) / p.total
	fmt.Fprintf(os.Stderr, "  [%3d%%] %d/%d  %s\n", pct, current, p.total, label)
}

func (p *progress) done(current int) {
	if p.total <= 0 {
		return
	}
	fmt.Fprintf(os.Stderr, "  [100%%] %d/%d done\n", current, p.total)
}

// devObj is the per-file canonical (type, name) pair used by
// collectCodeTables and queryCodeRefsForObject. Promoted to file scope so
// the worker pool in collectCodeTables can share the type with its helper.
type devObj struct {
	objType string
	objName string
}

// queryCodeRefsFunc is the signature of the "fetch CROSS/WBCROSSGT refs
// for one dev object" operation. It is declared as a package-level
// variable pointing at queryCodeRefsForObject so unit tests can swap in
// a deterministic fake to exercise the cache wrapper without needing a
// live SAP client.
var queryCodeRefsFunc = queryCodeRefsForObject

// queryCodeRefsCached wraps the per-object cross-ref fetch in a short-TTL
// L2 sqlite cache keyed by (objType, objName). A re-run of the same CR
// hits the cache for every scope object and skips all WBCROSSGT/CROSS
// traffic entirely. Source code changes relatively often, so the TTL is
// 1 hour — shorter than the DDIC cache window.
//
// When cache is nil (--no-cache or cache open failure) this degenerates
// to a direct call to queryCodeRefsFunc, making cache-on and cache-off
// behaviour trivially equivalent by construction (and directly tested).
func queryCodeRefsCached(ctx context.Context, client *adt.Client, name, objType string, cache *auditCache) []graph.TableCodeRef {
	key := "crossrefs:" + objType + ":" + strings.ToUpper(name)
	if cache != nil {
		var cached []graph.TableCodeRef
		if cache.getJSONTTL(key, shortCacheTTL, &cached) {
			return cached
		}
	}
	refs := queryCodeRefsFunc(ctx, client, name, objType)
	if cache != nil {
		cache.putJSON(key, refs)
	}
	return refs
}

// queryCodeRefsForObject pulls symbol references from WBCROSSGT (OO code)
// and CROSS (procedural code) for a single in-scope dev object. One
// object per call — SAP's freestyle query parser rejects multi-LIKE
// WHERE clauses ("LIKE is not allowed here"), so batching at the query
// level is impossible; we parallelise at the caller instead, see the
// worker pool in collectCodeTables.
//
// Returns every reference regardless of whether the target is actually
// a table — DD02L cross-check happens at the caller. Names with
// `\component` suffix are stripped to the base symbol.
func queryCodeRefsForObject(ctx context.Context, client *adt.Client, name, objType string) []graph.TableCodeRef {
	var out []graph.TableCodeRef
	nameUp := strings.ToUpper(name)

	// WBCROSSGT OTYPE='TY' — type references, which include DDIC tables
	// alongside classes/interfaces/data elements. Filter out later via DD02L.
	wbQuery := fmt.Sprintf(
		"SELECT INCLUDE, NAME FROM WBCROSSGT WHERE OTYPE = 'TY' AND INCLUDE LIKE '%s%%'",
		nameUp)
	if wbResult, err := client.RunQuery(ctx, wbQuery, 2000); err == nil && wbResult != nil {
		for _, row := range wbResult.Rows {
			inc := strings.ToUpper(strings.TrimSpace(fmt.Sprintf("%v", row["INCLUDE"])))
			sym := strings.ToUpper(strings.TrimSpace(fmt.Sprintf("%v", row["NAME"])))
			sym = stripComponentSuffix(sym)
			if sym == "" || sym == nameUp {
				continue
			}
			out = append(out, graph.TableCodeRef{
				Table:       sym,
				FromInclude: inc,
				RefKind:     "WBGT_TY",
				Source:      "WBCROSSGT",
			})
		}
	}

	// CROSS.TYPE='S' — structure/table references for classic procedural code.
	// For FUGR we probe both the name-prefix (ZFG%) and the sub-include
	// prefix (LZFG%) that covers LZFG_U01, LZFG_TOP, etc. Two separate
	// queries because SAP freestyle rejects OR-chained LIKE predicates.
	crossPatterns := []string{nameUp + "%"}
	if objType == "FUGR" {
		crossPatterns = append(crossPatterns, "L"+nameUp+"%")
	}
	for _, pattern := range crossPatterns {
		crossQuery := fmt.Sprintf(
			"SELECT INCLUDE, NAME FROM CROSS WHERE TYPE = 'S' AND INCLUDE LIKE '%s'",
			pattern)
		if crossResult, err := client.RunQuery(ctx, crossQuery, 2000); err == nil && crossResult != nil {
			for _, row := range crossResult.Rows {
				inc := strings.ToUpper(strings.TrimSpace(fmt.Sprintf("%v", row["INCLUDE"])))
				sym := strings.ToUpper(strings.TrimSpace(fmt.Sprintf("%v", row["NAME"])))
				sym = stripComponentSuffix(sym)
				if sym == "" || sym == nameUp {
					continue
				}
				out = append(out, graph.TableCodeRef{
					Table:       sym,
					FromInclude: inc,
					RefKind:     "CROSS_S",
					Source:      "CROSS",
				})
			}
		}
	}
	return out
}

// stripComponentSuffix removes the `\TY:component` or `\component` suffix that
// WBCROSSGT attaches when a reference points at a subcomponent of a type —
// e.g. `ZEXAMPLE_STRUCT\TY:FIELD` becomes `ZEXAMPLE_STRUCT`. The base name is
// what matches DD02L.
func stripComponentSuffix(name string) string {
	if idx := strings.Index(name, "\\"); idx >= 0 {
		return name[:idx]
	}
	return name
}

// plausibleTableName filters names that can possibly be DDIC tables before
// hitting DD02L. DDIC TABNAME is CHAR(30) max and contains only A-Z, 0-9,
// /, _. Class includes use `=` fillers and exceed 30 chars, programs can be
// up to 40 chars, etc. — all are rejected here to keep DD02L happy.
func plausibleTableName(name string) bool {
	if name == "" || len(name) > 30 {
		return false
	}
	for _, r := range name {
		switch {
		case r >= 'A' && r <= 'Z':
		case r >= '0' && r <= '9':
		case r == '_' || r == '/':
		default:
			return false
		}
	}
	return true
}

// runChunkedINQuery runs a query whose %s placeholder is the IN-clause body,
// batched to 5 values per call to respect the SAP freestyle 255-char literal
// limit. Returns the union of all result rows; first hard error short-circuits.
func runChunkedINQuery(ctx context.Context, client *adt.Client, tmpl string, values []string) ([]map[string]any, error) {
	const batchSize = 5
	var all []map[string]any
	for i := 0; i < len(values); i += batchSize {
		end := i + batchSize
		if end > len(values) {
			end = len(values)
		}
		batch := values[i:end]
		quoted := make([]string, len(batch))
		for j, v := range batch {
			quoted[j] = "'" + v + "'"
		}
		query := fmt.Sprintf(tmpl, strings.Join(quoted, ","))
		res, err := client.RunQuery(ctx, query, 500)
		if err != nil {
			return all, err
		}
		if res != nil {
			all = append(all, res.Rows...)
		}
	}
	return all, nil
}

// splitTransportsByFunction queries E070.TRFUNCTION and partitions the TR list
// into Workbench (K/S) and Customizing (W). Anything else is reported as Other
// so the user can see unusual function codes (like C/R/E) instead of silently
// dropping them.
func splitTransportsByFunction(ctx context.Context, client *adt.Client, trList []string) (*graph.CRTransportSplit, error) {
	split := &graph.CRTransportSplit{}
	if len(trList) == 0 {
		return split, nil
	}

	rows, err := runChunkedINQuery(ctx, client,
		"SELECT TRKORR, TRFUNCTION FROM E070 WHERE TRKORR IN (%s)", trList)
	if err != nil {
		return nil, fmt.Errorf("E070 TRFUNCTION query failed: %w", err)
	}

	for _, row := range rows {
		tr := strings.TrimSpace(fmt.Sprintf("%v", row["TRKORR"]))
		fn := strings.ToUpper(strings.TrimSpace(fmt.Sprintf("%v", row["TRFUNCTION"])))
		if tr == "" {
			continue
		}
		switch fn {
		case "K", "S", "T":
			// K = Workbench request, S = Development/correction, T = task (workbench task)
			split.WorkbenchTRs = append(split.WorkbenchTRs, tr)
		case "W", "Q":
			// W = Customizing request, Q = Customizing task
			split.CustomizingTRs = append(split.CustomizingTRs, tr)
		default:
			split.OtherTRs = append(split.OtherTRs, graph.CRTransportOther{TR: tr, Function: fn})
		}
	}
	sort.Strings(split.WorkbenchTRs)
	sort.Strings(split.CustomizingTRs)
	sort.Slice(split.OtherTRs, func(i, j int) bool {
		return split.OtherTRs[i].TR < split.OtherTRs[j].TR
	})
	return split, nil
}

func printCRConfigAuditText(r *graph.CRConfigAuditReport, details bool) {
	fmt.Printf("CR %s — Code ↔ Customizing Alignment Audit\n", r.CRID)
	fmt.Printf("Workbench TRs: %d | Customizing TRs: %d", len(r.Transports.WorkbenchTRs), len(r.Transports.CustomizingTRs))
	if len(r.Transports.OtherTRs) > 0 {
		fmt.Printf(" | Other: %d", len(r.Transports.OtherTRs))
	}
	fmt.Println()

	if len(r.Transports.WorkbenchTRs) > 0 {
		fmt.Printf("  Workbench:   %s\n", strings.Join(r.Transports.WorkbenchTRs, ", "))
	}
	if len(r.Transports.CustomizingTRs) > 0 {
		fmt.Printf("  Customizing: %s\n", strings.Join(r.Transports.CustomizingTRs, ", "))
	}
	for _, o := range r.Transports.OtherTRs {
		fmt.Printf("  Other [%s]: %s\n", o.Function, o.TR)
	}

	fmt.Printf("Tables read by code: %d (custom: %d, SAP standard: %d)  |  Tables with transported data: %d\n",
		r.Summary.TablesReadByCode, r.Summary.TablesCustomRead, r.Summary.TablesStandardRead, r.Summary.TablesInCustTRs)

	status := "ALIGNED"
	if !r.Summary.Aligned {
		status = "NOT ALIGNED"
	}
	fmt.Printf("  Tables:   Covered: %d   Missing: %d   Orphan: %d\n",
		r.Summary.Covered, r.Summary.Missing, r.Summary.Orphan)
	if r.Summary.MetadataReachable > 0 || r.Summary.MetadataInCR > 0 {
		fmt.Printf("  Metadata: Covered: %d   Missing: %d   Orphan: %d   (reachable: %d, in CR: %d)\n",
			r.Summary.MetadataCovered, r.Summary.MetadataMissing, r.Summary.MetadataOrphan,
			r.Summary.MetadataReachable, r.Summary.MetadataInCR)
	}
	if r.Summary.ValueFindings > 0 {
		fmt.Printf("  Values:   Covered: %d   Missing: %d   (findings: %d)\n",
			r.Summary.ValueCovered, r.Summary.ValueMissing, r.Summary.ValueFindings)
	}
	fmt.Printf("  →  %s\n\n", status)

	if len(r.Missing) > 0 {
		fmt.Println("MISSING — custom tables read by code, not in any CR transport:")
		for _, e := range r.Missing {
			fmt.Printf("  %s\n", e.Table)
			if details {
				for _, ref := range e.CodeRefs {
					fmt.Printf("    ← %s  (%s, %s)\n", ref.FromObject, ref.FromInclude, ref.Source)
				}
			}
		}
		fmt.Println()
	}

	if len(r.Orphan) > 0 {
		// Split orphans into two visual groups: those reachable from
		// in-scope code via a 1-hop call chain (annotation only, still
		// orphan for bookkeeping) and the genuinely unreferenced ones.
		// The transitive group is less alarming — the CR's code does
		// drive the table, just through a helper function we can't see
		// in direct code-refs. Showing the chain keeps the user informed
		// without quietly hiding a real issue.
		var transitive, deadEnd []graph.CoverageEntry
		for _, e := range r.Orphan {
			if len(e.TransitiveReach) > 0 {
				transitive = append(transitive, e)
			} else {
				deadEnd = append(deadEnd, e)
			}
		}

		if len(deadEnd) > 0 {
			fmt.Println("ORPHAN — transported rows for tables no code in CR references:")
			for _, e := range deadEnd {
				fmt.Printf("  %-25s  %d rows\n", e.Table, len(e.CustRows))
				if details {
					for _, row := range e.CustRows {
						fmt.Printf("    %s  %s  TABKEY=%s\n", row.TRKORR, row.ObjFunc, row.TabKey)
					}
				}
			}
			fmt.Println()
		}

		if len(transitive) > 0 {
			fmt.Println("ORPHAN (reachable via 1-hop call chain — likely intentional indirect reads):")
			for _, e := range transitive {
				fmt.Printf("  %-25s  %d rows\n", e.Table, len(e.CustRows))
				for _, hop := range e.TransitiveReach {
					if hop.Depth == 0 {
						fmt.Printf("    ← %s  (direct read in %s)\n", hop.FromScope, hop.ReaderInclude)
					} else {
						fmt.Printf("    ← %s → %s → %s\n", hop.FromScope, hop.Via, e.Table)
						fmt.Printf("        call site: %s  (CROSS TYPE='F' NAME='%s')\n",
							hop.CallerInclude, stripColonName(hop.Via))
						fmt.Printf("        read site: %s  (WBCROSSGT OTYPE='TY' NAME='%s')\n",
							hop.ReaderInclude, e.Table)
					}
				}
				if details {
					for _, row := range e.CustRows {
						fmt.Printf("      row: %s  %s  TABKEY=%s\n", row.TRKORR, row.ObjFunc, row.TabKey)
					}
				}
			}
			fmt.Println()
		}
	}

	if len(r.Covered) > 0 {
		fmt.Println("COVERED — tables both read by code AND transported:")
		for _, e := range r.Covered {
			fmt.Printf("  %-25s  %d rows,  %d code refs\n", e.Table, len(e.CustRows), len(e.CodeRefs))
			if details {
				for _, row := range e.CustRows {
					fmt.Printf("    [data] %s  %s  TABKEY=%s\n", row.TRKORR, row.ObjFunc, row.TabKey)
				}
				for _, ref := range e.CodeRefs {
					fmt.Printf("    [code] ← %s  (%s, %s)\n", ref.FromObject, ref.FromInclude, ref.Source)
				}
			}
		}
		fmt.Println()
	}

	if len(r.MetadataMissing) > 0 {
		fmt.Println("METADATA MISSING — DTEL/DOMA/CheckTable reachable from scope code but not in CR:")
		for _, m := range r.MetadataMissing {
			fmt.Printf("  %-6s %-30s  ← %s\n", m.Kind, m.Name, m.FromTable)
			if details {
				fmt.Printf("           path: %s\n", strings.Join(m.Path, " → "))
			}
		}
		fmt.Println()
	}

	if len(r.MetadataOrphan) > 0 {
		fmt.Println("METADATA ORPHAN — DDIC metadata in CR not reachable from any scope table:")
		for _, m := range r.MetadataOrphan {
			fmt.Printf("  %-6s %s\n", m.Kind, m.Name)
		}
		fmt.Println()
	}

	if details && len(r.MetadataCovered) > 0 {
		fmt.Printf("METADATA COVERED (%d):\n", len(r.MetadataCovered))
		for _, m := range r.MetadataCovered {
			fmt.Printf("  %-6s %-30s  path: %s\n", m.Kind, m.Name, strings.Join(m.Path, " → "))
		}
		fmt.Println()
	}

	if len(r.ValueMissing) > 0 {
		fmt.Println("VALUE MISSING — code supplies literal key not present in any transported row:")
		for _, f := range r.ValueMissing {
			flag := ""
			if f.IncompleteKey {
				flag = " [partial-key]"
			}
			fmt.Printf("  %-10s %-30s  ← %s  (%s, line %d)%s\n",
				f.Table, renderFieldMap(f.ExpectedKeys), f.SourceObject, f.Via, f.Row, flag)
			if details && f.Note != "" {
				fmt.Printf("             note: %s\n", f.Note)
			}
		}
		fmt.Println()
	}

	if details && len(r.ValueCovered) > 0 {
		fmt.Printf("VALUE COVERED (%d):\n", len(r.ValueCovered))
		for _, f := range r.ValueCovered {
			flag := ""
			if f.IncompleteKey {
				flag = " [partial-key]"
			}
			fmt.Printf("  %-10s %-30s  ← %s  (%s, line %d)  matched %s%s\n",
				f.Table, renderFieldMap(f.ExpectedKeys), f.SourceObject, f.Via, f.Row, f.MatchedKeyDisplay, flag)
		}
		fmt.Println()
	}

	if details && len(r.StandardReads) > 0 {
		fmt.Printf("SAP standard tables read by code (%d, informational):\n", len(r.StandardReads))
		names := make([]string, 0, len(r.StandardReads))
		for _, e := range r.StandardReads {
			names = append(names, e.Table)
		}
		// Print in columns of ~6 per line for compactness.
		for i := 0; i < len(names); i += 6 {
			end := i + 6
			if end > len(names) {
				end = len(names)
			}
			fmt.Printf("  %s\n", strings.Join(names[i:end], ", "))
		}
	}
}
