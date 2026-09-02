package adt

import (
	"fmt"
	"strconv"
	"strings"
)

// An activation that fails does not fail the request. SAP answers 200 with a
// checklist of messages, and the refusal is inside the body:
//
//	<chkl:properties checkExecuted="true" activationExecuted="false" .../>
//	<msg type="W" ...><shortText><txt>Activation was cancelled.</txt></shortText></msg>
//	<msg type="E" href=".../source/main#start=18,18" ...>
//	  <shortText><txt>Tables with headers are no longer supported in the OO context.</txt></shortText>
//	</msg>
//
// So every caller that wrote `_, err = c.Activate(...)` and looked only at err
// has been reading "this does not compile" as "this is active". In ExecuteABAP
// that produced the worst version of it: the run went on to ask ABAP Unit for
// the tests of a program that was never generated, ABAP Unit answered an empty
// <aunit:runResult/>, an empty runResult has no alerts, no alerts meant no
// failure, and a syntax error came back as "Executed successfully".
//
// The helpers here are the half that was missing — what in the checklist says
// no, and where it says it happened.

// activationErrorTypes are the message types that mean the object did not
// activate: SAP's E (error), A (abort) and X (short dump). W and I are produced
// by activations that succeed, so they prove nothing on their own.
const activationErrorTypes = "EAX"

// ActivationResultError converts SAP's HTTP-200 activation refusal into an
// error for callers that must not continue after a logical failure.
func ActivationResultError(result *ActivationResult) error {
	if result == nil {
		return fmt.Errorf("activation returned no result")
	}
	if result.Success {
		return nil
	}
	return fmt.Errorf("activation failed: %s", strings.Join(result.ProblemLines(), "; "))
}

// ErrorMessages returns the messages that say the object did not activate, in
// the order SAP listed them — the first is the one to lead with.
func (r *ActivationResult) ErrorMessages() []ActivationResultMessage {
	if r == nil {
		return nil
	}
	var errs []ActivationResultMessage
	for _, m := range r.Messages {
		if strings.ContainsAny(m.Type, activationErrorTypes) {
			errs = append(errs, m)
		}
	}
	return errs
}

// ProblemLines renders a refused activation as one line per message, in the
// "Line N: text" shape the deploy result already uses for syntax errors —
// because to whoever ran the deploy they are the same event: SAP would not take
// the source.
//
// A refusal that names no message still returns a line. Saying nothing here
// would put us back where we started, with a failure that reads as an empty
// success.
func (r *ActivationResult) ProblemLines() []string {
	if r == nil || r.Success {
		return nil
	}
	// A checklist can refuse without an E in it: activationExecuted="false" and
	// a type="W" "Activation was cancelled." is a real refusal whose only reason
	// is that warning. Leading with "SAP named no reason" while holding SAP's
	// reason is the same silence this file exists to end, so when nothing is
	// error-typed the warnings are what there is to report.
	msgs := r.ErrorMessages()
	if len(msgs) == 0 {
		msgs = r.Messages
	}

	var lines []string
	for _, m := range msgs {
		text := strings.TrimSpace(m.ShortText)
		if text == "" {
			text = "(SAP gave no text for this message)"
		}
		if line := m.SourceLine(); line > 0 {
			text = "Line " + strconv.Itoa(line) + ": " + text
		}
		lines = append(lines, text)
	}
	if len(lines) == 0 {
		lines = append(lines, "Activation was refused and SAP named no reason; the object is still inactive")
	}
	return lines
}

// SourceLine is the line of source the message points at, or zero when it
// points nowhere.
//
// The `line` attribute is not that line. A syntax error in a generated report
// arrives as line="1" with the real position in the href fragment
// (".../source/main#start=18,18"), so the fragment wins and the attribute is
// only a fallback for messages that carry no href at all.
func (m ActivationResultMessage) SourceLine() int {
	if line := uriFragmentLine(m.Href); line > 0 {
		return line
	}
	return m.Line
}

// uriFragmentLine pulls the line number out of an ADT source uri fragment,
// ".../source/main#start=18,0", and returns zero when there is not one.
//
// Both an activation message's href and a unit test stack entry's uri carry the
// position this way, which is why it lives here rather than in either.
func uriFragmentLine(uri string) int {
	_, fragment, found := strings.Cut(uri, "#start=")
	if !found {
		return 0
	}
	number, _, _ := strings.Cut(fragment, ",")
	line, err := strconv.Atoi(strings.TrimSpace(number))
	if err != nil {
		return 0
	}
	return line
}
