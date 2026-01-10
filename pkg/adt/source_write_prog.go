package adt

import (
	"context"
	"fmt"
	"strings"
)

// --- Program Write Operations ---

// writeSourceProg handles write operations for PROG (programs).
func (c *Client) writeSourceProg(ctx context.Context, name, source string, opts *WriteSourceOptions) (*WriteSourceResult, error) {
	result := &WriteSourceResult{
		ObjectType: "PROG",
		ObjectName: name,
	}

	actualMode := c.determineWriteMode(ctx, "PROG", name, opts)

	// Validate mode
	if actualMode == WriteModeCreate {
		if opts.Package == "" {
			result.Message = "Package is required for creating new programs"
			return result, nil
		}
		if opts.Description == "" {
			result.Message = "Description is required for creating new programs"
			return result, nil
		}
		return c.createProgram(ctx, name, source, opts)
	}

	return c.updateProgram(ctx, name, source, opts)
}

// createProgram creates a new program with source code and activates it.
// Workflow: CreateObject -> Lock -> UpdateSource -> Unlock -> Activate
func (c *Client) createProgram(ctx context.Context, name, source string, opts *WriteSourceOptions) (*WriteSourceResult, error) {
	name = strings.ToUpper(name)
	packageName := strings.ToUpper(opts.Package)

	// Check package restrictions
	if err := c.checkPackageSafety(packageName); err != nil {
		return nil, err
	}

	objectURL := fmt.Sprintf("/sap/bc/adt/programs/programs/%s", name)
	sourceURL := objectURL + "/source/main"

	result := &WriteSourceResult{
		ObjectType: "PROG",
		ObjectName: name,
		ObjectURL:  objectURL,
		Mode:       "created",
	}

	// Step 1: Create the program
	err := c.CreateObject(ctx, CreateObjectOptions{
		ObjectType:  ObjectTypeProgram,
		Name:        name,
		Description: opts.Description,
		PackageName: packageName,
		Transport:   opts.Transport,
	})
	if err != nil {
		result.Message = fmt.Sprintf("Failed to create program: %v", err)
		return result, nil
	}

	// Step 2: Lock
	lock, err := c.LockObject(ctx, objectURL, "MODIFY")
	if err != nil {
		result.Message = fmt.Sprintf("Failed to lock object: %v", err)
		return result, nil
	}

	defer func() {
		if !result.Success {
			_ = c.UnlockObject(ctx, objectURL, lock.LockHandle)
		}
	}()

	// Step 3: Update source
	err = c.UpdateSource(ctx, sourceURL, source, lock.LockHandle, opts.Transport)
	if err != nil {
		result.Message = fmt.Sprintf("Failed to update source: %v", err)
		return result, nil
	}

	// Step 4: Unlock
	err = c.UnlockObject(ctx, objectURL, lock.LockHandle)
	if err != nil {
		result.Message = fmt.Sprintf("Failed to unlock object: %v", err)
		return result, nil
	}

	// Step 5: Activate
	activation, err := c.Activate(ctx, objectURL, name)
	if err != nil {
		result.Message = fmt.Sprintf("Failed to activate: %v", err)
		result.Activation = activation
		return result, nil
	}

	result.Activation = activation
	if activation.Success {
		result.Success = true
		result.Message = "Program created and activated successfully"
	} else {
		result.Message = "Activation failed - check activation messages"
	}

	return result, nil
}

// updateProgram updates an existing program's source code.
// Workflow: SyntaxCheck -> Lock -> UpdateSource -> Unlock -> Activate
func (c *Client) updateProgram(ctx context.Context, name, source string, opts *WriteSourceOptions) (*WriteSourceResult, error) {
	name = strings.ToUpper(name)
	objectURL := fmt.Sprintf("/sap/bc/adt/programs/programs/%s", name)
	sourceURL := objectURL + "/source/main"

	result := &WriteSourceResult{
		ObjectType: "PROG",
		ObjectName: name,
		ObjectURL:  objectURL,
		Mode:       "updated",
	}

	// Step 1: Syntax check before making changes
	syntaxErrors, err := c.SyntaxCheck(ctx, objectURL, source)
	if err != nil {
		result.Message = fmt.Sprintf("Syntax check failed: %v", err)
		return result, nil
	}

	result.SyntaxErrors = syntaxErrors
	if hasFatalSyntaxErrors(syntaxErrors) {
		result.Message = "Source has syntax errors - not saved"
		return result, nil
	}

	// Step 2: Lock the object
	lock, err := c.LockObject(ctx, objectURL, "MODIFY")
	if err != nil {
		result.Message = fmt.Sprintf("Failed to lock object: %v", err)
		return result, nil
	}

	// Ensure we unlock on any error
	defer func() {
		if !result.Success {
			_ = c.UnlockObject(ctx, objectURL, lock.LockHandle)
		}
	}()

	// Step 3: Update source
	err = c.UpdateSource(ctx, sourceURL, source, lock.LockHandle, opts.Transport)
	if err != nil {
		result.Message = fmt.Sprintf("Failed to update source: %v", err)
		return result, nil
	}

	// Step 4: Unlock before activation (SAP requirement)
	err = c.UnlockObject(ctx, objectURL, lock.LockHandle)
	if err != nil {
		result.Message = fmt.Sprintf("Failed to unlock object: %v", err)
		return result, nil
	}

	// Step 5: Activate
	activation, err := c.Activate(ctx, objectURL, name)
	if err != nil {
		result.Message = fmt.Sprintf("Failed to activate: %v", err)
		result.Activation = activation
		return result, nil
	}

	result.Activation = activation
	if activation.Success {
		result.Success = true
		result.Message = "Program updated and activated successfully"
	} else {
		result.Message = "Activation failed - check activation messages"
	}

	return result, nil
}
