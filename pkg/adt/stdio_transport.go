package adt

// A transport command carries every ADT request to a child process over its
// stdin and stdout instead of a TCP connection. The child (a "helper") reaches
// the SAP system by its own means — typically it signs on with a corporate SSO
// that vsp cannot do itself — and vsp opens no socket and no local port.
//
// Wire format, both directions: a frame is a 4-byte big-endian length followed
// by exactly that many bytes of one raw HTTP/1.1 message. vsp writes a request
// (origin-form request line plus a Host header, always with Content-Length,
// never chunked); the helper answers with one complete response (status line,
// headers, body; Content-Length or chunked, read to the end of the frame).
// Exactly one request is in flight at a time: vsp writes the next frame only
// after it has read the previous answer. A frame larger than 64 MiB is an
// error. The helper exits when its stdin reaches EOF, logs only to stderr, and
// never prints secrets there, since vsp passes the helper's stderr through to
// its own and quotes the tail of it in error messages.
//
// The pipe has no way to resynchronise. When an exchange is abandoned half-way
// (the caller's context ends, the helper dies, a frame is malformed), the
// helper is killed and the transport stays broken: every later request fails
// with the same error. It is never restarted behind the caller's back, because
// a fresh helper would be a fresh logon and a fresh session, and the locks and
// stateful contexts of the old one would silently be gone.

import (
	"bufio"
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"regexp"
	"strings"
	"sync"
	"time"
)

// MaxStdioFrame caps one frame in either direction.
const MaxStdioFrame = 64 << 20

// stdioStderrLines is how much of the helper's stderr an error quotes.
const stdioStderrLines = 20

// stdioCloseGrace is how long Close waits for the helper to exit on EOF.
const stdioCloseGrace = 3 * time.Second

// stdioExitGrace is how long a failed exchange waits for the helper's exit
// status before killing it.
const stdioExitGrace = 2 * time.Second

// ErrTransportCmdAuth is returned when a transport command is combined with
// credentials vsp would otherwise send itself.
var ErrTransportCmdAuth = errors.New("transport_cmd carries its own authentication; remove user/password/cookies")

// errFrameTooLarge marks a frame over MaxStdioFrame.
var errFrameTooLarge = errors.New("frame too large")

// stdioStderr is where the helper's stderr is passed through to.
var stdioStderr io.Writer = os.Stderr

// StdioTransport is an http.RoundTripper that sends each request to a child
// process over stdin and reads the answer from its stdout. See the package
// comment above for the wire format.
type StdioTransport struct {
	argv []string
	name string // basename of argv[0], for messages

	// turn admits one exchange at a time; a channel rather than a mutex so a
	// waiter can give up with its context.
	turn chan struct{}

	mu       sync.Mutex // guards everything below
	started  bool
	cmd      *exec.Cmd
	stdin    *os.File
	stdout   *bufio.Reader
	stdoutF  *os.File
	tail     *lineTail
	exited   chan struct{} // closed when the child has been reaped
	waitErr  error
	broken   error
	jobClose func()
}

// NewStdioTransport returns a transport for argv (no shell). The child is not
// started until the first request.
func NewStdioTransport(argv []string) *StdioTransport {
	t := &StdioTransport{
		argv: append([]string(nil), argv...),
		turn: make(chan struct{}, 1),
	}
	if len(argv) > 0 {
		t.name = commandBase(argv[0])
	}
	return t
}

// commandBase is the program's name without its directory, on either
// platform's separators, so an error never carries a user's path.
func commandBase(p string) string {
	p = strings.TrimRight(p, `/\`)
	if i := strings.LastIndexAny(p, `/\`); i >= 0 {
		p = p[i+1:]
	}
	if p == "" {
		return "transport command"
	}
	return p
}

// TransportCmdName is the basename of a transport command, for status lines.
func TransportCmdName(argv []string) string {
	if len(argv) == 0 {
		return ""
	}
	return commandBase(argv[0])
}

// RoundTrip implements http.RoundTripper.
func (t *StdioTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	body, err := readRequestBody(req)
	if err != nil {
		return nil, err
	}
	if req.Header.Get("Authorization") != "" {
		return nil, ErrTransportCmdAuth
	}
	ctx := req.Context()

	select {
	case t.turn <- struct{}{}:
	case <-ctx.Done():
		return nil, ctx.Err()
	}
	defer func() { <-t.turn }()

	if err := t.ensureStarted(); err != nil {
		return nil, err
	}

	frame, err := encodeRequestFrame(req, body)
	if err != nil {
		return nil, err // nothing written: the pipe is still in step
	}

	t.mu.Lock()
	stdin, stdout := t.stdin, t.stdout
	t.mu.Unlock()

	type result struct {
		raw []byte
		err error
	}
	done := make(chan result, 1)
	go func() {
		if _, err := stdin.Write(frame); err != nil {
			done <- result{err: fmt.Errorf("writing request: %w", err)}
			return
		}
		raw, err := readFrame(stdout)
		done <- result{raw: raw, err: err}
	}()

	var r result
	select {
	case r = <-done:
	case <-ctx.Done():
		_ = t.fail(fmt.Errorf("request %s %s abandoned mid-exchange: %v", req.Method, req.URL.Path, ctx.Err()), true)
		return nil, ctx.Err()
	}
	if r.err != nil {
		// An oversized frame leaves the child mid-write: end it now. Any
		// other pipe error means the child is going or gone; give it a
		// moment, so its exit status can be reported.
		return nil, t.fail(r.err, errors.Is(r.err, errFrameTooLarge))
	}

	resp, err := http.ReadResponse(bufio.NewReader(bytes.NewReader(r.raw)), req)
	if err != nil {
		return nil, t.fail(fmt.Errorf("malformed response frame: %w", err), true)
	}
	respBody, err := io.ReadAll(resp.Body)
	resp.Body.Close()
	if err != nil {
		return nil, t.fail(fmt.Errorf("malformed response body: %w", err), true)
	}
	resp.Body = io.NopCloser(bytes.NewReader(respBody))
	if req.Method != http.MethodHead {
		// The body is read and de-chunked: describe what is held now.
		resp.ContentLength = int64(len(respBody))
		resp.TransferEncoding = nil
	}
	resp.Request = req
	return resp, nil
}

// readRequestBody reads and closes the request body, as a RoundTripper must.
func readRequestBody(req *http.Request) ([]byte, error) {
	if req.Body == nil || req.Body == http.NoBody {
		return nil, nil
	}
	defer req.Body.Close()
	b, err := io.ReadAll(io.LimitReader(req.Body, MaxStdioFrame+1))
	if err != nil {
		return nil, fmt.Errorf("reading request body: %w", err)
	}
	if len(b) > MaxStdioFrame {
		return nil, fmt.Errorf("request body exceeds the %d-byte frame limit of the transport command", MaxStdioFrame)
	}
	return b, nil
}

// encodeRequestFrame renders req as one length-prefixed HTTP/1.1 message.
func encodeRequestFrame(req *http.Request, body []byte) ([]byte, error) {
	out := req.Clone(req.Context())
	out.Body = nil
	out.ContentLength = int64(len(body))
	out.TransferEncoding = nil
	out.Close = false
	if len(body) > 0 {
		out.Body = io.NopCloser(bytes.NewReader(body))
	}
	var buf bytes.Buffer
	buf.Write([]byte{0, 0, 0, 0})
	if err := out.Write(&buf); err != nil {
		return nil, fmt.Errorf("encoding request: %w", err)
	}
	n := buf.Len() - 4
	if n > MaxStdioFrame {
		return nil, fmt.Errorf("request of %d bytes exceeds the %d-byte frame limit of the transport command", n, MaxStdioFrame)
	}
	frame := buf.Bytes()
	binary.BigEndian.PutUint32(frame[:4], uint32(n))
	return frame, nil
}

// readFrame reads one length-prefixed frame.
func readFrame(r io.Reader) ([]byte, error) {
	var hdr [4]byte
	if _, err := io.ReadFull(r, hdr[:]); err != nil {
		return nil, fmt.Errorf("reading response frame: %w", err)
	}
	n := binary.BigEndian.Uint32(hdr[:])
	if n > MaxStdioFrame {
		return nil, fmt.Errorf("%w: response frame of %d bytes, limit %d", errFrameTooLarge, n, MaxStdioFrame)
	}
	buf := make([]byte, n)
	if _, err := io.ReadFull(r, buf); err != nil {
		return nil, fmt.Errorf("reading response frame (%d bytes): %w", n, err)
	}
	return buf, nil
}

// ensureStarted starts the child on first use, or reports why it cannot run.
func (t *StdioTransport) ensureStarted() error {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.broken != nil {
		return t.broken
	}
	if t.started {
		return nil
	}
	t.started = true
	if len(t.argv) == 0 || t.argv[0] == "" {
		t.broken = errors.New("transport command is empty")
		return t.broken
	}

	inR, inW, err := os.Pipe()
	if err != nil {
		t.broken = fmt.Errorf("transport command %s: %w", t.name, err)
		return t.broken
	}
	outR, outW, err := os.Pipe()
	if err != nil {
		inR.Close()
		inW.Close()
		t.broken = fmt.Errorf("transport command %s: %w", t.name, err)
		return t.broken
	}
	tail := &lineTail{max: stdioStderrLines}
	cmd := exec.Command(t.argv[0], t.argv[1:]...)
	cmd.Stdin = inR
	cmd.Stdout = outW
	cmd.Stderr = io.MultiWriter(stdioStderr, tail)
	cmd.Env = helperEnv(helperEnviron())
	// A grandchild that keeps stderr open must not keep Wait from returning.
	cmd.WaitDelay = time.Second
	if err := cmd.Start(); err != nil {
		inR.Close()
		inW.Close()
		outR.Close()
		outW.Close()
		t.broken = fmt.Errorf("transport command %s is broken: could not start: %w", t.name, startCause(err))
		return t.broken
	}
	// The child holds its own ends now.
	inR.Close()
	outW.Close()

	// Windows: into a kill-on-close Job Object. This happens after the start;
	// see attachKillOnClose for the window that leaves, and why the first
	// frame is only written afterwards.
	if closeJob, jerr := attachKillOnClose(cmd.Process); jerr != nil {
		fmt.Fprintf(stdioStderr, "[transport-cmd] warning: %s will not be killed with vsp: %v\n", t.name, jerr)
	} else {
		t.jobClose = closeJob
	}

	t.cmd = cmd
	t.stdin = inW
	t.stdoutF = outR
	t.stdout = bufio.NewReaderSize(outR, 64<<10)
	t.tail = tail
	t.exited = make(chan struct{})
	go func(exited chan struct{}) {
		err := cmd.Wait()
		t.mu.Lock()
		t.waitErr = err
		t.mu.Unlock()
		close(exited)
	}(t.exited)
	return nil
}

// fail marks the transport broken with cause and returns the error every
// later request will get. kill ends the child at once: the pipe is out of step
// and nothing it says can be trusted any more. Without kill, the child is
// given a moment to exit by itself, so its exit status can be reported.
func (t *StdioTransport) fail(cause error, kill bool) error {
	t.mu.Lock()
	if t.broken != nil {
		err := t.broken
		t.mu.Unlock()
		return err
	}
	exited, cmd := t.exited, t.cmd
	t.mu.Unlock()

	if cmd != nil {
		if kill {
			_ = cmd.Process.Kill()
			<-exited
		} else {
			select {
			case <-exited:
			case <-time.After(stdioExitGrace):
				_ = cmd.Process.Kill()
				<-exited
			}
		}
	}

	t.mu.Lock()
	defer t.mu.Unlock()
	if t.broken != nil {
		return t.broken
	}
	msg := fmt.Sprintf("transport command %s is broken (it is not restarted): %v", t.name, cause)
	if cmd != nil && cmd.ProcessState != nil {
		msg += "; " + exitDescription(cmd.ProcessState, kill)
	}
	if t.tail != nil {
		if s := t.tail.String(); s != "" {
			msg += "; stderr tail:\n" + s
		}
	}
	t.broken = errors.New(msg)
	t.releaseLocked()
	return t.broken
}

// helperEnviron is the environment a transport command starts from. A
// program that loads a project's .env should set it, with SetHelperEnviron,
// to the environment it had before: a project must not reach the helper with
// LD_PRELOAD, PATH or the like.
var helperEnviron = os.Environ

// SetHelperEnviron sets where a transport command's environment comes from,
// before any is started. vsp passes its environment as it was at start-up,
// before ./.env was loaded.
func SetHelperEnviron(environ func() []string) { helperEnviron = environ }

// helperSecretName matches environment variable names that hold a secret.
var helperSecretName = regexp.MustCompile(`(?i)(PASSWORD|PASSWD|SECRET|TOKEN|COOKIE)`)

// helperEnv is vsp's environment minus what the helper has no business
// seeing: anything that looks like a secret by its name, and vsp's own logon
// and RFC settings (SAP_USER, SAP_USERNAME, SAP_PASS, SAP_SAML_USER, SAP_RFC_*;
// SAP_PASSWORD and VSP_<SYSTEM>_PASSWORD go by the pattern). The helper does
// its own sign-on. Everything else stays: PATH, SystemRoot, TEMP, proxies and
// the like are what a helper needs to run at all.
func helperEnv(environ []string) []string {
	out := make([]string, 0, len(environ))
	for _, kv := range environ {
		name, _, _ := strings.Cut(kv, "=")
		if name == "" { // Windows' per-drive "=C:=C:\..." entries
			out = append(out, kv)
			continue
		}
		upper := strings.ToUpper(name)
		switch {
		case helperSecretName.MatchString(name),
			upper == "SAP_USER", upper == "SAP_USERNAME", upper == "SAP_PASS",
			upper == "SAP_SAML_USER", strings.HasPrefix(upper, "SAP_RFC_"):
			continue
		}
		out = append(out, kv)
	}
	return out
}

// startCause strips the program's path from a start failure: errors name
// the command by its basename only.
func startCause(err error) error {
	var ee *exec.Error
	if errors.As(err, &ee) {
		return ee.Err
	}
	var pe *os.PathError
	if errors.As(err, &pe) {
		return pe.Err
	}
	return err
}

// exitDescription says how the child ended.
func exitDescription(ps *os.ProcessState, killed bool) string {
	if killed {
		return "killed by vsp"
	}
	if code := ps.ExitCode(); code >= 0 {
		return fmt.Sprintf("exit status %d", code)
	}
	return ps.String()
}

// releaseLocked closes the parent's pipe ends and the job handle.
func (t *StdioTransport) releaseLocked() {
	if t.stdin != nil {
		t.stdin.Close()
	}
	if t.stdoutF != nil {
		t.stdoutF.Close()
	}
	if t.jobClose != nil {
		t.jobClose()
		t.jobClose = nil
	}
}

// Close closes the child's stdin, waits up to three seconds for it to exit,
// then kills it. Every later request fails. Safe to call more than once and
// on a transport that never started.
func (t *StdioTransport) Close() error {
	t.mu.Lock()
	if t.broken == nil {
		t.broken = fmt.Errorf("transport command %s is broken: closed", t.name)
	}
	cmd, exited, stdin := t.cmd, t.exited, t.stdin
	t.started = true
	t.mu.Unlock()
	if cmd == nil {
		return nil
	}
	if stdin != nil {
		stdin.Close()
	}
	select {
	case <-exited:
	case <-time.After(stdioCloseGrace):
		_ = cmd.Process.Kill()
		<-exited
	}
	t.mu.Lock()
	t.releaseLocked()
	t.mu.Unlock()
	return nil
}

// lineTail keeps the last max lines written to it.
type lineTail struct {
	mu      sync.Mutex
	max     int
	lines   []string
	partial []byte
}

const lineTailMaxLine = 1024

func (l *lineTail) Write(p []byte) (int, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	for _, b := range p {
		if b == '\n' {
			l.push(string(l.partial))
			l.partial = l.partial[:0]
			continue
		}
		if len(l.partial) < lineTailMaxLine {
			l.partial = append(l.partial, b)
		}
	}
	return len(p), nil
}

func (l *lineTail) push(s string) {
	l.lines = append(l.lines, strings.TrimRight(s, "\r"))
	if len(l.lines) > l.max {
		l.lines = l.lines[len(l.lines)-l.max:]
	}
}

// String returns the kept lines, the unterminated last one included.
func (l *lineTail) String() string {
	l.mu.Lock()
	defer l.mu.Unlock()
	lines := append([]string(nil), l.lines...)
	if len(l.partial) > 0 {
		lines = append(lines, string(l.partial))
		if len(lines) > l.max {
			lines = lines[len(lines)-l.max:]
		}
	}
	return strings.Join(lines, "\n")
}

// failingRoundTripper refuses every request with one error: a configuration
// that must not send anything.
type failingRoundTripper struct{ err error }

func (f failingRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	if req.Body != nil {
		req.Body.Close()
	}
	return nil, f.err
}

// CheckTransportCmd refuses a transport command combined with credentials
// vsp would send itself.
func (c *Config) CheckTransportCmd() error {
	if len(c.TransportCmd) == 0 {
		return nil
	}
	if c.Username != "" || c.Password != "" || len(c.Cookies) > 0 {
		return ErrTransportCmdAuth
	}
	return nil
}

// closeHTTPTransport closes the RoundTripper of an *http.Client when it can
// be closed (a transport command's child process).
func closeHTTPTransport(d HTTPDoer) error {
	hc, ok := d.(*http.Client)
	if !ok || hc == nil {
		return nil
	}
	if c, ok := hc.Transport.(io.Closer); ok {
		return c.Close()
	}
	return nil
}

// CloseTransport ends the helper process of a transport command, if the
// transport has one. See Client.CloseTransport.
func (t *Transport) CloseTransport() error {
	if t == nil {
		return nil
	}
	return closeHTTPTransport(t.httpClient)
}
