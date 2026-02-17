package adt

import (
	"fmt"
	"strings"
)

// SafetyConfig defines protection parameters to prevent unintended system modifications
type SafetyConfig struct {
	// ReadOnly blocks all write operations (create, update, delete, activate)
	// When true, only read operations are allowed
	ReadOnly bool

	// BlockFreeSQL prevents execution of arbitrary SQL queries via RunQuery
	// Recommended for production to prevent accidental data access
	BlockFreeSQL bool

	// AllowedOps is a whitelist of operation types (if non-empty, only these are allowed)
	// Operation types:
	//   R - Read operations (GetClass, GetProgram, etc.)
	//   S - Search operations (SearchObject)
	//   Q - Query operations (GetTable, GetTableContents with predefined queries)
	//   F - Free SQL queries (RunQuery)
	//   C - Create operations (CreateObject, CreateTestInclude)
	//   U - Update operations (UpdateSource, UpdateClassInclude)
	//   D - Delete operations (DeleteObject)
	//   A - Activation operations (Activate)
	//   T - Test operations (RunUnitTests)
	//   L - Lock/Unlock operations
	//   I - Code intelligence (FindDefinition, CodeCompletion, etc.)
	//   W - Workflow operations (WriteClass, WriteProgram, CreateClassWithTests)
	//   X - Transport management (requires EnableTransports=true)
	// Example: "RSQ" = only reads, searches, and queries allowed
	AllowedOps string

	// DisallowedOps is a blacklist of operation types (takes precedence over AllowedOps)
	// Example: "CDUA" = block create, delete, update, activate
	DisallowedOps string

	// AllowedPackages restricts operations to specific packages (empty = all packages allowed)
	// Example: []string{"$TMP", "ZTEST", "Z*"} - only allow local and test packages
	// Supports wildcards: "Z*" matches all packages starting with Z
	AllowedPackages []string

	// DryRun mode - log operations but don't execute them (useful for testing)
	DryRun bool

	// EnableTransports explicitly enables transport management operations
	// By default false - requires conscious opt-in to work with transports
	// When false, all transport operations (create, release, list) are blocked
	EnableTransports bool

	// TransportReadOnly allows only read operations on transports (list, get)
	// When true, create/release/delete operations are blocked
	TransportReadOnly bool

	// AllowedTransports restricts transport operations to specific transports
	// Example: []string{"A4HK900110", "A4HK*", "DEV*"}
	// Supports wildcards: "A4HK*" matches all transports starting with A4HK
	// Empty = all transports allowed (within other restrictions)
	AllowedTransports []string

	// AllowTransportableEdits enables editing objects that require transport requests
	// By default false - only allows editing local objects ($TMP, $* packages)
	// When false:
	//   - Providing a transport parameter to EditSource/WriteSource will be rejected
	//   - Objects in transportable packages will fail at SAP level (no transport)
	// When true:
	//   - Transport parameter is accepted and passed to SAP
	//   - User takes responsibility for transport management
	// Use --allow-transportable-edits or SAP_ALLOW_TRANSPORTABLE_EDITS=true to enable
	AllowTransportableEdits bool
}

// DefaultSafetyConfig returns a safe default configuration (read-only, no free SQL)
func DefaultSafetyConfig() SafetyConfig {
	return SafetyConfig{
		ReadOnly:     true,
		BlockFreeSQL: true,
		AllowedOps:   "RSQTI", // Read, Search, Query, Test, Intelligence only
	}
}

// UnrestrictedSafetyConfig returns a configuration with no restrictions
// WARNING: Use with caution - allows all operations including destructive ones
func UnrestrictedSafetyConfig() SafetyConfig {
	return SafetyConfig{
		ReadOnly:     false,
		BlockFreeSQL: false,
		AllowedOps:   "", // Empty = all allowed
	}
}

// DevelopmentSafetyConfig returns a config suitable for development (local packages only)
func DevelopmentSafetyConfig() SafetyConfig {
	return SafetyConfig{
		ReadOnly:        false,
		BlockFreeSQL:    true,
		AllowedPackages: []string{"$TMP", "$TEST"},
	}
}

// OperationType represents different operation categories
type OperationType rune

const (
	OpRead         OperationType = 'R' // Read operations
	OpSearch       OperationType = 'S' // Search operations
	OpQuery        OperationType = 'Q' // Query operations (predefined)
	OpFreeSQL      OperationType = 'F' // Free SQL queries
	OpCreate       OperationType = 'C' // Create operations
	OpUpdate       OperationType = 'U' // Update operations
	OpDelete       OperationType = 'D' // Delete operations
	OpActivate     OperationType = 'A' // Activation
	OpTest         OperationType = 'T' // Unit tests
	OpLock         OperationType = 'L' // Lock/Unlock
	OpIntelligence OperationType = 'I' // Code intelligence
	OpWorkflow     OperationType = 'W' // High-level workflows
	OpTransport    OperationType = 'X' // Transport management (requires explicit opt-in)
)

// IsOperationAllowed checks if an operation type is allowed by the safety config
func (s *SafetyConfig) IsOperationAllowed(op OperationType) bool {
	opChar := rune(op)

	// Check DryRun - all operations are "allowed" but won't execute
	if s.DryRun {
		return true
	}

	// Check ReadOnly mode - blocks all write operations
	if s.ReadOnly {
		writeOps := "CDUAW" // Create, Delete, Update, Activate, Workflow
		if strings.ContainsRune(writeOps, opChar) {
			return false
		}
	}

	// Check BlockFreeSQL
	if s.BlockFreeSQL && op == OpFreeSQL {
		return false
	}

	// Check EnableTransports - transport operations require explicit opt-in
	if op == OpTransport && !s.EnableTransports {
		return false
	}

	// Check DisallowedOps (blacklist takes precedence)
	if s.DisallowedOps != "" && strings.ContainsRune(s.DisallowedOps, opChar) {
		return false
	}

	// Check AllowedOps (whitelist)
	if s.AllowedOps != "" && !strings.ContainsRune(s.AllowedOps, opChar) {
		return false
	}

	return true
}

// CheckOperation returns an error if the operation is not allowed
func (s *SafetyConfig) CheckOperation(op OperationType, opName string) error {
	if !s.IsOperationAllowed(op) {
		return fmt.Errorf("operation '%s' (type %c) is blocked by safety configuration", opName, op)
	}
	return nil
}

// IsPackageAllowed checks if operations on a given package are allowed
func (s *SafetyConfig) IsPackageAllowed(pkg string) bool {
	// Empty AllowedPackages = all packages allowed
	if len(s.AllowedPackages) == 0 {
		return true
	}

	pkg = strings.ToUpper(pkg)

	for _, allowed := range s.AllowedPackages {
		allowed = strings.ToUpper(allowed)

		// Exact match
		if allowed == pkg {
			return true
		}

		// Wildcard match (e.g., "Z*" matches "ZTEST", "ZRAY", etc.)
		if strings.HasSuffix(allowed, "*") {
			prefix := strings.TrimSuffix(allowed, "*")
			if strings.HasPrefix(pkg, prefix) {
				return true
			}
		}
	}

	return false
}

// CheckPackage returns an error if the package is not allowed
func (s *SafetyConfig) CheckPackage(pkg string) error {
	if !s.IsPackageAllowed(pkg) {
		return fmt.Errorf("operations on package '%s' are blocked by safety configuration (allowed: %v)",
			pkg, s.AllowedPackages)
	}
	return nil
}

// IsTransportAllowed checks if operations on a given transport are allowed
func (s *SafetyConfig) IsTransportAllowed(transport string) bool {
	// First check if transports are enabled at all
	if !s.EnableTransports {
		return false
	}

	// Empty AllowedTransports = all transports allowed
	if len(s.AllowedTransports) == 0 {
		return true
	}

	transport = strings.ToUpper(transport)

	for _, allowed := range s.AllowedTransports {
		allowed = strings.ToUpper(allowed)

		// Exact match
		if allowed == transport {
			return true
		}

		// Wildcard match (e.g., "A4HK*" matches "A4HK900110", etc.)
		if strings.HasSuffix(allowed, "*") {
			prefix := strings.TrimSuffix(allowed, "*")
			if strings.HasPrefix(transport, prefix) {
				return true
			}
		}
	}

	return false
}

// IsTransportWriteAllowed checks if write operations on transports are allowed
func (s *SafetyConfig) IsTransportWriteAllowed() bool {
	if !s.EnableTransports {
		return false
	}
	return !s.TransportReadOnly
}

// CheckTransport returns an error if the transport operation is not allowed
func (s *SafetyConfig) CheckTransport(transport, opName string, isWrite bool) error {
	// For read operations (ListTransports, GetTransport), allow if:
	// - EnableTransports is true, OR
	// - AllowTransportableEdits is true (user needs to discover transports)
	if !isWrite && (s.EnableTransports || s.AllowTransportableEdits) {
		// Read operation allowed, check transport whitelist if specified
		if transport != "" && transport != "*" && len(s.AllowedTransports) > 0 {
			if !s.isTransportInWhitelist(transport) {
				return fmt.Errorf("operation '%s' on transport '%s' is blocked by safety configuration (allowed: %v)",
					opName, transport, s.AllowedTransports)
			}
		}
		return nil
	}

	// For write operations or when neither flag is set, require EnableTransports
	if !s.EnableTransports {
		if s.AllowTransportableEdits && isWrite {
			return fmt.Errorf("transport write operation '%s' requires --enable-transports flag (--allow-transportable-edits only enables read operations)", opName)
		}
		return fmt.Errorf("transport operation '%s' is blocked: transports not enabled (use --enable-transports or SAP_ENABLE_TRANSPORTS=true)", opName)
	}

	// Check write permissions
	if isWrite && s.TransportReadOnly {
		return fmt.Errorf("transport write operation '%s' is blocked: transport read-only mode enabled", opName)
	}

	// Check transport whitelist (only for specific transport operations, not for list)
	if transport != "" && transport != "*" && len(s.AllowedTransports) > 0 {
		if !s.IsTransportAllowed(transport) {
			return fmt.Errorf("operation '%s' on transport '%s' is blocked by safety configuration (allowed: %v)",
				opName, transport, s.AllowedTransports)
		}
	}

	return nil
}

// CheckTransportableEdit returns an error if editing transportable objects is not allowed
// This should be called when a transport parameter is provided to write operations
func (s *SafetyConfig) CheckTransportableEdit(transport, opName string) error {
	if transport == "" {
		return nil // No transport = local object, always allowed
	}

	if !s.AllowTransportableEdits {
		return fmt.Errorf(
			"operation '%s' with transport '%s' is blocked: editing transportable objects is disabled.\n"+
				"Objects in transportable packages require explicit opt-in.\n"+
				"Use --allow-transportable-edits or SAP_ALLOW_TRANSPORTABLE_EDITS=true to enable.\n"+
				"WARNING: This allows modifications to non-local objects that may affect production systems.",
			opName, transport)
	}

	// If transportable edits are allowed, also check transport whitelist
	if len(s.AllowedTransports) > 0 && !s.isTransportInWhitelist(transport) {
		return fmt.Errorf("operation '%s' with transport '%s' is blocked by safety configuration (allowed transports: %v)",
			opName, transport, s.AllowedTransports)
	}

	return nil
}

// isTransportInWhitelist checks if a transport matches the AllowedTransports whitelist
// This is a helper that doesn't check EnableTransports (unlike IsTransportAllowed)
func (s *SafetyConfig) isTransportInWhitelist(transport string) bool {
	if len(s.AllowedTransports) == 0 {
		return true // Empty whitelist = all allowed
	}

	transport = strings.ToUpper(transport)

	for _, allowed := range s.AllowedTransports {
		allowed = strings.ToUpper(allowed)

		// Exact match
		if allowed == transport {
			return true
		}

		// Wildcard match (e.g., "DEVK*" matches "DEVK900123", etc.)
		if strings.HasSuffix(allowed, "*") {
			prefix := strings.TrimSuffix(allowed, "*")
			if strings.HasPrefix(transport, prefix) {
				return true
			}
		}
	}

	return false
}

// String returns a human-readable description of the safety configuration
func (s *SafetyConfig) String() string {
	var parts []string

	if s.ReadOnly {
		parts = append(parts, "READ-ONLY")
	}

	if s.BlockFreeSQL {
		parts = append(parts, "NO-FREE-SQL")
	}

	if s.DryRun {
		parts = append(parts, "DRY-RUN")
	}

	if s.AllowedOps != "" {
		parts = append(parts, fmt.Sprintf("AllowedOps=%s", s.AllowedOps))
	}

	if s.DisallowedOps != "" {
		parts = append(parts, fmt.Sprintf("DisallowedOps=%s", s.DisallowedOps))
	}

	if len(s.AllowedPackages) > 0 {
		parts = append(parts, fmt.Sprintf("AllowedPackages=%v", s.AllowedPackages))
	}

	if s.EnableTransports {
		parts = append(parts, "TRANSPORTS-ENABLED")
		if s.TransportReadOnly {
			parts = append(parts, "TRANSPORT-READ-ONLY")
		}
		if len(s.AllowedTransports) > 0 {
			parts = append(parts, fmt.Sprintf("AllowedTransports=%v", s.AllowedTransports))
		}
	}

	if s.AllowTransportableEdits {
		parts = append(parts, "TRANSPORTABLE-EDITS-ALLOWED")
	}

	if len(parts) == 0 {
		return "UNRESTRICTED"
	}

	return strings.Join(parts, ", ")
}
