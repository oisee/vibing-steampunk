// vsp is an MCP server providing ABAP Development Tools (ADT) functionality.
package main

import (
	"context"
	"fmt"
	"io"
	"math"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/joho/godotenv"
	"github.com/oisee/vibing-steampunk/internal/mcp"
	"github.com/oisee/vibing-steampunk/pkg/adt"
	"github.com/oisee/vibing-steampunk/pkg/config"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

var (
	// Version information (set by build flags)
	Version   = "dev"
	Commit    = "unknown"
	BuildDate = "unknown"
	// ReleaseRepo is the owner/name of the GitHub repository this binary is
	// released from, set by build flags; empty means the default
	// (oisee/vibing-steampunk).
	ReleaseRepo = ""
)

var cfg = &mcp.Config{}

var rootCmd = &cobra.Command{
	Use:   "vsp",
	Short: "ABAP Development Tools for AI agents and DevOps",
	Long: `vsp — ABAP Development Tools for AI agents and DevOps.

Single binary, 6 platforms, no dependencies. Download from GitHub releases,
point your MCP config at it, done.

Two modes of operation:

  MCP Server (default)  Connects Claude, Gemini CLI, Copilot, Codex, Qwen Code,
                        and other MCP-compatible agents to SAP systems.
                        98 tools (focused), 148 (expert), or 1 universal tool (hyperfocused).

  CLI Mode              Direct terminal access: search, source, export, debug.
                        Multi-system profiles. Useful for scripts and pipelines.

Quick start:
  # 1. MCP server (reads .env or SAP_* env vars)
  vsp --url https://host:44300 --user dev --password secret

  # 2. CLI mode with saved system profile
  vsp -s dev search "ZCL_ORDER*"
  vsp -s dev source CLAS ZCL_ORDER_PROCESSING
  vsp -s dev export '$ZPACKAGE' -o backup.zip

  # 3. Enterprise safety (hand to AI without fear)
  vsp --read-only                                    # no writes at all
  vsp --allowed-packages 'Z*,$TMP' --block-free-sql  # sandbox AI to custom code
  vsp --disallowed-ops CDUA                           # block create/delete/update/activate

  # 4. Through a helper that signs on itself and carries each ADT request
  #    over its stdin/stdout (frames: 4-byte big-endian length + one raw
  #    HTTP/1.1 message; one at a time; exit on stdin EOF; logs to stderr only)
  vsp --url https://sidecar.invalid --transport-cmd /opt/tools/adt-helper

Configuration files:
  .env          Default SAP connection (MCP server mode). SAP_URL, SAP_USER, etc.
  .vsp.json     Multi-system profiles for CLI mode (vsp -s dev, vsp -s prod).
  .mcp.json     MCP server entries for Claude Desktop / other MCP clients.

  vsp config init       Generate example files (.env.example, .vsp.json.example, .mcp.json.example)
  vsp config show       Display effective configuration
  vsp config mcp-to-vsp Import systems from .mcp.json into .vsp.json
  vsp config vsp-to-mcp Export .vsp.json systems to .mcp.json format
  vsp config tools      Manage per-tool visibility in .vsp.json

Configuration priority: CLI flags > env vars > .env file > defaults
Ready-to-use configs for 8 AI agents: docs/cli-agents/`,
	Version: fmt.Sprintf("%s (commit: %s, built: %s)", Version, Commit, BuildDate),
	PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
		// Also check SAP_VERBOSE env var (viper reads it, but resolveConfig
		// is only called for the MCP server mode, so we check it here too)
		if !cfg.Verbose {
			cfg.Verbose = viper.GetBool("VERBOSE")
		}
		if cfg.Verbose {
			adt.SetLogOutput(os.Stderr)
		}
		return nil
	},
	PersistentPostRun: func(cmd *cobra.Command, args []string) {
		if cfg.Verbose && lastClient != nil {
			if s := lastClient.CacheStats(); s.Enabled {
				fmt.Fprintf(os.Stderr, "[cache] %d hits, %d misses, %d entries, %d invalidations, ttl %s\n", s.Hits, s.Misses, s.Entries, s.Invalidations, s.TTL)
			}
		}
	},
	RunE: runServer,
}

func init() {
	// Load .env file if it exists. Not for the hidden SNC commands: vsp starts
	// them itself, and their environment is the one it handed over.
	if !sncServeInvocation(os.Args) {
		_ = godotenv.Load()
	}
	adt.SetHelperEnviron(func() []string { return append([]string(nil), startupEnviron...) })

	// Service URL
	rootCmd.Flags().StringVar(&cfg.BaseURL, "url", "", "SAP system URL (e.g., https://host:44300)")
	rootCmd.Flags().StringVar(&cfg.BaseURL, "service", "", "SAP system URL (alias for --url)")

	// Authentication flags
	rootCmd.Flags().StringVarP(&cfg.Username, "user", "u", "", "SAP username")
	rootCmd.Flags().StringVarP(&cfg.Password, "password", "p", "", "SAP password")
	rootCmd.Flags().StringVar(&cfg.Password, "pass", "", "SAP password (alias for --password)")

	// SAP connection options
	rootCmd.Flags().StringVar(&cfg.Client, "client", "001", "SAP client number")
	rootCmd.Flags().StringVar(&cfg.Language, "language", "EN", "SAP language")
	rootCmd.Flags().BoolVar(&cfg.InsecureSkipVerify, "insecure", false, "Skip TLS certificate verification")

	// Cookie authentication
	rootCmd.Flags().String("cookie-file", "", "Path to cookie file in Netscape format")
	rootCmd.Flags().String("cookie-string", "", "Cookie string (key1=val1; key2=val2)")

	// Browser-based SSO authentication
	rootCmd.Flags().Bool("browser-auth", false, "Open browser for SSO login (Kerberos, SAML, Keycloak)")
	rootCmd.Flags().Duration("browser-auth-timeout", 120*time.Second, "Timeout for browser-based SSO login")
	rootCmd.Flags().String("browser-exec", "", "Path to Chromium-based browser (default: auto-detect Edge, Chrome, Chromium)")
	rootCmd.Flags().String("cookie-save", "", "Save browser auth cookies to file for reuse with --cookie-file")

	// Programmatic SAML SSO authentication (no browser required)
	rootCmd.Flags().Bool("sso", false, "Authenticate through a browser SSO handshake, re-capturing the session automatically when it expires (ignores any SAP_USER/SAP_PASSWORD)")
	rootCmd.Flags().String("sso-system", "", "Name the cached SSO session (defaults to the URL's host)")
	rootCmd.Flags().String("sso-trigger-url", "", "Authentication-gated URL that starts the SSO redirect (default: this system's ADT root)")
	rootCmd.Flags().String("sso-profile", "", "Browser profile directory for SSO (a Windows path when running under WSL)")
	rootCmd.Flags().String("sso-helper", "", "Path to the Windows SSO capture helper (vsp-sso.exe), used under WSL")
	rootCmd.Flags().String("sso-on-expiry", "window", "When a silent SSO refresh needs a human: 'window' opens a browser, 'error' reports what to run")
	rootCmd.Flags().Bool("saml-auth", false, "Authenticate via programmatic SAML SSO (no browser, no MFA)")
	rootCmd.Flags().String("saml-user", "", "SAML/IAS username (email)")
	rootCmd.Flags().String("saml-password", "", "SAML/IAS password")
	rootCmd.Flags().String("credential-cmd", "", "External command returning JSON {\"username\":...,\"password\":...} (space-separated argv, no shell quoting — use a wrapper script for paths with spaces)")

	// Session keep-alive
	rootCmd.Flags().Duration("keepalive", 0, "Session keep-alive interval (e.g., 60s, 5m). Prevents session timeout during idle periods. 0 = disabled (default; see #168)")

	// Long calls
	rootCmd.Flags().Int("call-timeout", 0, "Default budget in seconds of one long MCP call (ExecuteABAP, ABAP Unit including coverage, ATC, deploy, source write, activation) that names no params.timeout; at most 3600. 0 = none: each request to SAP is limited to 60s. A negative value is a startup error")

	// Safety options
	rootCmd.Flags().BoolVar(&cfg.ReadOnly, "read-only", false, "Block all write operations (create, update, delete, activate)")
	rootCmd.Flags().BoolVar(&cfg.BlockFreeSQL, "block-free-sql", false, "Block execution of arbitrary SQL queries via RunQuery")
	rootCmd.Flags().StringVar(&cfg.AllowedOps, "allowed-ops", "", "Whitelist of allowed operation types (e.g., \"RSQ\" for Read, Search, Query only)")
	rootCmd.Flags().StringVar(&cfg.DisallowedOps, "disallowed-ops", "", "Blacklist of operation types to block (e.g., \"CDUA\" for Create, Delete, Update, Activate)")
	rootCmd.Flags().StringSliceVar(&cfg.AllowedPackages, "allowed-packages", nil, "Restrict operations to specific packages (comma-separated, supports wildcards like Z*)")
	rootCmd.Flags().BoolVar(&cfg.EnableTransports, "enable-transports", false, "Enable transport management operations (disabled by default for safety)")
	rootCmd.Flags().BoolVar(&cfg.TransportReadOnly, "transport-read-only", false, "Only allow read operations on transports (list, get)")
	rootCmd.Flags().StringSliceVar(&cfg.AllowedTransports, "allowed-transports", nil, "Restrict transport operations to specific transports (comma-separated, supports wildcards like A4HK*)")
	// This is persistent because CLI subcommands such as `source write` must
	// receive the same explicit transportable-edit opt-in as the MCP server.
	// Keep it separate from the other root-only safety flags: moving all of
	// them would broaden this command-line surface without solving #117.
	rootCmd.PersistentFlags().BoolVar(&cfg.AllowTransportableEdits, "allow-transportable-edits", false, "Allow editing objects in transportable packages (requires transport parameter)")
	rootCmd.Flags().StringVar(&cfg.CTSProject, "cts-project", "", "CTS project every request vsp creates is filed under, unless the call names one (for systems that require a project)")
	rootCmd.Flags().StringVar(&cfg.TransportTarget, "transport-target", "", "Transport target of every request vsp creates, unless the call names one")
	rootCmd.Flags().StringVar(&cfg.TransportChoice, "transport-choice", "auto", "A write with no transport named: auto picks the object's own or an open request of yours that fits (and creates one with --enable-transports); off leaves it to SAP, which generates a request per write")

	// Mode options
	rootCmd.Flags().StringVar(&cfg.Mode, "mode", "hyperfocused", "Tool mode: hyperfocused (single universal SAP tool), focused (98 tools), or expert (148 tools)")
	rootCmd.Flags().StringVar(&cfg.DisabledGroups, "disabled-groups", "", "Disable tool groups: 5/U=UI5, T=Tests, H=HANA, D=Debug, GC=gCTS, N=i18n")

	// Transport options
	rootCmd.Flags().StringVar(&cfg.Transport, "transport", "stdio", "Transport mode: stdio (default) or http")
	rootCmd.Flags().StringVar(&cfg.HTTPAddr, "http-addr", ":8080", "HTTP listen address for Streamable HTTP transport")

	// Feature configuration (safety network)
	// Values: "auto" (default), "on", "off"
	rootCmd.Flags().StringVar(&cfg.FeatureHANA, "feature-hana", "auto", "HANA database detection: auto, on, off")
	rootCmd.Flags().StringVar(&cfg.FeatureAbapGit, "feature-abapgit", "auto", "abapGit integration: auto, on, off")
	rootCmd.Flags().StringVar(&cfg.FeatureRAP, "feature-rap", "auto", "RAP/OData development: auto, on, off")
	rootCmd.Flags().StringVar(&cfg.FeatureAMDP, "feature-amdp", "auto", "AMDP/HANA debugger: auto, on, off")
	rootCmd.Flags().StringVar(&cfg.FeatureUI5, "feature-ui5", "auto", "UI5/Fiori BSP management: auto, on, off")
	rootCmd.Flags().StringVar(&cfg.FeatureTransport, "feature-transport", "auto", "CTS transport management: auto, on, off")

	// Debugger configuration
	rootCmd.Flags().StringVar(&cfg.TerminalID, "terminal-id", "", "SAP GUI terminal ID for cross-tool breakpoint sharing")

	// Output options
	rootCmd.PersistentFlags().BoolVarP(&cfg.Verbose, "verbose", "v", false, "Enable verbose output to stderr")

	// Bind flags to viper for environment variable support
	viper.BindPFlag("url", rootCmd.Flags().Lookup("url"))
	viper.BindPFlag("user", rootCmd.Flags().Lookup("user"))
	viper.BindPFlag("password", rootCmd.Flags().Lookup("password"))
	viper.BindPFlag("client", rootCmd.Flags().Lookup("client"))
	viper.BindPFlag("language", rootCmd.Flags().Lookup("language"))
	viper.BindPFlag("insecure", rootCmd.Flags().Lookup("insecure"))
	viper.BindPFlag("cookie-file", rootCmd.Flags().Lookup("cookie-file"))
	viper.BindPFlag("cookie-string", rootCmd.Flags().Lookup("cookie-string"))
	viper.BindPFlag("browser-auth", rootCmd.Flags().Lookup("browser-auth"))
	viper.BindPFlag("browser-auth-timeout", rootCmd.Flags().Lookup("browser-auth-timeout"))
	viper.BindPFlag("sso", rootCmd.Flags().Lookup("sso"))
	viper.BindPFlag("sso-system", rootCmd.Flags().Lookup("sso-system"))
	viper.BindPFlag("sso-trigger-url", rootCmd.Flags().Lookup("sso-trigger-url"))
	viper.BindPFlag("sso-profile", rootCmd.Flags().Lookup("sso-profile"))
	viper.BindPFlag("sso-helper", rootCmd.Flags().Lookup("sso-helper"))
	viper.BindPFlag("sso-on-expiry", rootCmd.Flags().Lookup("sso-on-expiry"))
	viper.BindPFlag("saml-auth", rootCmd.Flags().Lookup("saml-auth"))
	viper.BindPFlag("saml-user", rootCmd.Flags().Lookup("saml-user"))
	viper.BindPFlag("saml-password", rootCmd.Flags().Lookup("saml-password"))
	viper.BindPFlag("credential-cmd", rootCmd.Flags().Lookup("credential-cmd"))
	viper.BindPFlag("browser-exec", rootCmd.Flags().Lookup("browser-exec"))
	viper.BindPFlag("cookie-save", rootCmd.Flags().Lookup("cookie-save"))
	viper.BindPFlag("keepalive", rootCmd.Flags().Lookup("keepalive"))
	viper.BindPFlag("read-only", rootCmd.Flags().Lookup("read-only"))
	viper.BindPFlag("block-free-sql", rootCmd.Flags().Lookup("block-free-sql"))
	viper.BindPFlag("allowed-ops", rootCmd.Flags().Lookup("allowed-ops"))
	viper.BindPFlag("disallowed-ops", rootCmd.Flags().Lookup("disallowed-ops"))
	viper.BindPFlag("allowed-packages", rootCmd.Flags().Lookup("allowed-packages"))
	viper.BindPFlag("enable-transports", rootCmd.Flags().Lookup("enable-transports"))
	viper.BindPFlag("transport-read-only", rootCmd.Flags().Lookup("transport-read-only"))
	viper.BindPFlag("allowed-transports", rootCmd.Flags().Lookup("allowed-transports"))
	viper.BindPFlag("allow-transportable-edits", rootCmd.PersistentFlags().Lookup("allow-transportable-edits"))
	viper.BindPFlag("mode", rootCmd.Flags().Lookup("mode"))
	viper.BindPFlag("disabled-groups", rootCmd.Flags().Lookup("disabled-groups"))
	viper.BindPFlag("verbose", rootCmd.PersistentFlags().Lookup("verbose"))

	// Feature configuration
	viper.BindPFlag("feature-hana", rootCmd.Flags().Lookup("feature-hana"))
	viper.BindPFlag("feature-abapgit", rootCmd.Flags().Lookup("feature-abapgit"))
	viper.BindPFlag("feature-rap", rootCmd.Flags().Lookup("feature-rap"))
	viper.BindPFlag("feature-amdp", rootCmd.Flags().Lookup("feature-amdp"))
	viper.BindPFlag("feature-ui5", rootCmd.Flags().Lookup("feature-ui5"))
	viper.BindPFlag("feature-transport", rootCmd.Flags().Lookup("feature-transport"))

	// Debugger configuration
	viper.BindPFlag("terminal-id", rootCmd.Flags().Lookup("terminal-id"))

	// Set up environment variable mapping
	viper.SetEnvKeyReplacer(strings.NewReplacer("-", "_"))
	viper.AutomaticEnv()
	viper.SetEnvPrefix("SAP")
}

func runServer(cmd *cobra.Command, args []string) error {
	// Resolve configuration with priority: flags > env vars > defaults
	resolveConfig(cmd)

	// -s NAME naming a home-config system with an snc block or a
	// transport_cmd supplies the logon itself.
	if err := applyNamedTransportSystem(cmd, cfg); err != nil {
		cmd.SilenceUsage = true
		return err
	}

	// A transport command is exclusive with every logon flow of vsp's own,
	// and those below contact the URL: refuse the mix before any of them runs.
	if transportCmdErr != nil {
		return transportCmdErr
	}
	if err := transportCmdConflicts(cmd); err != nil {
		cmd.SilenceUsage = true
		return err
	}

	// Validate configuration
	if err := validateConfig(); err != nil {
		return err
	}

	// Long-call budget of the MCP server: flag > SAP_CALL_TIMEOUT env
	callTimeout, err := resolveCallTimeout(cmd)
	if err != nil {
		return err
	}
	cfg.CallTimeout = callTimeout

	// The identity pin, against the configured logon, before any of the
	// logon flows below can contact SAP: a wrong user or client is refused
	// here, before a request carries anything anywhere. A browser, SAML or
	// SSO logon has no user name yet; its user is checked after the preflight.
	startupSystems, _, _ := config.LoadSystems()
	userFlag := cmd.Flags().Changed("user")
	if err := applyServerExpect(cfg, startupSystems, userFlag, interactiveLogon(cmd)); err != nil {
		// A refusal, not a misspelt flag: the usage text would bury it.
		cmd.SilenceUsage = true
		return err
	}

	// Browser-based SSO authentication (must run before processCookieAuth)
	if err := processBrowserAuth(cmd); err != nil {
		return err
	}

	// Programmatic SAML SSO authentication (must run before processCookieAuth)
	if err := processSAMLAuth(cmd); err != nil {
		return err
	}

	// Browser single sign-on (must run before processCookieAuth)
	if err := processSSOAuth(cmd); err != nil {
		return err
	}

	// Process cookie authentication
	if err := processCookieAuth(cmd); err != nil {
		return err
	}

	logServerLogon(os.Stderr, cfg, startupSystems, userFlag)

	if cfg.Verbose {
		fmt.Fprintf(os.Stderr, "[VERBOSE] Starting vsp server\n")
		fmt.Fprintf(os.Stderr, "[VERBOSE] Mode: %s\n", cfg.Mode)
		if cfg.DisabledGroups != "" {
			fmt.Fprintf(os.Stderr, "[VERBOSE] Disabled groups: %s (5/U=UI5, T=Tests, H=HANA, D=Debug)\n", cfg.DisabledGroups)
		}
		fmt.Fprintf(os.Stderr, "[VERBOSE] SAP URL: %s\n", cfg.BaseURL)
		fmt.Fprintf(os.Stderr, "[VERBOSE] SAP Client: %s\n", cfg.Client)
		fmt.Fprintf(os.Stderr, "[VERBOSE] SAP Language: %s\n", cfg.Language)
		if len(cfg.TransportCmd) > 0 {
			fmt.Fprintf(os.Stderr, "[VERBOSE] Auth: transport command %s (it authenticates; no local port)\n", adt.TransportCmdName(cfg.TransportCmd))
		} else if cfg.Username != "" {
			fmt.Fprintf(os.Stderr, "[VERBOSE] Auth: Basic (user: %s)\n", cfg.Username)
		} else if cfg.ReauthFunc != nil {
			fmt.Fprintf(os.Stderr, "[VERBOSE] Auth: SSO/SAML (%d cookies, re-authenticates when the session expires)\n", len(cfg.Cookies))
		} else if len(cfg.Cookies) > 0 {
			fmt.Fprintf(os.Stderr, "[VERBOSE] Auth: Cookie (%d cookies)\n", len(cfg.Cookies))
		}

		// Safety status
		if cfg.ReadOnly {
			fmt.Fprintf(os.Stderr, "[VERBOSE] Safety: READ-ONLY mode enabled\n")
		}
		if cfg.BlockFreeSQL {
			fmt.Fprintf(os.Stderr, "[VERBOSE] Safety: Free SQL queries BLOCKED\n")
		}
		if cfg.AllowedOps != "" {
			fmt.Fprintf(os.Stderr, "[VERBOSE] Safety: Allowed operations: %s\n", cfg.AllowedOps)
		}
		if cfg.DisallowedOps != "" {
			fmt.Fprintf(os.Stderr, "[VERBOSE] Safety: Disallowed operations: %s\n", cfg.DisallowedOps)
		}
		if len(cfg.AllowedPackages) > 0 {
			fmt.Fprintf(os.Stderr, "[VERBOSE] Safety: Allowed packages: %v\n", cfg.AllowedPackages)
		}
		if cfg.EnableTransports {
			fmt.Fprintf(os.Stderr, "[VERBOSE] Safety: Transport management ENABLED\n")
		}
		if cfg.AllowTransportableEdits {
			fmt.Fprintf(os.Stderr, "[VERBOSE] Safety: Transportable edits ENABLED (can modify non-local objects)\n")
		}
		if !cfg.ReadOnly && !cfg.BlockFreeSQL && cfg.AllowedOps == "" && cfg.DisallowedOps == "" && len(cfg.AllowedPackages) == 0 {
			fmt.Fprintf(os.Stderr, "[VERBOSE] Safety: UNRESTRICTED (no safety checks active)\n")
		}
		if cfg.KeepAliveInterval > 0 {
			fmt.Fprintf(os.Stderr, "[VERBOSE] Session keep-alive: %s\n", cfg.KeepAliveInterval)
		}
	}

	// Load granular tool visibility and per-system settings from .vsp.json if present
	if systemsCfg, configPath, err := config.LoadSystems(); err == nil && systemsCfg != nil {
		if systemsCfg.Tools != nil {
			cfg.ToolsConfig = systemsCfg.Tools
			if cfg.Verbose {
				enabled := 0
				disabled := 0
				for _, v := range systemsCfg.Tools {
					if v {
						enabled++
					} else {
						disabled++
					}
				}
				fmt.Fprintf(os.Stderr, "[VERBOSE] Tool config loaded from %s: %d enabled, %d disabled\n", configPath, enabled, disabled)
			}
		}

		warnNamedSystemMismatch(os.Stderr, cfg, systemsCfg)

		applyDefaultSystemSettings(os.Stderr, cfg, systemsCfg)
	}

	// The binary's own identity, so SAP() can say which build answered. An
	// agent reporting a defect against "vsp" names nothing; against a commit it
	// names something.
	cfg.Build = sweepBuild()

	// Create and start MCP server
	srv := mcp.NewServer(cfg)

	return serveMCP(cmd, func() error {
		switch cfg.Transport {
		case "http":
			addr := cfg.HTTPAddr
			if cfg.Verbose {
				fmt.Fprintf(os.Stderr, "[VERBOSE] Transport: Streamable HTTP on %s\n", addr)
			}
			return srv.ServeHTTP(addr)
		default:
			return srv.ServeStdio()
		}
	})
}

// serveMCP runs the server once the command line is known to be good. From
// here on an error is the server's, not the caller's spelling of a flag, so
// cobra must not answer it with the usage text: an MCP client reads stderr as
// the server's log, and a shutdown used to fill it with every flag vsp has.
func serveMCP(cmd *cobra.Command, serve func() error) error {
	cmd.SilenceUsage = true
	return serve()
}

// warnNamedSystemMismatch says at startup when the system named by -s /
// SAP_SYSTEM is not the one SAP_URL and SAP_CLIENT connect to. RFC use
// refuses it later; this only makes the reason visible before then.
func warnNamedSystemMismatch(w io.Writer, c *mcp.Config, systemsCfg *config.SystemsConfig) {
	if c.SystemName == "" || systemsCfg == nil {
		return
	}
	sys, ok := systemsCfg.Systems[c.SystemName]
	if !ok {
		return
	}
	if err := mcp.NamedSystemMismatch(c.SystemName, sys, c.BaseURL, c.Client); err != nil {
		fmt.Fprintf(w, "[WARNING] %v; RFC calls will be refused\n", err)
	}
}

// applyDefaultSystemSettings fills what the flags and the environment left
// empty from the default system in .vsp.json: transport_attribute, and where a
// request vsp creates is filed, cts_project and transport_target. The CLI takes
// the same keys from the system it runs against (resolveSystemParams).
func applyDefaultSystemSettings(w io.Writer, c *mcp.Config, systemsCfg *config.SystemsConfig) {
	if systemsCfg == nil || systemsCfg.Default == "" {
		return
	}
	sys, err := systemsCfg.GetSystem(systemsCfg.Default)
	if err != nil {
		// Not fatal for the server, which takes its logon elsewhere; but a
		// default system that cannot be read (a refused transport_cmd, a
		// default naming no system) should not pass in silence.
		fmt.Fprintf(w, "[WARNING] default system settings from .vsp.json not applied: %v\n", err)
		return
	}
	if c.TransportAttribute == "" && sys.TransportAttribute != "" {
		c.TransportAttribute = sys.TransportAttribute
	}
	if c.CTSProject == "" && sys.CTSProject != "" {
		c.CTSProject = sys.CTSProject
	}
	if c.TransportTarget == "" && sys.TransportTarget != "" {
		c.TransportTarget = sys.TransportTarget
	}
}

func resolveConfig(cmd *cobra.Command) {
	// Check if cookie auth is explicitly requested via CLI flags OR env vars
	// If so, we should NOT load user/password from env/.env to avoid conflicts
	// Cookie auth takes precedence over basic auth since it's more explicit
	cookieAuthViaCLI := cmd.Flags().Changed("cookie-file") || cmd.Flags().Changed("cookie-string")
	cookieAuthViaEnv := viper.GetString("COOKIE_FILE") != "" || viper.GetString("COOKIE_STRING") != ""
	browserAuth, _ := cmd.Flags().GetBool("browser-auth")
	hasBrowserAuth := browserAuth || viper.GetBool("BROWSER_AUTH")
	samlAuth, _ := cmd.Flags().GetBool("saml-auth")
	hasSAMLAuth := samlAuth || viper.GetBool("SAML_AUTH")
	hasCookieAuth := cookieAuthViaCLI || cookieAuthViaEnv || hasBrowserAuth || hasSAMLAuth || ssoRequested(cmd)

	// Transport command: --transport-cmd/--transport-arg > SAP_TRANSPORT_CMD
	cfg.TransportCmd, transportCmdErr = resolveTransportCmd(cmd)

	// URL: flag > SAP_URL env
	if cfg.BaseURL == "" {
		cfg.BaseURL = viper.GetString("URL")
	}
	if cfg.BaseURL == "" {
		cfg.BaseURL = viper.GetString("SERVICE_URL")
	}

	// Username: flag > SAP_USER env (skip if cookie auth is present)
	if cfg.Username == "" && !hasCookieAuth {
		cfg.Username = viper.GetString("USER")
	}
	if cfg.Username == "" && !hasCookieAuth {
		cfg.Username = viper.GetString("USERNAME")
	}

	// Password: flag > SAP_PASSWORD env (skip if cookie auth is present)
	if cfg.Password == "" && !hasCookieAuth {
		cfg.Password = viper.GetString("PASSWORD")
	}
	if cfg.Password == "" && !hasCookieAuth {
		cfg.Password = viper.GetString("PASS")
	}

	// Client: flag > SAP_CLIENT env > default
	if !cmd.Flags().Changed("client") {
		if envClient := viper.GetString("CLIENT"); envClient != "" {
			cfg.Client = envClient
		}
	}

	// Language: flag > SAP_LANGUAGE env > default
	if !cmd.Flags().Changed("language") {
		if envLang := viper.GetString("LANGUAGE"); envLang != "" {
			cfg.Language = envLang
		}
	}

	// Insecure: flag > SAP_INSECURE env
	if !cmd.Flags().Changed("insecure") {
		cfg.InsecureSkipVerify = viper.GetBool("INSECURE")
	}

	// Mode: flag > SAP_MODE env > default (focused)
	if !cmd.Flags().Changed("mode") {
		if envMode := viper.GetString("MODE"); envMode != "" {
			cfg.Mode = envMode
		}
	}

	// DisabledGroups: flag > SAP_DISABLED_GROUPS env
	if !cmd.Flags().Changed("disabled-groups") {
		if envGroups := viper.GetString("DISABLED_GROUPS"); envGroups != "" {
			cfg.DisabledGroups = envGroups
		}
	}

	// Verbose: flag > SAP_VERBOSE env
	if !cmd.Flags().Changed("verbose") {
		cfg.Verbose = viper.GetBool("VERBOSE")
	}

	// Safety options: flag > SAP_* env
	if !cmd.Flags().Changed("read-only") {
		cfg.ReadOnly = viper.GetBool("READ_ONLY")
	}
	if !cmd.Flags().Changed("block-free-sql") {
		cfg.BlockFreeSQL = viper.GetBool("BLOCK_FREE_SQL")
	}
	if !cmd.Flags().Changed("allowed-ops") {
		cfg.AllowedOps = viper.GetString("ALLOWED_OPS")
	}
	if !cmd.Flags().Changed("disallowed-ops") {
		cfg.DisallowedOps = viper.GetString("DISALLOWED_OPS")
	}
	if !cmd.Flags().Changed("allowed-packages") {
		// Use GetString and split manually - GetStringSlice doesn't split comma-separated env vars
		if pkgStr := viper.GetString("ALLOWED_PACKAGES"); pkgStr != "" {
			cfg.AllowedPackages = splitCommaSeparated(pkgStr)
		}
	}
	if !cmd.Flags().Changed("enable-transports") {
		cfg.EnableTransports = viper.GetBool("ENABLE_TRANSPORTS")
	}
	if !cmd.Flags().Changed("transport-read-only") {
		cfg.TransportReadOnly = viper.GetBool("TRANSPORT_READ_ONLY")
	}
	if !cmd.Flags().Changed("allowed-transports") {
		// Use GetString and split manually - GetStringSlice doesn't split comma-separated env vars
		if transportStr := viper.GetString("ALLOWED_TRANSPORTS"); transportStr != "" {
			cfg.AllowedTransports = splitCommaSeparated(transportStr)
		}
	}
	if !cmd.Flags().Changed("allow-transportable-edits") {
		cfg.AllowTransportableEdits = viper.GetBool("ALLOW_TRANSPORTABLE_EDITS")
	}
	// The server's own system in .vsp.json, for its per-system settings.
	cfg.SystemName = systemName
	if cfg.SystemName == "" {
		cfg.SystemName = viper.GetString("SYSTEM")
	}
	if !cmd.Flags().Changed("transport-choice") {
		if v := viper.GetString("TRANSPORT_CHOICE"); v != "" {
			cfg.TransportChoice = v
		}
	}
	if !cmd.Flags().Changed("cts-project") {
		cfg.CTSProject = viper.GetString("CTS_PROJECT")
	}
	if !cmd.Flags().Changed("transport-target") {
		cfg.TransportTarget = viper.GetString("TRANSPORT_TARGET")
	}

	// Feature configuration: flag > SAP_FEATURE_* env
	if !cmd.Flags().Changed("feature-hana") {
		if v := viper.GetString("FEATURE_HANA"); v != "" {
			cfg.FeatureHANA = v
		}
	}
	if !cmd.Flags().Changed("feature-abapgit") {
		if v := viper.GetString("FEATURE_ABAPGIT"); v != "" {
			cfg.FeatureAbapGit = v
		}
	}
	if !cmd.Flags().Changed("feature-rap") {
		if v := viper.GetString("FEATURE_RAP"); v != "" {
			cfg.FeatureRAP = v
		}
	}
	if !cmd.Flags().Changed("feature-amdp") {
		if v := viper.GetString("FEATURE_AMDP"); v != "" {
			cfg.FeatureAMDP = v
		}
	}
	if !cmd.Flags().Changed("feature-ui5") {
		if v := viper.GetString("FEATURE_UI5"); v != "" {
			cfg.FeatureUI5 = v
		}
	}
	if !cmd.Flags().Changed("feature-transport") {
		if v := viper.GetString("FEATURE_TRANSPORT"); v != "" {
			cfg.FeatureTransport = v
		}
	}

	// Transport attribute for CR-level co-change: SAP_TRANSPORT_ATTRIBUTE env > .vsp.json
	if v := viper.GetString("TRANSPORT_ATTRIBUTE"); v != "" {
		cfg.TransportAttribute = strings.ToUpper(strings.TrimSpace(v))
	}

	// Terminal ID for debugger: flag > SAP_TERMINAL_ID env
	if !cmd.Flags().Changed("terminal-id") {
		if v := viper.GetString("TERMINAL_ID"); v != "" {
			cfg.TerminalID = v
		}
	}

	// Keep-alive interval: flag > SAP_KEEPALIVE env
	if !cmd.Flags().Changed("keepalive") {
		if v := viper.GetString("KEEPALIVE"); v != "" {
			if d, err := time.ParseDuration(v); err == nil {
				cfg.KeepAliveInterval = d
			}
		}
	} else {
		cfg.KeepAliveInterval, _ = cmd.Flags().GetDuration("keepalive")
	}
}

// resolveCallTimeout reads --call-timeout, else SAP_CALL_TIMEOUT: seconds
// (the env also takes a Go duration such as 5m). 0, or nothing set, means no
// budget of the server's own. Anything else that is not at least one second
// is an error, not a silent "no budget": a typo would otherwise take the limit
// the operator meant to set away without a word. Capped at MaxCallTimeout.
func resolveCallTimeout(cmd *cobra.Command) (time.Duration, error) {
	var d time.Duration
	var source string
	if cmd.Flags().Changed("call-timeout") {
		secs, err := cmd.Flags().GetInt("call-timeout")
		if err != nil {
			return 0, fmt.Errorf("--call-timeout: %w", err)
		}
		source = fmt.Sprintf("--call-timeout %d", secs)
		if secs < 0 {
			return 0, fmt.Errorf("%s: must be a number of seconds, at least 1 (0 for none)", source)
		}
		if secs > int(mcp.MaxCallTimeout/time.Second) {
			return mcp.MaxCallTimeout, nil
		}
		d = time.Duration(secs) * time.Second
	} else if v := strings.TrimSpace(viper.GetString("CALL_TIMEOUT")); v != "" {
		source = fmt.Sprintf("SAP_CALL_TIMEOUT=%q", v)
		if f, err := strconv.ParseFloat(v, 64); err == nil {
			if math.IsNaN(f) || math.IsInf(f, 0) || f < 0 {
				return 0, fmt.Errorf("%s: must be a number of seconds, at least 1 (0 for none)", source)
			}
			if f > mcp.MaxCallTimeout.Seconds() {
				return mcp.MaxCallTimeout, nil
			}
			d = time.Duration(f * float64(time.Second))
		} else if pd, err := time.ParseDuration(v); err == nil {
			if pd < 0 {
				return 0, fmt.Errorf("%s: must not be negative", source)
			}
			d = pd
		} else {
			return 0, fmt.Errorf("%s: not a number of seconds or a duration such as 5m", source)
		}
	}
	if d == 0 {
		return 0, nil
	}
	if d < time.Second {
		return 0, fmt.Errorf("%s: below 1s; give at least one second, or 0 for none", source)
	}
	if d > mcp.MaxCallTimeout {
		d = mcp.MaxCallTimeout
	}
	return d, nil
}

func validateConfig() error {
	if cfg.BaseURL == "" {
		return fmt.Errorf("SAP URL is required. Use --url flag or SAP_URL environment variable")
	}

	// Validate mode
	if cfg.Mode != "focused" && cfg.Mode != "expert" && cfg.Mode != "hyperfocused" {
		return fmt.Errorf("invalid mode: %s (must be 'focused', 'expert', or 'hyperfocused')", cfg.Mode)
	}

	// Check if we have either basic auth or cookies will be processed
	// Cookies are checked later in processCookieAuth
	return nil
}

func processBrowserAuth(cmd *cobra.Command) error {
	browserAuth, _ := cmd.Flags().GetBool("browser-auth")
	if !browserAuth && !viper.GetBool("BROWSER_AUTH") {
		return nil
	}

	if cfg.BaseURL == "" {
		return fmt.Errorf("--browser-auth requires --url to be set")
	}

	// Determine timeout
	timeout, _ := cmd.Flags().GetDuration("browser-auth-timeout")
	if !cmd.Flags().Changed("browser-auth-timeout") {
		if v := viper.GetString("BROWSER_AUTH_TIMEOUT"); v != "" {
			if d, err := time.ParseDuration(v); err == nil {
				timeout = d
			}
		}
	}

	// Determine browser executable
	browserExec, _ := cmd.Flags().GetString("browser-exec")
	if browserExec == "" {
		browserExec = viper.GetString("BROWSER_EXEC")
	}

	// Determine sap client
	sapClient := cfg.Client
	if sapClient == "" {
		sapClient = viper.GetString("CLIENT")
	}

	ctx := cmd.Context()
	cookies, err := adt.BrowserLogin(ctx, cfg.BaseURL, cfg.InsecureSkipVerify, timeout, browserExec, cfg.Verbose, sapClient)
	if err != nil {
		return fmt.Errorf("browser authentication failed: %w", err)
	}

	cfg.Cookies = cookies

	// Save cookies to file if requested
	cookieSave, _ := cmd.Flags().GetString("cookie-save")
	if cookieSave == "" {
		cookieSave = viper.GetString("COOKIE_SAVE")
	}
	if cookieSave != "" {
		if err := adt.SaveCookiesToFile(cookies, cfg.BaseURL, cookieSave); err != nil {
			fmt.Fprintf(os.Stderr, "[BROWSER-AUTH] Warning: failed to save cookies: %v\n", err)
		} else {
			fmt.Fprintf(os.Stderr, "[BROWSER-AUTH] Cookies saved to %s (reuse with --cookie-file). Note: file contains session secrets — do not share or commit.\n", cookieSave)
		}
	}

	return nil
}

// ssoRequested reports whether browser single sign-on was asked for.
func ssoRequested(cmd *cobra.Command) bool {
	sso, _ := cmd.Flags().GetBool("sso")
	return sso || viper.GetBool("SSO")
}

// processSSOAuth sets up browser single sign-on for the server.
//
// Unlike the other auth flows, this one does not just hand over cookies and
// retire: it leaves a way to capture new ones behind. A server that runs for
// days will outlive any session it starts with, and the point of the exercise
// is that nobody has to notice when it does.
func processSSOAuth(cmd *cobra.Command) error {
	if !ssoRequested(cmd) {
		return nil
	}
	if cfg.BaseURL == "" {
		return fmt.Errorf("--sso requires --url to be set")
	}

	// SSO is exclusive, and deliberately so. A username and password reaching
	// the client alongside the cookies would not merely be redundant: basic
	// auth wins in the transport, and the automatic recovery is switched off
	// with it, because a client that can resend a password has no reason to go
	// looking for a browser. Silently pairing the two would turn "session
	// refreshes itself" into "session dies quietly at some point", so any
	// credential that arrived by flag or environment is dropped here, loudly.
	if cfg.Username != "" || cfg.Password != "" {
		fmt.Fprintf(os.Stderr, "[SSO] ignoring the username/password supplied for %s — single sign-on is authoritative\n", cfg.BaseURL)
		cfg.Username, cfg.Password = "", ""
	}

	stringOpt := func(flag, viperKey string) string {
		if v, _ := cmd.Flags().GetString(flag); v != "" {
			return v
		}
		return viper.GetString(viperKey)
	}

	onExpiry := stringOpt("sso-on-expiry", "SSO_ON_EXPIRY")
	provider, err := adt.NewSSOProvider(adt.SSOConfig{
		System:      stringOpt("sso-system", "SSO_SYSTEM"),
		BaseURL:     cfg.BaseURL,
		Client:      cfg.Client,
		TriggerURL:  stringOpt("sso-trigger-url", "SSO_TRIGGER_URL"),
		Profile:     stringOpt("sso-profile", "SSO_PROFILE"),
		HelperPath:  stringOpt("sso-helper", "SSO_HELPER"),
		Interactive: !strings.EqualFold(onExpiry, "error"),
		Insecure:    cfg.InsecureSkipVerify,
		Verbose:     cfg.Verbose,
	})
	if err != nil {
		return err
	}

	cookies, err := provider.Cookies(cmd.Context())
	if err != nil {
		return fmt.Errorf("browser SSO: %w", err)
	}
	cfg.Cookies = cookies
	cfg.ReauthFunc = provider.Refresh
	cfg.ReauthTimeout = provider.ReauthBudget()

	if cfg.Verbose {
		fmt.Fprintf(os.Stderr, "[SSO] session ready (%d cookies), cached at %s\n",
			len(cookies), provider.CachePath())
	}
	return nil
}

func processSAMLAuth(cmd *cobra.Command) error {
	samlAuth, _ := cmd.Flags().GetBool("saml-auth")
	if !samlAuth && !viper.GetBool("SAML_AUTH") {
		return nil
	}

	if cfg.BaseURL == "" {
		return fmt.Errorf("--saml-auth requires --url to be set")
	}

	// Resolve credential source. Priority: credential-cmd > env vars > flags.
	credCmdStr, _ := cmd.Flags().GetString("credential-cmd")
	if credCmdStr == "" {
		credCmdStr = viper.GetString("CREDENTIAL_CMD")
		if credCmdStr != "" && cfg.Verbose {
			fmt.Fprintf(os.Stderr, "[SAML-AUTH] Warning: credential-cmd sourced from environment variable\n")
		}
	}

	var credProvider adt.CredentialProvider

	if credCmdStr != "" {
		// Credential command mode: parse and execute external command on each auth.
		credArgs := adt.ParseCredentialCmd(credCmdStr)
		if len(credArgs) == 0 {
			return fmt.Errorf("--credential-cmd: empty command after parsing")
		}
		credProvider = func(ctx context.Context) ([]byte, []byte, error) {
			user, pass, err := adt.RunCredentialCmd(ctx, credArgs, cfg.Verbose)
			if err != nil {
				return nil, nil, err
			}
			return []byte(user), []byte(pass), nil
		}
	} else {
		// Direct credentials mode: env vars > flags.
		samlUser, _ := cmd.Flags().GetString("saml-user")
		if samlUser == "" {
			samlUser = viper.GetString("SAML_USER")
		}
		samlPassword, _ := cmd.Flags().GetString("saml-password")
		if samlPassword == "" {
			samlPassword = viper.GetString("SAML_PASSWORD")
		}

		if samlUser == "" || samlPassword == "" {
			return fmt.Errorf("--saml-auth requires credentials: use --credential-cmd, --saml-user/--saml-password, or SAP_SAML_USER/SAP_SAML_PASSWORD env vars")
		}

		// Build credential provider that re-reads env vars on each call.
		// This supports credential rotation and avoids long-term retention.
		flagUser := samlUser
		flagPassword := samlPassword
		credProvider = func(ctx context.Context) ([]byte, []byte, error) {
			u := os.Getenv("SAP_SAML_USER")
			if u == "" {
				u = flagUser
			}
			p := os.Getenv("SAP_SAML_PASSWORD")
			if p == "" {
				p = flagPassword
			}
			return []byte(u), []byte(p), nil
		}
	}

	ctx := cmd.Context()
	cookies, err := adt.SAMLLogin(ctx, cfg.BaseURL, credProvider, cfg.InsecureSkipVerify, cfg.Verbose)
	if err != nil {
		return fmt.Errorf("SAML authentication failed: %w", err)
	}

	cfg.Cookies = cookies

	// Set re-auth function for 401 recovery.
	cfg.ReauthFunc = func(ctx context.Context) (map[string]string, error) {
		return adt.SAMLLogin(ctx, cfg.BaseURL, credProvider, cfg.InsecureSkipVerify, cfg.Verbose)
	}

	// Save cookies if requested.
	cookieSave, _ := cmd.Flags().GetString("cookie-save")
	if cookieSave == "" {
		cookieSave = viper.GetString("COOKIE_SAVE")
	}
	if cookieSave != "" {
		if err := adt.SaveCookiesToFile(cookies, cfg.BaseURL, cookieSave); err != nil {
			fmt.Fprintf(os.Stderr, "[SAML-AUTH] Warning: failed to save cookies: %v\n", err)
		} else {
			fmt.Fprintf(os.Stderr, "[SAML-AUTH] Cookies saved to %s (reuse with --cookie-file). Note: file contains session secrets — do not share or commit.\n", cookieSave)
		}
	}

	return nil
}

func processCookieAuth(cmd *cobra.Command) error {
	// A transport command authenticates on its own; it needs, and takes,
	// no other method.
	if transportCmdErr != nil {
		return transportCmdErr
	}
	if len(cfg.TransportCmd) > 0 {
		return transportCmdConflicts(cmd)
	}

	cookieFile, _ := cmd.Flags().GetString("cookie-file")
	cookieString, _ := cmd.Flags().GetString("cookie-string")

	// Check environment variables if flags not provided
	if cookieFile == "" {
		cookieFile = viper.GetString("COOKIE_FILE")
	}
	if cookieString == "" {
		cookieString = viper.GetString("COOKIE_STRING")
	}

	// Count authentication methods
	authMethods := 0
	if cfg.Username != "" && cfg.Password != "" {
		authMethods++
	}
	if cookieFile != "" {
		authMethods++
	}
	if cookieString != "" {
		authMethods++
	}
	// Browser auth already populated cfg.Cookies in processBrowserAuth
	if len(cfg.Cookies) > 0 {
		authMethods++
	}

	if authMethods > 1 {
		return fmt.Errorf("only one authentication method can be used at a time (basic auth, cookie-file, cookie-string, browser-auth, or saml-auth)")
	}

	if authMethods == 0 {
		return fmt.Errorf("authentication required. Use --user/--password, --cookie-file, --cookie-string, --browser-auth, --saml-auth, or --transport-cmd")
	}

	// If cookies already set by browser auth, we're done
	if len(cfg.Cookies) > 0 {
		return nil
	}

	// Process cookie file
	if cookieFile != "" {
		cookieFile, err := filepath.Abs(cookieFile)
		if err != nil {
			return fmt.Errorf("resolving cookie file path: %w", err)
		}

		cookies, err := adt.LoadCookiesFromFile(cookieFile)
		if err != nil {
			return fmt.Errorf("failed to load cookies from file: %w", err)
		}

		if len(cookies) == 0 {
			return fmt.Errorf("no cookies found in file: %s", cookieFile)
		}

		cfg.Cookies = cookies
		reauth, err := adt.NewCookieFileReauthFunc(cookieFile)
		if err != nil {
			return err
		}
		cfg.ReauthFunc = reauth
		cfg.ReauthReadOnly = true
		if cfg.Verbose {
			fmt.Fprintf(os.Stderr, "[VERBOSE] Loaded %d cookies from file: %s\n", len(cookies), cookieFile)
		}
	}

	// Process cookie string
	if cookieString != "" {
		cookies := adt.ParseCookieString(cookieString)
		if len(cookies) == 0 {
			return fmt.Errorf("failed to parse cookie string")
		}

		cfg.Cookies = cookies
		if cfg.Verbose {
			fmt.Fprintf(os.Stderr, "[VERBOSE] Parsed %d cookies from string\n", len(cookies))
		}
	}

	return nil
}

// splitCommaSeparated splits a comma-separated string into a slice, trimming whitespace.
// This is needed because viper.GetStringSlice doesn't properly split comma-separated env vars.
func splitCommaSeparated(s string) []string {
	if s == "" {
		return nil
	}
	parts := strings.Split(s, ",")
	result := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			result = append(result, p)
		}
	}
	return result
}

func main() {
	if sncServeInvocation(os.Args) {
		os.Exit(runSNCServe(os.Args, os.Stdin, os.Stdout, os.Stderr))
	}
	err := rootCmd.Execute()
	// The CLI's transport-command helpers, on success and on error alike.
	closeTransportCmds()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}
