package vsp

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/oisee/vibing-steampunk/internal/fakesap"
	"github.com/oisee/vibing-steampunk/pkg/adt"
)

// The JSON answer carried only the readers, and a reader whose source could
// not be read sat among them as "unconfirmed" — the word for "read, and the
// variable is not in it". The warning went to stderr, which a JSON consumer
// does not read. The gaps now travel in the document, as they do over MCP.
func TestWhereUsedConfigJSONCarriesItsGaps(t *testing.T) {
	srv := fakesap.New(t, fakesap.CrossDown(false))
	examplesAgainst(t, srv.Server)
	withFlags(t, graphWhereUsedConfigCmd, map[string]string{"format": "json"})
	graphWhereUsedConfigCmd.SetContext(context.Background())

	var err error
	stdout := captureStdout(t, func() {
		_ = captureStderr(t, func() { err = runGraphWhereUsedConfig(graphWhereUsedConfigCmd, []string{"ZGOLD_VAR"}) })
	})
	if err != nil {
		t.Fatal(err)
	}
	var got struct {
		Readers    []json.RawMessage `json:"readers"`
		Unsearched []adt.Unsearched  `json:"unsearched"`
		Notes      []string          `json:"notes"`
	}
	if err := json.Unmarshal([]byte(stdout), &got); err != nil {
		t.Fatalf("%v\n%s", err, stdout)
	}
	var objects []string
	for _, u := range got.Unsearched {
		if u.Reason == "" {
			t.Errorf("gap %s has no reason", u.Object)
		}
		objects = append(objects, u.Object)
	}
	want := "WBCROSSGT (object-oriented code),PROG ZGOLD_MISSING"
	if strings.Join(objects, ",") != want {
		t.Fatalf("unsearched = %q, want %q", objects, want)
	}
	if len(got.Notes) != 1 || !strings.HasPrefix(got.Notes[0], "2 of 3 objects could not be searched") {
		t.Fatalf("notes = %q", got.Notes)
	}
}

// With WBCROSSGT refused and CROSS finding nothing there are no candidates,
// and the JSON format printed a plain-text line instead of the document —
// dropping the one fact that mattered, that half the system was not asked.
func TestWhereUsedConfigJSONWithNoCandidatesStillCarriesItsGaps(t *testing.T) {
	w := fakesap.CrossDown(false)
	w.TVARVCReaders[1] = nil
	srv := fakesap.New(t, w)
	examplesAgainst(t, srv.Server)
	withFlags(t, graphWhereUsedConfigCmd, map[string]string{"format": "json"})
	graphWhereUsedConfigCmd.SetContext(context.Background())

	var err error
	stdout := captureStdout(t, func() {
		_ = captureStderr(t, func() { err = runGraphWhereUsedConfig(graphWhereUsedConfigCmd, []string{"ZGOLD_VAR"}) })
	})
	if err != nil {
		t.Fatal(err)
	}
	var got struct {
		Found      bool             `json:"found"`
		Unsearched []adt.Unsearched `json:"unsearched"`
		Notes      []string         `json:"notes"`
	}
	if err := json.Unmarshal([]byte(stdout), &got); err != nil {
		t.Fatalf("not JSON: %v\n%s", err, stdout)
	}
	if len(got.Unsearched) != 1 || got.Unsearched[0].Object != "WBCROSSGT (object-oriented code)" {
		t.Fatalf("unsearched = %+v, want the WBCROSSGT gap", got.Unsearched)
	}
	if len(got.Notes) != 1 || !strings.HasPrefix(got.Notes[0], "1 of 1 objects could not be searched") {
		t.Fatalf("notes = %q", got.Notes)
	}
}
