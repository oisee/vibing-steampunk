// Package mcp provides the MCP server implementation for ABAP ADT tools.
package mcp

import (
	"context"
	"crypto/subtle"
	"crypto/tls"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
	openrfc "github.com/oisee/open-rfc-go/rfc"
	"github.com/oisee/vibing-steampunk/pkg/adt"
)

// AsyncTask represents a background task status.
type AsyncTask struct {
	ID        string      `json:"id"`
	Type      string      `json:"type"`   // "report", "export", etc.
	Status    string      `json:"status"` // "running", "completed", "error"
	StartedAt time.Time   `json:"started_at"`
	EndedAt   *time.Time  `json:"ended_at,omitempty"`
	Result    interface{} `json:"result,omitempty"`
	Error     string      `json:"error,omitempty"`
}

// Server wraps the MCP server with ADT client.
type Server struct {
	mcpServer     *server.MCPServer
	adtClient     *adt.Client
	amdpWSClient  *adt.AMDPWebSocketClient  // WebSocket-based AMDP client (ZADT_VSP)
	debugWSClient *adt.DebugWebSocketClient // WebSocket-based debug client (ZADT_VSP)
	config        *Config                   // Server configuration for session manager creation
	featureProber *adt.FeatureProber        // Feature detection system (safety network)
	featureConfig adt.FeatureConfig         // Feature configuration

	// Shared classic-RFC client (lazily dialled, reused across tool calls, and
	// pinged while idle so a gateway timeout does not kill it)
	rfcMu       sync.Mutex
	rfcShared   *openrfc.Client
	rfcLastUsed time.Time

	// The one debug session this server holds across tool calls (see
	// handlers_debug_session.go); nil until the first debugger call.
	debugMu   sync.Mutex
	debugSess *debugSession

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

	// ClientCert, when set, is presented for TLS mutual auth (mTLS) instead of
	// a password. On macOS it is loaded from the keychain in main.go.
	ClientCert *tls.Certificate

	// ClientCertProvider resolves the cert lazily per TLS handshake (wins over
	// ClientCert): the server stays up without a cert and heals after SLC login.
	ClientCertProvider func() (*tls.Certificate, error)

	// Cookie authentication (alternative to basic auth)
	Cookies map[string]string

	// Build names the binary, as "v2.52.0 (commit abc1234, built ...)".
	//
	// It is a string the caller composes rather than three fields, because the
	// only thing this package does with it is print it, and three fields would
	// be three chances for one of them to go unset.
	Build string

	// Verbose output
	Verbose bool

	// Mode: focused or expert (default: focused)
	Mode string

	// DisabledGroups disables groups of tools using short codes:
	// 5/U = UI5/BSP tools, T = Test tools, H = HANA/AMDP debugger, D = ABAP Debugger
	// Example: "TH" disables Tests and HANA debugger tools
	DisabledGroups string

	// Safety configuration
	ReadOnly                bool
	BlockFreeSQL            bool
	AllowedOps              string
	DisallowedOps           string
	AllowedPackages         []string
	EnableTransports        bool     // Explicitly enable transport management (default: disabled)
	TransportReadOnly       bool     // Only allow read operations on transports (list, get)
	AllowedTransports       []string // Whitelist specific transports (supports wildcards like "A4HK*")
	AllowTransportableEdits bool     // Allow editing objects that require transport requests

	// Feature configuration (safety network)
	// Values: "auto" (default, probe system), "on" (force enabled), "off" (force disabled)
	FeatureHANA      string // HANA database detection (required for some AMDP features)
	FeatureAbapGit   string // abapGit integration
	FeatureRAP       string // RAP/OData development (DDLS, BDEF, SRVD, SRVB)
	FeatureAMDP      string // AMDP/HANA debugger
	FeatureUI5       string // UI5/Fiori BSP management
	FeatureTransport string // CTS transport management (distinct from EnableTransports safety)

	// Graph / co-change configuration
	TransportAttribute string // E070A attribute name for CR-level co-change aggregation

	// Debugger configuration
	TerminalID string // SAP GUI terminal ID for cross-tool breakpoint sharing

	// ReauthFunc is called on 401 to re-authenticate (e.g., re-run SAML dance).
	// Returns fresh cookies. Passed through to adt.Config.
	ReauthFunc func(ctx context.Context) (map[string]string, error)

	// ReauthTimeout caps one re-authentication attempt. Zero uses the client
	// default, which assumes the flow runs unattended; a browser sign-in that
	// stops to ask for a second factor needs considerably longer.
	ReauthTimeout time.Duration

	// Session keep-alive interval (0 = disabled)
	// Sends periodic pings to prevent session timeout during idle periods.
	// Useful for cookie/browser-auth where sessions expire server-side.
	KeepAliveInterval time.Duration

	// Transport mode: "stdio" (default) or "http"
	Transport string
	// HTTP address for Streamable HTTP transport (default: ":8080")
	HTTPAddr string

	// Granular tool visibility (from .vsp.json)
	// Key: tool name, Value: true=enabled, false=disabled
	// Takes highest priority over mode and disabled groups
	ToolsConfig map[string]bool
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
	if cfg.ClientCertProvider != nil {
		opts = append(opts, adt.WithClientCertProvider(cfg.ClientCertProvider))
	} else if cfg.ClientCert != nil {
		opts = append(opts, adt.WithClientCert(cfg.ClientCert))
	}
	if len(cfg.Cookies) > 0 {
		opts = append(opts, adt.WithCookies(cfg.Cookies))
	}
	if cfg.Verbose {
		opts = append(opts, adt.WithVerbose())
	}
	if cfg.ReauthFunc != nil {
		opts = append(opts, adt.WithReauthFunc(cfg.ReauthFunc))
	}
	if cfg.ReauthTimeout > 0 {
		opts = append(opts, adt.WithReauthTimeout(cfg.ReauthTimeout))
	}

	// Configure safety settings
	safety := adt.UnrestrictedSafetyConfig() // Default: unrestricted for backwards compatibility
	if cfg.ReadOnly {
		safety.ReadOnly = true
	}
	if cfg.BlockFreeSQL {
		safety.BlockFreeSQL = true
	}
	if cfg.AllowedOps != "" {
		safety.AllowedOps = cfg.AllowedOps
	}
	if cfg.DisallowedOps != "" {
		safety.DisallowedOps = cfg.DisallowedOps
	}
	if len(cfg.AllowedPackages) > 0 {
		safety.AllowedPackages = cfg.AllowedPackages
	}
	if cfg.EnableTransports {
		safety.EnableTransports = true
	}
	if cfg.TransportReadOnly {
		safety.TransportReadOnly = true
	}
	if len(cfg.AllowedTransports) > 0 {
		safety.AllowedTransports = cfg.AllowedTransports
	}
	if cfg.AllowTransportableEdits {
		safety.AllowTransportableEdits = true
	}
	opts = append(opts, adt.WithSafety(safety))

	adtClient := adt.NewClient(cfg.BaseURL, cfg.Username, cfg.Password, opts...)
	return NewServerWithClient(cfg, adtClient)
}

// NewServerWithClient builds a server around a client the caller already holds.
//
// The CLI resolves a system into a client of its own — carrying that system's
// cookies, its browser single sign-on with the refresh hook attached, and its
// declared safety — and none of that survives a round trip through Config.
// `vsp sweep` has to call handlers through the real dispatch path on the real
// connection, so it hands the client over rather than describing it.
func NewServerWithClient(cfg *Config, adtClient *adt.Client) *Server {
	// Set terminal ID for debugger operations
	// Priority: 1) Custom ID (SAP GUI), 2) User-based ID
	if cfg.TerminalID != "" {
		adt.SetTerminalID(cfg.TerminalID)
	}
	adt.SetTerminalIDUser(cfg.Username)

	// Configure feature detection (safety network)
	featureConfig := adt.FeatureConfig{
		HANA:      parseFeatureMode(cfg.FeatureHANA),
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

	// Register tools based on mode, disabled groups, and granular tool config
	s.registerTools(cfg.Mode, cfg.DisabledGroups, cfg.ToolsConfig)

	// Start session keep-alive if configured
	if cfg.KeepAliveInterval > 0 {
		adtClient.StartKeepAlive(cfg.KeepAliveInterval, cfg.Verbose)
	}

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

// ServeStdio starts the MCP server on stdin/stdout.
func (s *Server) ServeStdio() error {
	// A debuggee left attached when the server exits stays suspended in a work
	// process until its caller times out, so the session is released here as
	// well as on an explicit detach.
	defer s.closeDebugSession(context.Background())
	return server.ServeStdio(s.mcpServer)
}

// ServeHTTP starts the MCP server as a Streamable HTTP endpoint.
//
// The endpoint exposes the whole ADT tool surface under the operator's SAP
// credentials, so it is guarded twice:
//
//   - an API key (VSP_HTTP_API_KEY), compared in constant time. Without it a
//     bind to anything but a loopback address is refused outright, because an
//     unauthenticated remote caller would inherit those credentials.
//   - Origin validation, so a page the operator merely visits cannot drive a
//     loopback endpoint through DNS rebinding. Requests without an Origin (a
//     normal API client) pass; a cross-origin browser request does not.
//
// GET /health answers without either check, for liveness probes.
func (s *Server) ServeHTTP(addr string) error {
	apiKey := strings.TrimSpace(os.Getenv("VSP_HTTP_API_KEY"))
	if apiKey == "" && !isLoopbackAddr(addr) {
		return fmt.Errorf("refusing to serve %s without authentication: set VSP_HTTP_API_KEY (it exposes every ADT tool under your SAP credentials)", addr)
	}
	if apiKey == "" {
		fmt.Fprintf(os.Stderr, "[WARN] HTTP transport on %s has no API key: set VSP_HTTP_API_KEY. Loopback only, and Origin is still validated.\n", addr)
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"status":"ok"}`))
	})
	mux.Handle("/", requireAPIKey(apiKey, validateOrigin(server.NewStreamableHTTPServer(s.mcpServer))))

	httpServer := &http.Server{
		Addr:              addr,
		Handler:           mux,
		ReadHeaderTimeout: 10 * time.Second,
	}
	return httpServer.ListenAndServe()
}

// requireAPIKey rejects requests without a matching bearer token. An empty key
// disables the check (only reachable on a loopback bind, see ServeHTTP).
func requireAPIKey(apiKey string, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if apiKey == "" {
			next.ServeHTTP(w, r)
			return
		}
		presented := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
		if presented == "" {
			presented = r.Header.Get("X-API-Key")
		}
		if subtle.ConstantTimeCompare([]byte(presented), []byte(apiKey)) != 1 {
			w.Header().Set("WWW-Authenticate", `Bearer realm="vsp"`)
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// validateOrigin blocks cross-origin browser requests, which is what makes a
// loopback endpoint immune to DNS rebinding. A request with no Origin header is
// not a browser page and passes through.
func validateOrigin(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		origin := r.Header.Get("Origin")
		if origin == "" {
			next.ServeHTTP(w, r)
			return
		}
		u, err := url.Parse(origin)
		if err != nil || !sameHost(u.Host, r.Host) {
			http.Error(w, "forbidden origin", http.StatusForbidden)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// sameHost compares an Origin's host with the request's Host, ignoring a
// missing port on either side.
func sameHost(originHost, requestHost string) bool {
	strip := func(h string) string {
		if host, _, err := net.SplitHostPort(h); err == nil {
			return host
		}
		return h
	}
	return strings.EqualFold(strip(originHost), strip(requestHost))
}

// isLoopbackAddr reports whether a listen address is bound to loopback only.
// An empty or wildcard host (":8080", "0.0.0.0:8080", "[::]:8080") is not.
func isLoopbackAddr(addr string) bool {
	host, _, err := net.SplitHostPort(addr)
	if err != nil {
		host = addr
	}
	if host == "" || host == "0.0.0.0" || host == "::" || host == "[::]" {
		return false
	}
	if strings.EqualFold(host, "localhost") {
		return true
	}
	ip := net.ParseIP(strings.Trim(host, "[]"))
	return ip != nil && ip.IsLoopback()
}

// GetMCPServer returns the underlying MCP server (for custom transport setup).
func (s *Server) GetMCPServer() *server.MCPServer {
	return s.mcpServer
}

// newToolResultError creates an error result for tool execution failures.
func newToolResultError(message string) *mcp.CallToolResult {
	result := mcp.NewToolResultText(message)
	result.IsError = true
	return result
}

// applyWSAuth hands a WebSocket client the browser session, when the server is
// running on one.
//
// The WebSocket clients were built with a password as their only credential, so
// on a system reached through single sign-on — where no password exists — every
// feature behind ZADT_VSP was unreachable for a client-side reason. The upgrade
// request carries a cookie like any other HTTP request; this passes it on.
func (s *Server) applyWSAuth(setCookies func(map[string]string)) {
	// Ask the ADT client rather than the config. A session that expires is
	// replaced wholesale, and the config holds the map handed over at startup —
	// so a server that has been running long enough to re-authenticate would
	// open every WebSocket with the dead session while its ordinary calls
	// carried on working, which is a confusing way to fail.
	if live := s.adtClient.CurrentCookies(); len(live) > 0 {
		setCookies(live)
		return
	}
	if len(s.config.Cookies) > 0 {
		setCookies(s.config.Cookies)
	}
}

// ensureWSConnected ensures the WebSocket client is connected, creating it if needed.
// Returns error result if connection fails, nil on success.
func (s *Server) ensureWSConnected(ctx context.Context, toolName string) *mcp.CallToolResult {
	if s.amdpWSClient == nil || !s.amdpWSClient.IsConnected() {
		s.amdpWSClient = adt.NewAMDPWebSocketClient(
			s.config.BaseURL, s.config.Client, s.config.Username, s.config.Password, s.config.InsecureSkipVerify,
		)
		s.applyWSAuth(s.amdpWSClient.SetCookies)
		if s.config.ClientCertProvider != nil {
			s.amdpWSClient.SetClientCertProvider(s.config.ClientCertProvider)
		} else if s.config.ClientCert != nil {
			s.amdpWSClient.SetClientCert(s.config.ClientCert)
		}
		if err := s.amdpWSClient.Connect(ctx); err != nil {
			s.amdpWSClient = nil
			return newToolResultError(fmt.Sprintf("%s: WebSocket connect failed: %v", toolName, err))
		}
	}
	return nil
}

// requireActiveAMDPSession checks if there's an active AMDP debug session.
// Returns error result if no session, nil if session is active.
func (s *Server) requireActiveAMDPSession() *mcp.CallToolResult {
	if s.amdpWSClient == nil || !s.amdpWSClient.IsActive() {
		return newToolResultError("No active AMDP session. Use AMDPDebuggerStart first.")
	}
	return nil
}

// Tool handlers are in separate files:
// - handlers_read.go: GetProgram, GetClass, GetTable, etc.
// - handlers_system.go: GetSystemInfo, GetFeatures, etc.
// - handlers_analysis.go: GetCallGraph, TraceExecution, etc.
// - handlers_codeintel.go: FindDefinition, FindReferences, CodeCompletion, etc.
// - handlers_devtools.go: SyntaxCheck, Activate, ATC, etc.
// - handlers_crud.go: Lock, Create, Update, Delete, etc.
// - handlers_debugger.go: SetBreakpoint, DebuggerListen, etc.
// - handlers_amdp.go: AMDPDebugger* handlers
// - handlers_ui5.go: UI5ListApps, UI5GetApp, etc.
// - handlers_git.go: GitTypes, GitExport
// - handlers_report.go: RunReport, GetVariants, etc.
// - handlers_install.go: InstallZADTVSP, InstallAbapGit, etc.
// - handlers_transport.go: ListTransports, GetTransport, etc.
//
// Tool registration is in:
// - tools_register.go: registerTools() and all register*Tools() methods
// - tools_groups.go: toolGroups() - group definitions for --disabled-groups
// - tools_focused.go: focusedToolSet() - focused mode whitelist
// - tools_aliases.go: registerToolAliases() - short alias names
