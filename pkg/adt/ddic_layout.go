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

// ddicSource is where buildLayout gets the rows of a structure and the line
// of a table type; the client reads DD03L, DD40L and DD04L, a test supplies
// them.
type ddicSource struct {
	rows     func(structure string) ([]dd03lRow, error)
	lineType func(tableType string) (*lineType, error)
}

// lineType is what DD40L says a table type's line is: a structure by name,
// or an elementary type.
type lineType struct {
	Structure string
	Element   *dd03lRow
}

// StructureLayout reads a DDIC structure's components from DD03L, includes
// resolved and table-typed components followed into their line types, and
// returns them as a Layout.
func (c *Client) StructureLayout(ctx context.Context, name string) (*datacluster.Layout, error) {
	cache := map[string][]dd03lRow{}
	src := &ddicSource{}
	src.rows = func(n string) ([]dd03lRow, error) {
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
	src.lineType = func(t string) (*lineType, error) { return c.dd40lLine(ctx, t) }
	rows, err := src.rows(name)
	if err != nil {
		return nil, err
	}
	return buildLayout(strings.ToUpper(strings.TrimSpace(name)), rows, src, 0)
}

// dd40lLine reads a table type's line from DD40L: a structure (ROWKIND S),
// a data element (E with ROWTYPE), or a built-in type given right there.
func (c *Client) dd40lLine(ctx context.Context, tableType string) (*lineType, error) {
	query := fmt.Sprintf("SELECT rowkind, rowtype, datatype, leng, decimals FROM dd40l WHERE typename = '%s' AND as4local = 'A'", sqlQuote(strings.ToUpper(tableType)))
	res, err := c.RunQuery(ctx, query, 1)
	if err != nil {
		return nil, fmt.Errorf("reading DD40L for %s: %w", tableType, err)
	}
	if res == nil || len(res.Rows) == 0 {
		return nil, fmt.Errorf("DDIC has no active table type %s", tableType)
	}
	row := res.Rows[0]
	kind, rowType := cell(row, "ROWKIND"), cell(row, "ROWTYPE")
	switch kind {
	case "S":
		return &lineType{Structure: rowType}, nil
	case "E":
		el := &dd03lRow{Datatype: cell(row, "DATATYPE")}
		el.Leng, _ = strconv.Atoi(cell(row, "LENG"))
		el.Decimals, _ = strconv.Atoi(cell(row, "DECIMALS"))
		if rowType != "" {
			q := fmt.Sprintf("SELECT datatype, leng, decimals FROM dd04l WHERE rollname = '%s' AND as4local = 'A'", sqlQuote(rowType))
			r, err := c.RunQuery(ctx, q, 1)
			if err != nil {
				return nil, fmt.Errorf("reading DD04L for %s: %w", rowType, err)
			}
			if r != nil && len(r.Rows) > 0 {
				el.Datatype = cell(r.Rows[0], "DATATYPE")
				el.Leng, _ = strconv.Atoi(cell(r.Rows[0], "LENG"))
				el.Decimals, _ = strconv.Atoi(cell(r.Rows[0], "DECIMALS"))
			}
		}
		return &lineType{Element: el}, nil
	}
	return nil, fmt.Errorf("table type %s has a line of kind %q, which this reader does not lay out", tableType, kind)
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

// buildLayout turns DD03L rows into a Layout. src reads the rows of an
// included structure and the line of a table type; nesting deeper than
// twenty is taken as a cycle.
func buildLayout(name string, rows []dd03lRow, src *ddicSource, nesting int) (*datacluster.Layout, error) {
	if nesting > 20 {
		return nil, fmt.Errorf("structure %s nests deeper than twenty levels; probably an include cycle", name)
	}
	i := 0
	comps, err := parseComponents(name, rows, &i, 0, src, nesting)
	if err != nil {
		return nil, err
	}
	if i != len(rows) {
		return nil, fmt.Errorf("structure %s: DD03L row %d (%s) is at a depth that has no parent", name, i+1, rows[i].Field)
	}
	return &datacluster.Layout{Name: name, Components: comps}, nil
}

func parseComponents(name string, rows []dd03lRow, i *int, depth int, src *ddicSource, nesting int) ([]datacluster.Component, error) {
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
			incRows, err := src.rows(r.Precfield)
			if err != nil {
				return nil, err
			}
			sub, err := buildLayout(strings.ToUpper(r.Precfield), incRows, src, nesting+1)
			if err != nil {
				return nil, err
			}
			// The include's rows follow this one at the same depth, one per
			// row the include has itself (its own nested rows included).
			*i += 1 + len(incRows)
			comps = append(comps, datacluster.Component{Name: r.Field, Kind: datacluster.IncludeField, Type: strings.ToUpper(r.Precfield), Sub: sub})
		case r.Comptype == "S":
			*i++
			subComps, err := parseComponents(name+"-"+r.Field, rows, i, depth+1, src, nesting)
			if err != nil {
				return nil, err
			}
			comps = append(comps, datacluster.Component{Name: r.Field, Kind: datacluster.SubstructureField, Type: r.Rollname, Sub: &datacluster.Layout{Name: r.Rollname, Components: subComps}})
		case r.Comptype == "L":
			*i++
			comp := datacluster.Component{Name: r.Field, Kind: datacluster.TableField, Type: r.Rollname}
			if src.lineType != nil {
				line, err := src.lineType(r.Rollname)
				if err != nil {
					return nil, err
				}
				switch {
				case line.Structure != "":
					lineRows, err := src.rows(line.Structure)
					if err != nil {
						return nil, err
					}
					if comp.Sub, err = buildLayout(strings.ToUpper(line.Structure), lineRows, src, nesting+1); err != nil {
						return nil, err
					}
				case line.Element != nil:
					comp.Sub = &datacluster.Layout{Name: r.Rollname, Components: []datacluster.Component{
						{Name: "LINE", Kind: datacluster.ElementaryField, Type: line.Element.Datatype, Chars: line.Element.Leng, Decimals: line.Element.Decimals},
					}}
				}
			}
			comps = append(comps, comp)
		case r.Comptype == "R":
			return nil, fmt.Errorf("structure %s: component %s is a reference, which a cluster does not carry", name, r.Field)
		default:
			*i++
			comps = append(comps, datacluster.Component{Name: r.Field, Kind: datacluster.ElementaryField, Type: r.Datatype, Chars: r.Leng, Decimals: r.Decimals})
		}
	}
	return comps, nil
}
