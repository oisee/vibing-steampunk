package mcp

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"
)

// tools_parity_test.go pins the tool counts against the server and tells the
// reader, in a comment, to "update it AND every published copy". Nothing
// enforced that second half, so on 2026-09-02 README.md disagreed with the
// pinned numbers in three different ways at once — 101/146 in two places, a
// bare 100 in a third, and the correct 102/147 in five others. One file, three
// answers, none of them checked.
//
// This test is the missing enforcement. It reads the published docs the way a
// user does and asserts that every tool count printed next to a mode name is
// one the server actually registers.

// modeLine matches a doc line that talks about a tool mode at all. Lines that
// mention no mode are skipped: "10 tools for gCTS repository management" is a
// subset count and none of this test's business.
//
// The second alternative is for a table row labelled "Tools" whose mode names
// live in the header row above it — README.md has two of these, and the first
// version of this test walked straight past the one that said "101 essential |
// 146 complete".
var modeLine = regexp.MustCompile(`(?i)focused|expert|^\s*\|\s*\*\*Tools\*\*\s*\|`)

// countInLine pulls the numbers that are claims about how many tools a mode
// has. The three alternatives are the three shapes the docs actually use:
// "147 tools" / "102 curated tools", "Focused Mode Tools (102)", and
// "147 expert, 102 focused" / "101 essential | 146 complete".
var countInLine = regexp.MustCompile(
	`(?i)(\d+)\s+(?:individual\s+|curated\s+)?tools?\b` +
		`|Tools\s*\((\d+)\)` +
		`|(\d+)\s+(?:expert|focused|essential|complete)\b`)

func TestPublishedToolCountsMatchPinnedCounts(t *testing.T) {
	allowed := map[int]string{
		wantHyperfocusedTools: "hyperfocused",
		wantFocusedTools:      "focused",
		wantExpertTools:       "expert",
	}

	var allowedList []string
	for n, mode := range allowed {
		allowedList = append(allowedList, fmt.Sprintf("%d (%s)", n, mode))
	}

	for _, name := range []string{"README.md", "CLAUDE.md"} {
		path := filepath.Join("..", "..", name)
		raw, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("reading %s: %v", name, err)
		}

		checked := 0
		for i, line := range strings.Split(string(raw), "\n") {
			if !modeLine.MatchString(line) {
				continue
			}
			for _, m := range countInLine.FindAllStringSubmatch(line, -1) {
				for _, group := range m[1:] {
					if group == "" {
						continue
					}
					n, err := strconv.Atoi(group)
					if err != nil {
						continue
					}
					checked++
					if _, ok := allowed[n]; !ok {
						t.Errorf(
							"%s:%d claims %d tools, which no mode registers.\n"+
								"  line: %s\n"+
								"  registered: %s\n"+
								"  Fix the doc, or change the pinned counts in tools_parity_test.go "+
								"and every published copy with them.",
							name, i+1, n, strings.TrimSpace(line),
							strings.Join(allowedList, ", "))
					}
				}
			}
		}

		// A regex that silently stops matching would turn this test green
		// while the docs rotted, which is the exact failure it exists to
		// catch. README.md carries these counts in a dozen places; if it
		// suddenly carries none, the matcher broke, not the docs.
		if name == "README.md" && checked == 0 {
			t.Errorf("%s: found no tool-count claims at all — the matcher has stopped matching", name)
		}
	}
}

// docs/cli-agents states the mode counts in two shapes the README matcher does
// not read: "focused (98 tools) or expert (148)", plus its ES/RU/UA
// translations where "tools" is not English, and codex.md's table rows
// "| `focused` | 98 |". The expert count there sat at a stale 147 with nothing
// checking it. Here each count is tied to the pin for its own mode.
var cliAgentsModeCount = regexp.MustCompile(
	"(?i)\\b(focused|expert)\\s*\\((\\d+)" +
		"|\\|\\s*`(focused|expert)`\\s*\\|\\s*(\\d+)\\s*\\|")

func TestCLIAgentsDocsToolCountsMatchPinnedCounts(t *testing.T) {
	want := map[string]int{"focused": wantFocusedTools, "expert": wantExpertTools}

	files, err := filepath.Glob(filepath.Join("..", "..", "docs", "cli-agents", "*.md"))
	if err != nil || len(files) == 0 {
		t.Fatalf("no docs/cli-agents/*.md found (err=%v)", err)
	}
	for _, path := range files {
		raw, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("reading %s: %v", path, err)
		}
		seen := map[string]bool{}
		for i, line := range strings.Split(string(raw), "\n") {
			for _, m := range cliAgentsModeCount.FindAllStringSubmatch(line, -1) {
				mode, num := m[1], m[2]
				if mode == "" {
					mode, num = m[3], m[4]
				}
				mode = strings.ToLower(mode)
				n, _ := strconv.Atoi(num)
				seen[mode] = true
				if n != want[mode] {
					t.Errorf("%s:%d says %s has %d tools; the server registers %d.\n  line: %s",
						filepath.Base(path), i+1, mode, n, want[mode], strings.TrimSpace(line))
				}
			}
		}
		// Each of these files states both counts today. If one suddenly states
		// neither, the matcher broke; the docs did not get fixed.
		if !seen["focused"] || !seen["expert"] {
			t.Errorf("%s: expected both a focused and an expert tool count, found %v — the matcher has stopped matching",
				filepath.Base(path), seen)
		}
	}
}
