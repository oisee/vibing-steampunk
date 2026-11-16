package sncrfc

import (
	"errors"
	"regexp"
)

// Field describes the bounded subset supported by the no-cgo marshaler.
// Metadata and values are private memory; callers must not log them.
type Field struct {
	Name       string
	Type       uint32
	Direction  uint32
	Length     uint32
	Fields     []Field
	nativeType uintptr //nolint:unused // used by call_windows.go; the SDK exists only on Windows
}
type Description struct {
	Name         string
	Parameters   []Field
	nativeHandle uintptr //nolint:unused // used by call_windows.go; the SDK exists only on Windows
}
type functionAPI interface {
	describe(uintptr, string) (Description, error)
	invoke(uintptr, Description, map[string]any) (map[string]any, error)
}

// FunctionError reports only a fixed marshaling phase, never metadata or data.
type FunctionError struct {
	Phase      string
	Diagnostic string
	Type       *uint32
	Native     *NativeError
}

func (e *FunctionError) Error() string { return "RFC function phase failed" }
func (e *FunctionError) Unwrap() error {
	if e.Native == nil {
		return nil
	}
	return e.Native
}
func functionFailure(phase string, e error) error {
	var n *NativeError
	errors.As(e, &n)
	if n != nil {
		n = &NativeError{n.Code, n.Group}
	}
	result := &FunctionError{Phase: phase, Native: n}
	var own *FunctionError
	if errors.As(e, &own) {
		result.Diagnostic = own.Diagnostic
		result.Type = own.Type
	}
	return result
}
func marshalError(code string) error           { return &FunctionError{Diagnostic: code} }              //nolint:unused // used by call_windows.go; the SDK exists only on Windows
func typeError(code string, kind uint32) error { return &FunctionError{Diagnostic: code, Type: &kind} } //nolint:unused // used by call_windows.go; the SDK exists only on Windows
func SafeFunctionDiagnostic(code string) bool {
	return map[string]bool{"missing_api": true, "metadata_parameter_limit": true, "metadata_depth_limit": true, "metadata_field_limit": true, "value_depth_limit": true, "invalid_string": true, "invalid_bytes": true, "invalid_structure": true, "unknown_structure_field": true, "invalid_table": true, "unknown_table_field": true, "unsupported_input_type": true, "response_byte_limit": true, "invalid_native_byte_length": true, "invalid_native_string_length": true, "response_row_limit": true, "unsupported_output_type": true, "invalid_input_parameter": true}[code]
}

var functionName = regexp.MustCompile(`^[A-Z][A-Z0-9_]{0,29}$`)
var readFunctions = map[string]bool{"SADT_REST_RFC_ENDPOINT": true, "RFC_GET_FUNCTION_INTERFACE": true, "DDIF_FIELDINFO_GET": true, "RFC_GET_STRUCTURE_DEFINITION": true}

// Describe reads function metadata on the already authenticated connection.
func (c *Connection) Describe(name string) (Description, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.callError != nil {
		return Description{}, c.callError
	}
	if c.handle == 0 || !functionName.MatchString(name) || !readFunctions[name] {
		return Description{}, errors.New("closed connection or function outside read allowlist")
	}
	a, ok := c.api.(functionAPI)
	if !ok {
		return Description{}, errors.New("function API unavailable")
	}
	d, e := a.describe(c.handle, name)
	if e != nil {
		c.callError = functionFailure("metadata", e)
		return Description{}, c.callError
	}
	return d, nil
}

// Call supports only the explicit bootstrap reads and ADT reads, never arbitrary
// RFC execution. SADT is restricted to GET on the read allowlist in adtread.go.
func (c *Connection) Call(name string, input map[string]any) (map[string]any, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.callError != nil {
		return nil, c.callError
	}
	if c.handle == 0 || !readFunctions[name] {
		return nil, errors.New("closed connection or function outside read allowlist")
	}
	if name == "SADT_REST_RFC_ENDPOINT" {
		if e := validateADTRead(input, c.dataPreview); e != nil {
			return nil, e
		}
	}
	a, ok := c.api.(functionAPI)
	if !ok {
		return nil, errors.New("function API unavailable")
	}
	d, e := a.describe(c.handle, name)
	if e != nil {
		c.callError = functionFailure("metadata", e)
		return nil, c.callError
	}
	v, e := a.invoke(c.handle, d, input)
	if e != nil {
		var phase *FunctionError
		if errors.As(e, &phase) {
			c.callError = functionFailure(phase.Phase, e)
		} else {
			c.callError = functionFailure("function", e)
		}
		return nil, c.callError
	}
	return v, nil
}
