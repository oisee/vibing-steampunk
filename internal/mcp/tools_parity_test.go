package mcp

import (
	"sort"
	"strings"
	"testing"
)

// The published tool counts drifted from the code in three places at once —
// --help said one thing, the README another, and neither matched what the
// server registered. Nobody noticed because nothing asserted it. These tests
// are that assertion: change the surface and one of them fails, so the number
// is corrected in the same commit rather than a year later.
//
// If a count here changes deliberately, update it AND every published copy:
// cmd/vsp/main.go (the --mode flag help and the long usage) and README.md.
const (
	wantHyperfocusedTools = 1
	wantFocusedTools      = 100
	wantExpertTools       = 147
)

// serverForMode builds a server without touching a network. NewServer only
// constructs a client; nothing is dialled until a tool runs.
func serverForMode(t *testing.T, mode string) *Server {
	t.Helper()
	return NewServer(&Config{
		BaseURL:  "https://example.invalid",
		Username: "tester",
		Password: "unused",
		Client:   "000",
		Language: "EN",
		Mode:     mode,
	})
}

func toolNames(s *Server) []string {
	registered := s.mcpServer.ListTools()
	names := make([]string, 0, len(registered))
	for name := range registered {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

func TestToolCountsMatchWhatIsPublished(t *testing.T) {
	for mode, want := range map[string]int{
		"hyperfocused": wantHyperfocusedTools,
		"focused":      wantFocusedTools,
		"expert":       wantExpertTools,
	} {
		got := len(toolNames(serverForMode(t, mode)))
		if got != want {
			t.Errorf("%s registers %d tools, expected %d — if this is intended, "+
				"update this constant and every published count (cmd/vsp/main.go, README.md)",
				mode, got, want)
		}
	}
}

func TestHyperfocusedRegistersOnlyTheUniversalTool(t *testing.T) {
	names := toolNames(serverForMode(t, "hyperfocused"))
	if len(names) != 1 || names[0] != "SAP" {
		t.Errorf("hyperfocused registers %v, want exactly [SAP]", names)
	}
}

func TestFocusedIsASubsetOfExpert(t *testing.T) {
	// Focused is described as a whitelist over the full surface. If a tool
	// appears in focused and not in expert, the whitelist has drifted away from
	// what exists — which is how four gCTS entries came to name tools that are
	// registered nowhere.
	expert := map[string]bool{}
	for _, n := range toolNames(serverForMode(t, "expert")) {
		expert[n] = true
	}
	var orphans []string
	for _, n := range toolNames(serverForMode(t, "focused")) {
		if !expert[n] {
			orphans = append(orphans, n)
		}
	}
	if len(orphans) > 0 {
		t.Errorf("focused registers tools that expert does not: %s", strings.Join(orphans, ", "))
	}
}

func TestWhitelistNamesSomethingThatExists(t *testing.T) {
	// Every name in the focused whitelist should be a tool the server can
	// actually register in expert mode. A name that matches nothing is a
	// silent no-op, and reads as a capability that is present.
	expert := map[string]bool{}
	for _, n := range toolNames(serverForMode(t, "expert")) {
		expert[n] = true
	}

	var unknown []string
	for name := range focusedToolSet() {
		if !expert[name] {
			unknown = append(unknown, name)
		}
	}
	sort.Strings(unknown)
	if len(unknown) > 0 {
		t.Errorf("the focused whitelist names %d tools that are registered nowhere: %s\n"+
			"either register them or drop them from the whitelist",
			len(unknown), strings.Join(unknown, ", "))
	}
}

func TestUniversalToolReachesMostOfTheSurface(t *testing.T) {
	// The README claims the single SAP() tool covers most of what the individual
	// ones do. Some are genuinely unreachable through it — i18n, revision
	// history, AnalyzeABAPCode — and the exact figure has not been measured
	// mechanically. Until it is, this records the surface it is measured
	// against so the two cannot drift apart unnoticed.
	expert := len(toolNames(serverForMode(t, "expert")))
	if expert < 100 {
		t.Fatalf("expert registers %d tools, which cannot be right", expert)
	}
	t.Logf("universal tool measured against %d registered tools", expert)
}

func TestModesAreDistinct(t *testing.T) {
	counts := map[string]int{}
	for _, mode := range []string{"hyperfocused", "focused", "expert"} {
		counts[mode] = len(toolNames(serverForMode(t, mode)))
	}
	if !(counts["hyperfocused"] < counts["focused"] && counts["focused"] < counts["expert"]) {
		t.Errorf("modes are not ordered by size: %v", counts)
	}
}
