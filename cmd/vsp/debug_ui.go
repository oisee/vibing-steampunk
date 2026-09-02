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
	"time"

	"github.com/oisee/vibing-steampunk/pkg/adt"
	"github.com/spf13/cobra"
)

//go:embed debug_ui.html
var debugUIAssets embed.FS

// The debugger has had a working session layer since Phase 1 of #2 — a session
// that survives across tool calls, a stack, variables, stepping. What it never
// had was something to look at, and #2 was closed with two thirds of its design
// unbuilt (now #184).
//
// This is the smallest honest answer to "show me": a local page, served by vsp
// itself, driving the same pkg/adt debugger client the MCP tools drive. No DAP
// layer, no framework, no build step. It is a prototype for the Phase 3
// question — should vsp ship a UI at all, or stop at DAP and let editors be the
// front end — and it exists so that question can be answered by looking rather
// than by imagining.
var debugUICmd = &cobra.Command{
	Use:   "ui",
	Short: "Serve a local debugger UI on localhost",
	Long: `Serve a local web UI for the ABAP debugger.

It binds to localhost only, drives the same debugger client the MCP tools use,
and holds one session. Nothing is written to the SAP system: it listens,
attaches, steps, and reads the stack, variables and source.

  vsp debug ui                 # http://127.0.0.1:7799
  vsp debug ui --port 8080
  vsp debug ui --user DEVELOPER   # listen for that user's processes

Set a breakpoint in ADT or with 'vsp debug', run the ABAP, and the page picks
the session up.`,
	RunE: runDebugUI,
}

var (
	debugUIPort int
	debugUIUser string
)

func init() {
	debugUICmd.Flags().IntVar(&debugUIPort, "port", 7799, "Port to bind on localhost")
	debugUICmd.Flags().StringVar(&debugUIUser, "user", "", "Listen for this user's debuggees (defaults to the configured user)")
	debugCmd.AddCommand(debugUICmd)
}

// debugUIServer holds the one session the page talks to. A debug session is
// single-threaded on the SAP side, so this is deliberately one session and a
// mutex, not a pool.
type debugUIServer struct {
	client   *adt.Client
	attached bool
	debuggee *adt.Debuggee
}

type uiState struct {
	Attached  bool                `json:"attached"`
	Debuggee  *adt.Debuggee       `json:"debuggee,omitempty"`
	Stack     *adt.DebugStackInfo `json:"stack,omitempty"`
	Variables []adt.DebugVariable `json:"variables,omitempty"`
	Note      string              `json:"note,omitempty"`
}

func runDebugUI(cmd *cobra.Command, args []string) error {
	client := createADTClient()
	srv := &debugUIServer{client: client}

	mux := http.NewServeMux()
	mux.HandleFunc("/", srv.handleIndex)
	mux.HandleFunc("/api/state", srv.handleState)
	mux.HandleFunc("/api/listen", srv.handleListen)
	mux.HandleFunc("/api/step", srv.handleStep)
	mux.HandleFunc("/api/goto", srv.handleGoTo)
	mux.HandleFunc("/api/source", srv.handleSource)
	mux.HandleFunc("/api/detach", srv.handleDetach)

	addr := net.JoinHostPort("127.0.0.1", strconv.Itoa(debugUIPort))
	lc := &net.ListenConfig{}
	ln, err := lc.Listen(cmd.Context(), "tcp", addr)
	if err != nil {
		return fmt.Errorf("binding %s: %w", addr, err)
	}

	fmt.Fprintf(os.Stderr, "debugger UI on http://%s\n", addr)
	fmt.Fprintf(os.Stderr, "set a breakpoint, run the ABAP, then press Listen on the page\n")

	return (&http.Server{Handler: mux, ReadHeaderTimeout: 10 * time.Second}).Serve(ln)
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

func writeErr(w http.ResponseWriter, err error) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusBadGateway)
	_ = json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
}

// collect returns the whole picture in one response. The page is a view of a
// stopped process, so a partial answer is worse than a slow one.
func (s *debugUIServer) collect(ctx context.Context) *uiState {
	st := &uiState{Attached: s.attached, Debuggee: s.debuggee}
	if !s.attached {
		st.Note = "not attached — press Listen, then trigger the breakpoint"
		return st
	}

	stack, err := s.client.DebuggerGetStack(ctx, true)
	if err != nil {
		st.Note = fmt.Sprintf("stack unavailable: %v", err)
		return st
	}
	st.Stack = stack

	vars, err := s.client.DebuggerGetVariables(ctx, nil)
	if err != nil {
		// A missing stack is fatal to the view; missing variables are not.
		st.Note = fmt.Sprintf("variables unavailable: %v", err)
		return st
	}
	st.Variables = vars
	return st
}

func (s *debugUIServer) handleState(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, s.collect(r.Context()))
}

func (s *debugUIServer) handleListen(w http.ResponseWriter, r *http.Request) {
	user := debugUIUser
	if user == "" {
		user = cfg.Username
	}

	res, err := s.client.DebuggerListen(r.Context(), &adt.ListenOptions{
		DebuggingMode:  adt.DebuggingModeUser,
		User:           user,
		TimeoutSeconds: 60,
	})
	if err != nil {
		writeErr(w, err)
		return
	}
	if res.TimedOut || res.Debuggee == nil {
		writeJSON(w, &uiState{Note: "no debuggee within 60s — is the breakpoint set, and did the ABAP run?"})
		return
	}

	if _, err := s.client.DebuggerAttach(r.Context(), res.Debuggee.ID, user); err != nil {
		writeErr(w, err)
		return
	}
	s.attached = true
	s.debuggee = res.Debuggee
	writeJSON(w, s.collect(r.Context()))
}

func (s *debugUIServer) handleStep(w http.ResponseWriter, r *http.Request) {
	if !s.attached {
		writeJSON(w, s.collect(r.Context()))
		return
	}
	stepType := adt.DebugStepType(r.URL.Query().Get("type"))
	switch stepType {
	case adt.DebugStepInto, adt.DebugStepOver, adt.DebugStepReturn, adt.DebugStepContinue:
	default:
		http.Error(w, "unsupported step type", http.StatusBadRequest)
		return
	}

	if _, err := s.client.DebuggerStep(r.Context(), stepType, ""); err != nil {
		// Continue ends the session when nothing else is hit, and that is a
		// normal outcome rather than a failure.
		s.attached = false
		writeJSON(w, &uiState{Note: fmt.Sprintf("session ended after %s: %v", stepType, err)})
		return
	}
	writeJSON(w, s.collect(r.Context()))
}

func (s *debugUIServer) handleGoTo(w http.ResponseWriter, r *http.Request) {
	uri := r.URL.Query().Get("uri")
	if uri == "" || !s.attached {
		writeJSON(w, s.collect(r.Context()))
		return
	}
	if err := s.client.DebuggerGoToStack(r.Context(), uri); err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, s.collect(r.Context()))
}

// handleSource fetches the source of the frame being shown. An include is
// fetched as an include; anything else is read as a program, which is what the
// debugger's own stack entries name.
func (s *debugUIServer) handleSource(w http.ResponseWriter, r *http.Request) {
	program := r.URL.Query().Get("program")
	include := r.URL.Query().Get("include")

	name, src, err := program, "", error(nil)
	if include != "" && include != program {
		name = include
		src, err = s.client.GetInclude(r.Context(), include)
	} else if program != "" {
		src, err = s.client.GetProgram(r.Context(), program)
	} else {
		writeJSON(w, map[string]string{"source": "", "name": ""})
		return
	}
	if err != nil {
		writeJSON(w, map[string]string{"source": "", "name": name, "error": err.Error()})
		return
	}
	writeJSON(w, map[string]string{"source": src, "name": name})
}

func (s *debugUIServer) handleDetach(w http.ResponseWriter, r *http.Request) {
	if s.attached {
		_ = s.client.DebuggerDetach(r.Context())
	}
	s.attached = false
	s.debuggee = nil
	writeJSON(w, s.collect(r.Context()))
}
