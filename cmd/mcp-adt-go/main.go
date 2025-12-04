// mcp-adt-go is an MCP server providing ABAP Development Tools (ADT) functionality.
package main

import (
	"fmt"
	"os"
	"strings"

	"github.com/joho/godotenv"
	"github.com/oisee/vibing-steamer/internal/mcp"
	"github.com/oisee/vibing-steamer/pkg/adt"
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
	Use:   "mcp-adt-go",
	Short: "MCP server for SAP ABAP Development Tools (ADT)",
	Long: `mcp-adt-go is a Model Context Protocol (MCP) server that provides
ABAP Development Tools (ADT) functionality for AI assistants like Claude.

It exposes 36 tools for reading, writing, and managing ABAP code in SAP systems.

Examples:
  # Using environment variables
  SAP_URL=https://host:44300 SAP_USER=user SAP_PASSWORD=pass mcp-adt-go

  # Using command-line flags
  mcp-adt-go --url https://host:44300 --user admin --password secret

  # Using .env file
  mcp-adt-go  # reads from .env in current directory

  # Using cookie authentication
  mcp-adt-go --url https://host:44300 --cookie-string "session=abc123; token=xyz"
  mcp-adt-go --url https://host:44300 --cookie-file cookies.txt`,
	Version: fmt.Sprintf("%s (commit: %s, built: %s)", Version, Commit, BuildDate),
	RunE:    runServer,
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

	// Safety options
	rootCmd.Flags().BoolVar(&cfg.ReadOnly, "read-only", false, "Block all write operations (create, update, delete, activate)")
	rootCmd.Flags().BoolVar(&cfg.BlockFreeSQL, "block-free-sql", false, "Block execution of arbitrary SQL queries via RunQuery")
	rootCmd.Flags().StringVar(&cfg.AllowedOps, "allowed-ops", "", "Whitelist of allowed operation types (e.g., \"RSQ\" for Read, Search, Query only)")
	rootCmd.Flags().StringVar(&cfg.DisallowedOps, "disallowed-ops", "", "Blacklist of operation types to block (e.g., \"CDUA\" for Create, Delete, Update, Activate)")
	rootCmd.Flags().StringSliceVar(&cfg.AllowedPackages, "allowed-packages", nil, "Restrict operations to specific packages (comma-separated, supports wildcards like Z*)")

	// Mode options
	rootCmd.Flags().StringVar(&cfg.Mode, "mode", "focused", "Tool mode: focused (14 essential tools) or expert (all 43 tools)")

	// Output options
	rootCmd.Flags().BoolVarP(&cfg.Verbose, "verbose", "v", false, "Enable verbose output to stderr")

	// Bind flags to viper for environment variable support
	viper.BindPFlag("url", rootCmd.Flags().Lookup("url"))
	viper.BindPFlag("user", rootCmd.Flags().Lookup("user"))
	viper.BindPFlag("password", rootCmd.Flags().Lookup("password"))
	viper.BindPFlag("client", rootCmd.Flags().Lookup("client"))
	viper.BindPFlag("language", rootCmd.Flags().Lookup("language"))
	viper.BindPFlag("insecure", rootCmd.Flags().Lookup("insecure"))
	viper.BindPFlag("cookie-file", rootCmd.Flags().Lookup("cookie-file"))
	viper.BindPFlag("cookie-string", rootCmd.Flags().Lookup("cookie-string"))
	viper.BindPFlag("read-only", rootCmd.Flags().Lookup("read-only"))
	viper.BindPFlag("block-free-sql", rootCmd.Flags().Lookup("block-free-sql"))
	viper.BindPFlag("allowed-ops", rootCmd.Flags().Lookup("allowed-ops"))
	viper.BindPFlag("disallowed-ops", rootCmd.Flags().Lookup("disallowed-ops"))
	viper.BindPFlag("allowed-packages", rootCmd.Flags().Lookup("allowed-packages"))
	viper.BindPFlag("mode", rootCmd.Flags().Lookup("mode"))
	viper.BindPFlag("verbose", rootCmd.Flags().Lookup("verbose"))

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

	// Process cookie authentication
	if err := processCookieAuth(cmd); err != nil {
		return err
	}

	if cfg.Verbose {
		fmt.Fprintf(os.Stderr, "[VERBOSE] Starting mcp-adt-go server\n")
		fmt.Fprintf(os.Stderr, "[VERBOSE] Mode: %s\n", cfg.Mode)
		fmt.Fprintf(os.Stderr, "[VERBOSE] SAP URL: %s\n", cfg.BaseURL)
		fmt.Fprintf(os.Stderr, "[VERBOSE] SAP Client: %s\n", cfg.Client)
		fmt.Fprintf(os.Stderr, "[VERBOSE] SAP Language: %s\n", cfg.Language)
		if cfg.Username != "" {
			fmt.Fprintf(os.Stderr, "[VERBOSE] Auth: Basic (user: %s)\n", cfg.Username)
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
		if !cfg.ReadOnly && !cfg.BlockFreeSQL && cfg.AllowedOps == "" && cfg.DisallowedOps == "" && len(cfg.AllowedPackages) == 0 {
			fmt.Fprintf(os.Stderr, "[VERBOSE] Safety: UNRESTRICTED (no safety checks active)\n")
		}
	}

	// Create and start MCP server
	server := mcp.NewServer(cfg)
	return server.ServeStdio()
}

func resolveConfig(cmd *cobra.Command) {
	// Check if cookie auth is explicitly requested via CLI flags
	// If so, we should NOT load user/password from env/.env to avoid conflicts
	cookieAuthViaCLI := cmd.Flags().Changed("cookie-file") || cmd.Flags().Changed("cookie-string")

	// URL: flag > SAP_URL env
	if cfg.BaseURL == "" {
		cfg.BaseURL = viper.GetString("URL")
	}
	if cfg.BaseURL == "" {
		cfg.BaseURL = viper.GetString("SERVICE_URL")
	}

	// Username: flag > SAP_USER env (skip env if cookie auth via CLI)
	if cfg.Username == "" && !cookieAuthViaCLI {
		cfg.Username = viper.GetString("USER")
	}
	if cfg.Username == "" && !cookieAuthViaCLI {
		cfg.Username = viper.GetString("USERNAME")
	}

	// Password: flag > SAP_PASSWORD env (skip env if cookie auth via CLI)
	if cfg.Password == "" && !cookieAuthViaCLI {
		cfg.Password = viper.GetString("PASSWORD")
	}
	if cfg.Password == "" && !cookieAuthViaCLI {
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
		if pkgs := viper.GetStringSlice("ALLOWED_PACKAGES"); len(pkgs) > 0 {
			cfg.AllowedPackages = pkgs
		}
	}
}

func validateConfig() error {
	if cfg.BaseURL == "" {
		return fmt.Errorf("SAP URL is required. Use --url flag or SAP_URL environment variable")
	}

	// Validate mode
	if cfg.Mode != "focused" && cfg.Mode != "expert" {
		return fmt.Errorf("invalid mode: %s (must be 'focused' or 'expert')", cfg.Mode)
	}

	// Check if we have either basic auth or cookies will be processed
	// Cookies are checked later in processCookieAuth
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

	if authMethods > 1 {
		return fmt.Errorf("only one authentication method can be used at a time (basic auth, cookie-file, or cookie-string)")
	}

	if authMethods == 0 {
		return fmt.Errorf("authentication required. Use --user/--password, --cookie-file, or --cookie-string")
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

func main() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}
