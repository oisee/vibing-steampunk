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

// TestDescriptionsFitSAP pins every description to the 60 characters SAP
// allows for an object's short text. One of 61 made SAP refuse to create
// ZCL_VSP_GIT_SERVICE, and with it ZVSP_GIT_IMPORT, which uses it.
func TestDescriptionsFitSAP(t *testing.T) {
	for _, obj := range GetObjects() {
		if n := len([]rune(obj.Description)); n > 60 {
			t.Errorf("%s: description is %d characters, SAP allows 60: %q", obj.Name, n, obj.Description)
		}
	}
}
