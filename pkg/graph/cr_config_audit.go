package graph

import "sort"

// CRTransportSplit partitions the TRs of a single Change Request by their
// E070.TRFUNCTION code. Workbench holds K/S/T, Customizing holds W/Q, and
// anything else lands in OtherTRs so the audit can surface unusual codes
// instead of silently dropping them.
type CRTransportSplit struct {
	CRID           string             `json:"cr_id"`
	WorkbenchTRs   []string           `json:"workbench_trs"`
	CustomizingTRs []string           `json:"customizing_trs"`
	OtherTRs       []CRTransportOther `json:"other_trs,omitempty"`
}

// CRTransportOther captures TRs with TRFUNCTION codes we do not classify
// (e.g. C/R/E) so the user can see them in the report and decide whether
// to treat them as workbench or customizing.
type CRTransportOther struct {
	TR       string `json:"tr"`
	Function string `json:"function"`
}

// TableCodeRef records one code→table reference discovered via CROSS /
// WBCROSSGT or the parser. Multiple refs can exist per table.
type TableCodeRef struct {
	Table       string `json:"table"`
	FromObject  string `json:"from_object"`   // e.g. "CLAS:ZCL_FOO"
	FromInclude string `json:"from_include"`  // canonical include name from cross-ref
	RefKind     string `json:"ref_kind"`      // "DA" / "TABL" / "CDS_FROM" / ...
	Source      string `json:"source"`        // "CROSS" | "WBCROSSGT" | "PARSER"
}

// TableCustRow records one customizing data row transported in a TR that
// belongs to the CR. Values come from E071K.
type TableCustRow struct {
	Table   string `json:"table"`
	TRKORR  string `json:"trkorr"`
	TabKey  string `json:"tabkey"`
	ObjFunc string `json:"objfunc"` // I = insert, U = update, D = delete
}

// TransitiveReachHop records one "reachable via an intermediate call"
// chain from an in-scope object to an orphan table. FromScope is the
// starting object that lives inside the CR's code set; Via is the
// intermediate object (typically a FUGR or CLAS whose source we never
// see directly) that actually reads the orphan table. Depth is always
// 0 or 1 for v2a.3 — 0 means the scope object reads the table directly
// but we missed it in the forward scan (e.g. the FUGR name matches
// the table name exactly); 1 means one intermediate call is in the
// chain. Full N-hop propagation is a later refinement.
//
// CallerInclude and ReaderInclude point at the SAP include names that
// closed each half of the chain: CallerInclude calls the intermediate
// (depth 1) or reads the table directly (depth 0); ReaderInclude is
// the include inside the intermediate that does the actual SELECT on
// the orphan table. These are the hooks the user needs to open ADT
// and see the exact statements that form the chain.
type TransitiveReachHop struct {
	FromScope     string `json:"from_scope"`               // in-scope parent id, "TYPE:NAME"
	Via           string `json:"via"`                      // intermediate parent id, "TYPE:NAME"
	Depth         int    `json:"depth"`                    // 0 direct-missed | 1 one hop
	CallerInclude string `json:"caller_include,omitempty"` // SAP include where FromScope calls the intermediate
	ReaderInclude string `json:"reader_include,omitempty"` // SAP include where the intermediate reads the table
}

// CoverageEntry is one row of the final audit report. Depending on which
// bucket it lives in (Covered / Missing / Orphan) either side may be empty.
type CoverageEntry struct {
	Table         string         `json:"table"`
	DeliveryClass string         `json:"delivery_class,omitempty"`
	CodeRefs      []TableCodeRef `json:"code_refs,omitempty"`
	CustRows      []TableCustRow `json:"cust_rows,omitempty"`
	// TransitiveReach: when non-empty, this orphan is actually reached
	// from an in-scope object through a function-call chain. The entry
	// stays in the Orphan bucket (so the user sees the warning) but the
	// hops give context: the orphan data is not dead, it feeds a code
	// path inside this CR indirectly.
	TransitiveReach []TransitiveReachHop `json:"transitive_reach,omitempty"`
}

// ValueLevelFinding is one code-side literal lookup cross-checked against
// the set of transported row keys for its target table. It is the output
// unit of the v2a-min value-level audit: a single source statement that
// supplied literal key values, plus a verdict on whether those exact
// values are present in the CR's data side.
//
// IncompleteKey is sticky: when the code supplied fewer key fields than
// the registered business key, the match runs in subset mode and this
// flag lets the report explain that one code call may appear COVERED
// because a broader transport tuple subsumes it, not because an exact
// match exists.
type ValueLevelFinding struct {
	Table             string            `json:"table"`
	SourceObject      string            `json:"source_object"`
	Via               string            `json:"via"`  // "CALL_FUNCTION:APPL_LOG_INIT"
	Kind              string            `json:"kind"` // "known_call" (v2a-min); "direct_select" later
	Row               int               `json:"row"`
	ExpectedKeys      map[string]string `json:"expected_keys"`
	IncompleteKey     bool              `json:"incomplete_key,omitempty"`
	Status            string            `json:"status"` // "COVERED" | "MISSING"
	MatchedKeyDisplay string            `json:"matched_key,omitempty"`
	Note              string            `json:"note,omitempty"`
}

// MetadataRef is a DDIC metadata object (data element, domain, check table,
// search help) reached by walking the DD03L → DD04L → DD01L → DD07L chain
// out of a table in scope. Every entry records the path that got us here,
// so the report can explain "why is this DTEL in the graph?".
type MetadataRef struct {
	Kind      string   `json:"kind"`       // "DTEL" | "DOMA" | "CHKTAB" | "SHLP" | "FIXVAL"
	Name      string   `json:"name"`       // uppercased object name
	FromTable string   `json:"from_table"` // the table the chain started from (first hop)
	Path      []string `json:"path"`       // step-by-step trace, e.g. ["TABL:ZFOO","FIELD:KEY","DTEL:ZDTEL","DOMA:ZDOM"]
}

// CRConfigAuditReport is the final, rendered audit output.
type CRConfigAuditReport struct {
	CRID       string                    `json:"cr_id"`
	Transports CRTransportSplit          `json:"transports"`
	CodeTables map[string][]TableCodeRef `json:"code_tables,omitempty"`
	CustTables map[string][]TableCustRow `json:"cust_tables,omitempty"`

	// Covered: table is both read by code AND carried in a CR transport.
	Covered []CoverageEntry `json:"covered"`
	// Missing: custom (Z/Y) table read by code but not in any CR transport —
	// the primary alarm bucket.
	Missing []CoverageEntry `json:"missing"`
	// StandardReads: SAP-standard table read by code; listed for transparency
	// but never flagged as a gap, since SAP standard doesn't travel in CRs.
	StandardReads []CoverageEntry `json:"standard_reads,omitempty"`
	// Orphan: table has rows in a CR transport but no code in the CR reads it.
	Orphan []CoverageEntry `json:"orphan"`

	// DDIC metadata chain findings (v1.2a+). Collected by walking every table
	// in code scope through DD03L/DD04L/DD01L/DD07L. Missing = reachable from
	// scope-side code but not in CR; Orphan = transported but not reachable;
	// Covered = both.
	MetadataReachable map[string]MetadataRef `json:"metadata_reachable,omitempty"` // Kind:Name → ref
	MetadataInCR      map[string]MetadataRef `json:"metadata_in_cr,omitempty"`     // Kind:Name → ref
	MetadataCovered   []MetadataRef          `json:"metadata_covered,omitempty"`
	MetadataMissing   []MetadataRef          `json:"metadata_missing,omitempty"`
	MetadataOrphan    []MetadataRef          `json:"metadata_orphan,omitempty"`

	// Value-level findings (v2a-min). Only populated when the caller ran
	// extractCodeLiterals and matchValueLevelFindings, which hinges on
	// source-fetch per in-scope object. Missing entries are the primary
	// alarm — they mean a CALL FUNCTION to a customizing FM expects a
	// literal key whose exact value is not in any transported row.
	ValueFindings []ValueLevelFinding `json:"value_findings,omitempty"`
	ValueMissing  []ValueLevelFinding `json:"value_missing,omitempty"`
	ValueCovered  []ValueLevelFinding `json:"value_covered,omitempty"`

	Summary CRConfigAuditSummary `json:"summary"`
}

// CRConfigAuditSummary provides top-line numbers for the report.
type CRConfigAuditSummary struct {
	WorkbenchTRs       int `json:"workbench_trs"`
	CustomizingTRs     int `json:"customizing_trs"`
	TablesReadByCode   int `json:"tables_read_by_code"`
	TablesCustomRead   int `json:"tables_custom_read"`
	TablesStandardRead int `json:"tables_standard_read"`
	TablesInCustTRs    int `json:"tables_in_cust_trs"`
	Covered            int `json:"covered"`
	Missing            int `json:"missing"`
	Orphan             int `json:"orphan"`

	// DDIC metadata chain (v1.2a). Counts mirror the table buckets but live
	// on the metadata plane: data elements, domains, check tables, search
	// helps, domain fixed-value sets.
	MetadataReachable int `json:"metadata_reachable"`
	MetadataInCR      int `json:"metadata_in_cr"`
	MetadataCovered   int `json:"metadata_covered"`
	MetadataMissing   int `json:"metadata_missing"`
	MetadataOrphan    int `json:"metadata_orphan"`

	// Value-level (v2a-min).
	ValueFindings int `json:"value_findings"`
	ValueMissing  int `json:"value_missing"`
	ValueCovered  int `json:"value_covered"`

	Aligned bool `json:"aligned"` // all Missing==0 AND MetadataMissing==0 AND ValueMissing==0
}

// FinalizeCRConfigAuditReport cross-matches CodeTables against CustTables and
// populates Covered / Missing / StandardReads / Orphan buckets plus the Summary.
// Callers must have already filled CodeTables and CustTables; this is a pure
// function with no SAP dependencies so it stays cheap to unit test.
//
// A custom-namespace (Z/Y) table that code reads but no transport carries
// lands in Missing. A SAP-standard table that code reads lands in
// StandardReads (informational, never a gap). A table transported without
// code reading it lands in Orphan. A table in both ends up in Covered.
func FinalizeCRConfigAuditReport(r *CRConfigAuditReport) {
	deliveryClass := map[string]string{} // reserved for future DD02L enrichment

	codeTables := sortedKeys(r.CodeTables)
	custTables := sortedKeys(r.CustTables)

	custSet := make(map[string]bool, len(custTables))
	for _, t := range custTables {
		custSet[t] = true
	}
	codeSet := make(map[string]bool, len(codeTables))
	for _, t := range codeTables {
		codeSet[t] = true
	}

	customRead := 0
	standardRead := 0

	for _, t := range codeTables {
		entry := CoverageEntry{
			Table:         t,
			DeliveryClass: deliveryClass[t],
			CodeRefs:      r.CodeTables[t],
		}
		if custSet[t] {
			entry.CustRows = r.CustTables[t]
			r.Covered = append(r.Covered, entry)
			if IsStandardObject(t) {
				standardRead++
			} else {
				customRead++
			}
			continue
		}
		if IsStandardObject(t) {
			r.StandardReads = append(r.StandardReads, entry)
			standardRead++
		} else {
			r.Missing = append(r.Missing, entry)
			customRead++
		}
	}

	for _, t := range custTables {
		if codeSet[t] {
			continue // already handled under Covered
		}
		r.Orphan = append(r.Orphan, CoverageEntry{
			Table:         t,
			DeliveryClass: deliveryClass[t],
			CustRows:      r.CustTables[t],
		})
	}

	// Metadata cross-match: iterate reachable (from code-side tables) vs
	// in-CR (from E071 R3TR rows of DTEL/DOMA/SHLP/etc.). Only run if caller
	// populated the two input maps; leave buckets empty otherwise so v1.2a
	// can be disabled by passing nothing.
	reachableKeys := sortedKeys(r.MetadataReachable)
	inCRKeys := sortedKeys(r.MetadataInCR)
	reachableSet := make(map[string]bool, len(reachableKeys))
	for _, k := range reachableKeys {
		reachableSet[k] = true
	}
	inCRSet := make(map[string]bool, len(inCRKeys))
	for _, k := range inCRKeys {
		inCRSet[k] = true
	}
	for _, k := range reachableKeys {
		ref := r.MetadataReachable[k]
		if inCRSet[k] {
			r.MetadataCovered = append(r.MetadataCovered, ref)
		} else if !IsStandardObject(ref.Name) {
			r.MetadataMissing = append(r.MetadataMissing, ref)
		}
	}
	for _, k := range inCRKeys {
		if !reachableSet[k] {
			r.MetadataOrphan = append(r.MetadataOrphan, r.MetadataInCR[k])
		}
	}

	// Value-level bucketing (v2a-min). Caller populates r.ValueFindings
	// before calling us; we split into Covered/Missing based on the
	// per-finding Status tag the matcher set.
	for _, f := range r.ValueFindings {
		switch f.Status {
		case "COVERED":
			r.ValueCovered = append(r.ValueCovered, f)
		case "MISSING":
			r.ValueMissing = append(r.ValueMissing, f)
		}
	}

	r.Summary = CRConfigAuditSummary{
		WorkbenchTRs:       len(r.Transports.WorkbenchTRs),
		CustomizingTRs:     len(r.Transports.CustomizingTRs),
		TablesReadByCode:   len(codeTables),
		TablesCustomRead:   customRead,
		TablesStandardRead: standardRead,
		TablesInCustTRs:    len(custTables),
		Covered:            len(r.Covered),
		Missing:            len(r.Missing),
		Orphan:             len(r.Orphan),
		MetadataReachable:  len(reachableKeys),
		MetadataInCR:       len(inCRKeys),
		MetadataCovered:    len(r.MetadataCovered),
		MetadataMissing:    len(r.MetadataMissing),
		MetadataOrphan:     len(r.MetadataOrphan),
		ValueFindings:      len(r.ValueFindings),
		ValueCovered:       len(r.ValueCovered),
		ValueMissing:       len(r.ValueMissing),
		Aligned: len(r.Missing) == 0 &&
			len(r.Orphan) == 0 &&
			len(r.MetadataMissing) == 0 &&
			len(r.ValueMissing) == 0,
	}
}

func sortedKeys[V any](m map[string]V) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}
