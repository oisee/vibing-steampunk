package datacluster

import (
	"encoding/binary"
	"fmt"
)

// Version 5 is what kernels before the Unicode ones wrote, and what stays on
// disk after a Unicode conversion until something rewrites the row: EUFUNC
// test data from a 4.6 system, AQLDB queries, INDX entries nobody touched.
// The shape is the same idea with smaller numbers: a two-byte row length,
// four-byte descriptor entries without a decimals byte, single-byte names,
// and rows that are each introduced by a BB marker with no count and no
// framing around fixed and string parts — there were no strings to frame.
//
// Object header, 15 bytes plus the name:
//
//	0     object kind, as in version 6
//	1     type code
//	2-3   row length, big-endian
//	4-5   object length in the body, big-endian
//	6     name length in characters
//	7-14  eight bytes that differ per object and are not needed to read it
//	15-   name, one byte per character

const (
	legacyHeaderSize     = 15
	legacyDescriptorSize = 4
	legacyRow            = 0xBB
)

func (p *parser) legacyObject() (*Object, error) {
	if err := p.need(legacyHeaderSize); err != nil {
		return nil, err
	}
	h := p.data[p.pos:]
	obj := &Object{Kind: Kind(h[0]), TypeCode: h[1]}
	switch h[0] {
	case 2:
		obj.Kind = Structure
	case 3:
		obj.Kind = Table
	case 7:
		obj.Kind = Elementary
	}
	obj.RowLength = int(binary.BigEndian.Uint16(h[2:]))
	obj.Size = int(binary.BigEndian.Uint16(h[4:]))
	nameLen := int(h[6])
	p.pos += legacyHeaderSize
	if err := p.need(nameLen); err != nil {
		return nil, err
	}
	obj.Name = p.dec.text(p.data[p.pos : p.pos+nameLen])
	p.pos += nameLen

	switch obj.Kind {
	case Elementary:
		obj.Type = &Node{Path: "1", TypeCode: obj.TypeCode, Length: obj.RowLength}
	case Structure, Table:
		root, err := p.legacyDescriptor(obj.Kind)
		if err != nil {
			return nil, err
		}
		obj.Type = root
		if root.Length != obj.RowLength {
			return nil, fmt.Errorf("descriptor length %d does not match row length %d", root.Length, obj.RowLength)
		}
	default:
		return nil, fmt.Errorf("unknown object kind %#02x", byte(obj.Kind))
	}

	var leaves []*Node
	collect(obj.Type, &leaves)
	if sum := sumLeaves(leaves); sum != obj.RowLength {
		return nil, fmt.Errorf("fields sum to %d bytes, row is %d", sum, obj.RowLength)
	}
	for _, n := range leaves {
		if isStringType(n.TypeCode) || n.Table {
			return nil, fmt.Errorf("field %s is a %s, which a version 5 cluster was not expected to hold", n.Path, TypeName(n.TypeCode))
		}
		if !n.Filler {
			obj.Fields = append(obj.Fields, fieldOf(n))
		}
	}

	// Rows: BB and the row's bytes, repeated; a table with no rows has none.
	for {
		if p.pos >= len(p.data) || p.data[p.pos] != legacyRow {
			break
		}
		p.pos++
		if err := p.need(obj.RowLength); err != nil {
			return nil, err
		}
		run := p.data[p.pos : p.pos+obj.RowLength]
		p.pos += obj.RowLength
		var row []any
		for _, leaf := range leaves {
			if !leaf.Filler {
				row = append(row, p.dec.value(leaf, run[:leaf.Length]))
			}
			run = run[leaf.Length:]
		}
		obj.Rows = append(obj.Rows, row)
		if obj.Kind != Table {
			break
		}
	}
	if obj.Kind != Table && len(obj.Rows) == 0 {
		return nil, fmt.Errorf("%s object has no data row", obj.Kind)
	}
	return obj, nil
}

// legacyDescriptor reads the four-byte entries: marker, type code, length.
func (p *parser) legacyDescriptor(kind Kind) (*Node, error) {
	if err := p.need(legacyDescriptorSize); err != nil {
		return nil, err
	}
	open, close := byte(markObjStructBegin), byte(markObjStructEnd)
	if kind == Table {
		open, close = markObjTableBegin, markObjTableEnd
	}
	if p.data[p.pos] != open {
		return nil, fmt.Errorf("expected descriptor marker %#02x at offset %d, found %#02x", open, p.pos, p.data[p.pos])
	}
	root := &Node{TypeCode: p.data[p.pos+1], Length: int(binary.BigEndian.Uint16(p.data[p.pos+2:]))}
	p.pos += legacyDescriptorSize
	if err := p.legacyChildren(root, close, ""); err != nil {
		return nil, err
	}
	return root, nil
}

func (p *parser) legacyChildren(parent *Node, close byte, prefix string) error {
	for {
		if err := p.need(legacyDescriptorSize); err != nil {
			return err
		}
		e := p.data[p.pos:]
		marker, code, length := e[0], e[1], int(binary.BigEndian.Uint16(e[2:]))
		p.pos += legacyDescriptorSize
		if marker == close {
			if length != parent.Length {
				return fmt.Errorf("descriptor closes with length %d, opened with %d", length, parent.Length)
			}
			return nil
		}
		path := fmt.Sprintf("%s%d", prefix, countValues(parent)+1)
		switch marker {
		case markLeaf:
			parent.Children = append(parent.Children, &Node{Path: path, TypeCode: code, Length: length})
		case markFiller:
			parent.Children = append(parent.Children, &Node{TypeCode: code, Length: length, Filler: true})
		case markStructBegin, markIncludeBegin:
			child := &Node{Path: path, TypeCode: code, Length: length, Include: marker == markIncludeBegin}
			end := byte(markStructEnd)
			if marker == markIncludeBegin {
				end = markIncludeEnd
			}
			if err := p.legacyChildren(child, end, path+"."); err != nil {
				return err
			}
			parent.Children = append(parent.Children, child)
		default:
			return fmt.Errorf("unknown descriptor marker %#02x at offset %d", marker, p.pos-legacyDescriptorSize)
		}
	}
}
