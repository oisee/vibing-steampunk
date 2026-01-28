package adt

import (
	"strings"
	"testing"
)

func TestGetAbapHelpURL(t *testing.T) {
	tests := []struct {
		keyword string
		want    string
	}{
		{"SELECT", "https://help.sap.com/doc/abapdocu_latest_index_htm/latest/en-US/abapselect.htm"},
		{"select", "https://help.sap.com/doc/abapdocu_latest_index_htm/latest/en-US/abapselect.htm"},
		{"LOOP", "https://help.sap.com/doc/abapdocu_latest_index_htm/latest/en-US/abaploop.htm"},
		{"DATA", "https://help.sap.com/doc/abapdocu_latest_index_htm/latest/en-US/abapdata.htm"},
		{"", ""},
	}

	for _, tc := range tests {
		t.Run(tc.keyword, func(t *testing.T) {
			got := GetAbapHelpURL(tc.keyword)
			if got != tc.want {
				t.Errorf("GetAbapHelpURL(%q) = %q, want %q", tc.keyword, got, tc.want)
			}
		})
	}
}

func TestFormatAbapHelpQuery(t *testing.T) {
	tests := []struct {
		keyword     string
		wantContain []string
	}{
		{"SELECT", []string{"SAP", "ABAP", "SELECT", "help.sap.com"}},
		{"loop", []string{"SAP", "ABAP", "LOOP", "help.sap.com"}},
	}

	for _, tc := range tests {
		t.Run(tc.keyword, func(t *testing.T) {
			got := FormatAbapHelpQuery(tc.keyword)
			for _, want := range tc.wantContain {
				if !strings.Contains(got, want) {
					t.Errorf("FormatAbapHelpQuery(%q) = %q, want to contain %q", tc.keyword, got, want)
				}
			}
		})
	}
}
