package mcp

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/oisee/vibing-steampunk/pkg/adt"
)

// ExportToFile read only output_dir/outputPath. A caller that passed file_path
// — the name ImportFromFile uses for the same file — had it silently ignored,
// and the export landed in the bridge's working directory. file_path is now
// honoured when it names an export file, and refused with the expected name
// when it does not, instead of being dropped.
func TestExportOutputPath(t *testing.T) {
	dir := t.TempDir()
	good := filepath.Join(dir, "zdemo.prog.abap")

	cases := []struct {
		name    string
		args    map[string]any
		want    string
		wantErr string
	}{
		{"output_dir", map[string]any{"output_dir": dir}, dir, ""},
		{"outputPath wins", map[string]any{"outputPath": dir, "file_path": good}, dir, ""},
		{"file_path export name", map[string]any{"file_path": good}, good, ""},
		{"file_path wrong name", map[string]any{"file_path": filepath.Join(dir, "zdemo.txt")}, "", "zdemo.prog.abap"},
		{"nothing", map[string]any{}, "", ""},
	}
	for _, tc := range cases {
		got, err := exportOutputPath(tc.args, adt.ObjectTypeProgram, "ZDEMO", "")
		if tc.wantErr != "" {
			if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
				t.Errorf("%s: err = %v, want one naming %q", tc.name, err, tc.wantErr)
			}
			continue
		}
		if err != nil || got != tc.want {
			t.Errorf("%s: got %q, %v; want %q", tc.name, got, err, tc.want)
		}
	}
}
