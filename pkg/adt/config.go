// Package adt provides a Go client for SAP ABAP Development Tools (ADT) REST API.
package adt

import (
	"context"
	"crypto/tls"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// SessionType defines how the client manages server sessions.
type SessionType string

const (
	// SessionStateful maintains a server session via sap-contextid cookie.
	SessionStateful SessionType = "stateful"
	// SessionStateless does not persist sessions.
	SessionStateless SessionType = "stateless"
	// SessionKeep uses existing session if available, otherwise stateless.
	SessionKeep SessionType = "keep"
)

// Config holds the configuration for an ADT client connection.
type Config struct {
	// BaseURL is the SAP system URL (e.g., "https://vhcalnplci.dummy.nodomain:44300")
	BaseURL string
	// Username for SAP authentication
	Username string
	// Password for SAP authentication
	Password string
	// Client is the SAP client number (e.g., "001")
	Client string
	// Language for SAP session (e.g., "EN")
	Language string
	// InsecureSkipVerify disables TLS certificate verification
	InsecureSkipVerify bool
	// SessionType defines session management behavior
	SessionType SessionType
	// Timeout for HTTP requests
	Timeout time.Duration
	// Cookies for cookie-based authentication (alternative to basic auth)
	Cookies map[string]string
	// Verbose enables verbose logging
	Verbose bool
	// Safety defines protection parameters to prevent unintended modifications
	Safety SafetyConfig
	// Features controls optional feature detection and enablement
	Features FeatureConfig
	// TerminalID for debugger session (shared with SAP GUI for cross-tool debugging)
	TerminalID string

	// CTSProject and TransportTarget are what a request vsp creates is filed
	// under when the caller names neither: the CTS project (E070A
	// SAP_CTS_PROJECT) and the transport target. Empty leaves both to SAP.
	CTSProject      string
	TransportTarget string

	// Cache keeps successful GET responses for CacheTTL and hands them back
	// until a modifying request empties it. CacheStore is where they live;
	// nil means in memory, for the life of the process.
	Cache      bool
	CacheTTL   time.Duration
	CacheStore ResponseStore

	// ReauthFunc is called on 401 to re-authenticate (e.g., re-run SAML dance).
	// Returns fresh cookies for the SAP system. Only used when HasBasicAuth() is false.
	ReauthFunc func(ctx context.Context) (map[string]string, error)

	// ReauthTimeout caps one re-authentication attempt. Zero uses the default,
	// which suits a re-auth that runs unattended. Raise it where the flow may
	// stop to ask a human something — a browser sign-in with a second factor
	// takes far longer than any machine-to-machine handshake.
	ReauthTimeout time.Duration

	// ReauthReadOnly limits an externally refreshed credential source to a
	// safe, unlocked GET or HEAD retry. A new browser session cannot inherit an
	// ADT lock handle, and replaying a mutation after changing credentials leaves
	// its remote result unknowable. Cookie files opt into this narrow policy;
	// interactive SSO keeps its established recovery behaviour.
	ReauthReadOnly bool

	// ProxyContextIDGuard enables a workaround for session-holding proxy
	// chains such as the SAP Business Application Studio destination proxy
	// (HTTP_PROXY=127.0.0.1:8887 → secure-outbound-connectivity → BTP
	// destination → Cloud Connector). Verified against BAS: the chain keeps
	// the SAP sap-contextid itself — a live Set-Cookie for it never reaches
	// the client, deletion cookies do — and injects the stored context into
	// every request that carries no Cookie header. A stateless request served
	// in that context ends it on the SAP side, after which every following
	// request fails with ICMENOSESSION and the chain never recovers on its
	// own. The ICM honours the first sap-contextid in the Cookie header, so an
	// explicit empty "sap-contextid=" suppresses the injection. When enabled:
	// stateless requests carry that empty cookie (the stateful context
	// survives), the CSRF probe and every LOCK open a fresh stateful context
	// with it (the chain re-learns the live one from the response), and after
	// UNLOCK or DELETE a stateless probe without the cookie retires the context.
	// Also enabled via SAP_PROXY_CONTEXTID_GUARD=true.
	ProxyContextIDGuard bool

	// Expect pins the system, client and user this client must find on the
	// other end (see identity.go). Nil: no pin, no preflight, no extra request.
	Expect *IdentityPin
}

// Option is a functional option for configuring the ADT client.
type Option func(*Config)

// WithClient sets the SAP client number.
func WithClient(client string) Option {
	return func(c *Config) {
		c.Client = client
	}
}

// WithCache turns the response cache on. ttl 0 means DefaultCacheTTL.
func WithCache(ttl time.Duration) Option {
	return func(c *Config) {
		c.Cache = true
		c.CacheTTL = ttl
	}
}

// WithCacheStore turns the cache on with a store of the caller's choosing,
// such as pkg/cache's SQLite one that survives the process.
func WithCacheStore(store ResponseStore, ttl time.Duration) Option {
	return func(c *Config) {
		c.Cache = true
		c.CacheTTL = ttl
		c.CacheStore = store
	}
}

// WithLanguage sets the SAP session language.
func WithLanguage(lang string) Option {
	return func(c *Config) {
		c.Language = lang
	}
}

// WithInsecureSkipVerify disables TLS certificate verification.
func WithInsecureSkipVerify() Option {
	return func(c *Config) {
		c.InsecureSkipVerify = true
	}
}

// WithProxyContextIDGuard enables the session-holding-proxy workaround
// (see Config.ProxyContextIDGuard).
func WithProxyContextIDGuard() Option {
	return func(c *Config) {
		c.ProxyContextIDGuard = true
	}
}

// WithSessionType sets the session management behavior.
func WithSessionType(st SessionType) Option {
	return func(c *Config) {
		c.SessionType = st
	}
}

// WithTimeout sets the HTTP request timeout.
func WithTimeout(d time.Duration) Option {
	return func(c *Config) {
		c.Timeout = d
	}
}

// WithCookies sets cookies for cookie-based authentication.
func WithCookies(cookies map[string]string) Option {
	return func(c *Config) {
		c.Cookies = cookies
	}
}

// WithVerbose enables verbose logging.
func WithVerbose() Option {
	return func(c *Config) {
		c.Verbose = true
	}
}

// WithSafety sets the safety configuration.
func WithSafety(safety SafetyConfig) Option {
	return func(c *Config) {
		c.Safety = safety
	}
}

// WithReadOnly enables read-only mode (blocks all write operations).
func WithReadOnly() Option {
	return func(c *Config) {
		c.Safety.ReadOnly = true
	}
}

// WithBlockFreeSQL blocks execution of arbitrary SQL queries.
func WithBlockFreeSQL() Option {
	return func(c *Config) {
		c.Safety.BlockFreeSQL = true
	}
}

// WithAllowedPackages restricts operations to specific packages.
func WithAllowedPackages(packages ...string) Option {
	return func(c *Config) {
		c.Safety.AllowedPackages = packages
	}
}

// WithEnableTransports enables transport management operations.
// By default, transport operations are disabled - this flag explicitly enables them.
func WithEnableTransports() Option {
	return func(c *Config) {
		c.Safety.EnableTransports = true
	}
}

// WithTransportReadOnly allows only read operations on transports (list, get).
// Create, release, delete operations will be blocked.
func WithTransportReadOnly() Option {
	return func(c *Config) {
		c.Safety.TransportReadOnly = true
	}
}

// WithAllowedTransports restricts transport operations to specific transports.
// Supports wildcards: "A4HK*" matches all transports starting with A4HK.
func WithAllowedTransports(transports ...string) Option {
	return func(c *Config) {
		c.Safety.AllowedTransports = transports
	}
}

// WithTransportChoice sets how a write with no request named picks one:
// "auto" (the default) or "off".
func WithTransportChoice(mode string) Option {
	return func(c *Config) {
		c.Safety.TransportChoice = mode
	}
}

// WithCTSProject files every request vsp creates under this CTS project,
// unless the caller names another.
func WithCTSProject(project string) Option {
	return func(c *Config) {
		c.CTSProject = project
	}
}

// WithTransportTarget sets the target of every request vsp creates, unless the
// caller names another.
func WithTransportTarget(target string) Option {
	return func(c *Config) {
		c.TransportTarget = target
	}
}

// WithAllowTransportableEdits enables editing objects that require transport requests.
// By default, only local objects ($TMP, $* packages) can be edited.
// When enabled, users can provide transport parameters to EditSource/WriteSource.
// WARNING: This allows modifications to non-local objects that may affect production systems.
func WithAllowTransportableEdits() Option {
	return func(c *Config) {
		c.Safety.AllowTransportableEdits = true
	}
}

// HasBasicAuth returns true if username and password are configured.
func (c *Config) HasBasicAuth() bool {
	return c.Username != "" && c.Password != ""
}

// HasCookieAuth returns true if cookies are configured.
func (c *Config) HasCookieAuth() bool {
	return len(c.Cookies) > 0
}

// NewConfig creates a new Config with the given base URL, username, password,
// and optional configuration options.
func NewConfig(baseURL, username, password string, opts ...Option) *Config {
	cfg := &Config{
		BaseURL:     baseURL,
		Username:    username,
		Password:    password,
		Client:      "001",
		Language:    "EN",
		SessionType: SessionStateless,
		Timeout:     60 * time.Second,
		Safety:      UnrestrictedSafetyConfig(), // Default: no restrictions for backwards compatibility
		Features:    DefaultFeatureConfig(),     // Default: auto-detect all features
	}

	for _, opt := range opts {
		opt(cfg)
	}

	return cfg
}

// WithFeatures sets the feature configuration.
func WithFeatures(features FeatureConfig) Option {
	return func(c *Config) {
		c.Features = features
	}
}

// WithReauthFunc sets the re-authentication function for 401 recovery.
// Used by SAML auth to re-run the SAML dance when the session expires.
func WithReauthFunc(f func(ctx context.Context) (map[string]string, error)) Option {
	return func(c *Config) {
		c.ReauthFunc = f
	}
}

// WithReauthTimeout caps a single re-authentication attempt.
func WithReauthTimeout(d time.Duration) Option {
	return func(c *Config) {
		c.ReauthTimeout = d
	}
}

// WithReadOnlyReauth limits automatic session recovery to unlocked GET and
// HEAD requests. It is intended for credential sources that another process
// refreshes, such as --cookie-file.
func WithReadOnlyReauth() Option {
	return func(c *Config) {
		c.ReauthReadOnly = true
	}
}

// WithTerminalID sets the debugger terminal ID.
// Use the same ID as SAP GUI to enable cross-tool breakpoint sharing.
// SAP GUI stores this in: Windows Registry HKCU\Software\SAP\ABAP Debugging\TerminalID
// or on Linux/Mac: ~/.SAP/ABAPDebugging/terminalId
func WithTerminalID(terminalID string) Option {
	return func(c *Config) {
		c.TerminalID = terminalID
	}
}

// NewHTTPClient creates an http.Client configured for the given Config.
func (c *Config) NewHTTPClient() *http.Client {
	// One jar for the client's lifetime: session recovery empties it in place
	// (see Transport.resetCookieJar) rather than replacing client.Jar under
	// concurrent requests.
	jar := newResettableJar()

	transport := &http.Transport{
		Proxy: http.ProxyFromEnvironment, // Honor HTTP_PROXY/HTTPS_PROXY env vars
		TLSClientConfig: &tls.Config{
			InsecureSkipVerify: c.InsecureSkipVerify,
		},
	}

	client := &http.Client{
		Jar:       jar,
		Transport: transport,
		Timeout:   c.Timeout,
	}

	// Preserve ADT-critical headers across redirects.
	//
	// Go's default strips Authorization / WWW-Authenticate / Cookie / Cookie2
	// on cross-origin redirects per RFC 7235 §4.2 — SAP BTP/Cloud SAML flows
	// need Authorization back, otherwise the IdP dance drops it and the user
	// gets 401 even though curl works (issue #90).
	//
	// Custom headers like X-CSRF-Token and X-sap-adt-sessiontype are *not*
	// in Go's sensitive-headers list, so Go technically forwards them by
	// default. We re-set them explicitly anyway for two reasons:
	//   - defensive: guards against any Go version or middleware tweak that
	//     decides to strip custom headers on its own;
	//   - intent: makes it obvious in the code that these two headers are
	//     load-bearing for the lock→write→unlock ADT sequence. If either
	//     goes missing across a redirect, the second hop hits SAP with a
	//     fresh (stateless) session-type or a missing CSRF token, and the
	//     lock handle / mutation is rejected.
	// Only for hops that stay on the SAP host. Re-attaching unconditionally
	// sent Basic credentials and the session CSRF token to whatever host the
	// chain led to — and an expired session on an SSO system leads to the
	// identity provider, which is why redirectedAwayFromSAP exists.
	//
	// Off-host the headers are DELETED, not merely left unset. Go's own
	// makeHeadersCopier runs before CheckRedirect and copies every header that
	// is not on its sensitive list — Authorization, Www-Authenticate, Cookie
	// and Cookie2 are stripped cross-origin, X-CSRF-Token and
	// X-sap-adt-sessiontype are not. So declining to *set* them here left the
	// session's CSRF token going to the identity provider exactly as before;
	// only an explicit Del actually stops it.
	//
	// The other half of that ordering is why the same-host branch is nearly a
	// no-op: Go already preserves Authorization for a same-host or subdomain
	// hop, so the re-attach only ever added anything cross-origin — which is
	// now refused. A BTP SAML flow that genuinely needs Authorization on a
	// foreign host (issue #90's abap → abap-web hop) therefore no longer gets
	// it, and would need an explicit, named allowance for that one host rather
	// than a blanket "any host in the chain".
	//
	// The comparison is on the *hostname*, case-folded — not on host:port.
	// Two reasons, and they pull the same way:
	//   - `==` on the raw host made an ICM redirect that merely changed the
	//     case of the FQDN, or spelled out :443, look foreign, and the headers
	//     this handler exists to preserve were dropped on an intra-SAP hop.
	//   - the Del below must not be stricter than Go's own rule, which ignores
	//     the port entirely (shouldCopyHeaderOnRedirect compares hostnames).
	//     A box that answers on 44300 and redirects to 8443 is one machine;
	//     deleting Basic credentials there would break a hop that worked
	//     before this handler existed.
	//
	// The hostname alone is not enough, though: a hop from https to http on the
	// same host would send Basic credentials and the CSRF token in clear text.
	// So the scheme may never fall back — see keepsSAPCredentials. A port change
	// that stays on https, or climbs from http to https (the ICM's own HTTP
	// redirect), is still one machine and keeps its headers.
	//
	// redirectedAwayFromSAP (http.go) compares host:port with EqualFold, so it
	// is stricter on the port and identical on case; the difference only shows
	// on a port-changing hop, where this predicate is deliberately the looser
	// of the two.
	sapURL, _ := url.Parse(c.BaseURL)
	client.CheckRedirect = func(req *http.Request, via []*http.Request) error {
		if len(via) >= 10 {
			return fmt.Errorf("too many redirects")
		}
		if len(via) == 0 {
			return nil
		}
		if !keepsSAPCredentials(sapURL, req.URL) {
			req.Header.Del("Authorization")
			req.Header.Del("X-CSRF-Token")
			req.Header.Del("X-sap-adt-sessiontype")
			return nil
		}
		first := via[0]
		if auth := first.Header.Get("Authorization"); auth != "" {
			req.Header.Set("Authorization", auth)
		}
		if csrf := first.Header.Get("X-CSRF-Token"); csrf != "" {
			req.Header.Set("X-CSRF-Token", csrf)
		}
		if st := first.Header.Get("X-sap-adt-sessiontype"); st != "" {
			req.Header.Set("X-sap-adt-sessiontype", st)
		}
		return nil
	}

	return client
}

// keepsSAPCredentials reports whether a redirect target may still receive the
// Basic credentials, the CSRF token and the session type of the request that
// was sent to the SAP system at base: the same hostname (case-folded, port
// ignored) and no downgrade from https to http. An unparseable or
// scheme-less BaseURL has no hostname and keeps nothing — that holds the
// credentials-off-host rule; the alternative is to silently disable the whole
// handler.
func keepsSAPCredentials(base, target *url.URL) bool {
	if base == nil || target == nil || base.Hostname() == "" {
		return false
	}
	if !strings.EqualFold(base.Hostname(), target.Hostname()) {
		return false
	}
	return !strings.EqualFold(base.Scheme, "https") || strings.EqualFold(target.Scheme, "https")
}
