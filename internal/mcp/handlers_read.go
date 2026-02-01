// Package mcp provides the MCP server implementation for ABAP ADT tools.
// handlers_read.go contains handlers for read operations (GetProgram, GetClass, GetTable, etc.)
package mcp

import (
	"context"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/oisee/vibing-steampunk/pkg/adt"
)

// --- Read Routing ---
// Routes for this module:
//   read (simple source): PROG, INTF, INCL, TABL, STRU, XSLT, DTEL, DOMA, DDLS, BDEF, SRVD, VIEW
//   read (special):       CLAS (combined source), FUNC (needs group), FUGR/DEVC/MSAG/TRAN/TYPE_INFO (JSON)
//   query:                TABL_CONTENTS, SQL
//   analyze:              cds_dependencies

// routeReadAction routes basic read operations.
// Returns (result, true) if handled, (nil, false) if not handled by this module.
func (s *Server) routeReadAction(ctx context.Context, action, objectType, objectName string, params map[string]any) (*mcp.CallToolResult, bool, error) {
	switch action {
	case "read":
		switch objectType {
		// Simple source types - use unified GetSource
		case "PROG", "INTF", "INCL", "TABL", "STRU", "XSLT", "DTEL", "DOMA", "DDLS", "BDEF", "SRVD", "VIEW":
			if objectName == "" {
				return newToolResultError("object name is required"), true, nil
			}
			source, err := s.adtClient.GetSource(ctx, objectType, objectName, nil)
			if err != nil {
				return wrapErr("GetSource", err), true, nil
			}
			return mcp.NewToolResultText(source), true, nil

		case "CLAS":
			if objectName == "" {
				return newToolResultError("class_name is required"), true, nil
			}
			result, err := s.handleGetClass(ctx, newRequest(map[string]any{"class_name": objectName}))
			return result, true, err

		case "FUNC":
			if objectName == "" {
				return newToolResultError("function_name is required"), true, nil
			}
			functionGroup, _ := params["function_group"].(string)
			if functionGroup == "" {
				return newToolResultError("function_group is required in params"), true, nil
			}
			result, err := s.handleGetFunction(ctx, newRequest(map[string]any{
				"function_name":  objectName,
				"function_group": functionGroup,
			}))
			return result, true, err

		case "FUGR":
			if objectName == "" {
				return newToolResultError("function_group is required"), true, nil
			}
			result, err := s.handleGetFunctionGroup(ctx, newRequest(map[string]any{"function_group": objectName}))
			return result, true, err

		case "DEVC":
			if objectName == "" {
				return newToolResultError("package_name is required"), true, nil
			}
			args := map[string]any{"package_name": objectName}
			if maxResults, ok := params["maxResults"].(float64); ok {
				args["maxResults"] = maxResults
			}
			if offset, ok := params["offset"].(float64); ok {
				args["offset"] = offset
			}
			result, err := s.handleGetPackage(ctx, newRequest(args))
			return result, true, err

		case "MSAG":
			if objectName == "" {
				return newToolResultError("message_class is required"), true, nil
			}
			result, err := s.handleGetMessages(ctx, newRequest(map[string]any{"message_class": objectName}))
			return result, true, err

		case "TRAN":
			if objectName == "" {
				return newToolResultError("transaction_name is required"), true, nil
			}
			result, err := s.handleGetTransaction(ctx, newRequest(map[string]any{"transaction_name": objectName}))
			return result, true, err

		case "TYPE_INFO":
			if objectName == "" {
				return newToolResultError("type_name is required"), true, nil
			}
			result, err := s.handleGetTypeInfo(ctx, newRequest(map[string]any{"type_name": objectName}))
			return result, true, err
		}

	case "query":
		switch objectType {
		case "TABL_CONTENTS":
			tableName := objectName
			if tableName == "" {
				tableName, _ = params["table_name"].(string)
			}
			if tableName == "" {
				return newToolResultError("table_name is required"), true, nil
			}
			args := map[string]any{"table_name": tableName}
			if maxRows, ok := params["max_rows"].(float64); ok {
				args["max_rows"] = maxRows
			}
			if sqlQuery, ok := params["sql_query"].(string); ok {
				args["sql_query"] = sqlQuery
			}
			result, err := s.handleGetTableContents(ctx, newRequest(args))
			return result, true, err

		case "SQL":
			sqlQuery, _ := params["sql_query"].(string)
			if sqlQuery == "" {
				return newToolResultError("sql_query is required in params"), true, nil
			}
			args := map[string]any{"sql_query": sqlQuery}
			if maxRows, ok := params["max_rows"].(float64); ok {
				args["max_rows"] = maxRows
			}
			result, err := s.handleRunQuery(ctx, newRequest(args))
			return result, true, err
		}

	case "analyze":
		analysisType, _ := params["type"].(string)
		if analysisType == "cds_dependencies" {
			ddlsName := objectName
			if ddlsName == "" {
				ddlsName, _ = params["ddls_name"].(string)
			}
			if ddlsName == "" {
				return newToolResultError("ddls_name is required"), true, nil
			}
			args := map[string]any{"ddls_name": ddlsName}
			if level, ok := params["dependency_level"].(string); ok {
				args["dependency_level"] = level
			}
			if assoc, ok := params["with_associations"].(bool); ok {
				args["with_associations"] = assoc
			}
			if pkg, ok := params["context_package"].(string); ok {
				args["context_package"] = pkg
			}
			result, err := s.handleGetCDSDependencies(ctx, newRequest(args))
			return result, true, err
		}
	}

	return nil, false, nil
}

// --- Read Handlers ---

func (s *Server) handleGetClass(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	className, ok := request.Params.Arguments["class_name"].(string)
	if !ok || className == "" {
		return newToolResultError("class_name is required"), nil
	}

	source, err := s.adtClient.GetClassSource(ctx, className)
	if err != nil {
		return wrapErr("GetClass", err), nil
	}

	return mcp.NewToolResultText(source), nil
}

func (s *Server) handleGetFunction(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	functionName, ok := request.Params.Arguments["function_name"].(string)
	if !ok || functionName == "" {
		return newToolResultError("function_name is required"), nil
	}

	functionGroup, ok := request.Params.Arguments["function_group"].(string)
	if !ok || functionGroup == "" {
		return newToolResultError("function_group is required"), nil
	}

	source, err := s.adtClient.GetFunction(ctx, functionName, functionGroup)
	if err != nil {
		return wrapErr("GetFunction", err), nil
	}

	return mcp.NewToolResultText(source), nil
}

func (s *Server) handleGetFunctionGroup(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	groupName, ok := request.Params.Arguments["function_group"].(string)
	if !ok || groupName == "" {
		return newToolResultError("function_group is required"), nil
	}

	fg, err := s.adtClient.GetFunctionGroup(ctx, groupName)
	if err != nil {
		return wrapErr("GetFunctionGroup", err), nil
	}
	return newToolResultJSON(fg), nil
}

func (s *Server) handleGetTableContents(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	tableName, ok := request.Params.Arguments["table_name"].(string)
	if !ok || tableName == "" {
		return newToolResultError("table_name is required"), nil
	}

	maxRows := 100
	if mr, ok := request.Params.Arguments["max_rows"].(float64); ok && mr > 0 {
		maxRows = int(mr)
	}

	sqlQuery := ""
	if sq, ok := request.Params.Arguments["sql_query"].(string); ok {
		sqlQuery = sq
	}

	contents, err := s.adtClient.GetTableContents(ctx, tableName, maxRows, sqlQuery)
	if err != nil {
		return wrapErr("GetTableContents", err), nil
	}
	return newToolResultJSON(contents), nil
}

func (s *Server) handleRunQuery(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	sqlQuery, ok := request.Params.Arguments["sql_query"].(string)
	if !ok || sqlQuery == "" {
		return newToolResultError("sql_query is required"), nil
	}

	maxRows := 100
	if mr, ok := request.Params.Arguments["max_rows"].(float64); ok && mr > 0 {
		maxRows = int(mr)
	}

	contents, err := s.adtClient.RunQuery(ctx, sqlQuery, maxRows)
	if err != nil {
		return wrapErr("RunQuery", err), nil
	}
	return newToolResultJSON(contents), nil
}

func (s *Server) handleGetCDSDependencies(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	ddlsName, ok := request.Params.Arguments["ddls_name"].(string)
	if !ok || ddlsName == "" {
		return newToolResultError("ddls_name is required"), nil
	}

	opts := adt.CDSDependencyOptions{
		DependencyLevel:  "hierarchy",
		WithAssociations: false,
	}

	if level, ok := request.Params.Arguments["dependency_level"].(string); ok && level != "" {
		opts.DependencyLevel = level
	}

	if assoc, ok := request.Params.Arguments["with_associations"].(bool); ok {
		opts.WithAssociations = assoc
	}

	if pkg, ok := request.Params.Arguments["context_package"].(string); ok && pkg != "" {
		opts.ContextPackage = pkg
	}

	dependencyTree, err := s.adtClient.GetCDSDependencies(ctx, ddlsName, opts)
	if err != nil {
		return wrapErr("GetCDSDependencies", err), nil
	}

	// Add metadata summary
	return newToolResultJSON(map[string]any{
		"ddls_name":       ddlsName,
		"dependency_tree": dependencyTree,
		"statistics": map[string]any{
			"total_dependencies":    len(dependencyTree.FlattenDependencies()) - 1, // -1 to exclude root
			"dependency_depth":      dependencyTree.GetDependencyDepth(),
			"by_type":               dependencyTree.CountDependenciesByType(),
			"table_dependencies":    len(dependencyTree.GetTableDependencies()),
			"inactive_dependencies": len(dependencyTree.GetInactiveDependencies()),
			"cycles":                dependencyTree.FindCycles(),
		},
	}), nil
}

func (s *Server) handleGetPackage(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	packageName, ok := request.Params.Arguments["package_name"].(string)
	if !ok || packageName == "" {
		return newToolResultError("package_name is required"), nil
	}

	opts := &adt.PackageQueryOptions{MaxObjects: 100}
	if mr, ok := request.Params.Arguments["maxResults"].(float64); ok && mr > 0 {
		opts.MaxObjects = int(mr)
	}
	if off, ok := request.Params.Arguments["offset"].(float64); ok && off > 0 {
		opts.Offset = int(off)
	}

	pkg, err := s.adtClient.GetPackage(ctx, packageName, opts)
	if err != nil {
		return wrapErr("GetPackage", err), nil
	}
	return newToolResultJSON(pkg), nil
}

func (s *Server) handleGetMessages(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	msgClass, ok := request.Params.Arguments["message_class"].(string)
	if !ok || msgClass == "" {
		return newToolResultError("message_class is required"), nil
	}

	mc, err := s.adtClient.GetMessageClass(ctx, msgClass)
	if err != nil {
		return wrapErr("GetMessageClass", err), nil
	}
	return newToolResultJSON(mc), nil
}

func (s *Server) handleGetTransaction(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	tcode, ok := request.Params.Arguments["transaction_name"].(string)
	if !ok || tcode == "" {
		return newToolResultError("transaction_name is required"), nil
	}

	tran, err := s.adtClient.GetTransaction(ctx, tcode)
	if err != nil {
		return wrapErr("GetTransaction", err), nil
	}
	return newToolResultJSON(tran), nil
}

func (s *Server) handleGetTypeInfo(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	typeName, ok := request.Params.Arguments["type_name"].(string)
	if !ok || typeName == "" {
		return newToolResultError("type_name is required"), nil
	}

	typeInfo, err := s.adtClient.GetTypeInfo(ctx, typeName)
	if err != nil {
		return wrapErr("GetTypeInfo", err), nil
	}
	return newToolResultJSON(typeInfo), nil
}
