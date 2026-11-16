package main

// A transport command: a helper process that carries every ADT request over
// its stdin/stdout (see pkg/adt/stdio_transport.go). The helper authenticates,
// so it is exclusive with every other way of logging on.

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"sync"

	"github.com/oisee/vibing-steampunk/internal/mcp"
	"github.com/oisee/vibing-steampunk/pkg/adt"
	"github.com/oisee/vibing-steampunk/pkg/config"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

// transportCmdShellEnv is SAP_TRANSPORT_CMD as the process environment had it,
// read by a package variable initialiser so that it runs before any init()
// and so before .env is loaded. A transport command runs a program, and a
// .env in the working directory belongs to the project, not to the user:
// SAP_TRANSPORT_CMD is taken from the real environment only.
var transportCmdShellEnv, transportCmdInShellEnv = os.LookupEnv("SAP_TRANSPORT_CMD")

// startupEnviron is the environment before init loads ./.env (package
// variables are set before any init runs). A transport command starts from
// it: a project's .env must not reach the helper, e.g. with LD_PRELOAD.
var startupEnviron = os.Environ()

// transportCmdErr is why the transport command could not be resolved;
// resolveConfig has no error return, so the first auth step reports it.
var transportCmdErr error

func init() {
	rootCmd.Flags().String("transport-cmd", "", "Helper program that carries every ADT request over its stdin/stdout instead of a TCP connection (it authenticates; no user/password/cookies). --url still names the system, e.g. https://sidecar.invalid. Also SAP_TRANSPORT_CMD as a JSON array")
	rootCmd.Flags().StringArray("transport-arg", nil, "Argument for --transport-cmd (repeatable, passed as is, no shell)")
}

// resolveTransportCmd reads --transport-cmd plus --transport-arg, else
// SAP_TRANSPORT_CMD (a JSON array of strings, from the real environment only).
func resolveTransportCmd(cmd *cobra.Command) ([]string, error) {
	var exe string
	var args []string
	if f := cmd.Flags().Lookup("transport-cmd"); f != nil {
		exe = strings.TrimSpace(f.Value.String())
	}
	if f := cmd.Flags().Lookup("transport-arg"); f != nil {
		args, _ = cmd.Flags().GetStringArray("transport-arg")
	}
	if exe != "" {
		return append([]string{exe}, args...), nil
	}
	if len(args) > 0 {
		return nil, fmt.Errorf("--transport-arg needs --transport-cmd")
	}
	raw := os.Getenv("SAP_TRANSPORT_CMD")
	if strings.TrimSpace(raw) == "" {
		return nil, nil
	}
	if !transportCmdInShellEnv || transportCmdShellEnv != raw {
		return nil, fmt.Errorf("SAP_TRANSPORT_CMD is refused from a .env file: a project file must not be able to make vsp run a program; set it in the environment of the vsp process, or pass --transport-cmd")
	}
	return parseTransportCmdEnv(raw)
}

// parseTransportCmdEnv parses SAP_TRANSPORT_CMD: a JSON array of non-empty
// strings, the program first.
func parseTransportCmdEnv(raw string) ([]string, error) {
	const want = `SAP_TRANSPORT_CMD must be a JSON array of non-empty strings, the program first, e.g. ["/usr/local/bin/adt-helper","--verbose"]`
	var argv []string
	if err := json.Unmarshal([]byte(raw), &argv); err != nil {
		return nil, fmt.Errorf("%s: %v", want, err)
	}
	if len(argv) == 0 {
		return nil, fmt.Errorf("%s: the array is empty", want)
	}
	for i, a := range argv {
		if strings.TrimSpace(a) == "" {
			return nil, fmt.Errorf("%s: element %d is empty", want, i)
		}
	}
	return argv, nil
}

// transportCmdConflicts refuses a transport command alongside any credential
// or logon flow of vsp's own: the helper authenticates, and vsp sends nothing
// of its own. Nil without a transport command.
func transportCmdConflicts(cmd *cobra.Command) error {
	if len(cfg.TransportCmd) == 0 {
		return nil
	}
	var found []string
	if cfg.Username != "" || cfg.Password != "" {
		found = append(found, "user/password")
	}
	flagOrEnv := func(flag, key string) bool {
		if f := cmd.Flags().Lookup(flag); f != nil && f.Changed && f.Value.String() != "" && f.Value.String() != "false" {
			return true
		}
		v := strings.TrimSpace(viper.GetString(key))
		return v != "" && !strings.EqualFold(v, "false")
	}
	if flagOrEnv("cookie-file", "COOKIE_FILE") {
		found = append(found, "cookie-file")
	}
	if flagOrEnv("cookie-string", "COOKIE_STRING") {
		found = append(found, "cookie-string")
	}
	if flagOrEnv("browser-auth", "BROWSER_AUTH") {
		found = append(found, "browser-auth")
	}
	if flagOrEnv("saml-auth", "SAML_AUTH") {
		found = append(found, "saml-auth")
	}
	if flagOrEnv("sso", "SSO") {
		found = append(found, "sso")
	}
	if len(cfg.Cookies) > 0 {
		found = append(found, "cookies")
	}
	if len(found) == 0 {
		return nil
	}
	return fmt.Errorf("%w (found: %s)", adt.ErrTransportCmdAuth, strings.Join(found, ", "))
}

// transportCmdClosers are the helpers the CLI started, one per ADT client or
// transport built with a transport command. main closes them when the command
// returns, and the signal exits close them before os.Exit, so a helper is
// told to exit (stdin EOF) rather than left to notice vsp has gone.
var transportCmdClosers struct {
	mu  sync.Mutex
	fns []func() error
}

// trackTransportCmd registers a helper's close function.
func trackTransportCmd(fn func() error) {
	transportCmdClosers.mu.Lock()
	defer transportCmdClosers.mu.Unlock()
	transportCmdClosers.fns = append(transportCmdClosers.fns, fn)
}

// closeTransportCmds closes every registered helper, at most once each.
func closeTransportCmds() {
	transportCmdClosers.mu.Lock()
	fns := transportCmdClosers.fns
	transportCmdClosers.fns = nil
	transportCmdClosers.mu.Unlock()
	for _, fn := range fns {
		_ = fn()
	}
}

// applyNamedTransportSystem gives the MCP server its logon from a system in
// the home-directory config that carries its own: an snc block or a
// transport_cmd. With -s NAME naming such a system, the server takes the
// transport command, URL, client and read-only from it (an snc system is
// always read-only), so .mcp.json needs only ["-s", "NAME"]. The system is
// checked as the CLI checks it: GetSystem refuses a transport command or an
// snc block that does not come from ~/.vsp.json or ~/.vsp/systems.json. A
// URL, user, password or transport command given as well is refused, not
// overridden.
//
// Only the -s flag counts, not SAP_SYSTEM: vsp loads ./.env into the
// environment, and a project must not choose which system the server logs on
// to, nor make it start a helper.
func applyNamedTransportSystem(cmd *cobra.Command, c *mcp.Config) error {
	if f := cmd.Flag("system"); f == nil || !f.Changed || systemName == "" || systemName != c.SystemName {
		return nil
	}
	systems, _, err := config.LoadSystems()
	if err != nil || systems == nil {
		return nil
	}
	raw, ok := systems.Systems[c.SystemName]
	if !ok || (raw.SNC == nil && len(raw.TransportCmd) == 0) {
		return nil
	}
	via := "transport_cmd"
	if raw.SNC != nil {
		via = "snc block"
	}
	var given []string
	if len(c.TransportCmd) > 0 {
		given = append(given, "a transport command")
	}
	if c.BaseURL != "" {
		given = append(given, "a URL (--url or SAP_URL)")
	}
	if c.Username != "" || c.Password != "" {
		given = append(given, "user/password (--user, --password or SAP_USER/SAP_PASSWORD)")
	}
	if len(given) > 0 {
		return fmt.Errorf("system %q logs on through its %s; remove %s", c.SystemName, via, strings.Join(given, ", "))
	}
	sys, err := systems.GetSystem(c.SystemName)
	if err != nil {
		return err
	}
	if len(sys.TransportCmd) == 0 {
		return fmt.Errorf("system %q: its %s gave no transport command", c.SystemName, via)
	}
	if sys.Client != "" {
		if cmd.Flags().Changed("client") && c.Client != sys.Client {
			return fmt.Errorf("system %q: --client %s differs from its client %s", c.SystemName, c.Client, sys.Client)
		}
		c.Client = sys.Client
	}
	c.TransportCmd = sys.TransportCmd
	c.BaseURL = sys.URL
	c.ReadOnly = c.ReadOnly || sys.ReadOnly
	return nil
}
