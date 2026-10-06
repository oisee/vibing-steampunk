package main

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/oisee/vibing-steampunk/pkg/cache"
	"os"
	"path/filepath"
	"strings"
	"time"

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

	// TransportCmd is the argv of a helper that carries every ADT request
	// over its stdin/stdout and authenticates on its own.
	TransportCmd []string

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

	// Transport safety. allow-transportable-edits can be set explicitly on a
	// CLI subcommand; the remaining settings currently come from system config
	// or the environment.
	EnableTransports        bool
	TransportReadOnly       bool
	AllowedTransports       []string
	AllowTransportableEdits bool
	TransportChoice         string
	BlockFreeSQL            bool

	// Where a request vsp creates is filed: CTS project and transport target.
	CTSProject      string
	TransportTarget string

	Cache     bool
	CachePath string

	// Expect pins the identity (SID[.CLIENT][/USER]); ExpectSource says where
	// the pin came from. Empty: no pin, and no preflight request.
	Expect       string
	ExpectSource string
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
		// A transport command authenticates on its own (GetSystem has
		// refused it next to any credential, and outside the home directory).
		hasCookieAuth := sys.CookieFile != "" || sys.CookieString != ""
		if sys.Password == "" && !hasCookieAuth && !sys.UsesSSO() && len(sys.TransportCmd) == 0 {
			return nil, fmt.Errorf("auth not found for system '%s'. Set VSP_%s_PASSWORD env var, use cookie_file/cookie_string, set \"auth\": \"sso\", or set transport_cmd in ~/.vsp.json", effectiveName, strings.ToUpper(effectiveName))
		}

		verbose, _ := cmd.Flags().GetBool("verbose")
		if verbose || os.Getenv("VSP_VERBOSE") == "true" || os.Getenv("VSP_DEBUG") == "true" {
			fmt.Fprintf(os.Stderr, "[INFO] Using system '%s' from %s\n", effectiveName, path)
			fmt.Fprintf(os.Stderr, "[DEBUG] URL: %s, User: %s\n", sys.URL, sys.User)
		}

		allowTransportableEdits, err := resolveAllowTransportableEdits(cmd, sys.AllowTransportableEdits)
		if err != nil {
			return nil, err
		}
		expect, expectSource := cliExpect(sys.Expect)
		if expectSource == ".vsp.json" {
			expectSource = fmt.Sprintf(".vsp.json system %q", effectiveName)
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
			TransportCmd:       sys.TransportCmd,
			Auth:               sys.Auth,
			SSO:                sys.SSO,
			TransportAttribute: sys.TransportAttribute,
			ReadOnly:           sys.ReadOnly,
			AllowedPackages:    sys.AllowedPackages,

			EnableTransports:        sys.EnableTransports || envFlag("SAP_ENABLE_TRANSPORTS"),
			TransportReadOnly:       sys.TransportReadOnly || envFlag("SAP_TRANSPORT_READ_ONLY"),
			AllowedTransports:       firstNonEmptyList(sys.AllowedTransports, splitList(os.Getenv("SAP_ALLOWED_TRANSPORTS"))),
			AllowTransportableEdits: allowTransportableEdits,
			TransportChoice:         firstNonEmpty(sys.TransportChoice, os.Getenv("SAP_TRANSPORT_CHOICE")),
			CTSProject:              firstNonEmpty(sys.CTSProject, os.Getenv("SAP_CTS_PROJECT")),
			TransportTarget:         firstNonEmpty(sys.TransportTarget, os.Getenv("SAP_TRANSPORT_TARGET")),
			BlockFreeSQL:            sys.BlockFreeSQL || envFlag("SAP_BLOCK_FREE_SQL"),
			Cache:                   sys.Cache,
			CachePath:               sys.CachePath,
			Expect:                  expect,
			ExpectSource:            expectSource,
		}, nil
	}

	// Fall back to environment variables
	url := os.Getenv("SAP_URL")
	if url == "" {
		return nil, fmt.Errorf("SAP_URL not set. Use --system flag, set \"default\" in .vsp.json, or set SAP_* env vars")
	}

	user := os.Getenv("SAP_USER")
	password := os.Getenv("SAP_PASSWORD")
	transportCmd, err := resolveTransportCmd(cmd)
	if err != nil {
		return nil, err
	}
	if len(transportCmd) > 0 {
		// The helper authenticates; credentials of ours are refused, not
		// silently dropped.
		if user != "" || password != "" {
			return nil, fmt.Errorf("%w (found: SAP_USER/SAP_PASSWORD)", adt.ErrTransportCmdAuth)
		}
	} else if user == "" || password == "" {
		return nil, fmt.Errorf("SAP_USER and SAP_PASSWORD required (or SAP_TRANSPORT_CMD)")
	}

	cacheEnabled := strings.EqualFold(os.Getenv("VSP_CACHE"), "true")
	cachePath := os.Getenv("VSP_CACHE_PATH")
	if cacheEnabled && cachePath == "" {
		cachePath = ".vsp-cache/default.db"
	}

	allowTransportableEdits, err := resolveAllowTransportableEdits(cmd, false)
	if err != nil {
		return nil, err
	}
	envExpect, envExpectSource := cliExpect("")

	return &systemParams{
		URL:                url,
		User:               user,
		Password:           password,
		TransportCmd:       transportCmd,
		Client:             getEnvOrDefault("SAP_CLIENT", "001"),
		Language:           getEnvOrDefault("SAP_LANGUAGE", "EN"),
		Insecure:           os.Getenv("SAP_INSECURE") == "true",
		TransportAttribute: resolveTransportAttributeFromEnv(),
		ReadOnly:           strings.EqualFold(os.Getenv("SAP_READ_ONLY"), "true"),
		AllowedPackages:    splitList(os.Getenv("SAP_ALLOWED_PACKAGES")),
		// The transport settings travel with the opt-in: without the
		// allowlist, SAP_ALLOW_TRANSPORTABLE_EDITS=true would accept any
		// transport in this mode.
		EnableTransports:        envFlag("SAP_ENABLE_TRANSPORTS"),
		TransportReadOnly:       envFlag("SAP_TRANSPORT_READ_ONLY"),
		AllowedTransports:       splitList(os.Getenv("SAP_ALLOWED_TRANSPORTS")),
		AllowTransportableEdits: allowTransportableEdits,
		TransportChoice:         os.Getenv("SAP_TRANSPORT_CHOICE"),
		CTSProject:              os.Getenv("SAP_CTS_PROJECT"),
		TransportTarget:         os.Getenv("SAP_TRANSPORT_TARGET"),
		BlockFreeSQL:            envFlag("SAP_BLOCK_FREE_SQL"),
		Cache:                   cacheEnabled,
		CachePath:               cachePath,
		Expect:                  envExpect,
		ExpectSource:            envExpectSource,
	}, nil
}

// resolveAllowTransportableEdits keeps this high-impact opt-in distinct from
// the older boolean config merges. A changed Cobra flag must retain false so a
// caller can explicitly disable an inherited SAP_ALLOW_TRANSPORTABLE_EDITS=true
// value. Environment values are parsed strictly rather than silently treating a
// typo as false. The default remains false.
//
// Priority: explicit CLI flag > SAP_ALLOW_TRANSPORTABLE_EDITS > system config >
// default false.
func resolveAllowTransportableEdits(cmd *cobra.Command, configured bool) (bool, error) {
	// A command built without the root's persistent flags (tests, embedded
	// callers) has no flag to consult; the environment and config still apply.
	flag := cmd.Flags().Lookup("allow-transportable-edits")
	if flag != nil && flag.Changed {
		value, err := cmd.Flags().GetBool("allow-transportable-edits")
		if err != nil {
			return false, fmt.Errorf("read --allow-transportable-edits: %w", err)
		}
		return value, nil
	}

	raw, set := os.LookupEnv("SAP_ALLOW_TRANSPORTABLE_EDITS")
	if !set || strings.TrimSpace(raw) == "" {
		return configured, nil
	}
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "true", "1", "yes", "on":
		return true, nil
	case "false", "0", "no", "off":
		return false, nil
	default:
		return false, fmt.Errorf("SAP_ALLOW_TRANSPORTABLE_EDITS must be true or false, got %q", raw)
	}
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
func firstNonEmpty(configured, fromEnv string) string {
	if strings.TrimSpace(configured) != "" {
		return configured
	}
	return fromEnv
}

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
// responseCacheTTL reads VSP_CACHE_TTL (a Go duration such as 10m); the default is
// adt.DefaultCacheTTL.
func responseCacheTTL() time.Duration {
	if raw := strings.TrimSpace(os.Getenv("VSP_CACHE_TTL")); raw != "" {
		if d, err := time.ParseDuration(raw); err == nil && d > 0 {
			return d
		}
	}
	return adt.DefaultCacheTTL
}

// lastClient is the client the command built, so the root command can report
// the cache's counters when asked to be verbose.
var lastClient *adt.Client

func getClient(params *systemParams) (*adt.Client, error) {
	client, err := buildClient(params)
	if err == nil {
		lastClient = client
	}
	return client, err
}

func buildClient(params *systemParams) (*adt.Client, error) {
	opts := []adt.Option{
		adt.WithClient(params.Client),
		adt.WithLanguage(params.Language),
	}

	// The identity pin. The user a password logon would send is checked here,
	// so a wrong one is refused before it ever reaches SAP; a cookie or single
	// sign-on session has no user name until the system says it.
	basicUser := params.User
	if params.UsesSSO() || params.CookieFile != "" || params.CookieString != "" || len(params.TransportCmd) > 0 {
		basicUser = ""
	}
	pinOpt, err := cliPinOption(params, basicUser)
	if err != nil {
		return nil, err
	}
	if pinOpt != nil {
		opts = append(opts, pinOpt)
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
	if params.TransportChoice != "" {
		safety.TransportChoice = params.TransportChoice
	}
	if params.AllowTransportableEdits {
		safety.AllowTransportableEdits = true
		restricted = true
	}
	if restricted {
		opts = append(opts, adt.WithSafety(safety))
	}
	if params.CTSProject != "" {
		opts = append(opts, adt.WithCTSProject(params.CTSProject))
	}
	if params.TransportTarget != "" {
		opts = append(opts, adt.WithTransportTarget(params.TransportTarget))
	}
	if params.Insecure {
		opts = append(opts, adt.WithInsecureSkipVerify())
	}
	// The response cache: GET answers kept for a while, dropped on any
	// write. In memory by default; on SQLite when a path is configured, so
	// the next CLI run starts warm.
	if params.Cache {
		ttl := responseCacheTTL()
		if params.CachePath != "" {
			store, err := cache.NewResponseStore(params.CachePath)
			if err != nil {
				return nil, err
			}
			opts = append(opts, adt.WithCacheStore(store, ttl))
		} else {
			opts = append(opts, adt.WithCache(ttl))
		}
	}

	// A transport command: the helper carries every request and
	// authenticates, so vsp sends no credentials of its own.
	if len(params.TransportCmd) > 0 {
		if params.User != "" || params.Password != "" || params.CookieFile != "" || params.CookieString != "" || params.UsesSSO() {
			return nil, adt.ErrTransportCmdAuth
		}
		opts = append(opts, adt.WithTransportCmd(params.TransportCmd))
		return adt.NewClient(params.URL, "", "", opts...), nil
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
		cookieFile, err := filepath.Abs(params.CookieFile)
		if err != nil {
			return nil, fmt.Errorf("resolving cookie file path: %w", err)
		}
		cookies, err := adt.LoadCookiesFromFile(cookieFile)
		if err != nil {
			return nil, fmt.Errorf("failed to load cookies from %s: %w", cookieFile, err)
		}
		if len(cookies) == 0 {
			return nil, fmt.Errorf("no cookies found in file: %s", cookieFile)
		}
		reauth, err := adt.NewCookieFileReauthFunc(cookieFile)
		if err != nil {
			return nil, err
		}
		opts = append(opts, adt.WithCookies(cookies), adt.WithReauthFunc(reauth), adt.WithReadOnlyReauth())
		return adt.NewClient(params.URL, "", "", opts...), nil
	}
	if params.CookieString != "" {
		cookies := adt.ParseCookieString(params.CookieString)
		opts = append(opts, adt.WithCookies(cookies))
		return adt.NewClient(params.URL, "", "", opts...), nil
	}

	return adt.NewClient(params.URL, params.User, params.Password, opts...), nil
}

// getWSClient creates an AMDP WebSocket client for GitExport, authenticated as
// the system's ADT client is: its cookie_file, cookie_string or single sign-on
// session, or its password when it has none.
//
// The ADT client exists only to derive the WebSocket's credentials, so it is
// built without the response cache: with a cache_path that would open a
// SQLite store nothing here uses or closes.
func getWSClient(ctx context.Context, params *systemParams) (*adt.AMDPWebSocketClient, error) {
	noCache := *params
	noCache.Cache, noCache.CachePath = false, ""
	client, err := buildClient(&noCache)
	if err != nil {
		return nil, err
	}
	// The WebSocket logs on by itself; the pin is checked over ADT first.
	if err := client.VerifyIdentity(ctx); err != nil {
		return nil, err
	}
	wsClient := client.NewAMDPWebSocketClient()
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

With --exact the query is a name, not a pattern: only objects whose name
equals it (case-insensitive) are listed, still filtered by --type and --max.
The name is sent without a wildcard, which the quick search matches whole.
On a release that reads it as a prefix, at most 1000 matches are read: a
full window is reported, as inconclusive when it held no equal name or as
possibly incomplete when it did — add --type.

Examples:
  vsp -s a4h search "ZCL_*"
  vsp search "Z*ORDER*" --type CLAS --max 50
  vsp search ZCL_ORDER --exact
  vsp search ZCL_ORDER --exact --type CLAS`,
	Args: cobra.ExactArgs(1),
	RunE: runSearch,
}

func init() {
	searchCmd.Flags().StringVarP(&objectType, "type", "t", "", "Filter by object type (CLAS, PROG, INTF, etc.)")
	searchCmd.Flags().IntVarP(&maxResults, "max", "m", 100, "Maximum results")
	searchCmd.Flags().Bool("exact", false, "Only objects whose name equals the query (case-insensitive, no wildcards); a full 1000-match window is reported as inconclusive or incomplete, so add --type")
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

	exact, _ := cmd.Flags().GetBool("exact")
	filtered, err := searchObjects(ctx, client, query, adtType, maxResults, exact)
	if err != nil {
		return err
	}

	// Output results
	fmt.Printf("Found %d objects:\n", len(filtered))
	for _, r := range filtered {
		fmt.Printf("  %-10s %-40s %s\n", r.Type, r.Name, r.PackageName)
	}

	return nil
}

// searchObjects runs the search command's query: by pattern, or with exact
// by name.
func searchObjects(ctx context.Context, client *adt.Client, query, adtType string, maxResults int, exact bool) ([]adt.SearchResult, error) {
	var results []adt.SearchResult
	var err error
	if exact {
		var incomplete string
		results, incomplete, err = client.SearchObjectExact(ctx, query, adtType, maxResults)
		if err == nil && incomplete != "" {
			fmt.Fprintf(os.Stderr, "Note: %s\n", incomplete)
		}
	} else {
		results, err = client.SearchObjectByType(ctx, query, adtType, maxResults)
	}
	if err != nil {
		return nil, fmt.Errorf("search failed: %w", err)
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

	return filtered, nil
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
	addSourceReadFlags(sourceCmd)
}

// addSourceReadFlags gives a command that runs runSource the flags it reads.
func addSourceReadFlags(cmds ...*cobra.Command) {
	for _, c := range cmds {
		c.Flags().String("parent", "", "Function group name (required for FUNC type)")
		c.Flags().String("include", "", "Class include type: definitions, implementations, macros, testclasses (CLAS only)")
		c.Flags().String("method", "", "Method name to retrieve only that METHOD...ENDMETHOD block (CLAS only)")
		c.Flags().Bool("summary", false, "Print JSON metadata instead of the source: lines, bytes, sha256 (exact text, not normalised), sourceHash, uri")
		c.Flags().String("if-none-match", "", "sha256 from an earlier --summary: if the source still has it, print \"unchanged: source sha256 ...\" instead of the source")
	}
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

	summary, _ := cmd.Flags().GetBool("summary")
	ifNoneMatch, _ := cmd.Flags().GetString("if-none-match")
	if strings.TrimSpace(ifNoneMatch) != "" {
		if ifNoneMatch, err = adt.ParseIfNoneMatch(ifNoneMatch); err != nil {
			return err
		}
	}

	opts := &adt.GetSourceOptions{
		Parent:  parent,
		Include: include,
		Method:  method,
	}

	ctx := context.Background()
	if summary || ifNoneMatch != "" {
		// Never from the response cache: see adt.WithFreshReads.
		ctx = adt.WithFreshReads(ctx)
	}
	source, readURI, err := client.GetSourceWithURI(ctx, objType, name, opts)
	if err != nil {
		return fmt.Errorf("failed to get source: %w", err)
	}

	out := cmd.OutOrStdout()
	if summary || ifNoneMatch != "" {
		sum := adt.SummarizeSource(objType, name, opts, readURI, source)
		unchanged := ifNoneMatch != "" && ifNoneMatch == sum.SHA256
		if summary {
			if ifNoneMatch != "" {
				sum.Unchanged = &unchanged
			}
			data, _ := json.MarshalIndent(sum, "", "  ")
			_, err := fmt.Fprintln(out, string(data))
			return err
		}
		if unchanged {
			_, err := fmt.Fprintln(out, adt.SourceUnchangedText(sum, false))
			return err
		}
	}

	_, err = fmt.Fprint(out, source)
	return err
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
		if len(sys.TransportCmd) > 0 {
			authStatus = "transport-cmd:" + adt.TransportCmdName(sys.TransportCmd)
		} else if sys.CookieFile != "" {
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
		if userInfo == "" && len(sys.TransportCmd) > 0 {
			userInfo = "(helper)"
		} else if userInfo == "" {
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
