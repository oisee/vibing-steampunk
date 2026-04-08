package adt

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSaveCookiesToFile(t *testing.T) {
	cookies := map[string]string{
		"MYSAPSSO2":              "abc123",
		"sap-usercontext":        "sap-client=001",
		"SAP_SESSIONID_NPL_001": "session456",
	}

	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "cookies.txt")

	err := SaveCookiesToFile(cookies, "https://sap.example.com:44300", path)
	if err != nil {
		t.Fatalf("SaveCookiesToFile failed: %v", err)
	}

	// Verify the file was created with restrictive permissions
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("cannot stat cookie file: %v", err)
	}
	if info.Mode().Perm() != 0600 {
		t.Errorf("expected permissions 0600, got %o", info.Mode().Perm())
	}

	// Verify roundtrip: saved cookies can be loaded back
	loaded, err := LoadCookiesFromFile(path)
	if err != nil {
		t.Fatalf("LoadCookiesFromFile failed: %v", err)
	}
	if len(loaded) != len(cookies) {
		t.Errorf("expected %d cookies, got %d", len(cookies), len(loaded))
	}
	for name, expected := range cookies {
		if loaded[name] != expected {
			t.Errorf("cookie %s: expected %q, got %q", name, expected, loaded[name])
		}
	}
}

func TestSaveCookiesToFile_HTTPS(t *testing.T) {
	cookies := map[string]string{"test": "value"}
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "cookies.txt")

	err := SaveCookiesToFile(cookies, "https://sap.example.com:44300", path)
	if err != nil {
		t.Fatalf("SaveCookiesToFile failed: %v", err)
	}

	data, _ := os.ReadFile(path)
	content := string(data)
	if !strings.Contains(content, "TRUE\t/\tTRUE") {
		t.Error("expected secure flag TRUE for https URL")
	}
}

func TestSaveCookiesToFile_HTTP(t *testing.T) {
	cookies := map[string]string{"test": "value"}
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "cookies.txt")

	err := SaveCookiesToFile(cookies, "http://sap.example.com:8000", path)
	if err != nil {
		t.Fatalf("SaveCookiesToFile failed: %v", err)
	}

	data, _ := os.ReadFile(path)
	content := string(data)
	if !strings.Contains(content, "TRUE\t/\tFALSE") {
		t.Error("expected secure flag FALSE for http URL")
	}
}

func TestSaveCookiesToFile_InvalidURL(t *testing.T) {
	err := SaveCookiesToFile(map[string]string{"a": "b"}, "://bad", "/tmp/vsp_test_invalid.txt")
	if err == nil {
		t.Error("expected error for invalid URL")
	}
}

func TestBrowserLogin_InvalidURL(t *testing.T) {
	_, err := BrowserLogin(nil, "", false, 0, "", false)
	if err == nil {
		t.Error("expected error for empty URL")
	}

	_, err = BrowserLogin(nil, "not-a-url", false, 0, "", false)
	if err == nil {
		t.Error("expected error for invalid URL")
	}
}
