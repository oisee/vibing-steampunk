package adt

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/oisee/vibing-steampunk/pkg/datacluster"
)

// A cluster carries types, not names. DD03L carries both, for every DDIC
// structure, in one row per component, in order, with a DEPTH column that
// nests the components of a structured field. Includes are the one thing
// it does not nest: the included structure's rows follow the .INCLUDE row
// at the same depth, and how many of them there are is only known from the
// include's own DD03L rows. StructureLayout reads it all with free SQL and
// turns it into the Layout that pkg/datacluster lays over an object.

type dd03lRow struct {
	Position  int
	Depth     int
	Field     string
	Precfield string
	Comptype  string
	Rollname  string
	Datatype  string
	Leng      int
	Decimals  int
}

// StructureLayout reads a DDIC structure or table type's components from
// DD03L, includes resolved, and returns them as a Layout.
func (c *Client) StructureLayout(ctx context.Context, name string) (*datacluster.Layout, error) {
	cache := map[string][]dd03lRow{}
	var read func(string) ([]dd03lRow, error)
	read = func(n string) ([]dd03lRow, error) {
		n = strings.ToUpper(strings.TrimSpace(n))
		if rows, ok := cache[n]; ok {
			return rows, nil
		}
		rows, err := c.dd03lRows(ctx, n)
		if err != nil {
			return nil, err
		}
		if len(rows) == 0 {
			return nil, fmt.Errorf("DDIC has no active structure %s", n)
		}
		cache[n] = rows
		return rows, nil
	}
	rows, err := read(name)
	if err != nil {
		return nil, err
	}
	return buildLayout(strings.ToUpper(strings.TrimSpace(name)), rows, read, 0)
}

func (c *Client) dd03lRows(ctx context.Context, name string) ([]dd03lRow, error) {
	query := fmt.Sprintf("SELECT position, depth, fieldname, precfield, comptype, rollname, datatype, leng, decimals FROM dd03l WHERE tabname = '%s' AND as4local = 'A' ORDER BY position", sqlQuote(name))
	res, err := c.RunQuery(ctx, query, 5000)
	if err != nil {
		return nil, fmt.Errorf("reading DD03L for %s: %w", name, err)
	}
	if res == nil {
		return nil, nil
	}
	rows := make([]dd03lRow, 0, len(res.Rows))
	for _, r := range res.Rows {
		row := dd03lRow{
			Field: cell(r, "FIELDNAME"), Precfield: cell(r, "PRECFIELD"), Comptype: cell(r, "COMPTYPE"),
			Rollname: cell(r, "ROLLNAME"), Datatype: cell(r, "DATATYPE"),
		}
		row.Position, _ = strconv.Atoi(cell(r, "POSITION"))
		row.Depth, _ = strconv.Atoi(cell(r, "DEPTH"))
		row.Leng, _ = strconv.Atoi(cell(r, "LENG"))
		row.Decimals, _ = strconv.Atoi(cell(r, "DECIMALS"))
		rows = append(rows, row)
	}
	return rows, nil
}

func isIncludeRow(r dd03lRow) bool {
	return strings.HasPrefix(r.Field, ".INCLU") || strings.HasPrefix(r.Field, ".APPEND")
}

// buildLayout turns DD03L rows into a Layout. resolve reads the rows of an
// included structure; nesting deeper than twenty is taken as a cycle.
func buildLayout(name string, rows []dd03lRow, resolve func(string) ([]dd03lRow, error), nesting int) (*datacluster.Layout, error) {
	if nesting > 20 {
		return nil, fmt.Errorf("structure %s nests deeper than twenty levels; probably an include cycle", name)
	}
	i := 0
	comps, err := parseComponents(name, rows, &i, 0, resolve, nesting)
	if err != nil {
		return nil, err
	}
	if i != len(rows) {
		return nil, fmt.Errorf("structure %s: DD03L row %d (%s) is at a depth that has no parent", name, i+1, rows[i].Field)
	}
	return &datacluster.Layout{Name: name, Components: comps}, nil
}

func parseComponents(name string, rows []dd03lRow, i *int, depth int, resolve func(string) ([]dd03lRow, error), nesting int) ([]datacluster.Component, error) {
	var comps []datacluster.Component
	for *i < len(rows) {
		r := rows[*i]
		if r.Depth < depth {
			return comps, nil
		}
		if r.Depth > depth {
			return nil, fmt.Errorf("structure %s: row %s is at depth %d under a field that is not a structure", name, r.Field, r.Depth)
		}
		switch {
		case isIncludeRow(r):
			incRows, err := resolve(r.Precfield)
			if err != nil {
				return nil, err
			}
			sub, err := buildLayout(strings.ToUpper(r.Precfield), incRows, resolve, nesting+1)
			if err != nil {
				return nil, err
			}
			// The include's rows follow this one at the same depth, one per
			// row the include has itself (its own nested rows included).
			*i += 1 + len(incRows)
			comps = append(comps, datacluster.Component{Name: r.Field, Kind: datacluster.IncludeField, Type: strings.ToUpper(r.Precfield), Sub: sub})
		case r.Comptype == "S":
			*i++
			subComps, err := parseComponents(name+"-"+r.Field, rows, i, depth+1, resolve, nesting)
			if err != nil {
				return nil, err
			}
			comps = append(comps, datacluster.Component{Name: r.Field, Kind: datacluster.SubstructureField, Type: r.Rollname, Sub: &datacluster.Layout{Name: r.Rollname, Components: subComps}})
		case r.Comptype == "L":
			*i++
			comps = append(comps, datacluster.Component{Name: r.Field, Kind: datacluster.TableField, Type: r.Rollname})
		case r.Comptype == "R":
			return nil, fmt.Errorf("structure %s: component %s is a reference, which a cluster does not carry", name, r.Field)
		default:
			*i++
			comps = append(comps, datacluster.Component{Name: r.Field, Kind: datacluster.ElementaryField, Type: r.Datatype, Chars: r.Leng, Decimals: r.Decimals})
		}
	}
	return comps, nil
}
