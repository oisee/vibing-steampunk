package vsp

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/oisee/vibing-steampunk/pkg/adt"
	"github.com/oisee/vibing-steampunk/pkg/saprfc"
	"github.com/spf13/cobra"
)

// `vsp adt debug` is `vsp rfc debug` for systems that have no RFC channel — a
// cookie or a single sign-on, HTTPS only, no gateway port and no RFC password.
//
// It works because the debugger was never an RFC feature: listen, attach, stack
// and step are SAP's own ADT resources, and RFC was one way to carry them. What
// they actually need is a *session* — ADT keeps the debug session in an ABAP
// roll area — and over HTTPS that is the stateful ICF session selected by
// sap-contextid. So the requirement here is the same as there, expressed
// differently: one process, one session, held for the whole loop.

var adtCmd = &cobra.Command{
	Use:   "adt",
	Short: "Talk to ADT directly over HTTPS",
}

var adtDebugCmd = &cobra.Command{
	Use:   "debug",
	Short: "Drive the ABAP debugger over a stateful ADT session (no RFC needed)",
	Long: `Drive the ABAP debugger over ADT's own resources on one stateful HTTPS session.

The same commands as 'vsp rfc debug', minus the ones that need the ZADT_DEBUG
function group — those are function modules and need an RFC channel:

  eclipse [SECONDS]  listen, attach to the first debuggee, show the stack
  estep [KIND]       into (default) | over | out | continue
  estack             the call stack
  ebp <OBJECT> <LINE> [COND]
                     set a line breakpoint through ADT — no Z code needed
  ebps               the breakpoints this client has registered
  esys               toggle breakpoints inside SAP standard code (off by default)
  eunbp <ID|all>     remove one breakpoint, or all of them
  elocals            the current frame's own variables, with values
  evars [NAME …]     variable values (default roots @ROOT @DATAAGING)
  echildren <ID>     expand a structure, a table or a synthetic root
  eset <NAME> <VALUE>  overwrite a variable in the stopped frame
  eframe <N|STACK-URI> move the cursor to another frame, by number or URI
  astart [USER]      start an AMDP debug session (ADT's own, no Z code)
  abp <CLASS> <LINE>   AMDP breakpoint, after astart
  aresume [MAX]      wait for the AMDP debuggee to stop, skipping acknowledgements
  astep [over|continue] step the stopped AMDP debuggee
  atrace [MAX]       walk the stopped AMDP program, one JSON object per line
  astack             the stopped procedure's call stack, ABAP and native lines
  alocals [all]      everything in scope at the stop, from the stop itself
  avar <NAME>        read a variable of the stopped SQLScript
  astop              end the AMDP session
  erec [MAX]         record from here: one JSON object per stop
  evalues            record real values instead of «type:length» placeholders
  eraw               print the next e-command as the XML SAP sent
  adt <METHOD> <URI> [NAME=VALUE …] [@bodyfile]
                     any ADT request on this same session`,
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
		transport, err := statefulADTTransport(params, time.Duration(timeout)*time.Second)
		if err != nil {
			return err
		}

		// A cassette turns this run into a test that needs no system. The
		// recorder sits between the debugger and the wire, so what is captured
		// is exactly what the debugger asked and exactly what SAP answered —
		// nobody gets to write the answers by hand afterwards.
		cassettePath, _ := cmd.Flags().GetString("record")
		var recorder *saprfc.RecordingTransport
		if cassettePath != "" {
			recorder = saprfc.Recorder(transport)
			// A cassette is a tracked fixture, so nothing that names a live
			// account or box may reach it.
			recorder.Redact = map[string]string{}
			transport = recorder
			name, _ := cmd.Flags().GetString("record-system")
			if name == "" {
				name, _ = cmd.Flags().GetString("system")
			}
			defer func() {
				if rfcDebugUser != "" && rfcDebugUser != "TESTUSER" {
					recorder.Redact[rfcDebugUser] = "TESTUSER"
				}
				meta := saprfc.Cassette{System: name}
				if err := recorder.Save(cassettePath, meta); err != nil {
					fmt.Fprintf(os.Stderr, "! cassette not written: %v\n", err)
					return
				}
				fmt.Fprintf(os.Stderr, "%d exchanges recorded to %s\n", recorder.Count(), cassettePath)
			}()
		}

		ctx := context.Background()

		rfcDebugUser = strings.ToUpper(strings.TrimSpace(user))
		if rfcDebugUser == "" {
			rfcDebugUser = strings.ToUpper(params.User)
		}
		if rfcDebugUser == "" {
			// Under single sign-on there is no configured user — the cookie
			// carries a session, not a name — and the debugger cannot register
			// a listener without one. Ask the system instead of asking the
			// person: the transport organizer names the owner of the session,
			// over plain ADT, even when they own nothing.
			resolved, werr := saprfc.CurrentUser(ctx, transport)
			if werr != nil {
				return fmt.Errorf("nobody said whose debuggees to listen for, and the system would not say either (%w): pass --user", werr)
			}
			rfcDebugUser = resolved
			fmt.Fprintf(os.Stderr, "logged on as %s\n", rfcDebugUser)
		}

		dbg := saprfc.NewADTDebugger(transport, rfcDebugUser)
		defer func() { _ = dbg.Close(ctx) }()

		// A deferred close does not run when the process is signalled, and an
		// interrupted debug session is the common case rather than the rare
		// one: a -c script that hangs gets Ctrl-C or a timeout kill. What it
		// leaves behind is not tidy-up work — an AMDP session holds a debug
		// work process from a pool shared across the whole system, and
		// stopExisting only reaches your own user, so nobody else can release
		// it. One interrupted run cost the next debugger on that system five
		// minutes of DEBUGGER_NO_MORE_DBG_WPS.
		stop := make(chan os.Signal, 1)
		signal.Notify(stop, os.Interrupt, syscall.SIGTERM)
		defer signal.Stop(stop)
		go func() {
			sig, ok := <-stop
			if !ok {
				return
			}
			// The session's own context may already be cancelled, and the
			// cleanup still has to reach the server, so it gets a fresh one
			// with a bound short enough not to hang the exit.
			cleanup, cancel := context.WithTimeout(context.Background(), 20*time.Second)
			defer cancel()
			fmt.Fprintf(os.Stderr, "\n%v — releasing the debug session before exit\n", sig)
			_ = dbg.Close(cleanup)
			os.Exit(130)
		}()

		if script != "" {
			for _, line := range strings.Split(script, ";") {
				if err := runDebugCommand(ctx, dbg, strings.TrimSpace(line)); err != nil {
					return err
				}
			}
			return nil
		}

		fmt.Fprintf(os.Stderr, "ADT debug session as %s — 'help' for commands, 'quit' to end\n", rfcDebugUser)
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
	},
}

// statefulADTTransport builds one ADT transport and keeps it: a new transport
// is a new session, and a new session has no debuggee attached.
func statefulADTTransport(params *systemParams, timeout time.Duration) (saprfc.ADTTransport, error) {
	t, err := debugHTTPTransport(params, timeout, adt.SessionStateful)
	if err != nil {
		return nil, err
	}
	return saprfc.HTTPSession(t), nil
}

// statelessADTTransport builds a transport of the same logon that belongs to
// no session: a separate connection for the requests the debug session cannot
// carry while a request is still open on it (see saprfc.HTTPStateless).
func statelessADTTransport(params *systemParams, timeout time.Duration) (saprfc.ADTTransport, error) {
	t, err := debugHTTPTransport(params, timeout, adt.SessionStateless)
	if err != nil {
		return nil, err
	}
	return saprfc.HTTPStateless(t), nil
}

// withoutContext drops a stored sap-contextid: it selects a stateful session,
// which is exactly what a stateless side connection must not join.
func withoutContext(cookies map[string]string) map[string]string {
	out := make(map[string]string, len(cookies))
	for k, v := range cookies {
		if strings.EqualFold(k, "sap-contextid") {
			continue
		}
		out[k] = v
	}
	return out
}

// ssoAuthOptions are the single sign-on cookies and the refresh hook, both
// passed through filter. The refresh matters as much as the first set: on a
// re-authentication the transport replaces its cookies with whatever the
// refresh returns, so an unfiltered refresh would hand a stateless side
// connection the sap-contextid it must never send.
func ssoAuthOptions(cookies map[string]string, refresh func(context.Context) (map[string]string, error),
	budget time.Duration, filter func(map[string]string) map[string]string) []adt.Option {
	return []adt.Option{
		adt.WithCookies(filter(cookies)),
		adt.WithReauthFunc(func(ctx context.Context) (map[string]string, error) {
			fresh, err := refresh(ctx)
			if err != nil {
				return nil, err
			}
			return filter(fresh), nil
		}),
		adt.WithReauthTimeout(budget),
	}
}

// debugHTTPTransport builds the HTTP transport the debugger's ADT requests go
// over, in the given session type.
func debugHTTPTransport(params *systemParams, timeout time.Duration, session adt.SessionType) (*adt.Transport, error) {
	cookieFilter := func(c map[string]string) map[string]string { return c }
	if session != adt.SessionStateful {
		cookieFilter = withoutContext
	}
	opts := []adt.Option{
		adt.WithClient(params.Client),
		adt.WithLanguage(params.Language),
		adt.WithSessionType(session),
		// The debugger's listener is a request that deliberately does not answer
		// until something stops, so the client timeout has to outlast it. The
		// stock 60s turns a 90s listen into "context deadline exceeded" and the
		// caller never learns that the debuggee was fine.
		adt.WithTimeout(timeout),
	}
	if params.Insecure {
		opts = append(opts, adt.WithInsecureSkipVerify())
	}
	// The identity pin, as on every other client built from these params.
	basicUser := params.User
	if params.UsesSSO() || params.CookieFile != "" || params.CookieString != "" {
		basicUser = ""
	}
	pinOpt, err := cliPinOption(params, basicUser)
	if err != nil {
		return nil, err
	}
	if pinOpt != nil {
		opts = append(opts, pinOpt)
	}

	// Browser single sign-on, checked before the static cookie sources for the
	// same reason as everywhere else — and needed here most of all. A debug
	// session is the longest-lived thing vsp holds: a listener waits minutes,
	// and a session outlives the cookie that opened it. Without the refresh
	// hook the loop dies mid-debug, and it dies quietly, because an expired
	// SSO answers 200 with a sign-in page rather than 401.
	if params.UsesSSO() {
		provider, err := newSSOProvider(params)
		if err != nil {
			return nil, err
		}
		cookies, err := provider.Cookies(context.Background())
		if err != nil {
			return nil, err
		}
		opts = append(opts, ssoAuthOptions(cookies, provider.Refresh, provider.ReauthBudget(), cookieFilter)...)
		cfg := adt.NewConfig(params.URL, "", "", opts...)
		return adt.NewTransport(cfg), nil
	}

	user, password := params.User, params.Password
	switch {
	case params.CookieFile != "":
		cookies, err := adt.LoadCookiesFromFile(params.CookieFile)
		if err != nil {
			return nil, fmt.Errorf("loading cookies from %s: %w", params.CookieFile, err)
		}
		opts = append(opts, adt.WithCookies(cookieFilter(cookies)))
		user, password = "", ""
	case params.CookieString != "":
		opts = append(opts, adt.WithCookies(cookieFilter(adt.ParseCookieString(params.CookieString))))
		user, password = "", ""
	}

	cfg := adt.NewConfig(params.URL, user, password, opts...)
	return adt.NewTransport(cfg), nil
}

func init() {
	adtDebugCmd.Flags().String("user", "", "Whose debuggees to listen for (default: the logon user)")
	adtDebugCmd.Flags().StringP("command", "c", "", "Run a semicolon-separated script instead of going interactive")
	adtDebugCmd.Flags().Int("timeout", 300, "Seconds a single HTTP request may take; must exceed the listen timeout")
	adtDebugCmd.Flags().String("record", "", "Write every ADT exchange of this session to a cassette file, for replay in tests")
	adtDebugCmd.Flags().String("record-system", "", "Name the system the cassette came from (default: the --system name)")
	adtCmd.AddCommand(adtDebugCmd)
	rootCmd.AddCommand(adtCmd)
}
