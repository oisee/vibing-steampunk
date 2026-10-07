package adt

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
	"unicode/utf8"
)

// Upload transport: a released request's two files, the cofile K<nr>.<SID>
// and the data file R<nr>.<SID>, are written into DIR_TRANS of the connected
// system (cofiles/ and data/), and the request is added to that system's
// import buffer. Nothing is imported: the import stays a human step in STMS.
//
// The ABAP side is ZCL_VSP_TRANSPORT_SERVICE (domain "transport"). It writes
// only through SAP's EPS file layer with the logical directories $TR_COFI and
// $TR_DATA, never overwrites a file that exists, and sends tp exactly one
// command, the literal ADDTOBUFFER, for sy-sysid. Every check made here is
// made again there.

const (
	// TransportUploadMaxBytes caps the two files together.
	TransportUploadMaxBytes = 50 << 20
	// TransportCofileMaxBytes caps the cofile. A cofile is a few lines per
	// export and import step; one this large is not a cofile.
	TransportCofileMaxBytes = 1 << 20
	// transportUploadChunk is the payload of one WebSocket message, before
	// base64. ZADT_VSP parses every message character by character, so a
	// moderate chunk keeps each message well under a second there.
	transportUploadChunk = 256 << 10
	// transportDownloadChunk is what one download message asks for.
	transportDownloadChunk = 512 << 10
)

var (
	cofileNameRe = regexp.MustCompile(`^K([A-Z0-9]{6})\.([A-Z0-9]{3})$`)
	dataNameRe   = regexp.MustCompile(`^R([A-Z0-9]{6})\.([A-Z0-9]{3})$`)
	requestRe    = regexp.MustCompile(`^([A-Z0-9]{3})K([A-Z0-9]{6})$`)

	cofileTargetRe = regexp.MustCompile(`^[A-Z0-9/_]{1,20}(\.[0-9]{3})?$`)
	digitsRe       = regexp.MustCompile(`^[0-9]+$`)
	timestampRe    = regexp.MustCompile(`^[0-9]{14}$`)
	upperLetterRe  = regexp.MustCompile(`^[A-Z]$`)
	cofileStepRe   = regexp.MustCompile(`^[0-3]$`)
)

// TransportFiles is one request's cofile and data file, validated.
type TransportFiles struct {
	// Request is <SID>K<number>, as the file names give it.
	Request    string
	SID        string
	Number     string
	CofileName string
	DataName   string
	Cofile     []byte
	Data       []byte
}

// TransportRequestFromFileNames checks a cofile and a data file name and
// returns the request they belong to. The names are bare file names: no
// directory, no path separator. Both are required, and the number and SID of
// the two must be the same.
func TransportRequestFromFileNames(cofileName, dataName string) (request, sid, number string, err error) {
	if cofileName == "" || dataName == "" {
		return "", "", "", errors.New("both files are required: the cofile K<nr>.<SID> and the data file R<nr>.<SID>")
	}
	c := cofileNameRe.FindStringSubmatch(cofileName)
	if c == nil {
		return "", "", "", fmt.Errorf("cofile name %q is not K<6 alphanum>.<SID> (e.g. K900123.DEV or K9A0ZSA.S4D)", cofileName)
	}
	d := dataNameRe.FindStringSubmatch(dataName)
	if d == nil {
		return "", "", "", fmt.Errorf("data file name %q is not R<6 alphanum>.<SID> (e.g. R900123.DEV or R9A0ZSA.S4D)", dataName)
	}
	if c[1] != d[1] || c[2] != d[2] {
		return "", "", "", fmt.Errorf("%s and %s are not one request's files: number and SID must match", cofileName, dataName)
	}
	return c[2] + "K" + c[1], c[2], c[1], nil
}

// TransportFileNamesForRequest is the cofile and data file name of a request.
func TransportFileNamesForRequest(request string) (cofileName, dataName string, err error) {
	m := requestRe.FindStringSubmatch(strings.ToUpper(strings.TrimSpace(request)))
	if m == nil {
		return "", "", fmt.Errorf("request %q is not <SID>K<6 alphanum>", request)
	}
	return "K" + m[2] + "." + m[1], "R" + m[2] + "." + m[1], nil
}

// ValidateCofile checks that content has the shape of a cofile of a request
// exported from sid, the way STRF_READ_COFILE reads one: lines starting with
// '#' are directives; the first other non-blank line is the header
// "truser trfunction tarsystem step objcount..."; every line after it is a
// step "<system>[.<client>] <function> <retcode> <YYYYMMDDhhmmss> <host> <osuser>".
// An export step of sid must be among them -- a request that was never
// exported from sid has no data file of sid to go with it.
func ValidateCofile(content []byte, sid string) error {
	if len(content) == 0 {
		return errors.New("the cofile is empty")
	}
	if len(content) > TransportCofileMaxBytes {
		return fmt.Errorf("the cofile is %d bytes, over the %d-byte limit for a cofile", len(content), TransportCofileMaxBytes)
	}
	if bytes.IndexByte(content, 0) >= 0 {
		return errors.New("the cofile contains NUL bytes: it is not a cofile (is it the data file?)")
	}
	if !utf8.Valid(content) {
		return errors.New("the cofile is not text")
	}
	for _, r := range string(content) {
		if r < 0x20 && r != '\n' && r != '\r' && r != '\t' {
			return fmt.Errorf("the cofile contains control character 0x%02x: it is not a cofile", r)
		}
	}
	header, steps, export := false, 0, false
	for i, line := range strings.Split(string(content), "\n") {
		line = strings.TrimRight(line, "\r")
		if strings.TrimSpace(line) == "" || strings.HasPrefix(line, "#") {
			continue
		}
		f := strings.Fields(line)
		if !header {
			if err := validateCofileHeader(f); err != nil {
				return fmt.Errorf("cofile line %d (the header): %w", i+1, err)
			}
			header = true
			continue
		}
		if len(f) < 4 {
			return fmt.Errorf("cofile line %d: a step line has at least system, function, return code and time", i+1)
		}
		if !cofileTargetRe.MatchString(f[0]) || len(f[1]) != 1 || !digitsRe.MatchString(f[2]) || !timestampRe.MatchString(f[3]) {
			return fmt.Errorf("cofile line %d is not a step line (<system>[.<client>] <function> <retcode> <YYYYMMDDhhmmss> ...)", i+1)
		}
		steps++
		if strings.SplitN(f[0], ".", 2)[0] == sid && f[1] == "E" {
			export = true
		}
	}
	if !header {
		return errors.New("the cofile has no header line")
	}
	if steps == 0 {
		return errors.New("the cofile records no steps: the request was never exported")
	}
	if !export {
		return fmt.Errorf("the cofile records no export (step E) from %s, the system its name says it comes from", sid)
	}
	return nil
}

// cofileHeaderFields is how many fields a cofile header has at least.
const cofileHeaderFields = 13

func validateCofileHeader(f []string) error {
	// owner, request type, target, step and the nine object counts that
	// STRF_READ_COFILE reads: thirteen fields at least.
	if len(f) < cofileHeaderFields {
		return fmt.Errorf("a header has at least %d fields (owner, request type, target, step and nine object counts); this one has %d", cofileHeaderFields, len(f))
	}
	if !upperLetterRe.MatchString(f[1]) {
		return fmt.Errorf("request type %q is not a single letter", f[1])
	}
	if !cofileTargetRe.MatchString(f[2]) {
		return fmt.Errorf("target %q is not a system", f[2])
	}
	if !cofileStepRe.MatchString(f[3]) {
		return fmt.Errorf("step %q is not one of 0, 1, 2, 3", f[3])
	}
	// Nine object counts follow the step, which is what STRF_READ_COFILE
	// reads; a real header goes on (release, flags, client) and that is not
	// checked.
	for i, n := range f[4:] {
		if i == 9 {
			break
		}
		if !digitsRe.MatchString(n) {
			return fmt.Errorf("object count %q is not a number", n)
		}
	}
	return nil
}

// NewTransportFiles validates a request's two files: the names, the pairing,
// the sizes and the cofile's shape. It does no I/O.
func NewTransportFiles(cofileName string, cofile []byte, dataName string, data []byte) (*TransportFiles, error) {
	request, sid, number, err := TransportRequestFromFileNames(cofileName, dataName)
	if err != nil {
		return nil, err
	}
	if len(data) == 0 {
		return nil, fmt.Errorf("the data file %s is empty", dataName)
	}
	if total := len(cofile) + len(data); total > TransportUploadMaxBytes {
		return nil, fmt.Errorf("the two files are %d bytes together, over the %d-byte (50 MB) limit", total, TransportUploadMaxBytes)
	}
	if err := ValidateCofile(cofile, sid); err != nil {
		return nil, fmt.Errorf("%s: %w", cofileName, err)
	}
	return &TransportFiles{
		Request: request, SID: sid, Number: number,
		CofileName: cofileName, DataName: dataName,
		Cofile: cofile, Data: data,
	}, nil
}

// ReadTransportFiles reads a request's two files from local paths. Each must
// be a regular file, not a symbolic link, within the size limits; the names
// are the paths' base names.
func ReadTransportFiles(cofilePath, dataPath string) (*TransportFiles, error) {
	if cofilePath == "" || dataPath == "" {
		return nil, errors.New("both files are required: the cofile K<nr>.<SID> and the data file R<nr>.<SID>")
	}
	cofileName, dataName := filepath.Base(cofilePath), filepath.Base(dataPath)
	if _, _, _, err := TransportRequestFromFileNames(cofileName, dataName); err != nil {
		return nil, err
	}
	cofile, err := readRegularFile(cofilePath, TransportCofileMaxBytes)
	if err != nil {
		return nil, err
	}
	data, err := readRegularFile(dataPath, TransportUploadMaxBytes)
	if err != nil {
		return nil, err
	}
	return NewTransportFiles(cofileName, cofile, dataName, data)
}

func readRegularFile(path string, limit int64) ([]byte, error) {
	fi, err := os.Lstat(path)
	if err != nil {
		return nil, err
	}
	if fi.Mode()&os.ModeSymlink != 0 {
		return nil, fmt.Errorf("%s is a symbolic link; name the file itself", path)
	}
	if !fi.Mode().IsRegular() {
		return nil, fmt.Errorf("%s is not a regular file", path)
	}
	if fi.Size() > limit {
		return nil, fmt.Errorf("%s is %d bytes, over the %d-byte limit", path, fi.Size(), limit)
	}
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	if int64(len(b)) > limit {
		return nil, fmt.Errorf("%s is %d bytes, over the %d-byte limit", path, len(b), limit)
	}
	return b, nil
}

// --- gates -----------------------------------------------------------------

// CheckTransportUpload runs every policy check an upload of request needs,
// before anything is read or sent: not read-only (stated outright, because
// the read-only operation filter does not cover transport operations),
// transports enabled, transports not read-only, and request inside the
// transport whitelist. With an empty request it checks everything but the
// whitelist, so a caller can refuse before it has even looked at the files.
// It does no I/O.
func (c *Client) CheckTransportUpload(request string) error {
	const op = "UploadTransport"
	request = strings.ToUpper(strings.TrimSpace(request))
	if c.config.Safety.ReadOnly {
		return fmt.Errorf("transport write operation '%s' is blocked: read-only mode enabled", op)
	}
	if err := c.config.Safety.CheckTransport(request, op, true); err != nil {
		return err
	}
	if err := c.checkSafety(OpTransport, op); err != nil {
		return err
	}
	if request != "" && !requestRe.MatchString(request) {
		return fmt.Errorf("request %q is not <SID>K<6 alphanum>", request)
	}
	return nil
}

// CheckTransportBufferRead runs the checks a read of the import buffer, or of
// a request's files, needs: transports enabled (or transportable edits
// allowed, as for every transport read), and request -- when one is named --
// inside the transport whitelist. It does no I/O.
func (c *Client) CheckTransportBufferRead(request, op string) error {
	request = strings.ToUpper(strings.TrimSpace(request))
	if err := c.config.Safety.CheckTransport(request, op, false); err != nil {
		return err
	}
	if request != "" && !requestRe.MatchString(request) {
		return fmt.Errorf("request %q is not <SID>K<6 alphanum>", request)
	}
	return nil
}

// CheckTransportDownload runs the checks a download of a request's files
// needs. A data file can carry table contents, so this is a sensitive read:
// refused under read-only, and only with transports enabled; the whitelist
// applies. It does no I/O.
func (c *Client) CheckTransportDownload(request string) error {
	const op = "DownloadTransportFiles"
	request = strings.ToUpper(strings.TrimSpace(request))
	if c.config.Safety.ReadOnly {
		return fmt.Errorf("operation '%s' is blocked: read-only mode enabled (a data file can carry table contents)", op)
	}
	// The read gate also lets --allow-transportable-edits through; a
	// download needs transports enabled outright.
	if !c.config.Safety.EnableTransports {
		return fmt.Errorf("operation '%s' is blocked: transports not enabled (use --enable-transports or SAP_ENABLE_TRANSPORTS=true)", op)
	}
	return c.CheckTransportBufferRead(request, op)
}

// --- the ZADT_VSP transport domain -----------------------------------------

// TransportService is the part of ZADT_VSP's WebSocket client the transport
// domain needs. *DebugWebSocketClient and *AMDPWebSocketClient have it.
type TransportService interface {
	SendDomainRequest(ctx context.Context, domain, action string, params map[string]any, timeout time.Duration) (*WSResponse, error)
}

const transportDomain = "transport"

// TransportUploadResult is what an upload did.
type TransportUploadResult struct {
	Request    string `json:"request"`
	System     string `json:"system"`
	Client     string `json:"client"`
	CofileName string `json:"cofile"`
	DataName   string `json:"datafile"`
	CofileSize int    `json:"cofileSize"`
	DataSize   int    `json:"dataSize"`
	// FilesWritten says both of this upload's files are in DIR_TRANS, as
	// last confirmed -- by ZADT_VSP, or by reading DIR_TRANS back.
	FilesWritten bool `json:"filesWritten"`
	// CofileState and DataState say what is known of each file this upload
	// wrote: written (in DIR_TRANS), not_written (never written, or taken
	// back), or unknown.
	CofileState string `json:"cofileState"`
	DataState   string `json:"dataState"`
	CofilePath  string `json:"cofilePath,omitempty"`
	DataPath    string `json:"dataPath,omitempty"`
	// Status is the add's outcome as far as this call knows it: pending
	// (the job is released; ask TransportAddStatus), not_added (certain:
	// no job ran, or the files were taken back), or unknown.
	Status string `json:"status"`
	// Job is the background job doing the add, when one was started.
	Job *TransportJob `json:"job,omitempty"`
	// RolledBack says ZADT_VSP confirmed both files written by this upload
	// were deleted again.
	RolledBack bool   `json:"rolledBack,omitempty"`
	Note       string `json:"note"`
}

// TransportTPResult is what tp answered.
type TransportTPResult struct {
	Command    string   `json:"command,omitempty"`
	ReturnCode string   `json:"returnCode,omitempty"`
	Message    string   `json:"message,omitempty"`
	Stdout     []string `json:"stdout,omitempty"`
}

// TransportBufferEntry is one line of an import buffer.
type TransportBufferEntry struct {
	Request    string `json:"request"`
	Client     string `json:"client,omitempty"`
	SourceCli  string `json:"sourceClient,omitempty"`
	Function   string `json:"function,omitempty"`
	Owner      string `json:"owner,omitempty"`
	UModes     string `json:"umodes,omitempty"`
	ReturnCode string `json:"returnCode,omitempty"`
	Step       string `json:"step,omitempty"`
	ImpFlag    string `json:"impflg,omitempty"`
	// Raw is the buffer file's line for the request.
	Raw string `json:"raw,omitempty"`
}

// TransportBufferResult is a read of the connected system's import buffer.
type TransportBufferResult struct {
	System string `json:"system"`
	Client string `json:"client"`
	// Source is the file read, DIR_TRANS/buffer/<SID>; FileExists is false
	// when there is none (nothing was ever queued for the system).
	Source     string                 `json:"source,omitempty"`
	FileExists bool                   `json:"fileExists"`
	Request    string                 `json:"request,omitempty"`
	Total      int                    `json:"total"`
	Entries    []TransportBufferEntry `json:"entries"`
	// Truncated says there were more entries than were returned.
	Truncated bool               `json:"truncated,omitempty"`
	TP        *TransportTPResult `json:"tp,omitempty"`
}

// Contains reports whether request is in the buffer read.
func (b *TransportBufferResult) Contains(request string) (*TransportBufferEntry, bool) {
	for i := range b.Entries {
		if strings.EqualFold(b.Entries[i].Request, request) {
			return &b.Entries[i], true
		}
	}
	return nil, false
}

func transportCall(ctx context.Context, ws TransportService, action string, params map[string]any, timeout time.Duration, out any) error {
	if ws == nil {
		return errors.New("the transport upload needs ZADT_VSP (a WebSocket to the system)")
	}
	resp, err := ws.SendDomainRequest(ctx, transportDomain, action, params, timeout)
	if err != nil {
		return fmt.Errorf("transport.%s: %w", action, err)
	}
	if !resp.Success {
		if resp.Error != nil {
			if resp.Error.Code == "UNKNOWN_DOMAIN" {
				return fmt.Errorf("ZADT_VSP on this system has no transport service: deploy the current ZADT_VSP (vsp install zadt-vsp), which includes ZCL_VSP_TRANSPORT_SERVICE")
			}
			return &TransportServiceError{Action: action, Code: resp.Error.Code, Message: resp.Error.Message}
		}
		return fmt.Errorf("transport.%s failed", action)
	}
	if out == nil {
		return nil
	}
	if err := json.Unmarshal(resp.Data, out); err != nil {
		return fmt.Errorf("transport.%s: unreadable answer: %w", action, err)
	}
	return nil
}

// The tp step does not run in the ZADT_VSP session: tp is started over
// synchronous RFC, which an ABAP Push Channel may not do. add_to_buffer
// schedules background job ZVSP_TRANSPORT_BUFFER and answers at once with
// its number; the outcome is read with add_status (TransportAddStatus), which
// only says "queued" when the buffer file has the request.

// Outcomes of an add, as TransportAddStatus reports them.
const (
	// TransportQueued: the buffer file has the request and the job is done.
	TransportQueued = "queued"
	// TransportPending: the job has not finished.
	TransportPending = "pending"
	// TransportJobFailed: the job ended (finished or cancelled) and the
	// buffer file does not have the request.
	TransportJobFailed = "job_failed"
	// TransportNotAdded: certainly not added, nothing was queued by this
	// upload (no job was started, or its files were taken back).
	TransportNotAdded = "not_added"
	// TransportUnknown: not established -- check STMS and SM37.
	TransportUnknown = "unknown"
)

var transportPollInterval = 2 * time.Second

type transportJobStarted struct {
	Status   string `json:"status"`
	Ticket   string `json:"ticket"`
	Job      string `json:"job"`
	JobCount string `json:"job_count"`
	Request  string `json:"request"`
}

// TransportJob is the background job that adds a request to the buffer.
type TransportJob struct {
	Name  string `json:"name"`
	Count string `json:"count"`
}

// TransportAddStatus is what add_status reports: the outcome, and what it is
// based on.
type TransportAddStatus struct {
	Request  string `json:"request"`
	System   string `json:"system"`
	Outcome  string `json:"outcome"`
	Job      string `json:"job,omitempty"`
	JobCount string `json:"jobCount,omitempty"`
	JobFound bool   `json:"jobFound"`
	// JobTied says the job is this request's: its log or its variant
	// names the request.
	JobTied bool `json:"jobTied"`
	// JobRequest is the request the job's log or variant names.
	JobRequest    string   `json:"jobRequest,omitempty"`
	JobStatus     string   `json:"jobStatus,omitempty"`
	InBuffer      bool     `json:"inBuffer"`
	BufferError   string   `json:"bufferError,omitempty"`
	CofilePresent bool     `json:"cofilePresent"`
	DataPresent   bool     `json:"dataPresent"`
	JobLog        []string `json:"jobLog,omitempty"`
	// Push is what the job itself published when it finished, if it reached
	// this connection.
	Push *TransportPush `json:"push,omitempty"`
	Note string         `json:"note,omitempty"`
}

// TransportPush is the outcome the background job publishes (AMC
// ZVSP_TRANSPORT /buffer) to the WebSocket that started the upload.
type TransportPush struct {
	Request    string `json:"request"`
	JobCount   string `json:"job_count"`
	Outcome    string `json:"outcome"`
	Code       string `json:"code,omitempty"`
	Message    string `json:"message,omitempty"`
	TPCommand  string `json:"tp_command,omitempty"`
	TPRC       string `json:"tp_rc,omitempty"`
	TPMessage  string `json:"tp_message,omitempty"`
	InBuffer   bool   `json:"in_buffer"`
	RolledBack bool   `json:"rolled_back"`
}

// TransportPusher is a WebSocket client that receives ZADT_VSP's pushes.
// *DebugWebSocketClient and *AMDPWebSocketClient are.
type TransportPusher interface {
	PushEnabled() bool
	AwaitPush(ctx context.Context, id string) (*WSResponse, error)
	TakePush(id string) (*WSResponse, bool)
}

// transportPushID is the id of the push the job sends for jobCount.
func transportPushID(jobCount string) string { return "push:transport:" + jobCount }

func decodeTransportPush(r *WSResponse) *TransportPush {
	if r == nil {
		return nil
	}
	var p TransportPush
	if err := json.Unmarshal(r.Data, &p); err != nil {
		return nil
	}
	return &p
}

// transportSafetyPoll is how often a wait for the push also asks
// add_status, in case the job ended without publishing (a dump, a kill).
var transportSafetyPoll = 15 * time.Second

// Terminal says the outcome will not change by waiting.
func (s *TransportAddStatus) Terminal() bool {
	return s.Outcome == TransportQueued || s.Outcome == TransportJobFailed
}

type transportStatusAnswer struct {
	Request       string   `json:"request"`
	System        string   `json:"system"`
	Outcome       string   `json:"outcome"`
	Job           string   `json:"job"`
	JobCount      string   `json:"job_count"`
	JobFound      bool     `json:"job_found"`
	JobTied       bool     `json:"job_tied"`
	JobRequest    string   `json:"job_request"`
	JobStatus     string   `json:"job_status"`
	InBuffer      bool     `json:"in_buffer"`
	BufferError   string   `json:"buffer_error"`
	CofilePresent bool     `json:"cofile_present"`
	DataPresent   bool     `json:"data_present"`
	JobLog        []string `json:"job_log"`
}

// TransportAddStatus reads the outcome of adding request to the buffer by
// background job jobCount (empty: the buffer alone). It changes nothing.
func (c *Client) TransportAddStatus(ctx context.Context, ws TransportService, request, jobCount string) (*TransportAddStatus, error) {
	request = strings.ToUpper(strings.TrimSpace(request))
	if err := c.CheckTransportBufferRead(request, "TransportAddStatus"); err != nil {
		return nil, err
	}
	if !requestRe.MatchString(request) {
		return nil, fmt.Errorf("request %q is not <SID>K<6 digits>", request)
	}
	params := map[string]any{"request": request}
	if jobCount = strings.TrimSpace(jobCount); jobCount != "" {
		params["job"] = jobCount
	}
	var a transportStatusAnswer
	if err := transportCall(ctx, ws, "add_status", params, time.Minute, &a); err != nil {
		return nil, err
	}
	st := &TransportAddStatus{Request: request, System: a.System, Outcome: a.Outcome, Job: a.Job, JobCount: a.JobCount,
		JobFound: a.JobFound, JobTied: a.JobTied, JobRequest: a.JobRequest, JobStatus: strings.TrimSpace(a.JobStatus), InBuffer: a.InBuffer, BufferError: a.BufferError,
		CofilePresent: a.CofilePresent, DataPresent: a.DataPresent, JobLog: a.JobLog}
	if st.JobCount == "" {
		st.JobCount = jobCount
	}
	// Without a buffer read there is no verdict either way; nor from a job
	// that is not shown to be this request's.
	if st.BufferError != "" || (st.JobCount != "" && !st.JobTied) {
		st.Outcome = TransportUnknown
	}
	if st.Job == "" && st.JobCount != "" {
		st.Job = transportJobName
	}
	// "queued" is believed only with the buffer file behind it.
	if st.Outcome == TransportQueued && !st.InBuffer {
		st.Outcome = TransportUnknown
	}
	if p, ok := ws.(TransportPusher); ok && st.JobCount != "" {
		if r, ok := p.TakePush(transportPushID(st.JobCount)); ok {
			st.Push = decodeTransportPush(r)
		}
	}
	st.Note = transportOutcomeNote(st.Request, st.System, st.Outcome, st.Job, st.JobCount)
	return st, nil
}

// WaitTransportAdd waits until the add by job jobCount has a final outcome,
// or ctx ends. When ws receives pushes, it waits for the one the job
// publishes when it is done -- and asks add_status every so often in case
// the job ended without publishing; otherwise it asks add_status every few
// seconds. The outcome is always add_status's: a push alone does not make a
// request "queued". If the connection drops while waiting, reconnect (when
// given) supplies another for the status calls. When ctx ends first, the
// last status is returned with the outcome unknown.
func (c *Client) WaitTransportAdd(ctx context.Context, ws TransportService, request, jobCount string,
	reconnect func(context.Context) (TransportService, error)) (*TransportAddStatus, error) {
	p, ok := ws.(TransportPusher)
	if !ok || !p.PushEnabled() || jobCount == "" {
		return c.pollTransportAdd(ctx, ws, request, jobCount, nil, transportPollInterval)
	}
	type got struct {
		r   *WSResponse
		err error
	}
	pctx, cancel := context.WithCancel(ctx)
	defer cancel()
	pushed := make(chan got, 1)
	go func() {
		r, err := p.AwaitPush(pctx, transportPushID(jobCount))
		pushed <- got{r, err}
	}()
	var last *TransportAddStatus
	safety := time.NewTicker(transportSafetyPoll)
	defer safety.Stop()
	for {
		select {
		case g := <-pushed:
			switch {
			case g.err == nil:
				// The job is done; the status call confirms (the job may
				// still be ending, so ask until it says so).
				return c.pollTransportAdd(ctx, ws, request, jobCount, decodeTransportPush(g.r), time.Second)
			case errors.Is(g.err, ErrWebSocketClosed):
				if reconnect == nil {
					return unknownAfterWait(last, request, jobCount), g.err
				}
				ws2, err := reconnect(ctx)
				if err != nil {
					return unknownAfterWait(last, request, jobCount), fmt.Errorf("the connection dropped while waiting, and no new one: %w", err)
				}
				return c.pollTransportAdd(ctx, ws2, request, jobCount, nil, transportPollInterval)
			default:
				return unknownAfterWait(last, request, jobCount), g.err
			}
		case <-safety.C:
			if st, err := c.TransportAddStatus(ctx, ws, request, jobCount); err == nil {
				last = st
				if st.Terminal() {
					return st, nil
				}
			}
		case <-ctx.Done():
			return unknownAfterWait(last, request, jobCount), ctx.Err()
		}
	}
}

// pollTransportAdd asks add_status every interval until the outcome is final
// or ctx ends.
func (c *Client) pollTransportAdd(ctx context.Context, ws TransportService, request, jobCount string,
	push *TransportPush, interval time.Duration) (*TransportAddStatus, error) {
	var last *TransportAddStatus
	for {
		st, err := c.TransportAddStatus(ctx, ws, request, jobCount)
		var se *TransportServiceError
		if errors.As(err, &se) {
			// A refusal (the job is another request's, say) does not
			// change by asking again.
			out := unknownAfterWait(last, request, jobCount)
			out.Push = push
			return out, err
		}
		if err == nil {
			if st.Push == nil {
				st.Push = push
			}
			last = st
			if st.Terminal() {
				return st, nil
			}
		}
		select {
		case <-ctx.Done():
			out := unknownAfterWait(last, request, jobCount)
			if out.Push == nil {
				out.Push = push
			}
			return out, ctx.Err()
		case <-time.After(interval):
		}
	}
}

func unknownAfterWait(last *TransportAddStatus, request, jobCount string) *TransportAddStatus {
	if last == nil {
		last = &TransportAddStatus{Request: strings.ToUpper(request), JobCount: jobCount, Job: transportJobName}
	}
	out := *last
	out.Outcome = TransportUnknown
	out.Note = transportOutcomeNote(out.Request, out.System, out.Outcome, out.Job, out.JobCount)
	return &out
}

const transportJobName = "ZVSP_TRANSPORT_BUFFER"

// transportOutcomeNote says what an outcome means, without claiming more.
func transportOutcomeNote(request, system, outcome, job, jobCount string) string {
	if job == "" {
		job = transportJobName
	}
	jobRef := job + " " + jobCount
	if jobCount == "" {
		jobRef = "the latest " + job + " (its number never arrived)"
	}
	where := fmt.Sprintf("check the import queue in STMS and job %s in SM37", jobRef)
	switch outcome {
	case TransportQueued:
		return fmt.Sprintf("%s is in the import queue of %s and has NOT been imported. Import it, if at all, in STMS.", request, system)
	case TransportPending:
		return fmt.Sprintf("the add is still running; ask again later (vsp transport status %s --job %s), or %s", request, jobCount, where)
	case TransportJobFailed:
		return fmt.Sprintf("%s is not in the import queue: the job ended without adding it (see its log in SM37: %s %s)", request, job, jobCount)
	case TransportNotAdded:
		return fmt.Sprintf("%s was not added to the import queue", request)
	case "not_in_buffer":
		return fmt.Sprintf("%s is not in the import queue", request)
	}
	return fmt.Sprintf("whether %s is in the import queue is unknown -- %s", request, where)
}

// States of one of an upload's files.
const (
	FileWritten    = "written"
	FileNotWritten = "not_written"
	FileUnknown    = "unknown"
)

// setFiles records both files' states and derives FilesWritten from them.
func (r *TransportUploadResult) setFiles(cofile, data string) {
	r.CofileState, r.DataState = cofile, data
	r.FilesWritten = cofile == FileWritten && data == FileWritten
}

// observeFiles reads back, read-only, whether the upload's two files are in
// DIR_TRANS -- for when ZADT_VSP could not say (its answer was lost, it
// failed, or its cleanup was incomplete). A file there is this upload's:
// the upload refused to start while either existed, and wrote under the
// request's lock. If the read fails, both are unknown.
func (c *Client) observeFiles(ctx context.Context, ws TransportService, res *TransportUploadResult) {
	octx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 30*time.Second)
	defer cancel()
	st, err := c.TransportAddStatus(octx, ws, res.Request, "")
	if err != nil {
		res.setFiles(FileUnknown, FileUnknown)
		return
	}
	state := func(present bool) string {
		if present {
			return FileWritten
		}
		return FileNotWritten
	}
	res.setFiles(state(st.CofilePresent), state(st.DataPresent))
}

// TransportServiceError is a refusal or failure reported by
// ZCL_VSP_TRANSPORT_SERVICE.
type TransportServiceError struct {
	Action, Code, Message string
}

func (e *TransportServiceError) Error() string {
	return fmt.Sprintf("transport.%s: %s: %s", e.Action, e.Code, e.Message)
}

type transportBeginAnswer struct {
	AssemblyID string `json:"assembly_id"`
	Request    string `json:"request"`
	System     string `json:"system"`
	Client     string `json:"client"`
}

type transportCommitAnswer struct {
	Request    string `json:"request"`
	CofilePath string `json:"cofile_path"`
	DataPath   string `json:"data_path"`
	CofileSize int    `json:"cofile_size"`
	DataSize   int    `json:"data_size"`
}

type transportBufferAnswer struct {
	System     string `json:"system"`
	Client     string `json:"client"`
	Total      int    `json:"total"`
	Truncated  bool   `json:"truncated"`
	Source     string `json:"source"`
	FileExists bool   `json:"file_exists"`
	Entries    []struct {
		Request    string `json:"trkorr"`
		Client     string `json:"tarcli"`
		SourceCli  string `json:"srccli"`
		Function   string `json:"trfunction"`
		Owner      string `json:"owner"`
		UModes     string `json:"umodes"`
		ReturnCode string `json:"retcode"`
		Step       string `json:"step"`
		ImpFlag    string `json:"impflg"`
		Raw        string `json:"raw"`
	} `json:"entries"`
	Command    string   `json:"tp_command"`
	ReturnCode string   `json:"tp_rc"`
	Message    string   `json:"tp_message"`
	Stdout     []string `json:"stdout"`
}

// sameClient compares two clients, an empty one being the default 001.
func sameClient(a, b string) bool {
	norm := func(c string) string {
		if c = strings.TrimSpace(c); c == "" {
			return "001"
		}
		return c
	}
	return norm(a) == norm(b)
}

// UploadTransport writes files into DIR_TRANS of the connected system and has
// the request added to that system's import buffer. It never imports.
//
// Order: the gates; begin (ZADT_VSP re-validates the names and sizes, checks
// that neither file exists and answers which system and client it is); a
// check that this is the client the client was configured for; the chunks;
// commit (SHA-256 and size of each file, cofile shape, write data file then
// cofile, and on any failure delete what this commit wrote); and add_to_buffer,
// which releases background job ZVSP_TRANSPORT_BUFFER and answers at once.
// The result's Status is "pending" with the job; TransportAddStatus or
// WaitTransportAdd tell the outcome. There is no fixed wait here.
func (c *Client) UploadTransport(ctx context.Context, ws TransportService, files *TransportFiles) (*TransportUploadResult, error) {
	if files == nil {
		return nil, errors.New("no files to upload")
	}
	// The files are validated again: a caller may have built the struct.
	checked, err := NewTransportFiles(files.CofileName, files.Cofile, files.DataName, files.Data)
	if err != nil {
		return nil, err
	}
	if err := c.CheckTransportUpload(checked.Request); err != nil {
		return nil, err
	}
	files = checked
	res := &TransportUploadResult{
		Request: files.Request, CofileName: files.CofileName, DataName: files.DataName,
		CofileSize: len(files.Cofile), DataSize: len(files.Data),
	}

	var begin transportBeginAnswer
	if err := transportCall(ctx, ws, "upload_files", map[string]any{
		"step":          "begin",
		"cofile_name":   files.CofileName,
		"data_name":     files.DataName,
		"cofile_size":   len(files.Cofile),
		"data_size":     len(files.Data),
		"cofile_sha256": sha256Hex(files.Cofile),
		"data_sha256":   sha256Hex(files.Data),
	}, 60*time.Second, &begin); err != nil {
		return nil, err
	}
	res.System, res.Client = begin.System, begin.Client
	abort := func() {
		actx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 30*time.Second)
		defer cancel()
		_ = transportCall(actx, ws, "upload_files", map[string]any{"step": "abort", "assembly_id": begin.AssemblyID}, 30*time.Second, nil)
	}
	if begin.AssemblyID == "" {
		return nil, errors.New("transport.upload_files: ZADT_VSP answered begin without an assembly id")
	}
	if !strings.EqualFold(begin.Request, files.Request) {
		abort()
		return nil, fmt.Errorf("ZADT_VSP read the files as request %q, not %q; nothing was written", begin.Request, files.Request)
	}
	// The WebSocket goes to the server's own URL and client, so this holds
	// unless something in between routed it elsewhere. If it does not, the
	// files would land in another client's system than the one configured.
	if !sameClient(begin.Client, c.config.Client) {
		abort()
		return nil, fmt.Errorf("ZADT_VSP answered from client %s of %s, but this connection is configured for client %s; nothing was written",
			begin.Client, begin.System, strings.TrimSpace(c.config.Client))
	}

	for _, part := range []struct {
		kind string
		data []byte
	}{{"data", files.Data}, {"cofile", files.Cofile}} {
		for off := 0; off < len(part.data); off += transportUploadChunk {
			end := min(off+transportUploadChunk, len(part.data))
			if err := transportCall(ctx, ws, "upload_files", map[string]any{
				"step":        "chunk",
				"assembly_id": begin.AssemblyID,
				"file":        part.kind,
				"offset":      off,
				"chunk_b64":   base64.StdEncoding.EncodeToString(part.data[off:end]),
			}, 60*time.Second, nil); err != nil {
				abort()
				return nil, fmt.Errorf("sending the %s file at offset %d: %w; nothing was written", part.kind, off, err)
			}
		}
	}

	var commit transportCommitAnswer
	if err := transportCall(ctx, ws, "upload_files", map[string]any{
		"step": "commit", "assembly_id": begin.AssemblyID,
	}, 5*time.Minute, &commit); err != nil {
		// No add was asked for, whatever happened to the files.
		res.Status = TransportNotAdded
		var se *TransportServiceError
		if errors.As(err, &se) && se.Code != "SERVICE_EXCEPTION" && se.Code != "WRITE_FAILED_FILES_LEFT" {
			// ZADT_VSP refused the commit and confirmed that nothing of this
			// upload was left in DIR_TRANS.
			res.setFiles(FileNotWritten, FileNotWritten)
			res.Note = fmt.Sprintf("%s was not added to the import queue; the commit was refused (%s) and nothing was left in DIR_TRANS", files.Request, se.Code)
			return res, err
		}
		// The answer was lost, ZADT_VSP failed, or its cleanup was
		// incomplete: look at DIR_TRANS instead of assuming.
		c.observeFiles(ctx, ws, res)
		res.Note = fmt.Sprintf("%s was not added to the import queue (no add was sent); what is left in DIR_TRANS: cofile %s, data file %s",
			files.Request, res.CofileState, res.DataState)
		return res, err
	}
	res.setFiles(FileWritten, FileWritten)
	res.CofilePath, res.DataPath = commit.CofilePath, commit.DataPath

	var started transportJobStarted
	addErr := transportCall(ctx, ws, "add_to_buffer", map[string]any{"request": files.Request}, time.Minute, &started)
	if addErr != nil {
		var se *TransportServiceError
		switch {
		case errors.As(addErr, &se) && se.Code == "ADD_FAILED_ROLLED_BACK":
			// No job ran, and ZADT_VSP confirmed both files were taken back.
			res.Status, res.RolledBack = TransportNotAdded, true
			res.setFiles(FileNotWritten, FileNotWritten)
		case errors.As(addErr, &se) && (se.Code == "ADD_FAILED_FILES_KEPT" || se.Code == "FILES_MISSING"):
			// No job ran; the files are not all where they were. Look.
			res.Status = TransportNotAdded
			c.observeFiles(ctx, ws, res)
		case errors.As(addErr, &se) && (se.Code == "NOT_UPLOADED" || se.Code == "INVALID_REQUEST"):
			// Refused before any job was scheduled; the files are untouched.
			res.Status = TransportNotAdded
		default:
			// The answer was lost or ZADT_VSP failed: a job may have been
			// scheduled, and the files may have been taken back. Look.
			res.Status = TransportUnknown
			c.observeFiles(ctx, ws, res)
		}
		res.Note = transportOutcomeNote(files.Request, res.System, res.Status, transportJobName, "")
		return res, addErr
	}
	res.Status = TransportPending
	res.Job = &TransportJob{Name: started.Job, Count: started.JobCount}
	if res.Job.Count == "" {
		res.Job.Count = started.Ticket
	}
	res.Note = fmt.Sprintf("the files are in DIR_TRANS and job %s %s is adding %s to the import queue of %s; "+
		"its outcome: vsp transport status %s --job %s (nothing is imported either way)",
		res.Job.Name, res.Job.Count, files.Request, res.System, files.Request, res.Job.Count)
	return res, nil
}

// TransportBuffer reads the connected system's import buffer: ZADT_VSP reads
// the buffer file DIR_TRANS/buffer/<SID> through SAP's EPS read checks -- no
// tp, no job, no database write. With a request, only that request's entries
// are returned. It changes nothing.
func (c *Client) TransportBuffer(ctx context.Context, ws TransportService, request string) (*TransportBufferResult, error) {
	request = strings.ToUpper(strings.TrimSpace(request))
	if err := c.CheckTransportBufferRead(request, "TransportBuffer"); err != nil {
		return nil, err
	}
	params := map[string]any{}
	if request != "" {
		params["request"] = request
	}
	var a transportBufferAnswer
	if err := transportCall(ctx, ws, "show_buffer", params, time.Minute, &a); err != nil {
		return nil, err
	}
	out := &TransportBufferResult{System: a.System, Client: a.Client, Request: request, Total: a.Total, Truncated: a.Truncated,
		Source: a.Source, FileExists: a.FileExists,
		Entries: make([]TransportBufferEntry, 0, len(a.Entries))}
	for _, e := range a.Entries {
		out.Entries = append(out.Entries, TransportBufferEntry{
			Request: strings.TrimSpace(e.Request), Client: strings.TrimSpace(e.Client), SourceCli: strings.TrimSpace(e.SourceCli),
			Function: strings.TrimSpace(e.Function), Owner: strings.TrimSpace(e.Owner), UModes: strings.TrimSpace(e.UModes),
			ReturnCode: strings.TrimSpace(e.ReturnCode), Step: strings.TrimSpace(e.Step), ImpFlag: strings.TrimSpace(e.ImpFlag),
			Raw: strings.TrimSpace(e.Raw),
		})
	}
	return out, nil
}

type transportDownloadAnswer struct {
	Name     string `json:"name"`
	Size     int    `json:"size"`
	Offset   int    `json:"offset"`
	ChunkB64 string `json:"chunk_b64"`
}

// DownloadTransportFiles reads a request's cofile and data file from DIR_TRANS
// of the connected system. It changes nothing.
func (c *Client) DownloadTransportFiles(ctx context.Context, ws TransportService, request string) (*TransportFiles, error) {
	request = strings.ToUpper(strings.TrimSpace(request))
	if err := c.CheckTransportDownload(request); err != nil {
		return nil, err
	}
	cofileName, dataName, err := TransportFileNamesForRequest(request)
	if err != nil {
		return nil, err
	}
	read := func(kind string, limit int) ([]byte, error) {
		var out []byte
		for {
			var a transportDownloadAnswer
			if callErr := transportCall(ctx, ws, "download_files", map[string]any{
				"request": request, "file": kind, "offset": len(out), "length": transportDownloadChunk,
			}, 2*time.Minute, &a); callErr != nil {
				return nil, callErr
			}
			if a.Size > limit {
				return nil, fmt.Errorf("the %s file of %s is %d bytes, over the %d-byte limit", kind, request, a.Size, limit)
			}
			chunk, decErr := base64.StdEncoding.DecodeString(a.ChunkB64)
			if decErr != nil {
				return nil, fmt.Errorf("the %s file of %s: unreadable chunk: %w", kind, request, decErr)
			}
			if a.Offset != len(out) {
				return nil, fmt.Errorf("the %s file of %s: asked for offset %d, got %d", kind, request, len(out), a.Offset)
			}
			out = append(out, chunk...)
			if len(out) >= a.Size {
				if len(out) != a.Size {
					return nil, fmt.Errorf("the %s file of %s: read %d bytes of %d", kind, request, len(out), a.Size)
				}
				return out, nil
			}
			if len(chunk) == 0 {
				return nil, fmt.Errorf("the %s file of %s: no progress at offset %d of %d", kind, request, len(out), a.Size)
			}
		}
	}
	cofile, err := read("cofile", TransportCofileMaxBytes)
	if err != nil {
		return nil, err
	}
	data, err := read("data", TransportUploadMaxBytes)
	if err != nil {
		return nil, err
	}
	return &TransportFiles{
		Request: request, SID: request[:3], Number: request[4:],
		CofileName: cofileName, DataName: dataName, Cofile: cofile, Data: data,
	}, nil
}

func sha256Hex(b []byte) string {
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}
