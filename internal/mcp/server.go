// Package mcp provides the MCP server implementation for ABAP ADT tools.
package mcp

import (
	"strings"
	"sync"

	"github.com/mark3labs/mcp-go/server"
	"github.com/oisee/vibing-steampunk/pkg/adt"
)

// Server wraps the MCP server with ADT client.
type Server struct {
	mcpServer     *server.MCPServer
	adtClient     *adt.Client
	amdpWSClient  *adt.AMDPWebSocketClient  // WebSocket-based AMDP client (ZADT_VSP)
	debugWSClient *adt.DebugWebSocketClient // WebSocket-based debug client (ZADT_VSP)
	config        *Config                   // Server configuration for session manager creation
	featureProber *adt.FeatureProber        // Feature detection system (safety network)
	featureConfig adt.FeatureConfig         // Feature configuration

	// Async task management
	asyncTasks   map[string]*AsyncTask
	asyncTasksMu sync.RWMutex
	asyncTaskID  int64
}

// Config holds MCP server configuration.
type Config struct {
	// SAP connection settings
	BaseURL            string
	Username           string
	Password           string
	Client             string
	Language           string
	InsecureSkipVerify bool

	// Cookie authentication (alternative to basic auth)
	Cookies map[string]string

	// Verbose output
	Verbose bool

	// Safety configuration
	ReadOnly          bool
	BlockFreeSQL      bool
	AllowedOps        string
	DisallowedOps     string
	AllowedPackages   []string
	EnableTransports  bool     // Explicitly enable transport management (default: disabled)
	TransportReadOnly bool     // Only allow read operations on transports (list, get)
	AllowedTransports []string // Whitelist specific transports (supports wildcards like "A4HK*")

	// Feature configuration (safety network)
	// Values: "auto" (default, probe system), "on" (force enabled), "off" (force disabled)
	FeatureAbapGit   string // abapGit integration
	FeatureRAP       string // RAP/OData development (DDLS, BDEF, SRVD, SRVB)
	FeatureAMDP      string // AMDP/HANA debugger
	FeatureUI5       string // UI5/Fiori BSP management
	FeatureTransport string // CTS transport management (distinct from EnableTransports safety)

	// Debugger configuration
	TerminalID string // SAP GUI terminal ID for cross-tool breakpoint sharing
}

// NewServer creates a new MCP server for ABAP ADT tools.
func NewServer(cfg *Config) *Server {
	// Create ADT client
	opts := []adt.Option{
		adt.WithClient(cfg.Client),
		adt.WithLanguage(cfg.Language),
	}
	if cfg.InsecureSkipVerify {
		opts = append(opts, adt.WithInsecureSkipVerify())
	}
	if len(cfg.Cookies) > 0 {
		opts = append(opts, adt.WithCookies(cfg.Cookies))
	}
	if cfg.Verbose {
		opts = append(opts, adt.WithVerbose())
	}

	// Configure safety settings
	opts = append(opts, adt.WithSafety(buildSafetyConfig(cfg)))

	adtClient := adt.NewClient(cfg.BaseURL, cfg.Username, cfg.Password, opts...)

	// Set terminal ID for debugger operations
	// Priority: 1) Custom ID (SAP GUI), 2) User-based ID
	if cfg.TerminalID != "" {
		adt.SetTerminalID(cfg.TerminalID)
	}
	adt.SetTerminalIDUser(cfg.Username)

	// Configure feature detection (safety network)
	featureConfig := adt.FeatureConfig{
		AbapGit:   parseFeatureMode(cfg.FeatureAbapGit),
		RAP:       parseFeatureMode(cfg.FeatureRAP),
		AMDP:      parseFeatureMode(cfg.FeatureAMDP),
		UI5:       parseFeatureMode(cfg.FeatureUI5),
		Transport: parseFeatureMode(cfg.FeatureTransport),
	}

	// Create feature prober
	featureProber := adt.NewFeatureProber(adtClient, featureConfig, cfg.Verbose)

	// Create MCP server
	mcpServer := server.NewMCPServer(
		"mcp-abap-adt-go",
		"1.0.0",
		server.WithResourceCapabilities(true, true),
		server.WithLogging(),
	)

	s := &Server{
		mcpServer:     mcpServer,
		adtClient:     adtClient,
		config:        cfg,
		featureProber: featureProber,
		featureConfig: featureConfig,
		asyncTasks:    make(map[string]*AsyncTask),
	}

	// Register universal SAP tool
	s.registerUniversalTool()

	return s
}

// parseFeatureMode converts string to FeatureMode
func parseFeatureMode(s string) adt.FeatureMode {
	switch strings.ToLower(s) {
	case "on", "true", "1", "yes", "enabled":
		return adt.FeatureModeOn
	case "off", "false", "0", "no", "disabled":
		return adt.FeatureModeOff
	default:
		return adt.FeatureModeAuto
	}
}

// buildSafetyConfig creates safety configuration from server config
func buildSafetyConfig(cfg *Config) adt.SafetyConfig {
	safety := adt.UnrestrictedSafetyConfig()
	safety.ReadOnly = cfg.ReadOnly
	safety.BlockFreeSQL = cfg.BlockFreeSQL
	safety.EnableTransports = cfg.EnableTransports
	safety.TransportReadOnly = cfg.TransportReadOnly
	if cfg.AllowedOps != "" {
		safety.AllowedOps = cfg.AllowedOps
	}
	if cfg.DisallowedOps != "" {
		safety.DisallowedOps = cfg.DisallowedOps
	}
	if len(cfg.AllowedPackages) > 0 {
		safety.AllowedPackages = cfg.AllowedPackages
	}
	if len(cfg.AllowedTransports) > 0 {
		safety.AllowedTransports = cfg.AllowedTransports
	}
	return safety
}

// ServeStdio starts the MCP server on stdin/stdout.
func (s *Server) ServeStdio() error {
	return server.ServeStdio(s.mcpServer)
}
