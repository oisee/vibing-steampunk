// Package serve is `vsp snc-serve`, ported from the SNC sidecar's serve-stdio:
// vsp starts it as its transport command (pkg/adt StdioTransport) and speaks
// ADT over the child's stdin/stdout. There is no listener and no port; the
// channel exists only between the parent and this process, and it lives exactly
// as long as the parent keeps stdin open.
//
// Framing, both directions: a 4-byte big-endian length, then one raw HTTP/1.1
// message. Strictly one request in flight. The SDK is loaded only in a worker
// (the same executable, re-run as `snc-serve-worker`) in a guarded runtime
// directory. The first frame the supervisor reads from its worker is a JSON
// readiness report; it is never relayed to the client.
//
// Only GET on the read allowlist (sncrfc/adtread.go) is forwarded. Other requests
// are answered 403 locally and never reach SAP. Any desync, oversized frame,
// timeout or RFC failure stops the server; nothing is retried or restarted.
package serve

import (
	"bufio"
	"bytes"
	"context"
	"encoding/binary"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/oisee/vibing-steampunk/pkg/sncrfc"
)

// logPrefix starts every line this package writes to stderr.
const logPrefix = "vsp snc-serve:"

const (
	maxRequestFrame  = 1 << 20
	maxResponseFrame = 64 << 20
	stoppingHeader   = "X-Vsp-Snc-Stopping"
	refusedHeader    = "X-Vsp-Snc-Refused"
	// ADT over RFC issues no CSRF token, and only GET is forwarded, so CSRF has no
	// meaning on this channel. A client that fetches one gets this fixed value.
	readOnlyCSRFToken = "vsp-snc-read-only"

	// Command and WorkerCommand are the hidden vsp subcommands that run Main
	// and WorkerMain. The supervisor starts its worker as WorkerCommand.
	Command       = "snc-serve"
	WorkerCommand = "snc-serve-worker"
	// workerEnv marks a process started by the supervisor; WorkerMain refuses
	// to run without it.
	workerEnv = "VSP_SNC_WORKER"
)

var errFrameSize = errors.New("frame size out of bounds")

func readFrame(r io.Reader, max int) ([]byte, error) {
	var h [4]byte
	if _, err := io.ReadFull(r, h[:]); err != nil {
		return nil, err // io.EOF only on a clean boundary
	}
	n := binary.BigEndian.Uint32(h[:])
	if n == 0 || n > uint32(max) {
		return nil, errFrameSize
	}
	b := make([]byte, n)
	if _, err := io.ReadFull(r, b); err != nil {
		return nil, io.ErrUnexpectedEOF
	}
	return b, nil
}

func writeFrame(w io.Writer, b []byte) error {
	if len(b) == 0 || len(b) > maxResponseFrame {
		return errFrameSize
	}
	out := make([]byte, 4+len(b))
	binary.BigEndian.PutUint32(out, uint32(len(b)))
	copy(out[4:], b)
	_, err := w.Write(out)
	return err
}

type serveOptions struct {
	requestTimeout time.Duration
	verbose        bool
	dataPreview    bool
}

func parseServeFlags(args []string) (probeFlags, serveOptions, error) {
	var p probeFlags
	var o serveOptions
	f := flag.NewFlagSet(Command, flag.ContinueOnError)
	f.SetOutput(io.Discard)
	defineProbeFlags(f, &p)
	f.DurationVar(&o.requestTimeout, "request-timeout", 60*time.Second, "deadline for one ADT request")
	f.BoolVar(&o.verbose, "verbose", false, "one stderr line per request: method, path, status, size, time")
	f.BoolVar(&o.dataPreview, "allow-data-preview", false, "also forward bounded ADT data preview POSTs (SELECT only, row-capped)")
	if err := f.Parse(args); err != nil || f.NArg() != 0 {
		return p, o, errors.New("invalid " + Command + " arguments")
	}
	if o.requestTimeout < time.Second || o.requestTimeout > 10*time.Minute {
		return p, o, errors.New("request-timeout must be 1s..10m")
	}
	return p, o, validateProbeFlags(p)
}

// adtCall is one checked request. HEAD is forwarded as GET and answered
// without a body.
type adtCall struct {
	req  *http.Request
	body []byte
	head bool
}

// checkRequest parses one framed request and decides whether it may go to SAP.
// A refusal is a complete local HTTP response; the parsed request, when there is
// one, is returned with it for logging.
func checkRequest(b []byte, dataPreview bool) (*adtCall, []byte) {
	req, err := http.ReadRequest(bufio.NewReader(bytes.NewReader(b)))
	if err != nil {
		return nil, localResponse(http.StatusBadRequest, "malformed request", false)
	}
	body, err := io.ReadAll(io.LimitReader(req.Body, maxRequestFrame))
	c := &adtCall{req: req, body: body, head: req.Method == http.MethodHead}
	if err != nil {
		return c, localResponse(http.StatusBadRequest, "malformed request body", false)
	}
	switch {
	case req.Method == http.MethodGet || req.Method == http.MethodHead:
		if len(body) != 0 {
			return c, localResponse(http.StatusForbidden, "snc-serve is read-only: GET bodies are refused", false)
		}
		if !sncrfc.ADTReadAllowed(req.RequestURI) {
			return c, localResponse(http.StatusForbidden, "path is outside the snc-serve read allowlist", false)
		}
	case req.Method == http.MethodPost && dataPreview:
		if !sncrfc.ADTDataPreviewAllowed(req.RequestURI, body) {
			return c, localResponse(http.StatusForbidden, "only a bounded data preview SELECT may be posted", false)
		}
	default:
		return c, localResponse(http.StatusForbidden, "snc-serve is read-only: only GET (and data preview when enabled) is forwarded", false)
	}
	return c, nil
}

func localResponse(status int, text string, stopping bool) []byte {
	r := &http.Response{StatusCode: status, ProtoMajor: 1, ProtoMinor: 1, Header: http.Header{}, ContentLength: int64(len(text)), Body: io.NopCloser(strings.NewReader(text))}
	r.Header.Set("Content-Type", "text/plain; charset=utf-8")
	if stopping {
		r.Header.Set(stoppingHeader, "1")
	} else {
		r.Header.Set(refusedHeader, "1") // answered here; SAP never saw the request
	}
	var buf bytes.Buffer
	_ = r.Write(&buf)
	return buf.Bytes()
}

func adtRequest(c *adtCall) map[string]any {
	req := c.req
	names := make([]string, 0, len(req.Header))
	for k := range req.Header {
		names = append(names, k)
	}
	sort.Strings(names)
	headers := []map[string]any{}
	for _, k := range names {
		if !sncrfc.ADTReadHeaderAllowed(k) || strings.EqualFold(k, "x-csrf-token") {
			continue
		}
		for _, v := range req.Header[k] {
			if len(headers) < 16 && len(v) <= 1024 && !strings.ContainsAny(v, "\r\n\x00") {
				headers = append(headers, map[string]any{"NAME": k, "VALUE": v})
			}
		}
	}
	method, body := "GET", []byte{}
	if req.Method == http.MethodPost {
		method, body = "POST", c.body
	}
	return map[string]any{"REQUEST": map[string]any{
		"REQUEST_LINE":  map[string]any{"METHOD": method, "URI": req.RequestURI, "VERSION": "HTTP/1.1"},
		"HEADER_FIELDS": headers, "MESSAGE_BODY": body,
	}}
}

// Headers that describe the RFC hop or a session are not relayed.
var droppedResponseHeaders = map[string]bool{"content-length": true, "transfer-encoding": true, "connection": true, "keep-alive": true, "set-cookie": true}

func httpResponse(out map[string]any, csrfFetch, head bool) ([]byte, bool) {
	r, ok := out["RESPONSE"].(map[string]any)
	if !ok {
		return nil, false
	}
	line, ok := r["STATUS_LINE"].(map[string]any)
	if !ok {
		return nil, false
	}
	codeText, _ := line["STATUS_CODE"].(string)
	code, err := strconv.Atoi(strings.TrimSpace(codeText))
	if err != nil || code < 100 || code > 599 {
		return nil, false
	}
	body, ok := r["MESSAGE_BODY"].([]byte)
	if !ok {
		return nil, false
	}
	resp := &http.Response{StatusCode: code, ProtoMajor: 1, ProtoMinor: 1, Header: http.Header{}, ContentLength: int64(len(body)), Body: io.NopCloser(bytes.NewReader(body))}
	if reason, _ := line["REASON_PHRASE"].(string); strings.TrimSpace(reason) != "" && !strings.ContainsAny(reason, "\r\n") {
		resp.Status = strconv.Itoa(code) + " " + strings.TrimSpace(reason)
	}
	if hs, ok := r["HEADER_FIELDS"].([]map[string]any); ok {
		for _, h := range hs {
			name, _ := h["NAME"].(string)
			value, _ := h["VALUE"].(string)
			if name == "" || strings.ContainsAny(name, " :\r\n") || strings.ContainsAny(value, "\r\n") || droppedResponseHeaders[strings.ToLower(name)] {
				continue
			}
			resp.Header.Add(name, value)
		}
	}
	if head {
		resp.ContentLength, resp.Body = 0, http.NoBody
	}
	if csrfFetch && resp.Header.Get("X-Csrf-Token") == "" {
		resp.Header.Set("X-Csrf-Token", readOnlyCSRFToken)
	}
	var buf bytes.Buffer
	if resp.Write(&buf) != nil {
		return nil, false
	}
	return buf.Bytes(), true
}

// runServeWorker runs inside the guarded worker: log on, verify identity, report
// readiness, then answer framed requests until stdin closes.
func runServeWorker(ctx context.Context, args []string, in io.Reader, out io.Writer, open func(context.Context, probeFlags) (probeSession, error)) int {
	p, o, err := parseServeFlags(args)
	if err != nil {
		return 2
	}
	ready := func(stage string, e error) int {
		var buf bytes.Buffer
		code := reportProbeFailure(&buf, stage, e)
		_ = writeFrame(out, bytes.TrimSpace(buf.Bytes()))
		return code
	}
	raw, err := open(ctx, p)
	if err != nil {
		return ready("logon", err)
	}
	id := raw.Identity()
	if id.System != p.system || id.Client != p.client || !strings.EqualFold(id.User, p.user) {
		_ = raw.Close()
		return ready("identity", nil)
	}
	s, ok := raw.(adtSession)
	if !ok {
		_ = raw.Close()
		return ready("describe", nil)
	}
	if o.dataPreview {
		pv, ok := raw.(interface{ EnableDataPreview() })
		if !ok {
			_ = raw.Close()
			return ready("describe", nil)
		}
		pv.EnableDataPreview()
	}
	report, _ := json.Marshal(probeResult{OK: true, Identity: &id, SNC: true})
	if writeFrame(out, report) != nil {
		_ = s.Close()
		return 1
	}
	for {
		b, err := readFrame(in, maxRequestFrame)
		if err == io.EOF {
			if s.Close() != nil {
				return 1
			}
			return 0
		}
		if err != nil {
			_ = s.Close()
			return 1
		}
		c, refusal := checkRequest(b, o.dataPreview)
		if refusal != nil {
			if writeFrame(out, refusal) != nil {
				_ = s.Close()
				return 1
			}
			continue
		}
		result, err := s.Call("SADT_REST_RFC_ENDPOINT", adtRequest(c))
		var resp []byte
		if err == nil {
			resp, ok = httpResponse(result, strings.EqualFold(c.req.Header.Get("X-Csrf-Token"), "fetch"), c.head)
		}
		if err != nil || !ok {
			// The connection is terminal after a failed call; say so and stop.
			_ = writeFrame(out, localResponse(http.StatusBadGateway, "RFC call failed; vsp snc-serve is stopping", true))
			_ = s.Close()
			return 1
		}
		if writeFrame(out, resp) != nil {
			_ = s.Close()
			return 1
		}
	}
}

// WorkerMain is `vsp snc-serve-worker`: the guarded child that loads the SDK.
// It runs only when started by the supervisor (Main).
func WorkerMain(args []string, in io.Reader, out io.Writer) int {
	if os.Getenv(workerEnv) != "1" {
		return 2
	}
	p, _, err := parseServeFlags(args)
	if err != nil {
		return 2
	}
	ctx, cancel := context.WithTimeout(context.Background(), p.timeout)
	defer cancel()
	return runServeWorker(ctx, args, in, out, openProbe)
}

// Main is `vsp snc-serve`, the process the client starts. The SDK never loads
// here: it runs in a guarded worker whose output is accepted only as
// well-formed frames.
//
// The worker is started when the first request frame arrives, not before:
// on Windows vsp puts this process into its kill-on-close Job Object just
// after starting it, and a child started before that would not be in the job
// (see pkg/adt/stdio_transport_windows.go). vsp writes the first frame only
// after the assignment.
func Main(args []string, in io.Reader, out, errOut io.Writer) int {
	p, o, err := parseServeFlags(args)
	if err != nil {
		fmt.Fprintln(errOut, logPrefix, err)
		return 2
	}
	var w *workerProc
	kill := func(msg string) int {
		if w != nil {
			w.kill()
		}
		fmt.Fprintln(errOut, logPrefix, msg)
		return 1
	}
	for {
		reqFrame, err := readFrame(in, maxRequestFrame)
		if err == io.EOF {
			if w == nil {
				return 0
			}
			if err := w.close(); err != nil {
				fmt.Fprintln(errOut, logPrefix, err)
				return 1
			}
			return 0
		}
		if err != nil {
			return kill("client framing error: " + err.Error())
		}
		if w == nil {
			if w, err = startWorker(args, p); err != nil {
				fmt.Fprintln(errOut, logPrefix, err)
				return 1
			}
			if o.verbose {
				fmt.Fprintf(errOut, "%s ready %s.%s as %s over SNC\n", logPrefix, w.identity.System, w.identity.Client, w.identity.User)
			}
		}
		started := time.Now()
		method, uri := "?", "?"
		var resp []byte
		if c, refusal := checkRequest(reqFrame, o.dataPreview); refusal != nil {
			resp = refusal // never forwarded
			if c != nil {
				method, uri = c.req.Method, c.req.RequestURI
			}
		} else {
			method, uri = c.req.Method, c.req.RequestURI
			if resp, err = w.roundTrip(reqFrame, o.requestTimeout); err != nil {
				return kill(err.Error())
			}
		}
		parsed, err := http.ReadResponse(bufio.NewReader(bytes.NewReader(resp)), nil)
		if err != nil {
			return kill("worker response is not valid HTTP")
		}
		_ = parsed.Body.Close()
		if writeFrame(out, resp) != nil {
			return kill("client is gone")
		}
		if o.verbose {
			fmt.Fprintf(errOut, "%s %s %s -> %d, %d B, %d ms\n", logPrefix, method, uri, parsed.StatusCode, len(resp), time.Since(started).Milliseconds())
		}
		if parsed.Header.Get(stoppingHeader) == "1" {
			return kill("RFC call failed; stopped, no retry performed")
		}
	}
}
