// Package mcp provides the MCP server implementation for ABAP ADT tools.
// handlers_crud.go contains handlers for CRUD operations (lock, unlock, create, update, delete).
package mcp

import (
	"context"
	"encoding/json"
	"strings"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/oisee/vibing-steampunk/pkg/adt"
)

// --- CRUD Routing ---
// Routes for this module:
//   edit:    CLAS, INTF, PROG, DDLS, BDEF, SRVD, SRVB (with params.source) - high-level write
//   edit:    LOCK, UNLOCK, UPDATE_SOURCE, MOVE - low-level operations
//   create:  OBJECT, DEVC, TABL, CLONE
//   delete:  OBJECT
//   read:    CLAS_INFO
//   analyze: compare_source

// routeCRUDAction routes CRUD operations (lock, unlock, create, update, delete).
// Returns (result, true) if handled, (nil, false) if not handled by this module.
func (s *Server) routeCRUDAction(ctx context.Context, action, objectType, objectName string, params map[string]any) (*mcp.CallToolResult, bool, error) {
	switch action {
	case "edit":
		switch objectType {
		case "LOCK":
			objectURL := getStr(params, "object_url")
			if objectURL == "" {
				return newToolResultError("object_url is required in params"), true, nil
			}
			args := map[string]any{"object_url": objectURL}
			copyOptional(args, params, "access_mode")
			result, err := s.handleLockObject(ctx, newRequest(args))
			return result, true, err

		case "UNLOCK":
			objectURL := getStr(params, "object_url")
			lockHandle := getStr(params, "lock_handle")
			if objectURL == "" || lockHandle == "" {
				return newToolResultError("object_url and lock_handle are required in params"), true, nil
			}
			result, err := s.handleUnlockObject(ctx, newRequest(map[string]any{
				"object_url":  objectURL,
				"lock_handle": lockHandle,
			}))
			return result, true, err

		case "UPDATE_SOURCE":
			objectURL := getStr(params, "object_url")
			source := getStr(params, "source")
			lockHandle := getStr(params, "lock_handle")
			if objectURL == "" || source == "" || lockHandle == "" {
				return newToolResultError("object_url, source, and lock_handle are required in params"), true, nil
			}
			args := map[string]any{
				"object_url":  objectURL,
				"source":      source,
				"lock_handle": lockHandle,
			}
			copyOptional(args, params, "transport")
			result, err := s.handleUpdateSource(ctx, newRequest(args))
			return result, true, err

		case "MOVE":
			objType := getStr(params, "object_type")
			objName := objectName
			if objName == "" {
				objName = getStr(params, "object_name")
			}
			newPackage := getStr(params, "new_package")
			if objType == "" || objName == "" || newPackage == "" {
				return newToolResultError("object_type, object_name, and new_package are required"), true, nil
			}
			result, err := s.handleMoveObject(ctx, newRequest(map[string]any{
				"object_type": objType,
				"object_name": objName,
				"new_package": newPackage,
			}))
			return result, true, err

		case "CLAS", "INTF", "PROG", "DDLS", "BDEF", "SRVD", "SRVB":
			if objectName == "" {
				return newToolResultError("object name is required"), true, nil
			}
			source := getStr(params, "source")
			if source == "" {
				return newToolResultError("source is required in params"), true, nil
			}
			opts := &adt.WriteSourceOptions{
				Mode:        adt.WriteModeUpsert,
				Package:     getStr(params, "package"),
				Description: getStr(params, "description"),
				Transport:   getStr(params, "transport"),
				TestSource:  getStr(params, "test_source"),
			}
			if opts.Package == "" {
				opts.Package = "$TMP"
			}
			writeResult, err := s.adtClient.WriteSource(ctx, objectType, objectName, source, opts)
			if err != nil {
				return wrapErr("WriteSource", err), true, nil
			}
			return newToolResultJSON(writeResult), true, nil
		}

	case "create":
		switch objectType {
		case "OBJECT":
			objType := getStr(params, "object_type")
			name := objectName
			if name == "" {
				name = getStr(params, "name")
			}
			description := getStr(params, "description")
			packageName := getStr(params, "package_name")
			if objType == "" || name == "" || description == "" || packageName == "" {
				return newToolResultError("object_type, name, description, and package_name are required"), true, nil
			}
			args := map[string]any{
				"object_type":  objType,
				"name":         name,
				"description":  description,
				"package_name": packageName,
			}
			copyOptional(args, params, "transport", "parent_name", "service_definition", "binding_version", "binding_category")
			result, err := s.handleCreateObject(ctx, newRequest(args))
			return result, true, err

		case "DEVC":
			name := objectName
			if name == "" {
				name = getStr(params, "name")
			}
			description := getStr(params, "description")
			if name == "" || description == "" {
				return newToolResultError("name and description are required"), true, nil
			}
			args := map[string]any{
				"name":        name,
				"description": description,
			}
			copyOptional(args, params, "parent")
			result, err := s.handleCreatePackage(ctx, newRequest(args))
			return result, true, err

		case "TABL":
			name := objectName
			if name == "" {
				name = getStr(params, "name")
			}
			description := getStr(params, "description")
			fields := getStr(params, "fields")
			if name == "" || description == "" || fields == "" {
				return newToolResultError("name, description, and fields (JSON) are required"), true, nil
			}
			args := map[string]any{
				"name":        name,
				"description": description,
				"fields":      fields,
			}
			copyOptional(args, params, "package", "transport", "delivery_class")
			result, err := s.handleCreateTable(ctx, newRequest(args))
			return result, true, err

		case "CLONE":
			objType := getStr(params, "object_type")
			sourceName := objectName
			if sourceName == "" {
				sourceName = getStr(params, "source_name")
			}
			targetName := getStr(params, "target_name")
			pkg := getStr(params, "package")
			if objType == "" || sourceName == "" || targetName == "" || pkg == "" {
				return newToolResultError("object_type, source_name, target_name, and package are required"), true, nil
			}
			result, err := s.handleCloneObject(ctx, newRequest(map[string]any{
				"object_type": objType,
				"source_name": sourceName,
				"target_name": targetName,
				"package":     pkg,
			}))
			return result, true, err
		}

	case "delete":
		if objectType == "OBJECT" {
			objectURL := getStr(params, "object_url")
			lockHandle := getStr(params, "lock_handle")
			if objectURL == "" || lockHandle == "" {
				return newToolResultError("object_url and lock_handle are required in params"), true, nil
			}
			args := map[string]any{
				"object_url":  objectURL,
				"lock_handle": lockHandle,
			}
			copyOptional(args, params, "transport")
			result, err := s.handleDeleteObject(ctx, newRequest(args))
			return result, true, err
		}

	case "read":
		if objectType == "CLAS_INFO" {
			className := objectName
			if className == "" {
				className = getStr(params, "class_name")
			}
			if className == "" {
				return newToolResultError("class_name is required"), true, nil
			}
			result, err := s.handleGetClassInfo(ctx, newRequest(map[string]any{"class_name": className}))
			return result, true, err
		}

	case "analyze":
		analysisType := getStr(params, "type")
		if analysisType == "compare_source" {
			type1 := getStr(params, "type1")
			name1 := getStr(params, "name1")
			type2 := getStr(params, "type2")
			name2 := getStr(params, "name2")
			if type1 == "" || name1 == "" || type2 == "" || name2 == "" {
				return newToolResultError("type1, name1, type2, and name2 are required in params"), true, nil
			}
			args := map[string]any{
				"type1": type1,
				"name1": name1,
				"type2": type2,
				"name2": name2,
			}
			copyOptional(args, params, "include1", "parent1", "include2", "parent2")
			result, err := s.handleCompareSource(ctx, newRequest(args))
			return result, true, err
		}
	}

	return nil, false, nil
}

// --- CRUD Handlers ---

func (s *Server) handleLockObject(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	args := request.Params.Arguments
	objectURL, errResult := requireStr(args, "object_url")
	if errResult != nil {
		return errResult, nil
	}

	accessMode := getStr(args, "access_mode")
	if accessMode == "" {
		accessMode = "MODIFY"
	}

	result, err := s.adtClient.LockObject(ctx, objectURL, accessMode)
	if err != nil {
		return wrapErr("LockObject", err), nil
	}
	return newToolResultJSON(result), nil
}

func (s *Server) handleUnlockObject(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	args := request.Params.Arguments
	objectURL, errResult := requireStr(args, "object_url")
	if errResult != nil {
		return errResult, nil
	}
	lockHandle, errResult := requireStr(args, "lock_handle")
	if errResult != nil {
		return errResult, nil
	}

	err := s.adtClient.UnlockObject(ctx, objectURL, lockHandle)
	if err != nil {
		return wrapErr("UnlockObject", err), nil
	}
	return mcp.NewToolResultText("Object unlocked successfully"), nil
}

func (s *Server) handleUpdateSource(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	args := request.Params.Arguments
	objectURL, errResult := requireStr(args, "object_url")
	if errResult != nil {
		return errResult, nil
	}
	source, errResult := requireStr(args, "source")
	if errResult != nil {
		return errResult, nil
	}
	lockHandle, errResult := requireStr(args, "lock_handle")
	if errResult != nil {
		return errResult, nil
	}
	transport := getStr(args, "transport")

	// Append /source/main to object URL if not already present
	sourceURL := objectURL
	if !strings.HasSuffix(sourceURL, "/source/main") {
		sourceURL = objectURL + "/source/main"
	}

	err := s.adtClient.UpdateSource(ctx, sourceURL, source, lockHandle, transport)
	if err != nil {
		return wrapErr("UpdateSource", err), nil
	}
	return mcp.NewToolResultText("Source updated successfully"), nil
}

func (s *Server) handleCreateObject(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	args := request.Params.Arguments
	objectType, errResult := requireStr(args, "object_type")
	if errResult != nil {
		return errResult, nil
	}
	name, errResult := requireStr(args, "name")
	if errResult != nil {
		return errResult, nil
	}
	description, errResult := requireStr(args, "description")
	if errResult != nil {
		return errResult, nil
	}
	packageName, errResult := requireStr(args, "package_name")
	if errResult != nil {
		return errResult, nil
	}

	opts := adt.CreateObjectOptions{
		ObjectType:        adt.CreatableObjectType(objectType),
		Name:              name,
		Description:       description,
		PackageName:       packageName,
		Transport:         getStr(args, "transport"),
		ParentName:        getStr(args, "parent_name"),
		ServiceDefinition: getStr(args, "service_definition"),
		BindingVersion:    getStr(args, "binding_version"),
		BindingCategory:   getStr(args, "binding_category"),
	}

	err := s.adtClient.CreateObject(ctx, opts)
	if err != nil {
		return wrapErr("CreateObject", err), nil
	}

	// Return the object URL for convenience
	objURL := adt.GetObjectURL(opts.ObjectType, opts.Name, opts.ParentName)
	return newToolResultJSON(map[string]string{
		"status":     "created",
		"object_url": objURL,
	}), nil
}

func (s *Server) handleCreatePackage(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	args := request.Params.Arguments
	name, errResult := requireStr(args, "name")
	if errResult != nil {
		return errResult, nil
	}
	name = strings.ToUpper(name)
	if !strings.HasPrefix(name, "$") {
		return newToolResultError("package name must start with $ (local packages only)"), nil
	}

	description, errResult := requireStr(args, "description")
	if errResult != nil {
		return errResult, nil
	}
	parent := strings.ToUpper(getStr(args, "parent"))

	opts := adt.CreateObjectOptions{
		ObjectType:  adt.ObjectTypePackage,
		Name:        name,
		Description: description,
		PackageName: parent,
	}

	err := s.adtClient.CreateObject(ctx, opts)
	if err != nil {
		return wrapErr("CreatePackage", err), nil
	}

	result := map[string]string{
		"status":      "created",
		"package":     name,
		"description": description,
	}
	if parent != "" {
		result["parent"] = parent
	}
	return newToolResultJSON(result), nil
}

func (s *Server) handleCreateTable(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	args := request.Params.Arguments
	name, errResult := requireStr(args, "name")
	if errResult != nil {
		return errResult, nil
	}
	description, errResult := requireStr(args, "description")
	if errResult != nil {
		return errResult, nil
	}
	fieldsJSON, errResult := requireStr(args, "fields")
	if errResult != nil {
		return newToolResultError("fields is required (JSON array)"), nil
	}

	var fields []adt.TableField
	if err := json.Unmarshal([]byte(fieldsJSON), &fields); err != nil {
		return wrapErr("ParseFieldsJSON", err), nil
	}
	if len(fields) == 0 {
		return newToolResultError("At least one field is required"), nil
	}

	pkg := getStr(args, "package")
	if pkg == "" {
		pkg = "$TMP"
	} else {
		pkg = strings.ToUpper(pkg)
	}
	deliveryClass := getStr(args, "delivery_class")
	if deliveryClass == "" {
		deliveryClass = "A"
	} else {
		deliveryClass = strings.ToUpper(deliveryClass)
	}

	opts := adt.CreateTableOptions{
		Name:          name,
		Description:   description,
		Package:       pkg,
		Fields:        fields,
		Transport:     getStr(args, "transport"),
		DeliveryClass: deliveryClass,
	}

	err := s.adtClient.CreateTable(ctx, opts)
	if err != nil {
		return wrapErr("CreateTable", err), nil
	}

	return newToolResultJSON(map[string]any{
		"status":      "created",
		"table":       strings.ToUpper(name),
		"package":     pkg,
		"description": description,
		"fields":      len(fields),
	}), nil
}

func (s *Server) handleCompareSource(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	args := request.Params.Arguments
	type1, errResult := requireStr(args, "type1")
	if errResult != nil {
		return errResult, nil
	}
	name1, errResult := requireStr(args, "name1")
	if errResult != nil {
		return errResult, nil
	}
	type2, errResult := requireStr(args, "type2")
	if errResult != nil {
		return errResult, nil
	}
	name2, errResult := requireStr(args, "name2")
	if errResult != nil {
		return errResult, nil
	}

	opts1 := &adt.GetSourceOptions{
		Include: getStr(args, "include1"),
		Parent:  getStr(args, "parent1"),
	}
	opts2 := &adt.GetSourceOptions{
		Include: getStr(args, "include2"),
		Parent:  getStr(args, "parent2"),
	}

	diff, err := s.adtClient.CompareSource(ctx, type1, name1, type2, name2, opts1, opts2)
	if err != nil {
		return wrapErr("CompareSource", err), nil
	}
	return newToolResultJSON(diff), nil
}

func (s *Server) handleCloneObject(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	args := request.Params.Arguments
	objectType, errResult := requireStr(args, "object_type")
	if errResult != nil {
		return errResult, nil
	}
	sourceName, errResult := requireStr(args, "source_name")
	if errResult != nil {
		return errResult, nil
	}
	targetName, errResult := requireStr(args, "target_name")
	if errResult != nil {
		return errResult, nil
	}
	pkg, errResult := requireStr(args, "package")
	if errResult != nil {
		return errResult, nil
	}

	result, err := s.adtClient.CloneObject(ctx, objectType, sourceName, targetName, pkg)
	if err != nil {
		return wrapErr("CloneObject", err), nil
	}
	return newToolResultJSON(result), nil
}

func (s *Server) handleGetClassInfo(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	className, errResult := requireStr(request.Params.Arguments, "class_name")
	if errResult != nil {
		return errResult, nil
	}

	info, err := s.adtClient.GetClassInfo(ctx, className)
	if err != nil {
		return wrapErr("GetClassInfo", err), nil
	}
	return newToolResultJSON(info), nil
}

func (s *Server) handleDeleteObject(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	args := request.Params.Arguments
	objectURL, errResult := requireStr(args, "object_url")
	if errResult != nil {
		return errResult, nil
	}
	lockHandle, errResult := requireStr(args, "lock_handle")
	if errResult != nil {
		return errResult, nil
	}

	err := s.adtClient.DeleteObject(ctx, objectURL, lockHandle, getStr(args, "transport"))
	if err != nil {
		return wrapErr("DeleteObject", err), nil
	}
	return mcp.NewToolResultText("Object deleted successfully"), nil
}

func (s *Server) handleMoveObject(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	args := request.Params.Arguments
	objectType, errResult := requireStr(args, "object_type")
	if errResult != nil {
		return errResult, nil
	}
	objectName, errResult := requireStr(args, "object_name")
	if errResult != nil {
		return errResult, nil
	}
	newPackage, errResult := requireStr(args, "new_package")
	if errResult != nil {
		return errResult, nil
	}

	if err := s.ensureDebugWSClient(ctx); err != nil {
		return wrapErr("ConnectWebSocket", err), nil
	}

	result, err := s.debugWSClient.MoveObject(ctx, objectType, objectName, newPackage)
	if err != nil {
		return wrapErr("MoveObject", err), nil
	}

	if result.Success {
		return newToolResultJSON(map[string]any{
			"status":      "success",
			"object":      result.Object,
			"object_name": result.ObjName,
			"new_package": result.NewPackage,
			"message":     result.Message,
		}), nil
	}
	return newToolResultError("Move failed: " + result.Message), nil
}
