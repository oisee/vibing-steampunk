package main

import (
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/oisee/vibing-steampunk/internal/mcp"
	"github.com/oisee/vibing-steampunk/pkg/adt"
	"github.com/oisee/vibing-steampunk/pkg/config"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

// expectFlag is --expect: SID[.CLIENT][/USER] the connection must land on.
var expectFlag string

// shellEnv records which logon variables the shell itself set, before .env is
// loaded on top: "from SAP_USER" says nothing useful when the question is
// whether a stale shell or the project's .env supplied it.
var shellEnv = map[string]bool{}

func init() {
	for _, k := range []string{"SAP_USER", "SAP_USERNAME", "SAP_EXPECT"} {
		if _, ok := os.LookupEnv(k); ok {
			shellEnv[k] = true
		}
	}
	rootCmd.PersistentFlags().StringVar(&expectFlag, "expect", "",
		"Pin the connection: SID[.CLIENT][/USER], e.g. A4H.001/DEVELOPER. vsp refuses to log on when the configured user or client differs, and refuses all work when the system reports another SID, client or user on the first request (also SAP_EXPECT, or \"expect\" in .vsp.json)")
}

// envSource names where an environment variable came from.
func envSource(key string) string {
	if shellEnv[key] {
		return key + " (shell environment)"
	}
	return key + " (.env file)"
}

// pinSpec is a pin as written, with where it was written.
type pinSpec struct {
	Raw    string
	Source string
}

// resolveServerExpect finds the MCP server's pin: --expect, else SAP_EXPECT,
// else "expect" of the server's own system in .vsp.json (-s / SAP_SYSTEM, or
// the default system).
func resolveServerExpect(systemName string, systemsCfg *config.SystemsConfig) pinSpec {
	if v := strings.TrimSpace(expectFlag); v != "" {
		return pinSpec{v, "--expect"}
	}
	if v := strings.TrimSpace(os.Getenv("SAP_EXPECT")); v != "" {
		return pinSpec{v, envSource("SAP_EXPECT")}
	}
	if systemsCfg != nil {
		name := systemName
		if name == "" {
			name = systemsCfg.Default
		}
		if sys, ok := systemsCfg.Systems[name]; ok && strings.TrimSpace(sys.Expect) != "" {
			return pinSpec{strings.TrimSpace(sys.Expect), fmt.Sprintf(".vsp.json system %q", name)}
		} else if ok && sys.SNC != nil {
			return pinSpec{sys.SNC.DefaultExpect(), fmt.Sprintf(".vsp.json system %q (snc)", name)}
		}
	}
	return pinSpec{}
}

// parse turns the spec into a pin; nil without one.
func (p pinSpec) parse() (*adt.IdentityPin, error) {
	if p.Raw == "" {
		return nil, nil
	}
	pin, err := adt.ParseIdentityPin(p.Raw)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", p.Source, err)
	}
	return pin, nil
}

// applyServerExpect resolves the server's pin into c and checks it against
// the configured logon, before any credential is sent anywhere.
//
// interactive is a browser, SAML or SSO logon: whatever user name was also
// configured is not the one that logs on, so only the client is compared.
func applyServerExpect(c *mcp.Config, systemsCfg *config.SystemsConfig, userFlag, interactive bool) error {
	spec := resolveServerExpect(c.SystemName, systemsCfg)
	pin, err := spec.parse()
	if err != nil || pin == nil {
		return err
	}
	user := c.Username
	if interactive {
		user = ""
	}
	if err := pin.CheckConfigured(user, c.Client); err != nil {
		return fmt.Errorf("%w (pin from %s, logon from %s)", err, spec.Source, serverUserSource(c, userFlag))
	}
	c.Expect, c.ExpectSource = pin, spec.Source
	return nil
}

// serverUserSource says where the server's user name came from. The server
// takes its logon from flags and SAP_* only, never from .vsp.json.
// userFlag is whether --user was given.
func serverUserSource(c *mcp.Config, userFlag bool) string {
	switch {
	case c.Username == "" && len(c.TransportCmd) > 0:
		return "no user name (the transport command authenticates)"
	case c.Username == "":
		return "no user name (cookie or single sign-on session)"
	case userFlag:
		return "--user"
	case os.Getenv("SAP_USER") != "":
		return envSource("SAP_USER")
	default:
		return envSource("SAP_USERNAME")
	}
}

// logServerLogon says, once at startup, whose logon the server will use and
// where it came from. A server whose user came from a stale shell used to say
// nothing until SAP answered 401.
func logServerLogon(w io.Writer, c *mcp.Config, systemsCfg *config.SystemsConfig, userFlag bool) {
	who := "a cookie or single sign-on session"
	if len(c.TransportCmd) > 0 {
		who = "transport command " + adt.TransportCmdName(c.TransportCmd)
	} else if c.Username != "" {
		who = "user " + strings.ToUpper(c.Username)
	}
	fmt.Fprintf(w, "[vsp] SAP logon: %s from %s, client %s, %s\n", who, serverUserSource(c, userFlag), c.Client, c.BaseURL)
	if c.Expect != nil {
		fmt.Fprintf(w, "[vsp] pinned to %s (%s); checked before the first request\n", c.Expect, c.ExpectSource)
	}
	if systemsCfg == nil || c.Username == "" {
		return
	}
	name := c.SystemName
	if name == "" {
		name = systemsCfg.Default
	}
	if sys, ok := systemsCfg.Systems[name]; ok && sys.User != "" && !strings.EqualFold(sys.User, c.Username) {
		fmt.Fprintf(w, "[vsp] note: the MCP server takes its logon from flags and SAP_* only; .vsp.json system %q (user %s) is not used for it\n",
			name, strings.ToUpper(sys.User))
	}
}

// cliExpect finds a CLI command's pin: --expect, else the system's own
// "expect" in .vsp.json, else SAP_EXPECT.
func cliExpect(sysExpect string) (string, string) {
	if v := strings.TrimSpace(expectFlag); v != "" {
		return v, "--expect"
	}
	if v := strings.TrimSpace(sysExpect); v != "" {
		return v, ".vsp.json"
	}
	if v := strings.TrimSpace(os.Getenv("SAP_EXPECT")); v != "" {
		return v, envSource("SAP_EXPECT")
	}
	return "", ""
}

// cliPinOption turns the system's pin into a client option, refusing at once
// (and so before any logon) when the configured user or client differs.
func cliPinOption(params *systemParams, basicUser string) (adt.Option, error) {
	pin, err := params.pin()
	if err != nil || pin == nil {
		return nil, err
	}
	if err := pin.CheckConfigured(basicUser, params.Client); err != nil {
		return nil, fmt.Errorf("%w (pin from %s)", err, params.ExpectSource)
	}
	return adt.WithExpect(*pin), nil
}

// printLogonSources says, for `vsp config show`, where each mode takes its
// logon from. The two differ, and that difference is how a stale shell
// environment once logged an HTTP MCP server on as the wrong user: the server
// reads flags and SAP_* only, the CLI reads the .vsp.json system first.
func printLogonSources(w io.Writer, systemsCfg *config.SystemsConfig) {
	fmt.Fprintln(w, "\nLogon sources:")
	envUser := "none"
	switch {
	case os.Getenv("SAP_USER") != "":
		envUser = strings.ToUpper(os.Getenv("SAP_USER")) + " from " + envSource("SAP_USER")
	case os.Getenv("SAP_USERNAME") != "":
		envUser = strings.ToUpper(os.Getenv("SAP_USERNAME")) + " from " + envSource("SAP_USERNAME")
	}
	fmt.Fprintln(w, "  MCP server (vsp, no subcommand): --user/--password, else SAP_USER/SAP_PASSWORD; .vsp.json is not used for its logon")
	fmt.Fprintf(w, "    user now: %s\n", envUser)
	cli := "SAP_* environment (no .vsp.json default system)"
	if systemsCfg != nil && systemsCfg.Default != "" {
		user := "-"
		if sys, ok := systemsCfg.Systems[systemsCfg.Default]; ok && sys.User != "" {
			user = strings.ToUpper(sys.User)
		}
		cli = fmt.Sprintf(".vsp.json default system %q (user %s); -s picks another", systemsCfg.Default, user)
	}
	fmt.Fprintf(w, "  CLI subcommands: %s\n", cli)
	pin := "none"
	if v := strings.TrimSpace(os.Getenv("SAP_EXPECT")); v != "" {
		pin = strings.ToUpper(v) + " from " + envSource("SAP_EXPECT")
	}
	fmt.Fprintf(w, "  Identity pin from the environment (--expect and per-system \"expect\" also apply): %s\n", pin)
}

// pin parses the system's identity pin; nil without one.
func (p *systemParams) pin() (*adt.IdentityPin, error) {
	if p.Expect == "" {
		return nil, nil
	}
	pin, err := adt.ParseIdentityPin(p.Expect)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", p.ExpectSource, err)
	}
	return pin, nil
}

// interactiveLogon reports a logon flow that runs before the server starts
// and talks to SAP or an identity provider on its own: browser, SAML or SSO.
func interactiveLogon(cmd *cobra.Command) bool {
	browser, _ := cmd.Flags().GetBool("browser-auth")
	saml, _ := cmd.Flags().GetBool("saml-auth")
	return browser || saml || viper.GetBool("BROWSER_AUTH") || viper.GetBool("SAML_AUTH") || ssoRequested(cmd)
}
