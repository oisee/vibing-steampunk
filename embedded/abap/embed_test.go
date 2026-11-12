package embedded

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestEmbeddedSourcesMatchSrc pins what vsp install deploys to what the
// repository holds. The copies here once went seven months without a sync:
// install kept deploying an RFC service without the JSON parameter bridge
// (#206) and a helper class under a name src/ no longer used, while every
// fix landed in src/ alone.
func TestEmbeddedSourcesMatchSrc(t *testing.T) {
	normalize := func(s string) string { return strings.ReplaceAll(s, "\r\n", "\n") }

	for _, obj := range GetObjects() {
		name := FileName(obj)
		want, err := os.ReadFile(filepath.Join("..", "..", "src", name))
		if err != nil {
			t.Errorf("%s: no counterpart in src/: %v", obj.Name, err)
			continue
		}
		if normalize(obj.Source) != normalize(string(want)) {
			t.Errorf("%s: embedded/abap/%s differs from src/%s, so vsp install would deploy "+
				"a different class than the repository holds. Edit src/, then run: "+
				"go generate ./embedded/abap", obj.Name, name, name)
		}
	}
}

// SAP refuses a class or program whose short text is longer than 60
// characters; an install then fails half way through.
func TestEmbeddedDescriptionsFitSAP(t *testing.T) {
	for _, o := range GetObjects() {
		if n := len([]rune(o.Description)); n > 60 {
			t.Errorf("%s %s: description is %d characters, SAP takes 60", o.Type, o.Name, n)
		}
	}
}
