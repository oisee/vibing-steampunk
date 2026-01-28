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
