package itf

import (
	"strings"
	"testing"
)

func TestToMarkdown(t *testing.T) {
	// An IMG activity's text as DOKTL holds it: chapter markers, paragraphs
	// continued on unformatted lines, <ex> code, a numbered list, and empty
	// chapters.
	lines := []Line{
		{"UT", "&USE&"},
		{"AS", "This activity deletes the cleanup jobs for"},
		{"", "delta links."},
		{"AS", "It deletes all jobs that use the report"},
		{"", "<ex>/IWBEP/R_DELTA_LINK_CLEANUP</> as a job step."},
		{"UT", "&PRECONDITIONS&"},
		{"AS", "You need <zh>S_BTCH_JOB</> with these values:"},
		{"N1", "Job group (JOBGROUP): <ex>*</>"},
		{"N1", "Job action (JOBACTION): <ex>DELE</>"},
		{"AS", ""},
		{"UT", "&STANDARD_SETUP&"},
		{"AS", ""},
	}
	got := ToMarkdown(lines, nil)
	want := "## Use\n\nThis activity deletes the cleanup jobs for delta links.\n\nIt deletes all jobs that use the report `/IWBEP/R_DELTA_LINK_CLEANUP` as a job step.\n\n## Preconditions\n\nYou need **S_BTCH_JOB** with these values:\n\n1. Job group (JOBGROUP): `*`\n2. Job action (JOBACTION): `DELE`\n## Standard Setup\n"
	if got != want {
		t.Errorf("got:\n%s\nwant:\n%s", got, want)
	}
}

func TestIncludesAndLinks(t *testing.T) {
	lines := []Line{
		{"U1", "&FUNCTIONALITY&"},
		{"/:", "INCLUDE BAL_FU_LOG_CREATE OBJECT DOKU ID TX."},
		{"U1", "Related function modules"},
		{"AS", "<DS:TX.BAL_CH_SIMPLE>Simple log creation call</>"},
		{"/", "<DS:TX.BAL_CH_COLLECT>Methods of collecting messages</>"},
		{"/:", "INCLUDE BAL_FU_LOG_CREATE OBJECT DOKU ID TX"},
	}
	include := func(object, id string) ([]Line, error) {
		if object == "BAL_FU_LOG_CREATE" && id == "TX" {
			return []Line{{"AS", "Creates a log with the header <ex>I_S_LOG</>."}}, nil
		}
		return nil, nil
	}
	got := ToMarkdown(lines, include)
	for _, want := range []string{
		"## Functionality\n\nCreates a log with the header `I_S_LOG`.",
		"## Related function modules\n",
		"Simple log creation call (TX BAL_CH_SIMPLE)  \nMethods of collecting messages (TX BAL_CH_COLLECT)",
		"_[included text TX BAL_FU_LOG_CREATE]_", // the second time: seen, not repeated
	} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q in:\n%s", want, got)
		}
	}
	if strings.Count(got, "Creates a log") != 1 {
		t.Errorf("include rendered %d times", strings.Count(got, "Creates a log"))
	}
	// A character format that spans two DOKTL lines closes on the second.
	spanned := ToMarkdown([]Line{{"AS", "See <DS:TX.BAL_FUNCTION_GROUPS>the most important function"}, {"", "groups</> here."}}, nil)
	if spanned != "See the most important function groups (TX BAL_FUNCTION_GROUPS) here.\n" {
		t.Errorf("spanned tag: %q", spanned)
	}
	if ToMarkdown(lines[:2], nil) != "## Functionality\n\n_[included text TX BAL_FU_LOG_CREATE]_\n" {
		t.Errorf("without an includer: %q", ToMarkdown(lines[:2], nil))
	}
}

func TestInline(t *testing.T) {
	for in, want := range map[string]string{
		"a <ex>code</> b":           "a `code` b",
		"<zh>bold</> and <zk>it</>": "**bold** and *it*",
		"x <(>tag<)> y":             "x <tag> y",
		"<DS:DE.BALLEVEL>level</>":  "level (DE BALLEVEL)",
		"unclosed <zh>bold":         "unclosed **bold**",
		"plain & text":              "plain & text",
	} {
		if got := inline(in); got != want {
			t.Errorf("%q: got %q, want %q", in, got, want)
		}
	}
	if chapter("&FURTHER_HINTS&") != "Further Hints" || chapter("not a marker") != "" {
		t.Error("chapter markers")
	}
}
