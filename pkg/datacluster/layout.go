package datacluster

import (
	"fmt"
	"strings"
)

// A cluster names its objects but not their fields: the descriptor says
// "CHAR of 40 bytes", never "MSGID". A Layout is the DDIC side of that — the
// components of the structure the exporting program used — and Apply lays
// it over an object's descriptor, walking both trees together and refusing
// to guess when they disagree.

// ComponentKind says what a DDIC component is.
type ComponentKind int

const (
	// ElementaryField is a field with a type, length and decimals.
	ElementaryField ComponentKind = iota
	// SubstructureField is a component whose type is itself a structure; the
	// cluster writes it as a nested descriptor.
	SubstructureField
	// IncludeField is a structure included into this one; the cluster may write
	// it as a nested descriptor or splice its fields in place.
	IncludeField
	// TableField is a component whose type is an internal table type. Deep,
	// and not something this package knows how to read from a cluster.
	TableField
)

// Layout is one structure's component list.
type Layout struct {
	Name       string
	Components []Component
}

// Component is one DDIC field of a structure.
type Component struct {
	Name string
	Kind ComponentKind
	// Type is the DDIC data type (CHAR, DEC, INT4, STRG, ...) for elementary
	// components and the structure name for the others.
	Type string
	// Chars is the DDIC length: characters for character types, digits for
	// packed numbers, bytes for raw.
	Chars    int
	Decimals int
	// Sub is the layout of a substructure or include.
	Sub *Layout
}

// Apply names the object's fields after the layout. It fails, and names
// nothing, when the layout does not fit the descriptor: a different number
// of components, a nested structure where the layout has a field, a length
// or type that disagrees. Every mismatch says where.
func (o *Object) Apply(l *Layout) error {
	if l == nil {
		return fmt.Errorf("no layout")
	}
	if o.Kind == Elementary {
		if len(l.Components) != 1 || l.Components[0].Kind != ElementaryField {
			return fmt.Errorf("%s is an elementary object; %s has %d components", o.Name, l.Name, len(l.Components))
		}
		if err := checkLeaf(o.Type, l.Components[0], o.unicode()); err != nil {
			return err
		}
		o.Fields[0].Name = l.Components[0].Name
		return nil
	}
	names := map[string]string{} // path → name
	nodes := valueNodes(o.Type)
	i := 0
	if err := walkLayout(nodes, &i, l.Components, "", names, o.unicode()); err != nil {
		return fmt.Errorf("%s does not fit %s: %w", l.Name, o.Name, err)
	}
	if i != len(nodes) {
		return fmt.Errorf("%s does not fit %s: the cluster has %d more field(s) than the layout", l.Name, o.Name, len(nodes)-i)
	}
	nameFields(o.Fields, names)
	return nil
}

func nameFields(fields []Field, names map[string]string) {
	for k := range fields {
		fields[k].Name = names[fields[k].Path]
		nameFields(fields[k].Fields, names)
	}
}

// unicode reports whether character fields are two bytes per character,
// which the object cannot know on its own; it is set by Parse.
func (o *Object) unicode() bool { return o.charBytes == 2 }

func valueNodes(n *Node) []*Node {
	var out []*Node
	for _, ch := range n.Children {
		if !ch.Filler {
			out = append(out, ch)
		}
	}
	return out
}

func walkLayout(nodes []*Node, i *int, comps []Component, prefix string, names map[string]string, unicode bool) error {
	for _, c := range comps {
		switch c.Kind {
		case TableField:
			if *i >= len(nodes) {
				return fmt.Errorf("layout has %s%s but the cluster has no field left for it", prefix, c.Name)
			}
			n := nodes[*i]
			if !n.Table {
				return fmt.Errorf("layout has table %s%s where the cluster has a %s field", prefix, c.Name, TypeName(n.TypeCode))
			}
			names[n.Path] = prefix + c.Name
			*i++
			if c.Sub == nil {
				continue // the line type stays numbered
			}
			// The line type's own descriptor children, structures and all.
			lineNodes := valueNodes(n)
			j := 0
			if err := walkLayout(lineNodes, &j, c.Sub.Components, prefix+c.Name+"[].", names, unicode); err != nil {
				return err
			}
			if j != len(lineNodes) {
				return fmt.Errorf("table %s%s: the line has %d more field(s) than %s", prefix, c.Name, len(lineNodes)-j, c.Sub.Name)
			}
		case SubstructureField:
			if *i >= len(nodes) {
				return fmt.Errorf("layout has %s%s but the cluster has no field left for it", prefix, c.Name)
			}
			n := nodes[*i]
			if len(n.Children) == 0 {
				return fmt.Errorf("layout has structure %s%s where the cluster has a %s field", prefix, c.Name, TypeName(n.TypeCode))
			}
			if c.Sub == nil {
				return fmt.Errorf("structure %s%s has no component list", prefix, c.Name)
			}
			*i++
			sub := valueNodes(n)
			j := 0
			if err := walkLayout(sub, &j, c.Sub.Components, prefix+c.Name+".", names, unicode); err != nil {
				return err
			}
			if j != len(sub) {
				return fmt.Errorf("structure %s%s: the cluster has %d more field(s) than %s", prefix, c.Name, len(sub)-j, c.Sub.Name)
			}
		case IncludeField:
			if c.Sub == nil {
				return fmt.Errorf("include %s%s has no component list", prefix, c.Name)
			}
			// The kernel writes an include either as a nested descriptor of
			// its own or spliced into the parent; both occur.
			if *i < len(nodes) && nodes[*i].Include {
				n := nodes[*i]
				*i++
				sub := valueNodes(n)
				j := 0
				if err := walkLayout(sub, &j, c.Sub.Components, prefix, names, unicode); err != nil {
					return err
				}
				if j != len(sub) {
					return fmt.Errorf("include %s: the cluster has %d more field(s) than it", c.Sub.Name, len(sub)-j)
				}
				continue
			}
			if err := walkLayout(nodes, i, c.Sub.Components, prefix, names, unicode); err != nil {
				return err
			}
		default:
			if *i >= len(nodes) {
				return fmt.Errorf("layout has %s%s but the cluster has no field left for it", prefix, c.Name)
			}
			n := nodes[*i]
			if len(n.Children) > 0 {
				return fmt.Errorf("layout has field %s%s where the cluster has a structure of %d fields", prefix, c.Name, len(valueNodes(n)))
			}
			if err := checkLeaf(n, c, unicode); err != nil {
				return fmt.Errorf("%s%s: %w", prefix, c.Name, err)
			}
			names[n.Path] = prefix + c.Name
			*i++
		}
	}
	return nil
}

// checkLeaf compares one DDIC field with one cluster field by type family
// and byte length.
func checkLeaf(n *Node, c Component, unicode bool) error {
	code, bytes, ok := ddicToCluster(c, unicode)
	if !ok {
		return fmt.Errorf("DDIC type %s is not one this reader knows", c.Type)
	}
	if !sameFamily(code, n.TypeCode) {
		return fmt.Errorf("DDIC says %s, the cluster holds %s", c.Type, TypeName(n.TypeCode))
	}
	if bytes != n.Length {
		return fmt.Errorf("DDIC %s(%d) is %d bytes, the cluster field is %d", c.Type, c.Chars, bytes, n.Length)
	}
	if n.TypeCode == typePacked && c.Decimals != n.Decimals {
		return fmt.Errorf("DDIC has %d decimals, the cluster %d", c.Decimals, n.Decimals)
	}
	return nil
}

func sameFamily(a, b byte) bool {
	if a == b {
		return true
	}
	// A flat character-like field is written as CHAR whatever DDIC calls it.
	charLike := func(t byte) bool { return t == typeChar || t == typeNumc || t == typeDate || t == typeTime }
	return charLike(a) && charLike(b) && (a == typeChar || b == typeChar)
}

// ddicToCluster maps a DDIC data type and length to the cluster's type code
// and byte length.
func ddicToCluster(c Component, unicode bool) (code byte, bytes int, ok bool) {
	charBytes := 1
	if unicode {
		charBytes = 2
	}
	switch strings.ToUpper(c.Type) {
	case "CHAR", "CUKY", "UNIT", "LANG", "CLNT", "ACCP", "LCHR":
		return typeChar, c.Chars * charBytes, true
	case "NUMC", "PREC":
		return typeNumc, c.Chars * charBytes, true
	case "DATS":
		return typeDate, 8 * charBytes, true
	case "TIMS":
		return typeTime, 6 * charBytes, true
	case "DEC", "QUAN", "CURR":
		return typePacked, c.Chars/2 + 1, true
	case "INT1":
		return typeInt1, 1, true
	case "INT2":
		return typeInt2, 2, true
	case "INT4":
		return typeInt, 4, true
	case "INT8":
		return typeInt8, 8, true
	case "FLTP":
		return typeFloat, 8, true
	case "RAW", "LRAW", "GEOM_EWKB":
		return typeRaw, c.Chars, true
	case "STRG", "SSTR":
		return typeString, 8, true
	case "RSTR":
		return typeXString, 8, true
	case "D16D", "D16R", "D16S", "DF16_DEC", "DF16_RAW":
		return typeDecfloat16, 8, true
	case "D34D", "D34R", "D34S", "DF34_DEC", "DF34_RAW", "UTCL":
		return typeDecfloat34, 16, true
	}
	return 0, 0, false
}

// Records renders the rows as maps once the fields are named; unnamed fields
// keep their path as the key. A table component's rows become maps too when
// its line is named, and stay slices otherwise.
func (o *Object) Records() []map[string]any {
	return records(o.Fields, o.Rows)
}

func records(fields []Field, rows [][]any) []map[string]any {
	out := make([]map[string]any, len(rows))
	for r, row := range rows {
		m := make(map[string]any, len(row))
		for i, v := range row {
			f := fields[i]
			key := f.Path
			if f.Name != "" {
				// Inside a table's rows the name is local to the line: the
				// enclosing path is the map they sit in.
				key = f.Name
				if at := strings.LastIndex(key, "[]."); at >= 0 {
					key = key[at+3:]
				}
			}
			if nested, ok := v.([][]any); ok && len(f.Fields) > 0 && f.Fields[0].Name != "" {
				m[key] = records(f.Fields, nested)
				continue
			}
			m[key] = v
		}
		out[r] = m
	}
	return out
}

// TLINELayout is SAPscript's text line: the format column and the line.
// STXL stores every text as a table of these under the object name TLINE.
var TLINELayout = &Layout{Name: "TLINE", Components: []Component{
	{Name: "TDFORMAT", Type: "CHAR", Chars: 2},
	{Name: "TDLINE", Type: "CHAR", Chars: 132},
}}

// TextLine is one SAPscript line with its format.
type TextLine struct {
	Format string `json:"format"`
	Line   string `json:"line"`
}

// SAPscriptText reads an STXL cluster's TLINE table into lines and joins
// them into one text the way the editor shows it: a paragraph format starts
// a new line, "=" continues the previous one, "/" is a line break within a
// paragraph, "/:" and "/*" are command and comment lines.
func (c *Cluster) SAPscriptText() ([]TextLine, string, error) {
	obj := c.Object("TLINE")
	if obj == nil {
		return nil, "", fmt.Errorf("no TLINE object in the cluster (objects: %s)", c.objectNames())
	}
	if err := obj.Apply(TLINELayout); err != nil {
		return nil, "", err
	}
	var lines []TextLine
	var text strings.Builder
	for _, row := range obj.Rows {
		l := TextLine{Format: str(row[0]), Line: str(row[1])}
		lines = append(lines, l)
		switch strings.TrimSpace(l.Format) {
		case "=":
			text.WriteString(l.Line)
		default:
			if text.Len() > 0 {
				text.WriteByte('\n')
			}
			text.WriteString(l.Line)
		}
	}
	return lines, text.String(), nil
}

func (c *Cluster) objectNames() string {
	names := make([]string, len(c.Objects))
	for i, o := range c.Objects {
		names[i] = o.Name
	}
	return strings.Join(names, ", ")
}

func str(v any) string {
	if s, ok := v.(string); ok {
		return s
	}
	return fmt.Sprint(v)
}
