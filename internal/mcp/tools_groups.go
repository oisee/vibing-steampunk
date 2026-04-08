// Package mcp provides the MCP server implementation for ABAP ADT tools.
// tools_groups.go defines tool group definitions for selective disablement.
package mcp

// toolGroups defines groups of tools that can be selectively disabled.
// Short codes: 5/U=UI5, T=Tests, H=HANA, D=Debug, C=CTS, G=Git, R=Reports, I=Install, N=i18N, X=Experimental
func toolGroups() map[string][]string {
	groups := map[string][]string{
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
		"GC": { // gCTS tools (git-enabled Change Transport System)
			"GctsListRepositories", "GctsGetRepository", "GctsCreateRepository",
			"GctsDeleteRepository", "GctsCloneRepository", "GctsPull",
			"GctsCommit", "GctsListBranches", "GctsSwitchBranch", "GctsGetHistory",
		},
		"R": { // Report execution tools (via ZADT_VSP WebSocket)
			"RunReport", "GetVariants", "GetTextElements", "SetTextElements",
		},
		"I": { // Install/Setup tools
			"InstallZADTVSP",
			"InstallAbapGit",
			"ListDependencies",
			"InstallDummyTest",
			"DeployZip",
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
		"N": { // i18n/Translation tools
			"GetObjectTextsInLanguage", "GetDataElementLabels", "GetMessageClassTexts",
			"WriteMessageClassTexts", "WriteDataElementLabels",
			"GetTextPool", "CompareLanguages",
		},
	}
	// Map "U" to same tools as "5"
	groups["U"] = groups["5"]
	return groups
}
