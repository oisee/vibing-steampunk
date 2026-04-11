package graph

import (
	"strings"
	"testing"
)

func TestCoChangeToMermaid(t *testing.T) {
	result := &CoChangeResult{
		Target:          "CLAS:ZCL_FOO",
		TotalTransports: 3,
		CoChanges: []CoChangeEntry{
			{NodeID: "CLAS:ZCL_BAR", Name: "ZCL_BAR", Type: "CLAS", Count: 2, Transports: []string{"TR:A4HK1", "TR:A4HK2"}},
			{NodeID: "PROG:ZREPORT", Name: "ZREPORT", Type: "PROG", Count: 1, Transports: []string{"TR:A4HK1"}},
		},
	}

	mmd := CoChangeToMermaid(result)

	if !strings.Contains(mmd, "graph LR") {
		t.Error("Should start with graph LR")
	}
	if !strings.Contains(mmd, "CLAS ZCL_FOO") {
		t.Error("Should contain target node label")
	}
	if !strings.Contains(mmd, "2x") {
		t.Error("Should contain edge count label")
	}
	if !strings.Contains(mmd, "CLAS ZCL_BAR") {
		t.Error("Should contain co-change entry")
	}
}

func TestConfigUsageToMermaid(t *testing.T) {
	result := &ConfigUsageResult{
		Variable:     "TVARVC:ZKEKEKE",
		VariableName: "ZKEKEKE",
		Found:        true,
		Readers: []ConfigReaderEntry{
			{NodeID: "PROG:ZREPORT", Name: "ZREPORT", Type: "PROG", Confidence: "HIGH", Package: "$ZDEV"},
			{NodeID: "CLAS:ZCL_ORDER", Name: "ZCL_ORDER", Type: "CLAS", Confidence: "MEDIUM"},
		},
	}

	mmd := ConfigUsageToMermaid(result)

	if !strings.Contains(mmd, "graph LR") {
		t.Error("Should start with graph LR")
	}
	if !strings.Contains(mmd, "ZKEKEKE") {
		t.Error("Should contain variable name")
	}
	if !strings.Contains(mmd, "HIGH") {
		t.Error("Should contain HIGH confidence label")
	}
	if !strings.Contains(mmd, "MEDIUM") {
		t.Error("Should contain MEDIUM confidence label")
	}
	if !strings.Contains(mmd, "$ZDEV") {
		t.Error("Should contain package in edge label")
	}
	// HIGH nodes should have green styling
	if !strings.Contains(mmd, "fill:#065f46") {
		t.Error("HIGH confidence nodes should have green style")
	}
	// MEDIUM nodes should have amber styling
	if !strings.Contains(mmd, "fill:#92400e") {
		t.Error("MEDIUM confidence nodes should have amber style")
	}
}

func TestWrapMermaidHTML(t *testing.T) {
	html := WrapMermaidHTML("Test Title", "graph LR\n    A-->B")

	if !strings.Contains(html, "<!DOCTYPE html>") {
		t.Error("Should be valid HTML")
	}
	if !strings.Contains(html, "Test Title") {
		t.Error("Should contain title")
	}
	if !strings.Contains(html, "mermaid") {
		t.Error("Should contain mermaid div")
	}
	if !strings.Contains(html, "A-->B") {
		t.Error("Should contain mermaid text")
	}
	if !strings.Contains(html, "cdn.jsdelivr.net/npm/mermaid") {
		t.Error("Should include mermaid CDN script")
	}
}

func TestCoChangeToMermaid_Empty(t *testing.T) {
	result := &CoChangeResult{Target: "CLAS:ZCL_FOO"}

	mmd := CoChangeToMermaid(result)

	if !strings.Contains(mmd, "graph LR") {
		t.Error("Empty result should still produce valid mermaid")
	}
	if !strings.Contains(mmd, "CLAS ZCL_FOO") {
		t.Error("Should still show target node")
	}
}
