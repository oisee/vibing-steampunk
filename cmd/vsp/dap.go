package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/spf13/cobra"

	"github.com/oisee/vibing-steampunk/internal/dap"
	"github.com/oisee/vibing-steampunk/pkg/saprfc"
)

// `vsp dap` is the debugger for editors: a Debug Adapter Protocol server on
// stdin/stdout, so VS Code, nvim-dap and JetBrains can set breakpoints in ABAP
// and step through it on a real system.
//
// It is the same debugger as `vsp debug ui` and `vsp adt debug` — saprfc's
// Debugger on one stateful ADT session, SAP's own /sap/bc/adt/debugger*
// resources, no Z code — with the editor as the face. Like them it never runs
// the program: the session arms breakpoints and waits, and the program is
// started in SAP by whoever means to run it.
//
// It is CLI-only on purpose. It is not an MCP tool: a debug session starts
// when a person presses F5, not when a model decides to.

var dapCmd = &cobra.Command{
	Use:   "dap",
	Short: "Debug Adapter Protocol server on stdio, for debugging ABAP from an editor",
	Long: `Serve the Debug Adapter Protocol on stdin/stdout.

An editor starts it as a debug adapter; it is not meant to be run by hand.
The logon is vsp's usual one (.vsp.json system, or SAP_* env); the editor's
launch configuration may pick a system and a user, and never carries a
password.

  VS Code (launch.json, with any extension that runs an executable adapter):
    { "type": "abap-sap", "request": "attach", "name": "ABAP on A4H",
      "system": "a4h", "object": "ZVSP_DEBUG_DEMO", "sourceRoot": "${workspaceFolder}" }

  nvim-dap:
    dap.adapters.vsp = { type = "executable", command = "vsp", args = { "dap" } }

launch and attach both arm the breakpoints and wait: run the program in SAP
(SE38, a unit test, an RFC call, a job) and the editor stops on the line.

Breakpoints are set in files named by vsp's convention (zcl_x.clas.abap,
zrep.prog.abap, zgrp.fugr.z_fm.abap, ...), or in vsp://<OBJECT> or
vsp:///sap/bc/adt/... sources. A read-only system debugs, but variables
cannot be changed.`,
	Args: cobra.NoArgs,
	RunE: runDAP,
}

var (
	dapUser     string
	dapTimeout  int
	dapReadOnly bool
)

func init() {
	dapCmd.Flags().StringVar(&dapUser, "user", "", "Whose debuggees to catch (default: the launch's \"user\", then the logon user)")
	dapCmd.Flags().IntVar(&dapTimeout, "timeout", 300, "Seconds a single HTTP request may take; must exceed the listener's wait")
	dapCmd.Flags().BoolVar(&dapReadOnly, "read-only", false, "Refuse to change variables, whatever the system config says")
	rootCmd.AddCommand(dapCmd)
}

func runDAP(cmd *cobra.Command, _ []string) error {
	// stdout is the protocol. Anything else that prints to it — a warning
	// deep in config loading, a single sign-on prompt — would corrupt the
	// stream, so the process's stdout becomes stderr and only the adapter
	// keeps the real one.
	protocol := os.Stdout
	os.Stdout = os.Stderr
	defer func() { os.Stdout = protocol }()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	srv := dap.New(ctx, os.Stdin, protocol, dapOpener(cmd))

	// An editor stops an adapter by closing its pipes or by signalling it.
	// Either way the debuggee must not stay suspended in a work process.
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)
	defer signal.Stop(stop)
	go func() {
		if _, ok := <-stop; !ok {
			return
		}
		srv.Shutdown()
		os.Exit(130)
	}()

	return srv.Serve()
}

// dapOpener opens the debug session for a launch or attach, from vsp's own
// configuration. The adapter never sees a credential.
func dapOpener(cmd *cobra.Command) dap.Opener {
	return func(ctx context.Context, args dap.LaunchArgs) (*dap.Session, error) {
		if s := strings.TrimSpace(args.System); s != "" {
			systemName = s
		}
		params, err := resolveSystemParams(cmd)
		if err != nil {
			return nil, err
		}
		if params.URL == "" {
			return nil, fmt.Errorf("no SAP system configured: name one in .vsp.json and pass \"system\", or set SAP_URL")
		}
		timeout := time.Duration(dapTimeout) * time.Second
		if listen := time.Duration(args.ListenSeconds) * time.Second; listen+30*time.Second > timeout {
			return nil, fmt.Errorf("listenSeconds (%d) must be at least 30s below --timeout (%d)", args.ListenSeconds, dapTimeout)
		}
		transport, err := statefulADTTransport(params, timeout)
		if err != nil {
			return nil, err
		}

		user := strings.ToUpper(strings.TrimSpace(args.User))
		if user == "" {
			user = strings.ToUpper(strings.TrimSpace(dapUser))
		}
		if user == "" {
			user = strings.ToUpper(strings.TrimSpace(params.User))
		}
		if user == "" {
			// Single sign-on carries a session, not a name; ask the system.
			resolved, werr := saprfc.CurrentUser(ctx, transport)
			if werr != nil {
				return nil, fmt.Errorf("no user to listen for, and the system would not name the logon (%w): set \"user\" in the launch configuration", werr)
			}
			user = resolved
		}
		return &dap.Session{
			Debugger: saprfc.NewADTDebugger(transport, user),
			User:     user,
			System:   params.Name,
			ReadOnly: cliReadOnly(params) || dapReadOnly,
		}, nil
	}
}
