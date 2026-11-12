package hanadump

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestClosureRequestSkipsSeenAndSelectsSources(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "closure-request.json")
	os.WriteFile(path, []byte(`{"schema":1,"iteration":2,"requested":{"TABL":["ZTAB"," ZSEEN "],"CLAS":["ZCL_A"],"FUGR":["/NS/FG"],"DTEL":[""]},"seen":{"TABL":["ZSEEN"]},"gaps":[]}`), 0o600)
	r, err := ReadClosureRequest(path)
	if err != nil {
		t.Fatal(err)
	}
	if got := r.newNames("TABL"); !reflect.DeepEqual(got, []string{"ZTAB"}) {
		t.Errorf("seen names are skipped, blanks dropped: %q", got)
	}
	if got := r.newNames("DTEL"); len(got) != 0 {
		t.Errorf("empty names dropped: %q", got)
	}
	want := []string{`ZCL\_A`, `ZCL\_A=========================%`, `/NS/FG`, `/NS/SAPLFG`, `/NS/LFG%`}
	if got := r.SourcePatternsOf(); !reflect.DeepEqual(got, want) {
		t.Errorf("source kinds as names: got %q, want %q", got, want)
	}
	os.WriteFile(path, []byte(`{"schema":2}`), 0o600)
	if _, err := ReadClosureRequest(path); err == nil || !strings.Contains(err.Error(), "schema 2") {
		t.Errorf("another schema accepted: %v", err)
	}
}

func TestInChunks(t *testing.T) {
	values := make([]string, 1001)
	for i := range values {
		values[i] = "V"
	}
	var sizes []int
	inChunks(values, func(ph string, args []any) error {
		if strings.Count(ph, "?") != len(args) {
			t.Errorf("%d placeholders for %d args", strings.Count(ph, "?"), len(args))
		}
		sizes = append(sizes, len(args))
		return nil
	})
	if !reflect.DeepEqual(sizes, []int{500, 500, 1}) {
		t.Errorf("chunks %v", sizes)
	}
}
