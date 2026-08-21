package adt

import "testing"

// TestResolveWriteTransport covers the transport-reuse contract behind issue #144:
// fall back to the object's already-open request (LockResult.CorrNr) when the caller
// supplies no transport — but never at the expense of the transportable-edit policy.
func TestResolveWriteTransport(t *testing.T) {
	const (
		suppliedTR = "TRXX900100" // synthetic transport ids (not real system values)
		lockTR     = "TRXX900999"
	)

	tests := []struct {
		name       string
		opts       []Option
		supplied   string
		lockCorrNr string
		want       string
		wantErr    bool
	}{
		{
			name:       "explicit transport is used as-is (already gated upstream)",
			opts:       []Option{WithAllowTransportableEdits()},
			supplied:   suppliedTR,
			lockCorrNr: lockTR,
			want:       suppliedTR,
		},
		{
			name:       "no transport and no open request stays local (empty)",
			supplied:   "",
			lockCorrNr: "",
			want:       "",
		},
		{
			name:       "falls back to lock CorrNr when transportable edits are allowed",
			opts:       []Option{WithAllowTransportableEdits()},
			supplied:   "",
			lockCorrNr: lockTR,
			want:       lockTR,
		},
		{
			// The safety-critical case: the object is bound to an open request, but
			// transportable edits are disabled. Auto-reuse must NOT sneak the edit
			// past the gate just because the caller omitted the transport.
			name:       "fallback is blocked when transportable edits are disabled",
			opts:       nil, // AllowTransportableEdits defaults to false
			supplied:   "",
			lockCorrNr: lockTR,
			wantErr:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := NewClient("https://sap.example.com:44300", "user", "pass", tt.opts...)
			got, err := c.resolveWriteTransport(tt.supplied, tt.lockCorrNr, "TestOp")
			if tt.wantErr {
				if err == nil {
					t.Fatalf("expected error (gate must not be bypassed), got nil and result %q", got)
				}
				if got != "" {
					t.Fatalf("expected empty result on blocked fallback, got %q", got)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tt.want {
				t.Fatalf("resolveWriteTransport(%q, %q) = %q, want %q", tt.supplied, tt.lockCorrNr, got, tt.want)
			}
		})
	}
}
