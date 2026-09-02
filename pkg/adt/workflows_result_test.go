package adt

import (
	"strings"
	"testing"
)

func TestWriteSourceResultError(t *testing.T) {
	tests := []struct {
		name    string
		result  *WriteSourceResult
		wantErr string
	}{
		{name: "success", result: &WriteSourceResult{Success: true}},
		{name: "nil result", wantErr: "no result"},
		{name: "diagnostic", result: &WriteSourceResult{Message: "synthetic activation failure"}, wantErr: "synthetic activation failure"},
		{name: "missing diagnostic", result: &WriteSourceResult{}, wantErr: "without a diagnostic"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := WriteSourceResultError(tt.result)
			if tt.wantErr == "" {
				if err != nil {
					t.Fatalf("WriteSourceResultError() error = %v", err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("WriteSourceResultError() error = %v, want substring %q", err, tt.wantErr)
			}
		})
	}
}
