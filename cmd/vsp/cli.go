package main

import (
	"context"
	"crypto/tls"
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
	Name         string // resolved system name ("" when using bare SAP_* env vars)
	URL          string
	User         string
	Password     string
	Client       string
	Language     string
	Insecure     bool
	CookieFile   string
	CookieString string
	ClientCert   *tls.Certificate // mTLS client cert (macOS keychain), no password

	// Auth names the authentication method ("sso" for browser single sign-on).
	Auth string
	// SSO carries this system's single sign-on settings, if any.
	SSO *config.SSOSettings

	TransportAttribute string

	// Safety, as declared for this system. The CLI used to drop these on the
	// floor: a system marked read_only in .vsp.json was fully writable from
	// every subcommand, because only the MCP server ever applied a safety
	// config to its client.
	ReadOnly        bool
	AllowedPackages []string

	// Transport safety. The command line reaches these only through the system
	// config or the environment; the equivalent flags live on the root command
	// and are rejected by every subcommand.
	EnableTransports        bool
	TransportReadOnly       bool
	AllowedTransports       []string
	AllowTransportableEdits bool
	BlockFreeSQL            bool

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

		// Require some way to authenticate. An SSO system needs no stored
		// credential at all: the browser handshake produces one on demand.
		hasCookieAuth := sys.CookieFile != "" || sys.CookieString != ""
		if sys.Password == "" && !hasCookieAuth && !sys.UsesSSO() {
			return nil, fmt.Errorf("auth not found for system '%s'. Set VSP_%s_PASSWORD env var, use cookie_file/cookie_string, or set \"auth\": \"sso\"", effectiveName, strings.ToUpper(effectiveName))
		}

		verbose, _ := cmd.Flags().GetBool("verbose")
		if verbose || os.Getenv("VSP_VERBOSE") == "true" || os.Getenv("VSP_DEBUG") == "true" {
			fmt.Fprintf(os.Stderr, "[INFO] Using system '%s' from %s\n", effectiveName, path)
			fmt.Fprintf(os.Stderr, "[DEBUG] URL: %s, User: %s\n", sys.URL, sys.User)
		}

		return &systemParams{
			Name:               effectiveName,
			URL:                sys.URL,
			User:               sys.User,
			Password:           sys.Password,
			Client:             sys.Client,
			Language:           sys.Language,
			Insecure:           sys.Insecure,
			CookieFile:         sys.CookieFile,
			CookieString:       sys.CookieString,
			Auth:               sys.Auth,
			SSO:                sys.SSO,
			TransportAttribute: sys.TransportAttribute,
			ReadOnly:           sys.ReadOnly,
			AllowedPackages:    sys.AllowedPackages,

			EnableTransports:        sys.EnableTransports || envFlag("SAP_ENABLE_TRANSPORTS"),
			TransportReadOnly:       sys.TransportReadOnly || envFlag("SAP_TRANSPORT_READ_ONLY"),
			AllowedTransports:       firstNonEmptyList(sys.AllowedTransports, splitList(os.Getenv("SAP_ALLOWED_TRANSPORTS"))),
			AllowTransportableEdits: sys.AllowTransportableEdits || envFlag("SAP_ALLOW_TRANSPORTABLE_EDITS"),
			BlockFreeSQL:            sys.BlockFreeSQL || envFlag("SAP_BLOCK_FREE_SQL"),
			Cache:                   sys.Cache,
			CachePath:               sys.CachePath,
		}, nil
	}

	// Fall back to environment variables
	url := os.Getenv("SAP_URL")
	if url == "" {
		return nil, fmt.Errorf("SAP_URL not set. Use --system flag, set \"default\" in .vsp.json, or set SAP_* env vars")
	}

	user := os.Getenv("SAP_USER")
	password := os.Getenv("SAP_PASSWORD")

	// mTLS: SAP_CLIENT_CERT_CN (by CN) or SAP_CLIENT_CERT_ISSUER (freshest valid
	// cert from that CA) loads a macOS keychain identity — no password.
	var clientCert *tls.Certificate
	if certCN := os.Getenv("SAP_CLIENT_CERT_CN"); certCN != "" {
		c, err := adt.LoadKeychainClientCert(certCN)
		if err != nil {
			return nil, fmt.Errorf("client cert (CN=%s): %w", certCN, err)
		}
		clientCert = c
	} else if certIss := os.Getenv("SAP_CLIENT_CERT_ISSUER"); certIss != "" {
		c, err := adt.LoadKeychainClientCertByIssuers(splitTrimCLI(certIss))
		if err != nil {
			return nil, fmt.Errorf("client cert (issuer=%s): %w", certIss, err)
		}
		clientCert = c
	} else if user == "" || password == "" {
		return nil, fmt.Errorf("SAP_USER and SAP_PASSWORD required (or SAP_CLIENT_CERT_CN / SAP_CLIENT_CERT_ISSUER for mTLS)")
	}
	if clientCert != nil && user == "" && clientCert.Leaf != nil {
		user = clientCert.Leaf.Subject.CommonName // effective SAP user = cert CN
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
		ClientCert:         clientCert,
		Client:             getEnvOrDefault("SAP_CLIENT", "001"),
		Language:           getEnvOrDefault("SAP_LANGUAGE", "EN"),
		Insecure:           os.Getenv("SAP_INSECURE") == "true",
		TransportAttribute: resolveTransportAttributeFromEnv(),
		ReadOnly:           strings.EqualFold(os.Getenv("SAP_READ_ONLY"), "true"),
		AllowedPackages:    splitList(os.Getenv("SAP_ALLOWED_PACKAGES")),
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

// envFlag reads a boolean environment variable, accepting the spellings people
// actually type.
func envFlag(name string) bool {
	switch strings.ToLower(strings.TrimSpace(os.Getenv(name))) {
	case "true", "1", "yes", "on":
		return true
	}
	return false
}

// firstNonEmptyList returns the configured list, falling back to the environment.
func firstNonEmptyList(configured, fromEnv []string) []string {
	if len(configured) > 0 {
		return configured
	}
	return fromEnv
}

// splitList parses a comma-separated environment value into a list.
func splitList(v string) []string {
	var out []string
	for _, item := range strings.Split(v, ",") {
		if item = strings.TrimSpace(item); item != "" {
			out = append(out, item)
		}
	}
	return out
}

// getClient creates an ADT client from system params.
func getClient(params *systemParams) (*adt.Client, error) {
	opts := []adt.Option{
		adt.WithClient(params.Client),
		adt.WithLanguage(params.Language),
	}

	// Carry the system's declared safety into the client. Without this a
	// read_only system is only read-only when the MCP server is talking; every
	// CLI subcommand wrote happily, which is the opposite of what the setting
	// says and the opposite of what a careful person would assume.
	safety := adt.UnrestrictedSafetyConfig()
	restricted := false
	if params.ReadOnly {
		safety.ReadOnly, restricted = true, true
	}
	if len(params.AllowedPackages) > 0 {
		safety.AllowedPackages, restricted = params.AllowedPackages, true
	}
	if params.BlockFreeSQL {
		safety.BlockFreeSQL, restricted = true, true
	}
	// Transport safety is opt-in, so enabling it is not a restriction — but it
	// still has to reach the client, or the transport commands stay blocked no
	// matter how the system is configured.
	if params.EnableTransports {
		safety.EnableTransports = true
		restricted = true
	}
	if params.TransportReadOnly {
		safety.TransportReadOnly, restricted = true, true
	}
	if len(params.AllowedTransports) > 0 {
		safety.AllowedTransports, restricted = params.AllowedTransports, true
	}
	if params.AllowTransportableEdits {
		safety.AllowTransportableEdits = true
		restricted = true
	}
	if restricted {
		opts = append(opts, adt.WithSafety(safety))
	}
	if params.Insecure {
		opts = append(opts, adt.WithInsecureSkipVerify())
	}
	if params.ClientCert != nil {
		opts = append(opts, adt.WithClientCert(params.ClientCert))
	}

	// Browser single sign-on: cookies are fetched on demand and refreshed
	// automatically, so this is checked before the static cookie sources.
	if params.UsesSSO() {
		provider, err := newSSOProvider(params)
		if err != nil {
			return nil, err
		}
		cookies, err := provider.Cookies(context.Background())
		if err != nil {
			return nil, err
		}
		opts = append(opts,
			adt.WithCookies(cookies),
			// The HTTP layer calls this when a request comes back
			// unauthenticated, then retries. That is what keeps a long-running
			// session alive across cookie expiry without anyone intervening.
			adt.WithReauthFunc(provider.Refresh),
			// A recovery that may open a sign-in window has to outlast the
			// person using it; the default budget assumes nobody is asked
			// anything.
			adt.WithReauthTimeout(provider.ReauthBudget()),
		)
		return adt.NewClient(params.URL, "", "", opts...), nil
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

// systemCookies returns the browser session a system authenticates with, if it
// uses one. A system on a password has none, and that is not an error.
func systemCookies(ctx context.Context, params *systemParams) (map[string]string, error) {
	switch {
	case params.UsesSSO():
		provider, err := newSSOProvider(params)
		if err != nil {
			return nil, err
		}
		return provider.Cookies(ctx)
	case params.CookieFile != "":
		return adt.LoadCookiesFromFile(params.CookieFile)
	case params.CookieString != "":
		return adt.ParseCookieString(params.CookieString), nil
	}
	return nil, nil
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
	if params.ClientCert != nil {
		wsClient.SetClientCert(params.ClientCert)
	}

	// A system reached through single sign-on has no password to offer, and the
	// upgrade request carries a cookie as readily as any other.
	cookies, err := systemCookies(ctx, params)
	if err != nil {
		return nil, err
	}
	if len(cookies) > 0 {
		wsClient.SetCookies(cookies)
	}

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

	adtType := adt.CanonicalObjectType(objectType)
	if v, _ := cmd.Flags().GetBool("verbose"); v {
		fmt.Fprintf(os.Stderr, "[DEBUG] search: query=%q objectType=%q maxResults=%d\n",
			query, adtType, maxResults)
	}

	results, err := client.SearchObjectByType(ctx, query, adtType, maxResults)
	if err != nil {
		return fmt.Errorf("search failed: %w", err)
	}

	// Filter by type if specified. Compare against the canonical type, since
	// the server returns canonical codes (e.g. FUNC -> FUGR/FF, INCL -> PROG/I)
	// where the short form is not a prefix of the result type.
	filtered := results
	if adtType != "" {
		filtered = make([]adt.SearchResult, 0)
		for _, r := range results {
			if strings.EqualFold(r.Type, adtType) || strings.HasPrefix(r.Type, adtType+"/") {
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

// splitTrimCLI splits a comma-separated issuer list (see SAP_CLIENT_CERT_ISSUER).
func splitTrimCLI(s string) []string {
	var out []string
	for _, part := range strings.Split(s, ",") {
		if p := strings.TrimSpace(part); p != "" {
			out = append(out, p)
		}
	}
	return out
}
