package ctxcomp

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestMultiSourceProvider_LocalFile(t *testing.T) {
	// Create a temp directory with a test ABAP file
	dir := t.TempDir()
	content := "CLASS zcl_test DEFINITION PUBLIC.\n  PUBLIC SECTION.\nENDCLASS."
	if err := os.WriteFile(filepath.Join(dir, "zcl_test.clas.abap"), []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	provider := NewMultiSourceProvider(dir, nil)
	src, err := provider.GetSource(context.Background(), KindClass, "ZCL_TEST")
	if err != nil {
		t.Fatalf("GetSource error: %v", err)
	}
	if src != content {
		t.Errorf("got %q, want %q", src, content)
	}
}

func TestMultiSourceProvider_CachedContract(t *testing.T) {
	provider := NewMultiSourceProvider("", nil)
	provider.CacheContract(KindClass, "ZCL_CACHED", "cached contract source")

	src, err := provider.GetSource(context.Background(), KindClass, "ZCL_CACHED")
	if err != nil {
		t.Fatalf("GetSource error: %v", err)
	}
	if src != "cached contract source" {
		t.Errorf("got %q, want cached contract", src)
	}
}

func TestMultiSourceProvider_NotFound(t *testing.T) {
	provider := NewMultiSourceProvider("", nil) // no workspace, no ADT
	_, err := provider.GetSource(context.Background(), KindClass, "ZCL_NONEXISTENT")
	if err == nil {
		t.Error("expected error for missing source")
	}
}

func TestMultiSourceProvider_InterfaceLocal(t *testing.T) {
	dir := t.TempDir()
	content := "INTERFACE zif_test PUBLIC.\n  METHODS m1.\nENDINTERFACE."
	if err := os.WriteFile(filepath.Join(dir, "zif_test.intf.abap"), []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	provider := NewMultiSourceProvider(dir, nil)
	src, err := provider.GetSource(context.Background(), KindInterface, "ZIF_TEST")
	if err != nil {
		t.Fatalf("GetSource error: %v", err)
	}
	if src != content {
		t.Errorf("got %q, want %q", src, content)
	}
}
