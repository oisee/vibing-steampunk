package main

import (
	"context"
	"fmt"
	"os"
	"sort"
	"strings"

	"github.com/oisee/vibing-steampunk/pkg/adt"
	"github.com/oisee/vibing-steampunk/pkg/graph"
)

// matchValueLevelFindings takes the set of CodeLiteralCall entries the
// extractor found across every in-scope source object, looks up DD03L key
// metadata for each referenced target table, unpacks transported
// TABKEYs, and classifies each code-side tuple as ValueCovered or
// ValueMissing. Subset semantics are intentional — the code side is the
// predicate, the transport side is the universe: a code tuple matches if
// every field it supplied also appears in some transported row with the
// same literal value. IncompleteKey findings still classify the same way;
// the flag just tags them for the output layer.
//
// v2a-min deliberately does not produce ValueOrphan. Without near-complete
// extractor coverage, most transported rows would look orphan and flood
// the report. Once CALL FUNCTION + simple SELECT path both land, orphan
// classification becomes useful.
func matchValueLevelFindings(
	ctx context.Context,
	client *adt.Client,
	calls []CodeLiteralCall,
	custTables map[string][]graph.TableCustRow,
	cache *auditCache,
) ([]graph.ValueLevelFinding, error) {
	if len(calls) == 0 {
		return nil, nil
	}

	// Collect the set of target tables we will touch, so DD03L key-field
	// fetches run once per table no matter how many call sites point at
	// the same one. Order the list for stable progress output.
	tableSet := make(map[string]bool)
	for _, c := range calls {
		tableSet[c.Target] = true
	}
	tables := make([]string, 0, len(tableSet))
	for t := range tableSet {
		tables = append(tables, t)
	}
	sort.Strings(tables)

	keyFieldsByTable := make(map[string][]ddKeyField, len(tables))
	unpackedByTable := make(map[string][]map[string]string, len(tables))
	for _, t := range tables {
		kf, err := fetchKeyFields(ctx, client, t, cache)
		if err != nil {
			fmt.Fprintf(os.Stderr, "  [value-match warn] %v\n", err)
			continue
		}
		keyFieldsByTable[t] = kf

		// Pre-unpack every transported row once so N call sites against
		// the same table share a single unpack pass.
		rows := custTables[t]
		unpacked := make([]map[string]string, 0, len(rows))
		for _, row := range rows {
			unpacked = append(unpacked, unpackTabkey(row.TabKey, kf))
		}
		unpackedByTable[t] = unpacked
	}

	var findings []graph.ValueLevelFinding
	dropNonKey := 0
	dropNoTable := 0
	for _, call := range calls {
		// Direct-SELECT findings carry every literal WHERE predicate the
		// extractor saw, including non-key fields. Filter them down to
		// real DDIC key fields here — "WHERE status = 'X'" against a
		// status field that is not part of the table's primary key has
		// no business being a value-level finding. CALL_FUNCTION
		// findings are already pre-keyed via the FM registry's
		// keyFields, so this filter is a no-op for them.
		expected := copyFields(call.Fields)
		if call.Kind == "direct_select" {
			kf, ok := keyFieldsByTable[call.Target]
			if !ok || len(kf) == 0 {
				// Unknown table or no key metadata → nothing we can
				// validate. Skip silently but count for the summary.
				dropNoTable++
				continue
			}
			keySet := make(map[string]bool, len(kf))
			for _, f := range kf {
				keySet[f.Name] = true
			}
			filtered := make(map[string]string, len(expected))
			for k, v := range expected {
				if keySet[k] {
					filtered[k] = v
				}
			}
			if len(filtered) == 0 {
				// All literals were on non-key fields — common when
				// code does `SELECT WHERE status = 'X'` against tables
				// where status is not part of the primary key. Counted
				// in the summary so the user understands the gap.
				dropNonKey++
				if os.Getenv("VSP_VALUE_DEBUG") != "" {
					nonKeys := make([]string, 0, len(call.Fields))
					for k, v := range call.Fields {
						nonKeys = append(nonKeys, k+"='"+v+"'")
					}
					sort.Strings(nonKeys)
					keyNames := make([]string, len(kf))
					for i, f := range kf {
						keyNames[i] = f.Name
					}
					fmt.Fprintf(os.Stderr, "  [value debug] %s %s line %d: non-key WHERE %s (table key: %v)\n",
						call.SourceObject, call.Via, call.Row, strings.Join(nonKeys, " "), keyNames)
				}
				continue
			}
			expected = filtered
			// IncompleteKey for SELECT is "we matched fewer key fields
			// than the table actually has". E.g. a SELECT WHERE
			// object='ZTEST' on BALSUB only filled OBJECT, not
			// SUBOBJECT — the finding is still valid but partial-key.
			if len(expected) < len(kf) {
				call.IncompleteKey = true
			}
		}

		finding := graph.ValueLevelFinding{
			Table:         call.Target,
			SourceObject:  call.SourceObject,
			Via:           call.Via,
			Kind:          call.Kind,
			Row:           call.Row,
			ExpectedKeys:  expected,
			IncompleteKey: call.IncompleteKey,
		}

		if _, ok := keyFieldsByTable[call.Target]; !ok {
			// We never resolved the key metadata (table not in DD03L, or
			// the query failed). Mark it missing with an explanatory note
			// rather than silently dropping — the user needs to see that
			// this target could not be matched at all.
			finding.Status = "MISSING"
			finding.Note = "no DD03L key metadata available for target"
			findings = append(findings, finding)
			continue
		}

		unpacked := unpackedByTable[call.Target]
		matched := false
		for _, row := range unpacked {
			if subsetMatch(call.Fields, row) {
				matched = true
				// Record the exact matched row for traceability in the
				// report: a compact `field=value field=value` rendering
				// of only the keys the call actually cared about.
				finding.MatchedKeyDisplay = renderKeyMap(call.Fields, row)
				break
			}
		}
		if matched {
			finding.Status = "COVERED"
		} else {
			finding.Status = "MISSING"
		}
		findings = append(findings, finding)
	}

	if dropNonKey > 0 || dropNoTable > 0 {
		fmt.Fprintf(os.Stderr, "Value-level: dropped %d direct-SELECT calls (non-key WHERE: %d, unknown target: %d)\n",
			dropNonKey+dropNoTable, dropNonKey, dropNoTable)
	}

	// Stable output order: by (table, source object, via, row).
	sort.Slice(findings, func(i, j int) bool {
		if findings[i].Table != findings[j].Table {
			return findings[i].Table < findings[j].Table
		}
		if findings[i].SourceObject != findings[j].SourceObject {
			return findings[i].SourceObject < findings[j].SourceObject
		}
		if findings[i].Via != findings[j].Via {
			return findings[i].Via < findings[j].Via
		}
		return findings[i].Row < findings[j].Row
	})
	return findings, nil
}

// subsetMatch reports whether every field in `want` appears in `have`
// with the same value. Extra fields in `have` are allowed (that is the
// "transport carries a bigger tuple than the code asked for" case we
// want to cover). An empty `want` never matches — a CodeLiteralCall with
// zero fields should never have reached this function.
func subsetMatch(want, have map[string]string) bool {
	if len(want) == 0 {
		return false
	}
	for k, v := range want {
		if have[k] != v {
			return false
		}
	}
	return true
}

// copyFields duplicates the map so the finding owns its own copy and the
// downstream report cannot accidentally mutate the extractor's state.
func copyFields(m map[string]string) map[string]string {
	out := make(map[string]string, len(m))
	for k, v := range m {
		out[k] = v
	}
	return out
}

// renderKeyMap formats only the fields the call supplied, pulled out of
// the matched transport row, so the report shows "why is this covered"
// in the same shape the extractor saw.
func renderKeyMap(want, have map[string]string) string {
	keys := make([]string, 0, len(want))
	for k := range want {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	parts := make([]string, 0, len(keys))
	for _, k := range keys {
		parts = append(parts, fmt.Sprintf("%s=%s", k, have[k]))
	}
	return strings.Join(parts, " ")
}
