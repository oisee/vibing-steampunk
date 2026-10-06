package adt

import (
	"bufio"
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"sync"
	"testing"
	"time"
)

// The fake helper is this test binary itself, re-run with
// -test.run=^TestStdioHelperProcess$ and a marker after "--" (Go's
// helper-process pattern). It speaks the transport command's framing on
// stdin/stdout and decides what to do by the request path.

const stdioHelperMarker = "vsp-stdio-helper"

func stdioHelperArgv() []string {
	return []string{os.Args[0], "-test.run=^TestStdioHelperProcess$", "--", stdioHelperMarker}
}

func TestStdioHelperProcess(t *testing.T) {
	isHelper := false
	for i, a := range os.Args {
		if a == "--" && i+1 < len(os.Args) && os.Args[i+1] == stdioHelperMarker {
			isHelper = true
		}
	}
	if !isHelper {
		return
	}
	os.Exit(runStdioHelper(os.Stdin, os.Stdout, os.Stderr))
}

const stdioTestToken = "tok-123"

func runStdioHelper(in io.Reader, out io.Writer, errOut io.Writer) int {
	frames := make(chan []byte, 16)
	go func() {
		defer close(frames)
		r := bufio.NewReader(in)
		for {
			var hdr [4]byte
			if _, err := io.ReadFull(r, hdr[:]); err != nil {
				return // EOF: vsp is done with us
			}
			buf := make([]byte, binary.BigEndian.Uint32(hdr[:]))
			if _, err := io.ReadFull(r, buf); err != nil {
				fmt.Fprintln(errOut, "helper: short frame:", err)
				os.Exit(7)
			}
			frames <- buf
		}
	}()

	reply := func(status int, hdr map[string]string, body []byte) {
		var b bytes.Buffer
		fmt.Fprintf(&b, "HTTP/1.1 %d %s\r\n", status, http.StatusText(status))
		for k, v := range hdr {
			fmt.Fprintf(&b, "%s: %s\r\n", k, v)
		}
		fmt.Fprintf(&b, "Content-Length: %d\r\n\r\n", len(body))
		b.Write(body)
		var lp [4]byte
		binary.BigEndian.PutUint32(lp[:], uint32(b.Len()))
		out.Write(lp[:])
		out.Write(b.Bytes())
	}

	for frame := range frames {
		req, err := http.ReadRequest(bufio.NewReader(bytes.NewReader(frame)))
		if err != nil {
			fmt.Fprintln(errOut, "helper: malformed request frame:", err)
			return 8
		}
		body, _ := io.ReadAll(req.Body)
		if req.Header.Get("Authorization") != "" {
			fmt.Fprintln(errOut, "helper: got an Authorization header")
			return 10
		}

		switch {
		case req.URL.Path == "/big":
			big := bytes.Repeat([]byte("0123456789abcdef"), 300*1024/16)
			reply(200, map[string]string{
				"X-Got-Method": req.Method,
				"X-Got-Test":   req.Header.Get("X-Test"),
				"X-Got-Host":   req.Host,
				"Content-Type": "text/plain",
			}, big)
		case req.URL.Path == "/slow":
			// Interleaving check: while this request is being answered,
			// vsp must not have written another one.
			time.Sleep(15 * time.Millisecond)
			if len(frames) > 0 {
				fmt.Fprintln(errOut, "helper: INTERLEAVED frames")
				return 9
			}
			reply(200, nil, []byte("slow:"+req.URL.Query().Get("i")))
		case req.URL.Path == "/hang":
			time.Sleep(time.Hour)
		case req.URL.Path == "/oversize":
			var lp [4]byte
			binary.BigEndian.PutUint32(lp[:], MaxStdioFrame+1)
			out.Write(lp[:])
			time.Sleep(time.Hour)
		case req.URL.Path == "/die":
			fmt.Fprintln(errOut, "helper: starting to fail")
			fmt.Fprint(errOut, "boom: helper failed")
			return 3
		case req.URL.Path == "/cookie/set":
			reply(200, map[string]string{"Set-Cookie": "c1=v1; Path=/"}, nil)
		case req.URL.Path == "/cookie/echo":
			reply(200, nil, []byte("cookie:"+req.Header.Get("Cookie")))
		case strings.EqualFold(req.Header.Get("X-CSRF-Token"), "fetch"):
			reply(200, map[string]string{
				"X-CSRF-Token": stdioTestToken,
				"Set-Cookie":   "SAP_SESSIONID_TST=sess1; Path=/",
			}, nil)
		case req.Method == http.MethodPost:
			c, err := req.Cookie("SAP_SESSIONID_TST")
			if req.Header.Get("X-CSRF-Token") != stdioTestToken || err != nil || c.Value != "sess1" {
				reply(403, map[string]string{"X-CSRF-Token": "Required"}, []byte("CSRF token validation failed"))
				continue
			}
			reply(200, nil, []byte("posted:"+string(body)))
		default:
			reply(404, nil, []byte("no such path "+req.URL.Path))
		}
	}
	return 0
}

func newTestStdioTransport(t *testing.T) *StdioTransport {
	t.Helper()
	st := NewStdioTransport(stdioHelperArgv())
	t.Cleanup(func() { st.Close() })
	return st
}

func stdioGet(t *testing.T, st *StdioTransport, ctx context.Context, path string) (*http.Response, error) {
	t.Helper()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://sidecar.invalid"+path, nil)
	if err != nil {
		t.Fatal(err)
	}
	return st.RoundTrip(req)
}

func TestStdioTransport_GETRoundTrip(t *testing.T) {
	st := newTestStdioTransport(t)
	req, _ := http.NewRequest(http.MethodGet, "https://sidecar.invalid/big?sap-client=001", nil)
	req.Header.Set("X-Test", "hello")
	resp, err := st.RoundTrip(req)
	if err != nil {
		t.Fatalf("RoundTrip: %v", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != 200 {
		t.Fatalf("status %d", resp.StatusCode)
	}
	if want := 300 * 1024; len(body) != want {
		t.Fatalf("body %d bytes, want %d", len(body), want)
	}
	if !bytes.HasPrefix(body, []byte("0123456789abcdef0123")) {
		t.Fatalf("body starts %q", body[:20])
	}
	if got := resp.Header.Get("X-Got-Test"); got != "hello" {
		t.Errorf("request header reached the helper as %q", got)
	}
	if got := resp.Header.Get("X-Got-Host"); got != "sidecar.invalid" {
		t.Errorf("Host reached the helper as %q", got)
	}
	if got := resp.Header.Get("X-Got-Method"); got != "GET" {
		t.Errorf("method %q", got)
	}
	if resp.Request != req {
		t.Error("resp.Request is not the request sent")
	}
	// The same child answers a second time.
	resp2, err := st.RoundTrip(req)
	if err != nil || resp2.StatusCode != 200 {
		t.Fatalf("second RoundTrip: %v", err)
	}
}

func newStdioADTTransport(t *testing.T, opts ...Option) *Transport {
	t.Helper()
	opts = append([]Option{WithTransportCmd(stdioHelperArgv())}, opts...)
	cfg := NewConfig("https://sidecar.invalid", "", "", opts...)
	tr := NewTransport(cfg)
	t.Cleanup(func() { tr.CloseTransport() })
	return tr
}

func TestStdioTransport_CSRFThenPOST(t *testing.T) {
	tr := newStdioADTTransport(t)
	ctx := context.Background()
	for i := 1; i <= 2; i++ {
		resp, err := tr.Request(ctx, "/sap/bc/adt/vsp/test", &RequestOptions{Method: http.MethodPost, Body: []byte(fmt.Sprintf("x%d", i))})
		if err != nil {
			t.Fatalf("POST %d: %v", i, err)
		}
		if resp.StatusCode != 200 || string(resp.Body) != fmt.Sprintf("posted:x%d", i) {
			t.Fatalf("POST %d: %d %q", i, resp.StatusCode, resp.Body)
		}
	}
	if got := tr.getCSRFToken(); got != stdioTestToken {
		t.Errorf("CSRF token kept as %q, want %q", got, stdioTestToken)
	}
}

func TestStdioTransport_CookiesThroughJar(t *testing.T) {
	tr := newStdioADTTransport(t)
	ctx := context.Background()
	if _, err := tr.Request(ctx, "/cookie/set", nil); err != nil {
		t.Fatal(err)
	}
	resp, err := tr.Request(ctx, "/cookie/echo", nil)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(resp.Body), "c1=v1") {
		t.Fatalf("cookie set by the helper was not sent back: %q", resp.Body)
	}
}

func TestStdioTransport_CancelMidExchangeKillsChild(t *testing.T) {
	st := newTestStdioTransport(t)
	// Start the child with a good exchange first, so the cancel lands on a
	// running one.
	if _, err := stdioGet(t, st, context.Background(), "/cookie/set"); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()
	_, err := stdioGet(t, st, ctx, "/hang")
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("want the context error, got %v", err)
	}
	select {
	case <-st.exited:
	case <-time.After(5 * time.Second):
		t.Fatal("child still running after a cancelled exchange")
	}
	_, err = stdioGet(t, st, context.Background(), "/big")
	if err == nil || !strings.Contains(err.Error(), "broken") {
		t.Fatalf("next call after a cancel: want a broken error, got %v", err)
	}
}

func TestStdioTransport_ChildExitReportsStatusAndStderr(t *testing.T) {
	var stderr syncBuffer
	old := stdioStderr
	stdioStderr = &stderr
	t.Cleanup(func() { stdioStderr = old })

	st := newTestStdioTransport(t)
	_, err := stdioGet(t, st, context.Background(), "/die")
	if err == nil {
		t.Fatal("want an error")
	}
	msg := err.Error()
	for _, want := range []string{"exit status 3", "boom: helper failed", "helper: starting to fail", "broken", "transport command"} {
		if !strings.Contains(msg, want) {
			t.Errorf("error lacks %q: %s", want, msg)
		}
	}
	if strings.Contains(msg, "/") && strings.Contains(msg, os.Args[0]) {
		t.Errorf("error carries the full command path: %s", msg)
	}
	if !strings.Contains(stderr.String(), "boom: helper failed") {
		t.Errorf("stderr not passed through: %q", stderr.String())
	}
	_, err2 := stdioGet(t, st, context.Background(), "/big")
	if err2 == nil || err2.Error() != msg {
		t.Fatalf("next call: want the same broken error, got %v", err2)
	}
}

func TestStdioTransport_OversizedFrameFails(t *testing.T) {
	st := newTestStdioTransport(t)
	_, err := stdioGet(t, st, context.Background(), "/oversize")
	if err == nil || !strings.Contains(err.Error(), "frame too large") {
		t.Fatalf("want a frame-size error, got %v", err)
	}
	_, err = stdioGet(t, st, context.Background(), "/big")
	if err == nil || !strings.Contains(err.Error(), "broken") {
		t.Fatalf("next call: want broken, got %v", err)
	}
}

func TestStdioTransport_ConcurrentRoundTripsAreSerialised(t *testing.T) {
	st := newTestStdioTransport(t)
	var wg sync.WaitGroup
	errs := make(chan error, 64)
	for g := 0; g < 8; g++ {
		wg.Add(1)
		go func(g int) {
			defer wg.Done()
			for i := 0; i < 3; i++ {
				id := fmt.Sprintf("%d-%d", g, i)
				req, _ := http.NewRequest(http.MethodGet, "https://sidecar.invalid/slow?i="+id, nil)
				resp, err := st.RoundTrip(req)
				if err != nil {
					errs <- err
					return
				}
				b, _ := io.ReadAll(resp.Body)
				if string(b) != "slow:"+id {
					errs <- fmt.Errorf("request %s got answer %q", id, b)
				}
			}
		}(g)
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		t.Error(err)
	}
}

func TestStdioTransport_CloseEndsChild(t *testing.T) {
	st := newTestStdioTransport(t)
	if _, err := stdioGet(t, st, context.Background(), "/cookie/set"); err != nil {
		t.Fatal(err)
	}
	start := time.Now()
	if err := st.Close(); err != nil {
		t.Fatal(err)
	}
	if d := time.Since(start); d > 2*time.Second {
		t.Errorf("Close took %s: the helper should exit on stdin EOF", d)
	}
	if st.cmd.ProcessState == nil || st.cmd.ProcessState.ExitCode() != 0 {
		t.Errorf("helper did not exit cleanly on EOF: %v", st.cmd.ProcessState)
	}
	if _, err := stdioGet(t, st, context.Background(), "/big"); err == nil || !strings.Contains(err.Error(), "broken") {
		t.Fatalf("call after Close: %v", err)
	}
}

func TestStdioTransport_RefusesCredentials(t *testing.T) {
	cfg := NewConfig("https://sidecar.invalid", "TESTUSER", "secret", WithTransportCmd([]string{"/nonexistent/helper"}))
	tr := NewTransport(cfg)
	_, err := tr.Request(context.Background(), "/sap/bc/adt/core/discovery", nil)
	if err == nil || !errors.Is(err, ErrTransportCmdAuth) {
		t.Fatalf("want ErrTransportCmdAuth, got %v", err)
	}
	cfg2 := NewConfig("https://sidecar.invalid", "", "", WithTransportCmd([]string{"/nonexistent/helper"}), WithCookies(map[string]string{"MYSAPSSO2": "x"}))
	if err := cfg2.CheckTransportCmd(); !errors.Is(err, ErrTransportCmdAuth) {
		t.Fatalf("cookies with a transport command: %v", err)
	}
	// And at the wire: an Authorization header never reaches the helper.
	st := NewStdioTransport([]string{"/nonexistent/helper"})
	req, _ := http.NewRequest(http.MethodGet, "https://sidecar.invalid/x", nil)
	req.SetBasicAuth("TESTUSER", "secret")
	if _, err := st.RoundTrip(req); !errors.Is(err, ErrTransportCmdAuth) {
		t.Fatalf("Authorization header: %v", err)
	}
	if st.started {
		t.Error("the helper was started for a refused request")
	}
}

func TestStdioTransport_WebSocketRefuses(t *testing.T) {
	c := NewClient("https://sidecar.invalid", "", "", WithTransportCmd(stdioHelperArgv()))
	defer c.CloseTransport()
	for name, connect := range map[string]func(context.Context) error{
		"debug": c.NewDebugWebSocketClient().Connect,
		"amdp":  c.NewAMDPWebSocketClient().Connect,
	} {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		err := connect(ctx)
		cancel()
		if !errors.Is(err, ErrWebSocketOverTransportCmd) {
			t.Errorf("%s WebSocket over a transport command: want a refusal, got %v", name, err)
		}
	}
}

func TestLineTailKeepsLastLines(t *testing.T) {
	l := &lineTail{max: 3}
	fmt.Fprint(l, "a\nb\nc\nd\ne")
	if got := l.String(); got != "c\nd\ne" {
		t.Fatalf("got %q", got)
	}
}

func TestCommandBase(t *testing.T) {
	for in, want := range map[string]string{
		"/usr/local/bin/helper":        "helper",
		`C:\Tools\helper.exe`:          "helper.exe",
		"helper":                       "helper",
		"/mnt/c/Program Files/h/x.exe": "x.exe",
	} {
		if got := commandBase(in); got != want {
			t.Errorf("commandBase(%q) = %q, want %q", in, got, want)
		}
	}
}

// syncBuffer is a bytes.Buffer safe for the stderr copier and the test.
type syncBuffer struct {
	mu sync.Mutex
	b  bytes.Buffer
}

func (s *syncBuffer) Write(p []byte) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.b.Write(p)
}

func (s *syncBuffer) String() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.b.String()
}
