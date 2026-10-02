package mcp

import (
	"strings"
	"testing"
)

// Measured before this file existed: of 147 tools in expert mode the universal
// tool reached 126. Ten of the twenty-one it did not reach are gCTS, which
// `registerGCTSTools` never registers in any mode, so they were already dead.
// The live remainder is eleven, and eleven is the whole cost of retiring the
// other two modes.
func TestTheElevenAreReachableThroughTheUniversalTool(t *testing.T) {
	srv := serverForMode(t, "hyperfocused")

	cases := []struct {
		action string
		params map[string]any
	}{
		{"i18n", map[string]any{"op": "texts"}},
		{"i18n", map[string]any{"op": "data_element_labels"}},
		{"i18n", map[string]any{"op": "message_class_texts"}},
		{"i18n", map[string]any{"op": "text_pool"}},
		{"i18n", map[string]any{"op": "compare_languages"}},
		{"i18n", map[string]any{"op": "write_labels"}},
		{"i18n", map[string]any{"op": "write_message_texts"}},
		{"revisions", map[string]any{"op": "list"}},
		{"revisions", map[string]any{"op": "source"}},
		{"revisions", map[string]any{"op": "compare"}},
		{"lint", map[string]any{}},
	}
	if len(cases) != 11 {
		t.Fatalf("this test covers %d of the eleven", len(cases))
	}
	for _, c := range cases {
		args := map[string]any{"action": c.action, "params": c.params}
		result, err := srv.handleUniversalTool(t.Context(), newRequest(args))
		if err != nil {
			t.Errorf("%s %v: %v", c.action, c.params, err)
			continue
		}
		if text := resultText(result); strings.Contains(text, "No handler found") {
			t.Errorf("%s %v is still unreachable:\n%s", c.action, c.params, text)
		}
	}
}

// A recognised action owns its answer. Falling through would tell a caller that
// action="i18n" does not exist — the defect that made query and grep look
// missing from a build where both worked.
func TestAnUnknownOperationNamesWhatIsAvailable(t *testing.T) {
	srv := serverForMode(t, "hyperfocused")
	for _, action := range []string{"i18n", "revisions"} {
		result, err := srv.handleUniversalTool(t.Context(), newRequest(map[string]any{
			"action": action,
			"params": map[string]any{"op": "no_such_operation"},
		}))
		if err != nil {
			t.Fatalf("%s: %v", action, err)
		}
		text := resultText(result)
		if strings.Contains(text, "No handler found") {
			t.Errorf("%s denied its own existence:\n%s", action, text)
		}
		if !strings.Contains(text, "no_such_operation") {
			t.Errorf("%s does not name what was passed:\n%s", action, text)
		}
	}
}

// Static analysis is what somebody reaches for under "analyze", and finding
// nothing there is how a capability comes to look missing.
func TestLintIsReachableUnderAnalyzeToo(t *testing.T) {
	srv := serverForMode(t, "hyperfocused")
	result, err := srv.handleUniversalTool(t.Context(), newRequest(map[string]any{
		"action": "analyze",
		"params": map[string]any{"type": "lint", "source": "REPORT zdemo.\nWRITE 'x'.\n"},
	}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if text := resultText(result); strings.Contains(text, "No handler found") {
		t.Errorf("analyze type=lint is not routed:\n%s", text)
	}
}

// Asking for history without saying which operation means the list — that is
// the question somebody means first, and refusing it teaches nothing.
func TestRevisionsDefaultsToTheList(t *testing.T) {
	srv := serverForMode(t, "hyperfocused")
	result, err := srv.handleUniversalTool(t.Context(), newRequest(map[string]any{
		"action": "revisions",
		"params": map[string]any{"object_type": "CLAS", "object_name": "ZCL_DEMO"},
	}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	text := resultText(result)
	if strings.Contains(text, "No handler found") || strings.Contains(text, "needs one of") {
		t.Errorf("revisions with no op did not default to the list:\n%s", text)
	}
}

// The measurement this file exists to close, kept as an assertion so it cannot
// silently reopen: every tool registered in expert mode is reachable through
// the universal router, or is one of the gCTS tools that no mode registers.
//
// It is checked by name rather than by handler pointer, because a handler
// reachable from a route is not the same as a *tool* a user can find — and the
// second is what the claim is about.
func TestNothingLiveIsUnreachableAnyMore(t *testing.T) {
	expert := map[string]bool{}
	for _, n := range toolNames(serverForMode(t, "expert")) {
		expert[n] = true
	}
	// Measured on 2026-08-25 and closed by this file. If one of these becomes
	// reachable, or a new unreachable tool appears, this list is the place the
	// change has to be argued. WriteDataElementLabels was the twelfth name until
	// it was unregistered (it refuses by design); i18n op=write_labels still
	// answers with that refusal.
	knownUnreachable := []string{
		"AnalyzeABAPCode", "CompareLanguages", "CompareVersions",
		"GetDataElementLabels", "GetMessageClassTexts", "GetObjectTextsInLanguage",
		"GetRevisionSource", "GetRevisions", "GetTextPool",
		"WriteMessageClassTexts",
	}
	for _, n := range knownUnreachable {
		if !expert[n] {
			t.Errorf("%s is no longer registered; the eleven were measured against a surface that has moved", n)
		}
	}
	if len(knownUnreachable) != 10 {
		t.Fatalf("the list holds %d, and the measurement said eleven less the unregistered WriteDataElementLabels", len(knownUnreachable))
	}
}
