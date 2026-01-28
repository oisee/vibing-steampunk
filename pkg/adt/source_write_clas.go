package adt

import (
	"context"
	"fmt"
	"strings"
)

// --- Class Write Operations ---

// writeSourceClas handles write operations for CLAS (classes).
func (c *Client) writeSourceClas(ctx context.Context, name, source string, opts *WriteSourceOptions) (*WriteSourceResult, error) {
	result := &WriteSourceResult{
		ObjectType: "CLAS",
		ObjectName: name,
	}

	actualMode := c.determineWriteMode(ctx, "CLAS", name, opts)

	// Validate mode
	if actualMode == WriteModeCreate {
		if opts.Package == "" {
			result.Message = "Package is required for creating new classes"
			return result, nil
		}
		if opts.Description == "" {
			result.Message = "Description is required for creating new classes"
			return result, nil
		}
		return c.createClass(ctx, name, source, opts)
	}

	return c.updateClass(ctx, name, source, opts)
}

// createClass creates a new class with source code and activates it.
// Optionally includes test source if opts.TestSource is provided.
// Workflow: CreateObject -> Lock -> UpdateSource -> [CreateTestInclude -> UpdateTestSource] -> Unlock -> Activate -> [RunTests]
func (c *Client) createClass(ctx context.Context, name, source string, opts *WriteSourceOptions) (*WriteSourceResult, error) {
	name = strings.ToUpper(name)
	packageName := strings.ToUpper(opts.Package)

	// Check package restrictions
	if err := c.checkPackageSafety(packageName); err != nil {
		return nil, err
	}

	objectURL := fmt.Sprintf("/sap/bc/adt/oo/classes/%s", name)
	sourceURL := objectURL + "/source/main"

	result := &WriteSourceResult{
		ObjectType: "CLAS",
		ObjectName: name,
		ObjectURL:  objectURL,
		Mode:       "created",
	}

	// Step 1: Create the class
	err := c.CreateObject(ctx, CreateObjectOptions{
		ObjectType:  ObjectTypeClass,
		Name:        name,
		Description: opts.Description,
		PackageName: packageName,
		Transport:   opts.Transport,
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
			_ = c.UnlockObject(ctx, objectURL, lock.LockHandle)
		}
	}()

	// Step 3: Update main source
	err = c.UpdateSource(ctx, sourceURL, source, lock.LockHandle, opts.Transport)
	if err != nil {
		result.Message = fmt.Sprintf("Failed to update class source: %v", err)
		return result, nil
	}

	// Step 4: If test source provided, create and update test include
	if opts.TestSource != "" {
		err = c.CreateTestInclude(ctx, name, lock.LockHandle, opts.Transport)
		if err != nil {
			result.Message = fmt.Sprintf("Failed to create test include: %v", err)
			return result, nil
		}

		err = c.UpdateClassInclude(ctx, name, ClassIncludeTestClasses, opts.TestSource, lock.LockHandle, opts.Transport)
		if err != nil {
			result.Message = fmt.Sprintf("Failed to update test source: %v", err)
			return result, nil
		}
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

	if !activation.Success {
		result.Message = "Activation failed - check activation messages"
		return result, nil
	}

	// Step 7: Run unit tests if test source was provided
	if opts.TestSource != "" {
		flags := DefaultUnitTestFlags()
		testResult, err := c.RunUnitTests(ctx, objectURL, &flags)
		if err != nil {
			result.Message = fmt.Sprintf("Class activated but unit tests failed to run: %v", err)
			result.Success = true // Class was created successfully
			return result, nil
		}
		result.TestResults = testResult
	}

	result.Success = true
	if opts.TestSource != "" {
		result.Message = "Class created, activated, and unit tests executed successfully"
	} else {
		result.Message = "Class created and activated successfully"
	}

	return result, nil
}

// updateClass updates an existing class's source code.
// Supports method-level updates via opts.Method.
// Workflow: SyntaxCheck -> Lock -> UpdateSource -> Unlock -> Activate
func (c *Client) updateClass(ctx context.Context, name, source string, opts *WriteSourceOptions) (*WriteSourceResult, error) {
	name = strings.ToUpper(name)

	// Method-level update
	if opts.Method != "" {
		return c.updateClassMethod(ctx, name, opts.Method, source, opts.Transport)
	}

	objectURL := fmt.Sprintf("/sap/bc/adt/oo/classes/%s", name)
	sourceURL := objectURL + "/source/main"

	result := &WriteSourceResult{
		ObjectType: "CLAS",
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

	// Step 3: Update description if provided
	if opts.Description != "" {
		err = c.UpdateObjectDescription(ctx, objectURL, ObjectTypeClass, name, opts.Description, lock.LockHandle, opts.Transport)
		if err != nil {
			result.Message = fmt.Sprintf("Failed to update description: %v", err)
			return result, nil
		}
	}

	// Step 4: Update source
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
		result.Message = "Class updated and activated successfully"
	} else {
		result.Message = "Activation failed - check activation messages"
	}

	// Handle test source update if provided
	if opts.TestSource != "" {
		c.updateClassTestInclude(ctx, name, opts.TestSource, opts.Transport, result)
	}

	return result, nil
}

// updateClassTestInclude updates the test include for a class.
func (c *Client) updateClassTestInclude(ctx context.Context, name, testSource, transport string, result *WriteSourceResult) {
	objectURL := fmt.Sprintf("/sap/bc/adt/oo/classes/%s", name)

	// Lock for test update
	lock, err := c.LockObject(ctx, objectURL, "MODIFY")
	if err != nil {
		result.Message += fmt.Sprintf(" (Warning: Failed to lock for test update: %v)", err)
		return
	}

	// Update test include - try update first, create if it doesn't exist
	err = c.UpdateClassInclude(ctx, name, ClassIncludeTestClasses, testSource, lock.LockHandle, transport)
	if err != nil {
		// Try to create the test include first (it may not exist)
		createErr := c.CreateTestInclude(ctx, name, lock.LockHandle, transport)
		if createErr == nil {
			// Retry update after creating
			err = c.UpdateClassInclude(ctx, name, ClassIncludeTestClasses, testSource, lock.LockHandle, transport)
		}
	}

	unlockErr := c.UnlockObject(ctx, objectURL, lock.LockHandle)
	if err != nil {
		result.Message += fmt.Sprintf(" (Warning: Failed to update test include: %v)", err)
		return
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

// updateClassMethod updates a single method in a class.
// The source should be the METHOD...ENDMETHOD block.
func (c *Client) updateClassMethod(ctx context.Context, className, methodName, methodSource, transport string) (*WriteSourceResult, error) {
	className = strings.ToUpper(className)
	methodName = strings.ToUpper(methodName)
	objectURL := fmt.Sprintf("/sap/bc/adt/oo/classes/%s", strings.ToLower(className))

	result := &WriteSourceResult{
		ObjectType: "CLAS",
		ObjectName: className,
		ObjectURL:  objectURL,
		Method:     methodName,
		Mode:       "updated",
	}

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

	result.SyntaxErrors = syntaxErrors
	if hasFatalSyntaxErrors(syntaxErrors) {
		result.Message = fmt.Sprintf("Method %s has syntax errors - not saved", methodName)
		return result, nil
	}

	// Lock
	lock, err := c.LockObject(ctx, objectURL, "MODIFY")
	if err != nil {
		result.Message = fmt.Sprintf("Failed to lock class: %v", err)
		return result, nil
	}

	defer func() {
		if !result.Success {
			_ = c.UnlockObject(ctx, objectURL, lock.LockHandle)
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
