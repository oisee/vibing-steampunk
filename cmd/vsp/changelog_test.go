package main

import (
	"testing"

	"github.com/oisee/vibing-steampunk/pkg/graph"
)

func TestAggregateChangelogEntries_CollapsesTasksToParentRequest(t *testing.T) {
	headers := map[string]graph.TransportHeader{
		"A4HK900100": {
			TRKORR:     "A4HK900100",
			TRFUNCTION: "K",
			TRSTATUS:   "R",
			AS4USER:    "DEV",
			AS4DATE:    "20260407",
			AS4TEXT:    "Main request",
		},
		"A4HK900101": {
			TRKORR:     "A4HK900101",
			STRKORR:    "A4HK900100",
			TRFUNCTION: "S",
			TRSTATUS:   "D",
			AS4USER:    "DEV",
			AS4DATE:    "20260407",
			AS4TEXT:    "Task",
		},
	}
	refs := []transportRef{
		{TRKORR: "A4HK900101", PGMID: "R3TR", Object: "CLAS", ObjName: "ZCL_FOO"},
		{TRKORR: "A4HK900100", PGMID: "R3TR", Object: "PROG", ObjName: "ZREPORT"},
	}

	entries := aggregateChangelogEntries(headers, refs, "")
	if len(entries) != 1 {
		t.Fatalf("expected 1 grouped entry, got %d", len(entries))
	}
	if entries[0].Transport != "A4HK900100" {
		t.Fatalf("transport = %q, want A4HK900100", entries[0].Transport)
	}
	if len(entries[0].Objects) != 2 {
		t.Fatalf("expected 2 objects, got %d", len(entries[0].Objects))
	}
}

func TestAggregateChangelogEntries_RespectsSinceFilter(t *testing.T) {
	headers := map[string]graph.TransportHeader{
		"A4HK900100": {
			TRKORR:     "A4HK900100",
			TRFUNCTION: "K",
			TRSTATUS:   "R",
			AS4USER:    "DEV",
			AS4DATE:    "20260401",
			AS4TEXT:    "Old request",
		},
		"A4HK900200": {
			TRKORR:     "A4HK900200",
			TRFUNCTION: "K",
			TRSTATUS:   "R",
			AS4USER:    "DEV",
			AS4DATE:    "20260407",
			AS4TEXT:    "New request",
		},
	}
	refs := []transportRef{
		{TRKORR: "A4HK900100", PGMID: "R3TR", Object: "CLAS", ObjName: "ZCL_OLD"},
		{TRKORR: "A4HK900200", PGMID: "R3TR", Object: "CLAS", ObjName: "ZCL_NEW"},
	}

	entries := aggregateChangelogEntries(headers, refs, "20260405")
	if len(entries) != 1 {
		t.Fatalf("expected 1 filtered entry, got %d", len(entries))
	}
	if entries[0].Transport != "A4HK900200" {
		t.Fatalf("transport = %q, want A4HK900200", entries[0].Transport)
	}
}
