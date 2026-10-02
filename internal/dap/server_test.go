package dap

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/oisee/vibing-steampunk/pkg/saprfc"
)

// client is the editor's half of a DAP conversation.
type client struct {
	t      *testing.T
	in     *io.PipeWriter // to the adapter
	seq    int
	msgs   chan map[string]any
	events []map[string]any
	served chan error
}

const waitFor = 5 * time.Second

func startAdapter(t *testing.T, open Opener) (*client, *Server) {
	t.Helper()
	inR, inW := io.Pipe()
	outR, outW := io.Pipe()
	srv := New(context.Background(), inR, outW, open)
	c := &client{t: t, in: inW, msgs: make(chan map[string]any, 256), served: make(chan error, 1)}
	go func() {
		c.served <- srv.Serve()
		outW.Close()
	}()
	go func() {
		conn := NewConn(outR, io.Discard)
		for {
			raw, err := readRaw(conn)
			if err != nil {
				close(c.msgs)
				return
			}
			c.msgs <- raw
		}
	}()
	t.Cleanup(func() {
		inW.Close()
		select {
		case <-c.served:
		case <-time.After(waitFor):
			t.Error("the adapter did not finish after its input closed")
		}
	})
	return c, srv
}

// readRaw reads one adapter message as a generic map.
func readRaw(conn *Conn) (map[string]any, error) {
	body, err := conn.readFrame()
	if err != nil {
		return nil, err
	}
	var out map[string]any
	if err := json.Unmarshal(body, &out); err != nil {
		return nil, err
	}
	return out, nil
}

func frame(body string) string {
	return fmt.Sprintf("Content-Length: %d\r\n\r\n%s", len(body), body)
}

func (c *client) send(raw string) {
	c.t.Helper()
	if _, err := fmt.Fprintf(c.in, "Content-Length: %d\r\n\r\n%s", len(raw), raw); err != nil {
		c.t.Fatalf("writing to the adapter: %v", err)
	}
}

// request sends a request and returns its response, collecting the events
// that arrive meanwhile.
func (c *client) request(command string, args any) map[string]any {
	c.t.Helper()
	c.seq++
	body, _ := json.Marshal(map[string]any{"seq": c.seq, "type": "request", "command": command, "arguments": args})
	c.send(string(body))
	deadline := time.After(waitFor)
	for {
		select {
		case m, ok := <-c.msgs:
			if !ok {
				c.t.Fatalf("%s: the adapter closed the stream", command)
			}
			if m["type"] == "event" {
				c.events = append(c.events, m)
				continue
			}
			if m["type"] == "response" && int(m["request_seq"].(float64)) == c.seq {
				return m
			}
		case <-deadline:
			c.t.Fatalf("%s: no response", command)
		}
	}
}

func (c *client) ok(command string, args any) map[string]any {
	c.t.Helper()
	res := c.request(command, args)
	if res["success"] != true {
		c.t.Fatalf("%s failed: %v", command, res["message"])
	}
	body, _ := res["body"].(map[string]any)
	return body
}

// event waits for the named event, looking first at those already seen.
func (c *client) event(name string) map[string]any {
	c.t.Helper()
	for i, e := range c.events {
		if e["event"] == name {
			c.events = append(c.events[:i], c.events[i+1:]...)
			body, _ := e["body"].(map[string]any)
			return body
		}
	}
	deadline := time.After(waitFor)
	for {
		select {
		case m, ok := <-c.msgs:
			if !ok {
				c.t.Fatalf("waiting for %s: the adapter closed the stream", name)
			}
			if m["type"] != "event" {
				continue
			}
			if m["event"] == name {
				body, _ := m["body"].(map[string]any)
				return body
			}
			c.events = append(c.events, m)
		case <-deadline:
			c.t.Fatalf("no %s event", name)
		}
	}
}

// demoFile is ZVSP_DEBUG_DEMO checked out under vsp's file convention.
func demoFile(t *testing.T) string {
	t.Helper()
	src, err := os.ReadFile(filepath.Join("..", "..", "pkg", "saprfc", "testdata", "fixtures", "zvsp_debug_demo.prog.abap"))
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "zvsp_debug_demo.prog.abap")
	if err := os.WriteFile(path, src, 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func fakeOpener(fake *fakeADT, readOnly bool, opened *[]LaunchArgs) Opener {
	var mu sync.Mutex
	return func(_ context.Context, args LaunchArgs) (*Session, error) {
		mu.Lock()
		defer mu.Unlock()
		if opened != nil {
			*opened = append(*opened, args)
		}
		return &Session{Debugger: saprfc.NewADTDebugger(fake, "TESTUSER"), User: "TESTUSER", System: "FAKE", ReadOnly: readOnly}, nil
	}
}

func vars(t *testing.T, body map[string]any) map[string]map[string]any {
	t.Helper()
	out := map[string]map[string]any{}
	list, _ := body["variables"].([]any)
	for _, v := range list {
		m := v.(map[string]any)
		out[m["name"].(string)] = m
	}
	return out
}

func ref(m map[string]any) int {
	n, _ := m["variablesReference"].(float64)
	return int(n)
}

// The whole conversation an editor has with the adapter, against a fake ADT
// debugger: breakpoints before attach, the stop, the stack, the variable tree
// down into a table, a step, a continue to the end, and the disconnect.
func TestConversation(t *testing.T) {
	fake := newFakeADT()
	c, _ := startAdapter(t, fakeOpener(fake, false, nil))
	path := demoFile(t)

	caps := c.ok("initialize", map[string]any{"clientID": "test", "adapterID": "abap-sap", "linesStartAt1": true})
	if caps["supportsConfigurationDoneRequest"] != true || caps["supportsConditionalBreakpoints"] != false {
		t.Fatalf("capabilities: %v", caps)
	}

	// Set before the session exists: held, and placed at attach.
	bps := c.ok("setBreakpoints", map[string]any{
		"source":      map[string]any{"path": path, "name": "zvsp_debug_demo.prog.abap"},
		"breakpoints": []any{map[string]any{"line": 26}, map[string]any{"line": 1}},
	})
	if list := bps["breakpoints"].([]any); len(list) != 2 || list[0].(map[string]any)["verified"] != false {
		t.Fatalf("breakpoints before attach: %v", bps)
	}

	c.ok("attach", map[string]any{"object": "ZVSP_DEBUG_DEMO"})
	c.event("initialized")
	// The held breakpoints are placed now and reported as changed: 26 holds,
	// line 1 is refused with SAP's reason.
	verified := map[float64]any{}
	for i := 0; i < 2; i++ {
		e := c.event("breakpoint")["breakpoint"].(map[string]any)
		verified[e["line"].(float64)] = e["verified"]
		if e["line"].(float64) == 1 && !strings.Contains(fmt.Sprint(e["message"]), "Cannot create") {
			t.Errorf("refused breakpoint carries no reason: %v", e)
		}
	}
	if verified[26] != true || verified[1] != false {
		t.Fatalf("placement: %v", verified)
	}
	c.ok("configurationDone", nil)

	fake.run() // somebody runs the report in SAP
	stopped := c.event("stopped")
	if stopped["reason"] != "breakpoint" || stopped["threadId"].(float64) != threadID {
		t.Fatalf("stopped: %v", stopped)
	}

	threads := c.ok("threads", nil)["threads"].([]any)
	if len(threads) != 1 {
		t.Fatalf("threads: %v", threads)
	}

	st := c.ok("stackTrace", map[string]any{"threadId": 1})
	frames := st["stackFrames"].([]any)
	top := frames[0].(map[string]any)
	if top["line"].(float64) != 26 {
		t.Fatalf("top frame: %v", top)
	}
	if src := top["source"].(map[string]any); src["path"] != path {
		t.Fatalf("top frame should open the local file %s: %v", path, src)
	}
	if last := frames[len(frames)-1].(map[string]any); last["presentationHint"] != "label" || last["source"] != nil {
		t.Errorf("a screen frame has no ABAP source: %v", last)
	}

	scopes := c.ok("scopes", map[string]any{"frameId": top["id"]})["scopes"].([]any)
	globals := scopes[0].(map[string]any)
	if globals["name"] != "Globals" {
		t.Fatalf("scopes: %v", scopes)
	}
	v := vars(t, c.ok("variables", map[string]any{"variablesReference": ref(globals)}))
	if v["LV_COUNTER"]["value"] != "7" || ref(v["LV_COUNTER"]) != 0 {
		t.Errorf("LV_COUNTER: %v", v["LV_COUNTER"])
	}
	if ref(v["LS_ROW"]) == 0 || ref(v["LT_ROWS"]) == 0 {
		t.Fatalf("a structure and a table must expand: %v", v)
	}
	if v["LT_ROWS"]["value"] != "TY_ROWS [2 rows]" {
		t.Errorf("table value: %v", v["LT_ROWS"]["value"])
	}
	comps := vars(t, c.ok("variables", map[string]any{"variablesReference": ref(v["LS_ROW"])}))
	if comps["NAME"]["value"] != "second" || comps["ID"]["value"] != "2" {
		t.Errorf("structure components: %v", comps)
	}
	rows := vars(t, c.ok("variables", map[string]any{"variablesReference": ref(v["LT_ROWS"])}))
	if _, ok := rows["[1]"]; !ok || len(rows) != 2 {
		t.Errorf("table rows: %v", rows)
	}

	// Step into the FORM: two ABAP frames now, and the caller's scopes are
	// read after moving the ADT cursor to it.
	c.ok("stepIn", map[string]any{"threadId": 1})
	if r := c.event("stopped")["reason"]; r != "step" {
		t.Fatalf("after stepIn: %v", r)
	}
	frames = c.ok("stackTrace", map[string]any{"threadId": 1})["stackFrames"].([]any)
	if f := frames[0].(map[string]any); f["line"].(float64) != 9 || !strings.Contains(f["name"].(string), "DOUBLE") {
		t.Fatalf("in the form: %v", f)
	}
	c.ok("scopes", map[string]any{"frameId": frames[1].(map[string]any)["id"]})
	if fake.cursor != "/sap/bc/adt/debugger/stack/type/ABAP/position/1" {
		t.Errorf("the caller's frame was not selected: cursor %q", fake.cursor)
	}

	c.ok("stepOut", map[string]any{"threadId": 1})
	c.event("stopped")
	c.ok("next", map[string]any{"threadId": 1})
	// Line 28 is the last statement: stepping over it ends the program, and
	// the adapter goes back to listening for the next run.
	out := c.event("output")
	for !strings.Contains(fmt.Sprint(out["output"]), "ended") {
		out = c.event("output")
	}

	// Run the report again: it stops again on the same breakpoint.
	fake.run()
	c.event("stopped")
	c.ok("continue", map[string]any{"threadId": 1})
	for !strings.Contains(fmt.Sprint(c.event("output")["output"]), "ended") {
	}

	c.ok("disconnect", map[string]any{})
	select {
	case err := <-c.served:
		if err != nil {
			t.Fatalf("serve: %v", err)
		}
	case <-time.After(waitFor):
		t.Fatal("the adapter did not end after disconnect")
	}
	c.served <- nil // for the cleanup

	if !fake.listenerDeleted {
		t.Error("disconnect left the listener registered")
	}
	if fake.lastBPSet != 0 {
		t.Errorf("disconnect left %d breakpoints registered", fake.lastBPSet)
	}
}

// Disconnecting while the listener waits releases the session: the wait is
// cut short, the breakpoints are withdrawn and the listener deleted.
func TestDisconnectReleasesTheSession(t *testing.T) {
	fake := newFakeADT()
	c, _ := startAdapter(t, fakeOpener(fake, false, nil))
	path := demoFile(t)
	c.ok("initialize", map[string]any{})
	c.ok("launch", map[string]any{})
	c.event("initialized")
	c.ok("setBreakpoints", map[string]any{"source": map[string]any{"path": path}, "breakpoints": []any{map[string]any{"line": 26}}})
	c.ok("configurationDone", nil)

	deadline := time.Now().Add(waitFor)
	for {
		fake.mu.Lock()
		listening := fake.listening
		fake.mu.Unlock()
		if listening || time.Now().After(deadline) {
			break
		}
		time.Sleep(5 * time.Millisecond)
	}

	c.ok("disconnect", map[string]any{"terminateDebuggee": false})
	<-c.served
	c.served <- nil

	if !fake.listenerDeleted || !fake.detachTried {
		t.Errorf("listener deleted=%v detach tried=%v; calls: %v", fake.listenerDeleted, fake.detachTried, fake.calls())
	}
	if fake.lastBPSet != 0 {
		t.Errorf("%d breakpoints left registered", fake.lastBPSet)
	}
}

// The stream ending without a disconnect — the editor crashed — still
// releases the session.
func TestEndOfStreamReleasesTheSession(t *testing.T) {
	fake := newFakeADT()
	c, _ := startAdapter(t, fakeOpener(fake, false, nil))
	c.ok("initialize", map[string]any{})
	c.ok("attach", map[string]any{})
	c.ok("setBreakpoints", map[string]any{"source": map[string]any{"path": demoFile(t)}, "breakpoints": []any{map[string]any{"line": 26}}})
	c.ok("configurationDone", nil)
	fake.run()
	c.event("stopped")
	c.in.Close()
	select {
	case <-c.served:
	case <-time.After(waitFor):
		t.Fatal("the adapter did not end with its input")
	}
	c.served <- nil
	if !fake.listenerDeleted {
		t.Errorf("the session was not released; calls: %v", fake.calls())
	}
}

// Malformed frames are skipped with a note; the conversation carries on.
func TestMalformedFramesDoNotCrash(t *testing.T) {
	fake := newFakeADT()
	c, _ := startAdapter(t, fakeOpener(fake, false, nil))
	garbage := []string{
		"Content-Length: 7\r\n\r\n{nope!}",               // not JSON
		"Content-Length: banana\r\n\r\n",                 // unreadable length
		"X-Nothing: here\r\n\r\n",                        // no length at all
		"Content-Length: 99999999999\r\n\r\n",            // absurd length (overflow)
		"\r\n\r\n",                                       // blank lines
		"Content-Length: 2\r\n\r\n[]",                    // JSON, wrong shape
		frame(`{"seq":1,"type":"request","command":42}`), // wrong type
		"Content-Length: 5\r\n\r\n{\"a\":",               // truncated JSON
	}
	for _, g := range garbage {
		if _, err := io.WriteString(c.in, g); err != nil {
			t.Fatal(err)
		}
	}
	// Requests the adapter does not know, or before a session, fail cleanly.
	for _, cmd := range []string{"stackTrace", "variables", "continue", "frobnicate", "evaluate"} {
		if res := c.request(cmd, map[string]any{"variablesReference": 99}); res["success"] != false {
			t.Errorf("%s should fail without a session: %v", cmd, res)
		}
	}
	if res := c.request("setBreakpoints", "not an object"); res["success"] != false {
		t.Errorf("bad arguments should fail: %v", res)
	}
	caps := c.ok("initialize", map[string]any{})
	if caps["supportsConfigurationDoneRequest"] != true {
		t.Fatalf("the adapter is not answering after garbage: %v", caps)
	}
}

// A read-only system still debugs — breakpoints, stops, steps and variables
// — but refuses to change a variable, the line `vsp debug ui` and the REPL
// draw.
func TestReadOnlyRefusesSetVariable(t *testing.T) {
	for _, readOnly := range []bool{true, false} {
		t.Run(fmt.Sprintf("readOnly=%v", readOnly), func(t *testing.T) {
			fake := newFakeADT()
			c, _ := startAdapter(t, fakeOpener(fake, readOnly, nil))
			c.ok("initialize", map[string]any{})
			c.ok("attach", map[string]any{})
			c.ok("setBreakpoints", map[string]any{"source": map[string]any{"path": demoFile(t)}, "breakpoints": []any{map[string]any{"line": 26}}})
			c.ok("configurationDone", nil)
			fake.run()
			c.event("stopped")
			frames := c.ok("stackTrace", map[string]any{"threadId": 1})["stackFrames"].([]any)
			scopes := c.ok("scopes", map[string]any{"frameId": frames[0].(map[string]any)["id"]})["scopes"].([]any)
			g := ref(scopes[0].(map[string]any))
			c.ok("variables", map[string]any{"variablesReference": g})

			res := c.request("setVariable", map[string]any{"variablesReference": g, "name": "LV_COUNTER", "value": "900"})
			if readOnly {
				if res["success"] != false || !strings.Contains(strings.ToLower(fmt.Sprint(res["message"])), "read-only") {
					t.Fatalf("read-only setVariable: %v", res)
				}
				if fake.counter != "7" {
					t.Fatal("the value changed on a read-only system")
				}
				return
			}
			if res["success"] != true || fake.counter != "900" {
				t.Fatalf("setVariable: %v (counter %s)", res, fake.counter)
			}
		})
	}
}

// launch with noDebug is refused: vsp dap never runs code itself.
func TestNoDebugIsRefused(t *testing.T) {
	var opened []LaunchArgs
	c, _ := startAdapter(t, fakeOpener(newFakeADT(), false, &opened))
	c.ok("initialize", map[string]any{})
	if res := c.request("launch", map[string]any{"noDebug": true}); res["success"] != false {
		t.Fatalf("noDebug launch: %v", res)
	}
	if len(opened) != 0 {
		t.Fatal("a session was opened for a noDebug launch")
	}
}

// A frame in an object with no local file is served by reference, and its
// text read from SAP.
func TestFrameWithoutLocalFileIsServedByReference(t *testing.T) {
	fake := newFakeADT()
	c, _ := startAdapter(t, fakeOpener(fake, false, nil))
	c.ok("initialize", map[string]any{})
	c.ok("attach", map[string]any{})
	c.ok("setBreakpoints", map[string]any{"source": map[string]any{"path": "vsp://ZVSP_DEBUG_DEMO"}, "breakpoints": []any{map[string]any{"line": 26}}})
	c.ok("configurationDone", nil)
	fake.run()
	c.event("stopped")
	top := c.ok("stackTrace", map[string]any{"threadId": 1})["stackFrames"].([]any)[0].(map[string]any)
	src := top["source"].(map[string]any)
	n, _ := src["sourceReference"].(float64)
	if n == 0 || src["path"] != nil {
		t.Fatalf("expected a source reference: %v", src)
	}
	body := c.ok("source", map[string]any{"sourceReference": n, "source": src})
	if !strings.HasPrefix(body["content"].(string), "REPORT zvsp_debug_demo") {
		t.Fatalf("source: %v", body)
	}
}

// A codec round trip, and a frame bigger than the limit is skipped rather
// than allocated.
func TestCodec(t *testing.T) {
	var buf bytes.Buffer
	conn := NewConn(&buf, &buf)
	if err := conn.Emit("output", map[string]any{"output": "hi"}); err != nil {
		t.Fatal(err)
	}
	msg, err := conn.Read()
	if err != nil || msg.Type != "event" || msg.Seq != 1 {
		t.Fatalf("round trip: %+v %v", msg, err)
	}

	big := fmt.Sprintf("Content-Length: %d\r\n\r\n%s", maxFrameBytes+1, strings.Repeat("x", maxFrameBytes+1))
	conn = NewConn(strings.NewReader(big+"Content-Length: 2\r\n\r\n{}"), io.Discard)
	if _, err := conn.Read(); !IsFrameError(err) {
		t.Fatalf("oversized frame: %v", err)
	}
	if msg, err := conn.Read(); err != nil || msg == nil {
		t.Fatalf("the stream should carry on after an oversized frame: %v", err)
	}
}
