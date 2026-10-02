package dap

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/url"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/oisee/vibing-steampunk/pkg/adt"
	"github.com/oisee/vibing-steampunk/pkg/saprfc"
)

// threadID is the one thread the adapter reports. An ABAP debuggee is one work
// process stopped at one place; DAP needs a thread to hang the stop on.
const threadID = 1

// defaultListenSeconds is how long one listener request waits server-side
// before it is reissued. Short enough that a disconnect is honoured promptly
// even on a transport that cannot cancel a request in flight.
const defaultListenSeconds = 60

// cleanupTimeout bounds the teardown: releasing a debuggee and deleting the
// listener must reach SAP even when the client has gone, but must not hang the
// exit.
const cleanupTimeout = 30 * time.Second

// releaseStepTimeout bounds each step of the release on its own.
var releaseStepTimeout = 15 * time.Second

// listenBackoff is the wait after a failed listen, multiplied by the number of
// failures in a row.
var listenBackoff = time.Second

// beforeBreakpointPost runs between capturing the session and posting a
// breakpoint set. A test seam: it lets a test end the session in exactly the
// window a request could otherwise post into a released one. Nil in use.
var beforeBreakpointPost func()

// beforeResolve runs between capturing the session and resolving a vsp://
// object name through it. A test seam, like beforeBreakpointPost. Nil in use.
var beforeResolve func()

// errSessionEnded answers a request whose session was released while it waited.
var errSessionEnded = errors.New("session ended: the debug session was released; start a new one")

// lockSession takes the debugger for a request that captured sess earlier, and
// refuses if that session has since been released (or replaced by a newer
// launch). The check is made under dbgMu, the lock release takes before it
// clears anything, and release drops the session before taking it. So a
// request either reaches SAP before the release, which then cleans up after
// it, or not at all; it can never post into a session that was already given
// back, where what it set would stay armed with nobody to remove it.
//
// The alternative, making release wait for requests in flight, was not taken:
// release runs on the worker and from signal handlers, and must not depend on
// a request finishing.
func (s *Server) lockSession(sess *Session) error {
	s.dbgMu.Lock()
	s.mu.Lock()
	current := s.sess == sess && sess != nil
	s.mu.Unlock()
	if !current {
		s.dbgMu.Unlock()
		return errSessionEnded
	}
	return nil
}

// LaunchArgs are the arguments of launch and attach. They are the same for
// both: vsp never starts the program itself, so "launch" also means "arm the
// breakpoints and wait for the program to reach one".
type LaunchArgs struct {
	// System names a system from .vsp.json; empty means vsp's normal choice
	// (--system, then the config default, then SAP_* env).
	System string `json:"system,omitempty"`
	// User is whose debuggees to catch; empty means the logon user.
	User string `json:"user,omitempty"`
	// Object is the program, class or function module being debugged. It is
	// optional: it is resolved at launch so a typo fails early, and a source
	// whose file name follows no convention is mapped to it.
	Object string `json:"object,omitempty"`
	// Include narrows Object to one include, e.g. a function group's LZFGU01.
	Include string `json:"include,omitempty"`
	// SourceRoot is a directory of exported sources (vsp's or abapGit's file
	// names). Stack frames in objects found there open the local file.
	SourceRoot string `json:"sourceRoot,omitempty"`
	// SystemDebugging lets breakpoints in SAP standard code fire.
	SystemDebugging bool `json:"systemDebugging,omitempty"`
	// ListenSeconds is the server-side wait of one listener request.
	ListenSeconds int `json:"listenSeconds,omitempty"`
	// NoDebug is set by clients for "run without debugging", which vsp dap
	// does not do: it never runs code.
	NoDebug bool `json:"noDebug,omitempty"`
}

// Session is an opened debug session: the shared debugger layer plus what the
// adapter needs to know about how it was opened.
type Session struct {
	Debugger *saprfc.Debugger
	// User is whose debuggees the listener catches. Required.
	User string
	// System is the system's name, for messages.
	System string
	// ReadOnly is the system's read_only / SAP_READ_ONLY / --read-only. It
	// refuses what changes the debuggee's data (setVariable), the same line
	// `vsp debug ui` and the debug REPL draw.
	ReadOnly bool
}

// Opener opens the debug session for a launch or attach. It is where the
// logon comes from — vsp's normal config — so the adapter itself never sees
// or stores a credential.
type Opener func(ctx context.Context, args LaunchArgs) (*Session, error)

// Server is one DAP conversation.
type Server struct {
	conn *Conn
	open Opener
	ctx  context.Context

	// dbgMu serialises use of the debugger: one stateful ADT session answers
	// one request at a time. It is never held while waiting for the worker.
	dbgMu sync.Mutex

	// mu guards everything below.
	mu         sync.Mutex
	sess       *Session
	args       LaunchArgs
	target     string // the launch object's source URI, if one was named
	configured bool
	stopped    bool
	stopReason string
	frames     []adt.DebugStackEntry
	cursor     int // the frame index the ADT cursor sits on
	refs       map[int]*varRef
	srcRefs    map[int]string
	srcRefOf   map[string]int
	nextRef    int
	bps        map[string]*sourceBreakpoints // by normalised source URI
	bpDirty    bool
	nextBPID   int
	src        *sourceIndex
	lineBase   int  // 1 unless the client counts lines from 0
	colBase    int  // 1 unless the client counts columns from 0
	pathURI    bool // the client names files as file:// URIs (pathFormat "uri")
	worker     *worker
	ended      bool

	shutdownOnce sync.Once
}

type worker struct {
	cancel context.CancelFunc
	done   chan struct{}
}

// varRef is one expandable thing in the variables view.
type varRef struct {
	frame int
	id    string
	// names maps a child's displayed name to its debugger id, for setVariable.
	names map[string]string
}

// sourceBreakpoints is the client's breakpoint set for one source.
type sourceBreakpoints struct {
	path string
	uri  string
	bps  []*breakpoint
}

type breakpoint struct {
	id       int
	line     int
	verified bool
	message  string
}

// New makes a server on a byte stream.
func New(ctx context.Context, r io.Reader, w io.Writer, open Opener) *Server {
	return &Server{
		conn:     NewConn(r, w),
		open:     open,
		ctx:      ctx,
		refs:     map[int]*varRef{},
		srcRefs:  map[int]string{},
		srcRefOf: map[string]int{},
		bps:      map[string]*sourceBreakpoints{},
		src:      newSourceIndex(),
		lineBase: 1,
		colBase:  1,
	}
}

// Serve reads requests until the client disconnects or the stream ends. The
// debug session is released either way.
func (s *Server) Serve() error {
	defer s.Shutdown()
	for {
		msg, err := s.conn.Read()
		if err != nil {
			if IsFrameError(err) {
				s.output("stderr", "vsp dap: "+err.Error()+"\n")
				continue
			}
			if errors.Is(err, io.EOF) {
				return nil
			}
			s.output("stderr", "vsp dap: ending the session: "+err.Error()+"\n")
			return err
		}
		if msg.Type != "request" {
			continue
		}
		if s.dispatch(msg) {
			return nil
		}
	}
}

// dispatch handles one request and reports whether the conversation is over.
// A handler that panics fails its request instead of taking the adapter down
// and leaving a debuggee suspended in somebody's work process.
func (s *Server) dispatch(msg *ProtocolMessage) (done bool) {
	defer func() {
		if r := recover(); r != nil {
			_ = s.conn.Respond(msg, nil, fmt.Errorf("vsp dap: internal error in %s: %v", msg.Command, r))
			done = false
		}
	}()
	var (
		body any
		err  error
	)
	switch msg.Command {
	case "initialize":
		body, err = s.initialize(msg)
	case "launch", "attach":
		err = s.launch(msg)
		if err == nil {
			_ = s.conn.Respond(msg, nil, nil)
			_ = s.conn.Emit("initialized", nil)
			s.maybeStartListening()
			return false
		}
	case "setBreakpoints":
		body, err = s.setBreakpoints(msg)
	case "setExceptionBreakpoints":
		body = map[string]any{"breakpoints": []any{}}
	case "configurationDone":
		s.mu.Lock()
		s.configured = true
		s.mu.Unlock()
		_ = s.conn.Respond(msg, nil, nil)
		s.maybeStartListening()
		return false
	case "threads":
		body = s.threads()
	case "stackTrace":
		body, err = s.stackTrace(msg)
	case "scopes":
		body, err = s.scopes(msg)
	case "variables":
		body, err = s.variables(msg)
	case "setVariable":
		body, err = s.setVariable(msg)
	case "source":
		body, err = s.source(msg)
	case "continue", "next", "stepIn", "stepOut":
		kind := map[string]string{
			"continue": "stepContinue", "next": "stepOver",
			"stepIn": "stepInto", "stepOut": "stepReturn",
		}[msg.Command]
		if err = s.checkStopped(); err == nil {
			if msg.Command == "continue" {
				body = map[string]any{"allThreadsContinued": true}
			}
			// The answer goes first: a stopped event that overtakes it leaves
			// a client believing the thread runs while it is stopped.
			_ = s.conn.Respond(msg, body, nil)
			s.startWorker(kind, "breakpoint")
			return false
		}
	case "disconnect":
		s.disconnect(msg)
		_ = s.conn.Respond(msg, nil, nil)
		return true
	case "pause":
		err = errors.New("pause is not supported: ABAP stops at breakpoints only")
	case "evaluate":
		err = errors.New("evaluate is not supported yet; expand the variable in the Variables view")
	default:
		err = fmt.Errorf("vsp dap does not support %q", msg.Command)
	}
	_ = s.conn.Respond(msg, body, err)
	return false
}

func decode(msg *ProtocolMessage, v any) error {
	if len(msg.Arguments) == 0 {
		return nil
	}
	if err := json.Unmarshal(msg.Arguments, v); err != nil {
		return fmt.Errorf("%s: bad arguments: %w", msg.Command, err)
	}
	return nil
}

func (s *Server) initialize(msg *ProtocolMessage) (any, error) {
	var args struct {
		LinesStartAt1   *bool  `json:"linesStartAt1"`
		ColumnsStartAt1 *bool  `json:"columnsStartAt1"`
		PathFormat      string `json:"pathFormat"`
	}
	if err := decode(msg, &args); err != nil {
		return nil, err
	}
	s.mu.Lock()
	if args.LinesStartAt1 != nil && !*args.LinesStartAt1 {
		s.lineBase = 0
	}
	if args.ColumnsStartAt1 != nil && !*args.ColumnsStartAt1 {
		s.colBase = 0
	}
	s.pathURI = strings.EqualFold(args.PathFormat, "uri")
	s.mu.Unlock()
	return map[string]any{
		"supportsConfigurationDoneRequest":  true,
		"supportsConditionalBreakpoints":    false,
		"supportsHitConditionalBreakpoints": false,
		"supportsEvaluateForHovers":         false,
		"supportsSetVariable":               true,
		"supportsTerminateDebuggee":         true,
		"supportsStepBack":                  false,
		"supportsRestartRequest":            false,
		"exceptionBreakpointFilters":        []any{},
	}, nil
}

func (s *Server) launch(msg *ProtocolMessage) error {
	var args LaunchArgs
	if err := decode(msg, &args); err != nil {
		return err
	}
	if args.NoDebug {
		return errors.New("vsp dap only debugs: it never runs the program itself. Start the debug session, then run the program in SAP")
	}
	s.mu.Lock()
	already := s.sess != nil
	s.mu.Unlock()
	if already {
		return errors.New("a debug session is already open")
	}
	if args.ListenSeconds <= 0 {
		args.ListenSeconds = defaultListenSeconds
	}
	sess, err := s.open(s.ctx, args)
	if err != nil {
		return err
	}
	if sess == nil || sess.Debugger == nil {
		return errors.New("no debug session was opened")
	}
	if strings.TrimSpace(sess.User) == "" {
		_ = sess.Debugger.Close(s.ctx)
		return errors.New("no user to listen for: pass \"user\" in the launch configuration")
	}
	if args.SystemDebugging {
		sess.Debugger.SystemDebugging(true)
	}

	var target string
	if name := strings.TrimSpace(args.Include); name != "" || strings.TrimSpace(args.Object) != "" {
		if name == "" {
			name = strings.TrimSpace(args.Object)
		}
		s.dbgMu.Lock()
		uri, rerr := sess.Debugger.ResolveSourceURI(s.ctx, name)
		s.dbgMu.Unlock()
		if rerr != nil {
			_ = sess.Debugger.Close(s.ctx)
			return fmt.Errorf("resolving %s: %w", name, rerr)
		}
		target = normURI(uri)
	}
	if err := s.src.scan(args.SourceRoot); err != nil {
		s.output("console", fmt.Sprintf("sourceRoot %s not indexed: %v\n", args.SourceRoot, err))
	}

	s.mu.Lock()
	s.sess, s.args, s.target = sess, args, target
	if len(s.bps) > 0 {
		s.bpDirty = true
	}
	s.mu.Unlock()

	note := fmt.Sprintf("vsp dap: listening for debuggees of %s", sess.User)
	if sess.System != "" {
		note += " on " + sess.System
	}
	if target != "" {
		note += "; target " + target
	}
	if sess.ReadOnly {
		note += " (read-only: variables cannot be changed)"
	}
	s.output("console", note+"\n")

	// Breakpoints that arrived before the session existed are placed now;
	// sources that could only be resolved through the session get their turn.
	s.mu.Lock()
	var retry []*sourceBreakpoints
	for key, set := range s.bps {
		if set.uri == "" {
			retry = append(retry, set)
			delete(s.bps, key)
		}
	}
	s.mu.Unlock()
	for _, set := range retry {
		uri, err := s.sourceURI(dapSource{Path: set.path})
		s.mu.Lock()
		if err != nil {
			s.bps["path:"+set.path] = set
			for _, b := range set.bps {
				b.message = err.Error()
			}
		} else {
			set.uri = uri
			s.bps[uri] = set
			if !strings.HasPrefix(set.path, vspScheme) {
				s.src.remember(uri, set.path)
			}
		}
		s.mu.Unlock()
	}
	s.applyBreakpoints(true)
	return nil
}

// --- breakpoints --------------------------------------------------------

type dapSource struct {
	Name             string `json:"name,omitempty"`
	Path             string `json:"path,omitempty"`
	SourceReference  int    `json:"sourceReference,omitempty"`
	Origin           string `json:"origin,omitempty"`
	PresentationHint string `json:"presentationHint,omitempty"`
}

// sourceURI maps a DAP source to the ADT source URI its lines belong to.
func (s *Server) sourceURI(src dapSource) (string, error) {
	if src.SourceReference > 0 {
		s.mu.Lock()
		uri, ok := s.srcRefs[src.SourceReference]
		s.mu.Unlock()
		if ok {
			return uri, nil
		}
	}
	path := src.Path
	if ref, ok := strings.CutPrefix(path, vspScheme); ok {
		if strings.HasPrefix(strings.ToLower(ref), "/sap/bc/adt/") {
			return normURI(ref), nil
		}
		s.mu.Lock()
		sess := s.sess
		s.mu.Unlock()
		if sess == nil {
			return "", errors.New("pending: " + path + " is resolved when the session opens")
		}
		if beforeResolve != nil {
			beforeResolve()
		}
		if err := s.lockSession(sess); err != nil {
			return "", err
		}
		uri, err := sess.Debugger.ResolveSourceURI(s.ctx, strings.Trim(ref, "/"))
		s.dbgMu.Unlock()
		if err != nil {
			return "", err
		}
		return normURI(uri), nil
	}
	if path == "" {
		return "", errors.New("the source has neither a path nor a reference")
	}
	uri, err := fileTarget(path)
	if err == nil {
		return uri, nil
	}
	s.mu.Lock()
	target := s.target
	s.mu.Unlock()
	if target != "" {
		// The launch named the object, and this file's name says nothing the
		// convention can read: it is taken to be that object's source.
		return target, nil
	}
	return "", fmt.Errorf("%s: %v (name the file by vsp's convention, e.g. zcl_x.clas.abap, or set \"object\" in the launch configuration)", filepath.Base(path), err)
}

func (s *Server) setBreakpoints(msg *ProtocolMessage) (any, error) {
	var args struct {
		Source      dapSource `json:"source"`
		Breakpoints []struct {
			Line      int    `json:"line"`
			Condition string `json:"condition"`
		} `json:"breakpoints"`
		Lines []int `json:"lines"`
	}
	if err := decode(msg, &args); err != nil {
		return nil, err
	}
	args.Source.Path = s.fromClientPath(args.Source.Path)
	lines := make([]int, 0, len(args.Breakpoints))
	for _, b := range args.Breakpoints {
		lines = append(lines, b.Line)
	}
	if len(args.Breakpoints) == 0 {
		lines = append(lines, args.Lines...)
	}

	uri, uerr := s.sourceURI(args.Source)

	s.mu.Lock()
	key := uri
	if uerr != nil {
		// Unmapped: kept under its path so a later launch can retry it.
		key = "path:" + args.Source.Path
	}
	set := &sourceBreakpoints{path: args.Source.Path, uri: uri}
	for _, l := range lines {
		s.nextBPID++
		set.bps = append(set.bps, &breakpoint{id: s.nextBPID, line: l - s.lineBase + 1})
	}
	if len(set.bps) == 0 {
		delete(s.bps, key)
	} else {
		s.bps[key] = set
	}
	if uerr == nil && args.Source.Path != "" && !strings.HasPrefix(args.Source.Path, vspScheme) {
		s.src.remember(uri, args.Source.Path)
	}
	s.bpDirty = true
	s.mu.Unlock()

	if uerr != nil {
		for _, b := range set.bps {
			b.message = uerr.Error()
		}
	} else {
		s.applyBreakpoints(false)
	}
	return map[string]any{"breakpoints": s.dapBreakpoints(set)}, nil
}

func (s *Server) dapBreakpoints(set *sourceBreakpoints) []map[string]any {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]map[string]any, 0, len(set.bps))
	for _, b := range set.bps {
		out = append(out, s.dapBreakpointLocked(set, b))
	}
	return out
}

func (s *Server) dapBreakpointLocked(set *sourceBreakpoints, b *breakpoint) map[string]any {
	m := map[string]any{"id": b.id, "verified": b.verified, "line": b.line + s.lineBase - 1}
	if b.message != "" {
		m["message"] = b.message
	}
	if set.path != "" {
		m["source"] = dapSource{Name: filepath.Base(set.path), Path: s.toClientPathLocked(set.path)}
	}
	return m
}

// applyBreakpoints posts the client's whole breakpoint set, if it changed and
// the session can take a request now. ADT's breakpoint resource is a set per
// client, so every change posts all of them. With notify, the verification
// of each is sent as a breakpoint event — used when the set was placed later
// than the client asked.
//
// It is not called while a step or continue is in flight: the session is busy
// until the debuggee stops, so the set is placed at the next stop.
func (s *Server) applyBreakpoints(notify bool) {
	s.mu.Lock()
	sess := s.sess
	busy := s.worker != nil && !s.listening()
	if sess == nil || !s.bpDirty || busy {
		pending := sess == nil || busy
		for _, set := range s.bps {
			if set.uri == "" {
				continue
			}
			for _, b := range set.bps {
				if pending && !b.verified {
					b.message = "pending: placed when the debug session can take it"
				}
			}
		}
		s.mu.Unlock()
		return
	}
	var want []adt.Breakpoint
	for _, set := range s.bps {
		if set.uri == "" {
			continue
		}
		for _, b := range set.bps {
			want = append(want, adt.Breakpoint{Kind: adt.BreakpointKindLine, URI: set.uri, Line: b.line})
		}
	}
	s.bpDirty = false
	s.mu.Unlock()

	// A listener in flight holds the session; it is stopped, the set placed,
	// and the listener reissued.
	if beforeBreakpointPost != nil {
		beforeBreakpointPost()
	}
	relisten := s.stopListening()

	var (
		placed, rejected []adt.Breakpoint
		err              error
	)
	if err = s.lockSession(sess); err == nil {
		placed, err = sess.Debugger.ADTSetBreakpoints(s.ctx, want)
		rejected = sess.Debugger.Rejected()
		s.dbgMu.Unlock()
	} else {
		relisten = false
	}

	type key struct {
		uri  string
		line int
	}
	ok := map[key]bool{}
	for _, p := range placed {
		ok[key{normURI(p.URI), p.Line}] = true
	}
	why := map[key]string{}
	for _, r := range rejected {
		why[key{normURI(r.URI), r.Line}] = r.ErrorMessage
	}

	var events []map[string]any
	s.mu.Lock()
	for _, set := range s.bps {
		if set.uri == "" {
			continue
		}
		for _, b := range set.bps {
			k := key{set.uri, b.line}
			switch {
			case err != nil:
				b.verified, b.message = false, err.Error()
			case ok[k]:
				b.verified, b.message = true, ""
			case why[k] != "":
				b.verified, b.message = false, why[k]
			default:
				b.verified, b.message = false, "SAP did not place this breakpoint"
			}
			if notify {
				events = append(events, s.dapBreakpointLocked(set, b))
			}
		}
	}
	s.mu.Unlock()
	for _, e := range events {
		_ = s.conn.Emit("breakpoint", map[string]any{"reason": "changed", "breakpoint": e})
	}
	if relisten {
		// The listener may have caught a debuggee as it was interrupted; that
		// stop was delivered, and listening again over it would leave the
		// debuggee held with nobody watching.
		s.mu.Lock()
		idle := s.sess == sess && !s.stopped && !s.ended && s.worker == nil
		s.mu.Unlock()
		if idle {
			s.startWorker("", "")
		}
	}
}

// --- the background worker ---------------------------------------------

// listening reports whether the worker in flight is the listener (which may be
// interrupted) rather than a step (which may not). Callers hold mu.
func (s *Server) listening() bool { return s.worker != nil && s.stopReason == "listen" }

// maybeStartListening starts the listener once the session is open and the
// client has finished configuring.
func (s *Server) maybeStartListening() {
	s.mu.Lock()
	ready := s.sess != nil && s.configured && s.worker == nil && !s.stopped && !s.ended
	s.mu.Unlock()
	if ready {
		s.startWorker("", "")
	}
}

// stopListening interrupts a listener in flight and reports whether there was
// one. A step in flight is left alone.
func (s *Server) stopListening() bool {
	s.mu.Lock()
	w := s.worker
	if w == nil || !s.listening() {
		s.mu.Unlock()
		return false
	}
	s.mu.Unlock()
	w.cancel()
	<-w.done
	return true
}

// stopWorker interrupts whatever is in flight. Used on disconnect only: an
// interrupted step leaves the debuggee to the teardown.
func (s *Server) stopWorker() {
	s.mu.Lock()
	w := s.worker
	s.mu.Unlock()
	if w == nil {
		return
	}
	w.cancel()
	<-w.done
}

// startWorker runs a step of the given kind (empty: none) and then, if the
// debuggee did not stop, listens for the next one.
func (s *Server) startWorker(kind, reason string) {
	ctx, cancel := context.WithCancel(s.ctx)
	w := &worker{cancel: cancel, done: make(chan struct{})}
	s.mu.Lock()
	s.worker = w
	s.stopped = false
	s.frames = nil
	s.refs = map[int]*varRef{}
	if kind == "" {
		s.stopReason = "listen"
	} else {
		s.stopReason = "step"
	}
	s.mu.Unlock()

	go func() {
		defer close(w.done)
		defer func() {
			s.mu.Lock()
			if s.worker == w {
				s.worker = nil
			}
			s.mu.Unlock()
		}()
		if kind != "" {
			stopped, why, err := s.step(ctx, kind, reason)
			if ctx.Err() != nil {
				return
			}
			switch {
			case err != nil:
				s.output("stderr", fmt.Sprintf("vsp dap: %s failed: %v\n", kind, err))
				s.terminate()
				return
			case stopped:
				s.halt(why)
				return
			}
			s.output("console", "vsp dap: the program ended; listening for the next debuggee\n")
			s.mu.Lock()
			s.stopReason = "listen"
			s.mu.Unlock()
		}
		s.listen(ctx)
	}()
}

// step performs one step. It reports stopped=false, with no error, when the
// debuggee ran to its end.
func (s *Server) step(ctx context.Context, kind, reason string) (bool, string, error) {
	s.mu.Lock()
	sess := s.sess
	s.mu.Unlock()
	if sess == nil {
		return false, "", errors.New("no debug session")
	}
	s.dbgMu.Lock()
	res, err := sess.Debugger.ADTStep(ctx, kind)
	s.dbgMu.Unlock()
	if err != nil {
		if strings.Contains(err.Error(), "debuggeeEnded") {
			return false, "", nil
		}
		return false, "", err
	}
	if res != nil && len(res.Body) > 0 {
		if st, perr := adt.ParseStepXML(res.Body); perr == nil && len(st.ReachedBreakpoints) > 0 {
			return true, "breakpoint", nil
		}
	}
	if kind == "stepContinue" {
		return true, reason, nil
	}
	return true, "step", nil
}

// listen waits for a debuggee of the session's user, reissuing the listener
// until one stops or the context ends.
func (s *Server) listen(ctx context.Context) {
	s.mu.Lock()
	sess, seconds := s.sess, s.args.ListenSeconds
	s.mu.Unlock()
	if sess == nil {
		return // released while the worker started
	}
	failures := 0
	for ctx.Err() == nil {
		s.dbgMu.Lock()
		who, err := sess.Debugger.ADTListen(ctx, sess.User, saprfc.IDEID, saprfc.TerminalID, seconds)
		if who == nil || ctx.Err() != nil {
			s.dbgMu.Unlock()
			if ctx.Err() != nil {
				// Interrupted. A debuggee the listener already saw is not
				// attached yet; it waits in SAP for the next listener.
				return
			}
			if err != nil {
				failures++
				s.output("stderr", fmt.Sprintf("vsp dap: listening failed: %v\n", err))
				if failures >= 3 {
					s.terminate()
					return
				}
				select {
				case <-ctx.Done():
					return
				case <-time.After(time.Duration(failures) * listenBackoff):
				}
			}
			continue // an error retried, or nobody stopped in this window
		}
		failures = 0
		attached, attachErr := s.attach(ctx, sess, who)
		s.dbgMu.Unlock()

		if attached {
			// Once SAP has attached, the stop is delivered whatever happened
			// to this worker's context meanwhile: the program is held in a
			// work process, and dropping the stop would leave it there with
			// nobody watching.
			if attachErr != nil {
				s.output("stderr", fmt.Sprintf("vsp dap: the attach to %s answered %v, but the debuggee is attached\n", who.Program, attachErr))
			}
			s.output("console", fmt.Sprintf("vsp dap: %s stopped in %s/%s line %d\n", who.User, who.Program, who.Include, who.Line))
			s.halt("breakpoint")
			return
		}
		s.output("stderr", fmt.Sprintf("vsp dap: could not attach to %s at line %d: %v; listening on\n", who.Program, who.Line, attachErr))
	}
}

// attach attaches to a debuggee the listener caught and reports whether the
// session is now attached. An attach that answers with an error is checked
// rather than believed: an interrupted or failed request may still have
// attached on SAP's side, so the stack is read on a fresh context. Attached
// after all, it is a stop; not attached, anything SAP may hold is let go on a
// fresh context, and the caller listens on. Callers hold dbgMu.
func (s *Server) attach(ctx context.Context, sess *Session, who *saprfc.ADTDebuggee) (bool, error) {
	_, err := sess.Debugger.ADTAttach(ctx, who.ID, sess.User)
	if err == nil {
		return true, nil
	}
	fresh, cancel := context.WithTimeout(context.WithoutCancel(s.ctx), releaseStepTimeout)
	defer cancel()
	if _, perr := sess.Debugger.StackInfo(fresh); perr == nil {
		return true, err
	}
	// Not attached as far as the stack can tell. A detach on this session
	// releases a half-made attachment if there is one and is harmless if
	// there is not; it leaves the listener and the breakpoints in place.
	_, _ = sess.Debugger.ADT(fresh, "POST", "/sap/bc/adt/debugger?method=detach",
		[]saprfc.ADTHeader{{Name: "Accept", Value: "*/*"}}, nil)
	return false, err
}

// halt records a stop, places any breakpoints that changed while the program
// ran, and tells the client.
func (s *Server) halt(reason string) {
	s.mu.Lock()
	s.stopped = true
	s.stopReason = ""
	s.cursor = 0
	s.frames = nil
	s.refs = map[int]*varRef{}
	s.worker = nil
	dirty := s.bpDirty
	s.mu.Unlock()
	if dirty {
		s.applyBreakpoints(true)
	}
	_ = s.conn.Emit("stopped", map[string]any{"reason": reason, "threadId": threadID, "allThreadsStopped": true})
}

// terminate ends the debug session from the adapter's side. It releases
// SAP's side at once rather than waiting for a disconnect: an editor may keep
// the adapter running long after it shows "terminated", and until the release
// the breakpoints stay armed, the listener stays registered and a debuggee
// may sit in a work process. It runs on the worker, which holds no lock here.
func (s *Server) terminate() {
	s.mu.Lock()
	s.ended = true
	s.stopped = false
	s.mu.Unlock()
	s.release()
	_ = s.conn.Emit("terminated", nil)
}

func (s *Server) checkStopped() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.sess == nil {
		return errors.New("no debug session")
	}
	if !s.stopped {
		return errors.New("the program is not stopped")
	}
	return nil
}

// --- inspection -----------------------------------------------------------

func (s *Server) threads() any {
	s.mu.Lock()
	defer s.mu.Unlock()
	name := "ABAP"
	if s.sess != nil {
		name = "ABAP (" + s.sess.User + ")"
	}
	return map[string]any{"threads": []map[string]any{{"id": threadID, "name": name}}}
}

// stoppedSession returns the session if the program is stopped.
func (s *Server) stoppedSession() (*Session, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.sess == nil {
		return nil, errors.New("no debug session")
	}
	if !s.stopped {
		return nil, errors.New("the program is running")
	}
	return s.sess, nil
}

// stack reads the call stack once per stop.
func (s *Server) stack() ([]adt.DebugStackEntry, error) {
	sess, err := s.stoppedSession()
	if err != nil {
		return nil, err
	}
	s.mu.Lock()
	frames := s.frames
	s.mu.Unlock()
	if frames != nil {
		return frames, nil
	}
	if err := s.lockSession(sess); err != nil {
		return nil, err
	}
	info, err := sess.Debugger.StackInfo(s.ctx)
	s.dbgMu.Unlock()
	if err != nil {
		return nil, err
	}
	frames = append([]adt.DebugStackEntry{}, info.Stack...)
	s.mu.Lock()
	s.frames = frames
	s.mu.Unlock()
	return frames, nil
}

func (s *Server) sourceRef(uri string) int {
	n := normURI(uri)
	if ref, ok := s.srcRefOf[n]; ok {
		return ref
	}
	s.nextRef++
	s.srcRefs[s.nextRef] = n
	s.srcRefOf[n] = s.nextRef
	return s.nextRef
}

func frameName(e adt.DebugStackEntry) string {
	program := strings.TrimSpace(e.ProgramName)
	switch strings.ToUpper(e.EventType) {
	case "METHOD":
		if cls := strings.TrimRight(strings.TrimSuffix(program, "CP"), "="); cls != program && cls != "" {
			return cls + "->" + e.EventName
		}
		return e.EventName
	case "FUNCTION":
		return e.EventName
	case "":
		return program
	}
	if e.EventName == "" {
		return program
	}
	return fmt.Sprintf("%s %s (%s)", e.EventType, e.EventName, program)
}

func (s *Server) stackTrace(msg *ProtocolMessage) (any, error) {
	var args struct {
		StartFrame int `json:"startFrame"`
		Levels     int `json:"levels"`
	}
	if err := decode(msg, &args); err != nil {
		return nil, err
	}
	frames, err := s.stack()
	if err != nil {
		return nil, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	system := ""
	if s.sess != nil {
		system = s.sess.System
	}
	out := []map[string]any{}
	for i, e := range frames {
		if i < args.StartFrame {
			continue
		}
		if args.Levels > 0 && len(out) >= args.Levels {
			break
		}
		line := uriLine(e.URI)
		if line == 0 {
			line = e.Line
		}
		f := map[string]any{"id": i + 1, "name": frameName(e), "line": line + s.lineBase - 1, "column": s.colBase}
		abap := strings.EqualFold(e.StackType, "ABAP") && e.URI != "" && !strings.Contains(e.URI, "/vit/")
		switch {
		case !abap:
			f["presentationHint"] = "label"
		default:
			src := dapSource{}
			if p := s.src.lookup(e.URI); p != "" {
				src.Name, src.Path = filepath.Base(p), s.toClientPathLocked(p)
			} else {
				src.Name = strings.TrimSpace(e.IncludeName)
				if src.Name == "" {
					src.Name = strings.TrimSpace(e.ProgramName)
				}
				src.SourceReference = s.sourceRef(e.URI)
				src.Origin = strings.TrimSpace("SAP " + system)
			}
			if e.SystemProgram {
				src.PresentationHint = "deemphasize"
			}
			f["source"] = src
		}
		out = append(out, f)
	}
	return map[string]any{"stackFrames": out, "totalFrames": len(frames)}, nil
}

// onFrame moves the ADT cursor to frame i, so the variables read next are
// that frame's. Callers hold dbgMu.
func (s *Server) onFrame(sess *Session, i int) error {
	s.mu.Lock()
	cur := s.cursor
	var uri string
	if i >= 0 && i < len(s.frames) {
		uri = s.frames[i].StackURI
	}
	s.mu.Unlock()
	if i == cur {
		return nil
	}
	if uri == "" {
		return fmt.Errorf("frame %d cannot be inspected on this release", i+1)
	}
	if err := sess.Debugger.GoToFrame(s.ctx, uri); err != nil {
		return err
	}
	s.mu.Lock()
	s.cursor = i
	s.mu.Unlock()
	return nil
}

func (s *Server) newRef(frame int, id string) int {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.nextRef++
	s.refs[s.nextRef] = &varRef{frame: frame, id: id}
	return s.nextRef
}

func (s *Server) scopes(msg *ProtocolMessage) (any, error) {
	var args struct {
		FrameID int `json:"frameId"`
	}
	if err := decode(msg, &args); err != nil {
		return nil, err
	}
	frames, err := s.stack()
	if err != nil {
		return nil, err
	}
	i := args.FrameID - 1
	if i < 0 || i >= len(frames) {
		return nil, fmt.Errorf("no frame %d", args.FrameID)
	}
	sess, err := s.stoppedSession()
	if err != nil {
		return nil, err
	}
	if err := s.lockSession(sess); err != nil {
		return nil, err
	}
	err = s.onFrame(sess, i)
	var roots *adt.DebugChildVariablesInfo
	if err == nil {
		roots, err = sess.Debugger.Expand(s.ctx, "@ROOT")
	}
	s.dbgMu.Unlock()
	if err != nil {
		return nil, err
	}
	out := []map[string]any{}
	if roots != nil {
		for _, h := range roots.Hierarchies {
			name := h.ChildName
			if name == "" {
				name = strings.TrimPrefix(h.ChildID, "@")
			}
			scope := map[string]any{
				"name":               name,
				"variablesReference": s.newRef(i, h.ChildID),
				"expensive":          false,
			}
			switch strings.ToUpper(h.ChildID) {
			case "@LOCALS", "@PARAMETERS":
				scope["presentationHint"] = "locals"
			case "@GLOBALS":
				scope["presentationHint"] = "globals"
			case "@DATAAGING":
				scope["expensive"] = true
			}
			out = append(out, scope)
		}
	}
	return map[string]any{"scopes": out}, nil
}

func varValue(v adt.DebugVariable) string {
	switch v.MetaType {
	case adt.DebugMetaTypeTable:
		return fmt.Sprintf("%s [%d rows]", v.DeclaredTypeName, v.TableLines)
	case adt.DebugMetaTypeStructure:
		return "{…} " + v.DeclaredTypeName
	}
	value := strings.TrimSpace(v.Value)
	if v.IsValueIncomplete {
		value += " …"
	}
	return value
}

func (s *Server) variables(msg *ProtocolMessage) (any, error) {
	var args struct {
		VariablesReference int `json:"variablesReference"`
	}
	if err := decode(msg, &args); err != nil {
		return nil, err
	}
	s.mu.Lock()
	ref := s.refs[args.VariablesReference]
	s.mu.Unlock()
	if ref == nil {
		return nil, fmt.Errorf("variables reference %d is stale; the program has moved on", args.VariablesReference)
	}
	sess, err := s.stoppedSession()
	if err != nil {
		return nil, err
	}
	if err := s.lockSession(sess); err != nil {
		return nil, err
	}
	err = s.onFrame(sess, ref.frame)
	var info *adt.DebugChildVariablesInfo
	var sample *saprfc.TableSample
	if err == nil {
		info, err = sess.Debugger.Expand(s.ctx, ref.id)
		sample = sess.Debugger.LastTableSample()
	}
	s.dbgMu.Unlock()
	if err != nil {
		return nil, err
	}

	out := []map[string]any{}
	names := map[string]string{}
	if info != nil {
		have := map[string]bool{}
		for _, v := range info.Variables {
			have[strings.ToUpper(v.ID)] = true
			name := v.Name
			if name == "" || strings.HasPrefix(v.ID, ref.id+"[") {
				name = v.ID
			}
			// Components and rows are named by their full path; the tree
			// already shows the parent.
			if rest, ok := strings.CutPrefix(name, ref.id+"-"); ok && rest != "" {
				name = rest
			} else if rest, ok := strings.CutPrefix(name, ref.id); ok && strings.HasPrefix(rest, "[") {
				name = rest
			}
			names[name] = v.ID
			item := map[string]any{
				"name":               name,
				"value":              varValue(v),
				"type":               v.DeclaredTypeName,
				"variablesReference": 0,
				"evaluateName":       v.ID,
			}
			expandable := v.IsComplexType() && !(v.MetaType == adt.DebugMetaTypeTable && v.TableLines == 0)
			if expandable {
				item["variablesReference"] = s.newRef(ref.frame, v.ID)
			}
			if v.ReadOnly {
				item["presentationHint"] = map[string]any{"attributes": []string{"readOnly"}}
			}
			out = append(out, item)
		}
		// A synthetic node lists further synthetic nodes as hierarchies only.
		for _, h := range info.Hierarchies {
			if !strings.EqualFold(h.ParentID, ref.id) || have[strings.ToUpper(h.ChildID)] {
				continue
			}
			name := h.ChildName
			if name == "" {
				name = h.ChildID
			}
			out = append(out, map[string]any{
				"name": name, "value": "", "variablesReference": s.newRef(ref.frame, h.ChildID),
			})
		}
	}
	if sample.Partial() {
		out = append(out, map[string]any{
			"name":               "…",
			"value":              fmt.Sprintf("rows %s of %d shown", saprfc.FormatRowRanges(sample.Rows), sample.Lines),
			"variablesReference": 0,
		})
	}
	s.mu.Lock()
	ref.names = names
	s.mu.Unlock()
	return map[string]any{"variables": out}, nil
}

func (s *Server) setVariable(msg *ProtocolMessage) (any, error) {
	var args struct {
		VariablesReference int    `json:"variablesReference"`
		Name               string `json:"name"`
		Value              string `json:"value"`
	}
	if err := decode(msg, &args); err != nil {
		return nil, err
	}
	sess, err := s.stoppedSession()
	if err != nil {
		return nil, err
	}
	// Changing a value changes what the program computes and writes: the
	// same refusal the debug REPL makes on a read-only system.
	safety := adt.SafetyConfig{ReadOnly: sess.ReadOnly}
	if gerr := safety.CheckOperation(adt.OpWorkflow, "DebuggerSetVariable"); gerr != nil {
		return nil, fmt.Errorf("the system is read-only: changing a variable changes what the program computes and writes (%w)", gerr)
	}
	s.mu.Lock()
	ref := s.refs[args.VariablesReference]
	var id string
	if ref != nil {
		id = ref.names[args.Name]
	}
	s.mu.Unlock()
	if ref == nil || id == "" {
		return nil, fmt.Errorf("no variable %s here", args.Name)
	}
	if err := s.lockSession(sess); err != nil {
		return nil, err
	}
	defer s.dbgMu.Unlock()
	if err := s.onFrame(sess, ref.frame); err != nil {
		return nil, err
	}
	if err := sess.Debugger.SetVariable(s.ctx, id, args.Value); err != nil {
		return nil, err
	}
	value := args.Value
	if vars, err := sess.Debugger.Vars(s.ctx, []string{id}); err == nil && len(vars) > 0 {
		value = varValue(vars[0])
	}
	return map[string]any{"value": value}, nil
}

func (s *Server) source(msg *ProtocolMessage) (any, error) {
	var args struct {
		SourceReference int       `json:"sourceReference"`
		Source          dapSource `json:"source"`
	}
	if err := decode(msg, &args); err != nil {
		return nil, err
	}
	ref := args.Source.SourceReference
	if ref == 0 {
		ref = args.SourceReference
	}
	s.mu.Lock()
	uri, ok := s.srcRefs[ref]
	sess := s.sess
	s.mu.Unlock()
	if !ok || sess == nil {
		return nil, fmt.Errorf("unknown source reference %d", ref)
	}
	s.mu.Lock()
	busy := s.worker != nil
	s.mu.Unlock()
	if busy {
		return nil, errors.New("the debug session is busy; open the source again when the program stops")
	}
	if err := s.lockSession(sess); err != nil {
		return nil, err
	}
	res, err := sess.Debugger.ADT(s.ctx, "GET", uri, []saprfc.ADTHeader{{Name: "Accept", Value: "text/plain"}}, nil)
	s.dbgMu.Unlock()
	if err != nil {
		return nil, err
	}
	if res.Status != 200 {
		return nil, fmt.Errorf("reading %s: ADT %d", uri, res.Status)
	}
	return map[string]any{"content": string(res.Body), "mimeType": "text/x-abap"}, nil
}

// --- teardown -------------------------------------------------------------

func (s *Server) disconnect(msg *ProtocolMessage) {
	var args struct {
		TerminateDebuggee bool `json:"terminateDebuggee"`
	}
	_ = decode(msg, &args)
	if args.TerminateDebuggee {
		s.mu.Lock()
		stopped, sess := s.stopped, s.sess
		s.mu.Unlock()
		if stopped && sess != nil {
			ctx, cancel := context.WithTimeout(context.WithoutCancel(s.ctx), cleanupTimeout)
			if s.lockSession(sess) == nil {
				_, _ = sess.Debugger.ADTStep(ctx, "terminateDebuggee")
				s.dbgMu.Unlock()
			}
			cancel()
		}
	}
	s.Shutdown()
}

// Shutdown stops whatever is in flight and releases the session. It runs
// once, on disconnect, on the end of the stream, or from a signal handler.
func (s *Server) Shutdown() {
	s.shutdownOnce.Do(func() {
		s.stopWorker()
		s.release()
	})
}

// release gives back everything the session holds on SAP: the breakpoints
// this client registered, the attached debuggee (which runs on) and the
// listener registration. It is idempotent: the session is taken out under the
// lock, so a second caller finds nothing to do.
//
// Each step gets its own bounded context, detached from the server's own: a
// step that hangs must not spend the budget of the ones after it, and the
// server's context may already be cancelled when the release is needed most.
// A step that fails is reported and the next one still runs.
func (s *Server) release() {
	s.mu.Lock()
	sess := s.sess
	hadBreakpoints := len(s.bps) > 0
	s.sess = nil
	s.stopped = false
	s.mu.Unlock()
	if sess == nil {
		return
	}
	s.dbgMu.Lock()
	defer s.dbgMu.Unlock()

	bounded := func(fn func(context.Context) error) error {
		ctx, cancel := context.WithTimeout(context.WithoutCancel(s.ctx), releaseStepTimeout)
		defer cancel()
		return fn(ctx)
	}
	var held []string
	if hadBreakpoints {
		if err := bounded(sess.Debugger.ADTClearBreakpoints); err != nil {
			held = append(held, fmt.Sprintf("the breakpoints (%v)", err))
		}
	}
	// Detach releases the debuggee and deletes the listener. A first attempt
	// that fails gets one more on a fresh budget, and only the outcome of the
	// last attempt counts: an item that was released in the end is not
	// reported as held.
	err := bounded(sess.Debugger.ADTDetach)
	if err != nil {
		err = bounded(sess.Debugger.ADTDetach)
	}
	if cerr := bounded(sess.Debugger.Close); cerr != nil {
		held = append(held, fmt.Sprintf("the session (%v)", cerr))
	}
	if err != nil {
		// Close tries the detach once more when it is still owed. Asking
		// again settles what happened: a detach with nothing left to release
		// answers at once without a request, so only a release that is still
		// failing reports as held.
		err = bounded(sess.Debugger.ADTDetach)
	}
	if err != nil {
		held = append(held, fmt.Sprintf("the listener or the debuggee (%v)", err))
	}
	if len(held) > 0 {
		s.output("stderr", "vsp dap: SAP may still hold "+strings.Join(held, "; ")+
			"; see SM50/SM04 for a suspended work process and the user's external breakpoints\n")
	}
}

// fromClientPath turns a client's file:// URI into a local path when the
// client said it names files that way (pathFormat "uri").
func (s *Server) fromClientPath(p string) string {
	s.mu.Lock()
	asURI := s.pathURI
	s.mu.Unlock()
	if !asURI || !strings.HasPrefix(strings.ToLower(p), "file://") {
		return p
	}
	u, err := url.Parse(p)
	if err != nil {
		return p
	}
	path := u.Path
	// file:///C:/x on Windows is the path C:/x.
	if len(path) >= 3 && path[0] == '/' && path[2] == ':' {
		path = path[1:]
	}
	return filepath.FromSlash(path)
}

// toClientPathLocked is fromClientPath's inverse. Callers hold mu.
func (s *Server) toClientPathLocked(p string) string {
	if !s.pathURI || p == "" || strings.HasPrefix(p, vspScheme) {
		return p
	}
	slash := filepath.ToSlash(p)
	if !strings.HasPrefix(slash, "/") {
		slash = "/" + slash
	}
	return (&url.URL{Scheme: "file", Path: slash}).String()
}

func (s *Server) output(category, text string) {
	_ = s.conn.Emit("output", map[string]any{"category": category, "output": text})
}
