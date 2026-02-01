package adt

import (
	"context"
	"strings"
)

// --- Unified Write Tool ---

// WriteSourceMode specifies how WriteSource behaves
type WriteSourceMode string

const (
	WriteModeUpdate WriteSourceMode = "update" // Update existing object only
	WriteModeCreate WriteSourceMode = "create" // Create new object only
	WriteModeUpsert WriteSourceMode = "upsert" // Create if not exists, update if exists (default)
)

// WriteSourceOptions configures WriteSource behavior
type WriteSourceOptions struct {
	Mode        WriteSourceMode // update, create, upsert (default: upsert)
	Description string          // Object description (for create)
	Package     string          // Package name (for create)
	TestSource  string          // Test source for CLAS (auto-creates test include)
	Transport   string          // Transport request number
	Method      string          // For CLAS only: update only this method (source must be METHOD...ENDMETHOD block)
}

// WriteSourceResult represents the result of WriteSource operation
type WriteSourceResult struct {
	Success      bool                `json:"success"`
	ObjectType   string              `json:"objectType"`
	ObjectName   string              `json:"objectName"`
	ObjectURL    string              `json:"objectUrl"`
	Mode         string              `json:"mode"`             // "created" or "updated"
	Method       string              `json:"method,omitempty"` // Method name if method-level update
	SyntaxErrors []SyntaxCheckResult `json:"syntaxErrors,omitempty"`
	Activation   *ActivationResult   `json:"activation,omitempty"`
	TestResults  *UnitTestResult     `json:"testResults,omitempty"` // For CLAS with TestSource
	Message      string              `json:"message,omitempty"`
}

// WriteSource is a unified tool for writing ABAP source code across different object types.
// Replaces WriteProgram, WriteClass, CreateAndActivateProgram, CreateClassWithTests.
//
// Supported types:
//   - PROG: Programs
//   - CLAS: Classes (optionally with test source)
//   - INTF: Interfaces
//   - DDLS: CDS DDL Sources
//   - BDEF: Behavior Definitions
//   - SRVD: Service Definitions
//   - SRVB: Service Bindings
//
// Mode:
//   - upsert (default): Auto-detect if object exists, create or update accordingly
//   - create: Create new object only (fails if exists)
//   - update: Update existing object only (fails if not exists)
func (c *Client) WriteSource(ctx context.Context, objectType, name, source string, opts *WriteSourceOptions) (*WriteSourceResult, error) {
	// Safety check for workflow operations
	if err := c.checkSafety(OpWorkflow, "WriteSource"); err != nil {
		return nil, err
	}

	if opts == nil {
		opts = &WriteSourceOptions{Mode: WriteModeUpsert}
	}
	if opts.Mode == "" {
		opts.Mode = WriteModeUpsert
	}

	objectType = strings.ToUpper(objectType)
	name = strings.ToUpper(name)

	result := &WriteSourceResult{
		ObjectType: objectType,
		ObjectName: name,
	}

	// Validate object type and route to appropriate handler
	switch objectType {
	case "PROG":
		return c.writeSourceProg(ctx, name, source, opts)
	case "CLAS":
		return c.writeSourceClas(ctx, name, source, opts)
	case "INTF":
		return c.writeSourceIntf(ctx, name, source, opts)
	case "DDLS", "BDEF", "SRVD", "SRVB":
		return c.writeSourceRAP(ctx, objectType, name, source, opts)
	default:
		result.Message = "Unsupported object type: " + objectType + " (supported: PROG, CLAS, INTF, DDLS, BDEF, SRVD, SRVB)"
		return result, nil
	}
}

// checkObjectExists checks if an object exists by trying to read it.
func (c *Client) checkObjectExists(ctx context.Context, objectType, name string) bool {
	// For simple source types, use the config-driven approach
	if cfg := GetObjectSourceConfig(objectType); cfg != nil {
		_, err := c.getObjectSourceByType(ctx, objectType, name)
		return err == nil
	}

	// Handle complex types that aren't in the config
	switch objectType {
	case "CLAS":
		_, err := c.GetClass(ctx, name)
		return err == nil
	case "SRVB":
		_, err := c.GetSRVB(ctx, name)
		return err == nil
	default:
		return false
	}
}

// determineWriteMode determines the actual write mode based on options and object existence.
func (c *Client) determineWriteMode(ctx context.Context, objectType, name string, opts *WriteSourceOptions) WriteSourceMode {
	if opts.Mode == WriteModeUpsert {
		if c.checkObjectExists(ctx, objectType, name) {
			return WriteModeUpdate
		}
		return WriteModeCreate
	}
	return opts.Mode
}

// hasFatalSyntaxErrors returns true if any syntax error is fatal (E, A, or X severity).
func hasFatalSyntaxErrors(errors []SyntaxCheckResult) bool {
	for _, se := range errors {
		if se.Severity == "E" || se.Severity == "A" || se.Severity == "X" {
			return true
		}
	}
	return false
}

// --- Specialized Convenience Functions ---
// These provide a simpler API for common operations, delegating to WriteSource.

// WriteProgram updates an existing program's source code.
func (c *Client) WriteProgram(ctx context.Context, name, source, transport string) (*WriteSourceResult, error) {
	return c.WriteSource(ctx, "PROG", name, source, &WriteSourceOptions{
		Mode:      WriteModeUpdate,
		Transport: transport,
	})
}

// WriteClass updates an existing class's source code.
func (c *Client) WriteClass(ctx context.Context, name, source, transport string) (*WriteSourceResult, error) {
	return c.WriteSource(ctx, "CLAS", name, source, &WriteSourceOptions{
		Mode:      WriteModeUpdate,
		Transport: transport,
	})
}

// CreateAndActivateProgram creates a new program with source code.
func (c *Client) CreateAndActivateProgram(ctx context.Context, name, description, packageName, source, transport string) (*WriteSourceResult, error) {
	return c.WriteSource(ctx, "PROG", name, source, &WriteSourceOptions{
		Mode:        WriteModeCreate,
		Description: description,
		Package:     packageName,
		Transport:   transport,
	})
}

// CreateClassWithTests creates a new class with main source and test include.
func (c *Client) CreateClassWithTests(ctx context.Context, name, description, packageName, classSource, testSource, transport string) (*WriteSourceResult, error) {
	return c.WriteSource(ctx, "CLAS", name, classSource, &WriteSourceOptions{
		Mode:        WriteModeCreate,
		Description: description,
		Package:     packageName,
		TestSource:  testSource,
		Transport:   transport,
	})
}
