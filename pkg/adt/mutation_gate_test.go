package adt

import (
	"context"
	"net/http"
	"strings"
	"testing"
)

func TestCheckMutation_NoPolicy_Passes(t *testing.T) {
	cfg := NewConfig("https://sap.example.com:44300", "user", "pass")
	client := NewClientWithTransport(cfg, NewTransportWithClient(cfg, &mockTransportClient{
		responses: map[string]*http.Response{"discovery": newTestResponse("OK")},
	}))

	err := client.checkMutation(context.Background(), MutationContext{
		Op:        OpUpdate,
		OpName:    "TestOp",
		ObjectURL: "/sap/bc/adt/programs/programs/ZTEST",
	})
	if err != nil {
		t.Fatalf("expected no error when no policy configured, got: %v", err)
	}
}

func TestCheckMutation_OpType_Blocked(t *testing.T) {
	cfg := NewConfig("https://sap.example.com:44300", "user", "pass", WithReadOnly())
	client := NewClientWithTransport(cfg, NewTransportWithClient(cfg, &mockTransportClient{
		responses: map[string]*http.Response{"discovery": newTestResponse("OK")},
	}))

	err := client.checkMutation(context.Background(), MutationContext{
		Op:     OpUpdate,
		OpName: "TestOp",
	})
	if err == nil {
		t.Fatal("expected read-only mode to block OpUpdate")
	}
}

func TestCheckMutation_ExplicitPackage_NotInWhitelist(t *testing.T) {
	cfg := NewConfig("https://sap.example.com:44300", "user", "pass", WithAllowedPackages("$TMP"))
	client := NewClientWithTransport(cfg, NewTransportWithClient(cfg, &mockTransportClient{
		responses: map[string]*http.Response{"discovery": newTestResponse("OK")},
	}))

	err := client.checkMutation(context.Background(), MutationContext{
		Op:      OpCreate,
		OpName:  "CreateObject",
		Package: "ZOTHER",
	})
	if err == nil {
		t.Fatal("expected explicit package outside whitelist to be blocked")
	}
	if !strings.Contains(err.Error(), "ZOTHER") {
		t.Fatalf("expected error to mention blocked package, got: %v", err)
	}
}

func TestCheckMutation_ObjectURL_ResolvesADTPackage(t *testing.T) {
	mock := &mockTransportClient{
		responses: map[string]*http.Response{
			"search":    newSearchResponse("/sap/bc/adt/programs/programs/ztest", "PROG/P", "ZTEST", "ZOTHER"),
			"discovery": newTestResponse("OK"),
		},
	}
	cfg := NewConfig("https://sap.example.com:44300", "user", "pass", WithAllowedPackages("$TMP"))
	client := NewClientWithTransport(cfg, NewTransportWithClient(cfg, mock))

	err := client.checkMutation(context.Background(), MutationContext{
		Op:        OpUpdate,
		OpName:    "UpdateSource",
		ObjectURL: "/sap/bc/adt/programs/programs/ZTEST/source/main",
	})
	if err == nil {
		t.Fatal("expected object URL resolution to block non-whitelisted package")
	}
	if !strings.Contains(err.Error(), "ZOTHER") {
		t.Fatalf("expected error to mention resolved package, got: %v", err)
	}
}

func TestCheckMutation_UI5Surface_BlockedWhenPolicyActive(t *testing.T) {
	cfg := NewConfig("https://sap.example.com:44300", "user", "pass", WithAllowedPackages("$TMP"))
	client := NewClientWithTransport(cfg, NewTransportWithClient(cfg, &mockTransportClient{
		responses: map[string]*http.Response{"discovery": newTestResponse("OK")},
	}))

	err := client.checkMutation(context.Background(), MutationContext{
		Op:        OpUpdate,
		OpName:    "UI5UploadFile",
		ObjectURL: "MYAPP",
		Surface:   SurfaceUI5,
	})
	if err == nil {
		t.Fatal("expected UI5 surface to be blocked until app→package resolution lands")
	}
	if !strings.Contains(err.Error(), "UI5") {
		t.Fatalf("expected error to mention UI5, got: %v", err)
	}
}

func TestCheckMutation_UI5Surface_AllowedWhenNoPolicy(t *testing.T) {
	cfg := NewConfig("https://sap.example.com:44300", "user", "pass")
	client := NewClientWithTransport(cfg, NewTransportWithClient(cfg, &mockTransportClient{
		responses: map[string]*http.Response{"discovery": newTestResponse("OK")},
	}))

	err := client.checkMutation(context.Background(), MutationContext{
		Op:        OpUpdate,
		OpName:    "UI5UploadFile",
		ObjectURL: "MYAPP",
		Surface:   SurfaceUI5,
	})
	if err != nil {
		t.Fatalf("expected UI5 surface to pass when no package policy, got: %v", err)
	}
}

func TestCheckMutation_MissingObjectURLAndPackage_FailsClosed(t *testing.T) {
	cfg := NewConfig("https://sap.example.com:44300", "user", "pass", WithAllowedPackages("$TMP"))
	client := NewClientWithTransport(cfg, NewTransportWithClient(cfg, &mockTransportClient{
		responses: map[string]*http.Response{"discovery": newTestResponse("OK")},
	}))

	err := client.checkMutation(context.Background(), MutationContext{
		Op:     OpUpdate,
		OpName: "MysteryOp",
	})
	if err == nil {
		t.Fatal("expected gate to fail closed when neither ObjectURL nor Package is provided under policy")
	}
	if !strings.Contains(err.Error(), "MysteryOp") {
		t.Fatalf("expected error to mention op name, got: %v", err)
	}
}

func TestClient_UI5UploadFile_BlockedUnderAllowedPackages(t *testing.T) {
	cfg := NewConfig("https://sap.example.com:44300", "user", "pass", WithAllowedPackages("$TMP"))
	client := NewClientWithTransport(cfg, NewTransportWithClient(cfg, &mockTransportClient{
		responses: map[string]*http.Response{"discovery": newTestResponse("OK")},
	}))

	err := client.UI5UploadFile(context.Background(), "MYAPP", "/index.html", []byte("x"), "text/html")
	if err == nil {
		t.Fatal("expected UI5UploadFile to be blocked under AllowedPackages policy")
	}
}

func TestClient_UI5DeleteFile_BlockedUnderAllowedPackages(t *testing.T) {
	cfg := NewConfig("https://sap.example.com:44300", "user", "pass", WithAllowedPackages("$TMP"))
	client := NewClientWithTransport(cfg, NewTransportWithClient(cfg, &mockTransportClient{
		responses: map[string]*http.Response{"discovery": newTestResponse("OK")},
	}))

	err := client.UI5DeleteFile(context.Background(), "MYAPP", "/index.html")
	if err == nil {
		t.Fatal("expected UI5DeleteFile to be blocked under AllowedPackages policy")
	}
}

func TestClient_UI5DeleteApp_BlockedUnderAllowedPackages(t *testing.T) {
	cfg := NewConfig("https://sap.example.com:44300", "user", "pass", WithAllowedPackages("$TMP"))
	client := NewClientWithTransport(cfg, NewTransportWithClient(cfg, &mockTransportClient{
		responses: map[string]*http.Response{"discovery": newTestResponse("OK")},
	}))

	err := client.UI5DeleteApp(context.Background(), "MYAPP", "")
	if err == nil {
		t.Fatal("expected UI5DeleteApp to be blocked under AllowedPackages policy")
	}
}
