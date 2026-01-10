package adt

import (
	"context"
	"fmt"
	"strings"
)

// --- Interface Write Operations ---

// writeSourceIntf handles write operations for INTF (interfaces).
func (c *Client) writeSourceIntf(ctx context.Context, name, source string, opts *WriteSourceOptions) (*WriteSourceResult, error) {
	result := &WriteSourceResult{
		ObjectType: "INTF",
		ObjectName: name,
	}

	actualMode := c.determineWriteMode(ctx, "INTF", name, opts)

	// Validate mode
	if actualMode == WriteModeCreate {
		if opts.Package == "" {
			result.Message = "Package is required for creating new interfaces"
			return result, nil
		}
		if opts.Description == "" {
			result.Message = "Description is required for creating new interfaces"
			return result, nil
		}
		return c.createInterface(ctx, name, source, opts)
	}

	return c.updateInterface(ctx, name, source, opts)
}

// createInterface creates a new interface with source code and activates it.
// Workflow: CreateObject -> Lock -> SyntaxCheck -> UpdateSource -> Unlock -> Activate
func (c *Client) createInterface(ctx context.Context, name, source string, opts *WriteSourceOptions) (*WriteSourceResult, error) {
	name = strings.ToUpper(name)
	packageName := strings.ToUpper(opts.Package)

	// Check package restrictions
	if err := c.checkPackageSafety(packageName); err != nil {
		return nil, err
	}

	objectURL := fmt.Sprintf("/sap/bc/adt/oo/interfaces/%s", name)
	sourceURL := objectURL + "/source/main"

	result := &WriteSourceResult{
		ObjectType: "INTF",
		ObjectName: name,
		ObjectURL:  objectURL,
		Mode:       "created",
	}

	// Step 1: Create the interface
	err := c.CreateObject(ctx, CreateObjectOptions{
		ObjectType:  ObjectTypeInterface,
		Name:        name,
		Description: opts.Description,
		PackageName: packageName,
		Transport:   opts.Transport,
	})
	if err != nil {
		result.Message = fmt.Sprintf("Failed to create interface: %v", err)
		return result, nil
	}

	// Step 2: Syntax check
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

	// Step 3: Lock
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

	// Step 4: Update source
	err = c.UpdateSource(ctx, sourceURL, source, lock.LockHandle, opts.Transport)
	if err != nil {
		result.Message = fmt.Sprintf("Failed to update source: %v", err)
		return result, nil
	}

	// Step 5: Unlock
	err = c.UnlockObject(ctx, objectURL, lock.LockHandle)
	if err != nil {
		result.Message = fmt.Sprintf("Failed to unlock object: %v", err)
		return result, nil
	}

	// Step 6: Activate
	activation, err := c.Activate(ctx, objectURL, name)
	if err != nil {
		result.Message = fmt.Sprintf("Failed to activate: %v", err)
		result.Activation = activation
		return result, nil
	}

	result.Activation = activation
	if activation.Success {
		result.Success = true
		result.Message = "Interface created and activated successfully"
	} else {
		result.Message = "Activation failed - check activation messages"
	}

	return result, nil
}

// updateInterface updates an existing interface's source code.
// Workflow: SyntaxCheck -> Lock -> UpdateSource -> Unlock -> Activate
func (c *Client) updateInterface(ctx context.Context, name, source string, opts *WriteSourceOptions) (*WriteSourceResult, error) {
	name = strings.ToUpper(name)
	objectURL := fmt.Sprintf("/sap/bc/adt/oo/interfaces/%s", name)
	sourceURL := objectURL + "/source/main"

	result := &WriteSourceResult{
		ObjectType: "INTF",
		ObjectName: name,
		ObjectURL:  objectURL,
		Mode:       "updated",
	}

	// Step 1: Syntax check
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

	// Step 3: Update
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
		result.Message = "Interface updated and activated successfully"
	} else {
		result.Message = "Activation failed - check activation messages"
	}

	return result, nil
}
