package vsp

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/oisee/open-rfc-go/rfc"
	"github.com/oisee/vibing-steampunk/pkg/adt"
	"github.com/oisee/vibing-steampunk/pkg/saprfc"
	"github.com/spf13/cobra"
)

// `vsp rfc debug` drives the ABAP debugger through the ZADT_DEBUG_RFC facade on
// a pinned RFC conversation. Everything here runs inside ONE process for one
// reason: the ABAP session must survive between attach and step, and it only
// does so on a pinned connection. That is also why the interactive form exists
// — a short-lived command per step would lose the debuggee each time.

var rfcDebugCmd = &cobra.Command{
	Use:   "debug",
	Short: "Drive the ABAP debugger over a pinned RFC session (needs the ZADT_DEBUG facade)",
	Long: `Drive the ABAP debugger over one pinned RFC conversation.

Without arguments it opens an interactive session; with -c it runs a
semicolon-separated script and exits. Commands:

  state              where this session landed, and whether it is pinned
  bp <PROG>[/<INCL>] <LINE> [CONDITION]
                     set an external line breakpoint; name the include when
                     the line is inside a function module or class method
  bps                list external breakpoints (with program and line)
  unbp [PROG [LINE]] delete breakpoints, or "unbp all"
  listen [SECONDS]   block until a debuggee stops (default 60)
  catch [SECONDS]    listen, attach to the first debuggee, and show the stack
  attach <ID>        attach to a waiting debuggee
  step [KIND]        into (default) | over | out | continue
  stack              the attached debuggee's call stack
  detach             end the debugger session and stop the listener
  adt <METHOD> <URI> [NAME=VALUE …] [@bodyfile]
                     tunnel an ADT REST call through this same pinned session
  eclipse [SECONDS]  the same loop through SAP's own ADT debugger resources,
                     with no Z code on the server: listen, attach, stack
  estep [KIND]       ADT step: into (default) | over | return | continue
  estack             ADT call stack
  ebp <OBJECT> <LINE> [COND]
                     set a line breakpoint through ADT — no Z code needed;
                     OBJECT is a name the repository knows, or an ADT URI
  ebps               the breakpoints this client has registered
  esys               toggle breakpoints inside SAP standard code (off by
                     default — SAP refuses to stop there without it)
  eunbp <ID|all>     remove one breakpoint, or all of them
  elocals            the current frame's own variables, with values
  evars [NAME …]     variable values (default roots @ROOT @DATAAGING)
  echildren <ID>     expand a structure, a table or a synthetic root
  eset <NAME> <VALUE>
                     overwrite a variable in the stopped frame — the next
                     statement computes with the new value
  eframe <STACK-URI> move the cursor to another frame, to read its variables
  astart [USER]      start an AMDP debug session (ADT's own, no Z code)
  abp <CLASS> <LINE>   AMDP breakpoint, after astart
  aresume [MAX]      wait for the AMDP debuggee to stop, skipping acknowledgements
  astep [over|continue] step the stopped AMDP debuggee
  atrace [MAX]       walk the stopped AMDP program, one JSON object per line
  astack             the stopped procedure's call stack, ABAP and native lines
  alocals [all]      everything in scope at the stop, from the stop itself
  avar <NAME>        read a variable of the stopped SQLScript
  astop              end the AMDP session
  erec [MAX]         record from here: one JSON object per stop, stepping over
                     calls until the unit is left (default 200 stops)
  evalues            record real values instead of «type:length» placeholders
  eraw               print the next e-command as the XML SAP sent`,
	Args: cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		user, _ := cmd.Flags().GetString("user")
		script, _ := cmd.Flags().GetString("command")
		timeout, _ := cmd.Flags().GetInt("timeout")

		params, err := resolveSystemParams(cmd)
		if err != nil {
			return err
		}
		debugREPLReadOnly = cliReadOnly(params)

		return withRFCDestTimeout(cmd, time.Duration(timeout)*time.Second, func(ctx context.Context, c *rfc.Client, dest saprfc.Params) error {
			dbg, err := saprfc.NewDebugger(ctx, c, user)
			if err != nil {
				return err
			}
			// ADT wants the user named in the query string; the ABAP facade can
			// default it to SY-UNAME server-side, /debugger/listeners cannot.
			rfcDebugUser = user
			if rfcDebugUser == "" {
				rfcDebugUser = dest.User
			}
			defer func() { _ = dbg.Close(ctx) }()

			if script != "" {
				for _, line := range strings.Split(script, ";") {
					if err := runDebugCommand(ctx, dbg, strings.TrimSpace(line)); err != nil {
						return err
					}
				}
				return nil
			}

			fmt.Fprintln(os.Stderr, "pinned debug session — 'help' for commands, 'quit' to end")
			in := bufio.NewScanner(os.Stdin)
			for {
				fmt.Fprint(os.Stderr, "dbg> ")
				if !in.Scan() {
					return nil
				}
				line := strings.TrimSpace(in.Text())
				if line == "quit" || line == "exit" {
					return nil
				}
				if err := runDebugCommand(ctx, dbg, line); err != nil {
					fmt.Fprintln(os.Stderr, "!", err)
				}
			}
		})
	},
}

// adtTerminalID identifies this client to ADT's listener registry. ADT wants a
// 32-character id; it only has to be stable and distinct, not meaningful.
// amdpDebuggee is the stopped AMDP program this session is talking to. It
// arrives with the stop and every resource below the session is addressed by
// it, so it is remembered rather than asked for again.
var amdpDebuggee string

// amdpSession is kept because the data preview resource that serves
// table-valued variables lives outside the debugger and needs the whole
// address, including the HANA session id that only the start call hands out.
var amdpSession *saprfc.AMDPSession

// amdpStopBody is the last stop event, kept because it describes the whole
// scope and nothing else will hand that over a second time.
var amdpStopBody []byte

const adtTerminalID = "56535000000000000000000000006462"

// rfcDebugUser is whose debuggees the ADT flow listens for; the REPL sets it
// from --user, defaulting to the connection's logon user.
var rfcDebugUser string

// debugREPLReadOnly is the selected system's read_only / SAP_READ_ONLY, set by
// vsp rfc debug and vsp adt debug before their loop. Commands that change the
// running program or the system (eset, adt with a writing method) are refused
// when it is set.
var debugREPLReadOnly bool

// rfcDebugValues turns off redaction. A recording is business data by
// construction, so the shape of a value is reported and the value is not,
// unless somebody asks for it on purpose.
var rfcDebugValues bool

// rfcDebugSystem allows breakpoints in SAP's own code to fire. Off by default,
// matching what the system does anyway.
var rfcDebugSystem bool

// rfcDebugRaw makes the next e-command print SAP's XML instead of the model.
// It is one-shot: reading a document raw is a debugging act, not a mode.
var rfcDebugRaw bool

// runDebugCommand executes one line of the little command language.
func runDebugCommand(ctx context.Context, dbg *saprfc.Debugger, line string) error {
	fields := strings.Fields(line)
	if len(fields) == 0 {
		return nil
	}
	arg := func(i int) string {
		if i < len(fields) {
			return fields[i]
		}
		return ""
	}
	num := func(i int) int {
		n, _ := strconv.Atoi(arg(i))
		return n
	}

	var (
		out json.RawMessage
		err error
	)
	verb := strings.ToLower(fields[0])
	if verb != "eraw" {
		// One command's worth of raw output, then back to the model.
		defer func() { rfcDebugRaw = false }()
	}
	switch verb {
	case "help":
		fmt.Fprintln(os.Stderr, "state | bp <PROG>[/<INCL>] <LINE> [COND] | bps | unbp [PROG [LINE]|all] | "+
			"listen [SECONDS] | catch [SECONDS] | attach <ID> | step [into|over|out|continue] | stack | detach | quit\n"+
			"eclipse [SECONDS] | estep [KIND] | estack | elocals | evars [NAME …] | echildren <ID> | eraw | adt <METHOD> <URI>")
		return nil
	case "state":
		out, err = dbg.State(ctx)
	case "bp":
		if len(fields) < 3 {
			return fmt.Errorf("usage: bp <PROGRAM>[/<INCLUDE>] <LINE> [CONDITION]")
		}
		program, include, _ := strings.Cut(arg(1), "/")
		out, err = dbg.SetBreakpoint(ctx, program, include, num(2), strings.Join(fields[3:], " "))
	case "bps":
		out, err = dbg.Breakpoints(ctx)
	case "unbp":
		if strings.EqualFold(arg(1), "all") {
			out, err = dbg.DeleteBreakpoints(ctx, "", 0, true)
		} else {
			out, err = dbg.DeleteBreakpoints(ctx, arg(1), num(2), false)
		}
	case "listen":
		seconds := num(1)
		if seconds <= 0 {
			seconds = 60
		}
		fmt.Fprintf(os.Stderr, "waiting up to %ds for a debuggee…\n", seconds)
		out, err = dbg.Listen(ctx, seconds)
	case "catch":
		seconds := num(1)
		if seconds <= 0 {
			seconds = 60
		}
		fmt.Fprintf(os.Stderr, "waiting up to %ds for a debuggee…\n", seconds)
		who, attached, cerr := dbg.ListenAndAttach(ctx, seconds)
		if cerr != nil {
			return cerr
		}
		if who == nil {
			fmt.Fprintln(os.Stderr, "nobody stopped")
			return nil
		}
		fmt.Fprintf(os.Stderr, "attached to %s (%s) at %s/%s:%d\n",
			who.ID, who.User, who.Program, who.Include, who.Line)
		if len(attached) > 0 {
			printDebugJSON(attached)
		}
		out, err = dbg.Stack(ctx)
	case "attach":
		if arg(1) == "" {
			return fmt.Errorf("usage: attach <DEBUGGEE_ID>")
		}
		out, err = dbg.Attach(ctx, arg(1))
	case "step":
		out, err = dbg.Step(ctx, arg(1))
	case "stack":
		out, err = dbg.Stack(ctx)
	case "eclipse":
		seconds := num(1)
		if seconds <= 0 {
			seconds = 120
		}
		fmt.Fprintf(os.Stderr, "ADT listener, up to %ds…\n", seconds)
		who, stack, cerr := dbg.ADTCatch(ctx, rfcDebugUser, "vsp", adtTerminalID, seconds)
		if who == nil && cerr == nil {
			fmt.Fprintln(os.Stderr, "nobody stopped")
			return nil
		}
		if who != nil {
			fmt.Fprintf(os.Stderr, "%s (%s) at %s/%s:%d — %s %s\n",
				who.ID, who.User, who.Program, who.Include, who.Line, who.Type, who.Name)
		}
		if cerr != nil {
			if who == nil {
				return cerr
			}
			// The debuggee was caught and attached; only the stack read failed.
			// Reporting that as an error throws away a session that is alive
			// and attached — on a release with no stack resource, every catch
			// would look like a catch that did not happen. Say what went wrong
			// and leave the session usable.
			fmt.Fprintf(os.Stderr, "attached, but the stack could not be read: %v\n", cerr)
			return nil
		}
		if rfcDebugRaw {
			fmt.Println(string(stack.Body))
			return nil
		}
		info, perr := adt.ParseStackXML(stack.Body)
		if perr != nil {
			return perr
		}
		fmt.Print(saprfc.FormatStack(info))
		return nil
	case "estep":
		kind := map[string]string{
			"": "stepInto", "into": "stepInto", "over": "stepOver",
			"out": "stepReturn", "return": "stepReturn", "continue": "stepContinue",
		}[strings.ToLower(arg(1))]
		if kind == "" {
			return fmt.Errorf("step kinds: into, over, out, continue")
		}
		res, serr := dbg.ADTStep(ctx, kind)
		if serr != nil {
			return serr
		}
		fmt.Println(string(res.Body))
		return nil
	case "estack":
		if rfcDebugRaw {
			res, serr := dbg.ADTStack(ctx)
			if serr != nil {
				return serr
			}
			fmt.Println(string(res.Body))
			return nil
		}
		info, serr := dbg.StackInfo(ctx)
		if serr != nil {
			return serr
		}
		fmt.Print(saprfc.FormatStack(info))
		return nil
	case "esys":
		// Standard code is off by default because SAP keeps it off: a breakpoint
		// in a system program is accepted and then never fires.
		rfcDebugSystem = !rfcDebugSystem
		dbg.SystemDebugging(rfcDebugSystem)
		fmt.Fprintf(os.Stderr, "breakpoints in SAP standard code: %v\n", rfcDebugSystem)
		return nil
	case "ebp":
		if len(fields) < 3 {
			return fmt.Errorf("usage: ebp <OBJECT|ADT-URI> <LINE> [CONDITION]")
		}
		bps, berr := dbg.ADTAddBreakpoint(ctx, arg(1), num(2), strings.Join(fields[3:], " "))
		if berr != nil {
			return berr
		}
		fmt.Print(saprfc.FormatBreakpoints(bps))
		for _, r := range dbg.Rejected() {
			// SAP does not echo the uri of a line it refused, so name what was
			// asked for rather than printing an empty one.
			where := r.URI
			if where == "" {
				where = fmt.Sprintf("%s:%d", strings.ToUpper(arg(1)), num(2))
			}
			fmt.Fprintf(os.Stderr, "! not placed: %s — %s\n", where, r.ErrorMessage)
		}
		return nil
	case "ebps":
		bps, berr := dbg.ADTBreakpoints(ctx)
		if berr != nil {
			return berr
		}
		fmt.Print(saprfc.FormatBreakpoints(bps))
		return nil
	case "eunbp":
		if arg(1) == "" {
			return fmt.Errorf("usage: eunbp <ID|all>")
		}
		if strings.EqualFold(arg(1), "all") {
			if berr := dbg.ADTClearBreakpoints(ctx); berr != nil {
				return berr
			}
			fmt.Fprintln(os.Stderr, "all breakpoints removed")
			return nil
		}
		bps, berr := dbg.ADTDropBreakpoint(ctx, arg(1))
		if berr != nil {
			return berr
		}
		fmt.Print(saprfc.FormatBreakpoints(bps))
		return nil
	case "eset":
		if len(fields) < 3 {
			return fmt.Errorf("usage: eset <NAME> <VALUE> — overwrites a variable in the stopped frame")
		}
		if gerr := cliWorkflowGate(debugREPLReadOnly, "DebuggerSetVariable"); gerr != nil {
			return gerr
		}
		if serr := dbg.SetVariable(ctx, strings.ToUpper(arg(1)), strings.Join(fields[2:], " ")); serr != nil {
			return serr
		}
		fmt.Fprintf(os.Stderr, "%s set\n", strings.ToUpper(arg(1)))
		return nil
	case "eframe":
		if arg(1) == "" {
			return fmt.Errorf("usage: eframe <N|STACK-URI> — a frame number from estack, or its URI")
		}
		// A number is how a person refers to a frame; the URI is what SAP calls
		// it. Accept both, because the URI can only be had by reading a stack
		// first, which makes it useless in a script.
		var ferr error
		if n := num(1); n > 0 {
			ferr = dbg.GoToFrameAt(ctx, n)
		} else {
			ferr = dbg.GoToFrame(ctx, arg(1))
		}
		if ferr != nil {
			return ferr
		}
		fmt.Fprintln(os.Stderr, "cursor moved; elocals now reads that frame")
		return nil
	case "erec":
		// The recorder: walk from here and write one JSON object per stop.
		max := num(1)
		if max <= 0 {
			max = 200
		}
		n, rerr := dbg.Record(ctx, saprfc.RecordOptions{MaxStops: max, Redact: !rfcDebugValues},
			func(r saprfc.StopRecord) error {
				b, merr := json.Marshal(r)
				if merr != nil {
					return merr
				}
				fmt.Println(string(b))
				return nil
			})
		fmt.Fprintf(os.Stderr, "%d stops recorded\n", n)
		return rerr
	case "evalues":
		rfcDebugValues = !rfcDebugValues
		fmt.Fprintf(os.Stderr, "record real values: %v\n", rfcDebugValues)
		return nil
	case "eraw":
		rfcDebugRaw = true
		fmt.Fprintln(os.Stderr, "raw XML for the next e-command")
		return nil
	case "elocals":
		vars, verr := dbg.Locals(ctx)
		if verr != nil {
			return verr
		}
		fmt.Print(saprfc.FormatVariables(vars))
		return nil
	case "evars":
		if rfcDebugRaw {
			res, verr := dbg.ADTVariables(ctx, fields[1:])
			if verr != nil {
				return verr
			}
			fmt.Println(string(res.Body))
			return nil
		}
		vars, verr := dbg.Vars(ctx, fields[1:])
		if verr != nil {
			return verr
		}
		fmt.Print(saprfc.FormatVariables(vars))
		return nil
	case "echildren":
		if arg(1) == "" {
			return fmt.Errorf("usage: echildren <PARENT_ID> — an id from elocals or evars")
		}
		if rfcDebugRaw {
			res, verr := dbg.ADTChildVariables(ctx, fields[1:])
			if verr != nil {
				return verr
			}
			fmt.Println(string(res.Body))
			return nil
		}
		info, verr := dbg.Expand(ctx, arg(1))
		if verr != nil {
			return verr
		}
		if info == nil {
			fmt.Println("nothing under", arg(1))
			return nil
		}
		fmt.Print(saprfc.FormatVariables(info.Variables))
		// A table too large to read whole is sampled, and the reader has to be
		// told — rows 1..33 of a million look exactly like a table with 33 rows
		// in it.
		if sample := dbg.LastTableSample(); sample.Partial() {
			fmt.Fprintf(os.Stderr, "%d of %d rows: %s\n",
				len(sample.Rows), sample.Lines, saprfc.FormatRowRanges(sample.Rows))
		}
		return nil
	case "astart":
		// AMDP debugging over ADT's own resources. Everything about it has to
		// stay on this one connection: the server keeps the session in
		// class-data, so a second connection finds nothing.
		who := arg(1)
		if who == "" {
			who = rfcDebugUser
		}
		// Report each acknowledgement as it is drained rather than after the
		// wait: "VALID" is SAP saying it understood the breakpoint position,
		// and it is the signal a caller should wait for before running the
		// thing it wants to catch. Reported only once the wait is over, it is
		// missing precisely when the wait had to be killed.
		dbg.AMDPOnAck = func(kind, state, reason string) {
			if state == "" {
				fmt.Fprintf(os.Stderr, "· %s\n", kind)
				return
			}
			fmt.Fprintf(os.Stderr, "· %s — SAP calls the breakpoint %s%s\n", kind, state,
				map[bool]string{true: "", false: " — " + reason}[reason == ""])
		}
		session, aerr := dbg.AMDPStart(ctx, who, true)
		if aerr != nil {
			return aerr
		}
		amdpSession = session
		fmt.Fprintf(os.Stderr, "AMDP debug session %s\n", session.MainID)
		if session.HANASessionID != "" {
			// Printed because its presence is the evidence that the bridge to
			// the database side was actually established.
			fmt.Fprintf(os.Stderr, "bridged to HANA session %s\n", session.HANASessionID)
		}
		return nil
	case "abp":
		if len(fields) < 3 {
			return fmt.Errorf("usage: abp <CLASS> <LINE> — an AMDP breakpoint, after astart")
		}
		name := strings.ToUpper(arg(1))
		bp := saprfc.AMDPBreakpoint{
			ClientID: "vsp-1",
			URI: fmt.Sprintf("/sap/bc/adt/oo/classes/%s/source/main#start=%d",
				strings.ToLower(name), num(2)),
			Name: name,
			Type: "CLAS/OC",
		}
		if berr := dbg.AMDPSyncBreakpoints(ctx, saprfc.AMDPSyncFull, []saprfc.AMDPBreakpoint{bp}); berr != nil {
			return berr
		}
		fmt.Fprintf(os.Stderr, "AMDP breakpoint at %s:%d\n", name, num(2))
		return nil
	case "aresume":
		res, aerr := dbg.AMDPAwaitStop(ctx, num(1))
		if res != nil {
			// Kept whole: the stop carries the variables and the call stack,
			// and asking again for what it already said would be a request
			// spent on nothing.
			amdpStopBody = res.Body
			if pos := saprfc.AMDPStopPosition(res.Body); pos != nil {
				// Kept so astep and avar have something to address: every
				// resource below the session is per debuggee, and the id
				// arrives only with the stop.
				amdpDebuggee = pos.DebuggeeID
				fmt.Fprintf(os.Stderr, "stopped at %s:%d\n", pos.Procedure, pos.Line)
			}
		}
		if state, reason := dbg.AMDPBreakpointState(); state != "" {
			fmt.Fprintf(os.Stderr, "SAP calls the breakpoint %s%s\n", state,
				map[bool]string{true: "", false: " — " + reason}[reason == ""])
		}
		if aerr != nil {
			return aerr
		}
		fmt.Println(string(res.Body))
		return nil
	case "astep":
		kind := arg(1)
		if kind == "" {
			kind = "over"
		}
		pos, aerr := dbg.AMDPStepAndWait(ctx, amdpDebuggee, kind, 6)
		if aerr != nil {
			return aerr
		}
		if pos.DebuggeeID != "" {
			amdpDebuggee = pos.DebuggeeID
		}
		fmt.Fprintf(os.Stderr, "%s:%d\n", pos.Procedure, pos.Line)
		return nil
	case "atrace":
		n, aerr := dbg.AMDPTrace(ctx, amdpDebuggee, num(1), func(p saprfc.AMDPPosition) error {
			if p.DebuggeeID != "" {
				// A trace carries the debuggee forward exactly as astep does.
				// Without this, an avar after an atrace addresses the id the
				// first stop had, and a stale id is not an error here: the
				// resource answers 200 with nothing, which reads as "no such
				// variable" rather than "you asked the wrong debuggee".
				amdpDebuggee = p.DebuggeeID
			}
			b, merr := json.Marshal(map[string]any{
				"procedure": p.Procedure, "line": p.Line, "uri": p.URI,
			})
			if merr != nil {
				return merr
			}
			fmt.Println(string(b))
			return nil
		})
		fmt.Fprintf(os.Stderr, "%d stops traced\n", n)
		return aerr
	case "avar":
		if arg(1) == "" {
			return fmt.Errorf("usage: avar <NAME> — a variable of the stopped SQLScript")
		}
		res, aerr := dbg.AMDPReadVariable(ctx, amdpDebuggee, arg(1), num(2))
		if aerr != nil {
			return aerr
		}
		values := saprfc.AMDPScalarValues(res.Body)
		if len(values) == 0 {
			// The answer came back and named nothing, which is not the same as
			// a variable that is not in scope — say which happened.
			fmt.Fprintf(os.Stderr, "the debuggee answered about %s without a value\n", strings.ToUpper(arg(1)))
			return nil
		}
		for _, v := range values {
			fmt.Println(saprfc.FormatAMDPScalar(v))
		}
		return nil
	case "astack":
		if amdpStopBody == nil {
			return fmt.Errorf("nothing has stopped yet, so there is no stack to show")
		}
		frames := saprfc.AMDPCallStack(amdpStopBody)
		if len(frames) == 0 {
			fmt.Fprintln(os.Stderr, "the stop carried no call stack")
			return nil
		}
		for _, f := range frames {
			fmt.Println(saprfc.FormatAMDPFrame(f))
		}
		return nil
	case "alocals":
		// Everything in scope, from the stop itself: no request, because the
		// stop already said it.
		if amdpStopBody == nil {
			return fmt.Errorf("nothing has stopped yet, so there is no scope to show")
		}
		vars := saprfc.AMDPVariablesAtStop(amdpStopBody)
		if len(vars) == 0 {
			fmt.Fprintln(os.Stderr, "the stop named no variables")
			return nil
		}
		showSystem := strings.EqualFold(arg(1), "all")
		for _, v := range vars {
			if !showSystem && v.Scope == "system" {
				continue
			}
			fmt.Println(saprfc.FormatAMDPVariableInfo(v))
		}
		return nil
	case "atable":
		if arg(1) == "" {
			return fmt.Errorf("usage: atable <NAME> [ROWS] — a table-valued variable of the stopped SQLScript")
		}
		res, aerr := dbg.AMDPTableRows(ctx, amdpSession, amdpDebuggee, arg(1),
			saprfc.AMDPSchemaAtStop(amdpStopBody), num(2))
		if aerr != nil {
			// Say that this is a known gap rather than letting it read as a
			// fault in the caller's request: the address is right and the
			// server refuses to build its data provider from it.
			fmt.Fprintln(os.Stderr, "reading a table variable does not work yet — see AMDPTableRows for where it stands")
			return aerr
		}
		fmt.Println(string(res.Body))
		return nil
	case "astop":
		if aerr := dbg.AMDPTerminate(ctx, true); aerr != nil {
			return aerr
		}
		fmt.Fprintln(os.Stderr, "AMDP debug session ended")
		return nil
	case "adt":
		if len(fields) < 3 {
			return fmt.Errorf("usage: adt <METHOD> <URI> [NAME=VALUE …]")
		}
		if !rfcADTReadMethod(arg(1)) {
			if gerr := cliWorkflowGate(debugREPLReadOnly, "DebugADT "+strings.ToUpper(arg(1))); gerr != nil {
				return gerr
			}
		}
		// A missing Accept is filled in by the tunnel itself.
		var headers []saprfc.ADTHeader
		var body []byte
		for _, raw := range fields[3:] {
			if strings.HasPrefix(raw, "@") {
				// @path — the request body, read from a file, because a source
				// document does not fit on one prompt line.
				content, rerr := os.ReadFile(strings.TrimPrefix(raw, "@"))
				if rerr != nil {
					return rerr
				}
				body = content
				continue
			}
			name, value, ok := strings.Cut(raw, "=")
			if !ok {
				return fmt.Errorf("a header must be NAME=VALUE (or @file for a body), got %q", raw)
			}
			headers = append(headers, saprfc.ADTHeader{Name: name, Value: value})
		}
		res, aerr := dbg.ADT(ctx, arg(1), arg(2), headers, body)
		if aerr != nil {
			return aerr
		}
		fmt.Fprintf(os.Stderr, "%d %s · %d bytes · %s\n",
			res.Status, res.ReasonPhrase, len(res.Body), res.Header("content-type"))
		// Some ADT resources answer with the thing you need in a header and
		// nothing in the body — starting an AMDP debug session returns its id
		// only in Location, and a probe that prints just the body looks like it
		// got an answer while missing the answer. Print the headers that carry
		// an identifier or point somewhere.
		for _, name := range []string{"location", "content-location", "etag"} {
			if value := res.Header(name); value != "" {
				fmt.Fprintf(os.Stderr, "%s: %s\n", name, value)
			}
		}
		fmt.Println(string(res.Body))
		return nil
	case "detach":
		out, err = dbg.Detach(ctx)
	default:
		return fmt.Errorf("unknown command %q — try 'help'", fields[0])
	}
	if err != nil {
		return err
	}
	if len(out) > 0 {
		printDebugJSON(out)
	}
	return nil
}

func printDebugJSON(raw json.RawMessage) {
	var pretty any
	if json.Unmarshal(raw, &pretty) == nil {
		b, _ := json.MarshalIndent(pretty, "", "  ")
		fmt.Println(string(b))
		return
	}
	fmt.Println(string(raw))
}

func init() {
	rfcDebugCmd.Flags().String("user", "", "Whose debuggees to listen for (default: the logon user)")
	rfcDebugCmd.Flags().StringP("command", "c", "", "Run a semicolon-separated script instead of going interactive")
	rfcDebugCmd.Flags().Int("timeout", 300, "Seconds a single RFC call may take; must exceed the listen timeout")
	rfcCmd.AddCommand(rfcDebugCmd)
}
