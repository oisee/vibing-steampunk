// Package mcp provides the MCP server implementation for ABAP ADT tools.
// tools_register.go holds the mode logic (shouldRegister) and the top-level
// registration order. The register*Tools functions it calls live in one file
// per domain: tools_read.go, tools_crud.go, tools_debug.go, tools_transport.go, ...
package mcp

import (
	"strings"
	"unicode"
)

// registerTools registers ADT tools with the MCP server based on mode, disabled groups, and granular config.
// Mode "focused" registers essential tools.
// Mode "expert" registers all tools.
// DisabledGroups can disable specific tool groups using short codes:
//   - "GC" = gCTS; use "G,C" to disable Git and CTS together
//   - "5" or "U" = UI5/BSP tools (3 tools, read-only)
//   - "T" = Test tools: RunUnitTests, RunATCCheck (2 tools)
//   - "H" = HANA/AMDP debugger (7 tools)
//   - "D" = ABAP Debugger (6 session tools)
//   - "C" = CTS/Transport tools (5 tools)
//   - "G" = Git/abapGit tools (2 tools)
//   - "R" = Report tools (4 tools)
//   - "I" = Install tools (4 tools)
//   - "X" = EXPERIMENTAL: All debugger + RunReport (17 tools) - use to disable unreliable features
//
// toolsConfig from .vsp.json has highest priority:
//   - If tool is explicitly disabled (false), it will NOT be registered
//   - If tool is explicitly enabled (true), it WILL be registered (overrides focused mode)
//   - If tool is not in config, mode/disabledGroups rules apply
func (s *Server) registerTools(mode string, disabledGroups string, toolsConfig map[string]bool) {
	// Hyperfocused mode: the universal tool, and nothing else.
	if mode == "hyperfocused" {
		s.registerUniversalTool()
		return
	}

	focusedTools := focusedToolSet()

	disabledTools := disabledToolSet(disabledGroups)

	// Helper to check if tool should be registered
	shouldRegister := func(toolName string) bool {
		// Priority 1: Check granular tool config from .vsp.json (highest priority)
		if toolsConfig != nil {
			if enabled, exists := toolsConfig[toolName]; exists {
				return enabled // Explicit config overrides everything
			}
		}
		// Priority 2: Check if tool is disabled by group
		if disabledTools[toolName] {
			return false
		}
		// Priority 3: Check mode
		if mode == "expert" {
			return true // Expert mode: register all tools (except disabled)
		}
		return focusedTools[toolName] // Focused mode: only whitelisted tools (except disabled)
	}

	// The universal tool is registered in every mode, not only hyperfocused.
	//
	// It used to be hyperfocused-only, which meant an agent in focused or
	// expert could not reach a single one of the thirty-eight `analyze` types:
	// eight analysis handlers, every post-mortem type and every AMDP target are
	// routed through SAP() and registered as tools nowhere. Two of the three
	// modes advertised a capability surface that was missing a third of itself,
	// and nothing said so — the same disease as a tool whitelisted behind a
	// registration function nobody calls.
	//
	// It goes through shouldRegister like everything else, so a deployment that
	// wants it gone can still turn it off by name.
	if shouldRegister("SAP") {
		s.registerUniversalTool()
	}

	// Register all tools
	s.registerUnifiedTools(shouldRegister)
	s.registerReadTools(shouldRegister)
	s.registerSystemTools(shouldRegister)
	s.registerAnalysisTools(shouldRegister)
	s.registerDiagnosticsTools(shouldRegister)
	s.registerDebuggerTools(shouldRegister)
	s.registerSearchTools(shouldRegister)
	s.registerDevTools(shouldRegister)
	s.registerCRUDTools(shouldRegister)
	s.registerClassIncludeTools(shouldRegister)
	s.registerWorkflowTools(shouldRegister)
	s.registerFileTools(shouldRegister)
	s.registerEditTools(shouldRegister)
	s.registerGrepTools(shouldRegister)
	s.registerCodeIntelTools(shouldRegister)
	s.registerUI5Tools(shouldRegister)
	s.registerAMDPTools(shouldRegister)
	s.registerTransportTools(shouldRegister)
	s.registerGitTools(shouldRegister)
	s.registerReportTools(shouldRegister)
	s.registerInstallTools(shouldRegister)
	s.registerVersionHistoryTools(shouldRegister)
	s.registerTestingQualityTools(shouldRegister)
	s.registerI18NTools(shouldRegister)
	s.registerIAMTools(shouldRegister)

	// Register tool aliases for common operations
	s.registerToolAliases(shouldRegister)
}

func disabledToolSet(disabledGroups string) map[string]bool {
	groups := toolGroups()
	disabledTools := make(map[string]bool)
	for _, code := range parseDisabledGroupCodes(disabledGroups, groups) {
		for _, tool := range groups[code] {
			disabledTools[tool] = true
		}
	}
	return disabledTools
}

func parseDisabledGroupCodes(input string, groups map[string][]string) []string {
	input = strings.ToUpper(strings.TrimSpace(input))
	if input == "" {
		return nil
	}

	// An exact code wins before packed single-character compatibility, so GC
	// means the gCTS group rather than the Git (G) and CTS (C) groups together.
	if _, ok := groups[input]; ok {
		return []string{input}
	}

	// Delimiters make multi-code selections explicit: G,C disables Git and CTS.
	isSeparator := func(r rune) bool {
		return r == ',' || r == '/' || unicode.IsSpace(r)
	}
	if strings.IndexFunc(input, isSeparator) >= 0 {
		return strings.FieldsFunc(input, isSeparator)
	}

	// Preserve compact legacy forms such as 5THD, which are sequences of
	// single-character group codes.
	codes := make([]string, 0, len(input))
	for _, code := range input {
		key := string(code)
		if _, ok := groups[key]; ok {
			codes = append(codes, key)
		}
	}
	return codes
}
