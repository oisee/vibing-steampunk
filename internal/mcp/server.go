// Package mcp provides the MCP server implementation for ABAP ADT tools.
package mcp

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
	"github.com/oisee/vibing-steampunk/pkg/adt"
)

// AsyncTask represents a background task status.
type AsyncTask struct {
	ID        string      `json:"id"`
	Type      string      `json:"type"`       // "report", "export", etc.
	Status    string      `json:"status"`     // "running", "completed", "error"
	StartedAt time.Time   `json:"started_at"`
	EndedAt   *time.Time  `json:"ended_at,omitempty"`
	Result    interface{} `json:"result,omitempty"`
	Error     string      `json:"error,omitempty"`
}

// Server wraps the MCP server with ADT client.
type Server struct {
	mcpServer      *server.MCPServer
	adtClient      *adt.Client
	amdpWSClient   *adt.AMDPWebSocketClient   // WebSocket-based AMDP client (ZADT_VSP)
	debugWSClient  *adt.DebugWebSocketClient  // WebSocket-based debug client (ZADT_VSP)
	config         *Config                    // Server configuration for session manager creation
	featureProber  *adt.FeatureProber         // Feature detection system (safety network)
	featureConfig  adt.FeatureConfig          // Feature configuration

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

	// Mode: focused or expert (default: focused)
	Mode string

	// DisabledGroups disables groups of tools using short codes:
	// 5/U = UI5/BSP tools, T = Test tools, H = HANA/AMDP debugger, D = ABAP Debugger
	// Example: "TH" disables Tests and HANA debugger tools
	DisabledGroups string

	// Safety configuration
	ReadOnly         bool
	BlockFreeSQL     bool
	AllowedOps       string
	DisallowedOps    string
	AllowedPackages  []string
	EnableTransports  bool     // Explicitly enable transport management (default: disabled)
	TransportReadOnly bool     // Only allow read operations on transports (list, get)
	AllowedTransports []string // Whitelist specific transports (supports wildcards like "A4HK*")

	// Feature configuration (safety network)
	// Values: "auto" (default, probe system), "on" (force enabled), "off" (force disabled)
	FeatureHANA      string // HANA database detection (required for some AMDP features)
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
	opts = append(opts, adt.WithSafety(safety))

	adtClient := adt.NewClient(cfg.BaseURL, cfg.Username, cfg.Password, opts...)

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

	// Register tools based on mode and disabled groups
	s.registerTools(cfg.Mode, cfg.DisabledGroups)

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
	return server.ServeStdio(s.mcpServer)
}

// registerTools registers ADT tools with the MCP server based on mode and disabled groups.
// Mode "focused" registers essential tools.
// Mode "expert" registers all tools.
// DisabledGroups can disable specific tool groups using short codes:
//   - "5" or "U" = UI5/BSP tools (3 tools, read-only)
//   - "T" = Test tools: RunUnitTests, RunATCCheck (2 tools)
//   - "H" = HANA/AMDP debugger (7 tools)
//   - "D" = ABAP Debugger (6 session tools)
//   - "C" = CTS/Transport tools (5 tools)
//   - "G" = Git/abapGit tools (2 tools)
//   - "R" = Report tools (4 tools)
//   - "I" = Install tools (4 tools)
//   - "X" = EXPERIMENTAL: All debugger + RunReport (17 tools) - use to disable unreliable features
func (s *Server) registerTools(mode string, disabledGroups string) {
	// Define tool groups for selective disablement
	// Short codes: 5/U=UI5, T=Tests, H=HANA, D=Debug, C=CTS, X=Experimental
	toolGroups := map[string][]string{
		"5": { // UI5/BSP tools (also mapped as "U") - read-only, write ops need custom plugin
			"UI5ListApps", "UI5GetApp", "UI5GetFileContent",
		},
		"T": { // Test tools
			"RunUnitTests", "RunATCCheck",
		},
		"H": { // HANA/AMDP debugger
			"AMDPDebuggerStart", "AMDPDebuggerResume", "AMDPDebuggerStop",
			"AMDPDebuggerStep", "AMDPGetVariables", "AMDPSetBreakpoint", "AMDPGetBreakpoints",
		},
		"D": { // ABAP debugger (session tools - breakpoints via WebSocket ZADT_VSP)
			"DebuggerListen", "DebuggerAttach", "DebuggerDetach",
			"DebuggerStep", "DebuggerGetStack", "DebuggerGetVariables",
		},
		"C": { // CTS/Transport tools
			"ListTransports", "GetTransport",
			"CreateTransport", "ReleaseTransport", "DeleteTransport",
		},
		"G": { // Git/abapGit tools (via ZADT_VSP WebSocket)
			"GitTypes", "GitExport",
		},
		"R": { // Report execution tools (via ZADT_VSP WebSocket)
			"RunReport", "GetVariants", "GetTextElements", "SetTextElements",
		},
		"I": { // Install/Setup tools
			"InstallZADTVSP",
			"InstallAbapGit",
			"ListDependencies",
			"InstallDummyTest",
		},
		"X": { // EXPERIMENTAL - Tools requiring special setup or with known limitations
			// ABAP Debugger - requires ZADT_VSP WebSocket handler
			"SetBreakpoint", "GetBreakpoints", "DeleteBreakpoint",
			"DebuggerListen", "DebuggerAttach", "DebuggerDetach",
			"DebuggerStep", "DebuggerGetStack", "DebuggerGetVariables",
			// AMDP/HANA Debugger - experimental, session management issues
			"AMDPDebuggerStart", "AMDPDebuggerResume", "AMDPDebuggerStop",
			"AMDPDebuggerStep", "AMDPGetVariables", "AMDPSetBreakpoint", "AMDPGetBreakpoints",
			// RunReport - requires ZADT_VSP, APC context limitations
			"RunReport",
		},
	}
	// Map "U" to same tools as "5"
	toolGroups["U"] = toolGroups["5"]

	// Build set of disabled tools based on disabledGroups string
	disabledTools := make(map[string]bool)
	for _, code := range strings.ToUpper(disabledGroups) {
		if tools, ok := toolGroups[string(code)]; ok {
			for _, tool := range tools {
				disabledTools[tool] = true
			}
		}
	}

	// Define focused mode tool whitelist (41 essential tools)
	focusedTools := map[string]bool{
		// Unified tools (2)
		"GetSource":   true,
		"WriteSource": true,

		// Search tools (3) - foundation
		"GrepObjects":  true, // Multi-object search (replaces GrepObject)
		"GrepPackages": true, // Multi-package + recursive (replaces GrepPackage)
		"SearchObject": true,

		// Primary workflow (1)
		"EditSource": true,

		// Data/Metadata read (6)
		"GetTable":            true,
		"GetTableContents":    true,
		"RunQuery":            true,
		"GetPackage":          true, // Metadata: package contents
		"GetFunctionGroup":    true, // Metadata: function module list
		"GetCDSDependencies":  true, // CDS dependency tree
		"GetMessages":         true, // Message class texts (SE91)

		// Code intelligence (2)
		"FindDefinition":  true,
		"FindReferences":  true,

		// Development tools (11)
		"SyntaxCheck":         true,
		"RunUnitTests":        true,
		"RunATCCheck":         true,  // Code quality checks
		"Activate":            true,  // Re-activate objects without editing
		"ActivatePackage":     true,  // Batch activation of all inactive objects
		"PrettyPrint":         true,  // Format ABAP code
		"GetInactiveObjects":  true,  // List pending activations
		"CreatePackage":       true,  // Create local packages ($...)
		"CreateTable":         true,  // Create DDIC tables from JSON
		"CompareSource":       true,  // Diff two objects
		"CloneObject":         true,  // Copy object to new name
		"GetClassInfo":        true,  // Quick class metadata

		// Advanced/Edge cases (2)
		"LockObject":   true,
		"UnlockObject": true,

		// File-based operations (2)
		"ImportFromFile": true, // File → SAP (replaces DeployFromFile)
		"ExportToFile":   true, // SAP → File (replaces SaveToFile)

		// System information (2)
		"GetSystemInfo":         true, // System ID, release, kernel
		"GetInstalledComponents": true, // Installed software components

		// Code analysis (7)
		"GetCallGraph":       true, // Call hierarchy for methods/functions
		"GetObjectStructure": true, // Object explorer tree
		"GetCallersOf":       true, // Simplified up traversal
		"GetCalleesOf":       true, // Simplified down traversal
		"AnalyzeCallGraph":   true, // Call graph statistics
		"CompareCallGraphs":  true, // Compare static vs actual execution
		"TraceExecution":     true, // Composite RCA tool

		// Runtime errors / Short dumps (2)
		"ListDumps": true, // List runtime errors (consistent with List* pattern)
		"GetDump":   true, // Get dump details

		// ABAP Profiler / Traces (2)
		"ListTraces": true, // List trace files
		"GetTrace":   true, // Get trace analysis

		// SQL Trace / ST05 (2)
		"GetSQLTraceState": true, // Check if SQL trace is active
		"ListSQLTraces":    true, // List SQL trace files

		// External Breakpoints via WebSocket (ZADT_VSP)
		// REST API returns 403 CSRF, but WebSocket works perfectly
		"SetBreakpoint":    true, // Set line breakpoint
		"GetBreakpoints":   true, // List active breakpoints
		"DeleteBreakpoint": true, // Remove breakpoint
		"CallRFC":          true, // Call function module via WebSocket (trigger execution)
		"MoveObject":       true, // Move object to different package

		// Debugger Session (6)
		"DebuggerListen":       true, // Wait for debuggee to hit breakpoint
		"DebuggerAttach":       true, // Attach to debuggee
		"DebuggerDetach":       true, // Detach from debug session
		"DebuggerStep":         true, // Step through code
		"DebuggerGetStack":     true, // Get call stack
		"DebuggerGetVariables": true, // Get variable values

		// UI5/Fiori BSP Management (3 read-only - ADT filestore is read-only)
		"UI5ListApps":       true, // List UI5 applications
		"UI5GetApp":         true, // Get UI5 app details
		"UI5GetFileContent": true, // Get file content from UI5 app
		// Write ops disabled - ADT filestore API is read-only (405 on POST)
		// Future: implement via custom plugin using /UI5/CL_REPOSITORY_LOAD
		// "UI5UploadFile":     true, // Upload file to UI5 app
		// "UI5DeleteFile":     true, // Delete file from UI5 app
		// "UI5CreateApp":      true, // Create new UI5 app
		// "UI5DeleteApp":      true, // Delete UI5 app

		// AMDP (HANA) Debugger - expert mode only
		"AMDPDebuggerStart":  true,
		"AMDPDebuggerResume": true,
		"AMDPDebuggerStop":   true,
		"AMDPDebuggerStep":   true,
		"AMDPGetVariables":   true,
		"AMDPSetBreakpoint":  true,
		"AMDPGetBreakpoints": true,

		// CTS/Transport Management (2 read-only in focused mode)
		// Write operations (Create, Release, Delete) only in expert mode
		"ListTransports": true, // List transport requests
		"GetTransport":   true, // Get transport details with objects

		// Git/abapGit Integration (via ZADT_VSP WebSocket)
		"GitTypes":  true, // List 158 supported object types
		"GitExport": true, // Export packages/objects to abapGit ZIP

		// Report Execution (via ZADT_VSP WebSocket)
		"RunReport":        true, // Execute reports with params/variants, capture ALV
		"RunReportAsync":   true, // Background report execution with polling
		"GetAsyncResult":   true, // Retrieve async task results
		"GetVariants":      true, // List report variants
		"GetTextElements":  true, // Get program text elements
		"SetTextElements":  true, // Set program text elements

		// Install/Setup tools
		"InstallZADTVSP":   true, // Deploy ZADT_VSP WebSocket handler to SAP
		"InstallAbapGit":   true, // Deploy abapGit (standalone or dev edition) to SAP
		"ListDependencies": true, // List available dependencies for installation
		"InstallDummyTest": true, // Test tool for verifying Install* workflow
	}

	// Helper to check if tool should be registered
	shouldRegister := func(toolName string) bool {
		// Check if tool is disabled by group
		if disabledTools[toolName] {
			return false
		}
		if mode == "expert" {
			return true // Expert mode: register all tools (except disabled)
		}
		return focusedTools[toolName] // Focused mode: only whitelisted tools (except disabled)
	}

	// Unified Tools (Focused Mode) - NEW
	if shouldRegister("GetSource") {
		s.registerGetSource()
	}
	if shouldRegister("WriteSource") {
		s.registerWriteSource()
	}


	// GetProgram
	if shouldRegister("GetProgram") {
		s.mcpServer.AddTool(mcp.NewTool("GetProgram",
		mcp.WithDescription("Retrieve ABAP program source code"),
		mcp.WithString("program_name",
			mcp.Required(),
			mcp.Description("Name of the ABAP program"),
		),
	), s.handleGetProgram)
	}


	// GetClass
	if shouldRegister("GetClass") {
		s.mcpServer.AddTool(mcp.NewTool("GetClass",
		mcp.WithDescription("Retrieve ABAP class source code"),
		mcp.WithString("class_name",
			mcp.Required(),
			mcp.Description("Name of the ABAP class"),
		),
	), s.handleGetClass)
	}


	// GetInterface
	if shouldRegister("GetInterface") {
		s.mcpServer.AddTool(mcp.NewTool("GetInterface",
		mcp.WithDescription("Retrieve ABAP interface source code"),
		mcp.WithString("interface_name",
			mcp.Required(),
			mcp.Description("Name of the ABAP interface"),
		),
	), s.handleGetInterface)
	}


	// GetFunction
	if shouldRegister("GetFunction") {
		s.mcpServer.AddTool(mcp.NewTool("GetFunction",
		mcp.WithDescription("Retrieve ABAP Function Module source code"),
		mcp.WithString("function_name",
			mcp.Required(),
			mcp.Description("Name of the function module"),
		),
		mcp.WithString("function_group",
			mcp.Required(),
			mcp.Description("Name of the function group"),
		),
	), s.handleGetFunction)
	}


	// GetFunctionGroup
	if shouldRegister("GetFunctionGroup") {
		s.mcpServer.AddTool(mcp.NewTool("GetFunctionGroup",
		mcp.WithDescription("Retrieve ABAP Function Group source code"),
		mcp.WithString("function_group",
			mcp.Required(),
			mcp.Description("Name of the function group"),
		),
	), s.handleGetFunctionGroup)
	}


	// GetInclude
	if shouldRegister("GetInclude") {
		s.mcpServer.AddTool(mcp.NewTool("GetInclude",
		mcp.WithDescription("Retrieve ABAP Include Source Code"),
		mcp.WithString("include_name",
			mcp.Required(),
			mcp.Description("Name of the ABAP Include"),
		),
	), s.handleGetInclude)
	}


	// GetTable
	if shouldRegister("GetTable") {
		s.mcpServer.AddTool(mcp.NewTool("GetTable",
		mcp.WithDescription("Retrieve ABAP table structure"),
		mcp.WithString("table_name",
			mcp.Required(),
			mcp.Description("Name of the ABAP table"),
		),
	), s.handleGetTable)
	}


	// GetTableContents
	if shouldRegister("GetTableContents") {
		s.mcpServer.AddTool(mcp.NewTool("GetTableContents",
		mcp.WithDescription("Retrieve contents of an ABAP table. For simple queries use table_name + max_rows. For filtered queries use sql_query parameter with ABAP SQL syntax (use ASCENDING/DESCENDING, not ASC/DESC)."),
		mcp.WithString("table_name",
			mcp.Required(),
			mcp.Description("Name of the ABAP table"),
		),
		mcp.WithNumber("max_rows",
			mcp.Description("Maximum number of rows to retrieve (default 100). Use this instead of SQL LIMIT clause"),
		),
		mcp.WithString("sql_query",
			mcp.Description("Optional ABAP SQL SELECT statement. Uses ABAP syntax: ASCENDING/DESCENDING work, ASC/DESC fail. Example: SELECT * FROM T000 WHERE MANDT = '001' ORDER BY MANDT DESCENDING"),
		),
	), s.handleGetTableContents)
	}


	// RunQuery
	if shouldRegister("RunQuery") {
		s.mcpServer.AddTool(mcp.NewTool("RunQuery",
		mcp.WithDescription("Execute a freestyle SQL query against the SAP database. IMPORTANT: Uses ABAP SQL syntax, NOT standard SQL. Use ASCENDING/DESCENDING instead of ASC/DESC. Use max_rows parameter instead of LIMIT. GROUP BY and WHERE work normally."),
		mcp.WithString("sql_query",
			mcp.Required(),
			mcp.Description("ABAP SQL query. Example: SELECT carrid, COUNT(*) as cnt FROM sflight GROUP BY carrid ORDER BY cnt DESCENDING. Note: ASC/DESC keywords fail - use ASCENDING/DESCENDING"),
		),
		mcp.WithNumber("max_rows",
			mcp.Description("Maximum number of rows to retrieve (default 100). Use this instead of SQL LIMIT clause"),
		),
	), s.handleRunQuery)
	}


	// GetCDSDependencies
	if shouldRegister("GetCDSDependencies") {
		s.mcpServer.AddTool(mcp.NewTool("GetCDSDependencies",
		mcp.WithDescription("Retrieve CDS view FORWARD dependencies (tables/views this CDS reads FROM). Returns tree of base objects. Does NOT return reverse dependencies (where-used). Use with GetSource(DDLS) to read CDS source code."),
		mcp.WithString("ddls_name",
			mcp.Required(),
			mcp.Description("CDS DDL source name (e.g., 'ZRAY_00_I_DOC_NODE_00'). Use SearchObject to find CDS views first."),
		),
		mcp.WithString("dependency_level",
			mcp.Description("Level of dependency resolution: 'unit' (direct only) or 'hierarchy' (recursive). Default: 'hierarchy'"),
		),
		mcp.WithBoolean("with_associations",
			mcp.Description("Include modeled associations in dependency tree. Default: false"),
		),
		mcp.WithString("context_package",
			mcp.Description("Filter dependencies to specific package context"),
		),
	), s.handleGetCDSDependencies)
	}


	// GetStructure
	if shouldRegister("GetStructure") {
		s.mcpServer.AddTool(mcp.NewTool("GetStructure",
		mcp.WithDescription("Retrieve ABAP Structure"),
		mcp.WithString("structure_name",
			mcp.Required(),
			mcp.Description("Name of the ABAP Structure"),
		),
	), s.handleGetStructure)
	}


	// GetPackage
	if shouldRegister("GetPackage") {
		s.mcpServer.AddTool(mcp.NewTool("GetPackage",
		mcp.WithDescription("Retrieve ABAP package details"),
		mcp.WithString("package_name",
			mcp.Required(),
			mcp.Description("Name of the ABAP package"),
		),
	), s.handleGetPackage)
	}

	// GetMessages - Message class texts (SE91)
	if shouldRegister("GetMessages") {
		s.mcpServer.AddTool(mcp.NewTool("GetMessages",
			mcp.WithDescription("Get all messages from an ABAP message class (SE91). Returns message number, text for all messages in the class. Use SearchObject to find message classes first."),
			mcp.WithString("message_class",
				mcp.Required(),
				mcp.Description("Name of the message class (e.g., 'ZRAY_00', 'SY')"),
			),
		), s.handleGetMessages)
	}


	// GetTransaction
	if shouldRegister("GetTransaction") {
		s.mcpServer.AddTool(mcp.NewTool("GetTransaction",
		mcp.WithDescription("Retrieve ABAP transaction details"),
		mcp.WithString("transaction_name",
			mcp.Required(),
			mcp.Description("Name of the ABAP transaction"),
		),
	), s.handleGetTransaction)
	}


	// GetTypeInfo
	if shouldRegister("GetTypeInfo") {
		s.mcpServer.AddTool(mcp.NewTool("GetTypeInfo",
		mcp.WithDescription("Retrieve ABAP type information"),
		mcp.WithString("type_name",
			mcp.Required(),
			mcp.Description("Name of the ABAP type"),
		),
	), s.handleGetTypeInfo)
	}


	// --- System Information ---

	// GetSystemInfo
	if shouldRegister("GetSystemInfo") {
		s.mcpServer.AddTool(mcp.NewTool("GetSystemInfo",
			mcp.WithDescription("Get SAP system information (system ID, release, kernel, database)"),
		), s.handleGetSystemInfo)
	}

	// GetInstalledComponents
	if shouldRegister("GetInstalledComponents") {
		s.mcpServer.AddTool(mcp.NewTool("GetInstalledComponents",
			mcp.WithDescription("List installed software components with version information"),
		), s.handleGetInstalledComponents)
	}

	// GetConnectionInfo - Self-inspection tool
	// Always registered - useful for debugging and introspection
	s.mcpServer.AddTool(mcp.NewTool("GetConnectionInfo",
		mcp.WithDescription("Get current MCP connection info: user, URL, client. Useful for debugging and understanding current session context."),
	), s.handleGetConnectionInfo)

	// GetFeatures - Feature Detection (Safety Network)
	// Always registered - provides visibility into what's available
	s.mcpServer.AddTool(mcp.NewTool("GetFeatures",
		mcp.WithDescription("Probe SAP system for available features. Returns status of optional capabilities like abapGit, RAP/OData, AMDP debugging, UI5/BSP, and CTS transports. Use this to understand what features are available before attempting to use them."),
	), s.handleGetFeatures)

	// --- Code Analysis Infrastructure (CAI) ---

	// GetCallGraph
	if shouldRegister("GetCallGraph") {
		s.mcpServer.AddTool(mcp.NewTool("GetCallGraph",
			mcp.WithDescription("Get call hierarchy for methods/functions. Shows callers or callees of an ABAP object."),
			mcp.WithString("object_uri",
				mcp.Required(),
				mcp.Description("ADT URI of the object (e.g., /sap/bc/adt/oo/classes/ZCL_TEST/source/main#start=10,1)"),
			),
			mcp.WithString("direction",
				mcp.Description("Direction: 'callers' (who calls this) or 'callees' (what this calls). Default: callers"),
			),
			mcp.WithNumber("max_depth",
				mcp.Description("Maximum depth of call hierarchy (default: 3)"),
			),
			mcp.WithNumber("max_results",
				mcp.Description("Maximum number of results (default: 100)"),
			),
		), s.handleGetCallGraph)
	}

	// GetObjectStructure
	if shouldRegister("GetObjectStructure") {
		s.mcpServer.AddTool(mcp.NewTool("GetObjectStructure",
			mcp.WithDescription("Get object explorer tree structure. Returns hierarchical view of object components."),
			mcp.WithString("object_name",
				mcp.Required(),
				mcp.Description("Object name (e.g., ZCL_TEST, ZPROGRAM)"),
			),
			mcp.WithNumber("max_results",
				mcp.Description("Maximum number of results (default: 100)"),
			),
		), s.handleGetObjectStructure)
	}

	// GetCallersOf - simplified up traversal
	if shouldRegister("GetCallersOf") {
		s.mcpServer.AddTool(mcp.NewTool("GetCallersOf",
			mcp.WithDescription("Find all callers of an ABAP object (up traversal). Shows who calls this method/function. Simplified wrapper around GetCallGraph."),
			mcp.WithString("object_uri",
				mcp.Required(),
				mcp.Description("ADT URI of the object (e.g., /sap/bc/adt/oo/classes/ZCL_TEST/source/main#start=10,1)"),
			),
			mcp.WithNumber("max_depth",
				mcp.Description("Maximum depth of caller hierarchy (default: 5)"),
			),
		), s.handleGetCallersOf)
	}

	// GetCalleesOf - simplified down traversal
	if shouldRegister("GetCalleesOf") {
		s.mcpServer.AddTool(mcp.NewTool("GetCalleesOf",
			mcp.WithDescription("Find all callees of an ABAP object (down traversal). Shows what this method/function calls. Simplified wrapper around GetCallGraph."),
			mcp.WithString("object_uri",
				mcp.Required(),
				mcp.Description("ADT URI of the object (e.g., /sap/bc/adt/oo/classes/ZCL_TEST/source/main#start=10,1)"),
			),
			mcp.WithNumber("max_depth",
				mcp.Description("Maximum depth of callee hierarchy (default: 5)"),
			),
		), s.handleGetCalleesOf)
	}

	// AnalyzeCallGraph - get call graph statistics
	if shouldRegister("AnalyzeCallGraph") {
		s.mcpServer.AddTool(mcp.NewTool("AnalyzeCallGraph",
			mcp.WithDescription("Analyze call graph for an object. Returns statistics: total nodes, edges, max depth, nodes by type. Use for understanding code complexity and dependencies."),
			mcp.WithString("object_uri",
				mcp.Required(),
				mcp.Description("ADT URI of the object to analyze"),
			),
			mcp.WithString("direction",
				mcp.Description("Direction: 'callers' or 'callees' (default: callees)"),
			),
			mcp.WithNumber("max_depth",
				mcp.Description("Maximum depth to analyze (default: 5)"),
			),
		), s.handleAnalyzeCallGraph)
	}

	// CompareCallGraphs - compare static vs actual execution
	if shouldRegister("CompareCallGraphs") {
		s.mcpServer.AddTool(mcp.NewTool("CompareCallGraphs",
			mcp.WithDescription("Compare static call graph with actual execution trace. Identifies: common paths, untested paths (static only), and dynamic calls (actual only). Use for test coverage analysis and RCA."),
			mcp.WithString("object_uri",
				mcp.Required(),
				mcp.Description("ADT URI of the root object"),
			),
			mcp.WithString("trace_data",
				mcp.Required(),
				mcp.Description("JSON array of trace edges from execution (format: [{caller_name, callee_name}, ...])"),
			),
		), s.handleCompareCallGraphs)
	}

	// TraceExecution - composite RCA tool
	if shouldRegister("TraceExecution") {
		s.mcpServer.AddTool(mcp.NewTool("TraceExecution",
			mcp.WithDescription("COMPOSITE RCA TOOL: Performs traced execution analysis. 1) Builds static call graph from object, 2) Optionally runs unit tests, 3) Collects trace data, 4) Extracts actual call edges, 5) Compares static vs actual for root cause analysis."),
			mcp.WithString("object_uri",
				mcp.Required(),
				mcp.Description("ADT URI of the starting object for static call graph"),
			),
			mcp.WithNumber("max_depth",
				mcp.Description("Maximum depth for call graph traversal (default: 5)"),
			),
			mcp.WithBoolean("run_tests",
				mcp.Description("Run unit tests before collecting trace (default: false)"),
			),
			mcp.WithString("test_object_uri",
				mcp.Description("Object URI for tests to run (defaults to object_uri)"),
			),
			mcp.WithString("trace_user",
				mcp.Description("Filter traces by user (defaults to current user)"),
			),
		), s.handleTraceExecution)
	}

	// --- Runtime Errors / Short Dumps (RABAX) ---

	// ListDumps (renamed from GetDumps for consistency with List* pattern)
	if shouldRegister("ListDumps") {
		s.mcpServer.AddTool(mcp.NewTool("ListDumps",
			mcp.WithDescription("List runtime errors (short dumps) from the SAP system. Filter by user, exception type, program, date range."),
			mcp.WithString("user",
				mcp.Description("Filter by username"),
			),
			mcp.WithString("exception_type",
				mcp.Description("Filter by exception type (e.g., CX_SY_ZERODIVIDE)"),
			),
			mcp.WithString("program",
				mcp.Description("Filter by program name"),
			),
			mcp.WithString("package",
				mcp.Description("Filter by package"),
			),
			mcp.WithString("date_from",
				mcp.Description("Start date (YYYYMMDD format)"),
			),
			mcp.WithString("date_to",
				mcp.Description("End date (YYYYMMDD format)"),
			),
			mcp.WithNumber("max_results",
				mcp.Description("Maximum number of results (default: 100)"),
			),
		), s.handleListDumps)
	}

	// GetDump
	if shouldRegister("GetDump") {
		s.mcpServer.AddTool(mcp.NewTool("GetDump",
			mcp.WithDescription("Get full details of a specific runtime error (short dump) including stack trace."),
			mcp.WithString("dump_id",
				mcp.Required(),
				mcp.Description("Dump ID from ListDumps result"),
			),
		), s.handleGetDump)
	}

	// --- ABAP Profiler / Runtime Traces (ATRA) ---

	// ListTraces
	if shouldRegister("ListTraces") {
		s.mcpServer.AddTool(mcp.NewTool("ListTraces",
			mcp.WithDescription("List ABAP runtime traces (profiler results) from the SAP system."),
			mcp.WithString("user",
				mcp.Description("Filter by username"),
			),
			mcp.WithString("process_type",
				mcp.Description("Filter by process type"),
			),
			mcp.WithString("object_type",
				mcp.Description("Filter by object type"),
			),
			mcp.WithNumber("max_results",
				mcp.Description("Maximum number of results (default: 100)"),
			),
		), s.handleListTraces)
	}

	// GetTrace
	if shouldRegister("GetTrace") {
		s.mcpServer.AddTool(mcp.NewTool("GetTrace",
			mcp.WithDescription("Get trace analysis (hitlist, statements, or database accesses) for a specific trace."),
			mcp.WithString("trace_id",
				mcp.Required(),
				mcp.Description("Trace ID from ListTraces result"),
			),
			mcp.WithString("tool_type",
				mcp.Description("Analysis type: 'hitlist' (default), 'statements', 'dbAccesses'"),
			),
		), s.handleGetTrace)
	}

	// --- SQL Trace (ST05) ---

	// GetSQLTraceState
	if shouldRegister("GetSQLTraceState") {
		s.mcpServer.AddTool(mcp.NewTool("GetSQLTraceState",
			mcp.WithDescription("Check if SQL trace (ST05) is currently active."),
		), s.handleGetSQLTraceState)
	}

	// ListSQLTraces
	if shouldRegister("ListSQLTraces") {
		s.mcpServer.AddTool(mcp.NewTool("ListSQLTraces",
			mcp.WithDescription("List SQL trace files from ST05."),
			mcp.WithString("user",
				mcp.Description("Filter by username"),
			),
			mcp.WithNumber("max_results",
				mcp.Description("Maximum number of results (default: 100)"),
			),
		), s.handleListSQLTraces)
	}

	// --- Debugger Session (WebSocket-based via ZADT_VSP) ---
	// All debugger tools use WebSocket connection to ZADT_VSP for reliable operation.
	// REST-based breakpoint tools were removed due to CSRF issues.

	// SetBreakpoint - WebSocket-based (supports line, statement, and exception breakpoints)
	if shouldRegister("SetBreakpoint") {
		s.mcpServer.AddTool(mcp.NewTool("SetBreakpoint",
			mcp.WithDescription("Set a breakpoint in ABAP code. Supports three types: 'line' (specific location), 'statement' (ABAP keyword), 'exception' (exception class). For class methods, use 'method' parameter for include-relative line numbers. Uses WebSocket connection to ZADT_VSP."),
			mcp.WithString("kind",
				mcp.Description("Breakpoint type: 'line' (default), 'statement', or 'exception'"),
			),
			mcp.WithString("program",
				mcp.Description("Program name for line breakpoints (e.g., 'ZADT_DBG_PROG' or 'ZCL_MY_CLASS')"),
			),
			mcp.WithString("method",
				mcp.Description("Method name for class breakpoints. When specified, line number is relative to method start (line 1 = first line of METHOD implementation). Enables accurate breakpoints in class methods."),
			),
			mcp.WithNumber("line",
				mcp.Description("Line number for line breakpoints. Without 'method': pool-absolute line. With 'method': relative to method start."),
			),
			mcp.WithString("statement",
				mcp.Description("ABAP statement for statement breakpoints (e.g., 'CALL FUNCTION', 'SELECT', 'LOOP', 'CALL METHOD')"),
			),
			mcp.WithString("exception",
				mcp.Description("Exception class for exception breakpoints (e.g., 'CX_SY_ZERODIVIDE', 'CX_SY_OPEN_SQL_DB')"),
			),
		), s.handleSetBreakpoint)
	}

	// GetBreakpoints - WebSocket-based
	if shouldRegister("GetBreakpoints") {
		s.mcpServer.AddTool(mcp.NewTool("GetBreakpoints",
			mcp.WithDescription("Get all breakpoints registered in the current debug session. Uses WebSocket connection to ZADT_VSP."),
		), s.handleGetBreakpoints)
	}

	// DeleteBreakpoint - WebSocket-based
	if shouldRegister("DeleteBreakpoint") {
		s.mcpServer.AddTool(mcp.NewTool("DeleteBreakpoint",
			mcp.WithDescription("Delete a breakpoint by ID. Uses WebSocket connection to ZADT_VSP."),
			mcp.WithString("breakpoint_id",
				mcp.Required(),
				mcp.Description("ID of the breakpoint to delete"),
			),
		), s.handleDeleteBreakpoint)
	}

	// CallRFC - WebSocket-based RFC execution
	if shouldRegister("CallRFC") {
		s.mcpServer.AddTool(mcp.NewTool("CallRFC",
			mcp.WithDescription("Call a function module via WebSocket (ZADT_VSP). Useful for triggering ABAP code execution to hit breakpoints. Parameters are passed as key-value pairs."),
			mcp.WithString("function",
				mcp.Required(),
				mcp.Description("Function module name (e.g., 'RFC_PING', 'BAPI_USER_GET_DETAIL')"),
			),
			mcp.WithString("params",
				mcp.Description("JSON object with function parameters (e.g., '{\"IV_PARAM\":\"value\"}')"),
			),
		), s.handleCallRFC)
	}

	// MoveObject - Move object to different package via WebSocket
	if shouldRegister("MoveObject") {
		s.mcpServer.AddTool(mcp.NewTool("MoveObject",
			mcp.WithDescription("Move an ABAP object to a different package. Uses ZADT_VSP WebSocket to call TR_TADIR_INTERFACE. Requires ZADT_VSP deployed."),
			mcp.WithString("object_type",
				mcp.Required(),
				mcp.Description("Object type: CLAS, PROG, INTF, FUGR, TABL, etc."),
			),
			mcp.WithString("object_name",
				mcp.Required(),
				mcp.Description("Name of the object to move (e.g., 'ZCL_TEST')"),
			),
			mcp.WithString("new_package",
				mcp.Required(),
				mcp.Description("Target package (e.g., '$ZRAY', 'ZPACKAGE')"),
			),
		), s.handleMoveObject)
	}

	// DebuggerListen
	if shouldRegister("DebuggerListen") {
		s.mcpServer.AddTool(mcp.NewTool("DebuggerListen",
			mcp.WithDescription("Start a debug listener that waits for a debuggee to hit a breakpoint. This is a BLOCKING call that uses long-polling. Returns when a debuggee is caught, timeout occurs, or a conflict is detected."),
			mcp.WithString("user",
				mcp.Description("User to listen for (defaults to current user)"),
			),
			mcp.WithNumber("timeout",
				mcp.Description("Timeout in seconds (default: 60, max: 240)"),
			),
		), s.handleDebuggerListen)
	}

	// DebuggerAttach
	if shouldRegister("DebuggerAttach") {
		s.mcpServer.AddTool(mcp.NewTool("DebuggerAttach",
			mcp.WithDescription("Attach to a debuggee that has hit a breakpoint. Use the debuggee_id from DebuggerListen result."),
			mcp.WithString("debuggee_id",
				mcp.Required(),
				mcp.Description("ID of the debuggee (from DebuggerListen result)"),
			),
			mcp.WithString("user",
				mcp.Description("User for debugging (defaults to current user)"),
			),
		), s.handleDebuggerAttach)
	}

	// DebuggerDetach
	if shouldRegister("DebuggerDetach") {
		s.mcpServer.AddTool(mcp.NewTool("DebuggerDetach",
			mcp.WithDescription("Detach from the current debug session and release the debuggee."),
		), s.handleDebuggerDetach)
	}

	// DebuggerStep
	if shouldRegister("DebuggerStep") {
		s.mcpServer.AddTool(mcp.NewTool("DebuggerStep",
			mcp.WithDescription("Perform a step operation in the debugger."),
			mcp.WithString("step_type",
				mcp.Required(),
				mcp.Description("Step type: 'stepInto', 'stepOver', 'stepReturn', 'stepContinue', 'stepRunToLine', 'stepJumpToLine'"),
			),
			mcp.WithString("uri",
				mcp.Description("Target URI for stepRunToLine/stepJumpToLine (e.g., '/sap/bc/adt/programs/programs/ZTEST/source/main#start=42')"),
			),
		), s.handleDebuggerStep)
	}

	// DebuggerGetStack
	if shouldRegister("DebuggerGetStack") {
		s.mcpServer.AddTool(mcp.NewTool("DebuggerGetStack",
			mcp.WithDescription("Get the current call stack during a debug session."),
		), s.handleDebuggerGetStack)
	}

	// DebuggerGetVariables
	if shouldRegister("DebuggerGetVariables") {
		s.mcpServer.AddTool(mcp.NewTool("DebuggerGetVariables",
			mcp.WithDescription("Get variable values during a debug session. Use '@ROOT' to get top-level variables, or specific variable IDs to get their values."),
			mcp.WithArray("variable_ids",
				mcp.Description("Variable IDs to retrieve (e.g., ['@ROOT'] for top-level, or specific IDs like ['LV_COUNT', 'LS_DATA'])"),
			),
		), s.handleDebuggerGetVariables)
	}

	// SearchObject
	if shouldRegister("SearchObject") {
		s.mcpServer.AddTool(mcp.NewTool("SearchObject",
		mcp.WithDescription("Search for ABAP objects using quick search"),
		mcp.WithString("query",
			mcp.Required(),
			mcp.Description("Search query string (use * wildcard for partial match)"),
		),
		mcp.WithNumber("maxResults",
			mcp.Description("Maximum number of results to return (default 100)"),
		),
	), s.handleSearchObject)
	}


	// --- Development Tools ---

	// SyntaxCheck
	if shouldRegister("SyntaxCheck") {
		s.mcpServer.AddTool(mcp.NewTool("SyntaxCheck",
		mcp.WithDescription("Check ABAP source code for syntax errors"),
		mcp.WithString("object_url",
			mcp.Required(),
			mcp.Description("ADT URL of the object (e.g., /sap/bc/adt/programs/programs/ZTEST)"),
		),
		mcp.WithString("content",
			mcp.Required(),
			mcp.Description("ABAP source code to check"),
		),
	), s.handleSyntaxCheck)
	}


	// Activate
	if shouldRegister("Activate") {
		s.mcpServer.AddTool(mcp.NewTool("Activate",
		mcp.WithDescription("Activate an ABAP object"),
		mcp.WithString("object_url",
			mcp.Required(),
			mcp.Description("ADT URL of the object (e.g., /sap/bc/adt/programs/programs/ZTEST)"),
		),
		mcp.WithString("object_name",
			mcp.Required(),
			mcp.Description("Technical name of the object (e.g., ZTEST)"),
		),
	), s.handleActivate)
	}

	// ActivatePackage - Batch activation of inactive objects
	if shouldRegister("ActivatePackage") {
		s.mcpServer.AddTool(mcp.NewTool("ActivatePackage",
			mcp.WithDescription("Activate all inactive objects. Objects are sorted by dependency order (interfaces before classes). If no package specified, activates ALL inactive objects for current user."),
			mcp.WithString("package",
				mcp.Description("Package name to filter (optional, empty = all packages)"),
			),
			mcp.WithNumber("max_objects",
				mcp.Description("Maximum number of objects to activate (default: 100)"),
			),
		), s.handleActivatePackage)
	}

	// RunUnitTests
	if shouldRegister("RunUnitTests") {
		s.mcpServer.AddTool(mcp.NewTool("RunUnitTests",
		mcp.WithDescription("Run ABAP Unit tests for an object"),
		mcp.WithString("object_url",
			mcp.Required(),
			mcp.Description("ADT URL of the object (e.g., /sap/bc/adt/oo/classes/ZCL_TEST)"),
		),
		mcp.WithBoolean("include_dangerous",
			mcp.Description("Include dangerous risk level tests (default: false)"),
		),
		mcp.WithBoolean("include_long",
			mcp.Description("Include long duration tests (default: false)"),
		),
	), s.handleRunUnitTests)
	}

	// --- ATC (Code Quality) ---

	// RunATCCheck - Convenience tool (combines variant + run + worklist)
	if shouldRegister("RunATCCheck") {
		s.mcpServer.AddTool(mcp.NewTool("RunATCCheck",
			mcp.WithDescription("Run ATC (ABAP Test Cockpit) code quality check on an object. Returns findings with priority, check title, message, and location. Priority: 1=Error, 2=Warning, 3=Info."),
			mcp.WithString("object_url",
				mcp.Required(),
				mcp.Description("ADT URL of the object (e.g., /sap/bc/adt/oo/classes/ZCL_TEST)"),
			),
			mcp.WithString("variant",
				mcp.Description("Check variant name (empty = use system default)"),
			),
			mcp.WithNumber("max_results",
				mcp.Description("Maximum number of findings to return (default: 100)"),
			),
		), s.handleRunATCCheck)
	}

	// GetATCCustomizing - Expert mode: get ATC configuration
	if shouldRegister("GetATCCustomizing") {
		s.mcpServer.AddTool(mcp.NewTool("GetATCCustomizing",
			mcp.WithDescription("Get ATC system configuration including default check variant and exemption reasons"),
		), s.handleGetATCCustomizing)
	}


	// --- CRUD Operations ---

	// LockObject
	if shouldRegister("LockObject") {
		s.mcpServer.AddTool(mcp.NewTool("LockObject",
		mcp.WithDescription("Acquire an edit lock on an ABAP object"),
		mcp.WithString("object_url",
			mcp.Required(),
			mcp.Description("ADT URL of the object (e.g., /sap/bc/adt/programs/programs/ZTEST)"),
		),
		mcp.WithString("access_mode",
			mcp.Description("Access mode: MODIFY (default) or READ"),
		),
	), s.handleLockObject)
	}


	// UnlockObject
	if shouldRegister("UnlockObject") {
		s.mcpServer.AddTool(mcp.NewTool("UnlockObject",
		mcp.WithDescription("Release an edit lock on an ABAP object"),
		mcp.WithString("object_url",
			mcp.Required(),
			mcp.Description("ADT URL of the object (e.g., /sap/bc/adt/programs/programs/ZTEST)"),
		),
		mcp.WithString("lock_handle",
			mcp.Required(),
			mcp.Description("Lock handle from LockObject"),
		),
	), s.handleUnlockObject)
	}


	// UpdateSource
	if shouldRegister("UpdateSource") {
		s.mcpServer.AddTool(mcp.NewTool("UpdateSource",
		mcp.WithDescription("Write source code to an ABAP object (requires lock)"),
		mcp.WithString("object_url",
			mcp.Required(),
			mcp.Description("ADT URL of the object (e.g., /sap/bc/adt/programs/programs/ZTEST)"),
		),
		mcp.WithString("source",
			mcp.Required(),
			mcp.Description("ABAP source code to write"),
		),
		mcp.WithString("lock_handle",
			mcp.Required(),
			mcp.Description("Lock handle from LockObject"),
		),
		mcp.WithString("transport",
			mcp.Description("Transport request number (optional for local packages)"),
		),
	), s.handleUpdateSource)
	}


	// CreateObject
	if shouldRegister("CreateObject") {
		s.mcpServer.AddTool(mcp.NewTool("CreateObject",
		mcp.WithDescription("Create a new ABAP object. Supports: PROG/P (program), CLAS/OC (class), INTF/OI (interface), PROG/I (include), FUGR/F (function group), FUGR/FF (function module), DEVC/K (package), DDLS/DF (CDS view), BDEF/BDO (behavior definition), SRVD/SRV (service definition), SRVB/SVB (service binding)"),
		mcp.WithString("object_type",
			mcp.Required(),
			mcp.Description("Object type: PROG/P, CLAS/OC, INTF/OI, PROG/I, FUGR/F, FUGR/FF, DEVC/K, DDLS/DF, BDEF/BDO, SRVD/SRV, SRVB/SVB"),
		),
		mcp.WithString("name",
			mcp.Required(),
			mcp.Description("Object name (e.g., ZTEST_PROGRAM)"),
		),
		mcp.WithString("description",
			mcp.Required(),
			mcp.Description("Object description"),
		),
		mcp.WithString("package_name",
			mcp.Required(),
			mcp.Description("Package name (e.g., $TMP for local, ZPACKAGE for transportable)"),
		),
		mcp.WithString("transport",
			mcp.Description("Transport request number (required for non-local packages)"),
		),
		mcp.WithString("parent_name",
			mcp.Description("Parent name (required for function modules - the function group name)"),
		),
		// RAP-specific options
		mcp.WithString("service_definition",
			mcp.Description("For SRVB: the service definition name to bind"),
		),
		mcp.WithString("binding_version",
			mcp.Description("For SRVB: OData version 'V2' or 'V4' (default: V2)"),
		),
		mcp.WithString("binding_category",
			mcp.Description("For SRVB: '0' for Web API, '1' for UI (default: 0)"),
		),
	), s.handleCreateObject)
	}

	// CreatePackage - simplified package creation for focused mode
	if shouldRegister("CreatePackage") {
		s.mcpServer.AddTool(mcp.NewTool("CreatePackage",
		mcp.WithDescription("Create a new local ABAP package. Only local packages (starting with $) are supported. For development/testing purposes."),
		mcp.WithString("name",
			mcp.Required(),
			mcp.Description("Package name (must start with $, e.g., $ZTEST, $ZLOCAL_DEV)"),
		),
		mcp.WithString("description",
			mcp.Required(),
			mcp.Description("Package description"),
		),
		mcp.WithString("parent",
			mcp.Description("Parent package name (optional, e.g., $TMP). If not specified, creates a root-level local package."),
		),
	), s.handleCreatePackage)
	}

	// CreateTable - Create DDIC tables from JSON
	if shouldRegister("CreateTable") {
		s.mcpServer.AddTool(mcp.NewTool("CreateTable",
			mcp.WithDescription("Create a DDIC transparent table from a simple JSON definition. Handles full workflow: create → set source → activate. Supports common ABAP types: CHAR, NUMC, INT4, DEC, STRING, TIMESTAMPL, UUID, etc."),
			mcp.WithString("name",
				mcp.Required(),
				mcp.Description("Table name (uppercase, max 30 chars, must start with Z/Y)"),
			),
			mcp.WithString("description",
				mcp.Required(),
				mcp.Description("Short description of the table"),
			),
			mcp.WithString("package",
				mcp.Description("Target package (default: $TMP)"),
			),
			mcp.WithString("fields",
				mcp.Required(),
				mcp.Description("JSON array of fields: [{\"name\":\"ID\",\"type\":\"CHAR32\",\"key\":true},{\"name\":\"VALUE\",\"type\":\"STRING\"}]. Types: CHAR/CHARnn, NUMC/NUMCnn, INT4, DEC, STRING, TIMESTAMPL, UUID, DATS, TIMS, or data element name."),
			),
			mcp.WithString("transport",
				mcp.Description("Transport request number (optional for $TMP)"),
			),
			mcp.WithString("delivery_class",
				mcp.Description("Delivery class: A=Application (default), C=Customizing, L=Temporary"),
			),
		), s.handleCreateTable)
	}

	// CompareSource - Diff two objects
	if shouldRegister("CompareSource") {
		s.mcpServer.AddTool(mcp.NewTool("CompareSource",
			mcp.WithDescription("Compare source code of two objects and return unified diff. Supports all object types from GetSource."),
			mcp.WithString("type1",
				mcp.Required(),
				mcp.Description("Object type of first object: PROG, CLAS, INTF, FUNC, FUGR, INCL, DDLS, BDEF, SRVD"),
			),
			mcp.WithString("name1",
				mcp.Required(),
				mcp.Description("Name of first object"),
			),
			mcp.WithString("type2",
				mcp.Required(),
				mcp.Description("Object type of second object (can be same or different)"),
			),
			mcp.WithString("name2",
				mcp.Required(),
				mcp.Description("Name of second object"),
			),
			mcp.WithString("include1",
				mcp.Description("Class include type for first object if CLAS: definitions, implementations, macros, testclasses"),
			),
			mcp.WithString("include2",
				mcp.Description("Class include type for second object if CLAS"),
			),
			mcp.WithString("parent1",
				mcp.Description("Function group for first object if FUNC"),
			),
			mcp.WithString("parent2",
				mcp.Description("Function group for second object if FUNC"),
			),
		), s.handleCompareSource)
	}

	// CloneObject - Copy object to new name
	if shouldRegister("CloneObject") {
		s.mcpServer.AddTool(mcp.NewTool("CloneObject",
			mcp.WithDescription("Copy an ABAP object to a new name. Replaces object name in source. Supports PROG, CLAS, INTF."),
			mcp.WithString("object_type",
				mcp.Required(),
				mcp.Description("Object type: PROG, CLAS, INTF"),
			),
			mcp.WithString("source_name",
				mcp.Required(),
				mcp.Description("Name of object to copy"),
			),
			mcp.WithString("target_name",
				mcp.Required(),
				mcp.Description("Name for the new object"),
			),
			mcp.WithString("package",
				mcp.Required(),
				mcp.Description("Target package (e.g., $TMP)"),
			),
		), s.handleCloneObject)
	}

	// GetClassInfo - Quick class metadata
	if shouldRegister("GetClassInfo") {
		s.mcpServer.AddTool(mcp.NewTool("GetClassInfo",
			mcp.WithDescription("Get class metadata without full source: methods, attributes, interfaces, superclass, abstract/final status."),
			mcp.WithString("class_name",
				mcp.Required(),
				mcp.Description("Name of the ABAP class"),
			),
		), s.handleGetClassInfo)
	}

	// DeleteObject
	if shouldRegister("DeleteObject") {
		s.mcpServer.AddTool(mcp.NewTool("DeleteObject",
		mcp.WithDescription("Delete an ABAP object (requires lock)"),
		mcp.WithString("object_url",
			mcp.Required(),
			mcp.Description("ADT URL of the object (e.g., /sap/bc/adt/programs/programs/ZTEST)"),
		),
		mcp.WithString("lock_handle",
			mcp.Required(),
			mcp.Description("Lock handle from LockObject"),
		),
		mcp.WithString("transport",
			mcp.Description("Transport request number (optional for local packages)"),
		),
	), s.handleDeleteObject)
	}


	// --- Class Include Operations ---

	// GetClassInclude
	if shouldRegister("GetClassInclude") {
		s.mcpServer.AddTool(mcp.NewTool("GetClassInclude",
		mcp.WithDescription("Retrieve source code of a class include (definitions, implementations, macros, testclasses)"),
		mcp.WithString("class_name",
			mcp.Required(),
			mcp.Description("Name of the ABAP class"),
		),
		mcp.WithString("include_type",
			mcp.Required(),
			mcp.Description("Include type: main, definitions, implementations, macros, testclasses"),
		),
	), s.handleGetClassInclude)
	}


	// CreateTestInclude
	if shouldRegister("CreateTestInclude") {
		s.mcpServer.AddTool(mcp.NewTool("CreateTestInclude",
		mcp.WithDescription("Create the test classes include for a class (required before writing test code)"),
		mcp.WithString("class_name",
			mcp.Required(),
			mcp.Description("Name of the ABAP class"),
		),
		mcp.WithString("lock_handle",
			mcp.Required(),
			mcp.Description("Lock handle from LockObject (lock the parent class first)"),
		),
		mcp.WithString("transport",
			mcp.Description("Transport request number (optional for local packages)"),
		),
	), s.handleCreateTestInclude)
	}


	// UpdateClassInclude
	if shouldRegister("UpdateClassInclude") {
		s.mcpServer.AddTool(mcp.NewTool("UpdateClassInclude",
		mcp.WithDescription("Update source code of a class include (requires lock on parent class)"),
		mcp.WithString("class_name",
			mcp.Required(),
			mcp.Description("Name of the ABAP class"),
		),
		mcp.WithString("include_type",
			mcp.Required(),
			mcp.Description("Include type: main, definitions, implementations, macros, testclasses"),
		),
		mcp.WithString("source",
			mcp.Required(),
			mcp.Description("ABAP source code to write"),
		),
		mcp.WithString("lock_handle",
			mcp.Required(),
			mcp.Description("Lock handle from LockObject (lock the parent class first)"),
		),
		mcp.WithString("transport",
			mcp.Description("Transport request number (optional for local packages)"),
		),
	), s.handleUpdateClassInclude)
	}


	// PublishServiceBinding
	if shouldRegister("PublishServiceBinding") {
		s.mcpServer.AddTool(mcp.NewTool("PublishServiceBinding",
		mcp.WithDescription("Publish a service binding to make it available as OData service"),
		mcp.WithString("service_name",
			mcp.Required(),
			mcp.Description("Service binding name (e.g., ZTRAVEL_SB)"),
		),
		mcp.WithString("service_version",
			mcp.Description("Service version (default: 0001)"),
		),
	), s.handlePublishServiceBinding)
	}


	// UnpublishServiceBinding
	if shouldRegister("UnpublishServiceBinding") {
		s.mcpServer.AddTool(mcp.NewTool("UnpublishServiceBinding",
		mcp.WithDescription("Unpublish a service binding"),
		mcp.WithString("service_name",
			mcp.Required(),
			mcp.Description("Service binding name (e.g., ZTRAVEL_SB)"),
		),
		mcp.WithString("service_version",
			mcp.Description("Service version (default: 0001)"),
		),
	), s.handleUnpublishServiceBinding)
	}


	// --- Workflow Tools ---

	// WriteProgram
	if shouldRegister("WriteProgram") {
		s.mcpServer.AddTool(mcp.NewTool("WriteProgram",
		mcp.WithDescription("Update an existing program with syntax check and activation (Lock -> SyntaxCheck -> Update -> Unlock -> Activate)"),
		mcp.WithString("program_name",
			mcp.Required(),
			mcp.Description("Name of the ABAP program"),
		),
		mcp.WithString("source",
			mcp.Required(),
			mcp.Description("ABAP source code"),
		),
		mcp.WithString("transport",
			mcp.Description("Transport request number (optional for local packages)"),
		),
	), s.handleWriteProgram)
	}


	// WriteClass
	if shouldRegister("WriteClass") {
		s.mcpServer.AddTool(mcp.NewTool("WriteClass",
		mcp.WithDescription("Update an existing class with syntax check and activation (Lock -> SyntaxCheck -> Update -> Unlock -> Activate)"),
		mcp.WithString("class_name",
			mcp.Required(),
			mcp.Description("Name of the ABAP class"),
		),
		mcp.WithString("source",
			mcp.Required(),
			mcp.Description("ABAP class source code (definition and implementation)"),
		),
		mcp.WithString("transport",
			mcp.Description("Transport request number (optional for local packages)"),
		),
	), s.handleWriteClass)
	}


	// CreateAndActivateProgram
	if shouldRegister("CreateAndActivateProgram") {
		s.mcpServer.AddTool(mcp.NewTool("CreateAndActivateProgram",
		mcp.WithDescription("Create a new program with source code and activate it (Create -> Lock -> Update -> Unlock -> Activate)"),
		mcp.WithString("program_name",
			mcp.Required(),
			mcp.Description("Name of the ABAP program"),
		),
		mcp.WithString("description",
			mcp.Required(),
			mcp.Description("Program description"),
		),
		mcp.WithString("package_name",
			mcp.Required(),
			mcp.Description("Package name (e.g., $TMP for local)"),
		),
		mcp.WithString("source",
			mcp.Required(),
			mcp.Description("ABAP source code"),
		),
		mcp.WithString("transport",
			mcp.Description("Transport request number (required for non-local packages)"),
		),
	), s.handleCreateAndActivateProgram)
	}


	// CreateClassWithTests
	if shouldRegister("CreateClassWithTests") {
		s.mcpServer.AddTool(mcp.NewTool("CreateClassWithTests",
		mcp.WithDescription("Create a new class with unit tests and run them (Create -> Lock -> Update -> CreateTestInclude -> UpdateTest -> Unlock -> Activate -> RunTests)"),
		mcp.WithString("class_name",
			mcp.Required(),
			mcp.Description("Name of the ABAP class"),
		),
		mcp.WithString("description",
			mcp.Required(),
			mcp.Description("Class description"),
		),
		mcp.WithString("package_name",
			mcp.Required(),
			mcp.Description("Package name (e.g., $TMP for local)"),
		),
		mcp.WithString("class_source",
			mcp.Required(),
			mcp.Description("ABAP class source code (definition and implementation)"),
		),
		mcp.WithString("test_source",
			mcp.Required(),
			mcp.Description("ABAP unit test source code"),
		),
		mcp.WithString("transport",
			mcp.Description("Transport request number (required for non-local packages)"),
		),
	), s.handleCreateClassWithTests)
	}


	// --- File-Based Deployment Tools ---

	// DeployFromFile (Recommended)
	if shouldRegister("DeployFromFile") {
		s.mcpServer.AddTool(mcp.NewTool("DeployFromFile",
		mcp.WithDescription("✅ RECOMMENDED - Smart deploy from file: auto-detects if object exists and creates/updates accordingly. Solves token limit problem for large generated files (ML models, 3948+ lines). Example: DeployFromFile(file_path=\"/path/to/zcl_ml_iris.clas.abap\", package_name=\"$ZAML_IRIS\") deploys any size file. Workflow: Parse → Check existence → Create or Update → Lock → SyntaxCheck → Write → Unlock → Activate. Supports .clas.abap, .prog.abap, .intf.abap, .fugr.abap, .func.abap. Use this for all file-based deployments."),
		mcp.WithString("file_path",
			mcp.Required(),
			mcp.Description("Absolute path to ABAP source file"),
		),
		mcp.WithString("package_name",
			mcp.Required(),
			mcp.Description("Package name (required for new objects, e.g., $ZAML_IRIS)"),
		),
		mcp.WithString("transport",
			mcp.Description("Transport request number (optional for local packages)"),
		),
	), s.handleDeployFromFile)
	}


	// SaveToFile
	if shouldRegister("SaveToFile") {
		s.mcpServer.AddTool(mcp.NewTool("SaveToFile",
		mcp.WithDescription("Save ABAP object source to local file (SAP → File). Enables BIDIRECTIONAL SYNC WORKFLOW: (1) SaveToFile downloads object from SAP, (2) edit locally with vim/VS Code/AI assistants, (3) DeployFromFile uploads changes back to SAP. Example: SaveToFile(objType=\"CLAS/OC\", objectName=\"ZCL_ML_IRIS\", outputPath=\"./src/\") creates ./src/zcl_ml_iris.clas.abap. Then edit locally and use DeployFromFile to sync back. Recommended for iterative development. Auto-determines file extension."),
		mcp.WithString("objType",
			mcp.Required(),
			mcp.Description("Object type: CLAS/OC (class), PROG/P (program), INTF/OI (interface), FUGR/F (function group), FUGR/FF (function module)"),
		),
		mcp.WithString("objectName",
			mcp.Required(),
			mcp.Description("Object name (e.g., ZCL_ML_IRIS, ZAML_IRIS_DEMO)"),
		),
		mcp.WithString("outputPath",
			mcp.Description("Output file path or directory. If directory, filename is auto-generated with correct extension. If omitted, saves to current directory."),
		),
	), s.handleSaveToFile)
	}

	// ImportFromFile (alias for DeployFromFile - File → SAP, supports class includes)
	if shouldRegister("ImportFromFile") {
		s.registerImportFromFile()
	}

	// ExportToFile (alias for SaveToFile - SAP → File)
	if shouldRegister("ExportToFile") {
		s.registerExportToFile()
	}


	// RenameObject
	if shouldRegister("RenameObject") {
		s.mcpServer.AddTool(mcp.NewTool("RenameObject",
		mcp.WithDescription("Rename ABAP object by creating copy with new name and deleting old one. Useful for fixing naming conventions. Workflow: GetSource → Replace names → CreateNew → ActivateNew → DeleteOld"),
		mcp.WithString("objType",
			mcp.Required(),
			mcp.Description("Object type: CLAS/OC (class), PROG/P (program), INTF/OI (interface), FUGR/F (function group)"),
		),
		mcp.WithString("oldName",
			mcp.Required(),
			mcp.Description("Current object name"),
		),
		mcp.WithString("newName",
			mcp.Required(),
			mcp.Description("New object name"),
		),
		mcp.WithString("packageName",
			mcp.Required(),
			mcp.Description("Package name for new object (e.g., $ZAML_IRIS)"),
		),
		mcp.WithString("transport",
			mcp.Description("Transport request number (optional for local packages)"),
		),
	), s.handleRenameObject)
	}


	// --- Surgical Edit Tools ---

	// EditSource
	if shouldRegister("EditSource") {
		s.mcpServer.AddTool(mcp.NewTool("EditSource",
		mcp.WithDescription("Surgical string replacement on ABAP source code. Matches the Edit tool pattern for local files. Workflow: GetSource → FindReplace → SyntaxCheck → Lock → Update → Unlock → Activate. Example: EditSource(object_url=\"/sap/bc/adt/programs/programs/ZTEST\", old_string=\"METHOD foo.\\n  ENDMETHOD.\", new_string=\"METHOD foo.\\n  rv_result = 42.\\n  ENDMETHOD.\", replace_all=false, syntax_check=true). Requires unique match if replace_all=false. Use this for incremental edits between syntax checks - no need to download/upload full source!"),
		mcp.WithString("object_url",
			mcp.Required(),
			mcp.Description("ADT URL of object (e.g., /sap/bc/adt/programs/programs/ZTEST, /sap/bc/adt/oo/classes/zcl_test)"),
		),
		mcp.WithString("old_string",
			mcp.Required(),
			mcp.Description("Exact string to find and replace. Must be unique in source if replace_all=false. Include enough context (surrounding lines) to ensure uniqueness."),
		),
		mcp.WithString("new_string",
			mcp.Required(),
			mcp.Description("Replacement string. Can be multiline (use \\n). Length can differ from old_string."),
		),
		mcp.WithBoolean("replace_all",
			mcp.Description("If true, replace all occurrences. If false (default), require unique match. Default: false"),
		),
		mcp.WithBoolean("syntax_check",
			mcp.Description("If true (default), validate syntax before saving. If syntax errors found, changes are NOT saved. Default: true"),
		),
		mcp.WithBoolean("case_insensitive",
			mcp.Description("If true, ignore case when matching old_string. Useful for renaming variables regardless of case. Default: false"),
		),
		mcp.WithString("method",
			mcp.Description("For CLAS only: constrain search/replace to this method only. Prevents accidental edits in other methods. (optional)"),
		),
		mcp.WithString("transport",
			mcp.Description("Transport request number (required for objects not in $TMP package)"),
		),
	), s.handleEditSource)
	}


	// --- Grep/Search Tools ---

	// GrepObject
	if shouldRegister("GrepObject") {
		s.mcpServer.AddTool(mcp.NewTool("GrepObject",
		mcp.WithDescription("Search for regex pattern in a single ABAP object's source code. Returns matches with line numbers and optional context. Use for finding TODO comments, string literals, patterns, or code snippets before editing."),
		mcp.WithString("object_url",
			mcp.Required(),
			mcp.Description("ADT URL of object (e.g., /sap/bc/adt/programs/programs/ZTEST)"),
		),
		mcp.WithString("pattern",
			mcp.Required(),
			mcp.Description("Regular expression pattern (Go regexp syntax). Examples: 'TODO', 'lv_\\w+', 'SELECT.*FROM'"),
		),
		mcp.WithBoolean("case_insensitive",
			mcp.Description("If true, perform case-insensitive matching. Default: false"),
		),
		mcp.WithNumber("context_lines",
			mcp.Description("Number of lines to show before/after each match (like grep -C). Default: 0"),
		),
	), s.handleGrepObject)
	}


	// GrepPackage
	if shouldRegister("GrepPackage") {
		s.mcpServer.AddTool(mcp.NewTool("GrepPackage",
		mcp.WithDescription("Search for regex pattern across all source objects in an ABAP package. Returns matches grouped by object. Use for package-wide analysis, finding patterns across multiple programs/classes."),
		mcp.WithString("package_name",
			mcp.Required(),
			mcp.Description("Package name (e.g., $TMP, ZPACKAGE)"),
		),
		mcp.WithString("pattern",
			mcp.Required(),
			mcp.Description("Regular expression pattern (Go regexp syntax). Examples: 'TODO', 'lv_\\w+', 'SELECT.*FROM'"),
		),
		mcp.WithBoolean("case_insensitive",
			mcp.Description("If true, perform case-insensitive matching. Default: false"),
		),
		mcp.WithString("object_types",
			mcp.Description("Comma-separated object types to search (e.g., 'PROG/P,CLAS/OC'). Empty = search all source objects. Valid: PROG/P, CLAS/OC, INTF/OI, FUGR/F, FUGR/FF, PROG/I"),
		),
		mcp.WithNumber("max_results",
			mcp.Description("Maximum number of matching objects to return. 0 = unlimited. Default: 100"),
		),
	), s.handleGrepPackage)
	}

	// GrepObjects (unified multi-object search)
	if shouldRegister("GrepObjects") {
		s.registerGrepObjects()
	}

	// GrepPackages (unified multi-package search with recursive subpackages)
	if shouldRegister("GrepPackages") {
		s.registerGrepPackages()
	}


	// --- Code Intelligence Tools ---

	// FindDefinition
	if shouldRegister("FindDefinition") {
		s.mcpServer.AddTool(mcp.NewTool("FindDefinition",
		mcp.WithDescription("Navigate to the definition of a symbol at a given position in source code"),
		mcp.WithString("source_url",
			mcp.Required(),
			mcp.Description("ADT URL of the source file (e.g., /sap/bc/adt/programs/programs/ZTEST/source/main)"),
		),
		mcp.WithString("source",
			mcp.Required(),
			mcp.Description("Full source code of the file"),
		),
		mcp.WithNumber("line",
			mcp.Required(),
			mcp.Description("Line number (1-based)"),
		),
		mcp.WithNumber("start_column",
			mcp.Required(),
			mcp.Description("Start column of the symbol (1-based)"),
		),
		mcp.WithNumber("end_column",
			mcp.Required(),
			mcp.Description("End column of the symbol (1-based)"),
		),
		mcp.WithBoolean("implementation",
			mcp.Description("Navigate to implementation instead of definition (default: false)"),
		),
		mcp.WithString("main_program",
			mcp.Description("Main program for includes (optional)"),
		),
	), s.handleFindDefinition)
	}


	// FindReferences
	if shouldRegister("FindReferences") {
		s.mcpServer.AddTool(mcp.NewTool("FindReferences",
		mcp.WithDescription("Find all references to an ABAP object or symbol"),
		mcp.WithString("object_url",
			mcp.Required(),
			mcp.Description("ADT URL of the object (e.g., /sap/bc/adt/oo/classes/ZCL_TEST)"),
		),
		mcp.WithNumber("line",
			mcp.Description("Line number for position-based reference search (1-based, optional)"),
		),
		mcp.WithNumber("column",
			mcp.Description("Column number for position-based reference search (1-based, optional)"),
		),
	), s.handleFindReferences)
	}


	// CodeCompletion
	if shouldRegister("CodeCompletion") {
		s.mcpServer.AddTool(mcp.NewTool("CodeCompletion",
		mcp.WithDescription("Get code completion suggestions at a position in source code"),
		mcp.WithString("source_url",
			mcp.Required(),
			mcp.Description("ADT URL of the source file (e.g., /sap/bc/adt/programs/programs/ZTEST/source/main)"),
		),
		mcp.WithString("source",
			mcp.Required(),
			mcp.Description("Full source code of the file"),
		),
		mcp.WithNumber("line",
			mcp.Required(),
			mcp.Description("Line number (1-based)"),
		),
		mcp.WithNumber("column",
			mcp.Required(),
			mcp.Description("Column number (1-based)"),
		),
	), s.handleCodeCompletion)
	}


	// PrettyPrint
	if shouldRegister("PrettyPrint") {
		s.mcpServer.AddTool(mcp.NewTool("PrettyPrint",
		mcp.WithDescription("Format ABAP source code using the pretty printer"),
		mcp.WithString("source",
			mcp.Required(),
			mcp.Description("ABAP source code to format"),
		),
	), s.handlePrettyPrint)
	}


	// GetPrettyPrinterSettings
	if shouldRegister("GetPrettyPrinterSettings") {
		s.mcpServer.AddTool(mcp.NewTool("GetPrettyPrinterSettings",
		mcp.WithDescription("Get the current pretty printer (code formatter) settings"),
	), s.handleGetPrettyPrinterSettings)
	}


	// SetPrettyPrinterSettings
	if shouldRegister("SetPrettyPrinterSettings") {
		s.mcpServer.AddTool(mcp.NewTool("SetPrettyPrinterSettings",
		mcp.WithDescription("Update the pretty printer (code formatter) settings"),
		mcp.WithBoolean("indentation",
			mcp.Required(),
			mcp.Description("Enable automatic indentation"),
		),
		mcp.WithString("style",
			mcp.Required(),
			mcp.Description("Keyword style: toLower, toUpper, keywordUpper, keywordLower, keywordAuto, none"),
		),
	), s.handleSetPrettyPrinterSettings)
	}


	// GetTypeHierarchy
	if shouldRegister("GetTypeHierarchy") {
		s.mcpServer.AddTool(mcp.NewTool("GetTypeHierarchy",
		mcp.WithDescription("Get the type hierarchy (supertypes or subtypes) for a class/interface"),
		mcp.WithString("source_url",
			mcp.Required(),
			mcp.Description("ADT URL of the source file"),
		),
		mcp.WithString("source",
			mcp.Required(),
			mcp.Description("Full source code of the file"),
		),
		mcp.WithNumber("line",
			mcp.Required(),
			mcp.Description("Line number (1-based)"),
		),
		mcp.WithNumber("column",
			mcp.Required(),
			mcp.Description("Column number (1-based)"),
		),
		mcp.WithBoolean("super_types",
			mcp.Description("Get supertypes instead of subtypes (default: false = subtypes)"),
		),
	), s.handleGetTypeHierarchy)
	}

	// GetClassComponents - get class structure (methods, attributes, events)
	if shouldRegister("GetClassComponents") {
		s.mcpServer.AddTool(mcp.NewTool("GetClassComponents",
			mcp.WithDescription("Get the structure of a class - lists all methods, attributes, events, and other components with their visibility and properties"),
			mcp.WithString("class_url",
				mcp.Required(),
				mcp.Description("ADT URL of the class (e.g., /sap/bc/adt/oo/classes/ZCL_TEST)"),
			),
		), s.handleGetClassComponents)
	}

	// GetInactiveObjects - list objects that need activation
	if shouldRegister("GetInactiveObjects") {
		s.mcpServer.AddTool(mcp.NewTool("GetInactiveObjects",
			mcp.WithDescription("Get all inactive objects for the current user - objects that have been modified but not yet activated"),
		), s.handleGetInactiveObjects)
	}

	// Transport Management Tools (require EnableTransports flag)
	// GetUserTransports - list transport requests for a user
	if shouldRegister("GetUserTransports") {
		s.mcpServer.AddTool(mcp.NewTool("GetUserTransports",
			mcp.WithDescription("Get all transport requests for a user (requires --enable-transports flag). Returns both workbench and customizing requests grouped by target system."),
			mcp.WithString("user_name",
				mcp.Required(),
				mcp.Description("SAP user name (will be converted to uppercase)"),
			),
		), s.handleGetUserTransports)
	}

	// GetTransportInfo - get transport info for an object
	if shouldRegister("GetTransportInfo") {
		s.mcpServer.AddTool(mcp.NewTool("GetTransportInfo",
			mcp.WithDescription("Get transport information for an ABAP object (requires --enable-transports flag). Returns available transports and lock status."),
			mcp.WithString("object_url",
				mcp.Required(),
				mcp.Description("ADT URL of the object (e.g., /sap/bc/adt/programs/programs/ZTEST)"),
			),
			mcp.WithString("dev_class",
				mcp.Required(),
				mcp.Description("Development class/package of the object"),
			),
		), s.handleGetTransportInfo)
	}

	// ExecuteABAP - execute arbitrary ABAP code via unit test wrapper (Expert mode only)
	if shouldRegister("ExecuteABAP") {
		s.mcpServer.AddTool(mcp.NewTool("ExecuteABAP",
			mcp.WithDescription("Execute arbitrary ABAP code via unit test wrapper. Creates temp program, injects code into test method, runs via RunUnitTests, extracts results from assertion messages, cleans up. Use lv_result variable to return output. WARNING: Powerful tool - use responsibly."),
			mcp.WithString("code",
				mcp.Required(),
				mcp.Description("ABAP code to execute. Set lv_result variable to return output via assertion message."),
			),
			mcp.WithString("risk_level",
				mcp.Description("Risk level: harmless (default, no DB writes), dangerous (can write to DB), critical (full access)"),
			),
			mcp.WithString("return_variable",
				mcp.Description("Name of the variable to return (default: lv_result)"),
			),
			mcp.WithBoolean("keep_program",
				mcp.Description("Don't delete temp program after execution (for debugging)"),
			),
			mcp.WithString("program_prefix",
				mcp.Description("Prefix for temp program name (default: ZTEMP_EXEC_)"),
			),
		), s.handleExecuteABAP)
	}

	// --- UI5/Fiori BSP Management ---

	// UI5ListApps
	if shouldRegister("UI5ListApps") {
		s.mcpServer.AddTool(mcp.NewTool("UI5ListApps",
			mcp.WithDescription("List UI5/Fiori BSP applications. Use query parameter for filtering with wildcards (*)."),
			mcp.WithString("query",
				mcp.Description("Search query (supports * wildcard, e.g., 'Z*' for custom apps)"),
			),
			mcp.WithNumber("max_results",
				mcp.Description("Maximum number of results (default: 100)"),
			),
		), s.handleUI5ListApps)
	}

	// UI5GetApp
	if shouldRegister("UI5GetApp") {
		s.mcpServer.AddTool(mcp.NewTool("UI5GetApp",
			mcp.WithDescription("Get details of a UI5/Fiori BSP application including file structure."),
			mcp.WithString("app_name",
				mcp.Required(),
				mcp.Description("Name of the UI5 application"),
			),
		), s.handleUI5GetApp)
	}

	// UI5GetFileContent
	if shouldRegister("UI5GetFileContent") {
		s.mcpServer.AddTool(mcp.NewTool("UI5GetFileContent",
			mcp.WithDescription("Get content of a specific file within a UI5/Fiori BSP application."),
			mcp.WithString("app_name",
				mcp.Required(),
				mcp.Description("Name of the UI5 application"),
			),
			mcp.WithString("file_path",
				mcp.Required(),
				mcp.Description("Path to the file within the app (e.g., '/webapp/manifest.json')"),
			),
		), s.handleUI5GetFileContent)
	}

	// UI5UploadFile
	if shouldRegister("UI5UploadFile") {
		s.mcpServer.AddTool(mcp.NewTool("UI5UploadFile",
			mcp.WithDescription("Upload a file to a UI5/Fiori BSP application."),
			mcp.WithString("app_name",
				mcp.Required(),
				mcp.Description("Name of the UI5 application"),
			),
			mcp.WithString("file_path",
				mcp.Required(),
				mcp.Description("Path for the file within the app (e.g., '/webapp/Component.js')"),
			),
			mcp.WithString("content",
				mcp.Required(),
				mcp.Description("File content to upload"),
			),
			mcp.WithString("content_type",
				mcp.Description("Content type (e.g., 'application/javascript', 'application/json')"),
			),
		), s.handleUI5UploadFile)
	}

	// UI5DeleteFile
	if shouldRegister("UI5DeleteFile") {
		s.mcpServer.AddTool(mcp.NewTool("UI5DeleteFile",
			mcp.WithDescription("Delete a file from a UI5/Fiori BSP application."),
			mcp.WithString("app_name",
				mcp.Required(),
				mcp.Description("Name of the UI5 application"),
			),
			mcp.WithString("file_path",
				mcp.Required(),
				mcp.Description("Path to the file to delete (e.g., '/webapp/test.js')"),
			),
		), s.handleUI5DeleteFile)
	}

	// UI5CreateApp
	if shouldRegister("UI5CreateApp") {
		s.mcpServer.AddTool(mcp.NewTool("UI5CreateApp",
			mcp.WithDescription("Create a new UI5/Fiori BSP application."),
			mcp.WithString("app_name",
				mcp.Required(),
				mcp.Description("Name for the new UI5 application"),
			),
			mcp.WithString("description",
				mcp.Description("Description of the application"),
			),
			mcp.WithString("package",
				mcp.Required(),
				mcp.Description("Package name (e.g., '$TMP' for local, 'ZFIORI' for transportable)"),
			),
			mcp.WithString("transport",
				mcp.Description("Transport request number (optional for local packages)"),
			),
		), s.handleUI5CreateApp)
	}

	// UI5DeleteApp
	if shouldRegister("UI5DeleteApp") {
		s.mcpServer.AddTool(mcp.NewTool("UI5DeleteApp",
			mcp.WithDescription("Delete a UI5/Fiori BSP application."),
			mcp.WithString("app_name",
				mcp.Required(),
				mcp.Description("Name of the UI5 application to delete"),
			),
			mcp.WithString("transport",
				mcp.Description("Transport request number (optional for local packages)"),
			),
		), s.handleUI5DeleteApp)
	}

	// --- AMDP (HANA) Debugger ---

	// AMDPDebuggerStart
	if shouldRegister("AMDPDebuggerStart") {
		s.mcpServer.AddTool(mcp.NewTool("AMDPDebuggerStart",
			mcp.WithDescription("Start an AMDP (HANA SQLScript) debug session with persistent goroutine. Creates a background goroutine that maintains the HTTP session cookies. Use AMDPDebuggerStep/AMDPGetVariables to interact, AMDPDebuggerStop to terminate."),
			mcp.WithString("user",
				mcp.Description("User to debug (defaults to current user)"),
			),
		), s.handleAMDPDebuggerStart)
	}

	// AMDPDebuggerResume
	if shouldRegister("AMDPDebuggerResume") {
		s.mcpServer.AddTool(mcp.NewTool("AMDPDebuggerResume",
			mcp.WithDescription("Get current AMDP debug session status. In goroutine model, this returns the current state without blocking. The session manager goroutine handles events internally."),
		), s.handleAMDPDebuggerResume)
	}

	// AMDPDebuggerStop
	if shouldRegister("AMDPDebuggerStop") {
		s.mcpServer.AddTool(mcp.NewTool("AMDPDebuggerStop",
			mcp.WithDescription("Stop the AMDP debug session and terminate the background goroutine. Cleans up the HTTP session on SAP server."),
		), s.handleAMDPDebuggerStop)
	}

	// AMDPDebuggerStep
	if shouldRegister("AMDPDebuggerStep") {
		s.mcpServer.AddTool(mcp.NewTool("AMDPDebuggerStep",
			mcp.WithDescription("Perform a step operation in the AMDP debugger. Communicates via channel to the session manager goroutine."),
			mcp.WithString("step_type",
				mcp.Required(),
				mcp.Description("Step type: 'stepInto', 'stepOver', 'stepReturn', 'stepContinue'"),
			),
		), s.handleAMDPDebuggerStep)
	}

	// AMDPGetVariables
	if shouldRegister("AMDPGetVariables") {
		s.mcpServer.AddTool(mcp.NewTool("AMDPGetVariables",
			mcp.WithDescription("Get variable values during AMDP debugging. Communicates via channel to the session manager goroutine. Returns scalar, table, and array types."),
		), s.handleAMDPGetVariables)
	}

	// AMDPSetBreakpoint
	if shouldRegister("AMDPSetBreakpoint") {
		s.mcpServer.AddTool(mcp.NewTool("AMDPSetBreakpoint",
			mcp.WithDescription("Set a breakpoint in AMDP (SQLScript) code. Requires an active AMDP debug session. Specify the procedure name and line number."),
			mcp.WithString("proc_name",
				mcp.Required(),
				mcp.Description("AMDP procedure name (e.g., 'ZCL_TEST=>METHOD_NAME')"),
			),
			mcp.WithNumber("line",
				mcp.Required(),
				mcp.Description("Line number in the SQLScript code"),
			),
		), s.handleAMDPSetBreakpoint)
	}

	// AMDPGetBreakpoints
	if shouldRegister("AMDPGetBreakpoints") {
		s.mcpServer.AddTool(mcp.NewTool("AMDPGetBreakpoints",
			mcp.WithDescription("Get all breakpoints registered in the current AMDP debug session. Useful for verifying breakpoints are set correctly."),
		), s.handleAMDPGetBreakpoints)
	}

	// CTS/Transport Management Tools

	// ListTransports
	if shouldRegister("ListTransports") {
		s.mcpServer.AddTool(mcp.NewTool("ListTransports",
			mcp.WithDescription("List transport requests. Returns modifiable transports for a user. Requires --enable-transports flag."),
			mcp.WithString("user",
				mcp.Description("Username to list transports for (default: current user, '*' for all users)"),
			),
		), s.handleListTransports)
	}

	// GetTransport
	if shouldRegister("GetTransport") {
		s.mcpServer.AddTool(mcp.NewTool("GetTransport",
			mcp.WithDescription("Get detailed transport information including objects and tasks. Requires --enable-transports flag."),
			mcp.WithString("transport",
				mcp.Required(),
				mcp.Description("Transport request number (e.g., 'A4HK900094')"),
			),
		), s.handleGetTransport)
	}

	// CreateTransport (expert mode only)
	if shouldRegister("CreateTransport") {
		s.mcpServer.AddTool(mcp.NewTool("CreateTransport",
			mcp.WithDescription("Create a new transport request. Requires --enable-transports flag and not --transport-read-only."),
			mcp.WithString("description",
				mcp.Required(),
				mcp.Description("Transport description"),
			),
			mcp.WithString("package",
				mcp.Required(),
				mcp.Description("Target package (DEVCLASS)"),
			),
			mcp.WithString("transport_layer",
				mcp.Description("Transport layer (optional)"),
			),
			mcp.WithString("type",
				mcp.Description("Type: 'workbench' (default) or 'customizing'"),
			),
		), s.handleCreateTransport)
	}

	// ReleaseTransport (expert mode only)
	if shouldRegister("ReleaseTransport") {
		s.mcpServer.AddTool(mcp.NewTool("ReleaseTransport",
			mcp.WithDescription("Release a transport request. This action is IRREVERSIBLE. Requires --enable-transports flag and not --transport-read-only."),
			mcp.WithString("transport",
				mcp.Required(),
				mcp.Description("Transport request number"),
			),
			mcp.WithBoolean("ignore_locks",
				mcp.Description("Release even with locked objects (default: false)"),
			),
			mcp.WithBoolean("skip_atc",
				mcp.Description("Skip ATC quality checks (default: false)"),
			),
		), s.handleReleaseTransport)
	}

	// DeleteTransport (expert mode only)
	if shouldRegister("DeleteTransport") {
		s.mcpServer.AddTool(mcp.NewTool("DeleteTransport",
			mcp.WithDescription("Delete a transport request. Only modifiable transports can be deleted. Requires --enable-transports flag and not --transport-read-only."),
			mcp.WithString("transport",
				mcp.Required(),
				mcp.Description("Transport request number"),
			),
		), s.handleDeleteTransport)
	}

	// --- Git/abapGit Integration (via ZADT_VSP WebSocket) ---

	// GitTypes
	if shouldRegister("GitTypes") {
		s.mcpServer.AddTool(mcp.NewTool("GitTypes",
			mcp.WithDescription("Get list of supported abapGit object types. Returns 158 object types that can be exported/imported via abapGit. Requires abapGit to be installed on SAP system."),
		), s.handleGitTypes)
	}

	// GitExport
	if shouldRegister("GitExport") {
		s.mcpServer.AddTool(mcp.NewTool("GitExport",
			mcp.WithDescription("Export ABAP objects as abapGit-compatible ZIP. Supports 158 object types. Returns base64-encoded ZIP with files in abapGit format. Use packages OR objects parameter."),
			mcp.WithString("packages",
				mcp.Description("Comma-separated package names to export (e.g., '$ZRAY,$TMP'). Supports wildcards."),
			),
			mcp.WithString("objects",
				mcp.Description("JSON array of objects: [{\"type\":\"CLAS\",\"name\":\"ZCL_TEST\"}]"),
			),
			mcp.WithBoolean("include_subpackages",
				mcp.Description("Include subpackages when exporting by package (default: true)"),
			),
		), s.handleGitExport)
	}

	// --- Report Execution Tools (via ZADT_VSP WebSocket) ---

	// RunReport
	if shouldRegister("RunReport") {
		s.mcpServer.AddTool(mcp.NewTool("RunReport",
			mcp.WithDescription("Execute an ABAP selection-screen report with parameters or variant. Runs as background job and returns spool output. Requires ZADT_VSP WebSocket handler deployed."),
			mcp.WithString("report",
				mcp.Description("Report program name (e.g., 'RFITEMGL', 'ZREPORT_TEST')"),
				mcp.Required(),
			),
			mcp.WithString("variant",
				mcp.Description("Variant name to use for selection screen (optional)"),
			),
			mcp.WithString("params",
				mcp.Description("JSON object with selection screen parameters (e.g., '{\"P_BUKRS\":\"1000\",\"S_KUNNR\":{\"SIGN\":\"I\",\"OPTION\":\"EQ\",\"LOW\":\"0000001000\"}}'). Keys are parameter names."),
			),
		), s.handleRunReport)
	}

	// RunReportAsync - Background report execution
	if shouldRegister("RunReportAsync") {
		s.mcpServer.AddTool(mcp.NewTool("RunReportAsync",
			mcp.WithDescription("Start report execution in background. Returns task_id immediately. Use GetAsyncResult to poll for completion. Useful for long-running reports that would timeout."),
			mcp.WithString("report",
				mcp.Description("Report program name"),
				mcp.Required(),
			),
			mcp.WithString("variant",
				mcp.Description("Variant name (optional)"),
			),
			mcp.WithString("params",
				mcp.Description("JSON object with selection screen parameters"),
			),
		), s.handleRunReportAsync)
	}

	// GetAsyncResult - Retrieve async task results
	if shouldRegister("GetAsyncResult") {
		s.mcpServer.AddTool(mcp.NewTool("GetAsyncResult",
			mcp.WithDescription("Get result of an async task by ID. Returns status (running/completed/error) and result when done."),
			mcp.WithString("task_id",
				mcp.Description("Task ID from RunReportAsync"),
				mcp.Required(),
			),
			mcp.WithBoolean("wait",
				mcp.Description("If true, block until task completes (max 60s). Default: false (poll)"),
			),
		), s.handleGetAsyncResult)
	}

	// GetVariants
	if shouldRegister("GetVariants") {
		s.mcpServer.AddTool(mcp.NewTool("GetVariants",
			mcp.WithDescription("Get list of available variants for a report program. Returns variant names and whether they are protected."),
			mcp.WithString("report",
				mcp.Description("Report program name"),
				mcp.Required(),
			),
		), s.handleGetVariants)
	}

	// GetTextElements
	if shouldRegister("GetTextElements") {
		s.mcpServer.AddTool(mcp.NewTool("GetTextElements",
			mcp.WithDescription("Get program text elements (selection texts and text symbols). Selection texts describe parameters (P_BUKRS='Company Code'), text symbols are TEXT-001 etc."),
			mcp.WithString("program",
				mcp.Description("Program name"),
				mcp.Required(),
			),
			mcp.WithString("language",
				mcp.Description("Language key (e.g., 'E' for English, 'D' for German). Default: system language."),
			),
		), s.handleGetTextElements)
	}

	// SetTextElements
	if shouldRegister("SetTextElements") {
		s.mcpServer.AddTool(mcp.NewTool("SetTextElements",
			mcp.WithDescription("Set program text elements (selection texts, text symbols, and heading texts). Use for adding descriptions to selection screen parameters, text symbols, and list/column headings."),
			mcp.WithString("program",
				mcp.Description("Program name"),
				mcp.Required(),
			),
			mcp.WithString("language",
				mcp.Description("Language key (e.g., 'E' for English, 'D' for German). Default: system language."),
			),
			mcp.WithString("selection_texts",
				mcp.Description("JSON object of selection texts (e.g., '{\"P_BUKRS\":\"Company Code\",\"S_KUNNR\":\"Customer Range\"}')"),
			),
			mcp.WithString("text_symbols",
				mcp.Description("JSON object of text symbols (e.g., '{\"001\":\"Header Text\",\"002\":\"Footer\"}')"),
			),
			mcp.WithString("heading_texts",
				mcp.Description("JSON object of heading texts for list/column headings (e.g., '{\"001\":\"Report Title\",\"002\":\"Column Header\"}')"),
			),
		), s.handleSetTextElements)
	}

	// --- Install/Setup Tools ---

	// InstallZADTVSP
	if shouldRegister("InstallZADTVSP") {
		s.mcpServer.AddTool(mcp.NewTool("InstallZADTVSP",
			mcp.WithDescription("Deploy ZADT_VSP WebSocket handler to SAP system. Creates package and deploys 6 ABAP objects (interface + 5 classes) that enable WebSocket debugging, RFC calls, and abapGit export. After deployment, manual SAPC and SICF setup is required."),
			mcp.WithString("package",
				mcp.Description("Target package name (default: $ZADT_VSP). Must be local package starting with $."),
			),
			mcp.WithBoolean("skip_git_service",
				mcp.Description("Skip ZCL_VSP_GIT_SERVICE deployment if abapGit is not installed (default: false, auto-detected)"),
			),
			mcp.WithBoolean("check_only",
				mcp.Description("Only check prerequisites without deploying (default: false)"),
			),
		), s.handleInstallZADTVSP)
	}

	// ListDependencies
	if shouldRegister("ListDependencies") {
		s.mcpServer.AddTool(mcp.NewTool("ListDependencies",
			mcp.WithDescription("List available dependency packages that can be installed via InstallAbapGit. Shows abapGit editions and other optional dependencies."),
		), s.handleListDependencies)
	}

	// InstallAbapGit
	if shouldRegister("InstallAbapGit") {
		s.mcpServer.AddTool(mcp.NewTool("InstallAbapGit",
			mcp.WithDescription("Deploy abapGit to SAP system from embedded ZIP. Supports standalone (single program) or developer edition (full package structure). Parses abapGit-format ZIP and deploys via WriteSource."),
			mcp.WithString("edition",
				mcp.Description("Edition to install: 'standalone' (single program ZABAPGIT) or 'dev' (full $ZGIT_DEV packages). Default: standalone"),
			),
			mcp.WithString("package",
				mcp.Description("Target package name. Default: $ABAPGIT for standalone, $ZGIT_DEV for dev edition"),
			),
			mcp.WithBoolean("check_only",
				mcp.Description("Only check prerequisites and show deployment plan without deploying (default: false)"),
			),
		), s.handleInstallAbapGit)
	}

	// InstallDummyTest - Test tool to verify Install* workflow
	if shouldRegister("InstallDummyTest") {
		s.mcpServer.AddTool(mcp.NewTool("InstallDummyTest",
			mcp.WithDescription("Test tool that creates a simple interface and class to verify the Install* workflow (create, lock, update, unlock, activate, verify). Uses package $ZADT_INSTALL_TEST."),
			mcp.WithBoolean("check_only",
				mcp.Description("Only check prerequisites without deploying (default: false)"),
			),
			mcp.WithBoolean("cleanup",
				mcp.Description("Delete test objects after verification (default: false)"),
			),
		), s.handleInstallDummyTest)
	}

	// Register tool aliases for common operations
	// These provide short names for frequently used tools
	s.registerToolAliases(shouldRegister)
}

// registerToolAliases registers short alias names for frequently used tools.
// Aliases provide quick access: gs→GetSource, ws→WriteSource, etc.
func (s *Server) registerToolAliases(shouldRegister func(string) bool) {
	// Define aliases: alias -> canonical tool name
	// Only register alias if the canonical tool is registered
	type aliasInfo struct {
		canonical string
		desc      string
		handler   func(context.Context, mcp.CallToolRequest) (*mcp.CallToolResult, error)
	}

	aliases := map[string]aliasInfo{
		// Core read/write
		"gs": {"GetSource", "Alias for GetSource - read ABAP source code", s.handleGetSource},
		"ws": {"WriteSource", "Alias for WriteSource - write ABAP source code", s.handleWriteSource},
		"es": {"EditSource", "Alias for EditSource - surgical string replacement", s.handleEditSource},

		// Search
		"so": {"SearchObject", "Alias for SearchObject - find ABAP objects", s.handleSearchObject},
		"gro": {"GrepObjects", "Alias for GrepObjects - regex search in objects", s.handleGrepObjects},
		"grp": {"GrepPackages", "Alias for GrepPackages - regex search in packages", s.handleGrepPackages},

		// Common operations
		"gt": {"GetTable", "Alias for GetTable - get table structure", s.handleGetTable},
		"gtc": {"GetTableContents", "Alias for GetTableContents - read table data", s.handleGetTableContents},
		"rq": {"RunQuery", "Alias for RunQuery - execute SQL query", s.handleRunQuery},
		"sc": {"SyntaxCheck", "Alias for SyntaxCheck - check ABAP syntax", s.handleSyntaxCheck},
		"act": {"Activate", "Alias for Activate - activate ABAP object", s.handleActivate},

		// Testing
		"rut": {"RunUnitTests", "Alias for RunUnitTests - run ABAP unit tests", s.handleRunUnitTests},
		"atc": {"RunATCCheck", "Alias for RunATCCheck - run ATC code check", s.handleRunATCCheck},
	}

	for alias, info := range aliases {
		if shouldRegister(info.canonical) {
			s.mcpServer.AddTool(mcp.NewTool(alias,
				mcp.WithDescription(info.desc),
				// Aliases inherit all parameters from the canonical tool
				// The handler is the same, so parameters work identically
			), info.handler)
		}
	}
}

// newToolResultError creates an error result for tool execution failures.
func newToolResultError(message string) *mcp.CallToolResult {
	result := mcp.NewToolResultText(message)
	result.IsError = true
	return result
}

// ensureWSConnected ensures the WebSocket client is connected, creating it if needed.
// Returns error result if connection fails, nil on success.
func (s *Server) ensureWSConnected(ctx context.Context, toolName string) *mcp.CallToolResult {
	if s.amdpWSClient == nil || !s.amdpWSClient.IsConnected() {
		s.amdpWSClient = adt.NewAMDPWebSocketClient(
			s.config.BaseURL, s.config.Client, s.config.Username, s.config.Password, s.config.InsecureSkipVerify,
		)
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
// - handlers_diagnostics.go: ListDumps, ListTraces, etc.
// - handlers_devtools.go: SyntaxCheck, Activate, ATC, etc.
// - handlers_crud.go: Lock, Create, Update, Delete, etc.
// - handlers_debug.go: SetBreakpoint, DebuggerListen, etc.
// - handlers_amdp.go: AMDPDebugger* handlers
// - handlers_ui5.go: UI5ListApps, UI5GetApp, etc.
// - handlers_git.go: GitTypes, GitExport
// - handlers_report.go: RunReport, GetVariants, etc.
// - handlers_install.go: InstallZADTVSP, InstallAbapGit, etc.
// - handlers_transport.go: ListTransports, GetTransport, etc.
