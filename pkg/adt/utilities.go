package adt

import (
	"context"
	"fmt"
	"regexp"
	"strings"
)

// --- Utility Workflows ---

// RenameObjectResult contains the result of renaming an object.
type RenameObjectResult struct {
	OldName    string   `json:"oldName"`
	NewName    string   `json:"newName"`
	ObjectType string   `json:"objectType"`
	Success    bool     `json:"success"`
	Message    string   `json:"message,omitempty"`
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

// --- Clone Object Tool ---

// CloneObjectResult represents the result of cloning an object.
type CloneObjectResult struct {
	Success    bool   `json:"success"`
	SourceName string `json:"sourceName"`
	TargetName string `json:"targetName"`
	ObjectType string `json:"objectType"`
	Package    string `json:"package"`
	Message    string `json:"message"`
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
	Name         string   `json:"name"`
	Description  string   `json:"description,omitempty"`
	Package      string   `json:"package,omitempty"`
	Category     string   `json:"category,omitempty"`   // Regular, Abstract, Final
	Visibility   string   `json:"visibility,omitempty"` // Public, Protected, Private
	Superclass   string   `json:"superclass,omitempty"`
	Interfaces   []string `json:"interfaces,omitempty"`
	Methods      []string `json:"methods,omitempty"`
	Attributes   []string `json:"attributes,omitempty"`
	HasTestClass bool     `json:"hasTestClass"`
	IsAbstract   bool     `json:"isAbstract"`
	IsFinal      bool     `json:"isFinal"`
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
