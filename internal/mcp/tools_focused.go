// Package mcp provides the MCP server implementation for ABAP ADT tools.
// tools_focused.go defines the focused mode tool whitelist.
package mcp

// focusedToolSet returns the set of tools enabled in focused mode (81+ essential tools).
func focusedToolSet() map[string]bool {
	return map[string]bool{
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
		"GetTable":           true,
		"GetTableContents":   true,
		"RunQuery":           true,
		"GetPackage":         true, // Metadata: package contents
		"GetFunctionGroup":   true, // Metadata: function module list
		"GetCDSDependencies": true, // CDS dependency tree
		"GetMessages":        true, // Message class texts (SE91)

		// Code intelligence (3)
		"FindDefinition": true,
		"FindReferences": true,
		"GetContext":     true, // Dependency context compression

		// Development tools (11)
		"SyntaxCheck":        true,
		"RunUnitTests":       true,
		"RunATCCheck":        true,  // Code quality checks
		"Activate":           true,  // Re-activate objects without editing
		"ActivatePackage":    true,  // Batch activation of all inactive objects
		"PrettyPrint":        true,  // Format ABAP code
		"GetInactiveObjects": true,  // List pending activations
		"CreatePackage":      true,  // Create local packages ($...)
		"CreateTable":        true,  // Create DDIC tables from JSON
		"CompareSource":      true,  // Diff two objects
		"CloneObject":        true,  // Copy object to new name
		"GetClassInfo":       true,  // Quick class metadata

		// Advanced/Edge cases (2)
		"LockObject":   true,
		"UnlockObject": true,

		// File-based operations (2)
		"ImportFromFile": true, // File → SAP (replaces DeployFromFile)
		"ExportToFile":   true, // SAP → File (replaces SaveToFile)

		// System information (2)
		"GetSystemInfo":          true, // System ID, release, kernel
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

		// AMDP (HANA) Debugger - expert mode only
		"AMDPDebuggerStart":  true,
		"AMDPDebuggerResume": true,
		"AMDPDebuggerStop":   true,
		"AMDPDebuggerStep":   true,
		"AMDPGetVariables":   true,
		"AMDPSetBreakpoint":  true,
		"AMDPGetBreakpoints": true,

		// Intelligence Layer (4)
		"AnalyzeSQLPerformance": true, // SQL performance analysis (text + HANA plan)
		"GetImpactAnalysis":     true, // Multi-layer blast radius analysis
		"AnalyzeABAPCode":       true, // 21-rule ABAP source analysis
		"CheckRegression":       true, // Diff-based breaking change detection

		// CTS/Transport Management (2 read-only in focused mode)
		"ListTransports": true, // List transport requests
		"GetTransport":   true, // Get transport details with objects

		// Git/abapGit Integration (via ZADT_VSP WebSocket)
		"GitTypes":  true, // List 158 supported object types
		"GitExport": true, // Export packages/objects to abapGit ZIP

		// Report Execution (via ZADT_VSP WebSocket)
		"RunReport":      true, // Execute reports with params/variants, capture ALV
		"RunReportAsync": true, // Background report execution with polling
		"GetAsyncResult": true, // Retrieve async task results
		"GetVariants":    true, // List report variants
		"GetTextElements": true, // Get program text elements
		"SetTextElements": true, // Set program text elements

		// Install/Setup tools
		"InstallZADTVSP":   true, // Deploy ZADT_VSP WebSocket handler to SAP
		"InstallAbapGit":   true, // Deploy abapGit (standalone or dev edition) to SAP
		"ListDependencies": true, // List available dependencies for installation
		"InstallDummyTest": true, // Test tool for verifying Install* workflow
		"DeployZip":        true, // Deploy objects from abapGit-format ZIP to SAP package
	}
}
