package adt

import (
	"context"
	"encoding/xml"
	"errors"
	"fmt"
	"net/url"
	"strings"
)

// --- Compile-only check of an ad-hoc snippet ---

// CheckABAPFinding is one message of SAP's syntax check, placed in the
// caller's snippet.
type CheckABAPFinding struct {
	// Line is the line of the snippet, counting from one. Zero when SAP placed
	// the message outside the snippet — in the wrapper around it, typically at
	// ENDMETHOD for a block the snippet opened and never closed — and then
	// WrapperLine says where.
	Line int `json:"line"`
	// Column counts from one. The snippet is put into the wrapper unindented,
	// so a column inside the snippet is a column of what the caller wrote.
	Column int `json:"column,omitempty"`
	// Severity is "error", "warning" or "info".
	Severity string `json:"severity"`
	// Message is SAP's text, verbatim.
	Message     string `json:"message"`
	WrapperLine int    `json:"wrapperLine,omitempty"`
	// AfterSnippet marks a message SAP placed in the wrapper after the
	// snippet's last line. It is nearly always the snippet's fault all the
	// same: a last statement without its period runs on into the wrapper, and
	// a block left open is reported at ENDMETHOD.
	AfterSnippet bool `json:"afterSnippet,omitempty"`
}

// CheckABAPResult is the outcome of CheckABAP.
type CheckABAPResult struct {
	// OK is true when SAP found no error. Warnings do not make a snippet fail.
	OK       bool               `json:"ok"`
	Findings []CheckABAPFinding `json:"findings"`
	// ProgramName is the temporary program the snippet was checked in.
	ProgramName string `json:"programName"`
	// CleanedUp means the DELETE of the temporary program succeeded. It is
	// false, with Warnings saying why, when the program may still be in $TMP.
	CleanedUp bool     `json:"cleanedUp"`
	Warnings  []string `json:"warnings,omitempty"`
}

// checkABAPProgramPrefix names the temporary program, in the VSP domain the
// object naming convention asks for, and distinct from ExecuteABAP's
// ZTEMP_EXEC_ so a leftover says which tool left it.
const checkABAPProgramPrefix = "ZVSP_CHK_"

// CheckABAP type-checks a snippet without running it.
//
// The snippet is wrapped exactly as ExecuteABAP wraps it — the same generated
// report, the snippet inside the same test method — so what compiles here is
// what execute_abap would compile, and line numbers are translated back the
// same way.
//
// Why a temporary program rather than a check of content alone: ADT's
// checkruns accepts content for a program that does not exist and checks it
// without saving anything, but such a virtual program has no attributes, and
// fixed-point arithmetic in particular is off. Every ABAP SQL statement with a
// host variable (`INTO TABLE @DATA(lt)`) then fails with a fixed-point error,
// and that error hides every other error in the same statement — a misspelled
// column comes back as the fixed-point complaint and nothing else. A checker
// that cannot see into SELECTs is not one, so the snippet is checked in a real
// program, created the way ExecuteABAP creates it.
//
// What is written: the program is created in $TMP, and deleted again. Its
// source is never written — the check sends the source as content, which
// checkruns does not save — it is never activated and nothing runs. The
// delete is deferred the moment the create succeeds, so it is attempted on
// every path after that, including a failed check and a cancelled context.
//
// Being a create and a delete, it is refused under --read-only, and refused
// up front — before anything is created — wherever the create or the delete
// would be refused, so it never makes an object it is not allowed to remove.
func (c *Client) CheckABAP(ctx context.Context, code string) (result *CheckABAPResult, err error) {
	if strings.TrimSpace(code) == "" {
		return nil, errors.New("code is required")
	}
	for _, op := range []struct {
		op   OperationType
		name string
	}{{OpCreate, "CheckABAP (create the temporary program)"}, {OpLock, "CheckABAP (lock the temporary program for deletion)"}, {OpDelete, "CheckABAP (delete the temporary program)"}} {
		if gateErr := c.checkMutation(ctx, MutationContext{Op: op.op, OpName: op.name, Package: "$TMP"}); gateErr != nil {
			return nil, gateErr
		}
	}

	// The first free name of the caller's pool, as for ExecuteABAP. A failed
	// create whose object then turns out to exist is reported, not deleted:
	// nothing shows this call created it.
	programName, releaseName, createErr := c.createTemporaryProgram(ctx, checkABAPProgramPrefix, "Temp program for CheckABAP")
	if createErr != nil {
		if programName == "" {
			return nil, createErr
		}
		return nil, fmt.Errorf("creating the temporary program %s in $TMP: %w", programName, createErr)
	}
	// Deferred before the cleanup so that it runs after it: the name only goes
	// back to the pool once the program is deleted.
	defer releaseName()
	objectURL := "/sap/bc/adt/programs/programs/" + url.PathEscape(programName)
	source := executeWrapperSource(programName, "RISK LEVEL HARMLESS", "lv_result", code)

	// Created in $TMP, which the gate above approved: the delete need not look
	// the package up again (see ExecuteABAP).
	ctx = withMutationPackageChecked(ctx, objectURL)

	result = &CheckABAPResult{ProgramName: programName, Findings: []CheckABAPFinding{}}
	defer func() {
		cleanupCtx, cancel := failureCleanupContext(ctx)
		defer cancel()
		warnings := c.deleteTemporaryProgram(cleanupCtx, objectURL, programName)
		result.CleanedUp = len(warnings) == 0
		result.Warnings = append(result.Warnings, warnings...)
		if err != nil && len(warnings) > 0 {
			err = fmt.Errorf("%w (cleanup: %s)", err, strings.Join(warnings, "; "))
		}
	}()

	body, err := c.postCheckRun(ctx, objectURL, source)
	if err != nil {
		return result, fmt.Errorf("syntax check failed: %w", err)
	}
	if procErr := checkRunProcessed(body); procErr != nil {
		return result, procErr
	}
	messages, err := parseSyntaxCheckResults(body)
	if err != nil {
		return result, err
	}

	result.Findings = checkABAPFindings(messages, programName, payloadOffset(source), strings.Count(code, "\n")+1)
	result.OK = true
	for _, f := range result.Findings {
		if f.Severity == "error" {
			result.OK = false
		}
	}
	return result, nil
}

// checkRunProcessed refuses a check run SAP answered without checking.
//
// checkruns answers 200 either way, and a report it did not process carries no
// messages, which is exactly what a clean snippet looks like. The status is the
// only difference, so it is read before the silence is believed.
func checkRunProcessed(data []byte) error {
	var resp struct {
		Reports []struct {
			Status     string `xml:"status,attr"`
			StatusText string `xml:"statusText,attr"`
		} `xml:"checkReport"`
	}
	if err := xml.Unmarshal(data, &resp); err != nil {
		return fmt.Errorf("parsing syntax check response: %w", err)
	}
	if len(resp.Reports) == 0 {
		return errors.New("syntax check returned no report, so the code cannot be shown to have been checked")
	}
	for _, r := range resp.Reports {
		// Fail closed: a report that does not say it was processed is not
		// taken as a check. Every report A4H and the captured fixtures show
		// carries a status, so a missing one is a shape this code has not seen.
		if r.Status == "" {
			return errors.New("syntax check report has no status, so the code cannot be shown to have been checked")
		}
		if !strings.EqualFold(r.Status, "processed") {
			return fmt.Errorf("SAP did not check the code (status %s): %s", r.Status, r.StatusText)
		}
	}
	return nil
}

// checkABAPFindings places syntax check messages in the caller's snippet.
//
// offset is the wrapper line holding the snippet's first line, and lines the
// number of lines the snippet has. A message is placed in the snippet only
// when it names the temporary program and falls within those lines; anything
// else keeps its wrapper line and gets snippet line zero, rather than a line
// number the caller never wrote.
func checkABAPFindings(messages []SyntaxCheckResult, programName string, offset, lines int) []CheckABAPFinding {
	findings := make([]CheckABAPFinding, 0, len(messages))
	for _, m := range messages {
		f := CheckABAPFinding{
			Severity: checkSeverity(m.Severity),
			Message:  strings.TrimSpace(m.Text),
		}
		if m.Line > 0 {
			f.Column = m.Offset + 1
		}
		ours := uriNamesProgram(m.URI, programName)
		if ours && offset > 0 && m.Line >= offset && m.Line < offset+lines {
			f.Line = m.Line - offset + 1
		} else if ours {
			f.WrapperLine = m.Line
			f.AfterSnippet = offset > 0 && m.Line >= offset+lines
		} else {
			// Another object's position means nothing in this snippet.
			f.Column = 0
		}
		findings = append(findings, f)
	}
	return findings
}

// uriNamesProgram says whether a message URI points into programName.
//
// Only the path counts, and only a whole segment of it: the query of a message
// about another include can name the temporary program as its context
// (`?context=.../programs/zvsp_chk_…`), and that does not make its line one of
// ours.
func uriNamesProgram(uri, programName string) bool {
	if programName == "" {
		return false
	}
	path, _, _ := strings.Cut(uri, "?")
	path, _, _ = strings.Cut(path, "#")
	for _, segment := range strings.Split(path, "/") {
		if unescaped, err := url.PathUnescape(segment); err == nil {
			segment = unescaped
		}
		if strings.EqualFold(segment, programName) {
			return true
		}
	}
	return false
}

// checkSeverity spells a checkrun message type as a word. Anything SAP sends
// that is not a known warning or information is counted as an error: an
// unknown severity is not a reason to call a snippet clean.
func checkSeverity(t string) string {
	switch strings.ToUpper(strings.TrimSpace(t)) {
	case "W":
		return "warning"
	case "I", "S":
		return "info"
	default:
		return "error"
	}
}
