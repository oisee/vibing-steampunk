package adt

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"math/big"
	"net/url"
	"strconv"
	"strings"
	"sync"
)

// --- Execute ABAP Code via Unit Test ---

// ExecuteABAPResult represents the result of executing ABAP code via unit test.
type ExecuteABAPResult struct {
	Success       bool            `json:"success"`
	ProgramName   string          `json:"programName"`
	Output        []string        `json:"output"`              // Values returned via assertion messages
	RawAlerts     []UnitTestAlert `json:"rawAlerts,omitempty"` // Full alert details for debugging
	ExecutionTime float64         `json:"executionTime"`       // Execution time in seconds
	Message       string          `json:"message,omitempty"`
	CleanedUp     bool            `json:"cleanedUp"`

	// Failure is what stopped the code before it finished, when something did.
	Failure *ExecuteFailure `json:"failure,omitempty"`

	// ResultText is Output in the shape a caller wants to read: the one value
	// as a string when the code returned one, an array when it returned several
	// (RETURN_VALUE( ) more than once), and absent when nothing came back. It
	// is the full value, unwrapped from SAP's "Critical Assertion Error: '…'"
	// sentence and never shortened.
	ResultText any `json:"result_text,omitempty"`
}

// ExecuteFailure is a run that ended in the middle of the caller's code.
//
// ABAP Unit catches whatever the payload raises and reports it as an alert, so
// this evidence was always in the response — it was simply thrown away, and a
// division by zero came back as "Executed successfully (no output captured)".
// Note that ABAP Unit catching it also means there is no dump in ST22 to find
// afterwards: for this class of failure the response is the only witness.
type ExecuteFailure struct {
	// Kind and Severity are ABAP Unit's own words: "exception" / "critical" for
	// a runtime error, "failedAssertion" for an assert the payload made itself.
	// The two kinds declared below are ours, for the failures ABAP Unit never
	// saw because the code never reached it.
	Kind     string `json:"kind"`
	Severity string `json:"severity,omitempty"`
	// Title is SAP's summary, and for a runtime error it names the error:
	// "Exception Error <COMPUTE_INT_ZERODIVIDE>".
	Title   string   `json:"title"`
	Details []string `json:"details,omitempty"`
	// Line is the line of the *caller's* code, counting from one. The wrapper
	// is a generated report and its line numbers are meaningless to whoever
	// wrote the payload, so they are translated back. Zero when the stack did
	// not place the failure in the wrapper at all.
	Line int `json:"line,omitempty"`
}

// execResultMarker prefixes the assertion message that carries a value back out
// of the executed code. Every run that gets that far ends in a deliberate
// cl_abap_unit_assert=>fail — that is the only way a value leaves a test method
// — so "the test failed" says nothing here, and this marker is the whole
// difference between the failure we asked for and one we did not.
const execResultMarker = "EXEC_RESULT:"

// The two failure kinds that are not ABAP Unit's.
//
// Both mean the same thing to whoever submitted the code — none of it ran — and
// they are kept apart because the next move differs: a syntax error is fixed in
// the payload, while a run nobody can vouch for is a question about the system.
const (
	// ExecuteFailureSyntax is a payload that never ran because the program it
	// was wrapped in did not compile. SAP's own message says why.
	ExecuteFailureSyntax = "syntaxError"
	// ExecuteFailureNotRun is a run that activated and then left no evidence of
	// having happened: ABAP Unit reported no test class at all for a program
	// whose entire purpose is to hold one.
	ExecuteFailureNotRun = "notRun"
)

// payloadStartMarker is the line the generated wrapper puts immediately before
// the caller's code, and it is what makes a line number in the wrapper
// translatable back into a line number in what the caller actually wrote.
const payloadStartMarker = `" === USER CODE START ===`

// ExecuteABAPOptions configures ExecuteABAP behavior.
type ExecuteABAPOptions struct {
	// RiskLevel controls what operations the code can perform:
	// - "harmless" (default): No DB writes, no external calls
	// - "dangerous": Can write to DB, call external services
	// - "critical": Full system access (use with caution!)
	RiskLevel string

	// ReturnVariable is the name of the variable to return via assertion.
	// The code should set this variable, and its value will be returned.
	// If empty, uses "lv_result" by default.
	ReturnVariable string

	// KeepProgram prevents cleanup of the temp program (for debugging).
	KeepProgram bool

	// ProgramPrefix is the prefix for the temp program name.
	// Default is "ZTEMP_EXEC_". The full name is the prefix, the SAP user and
	// a slot digit (see pooledProgramName), and it is reused from call to call.
	ProgramPrefix string
}

// ExecuteABAP executes arbitrary ABAP code via a temporary unit test wrapper.
//
// This is a powerful tool that allows executing any ABAP code on the SAP system.
// The code is wrapped in a test class and executed via RunUnitTests.
// Return values are extracted from assertion messages.
//
// Workflow:
// 1. Take the first free name of the caller's pool (createTemporaryProgram)
// 2. Create program with test class wrapper
// 3. Inject user code into test method
// 4. Activate program
// 5. Run unit tests
// 6. Parse assertion messages for return values
// 7. Delete temp program (unless KeepProgram=true)
//
// Example:
//
//	result, err := client.ExecuteABAP(ctx, `
//	  DATA(lv_msg) = |Hello from SAP at { sy-datum } { sy-uzeit }|.
//	  DATA(lv_user) = sy-uname.
//	  lv_result = |{ lv_msg } by { lv_user }|.
//	`, nil)
//	// result.Output contains the assertion message with lv_result value
//
// Security: This is gated by OpWorkflow safety check.
func (c *Client) ExecuteABAP(ctx context.Context, code string, opts *ExecuteABAPOptions) (*ExecuteABAPResult, error) {
	// Safety check for workflow operations
	if err := c.checkSafety(OpWorkflow, "ExecuteABAP"); err != nil {
		return nil, err
	}

	if opts == nil {
		opts = &ExecuteABAPOptions{}
	}
	if opts.RiskLevel == "" {
		opts.RiskLevel = "harmless"
	}
	if opts.ReturnVariable == "" {
		opts.ReturnVariable = "lv_result"
	}
	if opts.ProgramPrefix == "" {
		opts.ProgramPrefix = "ZTEMP_EXEC_"
	}

	result := &ExecuteABAPResult{
		Output: []string{},
	}

	// Step 1: Create the temp program, under the first free name of the pool
	programName, releaseName, err := c.createTemporaryProgram(ctx, opts.ProgramPrefix, "Temp program for ExecuteABAP")
	result.ProgramName = programName
	if err != nil {
		result.Message = fmt.Sprintf("Failed to create temp program: %v", err)
		return result, nil
	}
	// Deferred first so that it runs last: the name only goes back to the pool
	// once the cleanup below has deleted the program, or kept it.
	defer releaseName()
	objectURL := fmt.Sprintf("/sap/bc/adt/programs/programs/%s", url.PathEscape(programName))

	// Build the test class wrapper source
	riskLevelABAP := "RISK LEVEL HARMLESS"
	switch strings.ToLower(opts.RiskLevel) {
	case "dangerous":
		riskLevelABAP = "RISK LEVEL DANGEROUS"
	case "critical":
		riskLevelABAP = "RISK LEVEL CRITICAL"
	}

	source := executeWrapperSource(programName, riskLevelABAP, opts.ReturnVariable, code)

	// The temp program lives in $TMP, and CreateObject only got that far
	// because "$TMP" passed the package whitelist. Record it so neither the
	// UpdateSource below nor the DeleteObject in the cleanup defer resolves the
	// package again while holding a lock (issue #91). The defer reads this
	// variable when it runs, so it sees the marked context too.
	ctx = withMutationPackageChecked(ctx, objectURL)

	// Ensure cleanup on any error (unless KeepProgram is set)
	defer func() {
		if !opts.KeepProgram {
			// The workflow may have returned because ctx was cancelled. Cleanup has
			// to keep the mutation-policy mark above, but cannot inherit that
			// cancellation or it will never reach SAP to release the temp object.
			cleanupCtx, cancel := failureCleanupContext(ctx)
			defer cancel()

			warnings := c.deleteTemporaryProgram(cleanupCtx, objectURL, programName)
			for _, warning := range warnings {
				appendExecuteCleanupWarning(result, warning)
			}
			if len(warnings) > 0 {
				return
			}

			result.CleanedUp = true
		}
	}()

	// Step 2: Lock and update source
	lock, err := c.LockObject(ctx, objectURL, "MODIFY")
	if err != nil {
		result.Message = fmt.Sprintf("Failed to lock temp program: %v", err)
		return result, nil
	}

	sourceURL := objectURL + "/source/main"
	err = c.UpdateSource(ctx, sourceURL, source, lock.LockHandle, "")
	if err != nil {
		result.Message = fmt.Sprintf("Failed to update source: %v", err)
		if unlockErr := c.releaseLockAfterFailure(ctx, objectURL, lock.LockHandle); unlockErr != nil {
			appendExecuteCleanupWarning(result, strandedLockAdvice(objectURL, unlockErr))
		}
		return result, nil
	}

	// Step 3: Unlock
	err = c.UnlockObject(ctx, objectURL, lock.LockHandle)
	if err != nil {
		result.Message = fmt.Sprintf("Failed to unlock: %v", err)
		// The UNLOCK above fails before it is sent when ctx has expired;
		// retry it detached, or the temp program stays locked (and, with
		// KeepProgram, nothing else would release it).
		if unlockErr := c.releaseLockAfterFailure(ctx, objectURL, lock.LockHandle); unlockErr != nil {
			appendExecuteCleanupWarning(result, strandedLockAdvice(objectURL, unlockErr))
		} else {
			result.Message += " — the lock was released on a retry"
		}
		return result, nil
	}

	// Step 4: Activate
	activation, err := c.Activate(ctx, objectURL, programName)
	if err != nil {
		result.Message = fmt.Sprintf("Failed to activate: %v", err)
		return result, nil
	}
	// Code that does not compile is refused right here, inside a 200, and going
	// on to Step 5 regardless is what let a syntax error report itself as a
	// success: ABAP Unit answers an empty <runResult/> for a program that was
	// never generated, and from there nothing can tell that emptiness apart
	// from a run that simply had nothing to say. Stop while the reason is still
	// in hand.
	if failure := compileFailure(activation, programName, payloadOffset(source)); failure != nil {
		result.Failure = failure
		result.Message = fmt.Sprintf("The code did not compile: %s", failure.Title)
		return result, nil
	}

	// Step 5: Run unit tests
	flags := UnitTestRunFlags{
		Harmless:  true,
		Dangerous: strings.ToLower(opts.RiskLevel) == "dangerous" || strings.ToLower(opts.RiskLevel) == "critical",
		Critical:  strings.ToLower(opts.RiskLevel) == "critical",
		Short:     true,
		Medium:    true,
		Long:      false,
	}

	testResult, err := c.RunUnitTests(ctx, objectURL, &flags)
	if err != nil {
		result.Message = fmt.Sprintf("Failed to run unit tests: %v", err)
		return result, nil
	}

	// Step 6: Parse results - extract assertion messages
	for _, class := range testResult.Classes {
		// A failure while the test class itself is being set up lands on the
		// class rather than on the method, and a payload that dies in a
		// CLASS-CONSTRUCTOR or a DATA declaration dies exactly there.
		alerts := append([]UnitTestAlert(nil), class.Alerts...)
		for _, method := range class.TestMethods {
			result.ExecutionTime += method.ExecutionTime
			alerts = append(alerts, method.Alerts...)
		}
		for _, alert := range alerts {
			result.RawAlerts = append(result.RawAlerts, alert)
			// Every alert that carries the marker is a value, wherever ABAP
			// Unit chose to file it. Reading only the method's alerts is how a
			// run could carry its result and still be reported as having
			// captured nothing.
			if output, found := alertExecResult(alert); found {
				result.Output = append(result.Output, output)
			}
		}
	}

	// A run that happened leaves a test class behind whether or not the payload
	// worked: the wrapper always ends in a deliberate assertion, so there is
	// always something to report. No class at all means ABAP Unit ran nothing
	// at all, and the one thing that silence must never be read as is a pass.
	if len(testResult.Classes) == 0 {
		result.Failure = &ExecuteFailure{
			Kind:  ExecuteFailureNotRun,
			Title: "ABAP Unit reported no test at all, so the code cannot be shown to have run",
			Details: []string{
				fmt.Sprintf("%s activated, and the test run then came back empty.", programName),
				"That is not evidence the code succeeded; it is the absence of evidence that it ran.",
			},
		}
	}

	// A test class above the system's risk level is "not executed": ABAP Unit
	// lists the class with no test method and says why only in a warning, with
	// severity "tolerable". That warning was the only report of a run that
	// never happened, and because only failures were checked, the run used to
	// be reported as a success. A listed method did run, even when it left
	// before the closing assertion (an early CHECK, RETURN or EXIT), so only a
	// result with no test method at all is a run that did not happen.
	if result.Failure == nil && len(testResult.Classes) > 0 && !anyTestMethod(testResult.Classes) && !anyExecResult(result.RawAlerts) && PayloadFailure(result.RawAlerts) == nil {
		result.Failure = &ExecuteFailure{
			Kind:  ExecuteFailureNotRun,
			Title: "ABAP Unit did not run the code to its end",
			Details: []string{
				fmt.Sprintf("%s activated, but the closing assertion that every completed run ends in never came back.", programName),
			},
		}
		for _, alert := range result.RawAlerts {
			result.Failure.Details = append(result.Failure.Details, alert.Title)
			result.Failure.Details = append(result.Failure.Details, alert.Details...)
			if result.Failure.Severity == "" {
				result.Failure.Title = alert.Title
				result.Failure.Severity = alert.Severity
			}
		}
	}

	if alert := PayloadFailure(result.RawAlerts); alert != nil {
		result.Failure = &ExecuteFailure{
			Kind:     alert.Kind,
			Severity: alert.Severity,
			Title:    alert.Title,
			Details:  alert.Details,
			Line:     payloadLine(*alert, programName, payloadOffset(source)),
		}
	}

	result.Success = result.Failure == nil
	result.ResultText = resultText(result.Output)
	switch {
	case result.Failure != nil:
		result.Message = fmt.Sprintf("The code did not finish: %s", result.Failure.Title)
	case len(result.Output) > 0:
		result.Message = fmt.Sprintf("Executed successfully, %d output(s) returned", len(result.Output))
	default:
		result.Message = "Executed successfully (no output captured)"
	}

	return result, nil
}

// deleteTemporaryProgram removes a program a workflow created for itself, and
// returns what went wrong; no warnings means the DELETE succeeded.
//
// DELETE is attempted once. A failed request is an unknown result, not
// permission to retry a potentially completed mutation, and a successful one
// is not followed by a read to verify the object is gone.
//
// Every warning names the program, because a warning means it may still be in
// $TMP and the name is what the user needs to find and delete it.
func (c *Client) deleteTemporaryProgram(ctx context.Context, objectURL, programName string) []string {
	lock, err := c.LockObject(ctx, objectURL, "MODIFY")
	if err != nil {
		return []string{fmt.Sprintf("could not lock the temporary program for cleanup, so %s is still in $TMP: %v", programName, err)}
	}
	if err := c.DeleteObject(ctx, objectURL, lock.LockHandle, ""); err != nil {
		warnings := []string{fmt.Sprintf("temporary-program DELETE outcome is unknown and was not retried, so %s may still be in $TMP: %v", programName, err)}
		if unlockErr := c.releaseLockAfterFailure(ctx, objectURL, lock.LockHandle); unlockErr != nil {
			warnings = append(warnings, strandedLockAdvice(objectURL, unlockErr))
		}
		return warnings
	}
	return nil
}

// temporaryProgramDigits is how many random digits follow a temporary
// program's prefix: the same eight the millisecond timestamp used to supply,
// so names keep their shape and length (ZTEMP_EXEC_ + 8 = 19 characters, well
// inside the 30 a program name may have).
const temporaryProgramDigits = 8

// temporaryProgramName returns prefix followed by eight random digits.
//
// The digits come from crypto/rand rather than the clock: two calls in the
// same millisecond — two agents, or one agent's parallel calls — used to get
// the same name, and the second create then met the first one's program.
func temporaryProgramName(prefix string) (string, error) {
	n, err := rand.Int(rand.Reader, big.NewInt(100_000_000))
	if err != nil {
		return "", fmt.Errorf("generating a temporary program name: %w", err)
	}
	return strings.ToUpper(fmt.Sprintf("%s%0*d", prefix, temporaryProgramDigits, n.Int64())), nil
}

// temporaryProgramSlots is how many names each SAP user's ExecuteABAP pool
// holds: as many runs of one user as may be in flight at once, across every
// session and process.
const temporaryProgramSlots = 10

// maxProgramNameLength is the longest name SAP gives a program.
const maxProgramNameLength = 30

// pooledProgramName returns the name of one slot of a user's pool: the prefix,
// the user, an underscore and the slot number, as in ZTEMP_EXEC_DEVELOPER_0.
//
// The user is cut short when the whole would not fit in 30 characters, and
// left out when it is unknown (SSO, a cookie file) or when the prefix leaves
// it no room; the name is then the prefix and the slot alone. Only the
// characters a program name may hold are kept from it.
func pooledProgramName(prefix, user string, slot int) (string, error) {
	prefix = strings.ToUpper(prefix)
	suffix := strconv.Itoa(slot)
	if len(prefix)+len(suffix) > maxProgramNameLength {
		return "", fmt.Errorf("program prefix %q is too long: a program name has at most %d characters, and the slot number needs %d", prefix, maxProgramNameLength, len(suffix))
	}

	user = strings.Map(func(r rune) rune {
		if (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '_' {
			return r
		}
		return -1
	}, strings.ToUpper(user))
	room := maxProgramNameLength - len(prefix) - len("_") - len(suffix)
	if user == "" || room <= 0 {
		return prefix + suffix, nil
	}
	if len(user) > room {
		user = user[:room]
	}
	return prefix + user + "_" + suffix, nil
}

// temporaryPrograms is the set of pool names this client's own runs hold.
//
// SAP already refuses to create a name that exists, and that is what keeps
// two processes apart. This set keeps two calls of one process from even
// trying the same name, so neither one's create runs into the other's lock.
type temporaryPrograms struct {
	mu    sync.Mutex
	inUse map[string]bool
}

func (p *temporaryPrograms) reserve(name string) bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.inUse[name] {
		return false
	}
	if p.inUse == nil {
		p.inUse = make(map[string]bool)
	}
	p.inUse[name] = true
	return true
}

func (p *temporaryPrograms) release(name string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	delete(p.inUse, name)
}

// createTemporaryProgram creates the program of one ExecuteABAP run under the
// first free name of the caller's pool, and returns the name together with
// the function that gives it back to this process's pool.
//
// The names are stable on purpose. The ICF keeps an entry in shared memory
// for every distinct URL it has served, and a random name per run made each
// run a new URL. On a busy system those entries filled the shared memory
// area until unrelated OData services failed with SYSTEM_NO_SHM_MEMORY. A
// pool of ten names per user costs at most ten entries per user, however
// many times it runs.
//
// The create is the arbiter. SAP refuses a name that already exists, so a
// run in progress in another session, or a program that a failed cleanup or
// keep_program left behind, sends this run on to the next slot. The lock
// cannot play that part: a run releases it once the source is written and
// activates and tests without it, so a free lock does not mean a free name.
//
// Any other failure ends the search under that slot's name, as it did with
// a random name. Trying the next slot could leave a second half-created
// program behind.
func (c *Client) createTemporaryProgram(ctx context.Context, prefix, description string) (string, func(), error) {
	var taken []string
	for slot := 0; slot < temporaryProgramSlots; slot++ {
		name, err := pooledProgramName(prefix, c.config.Username, slot)
		if err != nil {
			return "", nil, err
		}
		if !c.tempPrograms.reserve(name) {
			taken = append(taken, name)
			continue
		}
		err = c.CreateObject(ctx, CreateObjectOptions{
			ObjectType:  ObjectTypeProgram,
			Name:        name,
			Description: description,
			PackageName: "$TMP",
			// A failed create must not delete an object it cannot show is its own.
			leavePartialObject: true,
		})
		if err == nil {
			return name, func() { c.tempPrograms.release(name) }, nil
		}
		c.tempPrograms.release(name)
		if !temporaryNameTaken(err) {
			return name, nil, err
		}
		taken = append(taken, name)
	}
	return "", nil, fmt.Errorf("all %d temporary program names are in use (%s): other runs hold them, or a failed cleanup or keep_program left them in $TMP; delete the ones no run is using", temporaryProgramSlots, strings.Join(taken, ", "))
}

// temporaryNameTaken reports a create that failed because the name belongs to
// someone else. Either the program already exists, or another session holds
// the lock on it: CreateObject retried past that lock, then found the
// program there and left it alone.
func temporaryNameTaken(err error) bool {
	if isAlreadyExistsError(err) {
		return true
	}
	var pce *PartialCreateError
	return errors.As(err, &pce) && pce.LeftInPlace && isLockConflictError(pce.OriginalErr)
}

// Lean is the result as execute_abap and `vsp execute --json` report it: every
// field kept, except that the raw alerts are dropped once a value came back.
// They are then the same value again, wrapped in SAP's sentence and a stack —
// tokens a caller pays for and never reads. When no value came back they are
// the only evidence of what happened, and stay.
func (r ExecuteABAPResult) Lean() ExecuteABAPResult {
	if len(r.Output) > 0 {
		r.RawAlerts = nil
	}
	return r
}

// resultText is the result_text a caller reads: one value as itself, several
// as an array in the order the code returned them, none as nothing.
func resultText(output []string) any {
	switch len(output) {
	case 0:
		return nil
	case 1:
		return output[0]
	default:
		return append([]string(nil), output...)
	}
}

// alertExecResult is the value an alert hands back, if it is the closing
// assertion. One alert is one value: SAP puts the message in the title, and a
// detail is only read when the title does not carry it, so a value that is
// repeated there is not counted twice.
func alertExecResult(alert UnitTestAlert) (string, bool) {
	if output, found := execResult(alert.Title); found {
		return output, true
	}
	for _, detail := range alert.Details {
		if output, found := execResult(detail); found {
			return output, true
		}
	}
	return "", false
}

func appendExecuteCleanupWarning(result *ExecuteABAPResult, warning string) {
	if result.Message != "" {
		result.Message += " "
	}
	result.Message += "Cleanup warning: " + warning
}

// executeWrapperSource builds the throwaway report that the payload runs inside.
//
// It is a function rather than an inline literal because two other things
// depend on its exact shape — the marker that ends the run and hands a value
// back, and the line the payload starts on — and a template nobody can call is
// a template nobody can test.
func executeWrapperSource(programName, riskLevel, returnVariable, code string) string {
	return fmt.Sprintf(`REPORT %[1]s.

*& Generated by vsp ExecuteABAP: the code below runs inside a unit test.

CLASS ltc_executor DEFINITION FOR TESTING %[2]s DURATION SHORT.
  PUBLIC SECTION.
    METHODS execute_payload FOR TESTING.
  PRIVATE SECTION.
    DATA mv_vsp_returned TYPE abap_bool.
    METHODS return_value IMPORTING value TYPE any.
ENDCLASS.

CLASS ltc_executor IMPLEMENTATION.
  METHOD execute_payload.
    DATA %[3]s TYPE string.

    %[4]s
%[5]s
    " === USER CODE END ===

    " Hand back the return variable: always when RETURN_VALUE( ) was never
    " called, and otherwise only when it was set.
    IF mv_vsp_returned = abap_false OR %[3]s IS NOT INITIAL.
      cl_abap_unit_assert=>fail( msg = |%[6]s{ %[3]s }| ).
    ENDIF.
  ENDMETHOD.

  " RETURN_VALUE( x ) hands one value back at once, without leaving the
  " method, so a RETURN, CHECK or exception after it cannot lose it.
  METHOD return_value.
    DATA lv_vsp_text TYPE string.
    DATA lr_vsp_data TYPE REF TO data.
    DATA lo_vsp_object TYPE REF TO object.
    FIELD-SYMBOLS <lv_vsp_elem> TYPE simple.
    FIELD-SYMBOLS <lv_vsp_any> TYPE any.
    mv_vsp_returned = abap_true.
    DATA(lo_vsp_type) = cl_abap_typedescr=>describe_by_data( value ).
    CASE lo_vsp_type->kind.
      WHEN cl_abap_typedescr=>kind_elem.
        ASSIGN value TO <lv_vsp_elem>.
        lv_vsp_text = |{ <lv_vsp_elem> }|.
      WHEN cl_abap_typedescr=>kind_ref.
        CASE CAST cl_abap_refdescr( lo_vsp_type )->get_referenced_type( )->kind.
          WHEN cl_abap_typedescr=>kind_class OR cl_abap_typedescr=>kind_intf.
            lo_vsp_object = value.
            IF lo_vsp_object IS BOUND.
              lv_vsp_text = |<object { cl_abap_typedescr=>describe_by_object_ref( lo_vsp_object )->get_relative_name( ) }>|.
            ELSE.
              lv_vsp_text = '<initial reference>'.
            ENDIF.
          WHEN OTHERS.
            " A data reference hands back what it points to. A reference to
            " a reference is followed in a bounded loop, so a chain that
            " points back at itself cannot recurse without end.
            lr_vsp_data = value.
            DO 16 TIMES.
              IF lr_vsp_data IS NOT BOUND.
                EXIT.
              ENDIF.
              ASSIGN lr_vsp_data->* TO <lv_vsp_any>.
              DATA(lo_vsp_inner) = cl_abap_typedescr=>describe_by_data( <lv_vsp_any> ).
              IF lo_vsp_inner->kind <> cl_abap_typedescr=>kind_ref.
                return_value( <lv_vsp_any> ).
                RETURN.
              ENDIF.
              DATA(lv_vsp_inner_kind) = CAST cl_abap_refdescr( lo_vsp_inner )->get_referenced_type( )->kind.
              IF lv_vsp_inner_kind = cl_abap_typedescr=>kind_class OR lv_vsp_inner_kind = cl_abap_typedescr=>kind_intf.
                return_value( <lv_vsp_any> ).
                RETURN.
              ENDIF.
              lr_vsp_data = <lv_vsp_any>.
            ENDDO.
            IF lr_vsp_data IS NOT BOUND.
              lv_vsp_text = '<initial reference>'.
            ELSE.
              lv_vsp_text = '<reference chain too deep or cyclic>'.
            ENDIF.
        ENDCASE.
      WHEN OTHERS.
        " Structures and tables come back as JSON.
        TRY.
            DATA(lo_vsp_json) = cl_sxml_string_writer=>create( type = if_sxml=>co_xt_json ).
            CALL TRANSFORMATION id SOURCE value = value RESULT XML lo_vsp_json.
            lv_vsp_text = cl_abap_codepage=>convert_from( lo_vsp_json->get_output( ) ).
            " id wraps the value as {"VALUE":...}; hand back only the value.
            IF strlen( lv_vsp_text ) > 10 AND lv_vsp_text(9) = '{"VALUE":'.
              lv_vsp_text = substring( val = lv_vsp_text off = 9 len = strlen( lv_vsp_text ) - 10 ).
            ENDIF.
          CATCH cx_root INTO DATA(lx_vsp_json).
            lv_vsp_text = |<not serializable: { lx_vsp_json->get_text( ) }>|.
        ENDTRY.
    ENDCASE.
    cl_abap_unit_assert=>fail( msg = |%[6]s{ lv_vsp_text }| quit = if_aunit_constants=>no ).
  ENDMETHOD.
ENDCLASS.
`, programName, riskLevel, returnVariable, payloadStartMarker, code, execResultMarker)
}

// PayloadFailure picks the alert that says the executed code died rather than
// finished, and returns nil when nothing did.
//
// The deliberate closing assertion is skipped by its marker; what is left is
// what ABAP Unit caught on the way through — an uncaught exception, a zero
// divide, an assert the payload made itself. Warnings are left alone: they are
// things ABAP Unit noticed, not things that stopped the code.
func PayloadFailure(alerts []UnitTestAlert) *UnitTestAlert {
	var fallback *UnitTestAlert
	for i := range alerts {
		alert := alerts[i]
		if carriesExecResult(alert) {
			continue
		}
		// An exception is unambiguous, so it wins outright. Anything else has to
		// be severe enough to have ended the run, which rules out the
		// "tolerable" and "tolerant" severities SAP uses for the rest.
		if strings.EqualFold(alert.Kind, "exception") {
			return &alerts[i]
		}
		if fallback == nil && (strings.EqualFold(alert.Severity, "critical") || strings.EqualFold(alert.Severity, "fatal")) {
			fallback = &alerts[i]
		}
	}
	return fallback
}

// anyTestMethod reports whether ABAP Unit executed at least one test method.
// A class refused for its risk level comes back with none.
func anyTestMethod(classes []UnitTestClass) bool {
	for _, class := range classes {
		if len(class.TestMethods) > 0 {
			return true
		}
	}
	return false
}

// anyExecResult reports whether any alert is the closing assertion, that is
// whether the payload ran to its end.
func anyExecResult(alerts []UnitTestAlert) bool {
	for _, a := range alerts {
		if carriesExecResult(a) {
			return true
		}
	}
	return false
}

// carriesExecResult reports whether an alert is the closing assertion that
// hands a value back, rather than a failure.
func carriesExecResult(alert UnitTestAlert) bool {
	if _, found := execResult(alert.Title); found {
		return true
	}
	for _, detail := range alert.Details {
		if _, found := execResult(detail); found {
			return true
		}
	}
	return false
}

// execResult pulls the returned value out of an assertion message.
//
// The marker is looked for anywhere in the text rather than at the start,
// because SAP does not hand back the message that was passed to it: it wraps it
// in a sentence of its own and quotes it, so |EXEC_RESULT:42| comes back as
// "Critical Assertion Error: 'EXEC_RESULT:42'". Matching on a prefix is why
// every successful run used to report "no output captured" — the value was
// there the whole time, three words to the right of where anyone looked.
func execResult(text string) (string, bool) {
	before, value, found := strings.Cut(text, execResultMarker)
	if !found {
		return "", false
	}
	// The closing quote belongs to SAP's sentence, not to the value, and only
	// exists when the opening one does.
	if strings.HasSuffix(strings.TrimSpace(before), "'") {
		value = strings.TrimSuffix(value, "'")
	}
	return value, true
}

// payloadOffset returns the line of the generated wrapper that holds the first
// line of the caller's code.
//
// Deriving it from the source rather than counting the template by hand means
// an edit to the wrapper cannot silently start reporting line numbers that are
// off by three.
func payloadOffset(source string) int {
	for i, line := range strings.Split(source, "\n") {
		if strings.Contains(line, payloadStartMarker) {
			// i is zero-based and the payload begins on the next line, so the
			// first line of the caller's code is line i+2 of the wrapper.
			return i + 2
		}
	}
	return 0
}

// payloadLine translates a line in the generated wrapper into a line in what
// the caller wrote.
//
// The stack entry points at the wrapper, because that is what SAP compiled, and
// its uri carries the position as a fragment: ".../source/main#start=18,0".
// Only entries naming the wrapper are usable — a failure raised deeper in SAP
// standard code has frames whose line numbers belong to other programs
// entirely, and reporting one of those as "your line 4" would be a lie.
func payloadLine(alert UnitTestAlert, programName string, offset int) int {
	if offset <= 0 {
		return 0
	}
	for _, frame := range alert.Stack {
		if !strings.EqualFold(frame.Name, programName) {
			continue
		}
		line := uriFragmentLine(frame.URI)
		if line < offset {
			continue
		}
		return line - offset + 1
	}
	return 0
}

// compileFailure turns a refused activation into the failure the caller sees,
// and returns nil when the program activated.
//
// The title is SAP's first error verbatim, because a paraphrase of a syntax
// error is worth strictly less than the syntax error, and the rest are kept as
// details: one bad statement routinely produces three messages, and the second
// is often the one that explains the first.
func compileFailure(activation *ActivationResult, programName string, offset int) *ExecuteFailure {
	if activation == nil || activation.Success {
		return nil
	}

	failure := &ExecuteFailure{Kind: ExecuteFailureSyntax, Severity: "error"}
	messages := activation.ErrorMessages()
	if len(messages) == 0 {
		// Refused without naming a reason. Rare, and still a refusal — the
		// program is inactive either way — so this reports the fact it has and
		// says plainly that the reason is missing rather than inventing one.
		failure.Title = "The program was refused by activation, which gave no reason"
		failure.Details = activation.ProblemLines()
		return failure
	}

	failure.Title = strings.TrimSpace(messages[0].ShortText)
	failure.Line = callerLine(messages[0], programName, offset)
	for _, m := range messages[1:] {
		detail := strings.TrimSpace(m.ShortText)
		if line := callerLine(m, programName, offset); line > 0 {
			detail = fmt.Sprintf("line %d: %s", line, detail)
		}
		failure.Details = append(failure.Details, detail)
	}
	return failure
}

// callerLine translates the position in an activation message into a line of
// what the caller wrote.
//
// Same arithmetic as payloadLine and the same refusal to guess: a message about
// another object, or about the wrapper's own preamble, carries a line number
// that means nothing to whoever wrote the payload, and offering it as "your
// line 2" sends them to a line they never typed.
func callerLine(m ActivationResultMessage, programName string, offset int) int {
	if offset <= 0 || programName == "" {
		return 0
	}
	// The href spells the program in lower case where the checklist spells it in
	// upper, and neither casing is the authoritative one.
	if !strings.Contains(strings.ToUpper(m.Href), strings.ToUpper(programName)) {
		return 0
	}
	line := m.SourceLine()
	if line < offset {
		return 0
	}
	return line - offset + 1
}

// ExecuteABAPMultiple executes ABAP code that returns several values.
// Each call to RETURN_VALUE( ) in the code adds a value to the output.
//
// Every ExecuteABAP run now supports RETURN_VALUE( ), so this is the same call
// under its old name. The macro it used to wrap the code in handed back only
// the first value: cl_abap_unit_assert=>fail leaves the test method by default,
// so the loop that was meant to report the rest never got past its first turn.
//
// Example:
//
//	result, err := client.ExecuteABAPMultiple(ctx, `
//	  SELECT * FROM t000 INTO TABLE @DATA(lt_clients) UP TO 5 ROWS.
//	  LOOP AT lt_clients INTO DATA(ls_client).
//	    RETURN_VALUE( |Client { ls_client-mandt }: { ls_client-mtext }| ).
//	  ENDLOOP.
//	`, nil)
//	// result.Output contains one entry per client
func (c *Client) ExecuteABAPMultiple(ctx context.Context, code string, opts *ExecuteABAPOptions) (*ExecuteABAPResult, error) {
	return c.ExecuteABAP(ctx, code, opts)
}
