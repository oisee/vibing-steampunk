package vsp

import (
	"context"
	"fmt"
	"os"
	"sort"
	"time"

	"github.com/oisee/vibing-steampunk/pkg/adt"
	"github.com/spf13/cobra"
)

func init() {
	ssoCmd.AddCommand(ssoLoginCmd, ssoRefreshCmd, ssoStatusCmd, ssoClearCmd)
	rootCmd.AddCommand(ssoCmd)
}

var ssoCmd = &cobra.Command{
	Use:   "sso",
	Short: "Manage browser single sign-on sessions",
	Long: `Manage the browser SSO sessions vsp uses to reach systems that have no
password to give — Entra/SAML, Kerberos, IAS and friends.

A system opts in with "auth": "sso" in .vsp.json. From then on vsp obtains a
session by itself: it reuses the cached one, and when the server stops accepting
it, captures a new one and retries. These commands exist for the moments around
that — the first sign-in, a deliberate refresh, and looking at what is cached.

Under WSL the browser step runs as a Windows process, because the credential
that proves the device (an Entra Primary Refresh Token) is held by Windows and
is invisible from Linux. Only the resulting cookies come back across.`,
}

var ssoLoginCmd = &cobra.Command{
	Use:   "login",
	Short: "Sign in through a visible browser window",
	Long: `Open a browser window and sign in, then cache the session.

Use this for a system's first sign-in, or when a silent refresh reports that the
identity provider wants a human — an expired device token, a second factor, a
consent screen.`,
	RunE: runSSOLogin,
}

var ssoRefreshCmd = &cobra.Command{
	Use:   "refresh",
	Short: "Capture a fresh session, silently if possible",
	Long: `Replace the cached session with a new one.

The capture runs without a window first. If the identity provider wants a human
and the system allows it (the default), a browser window opens; with
"on_expiry": "error" the command reports what to run instead.`,
	RunE: runSSORefresh,
}

var ssoStatusCmd = &cobra.Command{
	Use:   "status",
	Short: "Show the cached session and whether it still works",
	RunE:  runSSOStatus,
}

var ssoClearCmd = &cobra.Command{
	Use:   "clear",
	Short: "Delete the cached session",
	RunE:  runSSOClear,
}

func init() {
	ssoStatusCmd.Flags().Bool("check", false, "probe the SAP system to see whether the cached session is still accepted")
}

// ssoParams resolves the system and confirms it is configured for SSO. Every
// sso subcommand starts here, so the error for a mis-configured system is the
// same one wherever it is met.
func ssoParams(cmd *cobra.Command) (*systemParams, error) {
	params, err := resolveSystemParams(cmd)
	if err != nil {
		return nil, err
	}
	if !params.UsesSSO() {
		name := params.Name
		if name == "" {
			name = "this system"
		}
		return nil, fmt.Errorf("%s is not configured for SSO — add \"auth\": \"sso\" to its entry in .vsp.json", name)
	}
	return params, nil
}

func runSSOLogin(cmd *cobra.Command, args []string) error {
	params, err := ssoParams(cmd)
	if err != nil {
		return err
	}
	provider, err := newSSOProvider(params)
	if err != nil {
		return err
	}
	cookies, err := provider.Login(cmd.Context())
	if err != nil {
		return err
	}
	reportCapture(params, provider, cookies)
	return nil
}

func runSSORefresh(cmd *cobra.Command, args []string) error {
	params, err := ssoParams(cmd)
	if err != nil {
		return err
	}
	provider, err := newSSOProvider(params)
	if err != nil {
		return err
	}
	cookies, err := provider.Refresh(cmd.Context())
	if err != nil {
		return err
	}
	reportCapture(params, provider, cookies)
	return nil
}

// reportCapture prints what was captured. Cookie names are shown, values never:
// a session cookie authenticates as the user with no password, and a terminal
// is a place where things get scrolled back, copied and pasted into issues.
func reportCapture(params *systemParams, provider *adt.SSOProvider, cookies map[string]string) {
	names := make([]string, 0, len(cookies))
	for name := range cookies {
		names = append(names, name)
	}
	sort.Strings(names)

	fmt.Printf("Session captured for %s\n", params.URL)
	fmt.Printf("  cookies: %v\n", names)
	fmt.Printf("  cached:  %s\n", provider.CachePath())
}

func runSSOStatus(cmd *cobra.Command, args []string) error {
	params, err := ssoParams(cmd)
	if err != nil {
		return err
	}
	provider, err := newSSOProvider(params)
	if err != nil {
		return err
	}

	sess, err := adt.LoadSSOSession(provider.CachePath())
	if err != nil {
		return err
	}
	fmt.Printf("System:  %s (%s)\n", params.Name, params.URL)
	fmt.Printf("Cache:   %s\n", provider.CachePath())
	if sess == nil {
		fmt.Println("Session: none cached — run `vsp sso login`")
		return nil
	}

	names := make([]string, 0, len(sess.Cookies))
	for name := range sess.Cookies {
		names = append(names, name)
	}
	sort.Strings(names)
	fmt.Printf("Session: captured %s ago (%s)\n",
		sess.Age().Truncate(time.Second), sess.CapturedAt.Local().Format(time.RFC3339))
	fmt.Printf("Cookies: %v\n", names)

	if check, _ := cmd.Flags().GetBool("check"); check {
		// A cached session says nothing about whether the server still honours
		// it — only the server does. Ask it.
		client, err := getClient(params)
		if err != nil {
			return err
		}
		ctx, cancel := context.WithTimeout(cmd.Context(), 30*time.Second)
		defer cancel()
		if _, err := client.GetSystemInfo(ctx); err != nil {
			fmt.Printf("Live:    no — %v\n", err)
			return nil
		}
		fmt.Println("Live:    yes")
	}
	return nil
}

func runSSOClear(cmd *cobra.Command, args []string) error {
	params, err := ssoParams(cmd)
	if err != nil {
		return err
	}
	provider, err := newSSOProvider(params)
	if err != nil {
		return err
	}
	if err := provider.Clear(); err != nil {
		return err
	}
	fmt.Printf("Cleared %s\n", provider.CachePath())
	return nil
}

// UsesSSO reports whether this system authenticates through browser SSO.
func (p *systemParams) UsesSSO() bool {
	return p != nil && (p.Auth == "sso" || p.Auth == "SSO")
}

// newSSOProvider builds the SSO provider for a system, translating the config
// block into the client-level settings.
func newSSOProvider(params *systemParams) (*adt.SSOProvider, error) {
	cfg := adt.SSOConfig{
		System:      params.Name,
		BaseURL:     params.URL,
		Client:      params.Client,
		Insecure:    params.Insecure,
		Interactive: params.SSO.InteractiveOnExpiry(),
		Verbose:     os.Getenv("VSP_VERBOSE") == "true" || os.Getenv("VSP_DEBUG") == "true",
	}
	if s := params.SSO; s != nil {
		cfg.TriggerURL = s.TriggerURL
		cfg.Profile = s.Profile
		cfg.HelperPath = s.Helper

		var err error
		if cfg.SilentTimeout, err = parseOptionalDuration(s.SilentTimeout, "sso.silent_timeout"); err != nil {
			return nil, err
		}
		if cfg.InteractiveTimeout, err = parseOptionalDuration(s.InteractiveTimeout, "sso.interactive_timeout"); err != nil {
			return nil, err
		}
	}
	return adt.NewSSOProvider(cfg)
}

// parseOptionalDuration parses a duration that may be absent.
func parseOptionalDuration(value, field string) (time.Duration, error) {
	if value == "" {
		return 0, nil
	}
	d, err := time.ParseDuration(value)
	if err != nil {
		return 0, fmt.Errorf("%s: %q is not a duration (try \"45s\" or \"5m\")", field, value)
	}
	return d, nil
}
