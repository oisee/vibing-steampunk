package main

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/spf13/cobra"

	"github.com/oisee/vibing-steampunk/pkg/saprfc"
	"github.com/oisee/vibing-steampunk/pkg/scripting"
)

var luaCmd = &cobra.Command{
	Use:   "lua [script.lua]",
	Short: "Run Lua scripts or interactive REPL",
	Long: `Run Lua scripts for automated debugging, testing, and analysis.

Without arguments, starts an interactive Lua REPL.
With a script file, executes the script.

Scripts get one debug session, held for the life of the script and opened only
if something asks for it — a pinned RFC conversation, or a stateful ADT session
where there is no gateway.

  Breakpoints   setBreakpoint(object, line, condition), setStatementBP("CALL BADI"),
                setExceptionBP("CX_SY_ZERODIVIDE"), setMessageBP("00", "001", "E"),
                getBreakpoints(), deleteBreakpoint(id), clearBreakpoints(),
                systemDebugging(true)   -- SAP's own code, off by default
  Session       listen(seconds) -- waits and attaches; attach(id), detach()
  Movement      stepOver(), stepInto(), stepReturn(), continue_(), runToLine(uri)
  State         locals(), getVariable("LV_X"), setVariable("LV_X", "42"),
                expand(id), getStack(), frame(stackUri)
  Recording     record(maxStops, withValues) -- one table per stop
  Everything else: searchObject, getSource, runUnitTests, query, lint, ...

See examples/lua/ for a script that captures a unit and then changes its inputs.

Examples:
  # Start interactive REPL
  vsp lua

  # Run a script
  vsp lua scripts/debug-pricing.lua

  # Execute inline script
  vsp lua -e 'print(searchObject("ZCL_*", "CLAS"))'`,
	Args: cobra.MaximumNArgs(1),
	RunE: runLua,
}

var (
	luaExec    string
	luaVerbose bool
)

func init() {
	luaCmd.Flags().StringVarP(&luaExec, "exec", "e", "", "Execute Lua code directly")
	luaCmd.Flags().BoolVarP(&luaVerbose, "verbose", "v", false, "Verbose output")

	rootCmd.AddCommand(luaCmd)
}

func runLua(cmd *cobra.Command, args []string) error {
	// Resolve configuration (same as MCP server)
	resolveConfig(cmd.Parent())

	// No validateConfig() here on purpose: it checks the global cfg.BaseURL,
	// which a named system (-s / .vsp.json) never populates, so it rejected
	// `-s a4h` before the resolver ever ran. createADTClientFor resolves the
	// system and reports a real error if none can be found.

	// No processCookieAuth here either: it reads the same global cfg. A named
	// system carries its own credentials through resolveSystemParams, which is
	// how deploy, deps and every other -s-aware command already work.

	// Create ADT client
	client, err := createADTClientFor(cmd)
	if err != nil {
		return err
	}

	// Create Lua engine
	engine := scripting.NewLuaEngine(client)
	defer engine.Close()

	// A debug session, opened only if the script asks for one. Debugging needs a
	// session that survives between calls, which the ordinary ADT client above
	// is not; this hands the engine one when it is wanted and costs nothing when
	// it is not.
	engine.SetDebuggerFactory(func(ctx context.Context) (*saprfc.Debugger, func(), error) {
		dest, err := rfcDestinationFor(cmd)
		if err == nil {
			c, derr := saprfc.OpenWithTimeout(ctx, dest, 5*time.Minute)
			if derr == nil {
				dbg, perr := saprfc.NewDebugger(ctx, c, dest.User)
				if perr == nil {
					return dbg, func() { _ = c.Close(ctx) }, nil
				}
				_ = c.Close(ctx)
			}
		}
		// No gateway, or no RFC user: the same debugger over a stateful ADT
		// session instead.
		sys, serr := resolveSystemParams(cmd)
		if serr != nil {
			return nil, nil, fmt.Errorf("no debug session: neither RFC nor a stateful ADT session could be opened: %w", serr)
		}
		transport, terr := statefulADTTransport(sys, 5*time.Minute)
		if terr != nil {
			return nil, nil, fmt.Errorf("no debug session over HTTPS either: %w", terr)
		}
		return saprfc.NewADTDebugger(transport, sys.User), func() {}, nil
	})

	// Set output for verbose mode
	if luaVerbose {
		fmt.Fprintf(os.Stderr, "[LUA] Connected to: %s\n", cfg.BaseURL)
		fmt.Fprintf(os.Stderr, "[LUA] Client: %s, Language: %s\n", cfg.Client, cfg.Language)
	}

	// Execute based on mode
	if luaExec != "" {
		// Execute inline code
		if err := engine.Execute(luaExec); err != nil {
			return fmt.Errorf("lua error: %w", err)
		}
		return nil
	}

	if len(args) > 0 {
		// Execute script file
		scriptFile := args[0]
		if _, err := os.Stat(scriptFile); os.IsNotExist(err) {
			return fmt.Errorf("script file not found: %s", scriptFile)
		}

		if luaVerbose {
			fmt.Fprintf(os.Stderr, "[LUA] Running script: %s\n", scriptFile)
		}

		if err := engine.ExecuteFile(scriptFile); err != nil {
			return fmt.Errorf("script error: %w", err)
		}
		return nil
	}

	// Interactive REPL mode
	engine.REPL()
	return nil
}
