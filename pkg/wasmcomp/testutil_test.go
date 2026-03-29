package wasmcomp

import (
	"encoding/binary"
	"math"
)

// wasmBuilder helps construct valid WASM binaries for testing.
type wasmBuilder struct {
	buf []byte
}

func newWasmBuilder() *wasmBuilder {
	w := &wasmBuilder{}
	// Magic + version
	w.buf = append(w.buf, 0x00, 0x61, 0x73, 0x6D) // \0asm
	w.buf = append(w.buf, 0x01, 0x00, 0x00, 0x00) // version 1
	return w
}

func (w *wasmBuilder) addSection(id byte, content []byte) {
	w.buf = append(w.buf, id)
	w.buf = append(w.buf, leb128u(uint32(len(content)))...)
	w.buf = append(w.buf, content...)
}

func (w *wasmBuilder) bytes() []byte {
	return w.buf
}

// buildTypeSection creates a type section with the given function types.
func buildTypeSection(types []FuncType) []byte {
	var buf []byte
	buf = append(buf, leb128u(uint32(len(types)))...)
	for _, ft := range types {
		buf = append(buf, 0x60) // functype
		buf = append(buf, leb128u(uint32(len(ft.Params)))...)
		for _, p := range ft.Params {
			buf = append(buf, byte(p))
		}
		buf = append(buf, leb128u(uint32(len(ft.Results)))...)
		for _, r := range ft.Results {
			buf = append(buf, byte(r))
		}
	}
	return buf
}

// buildFuncSection creates a function section mapping functions to type indices.
func buildFuncSection(typeIndices []int) []byte {
	var buf []byte
	buf = append(buf, leb128u(uint32(len(typeIndices)))...)
	for _, idx := range typeIndices {
		buf = append(buf, leb128u(uint32(idx))...)
	}
	return buf
}

// buildExportSection creates an export section.
func buildExportSection(exports []Export) []byte {
	var buf []byte
	buf = append(buf, leb128u(uint32(len(exports)))...)
	for _, exp := range exports {
		buf = append(buf, leb128u(uint32(len(exp.Name)))...)
		buf = append(buf, []byte(exp.Name)...)
		buf = append(buf, exp.Kind)
		buf = append(buf, leb128u(uint32(exp.Index))...)
	}
	return buf
}

// buildCodeSection creates a code section with function bodies.
func buildCodeSection(bodies [][]byte) []byte {
	var buf []byte
	buf = append(buf, leb128u(uint32(len(bodies)))...)
	for _, body := range bodies {
		buf = append(buf, leb128u(uint32(len(body)))...)
		buf = append(buf, body...)
	}
	return buf
}

// buildFuncBody creates a function body with local declarations and instructions.
func buildFuncBody(locals []ValType, code []byte) []byte {
	var buf []byte
	if len(locals) == 0 {
		buf = append(buf, 0x00) // 0 local declarations
	} else {
		// Group consecutive same-type locals
		var groups [][2]int // [count, type]
		for _, l := range locals {
			if len(groups) > 0 && groups[len(groups)-1][1] == int(l) {
				groups[len(groups)-1][0]++
			} else {
				groups = append(groups, [2]int{1, int(l)})
			}
		}
		buf = append(buf, leb128u(uint32(len(groups)))...)
		for _, g := range groups {
			buf = append(buf, leb128u(uint32(g[0]))...)
			buf = append(buf, byte(g[1]))
		}
	}
	buf = append(buf, code...)
	buf = append(buf, OpEnd) // end of function
	return buf
}

// leb128u encodes a uint32 as unsigned LEB128.
func leb128u(v uint32) []byte {
	var buf []byte
	for {
		b := byte(v & 0x7F)
		v >>= 7
		if v != 0 {
			b |= 0x80
		}
		buf = append(buf, b)
		if v == 0 {
			break
		}
	}
	return buf
}

// leb128s encodes an int32 as signed LEB128.
func leb128s(v int32) []byte {
	var buf []byte
	more := true
	for more {
		b := byte(v & 0x7F)
		v >>= 7
		if (v == 0 && b&0x40 == 0) || (v == -1 && b&0x40 != 0) {
			more = false
		} else {
			b |= 0x80
		}
		buf = append(buf, b)
	}
	return buf
}

// Suppress unused import warnings.
var _ = binary.LittleEndian
var _ = math.Float32frombits
