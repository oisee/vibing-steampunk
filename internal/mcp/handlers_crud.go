// Package mcp provides the MCP server implementation for ABAP ADT tools.
// handlers_crud.go contains handlers for CRUD operations (lock, unlock, create, update, delete).
package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/oisee/vibing-steampunk/pkg/adt"
)

// --- CRUD Handlers ---

func (s *Server) handleLockObject(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	objectURL, ok := request.GetArguments()["object_url"].(string)
	if !ok || objectURL == "" {
		return newToolResultError("object_url is required"), nil
	}

	accessMode := "MODIFY"
	if am, ok := request.GetArguments()["access_mode"].(string); ok && am != "" {
		accessMode = am
	}

	result, err := s.adtClient.LockObject(ctx, objectURL, accessMode)
	if err != nil {
		return newToolResultError(fmt.Sprintf("Failed to lock object: %v", err)), nil
	}

	output, _ := json.MarshalIndent(result, "", "  ")
	return mcp.NewToolResultText(string(output)), nil
}

func (s *Server) handleUnlockObject(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	objectURL, ok := request.GetArguments()["object_url"].(string)
	if !ok || objectURL == "" {
		return newToolResultError("object_url is required"), nil
	}

	lockHandle, ok := request.GetArguments()["lock_handle"].(string)
	if !ok || lockHandle == "" {
		return newToolResultError("lock_handle is required"), nil
	}

	err := s.adtClient.UnlockObject(ctx, objectURL, lockHandle)
	if err != nil {
		return newToolResultError(fmt.Sprintf("Failed to unlock object: %v", err)), nil
	}

	return mcp.NewToolResultText("Object unlocked successfully"), nil
}

func (s *Server) handleUpdateSource(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	objectURL, ok := request.GetArguments()["object_url"].(string)
	if !ok || objectURL == "" {
		return newToolResultError("object_url is required"), nil
	}

	source, ok := request.GetArguments()["source"].(string)
	if !ok || source == "" {
		return newToolResultError("source is required"), nil
	}

	lockHandle, ok := request.GetArguments()["lock_handle"].(string)
	if !ok || lockHandle == "" {
		return newToolResultError("lock_handle is required"), nil
	}

	transport := ""
	if t, ok := request.GetArguments()["transport"].(string); ok {
		transport = t
	}

	// Append /source/main to object URL if not already present
	sourceURL := objectURL
	if !strings.HasSuffix(sourceURL, "/source/main") {
		sourceURL = objectURL + "/source/main"
	}

	err := s.adtClient.UpdateSource(ctx, sourceURL, source, lockHandle, transport)
	if err != nil {
		return newToolResultError(fmt.Sprintf("Failed to update source: %v", err)), nil
	}

	return mcp.NewToolResultText("Source updated successfully"), nil
}

func (s *Server) handleCreateObject(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	objectType, ok := request.GetArguments()["object_type"].(string)
	if !ok || objectType == "" {
		return newToolResultError("object_type is required"), nil
	}

	name, ok := request.GetArguments()["name"].(string)
	if !ok || name == "" {
		return newToolResultError("name is required"), nil
	}

	description, ok := request.GetArguments()["description"].(string)
	if !ok || description == "" {
		return newToolResultError("description is required"), nil
	}

	packageName, ok := request.GetArguments()["package_name"].(string)
	if !ok || packageName == "" {
		return newToolResultError("package_name is required"), nil
	}

	transport := ""
	if t, ok := request.GetArguments()["transport"].(string); ok {
		transport = t
	}

	parentName := ""
	if p, ok := request.GetArguments()["parent_name"].(string); ok {
		parentName = p
	}

	// RAP-specific options
	serviceDefinition := ""
	if sd, ok := request.GetArguments()["service_definition"].(string); ok {
		serviceDefinition = sd
	}
	bindingVersion := ""
	if bv, ok := request.GetArguments()["binding_version"].(string); ok {
		bindingVersion = bv
	}
	bindingCategory := ""
	if bc, ok := request.GetArguments()["binding_category"].(string); ok {
		bindingCategory = bc
	}

	opts := adt.CreateObjectOptions{
		ObjectType:        adt.CreatableObjectType(objectType),
		Name:              name,
		Description:       description,
		PackageName:       packageName,
		Transport:         transport,
		ParentName:        parentName,
		ServiceDefinition: serviceDefinition,
		BindingVersion:    bindingVersion,
		BindingCategory:   bindingCategory,
	}

	err := s.adtClient.CreateObject(ctx, opts)
	if err != nil {
		return newToolResultError(fmt.Sprintf("Failed to create object: %v", err)), nil
	}

	// Return the object URL for convenience
	objURL := adt.GetObjectURL(opts.ObjectType, opts.Name, opts.ParentName)
	result := map[string]string{
		"status":     "created",
		"object_url": objURL,
	}
	output, _ := json.MarshalIndent(result, "", "  ")
	return mcp.NewToolResultText(string(output)), nil
}

func (s *Server) handleCreatePackage(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	name, ok := request.GetArguments()["name"].(string)
	if !ok || name == "" {
		return newToolResultError("name is required"), nil
	}

	name = strings.ToUpper(name)

	description, ok := request.GetArguments()["description"].(string)
	if !ok || description == "" {
		return newToolResultError("description is required"), nil
	}

	parent := ""
	if p, ok := request.GetArguments()["parent"].(string); ok && p != "" {
		parent = strings.ToUpper(p)
	}

	transport := ""
	if t, ok := request.GetArguments()["transport"].(string); ok && t != "" {
		transport = t
	}

	softwareComponent := ""
	if sc, ok := request.GetArguments()["software_component"].(string); ok && sc != "" {
		softwareComponent = strings.ToUpper(sc)
	}

	// Transportable packages require transport parameter
	if !strings.HasPrefix(name, "$") && transport == "" {
		return newToolResultError("transport is required for creating transportable packages (non-$ packages). Use --enable-transports flag."), nil
	}

	opts := adt.CreateObjectOptions{
		ObjectType:        adt.ObjectTypePackage,
		Name:              name,
		Description:       description,
		PackageName:       parent, // Parent package
		Transport:         transport,
		SoftwareComponent: softwareComponent,
	}

	err := s.adtClient.CreateObject(ctx, opts)
	if err != nil {
		return newToolResultError(fmt.Sprintf("Failed to create package: %v", err)), nil
	}

	result := map[string]string{
		"status":      "created",
		"package":     name,
		"description": description,
	}
	if parent != "" {
		result["parent"] = parent
	}
	output, _ := json.MarshalIndent(result, "", "  ")
	return mcp.NewToolResultText(string(output)), nil
}

func (s *Server) handleCreateTable(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	name, ok := request.GetArguments()["name"].(string)
	if !ok || name == "" {
		return newToolResultError("name is required"), nil
	}

	description, ok := request.GetArguments()["description"].(string)
	if !ok || description == "" {
		return newToolResultError("description is required"), nil
	}

	fieldsJSON, ok := request.GetArguments()["fields"].(string)
	if !ok || fieldsJSON == "" {
		return newToolResultError("fields is required (JSON array)"), nil
	}

	// Parse fields JSON
	var fields []adt.TableField
	if err := json.Unmarshal([]byte(fieldsJSON), &fields); err != nil {
		return newToolResultError(fmt.Sprintf("Invalid fields JSON: %v", err)), nil
	}

	if len(fields) == 0 {
		return newToolResultError("At least one field is required"), nil
	}

	// Optional parameters
	pkg := "$TMP"
	if p, ok := request.GetArguments()["package"].(string); ok && p != "" {
		pkg = strings.ToUpper(p)
	}

	transport := ""
	if t, ok := request.GetArguments()["transport"].(string); ok && t != "" {
		transport = t
	}

	deliveryClass := "A"
	if dc, ok := request.GetArguments()["delivery_class"].(string); ok && dc != "" {
		deliveryClass = strings.ToUpper(dc)
	}

	opts := adt.CreateTableOptions{
		Name:          name,
		Description:   description,
		Package:       pkg,
		Fields:        fields,
		Transport:     transport,
		DeliveryClass: deliveryClass,
	}

	err := s.adtClient.CreateTable(ctx, opts)
	if err != nil {
		return newToolResultError(fmt.Sprintf("Failed to create table: %v", err)), nil
	}

	result := map[string]interface{}{
		"status":      "created",
		"table":       strings.ToUpper(name),
		"package":     pkg,
		"description": description,
		"fields":      len(fields),
	}
	output, _ := json.MarshalIndent(result, "", "  ")
	return mcp.NewToolResultText(string(output)), nil
}

func (s *Server) handleCompareSource(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	type1, _ := request.GetArguments()["type1"].(string)
	name1, _ := request.GetArguments()["name1"].(string)
	type2, _ := request.GetArguments()["type2"].(string)
	name2, _ := request.GetArguments()["name2"].(string)

	if type1 == "" || name1 == "" || type2 == "" || name2 == "" {
		return newToolResultError("type1, name1, type2, and name2 are all required"), nil
	}

	// Build options for first object
	opts1 := &adt.GetSourceOptions{}
	if inc, ok := request.GetArguments()["include1"].(string); ok && inc != "" {
		opts1.Include = inc
	}
	if parent, ok := request.GetArguments()["parent1"].(string); ok && parent != "" {
		opts1.Parent = parent
	}

	// Build options for second object
	opts2 := &adt.GetSourceOptions{}
	if inc, ok := request.GetArguments()["include2"].(string); ok && inc != "" {
		opts2.Include = inc
	}
	if parent, ok := request.GetArguments()["parent2"].(string); ok && parent != "" {
		opts2.Parent = parent
	}

	diff, err := s.adtClient.CompareSource(ctx, type1, name1, type2, name2, opts1, opts2)
	if err != nil {
		return newToolResultError(fmt.Sprintf("CompareSource failed: %v", err)), nil
	}

	output, _ := json.MarshalIndent(diff, "", "  ")
	return mcp.NewToolResultText(string(output)), nil
}

func (s *Server) handleCloneObject(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	objectType, _ := request.GetArguments()["object_type"].(string)
	sourceName, _ := request.GetArguments()["source_name"].(string)
	targetName, _ := request.GetArguments()["target_name"].(string)
	pkg, _ := request.GetArguments()["package"].(string)

	if objectType == "" || sourceName == "" || targetName == "" || pkg == "" {
		return newToolResultError("object_type, source_name, target_name, and package are all required"), nil
	}

	result, err := s.adtClient.CloneObject(ctx, objectType, sourceName, targetName, pkg)
	if err != nil {
		return newToolResultError(fmt.Sprintf("CloneObject failed: %v", err)), nil
	}

	output, _ := json.MarshalIndent(result, "", "  ")
	return mcp.NewToolResultText(string(output)), nil
}

func (s *Server) handleGetClassInfo(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	className, _ := request.GetArguments()["class_name"].(string)
	if className == "" {
		return newToolResultError("class_name is required"), nil
	}

	info, err := s.adtClient.GetClassInfo(ctx, className)
	if err != nil {
		return newToolResultError(fmt.Sprintf("GetClassInfo failed: %v", err)), nil
	}

	output, _ := json.MarshalIndent(info, "", "  ")
	return mcp.NewToolResultText(string(output)), nil
}

func (s *Server) handleDeleteObject(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	objectURL, ok := request.GetArguments()["object_url"].(string)
	if !ok || objectURL == "" {
		return newToolResultError("object_url is required"), nil
	}

	lockHandle, ok := request.GetArguments()["lock_handle"].(string)
	if !ok || lockHandle == "" {
		return newToolResultError("lock_handle is required"), nil
	}

	transport := ""
	if t, ok := request.GetArguments()["transport"].(string); ok {
		transport = t
	}

	err := s.adtClient.DeleteObject(ctx, objectURL, lockHandle, transport)
	if err != nil {
		return newToolResultError(fmt.Sprintf("Failed to delete object: %v", err)), nil
	}

	return mcp.NewToolResultText("Object deleted successfully"), nil
}

func (s *Server) handleMoveObject(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	objectType, ok := request.GetArguments()["object_type"].(string)
	if !ok || objectType == "" {
		return newToolResultError("object_type is required"), nil
	}

	objectName, ok := request.GetArguments()["object_name"].(string)
	if !ok || objectName == "" {
		return newToolResultError("object_name is required"), nil
	}

	newPackage, ok := request.GetArguments()["new_package"].(string)
	if !ok || newPackage == "" {
		return newToolResultError("new_package is required"), nil
	}

	// Ensure WebSocket client is connected
	if err := s.ensureDebugWSClient(ctx); err != nil {
		return newToolResultError(fmt.Sprintf("Failed to connect to ZADT_VSP WebSocket: %v. Ensure ZADT_VSP is deployed and SAPC/SICF are configured.", err)), nil
	}

	result, err := s.debugWSClient.MoveObject(ctx, objectType, objectName, newPackage)
	if err != nil {
		return newToolResultError(fmt.Sprintf("MoveObject failed: %v", err)), nil
	}

	// Format result
	if result.Success {
		return mcp.NewToolResultText(fmt.Sprintf("Object moved successfully.\n\nObject: %s %s\nNew Package: %s\nMessage: %s",
			result.Object, result.ObjName, result.NewPackage, result.Message)), nil
	}
	return newToolResultError(fmt.Sprintf("Move failed: %s", result.Message)), nil
}
