package adt

import (
	"context"
	"net/http"
	"strings"
	"testing"
)

func TestActivationResultError(t *testing.T) {
	tests := []struct {
		name    string
		result  *ActivationResult
		wantErr string
	}{
		{name: "success", result: &ActivationResult{Success: true}},
		{name: "nil result", wantErr: "no result"},
		{
			name: "message diagnostic",
			result: &ActivationResult{Messages: []ActivationResultMessage{
				{Type: "E", ShortText: "synthetic activation failure"},
			}},
			wantErr: "synthetic activation failure",
		},
		{
			name:    "inactive diagnostic",
			result:  &ActivationResult{Inactive: []InactiveObject{{Name: "ZSYNTHETIC"}}},
			wantErr: "1 object(s) remain inactive",
		},
		{name: "missing diagnostic", result: &ActivationResult{}, wantErr: "without a diagnostic"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ActivationResultError(tt.result)
			if tt.wantErr == "" {
				if err != nil {
					t.Fatalf("ActivationResultError() error = %v", err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("ActivationResultError() error = %v, want substring %q", err, tt.wantErr)
			}
		})
	}
}

func TestRenameUsesSourceEndpoint(t *testing.T) {
	client := &Client{}
	got, err := client.buildSourceURL(ObjectTypeProgram, "ZSYNTHETIC")
	if err != nil {
		t.Fatalf("buildSourceURL() error = %v", err)
	}
	if !strings.HasSuffix(got, "/source/main") {
		t.Fatalf("buildSourceURL() = %q, want source endpoint", got)
	}
}

//nolint:bodyclose // Transport.Request owns and closes every synthetic response body.
func TestActivatePackageClassifiesLogicalFailure(t *testing.T) {
	inactiveXML := `<inactiveObjects><entry><object user="SYNTHETIC"><ref uri="/sap/bc/adt/programs/programs/ZSYNTHETIC" type="PROG/P" name="ZSYNTHETIC" parentUri="/sap/bc/adt/packages/$TMP"/></object></entry></inactiveObjects>`
	activationXML := `<activation><messages><msg type="E"><shortText><txt>synthetic activation failure</txt></shortText></msg></messages></activation>`
	mock := &mockHTTPClient{responses: []*http.Response{
		newMockResponse(http.StatusOK, inactiveXML, nil),
		newMockResponse(http.StatusOK, activationXML, nil),
	}}
	cfg := NewConfig("https://sap.example.com:44300", "user", "pass")
	transport := NewTransportWithClient(cfg, mock)
	transport.setCSRFToken("synthetic-token")
	client := NewClientWithTransport(cfg, transport)

	result, err := client.ActivatePackage(context.Background(), "$TMP", 10)
	if err != nil {
		t.Fatalf("ActivatePackage() error = %v", err)
	}
	if len(result.Activated) != 0 {
		t.Fatalf("activated = %d, want 0", len(result.Activated))
	}
	if len(result.Failed) != 1 || !strings.Contains(result.Failed[0].Reason, "synthetic activation failure") {
		t.Fatalf("failed = %#v, want one logical activation failure", result.Failed)
	}
	if len(mock.requests) != 2 {
		t.Fatalf("requests = %d, want inactive lookup plus activation", len(mock.requests))
	}
}

//nolint:bodyclose // Transport.Request owns and closes every synthetic response body.
func TestRenameStopsBeforeDeletingOldObjectWhenActivationFails(t *testing.T) {
	lockXML := `<?xml version="1.0"?><asx:abap xmlns:asx="http://www.sap.com/abapxml"><asx:values><DATA><LOCK_HANDLE>SYNTHETIC-HANDLE</LOCK_HANDLE></DATA></asx:values></asx:abap>`
	activationXML := `<activation><messages><msg type="E"><shortText><txt>synthetic activation failure</txt></shortText></msg></messages></activation>`
	mock := &mockHTTPClient{responses: []*http.Response{
		newMockResponse(http.StatusOK, "REPORT zold.", nil), // Read old source.
		newMockResponse(http.StatusOK, "", nil),             // Verify package.
		newMockResponse(http.StatusCreated, "", nil),        // Create new object.
		newMockResponse(http.StatusOK, lockXML, nil),        // Lock new object.
		newMockResponse(http.StatusOK, "", nil),             // Write new source.
		newMockResponse(http.StatusOK, "", nil),             // Unlock new object.
		newMockResponse(http.StatusOK, activationXML, nil),  // Logical activation failure.
	}}
	cfg := NewConfig("https://sap.example.com:44300", "user", "pass")
	transport := NewTransportWithClient(cfg, mock)
	transport.setCSRFToken("synthetic-token")
	client := NewClientWithTransport(cfg, transport)

	result, err := client.RenameObject(context.Background(), ObjectTypeProgram, "ZOLD", "ZNEW", "$TMP", "")
	if err != nil {
		t.Fatalf("RenameObject() error = %v", err)
	}
	if result.Success {
		t.Fatal("RenameObject() success = true after logical activation failure")
	}
	if len(result.Errors) == 0 || !strings.Contains(result.Errors[0], "synthetic activation failure") {
		t.Fatalf("errors = %#v, want activation diagnostic", result.Errors)
	}
	if len(mock.requests) != 7 {
		t.Fatalf("requests = %d, want flow to stop immediately after activation", len(mock.requests))
	}
	if got := mock.requests[4].URL.Path; !strings.HasSuffix(got, "/znew/source/main") {
		t.Fatalf("write path = %q, want new object's source endpoint", got)
	}
	for _, req := range mock.requests {
		if strings.Contains(req.URL.Path, "/zold") && req.URL.Query().Get("_action") == "LOCK" {
			t.Fatalf("old object was locked for deletion after activation failure: %s", req.URL.String())
		}
	}
}
