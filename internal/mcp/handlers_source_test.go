package mcp

import (
	"strings"
	"testing"

	"github.com/oisee/vibing-steampunk/pkg/adt"
)

func TestValidateWriteSourceResult(t *testing.T) {
	tests := []struct {
		name    string
		result  *adt.WriteSourceResult
		wantErr string
	}{
		{name: "success", result: &adt.WriteSourceResult{Success: true}},
		{name: "nil result", wantErr: "no result"},
		{name: "diagnostic", result: &adt.WriteSourceResult{Message: "synthetic syntax failure"}, wantErr: "synthetic syntax failure"},
		{name: "missing diagnostic", result: &adt.WriteSourceResult{}, wantErr: "without a diagnostic"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateWriteSourceResult(tt.result)
			if tt.wantErr == "" {
				if err != nil {
					t.Fatalf("validateWriteSourceResult() error = %v", err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("validateWriteSourceResult() error = %v, want substring %q", err, tt.wantErr)
			}
		})
	}
}
