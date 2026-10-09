package serve

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/oisee/vibing-steampunk/pkg/sncrfc"
	"github.com/oisee/vibing-steampunk/pkg/sncrfc/runtimeguard"
)

type frameResult struct {
	b   []byte
	err error
}

// workerProc is the guarded child that loads the SDK. The supervisor talks to it
// only in frames; its stderr is discarded so native output never leaks.
type workerProc struct {
	cmd      *exec.Cmd
	toWorker io.WriteCloser
	frames   chan frameResult
	cleanup  func() error
	identity sncrfc.Identity
	dead     bool
}

// startWorker starts snc-serve-worker (this executable) with the same arguments and waits for its
// readiness report, bounded by -timeout. Errors are safe to show.
func startWorker(args []string, p probeFlags) (*workerProc, error) {
	exe, err := os.Executable()
	if err != nil {
		return nil, errors.New("cannot locate own executable")
	}
	dir, cleanup, err := runtimeguard.Prepare()
	if err != nil {
		return nil, errors.New("cannot prepare the guarded runtime directory")
	}
	cmd := exec.Command(exe, append([]string{WorkerCommand}, args...)...)
	cmd.Dir = dir
	hideWorker(cmd)
	cmd.Env = append(os.Environ(), "RFC_TRACE=0", "RFC_TRACE_DIR="+dir, "CPIC_TRACE=0", "SNC_TRACE=0", workerEnv+"=1")
	cmd.Stderr = io.Discard
	toWorker, err := cmd.StdinPipe()
	if err != nil {
		_ = cleanup()
		return nil, errors.New("cannot open worker stdin")
	}
	fromWorker, err := cmd.StdoutPipe()
	if err != nil {
		_ = cleanup()
		return nil, errors.New("cannot open worker stdout")
	}
	if err := cmd.Start(); err != nil {
		_ = cleanup()
		return nil, errors.New("cannot start worker")
	}
	w := &workerProc{cmd: cmd, toWorker: toWorker, frames: make(chan frameResult, 1), cleanup: cleanup}
	go func() {
		for {
			b, err := readFrame(fromWorker, maxResponseFrame)
			w.frames <- frameResult{b, err}
			if err != nil {
				return
			}
		}
	}()
	fail := func(msg string) (*workerProc, error) { w.kill(); return nil, errors.New(msg) }
	b, err := w.next(p.timeout)
	if errors.Is(err, context.DeadlineExceeded) {
		return fail("logon deadline exceeded; no retry performed")
	}
	if err != nil {
		return fail("worker ended before logon completed")
	}
	var r probeResult
	d := json.NewDecoder(bytes.NewReader(b))
	d.DisallowUnknownFields()
	if d.Decode(&r) != nil {
		return fail("malformed worker readiness")
	}
	if !r.OK {
		stages := map[string]bool{"landscape": true, "profile": true, "logon": true, "identity": true, "describe": true, "timeout": true}
		stage := "worker"
		if stages[r.Stage] {
			stage = r.Stage
		}
		msg := "not ready at stage " + stage + "; no retry performed"
		if r.Code != nil {
			msg += fmt.Sprintf(" (rfc_code %d)", *r.Code)
		}
		return fail(msg)
	}
	if r.Identity == nil || r.Identity.System != p.system || r.Identity.Client != p.client || !strings.EqualFold(r.Identity.User, p.user) || !r.SNC {
		return fail("worker identity does not match the requested target")
	}
	w.identity = *r.Identity
	return w, nil
}

func (w *workerProc) next(d time.Duration) ([]byte, error) {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case f := <-w.frames:
		return f.b, f.err
	case <-t.C:
		return nil, context.DeadlineExceeded
	}
}

// roundTrip sends one already-checked request frame and returns the response
// frame. Any error leaves the worker killed; the caller must not reuse it.
func (w *workerProc) roundTrip(req []byte, timeout time.Duration) ([]byte, error) {
	if w.dead {
		return nil, errors.New("snc-serve worker is stopped; restart vsp")
	}
	if writeFrame(w.toWorker, req) != nil {
		w.kill()
		return nil, errors.New("worker is gone")
	}
	resp, err := w.next(timeout)
	if errors.Is(err, context.DeadlineExceeded) {
		w.kill()
		return nil, fmt.Errorf("request exceeded %s; snc-serve stopped", timeout)
	}
	if err != nil {
		w.kill()
		return nil, errors.New("worker output is not a valid frame")
	}
	return resp, nil
}

// close ends the worker cleanly: stdin EOF makes it close the RFC connection.
func (w *workerProc) close() error {
	if w.dead {
		return nil
	}
	w.dead = true
	_ = w.toWorker.Close()
	done := make(chan struct{})
	go func() { _ = w.cmd.Wait(); close(done) }()
	select {
	case <-done:
	case <-time.After(10 * time.Second):
		_ = w.cmd.Process.Kill()
		<-done
	}
	if w.cleanup() != nil {
		return errors.New("runtime cleanup failed")
	}
	return nil
}

func (w *workerProc) kill() {
	if w.dead {
		return
	}
	w.dead = true
	_ = w.cmd.Process.Kill()
	_ = w.toWorker.Close()
	_ = w.cmd.Wait()
	_ = w.cleanup()
}
