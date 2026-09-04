// Package adt provides a Go client for SAP ABAP Development Tools (ADT) REST API.
package adt

import (
	"context"
	"crypto/tls"
	"fmt"
	"net/http"
	"net/http/cookiejar"
	"time"
	"sync"
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

	// ReauthFunc is called on 401 to re-authenticate (e.g., re-run SAML dance).
	// Returns fresh cookies for the SAP system. Only used when HasBasicAuth() is false.
	ReauthFunc func(ctx context.Context) (map[string]string, error)

	// ReauthTimeout caps one re-authentication attempt. Zero uses the default,
	// which suits a re-auth that runs unattended. Raise it where the flow may
	// stop to ask a human something — a browser sign-in with a second factor
	// takes far longer than any machine-to-machine handshake.
	ReauthTimeout time.Duration

	// ClientCert, when set, is presented for TLS mutual authentication (mTLS)
	// instead of a password. On macOS it is loaded from the keychain
	// (LoadKeychainClientCert) so the private key never leaves the keystore.
	ClientCert *tls.Certificate

	// ClientCertProvider, when set, resolves the client certificate lazily at
	// each TLS handshake (takes precedence over ClientCert). This keeps the
	// server alive when no cert is available yet (SLC not logged in) and picks
	// up a fresh cert after an SLC re-login without a restart.
	ClientCertProvider func() (*tls.Certificate, error)
}

// tlsClientConfig builds the TLS config for outbound connections, adding the
// client certificate for mTLS when configured. When a client cert is present
// the max TLS version is pinned to 1.2 — the NetWeaver 7.50 ICM drops TLS 1.3
// client-certificate handshakes.
func (c *Config) tlsClientConfig() *tls.Config {
	t := &tls.Config{InsecureSkipVerify: c.InsecureSkipVerify}
	if c.ClientCertProvider != nil {
		provider := c.ClientCertProvider
		t.GetClientCertificate = func(*tls.CertificateRequestInfo) (*tls.Certificate, error) {
			return provider()
		}
		t.MaxVersion = tls.VersionTLS12
	} else if c.ClientCert != nil {
		t.Certificates = []tls.Certificate{*c.ClientCert}
		t.MaxVersion = tls.VersionTLS12
	}
	return t
}

// HasClientCert reports whether TLS client-certificate auth is configured.
func (c *Config) HasClientCert() bool {
	return c.ClientCert != nil || c.ClientCertProvider != nil
}

// NewCachingCertProvider wraps a certificate loader with caching: the loaded
// cert is reused until shortly before its NotAfter, then re-resolved. Failed
// loads are retried on every call (so an SLC login mid-session heals the next
// handshake without a restart).
func NewCachingCertProvider(load func() (*tls.Certificate, error)) func() (*tls.Certificate, error) {
	var mu sync.Mutex
	var cached *tls.Certificate
	return func() (*tls.Certificate, error) {
		mu.Lock()
		defer mu.Unlock()
		if cached != nil && cached.Leaf != nil &&
			time.Now().Add(5*time.Minute).Before(cached.Leaf.NotAfter) {
			return cached, nil
		}
		cert, err := load()
		if err != nil {
			return nil, err
		}
		cached = cert
		return cached, nil
	}
}

// Option is a functional option for configuring the ADT client.
type Option func(*Config)

// WithClient sets the SAP client number.
func WithClient(client string) Option {
	return func(c *Config) {
		c.Client = client
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

// WithClientCert sets a TLS client certificate for mutual-auth (mTLS) login.
func WithClientCert(cert *tls.Certificate) Option {
	return func(c *Config) {
		c.ClientCert = cert
	}
}

// WithClientCertProvider sets a lazy per-handshake certificate resolver for
// mutual-auth (mTLS) login. Takes precedence over WithClientCert.
func WithClientCertProvider(p func() (*tls.Certificate, error)) Option {
	return func(c *Config) {
		c.ClientCertProvider = p
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
	jar, _ := cookiejar.New(nil)

	transport := &http.Transport{
		Proxy:           http.ProxyFromEnvironment, // Honor HTTP_PROXY/HTTPS_PROXY env vars
		TLSClientConfig: c.tlsClientConfig(),
	}

	client := &http.Client{
		Jar:       jar,
		Transport: transport,
		Timeout:   c.Timeout,
	}

	// Preserve Authorization header across redirects.
	// Go's default strips it per RFC 7235 §4.2, but SAP BTP/Cloud
	// authentication flows require it to survive redirects.
	// Without this, BTP users get 401 even though curl works (issue #90).
	client.CheckRedirect = func(req *http.Request, via []*http.Request) error {
		if len(via) >= 10 {
			return fmt.Errorf("too many redirects")
		}
		if len(via) > 0 {
			if auth := via[0].Header.Get("Authorization"); auth != "" {
				req.Header.Set("Authorization", auth)
			}
		}
		return nil
	}

	return client
}
