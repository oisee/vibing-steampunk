package ctxcomp

import (
	"context"
	"fmt"
	"strings"
	"testing"
)

// mockProvider returns embedded ABAP files as source.
type mockProvider struct {
	sources map[string]string // "CLAS:NAME" → source
}

func (m *mockProvider) GetSource(_ context.Context, kind DependencyKind, name string) (string, error) {
	key := string(kind) + ":" + name
	if src, ok := m.sources[key]; ok {
		return src, nil
	}
	return "", fmt.Errorf("not found: %s", key)
}

func TestCompressor_EndToEnd(t *testing.T) {
	// Source that references ZCL_VSP_UTILS and ZIF_VSP_SERVICE
	source := `CLASS zcl_my_handler DEFINITION PUBLIC.
  PUBLIC SECTION.
    INTERFACES zif_vsp_service.
    DATA mo_utils TYPE REF TO zcl_vsp_utils.
ENDCLASS.

CLASS zcl_my_handler IMPLEMENTATION.
  METHOD zif_vsp_service~handle_message.
    zcl_vsp_utils=>escape_json( 'test' ).
  ENDMETHOD.
ENDCLASS.`

	utilsSrc := readEmbedded(t, "zcl_vsp_utils.clas.abap")
	intfSrc := readEmbedded(t, "zif_vsp_service.intf.abap")

	provider := &mockProvider{
		sources: map[string]string{
			"CLAS:ZCL_VSP_UTILS":  utilsSrc,
			"INTF:ZIF_VSP_SERVICE": intfSrc,
		},
	}

	comp := NewCompressor(provider, 20)
	result, err := comp.Compress(context.Background(), source, "ZCL_MY_HANDLER", "CLAS")
	if err != nil {
		t.Fatalf("Compress error: %v", err)
	}

	// Should have found deps
	if len(result.Dependencies) == 0 {
		t.Error("no dependencies found")
	}

	// Should have resolved contracts
	if result.Stats.DepsResolved == 0 {
		t.Error("no contracts resolved")
	}

	// Prologue should contain header
	if !strings.Contains(result.Prologue, "Dependency context for ZCL_MY_HANDLER") {
		t.Error("missing prologue header")
	}

	// Should contain ZCL_VSP_UTILS contract
	if !strings.Contains(result.Prologue, "ZCL_VSP_UTILS") {
		t.Error("missing ZCL_VSP_UTILS in prologue")
	}

	// Should contain ZIF_VSP_SERVICE contract
	if !strings.Contains(result.Prologue, "ZIF_VSP_SERVICE") {
		t.Error("missing ZIF_VSP_SERVICE in prologue")
	}

	// Should NOT reference self
	for _, d := range result.Dependencies {
		if d.Name == "ZCL_MY_HANDLER" {
			t.Error("should not include self as dependency")
		}
	}

	// Prologue should NOT contain IMPLEMENTATION
	if strings.Contains(strings.ToUpper(result.Prologue), "IMPLEMENTATION") {
		t.Error("prologue should not contain IMPLEMENTATION sections")
	}
}

func TestCompressor_MaxDeps(t *testing.T) {
	// Source with many dependencies
	var lines []string
	for i := 0; i < 30; i++ {
		lines = append(lines, fmt.Sprintf("DATA lo_%d TYPE REF TO zcl_dep_%03d.", i, i))
	}
	source := strings.Join(lines, "\n")

	provider := &mockProvider{sources: make(map[string]string)}
	for i := 0; i < 30; i++ {
		name := fmt.Sprintf("ZCL_DEP_%03d", i)
		provider.sources["CLAS:"+name] = fmt.Sprintf("CLASS %s DEFINITION PUBLIC.\n  PUBLIC SECTION.\n    METHODS m1.\nENDCLASS.", strings.ToLower(name))
	}

	comp := NewCompressor(provider, 5)
	result, err := comp.Compress(context.Background(), source, "ZCL_TEST", "CLAS")
	if err != nil {
		t.Fatalf("Compress error: %v", err)
	}

	if result.Stats.DepsResolved > 5 {
		t.Errorf("expected at most 5 resolved deps, got %d", result.Stats.DepsResolved)
	}
}

func TestCompressor_FailedDeps(t *testing.T) {
	source := "DATA lo TYPE REF TO zcl_missing."
	provider := &mockProvider{sources: make(map[string]string)} // empty — everything fails

	comp := NewCompressor(provider, 20)
	result, err := comp.Compress(context.Background(), source, "ZCL_TEST", "CLAS")
	if err != nil {
		t.Fatalf("Compress error: %v", err)
	}

	if result.Stats.DepsFailed != 1 {
		t.Errorf("expected 1 failed dep, got %d", result.Stats.DepsFailed)
	}

	// Prologue should be empty since all deps failed
	if result.Prologue != "" {
		t.Errorf("expected empty prologue for all-failed deps, got: %s", result.Prologue)
	}
}

func TestCompressor_EmptySource(t *testing.T) {
	provider := &mockProvider{sources: make(map[string]string)}
	comp := NewCompressor(provider, 20)
	result, err := comp.Compress(context.Background(), "", "ZCL_TEST", "CLAS")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result.Dependencies) != 0 {
		t.Errorf("expected 0 deps, got %d", len(result.Dependencies))
	}
}

func TestCompressor_CustomPrioritized(t *testing.T) {
	source := `DATA lo1 TYPE REF TO cl_standard.
DATA lo2 TYPE REF TO zcl_custom.`

	provider := &mockProvider{
		sources: map[string]string{
			"CLAS:CL_STANDARD": "CLASS cl_standard DEFINITION PUBLIC.\n  PUBLIC SECTION.\n    METHODS m1.\nENDCLASS.",
			"CLAS:ZCL_CUSTOM":  "CLASS zcl_custom DEFINITION PUBLIC.\n  PUBLIC SECTION.\n    METHODS m2.\nENDCLASS.",
		},
	}

	comp := NewCompressor(provider, 1) // only 1 dep allowed
	result, err := comp.Compress(context.Background(), source, "ZCL_TEST", "CLAS")
	if err != nil {
		t.Fatalf("Compress error: %v", err)
	}

	// Custom (Z*) should be prioritized over standard
	if result.Stats.DepsResolved != 1 {
		t.Fatalf("expected 1 resolved, got %d", result.Stats.DepsResolved)
	}
	if result.Contracts[0].Name != "ZCL_CUSTOM" {
		t.Errorf("expected ZCL_CUSTOM prioritized, got %s", result.Contracts[0].Name)
	}
}
