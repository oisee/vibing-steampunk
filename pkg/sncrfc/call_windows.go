package sncrfc

import (
	"errors"
	"runtime"
	"unicode/utf16"
	"unsafe"
)

// Independent Windows x64 declarations verified against SAP SDK 7.50 Doxygen:
// https://support.sap.com/content/dam/support/en_us/library/ssp/products/connectors/nwrfcsdk/sap_nwrfcsdk_750_19_documentation.zip
// RFC_PARAMETER_DESC orders type before direction. Cython declarations access
// members by name and must not be used to infer member order or enum values.
// Public SAP PyRFC corroborates the called function prototypes:
// https://raw.githubusercontent.com/SAP/PyRFC/main/src/pyrfc/csapnwrfc.pxd
// Enums are uint32; SAP_UC uint16; type/description handles pointer-sized.
// SAP RFCTYPE values are explicitly assigned, not sequential.
// https://help.sap.com/saphelp_em92/helpdata/de/48/a862055135307ce10000000a42189b/content.htm
const (
	typeStructure uint32 = 17
	typeString    uint32 = 29
	typeXString   uint32 = 30
)

type parameterDesc struct {
	name                                           [31]uint16
	kind, direction, nucLength, ucLength, decimals uint32
	typeHandle                                     uintptr
	defaultValue                                   [31]uint16
	text                                           [80]uint16
	optional                                       byte
	extended                                       uintptr
}
type fieldDesc struct {
	name                                                     [31]uint16
	kind, nucLength, nucOffset, ucLength, ucOffset, decimals uint32
	typeHandle, extended                                     uintptr
}
type callFrame struct {
	a    *sdkAPI
	pins runtime.Pinner
	info errorInfo
}

func (a *sdkAPI) frame() *callFrame { f := &callFrame{a: a}; f.pins.Pin(&f.info); return f }
func (f *callFrame) ptr(p any) uintptr {
	f.pins.Pin(p)
	switch v := p.(type) {
	case *uint32:
		return uintptr(unsafe.Pointer(v))
	case *uintptr:
		return uintptr(unsafe.Pointer(v))
	case *parameterDesc:
		return uintptr(unsafe.Pointer(v))
	case *fieldDesc:
		return uintptr(unsafe.Pointer(v))
	case *uint16:
		return uintptr(unsafe.Pointer(v))
	case *byte:
		return uintptr(unsafe.Pointer(v))
	}
	panic("unsupported pinned pointer")
}
func (f *callFrame) text(s string) uintptr {
	v := append(utf16.Encode([]rune(s)), 0)
	return f.ptr(&v[0])
}
func (f *callFrame) raw(name string, args ...uintptr) (uintptr, error) {
	p, e := f.a.lib.find(name)
	if e != nil {
		return 0, marshalError("missing_api")
	}
	f.info = errorInfo{}
	args = append(args, uintptr(unsafe.Pointer(&f.info)))
	v, _, _ := p.Call(args...)
	runtime.KeepAlive(f)
	return v, nil
}
func (f *callFrame) rc(name string, args ...uintptr) error {
	v, e := f.raw(name, args...)
	if e != nil {
		return e
	}
	if v != 0 {
		return &NativeError{uint32(v), f.info.group}
	}
	return nil
}
func (f *callFrame) handle(name string, args ...uintptr) (uintptr, error) {
	v, e := f.raw(name, args...)
	if e != nil {
		return 0, e
	}
	if v == 0 {
		return 0, &NativeError{f.info.code, f.info.group}
	}
	return v, nil
}

func (a *sdkAPI) describe(h uintptr, name string) (Description, error) {
	f := a.frame()
	defer f.pins.Unpin()
	d, e := f.handle("RfcGetFunctionDesc", h, f.text(name))
	if e != nil {
		return Description{}, e
	}
	var n uint32
	if e = f.rc("RfcGetParameterCount", d, f.ptr(&n)); e != nil {
		return Description{}, e
	}
	if n > 128 {
		return Description{}, marshalError("metadata_parameter_limit")
	}
	out := Description{Name: name, nativeHandle: d}
	budget := 512
	for i := uint32(0); i < n; i++ {
		p := new(parameterDesc)
		if e = f.rc("RfcGetParameterDescByIndex", d, uintptr(i), f.ptr(p)); e != nil {
			return Description{}, e
		}
		field := Field{Name: decodeUC(p.name[:]), Type: p.kind, Direction: p.direction, Length: p.ucLength, nativeType: p.typeHandle}
		if field.Fields, e = f.fields(p.typeHandle, 0, &budget); e != nil {
			return Description{}, e
		}
		out.Parameters = append(out.Parameters, field)
	}
	return out, nil
}
func (f *callFrame) fields(h uintptr, depth int, budget *int) ([]Field, error) {
	if h == 0 {
		return nil, nil
	}
	if depth > 8 {
		return nil, marshalError("metadata_depth_limit")
	}
	var n uint32
	if e := f.rc("RfcGetFieldCount", h, f.ptr(&n)); e != nil {
		return nil, e
	}
	if n > uint32(*budget) {
		return nil, marshalError("metadata_field_limit")
	}
	*budget -= int(n)
	out := make([]Field, 0, n)
	for i := uint32(0); i < n; i++ {
		p := new(fieldDesc)
		if e := f.rc("RfcGetFieldDescByIndex", h, uintptr(i), f.ptr(p)); e != nil {
			return nil, e
		}
		v := Field{Name: decodeUC(p.name[:]), Type: p.kind, Length: p.ucLength, nativeType: p.typeHandle}
		var e error
		v.Fields, e = f.fields(p.typeHandle, depth+1, budget)
		if e != nil {
			return nil, e
		}
		out = append(out, v)
	}
	return out, nil
}
func (a *sdkAPI) invoke(h uintptr, d Description, input map[string]any) (out map[string]any, err error) {
	f := a.frame()
	defer f.pins.Unpin()
	fun, e := f.handle("RfcCreateFunction", d.nativeHandle)
	if e != nil {
		return nil, functionFailure("create", e)
	}
	defer func() {
		if e := f.rc("RfcDestroyFunction", fun); e != nil {
			out = nil
			err = errors.Join(err, functionFailure("destroy", e))
		}
	}()
	known := map[string]Field{}
	for _, p := range d.Parameters {
		known[p.Name] = p
	}
	for name, value := range input {
		p, ok := known[name]
		if !ok || p.Direction&1 == 0 {
			return nil, functionFailure("input", marshalError("invalid_input_parameter"))
		}
		if e = f.set(fun, p, value, 0); e != nil {
			return nil, functionFailure("input", e)
		}
	}
	if e = f.rc("RfcInvoke", h, fun); e != nil {
		return nil, functionFailure("invoke", e)
	}
	out = map[string]any{}
	budget := 4 << 20
	rows := 512
	for _, p := range d.Parameters {
		if p.Direction&2 != 0 {
			v, e := f.get(fun, p, 0, &budget, &rows)
			if e != nil {
				return nil, functionFailure("output", e)
			}
			out[p.Name] = v
		}
	}
	return out, nil
}
func (f *callFrame) child(h uintptr, p Field) (uintptr, error) {
	var v uintptr
	name := "RfcGetStructure"
	if p.Type == 5 {
		name = "RfcGetTable"
	}
	e := f.rc(name, h, f.text(p.Name), f.ptr(&v))
	return v, e
}
func (f *callFrame) set(h uintptr, p Field, value any, depth int) error {
	if depth > 8 {
		return marshalError("value_depth_limit")
	}
	switch p.Type {
	case 0, 6, typeString:
		s, ok := value.(string)
		if !ok || len(s) > 65536 {
			return marshalError("invalid_string")
		}
		v := append(utf16.Encode([]rune(s)), 0)
		name := "RfcSetString"
		if p.Type == 0 {
			name = "RfcSetChars"
		}
		if p.Type == 6 {
			name = "RfcSetNum"
		}
		return f.rc(name, h, f.text(p.Name), f.ptr(&v[0]), uintptr(len(v)-1))
	case typeXString:
		b, ok := value.([]byte)
		if !ok || len(b) > 65536 {
			return marshalError("invalid_bytes")
		}
		n := len(b)
		if n == 0 {
			b = make([]byte, 1)
		}
		return f.rc("RfcSetXString", h, f.text(p.Name), f.ptr(&b[0]), uintptr(n))
	case typeStructure:
		values, ok := value.(map[string]any)
		if !ok {
			return marshalError("invalid_structure")
		}
		c, e := f.child(h, p)
		if e != nil {
			return e
		}
		fields := map[string]Field{}
		for _, v := range p.Fields {
			fields[v.Name] = v
		}
		for n, v := range values {
			field, ok := fields[n]
			if !ok {
				return marshalError("unknown_structure_field")
			}
			if e = f.set(c, field, v, depth+1); e != nil {
				return e
			}
		}
		return nil
	case 5:
		rows, ok := value.([]map[string]any)
		if !ok || len(rows) > 32 {
			return marshalError("invalid_table")
		}
		c, e := f.child(h, p)
		if e != nil {
			return e
		}
		for _, r := range rows {
			row, e := f.handle("RfcAppendNewRow", c)
			if e != nil {
				return e
			}
			for _, field := range p.Fields {
				v, ok := r[field.Name]
				if ok {
					if e = f.set(row, field, v, depth+1); e != nil {
						return e
					}
				}
			}
			for n := range r {
				found := false
				for _, field := range p.Fields {
					if field.Name == n {
						found = true
					}
				}
				if !found {
					return marshalError("unknown_table_field")
				}
			}
		}
		return nil
	default:
		return typeError("unsupported_input_type", p.Type)
	}
}
func (f *callFrame) get(h uintptr, p Field, depth int, budget, rows *int) (any, error) {
	if depth > 8 {
		return nil, marshalError("value_depth_limit")
	}
	switch p.Type {
	case 0, 6, typeString, typeXString:
		var n uint32
		if p.Type == 0 || p.Type == 6 {
			n = p.Length / 2
		} else {
			if e := f.rc("RfcGetStringLength", h, f.text(p.Name), f.ptr(&n)); e != nil {
				return nil, e
			}
		}
		bytes := uint64(n)*2 + 2
		if p.Type == typeXString {
			bytes = uint64(n) + 1
		}
		if bytes > uint64(*budget) {
			return nil, marshalError("response_byte_limit")
		}
		*budget -= int(bytes)
		if p.Type == typeXString {
			b := make([]byte, int(n)+1)
			var got uint32
			if e := f.rc("RfcGetXString", h, f.text(p.Name), f.ptr(&b[0]), uintptr(len(b)), f.ptr(&got)); e != nil {
				return nil, e
			}
			if got > n {
				return nil, marshalError("invalid_native_byte_length")
			}
			return b[:got], nil
		}
		b := make([]uint16, int(n)+1)
		var got uint32
		name := "RfcGetString"
		args := []uintptr{h, f.text(p.Name), f.ptr(&b[0]), uintptr(len(b)), f.ptr(&got)}
		if p.Type == 0 || p.Type == 6 {
			name = "RfcGetChars"
			if p.Type == 6 {
				name = "RfcGetNum"
			}
			args = args[:4]
			args[3] = uintptr(n)
			got = n
		}
		if e := f.rc(name, args...); e != nil {
			return nil, e
		}
		if got > n {
			return nil, marshalError("invalid_native_string_length")
		}
		return string(utf16.Decode(b[:got])), nil
	case typeStructure:
		c, e := f.child(h, p)
		if e != nil {
			return nil, e
		}
		return f.getFields(c, p.Fields, depth, budget, rows)
	case 5:
		c, e := f.child(h, p)
		if e != nil {
			return nil, e
		}
		var n uint32
		if e = f.rc("RfcGetRowCount", c, f.ptr(&n)); e != nil {
			return nil, e
		}
		if n > uint32(*rows) {
			return nil, marshalError("response_row_limit")
		}
		*rows -= int(n)
		out := make([]map[string]any, 0, n)
		for i := uint32(0); i < n; i++ {
			if e = f.rc("RfcMoveTo", c, uintptr(i)); e != nil {
				return nil, e
			}
			r, e := f.getFields(c, p.Fields, depth, budget, rows)
			if e != nil {
				return nil, e
			}
			out = append(out, r)
		}
		return out, nil
	default:
		return nil, typeError("unsupported_output_type", p.Type)
	}
}
func (f *callFrame) getFields(h uintptr, fields []Field, depth int, budget, rows *int) (map[string]any, error) {
	out := map[string]any{}
	for _, p := range fields {
		v, e := f.get(h, p, depth+1, budget, rows)
		if e != nil {
			return nil, e
		}
		out[p.Name] = v
	}
	return out, nil
}
