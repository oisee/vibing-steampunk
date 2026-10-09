package landscape

import (
	"context"
	"encoding/xml"
	"errors"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadDeclaredFileURIInclude(t *testing.T) {
	dir := t.TempDir()
	child := filepath.Join(dir, "shared fixture.xml")
	if err := os.WriteFile(child, []byte(directFixture), 0600); err != nil {
		t.Fatal(err)
	}
	uriPath := filepath.ToSlash(child)
	if len(uriPath) > 1 && uriPath[1] == ':' {
		uriPath = "/" + uriPath
	}
	u := (&url.URL{Scheme: "file", Path: uriPath}).String()
	var escaped strings.Builder
	if err := xml.EscapeText(&escaped, []byte(u)); err != nil {
		t.Fatal(err)
	}
	root := filepath.Join(dir, "root.xml")
	if err := os.WriteFile(root, []byte(`<Landscape><Includes><Include url="`+escaped.String()+`" index="0"/></Includes></Landscape>`), 0600); err != nil {
		t.Fatal(err)
	}
	l, err := Load(context.Background(), []string{root})
	if err != nil {
		t.Fatal("declared file URI include failed")
	}
	if _, err := l.Resolve("TST", "123", "Test", "fixture.dll"); err != nil {
		t.Fatal(err)
	}
}

func TestMissingLandscapeHasSafeDiagnostic(t *testing.T) {
	_, err := Load(context.Background(), []string{filepath.Join(t.TempDir(), "secret-host-missing.xml")})
	var diagnostic *LoadError
	if !errors.As(err, &diagnostic) || diagnostic.Code != "file_not_found" || diagnostic.Include || strings.Contains(err.Error(), "secret-host") {
		t.Fatal("missing root cannot be distinguished safely")
	}
}

func TestIncludeFailureHasSafeDiagnostic(t *testing.T) {
	dir := t.TempDir()
	root := filepath.Join(dir, "root.xml")
	if err := os.WriteFile(root, []byte(`<Landscape><Includes><Include url="secret-host-missing.xml" index="0"/></Includes></Landscape>`), 0600); err != nil {
		t.Fatal(err)
	}
	_, err := Load(context.Background(), []string{root})
	var diagnostic *LoadError
	if !errors.As(err, &diagnostic) || diagnostic.Code != "file_not_found" || !diagnostic.Include || strings.Contains(err.Error(), "secret-host") {
		t.Fatal("include failure cannot be distinguished safely")
	}
}

func TestIncludeCycleStopsWithoutFallback(t *testing.T) {
	dir := t.TempDir()
	root := filepath.Join(dir, "root.xml")
	if err := os.WriteFile(root, []byte(`<Landscape><Includes><Include url="root.xml" index="0"/></Includes></Landscape>`), 0600); err != nil {
		t.Fatal(err)
	}
	_, err := Load(context.Background(), []string{root})
	var diagnostic *LoadError
	if !errors.As(err, &diagnostic) || diagnostic.Code != "include_cycle" {
		t.Fatal("include cycle was not bounded")
	}
}
