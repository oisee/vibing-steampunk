package main

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/vinchacho/vibing-steampunk/pkg/adt"
	"github.com/spf13/cobra"
)

var debugCmd = &cobra.Command{
	Use:   "debug",
	Short: "Interactive ABAP debugger",
	Long: `Interactive command-line debugger for ABAP.

Attach to running ABAP processes, set breakpoints, step through code,
and inspect variables - all from the command line.

Commands:
  s, step      Step into next statement
  n, next      Step over (execute current line)
  o, out       Step out of current routine
  c, continue  Continue execution
  r, stack     Show call stack
  v, vars      Show local variables
  b <prog> <line>  Set breakpoint
  d <id>       Delete breakpoint
  l            List breakpoints
  q, quit      Detach and exit
  h, help      Show help

Examples:
  # Attach mode - wait for any debuggee
  vsp debug --attach

  # Attach to specific user's processes
  vsp debug --attach --user DEVELOPER

  # Set breakpoint and attach
  vsp debug --program ZTEST --line 42`,
	RunE: runDebug,
}

var (
	debugAttach  bool
	debugUser    string
	debugProgram string
	debugLine    int
	debugTimeout int
)

func init() {
	debugCmd.Flags().BoolVarP(&debugAttach, "attach", "a", true, "Attach mode - wait for debuggee")
	debugCmd.Flags().StringVarP(&debugUser, "user", "u", "", "User to debug (defaults to current user)")
	debugCmd.Flags().StringVarP(&debugProgram, "program", "p", "", "Program for initial breakpoint")
	debugCmd.Flags().IntVarP(&debugLine, "line", "l", 0, "Line for initial breakpoint")
	debugCmd.Flags().IntVarP(&debugTimeout, "timeout", "t", 120, "Listen timeout in seconds")

	rootCmd.AddCommand(debugCmd)
}

// debugSession holds the state for an interactive debug session.
type debugSession struct {
	client     *adt.Client
	wsClient   *adt.DebugWebSocketClient
	user       string
	attached   bool
	debuggeeID string
	ctx        context.Context
	cancel     context.CancelFunc
}

func runDebug(cmd *cobra.Command, args []string) error {
	// Resolve configuration (same as MCP server)
	resolveConfig(cmd.Parent())

	// Validate we have auth
	if err := validateConfig(); err != nil {
		return err
	}

	// Process cookie auth
	if err := processCookieAuth(cmd.Parent()); err != nil {
		return err
	}

	// Create ADT client
	client := createADTClient()

	// Get user for debugging
	user := debugUser
	if user == "" {
		user = cfg.Username
	}

	// Set terminal ID for this session
	adt.SetTerminalIDUser(user)

	// Create context with cancellation
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Handle Ctrl+C gracefully
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM)
	go func() {
		<-sigCh
		fmt.Println("\nReceived interrupt, cleaning up...")
		cancel()
	}()

	// Create WebSocket client for breakpoints
	wsClient := adt.NewDebugWebSocketClient(
		cfg.BaseURL,
		cfg.Client,
		cfg.Username,
		cfg.Password,
		cfg.InsecureSkipVerify,
	)

	// Try to connect WebSocket (optional - falls back to HTTP if unavailable)
	wsConnected := false
	if err := wsClient.Connect(ctx); err != nil {
		fmt.Fprintf(os.Stderr, "Note: WebSocket (ZADT_VSP) unavailable: %v\n", err)
		fmt.Fprintf(os.Stderr, "Using HTTP-only mode (breakpoints may have limited functionality)\n\n")
	} else {
		wsConnected = true
		defer wsClient.Close()
	}

	// Create session
	session := &debugSession{
		client:   client,
		wsClient: wsClient,
		user:     user,
		ctx:      ctx,
		cancel:   cancel,
	}

	// Set initial breakpoint if specified
	if debugProgram != "" && debugLine > 0 {
		if wsConnected {
			bpID, err := wsClient.SetLineBreakpoint(ctx, debugProgram, debugLine)
			if err != nil {
				return fmt.Errorf("failed to set breakpoint: %w", err)
			}
			fmt.Printf("Breakpoint set: %s at %s:%d\n", bpID, debugProgram, debugLine)
		} else {
			fmt.Println("Warning: Cannot set breakpoint without WebSocket connection")
		}
	}

	// Print banner
	printDebugBanner(user, cfg.BaseURL, wsConnected)

	// Enter REPL
	return session.repl()
}

func printDebugBanner(user, url string, wsConnected bool) {
	fmt.Println()
	fmt.Println("=== VSP ABAP Debugger ===")
	fmt.Printf("User: %s | System: %s\n", user, url)
	if wsConnected {
		fmt.Println("WebSocket: connected (ZADT_VSP)")
	} else {
		fmt.Println("WebSocket: not available (HTTP-only mode)")
	}
	fmt.Println()
	fmt.Println("Commands: s=step, n=next, o=out, c=continue, r=stack, v=vars, q=quit, h=help")
	fmt.Println()
}

func (s *debugSession) repl() error {
	reader := bufio.NewReader(os.Stdin)

	for {
		// Check if context cancelled
		select {
		case <-s.ctx.Done():
			return nil
		default:
		}

		// Print prompt
		if s.attached {
			fmt.Print("(dbg) > ")
		} else {
			fmt.Print("> ")
		}

		// Read input
		line, err := reader.ReadString('\n')
		if err != nil {
			return nil // EOF or error
		}

		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		// Parse and execute command
		parts := strings.Fields(line)
		cmd := strings.ToLower(parts[0])
		args := parts[1:]

		switch cmd {
		case "q", "quit", "exit":
			if s.attached {
				s.detach()
			}
			fmt.Println("Goodbye!")
			return nil

		case "h", "help", "?":
			s.printHelp()

		case "a", "attach", "listen":
			if err := s.attach(); err != nil {
				fmt.Printf("Error: %v\n", err)
			}

		case "s", "step", "si":
			if err := s.step(adt.DebugStepInto); err != nil {
				fmt.Printf("Error: %v\n", err)
			}

		case "n", "next", "so":
			if err := s.step(adt.DebugStepOver); err != nil {
				fmt.Printf("Error: %v\n", err)
			}

		case "o", "out", "sr":
			if err := s.step(adt.DebugStepReturn); err != nil {
				fmt.Printf("Error: %v\n", err)
			}

		case "c", "continue", "cont":
			if err := s.step(adt.DebugStepContinue); err != nil {
				fmt.Printf("Error: %v\n", err)
			}

		case "r", "stack", "bt", "backtrace":
			if err := s.showStack(); err != nil {
				fmt.Printf("Error: %v\n", err)
			}

		case "v", "vars", "locals":
			if err := s.showVariables(); err != nil {
				fmt.Printf("Error: %v\n", err)
			}

		case "b", "break", "bp":
			if err := s.setBreakpoint(args); err != nil {
				fmt.Printf("Error: %v\n", err)
			}

		case "d", "delete", "del":
			if err := s.deleteBreakpoint(args); err != nil {
				fmt.Printf("Error: %v\n", err)
			}

		case "l", "list", "bps":
			if err := s.listBreakpoints(); err != nil {
				fmt.Printf("Error: %v\n", err)
			}

		case "detach":
			s.detach()

		case "info":
			s.printInfo()

		case "run", "exec":
			if err := s.runProgram(args); err != nil {
				fmt.Printf("Error: %v\n", err)
			}

		case "call", "rfc":
			if err := s.callRFC(args); err != nil {
				fmt.Printf("Error: %v\n", err)
			}

		default:
			fmt.Printf("Unknown command: %s (type 'h' for help)\n", cmd)
		}
	}
}

func (s *debugSession) printHelp() {
	fmt.Println(`
Commands:
  Execution:
    s, step      Step into next statement
    n, next      Step over (execute current line)
    o, out       Step out of current routine
    c, continue  Continue execution to next breakpoint

  Inspection:
    r, stack     Show call stack
    v, vars      Show local variables

  Breakpoints:
    b <prog> <line>  Set line breakpoint
    d <id>           Delete breakpoint
    l, list          List all breakpoints

  Trigger:
    run <prog> [variant]   Run program via RFC (hits breakpoints)
    call <fm> [p=v ...]    Call function module

  Session:
    a, attach    Wait for debuggee (listen mode)
    detach       Detach from current debuggee
    info         Show session info

  General:
    h, help      Show this help
    q, quit      Exit debugger`)
}

func (s *debugSession) attach() error {
	if s.attached {
		fmt.Println("Already attached. Use 'detach' first.")
		return nil
	}

	fmt.Printf("Waiting for debuggee (timeout: %ds)...\n", debugTimeout)
	fmt.Println("Trigger code execution in SAP (run report, unit test, etc.)")

	opts := &adt.ListenOptions{
		DebuggingMode:  adt.DebuggingModeUser,
		User:           s.user,
		TimeoutSeconds: debugTimeout,
	}

	result, err := s.client.DebuggerListen(s.ctx, opts)
	if err != nil {
		return fmt.Errorf("listen failed: %w", err)
	}

	if result.TimedOut {
		fmt.Println("Timeout - no debuggee caught")
		return nil
	}

	if result.Conflict != nil {
		return fmt.Errorf("conflict: %s", result.Conflict.ConflictText)
	}

	if result.Debuggee == nil {
		return fmt.Errorf("no debuggee returned")
	}

	// Attach to debuggee
	fmt.Printf("Caught debuggee: %s at %s:%d\n",
		result.Debuggee.Program, result.Debuggee.Include, result.Debuggee.Line)

	attachResult, err := s.client.DebuggerAttach(s.ctx, result.Debuggee.ID, s.user)
	if err != nil {
		return fmt.Errorf("attach failed: %w", err)
	}

	s.attached = true
	s.debuggeeID = result.Debuggee.ID

	fmt.Printf("Attached! Session: %s\n", attachResult.DebugSessionID)
	fmt.Printf("%s:%d\n", result.Debuggee.Program, result.Debuggee.Line)

	return nil
}

func (s *debugSession) detach() {
	if !s.attached {
		fmt.Println("Not attached")
		return
	}

	if err := s.client.DebuggerDetach(s.ctx); err != nil {
		fmt.Printf("Warning: detach error: %v\n", err)
	}

	s.attached = false
	s.debuggeeID = ""
	fmt.Println("Detached")
}

func (s *debugSession) step(stepType adt.DebugStepType) error {
	if !s.attached {
		return fmt.Errorf("not attached - use 'attach' first")
	}

	result, err := s.client.DebuggerStep(s.ctx, stepType, "")
	if err != nil {
		// Check if session ended
		if strings.Contains(err.Error(), "session") {
			s.attached = false
			s.debuggeeID = ""
			fmt.Println("Debug session ended")
			return nil
		}
		return err
	}

	// Check if debuggee terminated
	if !result.IsSteppingPossible {
		s.attached = false
		s.debuggeeID = ""
		fmt.Println("Debuggee terminated")
		return nil
	}

	// Show current position
	stack, err := s.client.DebuggerGetStack(s.ctx, false)
	if err == nil && len(stack.Stack) > 0 {
		top := stack.Stack[0]
		fmt.Printf("%s:%d  %s\n", top.ProgramName, top.Line, top.EventName)
	}

	return nil
}

func (s *debugSession) showStack() error {
	if !s.attached {
		return fmt.Errorf("not attached - use 'attach' first")
	}

	stack, err := s.client.DebuggerGetStack(s.ctx, true)
	if err != nil {
		return err
	}

	fmt.Println("\nCall Stack:")
	for i, frame := range stack.Stack {
		marker := "  "
		if i == stack.DebugCursorStackIndex {
			marker = "→ "
		}
		sysMarker := ""
		if frame.SystemProgram {
			sysMarker = " (system)"
		}
		fmt.Printf("%s#%d  %s:%d  %s%s\n",
			marker, frame.StackPosition, frame.ProgramName, frame.Line, frame.EventName, sysMarker)
	}
	fmt.Println()

	return nil
}

func (s *debugSession) showVariables() error {
	if !s.attached {
		return fmt.Errorf("not attached - use 'attach' first")
	}

	// Get top-level variables
	childVars, err := s.client.DebuggerGetChildVariables(s.ctx, []string{"@ROOT"})
	if err != nil {
		return err
	}

	if childVars == nil || len(childVars.Variables) == 0 {
		fmt.Println("No variables in current scope")
		return nil
	}

	fmt.Println("\nVariables:")
	for _, v := range childVars.Variables {
		typeInfo := v.DeclaredTypeName
		if typeInfo == "" {
			typeInfo = string(v.MetaType)
		}

		value := v.Value
		if v.IsValueIncomplete {
			value += "..."
		}
		if v.TableLines > 0 {
			value = fmt.Sprintf("<%d rows>", v.TableLines)
		}

		fmt.Printf("  %s = %s (%s)\n", v.Name, value, typeInfo)
	}
	fmt.Println()

	return nil
}

func (s *debugSession) setBreakpoint(args []string) error {
	if len(args) < 2 {
		return fmt.Errorf("usage: b <program> <line>")
	}

	program := strings.ToUpper(args[0])
	line, err := strconv.Atoi(args[1])
	if err != nil {
		return fmt.Errorf("invalid line number: %s", args[1])
	}

	// Try WebSocket first (preferred)
	if s.wsClient != nil {
		ctx, cancel := context.WithTimeout(s.ctx, 10*time.Second)
		defer cancel()

		bpID, err := s.wsClient.SetLineBreakpoint(ctx, program, line)
		if err != nil {
			return fmt.Errorf("set breakpoint failed: %w", err)
		}
		fmt.Printf("Breakpoint %s set at %s:%d\n", bpID, program, line)
		return nil
	}

	return fmt.Errorf("WebSocket not connected - cannot set breakpoints")
}

func (s *debugSession) deleteBreakpoint(args []string) error {
	if len(args) < 1 {
		return fmt.Errorf("usage: d <breakpoint-id>")
	}

	bpID := args[0]

	if s.wsClient != nil {
		ctx, cancel := context.WithTimeout(s.ctx, 10*time.Second)
		defer cancel()

		if err := s.wsClient.DeleteBreakpoint(ctx, bpID); err != nil {
			return fmt.Errorf("delete breakpoint failed: %w", err)
		}
		fmt.Printf("Breakpoint %s deleted\n", bpID)
		return nil
	}

	return fmt.Errorf("WebSocket not connected - cannot delete breakpoints")
}

func (s *debugSession) listBreakpoints() error {
	if s.wsClient != nil {
		ctx, cancel := context.WithTimeout(s.ctx, 10*time.Second)
		defer cancel()

		bps, err := s.wsClient.GetBreakpoints(ctx)
		if err != nil {
			return fmt.Errorf("get breakpoints failed: %w", err)
		}

		if len(bps) == 0 {
			fmt.Println("No breakpoints set")
			return nil
		}

		fmt.Println("\nBreakpoints:")
		for _, bp := range bps {
			id, _ := bp["id"].(string)
			program, _ := bp["program"].(string)
			line, _ := bp["line"].(float64)
			fmt.Printf("  %s: %s:%d\n", id, program, int(line))
		}
		fmt.Println()
		return nil
	}

	// Fallback to HTTP API
	bps, err := s.client.GetExternalBreakpoints(s.ctx, s.user)
	if err != nil {
		return err
	}

	if len(bps.Breakpoints) == 0 {
		fmt.Println("No breakpoints set")
		return nil
	}

	fmt.Println("\nBreakpoints:")
	for _, bp := range bps.Breakpoints {
		fmt.Printf("  %s: %s (kind: %s)\n", bp.ID, bp.URI, bp.Kind)
	}
	fmt.Println()

	return nil
}

func (s *debugSession) printInfo() {
	fmt.Println("\nSession Info:")
	fmt.Printf("  User: %s\n", s.user)
	fmt.Printf("  System: %s\n", cfg.BaseURL)
	fmt.Printf("  Attached: %v\n", s.attached)
	if s.debuggeeID != "" {
		fmt.Printf("  Debuggee: %s\n", s.debuggeeID)
	}
	fmt.Println()
}

// runProgram triggers a program and auto-attaches to catch breakpoints.
// Flow: Start listener → Trigger program → Auto-attach when breakpoint hits
func (s *debugSession) runProgram(args []string) error {
	if len(args) < 1 {
		fmt.Println("Usage: run <program> [variant]")
		fmt.Println("Example: run ZADT_DEBUG_TRIGGER")
		fmt.Println("         run ZADT_DEBUG_TRIGGER TESTVAR")
		fmt.Println("\nThis will: set up listener → trigger program → auto-attach")
		return nil
	}

	if s.wsClient == nil {
		return fmt.Errorf("WebSocket not connected - cannot run programs")
	}

	if s.attached {
		return fmt.Errorf("already attached - use 'detach' first")
	}

	program := strings.ToUpper(args[0])
	variant := ""
	if len(args) > 1 {
		variant = strings.ToUpper(args[1])
	}

	fmt.Printf("Running %s with auto-attach...\n", program)

	// Channel to receive debuggee from listener
	type listenResult struct {
		debuggee *adt.Debuggee
		err      error
	}
	resultCh := make(chan listenResult, 1)

	// Start listener in background goroutine
	go func() {
		opts := &adt.ListenOptions{
			DebuggingMode:  adt.DebuggingModeUser,
			User:           s.user,
			TimeoutSeconds: 30, // Short timeout for triggered execution
		}

		result, err := s.client.DebuggerListen(s.ctx, opts)
		if err != nil {
			resultCh <- listenResult{err: err}
			return
		}

		if result.TimedOut {
			resultCh <- listenResult{err: fmt.Errorf("timeout - no breakpoint hit")}
			return
		}

		if result.Conflict != nil {
			resultCh <- listenResult{err: fmt.Errorf("conflict: %s", result.Conflict.ConflictText)}
			return
		}

		resultCh <- listenResult{debuggee: result.Debuggee}
	}()

	// Give listener a moment to start
	time.Sleep(100 * time.Millisecond)

	// Trigger the program via WebSocket report domain (uses SUBMIT)
	fmt.Printf("Triggering %s via WebSocket SUBMIT...\n", program)

	// RunReport is async - doesn't wait for completion (blocked on breakpoint)
	if err := s.wsClient.RunReport(s.ctx, program, variant); err != nil {
		return fmt.Errorf("failed to trigger report: %w", err)
	}

	// Wait for listener to catch debuggee
	fmt.Println("Waiting for breakpoint...")

	select {
	case result := <-resultCh:
		if result.err != nil {
			return result.err
		}

		if result.debuggee == nil {
			return fmt.Errorf("no debuggee caught")
		}

		// Auto-attach
		fmt.Printf("Caught at %s:%d\n", result.debuggee.Program, result.debuggee.Line)

		attachResult, err := s.client.DebuggerAttach(s.ctx, result.debuggee.ID, s.user)
		if err != nil {
			return fmt.Errorf("attach failed: %w", err)
		}

		s.attached = true
		s.debuggeeID = result.debuggee.ID
		fmt.Printf("Attached! Session: %s\n", attachResult.DebugSessionID)

	case <-s.ctx.Done():
		return s.ctx.Err()

	case <-time.After(35 * time.Second):
		return fmt.Errorf("timeout waiting for breakpoint")
	}

	return nil
}

// callRFC calls a function module to trigger code execution.
func (s *debugSession) callRFC(args []string) error {
	if len(args) < 1 {
		fmt.Println("Usage: call <function_module> [param=value ...]")
		fmt.Println("Example: call RFC_PING")
		fmt.Println("         call BAPI_USER_GET_DETAIL USERNAME=TESTUSER")
		return nil
	}

	if s.wsClient == nil {
		return fmt.Errorf("WebSocket not connected - cannot call RFC")
	}

	fm := strings.ToUpper(args[0])
	params := make(map[string]string)

	// Parse param=value pairs
	for _, arg := range args[1:] {
		parts := strings.SplitN(arg, "=", 2)
		if len(parts) == 2 {
			params[strings.ToUpper(parts[0])] = parts[1]
		}
	}

	fmt.Printf("Calling %s...\n", fm)

	ctx, cancel := context.WithTimeout(s.ctx, 60*time.Second)
	defer cancel()

	result, err := s.wsClient.CallRFC(ctx, fm, params)
	if err != nil {
		return fmt.Errorf("RFC call failed: %w", err)
	}

	fmt.Printf("Result: subrc=%d\n", result.Subrc)

	// Print exports if any
	if len(result.Exports) > 0 {
		fmt.Println("Exports:")
		for k, v := range result.Exports {
			fmt.Printf("  %s = %v\n", k, v)
		}
	}

	return nil
}
