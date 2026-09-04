// Package datacluster reads ABAP data clusters — the byte stream that
// EXPORT ... TO DATABASE writes into INDX-like tables such as BALDAT, INDX and
// STXL, and that only IMPORT was ever meant to read back.
//
// A cluster row on the database is one fragment; the fragments of one key are
// joined in SRTF2 order and trimmed to CLUSTR, and the result is the stream
// this package parses: a sixteen-byte header, a body that is usually
// LZH-compressed (see pkg/sapcompress), and in it one object per name the
// EXPORT statement gave. Each object carries a type descriptor — kind, length
// and decimals for every field, nested for structures — followed by its data,
// so the shape comes back exactly. What does not come back is field names:
// the kernel writes types, not DDIC, so fields are numbered here and a caller
// that knows the structure lays names over them.
//
// The format is not documented by SAP. What is here was read off genuine
// clusters: an INDX record written by a fixture program on a 7.58 system with
// every elementary type in it, both compressed and not, and BALDAT records
// from a 7.5x system. Every assumption is checked against those in the tests.
package datacluster

import (
	"encoding/binary"
	"errors"
	"fmt"

	"github.com/oisee/vibing-steampunk/pkg/sapcompress"
)

// HeaderSize is the fixed prefix before the compressed or plain body.
const HeaderSize = 16

// Kind says what the EXPORT statement handed over under a name.
type Kind byte

const (
	// Elementary is a single field: a variable of an elementary type.
	Elementary Kind = 1
	// Structure is a flat or nested structure.
	Structure Kind = 5
	// Table is an internal table.
	Table Kind = 6
)

func (k Kind) String() string {
	switch k {
	case Elementary:
		return "elementary"
	case Structure:
		return "structure"
	case Table:
		return "table"
	}
	return fmt.Sprintf("kind %d", byte(k))
}

// Cluster is one parsed data cluster.
type Cluster struct {
	// Version is the format version byte after the FF marker; 6 on every
	// system seen so far.
	Version byte
	// Codepage is the SAP code page the character data is in: 4103 is
	// UTF-16 little-endian, 4102 big-endian, anything else a single-byte page.
	Codepage string
	// Compressed says whether the body was LZH/LZC-compressed on the database.
	Compressed bool
	// Algorithm names the compression when there was one.
	Algorithm string
	// Objects are the exported values, in the order the EXPORT named them.
	Objects []Object
}

// Object is one named export: an elementary field, a structure, or a table.
type Object struct {
	Name string
	Kind Kind
	// TypeCode is the ABAP type code of the object as a whole — the element
	// type for an elementary object, the structure type code (0x0E flat,
	// 0x0F deep) for structures and table line types.
	TypeCode byte
	// RowLength is the byte length of one row or of the structure, including
	// alignment fillers and the eight-byte references of string fields.
	RowLength int
	// Size is the object's total length in the plain body, which the kernel
	// writes for uncompressed clusters and leaves zero for compressed ones.
	Size int
	// Type is the full descriptor tree.
	Type *Node
	// Fields are the leaves of Type in order, alignment fillers left out;
	// every row has one value per field.
	Fields []Field
	// Rows are the decoded values: one row for elementary and structure
	// objects, one per line for tables.
	Rows [][]any
}

// Node is one entry of an object's type descriptor.
type Node struct {
	// Path names the node by position: "3" is the third field of the object,
	// "3.2" the second field inside it.
	Path     string
	TypeCode byte
	Length   int
	Decimals int
	// Filler marks alignment padding the kernel inserted between fields; it
	// carries no value.
	Filler bool
	// Include marks a structure that was written as an include rather than a
	// substructure; the distinction is the kernel's, not this package's.
	Include  bool
	Children []*Node
}

// Field is one leaf of the type descriptor: something that has a value.
type Field struct {
	Path     string `json:"path"`
	Type     string `json:"type"`
	TypeCode byte   `json:"typeCode"`
	Length   int    `json:"length"`
	Decimals int    `json:"decimals,omitempty"`
}

// Parse decodes a whole cluster, decompressing the body when it is compressed.
func Parse(blob []byte) (*Cluster, error) {
	if len(blob) < HeaderSize {
		return nil, fmt.Errorf("datacluster: %d bytes is shorter than the %d-byte header", len(blob), HeaderSize)
	}
	if blob[0] != 0xFF {
		return nil, fmt.Errorf("datacluster: no cluster marker (first byte %#02x, want 0xFF)", blob[0])
	}
	c := &Cluster{Version: blob[1], Codepage: string(blob[8:12])}
	body := blob[HeaderSize:]
	switch blob[4] {
	case 1:
	case 2:
		h, err := sapcompress.ParseHeader(body)
		if err != nil {
			return nil, fmt.Errorf("datacluster: header says compressed but %w", err)
		}
		c.Compressed, c.Algorithm = true, h.Algorithm.String()
		if body, err = sapcompress.Decompress(body); err != nil {
			return nil, fmt.Errorf("datacluster: %w", err)
		}
	default:
		return nil, fmt.Errorf("datacluster: unknown body format %#02x", blob[4])
	}
	dec, err := newDecoder(c.Codepage)
	if err != nil {
		return nil, err
	}
	p := &parser{data: body, dec: dec}
	for {
		if p.pos >= len(p.data) {
			return nil, errors.New("datacluster: stream ends without the end marker")
		}
		if p.data[p.pos] == markEnd {
			break
		}
		obj, err := p.object()
		if err != nil {
			return nil, fmt.Errorf("datacluster: object %d at offset %d: %w", len(c.Objects)+1, p.pos, err)
		}
		c.Objects = append(c.Objects, *obj)
	}
	return c, nil
}

// Object returns the export with that name, or nil.
func (c *Cluster) Object(name string) *Object {
	for i := range c.Objects {
		if c.Objects[i].Name == name {
			return &c.Objects[i]
		}
	}
	return nil
}

// Stream markers. The descriptor markers come in begin/end pairs; the data
// markers frame a table, a row, a run of fixed-length bytes, and one string
// value stored out of line.
const (
	markObjStructBegin  = 0xAB // structure object descriptor
	markObjStructEnd    = 0xAC
	markObjTableBegin   = 0xAD // table line type descriptor
	markObjTableEnd     = 0xAE
	markStructBegin     = 0xA0 // nested substructure
	markStructEnd       = 0xA1
	markIncludeBegin    = 0xAB // include inside a line type (same byte as an object structure)
	markIncludeEnd      = 0xAC
	markLeaf            = 0xAA
	markFiller          = 0xAF
	markRow             = 0xBC // fixed-length bytes of a row
	markRowEnd          = 0xBD
	markTable           = 0xBE // row length + row count
	markTableEnd        = 0xBF
	markString          = 0xCA // out-of-line string / xstring value
	markStringEnd       = 0xCB
	markEnd             = 0x04
	descriptorEntrySize = 7
)

type parser struct {
	data []byte
	pos  int
	dec  *decoder
}

func (p *parser) need(n int) error {
	if p.pos+n > len(p.data) {
		return fmt.Errorf("truncated: need %d bytes at offset %d, have %d", n, p.pos, len(p.data)-p.pos)
	}
	return nil
}

func (p *parser) u32() (int, error) {
	if err := p.need(4); err != nil {
		return 0, err
	}
	v := int(binary.BigEndian.Uint32(p.data[p.pos:]))
	p.pos += 4
	return v, nil
}

// object reads one export: its header, its descriptor, its data.
//
// Header layout, 32 bytes plus the name:
//
//	0     object kind: 01 elementary, 05 structure, 03 or 06 table
//	1     type code of the element or of the (line) structure
//	2     0
//	3-6   row length, big-endian
//	7-10  object length in the plain body, big-endian; 0 when compressed
//	11    name length in characters
//	12-31 0
//	32-   name, UTF-16LE
func (p *parser) object() (*Object, error) {
	if err := p.need(32); err != nil {
		return nil, err
	}
	h := p.data[p.pos:]
	obj := &Object{Kind: Kind(h[0]), TypeCode: h[1]}
	if obj.Kind == 3 {
		obj.Kind = Table
	}
	obj.RowLength = int(binary.BigEndian.Uint32(h[3:]))
	obj.Size = int(binary.BigEndian.Uint32(h[7:]))
	nameLen := int(h[11])
	p.pos += 32
	if err := p.need(nameLen * 2); err != nil {
		return nil, err
	}
	name, nameErr := decodeUTF16(p.data[p.pos:p.pos+nameLen*2], false)
	if nameErr != nil {
		return nil, fmt.Errorf("object name: %w", nameErr)
	}
	obj.Name = name
	p.pos += nameLen * 2

	switch obj.Kind {
	case Elementary:
		// An elementary packed object carries no decimals anywhere; the
		// caller who exported it knows. It comes back as a digit string.
		obj.Type = &Node{Path: "1", TypeCode: obj.TypeCode, Length: obj.RowLength}
	case Structure, Table:
		root, err := p.descriptor(obj.Kind)
		if err != nil {
			return nil, err
		}
		obj.Type = root
		if got := root.Length; got != obj.RowLength {
			return nil, fmt.Errorf("descriptor length %d does not match row length %d", got, obj.RowLength)
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
		if n.Filler {
			continue
		}
		obj.Fields = append(obj.Fields, Field{Path: n.Path, Type: TypeName(n.TypeCode), TypeCode: n.TypeCode, Length: n.Length, Decimals: n.Decimals})
	}

	if obj.Kind == Table {
		if err := p.need(9); err != nil {
			return nil, err
		}
		if p.data[p.pos] != markTable {
			return nil, fmt.Errorf("expected table data marker at offset %d, found %#02x", p.pos, p.data[p.pos])
		}
		p.pos++
		rowLen, _ := p.u32()
		count, _ := p.u32()
		if rowLen != obj.RowLength {
			return nil, fmt.Errorf("table data row length %d, descriptor %d", rowLen, obj.RowLength)
		}
		for i := 0; i < count; i++ {
			row, err := p.row(leaves)
			if err != nil {
				return nil, fmt.Errorf("row %d: %w", i+1, err)
			}
			obj.Rows = append(obj.Rows, row)
		}
		if err := p.need(1); err != nil {
			return nil, err
		}
		if p.data[p.pos] != markTableEnd {
			return nil, fmt.Errorf("expected end of table at offset %d, found %#02x", p.pos, p.data[p.pos])
		}
		p.pos++
		return obj, nil
	}
	row, err := p.row(leaves)
	if err != nil {
		return nil, err
	}
	obj.Rows = [][]any{row}
	return obj, nil
}

// descriptor reads the type tree of a structure or table object. Entries are
// seven bytes: marker, type code, decimals, big-endian length.
func (p *parser) descriptor(kind Kind) (*Node, error) {
	if err := p.need(descriptorEntrySize); err != nil {
		return nil, err
	}
	open, close := byte(markObjStructBegin), byte(markObjStructEnd)
	if kind == Table {
		open, close = markObjTableBegin, markObjTableEnd
	}
	if p.data[p.pos] != open {
		return nil, fmt.Errorf("expected descriptor marker %#02x at offset %d, found %#02x", open, p.pos, p.data[p.pos])
	}
	root := &Node{TypeCode: p.data[p.pos+1], Decimals: int(p.data[p.pos+2]), Length: int(binary.BigEndian.Uint32(p.data[p.pos+3:]))}
	p.pos += descriptorEntrySize
	if err := p.children(root, close, ""); err != nil {
		return nil, err
	}
	return root, nil
}

func (p *parser) children(parent *Node, close byte, prefix string) error {
	for {
		if err := p.need(descriptorEntrySize); err != nil {
			return err
		}
		e := p.data[p.pos:]
		marker, code, dec, length := e[0], e[1], int(e[2]), int(binary.BigEndian.Uint32(e[3:]))
		p.pos += descriptorEntrySize
		if marker == close {
			if length != parent.Length {
				return fmt.Errorf("descriptor closes with length %d, opened with %d", length, parent.Length)
			}
			return nil
		}
		path := fmt.Sprintf("%s%d", prefix, countValues(parent)+1)
		switch marker {
		case markLeaf:
			parent.Children = append(parent.Children, &Node{Path: path, TypeCode: code, Decimals: dec, Length: length})
		case markFiller:
			parent.Children = append(parent.Children, &Node{TypeCode: code, Length: length, Filler: true})
		case markStructBegin, markIncludeBegin:
			child := &Node{Path: path, TypeCode: code, Decimals: dec, Length: length, Include: marker == markIncludeBegin}
			end := byte(markStructEnd)
			if marker == markIncludeBegin {
				end = markIncludeEnd
			}
			if err := p.children(child, end, path+"."); err != nil {
				return err
			}
			parent.Children = append(parent.Children, child)
		default:
			return fmt.Errorf("unknown descriptor marker %#02x at offset %d", marker, p.pos-descriptorEntrySize)
		}
	}
}

func countValues(n *Node) int {
	c := 0
	for _, ch := range n.Children {
		if !ch.Filler {
			c++
		}
	}
	return c
}

func collect(n *Node, out *[]*Node) {
	if len(n.Children) == 0 {
		*out = append(*out, n)
		return
	}
	for _, ch := range n.Children {
		collect(ch, out)
	}
}

func sumLeaves(leaves []*Node) int {
	s := 0
	for _, n := range leaves {
		s += n.Length
	}
	return s
}

// row reads one row's data. The fixed-length fields come in runs framed by
// BC..BD, one run up to each string field; a string's value sits between
// those runs framed by CA..CB, and its eight-byte reference in the row is
// not written at all. The row is complete when every field has a value —
// there is no end-of-row marker, the last BC run simply closes.
func (p *parser) row(leaves []*Node) ([]any, error) {
	values := make([]any, 0, len(leaves))
	i := 0 // next leaf to fill
	for i < len(leaves) {
		if err := p.need(1); err != nil {
			return nil, err
		}
		marker := p.data[p.pos]
		p.pos++
		switch marker {
		case markRow:
			n, err := p.u32()
			if err != nil {
				return nil, err
			}
			if err := p.need(n + 1); err != nil {
				return nil, err
			}
			run := p.data[p.pos : p.pos+n]
			p.pos += n
			if p.data[p.pos] != markRowEnd {
				return nil, fmt.Errorf("fixed run not closed at offset %d (found %#02x)", p.pos, p.data[p.pos])
			}
			p.pos++
			for len(run) > 0 {
				if i >= len(leaves) {
					return nil, fmt.Errorf("%d bytes of row data left after the last field", len(run))
				}
				leaf := leaves[i]
				if isStringType(leaf.TypeCode) {
					return nil, fmt.Errorf("field %s is a string but the row supplies fixed bytes for it", leaf.Path)
				}
				if len(run) < leaf.Length {
					return nil, fmt.Errorf("field %s needs %d bytes, run has %d", leaf.Path, leaf.Length, len(run))
				}
				if !leaf.Filler {
					values = append(values, p.dec.value(leaf, run[:leaf.Length]))
				}
				run = run[leaf.Length:]
				i++
			}
		case markString:
			n, err := p.u32()
			if err != nil {
				return nil, err
			}
			if err := p.need(n + 1); err != nil {
				return nil, err
			}
			raw := p.data[p.pos : p.pos+n]
			p.pos += n
			if p.data[p.pos] != markStringEnd {
				return nil, fmt.Errorf("string value not closed at offset %d", p.pos)
			}
			p.pos++
			for i < len(leaves) && leaves[i].Filler {
				i++
			}
			if i >= len(leaves) || !isStringType(leaves[i].TypeCode) {
				return nil, fmt.Errorf("string value at offset %d has no string field to land in", p.pos)
			}
			values = append(values, p.dec.stringValue(leaves[i], raw))
			i++
		default:
			return nil, fmt.Errorf("unexpected marker %#02x in row data at offset %d", marker, p.pos-1)
		}
	}
	return values, nil
}
