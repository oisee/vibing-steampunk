package dap

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/oisee/vibing-steampunk/pkg/saprfc"
)

// The fake serialises its stateful session the way SAP does, which is what
// the tests below depend on: a listener the client gave up on still holds the
// session, a request sent behind it waits, and a stateless request does not.
func TestFakeSerialisesTheSession(t *testing.T) {
	fake := newFakeADT()
	fake.second, fake.listenEndsOnDelete = time.Second, true

	lctx, stop := context.WithCancel(context.Background())
	listened := make(chan error, 1)
	go func() {
		_, err := fake.Do(lctx, saprfc.ADTRequest{Method: "POST", URI: "/sap/bc/adt/debugger/listeners?timeout=60"})
		listened <- err
	}()
	if !fake.waitFor(func(f *fakeADT) bool { return f.openListens == 1 }) {
		t.Fatal("the listener never started")
	}
	stop()
	if err := <-listened; err == nil {
		t.Fatal("an abandoned listen answered")
	}

	// The session is still busy with the abandoned listen.
	qctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	_, err := fake.Do(qctx, saprfc.ADTRequest{Method: "POST", URI: "/sap/bc/adt/debugger/breakpoints"})
	cancel()
	if err == nil {
		t.Fatal("a request on the session went past the open listen")
	}

	// A stateless request is not queued; and where SAP ends a listen whose
	// listener is removed, removing it frees the session.
	dctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	if _, err := fake.aside().Do(dctx, saprfc.ADTRequest{Method: "DELETE", URI: "/sap/bc/adt/debugger/listeners"}); err != nil {
		t.Fatalf("a stateless request was held up: %v", err)
	}
	bctx, cancel2 := context.WithTimeout(context.Background(), time.Second)
	defer cancel2()
	if _, err := fake.Do(bctx, saprfc.ADTRequest{Method: "POST", URI: "/sap/bc/adt/debugger/breakpoints"}); err != nil {
		t.Fatalf("the session stayed busy after the listener was removed: %v", err)
	}
}

// warnings returns the "may still hold" outputs among the events seen.
func warnings(c *client) []string {
	var out []string
	for _, e := range c.events {
		body, _ := e["body"].(map[string]any)
		if text := fmt.Sprint(body["output"]); e["event"] == "output" && strings.Contains(text, "SAP may still hold") {
			out = append(out, text)
		}
	}
	return out
}

// A disconnect while the listener waits is not queued behind it. Measured on
// A4H before this was fixed: the breakpoint clear queued behind the open
// listen, timed out, and the disconnect took 58 s and warned that SAP may
// still hold the breakpoints — which it did not.
//
// Here the listen holds the session for 6 s after the client gives up on it
// (listenSeconds 60, a "second" of 100 ms), and each release step may take
// 2 s. The disconnect has to be answered well inside the listen.
func TestDisconnectDuringAListenIsNotQueuedBehindIt(t *testing.T) {
	cases := map[string]struct {
		endsOnDelete bool
		slice        int
		within       time.Duration
	}{
		// The listener's removal from the side connection ends the listen.
		"SAP ends the listen when its listener is removed": {endsOnDelete: true, slice: 60, within: time.Second},
		// It does not: the listen runs out its slice, and the release waits
		// for one slice at most, not the whole listenSeconds.
		"SAP runs the listen to its timeout": {endsOnDelete: false, slice: 10, within: 3 * time.Second},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			savedStep, savedSlice := releaseStepTimeout, listenSliceSeconds
			releaseStepTimeout, listenSliceSeconds = 2*time.Second, tc.slice
			t.Cleanup(func() { releaseStepTimeout, listenSliceSeconds = savedStep, savedSlice })

			fake := newFakeADT()
			fake.second, fake.listenEndsOnDelete = 100*time.Millisecond, tc.endsOnDelete
			c := armed(t, fake)

			start := time.Now()
			res := c.request("disconnect", map[string]any{})
			took := time.Since(start)
			if res["success"] != true {
				t.Fatalf("disconnect failed: %v", res["message"])
			}
			if took > tc.within {
				t.Errorf("the disconnect took %v, want under %v; calls: %v; given up while queued: %v", took, tc.within, fake.calls(), fake.queuedOut)
			}

			fake.mu.Lock()
			bps, lastSet, deleted, listening := len(fake.bps), fake.lastBPSet, fake.listenerDeleted, fake.listening
			fake.mu.Unlock()
			if bps != 0 || lastSet != 0 {
				t.Errorf("breakpoints left registered: %d", bps)
			}
			if !deleted || listening {
				t.Errorf("the listener is still registered: deleted=%v listening=%v", deleted, listening)
			}
			if w := warnings(c); len(w) > 0 {
				t.Errorf("warned about what was released: %v", w)
			}
		})
	}
}

// A clear that is genuinely stuck — not merely queued — is still reported,
// and says that its removal could not be verified.
func TestStuckBreakpointClearStillWarns(t *testing.T) {
	saved := releaseStepTimeout
	releaseStepTimeout = 200 * time.Millisecond
	t.Cleanup(func() { releaseStepTimeout = saved })

	fake := newFakeADT()
	fake.hangBPClear = true
	c := armed(t, fake)
	c.ok("disconnect", map[string]any{})

	w := warnings(c)
	if len(w) != 1 || !strings.Contains(w[0], "breakpoints") || !strings.Contains(w[0], "could not verify") {
		t.Fatalf("want one warning about unverified breakpoints; got %v", w)
	}
	fake.mu.Lock()
	deleted := fake.listenerDeleted
	fake.mu.Unlock()
	if !deleted {
		t.Errorf("a stuck clear kept the listener registered; calls: %v", fake.calls())
	}
}

// With no listen open — the program is stopped and attached — the release
// runs on the session alone, in the order it always has: the side connection
// is for a session that is busy, and removing the listener before the
// debuggee is let go is not what a stopped release does.
func TestStoppedReleaseStaysOnTheSession(t *testing.T) {
	fake := newFakeADT()
	c := armed(t, fake)
	fake.run()
	c.event("stopped")
	c.ok("disconnect", map[string]any{})

	for _, call := range fake.calls() {
		if strings.HasPrefix(call, "aside ") {
			t.Errorf("the release of a stopped session used the side connection: %v", fake.calls())
			break
		}
	}
	fake.mu.Lock()
	deleted, attached := fake.listenerDeleted, fake.attached
	fake.mu.Unlock()
	if !deleted || attached {
		t.Errorf("after the release: listener deleted=%v still attached=%v", deleted, attached)
	}
	if w := warnings(c); len(w) > 0 {
		t.Errorf("warned: %v", w)
	}
}
