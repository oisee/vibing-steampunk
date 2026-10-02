package saprfc

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/oisee/open-rfc-go/rfc"
)

// fakeExports stands in for rfc.Result.
type fakeExports struct {
	tables map[string][]map[string]any
}

func (f fakeExports) Get(string) any                     { return nil }
func (f fakeExports) Table(name string) []map[string]any { return f.tables[name] }

// fakeTPALOG answers RFC_READ_TABLE on TPALOG with rows given as
// "TRKORR|TRCLI|TRSTEP|RETCODE|TRTIME", and records what it was asked.
type fakeTPALOG struct {
	rows  []string
	calls []string
	in    rfc.Params
}

func (f *fakeTPALOG) call(_ context.Context, fm string, in rfc.Params) (exports, error) {
	f.calls = append(f.calls, fm)
	if fm != "RFC_READ_TABLE" {
		return nil, errors.New("unexpected " + fm)
	}
	f.in = in
	var data []map[string]any
	for _, r := range f.rows {
		data = append(data, map[string]any{"WA": r})
	}
	var fields []map[string]any
	for _, fl := range in["FIELDS"].([]map[string]any) {
		fields = append(fields, map[string]any{"FIELDNAME": fl["FIELDNAME"]})
	}
	return fakeExports{tables: map[string][]map[string]any{"DATA": data, "FIELDS": fields}}, nil
}

func TestImportLogs_StepsByRequestOldestFirst(t *testing.T) {
	f := &fakeTPALOG{rows: []string{
		"XYZK900001|100|I|0004|20260102100005",
		"XYZK900001|ALL|L|0000|20260102100000",
		"XYZK900001|ALL|G|0000|20260102100009",
	}}
	logs, err := importLogs(context.Background(), f.call, []string{"xyzk900001", "XYZK900002"}, "")
	if err != nil {
		t.Fatalf("importLogs: %v", err)
	}
	if f.in["QUERY_TABLE"] != "TPALOG" || strings.Join(f.calls, ",") != "RFC_READ_TABLE" {
		t.Fatalf("read %v with %v", f.in["QUERY_TABLE"], f.calls)
	}
	if len(logs) != 2 || logs[0].Request != "XYZK900001" || logs[1].Request != "XYZK900002" {
		t.Fatalf("logs = %+v", logs)
	}
	steps := make([]string, 0, len(logs[0].Steps))
	for _, st := range logs[0].Steps {
		steps = append(steps, st.Step)
	}
	if strings.Join(steps, "") != "LIG" || logs[0].MaxRC != "0004" {
		t.Errorf("XYZK900001 = %+v, want steps L, I, G and rc 0004", logs[0])
	}
	// Not imported here: an empty list, not a missing one.
	if logs[1].Steps == nil || len(logs[1].Steps) != 0 || logs[1].MaxRC != "" {
		t.Errorf("XYZK900002 = %+v, want no steps", logs[1])
	}
}

func TestImportLogs_StepsSinceAGivenTime(t *testing.T) {
	f := &fakeTPALOG{rows: []string{
		"XYZK900001|100|I|0000|20260101100000",
		"XYZK900001|ALL|L|0000|20260102100000",
		"XYZK900001|100|I|0008|20260102100005",
	}}
	for _, since := range []string{"20260102000000", "20260102"} {
		logs, err := importLogs(context.Background(), f.call, []string{"XYZK900001"}, since)
		if err != nil {
			t.Fatalf("importLogs since %s: %v", since, err)
		}
		if len(logs[0].Steps) != 2 || logs[0].MaxRC != "0008" {
			t.Errorf("since %s: %+v, want the two steps of the 2nd and rc 0008", since, logs[0])
		}
	}
}

func TestImportLogs_RefusesBeforeReading(t *testing.T) {
	for name, tc := range map[string]struct {
		reqs  []string
		since string
	}{
		"no request":  {nil, ""},
		"bad request": {[]string{"X' OR '1'='1"}, ""},
		"bad since":   {[]string{"XYZK900001"}, "yesterday"},
		"9 digits":    {[]string{"XYZK900001"}, "202601021"},
		"13 digits":   {[]string{"XYZK900001"}, "2026010210000"},
	} {
		f := &fakeTPALOG{}
		if _, err := importLogs(context.Background(), f.call, tc.reqs, tc.since); err == nil {
			t.Errorf("%s: accepted", name)
		}
		if len(f.calls) != 0 {
			t.Errorf("%s: read before refusing", name)
		}
	}
}

// The request list is split over RFC_READ_TABLE's 72-character OPTIONS
// lines, which break only between tokens: eight requests must still fit.
func TestImportLogs_ManyRequestsFitTheOptionsLines(t *testing.T) {
	var reqs []string
	for i := 1; i <= 8; i++ {
		reqs = append(reqs, "XYZK90000"+string(rune('0'+i)))
	}
	f := &fakeTPALOG{}
	if _, err := importLogs(context.Background(), f.call, reqs, ""); err != nil {
		t.Fatalf("importLogs with %d requests: %v", len(reqs), err)
	}
	opts, _ := f.in["OPTIONS"].([]map[string]any)
	if len(opts) < 2 {
		t.Fatalf("OPTIONS = %v, want the list split over several lines", opts)
	}
	var lines []string
	for _, o := range opts {
		if l := o["TEXT"].(string); len(l) > optionsLineLen {
			t.Errorf("OPTIONS line %q is longer than %d", l, optionsLineLen)
		} else {
			lines = append(lines, l)
		}
	}
	if joined := strings.Join(lines, " "); !strings.Contains(joined, "'XYZK900008'") {
		t.Errorf("OPTIONS %q lost a request", joined)
	}
}

// TARSYSTEM is read and returned per step, and a request named twice is
// read and answered once.
func TestImportLogs_TargetPerStepAndNoDuplicateRequests(t *testing.T) {
	f := &fakeTPALOG{rows: []string{
		"XYZK900001|100|I|0000|20260102100005|XYZ",
		"XYZK900001|ALL|E|0000|20260101100000|XYZ.100",
	}}
	logs, err := importLogs(context.Background(), f.call, []string{"XYZK900001", "xyzk900001 "}, "")
	if err != nil {
		t.Fatalf("importLogs: %v", err)
	}
	var fields []string
	for _, fl := range f.in["FIELDS"].([]map[string]any) {
		fields = append(fields, fmt.Sprint(fl["FIELDNAME"]))
	}
	if !strings.Contains(strings.Join(fields, ","), "TARSYSTEM") {
		t.Errorf("FIELDS = %v, want TARSYSTEM", fields)
	}
	if len(logs) != 1 {
		t.Fatalf("logs = %+v, want one entry for the twice-named request", logs)
	}
	if len(logs[0].Steps) != 2 || logs[0].Steps[0].Target != "XYZ.100" || logs[0].Steps[1].Target != "XYZ" {
		t.Errorf("steps = %+v, want targets XYZ.100 then XYZ", logs[0].Steps)
	}
}
