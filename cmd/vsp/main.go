// vsp is an MCP server providing ABAP Development Tools (ADT) functionality.
package main

import (
	"context"
	"fmt"
	"os"
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
)

var cfg = &mcp.Config{}

var rootCmd = &cobra.Command{
	Use:   "vsp",
	Short: "ABAP Development Tools for AI agents and DevOps",
	Long: `vsp — ABAP Development Tools for AI agents and DevOps.

Single binary, 9 platforms, no dependencies. Download from GitHub releases,
point your MCP config at it, done.

Two modes of operation:

  MCP Server (default)  Connects Claude, Gemini CLI, Copilot, Codex, Qwen Code,
                        and other MCP-compatible agents to SAP systems.
                        81 tools (focused), 122 (expert), or 1 universal tool (hyperfocused).

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
	RunE: runServer,
}

func init() {
	// Load .env file if it exists
	godotenv.Load()

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
	rootCmd.Flags().Bool("saml-auth", false, "Authenticate via programmatic SAML SSO (no browser, no MFA)")
	rootCmd.Flags().String("saml-user", "", "SAML/IAS username (email)")
	rootCmd.Flags().String("saml-password", "", "SAML/IAS password")

	// Session keep-alive
	rootCmd.Flags().Duration("keepalive", 5*time.Minute, "Session keep-alive interval (e.g., 60s, 5m). Prevents session timeout during idle periods. 0 = disabled")

	// Safety options
	rootCmd.Flags().BoolVar(&cfg.ReadOnly, "read-only", false, "Block all write operations (create, update, delete, activate)")
	rootCmd.Flags().BoolVar(&cfg.BlockFreeSQL, "block-free-sql", false, "Block execution of arbitrary SQL queries via RunQuery")
	rootCmd.Flags().StringVar(&cfg.AllowedOps, "allowed-ops", "", "Whitelist of allowed operation types (e.g., \"RSQ\" for Read, Search, Query only)")
	rootCmd.Flags().StringVar(&cfg.DisallowedOps, "disallowed-ops", "", "Blacklist of operation types to block (e.g., \"CDUA\" for Create, Delete, Update, Activate)")
	rootCmd.Flags().StringSliceVar(&cfg.AllowedPackages, "allowed-packages", nil, "Restrict operations to specific packages (comma-separated, supports wildcards like Z*)")
	rootCmd.Flags().BoolVar(&cfg.EnableTransports, "enable-transports", false, "Enable transport management operations (disabled by default for safety)")
	rootCmd.Flags().BoolVar(&cfg.TransportReadOnly, "transport-read-only", false, "Only allow read operations on transports (list, get)")
	rootCmd.Flags().StringSliceVar(&cfg.AllowedTransports, "allowed-transports", nil, "Restrict transport operations to specific transports (comma-separated, supports wildcards like A4HK*)")
	rootCmd.Flags().BoolVar(&cfg.AllowTransportableEdits, "allow-transportable-edits", false, "Allow editing objects in transportable packages (requires transport parameter)")

	// Mode options
	rootCmd.Flags().StringVar(&cfg.Mode, "mode", "hyperfocused", "Tool mode: hyperfocused (single universal SAP tool), focused (100 tools), or expert (147 tools)")
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
	viper.BindPFlag("saml-auth", rootCmd.Flags().Lookup("saml-auth"))
	viper.BindPFlag("saml-user", rootCmd.Flags().Lookup("saml-user"))
	viper.BindPFlag("saml-password", rootCmd.Flags().Lookup("saml-password"))
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
	viper.BindPFlag("allow-transportable-edits", rootCmd.Flags().Lookup("allow-transportable-edits"))
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

	// Validate configuration
	if err := validateConfig(); err != nil {
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

	// Process cookie authentication
	if err := processCookieAuth(cmd); err != nil {
		return err
	}

	if cfg.Verbose {
		fmt.Fprintf(os.Stderr, "[VERBOSE] Starting vsp server\n")
		fmt.Fprintf(os.Stderr, "[VERBOSE] Mode: %s\n", cfg.Mode)
		if cfg.DisabledGroups != "" {
			fmt.Fprintf(os.Stderr, "[VERBOSE] Disabled groups: %s (5/U=UI5, T=Tests, H=HANA, D=Debug)\n", cfg.DisabledGroups)
		}
		fmt.Fprintf(os.Stderr, "[VERBOSE] SAP URL: %s\n", cfg.BaseURL)
		fmt.Fprintf(os.Stderr, "[VERBOSE] SAP Client: %s\n", cfg.Client)
		fmt.Fprintf(os.Stderr, "[VERBOSE] SAP Language: %s\n", cfg.Language)
		if cfg.Username != "" {
			fmt.Fprintf(os.Stderr, "[VERBOSE] Auth: Basic (user: %s)\n", cfg.Username)
		} else if cfg.ReauthFunc != nil {
			fmt.Fprintf(os.Stderr, "[VERBOSE] Auth: SAML (%d cookies, re-auth on 401)\n", len(cfg.Cookies))
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

		// Load transport_attribute from default system if not already set via env
		if cfg.TransportAttribute == "" && systemsCfg.Default != "" {
			if sys, err := systemsCfg.GetSystem(systemsCfg.Default); err == nil && sys.TransportAttribute != "" {
				cfg.TransportAttribute = sys.TransportAttribute
			}
		}
	}

	// Create and start MCP server
	srv := mcp.NewServer(cfg)

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
	hasCookieAuth := cookieAuthViaCLI || cookieAuthViaEnv || hasBrowserAuth || hasSAMLAuth

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

	ctx := context.Background()
	cookies, err := adt.BrowserLogin(ctx, cfg.BaseURL, cfg.InsecureSkipVerify, timeout, browserExec, cfg.Verbose)
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
			fmt.Fprintf(os.Stderr, "[BROWSER-AUTH] Cookies saved to %s (reuse with --cookie-file)\n", cookieSave)
		}
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

	samlUser, _ := cmd.Flags().GetString("saml-user")
	if samlUser == "" {
		samlUser = viper.GetString("SAML_USER")
	}
	samlPassword, _ := cmd.Flags().GetString("saml-password")
	if samlPassword == "" {
		samlPassword = viper.GetString("SAML_PASSWORD")
	}

	if samlUser == "" || samlPassword == "" {
		return fmt.Errorf("--saml-auth requires --saml-user and --saml-password (or SAP_SAML_USER / SAP_SAML_PASSWORD env vars)")
	}

	// Build credential provider that re-reads env vars on each call.
	// This supports credential rotation and avoids long-term retention.
	flagUser := samlUser
	flagPassword := samlPassword
	credProvider := func(ctx context.Context) ([]byte, []byte, error) {
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

	ctx := context.Background()
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
			fmt.Fprintf(os.Stderr, "[SAML-AUTH] Cookies saved to %s (reuse with --cookie-file)\n", cookieSave)
		}
	}

	return nil
}

func processCookieAuth(cmd *cobra.Command) error {
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
		return fmt.Errorf("authentication required. Use --user/--password, --cookie-file, --cookie-string, --browser-auth, or --saml-auth")
	}

	// If cookies already set by browser auth, we're done
	if len(cfg.Cookies) > 0 {
		return nil
	}

	// Process cookie file
	if cookieFile != "" {
		if _, err := os.Stat(cookieFile); os.IsNotExist(err) {
			return fmt.Errorf("cookie file not found: %s", cookieFile)
		}

		cookies, err := adt.LoadCookiesFromFile(cookieFile)
		if err != nil {
			return fmt.Errorf("failed to load cookies from file: %w", err)
		}

		if len(cookies) == 0 {
			return fmt.Errorf("no cookies found in file: %s", cookieFile)
		}

		cfg.Cookies = cookies
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
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}
