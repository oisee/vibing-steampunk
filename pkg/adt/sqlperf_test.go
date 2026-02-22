package adt

import (
	"context"
	"net/http"
	"testing"
)

func TestAnalyzePlanNodes_FullTableScan(t *testing.T) {
	nodes := []SQLPlanNode{
		{
			ID:       1,
			Operator: "TABLE SCAN",
			Table:    "MARA",
			Rows:     50000,
			Cost:     500,
		},
	}
	findings := AnalyzePlanNodes(nodes)
	if len(findings) == 0 {
		t.Fatal("expected findings for full table scan on large table")
	}

	found := false
	for _, f := range findings {
		if f.Type == "full_table_scan" && f.Severity == "critical" {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected critical full_table_scan finding")
	}
}

func TestAnalyzePlanNodes_SmallTableScan(t *testing.T) {
	nodes := []SQLPlanNode{
		{
			ID:       1,
			Operator: "TABLE SCAN",
			Table:    "T000",
			Rows:     5,
			Cost:     1,
		},
	}
	findings := AnalyzePlanNodes(nodes)
	if len(findings) == 0 {
		t.Fatal("expected finding for small table scan")
	}

	found := false
	for _, f := range findings {
		if f.Type == "full_scan_small" && f.Severity == "info" {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected info full_scan_small finding")
	}
}

func TestAnalyzePlanNodes_NestedLoopLarge(t *testing.T) {
	nodes := []SQLPlanNode{
		{
			ID:       1,
			Operator: "NESTED LOOP JOIN",
			Cost:     2000,
			Rows:     100000,
			Children: []SQLPlanNode{
				{ID: 2, Operator: "INDEX SCAN", Table: "BKPF", Rows: 50000, Cost: 100},
				{ID: 3, Operator: "INDEX SCAN", Table: "BSEG", Rows: 200000, Cost: 1500},
			},
		},
	}
	findings := AnalyzePlanNodes(nodes)

	found := false
	for _, f := range findings {
		if f.Type == "nested_loop_large" && f.Severity == "high" {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected high nested_loop_large finding")
	}
}

func TestAnalyzePlanNodes_GoodPlan(t *testing.T) {
	nodes := []SQLPlanNode{
		{
			ID:       1,
			Operator: "INDEX SCAN",
			Table:    "MARA",
			Index:    "MARA~001",
			Rows:     10,
			Cost:     5,
		},
	}
	findings := AnalyzePlanNodes(nodes)
	if len(findings) != 0 {
		t.Errorf("expected no findings for good plan, got %d: %+v", len(findings), findings)
	}
}

func TestAnalyzePlanNodes_Empty(t *testing.T) {
	findings := AnalyzePlanNodes(nil)
	if len(findings) != 0 {
		t.Errorf("expected no findings for empty nodes, got %d", len(findings))
	}
	findings = AnalyzePlanNodes([]SQLPlanNode{})
	if len(findings) != 0 {
		t.Errorf("expected no findings for empty slice, got %d", len(findings))
	}
}

func TestAnalyzeSQLText_ABAPSelectStar(t *testing.T) {
	query := `SELECT * FROM mara WHERE matnr = @lv_matnr INTO TABLE @lt_result.`
	findings := AnalyzeSQLText(query)

	found := false
	for _, f := range findings {
		if f.Type == "select_star" {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected select_star finding for SELECT * FROM mara")
	}
}

func TestAnalyzeSQLText_ABAPMissingWhere(t *testing.T) {
	query := `SELECT matnr maktx FROM mara INTO TABLE @lt_result.`
	findings := AnalyzeSQLText(query)

	found := false
	for _, f := range findings {
		if f.Type == "missing_where" {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected missing_where finding for SELECT without WHERE")
	}
}

func TestStripABAPSQLSyntax(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{
			name:  "host variables",
			input: `SELECT matnr FROM mara WHERE matnr = @lv_matnr`,
			want:  `SELECT matnr FROM mara WHERE matnr = lv_matnr`,
		},
		{
			name:  "INTO TABLE",
			input: `SELECT matnr FROM mara INTO TABLE @lt_result.`,
			want:  `SELECT matnr FROM mara`,
		},
		{
			name:  "UP TO n ROWS",
			input: `SELECT matnr FROM mara UP TO 100 ROWS WHERE matnr LIKE 'A%'`,
			want:  `SELECT matnr FROM mara WHERE matnr LIKE 'A%'`,
		},
		{
			name:  "APPENDING TABLE",
			input: `SELECT matnr FROM mara APPENDING TABLE @lt_result WHERE matnr = 'X'.`,
			want:  `SELECT matnr FROM mara WHERE matnr = 'X'`,
		},
		{
			name:  "FOR ALL ENTRIES",
			input: `SELECT matnr FROM mara FOR ALL ENTRIES IN @lt_keys WHERE matnr = @lt_keys-matnr`,
			want:  `SELECT matnr FROM mara WHERE matnr = lt_keys-matnr`,
		},
		{
			name:  "combined",
			input: `SELECT matnr maktx FROM mara INTO CORRESPONDING FIELDS OF TABLE @lt_result UP TO 50 ROWS WHERE matnr IN @lt_range.`,
			want:  `SELECT matnr maktx FROM mara WHERE matnr IN lt_range`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := stripABAPSQLSyntax(tt.input)
			if got != tt.want {
				t.Errorf("stripABAPSQLSyntax(%q)\n  got:  %q\n  want: %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestCalculateSQLScore(t *testing.T) {
	tests := []struct {
		name     string
		findings []SQLPerformanceFinding
		want     string
	}{
		{
			name:     "no findings",
			findings: nil,
			want:     "good",
		},
		{
			name: "info only",
			findings: []SQLPerformanceFinding{
				{Severity: "info"},
			},
			want: "good",
		},
		{
			name: "has high",
			findings: []SQLPerformanceFinding{
				{Severity: "info"},
				{Severity: "high"},
			},
			want: "warning",
		},
		{
			name: "has critical",
			findings: []SQLPerformanceFinding{
				{Severity: "critical"},
				{Severity: "info"},
			},
			want: "critical",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := calculateSQLScore(tt.findings)
			if got != tt.want {
				t.Errorf("calculateSQLScore() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestClient_AnalyzeSQLPerformance(t *testing.T) {
	// Test with HANA unavailable (text-only path)
	t.Run("non-HANA", func(t *testing.T) {
		mock := &mockTransportClient{
			responses: map[string]*http.Response{},
		}
		cfg := NewConfig("https://sap.example.com:44300", "user", "pass")
		transport := NewTransportWithClient(cfg, mock)
		client := NewClientWithTransport(cfg, transport)

		result, err := client.AnalyzeSQLPerformance(context.Background(),
			`SELECT * FROM mara INTO TABLE @lt_result.`, false)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if result == nil {
			t.Fatal("expected non-nil result")
		}
		if result.HANAAvailable {
			t.Error("expected HANAAvailable=false")
		}
		if len(result.Warnings) == 0 {
			t.Error("expected warning about HANA not available")
		}
		if len(result.TextFindings) == 0 {
			t.Error("expected text findings for SELECT *")
		}
		if result.Summary.Score == "" {
			t.Error("expected non-empty score")
		}
	})

	// Test with HANA available (plan + text)
	t.Run("HANA", func(t *testing.T) {
		planXML := `<?xml version="1.0" encoding="utf-8"?>
<explainResult>
  <node id="1" operator="TABLE SCAN" tableName="MARA" cost="500" outputRowCount="50000">
  </node>
</explainResult>`

		mock := &mockTransportClient{
			responses: map[string]*http.Response{
				"datapreview": newTestResponse(planXML),
				"discovery":   newTestResponse("OK"),
			},
		}
		cfg := NewConfig("https://sap.example.com:44300", "user", "pass")
		transport := NewTransportWithClient(cfg, mock)
		// Pre-set CSRF token to avoid CSRF fetch in tests (POST requires it).
		// newTestResponse uses non-canonical http.Header literal which Header.Get() can't find.
		transport.csrfToken = "test-token"
		client := NewClientWithTransport(cfg, transport)

		result, err := client.AnalyzeSQLPerformance(context.Background(),
			`SELECT * FROM MARA`, true)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if !result.HANAAvailable {
			t.Error("expected HANAAvailable=true")
		}
		if len(result.PlanFindings) == 0 {
			t.Error("expected plan findings for full table scan")
		}
		if len(result.TextFindings) == 0 {
			t.Error("expected text findings for SELECT *")
		}
		if result.Summary.Score != "critical" {
			t.Errorf("expected critical score, got %q", result.Summary.Score)
		}
	})
}
