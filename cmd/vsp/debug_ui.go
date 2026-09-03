package main

import (
	"context"
	"embed"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/spf13/cobra"

	"github.com/oisee/open-rfc-go/rfc"

	"github.com/oisee/vibing-steampunk/pkg/adt"
	"github.com/oisee/vibing-steampunk/pkg/saprfc"
)

//go:embed debug_ui.html
var debugUIAssets embed.FS

// A local face for the debugger.
//
// It rides on the same pinned RFC conversation `vsp rfc debug` uses, and that
// is not an implementation detail: a breakpoint registered by one process does
// not survive it, and a listener registered through the Z facade never receives
// a debuggee whose breakpoint was registered through ADT. Both were learned the
// hard way. One session sets the breakpoint, listens, attaches and steps, which
// is the only arrangement that actually stops.
//
// Phase 1 of #2 built the session layer; this is the "show me" for #184, where
// the open question is whether vsp should ship a UI at all or stop at a DAP
// shim and let editors be the front end.
var debugUICmd = &cobra.Command{
	Use:   "ui [OBJECT]",
	Short: "Serve a local debugger UI on localhost",
	Args:  cobra.MaximumNArgs(1),
	Long: `Serve a local web UI for the ABAP debugger.

Name an object and the page opens ready to debug it — a report, or a function
module, which is resolved to its function group's include for you.

  vsp -s a4h debug ui                     # http://127.0.0.1:7799
  vsp -s a4h debug ui RFC_SYSTEM_INFO     # opens on that function module
  vsp -s a4h debug ui ZVSP_DEBUG_PLAY --http-port 8080

Set the line, press Set breakpoint, then Run — for a function module the page
calls it over a second RFC connection, so the breakpoint fires in a session it
started itself. Standard SAP code needs "system code" switched on; SAP accepts
a breakpoint there and silently never stops without it.`,
	RunE: runDebugUI,
}

var (
	debugUIPort    int
	debugUIUser    string
	debugUITimeout int
)

func init() {
	// Not "port": rfcDestinationFor reads a --port flag as the RFC gateway port
	// (rfc.go:468, declared on rfcCmd as a persistent flag). Naming the HTTP
	// port that way made this command dial the SAP gateway on 7799 and hang
	// before it ever printed a line.
	debugUICmd.Flags().IntVar(&debugUIPort, "http-port", 7799, "Port to bind the UI on localhost")
	debugUICmd.Flags().StringVar(&debugUIUser, "user", "", "Whose debuggees to listen for (default: the logon user)")
	debugUICmd.Flags().IntVar(&debugUITimeout, "timeout", 600, "Seconds a single RFC call may take; must exceed the listen timeout")
	debugCmd.AddCommand(debugUICmd)
}

// debugUIServer owns the one pinned debug session. A pinned RFC conversation is
// single-threaded, so every handler takes the same lock — the page can fire
// concurrent fetches and the session must not see two calls at once.
type debugUIServer struct {
	mu     sync.Mutex
	dbg    *saprfc.Debugger
	dest   saprfc.Params
	user   string
	target string
	line   int
	sysDbg bool

	attached bool
	debuggee *saprfc.ADTDebuggee
}

type uiState struct {
	Target    string              `json:"target"`
	Line      int                 `json:"line"`
	SystemDbg bool                `json:"systemDebugging"`
	Attached  bool                `json:"attached"`
	Where     string              `json:"where,omitempty"`
	Stack     *adt.DebugStackInfo `json:"stack,omitempty"`
	Variables []adt.DebugVariable `json:"variables,omitempty"`
	Note      string              `json:"note,omitempty"`
}

func runDebugUI(cmd *cobra.Command, args []string) error {
	target := ""
	if len(args) == 1 {
		target = strings.ToUpper(strings.TrimSpace(args[0]))
	}

	return withRFCDestTimeout(cmd, time.Duration(debugUITimeout)*time.Second, func(ctx context.Context, c *rfc.Client, dest saprfc.Params) error {
		user := debugUIUser
		if user == "" {
			user = dest.User
		}
		dbg, err := saprfc.NewDebugger(ctx, c, user)
		if err != nil {
			return err
		}
		defer func() { _ = dbg.Close(ctx) }()

		srv := &debugUIServer{dbg: dbg, dest: dest, user: user, target: target, line: 1}

		mux := http.NewServeMux()
		mux.HandleFunc("/", srv.handleIndex)
		mux.HandleFunc("/api/state", srv.handleState)
		mux.HandleFunc("/api/bp", srv.handleBreakpoint)
		mux.HandleFunc("/api/sys", srv.handleSystemDebugging)
		mux.HandleFunc("/api/listen", srv.handleListen)
		mux.HandleFunc("/api/run", srv.handleRun)
		mux.HandleFunc("/api/step", srv.handleStep)
		mux.HandleFunc("/api/source", srv.handleSource)
		mux.HandleFunc("/api/detach", srv.handleDetach)

		addr := net.JoinHostPort("127.0.0.1", strconv.Itoa(debugUIPort))
		lc := &net.ListenConfig{}
		ln, err := lc.Listen(ctx, "tcp", addr)
		if err != nil {
			return fmt.Errorf("binding %s: %w", addr, err)
		}
		fmt.Fprintf(os.Stderr, "debugger UI on http://%s (user %s)\n", addr, user)
		if target != "" {
			fmt.Fprintf(os.Stderr, "target: %s\n", target)
		}
		return (&http.Server{Handler: mux, ReadHeaderTimeout: 10 * time.Second}).Serve(ln)
	})
}

func (s *debugUIServer) handleIndex(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}
	page, err := debugUIAssets.ReadFile("debug_ui.html")
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = w.Write(page)
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(v)
}

// snapshot builds the whole picture. Callers hold the lock.
func (s *debugUIServer) snapshot(ctx context.Context, note string) *uiState {
	st := &uiState{Target: s.target, Line: s.line, SystemDbg: s.sysDbg, Attached: s.attached, Note: note}
	if s.debuggee != nil {
		st.Where = fmt.Sprintf("%s/%s:%d", s.debuggee.Program, s.debuggee.Include, s.debuggee.Line)
	}
	if !s.attached {
		if st.Note == "" {
			st.Note = "not attached — set a breakpoint, press Run, then Listen"
		}
		return st
	}

	if res, err := s.dbg.ADTStack(ctx); err == nil {
		if info, perr := adt.ParseStackXML(res.Body); perr == nil {
			st.Stack = info
		}
	}
	if vars, err := s.dbg.Locals(ctx); err == nil {
		st.Variables = vars
	}
	return st
}

func (s *debugUIServer) handleState(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	defer s.mu.Unlock()
	writeJSON(w, s.snapshot(r.Context(), ""))
}

func (s *debugUIServer) handleSystemDebugging(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.sysDbg = r.URL.Query().Get("on") == "true"
	s.dbg.SystemDebugging(s.sysDbg)
	note := "breakpoints in SAP standard code: off"
	if s.sysDbg {
		note = "breakpoints in SAP standard code: on"
	}
	writeJSON(w, s.snapshot(r.Context(), note))
}

func (s *debugUIServer) handleBreakpoint(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if obj := strings.ToUpper(strings.TrimSpace(r.URL.Query().Get("object"))); obj != "" {
		s.target = obj
	}
	if n, err := strconv.Atoi(r.URL.Query().Get("line")); err == nil && n > 0 {
		s.line = n
	}
	if s.target == "" {
		writeJSON(w, s.snapshot(r.Context(), "name an object first"))
		return
	}

	bps, err := s.dbg.ADTAddBreakpoint(r.Context(), s.target, s.line, "")
	if err != nil {
		writeJSON(w, s.snapshot(r.Context(), fmt.Sprintf("breakpoint refused: %v", err)))
		return
	}
	// SAP accepts the request and reports per-breakpoint refusals separately;
	// a caller that only checks err believes it set something it did not.
	for _, rej := range s.dbg.Rejected() {
		writeJSON(w, s.snapshot(r.Context(), fmt.Sprintf("not placed at %s:%d — %s", s.target, s.line, rej.ErrorMessage)))
		return
	}
	where := fmt.Sprintf("%s:%d", s.target, s.line)
	if len(bps) > 0 && bps[0].URI != "" {
		where = bps[0].URI
	}
	writeJSON(w, s.snapshot(r.Context(), "breakpoint set at "+where))
}

func (s *debugUIServer) handleListen(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	defer s.mu.Unlock()

	seconds := 60
	if n, err := strconv.Atoi(r.URL.Query().Get("seconds")); err == nil && n > 0 {
		seconds = n
	}
	// Listen stays a button of its own because the trigger is not always ours:
	// a colleague in SE38, a scheduled job, an agent on another machine.
	s.catchLocked(w, r, seconds, "")
}

// catchLocked waits for a debuggee and attaches. Callers hold the lock.
func (s *debugUIServer) catchLocked(w http.ResponseWriter, r *http.Request, seconds int, prefix string) {
	who, _, err := s.dbg.ADTCatch(r.Context(), s.user, "vsp", adtTerminalID, seconds)
	if err != nil && who == nil {
		writeJSON(w, s.snapshot(r.Context(), fmt.Sprintf("%slisten failed: %v", prefix, err)))
		return
	}
	if who == nil {
		writeJSON(w, s.snapshot(r.Context(),
			fmt.Sprintf("%snobody stopped within %ds — is the line executable, and is system code switched on if it is SAP's own?", prefix, seconds)))
		return
	}
	s.attached = true
	s.debuggee = who
	note := prefix + "stopped"
	if err != nil {
		note = fmt.Sprintf("%sattached, but the stack could not be read: %v", prefix, err)
	}
	writeJSON(w, s.snapshot(r.Context(), note))
}

// handleRun is the one button: make sure a breakpoint exists, call the target
// on a SECOND RFC connection, then listen — all in one request.
//
// It has to be a second connection: this process's own conversation is pinned
// to the debug session, and calling through it would deadlock against the
// breakpoint it is about to wait on.
//
// The order matters and is not obvious. Listening first and calling second
// cannot work, because the listen blocks. Calling first and listening second
// looks like a race and is not: the debuggee parks at the breakpoint and waits
// to be collected, so a listener arriving a moment later still finds it.
func (s *debugUIServer) handleRun(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if obj := strings.ToUpper(strings.TrimSpace(r.URL.Query().Get("object"))); obj != "" {
		s.target = obj
	}
	if n, err := strconv.Atoi(r.URL.Query().Get("line")); err == nil && n > 0 {
		s.line = n
	}
	if s.target == "" {
		writeJSON(w, s.snapshot(r.Context(), "name an object first"))
		return
	}

	// Nothing stops without a breakpoint, and making someone press two buttons
	// in a fixed order to learn that is a poor way to teach it. If this session
	// has registered none, set one where the fields point.
	if existing, err := s.dbg.ADTBreakpoints(r.Context()); err == nil && len(existing) == 0 {
		if _, err := s.dbg.ADTAddBreakpoint(r.Context(), s.target, s.line, ""); err != nil {
			writeJSON(w, s.snapshot(r.Context(), fmt.Sprintf("breakpoint refused: %v", err)))
			return
		}
		for _, rej := range s.dbg.Rejected() {
			writeJSON(w, s.snapshot(r.Context(), fmt.Sprintf("not placed at %s:%d — %s", s.target, s.line, rej.ErrorMessage)))
			return
		}
	}

	target, dest := s.target, s.dest
	go func() {
		ctx := context.WithoutCancel(context.Background())
		c, err := saprfc.OpenWithTimeout(ctx, dest, 120*time.Second)
		if err != nil {
			return
		}
		defer c.Close(ctx)
		// The result is deliberately discarded: this call exists to make the
		// breakpoint fire, and while it is stopped it will not return anyway.
		_, _ = c.Call(ctx, target, rfc.Params{})
	}()

	seconds := 60
	if n, err := strconv.Atoi(r.URL.Query().Get("seconds")); err == nil && n > 0 {
		seconds = n
	}
	s.catchLocked(w, r, seconds, "called "+target+" — ")
}

func (s *debugUIServer) handleStep(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if !s.attached {
		writeJSON(w, s.snapshot(r.Context(), "not attached"))
		return
	}
	kind := map[string]string{
		"into": "stepInto", "over": "stepOver",
		"out": "stepReturn", "continue": "stepContinue",
	}[strings.ToLower(r.URL.Query().Get("type"))]
	if kind == "" {
		http.Error(w, "step kinds: into, over, out, continue", http.StatusBadRequest)
		return
	}

	if _, err := s.dbg.ADTStep(r.Context(), kind); err != nil {
		s.attached = false
		s.debuggee = nil
		writeJSON(w, s.snapshot(r.Context(), fmt.Sprintf("session ended after %s: %v", kind, err)))
		return
	}
	writeJSON(w, s.snapshot(r.Context(), ""))
}

// handleSource reads a frame's source through the same pinned session, so no
// second HTTP client and no second logon are needed.
func (s *debugUIServer) handleSource(w http.ResponseWriter, r *http.Request) {
	object := strings.TrimSpace(r.URL.Query().Get("object"))
	if object == "" {
		writeJSON(w, map[string]string{"source": "", "name": ""})
		return
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	uri, err := s.dbg.ResolveSourceURI(r.Context(), object)
	if err != nil {
		writeJSON(w, map[string]string{"source": "", "name": object, "error": err.Error()})
		return
	}
	res, err := s.dbg.ADT(r.Context(), "GET", uri, []saprfc.ADTHeader{{Name: "Accept", Value: "text/plain"}}, nil)
	if err != nil {
		writeJSON(w, map[string]string{"source": "", "name": object, "error": err.Error()})
		return
	}
	writeJSON(w, map[string]string{"source": string(res.Body), "name": object})
}

func (s *debugUIServer) handleDetach(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.attached {
		_ = s.dbg.ADTDetach(r.Context())
	}
	s.attached = false
	s.debuggee = nil
	writeJSON(w, s.snapshot(r.Context(), "detached"))
}
