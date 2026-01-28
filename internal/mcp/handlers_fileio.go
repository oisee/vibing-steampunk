// Package mcp provides the MCP server implementation for ABAP ADT tools.
// handlers_fileio.go contains handlers for file-based deployment operations.
package mcp

import (
	"context"
	"strings"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/oisee/vibing-steampunk/pkg/adt"
)

// --- FileIO Routing ---
// Routes for this module:
//   edit: type=rename, type=edit_source, DEPLOY_FILE
//   deploy: import, export

// routeFileIOAction routes file IO actions.
// Returns (result, true) if handled, (nil, false) if not handled by this module.
func (s *Server) routeFileIOAction(ctx context.Context, action, objectType, objectName string, params map[string]any) (*mcp.CallToolResult, bool, error) {
	switch action {
	case "edit":
		editType, _ := params["type"].(string)
		switch editType {
		case "edit_source":
			objectURL, _ := params["object_url"].(string)
			oldString, _ := params["old_string"].(string)
			newString, _ := params["new_string"].(string)
			if objectURL == "" || oldString == "" {
				return newToolResultError("object_url and old_string are required for edit_source"), true, nil
			}
			args := map[string]any{
				"object_url": objectURL,
				"old_string": oldString,
				"new_string": newString,
			}
			if replaceAll, ok := params["replace_all"].(bool); ok {
				args["replace_all"] = replaceAll
			}
			if syntaxCheck, ok := params["syntax_check"].(bool); ok {
				args["syntax_check"] = syntaxCheck
			}
			if caseInsensitive, ok := params["case_insensitive"].(bool); ok {
				args["case_insensitive"] = caseInsensitive
			}
			if method, ok := params["method"].(string); ok {
				args["method"] = method
			}
			result, err := s.handleEditSource(ctx, newRequest(args))
			return result, true, err

		case "rename":
			objType, _ := params["objType"].(string)
			oldName, _ := params["oldName"].(string)
			newName, _ := params["newName"].(string)
			packageName, _ := params["packageName"].(string)
			transport, _ := params["transport"].(string)
			if objType == "" || oldName == "" || newName == "" || packageName == "" {
				return newToolResultError("rename requires objType, oldName, newName, packageName in params"), true, nil
			}
			args := map[string]any{
				"objType":     objType,
				"oldName":     oldName,
				"newName":     newName,
				"packageName": packageName,
			}
			if transport != "" {
				args["transport"] = transport
			}
			result, err := s.handleRenameObject(ctx, newRequest(args))
			return result, true, err
		}

	case "deploy":
		switch objectType {
		case "import", "DEPLOY_FILE":
			filePath, _ := params["file_path"].(string)
			packageName, _ := params["package_name"].(string)
			if filePath == "" || packageName == "" {
				return newToolResultError("file_path and package_name are required for deploy import"), true, nil
			}
			args := map[string]any{
				"file_path":    filePath,
				"package_name": packageName,
			}
			if transport, ok := params["transport"].(string); ok {
				args["transport"] = transport
			}
			result, err := s.handleDeployFromFile(ctx, newRequest(args))
			return result, true, err

		case "export":
			objType, _ := params["object_type"].(string)
			if objType == "" {
				objType = objectName
			}
			objName, _ := params["object_name"].(string)
			outputDir, _ := params["output_dir"].(string)
			if objName == "" || outputDir == "" {
				return newToolResultError("object_name and output_dir are required for deploy export"), true, nil
			}
			args := map[string]any{
				"object_type": objType,
				"object_name": objName,
				"output_dir":  outputDir,
			}
			if include, ok := params["include"].(string); ok {
				args["include"] = include
			}
			if parent, ok := params["parent"].(string); ok {
				args["parent"] = parent
			}
			result, err := s.handleSaveToFile(ctx, newRequest(args))
			return result, true, err
		}
	}

	return nil, false, nil
}

// --- File-Based Deployment Handlers ---

// Note: CreateFromFile and UpdateFromFile handlers removed - use DeployFromFile instead

func (s *Server) handleDeployFromFile(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	filePath, ok := request.Params.Arguments["file_path"].(string)
	if !ok || filePath == "" {
		return newToolResultError("file_path is required"), nil
	}

	packageName, ok := request.Params.Arguments["package_name"].(string)
	if !ok || packageName == "" {
		return newToolResultError("package_name is required"), nil
	}

	transport := ""
	if t, ok := request.Params.Arguments["transport"].(string); ok {
		transport = t
	}

	result, err := s.adtClient.DeployFromFile(ctx, filePath, packageName, transport)
	if err != nil {
		return wrapErr("DeployFromFile", err), nil
	}
	return newToolResultJSON(result), nil
}

func (s *Server) handleSaveToFile(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	objTypeStr, _ := request.Params.Arguments["object_type"].(string)
	if objTypeStr == "" {
		return newToolResultError("object_type is required (e.g., PROG, CLAS, INTF, FUGR, FUNC, DDLS, BDEF, SRVD)"), nil
	}

	objectName, _ := request.Params.Arguments["object_name"].(string)
	if objectName == "" {
		return newToolResultError("object_name is required"), nil
	}

	outputDir, _ := request.Params.Arguments["output_dir"].(string)
	includeStr, _ := request.Params.Arguments["include"].(string)

	// Parse object type
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
	if objType == adt.ObjectTypeClass && includeStr != "" && strings.ToLower(includeStr) != "main" {
		var includeType adt.ClassIncludeType
		switch strings.ToLower(includeStr) {
		case "testclasses":
			includeType = adt.ClassIncludeTestClasses
		case "definitions":
			includeType = adt.ClassIncludeDefinitions
		case "implementations":
			includeType = adt.ClassIncludeImplementations
		case "macros":
			includeType = adt.ClassIncludeMacros
		default:
			return newToolResultError("invalid include type: " + includeStr + " (expected: main, testclasses, definitions, implementations, macros)"), nil
		}

		result, err := s.adtClient.SaveClassIncludeToFile(ctx, objectName, includeType, outputDir)
		if err != nil {
			return wrapErr("SaveClassIncludeToFile", err), nil
		}
		return newToolResultJSON(result), nil
	}

	result, err := s.adtClient.SaveToFile(ctx, objType, objectName, outputDir)
	if err != nil {
		return wrapErr("SaveToFile", err), nil
	}
	return newToolResultJSON(result), nil
}

func (s *Server) handleRenameObject(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	objTypeStr, ok := request.Params.Arguments["objType"].(string)
	if !ok || objTypeStr == "" {
		return newToolResultError("objType is required (e.g., CLAS/OC, PROG/P, INTF/OI, FUGR/F)"), nil
	}

	oldName, ok := request.Params.Arguments["oldName"].(string)
	if !ok || oldName == "" {
		return newToolResultError("oldName is required"), nil
	}

	newName, ok := request.Params.Arguments["newName"].(string)
	if !ok || newName == "" {
		return newToolResultError("newName is required"), nil
	}

	packageName, ok := request.Params.Arguments["packageName"].(string)
	if !ok || packageName == "" {
		return newToolResultError("packageName is required"), nil
	}

	transport := ""
	if t, ok := request.Params.Arguments["transport"].(string); ok {
		transport = t
	}

	// Parse object type
	objType := adt.CreatableObjectType(objTypeStr)

	result, err := s.adtClient.RenameObject(ctx, objType, oldName, newName, packageName, transport)
	if err != nil {
		return wrapErr("RenameObject", err), nil
	}
	return newToolResultJSON(result), nil
}

func (s *Server) handleEditSource(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	objectURL, ok := request.Params.Arguments["object_url"].(string)
	if !ok || objectURL == "" {
		return newToolResultError("object_url is required"), nil
	}

	oldString, ok := request.Params.Arguments["old_string"].(string)
	if !ok || oldString == "" {
		return newToolResultError("old_string is required"), nil
	}

	newString, ok := request.Params.Arguments["new_string"].(string)
	if !ok {
		return newToolResultError("new_string is required"), nil
	}

	replaceAll := false
	if r, ok := request.Params.Arguments["replace_all"].(bool); ok {
		replaceAll = r
	}

	syntaxCheck := true
	if sc, ok := request.Params.Arguments["syntax_check"].(bool); ok {
		syntaxCheck = sc
	}

	caseInsensitive := false
	if ci, ok := request.Params.Arguments["case_insensitive"].(bool); ok {
		caseInsensitive = ci
	}

	method := ""
	if m, ok := request.Params.Arguments["method"].(string); ok {
		method = m
	}

	opts := &adt.EditSourceOptions{
		ReplaceAll:      replaceAll,
		SyntaxCheck:     syntaxCheck,
		CaseInsensitive: caseInsensitive,
		Method:          method,
	}

	result, err := s.adtClient.EditSourceWithOptions(ctx, objectURL, oldString, newString, opts)
	if err != nil {
		return wrapErr("EditSource", err), nil
	}
	return newToolResultJSON(result), nil
}
