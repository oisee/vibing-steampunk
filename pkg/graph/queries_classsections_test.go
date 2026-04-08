package graph

import (
	"strings"
	"testing"
)

func TestClassifySections_Basic(t *testing.T) {
	elements := []ClassStructureElement{
		{Name: "GET_DATA", ADTType: "CLAS/OM", Visibility: "public", Level: "instance"},
		{Name: "CREATE", ADTType: "CLAS/OM", Visibility: "public", Level: "instance"},
		{Name: "FACTORY", ADTType: "CLAS/OM", Visibility: "public", Level: "static"},
		{Name: "VALIDATE", ADTType: "CLAS/OM", Visibility: "protected", Level: "instance"},
		{Name: "HELPER", ADTType: "CLAS/OM", Visibility: "private", Level: "instance"},
		{Name: "MV_DATA", ADTType: "CLAS/OA", Visibility: "private", Level: "instance"},
		{Name: "TY_DATA", ADTType: "CLAS/OT", Visibility: "public", Level: ""},
		{Name: "CHANGED", ADTType: "CLAS/OO", Visibility: "public", Level: "instance"},
	}

	result := ClassifySections("ZCL_TEST", elements)

	if result.ClassName != "ZCL_TEST" {
		t.Errorf("ClassName: got %q", result.ClassName)
	}
	if result.Summary.TotalMembers != 8 {
		t.Errorf("TotalMembers: got %d, want 8", result.Summary.TotalMembers)
	}

	// Verify sections exist in order: PUBLIC, PROTECTED, PRIVATE
	if len(result.Sections) != 3 {
		t.Fatalf("Expected 3 sections, got %d", len(result.Sections))
	}
	if result.Sections[0].Visibility != "PUBLIC" {
		t.Errorf("First section: got %q, want PUBLIC", result.Sections[0].Visibility)
	}
	if result.Sections[1].Visibility != "PROTECTED" {
		t.Errorf("Second section: got %q, want PROTECTED", result.Sections[1].Visibility)
	}
	if result.Sections[2].Visibility != "PRIVATE" {
		t.Errorf("Third section: got %q, want PRIVATE", result.Sections[2].Visibility)
	}

	// PUBLIC section: 3 methods + 1 type + 1 event
	pub := result.Sections[0]
	if len(pub.Methods) != 3 {
		t.Errorf("PUBLIC methods: got %d, want 3", len(pub.Methods))
	}
	if len(pub.Types) != 1 {
		t.Errorf("PUBLIC types: got %d, want 1", len(pub.Types))
	}
	if len(pub.Events) != 1 {
		t.Errorf("PUBLIC events: got %d, want 1", len(pub.Events))
	}

	// PRIVATE section: 1 method + 1 attribute
	priv := result.Sections[2]
	if len(priv.Methods) != 1 {
		t.Errorf("PRIVATE methods: got %d, want 1", len(priv.Methods))
	}
	if len(priv.Attributes) != 1 {
		t.Errorf("PRIVATE attributes: got %d, want 1", len(priv.Attributes))
	}
}

func TestClassifySections_Sorting(t *testing.T) {
	elements := []ClassStructureElement{
		{Name: "ZEBRA", ADTType: "CLAS/OM", Visibility: "public"},
		{Name: "ALPHA", ADTType: "CLAS/OM", Visibility: "public"},
		{Name: "MIDDLE", ADTType: "CLAS/OM", Visibility: "public"},
	}

	result := ClassifySections("ZCL_TEST", elements)

	methods := result.Sections[0].Methods
	if methods[0].Name != "ALPHA" || methods[1].Name != "MIDDLE" || methods[2].Name != "ZEBRA" {
		t.Errorf("Methods not sorted: got %s, %s, %s", methods[0].Name, methods[1].Name, methods[2].Name)
	}
}

func TestClassifySections_StaticLevel(t *testing.T) {
	elements := []ClassStructureElement{
		{Name: "FACTORY", ADTType: "CLAS/OM", Visibility: "public", Level: "static"},
		{Name: "INSTANCE_M", ADTType: "CLAS/OM", Visibility: "public", Level: "instance"},
	}

	result := ClassifySections("ZCL_TEST", elements)

	for _, m := range result.Sections[0].Methods {
		if m.Name == "FACTORY" && m.Level != "static" {
			t.Error("FACTORY should be static")
		}
		if m.Name == "INSTANCE_M" && m.Level != "instance" {
			t.Error("INSTANCE_M should be instance")
		}
	}
}

func TestClassifySections_Empty(t *testing.T) {
	result := ClassifySections("ZCL_EMPTY", nil)

	if result.Summary.TotalMembers != 0 {
		t.Errorf("Empty class: got %d members", result.Summary.TotalMembers)
	}
	if len(result.Sections) != 0 {
		t.Errorf("Empty class: got %d sections", len(result.Sections))
	}
}

func TestClassifySections_EmptyVisibilityDefaultsToPrivate(t *testing.T) {
	elements := []ClassStructureElement{
		{Name: "MYSTERY", ADTType: "CLAS/OM", Visibility: ""},
	}

	result := ClassifySections("ZCL_TEST", elements)

	if result.Summary.ByVisibility["PRIVATE"] != 1 {
		t.Error("Empty visibility should default to PRIVATE")
	}
}

func TestClassifySections_Summary(t *testing.T) {
	elements := []ClassStructureElement{
		{Name: "M1", ADTType: "CLAS/OM", Visibility: "public"},
		{Name: "M2", ADTType: "CLAS/OM", Visibility: "public"},
		{Name: "M3", ADTType: "CLAS/OM", Visibility: "private"},
		{Name: "A1", ADTType: "CLAS/OA", Visibility: "private"},
	}

	result := ClassifySections("ZCL_TEST", elements)

	if result.Summary.ByVisibility["PUBLIC"] != 2 {
		t.Errorf("PUBLIC count: got %d, want 2", result.Summary.ByVisibility["PUBLIC"])
	}
	if result.Summary.ByVisibility["PRIVATE"] != 2 {
		t.Errorf("PRIVATE count: got %d, want 2", result.Summary.ByVisibility["PRIVATE"])
	}
	if result.Summary.ByKind["method"] != 3 {
		t.Errorf("method count: got %d, want 3", result.Summary.ByKind["method"])
	}
	if result.Summary.ByKind["attribute"] != 1 {
		t.Errorf("attribute count: got %d, want 1", result.Summary.ByKind["attribute"])
	}
}

func TestFormatClassSections(t *testing.T) {
	elements := []ClassStructureElement{
		{Name: "GET_DATA", ADTType: "CLAS/OM", Visibility: "public", Level: "instance"},
		{Name: "FACTORY", ADTType: "CLAS/OM", Visibility: "public", Level: "static"},
		{Name: "MV_DATA", ADTType: "CLAS/OA", Visibility: "private", Level: "instance"},
	}

	result := ClassifySections("ZCL_TEST", elements)
	text := FormatClassSections(result)

	if !strings.Contains(text, "ZCL_TEST") {
		t.Error("Should contain class name")
	}
	if !strings.Contains(text, "=== PUBLIC ===") {
		t.Error("Should contain PUBLIC section")
	}
	if !strings.Contains(text, "FACTORY [static]") {
		t.Error("Should show static annotation")
	}
	if !strings.Contains(text, "MV_DATA") {
		t.Error("Should contain attribute")
	}
	if !strings.Contains(text, "=== PRIVATE ===") {
		t.Error("Should contain PRIVATE section")
	}
}

func TestClassifySections_SkipsEmptySections(t *testing.T) {
	elements := []ClassStructureElement{
		{Name: "M1", ADTType: "CLAS/OM", Visibility: "public"},
		// No protected or private members
	}

	result := ClassifySections("ZCL_TEST", elements)

	if len(result.Sections) != 1 {
		t.Errorf("Should only have 1 section (PUBLIC), got %d", len(result.Sections))
	}
}
