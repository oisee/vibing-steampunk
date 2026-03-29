package deps

import "testing"

func TestGetDependencyZIP(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		wantData bool
	}{
		{name: "standalone-lowercase", input: "abapgit-standalone", wantData: true},
		{name: "standalone-uppercase", input: "ABAPGIT-STANDALONE", wantData: true},
		{name: "full", input: "abapgit-full", wantData: true},
		{name: "dev-alias-trimmed", input: "  abapgit-dev  ", wantData: true},
		{name: "unknown", input: "does-not-exist", wantData: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := GetDependencyZIP(tt.input)
			if tt.wantData && got == nil {
				t.Fatalf("expected ZIP data for %q, got nil", tt.input)
			}
			if !tt.wantData && got != nil {
				t.Fatalf("expected nil for %q, got %d bytes", tt.input, len(got))
			}
		})
	}
}
