package adt

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
)

// --- Unified Read Tool ---

// GetSourceOptions configures GetSource behavior
type GetSourceOptions struct {
	Parent  string // Function group name (required for FUNC type)
	Include string // Class include type: definitions, implementations, macros, testclasses (optional for CLAS type)
	Method  string // Method name for method-level source extraction (optional for CLAS type)
}

// GetSource is a unified tool for reading ABAP source code across different object types.
//
// Simple source types (use getObjectSourceByType internally):
//   - PROG: Programs
//   - INTF: Interfaces
//   - INCL: Includes
//   - DDLS: CDS DDL sources
//   - VIEW: DDIC database views (classic SE11)
//   - BDEF: Behavior Definitions (RAP)
//   - SRVD: Service Definitions (RAP)
//   - TABL: Database tables
//   - STRU: Structures
//   - DTEL: Data elements
//   - DOMA: Domains
//   - XSLT: Simple Transformations
//
// Complex types (special handling):
//   - CLAS: Classes (with include/method options)
//   - FUNC: Function modules (requires parent function group)
//   - FUGR: Function groups (returns JSON metadata)
//   - SRVB: Service Bindings (returns JSON metadata)
//   - MSAG: Message classes (returns JSON metadata)
func (c *Client) GetSource(ctx context.Context, objectType, name string, opts *GetSourceOptions) (string, error) {
	// Safety check for read operations
	if err := c.checkSafety(OpRead, "GetSource"); err != nil {
		return "", err
	}

	if opts == nil {
		opts = &GetSourceOptions{}
	}

	objectType = strings.ToUpper(objectType)

	// Handle complex types that need special logic
	switch objectType {
	case "CLAS":
		name = strings.ToUpper(name)
		// Method-level source extraction
		if opts.Method != "" {
			return c.GetClassMethodSource(ctx, name, opts.Method)
		}
		// Include-level source extraction
		if opts.Include != "" {
			return c.GetClassInclude(ctx, name, ClassIncludeType(opts.Include))
		}
		return c.GetClassSource(ctx, name)

	case "FUNC":
		if opts.Parent == "" {
			return "", fmt.Errorf("parent (function group name) is required for FUNC type")
		}
		return c.GetFunction(ctx, strings.ToUpper(name), opts.Parent)

	case "FUGR":
		// GetFunctionGroup returns JSON metadata (function module list), not source
		fg, err := c.GetFunctionGroup(ctx, strings.ToUpper(name))
		if err != nil {
			return "", err
		}
		data, err := json.Marshal(fg)
		if err != nil {
			return "", fmt.Errorf("failed to serialize function group: %w", err)
		}
		return string(data), nil

	case "SRVB":
		// GetSRVB returns metadata structure, serialize to JSON
		sb, err := c.GetSRVB(ctx, name)
		if err != nil {
			return "", err
		}
		data, err := json.Marshal(sb)
		if err != nil {
			return "", fmt.Errorf("failed to serialize service binding: %w", err)
		}
		return string(data), nil

	case "MSAG":
		// GetMessageClass returns JSON metadata (message list), not source
		mc, err := c.GetMessageClass(ctx, strings.ToUpper(name))
		if err != nil {
			return "", err
		}
		data, err := json.Marshal(mc)
		if err != nil {
			return "", fmt.Errorf("failed to serialize message class: %w", err)
		}
		return string(data), nil
	}

	// For simple source types, use the config-driven approach
	if cfg := GetObjectSourceConfig(objectType); cfg != nil {
		return c.getObjectSourceByType(ctx, objectType, name)
	}

	// Build helpful error message with all supported types
	simpleTypes := SupportedSourceTypes()
	complexTypes := []string{"CLAS", "FUNC", "FUGR", "SRVB", "MSAG"}
	allTypes := append(simpleTypes, complexTypes...)
	return "", fmt.Errorf("unsupported object type: %s (supported: %s)", objectType, strings.Join(allTypes, ", "))
}
