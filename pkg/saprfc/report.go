package saprfc

import (
	"context"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/oisee/open-rfc-go/rfc"
)

// Running a report is the classic thing ADT cannot do: the APC WebSocket forbids
// SUBMIT (APC_ILLEGAL_STATEMENT), which is why issues about running reports from
// vsp were closed as an architectural limit. Over RFC there is no such ban — the
// XBP BAPIs schedule the report as a background job, TBTCO carries its status, and
// XBP returns the spool it produced.

// ReportParam is one selection-screen parameter or select-option line.
type ReportParam struct {
	Name   string `json:"name"`             // SELNAME, e.g. "P_MATNR" or "S_WERKS"
	Kind   string `json:"kind,omitempty"`   // "P" parameter, "S" select-option (default P)
	Sign   string `json:"sign,omitempty"`   // "I" include (default) or "E" exclude
	Option string `json:"option,omitempty"` // "EQ" (default), "BT", "CP", …
	Low    string `json:"low"`
	High   string `json:"high,omitempty"`
}

// JobRun is the outcome of scheduling a report.
type JobRun struct {
	Report    string        `json:"report"`
	JobName   string        `json:"job_name"`
	JobCount  string        `json:"job_count"`
	Status    string        `json:"status,omitempty"`      // TBTCO STATUS once known
	StatusFor string        `json:"status_text,omitempty"` // human reading of that letter
	Spool     string        `json:"spool,omitempty"`       // spool list, when requested and available
	JobLog    []JobLogEntry `json:"job_log,omitempty"`     // job log, when requested
	// Started says the job has begun. op "run" sets it once
	// BAPI_XBP_JOB_START_ASAP succeeded: from then on an error says nothing
	// about the job, which may be queued or running. op "job" is a poller in
	// another process and has no such flag to inherit, so it derives Started
	// from the TBTCO status letter instead. The two are not the same fact --
	// code that needs "has it begun" must read the letter (JobStarted).
	Started bool `json:"started"`
	// StartStamp is TBTCO's start stamp (STRTDATE/STRTTIME) exactly as the
	// system wrote it, "YYYYMMDD HHMMSS". The poller reads it alongside the
	// status letter and it is the system's own clock, not an instant in this
	// process's zone, so it is carried verbatim and never parsed into a time --
	// a typed instant would be read as UTC and be wrong by the offset. Empty
	// when the job has not begun, or on a system that leaves the columns blank.
	StartStamp string `json:"start_stamp,omitempty"`
	// SpoolTruncated says Spool holds only the first part of the list.
	SpoolTruncated bool `json:"spool_truncated,omitempty"`
}

// jobStatusText turns a TBTCO status letter into words.
func jobStatusText(s string) string {
	switch strings.ToUpper(strings.TrimSpace(s)) {
	case "P":
		return "scheduled"
	case "S":
		return "released"
	case "R":
		return "running"
	case "F":
		return "finished"
	case "A":
		return "cancelled"
	case "Y":
		return "ready"
	case "":
		return ""
	}
	return "unknown"
}

// statusLetter normalises a TBTCO letter once, so the predicates below cannot
// disagree about what "r", " A " or "" mean.
func statusLetter(s string) string { return strings.ToUpper(strings.TrimSpace(s)) }

// startedLetters are the letters that mean the job has begun: a job that is over
// has begun, so F and A are in it. notEndedLetters are the letters a wait loop
// keeps waiting on: the job has not begun (P, S, Y) or it is running (R).
// Anything else is terminal, including a letter this code does not know -- which
// is what the wait loop meant when it spelled the same four letters as one
// string.
var (
	startedLetters  = []string{"R", "F", "A"}
	notEndedLetters = []string{"P", "S", "R", "Y"}
)

// JobStarted reports whether a TBTCO status letter says the job has begun.
// R (active), F (finished) and A (cancelled) all mean it ran; P (scheduled),
// S (released) and Y (ready) mean it has not.
//
// A caller that learns a job's status later -- op "job" polls one that another
// process started -- has only this letter to go on. A flag held in the process
// that scheduled the job is true there and false everywhere else, which is how
// a running job came to be reported as "started": false.
func JobStarted(s string) bool { return slices.Contains(startedLetters, statusLetter(s)) }

// PolledJobStatus is what a poller reads of a job another process started: the
// TBTCO status letter, and the start stamp that comes with it. It carries no
// reading of the letter, because that is derived from the letter in one place
// (jobStatusText) and a second copy could only disagree with it.
type PolledJobStatus struct {
	Status     string // TBTCO STATUS: P, S, Y, R, F or A; "" when TBTCO has no such row
	StartStamp string // STRTDATE/STRTTIME as written, "" until the job has begun
}

// PolledJob is the JobRun a poller reports for a job another process started.
//
// Building it here rather than at the call site is the point. Started means one
// thing in the process that scheduled the job -- XBP accepted the start -- and
// another in a poller, which has only the status letter. While the poller built
// its JobRun as a literal it could forget Started entirely, and did: a job SM37
// showed as Active came back as "started": false (#353). A constructor that
// fills Started and StatusFor from the one letter, and takes the start stamp
// the same read returned, is a place that cannot be forgotten.
func PolledJob(jobName, jobCount string, st PolledJobStatus) *JobRun {
	return &JobRun{
		JobName:    strings.ToUpper(strings.TrimSpace(jobName)),
		JobCount:   jobCount,
		Status:     st.Status,
		StatusFor:  jobStatusText(st.Status),
		Started:    JobStarted(st.Status),
		StartStamp: st.StartStamp,
	}
}

// RunReport runs a report as a background job through the XBP interface.
//
// SUBST_START_REPORT_IN_BATCH looks like the obvious call, but it picks a batch
// server itself and fails with BATCH_SCHEDULING_FAILED (XM262) on systems where
// that selection does not resolve — a containerised A4H, for one, even with
// SAP_ALL and free batch work processes. The XBP BAPIs are the supported external
// scheduler interface: they take the target server explicitly, so they work where
// SUBST does not, and they carry a BAPIRET2 that says what went wrong.
func RunReport(ctx context.Context, c *rfc.Client, report, jobName string, params []ReportParam, wait time.Duration) (*JobRun, error) {
	return RunReportWith(ctx, c, ReportRequest{Report: report, JobName: jobName, Params: params, Wait: wait})
}

// ReportRequest is one report run: the report, the job it runs in, and what it
// runs with -- a saved variant, selection values, or both (the values then
// override the variant's).
type ReportRequest struct {
	Report  string
	JobName string // defaults to VSP_<REPORT>
	Variant string
	Params  []ReportParam
	Wait    time.Duration // how long to wait for the job to end; zero returns at once
}

// RunReportWith runs a report as a background job through the XBP interface.
func RunReportWith(ctx context.Context, c *rfc.Client, r ReportRequest) (*JobRun, error) {
	report, jobName, params, wait := strings.ToUpper(strings.TrimSpace(r.Report)), r.JobName, r.Params, r.Wait
	if report == "" {
		return nil, fmt.Errorf("a report name is required")
	}
	if jobName == "" {
		jobName = "VSP_" + report
	}
	if len(jobName) > 32 {
		jobName = jobName[:32]
	}

	run := &JobRun{Report: report, JobName: jobName}
	// The XBP calls share the XMI session BAPI_XMI_LOGON opened, so they run
	// on one pinned connection; a pooled Client.Call may land on another.
	err := withXMI(ctx, c, func(s caller) error {
		opened, err := s.Call(ctx, "BAPI_XBP_JOB_OPEN", rfc.Params{
			"JOBNAME": jobName, "EXTERNAL_USER_NAME": xbpUser,
		})
		if err != nil {
			return fmt.Errorf("BAPI_XBP_JOB_OPEN: %w", err)
		}
		if berr := bapiError("BAPI_XBP_JOB_OPEN", opened.Get("RETURN")); berr != nil {
			return berr
		}
		run.JobCount = strings.TrimSpace(fmt.Sprint(opened.Get("JOBCOUNT")))

		// A job that was opened but never started stays in SM37 as "scheduled"
		// forever. If anything below fails before the start, take it out again
		// -- also when the caller gave up, the likeliest reason it failed.
		defer func() {
			if !run.Started {
				cctx, cancel := cleanupContext(ctx)
				defer cancel()
				_ = deleteJob(cctx, s, jobName, run.JobCount)
			}
		}()

		step := abapStep(jobName, run.JobCount, report, r.Variant, params)
		added, err := s.Call(ctx, "BAPI_XBP_JOB_ADD_ABAP_STEP", step)
		if err != nil {
			return fmt.Errorf("BAPI_XBP_JOB_ADD_ABAP_STEP: %w", err)
		}
		if berr := bapiError("BAPI_XBP_JOB_ADD_ABAP_STEP", added.Get("RETURN")); berr != nil {
			return berr
		}

		server, err := applicationServer(ctx, s)
		if err != nil {
			return err
		}
		start, err := s.Call(ctx, "BAPI_XBP_JOB_START_ASAP", rfc.Params{
			"JOBNAME": jobName, "JOBCOUNT": run.JobCount,
			"EXTERNAL_USER_NAME": xbpUser, "TARGET_SERVER": server,
		})
		if err != nil {
			return fmt.Errorf("BAPI_XBP_JOB_START_ASAP: %w", err)
		}
		if berr := bapiError("BAPI_XBP_JOB_START_ASAP", start.Get("RETURN")); berr != nil {
			return berr
		}
		run.Started = true
		return nil
	})
	if err != nil {
		if run.JobCount == "" {
			return nil, err
		}
		return run, err
	}

	if wait <= 0 {
		return run, nil
	}
	deadline := time.Now().Add(wait)
	for {
		st, err := JobStatusDetail(ctx, c, run.JobName, run.JobCount)
		if err != nil {
			return run, fmt.Errorf("job %s / %s started, but %w", run.JobName, run.JobCount, err)
		}
		// The wait loop sees the same letters a poller would, so both the letter
		// and its reading come from the one place that owns that mapping rather
		// than being spelled again here. run.Started is left alone: it is the
		// scheduler's own fact -- XBP accepted the start -- and not the letter's.
		polled := PolledJob(run.JobName, run.JobCount, st)
		run.Status, run.StatusFor = polled.Status, polled.StatusFor
		if run.StartStamp == "" {
			run.StartStamp = polled.StartStamp
		}
		// Not ended: P scheduled, S released, R running, Y ready. Anything else
		// is terminal, as this loop already treated it when the same four
		// letters were spelled as one string.
		if st.Status != "" && !slices.Contains(notEndedLetters, statusLetter(st.Status)) {
			return run, nil
		}
		if time.Now().After(deadline) {
			return run, nil // still running; the caller has the job id to follow up
		}
		select {
		case <-ctx.Done():
			return run, ctx.Err()
		case <-time.After(time.Second):
		}
	}
}

// abapStep is the BAPI_XBP_JOB_ADD_ABAP_STEP input for one report step.
func abapStep(jobName, jobCount, report, variant string, params []ReportParam) rfc.Params {
	step := rfc.Params{
		"JOBNAME": jobName, "JOBCOUNT": jobCount,
		"EXTERNAL_USER_NAME": xbpUser, "ABAP_PROGRAM_NAME": report,
		// Without print parameters a background step writes its list nowhere and
		// TBTCP-LISTIDENT stays 0, so there is nothing to read afterwards.
		// ALLPRIPAR is the classic PRI_PARAMS set: hold the request on the default
		// device rather than printing it.
		"ALLPRIPAR": map[string]any{
			"PDEST": printDestination, // spool device
			"PRIMM": " ",              // do not print immediately
			"PRREL": " ",              // keep the request after printing
			"PEXPI": "8",              // days before it expires
			"LINSZ": 255,
			"LINCT": 65,
		},
	}
	if variant = strings.ToUpper(strings.TrimSpace(variant)); variant != "" {
		step["ABAP_VARIANT_NAME"] = variant
	}
	if rows := selectionRows(params); len(rows) > 0 {
		step["SELINFO"] = rows
	}
	return step
}

// deleteJob removes a job through XBP. It runs inside the caller's XMI session.
func deleteJob(ctx context.Context, c caller, jobName, jobCount string) error {
	res, err := c.Call(ctx, "BAPI_XBP_JOB_DELETE", rfc.Params{
		"JOBNAME": jobName, "JOBCOUNT": jobCount, "EXTERNAL_USER_NAME": xbpUser,
	})
	if err != nil {
		return fmt.Errorf("BAPI_XBP_JOB_DELETE: %w", err)
	}
	return bapiError("BAPI_XBP_JOB_DELETE", res.Get("RETURN"))
}

// DeleteJob removes a job that has not started, or has ended.
func DeleteJob(ctx context.Context, c *rfc.Client, jobName, jobCount string) error {
	return withXMI(ctx, c, func(s caller) error {
		return deleteJob(ctx, s, jobName, jobCount)
	})
}

// xbpUser is the external scheduler identity XBP records against the job.
const xbpUser = "vsp"

// xmiLogon opens the XMI session the XBP BAPIs require.
func xmiLogon(ctx context.Context, c caller) error {
	res, err := c.Call(ctx, "BAPI_XMI_LOGON", rfc.Params{
		"EXTCOMPANY": "vsp", "EXTPRODUCT": "vibing-steampunk", "INTERFACE": "XBP", "VERSION": "3.0",
	})
	if err != nil {
		return fmt.Errorf("BAPI_XMI_LOGON: %w", err)
	}
	return bapiError("BAPI_XMI_LOGON", res.Get("RETURN"))
}

// caller is what the XBP helpers call through: a pooled *rfc.Client, or the
// *rfc.Session an XMI conversation is pinned to.
type caller interface {
	Call(ctx context.Context, functionName string, in rfc.Params) (rfc.Result, error)
}

// withXMI runs fn inside one XMI session on one pinned connection: logon, the
// calls, logoff. The XBP BAPIs need the session BAPI_XMI_LOGON opened, and
// Client.Call takes any pooled connection, so consecutive calls through it
// can run in different SAP sessions.
func withXMI(ctx context.Context, c *rfc.Client, fn func(s caller) error) error {
	s, err := c.Pin(ctx)
	if err != nil {
		return err
	}
	defer s.Close()
	if err := xmiLogon(ctx, s); err != nil {
		return err
	}
	defer func() {
		cctx, cancel := cleanupContext(ctx)
		defer cancel()
		_, _ = s.Call(cctx, "BAPI_XMI_LOGOFF", rfc.Params{"INTERFACE": "XBP"})
	}()
	return fn(s)
}

// cleanupTimeout bounds compensation that must run after the caller gave up.
const cleanupTimeout = 30 * time.Second

// cleanupContext is for compensation -- deleting a job that never started,
// logging off -- that has to happen even when ctx was cancelled.
func cleanupContext(ctx context.Context) (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.WithoutCancel(ctx), cleanupTimeout)
}

// sqlLiteral escapes a value for a quoted RFC_READ_TABLE WHERE literal.
func sqlLiteral(v string) string {
	return strings.ReplaceAll(v, "'", "''")
}

// applicationServer returns this instance's name, which XBP wants as the job's
// target server. RFC_SYSTEM_INFO reports it as RFCDEST (host_SID_nn).
func applicationServer(ctx context.Context, c caller) (string, error) {
	info, err := c.Call(ctx, "RFC_SYSTEM_INFO", nil)
	if err != nil {
		return "", fmt.Errorf("resolving the target server: %w", err)
	}
	m, _ := info.Get("RFCSI_EXPORT").(map[string]any)
	server := strings.TrimSpace(fmt.Sprint(m["RFCDEST"]))
	if server == "" {
		return "", fmt.Errorf("could not determine the application server name")
	}
	return server, nil
}

// selectionRows turns parameters into XBP's SELINFO rows.
func selectionRows(params []ReportParam) []map[string]any {
	rows := make([]map[string]any, 0, len(params))
	for _, p := range params {
		rows = append(rows, map[string]any{
			"SELNAME": strings.ToUpper(p.Name),
			"KIND":    firstNonEmpty(strings.ToUpper(p.Kind), "P"),
			"SIGN":    firstNonEmpty(strings.ToUpper(p.Sign), "I"),
			"OPTION":  firstNonEmpty(strings.ToUpper(p.Option), "EQ"),
			"LOW":     p.Low,
			"HIGH":    p.High,
		})
	}
	return rows
}

// bapiError turns a BAPIRET2 of type E or A into a Go error.
func bapiError(call string, ret any) error {
	m, ok := ret.(map[string]any)
	if !ok {
		return nil
	}
	t := strings.ToUpper(strings.TrimSpace(fmt.Sprint(m["TYPE"])))
	if t != "E" && t != "A" {
		return nil
	}
	msg := strings.TrimSpace(fmt.Sprint(m["MESSAGE"]))
	if msg == "" {
		msg = fmt.Sprintf("%v%v", m["ID"], m["NUMBER"])
	}
	return fmt.Errorf("%s: %s", call, msg)
}

// JobStatus reads one job's TBTCO status letter and its reading in words. It is
// the two-value view of JobStatusDetail, for callers that have no use for the
// start stamp.
func JobStatus(ctx context.Context, c *rfc.Client, jobName, jobCount string) (string, string, error) {
	st, err := JobStatusDetail(ctx, c, jobName, jobCount)
	return st.Status, jobStatusText(st.Status), err
}

// JobStatusDetail reads a job's status letter and the start stamp that goes
// with it, in one TBTCO row. A caller that has to report whether a job has begun
// wants both: the letter says it, the stamp says since when (#353), and asking
// twice could answer between two different states of the job.
func JobStatusDetail(ctx context.Context, c *rfc.Client, jobName, jobCount string) (PolledJobStatus, error) {
	status, stamp, err := jobStatus(ctx, c, jobName, jobCount)
	return PolledJobStatus{Status: status, StartStamp: stamp}, err
}

// jobStatus reads one job's TBTCO status letter and start stamp. The stamp is
// returned as the system wrote it: the columns are the system's own clock and
// the row carries no zone, so parsing them into a time would invent one. A
// blank or half-written pair is no stamp rather than a date with a clock.
func jobStatus(ctx context.Context, c *rfc.Client, jobName, jobCount string) (string, string, error) {
	where := fmt.Sprintf("JOBNAME = '%s' AND JOBCOUNT = '%s'", sqlLiteral(jobName), sqlLiteral(jobCount))
	rows, err := ReadTable(ctx, c, "TBTCO", where, []string{"STATUS", "STRTDATE", "STRTTIME"}, 1)
	if err != nil {
		return "", "", fmt.Errorf("reading job status: %w", err)
	}
	if len(rows) == 0 {
		return "", "", nil
	}
	row := rows[0]
	date, clock := strings.TrimSpace(row["STRTDATE"]), strings.TrimSpace(row["STRTTIME"])
	if len(date) != 8 || len(clock) != 6 {
		return strings.TrimSpace(row["STATUS"]), "", nil
	}
	return strings.TrimSpace(row["STATUS"]), date + " " + clock, nil
}

// printDestination is the spool device a step prints to. LP01 exists on every
// system; the request is held rather than printed.
const printDestination = "LP01"

// ReadSpool returns a finished job's spool list through the XBP BAPIs, which need
// their own XMI session — hence the logon and logoff around the read. A job that
// produced no spool (TBTCP-LISTIDENT is zero) yields an empty string, not an error.
func ReadSpool(ctx context.Context, c *rfc.Client, jobName, jobCount string) (string, error) {
	return ReadSpoolStep(ctx, c, jobName, jobCount, 1)
}

// ReadSpoolStep reads the spool list of one step of a job.
func ReadSpoolStep(ctx context.Context, c *rfc.Client, jobName, jobCount string, step int) (string, error) {
	var res rfc.Result
	err := withXMI(ctx, c, func(s caller) error {
		var err error
		res, err = s.Call(ctx, "BAPI_XBP_JOB_SPOOLLIST_READ", rfc.Params{
			"JOBNAME": jobName, "JOBCOUNT": jobCount, "EXTERNAL_USER_NAME": xbpUser,
			"STEP_NUMBER": step, // XBP requires it
		})
		if err != nil {
			return fmt.Errorf("BAPI_XBP_JOB_SPOOLLIST_READ: %w", err)
		}
		return nil
	})
	if err != nil {
		return "", err
	}
	var b strings.Builder
	for _, row := range res.Table("SPOOL_LIST") {
		for _, key := range []string{"LINE", "SPOOLLIST"} {
			if v, ok := row[key]; ok {
				fmt.Fprintln(&b, strings.TrimRight(fmt.Sprint(v), " "))
				break
			}
		}
	}
	return b.String(), nil
}

func asInt32(v any) int32 {
	switch n := v.(type) {
	case int32:
		return n
	case int64:
		return int32(n)
	case int:
		return int32(n)
	case float64:
		return int32(n)
	}
	return 0
}
