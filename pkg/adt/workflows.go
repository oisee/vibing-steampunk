package adt

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// --- Workflow Tools ---
// These tools combine multiple operations into atomic workflows for simpler usage.

// WriteProgramResult represents the result of writing a program.
type WriteProgramResult struct {
	Success      bool                       `json:"success"`
	ProgramName  string                     `json:"programName"`
	ObjectURL    string                     `json:"objectUrl"`
	SyntaxErrors []SyntaxCheckResult        `json:"syntaxErrors,omitempty"`
	Activation   *ActivationResult          `json:"activation,omitempty"`
	Message      string                     `json:"message,omitempty"`
}

// WriteProgram performs Lock -> SyntaxCheck -> UpdateSource -> Unlock -> Activate workflow.
// This is a convenience method for updating existing programs.
func (c *Client) WriteProgram(ctx context.Context, programName string, source string, transport string) (*WriteProgramResult, error) {
	// Safety check for workflow operations
	if err := c.checkSafety(OpWorkflow, "WriteProgram"); err != nil {
		return nil, err
	}

	programName = strings.ToUpper(programName)
	objectURL := fmt.Sprintf("/sap/bc/adt/programs/programs/%s", programName)
	sourceURL := objectURL + "/source/main"

	result := &WriteProgramResult{
		ProgramName: programName,
		ObjectURL:   objectURL,
	}

	// Step 1: Syntax check before making changes
	syntaxErrors, err := c.SyntaxCheck(ctx, objectURL, source)
	if err != nil {
		result.Message = fmt.Sprintf("Syntax check failed: %v", err)
		return result, nil
	}

	// Check for syntax errors
	for _, se := range syntaxErrors {
		if se.Severity == "E" || se.Severity == "A" || se.Severity == "X" {
			result.SyntaxErrors = syntaxErrors
			result.Message = "Source has syntax errors - not saved"
			return result, nil
		}
	}
	result.SyntaxErrors = syntaxErrors // Include warnings if any

	// Step 2: Lock the object
	lock, err := c.LockObject(ctx, objectURL, "MODIFY")
	if err != nil {
		result.Message = fmt.Sprintf("Failed to lock object: %v", err)
		return result, nil
	}

	// Ensure we unlock on any error
	defer func() {
		if !result.Success {
			c.UnlockObject(ctx, objectURL, lock.LockHandle)
		}
	}()

	// Step 3: Update source
	err = c.UpdateSource(ctx, sourceURL, source, lock.LockHandle, transport)
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
	activation, err := c.Activate(ctx, objectURL, programName)
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

// WriteClassResult represents the result of writing a class.
type WriteClassResult struct {
	Success      bool                       `json:"success"`
	ClassName    string                     `json:"className"`
	ObjectURL    string                     `json:"objectUrl"`
	SyntaxErrors []SyntaxCheckResult        `json:"syntaxErrors,omitempty"`
	Activation   *ActivationResult          `json:"activation,omitempty"`
	Message      string                     `json:"message,omitempty"`
}

// WriteClass performs Lock -> SyntaxCheck -> UpdateSource -> Unlock -> Activate workflow for classes.
func (c *Client) WriteClass(ctx context.Context, className string, source string, transport string) (*WriteClassResult, error) {
	// Safety check for workflow operations
	if err := c.checkSafety(OpWorkflow, "WriteClass"); err != nil {
		return nil, err
	}

	className = strings.ToUpper(className)
	objectURL := fmt.Sprintf("/sap/bc/adt/oo/classes/%s", className)
	sourceURL := objectURL + "/source/main"

	result := &WriteClassResult{
		ClassName: className,
		ObjectURL: objectURL,
	}

	// Step 1: Syntax check
	syntaxErrors, err := c.SyntaxCheck(ctx, objectURL, source)
	if err != nil {
		result.Message = fmt.Sprintf("Syntax check failed: %v", err)
		return result, nil
	}

	// Check for syntax errors
	for _, se := range syntaxErrors {
		if se.Severity == "E" || se.Severity == "A" || se.Severity == "X" {
			result.SyntaxErrors = syntaxErrors
			result.Message = "Source has syntax errors - not saved"
			return result, nil
		}
	}
	result.SyntaxErrors = syntaxErrors

	// Step 2: Lock
	lock, err := c.LockObject(ctx, objectURL, "MODIFY")
	if err != nil {
		result.Message = fmt.Sprintf("Failed to lock object: %v", err)
		return result, nil
	}

	defer func() {
		if !result.Success {
			c.UnlockObject(ctx, objectURL, lock.LockHandle)
		}
	}()

	// Step 3: Update source
	err = c.UpdateSource(ctx, sourceURL, source, lock.LockHandle, transport)
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
	activation, err := c.Activate(ctx, objectURL, className)
	if err != nil {
		result.Message = fmt.Sprintf("Failed to activate: %v", err)
		result.Activation = activation
		return result, nil
	}

	result.Activation = activation
	if activation.Success {
		result.Success = true
		result.Message = "Class updated and activated successfully"
	} else {
		result.Message = "Activation failed - check activation messages"
	}

	return result, nil
}

// CreateProgramResult represents the result of creating a program.
type CreateProgramResult struct {
	Success      bool                `json:"success"`
	ProgramName  string              `json:"programName"`
	ObjectURL    string              `json:"objectUrl"`
	SyntaxErrors []SyntaxCheckResult `json:"syntaxErrors,omitempty"`
	Activation   *ActivationResult   `json:"activation,omitempty"`
	Message      string              `json:"message,omitempty"`
}

// CreateAndActivateProgram creates a new program with source code and activates it.
// Workflow: CreateObject -> Lock -> UpdateSource -> Unlock -> Activate
func (c *Client) CreateAndActivateProgram(ctx context.Context, programName string, description string, packageName string, source string, transport string) (*CreateProgramResult, error) {
	// Safety check for workflow operations
	if err := c.checkSafety(OpWorkflow, "CreateAndActivateProgram"); err != nil {
		return nil, err
	}

	programName = strings.ToUpper(programName)
	packageName = strings.ToUpper(packageName)

	// Check package restrictions
	if err := c.checkPackageSafety(packageName); err != nil {
		return nil, err
	}

	objectURL := fmt.Sprintf("/sap/bc/adt/programs/programs/%s", programName)
	sourceURL := objectURL + "/source/main"

	result := &CreateProgramResult{
		ProgramName: programName,
		ObjectURL:   objectURL,
	}

	// Step 1: Create the program
	err := c.CreateObject(ctx, CreateObjectOptions{
		ObjectType:  ObjectTypeProgram,
		Name:        programName,
		Description: description,
		PackageName: packageName,
		Transport:   transport,
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
			c.UnlockObject(ctx, objectURL, lock.LockHandle)
		}
	}()

	// Step 3: Update source
	err = c.UpdateSource(ctx, sourceURL, source, lock.LockHandle, transport)
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
	activation, err := c.Activate(ctx, objectURL, programName)
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

// CreateClassWithTestsResult represents the result of creating a class with unit tests.
type CreateClassWithTestsResult struct {
	Success        bool              `json:"success"`
	ClassName      string            `json:"className"`
	ObjectURL      string            `json:"objectUrl"`
	Activation     *ActivationResult `json:"activation,omitempty"`
	UnitTestResult *UnitTestResult   `json:"unitTestResult,omitempty"`
	Message        string            `json:"message,omitempty"`
}

// CreateClassWithTests creates a new class with unit tests and runs them.
// Workflow: CreateObject -> Lock -> UpdateSource -> CreateTestInclude -> UpdateClassInclude -> Unlock -> Activate -> RunUnitTests
func (c *Client) CreateClassWithTests(ctx context.Context, className string, description string, packageName string, classSource string, testSource string, transport string) (*CreateClassWithTestsResult, error) {
	// Safety check for workflow operations
	if err := c.checkSafety(OpWorkflow, "CreateClassWithTests"); err != nil {
		return nil, err
	}

	className = strings.ToUpper(className)
	packageName = strings.ToUpper(packageName)

	// Check package restrictions
	if err := c.checkPackageSafety(packageName); err != nil {
		return nil, err
	}

	objectURL := fmt.Sprintf("/sap/bc/adt/oo/classes/%s", className)
	sourceURL := objectURL + "/source/main"

	result := &CreateClassWithTestsResult{
		ClassName: className,
		ObjectURL: objectURL,
	}

	// Step 1: Create the class
	err := c.CreateObject(ctx, CreateObjectOptions{
		ObjectType:  ObjectTypeClass,
		Name:        className,
		Description: description,
		PackageName: packageName,
		Transport:   transport,
	})
	if err != nil {
		result.Message = fmt.Sprintf("Failed to create class: %v", err)
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
			c.UnlockObject(ctx, objectURL, lock.LockHandle)
		}
	}()

	// Step 3: Update main source
	err = c.UpdateSource(ctx, sourceURL, classSource, lock.LockHandle, transport)
	if err != nil {
		result.Message = fmt.Sprintf("Failed to update class source: %v", err)
		return result, nil
	}

	// Step 4: Create test include
	err = c.CreateTestInclude(ctx, className, lock.LockHandle, transport)
	if err != nil {
		result.Message = fmt.Sprintf("Failed to create test include: %v", err)
		return result, nil
	}

	// Step 5: Update test include
	err = c.UpdateClassInclude(ctx, className, ClassIncludeTestClasses, testSource, lock.LockHandle, transport)
	if err != nil {
		result.Message = fmt.Sprintf("Failed to update test source: %v", err)
		return result, nil
	}

	// Step 6: Unlock
	err = c.UnlockObject(ctx, objectURL, lock.LockHandle)
	if err != nil {
		result.Message = fmt.Sprintf("Failed to unlock object: %v", err)
		return result, nil
	}

	// Step 7: Activate
	activation, err := c.Activate(ctx, objectURL, className)
	if err != nil {
		result.Message = fmt.Sprintf("Failed to activate: %v", err)
		result.Activation = activation
		return result, nil
	}
	result.Activation = activation

	if !activation.Success {
		result.Message = "Activation failed - check activation messages"
		return result, nil
	}

	// Step 8: Run unit tests
	flags := DefaultUnitTestFlags()
	testResult, err := c.RunUnitTests(ctx, objectURL, &flags)
	if err != nil {
		result.Message = fmt.Sprintf("Class activated but unit tests failed to run: %v", err)
		result.Success = true // Class was created successfully
		return result, nil
	}

	result.UnitTestResult = testResult
	result.Success = true
	result.Message = "Class created, activated, and unit tests executed successfully"

	return result, nil
}
// --- File-Based Deployment Workflows ---

// DeployResult contains the result of a file deployment operation.
type DeployResult struct {
	ObjectURL     string   `json:"objectUrl"`
	ObjectName    string   `json:"objectName"`
	ObjectType    string   `json:"objectType"`
	FilePath      string   `json:"filePath"`
	Success       bool     `json:"success"`
	Created       bool     `json:"created"` // true if created, false if updated
	SyntaxErrors  []string `json:"syntaxErrors,omitempty"`
	Errors        []string `json:"errors,omitempty"`
	Message       string   `json:"message,omitempty"`
}

// CreateFromFile creates a new ABAP object from a file and activates it.
//
// Workflow: Parse → Create → Lock → SyntaxCheck → Write → Unlock → Activate
//
// The function automatically detects the object type and name from the file extension
// and content. Supported file extensions: .clas.abap, .prog.abap, .intf.abap
//
// Example:
//   result, err := client.CreateFromFile(ctx, "/path/to/zcl_test.clas.abap", "$TMP", "")
func (c *Client) CreateFromFile(ctx context.Context, filePath, packageName, transport string) (*DeployResult, error) {
	// Safety check
	if err := c.checkSafety(OpCreate, "CreateFromFile"); err != nil {
		return nil, err
	}

	// 1. Parse file to detect type and name
	info, err := ParseABAPFile(filePath)
	if err != nil {
		return nil, fmt.Errorf("parsing file: %w", err)
	}

	// 2. Read source code
	sourceBytes, err := os.ReadFile(filePath)
	if err != nil {
		return nil, fmt.Errorf("reading file: %w", err)
	}
	source := string(sourceBytes)

	// 3. Create object
	err = c.CreateObject(ctx, CreateObjectOptions{
		ObjectType:  info.ObjectType,
		Name:        info.ObjectName,
		Description: info.Description,
		PackageName: packageName,
		Transport:   transport,
	})
	if err != nil {
		return &DeployResult{
			FilePath:   filePath,
			ObjectName: info.ObjectName,
			ObjectType: string(info.ObjectType),
			Success:    false,
			Errors:     []string{fmt.Sprintf("create failed: %v", err)},
			Message:    fmt.Sprintf("Failed to create %s %s", info.ObjectType, info.ObjectName),
		}, nil
	}

	// 4. Build object URL
	objectURL, err := c.buildObjectURL(info.ObjectType, info.ObjectName)
	if err != nil {
		return nil, err
	}

	// 5. Lock object
	lockResult, err := c.LockObject(ctx, objectURL, "MODIFY")
	if err != nil {
		return &DeployResult{
			FilePath:   filePath,
			ObjectURL:  objectURL,
			ObjectName: info.ObjectName,
			ObjectType: string(info.ObjectType),
			Success:    false,
			Errors:     []string{fmt.Sprintf("lock failed: %v", err)},
			Message:    fmt.Sprintf("Object created but failed to lock: %v", err),
		}, nil
	}

	// Ensure unlock on any error
	unlocked := false
	defer func() {
		if !unlocked {
			_ = c.UnlockObject(ctx, objectURL, lockResult.LockHandle)
		}
	}()

	// 6. Syntax check (optional pre-check)
	syntaxErrors, err := c.SyntaxCheck(ctx, objectURL, source)
	if err != nil {
		return &DeployResult{
			FilePath:   filePath,
			ObjectURL:  objectURL,
			ObjectName: info.ObjectName,
			ObjectType: string(info.ObjectType),
			Success:    false,
			Errors:     []string{fmt.Sprintf("syntax check failed: %v", err)},
			Message:    fmt.Sprintf("Object created but syntax check failed: %v", err),
		}, nil
	}

	if len(syntaxErrors) > 0 {
		// Convert syntax errors to strings
		errorMsgs := make([]string, len(syntaxErrors))
		for i, e := range syntaxErrors {
			errorMsgs[i] = fmt.Sprintf("Line %d: %s", e.Line, e.Text)
		}
		return &DeployResult{
			FilePath:     filePath,
			ObjectURL:    objectURL,
			ObjectName:   info.ObjectName,
			ObjectType:   string(info.ObjectType),
			Success:      false,
			SyntaxErrors: errorMsgs,
			Message:      fmt.Sprintf("Object created but has %d syntax errors", len(syntaxErrors)),
		}, nil
	}

	// 7. Write source
	err = c.UpdateSource(ctx, objectURL, source, lockResult.LockHandle, transport)
	if err != nil {
		return &DeployResult{
			FilePath:   filePath,
			ObjectURL:  objectURL,
			ObjectName: info.ObjectName,
			ObjectType: string(info.ObjectType),
			Success:    false,
			Errors:     []string{fmt.Sprintf("write source failed: %v", err)},
			Message:    fmt.Sprintf("Object created but failed to write source: %v", err),
		}, nil
	}

	// 8. Unlock
	err = c.UnlockObject(ctx, objectURL, lockResult.LockHandle)
	unlocked = true
	if err != nil {
		return &DeployResult{
			FilePath:   filePath,
			ObjectURL:  objectURL,
			ObjectName: info.ObjectName,
			ObjectType: string(info.ObjectType),
			Success:    false,
			Errors:     []string{fmt.Sprintf("unlock failed: %v", err)},
			Message:    fmt.Sprintf("Source written but failed to unlock: %v", err),
		}, nil
	}

	// 9. Activate
	_, err = c.Activate(ctx, objectURL, info.ObjectName)
	if err != nil {
		return &DeployResult{
			FilePath:   filePath,
			ObjectURL:  objectURL,
			ObjectName: info.ObjectName,
			ObjectType: string(info.ObjectType),
			Success:    false,
			Errors:     []string{fmt.Sprintf("activation failed: %v", err)},
			Message:    fmt.Sprintf("Source written but activation failed: %v", err),
		}, nil
	}

	return &DeployResult{
		FilePath:   filePath,
		ObjectURL:  objectURL,
		ObjectName: info.ObjectName,
		ObjectType: string(info.ObjectType),
		Success:    true,
		Created:    true,
		Message:    fmt.Sprintf("Successfully created and activated %s %s from %s", info.ObjectType, info.ObjectName, filePath),
	}, nil
}

// UpdateFromFile updates an existing ABAP object from a file.
//
// Workflow: Parse → Lock → SyntaxCheck → Write → Unlock → Activate
//
// Example:
//   result, err := client.UpdateFromFile(ctx, "/path/to/zcl_test.clas.abap", "")
func (c *Client) UpdateFromFile(ctx context.Context, filePath, transport string) (*DeployResult, error) {
	// Safety check
	if err := c.checkSafety(OpUpdate, "UpdateFromFile"); err != nil {
		return nil, err
	}

	// 1. Parse file to detect type and name
	info, err := ParseABAPFile(filePath)
	if err != nil {
		return nil, fmt.Errorf("parsing file: %w", err)
	}

	// 2. Read source code
	sourceBytes, err := os.ReadFile(filePath)
	if err != nil {
		return nil, fmt.Errorf("reading file: %w", err)
	}
	source := string(sourceBytes)

	// 3. Build object URL
	objectURL, err := c.buildObjectURL(info.ObjectType, info.ObjectName)
	if err != nil {
		return nil, err
	}

	// 4. Lock object
	lockResult, err := c.LockObject(ctx, objectURL, "MODIFY")
	if err != nil {
		return &DeployResult{
			FilePath:   filePath,
			ObjectURL:  objectURL,
			ObjectName: info.ObjectName,
			ObjectType: string(info.ObjectType),
			Success:    false,
			Errors:     []string{fmt.Sprintf("lock failed: %v", err)},
			Message:    fmt.Sprintf("Failed to lock object: %v", err),
		}, nil
	}

	// Ensure unlock on any error
	unlocked := false
	defer func() {
		if !unlocked {
			_ = c.UnlockObject(ctx, objectURL, lockResult.LockHandle)
		}
	}()

	// 5. Syntax check
	syntaxErrors, err := c.SyntaxCheck(ctx, objectURL, source)
	if err != nil {
		return &DeployResult{
			FilePath:   filePath,
			ObjectURL:  objectURL,
			ObjectName: info.ObjectName,
			ObjectType: string(info.ObjectType),
			Success:    false,
			Errors:     []string{fmt.Sprintf("syntax check failed: %v", err)},
			Message:    fmt.Sprintf("Syntax check failed: %v", err),
		}, nil
	}

	if len(syntaxErrors) > 0 {
		// Convert syntax errors to strings
		errorMsgs := make([]string, len(syntaxErrors))
		for i, e := range syntaxErrors {
			errorMsgs[i] = fmt.Sprintf("Line %d: %s", e.Line, e.Text)
		}
		return &DeployResult{
			FilePath:     filePath,
			ObjectURL:    objectURL,
			ObjectName:   info.ObjectName,
			ObjectType:   string(info.ObjectType),
			Success:      false,
			SyntaxErrors: errorMsgs,
			Message:      fmt.Sprintf("Source has %d syntax errors", len(syntaxErrors)),
		}, nil
	}

	// 6. Write source
	err = c.UpdateSource(ctx, objectURL, source, lockResult.LockHandle, transport)
	if err != nil {
		return &DeployResult{
			FilePath:   filePath,
			ObjectURL:  objectURL,
			ObjectName: info.ObjectName,
			ObjectType: string(info.ObjectType),
			Success:    false,
			Errors:     []string{fmt.Sprintf("write source failed: %v", err)},
			Message:    fmt.Sprintf("Failed to write source: %v", err),
		}, nil
	}

	// 7. Unlock
	err = c.UnlockObject(ctx, objectURL, lockResult.LockHandle)
	unlocked = true
	if err != nil {
		return &DeployResult{
			FilePath:   filePath,
			ObjectURL:  objectURL,
			ObjectName: info.ObjectName,
			ObjectType: string(info.ObjectType),
			Success:    false,
			Errors:     []string{fmt.Sprintf("unlock failed: %v", err)},
			Message:    fmt.Sprintf("Source written but failed to unlock: %v", err),
		}, nil
	}

	// 8. Activate
	_, err = c.Activate(ctx, objectURL, info.ObjectName)
	if err != nil {
		return &DeployResult{
			FilePath:   filePath,
			ObjectURL:  objectURL,
			ObjectName: info.ObjectName,
			ObjectType: string(info.ObjectType),
			Success:    false,
			Errors:     []string{fmt.Sprintf("activation failed: %v", err)},
			Message:    fmt.Sprintf("Source written but activation failed: %v", err),
		}, nil
	}

	return &DeployResult{
		FilePath:   filePath,
		ObjectURL:  objectURL,
		ObjectName: info.ObjectName,
		ObjectType: string(info.ObjectType),
		Success:    true,
		Created:    false,
		Message:    fmt.Sprintf("Successfully updated and activated %s %s from %s", info.ObjectType, info.ObjectName, filePath),
	}, nil
}

// DeployFromFile intelligently creates or updates an object from a file.
//
// Workflow: Parse → CheckExists → CreateFromFile OR UpdateFromFile
//
// This is the recommended method for deploying ABAP objects from files as it
// automatically determines whether to create a new object or update an existing one.
//
// Example:
//   result, err := client.DeployFromFile(ctx, "/path/to/zcl_test.clas.abap", "$TMP", "")
func (c *Client) DeployFromFile(ctx context.Context, filePath, packageName, transport string) (*DeployResult, error) {
	// 1. Parse file
	info, err := ParseABAPFile(filePath)
	if err != nil {
		return nil, fmt.Errorf("parsing file: %w", err)
	}

	// 2. Check if object exists
	objectURL, err := c.buildObjectURL(info.ObjectType, info.ObjectName)
	if err != nil {
		return nil, err
	}

	// Try to get object (if 404, doesn't exist)
	_, err = c.transport.Request(ctx, objectURL, &RequestOptions{
		Method: "GET",
		Accept: "text/plain",
	})

	if err != nil {
		// Object doesn't exist - create it
		return c.CreateFromFile(ctx, filePath, packageName, transport)
	}

	// Object exists - update it
	return c.UpdateFromFile(ctx, filePath, transport)
}

// buildObjectURL constructs the ADT URL for an object type and name
func (c *Client) buildObjectURL(objType CreatableObjectType, name string) (string, error) {
	name = strings.ToLower(name)
	switch objType {
	case ObjectTypeClass:
		return fmt.Sprintf("/sap/bc/adt/oo/classes/%s", name), nil
	case ObjectTypeProgram:
		return fmt.Sprintf("/sap/bc/adt/programs/programs/%s", name), nil
	case ObjectTypeInterface:
		return fmt.Sprintf("/sap/bc/adt/oo/interfaces/%s", name), nil
	case ObjectTypeFunctionGroup:
		return fmt.Sprintf("/sap/bc/adt/functions/groups/%s", name), nil
	case ObjectTypeInclude:
		return fmt.Sprintf("/sap/bc/adt/programs/includes/%s", name), nil
	default:
		return "", fmt.Errorf("unsupported object type for URL building: %s", objType)
	}
}

// --- Utility Workflows ---

// RenameObjectResult contains the result of renaming an object.
type RenameObjectResult struct {
	OldName    string `json:"oldName"`
	NewName    string `json:"newName"`
	ObjectType string `json:"objectType"`
	Success    bool   `json:"success"`
	Message    string `json:"message,omitempty"`
	Errors     []string `json:"errors,omitempty"`
}

// RenameObject renames an ABAP object by creating a copy with the new name and deleting the old one.
//
// Workflow: GetSource → CreateNew → ActivateNew → DeleteOld
//
// This is a destructive operation - use with caution!
func (c *Client) RenameObject(ctx context.Context, objType CreatableObjectType, oldName, newName, packageName, transport string) (*RenameObjectResult, error) {
	// Safety check
	if err := c.checkSafety(OpDelete, "RenameObject"); err != nil {
		return nil, err
	}

	result := &RenameObjectResult{
		OldName:    oldName,
		NewName:    newName,
		ObjectType: string(objType),
	}

	// 1. Get old object source
	oldURL, err := c.buildObjectURL(objType, oldName)
	if err != nil {
		return nil, err
	}

	resp, err := c.transport.Request(ctx, oldURL+"/source/main", &RequestOptions{
		Method: "GET",
		Accept: "text/plain",
	})
	if err != nil {
		result.Errors = append(result.Errors, fmt.Sprintf("Failed to read old object: %v", err))
		return result, nil
	}
	oldSource := string(resp.Body)

	// 2. Replace old name with new name in source
	newSource := strings.ReplaceAll(oldSource, strings.ToUpper(oldName), strings.ToUpper(newName))
	newSource = strings.ReplaceAll(newSource, strings.ToLower(oldName), strings.ToLower(newName))

	// 3. Create new object
	err = c.CreateObject(ctx, CreateObjectOptions{
		ObjectType:  objType,
		Name:        newName,
		Description: fmt.Sprintf("Renamed from %s", oldName),
		PackageName: packageName,
		Transport:   transport,
	})
	if err != nil {
		result.Errors = append(result.Errors, fmt.Sprintf("Failed to create new object: %v", err))
		return result, nil
	}

	// 4. Write source to new object
	newURL, _ := c.buildObjectURL(objType, newName)
	lockResult, err := c.LockObject(ctx, newURL, "MODIFY")
	if err != nil {
		result.Errors = append(result.Errors, fmt.Sprintf("Failed to lock new object: %v", err))
		return result, nil
	}

	defer func() {
		_ = c.UnlockObject(ctx, newURL, lockResult.LockHandle)
	}()

	err = c.UpdateSource(ctx, newURL, newSource, lockResult.LockHandle, transport)
	if err != nil {
		result.Errors = append(result.Errors, fmt.Sprintf("Failed to write source: %v", err))
		return result, nil
	}

	_ = c.UnlockObject(ctx, newURL, lockResult.LockHandle)

	// 5. Activate new object
	_, err = c.Activate(ctx, newURL, newName)
	if err != nil {
		result.Errors = append(result.Errors, fmt.Sprintf("Failed to activate new object: %v", err))
		return result, nil
	}

	// 6. Delete old object
	oldLockResult, err := c.LockObject(ctx, oldURL, "MODIFY")
	if err != nil {
		result.Message = fmt.Sprintf("New object %s created successfully, but failed to lock old object %s for deletion: %v. Please delete manually.", newName, oldName, err)
		result.Success = true
		return result, nil
	}

	err = c.DeleteObject(ctx, oldURL, oldLockResult.LockHandle, transport)
	if err != nil {
		result.Message = fmt.Sprintf("New object %s created successfully, but failed to delete old object %s: %v. Please delete manually.", newName, oldName, err)
		result.Success = true
		return result, nil
	}

	result.Success = true
	result.Message = fmt.Sprintf("Successfully renamed %s to %s", oldName, newName)
	return result, nil
}

// SaveToFileResult contains the result of saving an object to a file.
type SaveToFileResult struct {
	ObjectName string `json:"objectName"`
	ObjectType string `json:"objectType"`
	FilePath   string `json:"filePath"`
	LineCount  int    `json:"lineCount"`
	Success    bool   `json:"success"`
	Message    string `json:"message,omitempty"`
}

// SaveToFile saves an ABAP object's source code to a local file.
//
// Workflow: GetSource → WriteFile
//
// The file extension is automatically determined based on object type.
func (c *Client) SaveToFile(ctx context.Context, objType CreatableObjectType, objectName, outputPath string) (*SaveToFileResult, error) {
	result := &SaveToFileResult{
		ObjectName: objectName,
		ObjectType: string(objType),
	}

	// 1. Determine file extension
	var ext string
	switch objType {
	case ObjectTypeClass:
		ext = ".clas.abap"
	case ObjectTypeProgram:
		ext = ".prog.abap"
	case ObjectTypeInterface:
		ext = ".intf.abap"
	case ObjectTypeFunctionGroup:
		ext = ".fugr.abap"
	case ObjectTypeInclude:
		ext = ".abap"
	default:
		ext = ".abap"
	}

	// 2. Build file path
	if outputPath == "" {
		outputPath = "."
	}
	if !strings.HasSuffix(outputPath, ext) {
		// outputPath is a directory
		objectName = strings.ToLower(objectName)
		result.FilePath = filepath.Join(outputPath, objectName+ext)
	} else {
		result.FilePath = outputPath
	}

	// 3. Get object source
	objectURL, err := c.buildObjectURL(objType, objectName)
	if err != nil {
		return nil, err
	}

	resp, err := c.transport.Request(ctx, objectURL+"/source/main", &RequestOptions{
		Method: "GET",
		Accept: "text/plain",
	})
	if err != nil {
		result.Message = fmt.Sprintf("Failed to read object: %v", err)
		return result, nil
	}

	source := string(resp.Body)
	result.LineCount = len(strings.Split(source, "\n"))

	// 4. Write to file
	err = os.WriteFile(result.FilePath, []byte(source), 0644)
	if err != nil {
		result.Message = fmt.Sprintf("Failed to write file: %v", err)
		return result, nil
	}

	result.Success = true
	result.Message = fmt.Sprintf("Saved %s %s to %s (%d lines)", objType, objectName, result.FilePath, result.LineCount)
	return result, nil
}

// --- Surgical Edit Tools ---

// EditSourceResult represents the result of editing source code.
type EditSourceResult struct {
	Success       bool                `json:"success"`
	ObjectURL     string              `json:"objectUrl"`
	ObjectName    string              `json:"objectName"`
	MatchCount    int                 `json:"matchCount"`
	OldString     string              `json:"oldString,omitempty"`
	NewString     string              `json:"newString,omitempty"`
	SyntaxErrors  []string            `json:"syntaxErrors,omitempty"`
	Activation    *ActivationResult   `json:"activation,omitempty"`
	Message       string              `json:"message,omitempty"`
}

// countMatches counts occurrences of substring in s (case-sensitive or case-insensitive)
func countMatches(s, substr string, caseInsensitive bool) int {
	if !caseInsensitive {
		return strings.Count(s, substr)
	}
	// Case-insensitive count
	count := 0
	sLower := strings.ToLower(s)
	substrLower := strings.ToLower(substr)
	pos := 0
	for {
		idx := strings.Index(sLower[pos:], substrLower)
		if idx == -1 {
			break
		}
		count++
		pos += idx + len(substrLower)
	}
	return count
}

// replaceMatches replaces occurrences of old with new in s (case-sensitive or case-insensitive)
func replaceMatches(s, old, new string, replaceAll, caseInsensitive bool) string {
	if !caseInsensitive {
		if replaceAll {
			return strings.ReplaceAll(s, old, new)
		}
		return strings.Replace(s, old, new, 1)
	}
	// Case-insensitive replace
	sLower := strings.ToLower(s)
	oldLower := strings.ToLower(old)

	var result strings.Builder
	pos := 0
	replacements := 0

	for {
		idx := strings.Index(sLower[pos:], oldLower)
		if idx == -1 {
			result.WriteString(s[pos:])
			break
		}

		// Write everything before match
		result.WriteString(s[pos : pos+idx])
		// Write replacement
		result.WriteString(new)

		pos += idx + len(old)
		replacements++

		if !replaceAll && replacements >= 1 {
			result.WriteString(s[pos:])
			break
		}
	}

	return result.String()
}

// EditSource performs surgical string replacement on ABAP source code.
//
// This tool matches the Edit pattern used for local files, enabling surgical
// edits without downloading/uploading full source each time.
//
// Workflow: GetSource → FindReplace → SyntaxCheck → Lock → UpdateSource → Unlock → Activate
//
// Parameters:
//   - objectURL: ADT URL (e.g., /sap/bc/adt/programs/programs/ZTEST)
//   - oldString: Exact string to find (must be unique in source)
//   - newString: Replacement string
//   - replaceAll: If true, replace all occurrences; if false, require unique match
//   - syntaxCheck: If true, validate syntax before saving (default: true)
//   - caseInsensitive: If true, ignore case when matching (default: false)
//
// Example:
//   EditSource(ctx, "/sap/bc/adt/programs/programs/ZTEST",
//     "METHOD foo.\n  ENDMETHOD.",
//     "METHOD foo.\n  rv_result = 42.\n  ENDMETHOD.",
//     false, true, false)
func (c *Client) EditSource(ctx context.Context, objectURL, oldString, newString string, replaceAll, syntaxCheck, caseInsensitive bool) (*EditSourceResult, error) {
	// Safety check
	if err := c.checkSafety(OpUpdate, "EditSource"); err != nil {
		return nil, err
	}

	result := &EditSourceResult{
		ObjectURL: objectURL,
		OldString: oldString,
		NewString: newString,
	}

	// Extract object name from URL for error messages
	parts := strings.Split(objectURL, "/")
	if len(parts) > 0 {
		result.ObjectName = parts[len(parts)-1]
	}

	// 1. Get current source
	sourceURL := objectURL
	if !strings.HasSuffix(sourceURL, "/source/main") {
		sourceURL = objectURL + "/source/main"
	}

	resp, err := c.transport.Request(ctx, sourceURL, &RequestOptions{
		Method: "GET",
		Accept: "text/plain",
	})
	if err != nil {
		result.Message = fmt.Sprintf("Failed to read source: %v", err)
		return result, nil
	}
	source := string(resp.Body)

	// 2. Check match count
	matchCount := countMatches(source, oldString, caseInsensitive)
	result.MatchCount = matchCount

	if matchCount == 0 {
		if caseInsensitive {
			result.Message = "old_string not found in source (case-insensitive). Check for exact match (including whitespace, line breaks)."
		} else {
			result.Message = "old_string not found in source. Check for exact match (including whitespace, line breaks, case)."
		}
		return result, nil
	}

	if !replaceAll && matchCount > 1 {
		result.Message = fmt.Sprintf("old_string matches %d locations (not unique). Set replaceAll=true to replace all, or include more surrounding context to make match unique.", matchCount)
		return result, nil
	}

	// 3. Apply replacement
	newSource := replaceMatches(source, oldString, newString, replaceAll, caseInsensitive)

	// 4. Optional syntax check
	if syntaxCheck {
		syntaxErrors, err := c.SyntaxCheck(ctx, objectURL, newSource)
		if err != nil {
			result.Message = fmt.Sprintf("Syntax check failed: %v", err)
			return result, nil
		}

		if len(syntaxErrors) > 0 {
			// Convert to string messages
			errorMsgs := make([]string, len(syntaxErrors))
			for i, e := range syntaxErrors {
				errorMsgs[i] = fmt.Sprintf("Line %d: %s", e.Line, e.Text)
			}
			result.SyntaxErrors = errorMsgs
			result.Message = fmt.Sprintf("Edit would introduce %d syntax errors. Changes NOT saved.", len(syntaxErrors))
			return result, nil
		}
	}

	// 5. Lock object
	lockResult, err := c.LockObject(ctx, objectURL, "MODIFY")
	if err != nil {
		result.Message = fmt.Sprintf("Failed to lock object: %v", err)
		return result, nil
	}

	// Ensure unlock
	unlocked := false
	defer func() {
		if !unlocked {
			_ = c.UnlockObject(ctx, objectURL, lockResult.LockHandle)
		}
	}()

	// 6. Update source
	err = c.UpdateSource(ctx, sourceURL, newSource, lockResult.LockHandle, "")
	if err != nil {
		result.Message = fmt.Sprintf("Failed to update source: %v", err)
		return result, nil
	}

	// 7. Unlock
	err = c.UnlockObject(ctx, objectURL, lockResult.LockHandle)
	unlocked = true
	if err != nil {
		result.Message = fmt.Sprintf("Source updated but unlock failed: %v", err)
		return result, nil
	}

	// 8. Activate
	activation, err := c.Activate(ctx, objectURL, result.ObjectName)
	if err != nil {
		result.Message = fmt.Sprintf("Source updated but activation failed: %v", err)
		return result, nil
	}
	result.Activation = activation

	result.Success = true
	if replaceAll {
		result.Message = fmt.Sprintf("Successfully replaced %d occurrences and activated %s", matchCount, result.ObjectName)
	} else {
		result.Message = fmt.Sprintf("Successfully edited and activated %s", result.ObjectName)
	}
	return result, nil
}

// --- Grep/Search Tools ---

// GrepMatch represents a single match in a grep search.
type GrepMatch struct {
	LineNumber    int      `json:"lineNumber"`
	MatchedLine   string   `json:"matchedLine"`
	ContextBefore []string `json:"contextBefore,omitempty"`
	ContextAfter  []string `json:"contextAfter,omitempty"`
}

// GrepObjectResult represents the result of grepping a single ABAP object.
type GrepObjectResult struct {
	Success    bool        `json:"success"`
	ObjectURL  string      `json:"objectUrl"`
	ObjectName string      `json:"objectName"`
	ObjectType string      `json:"objectType,omitempty"`
	Matches    []GrepMatch `json:"matches"`
	MatchCount int         `json:"matchCount"`
	Message    string      `json:"message,omitempty"`
}

// GrepPackageResult represents the result of grepping an ABAP package.
type GrepPackageResult struct {
	Success     bool               `json:"success"`
	PackageName string             `json:"packageName"`
	Objects     []GrepObjectResult `json:"objects"`
	TotalMatches int               `json:"totalMatches"`
	Message     string             `json:"message,omitempty"`
}

// GrepObject searches for a regex pattern in a single ABAP object's source code.
//
// Parameters:
//   - objectURL: ADT URL of the object (e.g., /sap/bc/adt/programs/programs/ZTEST)
//   - pattern: Regular expression pattern (Go regexp syntax)
//   - caseInsensitive: If true, perform case-insensitive matching
//   - contextLines: Number of lines to include before/after each match (default: 0)
//
// Returns matches with line numbers and optional context lines.
func (c *Client) GrepObject(ctx context.Context, objectURL, pattern string, caseInsensitive bool, contextLines int) (*GrepObjectResult, error) {
	result := &GrepObjectResult{
		ObjectURL: objectURL,
		Matches:   []GrepMatch{},
	}

	// Extract object name from URL
	parts := strings.Split(objectURL, "/")
	if len(parts) > 0 {
		result.ObjectName = parts[len(parts)-1]
	}

	// Compile regex pattern
	regexPattern := pattern
	if caseInsensitive {
		regexPattern = "(?i)" + pattern
	}
	re, err := regexp.Compile(regexPattern)
	if err != nil {
		result.Message = fmt.Sprintf("Invalid regex pattern: %v", err)
		return result, nil
	}

	// Get source code
	sourceURL := objectURL
	if !strings.HasSuffix(sourceURL, "/source/main") {
		sourceURL = objectURL + "/source/main"
	}

	resp, err := c.transport.Request(ctx, sourceURL, &RequestOptions{
		Method: "GET",
		Accept: "text/plain",
	})
	if err != nil {
		result.Message = fmt.Sprintf("Failed to read source: %v", err)
		return result, nil
	}

	source := string(resp.Body)
	lines := strings.Split(source, "\n")

	// Search for matches
	for i, line := range lines {
		if re.MatchString(line) {
			match := GrepMatch{
				LineNumber:  i + 1, // 1-based line numbers
				MatchedLine: line,
			}

			// Add context lines before
			if contextLines > 0 {
				start := i - contextLines
				if start < 0 {
					start = 0
				}
				match.ContextBefore = lines[start:i]
			}

			// Add context lines after
			if contextLines > 0 {
				end := i + contextLines + 1
				if end > len(lines) {
					end = len(lines)
				}
				match.ContextAfter = lines[i+1 : end]
			}

			result.Matches = append(result.Matches, match)
		}
	}

	result.MatchCount = len(result.Matches)
	result.Success = true

	if result.MatchCount == 0 {
		result.Message = "No matches found"
	} else {
		result.Message = fmt.Sprintf("Found %d match(es) in %s", result.MatchCount, result.ObjectName)
	}

	return result, nil
}

// GrepPackage searches for a regex pattern across all objects in an ABAP package.
//
// Parameters:
//   - packageName: Name of the package (e.g., $TMP, ZPACKAGE)
//   - pattern: Regular expression pattern (Go regexp syntax)
//   - caseInsensitive: If true, perform case-insensitive matching
//   - objectTypes: Filter by object types (e.g., ["CLAS/OC", "PROG/P"]). Empty = search all.
//   - maxResults: Maximum number of matching objects to return (0 = unlimited)
//
// Returns matches grouped by object with match counts.
func (c *Client) GrepPackage(ctx context.Context, packageName, pattern string, caseInsensitive bool, objectTypes []string, maxResults int) (*GrepPackageResult, error) {
	result := &GrepPackageResult{
		PackageName: packageName,
		Objects:     []GrepObjectResult{},
	}

	// Get package contents
	packageContent, err := c.GetPackage(ctx, packageName)
	if err != nil {
		result.Message = fmt.Sprintf("Failed to read package: %v", err)
		return result, nil
	}

	// Build object type filter map
	typeFilter := make(map[string]bool)
	if len(objectTypes) > 0 {
		for _, t := range objectTypes {
			typeFilter[t] = true
		}
	}

	// Search each object in package
	objectsSearched := 0
	for _, obj := range packageContent.Objects {
		// Apply object type filter
		if len(typeFilter) > 0 && !typeFilter[obj.Type] {
			continue
		}

		// Skip non-source objects (tables, structures, etc.)
		if !isSourceObject(obj.Type) {
			continue
		}

		// Grep this object
		objResult, err := c.GrepObject(ctx, obj.URI, pattern, caseInsensitive, 0)
		if err != nil {
			continue // Skip objects that fail
		}

		// Only include objects with matches
		if objResult.MatchCount > 0 {
			objResult.ObjectType = obj.Type
			result.Objects = append(result.Objects, *objResult)
			result.TotalMatches += objResult.MatchCount

			// Check max results limit
			objectsSearched++
			if maxResults > 0 && objectsSearched >= maxResults {
				break
			}
		}
	}

	result.Success = true
	if result.TotalMatches == 0 {
		result.Message = "No matches found in package"
	} else {
		result.Message = fmt.Sprintf("Found %d match(es) across %d object(s) in package %s",
			result.TotalMatches, len(result.Objects), packageName)
	}

	return result, nil
}

// isSourceObject returns true if the object type contains source code that can be searched.
func isSourceObject(objectType string) bool {
	sourceTypes := map[string]bool{
		"PROG/P":  true, // Programs
		"CLAS/OC": true, // Classes
		"INTF/OI": true, // Interfaces
		"FUGR/F":  true, // Function groups
		"FUGR/FF": true, // Function modules
		"PROG/I":  true, // Includes
	}
	return sourceTypes[objectType]
}

// --- Unified Tools (Focused Mode) ---

// GetSourceOptions configures GetSource behavior
type GetSourceOptions struct {
	Parent  string // Function group name (required for FUNC type)
	Include string // Class include type: definitions, implementations, macros, testclasses (optional for CLAS type)
}

// GetSource is a unified tool for reading ABAP source code across different object types.
// Replaces GetProgram, GetClass, GetInterface, GetFunction, GetInclude, GetFunctionGroup, GetClassInclude.
//
// Supported types:
//   - PROG: Programs (name = program name)
//   - CLAS: Classes (name = class name, include = definitions|implementations|macros|testclasses)
//   - INTF: Interfaces (name = interface name)
//   - FUNC: Function modules (name = function module name, parent = function group name)
//   - FUGR: Function groups (name = function group name)
//   - INCL: Includes (name = include name)
func (c *Client) GetSource(ctx context.Context, objectType, name string, opts *GetSourceOptions) (string, error) {
	// Safety check for read operations
	if err := c.checkSafety(OpRead, "GetSource"); err != nil {
		return "", err
	}

	if opts == nil {
		opts = &GetSourceOptions{}
	}

	objectType = strings.ToUpper(objectType)
	name = strings.ToUpper(name)

	switch objectType {
	case "PROG":
		return c.GetProgram(ctx, name)

	case "CLAS":
		if opts.Include != "" {
			return c.GetClassInclude(ctx, name, ClassIncludeType(opts.Include))
		}
		return c.GetClassSource(ctx, name)

	case "INTF":
		return c.GetInterface(ctx, name)

	case "FUNC":
		if opts.Parent == "" {
			return "", fmt.Errorf("parent (function group name) is required for FUNC type")
		}
		return c.GetFunction(ctx, name, opts.Parent)

	case "FUGR":
		// GetFunctionGroup returns JSON metadata (function module list), not source
		fg, err := c.GetFunctionGroup(ctx, name)
		if err != nil {
			return "", err
		}
		// Serialize to JSON for display
		data, err := json.Marshal(fg)
		if err != nil {
			return "", fmt.Errorf("failed to serialize function group: %w", err)
		}
		return string(data), nil

	case "INCL":
		return c.GetInclude(ctx, name)

	default:
		return "", fmt.Errorf("unsupported object type: %s (supported: PROG, CLAS, INTF, FUNC, FUGR, INCL)", objectType)
	}
}

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
}

// WriteSourceResult represents the result of WriteSource operation
type WriteSourceResult struct {
	Success       bool                       `json:"success"`
	ObjectType    string                     `json:"objectType"`
	ObjectName    string                     `json:"objectName"`
	ObjectURL     string                     `json:"objectUrl"`
	Mode          string                     `json:"mode"` // "created" or "updated"
	SyntaxErrors  []SyntaxCheckResult        `json:"syntaxErrors,omitempty"`
	Activation    *ActivationResult          `json:"activation,omitempty"`
	TestResults   *UnitTestResult            `json:"testResults,omitempty"` // For CLAS with TestSource
	Message       string                     `json:"message,omitempty"`
}

// WriteSource is a unified tool for writing ABAP source code across different object types.
// Replaces WriteProgram, WriteClass, CreateAndActivateProgram, CreateClassWithTests.
//
// Supported types:
//   - PROG: Programs
//   - CLAS: Classes (optionally with test source)
//   - INTF: Interfaces
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

	// Validate object type
	switch objectType {
	case "PROG", "CLAS", "INTF":
		// Supported types
	default:
		result.Message = fmt.Sprintf("Unsupported object type: %s (supported: PROG, CLAS, INTF)", objectType)
		return result, nil
	}

	// Determine if object exists (for upsert mode)
	objectExists := false
	if opts.Mode == WriteModeUpsert {
		// Try to check if object exists
		switch objectType {
		case "PROG":
			_, err := c.GetProgram(ctx, name)
			objectExists = (err == nil)
		case "CLAS":
			_, err := c.GetClass(ctx, name)
			objectExists = (err == nil)
		case "INTF":
			_, err := c.GetInterface(ctx, name)
			objectExists = (err == nil)
		}
	}

	// Determine actual operation mode
	var actualMode WriteSourceMode
	if opts.Mode == WriteModeUpsert {
		if objectExists {
			actualMode = WriteModeUpdate
		} else {
			actualMode = WriteModeCreate
		}
	} else {
		actualMode = opts.Mode
	}

	// Validate mode vs existence
	if actualMode == WriteModeCreate && objectExists {
		result.Message = fmt.Sprintf("Object %s already exists (use mode=update or mode=upsert)", name)
		return result, nil
	}
	if actualMode == WriteModeUpdate && !objectExists {
		result.Message = fmt.Sprintf("Object %s does not exist (use mode=create or mode=upsert)", name)
		return result, nil
	}

	// Execute create or update workflow
	if actualMode == WriteModeCreate {
		return c.writeSourceCreate(ctx, objectType, name, source, opts)
	} else {
		return c.writeSourceUpdate(ctx, objectType, name, source, opts)
	}
}

// writeSourceCreate handles creation workflow
func (c *Client) writeSourceCreate(ctx context.Context, objectType, name, source string, opts *WriteSourceOptions) (*WriteSourceResult, error) {
	result := &WriteSourceResult{
		ObjectType: objectType,
		ObjectName: name,
		Mode:       "created",
	}

	// Validate required fields for create
	if opts.Package == "" {
		result.Message = "Package is required for creating new objects"
		return result, nil
	}
	if opts.Description == "" {
		result.Message = "Description is required for creating new objects"
		return result, nil
	}

	// Use existing Create*AndActivate* workflows
	switch objectType {
	case "PROG":
		progResult, err := c.CreateAndActivateProgram(ctx, name, opts.Description, opts.Package, source, opts.Transport)
		if err != nil {
			result.Message = fmt.Sprintf("Failed to create program: %v", err)
			return result, nil
		}
		result.Success = progResult.Success
		result.ObjectURL = progResult.ObjectURL
		result.SyntaxErrors = progResult.SyntaxErrors
		result.Activation = progResult.Activation
		result.Message = progResult.Message
		return result, nil

	case "CLAS":
		if opts.TestSource != "" {
			classResult, err := c.CreateClassWithTests(ctx, name, opts.Description, opts.Package, source, opts.TestSource, opts.Transport)
			if err != nil {
				result.Message = fmt.Sprintf("Failed to create class with tests: %v", err)
				return result, nil
			}
			result.Success = classResult.Success
			result.ObjectURL = classResult.ObjectURL
			result.Activation = classResult.Activation
			result.TestResults = classResult.UnitTestResult
			result.Message = classResult.Message
			return result, nil
		} else {
			// Create class without tests - use CreateObject + WriteClass workflow
			objectURL := fmt.Sprintf("/sap/bc/adt/oo/classes/%s", name)
			result.ObjectURL = objectURL

			// Create object
			err := c.CreateObject(ctx, CreateObjectOptions{
				ObjectType:  ObjectTypeClass,
				Name:        name,
				Description: opts.Description,
				PackageName: opts.Package,
				Transport:   opts.Transport,
			})
			if err != nil {
				result.Message = fmt.Sprintf("Failed to create class: %v", err)
				return result, nil
			}

			// Write source
			writeResult, err := c.WriteClass(ctx, name, source, opts.Transport)
			if err != nil {
				result.Message = fmt.Sprintf("Class created but failed to write source: %v", err)
				return result, nil
			}

			result.Success = writeResult.Success
			result.SyntaxErrors = writeResult.SyntaxErrors
			result.Activation = writeResult.Activation
			result.Message = writeResult.Message
			return result, nil
		}

	case "INTF":
		objectURL := fmt.Sprintf("/sap/bc/adt/oo/interfaces/%s", name)
		result.ObjectURL = objectURL

		// Create object
		err := c.CreateObject(ctx, CreateObjectOptions{
			ObjectType:  ObjectTypeInterface,
			Name:        name,
			Description: opts.Description,
			PackageName: opts.Package,
			Transport:   opts.Transport,
		})
		if err != nil {
			result.Message = fmt.Sprintf("Failed to create interface: %v", err)
			return result, nil
		}

		// Write source (using WriteProgram logic for interface)
		sourceURL := objectURL + "/source/main"

		// Syntax check
		syntaxErrors, err := c.SyntaxCheck(ctx, objectURL, source)
		if err != nil {
			result.Message = fmt.Sprintf("Syntax check failed: %v", err)
			return result, nil
		}

		// Check for syntax errors
		for _, se := range syntaxErrors {
			if se.Severity == "E" || se.Severity == "A" || se.Severity == "X" {
				result.SyntaxErrors = syntaxErrors
				result.Message = "Source has syntax errors - not saved"
				return result, nil
			}
		}
		result.SyntaxErrors = syntaxErrors

		// Lock
		lock, err := c.LockObject(ctx, objectURL, "MODIFY")
		if err != nil {
			result.Message = fmt.Sprintf("Failed to lock object: %v", err)
			return result, nil
		}

		defer func() {
			if !result.Success {
				c.UnlockObject(ctx, objectURL, lock.LockHandle)
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
			result.Message = "Interface created and activated successfully"
		} else {
			result.Message = "Activation failed - check activation messages"
		}

		return result, nil

	default:
		result.Message = fmt.Sprintf("Unsupported object type for creation: %s", objectType)
		return result, nil
	}
}

// writeSourceUpdate handles update workflow
func (c *Client) writeSourceUpdate(ctx context.Context, objectType, name, source string, opts *WriteSourceOptions) (*WriteSourceResult, error) {
	result := &WriteSourceResult{
		ObjectType: objectType,
		ObjectName: name,
		Mode:       "updated",
	}

	// Use existing Write* workflows
	switch objectType {
	case "PROG":
		progResult, err := c.WriteProgram(ctx, name, source, opts.Transport)
		if err != nil {
			result.Message = fmt.Sprintf("Failed to update program: %v", err)
			return result, nil
		}
		result.Success = progResult.Success
		result.ObjectURL = progResult.ObjectURL
		result.SyntaxErrors = progResult.SyntaxErrors
		result.Activation = progResult.Activation
		result.Message = progResult.Message
		return result, nil

	case "CLAS":
		classResult, err := c.WriteClass(ctx, name, source, opts.Transport)
		if err != nil {
			result.Message = fmt.Sprintf("Failed to update class: %v", err)
			return result, nil
		}
		result.Success = classResult.Success
		result.ObjectURL = classResult.ObjectURL
		result.SyntaxErrors = classResult.SyntaxErrors
		result.Activation = classResult.Activation
		result.Message = classResult.Message

		// If test source provided, update test include
		if opts.TestSource != "" {
			objectURL := fmt.Sprintf("/sap/bc/adt/oo/classes/%s", name)

			// Lock for test update
			lock, err := c.LockObject(ctx, objectURL, "MODIFY")
			if err != nil {
				result.Message += fmt.Sprintf(" (Warning: Failed to lock for test update: %v)", err)
				return result, nil
			}

			// Update test include
			err = c.UpdateClassInclude(ctx, name, "testclasses", opts.TestSource, lock.LockHandle, opts.Transport)
			unlockErr := c.UnlockObject(ctx, objectURL, lock.LockHandle)
			if err != nil {
				result.Message += fmt.Sprintf(" (Warning: Failed to update test include: %v)", err)
				return result, nil
			}
			if unlockErr != nil {
				result.Message += fmt.Sprintf(" (Warning: Failed to unlock after test update: %v)", unlockErr)
			}

			// Run tests
			testResult, err := c.RunUnitTests(ctx, objectURL, nil)
			if err == nil {
				result.TestResults = testResult
			}
		}

		return result, nil

	case "INTF":
		// Similar to WriteProgram workflow
		objectURL := fmt.Sprintf("/sap/bc/adt/oo/interfaces/%s", name)
		sourceURL := objectURL + "/source/main"
		result.ObjectURL = objectURL

		// Syntax check
		syntaxErrors, err := c.SyntaxCheck(ctx, objectURL, source)
		if err != nil {
			result.Message = fmt.Sprintf("Syntax check failed: %v", err)
			return result, nil
		}

		for _, se := range syntaxErrors {
			if se.Severity == "E" || se.Severity == "A" || se.Severity == "X" {
				result.SyntaxErrors = syntaxErrors
				result.Message = "Source has syntax errors - not saved"
				return result, nil
			}
		}
		result.SyntaxErrors = syntaxErrors

		// Lock
		lock, err := c.LockObject(ctx, objectURL, "MODIFY")
		if err != nil {
			result.Message = fmt.Sprintf("Failed to lock object: %v", err)
			return result, nil
		}

		defer func() {
			if !result.Success {
				c.UnlockObject(ctx, objectURL, lock.LockHandle)
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
			result.Message = "Interface updated and activated successfully"
		} else {
			result.Message = "Activation failed - check activation messages"
		}

		return result, nil

	default:
		result.Message = fmt.Sprintf("Unsupported object type for update: %s", objectType)
		return result, nil
	}
}
