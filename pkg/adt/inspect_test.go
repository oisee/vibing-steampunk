package adt

import (
	"strings"
	"testing"
	"time"
)

func TestVariantMarkdown(t *testing.T) {
	v := &Variant{Report: "ZDEMO_RUN", Name: "MONTH_END", Text: "Month end", CreatedBy: "TESTUSER", Created: time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC),
		Fields: []VariantField{
			{Name: "P_BUKRS", Label: "Company Code", Kind: "P", Type: "C", DDIC: "T001-BUKRS", Value: "1000"},
			{Name: "S_DATE", Label: "Posting Date", Kind: "S", Type: "D", Ranges: []Range{{Sign: "I", Option: "BT", Low: "20260901", High: "20260930"}, {Sign: "E", Option: "EQ", Low: "20260915"}}},
			{Name: "P_TEST", Kind: "P", Type: "C", Value: "X"},
		}}
	md := VariantMarkdown(v)
	for _, want := range []string{"# ZDEMO_RUN / MONTH_END", "Month end", "Created by TESTUSER on 2026-09-01.",
		"| P_BUKRS | Company Code | P | C T001-BUKRS | 1000 |", "| S_DATE | Posting Date | S | D | I BT 20260901 … 20260930<br>E EQ 20260915 |", "| P_TEST |  | P | C | X |"} {
		if !strings.Contains(md, want) {
			t.Errorf("missing %q in:\n%s", want, md)
		}
	}
}

func TestFunctionTestMarkdown(t *testing.T) {
	d := &FunctionTestData{Function: "ZDEMO_CALC", Group: "ZDEMO",
		Interface: []FunctionTestParam{{Name: "IV_AMOUNT", Type: "P", Length: "8"}, {Name: "EV_TAX", Type: "P", Length: "8"}},
		Sets: []FunctionTestSet{{Number: "001", Title: "ten percent", Date: "20260901", Time: "120000",
			Inputs: map[string]any{"IV_AMOUNT": "100.00", "IV_RATE": "10"}, Outputs: map[string]any{"EV_TAX": "10.00"}, Runtime: "42 µs", RC: "0"}}}
	md := FunctionTestMarkdown(d)
	for _, want := range []string{"# Test data of ZDEMO_CALC (ZDEMO)", "| IV_AMOUNT | P | 8 |  |", "## Set 001 — ten percent (20260901 120000)",
		"| IV_AMOUNT | 100.00 |\n| IV_RATE | 10 |", "| EV_TAX | 10.00 |", "runtime 42 µs, rc 0"} {
		if !strings.Contains(md, want) {
			t.Errorf("missing %q in:\n%s", want, md)
		}
	}
}
