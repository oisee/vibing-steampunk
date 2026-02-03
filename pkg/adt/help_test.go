package adt

import (
	"context"
	"testing"
)

func TestGetAbapHelpURL(t *testing.T) {
	tests := []struct {
		name    string
		keyword string
		want    string
	}{
		{
			name:    "SELECT keyword",
			keyword: "SELECT",
			want:    "https://help.sap.com/doc/abapdocu_latest_index_htm/latest/en-US/abapselect.htm",
		},
		{
			name:    "LOOP keyword",
			keyword: "LOOP",
			want:    "https://help.sap.com/doc/abapdocu_latest_index_htm/latest/en-US/abaploop.htm",
		},
		{
			name:    "lowercase input",
			keyword: "data",
			want:    "https://help.sap.com/doc/abapdocu_latest_index_htm/latest/en-US/abapdata.htm",
		},
		{
			name:    "mixed case input",
			keyword: "Method",
			want:    "https://help.sap.com/doc/abapdocu_latest_index_htm/latest/en-US/abapmethod.htm",
		},
		{
			name:    "with spaces",
			keyword: "  READ TABLE  ",
			want:    "https://help.sap.com/doc/abapdocu_latest_index_htm/latest/en-US/abapread table.htm",
		},
		{
			name:    "empty string",
			keyword: "",
			want:    "",
		},
		{
			name:    "whitespace only",
			keyword: "   ",
			want:    "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := GetAbapHelpURL(tt.keyword)
			if got != tt.want {
				t.Errorf("GetAbapHelpURL(%q) = %q, want %q", tt.keyword, got, tt.want)
			}
		})
	}
}

func TestFormatAbapHelpQuery(t *testing.T) {
	tests := []struct {
		name    string
		keyword string
		want    string
	}{
		{
			name:    "SELECT keyword",
			keyword: "SELECT",
			want:    "SAP ABAP SELECT statement syntax documentation site:help.sap.com",
		},
		{
			name:    "lowercase input",
			keyword: "loop",
			want:    "SAP ABAP LOOP statement syntax documentation site:help.sap.com",
		},
		{
			name:    "with spaces",
			keyword: "  data  ",
			want:    "SAP ABAP DATA statement syntax documentation site:help.sap.com",
		},
		{
			name:    "empty string",
			keyword: "",
			want:    "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := FormatAbapHelpQuery(tt.keyword)
			if got != tt.want {
				t.Errorf("FormatAbapHelpQuery(%q) = %q, want %q", tt.keyword, got, tt.want)
			}
		})
	}
}

func TestClientGetAbapHelp(t *testing.T) {
	// Create a mock client (no actual HTTP calls needed for Level 1)
	client := &Client{}

	tests := []struct {
		name        string
		keyword     string
		wantKeyword string
		wantErr     bool
	}{
		{
			name:        "valid keyword",
			keyword:     "SELECT",
			wantKeyword: "SELECT",
			wantErr:     false,
		},
		{
			name:        "lowercase becomes uppercase",
			keyword:     "loop",
			wantKeyword: "LOOP",
			wantErr:     false,
		},
		{
			name:        "empty keyword",
			keyword:     "",
			wantKeyword: "",
			wantErr:     true,
		},
		{
			name:        "whitespace only",
			keyword:     "   ",
			wantKeyword: "",
			wantErr:     true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := client.GetAbapHelp(context.Background(), tt.keyword)

			if tt.wantErr {
				if err == nil {
					t.Errorf("GetAbapHelp(%q) expected error, got nil", tt.keyword)
				}
				return
			}

			if err != nil {
				t.Errorf("GetAbapHelp(%q) unexpected error: %v", tt.keyword, err)
				return
			}

			if result.Keyword != tt.wantKeyword {
				t.Errorf("GetAbapHelp(%q).Keyword = %q, want %q", tt.keyword, result.Keyword, tt.wantKeyword)
			}

			if result.URL == "" {
				t.Errorf("GetAbapHelp(%q).URL is empty", tt.keyword)
			}

			if result.SearchQuery == "" {
				t.Errorf("GetAbapHelp(%q).SearchQuery is empty", tt.keyword)
			}

			// Documentation should be empty for Level 1 (no ZADT_VSP)
			if result.Documentation != "" {
				t.Errorf("GetAbapHelp(%q).Documentation should be empty without ZADT_VSP", tt.keyword)
			}
		})
	}
}
