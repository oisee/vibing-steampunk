// Package config provides system configuration management for vsp CLI.
package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// SystemConfig represents a SAP system configuration.
type SystemConfig struct {
	URL      string `json:"url"`
	User     string `json:"user,omitempty"`
	Password string `json:"password,omitempty"` // Not recommended, use env var
	Client   string `json:"client,omitempty"`
	Language string `json:"language,omitempty"`
	Insecure bool   `json:"insecure,omitempty"`

	// Optional CTS correlation attribute (for CR-level grouping, e.g. SAPTEST/ZCR)
	TransportAttribute string `json:"transport_attribute,omitempty"`

	// Optional analysis cache (opt-in)
	Cache     bool   `json:"cache,omitempty"`      // Enable SQLite analysis cache
	CachePath string `json:"cache_path,omitempty"` // Custom cache path (default: .vsp-cache/<system>.db)

	// Cookie authentication (alternative to user/password)
	CookieFile   string `json:"cookie_file,omitempty"`   // Path to Netscape-format cookie file
	CookieString string `json:"cookie_string,omitempty"` // Inline cookie string

	// Auth names the authentication method explicitly. Empty infers one from
	// the fields above: cookies if present, otherwise user/password. Set it to
	// "sso" to authenticate through a browser single sign-on handshake.
	Auth string `json:"auth,omitempty"`

	// SSO tunes browser single sign-on. Every field is optional — with auth set
	// to "sso" and nothing else configured, vsp derives a trigger URL from this
	// system's URL and lets the capture pick its own browser profile.
	SSO *SSOSettings `json:"sso,omitempty"`

	// Classic RFC (open-rfc-go) settings. The host defaults to the URL's host and
	// the gateway port to 3300 + system number; set rfc_port to override directly.
	// Credentials default to the RFC environment (SAP_USER/SAP_PASSWORD), then to
	// this system's user/password.
	RFCHost     string `json:"rfc_host,omitempty"`
	RFCSysnr    string `json:"rfc_sysnr,omitempty"`
	RFCPort     int    `json:"rfc_port,omitempty"`
	RFCUser     string `json:"rfc_user,omitempty"`
	RFCPassword string `json:"rfc_password,omitempty"`

	// Optional safety settings per system
	ReadOnly        bool     `json:"read_only,omitempty"`
	AllowedPackages []string `json:"allowed_packages,omitempty"`

	// Transport safety, per system. These exist here and not only as server
	// flags because the command line has no other way to reach them: the flags
	// belong to the root command, which is the MCP server, so a subcommand that
	// tried to pass one was told the flag does not exist. Without these fields
	// `vsp transport list` could never succeed, whatever the caller typed.
	EnableTransports        bool     `json:"enable_transports,omitempty"`
	TransportReadOnly       bool     `json:"transport_read_only,omitempty"`
	AllowedTransports       []string `json:"allowed_transports,omitempty"`
	AllowTransportableEdits bool     `json:"allow_transportable_edits,omitempty"`
	TransportChoice         string   `json:"transport_choice,omitempty"` // auto (default) or off
	BlockFreeSQL            bool     `json:"block_free_sql,omitempty"`
}

// SSOSettings configures browser single sign-on for one system.
type SSOSettings struct {
	// TriggerURL is the authentication-gated page whose loading starts the SSO
	// redirect chain. Defaults to this system's ADT root, which every system
	// vsp can talk to has. Point it elsewhere — a Fiori launchpad, say — on
	// systems that gate single sign-on at a different entry point.
	TriggerURL string `json:"trigger_url,omitempty"`

	// Profile is the browser profile directory. A persistent one lets later
	// refreshes reuse the identity provider's session. Under WSL the browser
	// runs on the Windows side, so this must be a Windows path.
	Profile string `json:"profile,omitempty"`

	// Helper overrides the path to the Windows capture helper (vsp-sso.exe),
	// which is how the browser step runs under WSL.
	Helper string `json:"helper,omitempty"`

	// OnExpiry decides what happens when a silent refresh cannot finish because
	// the identity provider wants a human: "window" (the default) opens a
	// browser window to sign in, "error" reports it and names the command to
	// run instead. Prefer "error" where nobody is watching the screen.
	OnExpiry string `json:"on_expiry,omitempty"`

	// SilentTimeout and InteractiveTimeout override the capture budgets, as Go
	// durations ("45s", "5m").
	SilentTimeout      string `json:"silent_timeout,omitempty"`
	InteractiveTimeout string `json:"interactive_timeout,omitempty"`
}

// UsesSSO reports whether this system authenticates through browser SSO.
func (s *SystemConfig) UsesSSO() bool {
	return strings.EqualFold(s.Auth, "sso")
}

// InteractiveOnExpiry reports whether a failed silent refresh may open a
// browser window. Opening one is the default: the alternative leaves a session
// broken until someone notices and runs a command by hand. A nil receiver means
// nothing was configured, so the default applies.
func (s *SSOSettings) InteractiveOnExpiry() bool {
	if s == nil || s.OnExpiry == "" {
		return true
	}
	return !strings.EqualFold(s.OnExpiry, "error")
}

// SystemsConfig is the root configuration containing all systems.
type SystemsConfig struct {
	Systems map[string]SystemConfig `json:"systems"`
	Default string                  `json:"default,omitempty"`

	// Tools configuration - granular tool visibility control
	// Key: tool name, Value: true=enabled, false=disabled
	// Tools not listed are enabled by default
	Tools map[string]bool `json:"tools,omitempty"`
}

// ConfigPaths returns the list of paths to search for systems config.
func ConfigPaths() []string {
	paths := []string{
		".vsp.json",         // Current directory (preferred)
		".vsp/systems.json", // Current directory .vsp folder
	}

	// Add home directory paths
	if home, err := os.UserHomeDir(); err == nil {
		paths = append(paths,
			filepath.Join(home, ".vsp.json"),
			filepath.Join(home, ".vsp", "systems.json"),
		)
	}

	return paths
}

// LoadSystems loads systems configuration from the first found config file.
func LoadSystems() (*SystemsConfig, string, error) {
	for _, path := range ConfigPaths() {
		if _, err := os.Stat(path); err == nil {
			cfg, err := LoadSystemsFromFile(path)
			if err != nil {
				return nil, path, err
			}
			return cfg, path, nil
		}
	}
	return nil, "", nil // No config file found (not an error)
}

// LoadSystemsFromFile loads systems configuration from a specific file.
func LoadSystemsFromFile(path string) (*SystemsConfig, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read config file: %w", err)
	}

	var cfg SystemsConfig
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("failed to parse config file: %w", err)
	}

	return &cfg, nil
}

// GetSystem retrieves a system configuration by name, resolving password from env.
func (c *SystemsConfig) GetSystem(name string) (*SystemConfig, error) {
	sys, ok := c.Systems[name]
	if !ok {
		// List available systems in error
		available := make([]string, 0, len(c.Systems))
		for k := range c.Systems {
			available = append(available, k)
		}
		return nil, fmt.Errorf("system '%s' not found. Available: %s", name, strings.Join(available, ", "))
	}

	// Resolve password from environment variable if not set
	if sys.Password == "" {
		// Try VSP_<SYSTEM>_PASSWORD (e.g., VSP_A4H_PASSWORD)
		envKey := fmt.Sprintf("VSP_%s_PASSWORD", strings.ToUpper(name))
		if pwd := os.Getenv(envKey); pwd != "" {
			sys.Password = pwd
		}
	}

	if sys.TransportAttribute == "" {
		envKey := fmt.Sprintf("VSP_%s_TRANSPORT_ATTRIBUTE", strings.ToUpper(name))
		if attr := strings.TrimSpace(os.Getenv(envKey)); attr != "" {
			sys.TransportAttribute = strings.ToUpper(attr)
		}
	}
	if sys.TransportAttribute == "" {
		if attr := strings.TrimSpace(os.Getenv("VSP_TRANSPORT_ATTRIBUTE")); attr != "" {
			sys.TransportAttribute = strings.ToUpper(attr)
		}
	} else {
		sys.TransportAttribute = strings.ToUpper(strings.TrimSpace(sys.TransportAttribute))
	}

	// Resolve RFC credentials: VSP_<SYSTEM>_RFC_PASSWORD, then the RFC
	// environment (SAP_USER/SAP_PASSWORD) used by the open-rfc-go tooling.
	if sys.RFCPassword == "" {
		envKey := fmt.Sprintf("VSP_%s_RFC_PASSWORD", strings.ToUpper(name))
		if pwd := os.Getenv(envKey); pwd != "" {
			sys.RFCPassword = pwd
		}
	}
	if sys.RFCPassword == "" {
		if pwd := os.Getenv("SAP_PASSWORD"); pwd != "" {
			sys.RFCPassword = pwd
		}
	}
	if sys.RFCUser == "" {
		if u := os.Getenv("SAP_USER"); u != "" {
			sys.RFCUser = u
		}
	}

	// Resolve cache from env if not set in config
	if !sys.Cache {
		envKey := fmt.Sprintf("VSP_%s_CACHE", strings.ToUpper(name))
		if strings.EqualFold(os.Getenv(envKey), "true") {
			sys.Cache = true
		}
	}
	if !sys.Cache {
		if strings.EqualFold(os.Getenv("VSP_CACHE"), "true") {
			sys.Cache = true
		}
	}
	if sys.Cache && sys.CachePath == "" {
		sys.CachePath = fmt.Sprintf(".vsp-cache/%s.db", strings.ToLower(name))
	}

	// Apply defaults
	if sys.Client == "" {
		sys.Client = "001"
	}
	if sys.Language == "" {
		sys.Language = "EN"
	}

	return &sys, nil
}

// ListSystems returns a list of configured system names.
func (c *SystemsConfig) ListSystems() []string {
	systems := make([]string, 0, len(c.Systems))
	for name := range c.Systems {
		systems = append(systems, name)
	}
	return systems
}

// ExampleConfig returns an example configuration for documentation.
func ExampleConfig() string {
	example := SystemsConfig{
		Default: "dev",
		Systems: map[string]SystemConfig{
			"dev": {
				URL:                "http://dev.example.com:50000",
				User:               "DEVELOPER",
				Client:             "001",
				TransportAttribute: "SAPTEST",
				Cache:              true,
			},
			"a4h": {
				URL:      "http://a4h.local:50000",
				User:     "ADMIN",
				Client:   "001",
				Insecure: true,
			},
			"prod": {
				URL:             "https://prod.example.com:44300",
				User:            "READONLY_USER",
				Client:          "100",
				ReadOnly:        true,
				AllowedPackages: []string{"Z*", "Y*"},
			},
			"sso": {
				URL:    "https://sso.example.com",
				Client: "100",
				Auth:   "sso",
			},
		},
	}

	data, _ := json.MarshalIndent(example, "", "  ")
	return string(data)
}

// IsToolEnabled checks if a tool is enabled in the configuration.
// Tools not explicitly listed are enabled by default.
func (c *SystemsConfig) IsToolEnabled(toolName string) bool {
	if c.Tools == nil {
		return true
	}
	enabled, exists := c.Tools[toolName]
	if !exists {
		return true // Default: enabled
	}
	return enabled
}

// GetDisabledTools returns a list of explicitly disabled tools.
func (c *SystemsConfig) GetDisabledTools() []string {
	var disabled []string
	for name, enabled := range c.Tools {
		if !enabled {
			disabled = append(disabled, name)
		}
	}
	return disabled
}

// SetToolEnabled sets the enabled state for a tool.
func (c *SystemsConfig) SetToolEnabled(toolName string, enabled bool) {
	if c.Tools == nil {
		c.Tools = make(map[string]bool)
	}
	c.Tools[toolName] = enabled
}

// SaveToFile saves the configuration to a file.
func (c *SystemsConfig) SaveToFile(path string) error {
	data, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal config: %w", err)
	}
	if err := os.WriteFile(path, data, 0600); err != nil {
		return fmt.Errorf("failed to write config: %w", err)
	}
	return nil
}

// DefaultDisabledTools returns the list of tools that should be disabled by default.
// These are experimental or non-working tools.
func DefaultDisabledTools() []string {
	return []string{
		// AMDP/HANA Debugger - session management issues
		"AMDPDebuggerStart", "AMDPDebuggerResume", "AMDPDebuggerStop",
		"AMDPDebuggerStep", "AMDPGetVariables", "AMDPSetBreakpoint", "AMDPGetBreakpoints",
		// The ABAP debugger and its breakpoints were here until 2026-08-21,
		// disabled because a stateless client cannot hold a debug session. The
		// server holds one now (internal/mcp/handlers_debug_session.go) and both
		// halves run against SAP's own ADT resources, over the RFC tunnel or over
		// stateful HTTPS, with no ZADT_VSP and no Z code at all.
		// UI5 write operations - need alternate API
		"UI5CreateApp", "UI5DeleteApp", "UI5DeleteFile", "UI5UploadFile",
	}
}
