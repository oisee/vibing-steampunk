package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/oisee/open-rfc-go/rfc"
	"github.com/spf13/cobra"

	"github.com/oisee/vibing-steampunk/pkg/adt"
	"github.com/oisee/vibing-steampunk/pkg/config"
	"github.com/oisee/vibing-steampunk/pkg/saprfc"
)

var rfcCmd = &cobra.Command{
	Use:   "rfc",
	Short: "Call SAP function modules over classic RFC (SDK-free)",
	Long: `Call ABAP function modules over classic RFC, against the same system vsp
uses for ADT. The gateway host defaults to the system URL's host and the port to
3300 + system number; override with --rfc-host / --sysnr / --port, or per system
with rfc_host / rfc_sysnr / rfc_port in .vsp.json.

RFC logon uses rfc_user/rfc_password (which fall back to SAP_USER/SAP_PASSWORD),
otherwise the system's own user/password.`,
}

var rfcInfoCmd = &cobra.Command{
	Use:   "info",
	Short: "RFC system info (RFC_SYSTEM_INFO)",
	RunE: func(cmd *cobra.Command, args []string) error {
		return withRFC(cmd, func(ctx context.Context, c *rfc.Client) error {
			r, err := c.Call(ctx, "RFC_SYSTEM_INFO", nil)
			if err != nil {
				return err
			}
			return emitRFC(r.Get("RFCSI_EXPORT"))
		})
	},
}

var rfcPingCmd = &cobra.Command{
	Use:   "ping",
	Short: "RFC connection test (RFC_PING)",
	RunE: func(cmd *cobra.Command, args []string) error {
		return withRFC(cmd, func(ctx context.Context, c *rfc.Client) error {
			if _, err := c.Call(ctx, "RFC_PING", nil); err != nil {
				return err
			}
			fmt.Println("ok")
			return nil
		})
	},
}

var rfcDescribeCmd = &cobra.Command{
	Use:   "describe <FUNCTION_MODULE>",
	Short: "Describe an FM interface as an MCP-tool JSON Schema",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		return withRFC(cmd, func(ctx context.Context, c *rfc.Client) error {
			tool, err := c.DescribeTool(ctx, strings.ToUpper(args[0]))
			if err != nil {
				return err
			}
			return emitRFC(tool)
		})
	},
}

var rfcCallCmd = &cobra.Command{
	Use:   "call <FUNCTION_MODULE> [json]",
	Short: "Call a function module with JSON parameters",
	Long: `Call any RFC-enabled function module. Parameters are a JSON object, given
inline, with --file, or on stdin; values are coerced to each parameter's type.`,
	Args: cobra.RangeArgs(1, 2),
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := rfcWriteGate(cmd, "RFCCall"); err != nil {
			return err
		}
		raw := ""
		if len(args) > 1 {
			raw = args[1]
		}
		if file, _ := cmd.Flags().GetString("file"); file != "" {
			b, err := os.ReadFile(file)
			if err != nil {
				return err
			}
			raw = string(b)
		}
		if stdin, _ := cmd.Flags().GetBool("stdin"); stdin {
			b, err := readAllStdin()
			if err != nil {
				return err
			}
			raw = string(b)
		}
		params := rfc.Params{}
		if strings.TrimSpace(raw) != "" {
			dec := json.NewDecoder(strings.NewReader(raw))
			dec.UseNumber()
			var obj map[string]any
			if err := dec.Decode(&obj); err != nil {
				return fmt.Errorf("parameters must be a JSON object: %w", err)
			}
			params = obj
		}
		return withRFC(cmd, func(ctx context.Context, c *rfc.Client) error {
			r, err := c.Call(ctx, strings.ToUpper(args[0]), params)
			if err != nil {
				return err
			}
			return emitRFC(r)
		})
	},
}

var rfcADTCmd = &cobra.Command{
	Use:   "adt <METHOD> <URI>",
	Short: "Send an ADT REST request through the classic-RFC tunnel (no HTTP)",
	Long: `Tunnel an ADT REST request over classic RFC, via SAP's own
SADT_REST_RFC_ENDPOINT. The request reaches the same handlers the ICF nodes under
/sap/bc/adt/ serve, so this works on systems whose HTTP port is closed entirely.

The status line and headers go to stderr, the body to stdout, so a body can be
piped or redirected as it stands.

  vsp rfc adt GET /sap/bc/adt/discovery
  vsp rfc adt GET /sap/bc/adt/programs/programs/RSUSR000/source/main -H Accept=text/plain

No CSRF token is fetched and no session is kept, so this is a read-only door:
use ADT over HTTP for stateful, token-protected flows. On a read-only system
(read_only in .vsp.json, or SAP_READ_ONLY) only GET, HEAD and OPTIONS are sent.`,
	Args: cobra.ExactArgs(2),
	// A 4xx/5xx from ADT is a result, not a usage mistake: report it and exit
	// non-zero without dumping the flag list over the response body.
	SilenceUsage: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		req := saprfc.ADTRequest{Method: args[0], URI: args[1]}
		if !rfcADTReadMethod(req.Method) {
			if err := rfcWriteGate(cmd, "RFCADT "+strings.ToUpper(req.Method)); err != nil {
				return err
			}
		}
		raw, _ := cmd.Flags().GetStringArray("header")
		for _, kv := range raw {
			name, value, found := strings.Cut(kv, "=")
			if !found {
				return fmt.Errorf("header %q must be NAME=VALUE", kv)
			}
			req.Headers = append(req.Headers, saprfc.ADTHeader{Name: name, Value: value})
		}
		if body, _ := cmd.Flags().GetString("body"); body != "" {
			b, err := os.ReadFile(body)
			if err != nil {
				return err
			}
			req.Body = b
		}
		return withRFC(cmd, func(ctx context.Context, c *rfc.Client) error {
			res, err := saprfc.CallADT(ctx, c, req)
			if err != nil {
				return err
			}
			fmt.Fprintf(os.Stderr, "%s %d %s\n", res.Version, res.Status, res.ReasonPhrase)
			for _, h := range res.Headers {
				fmt.Fprintf(os.Stderr, "%s: %s\n", h.Name, h.Value)
			}
			fmt.Fprintf(os.Stderr, "(%d bytes)\n", len(res.Body))
			if out, _ := cmd.Flags().GetString("output"); out != "" {
				return os.WriteFile(out, res.Body, 0o644)
			}
			_, err = os.Stdout.Write(res.Body)
			if err == nil && res.Status >= 400 {
				return fmt.Errorf("ADT returned HTTP %d %s", res.Status, res.ReasonPhrase)
			}
			return err
		})
	},
}

var rfcProbeCmd = &cobra.Command{
	Use:   "probe",
	Short: "Fingerprint the system over RFC (release, components, helpers, authorizations)",
	Long: `Gather what you want to know before trusting a system with real work: what it
is, which components are installed, whether the vsp/abapGit helpers are present, and
which function modules this user is actually authorized to call — the last of which
ADT cannot answer. Read-only: nothing is executed or written.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		return withRFCDest(cmd, func(ctx context.Context, c *rfc.Client, dest saprfc.Params) error {
			probe, err := saprfc.RunProbe(ctx, c, dest)
			if err != nil {
				return err
			}
			if format, _ := cmd.Flags().GetString("format"); format == "json" {
				return emitRFC(probe)
			}
			fmt.Print(probe.Text())
			return nil
		})
	},
}

var rfcExportCmd = &cobra.Command{
	Use:   "export <PACKAGE>",
	Short: "Serialize a package to an abapGit ZIP over RFC",
	Long: `Serialize an ABAP package into an abapGit ZIP with a single RFC call to
abapGit's own Z_ABAPGIT_SERIALIZE_PACKAGE. Needs abapGit installed on the system;
it needs no vsp helper, and no HTTP.`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		out, _ := cmd.Flags().GetString("output")
		if out == "" {
			out = strings.ToLower(strings.TrimPrefix(args[0], "$")) + ".zip"
		}
		opts := saprfc.ExportOptions{}
		opts.FolderLogic, _ = cmd.Flags().GetString("folder-logic")
		opts.MainLanguageOnly, _ = cmd.Flags().GetBool("main-lang-only")
		return withRFC(cmd, func(ctx context.Context, c *rfc.Client) error {
			zip, err := saprfc.ExportPackage(ctx, c, args[0], opts)
			if err != nil {
				return err
			}
			if err := os.WriteFile(out, zip, 0o644); err != nil {
				return err
			}
			fmt.Fprintf(os.Stderr, "wrote %s (%d bytes)\n", out, len(zip))
			return nil
		})
	},
}

var rfcRunCmd = &cobra.Command{
	Use:   "run <REPORT>",
	Short: "Run an ABAP report as a background job over RFC",
	Long: `Schedule a report as a background job (the XBP BAPIs), optionally
wait for it to finish, and optionally fetch its spool. This is the thing the ADT
WebSocket path cannot do — APC forbids SUBMIT — and it needs no helper on the system.

  vsp rfc run RSPARAM --wait 60
  vsp rfc run ZMY_REPORT -p P_WERKS=1000 -p S_MATNR=M1 --wait 120 --spool
  vsp rfc run ZMY_REPORT --variant DEFAULT --wait 60 --spool`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := rfcWriteGate(cmd, "RFCRunReport"); err != nil {
			return err
		}
		raw, _ := cmd.Flags().GetStringArray("param")
		var params []saprfc.ReportParam
		for _, kv := range raw {
			name, value, found := strings.Cut(kv, "=")
			if !found {
				return fmt.Errorf("parameter %q must be NAME=VALUE", kv)
			}
			params = append(params, saprfc.ReportParam{Name: name, Low: value})
		}
		jobName, _ := cmd.Flags().GetString("job-name")
		waitSecs, _ := cmd.Flags().GetInt("wait")
		wantSpool, _ := cmd.Flags().GetBool("spool")

		return withRFC(cmd, func(ctx context.Context, c *rfc.Client) error {
			variant, _ := cmd.Flags().GetString("variant")
			run, err := saprfc.RunReportWith(ctx, c, saprfc.ReportRequest{
				Report: args[0], JobName: jobName, Variant: variant, Params: params,
				Wait: time.Duration(waitSecs) * time.Second,
			})
			if err != nil {
				return err
			}
			if wantSpool && run.Status == "F" {
				spool, serr := saprfc.ReadSpool(ctx, c, run.JobName, run.JobCount)
				if serr != nil {
					fmt.Fprintln(os.Stderr, "spool unavailable:", serr)
				} else {
					run.Spool = spool
				}
			}
			return emitRFC(run)
		})
	},
}

var rfcSpoolCmd = &cobra.Command{
	Use:   "spool <JOBNAME> <JOBCOUNT>",
	Short: "Read a background job's spool list over RFC",
	Args:  cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		step, _ := cmd.Flags().GetInt("step")
		return withRFC(cmd, func(ctx context.Context, c *rfc.Client) error {
			out, err := saprfc.ReadSpoolStep(ctx, c, args[0], args[1], step)
			if err != nil {
				return err
			}
			if out == "" {
				fmt.Fprintln(os.Stderr, "the job produced no spool list")
				return nil
			}
			fmt.Print(out)
			return nil
		})
	},
}

// The debugger's read half needs no ABAP on the server: the waiting debuggees
// and the external breakpoints are ordinary transparent tables.

var rfcDebuggeesCmd = &cobra.Command{
	Use:   "debuggees",
	Short: "List the ABAP sessions parked in the debugger, waiting to be attached",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		user, _ := cmd.Flags().GetString("user")
		asJSON, _ := cmd.Flags().GetBool("json")
		return withRFC(cmd, func(ctx context.Context, c *rfc.Client) error {
			found, err := saprfc.WaitingDebuggees(ctx, c, user)
			if err != nil {
				return err
			}
			if asJSON {
				return emitRFC(found)
			}
			if len(found) == 0 {
				fmt.Fprintln(os.Stderr, "nobody is waiting in the debugger")
				return nil
			}
			for _, d := range found {
				fmt.Println(d.Text())
			}
			return nil
		})
	},
}

var rfcBreakpointsCmd = &cobra.Command{
	Use:   "breakpoints",
	Short: "List the external breakpoints registered for a user",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		user, _ := cmd.Flags().GetString("user")
		return withRFC(cmd, func(ctx context.Context, c *rfc.Client) error {
			bps, err := saprfc.ExternalBreakpoints(ctx, c, user)
			if err != nil {
				return err
			}
			if len(bps) == 0 {
				fmt.Fprintln(os.Stderr, "no external breakpoints are registered")
				return nil
			}
			return emitRFC(bps)
		})
	},
}

var rfcWatchCmd = &cobra.Command{
	Use:   "watch",
	Short: "Watch for debuggees hitting a breakpoint, and report each one as it appears",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		user, _ := cmd.Flags().GetString("user")
		every, _ := cmd.Flags().GetInt("interval")
		forSecs, _ := cmd.Flags().GetInt("for")
		return withRFC(cmd, func(ctx context.Context, c *rfc.Client) error {
			return saprfc.WatchDebuggees(ctx, c, user,
				time.Duration(every)*time.Second, time.Duration(forSecs)*time.Second,
				func(d saprfc.Debuggee) { fmt.Println(d.Text()) })
		})
	},
}

var rfcSearchCmd = &cobra.Command{
	Use:   "search <pattern>",
	Short: "Find RFC-enabled function modules (name mask, * wildcard)",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		like := strings.ReplaceAll(strings.ToUpper(args[0]), "*", "%")
		if !strings.Contains(like, "%") {
			like = "%" + like + "%"
		}
		where := "FUNCNAME LIKE '" + like + "'"
		if all, _ := cmd.Flags().GetBool("all"); !all {
			// TFDIR-FMODE has two remote values: 'R' is a remote-enabled module
			// and 'X' a remote-enabled module whose interface is basXML-capable,
			// which SAP sets on every FM with deep/nested parameters —
			// SADT_REST_RFC_ENDPOINT among them. Filtering on 'R' alone hides
			// them and makes a callable module look local.
			where += " AND FMODE IN ( 'R', 'X' )"
		}
		top, _ := cmd.Flags().GetInt("top")
		return withRFC(cmd, func(ctx context.Context, c *rfc.Client) error {
			rows, err := saprfc.ReadTable(ctx, c, "TFDIR", where, []string{"FUNCNAME", "PNAME"}, top)
			if err != nil {
				return err
			}
			return emitRFC(rows)
		})
	},
}

var rfcReadTableCmd = &cobra.Command{
	Use:   "read-table <table>",
	Short: "Read a table over RFC (RFC_READ_TABLE)",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		where, _ := cmd.Flags().GetString("where")
		if err := rfcFreeSQLGate(cmd, where); err != nil {
			return err
		}
		top, _ := cmd.Flags().GetInt("top")
		var fields []string
		if f, _ := cmd.Flags().GetString("fields"); f != "" {
			for _, x := range strings.Split(f, ",") {
				if x = strings.TrimSpace(x); x != "" {
					fields = append(fields, strings.ToUpper(x))
				}
			}
		}
		return withRFC(cmd, func(ctx context.Context, c *rfc.Client) error {
			rows, err := saprfc.ReadTable(ctx, c, strings.ToUpper(args[0]), where, fields, top)
			if err != nil {
				return err
			}
			return emitRFC(rows)
		})
	},
}

// rfcWriteGate refuses an rfc subcommand that can change the system — calling
// a function module, running a report, a non-GET ADT request — when the
// selected system is read-only: read_only in .vsp.json, or SAP_READ_ONLY. It
// is the same workflow-operation check the MCP server applies to rfc call and
// RunReport, made before any logon to the gateway.
func rfcWriteGate(cmd *cobra.Command, opName string) error {
	params, err := resolveSystemParams(cmd)
	if err != nil {
		return err
	}
	return cliWorkflowGate(cliReadOnly(params), opName)
}

// rfcFreeSQLGate refuses a caller's WHERE clause for RFC_READ_TABLE when the
// system blocks free SQL (block_free_sql in .vsp.json, or SAP_BLOCK_FREE_SQL):
// it is a free query on any table, as the MCP server's read_table treats it.
// A read without one, and search's own TFDIR filter, are not affected.
func rfcFreeSQLGate(cmd *cobra.Command, where string) error {
	if strings.TrimSpace(where) == "" {
		return nil
	}
	params, err := resolveSystemParams(cmd)
	if err != nil {
		return err
	}
	safety := adt.SafetyConfig{BlockFreeSQL: params.BlockFreeSQL} // block_free_sql or SAP_BLOCK_FREE_SQL
	return safety.CheckOperation(adt.OpFreeSQL, "RFCReadTable")
}

// cliReadOnly says whether the selected system is read-only for the CLI:
// read_only in .vsp.json, or SAP_READ_ONLY.
func cliReadOnly(params *systemParams) bool {
	return params.ReadOnly || envFlag("SAP_READ_ONLY")
}

// cliWorkflowGate is the workflow-operation check the MCP server applies to
// rfc call and RunReport, for a CLI command on a system that may be read-only.
func cliWorkflowGate(readOnly bool, opName string) error {
	safety := adt.SafetyConfig{ReadOnly: readOnly}
	return safety.CheckOperation(adt.OpWorkflow, opName)
}

// rfcADTReadMethod says whether an ADT request through the RFC tunnel only
// reads. Anything else may write, and is gated.
func rfcADTReadMethod(method string) bool {
	switch strings.ToUpper(strings.TrimSpace(method)) {
	case "GET", "HEAD", "OPTIONS":
		return true
	}
	return false
}

// withRFC resolves the RFC destination for the selected system and runs fn.
func withRFC(cmd *cobra.Command, fn func(context.Context, *rfc.Client) error) error {
	return withRFCTimeout(cmd, 0, fn)
}

// withRFCTimeout is withRFC for commands that make a call which blocks
// server-side on purpose: the client must not give up before the server does.
func withRFCTimeout(cmd *cobra.Command, timeout time.Duration, fn func(context.Context, *rfc.Client) error) error {
	return withRFCDestTimeout(cmd, timeout, func(ctx context.Context, c *rfc.Client, _ saprfc.Params) error {
		return fn(ctx, c)
	})
}

// withRFCDest is withRFC for callers that also need the resolved destination.
func withRFCDest(cmd *cobra.Command, fn func(context.Context, *rfc.Client, saprfc.Params) error) error {
	return withRFCDestTimeout(cmd, 0, fn)
}

func withRFCDestTimeout(cmd *cobra.Command, timeout time.Duration, fn func(context.Context, *rfc.Client, saprfc.Params) error) error {
	dest, err := rfcDestinationFor(cmd)
	if err != nil {
		return err
	}
	ctx := context.Background()
	c, err := saprfc.OpenWithTimeout(ctx, dest, timeout)
	if err != nil {
		return fmt.Errorf("RFC logon to %s:%d failed: %w", dest.Host, dest.Port, err)
	}
	defer c.Close(ctx)
	return fn(ctx, c, dest)
}

// rfcDestinationFor resolves where RFC calls from this command go: the system's
// settings, its .vsp.json RFC section, the environment, and any flag override.
// It is separate from the dialling so that callers which are not RFC commands —
// the Lua engine, which only wants a destination if a script asks to debug — can
// use the same resolution without inheriting the RFC flag set.
func rfcDestinationFor(cmd *cobra.Command) (saprfc.Params, error) {
	params, err := resolveSystemParams(cmd)
	if err != nil {
		return saprfc.Params{}, err
	}
	in := saprfc.Input{
		URL: params.URL, User: params.User, Password: params.Password,
		Client: params.Client, Language: params.Language,
	}
	// Per-system RFC settings, when the system came from .vsp.json.
	if params.Name != "" {
		if cfg, _, cerr := config.LoadSystems(); cerr == nil && cfg != nil {
			if sys, serr := cfg.GetSystem(params.Name); serr == nil {
				in.RFCHost, in.RFCSysnr, in.RFCPort = sys.RFCHost, sys.RFCSysnr, sys.RFCPort
				in.RFCUser, in.RFCPassword = sys.RFCUser, sys.RFCPassword
			}
		}
	} else {
		in.RFCUser, in.RFCPassword = os.Getenv("SAP_USER"), os.Getenv("SAP_PASSWORD")
	}
	in.HostFlag, _ = cmd.Flags().GetString("rfc-host")
	in.SysnrFlag, _ = cmd.Flags().GetString("sysnr")
	in.PortFlag, _ = cmd.Flags().GetInt("port")
	in.UserFlag, _ = cmd.Flags().GetString("rfc-user")
	// The identity pin travels with the destination: Open refuses an RFC
	// user or client that contradicts it before dialling, and a gateway of
	// another system after logon.
	if in.Expect, err = params.pin(); err != nil {
		return saprfc.Params{}, err
	}

	dest, err := saprfc.Resolve(in)
	if err != nil {
		return saprfc.Params{}, err
	}
	if verbose, _ := cmd.Flags().GetBool("verbose"); verbose {
		fmt.Fprintf(os.Stderr, "[INFO] RFC %s:%d (sysnr %s) client %s user %s\n", dest.Host, dest.Port, dest.Sysnr, dest.Client, dest.User)
	}
	return dest, nil
}

func emitRFC(v any) error {
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	fmt.Println(string(b))
	return nil
}

func readAllStdin() ([]byte, error) {
	var b []byte
	buf := make([]byte, 4096)
	for {
		n, err := os.Stdin.Read(buf)
		b = append(b, buf[:n]...)
		if err != nil {
			if err.Error() == "EOF" {
				return b, nil
			}
			return b, nil
		}
	}
}

func init() {
	rfcCmd.PersistentFlags().String("rfc-host", "", "RFC gateway host (default: host from the system URL)")
	rfcCmd.PersistentFlags().String("sysnr", "", "SAP system number, 00..99 (default: derived from the URL port)")
	rfcCmd.PersistentFlags().Int("port", 0, "RFC gateway port (default: 3300 + system number)")
	rfcCmd.PersistentFlags().String("rfc-user", "", "RFC logon user (default: rfc_user / SAP_USER / the system's user)")

	rfcADTCmd.Flags().StringArrayP("header", "H", nil, "Request header NAME=VALUE (repeatable)")
	rfcADTCmd.Flags().String("body", "", "Read the request body from a file")
	rfcADTCmd.Flags().StringP("output", "o", "", "Write the response body here instead of stdout")

	rfcCallCmd.Flags().String("file", "", "read JSON parameters from a file")
	rfcCallCmd.Flags().Bool("stdin", false, "read JSON parameters from stdin")
	rfcSearchCmd.Flags().Bool("all", false, "include function modules that are not RFC-enabled")
	rfcSearchCmd.Flags().Int("top", 100, "maximum rows")
	rfcReadTableCmd.Flags().String("where", "", "WHERE clause (refused when the system blocks free SQL)")
	rfcReadTableCmd.Flags().String("fields", "", "comma-separated column list")
	rfcReadTableCmd.Flags().Int("top", 0, "maximum rows (0 = all)")

	rfcProbeCmd.Flags().String("format", "text", "Output format: text or json")
	rfcExportCmd.Flags().StringP("output", "o", "", "Write the ZIP here (default: <package>.zip)")
	rfcExportCmd.Flags().String("folder-logic", "", "abapGit folder logic: FULL or PREFIX")
	rfcExportCmd.Flags().Bool("main-lang-only", false, "Serialize the main language only")
	rfcRunCmd.Flags().StringArrayP("param", "p", nil, "Selection parameter NAME=VALUE (repeatable)")
	rfcRunCmd.Flags().String("job-name", "", "Background job name (default: VSP_<REPORT>)")
	rfcRunCmd.Flags().String("variant", "", "Run with this saved variant; -p values override it")
	rfcRunCmd.Flags().Int("wait", 0, "Seconds to wait for the job to finish (0 = do not wait)")
	rfcRunCmd.Flags().Bool("spool", false, "Fetch the spool list once the job has finished")
	rfcSpoolCmd.Flags().Int("step", 1, "Job step number")
	rfcDebuggeesCmd.Flags().String("user", "", "Only this user's debuggees (default: everyone)")
	rfcDebuggeesCmd.Flags().Bool("json", false, "Emit JSON instead of one line per debuggee")
	rfcBreakpointsCmd.Flags().String("user", "", "Whose breakpoints to list (default: everyone)")
	rfcWatchCmd.Flags().String("user", "", "Only this user's debuggees (default: everyone)")
	rfcWatchCmd.Flags().Int("interval", 2, "Seconds between polls")
	rfcWatchCmd.Flags().Int("for", 0, "Stop after this many seconds (0 = until interrupted)")
	rfcCmd.AddCommand(rfcInfoCmd, rfcPingCmd, rfcProbeCmd, rfcADTCmd, rfcExportCmd, rfcRunCmd, rfcSpoolCmd, rfcDescribeCmd, rfcCallCmd, rfcSearchCmd, rfcReadTableCmd,
		rfcDebuggeesCmd, rfcBreakpointsCmd, rfcWatchCmd)
	rootCmd.AddCommand(rfcCmd)
}
