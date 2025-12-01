package adt

import (
	"testing"
)

func TestParseSearchResults(t *testing.T) {
	xml := `<?xml version="1.0" encoding="UTF-8"?>
<adtcore:objectReferences xmlns:adtcore="http://www.sap.com/adt/core">
  <adtcore:objectReference adtcore:uri="/sap/bc/adt/programs/programs/ztest_program" adtcore:type="PROG/P" adtcore:name="ZTEST_PROGRAM" adtcore:packageName="ZTEST" adtcore:description="Test Program"/>
  <adtcore:objectReference adtcore:uri="/sap/bc/adt/oo/classes/zcl_test_class" adtcore:type="CLAS/OC" adtcore:name="ZCL_TEST_CLASS" adtcore:packageName="ZTEST" adtcore:description="Test Class"/>
</adtcore:objectReferences>`

	results, err := ParseSearchResults([]byte(xml))
	if err != nil {
		t.Fatalf("ParseSearchResults failed: %v", err)
	}

	if len(results) != 2 {
		t.Fatalf("Expected 2 results, got %d", len(results))
	}

	// Check first result
	if results[0].Name != "ZTEST_PROGRAM" {
		t.Errorf("First result name = %v, want ZTEST_PROGRAM", results[0].Name)
	}
	if results[0].Type != "PROG/P" {
		t.Errorf("First result type = %v, want PROG/P", results[0].Type)
	}
	if results[0].PackageName != "ZTEST" {
		t.Errorf("First result package = %v, want ZTEST", results[0].PackageName)
	}

	// Check second result
	if results[1].Name != "ZCL_TEST_CLASS" {
		t.Errorf("Second result name = %v, want ZCL_TEST_CLASS", results[1].Name)
	}
}

func TestParseSearchResults_Empty(t *testing.T) {
	xml := `<?xml version="1.0" encoding="UTF-8"?>
<adtcore:objectReferences xmlns:adtcore="http://www.sap.com/adt/core">
</adtcore:objectReferences>`

	results, err := ParseSearchResults([]byte(xml))
	if err != nil {
		t.Fatalf("ParseSearchResults failed: %v", err)
	}

	if len(results) != 0 {
		t.Errorf("Expected 0 results, got %d", len(results))
	}
}

func TestFindLink(t *testing.T) {
	links := []Link{
		{Rel: "self", Href: "/self"},
		{Rel: "http://www.sap.com/adt/relations/source/main", Href: "/source/main"},
		{Rel: "edit", Href: "/edit"},
	}

	tests := []struct {
		rel      string
		wantHref string
		wantNil  bool
	}{
		{"self", "/self", false},
		{"http://www.sap.com/adt/relations/source/main", "/source/main", false},
		{"edit", "/edit", false},
		{"nonexistent", "", true},
	}

	for _, tt := range tests {
		t.Run(tt.rel, func(t *testing.T) {
			link := FindLink(links, tt.rel)
			if tt.wantNil {
				if link != nil {
					t.Errorf("Expected nil, got %v", link)
				}
			} else {
				if link == nil {
					t.Fatal("Expected link, got nil")
				}
				if link.Href != tt.wantHref {
					t.Errorf("Href = %v, want %v", link.Href, tt.wantHref)
				}
			}
		})
	}
}

func TestFindLinkByType(t *testing.T) {
	links := []Link{
		{Type: "application/xml", Href: "/xml"},
		{Type: "text/plain", Href: "/text"},
		{Type: "application/vnd.sap.adt.programs.v2+xml", Href: "/programs"},
	}

	tests := []struct {
		contentType string
		wantHref    string
		wantNil     bool
	}{
		{"application/xml", "/xml", false},
		{"text/plain", "/text", false},
		{"programs", "/programs", false},
		{"nonexistent", "", true},
	}

	for _, tt := range tests {
		t.Run(tt.contentType, func(t *testing.T) {
			link := FindLinkByType(links, tt.contentType)
			if tt.wantNil {
				if link != nil {
					t.Errorf("Expected nil, got %v", link)
				}
			} else {
				if link == nil {
					t.Fatal("Expected link, got nil")
				}
				if link.Href != tt.wantHref {
					t.Errorf("Href = %v, want %v", link.Href, tt.wantHref)
				}
			}
		})
	}
}

func TestExtractSourceLink(t *testing.T) {
	tests := []struct {
		name     string
		links    []Link
		wantHref string
	}{
		{
			name: "main source relation",
			links: []Link{
				{Rel: "http://www.sap.com/adt/relations/source/main", Href: "/source/main"},
			},
			wantHref: "/source/main",
		},
		{
			name: "source relation",
			links: []Link{
				{Rel: "http://www.sap.com/adt/relations/source", Href: "/source"},
			},
			wantHref: "/source",
		},
		{
			name: "simple source",
			links: []Link{
				{Rel: "source", Href: "/src"},
			},
			wantHref: "/src",
		},
		{
			name: "prefers main over source",
			links: []Link{
				{Rel: "http://www.sap.com/adt/relations/source", Href: "/source"},
				{Rel: "http://www.sap.com/adt/relations/source/main", Href: "/source/main"},
			},
			wantHref: "/source/main",
		},
		{
			name:     "no source link",
			links:    []Link{{Rel: "edit", Href: "/edit"}},
			wantHref: "",
		},
		{
			name:     "empty links",
			links:    []Link{},
			wantHref: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ExtractSourceLink(tt.links)
			if got != tt.wantHref {
				t.Errorf("ExtractSourceLink() = %v, want %v", got, tt.wantHref)
			}
		})
	}
}

func TestParseObjectStructure(t *testing.T) {
	xml := `<?xml version="1.0" encoding="UTF-8"?>
<adtcore:objectStructure xmlns:adtcore="http://www.sap.com/adt/core"
    adtcore:uri="/sap/bc/adt/programs/programs/ztest"
    adtcore:type="PROG/P"
    adtcore:name="ZTEST">
  <adtcore:link href="/sap/bc/adt/programs/programs/ztest/source/main" rel="http://www.sap.com/adt/relations/source/main" type="text/plain"/>
</adtcore:objectStructure>`

	obj, err := ParseObjectStructure([]byte(xml))
	if err != nil {
		t.Fatalf("ParseObjectStructure failed: %v", err)
	}

	if obj.Name != "ZTEST" {
		t.Errorf("Name = %v, want ZTEST", obj.Name)
	}
	if obj.Type != "PROG/P" {
		t.Errorf("Type = %v, want PROG/P", obj.Type)
	}
}
