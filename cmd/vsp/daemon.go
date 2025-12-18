// Debug daemon - HTTP server for persistent ABAP debug sessions
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/oisee/vibing-steampunk/pkg/adt"
	"github.com/spf13/cobra"
)

// DaemonConfig holds debug daemon configuration
type DaemonConfig struct {
	Port    int
	Host    string
	Verbose bool
}

var daemonCfg = &DaemonConfig{}

// DebugSession represents an active debug session
type DebugSession struct {
	ID            string                 `json:"id"`
	Status        string                 `json:"status"` // "waiting", "attached", "stepping", "stopped"
	User          string                 `json:"user"`
	StartTime     time.Time              `json:"startTime"`
	DebuggeeID    string                 `json:"debuggeeId,omitempty"`
	CurrentURI    string                 `json:"currentUri,omitempty"`
	CurrentLine   int                    `json:"currentLine,omitempty"`
	Breakpoints   []BreakpointInfo       `json:"breakpoints,omitempty"`
	Stack         []adt.DebugStackEntry  `json:"stack,omitempty"`
	Variables     []adt.DebugVariable    `json:"variables,omitempty"`
	Error         string                 `json:"error,omitempty"`
	ListenerDone  chan struct{}          `json:"-"`
	mu            sync.RWMutex           `json:"-"`
}

// BreakpointInfo represents a breakpoint
type BreakpointInfo struct {
	ID        string `json:"id"`
	Kind      string `json:"kind"` // "line", "exception", "statement"
	URI       string `json:"uri,omitempty"`
	Line      int    `json:"line,omitempty"`
	Exception string `json:"exception,omitempty"`
	Statement string `json:"statement,omitempty"`
	Condition string `json:"condition,omitempty"`
}

// DebugDaemon manages debug sessions
type DebugDaemon struct {
	client   *adt.Client
	session  *DebugSession
	mu       sync.RWMutex
	verbose  bool
}

// API request/response types
type StartSessionRequest struct {
	User    string `json:"user,omitempty"`
	Timeout int    `json:"timeout,omitempty"` // seconds, default 60
}

type SetBreakpointRequest struct {
	Kind      string `json:"kind"` // "line", "exception", "statement"
	URI       string `json:"uri,omitempty"`
	Line      int    `json:"line,omitempty"`
	Exception string `json:"exception,omitempty"`
	Statement string `json:"statement,omitempty"`
	Condition string `json:"condition,omitempty"`
}

type StepRequest struct {
	Type string `json:"type"` // "stepInto", "stepOver", "stepReturn", "stepContinue"
	URI  string `json:"uri,omitempty"`
}

type VariablesRequest struct {
	IDs []string `json:"ids,omitempty"` // Variable IDs, default ["@ROOT"]
}

type APIResponse struct {
	Success bool        `json:"success"`
	Data    interface{} `json:"data,omitempty"`
	Error   string      `json:"error,omitempty"`
}

var daemonCmd = &cobra.Command{
	Use:   "debug-daemon",
	Short: "Start debug daemon HTTP server",
	Long: `Start a standalone HTTP server that maintains persistent ABAP debug sessions.

The daemon exposes a REST API on localhost for debug operations.
Sessions survive across multiple client requests.

Examples:
  # Start daemon on default port 9999
  vsp debug-daemon

  # Start on custom port
  vsp debug-daemon --port 8080

  # With verbose logging
  vsp debug-daemon --verbose

API Endpoints:
  POST   /session           - Start debug listener
  GET    /session           - Get session status
  DELETE /session           - Stop session and detach

  POST   /breakpoint        - Set breakpoint
  GET    /breakpoints       - List breakpoints
  DELETE /breakpoint/{id}   - Delete breakpoint

  POST   /step              - Step (stepInto, stepOver, stepReturn, stepContinue)
  GET    /stack             - Get call stack
  GET    /variables         - Get variables
  POST   /variables         - Get specific variables`,
	RunE: runDaemon,
}

func init() {
	daemonCmd.Flags().IntVarP(&daemonCfg.Port, "port", "P", 9999, "HTTP server port")
	daemonCmd.Flags().StringVar(&daemonCfg.Host, "host", "localhost", "HTTP server host (use 0.0.0.0 for external access)")
	daemonCmd.Flags().BoolVarP(&daemonCfg.Verbose, "verbose", "v", false, "Enable verbose logging")

	rootCmd.AddCommand(daemonCmd)
}

func runDaemon(cmd *cobra.Command, args []string) error {
	// Load config same as main server
	resolveConfig(cmd)

	if err := validateConfig(); err != nil {
		return err
	}

	if err := processCookieAuth(cmd); err != nil {
		return err
	}

	// Create ADT client with options
	// Use longer timeout for debug operations (5 minutes)
	var opts []adt.Option
	opts = append(opts, adt.WithClient(cfg.Client))
	opts = append(opts, adt.WithLanguage(cfg.Language))
	opts = append(opts, adt.WithTimeout(5*time.Minute)) // Extended timeout for debugging
	if cfg.InsecureSkipVerify {
		opts = append(opts, adt.WithInsecureSkipVerify())
	}
	if len(cfg.Cookies) > 0 {
		opts = append(opts, adt.WithCookies(cfg.Cookies))
	}

	client := adt.NewClient(cfg.BaseURL, cfg.Username, cfg.Password, opts...)

	daemon := &DebugDaemon{
		client:  client,
		verbose: daemonCfg.Verbose || cfg.Verbose,
	}

	// Set up HTTP routes
	mux := http.NewServeMux()

	// Session endpoints
	mux.HandleFunc("/session", daemon.handleSession)

	// Breakpoint endpoints
	mux.HandleFunc("/breakpoint", daemon.handleBreakpoint)
	mux.HandleFunc("/breakpoints", daemon.handleBreakpoints)

	// Debug control endpoints
	mux.HandleFunc("/step", daemon.handleStep)
	mux.HandleFunc("/stack", daemon.handleStack)
	mux.HandleFunc("/variables", daemon.handleVariables)

	// Health check
	mux.HandleFunc("/health", daemon.handleHealth)

	addr := fmt.Sprintf("%s:%d", daemonCfg.Host, daemonCfg.Port)
	server := &http.Server{
		Addr:    addr,
		Handler: corsMiddleware(loggingMiddleware(mux, daemon.verbose)),
	}

	// Graceful shutdown
	done := make(chan struct{})
	go func() {
		sigCh := make(chan os.Signal, 1)
		signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
		<-sigCh

		fmt.Fprintf(os.Stderr, "\n[DAEMON] Shutting down...\n")
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		server.Shutdown(ctx)
		close(done)
	}()

	fmt.Fprintf(os.Stderr, "[DAEMON] Starting debug daemon on http://%s\n", addr)
	fmt.Fprintf(os.Stderr, "[DAEMON] SAP System: %s\n", cfg.BaseURL)
	fmt.Fprintf(os.Stderr, "[DAEMON] Press Ctrl+C to stop\n")
	fmt.Fprintf(os.Stderr, "\n")
	fmt.Fprintf(os.Stderr, "Available endpoints:\n")
	fmt.Fprintf(os.Stderr, "  POST   /session     - Start debug listener\n")
	fmt.Fprintf(os.Stderr, "  GET    /session     - Get session status\n")
	fmt.Fprintf(os.Stderr, "  DELETE /session     - Stop session\n")
	fmt.Fprintf(os.Stderr, "  POST   /breakpoint  - Set breakpoint\n")
	fmt.Fprintf(os.Stderr, "  GET    /breakpoints - List breakpoints\n")
	fmt.Fprintf(os.Stderr, "  POST   /step        - Step execution\n")
	fmt.Fprintf(os.Stderr, "  GET    /stack       - Get call stack\n")
	fmt.Fprintf(os.Stderr, "  GET    /variables   - Get variables\n")
	fmt.Fprintf(os.Stderr, "  GET    /health      - Health check\n")
	fmt.Fprintf(os.Stderr, "\n")

	if err := server.ListenAndServe(); err != http.ErrServerClosed {
		return fmt.Errorf("server error: %w", err)
	}

	<-done
	return nil
}

// Middleware for CORS (allows browser testing)
func corsMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, DELETE, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type")

		if r.Method == "OPTIONS" {
			w.WriteHeader(http.StatusOK)
			return
		}

		next.ServeHTTP(w, r)
	})
}

// Logging middleware
func loggingMiddleware(next http.Handler, verbose bool) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		next.ServeHTTP(w, r)
		if verbose {
			fmt.Fprintf(os.Stderr, "[%s] %s %s %v\n",
				time.Now().Format("15:04:05"), r.Method, r.URL.Path, time.Since(start))
		}
	})
}

// writeJSON writes a JSON response
func writeJSON(w http.ResponseWriter, status int, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(data)
}

// writeError writes an error response
func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, APIResponse{Success: false, Error: msg})
}

// writeSuccess writes a success response
func writeSuccess(w http.ResponseWriter, data interface{}) {
	writeJSON(w, http.StatusOK, APIResponse{Success: true, Data: data})
}

// Health check handler
func (d *DebugDaemon) handleHealth(w http.ResponseWriter, r *http.Request) {
	writeSuccess(w, map[string]interface{}{
		"status": "ok",
		"time":   time.Now().Format(time.RFC3339),
	})
}

// Session handlers
func (d *DebugDaemon) handleSession(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case "GET":
		d.getSession(w, r)
	case "POST":
		d.startSession(w, r)
	case "DELETE":
		d.stopSession(w, r)
	default:
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}

func (d *DebugDaemon) getSession(w http.ResponseWriter, r *http.Request) {
	d.mu.RLock()
	session := d.session
	d.mu.RUnlock()

	if session == nil {
		writeSuccess(w, map[string]interface{}{
			"status": "no_session",
		})
		return
	}

	session.mu.RLock()
	defer session.mu.RUnlock()

	writeSuccess(w, session)
}

func (d *DebugDaemon) startSession(w http.ResponseWriter, r *http.Request) {
	var req StartSessionRequest
	if r.Body != nil {
		json.NewDecoder(r.Body).Decode(&req)
	}

	// Defaults
	if req.Timeout == 0 {
		req.Timeout = 60
	}
	if req.User == "" {
		req.User = cfg.Username
	}

	d.mu.Lock()
	if d.session != nil && d.session.Status != "stopped" {
		d.mu.Unlock()
		writeError(w, http.StatusConflict, "session already active")
		return
	}

	session := &DebugSession{
		ID:           fmt.Sprintf("dbg-%d", time.Now().UnixNano()),
		Status:       "waiting",
		User:         req.User,
		StartTime:    time.Now(),
		ListenerDone: make(chan struct{}),
	}
	d.session = session
	d.mu.Unlock()

	// Start listener in background
	go d.runListener(session, req.Timeout)

	writeSuccess(w, map[string]interface{}{
		"id":      session.ID,
		"status":  session.Status,
		"user":    session.User,
		"timeout": req.Timeout,
		"message": fmt.Sprintf("Debug listener started for user %s (timeout: %ds)", req.User, req.Timeout),
	})
}

func (d *DebugDaemon) runListener(session *DebugSession, timeout int) {
	defer close(session.ListenerDone)

	ctx := context.Background()

	if d.verbose {
		fmt.Fprintf(os.Stderr, "[DAEMON] Starting debug listener for user %s (timeout: %ds)\n", session.User, timeout)
	}

	listenOpts := &adt.ListenOptions{
		User:           session.User,
		TimeoutSeconds: timeout,
	}
	result, err := d.client.DebuggerListen(ctx, listenOpts)

	session.mu.Lock()
	defer session.mu.Unlock()

	if err != nil {
		session.Status = "error"
		session.Error = err.Error()
		if d.verbose {
			fmt.Fprintf(os.Stderr, "[DAEMON] Listener error: %v\n", err)
		}
		return
	}

	if result == nil || result.TimedOut || result.Debuggee == nil {
		session.Status = "timeout"
		if d.verbose {
			fmt.Fprintf(os.Stderr, "[DAEMON] Listener timeout - no debuggee caught\n")
		}
		return
	}

	// Debuggee caught!
	session.DebuggeeID = result.Debuggee.ID
	session.Status = "caught"

	if d.verbose {
		fmt.Fprintf(os.Stderr, "[DAEMON] Debuggee caught! ID: %s\n", result.Debuggee.ID)
	}

	// Auto-attach
	_, err = d.client.DebuggerAttach(ctx, result.Debuggee.ID, session.User)
	if err != nil {
		session.Status = "attach_failed"
		session.Error = err.Error()
		if d.verbose {
			fmt.Fprintf(os.Stderr, "[DAEMON] Attach failed: %v\n", err)
		}
		return
	}

	session.Status = "attached"

	// Get initial stack
	stackInfo, err := d.client.DebuggerGetStack(ctx, false)
	if err == nil && stackInfo != nil && len(stackInfo.Stack) > 0 {
		session.Stack = stackInfo.Stack
		session.CurrentURI = stackInfo.Stack[0].URI
		session.CurrentLine = stackInfo.Stack[0].Line
	}

	if d.verbose {
		fmt.Fprintf(os.Stderr, "[DAEMON] Attached to debuggee\n")
	}
}

func (d *DebugDaemon) stopSession(w http.ResponseWriter, r *http.Request) {
	d.mu.Lock()
	session := d.session
	d.mu.Unlock()

	if session == nil {
		writeError(w, http.StatusNotFound, "no active session")
		return
	}

	ctx := context.Background()

	// Try to detach
	if session.DebuggeeID != "" {
		d.client.DebuggerDetach(ctx)
	}

	session.mu.Lock()
	session.Status = "stopped"
	session.mu.Unlock()

	writeSuccess(w, map[string]interface{}{
		"message": "session stopped",
	})
}

// Breakpoint handlers
func (d *DebugDaemon) handleBreakpoint(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case "POST":
		d.setBreakpoint(w, r)
	case "DELETE":
		d.deleteBreakpoint(w, r)
	default:
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}

func (d *DebugDaemon) handleBreakpoints(w http.ResponseWriter, r *http.Request) {
	if r.Method != "GET" {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	d.listBreakpoints(w, r)
}

func (d *DebugDaemon) setBreakpoint(w http.ResponseWriter, r *http.Request) {
	var req SetBreakpointRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON")
		return
	}

	if req.Kind == "" {
		writeError(w, http.StatusBadRequest, "kind is required (line, exception, statement)")
		return
	}

	ctx := context.Background()
	user := cfg.Username

	// Build the breakpoint
	bp := adt.Breakpoint{
		Enabled:   true,
		Condition: req.Condition,
	}

	switch req.Kind {
	case "line":
		if req.URI == "" || req.Line == 0 {
			writeError(w, http.StatusBadRequest, "line breakpoint requires uri and line")
			return
		}
		bp.Kind = adt.BreakpointKindLine
		bp.URI = req.URI
		bp.Line = req.Line
	case "exception":
		if req.Exception == "" {
			writeError(w, http.StatusBadRequest, "exception breakpoint requires exception class")
			return
		}
		bp.Kind = adt.BreakpointKindException
		bp.Exception = req.Exception
	case "statement":
		if req.Statement == "" {
			writeError(w, http.StatusBadRequest, "statement breakpoint requires statement type")
			return
		}
		bp.Kind = adt.BreakpointKindStatement
		bp.Statement = req.Statement
	default:
		writeError(w, http.StatusBadRequest, "invalid kind (must be line, exception, or statement)")
		return
	}

	bpReq := &adt.BreakpointRequest{
		User:        user,
		Breakpoints: []adt.Breakpoint{bp},
	}

	result, err := d.client.SetExternalBreakpoint(ctx, bpReq)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	// Track breakpoint in session - use first returned breakpoint
	var bpInfo BreakpointInfo
	if result != nil && len(result.Breakpoints) > 0 {
		rbp := result.Breakpoints[0]
		bpInfo = BreakpointInfo{
			ID:        rbp.ID,
			Kind:      req.Kind,
			URI:       rbp.URI,
			Line:      rbp.Line,
			Exception: rbp.Exception,
			Statement: rbp.Statement,
			Condition: rbp.Condition,
		}
	} else {
		// Fallback to request data
		bpInfo = BreakpointInfo{
			Kind:      req.Kind,
			URI:       req.URI,
			Line:      req.Line,
			Exception: req.Exception,
			Statement: req.Statement,
			Condition: req.Condition,
		}
	}

	d.mu.Lock()
	if d.session != nil {
		d.session.mu.Lock()
		d.session.Breakpoints = append(d.session.Breakpoints, bpInfo)
		d.session.mu.Unlock()
	}
	d.mu.Unlock()

	writeSuccess(w, bpInfo)
}

func (d *DebugDaemon) listBreakpoints(w http.ResponseWriter, r *http.Request) {
	ctx := context.Background()
	user := cfg.Username

	bps, err := d.client.GetExternalBreakpoints(ctx, user)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	writeSuccess(w, bps)
}

func (d *DebugDaemon) deleteBreakpoint(w http.ResponseWriter, r *http.Request) {
	id := r.URL.Query().Get("id")
	if id == "" {
		writeError(w, http.StatusBadRequest, "breakpoint id required")
		return
	}

	ctx := context.Background()
	user := cfg.Username

	err := d.client.DeleteExternalBreakpoint(ctx, id, user)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	writeSuccess(w, map[string]interface{}{
		"deleted": id,
	})
}

// Debug control handlers
func (d *DebugDaemon) handleStep(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	d.mu.RLock()
	session := d.session
	d.mu.RUnlock()

	if session == nil || session.Status != "attached" {
		writeError(w, http.StatusBadRequest, "no attached debug session")
		return
	}

	var req StepRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON")
		return
	}

	if req.Type == "" {
		req.Type = "stepOver"
	}

	// Map string to DebugStepType
	var stepType adt.DebugStepType
	switch req.Type {
	case "stepInto":
		stepType = adt.DebugStepInto
	case "stepOver":
		stepType = adt.DebugStepOver
	case "stepReturn":
		stepType = adt.DebugStepReturn
	case "stepContinue":
		stepType = adt.DebugStepContinue
	case "stepRunToLine":
		stepType = adt.DebugStepRunToLine
	case "stepJumpToLine":
		stepType = adt.DebugStepJumpToLine
	case "terminate":
		stepType = adt.DebugTerminate
	default:
		writeError(w, http.StatusBadRequest, "invalid step type")
		return
	}

	ctx := context.Background()
	result, err := d.client.DebuggerStep(ctx, stepType, req.URI)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	// Update session with fresh stack
	stackInfo, _ := d.client.DebuggerGetStack(ctx, false)
	session.mu.Lock()
	if stackInfo != nil && len(stackInfo.Stack) > 0 {
		session.Stack = stackInfo.Stack
		session.CurrentURI = stackInfo.Stack[0].URI
		session.CurrentLine = stackInfo.Stack[0].Line
	}
	session.mu.Unlock()

	writeSuccess(w, result)
}

func (d *DebugDaemon) handleStack(w http.ResponseWriter, r *http.Request) {
	if r.Method != "GET" {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	d.mu.RLock()
	session := d.session
	d.mu.RUnlock()

	if session == nil || session.Status != "attached" {
		writeError(w, http.StatusBadRequest, "no attached debug session")
		return
	}

	ctx := context.Background()
	stackInfo, err := d.client.DebuggerGetStack(ctx, false)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	// Update session
	session.mu.Lock()
	if stackInfo != nil {
		session.Stack = stackInfo.Stack
		if len(stackInfo.Stack) > 0 {
			session.CurrentURI = stackInfo.Stack[0].URI
			session.CurrentLine = stackInfo.Stack[0].Line
		}
	}
	session.mu.Unlock()

	writeSuccess(w, stackInfo)
}

func (d *DebugDaemon) handleVariables(w http.ResponseWriter, r *http.Request) {
	d.mu.RLock()
	session := d.session
	d.mu.RUnlock()

	if session == nil || session.Status != "attached" {
		writeError(w, http.StatusBadRequest, "no attached debug session")
		return
	}

	var ids []string

	if r.Method == "POST" {
		var req VariablesRequest
		json.NewDecoder(r.Body).Decode(&req)
		ids = req.IDs
	}

	if len(ids) == 0 {
		ids = []string{"@ROOT"}
	}

	ctx := context.Background()
	vars, err := d.client.DebuggerGetVariables(ctx, ids)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	// Update session
	session.mu.Lock()
	session.Variables = vars
	session.mu.Unlock()

	writeSuccess(w, vars)
}
