package main

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/oisee/vibing-steampunk/pkg/adt"
)

// ddKeyField is the slice of DD03L metadata needed to unpack an E071K
// TABKEY into its component field values. Name is the DDIC field name,
// Length is the byte width of the key field as declared in INTLEN.
type ddKeyField struct {
	Name   string `json:"name"`
	Length int    `json:"length"`
}

// fetchKeyFields returns the ordered list of non-client key fields for the
// given DDIC table. "Non-client" because every client-dependent table in
// SAP stores MANDT as the first key field, but MANDT is never relevant
// for code-literal matching — no ABAP CALL FUNCTION supplies a MANDT
// literal — so we strip it here and normalise the TABKEY offsets around
// that assumption in unpackTabkey below.
//
// Results are cached in the L2 sqlite store under dd03l:keys:<TABLE>.
// DD03L is effectively static within the 24h TTL window the cache uses.
func fetchKeyFields(ctx context.Context, client *adt.Client, table string, cache *auditCache) ([]ddKeyField, error) {
	table = strings.ToUpper(strings.TrimSpace(table))
	if table == "" {
		return nil, nil
	}
	key := "dd03l:keys:" + table
	if cache != nil {
		var cached []ddKeyField
		if cache.getJSON(key, &cached) {
			return cached, nil
		}
	}
	q := fmt.Sprintf(
		"SELECT FIELDNAME, INTLEN, POSITION FROM DD03L WHERE TABNAME = '%s' AND KEYFLAG = 'X' AND AS4LOCAL = 'A' ORDER BY POSITION",
		table)
	res, err := client.RunQuery(ctx, q, 200)
	if err != nil {
		return nil, fmt.Errorf("DD03L key-fields query for %s failed: %w", table, err)
	}
	if res == nil {
		return nil, nil
	}
	out := make([]ddKeyField, 0, len(res.Rows))
	for _, row := range res.Rows {
		name := strings.ToUpper(strings.TrimSpace(fmt.Sprintf("%v", row["FIELDNAME"])))
		if name == "" || name == "MANDT" || name == "CLIENT" {
			continue
		}
		lenStr := strings.TrimSpace(fmt.Sprintf("%v", row["INTLEN"]))
		length, err := strconv.Atoi(lenStr)
		if err != nil || length <= 0 {
			continue
		}
		out = append(out, ddKeyField{Name: name, Length: length})
	}
	if cache != nil {
		cache.putJSON(key, out)
	}
	return out, nil
}

// unpackTabkey splits a single E071K.TABKEY string into a map of
// field → value using DD03L key-field widths for the given table. The
// leading 3 bytes of TABKEY are the client (MANDT) and are skipped — the
// fetchKeyFields helper has already filtered MANDT out of the key list,
// so offsets start at byte 3 and advance by each field's declared length.
//
// Values are right-trimmed (CHAR fields come padded with spaces in TABKEY)
// and uppercased so they compare cleanly against extractCodeLiterals
// output, which normalises to upper.
//
// The function is tolerant of a TABKEY shorter than the declared key
// length total — transports sometimes record only leading key fields and
// leave the tail blank. Missing fields simply don't end up in the map.
func unpackTabkey(tabkey string, keyFields []ddKeyField) map[string]string {
	out := make(map[string]string, len(keyFields))
	const mandtWidth = 3
	offset := mandtWidth
	if len(tabkey) < mandtWidth {
		return out
	}
	for _, f := range keyFields {
		if offset >= len(tabkey) {
			break
		}
		end := offset + f.Length
		if end > len(tabkey) {
			end = len(tabkey)
		}
		raw := tabkey[offset:end]
		val := strings.ToUpper(strings.TrimRight(raw, " "))
		if val != "" {
			out[f.Name] = val
		}
		offset = end
	}
	return out
}
