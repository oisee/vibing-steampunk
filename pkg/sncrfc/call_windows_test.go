package sncrfc

import (
	"errors"
	"testing"
	"unicode/utf16"
	"unsafe"
)

func TestNestedUnicodeAndXStringMarshaling(t *testing.T) {
	var chars []uint16
	var binary []byte
	l := &callLibrary{functions: map[string]procedureFunc{
		"RfcGetStructure": func(v ...uintptr) (uintptr, uintptr, error) {
			*(*uintptr)(ticketTestPointer(v[2])) = 77
			return 0, 0, nil
		},
		"RfcSetString": func(v ...uintptr) (uintptr, uintptr, error) {
			if v[0] != 77 {
				t.Fatal("structure handle not used")
			}
			chars = append([]uint16(nil), unsafe.Slice((*uint16)(ticketTestPointer(v[2])), int(v[3]))...)
			return 0, 0, nil
		},
		"RfcSetXString": func(v ...uintptr) (uintptr, uintptr, error) {
			binary = append([]byte(nil), unsafe.Slice((*byte)(ticketTestPointer(v[2])), int(v[3]))...)
			return 0, 0, nil
		},
	}}
	a := &sdkAPI{lib: l}
	f := a.frame()
	defer f.pins.Unpin()
	p := Field{Name: "REQUEST", Type: typeStructure, Fields: []Field{{Name: "TEXT", Type: typeString}, {Name: "BODY", Type: typeXString}}}
	if e := f.set(42, p, map[string]any{"TEXT": "A😀Б", "BODY": []byte{0, 255, 1}}, 0); e != nil {
		t.Fatal(e)
	}
	if string(utf16.Decode(chars)) != "A😀Б" || len(chars) != 4 || len(binary) != 3 || binary[1] != 255 {
		t.Fatal("unicode or binary altered")
	}
}

type procedureFunc func(...uintptr) (uintptr, uintptr, error)

func (f procedureFunc) Call(v ...uintptr) (uintptr, uintptr, error) { return f(v...) }

type callLibrary struct {
	functions map[string]procedureFunc
	calls     []string
}

func (l *callLibrary) find(n string) (procedure, error) {
	f, ok := l.functions[n]
	if !ok {
		return nil, errors.New("missing")
	}
	return procedureFunc(func(v ...uintptr) (uintptr, uintptr, error) { l.calls = append(l.calls, n); return f(v...) }), nil
}
func (l *callLibrary) release() error { return nil }
func TestFunctionABIStructures(t *testing.T) {
	var p parameterDesc
	var f fieldDesc
	if unsafe.Offsetof(p.kind) != 64 || unsafe.Offsetof(p.direction) != 68 {
		t.Fatal("parameter type/direction ABI order mismatch")
	}
	if typeStructure != 17 || typeString != 29 || typeXString != 30 || unsafe.Sizeof(p) != 328 || unsafe.Offsetof(p.typeHandle) != 88 || unsafe.Offsetof(p.extended) != 320 || unsafe.Sizeof(f) != 104 || unsafe.Offsetof(f.typeHandle) != 88 {
		t.Fatal("function ABI mismatch")
	}
}

func TestDescribeReadsSDKTypeDirectionTuple(t *testing.T) {
	l := &callLibrary{functions: map[string]procedureFunc{
		"RfcGetFunctionDesc": func(...uintptr) (uintptr, uintptr, error) { return 3, 0, nil },
		"RfcGetParameterCount": func(v ...uintptr) (uintptr, uintptr, error) {
			*(*uint32)(ticketTestPointer(v[1])) = 1
			return 0, 0, nil
		},
		"RfcGetParameterDescByIndex": func(v ...uintptr) (uintptr, uintptr, error) {
			p := ticketTestPointer(v[2])
			copy(unsafe.Slice((*uint16)(p), 31), utf16.Encode([]rune("REQUEST")))
			*(*uint32)(unsafe.Add(p, 64)) = 17
			*(*uint32)(unsafe.Add(p, 68)) = 1
			return 0, 0, nil
		},
	}}
	a := &sdkAPI{lib: l}
	d, e := a.describe(42, "SADT_REST_RFC_ENDPOINT")
	if e != nil || len(d.Parameters) != 1 || d.Parameters[0].Name != "REQUEST" || d.Parameters[0].Type != 17 || d.Parameters[0].Direction != 1 {
		t.Fatal("SDK parameter tuple decoded incorrectly")
	}
}
func TestOutputBoundsBeforeAllocation(t *testing.T) {
	l := &callLibrary{functions: map[string]procedureFunc{"RfcGetStringLength": func(v ...uintptr) (uintptr, uintptr, error) {
		*(*uint32)(ticketTestPointer(v[2])) = 0xffffffff
		return 0, 0, nil
	}}}
	a := &sdkAPI{lib: l}
	f := a.frame()
	defer f.pins.Unpin()
	budget := 4 << 20
	rows := 512
	if _, e := f.get(42, Field{Name: "BODY", Type: typeXString}, 0, &budget, &rows); e == nil {
		t.Fatal("oversize accepted")
	}
	if len(l.calls) != 1 {
		t.Fatal("native data fetch despite bound")
	}
}
func TestInvokeFailureDestroysFunctionOnce(t *testing.T) {
	destroyed := 0
	l := &callLibrary{functions: map[string]procedureFunc{"RfcCreateFunction": func(...uintptr) (uintptr, uintptr, error) { return 17, 0, nil }, "RfcInvoke": func(v ...uintptr) (uintptr, uintptr, error) {
		info := (*errorInfo)(ticketTestPointer(v[2]))
		info.group = 7
		return 29, 0, nil
	}, "RfcDestroyFunction": func(...uintptr) (uintptr, uintptr, error) { destroyed++; return 0, 0, nil }}}
	a := &sdkAPI{lib: l}
	_, e := a.invoke(42, Description{nativeHandle: 3}, nil)
	var n *NativeError
	if !errors.As(e, &n) || n.Code != 29 || destroyed != 1 {
		t.Fatal("invoke failure/cleanup mismatch")
	}
}

func TestInvalidInputFailsBeforeInvokeAndReportsPhase(t *testing.T) {
	invokes := 0
	destroyed := 0
	l := &callLibrary{functions: map[string]procedureFunc{
		"RfcCreateFunction":  func(...uintptr) (uintptr, uintptr, error) { return 17, 0, nil },
		"RfcInvoke":          func(...uintptr) (uintptr, uintptr, error) { invokes++; return 0, 0, nil },
		"RfcDestroyFunction": func(...uintptr) (uintptr, uintptr, error) { destroyed++; return 0, 0, nil },
	}}
	a := &sdkAPI{lib: l}
	_, e := a.invoke(42, Description{nativeHandle: 3, Parameters: []Field{{Name: "REQUEST", Direction: 1, Type: typeString}}}, map[string]any{"REQUEST": map[string]any{}})
	var phase *FunctionError
	if !errors.As(e, &phase) || phase.Phase != "input" || invokes != 0 || destroyed != 1 {
		t.Fatal("invalid input reached backend or cleanup/phase lost")
	}
}

// ticketTestPointer turns a uintptr argument of a fake native call back into
// the pointer the code under test passed (kept under its sidecar name).
func ticketTestPointer(v uintptr) unsafe.Pointer { return *(*unsafe.Pointer)(unsafe.Pointer(&v)) }
