package install

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/oisee/vibing-steampunk/pkg/adt"
)

type fakeClient struct {
	packageProbes []packageProbe
	probeIndex    int
	createErr     error
	createCalls   int
	writeResult   *adt.WriteSourceResult
	writeErr      error
	writeOpts     *adt.WriteSourceOptions
	readBack      string
	readBackErr   error
}

type packageProbe struct {
	exists bool
	err    error
}

func (f *fakeClient) PackageExists(context.Context, string) (bool, error) {
	if f.probeIndex >= len(f.packageProbes) {
		return false, errors.New("unexpected package probe")
	}
	probe := f.packageProbes[f.probeIndex]
	f.probeIndex++
	return probe.exists, probe.err
}

func (f *fakeClient) CreateObject(context.Context, adt.CreateObjectOptions) error {
	f.createCalls++
	return f.createErr
}

func (f *fakeClient) WriteSource(_ context.Context, _, _, _ string, opts *adt.WriteSourceOptions) (*adt.WriteSourceResult, error) {
	f.writeOpts = opts
	return f.writeResult, f.writeErr
}

func (f *fakeClient) GetSource(context.Context, string, string, *adt.GetSourceOptions) (string, error) {
	return f.readBack, f.readBackErr
}

func TestEnsurePackage(t *testing.T) {
	tests := []struct {
		name        string
		fake        *fakeClient
		wantCreated bool
		wantErr     bool
		wantCreates int
	}{
		{name: "existing", fake: &fakeClient{packageProbes: []packageProbe{{exists: true}}}},
		{name: "created and verified", fake: &fakeClient{packageProbes: []packageProbe{{}, {exists: true}}}, wantCreated: true, wantCreates: 1},
		{name: "concurrent create verified", fake: &fakeClient{packageProbes: []packageProbe{{}, {exists: true}}, createErr: errors.New("already exists")}, wantCreates: 1},
		{name: "initial probe inconclusive", fake: &fakeClient{packageProbes: []packageProbe{{err: errors.New("synthetic auth failure")}}}, wantErr: true},
		{name: "success but absent", fake: &fakeClient{packageProbes: []packageProbe{{}, {}}}, wantErr: true, wantCreates: 1},
		{name: "create failed and absent", fake: &fakeClient{packageProbes: []packageProbe{{}, {}}, createErr: errors.New("synthetic create failure")}, wantErr: true, wantCreates: 1},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			created, err := EnsurePackage(context.Background(), tt.fake, "$SYNTHETIC", "Synthetic package")
			if (err != nil) != tt.wantErr {
				t.Fatalf("error = %v, wantErr=%t", err, tt.wantErr)
			}
			if created != tt.wantCreated {
				t.Fatalf("created = %t, want %t", created, tt.wantCreated)
			}
			if tt.fake.createCalls != tt.wantCreates {
				t.Fatalf("create calls = %d, want %d", tt.fake.createCalls, tt.wantCreates)
			}
		})
	}
}

func TestDeploySource(t *testing.T) {
	tests := []struct {
		name    string
		fake    *fakeClient
		wantErr string
	}{
		{name: "verified", fake: &fakeClient{writeResult: &adt.WriteSourceResult{Success: true, Activation: &adt.ActivationResult{Success: true}}, readBack: "synthetic source"}},
		{name: "transport error", fake: &fakeClient{writeErr: errors.New("synthetic request failure")}, wantErr: "synthetic request failure"},
		{name: "nil result", fake: &fakeClient{}, wantErr: "no result"},
		{name: "logical failure", fake: &fakeClient{writeResult: &adt.WriteSourceResult{Message: "synthetic workflow failure"}}, wantErr: "synthetic workflow failure"},
		{name: "syntax failure", fake: &fakeClient{writeResult: &adt.WriteSourceResult{Success: true, SyntaxErrors: []adt.SyntaxCheckResult{{Severity: "E"}}}}, wantErr: "syntax error"},
		{name: "activation failure", fake: &fakeClient{writeResult: &adt.WriteSourceResult{Success: true, Activation: &adt.ActivationResult{Messages: []adt.ActivationResultMessage{{Type: "E", ShortText: "synthetic activation failure"}}}}}, wantErr: "activation failure"},
		{name: "read-back failure", fake: &fakeClient{writeResult: &adt.WriteSourceResult{Success: true}, readBackErr: errors.New("synthetic read failure")}, wantErr: "read-back failed"},
		{name: "empty read-back", fake: &fakeClient{writeResult: &adt.WriteSourceResult{Success: true}, readBack: "  "}, wantErr: "read-back was empty"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			opts := &adt.WriteSourceOptions{Description: "Synthetic object"}
			_, err := DeploySource(context.Background(), tt.fake, "CLAS", "ZCL_SYNTHETIC", "synthetic source", opts)
			if tt.wantErr == "" && err != nil {
				t.Fatal(err)
			}
			if tt.wantErr != "" && (err == nil || !strings.Contains(err.Error(), tt.wantErr)) {
				t.Fatalf("error = %v, want substring %q", err, tt.wantErr)
			}
			if tt.fake.writeOpts != opts {
				t.Fatal("write options were not forwarded")
			}
		})
	}
}
