package adt

import (
	"fmt"
	"strings"
	"testing"
)

// The CLI reported a refused activation as "Activation failed - check
// activation messages" and never showed one: the messages were parsed into
// result.Activation and dropped on the way to the error it printed. abapGit's
// standalone install on a system missing some of the types it names failed
// that way with 176 messages and no word of any of them.
func TestWriteSourceResultReportCarriesActivationMessages(t *testing.T) {
	activation := &ActivationResult{Success: false}
	activation.Messages = append(activation.Messages, ActivationResultMessage{
		Type: "W", ShortText: "Activation was cancelled.",
	})
	for i := 1; i <= 30; i++ {
		activation.Messages = append(activation.Messages, ActivationResultMessage{
			ObjDescr:  "Program ZABAPGIT_STANDALONE",
			Type:      "E",
			Line:      1,
			Href:      fmt.Sprintf("/sap/bc/adt/programs/programs/zabapgit_standalone/source/main#start=%d,4", 100+i),
			ShortText: fmt.Sprintf("Type ZIF_UNKNOWN_%d is unknown.", i),
		})
	}
	result := &WriteSourceResult{
		ObjectType: "PROG", ObjectName: "ZABAPGIT_STANDALONE",
		Message:    "Activation failed - check activation messages",
		Activation: activation,
	}

	err := WriteSourceResultReport(result)
	if err == nil {
		t.Fatal("a refused activation must be an error")
	}
	text := err.Error()
	// The verdict alone stays short: a caller that sends the structured
	// result as well (MCP) must not send the messages twice.
	if verdict := WriteSourceResultError(result).Error(); strings.Contains(verdict, "ZIF_UNKNOWN") {
		t.Errorf("the verdict repeats the messages: %s", verdict)
	}
	for _, want := range []string{
		"Activation failed - check activation messages",
		"[E] Program ZABAPGIT_STANDALONE, line 101: Type ZIF_UNKNOWN_1 is unknown.",
		"[E] Program ZABAPGIT_STANDALONE, line 120: Type ZIF_UNKNOWN_20 is unknown.",
		"... and 11 more",
	} {
		if !strings.Contains(text, want) {
			t.Errorf("the error must carry %q; got:\n%s", want, text)
		}
	}
	// Errors first, and the cap holds: the 21st error and the warning are
	// counted, not listed.
	if strings.Contains(text, "ZIF_UNKNOWN_21 ") || strings.Contains(text, "Activation was cancelled") {
		t.Errorf("more than %d messages listed, or a warning ahead of an error:\n%s", ActivationMessageLimit, text)
	}
}

func TestActivationMessageLines(t *testing.T) {
	if lines := (&ActivationResult{Success: true}).MessageLines(20); lines != nil {
		t.Fatalf("a successful activation renders nothing, got %v", lines)
	}
	var nilResult *ActivationResult
	if lines := nilResult.MessageLines(20); lines != nil {
		t.Fatalf("no activation renders nothing, got %v", lines)
	}

	refused := &ActivationResult{Messages: []ActivationResultMessage{
		{Type: "W", ShortText: "Activation was cancelled."},
		{Type: "E", ObjDescr: "Class ZCL_X", Line: 7, ShortText: " Field LV_Y is unknown. "},
		{Type: "E", ShortText: ""},
	}}
	got := refused.MessageLines(0)
	want := []string{
		"[E] Class ZCL_X, line 7: Field LV_Y is unknown.",
		"[E] (SAP gave no text for this message)",
		"[W] Activation was cancelled.",
	}
	if strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Fatalf("got\n%s\nwant\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}

	// No messages at all: still a reason, never silence.
	if lines := (&ActivationResult{}).MessageLines(20); len(lines) != 1 || !strings.Contains(lines[0], "SAP named no reason") {
		t.Fatalf("a refusal without messages must still say something, got %v", lines)
	}
}

// One message cannot defeat the count cap by its size, nor break the
// one-message-per-line shape with line breaks or terminal control codes.
func TestActivationMessageFieldsAreBoundedAndOneLine(t *testing.T) {
	huge := strings.Repeat("x", 100000)
	r := &ActivationResult{Messages: []ActivationResultMessage{
		{Type: "E", ObjDescr: "Program\nZX\x1b[31m", ShortText: "first\r\nsecond\tthird " + huge},
	}}
	lines := r.MessageLines(20)
	if len(lines) != 1 {
		t.Fatalf("got %d lines", len(lines))
	}
	line := lines[0]
	if strings.ContainsAny(line, "\n\r\t\x1b") {
		t.Fatalf("control characters survived: %q", line[:80])
	}
	if !strings.HasPrefix(line, "[E] Program ZX [31m: first second third x") {
		t.Fatalf("got %q", line[:80])
	}
	if n := len([]rune(line)); n > 2*activationFieldLimit+20 || !strings.HasSuffix(line, "…") {
		t.Fatalf("a %d-rune message was not cut: %d runes, ends %q", len(huge), n, line[len(line)-10:])
	}
}

// A refusal that names only inactive objects lists the first
// ActivationMessageLimit of them, each bounded and on one line, and counts
// the rest.
func TestProblemLinesBoundsTheInactiveList(t *testing.T) {
	r := &ActivationResult{}
	for i := 0; i < 500; i++ {
		r.Inactive = append(r.Inactive, InactiveObject{
			Name: fmt.Sprintf("ZCL_%03d", i),
			URI:  "/sap/bc/adt/oo/classes/zcl\n" + strings.Repeat("x", 1000),
		})
	}
	lines := r.ProblemLines()
	if len(lines) != 1 {
		t.Fatalf("got %d lines", len(lines))
	}
	line := lines[0]
	if !strings.Contains(line, "and 480 more") || strings.Contains(line, "ZCL_020") {
		t.Fatalf("the list was not capped at %d: %.200q", ActivationMessageLimit, line)
	}
	if strings.Contains(line, "\n") || len([]rune(line)) > (activationFieldLimit+4)*ActivationMessageLimit+100 {
		t.Fatalf("the line is not bounded or not one line: %d runes", len([]rune(line)))
	}
}
