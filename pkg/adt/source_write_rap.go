package adt

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
)

// --- RAP Object Write Operations ---
// Handles DDLS (CDS DDL Sources), BDEF (Behavior Definitions),
// SRVD (Service Definitions), and SRVB (Service Bindings).

// writeSourceRAP handles write operations for RAP objects.
func (c *Client) writeSourceRAP(ctx context.Context, objectType, name, source string, opts *WriteSourceOptions) (*WriteSourceResult, error) {
	result := &WriteSourceResult{
		ObjectType: objectType,
		ObjectName: name,
	}

	actualMode := c.determineWriteMode(ctx, objectType, name, opts)

	// Validate mode
	if actualMode == WriteModeCreate {
		if opts.Package == "" {
			result.Message = fmt.Sprintf("Package is required for creating new %s", objectType)
			return result, nil
		}
		if opts.Description == "" {
			result.Message = fmt.Sprintf("Description is required for creating new %s", objectType)
			return result, nil
		}
		return c.createRAP(ctx, objectType, name, source, opts)
	}

	return c.updateRAP(ctx, objectType, name, source, opts)
}

// createRAP creates a new RAP object (DDLS, BDEF, SRVD, SRVB) with source code and activates it.
func (c *Client) createRAP(ctx context.Context, objectType, name, source string, opts *WriteSourceOptions) (*WriteSourceResult, error) {
	name = strings.ToUpper(name)
	packageName := strings.ToUpper(opts.Package)

	// Check package restrictions
	if err := c.checkPackageSafety(packageName); err != nil {
		return nil, err
	}

	// Special handling for SRVB (Service Binding)
	if objectType == "SRVB" {
		return c.createSRVB(ctx, name, source, opts)
	}

	// Get object type and URL
	var objType CreatableObjectType
	var objectURL string
	switch objectType {
	case "DDLS":
		objType = ObjectTypeDDLS
		objectURL = GetObjectURL(ObjectTypeDDLS, name, "")
	case "BDEF":
		objType = ObjectTypeBDEF
		objectURL = GetObjectURL(ObjectTypeBDEF, name, "")
	case "SRVD":
		objType = ObjectTypeSRVD
		objectURL = GetObjectURL(ObjectTypeSRVD, name, "")
	default:
		return &WriteSourceResult{
			ObjectType: objectType,
			ObjectName: name,
			Message:    fmt.Sprintf("Unsupported RAP object type: %s", objectType),
		}, nil
	}

	sourceURL := objectURL + "/source/main"

	result := &WriteSourceResult{
		ObjectType: objectType,
		ObjectName: name,
		ObjectURL:  objectURL,
		Mode:       "created",
	}

	// Create object first
	// For BDEF, include source in creation (ADT API requirement)
	createOpts := CreateObjectOptions{
		ObjectType:  objType,
		Name:        name,
		Description: opts.Description,
		PackageName: packageName,
		Transport:   opts.Transport,
	}
	if objectType == "BDEF" {
		createOpts.Source = source // BDEF requires source embedded in creation request
	}
	err := c.CreateObject(ctx, createOpts)
	if err != nil {
		result.Message = fmt.Sprintf("Failed to create %s: %v", objectType, err)
		return result, nil
	}

	// For BDEF, creation creates empty shell, then update source
	if objectType == "BDEF" {
		return c.updateBDEFAfterCreate(ctx, objectURL, name, source, opts, result), nil
	}

	// Syntax check (for DDLS, SRVD)
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

	// Lock
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

	// Update source
	err = c.UpdateSource(ctx, sourceURL, source, lock.LockHandle, opts.Transport)
	if err != nil {
		result.Message = fmt.Sprintf("Failed to update source: %v", err)
		return result, nil
	}

	// Unlock
	err = c.UnlockObject(ctx, objectURL, lock.LockHandle)
	if err != nil {
		result.Message = fmt.Sprintf("Failed to unlock object: %v", err)
		return result, nil
	}

	// Activate
	activation, err := c.Activate(ctx, objectURL, name)
	if err != nil {
		result.Message = fmt.Sprintf("Failed to activate: %v", err)
		result.Activation = activation
		return result, nil
	}

	result.Activation = activation
	if activation.Success {
		result.Success = true
		result.Message = fmt.Sprintf("%s created and activated successfully", objectType)
	} else {
		result.Message = "Activation failed - check activation messages"
	}

	return result, nil
}

// updateBDEFAfterCreate handles the special case of updating BDEF source after creation.
func (c *Client) updateBDEFAfterCreate(ctx context.Context, objectURL, name, source string, opts *WriteSourceOptions, result *WriteSourceResult) *WriteSourceResult {
	sourceURL := objectURL + "/source/main"

	// Lock
	lock, err := c.LockObject(ctx, objectURL, "MODIFY")
	if err != nil {
		result.Message = fmt.Sprintf("Failed to lock BDEF: %v", err)
		return result
	}

	// Update source
	err = c.UpdateSource(ctx, sourceURL, source, lock.LockHandle, opts.Transport)
	if err != nil {
		// Unlock on failure
		_ = c.UnlockObject(ctx, objectURL, lock.LockHandle)
		result.Message = fmt.Sprintf("Failed to update BDEF source: %v", err)
		return result
	}

	// Unlock
	err = c.UnlockObject(ctx, objectURL, lock.LockHandle)
	if err != nil {
		result.Message = fmt.Sprintf("Failed to unlock BDEF: %v", err)
		return result
	}

	// Activate
	activation, err := c.Activate(ctx, objectURL, name)
	if err != nil {
		result.Message = fmt.Sprintf("Failed to activate: %v", err)
		result.Activation = activation
		return result
	}
	result.Activation = activation
	if activation.Success {
		result.Success = true
		result.Message = "BDEF created and activated successfully"
	} else {
		result.Message = "Activation failed - check activation messages"
	}
	return result
}

// createSRVB creates a new Service Binding.
// Source is expected to be JSON configuration.
func (c *Client) createSRVB(ctx context.Context, name, source string, opts *WriteSourceOptions) (*WriteSourceResult, error) {
	packageName := strings.ToUpper(opts.Package)

	// Check package restrictions
	if err := c.checkPackageSafety(packageName); err != nil {
		return nil, err
	}

	// Parse JSON to get binding parameters
	var srvbConfig struct {
		ServiceDefName  string `json:"serviceDefName"`
		BindingType     string `json:"bindingType"`     // ODATA
		BindingVersion  string `json:"bindingVersion"`  // V2 or V4
		BindingCategory string `json:"bindingCategory"` // 0=WebAPI, 1=UI
	}
	if err := json.Unmarshal([]byte(source), &srvbConfig); err != nil {
		return &WriteSourceResult{
			ObjectType: "SRVB",
			ObjectName: name,
			Message:    fmt.Sprintf("Invalid SRVB JSON config: %v (expected: {\"serviceDefName\":\"...\",\"bindingType\":\"ODATA\",\"bindingVersion\":\"V4\"})", err),
		}, nil
	}

	// Validate required fields
	if srvbConfig.ServiceDefName == "" {
		return &WriteSourceResult{
			ObjectType: "SRVB",
			ObjectName: name,
			Message:    "serviceDefName is required in SRVB config",
		}, nil
	}

	// Set defaults
	if srvbConfig.BindingType == "" {
		srvbConfig.BindingType = "ODATA"
	}
	if srvbConfig.BindingVersion == "" {
		srvbConfig.BindingVersion = "V4"
	}
	if srvbConfig.BindingCategory == "" {
		srvbConfig.BindingCategory = "0" // Web API
	}

	objectURL := fmt.Sprintf("/sap/bc/adt/businessservices/bindings/%s", strings.ToLower(name))

	result := &WriteSourceResult{
		ObjectType: "SRVB",
		ObjectName: name,
		ObjectURL:  objectURL,
		Mode:       "created",
	}

	// Create SRVB
	err := c.CreateObject(ctx, CreateObjectOptions{
		ObjectType:        ObjectTypeSRVB,
		Name:              name,
		Description:       opts.Description,
		PackageName:       packageName,
		ServiceDefinition: srvbConfig.ServiceDefName,
		BindingType:       srvbConfig.BindingType,
		BindingVersion:    srvbConfig.BindingVersion,
		BindingCategory:   srvbConfig.BindingCategory,
		Transport:         opts.Transport,
	})
	if err != nil {
		result.Message = fmt.Sprintf("Failed to create SRVB: %v", err)
		return result, nil
	}

	// Activate
	activation, err := c.Activate(ctx, objectURL, name)
	if err != nil {
		result.Message = fmt.Sprintf("Failed to activate: %v", err)
		result.Activation = activation
		return result, nil
	}
	result.Activation = activation
	if activation.Success {
		result.Success = true
		result.Message = "SRVB created and activated successfully"
	} else {
		result.Message = "Activation failed - check activation messages"
	}
	return result, nil
}

// updateRAP updates an existing RAP object's source code.
func (c *Client) updateRAP(ctx context.Context, objectType, name, source string, opts *WriteSourceOptions) (*WriteSourceResult, error) {
	name = strings.ToUpper(name)

	// Get object URL
	var objectURL string
	switch objectType {
	case "DDLS":
		objectURL = GetObjectURL(ObjectTypeDDLS, name, "")
	case "BDEF":
		objectURL = GetObjectURL(ObjectTypeBDEF, name, "")
	case "SRVD":
		objectURL = GetObjectURL(ObjectTypeSRVD, name, "")
	case "SRVB":
		// SRVB updates are not typical - it's config-based
		return &WriteSourceResult{
			ObjectType: objectType,
			ObjectName: name,
			Message:    "SRVB updates are not supported - recreate with new configuration",
		}, nil
	default:
		return &WriteSourceResult{
			ObjectType: objectType,
			ObjectName: name,
			Message:    fmt.Sprintf("Unsupported RAP object type: %s", objectType),
		}, nil
	}

	sourceURL := objectURL + "/source/main"

	result := &WriteSourceResult{
		ObjectType: objectType,
		ObjectName: name,
		ObjectURL:  objectURL,
		Mode:       "updated",
	}

	// Syntax check
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

	// Lock
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

	// Update
	err = c.UpdateSource(ctx, sourceURL, source, lock.LockHandle, opts.Transport)
	if err != nil {
		result.Message = fmt.Sprintf("Failed to update source: %v", err)
		return result, nil
	}

	// Unlock
	err = c.UnlockObject(ctx, objectURL, lock.LockHandle)
	if err != nil {
		result.Message = fmt.Sprintf("Failed to unlock object: %v", err)
		return result, nil
	}

	// Activate
	activation, err := c.Activate(ctx, objectURL, name)
	if err != nil {
		result.Message = fmt.Sprintf("Failed to activate: %v", err)
		result.Activation = activation
		return result, nil
	}

	result.Activation = activation
	if activation.Success {
		result.Success = true
		result.Message = fmt.Sprintf("%s updated and activated successfully", objectType)
	} else {
		result.Message = "Activation failed - check activation messages"
	}

	return result, nil
}
