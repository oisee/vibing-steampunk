package main

import (
	"context"
	"fmt"
	"os"
	"sort"
	"strings"

	"github.com/oisee/vibing-steampunk/pkg/adt"
)

// CRDevObject is a canonical, de-duplicated parent-level entry derived from
// one or more E071 rows (R3TR and/or LIMU) belonging to a CR's transports.
// LIMU subcomponents are already collapsed to their parent object here, so
// callers can reason in terms of whole classes / programs / groups without
// caring about which specific method or include triggered the transport.
type CRDevObject struct {
	ObjectType string   // "CLAS" | "PROG" | "INTF" | "FUGR" | "DDLS" | "TABL" | ...
	ObjectName string   // parent object name, upper-cased
	LIMUKinds  []string // granular pieces seen (e.g. "METH", "CPRI") — empty if only R3TR
	Package    string   // DEVCLASS from TADIR; empty if TADIR lookup failed or object is deleted
}

// CRDeletedRef is an E071 entry that cannot be resolved to a live object:
// either TADIR does not know it or DELFLAG='X'. Common cause is a stale
// transport pointing at a class that has since been deleted. cr-boundaries
// currently emits 404 WARNs for these; reporting them explicitly lets the
// caller treat them as a separate bucket.
type CRDeletedRef struct {
	ObjectType string
	ObjectName string
	Source     string // "R3TR" or "LIMU(<kind>)"
}

// collectCRDevObjects is the single entry-point both cr-boundaries and
// cr-config-audit use to extract the set of parent dev objects covered by a
// CR's transports. It queries E071 for both R3TR and LIMU rows, collapses
// LIMU subcomponents to their parent (method→class, report→program, etc.),
// and cross-checks every resulting parent name against TADIR so deleted or
// nonexistent entries are surfaced separately rather than silently errored
// out at source-fetch time.
//
// The returned CRDevObject and CRDeletedRef slices are sorted for stable
// output. Any LIMU kinds we do not know how to map (e.g. DOCU, MESS) are
// ignored; the caller gets a warning count but they are not considered code
// objects.
func collectCRDevObjects(ctx context.Context, client *adt.Client, trList []string) ([]CRDevObject, []CRDeletedRef, error) {
	if len(trList) == 0 {
		return nil, nil, nil
	}

	// One query per type+TR-batch: E071 rows for R3TR and LIMU in the TR set.
	// Batched to 5 TRs per IN clause (SAP freestyle 255-char literal limit).
	rows, err := runChunkedINQuery(ctx, client,
		"SELECT TRKORR, PGMID, OBJECT, OBJ_NAME FROM E071 WHERE PGMID IN ('R3TR','LIMU') AND TRKORR IN (%s)",
		trList)
	if err != nil {
		return nil, nil, fmt.Errorf("E071 query failed: %w", err)
	}

	type parentKey struct{ objType, objName string }
	parents := make(map[parentKey]*CRDevObject)
	var unknownLIMU int

	// Cache FUGR lookup results so we only query TFDIR once per FM name.
	// Pre-populate the whole cache in one batched pass across the entire
	// input row set, so resolveLIMUParent can route LIMU FUNC → FUGR
	// without an N+1 per-row TFDIR round-trip.
	fmToFugr := make(map[string]string)
	var fmNames []string
	fmSeen := make(map[string]bool)
	for _, row := range rows {
		if strings.ToUpper(strings.TrimSpace(fmt.Sprintf("%v", row["PGMID"]))) != "LIMU" {
			continue
		}
		if strings.ToUpper(strings.TrimSpace(fmt.Sprintf("%v", row["OBJECT"]))) != "FUNC" {
			continue
		}
		nm := strings.ToUpper(strings.TrimSpace(fmt.Sprintf("%v", row["OBJ_NAME"])))
		if nm != "" && !fmSeen[nm] {
			fmSeen[nm] = true
			fmNames = append(fmNames, nm)
		}
	}
	if len(fmNames) > 0 {
		if err := batchFMToFugr(ctx, client, fmNames, fmToFugr); err != nil {
			fmt.Fprintf(os.Stderr, "  (TFDIR batch for %d FMs failed: %v — falling back to per-row lookup)\n", len(fmNames), err)
		}
	}

	for _, row := range rows {
		pgmid := strings.ToUpper(strings.TrimSpace(fmt.Sprintf("%v", row["PGMID"])))
		objType := strings.ToUpper(strings.TrimSpace(fmt.Sprintf("%v", row["OBJECT"])))
		objName := strings.ToUpper(strings.TrimSpace(fmt.Sprintf("%v", row["OBJ_NAME"])))
		if objType == "" || objName == "" {
			continue
		}

		var parentType, parentName, limuKind string
		if pgmid == "R3TR" {
			parentType, parentName = objType, objName
		} else {
			limuKind = objType
			parentType, parentName = resolveLIMUParent(objType, objName, client, ctx, fmToFugr)
			if parentType == "" {
				unknownLIMU++
				continue
			}
		}

		key := parentKey{parentType, parentName}
		p, ok := parents[key]
		if !ok {
			p = &CRDevObject{ObjectType: parentType, ObjectName: parentName}
			parents[key] = p
		}
		if limuKind != "" {
			// Dedup LIMU kinds — we don't need every occurrence, just the
			// set of granular piece types seen for this parent.
			already := false
			for _, k := range p.LIMUKinds {
				if k == limuKind {
					already = true
					break
				}
			}
			if !already {
				p.LIMUKinds = append(p.LIMUKinds, limuKind)
			}
		}
	}

	if unknownLIMU > 0 {
		fmt.Fprintf(os.Stderr, "  (skipped %d LIMU entries of unsupported kinds — DOCU, MESS, etc.)\n", unknownLIMU)
	}

	// TADIR filter: keep only parents that actually exist and are not deleted.
	// Collect (type, name) pairs, batch-query TADIR, then split live vs deleted.
	tKeys := make([]tadirKey, 0, len(parents))
	for k := range parents {
		tKeys = append(tKeys, tadirKey{k.objType, k.objName})
	}
	alive, err := lookupTADIRAlive(ctx, client, tKeys)
	if err != nil {
		return nil, nil, err
	}

	var live []CRDevObject
	var deleted []CRDeletedRef
	for key, p := range parents {
		tKey := tadirKey{key.objType, key.objName}
		tadirRow, found := alive[tKey]
		// Fallback: some older systems report an object under a slightly
		// different type (CLAS/INTF interchangeability). Try by name only
		// if the strict (type, name) lookup missed.
		if !found {
			for k, v := range alive {
				if k.objName == key.objName && v.matches(key.objType) {
					tadirRow = v
					found = true
					break
				}
			}
		}
		if !found || tadirRow.deleted || !tadirRow.matches(key.objType) {
			src := "R3TR"
			if len(p.LIMUKinds) > 0 {
				src = "LIMU(" + strings.Join(p.LIMUKinds, ",") + ")"
			}
			deleted = append(deleted, CRDeletedRef{
				ObjectType: key.objType,
				ObjectName: key.objName,
				Source:     src,
			})
			continue
		}
		p.Package = tadirRow.devclass
		live = append(live, *p)
	}

	sort.Slice(live, func(i, j int) bool {
		if live[i].ObjectType != live[j].ObjectType {
			return live[i].ObjectType < live[j].ObjectType
		}
		return live[i].ObjectName < live[j].ObjectName
	})
	sort.Slice(deleted, func(i, j int) bool {
		if deleted[i].ObjectType != deleted[j].ObjectType {
			return deleted[i].ObjectType < deleted[j].ObjectType
		}
		return deleted[i].ObjectName < deleted[j].ObjectName
	})

	return live, deleted, nil
}

// resolveLIMUParent maps a LIMU entry to its parent R3TR object. Returns
// ("", "") for LIMU kinds we do not recognise — the caller treats those as
// "skip and count as unknown".
//
// METH/CPRI/CPRO/CPUB/CLSD/CINC → CLAS. For METH, OBJ_NAME is formatted as
// "<CLASSNAME>             <METHOD>" with the class padded to a fixed width;
// we extract the class by splitting on whitespace.
// REPS → PROG. TABD → TABL. INTD → INTF. DTED → DTEL. DOMD → DOMA.
// FUNC → FUGR, resolved via TFDIR (cached in fmToFugr).
func resolveLIMUParent(object, objName string, client *adt.Client, ctx context.Context, fmToFugr map[string]string) (string, string) {
	switch object {
	case "METH":
		// First whitespace-separated token is the class name.
		if idx := strings.IndexFunc(objName, func(r rune) bool { return r == ' ' || r == '\t' }); idx > 0 {
			return "CLAS", strings.ToUpper(objName[:idx])
		}
		return "CLAS", strings.ToUpper(objName)
	case "CPRI", "CPRO", "CPUB", "CLSD", "CINC":
		return "CLAS", strings.ToUpper(objName)
	case "INTD":
		return "INTF", strings.ToUpper(objName)
	case "REPS":
		return "PROG", strings.ToUpper(objName)
	case "TABD":
		return "TABL", strings.ToUpper(objName)
	case "DTED":
		return "DTEL", strings.ToUpper(objName)
	case "DOMD":
		return "DOMA", strings.ToUpper(objName)
	case "FUNC":
		name := strings.ToUpper(objName)
		if fg, ok := fmToFugr[name]; ok {
			return "FUGR", fg
		}
		fg := queryFMToFugr(ctx, client, name)
		fmToFugr[name] = fg
		if fg == "" {
			return "", ""
		}
		return "FUGR", fg
	}
	return "", ""
}

// batchFMToFugr pre-populates fmToFugr by querying TFDIR for up to `batchSize`
// function-module names per round-trip instead of one per name. This drops
// LIMU FUNC resolution for a CR with ~50 transported FMs from 50 sequential
// TFDIR round-trips to ~10 batched ones.
func batchFMToFugr(ctx context.Context, client *adt.Client, names []string, cache map[string]string) error {
	const batchSize = 5
	for i := 0; i < len(names); i += batchSize {
		end := i + batchSize
		if end > len(names) {
			end = len(names)
		}
		batch := names[i:end]
		quoted := make([]string, len(batch))
		for j, n := range batch {
			quoted[j] = "'" + strings.ToUpper(n) + "'"
		}
		q := fmt.Sprintf("SELECT FUNCNAME, PNAME FROM TFDIR WHERE FUNCNAME IN (%s)", strings.Join(quoted, ","))
		res, err := client.RunQuery(ctx, q, 100)
		if err != nil {
			return err
		}
		if res == nil {
			continue
		}
		for _, row := range res.Rows {
			fn := strings.ToUpper(strings.TrimSpace(fmt.Sprintf("%v", row["FUNCNAME"])))
			pn := strings.ToUpper(strings.TrimSpace(fmt.Sprintf("%v", row["PNAME"])))
			cache[fn] = strings.TrimPrefix(pn, "SAPL")
		}
	}
	// Record a negative hit for anything TFDIR did not answer, so the per-row
	// fallback doesn't loop back and re-query the same missing name.
	for _, n := range names {
		if _, ok := cache[strings.ToUpper(n)]; !ok {
			cache[strings.ToUpper(n)] = ""
		}
	}
	return nil
}

// queryFMToFugr returns the parent function group for a function module name
// by looking at TFDIR.PNAME (format: "SAPL<group>"). Returns empty if unknown.
func queryFMToFugr(ctx context.Context, client *adt.Client, fmName string) string {
	q := fmt.Sprintf("SELECT PNAME FROM TFDIR WHERE FUNCNAME = '%s'", fmName)
	res, err := client.RunQuery(ctx, q, 1)
	if err != nil || res == nil || len(res.Rows) == 0 {
		return ""
	}
	pname := strings.ToUpper(strings.TrimSpace(fmt.Sprintf("%v", res.Rows[0]["PNAME"])))
	return strings.TrimPrefix(pname, "SAPL")
}

// tadirRow holds the TADIR fields we care about. `matches` answers "does this
// TADIR row agree with the object type we collected from E071?" — a defensive
// check because LIMU→parent resolution can misfire on unusual naming, and we
// would rather drop such entries than confuse downstream analysis.
type tadirRow struct {
	objType  string
	devclass string
	deleted  bool
}

func (r tadirRow) matches(wantType string) bool {
	// Allow exact match and a couple of benign equivalents.
	if r.objType == wantType {
		return true
	}
	// Some older systems store CLAS/INTF interchangeably for interfaces; be permissive.
	if wantType == "INTF" && r.objType == "CLAS" {
		return true
	}
	return false
}

// tadirKey is the composite TADIR lookup key. Keying by name alone would
// collapse cases where the same name exists under multiple object types
// (e.g. a class and an interface with the same identifier) and could route
// a deleted row to the wrong parent. Keying by (type, name) keeps the
// mapping unambiguous.
type tadirKey struct{ objType, objName string }

// lookupTADIRAlive returns a map (type,name)→tadirRow for every unique
// object we want to confirm as live. Deleted entries (DELFLAG='X') are
// marked with deleted=true so the caller can report them explicitly.
// Missing entries simply do not appear in the map.
func lookupTADIRAlive(ctx context.Context, client *adt.Client, keys []tadirKey) (map[tadirKey]tadirRow, error) {
	out := make(map[tadirKey]tadirRow)
	if len(keys) == 0 {
		return out, nil
	}
	// Dedup, and collect the unique list of names — TADIR lookups batch by
	// name (OBJ_NAME IN (...)) while the result routing keys on (type, name).
	seen := make(map[tadirKey]bool, len(keys))
	nameSet := make(map[string]bool, len(keys))
	var uniqNames []string
	for _, k := range keys {
		u := tadirKey{strings.ToUpper(k.objType), strings.ToUpper(k.objName)}
		if !seen[u] {
			seen[u] = true
		}
		if !nameSet[u.objName] {
			nameSet[u.objName] = true
			uniqNames = append(uniqNames, u.objName)
		}
	}
	const batchSize = 5
	for i := 0; i < len(uniqNames); i += batchSize {
		end := i + batchSize
		if end > len(uniqNames) {
			end = len(uniqNames)
		}
		batch := uniqNames[i:end]
		quoted := make([]string, len(batch))
		for j, n := range batch {
			quoted[j] = "'" + n + "'"
		}
		query := fmt.Sprintf(
			"SELECT OBJECT, OBJ_NAME, DEVCLASS, DELFLAG FROM TADIR WHERE PGMID = 'R3TR' AND OBJ_NAME IN (%s)",
			strings.Join(quoted, ","))
		res, err := client.RunQuery(ctx, query, 100)
		if err != nil {
			return nil, fmt.Errorf("TADIR query failed: %w", err)
		}
		if res == nil {
			continue
		}
		for _, row := range res.Rows {
			name := strings.ToUpper(strings.TrimSpace(fmt.Sprintf("%v", row["OBJ_NAME"])))
			objType := strings.ToUpper(strings.TrimSpace(fmt.Sprintf("%v", row["OBJECT"])))
			devclass := strings.ToUpper(strings.TrimSpace(fmt.Sprintf("%v", row["DEVCLASS"])))
			delflag := strings.ToUpper(strings.TrimSpace(fmt.Sprintf("%v", row["DELFLAG"])))
			k := tadirKey{objType, name}
			// Prefer live entries if the same (type, name) has multiple rows.
			existing, have := out[k]
			if have && !existing.deleted {
				continue
			}
			out[k] = tadirRow{
				objType:  objType,
				devclass: devclass,
				deleted:  delflag == "X",
			}
		}
	}
	return out, nil
}
