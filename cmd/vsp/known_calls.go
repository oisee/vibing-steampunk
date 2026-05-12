package main

// knownCall is a compile-time entry describing a known SAP customizing
// function module: which DDIC table it reads or writes, how each exported
// FM parameter maps to a field of that table, and which of those fields
// together form the business key we want to cross-check against the CR's
// transported rows.
//
// keyFields is intentionally a separate list from argMap values. Not every
// mapped parameter is part of the identity of a row — e.g. APPL_LOG_INIT
// has more parameters than just OBJECT and SUBOBJECT, but only those two
// form the primary key of BALSUB. The extractor uses keyFields to decide
// whether a caller supplied the full business key (IncompleteKey=false)
// or only a subset; the matcher uses it to drive subset semantics.
type knownCall struct {
	table     string            // target DDIC table name (upper-cased)
	argMap    map[string]string // lowercase FM param name → upper-cased key field
	keyFields []string          // upper-cased ordered business key
}

// knownCustCalls is the v2a-min registry of SAP customizing function
// modules we recognise. Kept deliberately short — signal per entry is
// high and any false-positive here shows up immediately as a real CR
// finding, not as extractor noise.
//
// Structure additions should land with a one-line comment explaining why
// the mapping matters for customizing alignment. "Because SAP standard"
// is never a justification — we only register FMs whose input literals
// double as customizing table keys.
var knownCustCalls = map[string]knownCall{
	// BAL (Application Log) — the prod-incident fixture from TR-EXAMPLE.
	// APPL_LOG_INIT registers a log handle by (OBJECT, SUBOBJECT), which
	// must already exist in BALSUB (and its text table BALSUBT).
	"APPL_LOG_INIT": {
		table: "BALSUB",
		argMap: map[string]string{
			"object":     "OBJECT",
			"sub_object": "SUBOBJECT",
		},
		keyFields: []string{"OBJECT", "SUBOBJECT"},
	},
	"APPL_LOG_SET_OBJECT_AND_SUBOBJECT": {
		table: "BALSUB",
		argMap: map[string]string{
			"object":     "OBJECT",
			"sub_object": "SUBOBJECT",
		},
		keyFields: []string{"OBJECT", "SUBOBJECT"},
	},

	// Number range object lookups travel via NRIV / TNRO. The FM parameter
	// name is OBJECT, which matches the NRO OBJECT key field directly.
	"NUMBER_GET_NEXT": {
		table: "TNRO",
		argMap: map[string]string{
			"object":      "OBJECT",
			"nr_range_nr": "NRRANGENR",
		},
		keyFields: []string{"OBJECT", "NRRANGENR"},
	},
}
