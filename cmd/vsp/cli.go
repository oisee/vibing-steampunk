package main

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/oisee/vibing-steampunk/pkg/adt"
	"github.com/oisee/vibing-steampunk/pkg/config"
	"github.com/spf13/cobra"
)

var (
	systemName string
	outputFile string
	objectType string
	maxResults int
)

func init() {
	// Add persistent --system flag to root command
	rootCmd.PersistentFlags().StringVarP(&systemName, "system", "s", "", "System name from config (e.g., 'a4h')")

	// Add CLI subcommands
	rootCmd.AddCommand(exportCmd)
	rootCmd.AddCommand(searchCmd)
	rootCmd.AddCommand(sourceCmd)
	rootCmd.AddCommand(systemsCmd)
}

// systemParams holds resolved system parameters.
type systemParams struct {
	URL          string
	User         string
	Password     string
	Client       string
	Language     string
	Insecure     bool
	CookieFile   string
	CookieString string

	TransportAttribute string

	Cache     bool
	CachePath string
}

// resolveSystemParams resolves system parameters from --system flag or env vars.
func resolveSystemParams(cmd *cobra.Command) (*systemParams, error) {
	// Debug: show which system is being used
	verbose, _ := cmd.Flags().GetBool("verbose")
	if verbose || os.Getenv("VSP_DEBUG") == "true" {
		fmt.Fprintf(os.Stderr, "[DEBUG] resolveSystemParams: systemName=%q\n", systemName)
	}

	// Resolve effective system name: --system flag > .vsp.json default
	effectiveName := systemName
	if effectiveName == "" {
		if cfg, _, err := config.LoadSystems(); err == nil && cfg != nil && cfg.Default != "" {
			effectiveName = cfg.Default
			if verbose || os.Getenv("VSP_DEBUG") == "true" {
				fmt.Fprintf(os.Stderr, "[DEBUG] No --system flag, using default '%s' from .vsp.json\n", effectiveName)
			}
		}
	}

	// If we have a system name (explicit or default), load from systems config
	if effectiveName != "" {
		cfg, path, err := config.LoadSystems()
		if err != nil {
			return nil, fmt.Errorf("failed to load systems config: %w", err)
		}
		if cfg == nil {
			return nil, fmt.Errorf("no systems config found. Create .vsp.json or ~/.vsp.json\n\nExample:\n%s", config.ExampleConfig())
		}

		sys, err := cfg.GetSystem(effectiveName)
		if err != nil {
			return nil, err
		}

		// Require either password or cookie auth
		hasCookieAuth := sys.CookieFile != "" || sys.CookieString != ""
		if sys.Password == "" && !hasCookieAuth {
			return nil, fmt.Errorf("auth not found for system '%s'. Set VSP_%s_PASSWORD env var or use cookie_file/cookie_string", effectiveName, strings.ToUpper(effectiveName))
		}

		verbose, _ := cmd.Flags().GetBool("verbose")
		if verbose || os.Getenv("VSP_VERBOSE") == "true" || os.Getenv("VSP_DEBUG") == "true" {
			fmt.Fprintf(os.Stderr, "[INFO] Using system '%s' from %s\n", effectiveName, path)
			fmt.Fprintf(os.Stderr, "[DEBUG] URL: %s, User: %s\n", sys.URL, sys.User)
		}

		return &systemParams{
			URL:                sys.URL,
			User:               sys.User,
			Password:           sys.Password,
			Client:             sys.Client,
			Language:           sys.Language,
			Insecure:           sys.Insecure,
			CookieFile:         sys.CookieFile,
			CookieString:       sys.CookieString,
			TransportAttribute: sys.TransportAttribute,
			Cache:              sys.Cache,
			CachePath:          sys.CachePath,
		}, nil
	}

	// Fall back to environment variables
	url := os.Getenv("SAP_URL")
	if url == "" {
		return nil, fmt.Errorf("SAP_URL not set. Use --system flag, set \"default\" in .vsp.json, or set SAP_* env vars")
	}

	user := os.Getenv("SAP_USER")
	password := os.Getenv("SAP_PASSWORD")
	if user == "" || password == "" {
		return nil, fmt.Errorf("SAP_USER and SAP_PASSWORD required")
	}

	cacheEnabled := strings.EqualFold(os.Getenv("VSP_CACHE"), "true")
	cachePath := os.Getenv("VSP_CACHE_PATH")
	if cacheEnabled && cachePath == "" {
		cachePath = ".vsp-cache/default.db"
	}

	return &systemParams{
		URL:                url,
		User:               user,
		Password:           password,
		Client:             getEnvOrDefault("SAP_CLIENT", "001"),
		Language:           getEnvOrDefault("SAP_LANGUAGE", "EN"),
		Insecure:           os.Getenv("SAP_INSECURE") == "true",
		TransportAttribute: resolveTransportAttributeFromEnv(),
		Cache:              cacheEnabled,
		CachePath:          cachePath,
	}, nil
}

func resolveTransportAttributeFromEnv() string {
	if v := strings.TrimSpace(os.Getenv("VSP_TRANSPORT_ATTRIBUTE")); v != "" {
		return strings.ToUpper(v)
	}
	return ""
}

// getClient creates an ADT client from system params.
func getClient(params *systemParams) (*adt.Client, error) {
	opts := []adt.Option{
		adt.WithClient(params.Client),
		adt.WithLanguage(params.Language),
	}
	if params.Insecure {
		opts = append(opts, adt.WithInsecureSkipVerify())
	}

	// Use cookie auth if available
	if params.CookieFile != "" {
		cookies, err := adt.LoadCookiesFromFile(params.CookieFile)
		if err != nil {
			return nil, fmt.Errorf("failed to load cookies from %s: %w", params.CookieFile, err)
		}
		opts = append(opts, adt.WithCookies(cookies))
		return adt.NewClient(params.URL, "", "", opts...), nil
	}
	if params.CookieString != "" {
		cookies := adt.ParseCookieString(params.CookieString)
		opts = append(opts, adt.WithCookies(cookies))
		return adt.NewClient(params.URL, "", "", opts...), nil
	}

	return adt.NewClient(params.URL, params.User, params.Password, opts...), nil
}

// getWSClient creates an AMDP WebSocket client for GitExport.
func getWSClient(ctx context.Context, params *systemParams) (*adt.AMDPWebSocketClient, error) {
	// NewAMDPWebSocketClient(baseURL, client, user, password, insecure)
	wsClient := adt.NewAMDPWebSocketClient(
		params.URL,
		params.Client,
		params.User,
		params.Password,
		params.Insecure,
	)

	if err := wsClient.Connect(ctx); err != nil {
		return nil, fmt.Errorf("failed to connect WebSocket: %w", err)
	}

	return wsClient, nil
}

func getEnvOrDefault(key, defaultVal string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return defaultVal
}

// --- export command ---

var exportCmd = &cobra.Command{
	Use:   "export <packages...>",
	Short: "Export packages to ZIP (abapGit format)",
	Long: `Export one or more packages to a ZIP file in abapGit-compatible format.

Examples:
  vsp -s a4h export '$ZORK' '$ZLLM' -o packages.zip
  vsp export '$TMP' --output my-package.zip
  vsp -s dev export 'Z*' --subpackages`,
	Args: cobra.MinimumNArgs(1),
	RunE: runExport,
}

func init() {
	exportCmd.Flags().StringVarP(&outputFile, "output", "o", "export.zip", "Output ZIP file path")
	exportCmd.Flags().BoolP("subpackages", "r", true, "Include subpackages")
}

func runExport(cmd *cobra.Command, args []string) error {
	params, err := resolveSystemParams(cmd)
	if err != nil {
		return err
	}

	ctx := context.Background()
	wsClient, err := getWSClient(ctx, params)
	if err != nil {
		return err
	}
	defer wsClient.Close()

	includeSubpackages, _ := cmd.Flags().GetBool("subpackages")

	fmt.Fprintf(os.Stderr, "Exporting packages: %s\n", strings.Join(args, ", "))

	zipData, result, err := wsClient.GitExportToBytes(ctx, adt.GitExportParams{
		Packages:           args,
		IncludeSubpackages: includeSubpackages,
	})
	if err != nil {
		return fmt.Errorf("export failed: %w", err)
	}

	if err := os.WriteFile(outputFile, zipData, 0644); err != nil {
		return fmt.Errorf("failed to write ZIP file: %w", err)
	}

	fmt.Printf("Exported %d objects to %s (%d bytes)\n", result.ObjectCount, outputFile, len(zipData))
	return nil
}

// --- search command ---

var searchCmd = &cobra.Command{
	Use:   "search <query>",
	Short: "Search for ABAP objects",
	Long: `Search for ABAP objects by name pattern.

Examples:
  vsp -s a4h search "ZCL_*"
  vsp search "Z*ORDER*" --type CLAS --max 50`,
	Args: cobra.ExactArgs(1),
	RunE: runSearch,
}

func init() {
	searchCmd.Flags().StringVarP(&objectType, "type", "t", "", "Filter by object type (CLAS, PROG, INTF, etc.)")
	searchCmd.Flags().IntVarP(&maxResults, "max", "m", 100, "Maximum results")
}

func runSearch(cmd *cobra.Command, args []string) error {
	params, err := resolveSystemParams(cmd)
	if err != nil {
		return err
	}

	client, err := getClient(params)
	if err != nil {
		return err
	}
	query := args[0]
	ctx := context.Background()

	results, err := client.SearchObject(ctx, query, maxResults)
	if err != nil {
		return fmt.Errorf("search failed: %w", err)
	}

	// Filter by type if specified
	filtered := results
	if objectType != "" {
		filtered = make([]adt.SearchResult, 0)
		for _, r := range results {
			if strings.EqualFold(r.Type, objectType) || strings.HasPrefix(r.Type, objectType+"/") {
				filtered = append(filtered, r)
			}
		}
	}

	// Output results
	fmt.Printf("Found %d objects:\n", len(filtered))
	for _, r := range filtered {
		fmt.Printf("  %-10s %-40s %s\n", r.Type, r.Name, r.PackageName)
	}

	return nil
}

// --- source command ---

var sourceCmd = &cobra.Command{
	Use:   "source [type] [name]",
	Short: "Get ABAP source code",
	Long: `Retrieve source code for an ABAP object.

Subcommands:
  read     Read source code (same as 'vsp source <type> <name>')
  write    Write source code from stdin
  edit     Surgical string replacement
  context  Source with compressed dependency contracts

Examples:
  vsp -s a4h source CLAS ZCL_MY_CLASS
  vsp source PROG ZTEST_PROGRAM
  vsp source read CLAS ZCL_MY_CLASS
  vsp source write CLAS ZCL_FOO < file.abap
  vsp source edit CLAS ZCL_FOO --old "old" --new "new"
  vsp source context CLAS ZCL_FOO`,
	Args: cobra.ArbitraryArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		if len(args) == 2 {
			return runSource(cmd, args)
		}
		return cmd.Help()
	},
}

func init() {
	sourceCmd.Flags().String("parent", "", "Function group name (required for FUNC type)")
	sourceCmd.Flags().String("include", "", "Class include type: definitions, implementations, macros, testclasses (CLAS only)")
	sourceCmd.Flags().String("method", "", "Method name to retrieve only that METHOD...ENDMETHOD block (CLAS only)")
}

func runSource(cmd *cobra.Command, args []string) error {
	params, err := resolveSystemParams(cmd)
	if err != nil {
		return err
	}

	client, err := getClient(params)
	if err != nil {
		return err
	}
	objType := strings.ToUpper(args[0])
	name := strings.ToUpper(args[1])

	parent, _ := cmd.Flags().GetString("parent")
	include, _ := cmd.Flags().GetString("include")
	method, _ := cmd.Flags().GetString("method")

	opts := &adt.GetSourceOptions{
		Parent:  parent,
		Include: include,
		Method:  method,
	}

	ctx := context.Background()
	source, err := client.GetSource(ctx, objType, name, opts)
	if err != nil {
		return fmt.Errorf("failed to get source: %w", err)
	}

	fmt.Print(source)
	return nil
}

// --- systems command ---

var systemsCmd = &cobra.Command{
	Use:   "systems",
	Short: "List configured systems",
	Long: `List all configured SAP systems from the systems config file.

Config file locations (searched in order):
  .vsp-systems.json
  .vsp/systems.json
  ~/.vsp-systems.json
  ~/.vsp/systems.json`,
	RunE: runSystems,
}

func init() {
	systemsCmd.AddCommand(systemsInitCmd)
}

func runSystems(cmd *cobra.Command, args []string) error {
	cfg, path, err := config.LoadSystems()
	if err != nil {
		return err
	}

	if cfg == nil {
		fmt.Println("No systems config found.")
		fmt.Println("\nCreate .vsp-systems.json with:")
		fmt.Println(config.ExampleConfig())
		return nil
	}

	fmt.Printf("Config: %s\n\n", path)
	fmt.Println("Systems:")
	for name, sys := range cfg.Systems {
		defaultMark := ""
		if name == cfg.Default {
			defaultMark = " (default)"
		}

		// Determine auth method
		authStatus := ""
		if sys.CookieFile != "" {
			authStatus = fmt.Sprintf("cookie-file:%s", sys.CookieFile)
		} else if sys.CookieString != "" {
			authStatus = "cookie-string:***"
		} else {
			// Password auth
			if sys.Password != "" {
				authStatus = "pwd:inline"
			} else if os.Getenv(fmt.Sprintf("VSP_%s_PASSWORD", strings.ToUpper(name))) != "" {
				authStatus = "pwd:env ✓"
			} else {
				authStatus = "pwd:env ✗"
			}
		}

		userInfo := sys.User
		if userInfo == "" {
			userInfo = "(cookie)"
		}
		fmt.Printf("  %-12s %s [%s@%s] %s%s\n", name, sys.URL, userInfo, sys.Client, authStatus, defaultMark)
	}

	return nil
}

var systemsInitCmd = &cobra.Command{
	Use:   "init",
	Short: "Create example systems config",
	RunE: func(cmd *cobra.Command, args []string) error {
		configPath := ".vsp-systems.json"
		if _, err := os.Stat(configPath); err == nil {
			return fmt.Errorf("%s already exists", configPath)
		}

		if err := os.WriteFile(configPath, []byte(config.ExampleConfig()), 0600); err != nil {
			return err
		}

		fmt.Printf("Created %s\n", configPath)
		fmt.Println("\nEdit the file to add your SAP systems.")
		fmt.Println("Set passwords via environment variables: VSP_<SYSTEM>_PASSWORD")
		return nil
	},
}
