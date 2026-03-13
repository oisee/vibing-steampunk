package deps

import "testing"

func TestGetDependencyZIP(t *testing.T) {
	tests := []struct {
		name  string
		input string
	}{
		{name: "standalone-lowercase", input: "abapgit-standalone"},
		{name: "standalone-uppercase", input: "ABAPGIT-STANDALONE"},
		{name: "dev-trimmed", input: "  abapgit-dev  "},
		{name: "unknown", input: "does-not-exist"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := GetDependencyZIP(tt.input); got != nil {
				t.Fatalf("expected nil ZIP for %q in placeholder build, got %d bytes", tt.input, len(got))
			}
		})
	}
}
