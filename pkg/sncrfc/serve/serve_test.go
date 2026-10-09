package serve

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
	"strconv"
	"strings"
	"testing"

	"github.com/oisee/vibing-steampunk/pkg/sncrfc"
)

var serveArgs = []string{"-system", "TST", "-client", "123", "-connection", "fixture", "-user", "TESTUSER", "-dll", "fixture.dll", "-snc-lib", "fixture.dll"}

// adtFake is a logged-on session that answers every call with status and body.
type adtFake struct {
	closed, calls int
	status        string
	body          []byte
}

func (f *adtFake) Identity() sncrfc.Identity {
	return sncrfc.Identity{System: "TST", Client: "123", User: "TESTUSER"}
}
func (f *adtFake) Close() error { f.closed++; return nil }
func (f *adtFake) Call(string, map[string]any) (map[string]any, error) {
	f.calls++
	return map[string]any{"RESPONSE": map[string]any{"STATUS_LINE": map[string]any{"STATUS_CODE": f.status}, "MESSAGE_BODY": f.body}}, nil
}

type serveFake struct {
	adtFake
	inputs []map[string]any
	fail   bool
}

func (f *serveFake) Call(_ string, in map[string]any) (map[string]any, error) {
	f.calls++
	f.inputs = append(f.inputs, in)
	if f.fail {
		return nil, errors.New("fixture failure")
	}
	return map[string]any{"RESPONSE": map[string]any{
		"STATUS_LINE":   map[string]any{"STATUS_CODE": "200", "REASON_PHRASE": "OK"},
		"HEADER_FIELDS": []map[string]any{{"NAME": "Content-Type", "VALUE": "text/plain"}, {"NAME": "x-csrf-token", "VALUE": "fixture-token"}, {"NAME": "Set-Cookie", "VALUE": "SAP_SESSIONID=x"}},
		"MESSAGE_BODY":  []byte("source"),
	}}, nil
}

func frames(t *testing.T, reqs ...string) *bytes.Buffer {
	var b bytes.Buffer
	for _, r := range reqs {
		if writeFrame(&b, []byte(r)) != nil {
			t.Fatal("write frame")
		}
	}
	return &b
}

func get(uri string, extra string) string {
	return "GET " + uri + " HTTP/1.1\r\nHost: sidecar.invalid\r\nAccept: text/plain\r\nCookie: secret\r\nAuthorization: Basic eA==\r\n" + extra + "\r\n"
}

func responses(t *testing.T, out *bytes.Buffer) []*http.Response {
	var rs []*http.Response
	first := true
	for {
		b, err := readFrame(out, maxResponseFrame)
		if err == io.EOF {
			return rs
		}
		if err != nil {
			t.Fatal("bad frame from worker")
		}
		if first { // readiness report
			first = false
			continue
		}
		r, err := http.ReadResponse(bufio.NewReader(bytes.NewReader(b)), nil)
		if err != nil {
			t.Fatal("bad HTTP from worker")
		}
		rs = append(rs, r)
	}
}

func TestServeWorkerForwardsReadsAndRefusesTheRest(t *testing.T) {
	f := &serveFake{}
	in := frames(t,
		get("/sap/bc/adt/programs/programs/zfixture/source/main", "x-csrf-token: fetch\r\n"),
		"POST /sap/bc/adt/datapreview/freestyle HTTP/1.1\r\nHost: x\r\nContent-Length: 6\r\n\r\nSELECT",
		get("/sap/bc/adt/security/reentranceticket", ""),
		get("/sap/bc/adt/programs/%2e%2e/x", ""),
	)
	var out bytes.Buffer
	code := runServeWorker(context.Background(), serveArgs, in, &out, func(context.Context, probeFlags) (probeSession, error) { return f, nil })
	if code != 0 || f.closed != 1 || f.calls != 1 {
		t.Fatalf("code %d closed %d calls %d", code, f.closed, f.calls)
	}
	rs := responses(t, &out)
	if len(rs) != 4 || rs[0].StatusCode != 200 || rs[1].StatusCode != 403 || rs[2].StatusCode != 403 || rs[3].StatusCode != 403 {
		t.Fatal("wrong statuses")
	}
	if rs[0].Header.Get("x-csrf-token") != "fixture-token" || rs[0].Header.Get("Set-Cookie") != "" {
		t.Fatal("response headers not filtered")
	}
	body, _ := io.ReadAll(rs[0].Body)
	if string(body) != "source" {
		t.Fatal("body lost")
	}
	req := f.inputs[0]["REQUEST"].(map[string]any)
	for _, h := range req["HEADER_FIELDS"].([]map[string]any) {
		if n := strings.ToLower(h["NAME"].(string)); n == "cookie" || n == "authorization" || n == "host" || n == "x-csrf-token" {
			t.Fatal("credential header forwarded: " + n)
		}
	}
}

type noTokenFake struct{ serveFake }

func (f *noTokenFake) Call(_ string, in map[string]any) (map[string]any, error) {
	f.calls++
	return map[string]any{"RESPONSE": map[string]any{"STATUS_LINE": map[string]any{"STATUS_CODE": "200"}, "MESSAGE_BODY": []byte("x")}}, nil
}

func TestServeWorkerAnswersCSRFFetchWithFixedToken(t *testing.T) {
	f := &noTokenFake{}
	in := frames(t, get("/sap/bc/adt/discovery", "x-csrf-token: fetch\r\n"), get("/sap/bc/adt/discovery", "x-csrf-token: anything\r\n"))
	var out bytes.Buffer
	if runServeWorker(context.Background(), serveArgs, in, &out, func(context.Context, probeFlags) (probeSession, error) { return f, nil }) != 0 {
		t.Fatal("worker failed")
	}
	rs := responses(t, &out)
	if len(rs) != 2 || rs[0].Header.Get("X-Csrf-Token") != readOnlyCSRFToken || rs[1].Header.Get("X-Csrf-Token") != "" || rs[1].StatusCode != 200 {
		t.Fatal("CSRF handling wrong")
	}
}

func TestServeWorkerStopsAfterRFCFailure(t *testing.T) {
	f := &serveFake{fail: true}
	in := frames(t, get("/sap/bc/adt/discovery", ""), get("/sap/bc/adt/discovery", ""))
	var out bytes.Buffer
	if runServeWorker(context.Background(), serveArgs, in, &out, func(context.Context, probeFlags) (probeSession, error) { return f, nil }) == 0 {
		t.Fatal("failure reported as success")
	}
	rs := responses(t, &out)
	if f.calls != 1 || f.closed != 1 || len(rs) != 1 || rs[0].StatusCode != 502 || rs[0].Header.Get(stoppingHeader) != "1" {
		t.Fatal("did not stop after RFC failure")
	}
}

func TestServeWorkerRefusesWrongIdentity(t *testing.T) {
	f := &serveFake{}
	args := append([]string{}, serveArgs...)
	args[7] = "OTHERUSER"
	var out bytes.Buffer
	if runServeWorker(context.Background(), args, frames(t), &out, func(context.Context, probeFlags) (probeSession, error) { return f, nil }) == 0 || f.closed != 1 {
		t.Fatal("identity mismatch accepted")
	}
	b, _ := readFrame(&out, maxResponseFrame)
	if !bytes.Contains(b, []byte(`"stage":"identity"`)) {
		t.Fatal("identity stage missing")
	}
}

func TestFramesRejectOversizeAndTruncation(t *testing.T) {
	var b bytes.Buffer
	b.Write([]byte{0, 0x20, 0, 0}) // 2 MB request
	if _, err := readFrame(&b, maxRequestFrame); err != errFrameSize {
		t.Fatal("oversize accepted")
	}
	b.Reset()
	b.Write([]byte{0, 0, 0, 10, 'a'})
	if _, err := readFrame(&b, maxRequestFrame); err != io.ErrUnexpectedEOF {
		t.Fatal("truncation accepted")
	}
	b.Reset()
	b.Write([]byte{0, 0, 0, 0})
	if _, err := readFrame(&b, maxRequestFrame); err != errFrameSize {
		t.Fatal("empty frame accepted")
	}
}

func TestADTReadAllowlist(t *testing.T) {
	for uri, want := range map[string]bool{
		"/sap/bc/adt/discovery":                    true,
		"/sap/bc/adt/oo/classes/zcl_x/source/main": true,
		"/sap/bc/adt/repository/informationsystem/search?operation=quickSearch&query=Z*": true,
		"/sap/bc/adt/runtime/dumps":             true,
		"/sap/bc/adt/security/reentranceticket": false,
		"/sap/bc/adt/datapreview/freestyle":     false,
		"/sap/bc/adt/activation":                false,
		"/sap/bc/adt/programs/%2E%2E/x":         false,
		"/sap/bc/adt/programs/../security/x":    false,
		"/sap/bc/adt//programs/x":               false,
		"/sap/public/bc/icf/logoff":             false,
		"/sap/bc/adt/discoveryx":                false,
	} {
		if sncrfc.ADTReadAllowed(uri) != want {
			t.Errorf("%s: want %v", uri, want)
		}
	}
}

func TestDataPreviewIsOptInAndBounded(t *testing.T) {
	post := func(uri, body string) []byte {
		return []byte("POST " + uri + " HTTP/1.1\r\nHost: x\r\nContent-Type: text/plain\r\nContent-Length: " + strconv.Itoa(len(body)) + "\r\n\r\n" + body)
	}
	ok := post("/sap/bc/adt/datapreview/freestyle?rowNumber=100", "SELECT * FROM t000")
	if _, refusal := checkRequest(ok, false); refusal == nil {
		t.Fatal("data preview forwarded without opt-in")
	}
	if _, refusal := checkRequest(ok, true); refusal != nil {
		t.Fatal("bounded SELECT refused with opt-in")
	}
	for _, bad := range [][]byte{
		post("/sap/bc/adt/datapreview/freestyle", "SELECT * FROM t000"),                  // no row cap
		post("/sap/bc/adt/datapreview/freestyle?rowNumber=100000", "SELECT * FROM t000"), // too many rows
		post("/sap/bc/adt/datapreview/freestyle?rowNumber=10", "DELETE FROM t000"),
		post("/sap/bc/adt/datapreview/ddic?rowNumber=10&ddicEntityName=T000;X", ""),
		post("/sap/bc/adt/activation?rowNumber=10", "SELECT"),
		post("/sap/bc/adt/datapreview/freestyle?rowNumber=10&other=1", "SELECT * FROM t000"),
		post("/sap/bc/adt/datapreview/freestyle?rowNumber=10", "SELECT bname, pwdsaltedhash FROM usr02"),
		post("/sap/bc/adt/datapreview/freestyle?rowNumber=10", "SELECT * FROM t000 INNER JOIN rfcdes ON rfcdes~rfcdest = t000~mandt"),
		post("/sap/bc/adt/datapreview/ddic?rowNumber=10&ddicEntityName=USR02", ""),
	} {
		if _, refusal := checkRequest(bad, true); refusal == nil {
			t.Fatalf("unsafe POST accepted: %q", bad)
		}
	}
	if _, refusal := checkRequest(post("/sap/bc/adt/datapreview/ddic?rowNumber=50&ddicEntityName=T000", ""), true); refusal != nil {
		t.Fatal("ddic preview refused")
	}
	head := []byte("HEAD /sap/bc/adt/core/discovery HTTP/1.1\r\nHost: x\r\n\r\n")
	if c, refusal := checkRequest(head, false); refusal != nil || !c.head {
		t.Fatal("HEAD on an allowlisted path refused")
	}
}

func TestSecretTableMatchIsByWholeName(t *testing.T) {
	for s, want := range map[string]bool{"SELECT * FROM usr02": true, "select * from USR02 where bname = 'X'": true, "SELECT * FROM zusr02_copy": false, "SELECT * FROM usr021": false, "SELECT * FROM /abc/usr02": false, "SELECT * FROM t000": false} {
		if sncrfc.MentionsSecretTable(s) != want {
			t.Errorf("%q: want %v", s, want)
		}
	}
}

// The supervisor starts no worker before the first request frame (the Job
// Object rule of vsp's transport command), and a client that closes stdin
// without a request ends it cleanly.
func TestMainStartsNoWorkerWithoutARequest(t *testing.T) {
	var out, errOut bytes.Buffer
	if code := Main(serveArgs, &bytes.Buffer{}, &out, &errOut); code != 0 || out.Len() != 0 {
		t.Fatalf("code %d, out %q, stderr %q", code, out.String(), errOut.String())
	}
}

func TestMainRefusesBadArgumentsBeforeReading(t *testing.T) {
	var out, errOut bytes.Buffer
	if code := Main([]string{"-system", "TST"}, frames(t, get("/sap/bc/adt/discovery", "")), &out, &errOut); code != 2 || out.Len() != 0 {
		t.Fatalf("code %d, out %q", code, out.String())
	}
}

func TestWorkerMainRefusesWithoutSupervisor(t *testing.T) {
	t.Setenv(workerEnv, "")
	var out bytes.Buffer
	if code := WorkerMain(serveArgs, &bytes.Buffer{}, &out); code != 2 || out.Len() != 0 {
		t.Fatalf("code %d, out %q", code, out.String())
	}
}
