package main

import (
	"strings"
	"testing"

	"github.com/oisee/vibing-steampunk/embedded/deps"
)

func TestDeploymentSummaryError(t *testing.T) {
	if err := deploymentSummaryError(0, 0); err != nil {
		t.Fatalf("deploymentSummaryError(0, 0) = %v, want nil", err)
	}
	if err := deploymentSummaryError(2, 1); err == nil || !strings.Contains(err.Error(), "2 failed and 1 skipped") {
		t.Fatalf("deploymentSummaryError(2, 1) = %v, want failure and skip counts", err)
	}
}

func TestCopyObjectSupported(t *testing.T) {
	tests := []struct {
		name       string
		object     deps.DeploymentObject
		want       bool
		wantReason string
	}{
		{name: "program", object: deps.DeploymentObject{Type: "PROG"}, want: true},
		{name: "class main", object: deps.DeploymentObject{Type: "CLAS"}, want: true},
		{name: "class includes", object: deps.DeploymentObject{Type: "CLAS", Includes: map[string]string{"testclasses": "synthetic"}}, wantReason: "include deployment"},
		{name: "unsupported type", object: deps.DeploymentObject{Type: "TABL"}, wantReason: "not supported"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, reason := copyObjectSupported(tt.object)
			if got != tt.want {
				t.Fatalf("copyObjectSupported() = %v, want %v", got, tt.want)
			}
			if tt.wantReason != "" && !strings.Contains(reason, tt.wantReason) {
				t.Fatalf("reason = %q, want substring %q", reason, tt.wantReason)
			}
		})
	}
}
