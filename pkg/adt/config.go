// Package adt provides a Go client for SAP ABAP Development Tools (ADT) REST API.
package adt

import (
	"crypto/tls"
	"net/http"
	"net/http/cookiejar"
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
		SessionType: SessionStateful,
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
		Proxy: http.ProxyFromEnvironment,
		TLSClientConfig: &tls.Config{
			InsecureSkipVerify: c.InsecureSkipVerify,
		},
		// Connection pooling settings to avoid overwhelming SAP server
		MaxIdleConns:        100,
		MaxIdleConnsPerHost: 30, // Keep idle connections for reuse (match MaxConnsPerHost)
		MaxConnsPerHost:     30, // Concurrent connections per host
		IdleConnTimeout:     90 * time.Second,
	}

	return &http.Client{
		Jar:       jar,
		Transport: transport,
		Timeout:   c.Timeout,
	}
}
