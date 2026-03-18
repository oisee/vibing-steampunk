// Package mcp provides the MCP server implementation for ABAP ADT tools.
// handlers_read.go contains handlers for read operations (GetProgram, GetClass, GetTable, etc.)
package mcp

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/oisee/vibing-steampunk/pkg/adt"
)

// --- Read Handlers ---

func (s *Server) handleGetProgram(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	programName, ok := request.Params.Arguments["program_name"].(string)
	if !ok || programName == "" {
		return newToolResultError("program_name is required"), nil
	}

	source, err := s.adtClient.GetProgram(ctx, programName)
	if err != nil {
		return newToolResultError(fmt.Sprintf("Failed to get program: %v", err)), nil
	}

	return mcp.NewToolResultText(source), nil
}

func (s *Server) handleGetClass(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	className, ok := request.Params.Arguments["class_name"].(string)
	if !ok || className == "" {
		return newToolResultError("class_name is required"), nil
	}

	source, err := s.adtClient.GetClassSource(ctx, className)
	if err != nil {
		return newToolResultError(fmt.Sprintf("Failed to get class: %v", err)), nil
	}

	return mcp.NewToolResultText(source), nil
}

func (s *Server) handleGetInterface(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	interfaceName, ok := request.Params.Arguments["interface_name"].(string)
	if !ok || interfaceName == "" {
		return newToolResultError("interface_name is required"), nil
	}

	source, err := s.adtClient.GetInterface(ctx, interfaceName)
	if err != nil {
		return newToolResultError(fmt.Sprintf("Failed to get interface: %v", err)), nil
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
		return newToolResultError(fmt.Sprintf("Failed to get function: %v", err)), nil
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
		return newToolResultError(fmt.Sprintf("Failed to get function group: %v", err)), nil
	}

	result, _ := json.MarshalIndent(fg, "", "  ")
	return mcp.NewToolResultText(string(result)), nil
}

func (s *Server) handleGetInclude(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	includeName, ok := request.Params.Arguments["include_name"].(string)
	if !ok || includeName == "" {
		return newToolResultError("include_name is required"), nil
	}

	source, err := s.adtClient.GetInclude(ctx, includeName)
	if err != nil {
		return newToolResultError(fmt.Sprintf("Failed to get include: %v", err)), nil
	}

	return mcp.NewToolResultText(source), nil
}

func (s *Server) handleGetTable(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	tableName, ok := request.Params.Arguments["table_name"].(string)
	if !ok || tableName == "" {
		return newToolResultError("table_name is required"), nil
	}

	source, err := s.adtClient.GetTable(ctx, tableName)
	if err != nil {
		return newToolResultError(fmt.Sprintf("Failed to get table: %v", err)), nil
	}

	return mcp.NewToolResultText(source), nil
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

	offset := 0
	if o, ok := request.Params.Arguments["offset"].(float64); ok && o > 0 {
		offset = int(o)
	}

	sqlQuery := ""
	if sq, ok := request.Params.Arguments["sql_query"].(string); ok {
		sqlQuery = sq
	}

	columnsOnly := false
	if co, ok := request.Params.Arguments["columns_only"].(bool); ok {
		columnsOnly = co
	}

	contents, err := s.adtClient.GetTableContentsWithOptions(ctx, tableName, &adt.GetTableContentsOptions{
		MaxRows:     maxRows,
		Offset:      offset,
		SQLFilter:   sqlQuery,
		ColumnsOnly: columnsOnly,
	})
	if err != nil {
		return newToolResultError(fmt.Sprintf("Failed to get table contents: %v", err)), nil
	}

	result, _ := json.MarshalIndent(contents, "", "  ")
	return mcp.NewToolResultText(string(result)), nil
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
		return newToolResultError(fmt.Sprintf("Failed to run query: %v", err)), nil
	}

	result, _ := json.MarshalIndent(contents, "", "  ")
	return mcp.NewToolResultText(string(result)), nil
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
		return newToolResultError(fmt.Sprintf("Failed to get CDS dependencies: %v", err)), nil
	}

	// Add metadata summary
	summary := map[string]interface{}{
		"ddls_name":       ddlsName,
		"dependency_tree": dependencyTree,
		"statistics": map[string]interface{}{
			"total_dependencies":    len(dependencyTree.FlattenDependencies()) - 1, // -1 to exclude root
			"dependency_depth":      dependencyTree.GetDependencyDepth(),
			"by_type":               dependencyTree.CountDependenciesByType(),
			"table_dependencies":    len(dependencyTree.GetTableDependencies()),
			"inactive_dependencies": len(dependencyTree.GetInactiveDependencies()),
			"cycles":                dependencyTree.FindCycles(),
		},
	}

	jsonResult, _ := json.MarshalIndent(summary, "", "  ")
	return mcp.NewToolResultText(string(jsonResult)), nil
}

func (s *Server) handleGetStructure(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	structName, ok := request.Params.Arguments["structure_name"].(string)
	if !ok || structName == "" {
		return newToolResultError("structure_name is required"), nil
	}

	source, err := s.adtClient.GetStructure(ctx, structName)
	if err != nil {
		return newToolResultError(fmt.Sprintf("Failed to get structure: %v", err)), nil
	}

	return mcp.NewToolResultText(source), nil
}

func (s *Server) handleGetPackage(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	packageName, ok := request.Params.Arguments["package_name"].(string)
	if !ok || packageName == "" {
		return newToolResultError("package_name is required"), nil
	}

	pkg, err := s.adtClient.GetPackage(ctx, packageName)
	if err != nil {
		return newToolResultError(fmt.Sprintf("Failed to get package: %v", err)), nil
	}

	result, _ := json.MarshalIndent(pkg, "", "  ")
	return mcp.NewToolResultText(string(result)), nil
}

func (s *Server) handleGetMessages(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	msgClass, ok := request.Params.Arguments["message_class"].(string)
	if !ok || msgClass == "" {
		return newToolResultError("message_class is required"), nil
	}

	mc, err := s.adtClient.GetMessageClass(ctx, msgClass)
	if err != nil {
		return newToolResultError(fmt.Sprintf("Failed to get message class: %v", err)), nil
	}

	result, _ := json.MarshalIndent(mc, "", "  ")
	return mcp.NewToolResultText(string(result)), nil
}

func (s *Server) handleGetTransaction(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	tcode, ok := request.Params.Arguments["transaction_name"].(string)
	if !ok || tcode == "" {
		return newToolResultError("transaction_name is required"), nil
	}

	tran, err := s.adtClient.GetTransaction(ctx, tcode)
	if err != nil {
		return newToolResultError(fmt.Sprintf("Failed to get transaction: %v", err)), nil
	}

	result, _ := json.MarshalIndent(tran, "", "  ")
	return mcp.NewToolResultText(string(result)), nil
}

func (s *Server) handleGetTypeInfo(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	typeName, ok := request.Params.Arguments["type_name"].(string)
	if !ok || typeName == "" {
		return newToolResultError("type_name is required"), nil
	}

	typeInfo, err := s.adtClient.GetTypeInfo(ctx, typeName)
	if err != nil {
		return newToolResultError(fmt.Sprintf("Failed to get type info: %v", err)), nil
	}

	result, _ := json.MarshalIndent(typeInfo, "", "  ")
	return mcp.NewToolResultText(string(result)), nil
}

