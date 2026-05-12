package adt

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

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
	result := &RenameObjectResult{
		OldName:    oldName,
		NewName:    newName,
		ObjectType: string(objType),
	}

	oldURL, err := c.buildObjectURL(objType, oldName)
	if err != nil {
		return nil, err
	}

	// Unified mutation policy gate for the old object being deleted.
	if err := c.checkMutation(ctx, MutationContext{
		Op:        OpDelete,
		OpName:    "RenameObject",
		ObjectURL: oldURL,
		Transport: transport,
	}); err != nil {
		return nil, err
	}

	// If a target package is supplied, gate the create side as well.
	if packageName != "" {
		if err := c.checkMutation(ctx, MutationContext{
			Op:        OpCreate,
			OpName:    "RenameObject",
			Package:   packageName,
			Transport: transport,
		}); err != nil {
			return nil, err
		}
	}

	// 1. Get old object source
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
func (c *Client) SaveToFile(ctx context.Context, objType CreatableObjectType, objectName, parentName, outputPath string) (*SaveToFileResult, error) {
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
	case ObjectTypeFunctionMod:
		ext = ".func.abap"
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
	objectURL, err := c.buildObjectURLWithParent(objType, objectName, parentName)
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
