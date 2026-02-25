// Package mcp provides the MCP server implementation for ABAP ADT tools.
// handlers_fileio.go contains handlers for file-based deployment operations.
package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/oisee/vibing-steampunk/pkg/adt"
)

// --- File-Based Deployment Handlers ---

// Note: CreateFromFile and UpdateFromFile handlers removed - use DeployFromFile instead

func (s *Server) handleDeployFromFile(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	filePath, ok := request.GetArguments()["file_path"].(string)
	if !ok || filePath == "" {
		return newToolResultError("file_path is required"), nil
	}

	packageName, ok := request.GetArguments()["package_name"].(string)
	if !ok || packageName == "" {
		return newToolResultError("package_name is required"), nil
	}

	transport := ""
	if t, ok := request.GetArguments()["transport"].(string); ok {
		transport = t
	}

	result, err := s.adtClient.DeployFromFile(ctx, filePath, packageName, transport)
	if err != nil {
		return newToolResultError(fmt.Sprintf("DeployFromFile failed: %v", err)), nil
	}

	output, _ := json.MarshalIndent(result, "", "  ")
	return mcp.NewToolResultText(string(output)), nil
}

func (s *Server) handleSaveToFile(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	// Support both old (objType/objectName/outputPath) and new (object_type/object_name/output_dir) parameter names
	objTypeStr, ok := request.GetArguments()["objType"].(string)
	if !ok || objTypeStr == "" {
		objTypeStr, ok = request.GetArguments()["object_type"].(string)
		if !ok || objTypeStr == "" {
			return newToolResultError("object_type is required (e.g., PROG, CLAS, INTF, FUGR, FUNC, DDLS, BDEF, SRVD)"), nil
		}
	}

	objectName, ok := request.GetArguments()["objectName"].(string)
	if !ok || objectName == "" {
		objectName, ok = request.GetArguments()["object_name"].(string)
		if !ok || objectName == "" {
			return newToolResultError("object_name is required"), nil
		}
	}

	outputPath := ""
	if p, ok := request.GetArguments()["outputPath"].(string); ok {
		outputPath = p
	} else if p, ok := request.GetArguments()["output_dir"].(string); ok {
		outputPath = p
	}

	// Check for include parameter (for class includes)
	includeStr := ""
	if inc, ok := request.GetArguments()["include"].(string); ok {
		includeStr = strings.ToLower(inc)
	}

	// Parse object type - support both short (PROG) and full (PROG/P) format
	var objType adt.CreatableObjectType
	switch strings.ToUpper(objTypeStr) {
	case "PROG", "PROG/P":
		objType = adt.ObjectTypeProgram
	case "CLAS", "CLAS/OC":
		objType = adt.ObjectTypeClass
	case "INTF", "INTF/OI":
		objType = adt.ObjectTypeInterface
	case "FUGR", "FUGR/F":
		objType = adt.ObjectTypeFunctionGroup
	case "FUNC", "FUGR/FF":
		objType = adt.ObjectTypeFunctionMod
	case "INCL", "PROG/I":
		objType = adt.ObjectTypeInclude
	// RAP types
	case "DDLS", "DDLS/DF":
		objType = adt.ObjectTypeDDLS
	case "BDEF", "BDEF/BDO":
		objType = adt.ObjectTypeBDEF
	case "SRVD", "SRVD/SRV":
		objType = adt.ObjectTypeSRVD
	default:
		objType = adt.CreatableObjectType(objTypeStr)
	}

	// Handle class includes
	if objType == adt.ObjectTypeClass && includeStr != "" && includeStr != "main" {
		var includeType adt.ClassIncludeType
		switch includeStr {
		case "testclasses":
			includeType = adt.ClassIncludeTestClasses
		case "definitions":
			includeType = adt.ClassIncludeDefinitions
		case "implementations":
			includeType = adt.ClassIncludeImplementations
		case "macros":
			includeType = adt.ClassIncludeMacros
		default:
			return newToolResultError(fmt.Sprintf("invalid include type: %s (expected: main, testclasses, definitions, implementations, macros)", includeStr)), nil
		}

		result, err := s.adtClient.SaveClassIncludeToFile(ctx, objectName, includeType, outputPath)
		if err != nil {
			return newToolResultError(fmt.Sprintf("SaveClassIncludeToFile failed: %v", err)), nil
		}
		output, _ := json.MarshalIndent(result, "", "  ")
		return mcp.NewToolResultText(string(output)), nil
	}

	result, err := s.adtClient.SaveToFile(ctx, objType, objectName, outputPath)
	if err != nil {
		return newToolResultError(fmt.Sprintf("SaveToFile failed: %v", err)), nil
	}

	output, _ := json.MarshalIndent(result, "", "  ")
	return mcp.NewToolResultText(string(output)), nil
}

func (s *Server) handleRenameObject(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	objTypeStr, ok := request.GetArguments()["objType"].(string)
	if !ok || objTypeStr == "" {
		return newToolResultError("objType is required (e.g., CLAS/OC, PROG/P, INTF/OI, FUGR/F)"), nil
	}

	oldName, ok := request.GetArguments()["oldName"].(string)
	if !ok || oldName == "" {
		return newToolResultError("oldName is required"), nil
	}

	newName, ok := request.GetArguments()["newName"].(string)
	if !ok || newName == "" {
		return newToolResultError("newName is required"), nil
	}

	packageName, ok := request.GetArguments()["packageName"].(string)
	if !ok || packageName == "" {
		return newToolResultError("packageName is required"), nil
	}

	transport := ""
	if t, ok := request.GetArguments()["transport"].(string); ok {
		transport = t
	}

	// Parse object type
	objType := adt.CreatableObjectType(objTypeStr)

	result, err := s.adtClient.RenameObject(ctx, objType, oldName, newName, packageName, transport)
	if err != nil {
		return newToolResultError(fmt.Sprintf("RenameObject failed: %v", err)), nil
	}

	output, _ := json.MarshalIndent(result, "", "  ")
	return mcp.NewToolResultText(string(output)), nil
}

func (s *Server) handleEditSource(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	objectURL, ok := request.GetArguments()["object_url"].(string)
	if !ok || objectURL == "" {
		return newToolResultError("object_url is required"), nil
	}

	oldString, ok := request.GetArguments()["old_string"].(string)
	if !ok || oldString == "" {
		return newToolResultError("old_string is required"), nil
	}

	newString, ok := request.GetArguments()["new_string"].(string)
	if !ok {
		return newToolResultError("new_string is required"), nil
	}

	replaceAll := false
	if r, ok := request.GetArguments()["replace_all"].(bool); ok {
		replaceAll = r
	}

	syntaxCheck := true
	if sc, ok := request.GetArguments()["syntax_check"].(bool); ok {
		syntaxCheck = sc
	}

	caseInsensitive := false
	if ci, ok := request.GetArguments()["case_insensitive"].(bool); ok {
		caseInsensitive = ci
	}

	method := ""
	if m, ok := request.GetArguments()["method"].(string); ok {
		method = m
	}

	transport := ""
	if t, ok := request.GetArguments()["transport"].(string); ok {
		transport = t
	}

	opts := &adt.EditSourceOptions{
		ReplaceAll:      replaceAll,
		SyntaxCheck:     syntaxCheck,
		CaseInsensitive: caseInsensitive,
		Method:          method,
		Transport:       transport,
	}

	result, err := s.adtClient.EditSourceWithOptions(ctx, objectURL, oldString, newString, opts)
	if err != nil {
		return newToolResultError(fmt.Sprintf("EditSource failed: %v", err)), nil
	}

	output, _ := json.MarshalIndent(result, "", "  ")
	return mcp.NewToolResultText(string(output)), nil
}
