package main

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/oisee/vibing-steampunk/embedded/deps"
	"github.com/oisee/vibing-steampunk/pkg/adt"
)

type fakeCopyPackageClient struct {
	exists      bool
	existsErr   error
	createErr   error
	createCalls int
}

func (f *fakeCopyPackageClient) PackageExists(context.Context, string) (bool, error) {
	return f.exists, f.existsErr
}

func (f *fakeCopyPackageClient) CreateObject(context.Context, adt.CreateObjectOptions) error {
	f.createCalls++
	return f.createErr
}

func TestValidateCopyWriteResult(t *testing.T) {
	tests := []struct {
		name    string
		result  *adt.WriteSourceResult
		wantErr string
	}{
		{name: "success", result: &adt.WriteSourceResult{Success: true}},
		{name: "nil result", wantErr: "no result"},
		{name: "diagnostic", result: &adt.WriteSourceResult{Message: "synthetic activation failure"}, wantErr: "synthetic activation failure"},
		{name: "missing diagnostic", result: &adt.WriteSourceResult{}, wantErr: "without a diagnostic"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateCopyWriteResult(tt.result)
			if tt.wantErr == "" {
				if err != nil {
					t.Fatalf("validateCopyWriteResult() error = %v", err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("validateCopyWriteResult() error = %v, want substring %q", err, tt.wantErr)
			}
		})
	}
}

func TestDeploymentSummaryError(t *testing.T) {
	if err := deploymentSummaryError(0); err != nil {
		t.Fatalf("deploymentSummaryError(0) = %v, want nil", err)
	}
	if err := deploymentSummaryError(2); err == nil || !strings.Contains(err.Error(), "2 failed") {
		t.Fatalf("deploymentSummaryError(2) = %v, want failure count", err)
	}
}

func TestEnsureCopyTargetPackage(t *testing.T) {
	tests := []struct {
		name        string
		fake        *fakeCopyPackageClient
		wantCreated bool
		wantErr     string
		wantCalls   int
	}{
		{name: "existing", fake: &fakeCopyPackageClient{exists: true}},
		{name: "missing", fake: &fakeCopyPackageClient{}, wantCreated: true, wantCalls: 1},
		{name: "inconclusive lookup", fake: &fakeCopyPackageClient{existsErr: errors.New("synthetic authentication failure")}, wantErr: "authentication failure"},
		{name: "create failure", fake: &fakeCopyPackageClient{createErr: errors.New("synthetic create failure")}, wantErr: "create failure", wantCalls: 1},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			created, err := ensureCopyTargetPackage(context.Background(), tt.fake, "$SYNTHETIC", "synthetic.zip")
			if created != tt.wantCreated {
				t.Fatalf("created = %v, want %v", created, tt.wantCreated)
			}
			if tt.wantErr == "" && err != nil {
				t.Fatalf("ensureCopyTargetPackage() error = %v", err)
			}
			if tt.wantErr != "" && (err == nil || !strings.Contains(err.Error(), tt.wantErr)) {
				t.Fatalf("ensureCopyTargetPackage() error = %v, want substring %q", err, tt.wantErr)
			}
			if tt.fake.createCalls != tt.wantCalls {
				t.Fatalf("CreateObject calls = %d, want %d", tt.fake.createCalls, tt.wantCalls)
			}
		})
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
		{name: "class main", object: deps.DeploymentObject{Type: "CLAS"}, want: true}, //nolint:misspell // CLAS is the SAP object type.
		{
			name:       "class includes",
			object:     deps.DeploymentObject{Type: "CLAS", Includes: map[string]string{"testclasses": "synthetic"}}, //nolint:misspell // CLAS is the SAP object type.
			wantReason: "include deployment",
		},
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
