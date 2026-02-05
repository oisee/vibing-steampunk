package adt

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
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
	objectURL := fmt.Sprintf("/sap/bc/adt/programs/programs/%s", url.PathEscape(programName))
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
	objectURL := fmt.Sprintf("/sap/bc/adt/oo/classes/%s", url.PathEscape(className))
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

	objectURL := fmt.Sprintf("/sap/bc/adt/programs/programs/%s", url.PathEscape(programName))
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

	objectURL := fmt.Sprintf("/sap/bc/adt/oo/classes/%s", url.PathEscape(className))
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
		ParentName:  info.ParentName, // For function modules: function group name
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
	objectURL, err := c.buildObjectURLWithParent(info.ObjectType, info.ObjectName, info.ParentName)
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

	// 7. Write source (need source URL, not object URL)
	sourceURL, err := c.buildSourceURL(info.ObjectType, info.ObjectName)
	if err != nil {
		return nil, err
	}
	err = c.UpdateSource(ctx, sourceURL, source, lockResult.LockHandle, transport)
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

	// Check if this is a class include (testclasses, locals_def, etc.)
	isClassInclude := info.ObjectType == ObjectTypeClass &&
		info.ClassIncludeType != "" &&
		info.ClassIncludeType != ClassIncludeMain

	// 2. Read source code
	sourceBytes, err := os.ReadFile(filePath)
	if err != nil {
		return nil, fmt.Errorf("reading file: %w", err)
	}
	source := string(sourceBytes)

	// 3. Build object URL (for class includes, this is the parent class URL)
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

	// 5. Syntax check (skip for class includes - will check after update)
	if !isClassInclude {
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
	}

	// 6. Write source
	if isClassInclude {
		// For class includes, use UpdateClassInclude
		// First try to update - if fails with 404, create the include first
		err = c.UpdateClassInclude(ctx, info.ObjectName, info.ClassIncludeType, source, lockResult.LockHandle, transport)
		if err != nil {
			// Try to create the include first (for testclasses)
			if info.ClassIncludeType == ClassIncludeTestClasses {
				createErr := c.CreateTestInclude(ctx, info.ObjectName, lockResult.LockHandle, transport)
				if createErr == nil {
					// Retry update
					err = c.UpdateClassInclude(ctx, info.ObjectName, info.ClassIncludeType, source, lockResult.LockHandle, transport)
				}
			}
		}
		if err != nil {
			return &DeployResult{
				FilePath:   filePath,
				ObjectURL:  objectURL,
				ObjectName: info.ObjectName,
				ObjectType: fmt.Sprintf("%s.%s", info.ObjectType, info.ClassIncludeType),
				Success:    false,
				Errors:     []string{fmt.Sprintf("write class include failed: %v", err)},
				Message:    fmt.Sprintf("Failed to write class include: %v", err),
			}, nil
		}
	} else {
		// Regular source update
		sourceURL, err := c.buildSourceURL(info.ObjectType, info.ObjectName)
		if err != nil {
			return nil, err
		}
		err = c.UpdateSource(ctx, sourceURL, source, lockResult.LockHandle, transport)
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

	// Build result message
	objTypeStr := string(info.ObjectType)
	if isClassInclude {
		objTypeStr = fmt.Sprintf("%s.%s", info.ObjectType, info.ClassIncludeType)
	}

	return &DeployResult{
		FilePath:   filePath,
		ObjectURL:  objectURL,
		ObjectName: info.ObjectName,
		ObjectType: objTypeStr,
		Success:    true,
		Created:    false,
		Message:    fmt.Sprintf("Successfully updated and activated %s %s from %s", objTypeStr, info.ObjectName, filePath),
	}, nil
}

// DeployFromFile intelligently creates or updates an object from a file.
//
// Workflow: Parse → CheckExists → CreateFromFile OR UpdateFromFile
//
// This is the recommended method for deploying ABAP objects from files as it
// automatically determines whether to create a new object or update an existing one.
//
// Supports class includes (.clas.testclasses.abap, .clas.locals_def.abap, etc.)
// For class includes, the parent class must already exist.
//
// Example:
//   result, err := client.DeployFromFile(ctx, "/path/to/zcl_test.clas.abap", "$TMP", "")
//   result, err := client.DeployFromFile(ctx, "/path/to/zcl_test.clas.testclasses.abap", "$TMP", "")
func (c *Client) DeployFromFile(ctx context.Context, filePath, packageName, transport string) (*DeployResult, error) {
	// 1. Parse file
	info, err := ParseABAPFile(filePath)
	if err != nil {
		return nil, fmt.Errorf("parsing file: %w", err)
	}

	// Check if this is a class include
	isClassInclude := info.ObjectType == ObjectTypeClass &&
		info.ClassIncludeType != "" &&
		info.ClassIncludeType != ClassIncludeMain

	// Check if this is a function module (requires parent function group)
	isFunctionModule := info.ObjectType == ObjectTypeFunctionMod

	// 2. Check if object exists
	objectURL, err := c.buildObjectURLWithParent(info.ObjectType, info.ObjectName, info.ParentName)
	if err != nil {
		if isFunctionModule && info.ParentName == "" {
			return nil, fmt.Errorf("function module file must follow pattern: {fugr_name}.fugr.{func_name}.func.abap")
		}
		return nil, err
	}

	// Try to get object (if 404, doesn't exist)
	_, err = c.transport.Request(ctx, objectURL, &RequestOptions{
		Method: "GET",
		Accept: "text/plain",
	})

	if err != nil {
		// Check if it's specifically a 404 Not Found error
		if IsNotFoundError(err) {
			// Object doesn't exist
			if isClassInclude {
				// For class includes, the parent class must exist
				return &DeployResult{
					FilePath:   filePath,
					ObjectURL:  objectURL,
					ObjectName: info.ObjectName,
					ObjectType: fmt.Sprintf("%s.%s", info.ObjectType, info.ClassIncludeType),
					Success:    false,
					Errors:     []string{"parent class does not exist"},
					Message:    fmt.Sprintf("Cannot deploy class include: parent class %s does not exist. Create the class first.", info.ObjectName),
				}, nil
			}
			// Regular object - create it
			return c.CreateFromFile(ctx, filePath, packageName, transport)
		}
		// For other errors (session timeout, network issues, etc.), proceed with update
		// The parent class might still exist - let UpdateFromFile handle it
	}

	// Object exists - update it (handles both regular objects and class includes)
	return c.UpdateFromFile(ctx, filePath, transport)
}

// buildObjectURL constructs the ADT URL for an object type and name
func (c *Client) buildObjectURL(objType CreatableObjectType, name string) (string, error) {
	return c.buildObjectURLWithParent(objType, name, "")
}

// buildObjectURLWithParent constructs the ADT URL for an object type with optional parent
func (c *Client) buildObjectURLWithParent(objType CreatableObjectType, name, parentName string) (string, error) {
	name = strings.ToLower(name)
	// URL encode to handle namespaced objects like /DMO/...
	encodedName := url.PathEscape(name)
	switch objType {
	case ObjectTypeClass:
		return fmt.Sprintf("/sap/bc/adt/oo/classes/%s", encodedName), nil
	case ObjectTypeProgram:
		return fmt.Sprintf("/sap/bc/adt/programs/programs/%s", encodedName), nil
	case ObjectTypeInterface:
		return fmt.Sprintf("/sap/bc/adt/oo/interfaces/%s", encodedName), nil
	case ObjectTypeFunctionGroup:
		return fmt.Sprintf("/sap/bc/adt/functions/groups/%s", encodedName), nil
	case ObjectTypeFunctionMod:
		if parentName == "" {
			return "", fmt.Errorf("function module requires parent function group name")
		}
		parentName = strings.ToLower(parentName)
		encodedParent := url.PathEscape(parentName)
		return fmt.Sprintf("/sap/bc/adt/functions/groups/%s/fmodules/%s", encodedParent, encodedName), nil
	case ObjectTypeInclude:
		return fmt.Sprintf("/sap/bc/adt/programs/includes/%s", encodedName), nil
	// RAP object types
	case ObjectTypeDDLS:
		return fmt.Sprintf("/sap/bc/adt/ddic/ddl/sources/%s", encodedName), nil
	case ObjectTypeBDEF:
		return fmt.Sprintf("/sap/bc/adt/bo/behaviordefinitions/%s", encodedName), nil
	case ObjectTypeSRVD:
		return fmt.Sprintf("/sap/bc/adt/ddic/srvd/sources/%s", encodedName), nil
	default:
		return "", fmt.Errorf("unsupported object type for URL building: %s", objType)
	}
}

// buildSourceURL constructs the source URL for an object (object URL + /source/main)
func (c *Client) buildSourceURL(objType CreatableObjectType, name string) (string, error) {
	objectURL, err := c.buildObjectURL(objType, name)
	if err != nil {
		return "", err
	}
	return objectURL + "/source/main", nil
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
	// RAP object types (using ABAPGit-compatible extensions)
	case ObjectTypeDDLS:
		ext = ".ddls.asddls"
	case ObjectTypeBDEF:
		ext = ".bdef.asbdef"
	case ObjectTypeSRVD:
		ext = ".srvd.srvdsrv"
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
		// Replace namespace slashes with # for filesystem compatibility (abapGit convention)
		safeFileName := strings.ReplaceAll(objectName, "/", "#")
		result.FilePath = filepath.Join(outputPath, safeFileName+ext)
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

// SaveClassIncludeToFile saves a class include's source code to a local file.
//
// Workflow: GetClassInclude → WriteFile
//
// The file extension is determined by the include type (abapGit-compatible):
//   - testclasses  → .clas.testclasses.abap
//   - definitions  → .clas.locals_def.abap
//   - implementations → .clas.locals_imp.abap
//   - macros       → .clas.macros.abap
//   - main         → .clas.abap
func (c *Client) SaveClassIncludeToFile(ctx context.Context, className string, includeType ClassIncludeType, outputPath string) (*SaveToFileResult, error) {
	result := &SaveToFileResult{
		ObjectName: className,
		ObjectType: fmt.Sprintf("CLAS.%s", includeType),
	}

	// 1. Determine file extension based on include type
	var ext string
	switch includeType {
	case ClassIncludeTestClasses:
		ext = ".clas.testclasses.abap"
	case ClassIncludeDefinitions:
		ext = ".clas.locals_def.abap"
	case ClassIncludeImplementations:
		ext = ".clas.locals_imp.abap"
	case ClassIncludeMacros:
		ext = ".clas.macros.abap"
	case ClassIncludeMain, "":
		ext = ".clas.abap"
	default:
		ext = ".clas.abap"
	}

	// 2. Build file path
	if outputPath == "" {
		outputPath = "."
	}
	if !strings.HasSuffix(outputPath, ext) {
		// outputPath is a directory
		className = strings.ToLower(className)
		// Replace namespace slashes with # for filesystem compatibility (abapGit convention)
		safeFileName := strings.ReplaceAll(className, "/", "#")
		result.FilePath = filepath.Join(outputPath, safeFileName+ext)
	} else {
		result.FilePath = outputPath
	}

	// 3. Get class include source
	source, err := c.GetClassInclude(ctx, className, includeType)
	if err != nil {
		result.Message = fmt.Sprintf("Failed to read class include: %v", err)
		return result, nil
	}

	result.LineCount = len(strings.Split(source, "\n"))

	// 4. Write to file
	err = os.WriteFile(result.FilePath, []byte(source), 0644)
	if err != nil {
		result.Message = fmt.Sprintf("Failed to write file: %v", err)
		return result, nil
	}

	result.Success = true
	result.Message = fmt.Sprintf("Saved %s %s.%s to %s (%d lines)", "CLAS", className, includeType, result.FilePath, result.LineCount)
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
	Method        string              `json:"method,omitempty"` // Method name if method-level edit
}

// EditSourceOptions provides optional parameters for EditSource.
type EditSourceOptions struct {
	ReplaceAll      bool   // If true, replace all occurrences; if false, require unique match
	SyntaxCheck     bool   // If true, validate syntax before saving (default: true if not set)
	CaseInsensitive bool   // If true, ignore case when matching
	Method          string // For CLAS only: constrain search/replace to this method only
	Transport       string // Transport request number (required for non-$TMP packages)
}

// normalizeLineEndings converts CRLF to LF for consistent matching
// SAP ADT returns source with \r\n but AI assistants typically send \n
func normalizeLineEndings(s string) string {
	return strings.ReplaceAll(s, "\r\n", "\n")
}

// countMatches counts occurrences of substring in s (case-sensitive or case-insensitive)
// Line endings are normalized (CRLF → LF) before comparison
func countMatches(s, substr string, caseInsensitive bool) int {
	// Normalize line endings - SAP uses CRLF, AI sends LF
	s = normalizeLineEndings(s)
	substr = normalizeLineEndings(substr)

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
// Line endings are normalized (CRLF → LF) for consistent matching
func replaceMatches(s, old, new string, replaceAll, caseInsensitive bool) string {
	// Normalize line endings - SAP uses CRLF, AI sends LF
	s = normalizeLineEndings(s)
	old = normalizeLineEndings(old)
	new = normalizeLineEndings(new)

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
// This is a backward-compatible wrapper for EditSourceWithOptions.
func (c *Client) EditSource(ctx context.Context, objectURL, oldString, newString string, replaceAll, syntaxCheck, caseInsensitive bool) (*EditSourceResult, error) {
	return c.EditSourceWithOptions(ctx, objectURL, oldString, newString, &EditSourceOptions{
		ReplaceAll:      replaceAll,
		SyntaxCheck:     syntaxCheck,
		CaseInsensitive: caseInsensitive,
	})
}

// EditSourceWithOptions performs surgical string replacement on ABAP source code with options.
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
//   - opts: Optional parameters (ReplaceAll, SyntaxCheck, CaseInsensitive, Method)
//
// Method-level isolation (CLAS only):
//   When opts.Method is set, the search is constrained to the specified method only.
//   This prevents accidental edits in other methods when the same pattern exists elsewhere.
//
// Example:
//   EditSourceWithOptions(ctx, "/sap/bc/adt/oo/classes/ZCL_TEST",
//     "METHOD foo.\n  ENDMETHOD.",
//     "METHOD foo.\n  rv_result = 42.\n  ENDMETHOD.",
//     &EditSourceOptions{Method: "FOO"})
func (c *Client) EditSourceWithOptions(ctx context.Context, objectURL, oldString, newString string, opts *EditSourceOptions) (*EditSourceResult, error) {
	// Safety check
	if err := c.checkSafety(OpUpdate, "EditSource"); err != nil {
		return nil, err
	}

	// Default options
	if opts == nil {
		opts = &EditSourceOptions{SyntaxCheck: true}
	}

	// Check if transportable edits are allowed when transport is specified
	if err := c.checkTransportableEdit(opts.Transport, "EditSource"); err != nil {
		return nil, err
	}
	// SyntaxCheck defaults to true if not explicitly set (zero value is false, so we need to handle this)
	// Note: caller should explicitly set SyntaxCheck=false if they don't want it

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

	// Detect if this is a class URL (not an include)
	isClass := strings.Contains(objectURL, "/sap/bc/adt/oo/classes/") && !strings.Contains(objectURL, "/includes/")
	var classNameForMethod string
	if isClass && opts.Method != "" {
		// Extract class name for method-level isolation
		classesPrefix := "/sap/bc/adt/oo/classes/"
		if idx := strings.Index(objectURL, classesPrefix); idx >= 0 {
			rest := objectURL[idx+len(classesPrefix):]
			if slashIdx := strings.Index(rest, "/"); slashIdx > 0 {
				classNameForMethod = rest[:slashIdx]
			} else {
				classNameForMethod = rest
			}
		}
		result.Method = opts.Method
	}

	// Detect if this is a class include (e.g., /sap/bc/adt/oo/classes/ZCL_FOO/includes/testclasses)
	isClassInclude := strings.Contains(objectURL, "/includes/")
	var className string
	var includeType ClassIncludeType
	var parentClassURL string

	if isClassInclude {
		// Parse class name and include type from URL
		// URL format: /sap/bc/adt/oo/classes/{class_name}/includes/{include_type}
		includesIdx := strings.Index(objectURL, "/includes/")
		if includesIdx > 0 {
			classesPrefix := "/sap/bc/adt/oo/classes/"
			if strings.Contains(objectURL, classesPrefix) {
				classStart := strings.Index(objectURL, classesPrefix) + len(classesPrefix)
				className = objectURL[classStart:includesIdx]
				includeType = ClassIncludeType(objectURL[includesIdx+len("/includes/"):])
				parentClassURL = objectURL[:includesIdx]
			}
		}
	}

	// 1. Get current source
	// For class includes, the source is accessed directly without /source/main suffix
	sourceURL := objectURL
	if !isClassInclude && !strings.HasSuffix(sourceURL, "/source/main") {
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

	// Method-level isolation: constrain search to the specified method only
	var methodStart, methodEnd int
	if classNameForMethod != "" && opts.Method != "" {
		methods, err := c.GetClassMethods(ctx, classNameForMethod)
		if err != nil {
			result.Message = fmt.Sprintf("Failed to get class methods: %v", err)
			return result, nil
		}

		// Find the specified method
		var foundMethod *MethodInfo
		methodNameUpper := strings.ToUpper(opts.Method)
		for i := range methods {
			if methods[i].Name == methodNameUpper {
				foundMethod = &methods[i]
				break
			}
		}
		if foundMethod == nil {
			result.Message = fmt.Sprintf("Method %s not found in class %s", opts.Method, classNameForMethod)
			return result, nil
		}

		methodStart = foundMethod.ImplementationStart
		methodEnd = foundMethod.ImplementationEnd

		// Extract method source for match counting
		sourceLines := strings.Split(source, "\n")
		if methodEnd > len(sourceLines) {
			methodEnd = len(sourceLines)
		}
		if methodStart < 1 {
			methodStart = 1
		}
		methodSource := strings.Join(sourceLines[methodStart-1:methodEnd], "\n")

		// Count matches in method source only
		matchCount := countMatches(methodSource, oldString, opts.CaseInsensitive)
		result.MatchCount = matchCount

		if matchCount == 0 {
			if opts.CaseInsensitive {
				result.Message = fmt.Sprintf("old_string not found in method %s (case-insensitive). Check for exact match.", opts.Method)
			} else {
				result.Message = fmt.Sprintf("old_string not found in method %s. Check for exact match.", opts.Method)
			}
			return result, nil
		}

		if !opts.ReplaceAll && matchCount > 1 {
			result.Message = fmt.Sprintf("old_string matches %d locations in method %s (not unique). Set replaceAll=true or include more context.", matchCount, opts.Method)
			return result, nil
		}

		// Apply replacement only within method boundaries
		newMethodSource := replaceMatches(methodSource, oldString, newString, opts.ReplaceAll, opts.CaseInsensitive)

		// Reconstruct full source with the edited method
		var newSourceLines []string
		newSourceLines = append(newSourceLines, sourceLines[:methodStart-1]...)
		newSourceLines = append(newSourceLines, strings.Split(newMethodSource, "\n")...)
		newSourceLines = append(newSourceLines, sourceLines[methodEnd:]...)
		source = strings.Join(newSourceLines, "\n")
	} else {
		// Non-method edit: check match count in full source
		matchCount := countMatches(source, oldString, opts.CaseInsensitive)
		result.MatchCount = matchCount

		if matchCount == 0 {
			if opts.CaseInsensitive {
				result.Message = "old_string not found in source (case-insensitive). Check for exact match (including whitespace, line breaks)."
			} else {
				result.Message = "old_string not found in source. Check for exact match (including whitespace, line breaks, case)."
			}
			return result, nil
		}

		if !opts.ReplaceAll && matchCount > 1 {
			result.Message = fmt.Sprintf("old_string matches %d locations (not unique). Set replaceAll=true to replace all, or include more surrounding context to make match unique.", matchCount)
			return result, nil
		}

		// Apply replacement
		source = replaceMatches(source, oldString, newString, opts.ReplaceAll, opts.CaseInsensitive)
	}

	newSource := source

	// 4. Optional syntax check
	if opts.SyntaxCheck {
		// For class includes, pass the include URL directly - SyntaxCheck handles it
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

	// 5. Lock object (for class includes, lock the parent class)
	lockURL := objectURL
	if isClassInclude && parentClassURL != "" {
		lockURL = parentClassURL
	}
	lockResult, err := c.LockObject(ctx, lockURL, "MODIFY")
	if err != nil {
		result.Message = fmt.Sprintf("Failed to lock object: %v", err)
		return result, nil
	}

	// Ensure unlock
	unlocked := false
	defer func() {
		if !unlocked {
			_ = c.UnlockObject(ctx, lockURL, lockResult.LockHandle)
		}
	}()

	// 6. Update source
	if isClassInclude && className != "" {
		// Use UpdateClassInclude for class includes
		err = c.UpdateClassInclude(ctx, className, includeType, newSource, lockResult.LockHandle, opts.Transport)
	} else {
		err = c.UpdateSource(ctx, sourceURL, newSource, lockResult.LockHandle, opts.Transport)
	}
	if err != nil {
		result.Message = fmt.Sprintf("Failed to update source: %v", err)
		return result, nil
	}

	// 7. Unlock
	err = c.UnlockObject(ctx, lockURL, lockResult.LockHandle)
	unlocked = true
	if err != nil {
		result.Message = fmt.Sprintf("Source updated but unlock failed: %v", err)
		return result, nil
	}

	// 8. Activate (for class includes, activate the parent class)
	activateURL := objectURL
	activateName := result.ObjectName
	if isClassInclude && parentClassURL != "" && className != "" {
		activateURL = parentClassURL
		activateName = className
	}
	activation, err := c.Activate(ctx, activateURL, activateName)
	if err != nil {
		result.Message = fmt.Sprintf("Source updated but activation failed: %v", err)
		return result, nil
	}
	result.Activation = activation

	result.Success = true
	if opts.Method != "" {
		result.Message = fmt.Sprintf("Successfully edited method %s and activated %s", opts.Method, result.ObjectName)
	} else if opts.ReplaceAll {
		result.Message = fmt.Sprintf("Successfully replaced %d occurrences and activated %s", result.MatchCount, result.ObjectName)
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

// GrepObjectsResult represents the result of grepping multiple ABAP objects.
type GrepObjectsResult struct {
	Success      bool               `json:"success"`
	Objects      []GrepObjectResult `json:"objects"`
	TotalMatches int                `json:"totalMatches"`
	Message      string             `json:"message,omitempty"`
}

// GrepObjects searches for a regex pattern in multiple ABAP objects' source code.
// This is a unified tool that handles both single and multiple object searches.
//
// Parameters:
//   - objectURLs: Array of ADT URLs (e.g., ["/sap/bc/adt/programs/programs/ZTEST"])
//   - pattern: Regular expression pattern (Go regexp syntax)
//   - caseInsensitive: If true, perform case-insensitive matching
//   - contextLines: Number of lines to include before/after each match (default: 0)
//
// Returns aggregated matches across all objects with per-object breakdown.
func (c *Client) GrepObjects(ctx context.Context, objectURLs []string, pattern string, caseInsensitive bool, contextLines int) (*GrepObjectsResult, error) {
	result := &GrepObjectsResult{
		Objects: []GrepObjectResult{},
	}

	if len(objectURLs) == 0 {
		result.Message = "No object URLs provided"
		return result, nil
	}

	// Search each object
	for _, objectURL := range objectURLs {
		objResult, err := c.GrepObject(ctx, objectURL, pattern, caseInsensitive, contextLines)
		if err != nil {
			// Log error but continue with other objects
			continue
		}

		// Only include objects with matches
		if objResult.MatchCount > 0 {
			result.Objects = append(result.Objects, *objResult)
			result.TotalMatches += objResult.MatchCount
		}
	}

	result.Success = true
	if result.TotalMatches == 0 {
		result.Message = fmt.Sprintf("No matches found in %d object(s)", len(objectURLs))
	} else {
		result.Message = fmt.Sprintf("Found %d match(es) across %d object(s)", result.TotalMatches, len(result.Objects))
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

// GrepPackagesResult represents the result of grepping multiple ABAP packages.
type GrepPackagesResult struct {
	Success      bool               `json:"success"`
	Packages     []string           `json:"packages"` // List of searched packages
	Objects      []GrepObjectResult `json:"objects"`
	TotalMatches int                `json:"totalMatches"`
	Message      string             `json:"message,omitempty"`
}

// GrepPackages searches for a regex pattern across multiple ABAP packages.
// This is a unified tool that handles single, multiple, and recursive package searches.
//
// Parameters:
//   - packages: Array of package names (e.g., ["$TMP"], ["$TMP", "ZLOCAL"])
//   - includeSubpackages: If true, recursively search all subpackages
//   - pattern: Regular expression pattern (Go regexp syntax)
//   - caseInsensitive: If true, perform case-insensitive matching
//   - objectTypes: Filter by object types (e.g., ["CLAS/OC", "PROG/P"]). Empty = search all.
//   - maxResults: Maximum number of matching objects to return (0 = unlimited)
//
// Returns aggregated matches across all packages with per-object breakdown.
func (c *Client) GrepPackages(ctx context.Context, packages []string, includeSubpackages bool, pattern string, caseInsensitive bool, objectTypes []string, maxResults int) (*GrepPackagesResult, error) {
	result := &GrepPackagesResult{
		Packages: []string{},
		Objects:  []GrepObjectResult{},
	}

	if len(packages) == 0 {
		result.Message = "No packages provided"
		return result, nil
	}

	// Collect all packages to search (including subpackages if requested)
	packagesToSearch := []string{}
	for _, pkg := range packages {
		if includeSubpackages {
			// Get package tree (including subpackages)
			subPackages, err := c.collectSubpackages(ctx, pkg)
			if err != nil {
				// If error getting subpackages, just search the main package
				packagesToSearch = append(packagesToSearch, pkg)
			} else {
				packagesToSearch = append(packagesToSearch, subPackages...)
			}
		} else {
			packagesToSearch = append(packagesToSearch, pkg)
		}
	}

	result.Packages = packagesToSearch

	// Search each package
	totalObjectsSearched := 0
	for _, packageName := range packagesToSearch {
		pkgResult, err := c.GrepPackage(ctx, packageName, pattern, caseInsensitive, objectTypes, maxResults-totalObjectsSearched)
		if err != nil {
			// Log error but continue with other packages
			continue
		}

		// Append results
		result.Objects = append(result.Objects, pkgResult.Objects...)
		result.TotalMatches += pkgResult.TotalMatches
		totalObjectsSearched += len(pkgResult.Objects)

		// Check if we've reached max results
		if maxResults > 0 && totalObjectsSearched >= maxResults {
			break
		}
	}

	result.Success = true
	if result.TotalMatches == 0 {
		result.Message = fmt.Sprintf("No matches found in %d package(s)", len(result.Packages))
	} else {
		result.Message = fmt.Sprintf("Found %d match(es) across %d object(s) in %d package(s)",
			result.TotalMatches, len(result.Objects), len(result.Packages))
	}

	return result, nil
}

// collectSubpackages recursively collects a package and all its subpackages.
func (c *Client) collectSubpackages(ctx context.Context, packageName string) ([]string, error) {
	packages := []string{packageName}

	// Get package contents
	content, err := c.GetPackage(ctx, packageName)
	if err != nil {
		return packages, err
	}

	// Check if package content has subpackages
	// PackageContent has a SubPackages field ([]string) if it exists
	if content.SubPackages != nil && len(content.SubPackages) > 0 {
		for _, subpkgName := range content.SubPackages {
			// Recursively collect subpackages
			subPackages, err := c.collectSubpackages(ctx, subpkgName)
			if err == nil {
				packages = append(packages, subPackages...)
			}
		}
	}

	return packages, nil
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
	Method  string // Method name for method-level source extraction (optional for CLAS type)
}

// GetSource is a unified tool for reading ABAP source code across different object types.
// Replaces GetProgram, GetClass, GetInterface, GetFunction, GetInclude, GetFunctionGroup, GetClassInclude.
//
// Supported types:
//   - PROG: Programs (name = program name)
//   - CLAS: Classes (name = class name, include = definitions|implementations|macros|testclasses, method = method name)
//   - INTF: Interfaces (name = interface name)
//   - FUNC: Function modules (name = function module name, parent = function group name)
//   - FUGR: Function groups (name = function group name)
//   - INCL: Includes (name = include name)
//   - DDLS: CDS DDL sources (name = DDL source name)
//   - VIEW: DDIC database views (name = view name) - classic SE11 views
//   - BDEF: Behavior Definitions (name = BDEF name) - RAP behavior implementation
//   - SRVD: Service Definitions (name = SRVD name) - RAP service exposure
//   - SRVB: Service Bindings (name = SRVB name) - RAP protocol binding (returns JSON metadata)
//   - MSAG: Message classes (name = message class name) - returns JSON with all messages
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
		// Method-level source extraction
		if opts.Method != "" {
			return c.GetClassMethodSource(ctx, name, opts.Method)
		}
		// Include-level source extraction
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

	case "DDLS":
		return c.GetDDLS(ctx, name)

	case "VIEW":
		return c.GetView(ctx, name)

	case "BDEF":
		return c.GetBDEF(ctx, name)

	case "SRVD":
		return c.GetSRVD(ctx, name)

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
		mc, err := c.GetMessageClass(ctx, name)
		if err != nil {
			return "", err
		}
		// Serialize to JSON for display
		data, err := json.Marshal(mc)
		if err != nil {
			return "", fmt.Errorf("failed to serialize message class: %w", err)
		}
		return string(data), nil

	default:
		return "", fmt.Errorf("unsupported object type: %s (supported: PROG, CLAS, INTF, FUNC, FUGR, INCL, DDLS, VIEW, BDEF, SRVD, SRVB, MSAG)", objectType)
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
	Method      string          // For CLAS only: update only this method (source must be METHOD...ENDMETHOD block)
}

// WriteSourceResult represents the result of WriteSource operation
type WriteSourceResult struct {
	Success       bool                       `json:"success"`
	ObjectType    string                     `json:"objectType"`
	ObjectName    string                     `json:"objectName"`
	ObjectURL     string                     `json:"objectUrl"`
	Mode          string                     `json:"mode"` // "created" or "updated"
	Method        string                     `json:"method,omitempty"` // Method name if method-level update
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

	// Check if transportable edits are allowed when transport is specified
	if err := c.checkTransportableEdit(opts.Transport, "WriteSource"); err != nil {
		return nil, err
	}

	objectType = strings.ToUpper(objectType)
	name = strings.ToUpper(name)

	result := &WriteSourceResult{
		ObjectType: objectType,
		ObjectName: name,
	}

	// Validate object type
	switch objectType {
	case "PROG", "CLAS", "INTF", "DDLS", "BDEF", "SRVD", "SRVB":
		// Supported types
	default:
		result.Message = fmt.Sprintf("Unsupported object type: %s (supported: PROG, CLAS, INTF, DDLS, BDEF, SRVD, SRVB)", objectType)
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
		case "DDLS":
			_, err := c.GetDDLS(ctx, name)
			objectExists = (err == nil)
		case "BDEF":
			_, err := c.GetBDEF(ctx, name)
			objectExists = (err == nil)
		case "SRVD":
			_, err := c.GetSRVD(ctx, name)
			objectExists = (err == nil)
		case "SRVB":
			_, err := c.GetSRVB(ctx, name)
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
			objectURL := fmt.Sprintf("/sap/bc/adt/oo/classes/%s", url.PathEscape(name))
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
		objectURL := fmt.Sprintf("/sap/bc/adt/oo/interfaces/%s", url.PathEscape(name))
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

	case "DDLS", "BDEF", "SRVD":
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
		}
		result.ObjectURL = objectURL
		sourceURL := objectURL + "/source/main"

		// Create object first
		// For BDEF, include source in creation (ADT API requirement)
		createOpts := CreateObjectOptions{
			ObjectType:  objType,
			Name:        name,
			Description: opts.Description,
			PackageName: opts.Package,
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
			sourceURL := objectURL + "/source/main"

			// Lock
			lock, err := c.LockObject(ctx, objectURL, "MODIFY")
			if err != nil {
				result.Message = fmt.Sprintf("Failed to lock BDEF: %v", err)
				return result, nil
			}

			// Update source
			err = c.UpdateSource(ctx, sourceURL, source, lock.LockHandle, opts.Transport)
			if err != nil {
				// Unlock on failure
				_ = c.UnlockObject(ctx, objectURL, lock.LockHandle)
				result.Message = fmt.Sprintf("Failed to update BDEF source: %v", err)
				return result, nil
			}

			// Unlock
			err = c.UnlockObject(ctx, objectURL, lock.LockHandle)
			if err != nil {
				result.Message = fmt.Sprintf("Failed to unlock BDEF: %v", err)
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

		// Syntax check (for DDLS, SRVD)
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
			result.Message = fmt.Sprintf("%s created and activated successfully", objectType)
		} else {
			result.Message = "Activation failed - check activation messages"
		}

		return result, nil

	case "SRVB":
		// SRVB (Service Binding) - source is JSON configuration
		// Parse JSON to get binding parameters
		var srvbConfig struct {
			ServiceDefName string `json:"serviceDefName"`
			BindingType    string `json:"bindingType"`    // ODATA
			BindingVersion string `json:"bindingVersion"` // V2 or V4
			BindingCategory string `json:"bindingCategory"` // 0=WebAPI, 1=UI
		}
		if err := json.Unmarshal([]byte(source), &srvbConfig); err != nil {
			result.Message = fmt.Sprintf("Invalid SRVB JSON config: %v (expected: {\"serviceDefName\":\"...\",\"bindingType\":\"ODATA\",\"bindingVersion\":\"V4\"})", err)
			return result, nil
		}

		// Validate required fields
		if srvbConfig.ServiceDefName == "" {
			result.Message = "serviceDefName is required in SRVB config"
			return result, nil
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

		objectURL := fmt.Sprintf("/sap/bc/adt/businessservices/bindings/%s", url.PathEscape(strings.ToLower(name)))
		result.ObjectURL = objectURL

		// Create SRVB
		err := c.CreateObject(ctx, CreateObjectOptions{
			ObjectType:        ObjectTypeSRVB,
			Name:              name,
			Description:       opts.Description,
			PackageName:       opts.Package,
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

	default:
		result.Message = fmt.Sprintf("Unsupported object type for creation: %s", objectType)
		return result, nil
	}
}

// --- Execute ABAP Code via Unit Test ---

// ExecuteABAPResult represents the result of executing ABAP code via unit test.
type ExecuteABAPResult struct {
	Success       bool     `json:"success"`
	ProgramName   string   `json:"programName"`
	Output        []string `json:"output"`        // Values returned via assertion messages
	RawAlerts     []UnitTestAlert `json:"rawAlerts,omitempty"` // Full alert details for debugging
	ExecutionTime float64  `json:"executionTime"` // Execution time in seconds
	Message       string   `json:"message,omitempty"`
	CleanedUp     bool     `json:"cleanedUp"`
}

// ExecuteABAPOptions configures ExecuteABAP behavior.
type ExecuteABAPOptions struct {
	// RiskLevel controls what operations the code can perform:
	// - "harmless" (default): No DB writes, no external calls
	// - "dangerous": Can write to DB, call external services
	// - "critical": Full system access (use with caution!)
	RiskLevel string

	// ReturnVariable is the name of the variable to return via assertion.
	// The code should set this variable, and its value will be returned.
	// If empty, uses "lv_result" by default.
	ReturnVariable string

	// KeepProgram prevents cleanup of the temp program (for debugging).
	KeepProgram bool

	// ProgramPrefix is the prefix for the temp program name.
	// Default is "ZTEMP_EXEC_".
	ProgramPrefix string
}

// ExecuteABAP executes arbitrary ABAP code via a temporary unit test wrapper.
//
// This is a powerful tool that allows executing any ABAP code on the SAP system.
// The code is wrapped in a test class and executed via RunUnitTests.
// Return values are extracted from assertion messages.
//
// Workflow:
// 1. Generate unique temp program name
// 2. Create program with test class wrapper
// 3. Inject user code into test method
// 4. Activate program
// 5. Run unit tests
// 6. Parse assertion messages for return values
// 7. Delete temp program (unless KeepProgram=true)
//
// Example:
//
//	result, err := client.ExecuteABAP(ctx, `
//	  DATA(lv_msg) = |Hello from SAP at { sy-datum } { sy-uzeit }|.
//	  DATA(lv_user) = sy-uname.
//	  lv_result = |{ lv_msg } by { lv_user }|.
//	`, nil)
//	// result.Output contains the assertion message with lv_result value
//
// Security: This is gated by OpWorkflow safety check.
func (c *Client) ExecuteABAP(ctx context.Context, code string, opts *ExecuteABAPOptions) (*ExecuteABAPResult, error) {
	// Safety check for workflow operations
	if err := c.checkSafety(OpWorkflow, "ExecuteABAP"); err != nil {
		return nil, err
	}

	if opts == nil {
		opts = &ExecuteABAPOptions{}
	}
	if opts.RiskLevel == "" {
		opts.RiskLevel = "harmless"
	}
	if opts.ReturnVariable == "" {
		opts.ReturnVariable = "lv_result"
	}
	if opts.ProgramPrefix == "" {
		opts.ProgramPrefix = "ZTEMP_EXEC_"
	}

	result := &ExecuteABAPResult{
		Output: []string{},
	}

	// Generate unique program name using timestamp
	timestamp := fmt.Sprintf("%d", time.Now().UnixNano()/1000000) // milliseconds
	programName := strings.ToUpper(opts.ProgramPrefix + timestamp[len(timestamp)-8:]) // Last 8 digits
	result.ProgramName = programName
	objectURL := fmt.Sprintf("/sap/bc/adt/programs/programs/%s", url.PathEscape(programName))

	// Build the test class wrapper source
	riskLevelABAP := "RISK LEVEL HARMLESS"
	switch strings.ToLower(opts.RiskLevel) {
	case "dangerous":
		riskLevelABAP = "RISK LEVEL DANGEROUS"
	case "critical":
		riskLevelABAP = "RISK LEVEL CRITICAL"
	}

	source := fmt.Sprintf(`REPORT %s.

*&---------------------------------------------------------------------*
*& Auto-generated program for code execution via unit test
*& Generated by vsp ExecuteABAP workflow
*&---------------------------------------------------------------------*

CLASS ltc_executor DEFINITION FOR TESTING %s DURATION SHORT.
  PUBLIC SECTION.
    METHODS execute_payload FOR TESTING.
ENDCLASS.

CLASS ltc_executor IMPLEMENTATION.
  METHOD execute_payload.
    DATA %s TYPE string.

    " === USER CODE START ===
%s
    " === USER CODE END ===

    " Return result via assertion message
    cl_abap_unit_assert=>fail( msg = |EXEC_RESULT:{ %s }| ).
  ENDMETHOD.
ENDCLASS.
`, programName, riskLevelABAP, opts.ReturnVariable, code, opts.ReturnVariable)

	// Step 1: Create the temp program
	err := c.CreateObject(ctx, CreateObjectOptions{
		ObjectType:  ObjectTypeProgram,
		Name:        programName,
		Description: "Temp program for ExecuteABAP",
		PackageName: "$TMP",
	})
	if err != nil {
		result.Message = fmt.Sprintf("Failed to create temp program: %v", err)
		return result, nil
	}

	// Ensure cleanup on any error (unless KeepProgram is set)
	defer func() {
		if !opts.KeepProgram {
			// Try to delete the program
			lock, lockErr := c.LockObject(ctx, objectURL, "MODIFY")
			if lockErr == nil {
				_ = c.DeleteObject(ctx, objectURL, lock.LockHandle, "")
				result.CleanedUp = true
			}
		}
	}()

	// Step 2: Lock and update source
	lock, err := c.LockObject(ctx, objectURL, "MODIFY")
	if err != nil {
		result.Message = fmt.Sprintf("Failed to lock temp program: %v", err)
		return result, nil
	}

	sourceURL := objectURL + "/source/main"
	err = c.UpdateSource(ctx, sourceURL, source, lock.LockHandle, "")
	if err != nil {
		_ = c.UnlockObject(ctx, objectURL, lock.LockHandle)
		result.Message = fmt.Sprintf("Failed to update source: %v", err)
		return result, nil
	}

	// Step 3: Unlock
	err = c.UnlockObject(ctx, objectURL, lock.LockHandle)
	if err != nil {
		result.Message = fmt.Sprintf("Failed to unlock: %v", err)
		return result, nil
	}

	// Step 4: Activate
	_, err = c.Activate(ctx, objectURL, programName)
	if err != nil {
		result.Message = fmt.Sprintf("Failed to activate: %v", err)
		return result, nil
	}

	// Step 5: Run unit tests
	flags := UnitTestRunFlags{
		Harmless:  true,
		Dangerous: strings.ToLower(opts.RiskLevel) == "dangerous" || strings.ToLower(opts.RiskLevel) == "critical",
		Critical:  strings.ToLower(opts.RiskLevel) == "critical",
		Short:     true,
		Medium:    true,
		Long:      false,
	}

	testResult, err := c.RunUnitTests(ctx, objectURL, &flags)
	if err != nil {
		result.Message = fmt.Sprintf("Failed to run unit tests: %v", err)
		return result, nil
	}

	// Step 6: Parse results - extract assertion messages
	for _, class := range testResult.Classes {
		for _, method := range class.TestMethods {
			result.ExecutionTime += method.ExecutionTime
			for _, alert := range method.Alerts {
				result.RawAlerts = append(result.RawAlerts, alert)

				// Look for our EXEC_RESULT marker in the alert title
				if strings.HasPrefix(alert.Title, "EXEC_RESULT:") {
					output := strings.TrimPrefix(alert.Title, "EXEC_RESULT:")
					result.Output = append(result.Output, output)
				}

				// Also check details for additional output
				for _, detail := range alert.Details {
					if strings.HasPrefix(detail, "EXEC_RESULT:") {
						output := strings.TrimPrefix(detail, "EXEC_RESULT:")
						result.Output = append(result.Output, output)
					}
				}
			}
		}
	}

	result.Success = true
	if len(result.Output) > 0 {
		result.Message = fmt.Sprintf("Executed successfully, %d output(s) returned", len(result.Output))
	} else {
		result.Message = "Executed successfully (no output captured)"
	}

	return result, nil
}

// ExecuteABAPMultiple executes ABAP code and returns multiple results via chained assertions.
// Each call to RETURN_VALUE( ) in the code adds a value to the output.
//
// Example:
//
//	result, err := client.ExecuteABAPMultiple(ctx, `
//	  SELECT * FROM t000 INTO TABLE @DATA(lt_clients) UP TO 5 ROWS.
//	  LOOP AT lt_clients INTO DATA(ls_client).
//	    RETURN_VALUE( |Client { ls_client-mandt }: { ls_client-mtext }| ).
//	  ENDLOOP.
//	`, nil)
//	// result.Output contains one entry per client
func (c *Client) ExecuteABAPMultiple(ctx context.Context, code string, opts *ExecuteABAPOptions) (*ExecuteABAPResult, error) {
	// Wrap the code with a macro that chains assertions
	wrappedCode := `
    DATA lt_exec_results TYPE string_table.

    DEFINE RETURN_VALUE.
      APPEND &1 TO lt_exec_results.
    END-OF-DEFINITION.

    ` + code + `

    " Output all collected results
    DATA lv_idx TYPE i.
    LOOP AT lt_exec_results INTO DATA(lv_exec_result).
      lv_idx = lv_idx + 1.
      cl_abap_unit_assert=>fail( msg = |EXEC_RESULT:{ lv_exec_result }| ).
    ENDLOOP.

    " Mark completion
    lv_result = |Completed with { lines( lt_exec_results ) } results|.
`

	return c.ExecuteABAP(ctx, wrappedCode, opts)
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
		// Method-level update: replace only the specified method
		if opts.Method != "" {
			methodResult, err := c.writeClassMethodUpdate(ctx, name, opts.Method, source, opts.Transport)
			if err != nil {
				result.Message = fmt.Sprintf("Failed to update method: %v", err)
				return result, nil
			}
			result.Success = methodResult.Success
			result.ObjectURL = methodResult.ObjectURL
			result.Method = methodResult.Method
			result.SyntaxErrors = methodResult.SyntaxErrors
			result.Activation = methodResult.Activation
			result.Message = methodResult.Message
			return result, nil
		}

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
			objectURL := fmt.Sprintf("/sap/bc/adt/oo/classes/%s", url.PathEscape(name))

			// Lock for test update
			lock, err := c.LockObject(ctx, objectURL, "MODIFY")
			if err != nil {
				result.Message += fmt.Sprintf(" (Warning: Failed to lock for test update: %v)", err)
				return result, nil
			}

			// Update test include - try update first, create if it doesn't exist
			err = c.UpdateClassInclude(ctx, name, "testclasses", opts.TestSource, lock.LockHandle, opts.Transport)
			if err != nil {
				// Try to create the test include first (it may not exist)
				createErr := c.CreateTestInclude(ctx, name, lock.LockHandle, opts.Transport)
				if createErr == nil {
					// Retry update after creating
					err = c.UpdateClassInclude(ctx, name, "testclasses", opts.TestSource, lock.LockHandle, opts.Transport)
				}
			}
			unlockErr := c.UnlockObject(ctx, objectURL, lock.LockHandle)
			if err != nil {
				result.Message += fmt.Sprintf(" (Warning: Failed to update test include: %v)", err)
				return result, nil
			}
			if unlockErr != nil {
				result.Message += fmt.Sprintf(" (Warning: Failed to unlock after test update: %v)", unlockErr)
			}

			// Activate the test include
			testIncludeURL := objectURL + "/includes/testclasses"
			_, activateErr := c.Activate(ctx, testIncludeURL, name)
			if activateErr != nil {
				result.Message += fmt.Sprintf(" (Warning: Failed to activate test include: %v)", activateErr)
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
		objectURL := fmt.Sprintf("/sap/bc/adt/oo/interfaces/%s", url.PathEscape(name))
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

	case "DDLS", "BDEF", "SRVD":
		// Get object URL
		var objectURL string
		switch objectType {
		case "DDLS":
			objectURL = GetObjectURL(ObjectTypeDDLS, name, "")
		case "BDEF":
			objectURL = GetObjectURL(ObjectTypeBDEF, name, "")
		case "SRVD":
			objectURL = GetObjectURL(ObjectTypeSRVD, name, "")
		}
		result.ObjectURL = objectURL
		sourceURL := objectURL + "/source/main"

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
			result.Message = fmt.Sprintf("%s updated and activated successfully", objectType)
		} else {
			result.Message = "Activation failed - check activation messages"
		}

		return result, nil

	default:
		result.Message = fmt.Sprintf("Unsupported object type for update: %s", objectType)
		return result, nil
	}
}

// writeClassMethodUpdate updates a single method in a class.
// The source should be the METHOD...ENDMETHOD block.
func (c *Client) writeClassMethodUpdate(ctx context.Context, className, methodName, methodSource, transport string) (*WriteSourceResult, error) {
	result := &WriteSourceResult{
		ObjectType: "CLAS",
		ObjectName: className,
		Method:     strings.ToUpper(methodName),
		Mode:       "updated",
	}

	className = strings.ToUpper(className)
	methodName = strings.ToUpper(methodName)
	objectURL := fmt.Sprintf("/sap/bc/adt/oo/classes/%s", url.PathEscape(strings.ToLower(className)))
	result.ObjectURL = objectURL

	// Get method boundaries
	methods, err := c.GetClassMethods(ctx, className)
	if err != nil {
		result.Message = fmt.Sprintf("Failed to get class methods: %v", err)
		return result, nil
	}

	// Find the specified method
	var foundMethod *MethodInfo
	for i := range methods {
		if methods[i].Name == methodName {
			foundMethod = &methods[i]
			break
		}
	}
	if foundMethod == nil {
		result.Message = fmt.Sprintf("Method %s not found in class %s", methodName, className)
		return result, nil
	}

	if foundMethod.ImplementationStart == 0 || foundMethod.ImplementationEnd == 0 {
		result.Message = fmt.Sprintf("Method %s has no implementation lines (abstract method?)", methodName)
		return result, nil
	}

	// Get current class source
	currentSource, err := c.GetClassSource(ctx, className)
	if err != nil {
		result.Message = fmt.Sprintf("Failed to get current class source: %v", err)
		return result, nil
	}

	// Split into lines
	sourceLines := strings.Split(currentSource, "\n")
	if foundMethod.ImplementationEnd > len(sourceLines) {
		result.Message = fmt.Sprintf("Method line range (%d-%d) exceeds source lines (%d)",
			foundMethod.ImplementationStart, foundMethod.ImplementationEnd, len(sourceLines))
		return result, nil
	}

	// Reconstruct source with new method implementation
	var newSourceLines []string
	newSourceLines = append(newSourceLines, sourceLines[:foundMethod.ImplementationStart-1]...)
	newSourceLines = append(newSourceLines, strings.Split(methodSource, "\n")...)
	newSourceLines = append(newSourceLines, sourceLines[foundMethod.ImplementationEnd:]...)
	newSource := strings.Join(newSourceLines, "\n")

	// Syntax check
	syntaxErrors, err := c.SyntaxCheck(ctx, objectURL, newSource)
	if err != nil {
		result.Message = fmt.Sprintf("Syntax check failed: %v", err)
		return result, nil
	}

	for _, se := range syntaxErrors {
		if se.Severity == "E" || se.Severity == "A" || se.Severity == "X" {
			result.SyntaxErrors = syntaxErrors
			result.Message = fmt.Sprintf("Method %s has syntax errors - not saved", methodName)
			return result, nil
		}
	}
	result.SyntaxErrors = syntaxErrors

	// Lock
	lock, err := c.LockObject(ctx, objectURL, "MODIFY")
	if err != nil {
		result.Message = fmt.Sprintf("Failed to lock class: %v", err)
		return result, nil
	}

	defer func() {
		if !result.Success {
			c.UnlockObject(ctx, objectURL, lock.LockHandle)
		}
	}()

	// Update
	sourceURL := objectURL + "/source/main"
	err = c.UpdateSource(ctx, sourceURL, newSource, lock.LockHandle, transport)
	if err != nil {
		result.Message = fmt.Sprintf("Failed to update class source: %v", err)
		return result, nil
	}

	// Unlock
	err = c.UnlockObject(ctx, objectURL, lock.LockHandle)
	if err != nil {
		result.Message = fmt.Sprintf("Failed to unlock class: %v", err)
		return result, nil
	}

	// Activate
	activation, err := c.Activate(ctx, objectURL, className)
	if err != nil {
		result.Message = fmt.Sprintf("Failed to activate class: %v", err)
		result.Activation = activation
		return result, nil
	}

	result.Activation = activation
	if activation.Success {
		result.Success = true
		result.Message = fmt.Sprintf("Method %s updated and class %s activated successfully", methodName, className)
	} else {
		result.Message = fmt.Sprintf("Method %s updated but activation failed - check activation messages", methodName)
	}

	return result, nil
}

// --- Compare Source Tool ---

// SourceDiff represents a diff between two sources.
type SourceDiff struct {
	Object1     string   `json:"object1"`
	Object2     string   `json:"object2"`
	Identical   bool     `json:"identical"`
	AddedLines  int      `json:"addedLines"`
	RemovedLines int     `json:"removedLines"`
	Diff        string   `json:"diff"`
}

// CompareSource compares source code of two objects and returns a unified diff.
// Supports comparing any two objects that can be read via GetSource.
func (c *Client) CompareSource(ctx context.Context, type1, name1, type2, name2 string, opts1, opts2 *GetSourceOptions) (*SourceDiff, error) {
	// Get source of first object
	source1, err := c.GetSource(ctx, type1, name1, opts1)
	if err != nil {
		return nil, fmt.Errorf("getting source for %s %s: %w", type1, name1, err)
	}

	// Get source of second object
	source2, err := c.GetSource(ctx, type2, name2, opts2)
	if err != nil {
		return nil, fmt.Errorf("getting source for %s %s: %w", type2, name2, err)
	}

	result := &SourceDiff{
		Object1:   fmt.Sprintf("%s:%s", type1, name1),
		Object2:   fmt.Sprintf("%s:%s", type2, name2),
		Identical: source1 == source2,
	}

	if result.Identical {
		result.Diff = "Sources are identical"
		return result, nil
	}

	// Generate unified diff
	lines1 := strings.Split(source1, "\n")
	lines2 := strings.Split(source2, "\n")

	diff := generateUnifiedDiff(result.Object1, result.Object2, lines1, lines2)
	result.Diff = diff

	// Count added/removed lines
	for _, line := range strings.Split(diff, "\n") {
		if strings.HasPrefix(line, "+") && !strings.HasPrefix(line, "+++") {
			result.AddedLines++
		} else if strings.HasPrefix(line, "-") && !strings.HasPrefix(line, "---") {
			result.RemovedLines++
		}
	}

	return result, nil
}

// generateUnifiedDiff creates a unified diff between two sets of lines.
func generateUnifiedDiff(name1, name2 string, lines1, lines2 []string) string {
	var diff strings.Builder

	diff.WriteString(fmt.Sprintf("--- %s\n", name1))
	diff.WriteString(fmt.Sprintf("+++ %s\n", name2))

	// Simple LCS-based diff algorithm
	m, n := len(lines1), len(lines2)

	// Build LCS table
	lcs := make([][]int, m+1)
	for i := range lcs {
		lcs[i] = make([]int, n+1)
	}
	for i := 1; i <= m; i++ {
		for j := 1; j <= n; j++ {
			if lines1[i-1] == lines2[j-1] {
				lcs[i][j] = lcs[i-1][j-1] + 1
			} else if lcs[i-1][j] > lcs[i][j-1] {
				lcs[i][j] = lcs[i-1][j]
			} else {
				lcs[i][j] = lcs[i][j-1]
			}
		}
	}

	// Backtrack to generate diff
	type diffLine struct {
		op   byte // ' ', '+', '-'
		text string
	}
	var diffLines []diffLine

	i, j := m, n
	for i > 0 || j > 0 {
		if i > 0 && j > 0 && lines1[i-1] == lines2[j-1] {
			diffLines = append([]diffLine{{' ', lines1[i-1]}}, diffLines...)
			i--
			j--
		} else if j > 0 && (i == 0 || lcs[i][j-1] >= lcs[i-1][j]) {
			diffLines = append([]diffLine{{'+', lines2[j-1]}}, diffLines...)
			j--
		} else {
			diffLines = append([]diffLine{{'-', lines1[i-1]}}, diffLines...)
			i--
		}
	}

	// Output hunks with context
	const contextLines = 3
	inHunk := false
	hunkStart1, hunkStart2 := 0, 0
	hunkLen1, hunkLen2 := 0, 0
	var hunkContent strings.Builder
	contextBefore := make([]diffLine, 0, contextLines)
	pendingContext := 0

	flushHunk := func() {
		if hunkLen1 > 0 || hunkLen2 > 0 {
			diff.WriteString(fmt.Sprintf("@@ -%d,%d +%d,%d @@\n", hunkStart1, hunkLen1, hunkStart2, hunkLen2))
			diff.WriteString(hunkContent.String())
		}
		hunkContent.Reset()
		inHunk = false
		hunkLen1, hunkLen2 = 0, 0
	}

	line1, line2 := 1, 1
	for _, dl := range diffLines {
		if dl.op == ' ' {
			if inHunk {
				pendingContext++
				hunkContent.WriteString(fmt.Sprintf(" %s\n", dl.text))
				hunkLen1++
				hunkLen2++
				if pendingContext >= contextLines*2 {
					// Too much context, close hunk
					flushHunk()
					contextBefore = contextBefore[:0]
				}
			} else {
				// Accumulate context before a hunk
				if len(contextBefore) >= contextLines {
					contextBefore = contextBefore[1:]
				}
				contextBefore = append(contextBefore, dl)
			}
			line1++
			line2++
		} else {
			pendingContext = 0
			if !inHunk {
				// Start new hunk
				inHunk = true
				hunkStart1 = line1 - len(contextBefore)
				hunkStart2 = line2 - len(contextBefore)
				if hunkStart1 < 1 { hunkStart1 = 1 }
				if hunkStart2 < 1 { hunkStart2 = 1 }
				// Add context before
				for _, ctx := range contextBefore {
					hunkContent.WriteString(fmt.Sprintf(" %s\n", ctx.text))
					hunkLen1++
					hunkLen2++
				}
				contextBefore = contextBefore[:0]
			}
			hunkContent.WriteString(fmt.Sprintf("%c%s\n", dl.op, dl.text))
			if dl.op == '-' {
				hunkLen1++
				line1++
			} else {
				hunkLen2++
				line2++
			}
		}
	}
	flushHunk()

	return diff.String()
}

// --- Clone Object Tool ---

// CloneObjectResult represents the result of cloning an object.
type CloneObjectResult struct {
	Success     bool   `json:"success"`
	SourceName  string `json:"sourceName"`
	TargetName  string `json:"targetName"`
	ObjectType  string `json:"objectType"`
	Package     string `json:"package"`
	Message     string `json:"message"`
}

// CloneObject copies an ABAP object to a new name.
// Supported types: PROG, CLAS, INTF
func (c *Client) CloneObject(ctx context.Context, objectType, sourceName, targetName, targetPackage string) (*CloneObjectResult, error) {
	// Safety check
	if err := c.checkSafety(OpCreate, "CloneObject"); err != nil {
		return nil, err
	}

	result := &CloneObjectResult{
		SourceName: sourceName,
		TargetName: targetName,
		ObjectType: objectType,
		Package:    targetPackage,
	}

	// Get source of original object
	source, err := c.GetSource(ctx, objectType, sourceName, nil)
	if err != nil {
		result.Message = fmt.Sprintf("Failed to get source: %v", err)
		return result, nil
	}

	// Replace object name in source
	objectType = strings.ToUpper(objectType)
	sourceName = strings.ToUpper(sourceName)
	targetName = strings.ToUpper(targetName)

	// Replace the object name in the source code
	var newSource string
	switch objectType {
	case "PROG":
		// Replace REPORT <old> with REPORT <new>
		re := regexp.MustCompile(`(?i)(REPORT\s+)` + regexp.QuoteMeta(sourceName))
		newSource = re.ReplaceAllString(source, "${1}"+targetName)
	case "CLAS":
		// Replace CLASS <old> with CLASS <new>
		re := regexp.MustCompile(`(?i)(CLASS\s+)` + regexp.QuoteMeta(sourceName))
		newSource = re.ReplaceAllString(source, "${1}"+targetName)
	case "INTF":
		// Replace INTERFACE <old> with INTERFACE <new>
		re := regexp.MustCompile(`(?i)(INTERFACE\s+)` + regexp.QuoteMeta(sourceName))
		newSource = re.ReplaceAllString(source, "${1}"+targetName)
	default:
		result.Message = fmt.Sprintf("Unsupported object type for cloning: %s", objectType)
		return result, nil
	}

	// Write as new object
	description := fmt.Sprintf("Copy of %s", sourceName)
	writeResult, err := c.WriteSource(ctx, objectType, targetName, newSource, &WriteSourceOptions{
		Package:     targetPackage,
		Description: description,
		Mode:        "create",
	})
	if err != nil {
		result.Message = fmt.Sprintf("Failed to create clone: %v", err)
		return result, nil
	}

	if !writeResult.Success {
		result.Message = writeResult.Message
		return result, nil
	}

	result.Success = true
	result.Message = fmt.Sprintf("Successfully cloned %s to %s", sourceName, targetName)
	return result, nil
}

// --- GetClassInfo Tool ---

// ClassInfo contains metadata about an ABAP class.
type ClassInfo struct {
	Name          string   `json:"name"`
	Description   string   `json:"description,omitempty"`
	Package       string   `json:"package,omitempty"`
	Category      string   `json:"category,omitempty"`      // Regular, Abstract, Final
	Visibility    string   `json:"visibility,omitempty"`    // Public, Protected, Private
	Superclass    string   `json:"superclass,omitempty"`
	Interfaces    []string `json:"interfaces,omitempty"`
	Methods       []string `json:"methods,omitempty"`
	Attributes    []string `json:"attributes,omitempty"`
	HasTestClass  bool     `json:"hasTestClass"`
	IsAbstract    bool     `json:"isAbstract"`
	IsFinal       bool     `json:"isFinal"`
}

// GetClassInfo retrieves class metadata without full source code.
// Uses GetObjectStructure for quick metadata extraction.
func (c *Client) GetClassInfo(ctx context.Context, className string) (*ClassInfo, error) {
	// Safety check
	if err := c.checkSafety(OpRead, "GetClassInfo"); err != nil {
		return nil, err
	}

	className = strings.ToUpper(className)

	// Get object structure
	structure, err := c.GetObjectStructureCAI(ctx, className, 100)
	if err != nil {
		return nil, fmt.Errorf("getting class structure: %w", err)
	}

	info := &ClassInfo{
		Name:       className,
		Methods:    make([]string, 0),
		Attributes: make([]string, 0),
		Interfaces: make([]string, 0),
	}

	// Parse root node
	if structure != nil {
		info.Description = structure.Description

		// Recursive function to extract info from tree
		var extractInfo func(node *ObjectExplorerNode)
		extractInfo = func(node *ObjectExplorerNode) {
			nodeType := strings.ToUpper(node.Type)
			nodeName := node.Name

			switch {
			case strings.Contains(nodeType, "METHOD"):
				info.Methods = append(info.Methods, nodeName)
			case strings.Contains(nodeType, "ATTR"):
				info.Attributes = append(info.Attributes, nodeName)
			case strings.Contains(nodeType, "INTF"):
				info.Interfaces = append(info.Interfaces, nodeName)
			case strings.Contains(nodeType, "TEST"):
				info.HasTestClass = true
			}

			// Check for superclass in description
			if strings.Contains(strings.ToUpper(node.Description), "INHERITING") {
				parts := strings.Fields(node.Description)
				for i, p := range parts {
					if strings.ToUpper(p) == "FROM" && i+1 < len(parts) {
						info.Superclass = parts[i+1]
					}
				}
			}

			// Recurse into children
			for i := range node.Children {
				extractInfo(&node.Children[i])
			}
		}

		extractInfo(structure)
	}

	// Check for abstract/final in main source (quick scan)
	source, err := c.GetClassSource(ctx, className)
	if err == nil {
		sourceUpper := strings.ToUpper(source)
		if strings.Contains(sourceUpper, "CLASS "+className+" DEFINITION ABSTRACT") ||
			strings.Contains(sourceUpper, "ABSTRACT DEFINITION") {
			info.IsAbstract = true
			info.Category = "Abstract"
		}
		if strings.Contains(sourceUpper, "CLASS "+className+" DEFINITION FINAL") ||
			strings.Contains(sourceUpper, "FINAL DEFINITION") {
			info.IsFinal = true
			info.Category = "Final"
		}
		if info.Category == "" {
			info.Category = "Regular"
		}

		// Extract package from source header if present
		lines := strings.Split(source, "\n")
		for _, line := range lines[:min(20, len(lines))] {
			if strings.Contains(strings.ToUpper(line), "DEVC") {
				// Try to extract package
				re := regexp.MustCompile(`DEVC\s+(\$?\w+)`)
				if matches := re.FindStringSubmatch(line); len(matches) > 1 {
					info.Package = matches[1]
				}
			}
		}
	}

	return info, nil
}
