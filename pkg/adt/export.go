package adt

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// --- Export Operations (SAP → Local File) ---

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
	ext := getFileExtension(objType)

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
	ext := getClassIncludeExtension(includeType)

	// 2. Build file path
	if outputPath == "" {
		outputPath = "."
	}
	if !strings.HasSuffix(outputPath, ext) {
		// outputPath is a directory
		className = strings.ToLower(className)
		result.FilePath = filepath.Join(outputPath, className+ext)
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
	result.Message = fmt.Sprintf("Saved CLAS %s.%s to %s (%d lines)", className, includeType, result.FilePath, result.LineCount)
	return result, nil
}

// getFileExtension returns the abapGit-compatible file extension for an object type.
func getFileExtension(objType CreatableObjectType) string {
	switch objType {
	case ObjectTypeClass:
		return ".clas.abap"
	case ObjectTypeProgram:
		return ".prog.abap"
	case ObjectTypeInterface:
		return ".intf.abap"
	case ObjectTypeFunctionGroup:
		return ".fugr.abap"
	case ObjectTypeInclude:
		return ".abap"
	case ObjectTypeDDLS:
		return ".ddls.asddls"
	case ObjectTypeBDEF:
		return ".bdef.asbdef"
	case ObjectTypeSRVD:
		return ".srvd.srvdsrv"
	default:
		return ".abap"
	}
}

// getClassIncludeExtension returns the abapGit-compatible file extension for a class include type.
func getClassIncludeExtension(includeType ClassIncludeType) string {
	switch includeType {
	case ClassIncludeTestClasses:
		return ".clas.testclasses.abap"
	case ClassIncludeDefinitions:
		return ".clas.locals_def.abap"
	case ClassIncludeImplementations:
		return ".clas.locals_imp.abap"
	case ClassIncludeMacros:
		return ".clas.macros.abap"
	case ClassIncludeMain, "":
		return ".clas.abap"
	default:
		return ".clas.abap"
	}
}
