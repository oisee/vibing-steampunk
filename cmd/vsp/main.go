// vsp is an MCP server providing ABAP Development Tools (ADT) functionality.
package main

import (
	"fmt"
	"os"
	"strings"

	"github.com/joho/godotenv"
	"github.com/oisee/vibing-steampunk/internal/mcp"
	"github.com/oisee/vibing-steampunk/pkg/adt"
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
	Short: "MCP server for SAP ABAP Development Tools (ADT)",
	Long: `vsp is a Model Context Protocol (MCP) server that provides
ABAP Development Tools (ADT) functionality for AI assistants like Claude.

It exposes a single universal SAP tool that routes to 86+ internal operations for reading, writing, and managing ABAP code in SAP systems.

Examples:
  # Using environment variables
  SAP_URL=https://host:44300 SAP_USER=user SAP_PASSWORD=pass vsp

  # Using command-line flags
  vsp --url https://host:44300 --user admin --password secret

  # Using .env file
  vsp  # reads from .env in current directory

  # Using cookie authentication
  vsp --url https://host:44300 --cookie-string "session=abc123; token=xyz"
  vsp --url https://host:44300 --cookie-file cookies.txt`,
	Version: fmt.Sprintf("%s (commit: %s, built: %s)", Version, Commit, BuildDate),
	RunE:    runServer,
}

// stringFlag defines a string CLI flag
type stringFlag struct {
	name, shorthand, defaultValue, description string
}

// boolFlag defines a bool CLI flag
type boolFlag struct {
	name, shorthand, description string
	defaultValue                 bool
}

// sliceFlag defines a string slice CLI flag
type sliceFlag struct {
	name, description string
}

var stringFlags = []stringFlag{
	{"url", "", "", "SAP system URL (e.g., https://host:44300)"},
	{"service", "", "", "SAP system URL (alias for --url)"},
	{"user", "u", "", "SAP username"},
	{"password", "p", "", "SAP password"},
	{"pass", "", "", "SAP password (alias for --password)"},
	{"client", "", "001", "SAP client number"},
	{"language", "", "EN", "SAP language"},
	{"cookie-file", "", "", "Path to cookie file in Netscape format"},
	{"cookie-string", "", "", "Cookie string (key1=val1; key2=val2)"},
	{"allowed-ops", "", "", "Whitelist of allowed operation types (e.g., \"RSQ\" for Read, Search, Query only)"},
	{"disallowed-ops", "", "", "Blacklist of operation types to block (e.g., \"CDUA\" for Create, Delete, Update, Activate)"},
	{"feature-abapgit", "", "auto", "abapGit integration: auto, on, off"},
	{"feature-rap", "", "auto", "RAP/OData development: auto, on, off"},
	{"feature-amdp", "", "auto", "AMDP/HANA debugger: auto, on, off"},
	{"feature-ui5", "", "auto", "UI5/Fiori BSP management: auto, on, off"},
	{"feature-transport", "", "auto", "CTS transport management: auto, on, off"},
	{"terminal-id", "", "", "SAP GUI terminal ID for cross-tool breakpoint sharing"},
}

var boolFlags = []boolFlag{
	{"insecure", "", "Skip TLS certificate verification", false},
	{"read-only", "", "Block all write operations (create, update, delete, activate)", false},
	{"block-free-sql", "", "Block execution of arbitrary SQL queries via RunQuery", false},
	{"enable-transports", "", "Enable transport management operations (disabled by default for safety)", false},
	{"transport-read-only", "", "Only allow read operations on transports (list, get)", false},
	{"verbose", "v", "Enable verbose output to stderr", false},
}

var sliceFlags = []sliceFlag{
	{"allowed-packages", "Restrict operations to specific packages (comma-separated, supports wildcards like Z*)"},
	{"allowed-transports", "Restrict transport operations to specific transports (comma-separated, supports wildcards like A4HK*)"},
}

func init() {
	// Load .env file if it exists (ignore error - file is optional)
	_ = godotenv.Load()

	// Register string flags
	for _, f := range stringFlags {
		if f.shorthand != "" {
			rootCmd.Flags().StringP(f.name, f.shorthand, f.defaultValue, f.description)
		} else {
			rootCmd.Flags().String(f.name, f.defaultValue, f.description)
		}
		_ = viper.BindPFlag(f.name, rootCmd.Flags().Lookup(f.name))
	}

	// Register bool flags
	for _, f := range boolFlags {
		if f.shorthand != "" {
			rootCmd.Flags().BoolP(f.name, f.shorthand, f.defaultValue, f.description)
		} else {
			rootCmd.Flags().Bool(f.name, f.defaultValue, f.description)
		}
		_ = viper.BindPFlag(f.name, rootCmd.Flags().Lookup(f.name))
	}

	// Register slice flags
	for _, f := range sliceFlags {
		rootCmd.Flags().StringSlice(f.name, nil, f.description)
		_ = viper.BindPFlag(f.name, rootCmd.Flags().Lookup(f.name))
	}

	// Set up environment variable mapping
	viper.SetEnvKeyReplacer(strings.NewReplacer("-", "_"))
	viper.AutomaticEnv()
	viper.SetEnvPrefix("SAP")
}

func runServer(cmd *cobra.Command, _ []string) error {
	// Resolve configuration with priority: flags > env vars > defaults
	resolveConfig(cmd)

	// Validate configuration
	if err := validateConfig(); err != nil {
		return err
	}

	// Process cookie authentication
	if err := processCookieAuth(cmd); err != nil {
		return err
	}

	// Verbose startup logging
	if cfg.Verbose {
		adt.SetLogOutput(os.Stderr)
		logStartupInfo()
	}

	// Create and start MCP server
	server := mcp.NewServer(cfg)
	return server.ServeStdio()
}

// logStartupInfo outputs verbose startup information
func logStartupInfo() {
	fmt.Fprintf(os.Stderr, "[VERBOSE] Starting vsp server (unified mode - 1 SAP tool)\n")
	fmt.Fprintf(os.Stderr, "[VERBOSE] SAP URL: %s\n", cfg.BaseURL)
	fmt.Fprintf(os.Stderr, "[VERBOSE] SAP Client: %s\n", cfg.Client)
	fmt.Fprintf(os.Stderr, "[VERBOSE] SAP Language: %s\n", cfg.Language)

	if cfg.Username != "" {
		fmt.Fprintf(os.Stderr, "[VERBOSE] Auth: Basic (user: %s)\n", cfg.Username)
	} else if len(cfg.Cookies) > 0 {
		fmt.Fprintf(os.Stderr, "[VERBOSE] Auth: Cookie (%d cookies)\n", len(cfg.Cookies))
	}

	logSafetyStatus()
}

// logSafetyStatus outputs verbose safety configuration
func logSafetyStatus() {
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
	if !cfg.ReadOnly && !cfg.BlockFreeSQL && cfg.AllowedOps == "" && cfg.DisallowedOps == "" && len(cfg.AllowedPackages) == 0 {
		fmt.Fprintf(os.Stderr, "[VERBOSE] Safety: UNRESTRICTED (no safety checks active)\n")
	}
}

func resolveConfig(cmd *cobra.Command) {
	hasCookieAuth := detectCookieAuth(cmd)
	resolveConnectionConfig(cmd, hasCookieAuth)
	resolveSafetyConfig(cmd)
	resolveFeatureConfig(cmd)
}

// detectCookieAuth checks if cookie authentication is requested
func detectCookieAuth(cmd *cobra.Command) bool {
	cookieAuthViaCLI := cmd.Flags().Changed("cookie-file") || cmd.Flags().Changed("cookie-string")
	cookieAuthViaEnv := viper.GetString("COOKIE_FILE") != "" || viper.GetString("COOKIE_STRING") != ""
	return cookieAuthViaCLI || cookieAuthViaEnv
}

// resolveConnectionConfig resolves URL, auth, and connection settings
func resolveConnectionConfig(cmd *cobra.Command, hasCookieAuth bool) {
	// URL: flag > SAP_URL > SAP_SERVICE_URL
	cfg.BaseURL = getFirstNonEmpty("URL", "SERVICE_URL")

	// Username/Password: skip if cookie auth is present
	if !hasCookieAuth {
		if cfg.Username == "" {
			cfg.Username = getFirstNonEmpty("USER", "USERNAME")
		}
		if cfg.Password == "" {
			cfg.Password = getFirstNonEmpty("PASSWORD", "PASS")
		}
	}

	// Client: flag > env > default
	if !cmd.Flags().Changed("client") {
		if v := viper.GetString("CLIENT"); v != "" {
			cfg.Client = v
		}
	} else {
		cfg.Client, _ = cmd.Flags().GetString("client")
	}

	// Language: flag > env > default
	if !cmd.Flags().Changed("language") {
		if v := viper.GetString("LANGUAGE"); v != "" {
			cfg.Language = v
		}
	} else {
		cfg.Language, _ = cmd.Flags().GetString("language")
	}

	// Simple bool/string from env if flag not changed
	if !cmd.Flags().Changed("insecure") {
		cfg.InsecureSkipVerify = viper.GetBool("INSECURE")
	} else {
		cfg.InsecureSkipVerify, _ = cmd.Flags().GetBool("insecure")
	}

	if !cmd.Flags().Changed("verbose") {
		cfg.Verbose = viper.GetBool("VERBOSE")
	} else {
		cfg.Verbose, _ = cmd.Flags().GetBool("verbose")
	}
}

// resolveSafetyConfig resolves safety-related settings
func resolveSafetyConfig(cmd *cobra.Command) {
	resolveBool(cmd, "read-only", "READ_ONLY", &cfg.ReadOnly)
	resolveBool(cmd, "block-free-sql", "BLOCK_FREE_SQL", &cfg.BlockFreeSQL)
	resolveBool(cmd, "enable-transports", "ENABLE_TRANSPORTS", &cfg.EnableTransports)
	resolveBool(cmd, "transport-read-only", "TRANSPORT_READ_ONLY", &cfg.TransportReadOnly)

	resolveString(cmd, "allowed-ops", "ALLOWED_OPS", &cfg.AllowedOps)
	resolveString(cmd, "disallowed-ops", "DISALLOWED_OPS", &cfg.DisallowedOps)

	resolveStringSlice(cmd, "allowed-packages", "ALLOWED_PACKAGES", &cfg.AllowedPackages)
	resolveStringSlice(cmd, "allowed-transports", "ALLOWED_TRANSPORTS", &cfg.AllowedTransports)
}

// resolveFeatureConfig resolves feature flag settings
func resolveFeatureConfig(cmd *cobra.Command) {
	resolveString(cmd, "feature-abapgit", "FEATURE_ABAPGIT", &cfg.FeatureAbapGit)
	resolveString(cmd, "feature-rap", "FEATURE_RAP", &cfg.FeatureRAP)
	resolveString(cmd, "feature-amdp", "FEATURE_AMDP", &cfg.FeatureAMDP)
	resolveString(cmd, "feature-ui5", "FEATURE_UI5", &cfg.FeatureUI5)
	resolveString(cmd, "feature-transport", "FEATURE_TRANSPORT", &cfg.FeatureTransport)
	resolveString(cmd, "terminal-id", "TERMINAL_ID", &cfg.TerminalID)
}

// Helper functions for config resolution

func getFirstNonEmpty(keys ...string) string {
	for _, key := range keys {
		if v := viper.GetString(key); v != "" {
			return v
		}
	}
	return ""
}

func resolveBool(cmd *cobra.Command, flag, envKey string, target *bool) {
	if !cmd.Flags().Changed(flag) {
		*target = viper.GetBool(envKey)
	} else {
		*target, _ = cmd.Flags().GetBool(flag)
	}
}

func resolveString(cmd *cobra.Command, flag, envKey string, target *string) {
	if !cmd.Flags().Changed(flag) {
		if v := viper.GetString(envKey); v != "" {
			*target = v
		}
	} else {
		*target, _ = cmd.Flags().GetString(flag)
	}
}

func resolveStringSlice(cmd *cobra.Command, flag, envKey string, target *[]string) {
	if !cmd.Flags().Changed(flag) {
		if v := viper.GetStringSlice(envKey); len(v) > 0 {
			*target = v
		}
	} else {
		*target, _ = cmd.Flags().GetStringSlice(flag)
	}
}

func validateConfig() error {
	if cfg.BaseURL == "" {
		return fmt.Errorf("SAP URL is required. Use --url flag or SAP_URL environment variable")
	}
	return nil
}

func processCookieAuth(cmd *cobra.Command) error {
	cookieFile := getStringFlag(cmd, "cookie-file", "COOKIE_FILE")
	cookieString := getStringFlag(cmd, "cookie-string", "COOKIE_STRING")

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

	if authMethods > 1 {
		return fmt.Errorf("only one authentication method can be used at a time (basic auth, cookie-file, or cookie-string)")
	}

	if authMethods == 0 {
		return fmt.Errorf("authentication required. Use --user/--password, --cookie-file, or --cookie-string")
	}

	// Process cookie file
	if cookieFile != "" {
		cookies, err := loadCookieFile(cookieFile)
		if err != nil {
			return err
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

func getStringFlag(cmd *cobra.Command, flag, envKey string) string {
	if v, _ := cmd.Flags().GetString(flag); v != "" {
		return v
	}
	return viper.GetString(envKey)
}

func loadCookieFile(path string) (map[string]string, error) {
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return nil, fmt.Errorf("cookie file not found: %s", path)
	}

	cookies, err := adt.LoadCookiesFromFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to load cookies from file: %w", err)
	}

	if len(cookies) == 0 {
		return nil, fmt.Errorf("no cookies found in file: %s", path)
	}

	return cookies, nil
}

func main() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}
