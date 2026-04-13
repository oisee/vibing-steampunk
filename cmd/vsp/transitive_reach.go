package main

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/oisee/vibing-steampunk/pkg/adt"
	"github.com/oisee/vibing-steampunk/pkg/graph"
)

// runTransitiveReach walks every orphan table in the audit report
// backwards through the call graph, one hop deep, to find whether any
// in-scope code object reaches the orphan via a single function-call
// indirection. For each orphan it annotates `entry.TransitiveReach`
// with the discovered (FromScope, Via) pairs.
//
// The algorithm is pure SQL — it reuses the CROSS and WBCROSSGT
// tables that collectCodeTables already walks for the direct case.
// No source code is fetched: for each orphan table T we ask the two
// questions "who reads T?" and "who calls those readers?", then
// intersect the callers with the in-scope object set.
//
// Scope: single hop (v2a.3). Longer chains (N-hop) would need either
// recursive SQL — which SAP freestyle does not support — or a BFS
// over cached adjacency lists, which is v3 territory. One hop is
// the empirically-common case for "helper FM that reads a config
// table on behalf of a caller" and it closes a common false-positive
// class that the audit reports today, where a customising table looks
// orphan because the reader lives inside a helper FUGR that is not
// itself in the CR but is called from something that is.
func runTransitiveReach(
	ctx context.Context,
	client *adt.Client,
	report *graph.CRConfigAuditReport,
	scope map[string]bool,
	cache *auditCache,
) {
	if len(report.Orphan) == 0 || len(scope) == 0 {
		return
	}

	fmt.Fprintf(os.Stderr, "Transitive: checking %d orphan tables for 1-hop reach from scope...\n", len(report.Orphan))
	prog := newProgress(len(report.Orphan), 2)
	hits := 0
	for i := range report.Orphan {
		prog.tick(i, report.Orphan[i].Table)
		reach := findTransitiveReach(ctx, client, report.Orphan[i].Table, scope, cache)
		if len(reach) > 0 {
			report.Orphan[i].TransitiveReach = reach
			hits++
		}
	}
	prog.done(len(report.Orphan))
	if hits > 0 {
		fmt.Fprintf(os.Stderr, "Transitive: %d of %d orphans reachable via 1-hop call chain\n", hits, len(report.Orphan))
	}
}

// findTransitiveReach does the two-step lookup for one orphan table.
// Cached in L2 sqlite under `transitive:<table>` with short TTL so
// repeated audits on the same CR within an hour skip the SAP traffic.
func findTransitiveReach(
	ctx context.Context,
	client *adt.Client,
	table string,
	scope map[string]bool,
	cache *auditCache,
) []graph.TransitiveReachHop {
	table = strings.ToUpper(table)
	// Version the cache key so a binary upgrade that changes the
	// TransitiveReachHop struct shape does not hand back stale
	// partially-populated entries. Bump the v<n> prefix whenever
	// the persisted shape changes.
	cacheKey := "transitive:v2:" + table

	if cache != nil {
		var cached []graph.TransitiveReachHop
		if cache.getJSONTTL(cacheKey, shortCacheTTL, &cached) {
			return filterHopsByScope(cached, scope)
		}
	}

	// Step 1: direct readers of the table — every include that
	// touches `table` via WBCROSSGT OTYPE='TY' or CROSS TYPE='S'.
	readerIncludes := queryTableReaderIncludes(ctx, client, table)
	if len(readerIncludes) == 0 {
		return nil
	}

	// Map each include to its parent object (CLAS/INTF/FUGR/PROG)
	// but remember one representative include per parent — that's
	// what we render to the user so they can jump straight to the
	// statement that reads the table.
	readerParentToInclude := map[parentObj]string{}
	for _, inc := range readerIncludes {
		if p, ok := includeToParent(inc); ok {
			if _, already := readerParentToInclude[p]; !already {
				readerParentToInclude[p] = inc
			}
		}
	}

	// Step 2: for each reader parent, find who calls it. The call
	// originator — mapped back to a parent object — is the candidate
	// for matching against the in-scope set. We also keep one
	// representative "caller include" per in-scope match so the
	// finding can point at the exact statement that forms the chain.
	allHops := []graph.TransitiveReachHop{}
	for reader, readerInclude := range readerParentToInclude {
		// A reader that is ITSELF in scope means this orphan is
		// actually a direct read we missed in the forward scan —
		// typical when the reader FUGR/CLAS shares its name with
		// the table (so the forward-scan prefix match didn't fire).
		readerID := reader.objType + ":" + reader.objName
		if scope[readerID] {
			allHops = append(allHops, graph.TransitiveReachHop{
				FromScope:     readerID,
				Via:           readerID,
				Depth:         0,
				ReaderInclude: readerInclude,
			})
			continue
		}

		callerIncludes := queryCallersOfObject(ctx, client, reader)
		// Keep one include per caller parent — that's what the
		// report renders; the rest are uninteresting duplicates
		// from sibling sub-includes of the same FUGR.
		callerParentToInclude := map[parentObj]string{}
		for _, inc := range callerIncludes {
			if p, ok := includeToParent(inc); ok {
				if _, already := callerParentToInclude[p]; !already {
					callerParentToInclude[p] = inc
				}
			}
		}
		for caller, callerInclude := range callerParentToInclude {
			callerID := caller.objType + ":" + caller.objName
			if scope[callerID] {
				allHops = append(allHops, graph.TransitiveReachHop{
					FromScope:     callerID,
					Via:           readerID,
					Depth:         1,
					CallerInclude: callerInclude,
					ReaderInclude: readerInclude,
				})
			}
		}
	}

	if cache != nil {
		cache.putJSON(cacheKey, allHops)
	}
	return allHops
}

// queryTableReaderIncludes returns every include that references the
// given DDIC table in WBCROSSGT OTYPE='TY' or CROSS TYPE='S'. The two
// lookups together cover OO code and classic procedural code.
func queryTableReaderIncludes(ctx context.Context, client *adt.Client, table string) []string {
	seen := map[string]bool{}
	var out []string

	wb := fmt.Sprintf(
		"SELECT INCLUDE FROM WBCROSSGT WHERE OTYPE = 'TY' AND NAME = '%s'",
		table)
	if res, err := client.RunQuery(ctx, wb, 2000); err == nil && res != nil {
		for _, row := range res.Rows {
			inc := strings.ToUpper(strings.TrimSpace(fmt.Sprintf("%v", row["INCLUDE"])))
			if inc != "" && !seen[inc] {
				seen[inc] = true
				out = append(out, inc)
			}
		}
	}

	cr := fmt.Sprintf(
		"SELECT INCLUDE FROM CROSS WHERE TYPE = 'S' AND NAME = '%s'",
		table)
	if res, err := client.RunQuery(ctx, cr, 2000); err == nil && res != nil {
		for _, row := range res.Rows {
			inc := strings.ToUpper(strings.TrimSpace(fmt.Sprintf("%v", row["INCLUDE"])))
			if inc != "" && !seen[inc] {
				seen[inc] = true
				out = append(out, inc)
			}
		}
	}
	return out
}

// queryCallersOfObject returns includes that reference the given
// parent object as a callee. The query shape depends on the object
// type: FUGRs are called via CROSS TYPE='F' (function-module call),
// CLAS / INTF via WBCROSSGT OTYPE='TY' (the class / interface name
// appears as a type reference when code calls methods on it).
// PROG is queried via CROSS TYPE='P' (SUBMIT).
func queryCallersOfObject(ctx context.Context, client *adt.Client, obj parentObj) []string {
	seen := map[string]bool{}
	var out []string

	addIfNew := func(inc string) {
		inc = strings.ToUpper(strings.TrimSpace(inc))
		if inc != "" && !seen[inc] {
			seen[inc] = true
			out = append(out, inc)
		}
	}

	switch obj.objType {
	case "FUGR":
		// Callers of a function module: CROSS TYPE='F' NAME=<fmname>.
		// For a function group the callable FM names usually match the
		// group name for single-FM groups (common in customer code);
		// for multi-FM groups we would need TFDIR to enumerate all
		// FMs first. As a safe approximation we search by the group
		// name itself — which catches groups whose main FM shares
		// its name with the group, and misses finer-grained cases.
		q := fmt.Sprintf(
			"SELECT INCLUDE FROM CROSS WHERE TYPE = 'F' AND NAME = '%s'",
			obj.objName)
		if res, err := client.RunQuery(ctx, q, 2000); err == nil && res != nil {
			for _, row := range res.Rows {
				addIfNew(fmt.Sprintf("%v", row["INCLUDE"]))
			}
		}
	case "CLAS", "INTF":
		// Callers reference the class / interface name as a TY entry
		// in WBCROSSGT — DATA declarations, TYPE REF TO, method calls,
		// etc. all funnel through OTYPE='TY'.
		q := fmt.Sprintf(
			"SELECT INCLUDE FROM WBCROSSGT WHERE OTYPE = 'TY' AND NAME = '%s'",
			obj.objName)
		if res, err := client.RunQuery(ctx, q, 2000); err == nil && res != nil {
			for _, row := range res.Rows {
				addIfNew(fmt.Sprintf("%v", row["INCLUDE"]))
			}
		}
	case "PROG":
		// SUBMIT from another program → CROSS TYPE='P'.
		q := fmt.Sprintf(
			"SELECT INCLUDE FROM CROSS WHERE TYPE = 'P' AND NAME = '%s'",
			obj.objName)
		if res, err := client.RunQuery(ctx, q, 2000); err == nil && res != nil {
			for _, row := range res.Rows {
				addIfNew(fmt.Sprintf("%v", row["INCLUDE"]))
			}
		}
	}
	return out
}

// parentObj is the (type, name) pair used by the transitive walker
// to talk about ADT objects uniformly.
type parentObj struct {
	objType string
	objName string
}

// includeToParent reverse-engineers the parent object of a SAP include
// name by recognising the standard naming conventions:
//
//   - `ZCL_FOO========CP` / `==CI` / `==CO` / `==CM001` / `==CCAU` ... → CLAS ZCL_FOO
//   - `ZIF_FOO========IP` / `==IS` / `==IF`                              → INTF ZIF_FOO
//   - `SAPLZFOO`                                                          → FUGR ZFOO
//   - `LZFOOU01` / `LZFOOTOP` / `LZFOOF01` / `LZFOOI01` / ...             → FUGR ZFOO
//   - Anything else that looks like a plausible identifier                → PROG <include>
//
// Returns ok=false for names that look nothing like a SAP include (empty,
// contains whitespace, etc.), so the caller can skip noise rows.
func includeToParent(include string) (parentObj, bool) {
	include = strings.ToUpper(strings.TrimSpace(include))
	if include == "" {
		return parentObj{}, false
	}

	// Class / interface include: contains "========".
	if idx := strings.Index(include, "========"); idx > 0 {
		name := include[:idx]
		suffix := include[idx+8:]
		// Interface includes end in one of IP / IS / IF. Everything
		// else (CP, CI, CO, CS, CM001..CMNNN, CCAU, CCIMP, CCDEF,
		// CPUB, CPRI, CPRO, CU) is a class include.
		if suffix == "IP" || suffix == "IS" || suffix == "IF" {
			return parentObj{"INTF", name}, true
		}
		return parentObj{"CLAS", name}, true
	}

	// Function group header.
	if strings.HasPrefix(include, "SAPL") && len(include) > 4 {
		return parentObj{"FUGR", include[4:]}, true
	}

	// Function group include: L<NAME><suffix>. Suffix is one of the
	// standard SAP forms — UXX, TOP, U01..U99, F01.., I01.., E01..,
	// O01.., P01.., T00.., plus namespaced variants. We walk the
	// suffix from the right until we hit a non-digit non-letter
	// character or a section-start letter.
	if strings.HasPrefix(include, "L") && len(include) > 2 {
		stripped := include[1:]
		// Try known multi-char suffixes longest-first so "TOP"/"UXX"
		// are preferred over single-char fallbacks.
		for _, suffix := range []string{"UXX", "TOP", "TXX"} {
			if strings.HasSuffix(stripped, suffix) {
				return parentObj{"FUGR", strings.TrimSuffix(stripped, suffix)}, true
			}
		}
		// Numeric suffixes: U01..U99 / F01..F99 / I01..I99 etc. We
		// walk right-to-left skipping digits and then require the
		// next character to be one of the known section letters.
		i := len(stripped) - 1
		for i >= 0 && stripped[i] >= '0' && stripped[i] <= '9' {
			i--
		}
		if i >= 0 && i < len(stripped)-1 {
			switch stripped[i] {
			case 'U', 'F', 'I', 'E', 'O', 'P', 'T':
				return parentObj{"FUGR", stripped[:i]}, true
			}
		}
	}

	// Fallback: treat as standalone program / report. The caller's
	// scope check will filter if the name is not in scope anyway.
	return parentObj{"PROG", include}, true
}

// filterHopsByScope removes cached hops whose FromScope object is not
// in the current scope set. The cache is keyed by table, not by CR,
// so a hop recorded on one run may reference an in-scope object from
// a different CR's scope.
func filterHopsByScope(hops []graph.TransitiveReachHop, scope map[string]bool) []graph.TransitiveReachHop {
	out := hops[:0]
	for _, h := range hops {
		if scope[h.FromScope] {
			out = append(out, h)
		}
	}
	return out
}
