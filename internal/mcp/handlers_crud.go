// Package mcp provides the MCP server implementation for ABAP ADT tools.
// handlers_crud.go contains handlers for CRUD operations (lock, unlock, create, update, delete).
package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/mark3labs/mcp-go/mcp"

	"github.com/oisee/vibing-steampunk/pkg/adt"
)

// routeCRUDAction routes "edit" for LOCK/UNLOCK/UPDATE_SOURCE/
// RECOVER_FAILED_CREATE, "create" for OBJECT/DEVC/TABL/CLONE, "delete"
// for OBJECT.
func (s *Server) routeCRUDAction(ctx context.Context, action, objectType, objectName string, params map[string]any) (*mcp.CallToolResult, bool, error) {
	if action == "edit" {
		switch objectType {
		case "LOCK":
			return s.callHandler(ctx, s.handleLockObject, params)
		case "UNLOCK":
			return s.callHandler(ctx, s.handleUnlockObject, params)
		case "UPDATE_SOURCE":
			return s.callHandler(ctx, s.handleUpdateSource, params)
		case "MOVE":
			return s.callHandler(ctx, s.handleMoveObject, params)
		case "COMPARE_SOURCE":
			return s.callHandler(ctx, s.handleCompareSource, params)
		case "RECOVER_FAILED_CREATE":
			return s.callHandler(ctx, s.handleRecoverFailedCreate, params)
		}
	}

	if action == "create" {
		switch objectType {
		case "OBJECT":
			return s.callHandler(ctx, s.handleCreateObject, params)
		case "DEVC":
			return s.callHandler(ctx, s.handleCreatePackage, params)
		case "TABL":
			return s.callHandler(ctx, s.handleCreateTable, params)
		case "CLONE":
			return s.callHandler(ctx, s.handleCloneObject, params)
		case "ENHO":
			return s.callHandler(ctx, s.handleCreateSourceCodePlugin, params)
		case "BADI_IMPL":
			return s.callHandler(ctx, s.handleCreateBadiImplementation, params)
		case "DOMA":
			return s.callHandler(ctx, s.handleCreateDomain, withName(params, objectName))
		case "DTEL":
			return s.callHandler(ctx, s.handleCreateDataElement, withName(params, objectName))
		case "STRUCT", "APPEND":
			return s.callHandler(ctx, s.handleCreateStructure, withName(params, objectName))
		}
	}

	if action == "delete" {
		switch objectType {
		case "OBJECT", "":
			if getStringParam(params, "object_url") != "" {
				return s.callHandler(ctx, s.handleDeleteObject, params)
			}
		default:
			// delete <TYPE> <NAME>, as read, edit and create take it (#240).
			// A type with no delete here falls through, so UI5_FILE and
			// UI5_APP still reach routeUI5Action.
			if objectName != "" && deletableByName(objectType) {
				args := copyParams(params)
				args["object_type"] = objectType
				args["object_name"] = objectName
				return s.callHandler(ctx, s.handleDeleteByName, args)
			}
		}
	}

	// read CLASS_INFO
	if action == "read" && objectType == "CLASS_INFO" {
		return s.callHandler(ctx, s.handleGetClassInfo, map[string]any{"class_name": objectName})
	}

	return nil, false, nil
}

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

	transport, _ := request.GetArguments()["transport"].(string)

	result, err := s.adtClient.LockObject(ctx, objectURL, accessMode, transport)
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

	// Optional. Left empty, this call takes and releases its own lock, so the
	// handle never has to survive a model turn (#169).
	lockHandle := ""
	if lh, ok := request.GetArguments()["lock_handle"].(string); ok {
		lockHandle = lh
	}

	transport := ""
	if t, ok := request.GetArguments()["transport"].(string); ok {
		transport = t
	}

	// A class include (/oo/classes/<name>/includes/<include>) is written at
	// its own URL and locked through its class. Every other object takes
	// /source/main, appended if not already present (#242).
	sourceURL := objectURL
	classURL, includeSourceURL, isClassInclude, err := adt.SplitClassIncludeURL(objectURL)
	switch {
	case err != nil:
		return newToolResultError(fmt.Sprintf("Failed to update source: %v", err)), nil
	case isClassInclude:
		objectURL, sourceURL = classURL, includeSourceURL
	case !strings.HasSuffix(sourceURL, "/source/main"):
		sourceURL = objectURL + "/source/main"
	}

	updateCtx := ctx
	if lockHandle == "" {
		// Resolve and approve the package before acquiring the session-bound
		// lock. UpdateSource reuses this per-object marker, so it still runs
		// every policy check but does not issue a stateless SearchObject inside
		// the LOCK -> PUT -> UNLOCK window (#169).
		updateCtx, err = s.adtClient.PrepareSourceUpdate(ctx, objectURL, transport)
		if err != nil {
			return newToolResultError(fmt.Sprintf("Failed to update source: %v", err)), nil
		}
	}

	err = s.withObjectLock(updateCtx, objectURL, lockHandle, transport, func(handle string) error {
		return s.adtClient.UpdateSource(updateCtx, sourceURL, source, handle, transport)
	})
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

	rfcEnabled := false
	if r, ok := request.GetArguments()["rfc_enabled"].(bool); ok {
		rfcEnabled = r
	}
	source := ""
	if src, ok := request.GetArguments()["source"].(string); ok {
		source = src
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

	// A function module needs more than the creation POST: the creation
	// document's fmodule:processingType is ignored (the module comes back
	// "normal" however loudly the request asked for "rfc"), and its signature
	// lives in the source. CreateFunctionModule does the full flow — create,
	// flip the remote-enabled flag, write the source, activate — so
	// remote-enabled modules no longer need a hand-built shell in SE37.
	if opts.ObjectType == adt.ObjectTypeFunctionMod && (rfcEnabled || source != "") {
		if parentName == "" {
			return newToolResultError("parent_name (the function group) is required for FUGR/FF"), nil
		}
		fmResult, err := s.adtClient.CreateFunctionModule(ctx, adt.CreateFunctionModuleOptions{
			Group:       parentName,
			Name:        name,
			Description: description,
			PackageName: packageName,
			Transport:   transport,
			RFCEnabled:  rfcEnabled,
			Source:      source,
		})
		if err != nil {
			return newToolResultError(fmt.Sprintf("Failed to create function module: %v", err)), nil
		}
		output, _ := json.MarshalIndent(fmResult, "", "  ")
		return mcp.NewToolResultText(string(output)), nil
	}

	if opts.ObjectType == adt.ObjectTypeMessageClass {
		return s.createMessageClass(ctx, opts, request.GetArguments())
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
		FieldsJSON:    fieldsJSON,
		Transport:     transport,
		DeliveryClass: deliveryClass,
	}

	// client_dependent: true/false (or "true"/"false"); left out, a client key
	// field is added unless the first key field already is one. The fields
	// and this flag are handed over raw: CreateTable parses them after its
	// mutation gate (issue #254), so --read-only and --allowed-packages answer
	// a blocked caller before any complaint about the spec does.
	if v, present := request.GetArguments()["client_dependent"]; present && v != nil {
		var raw string
		switch t := v.(type) {
		case bool:
			raw = strconv.FormatBool(t)
		case string:
			raw = t
		default:
			raw = fmt.Sprintf("%v (%T)", v, v) // not a bool; refused after the gate
		}
		opts.ClientDependentArg = &raw
	}

	if err := s.adtClient.CreateTable(ctx, opts); err != nil {
		return newToolResultError(fmt.Sprintf("Failed to create table: %v", err)), nil
	}
	// CreateTable accepted these, so neither call can fail here.
	resolved, _ := adt.ResolveCreateTableSpec(opts)
	clientField, clientAdded, _ := adt.TableClientField(resolved)

	result := map[string]interface{}{
		"status":      "created",
		"table":       strings.ToUpper(name),
		"package":     pkg,
		"description": description,
		"fields":      len(resolved.Fields),
	}
	if clientField == "" {
		result["client_dependent"] = false
	} else {
		result["client_dependent"] = true
		result["client_field"] = clientField
		if clientAdded {
			result["client_field_added"] = true
		}
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

// handleRecoverFailedCreate is the MCP-facing recovery primitive for a
// zombie object that a previous CreateObject attempt left behind
// (e.g. after a 5xx where SAP persisted the skeleton before the HTTP
// response failed). The caller does NOT need a lock handle from the
// original session — the handler itself probes existence, acquires a
// fresh lock, and drives DeleteObject. See pkg/adt.RecoverFailedCreate
// and the partial-create RCA for the full story.
//
// Inputs (all required except transport / parent_name):
//
//	object_type   — CLAS / PROG / INTF / FUGR / DDLS / ...
//	name          — object name
//	package_name  — for the safety gate; must be in allowed list
//	parent_name   — required for FUNC (parent function group)
//	transport     — optional TR the zombie was attached to
//
// The output is a structured JSON document describing what was tried:
//
//	{
//	  "status": "cleaned" | "already_clean" | "partial" | "probe_failed",
//	  "object_url": "...",
//	  "package": "...",
//	  "transport": "...",
//	  "cleanup_actions": ["..."],
//	  "manual_steps":    ["..."]
//	}
//
// Manual steps are only populated when the cleanup could not finish
// — typically because another user holds a lock or because DeleteObject
// itself failed. The operator can copy those directly into SAPGUI.
func (s *Server) handleRecoverFailedCreate(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	objectType, ok := request.GetArguments()["object_type"].(string)
	if !ok || objectType == "" {
		return newToolResultError("object_type is required"), nil
	}
	name, ok := request.GetArguments()["name"].(string)
	if !ok || name == "" {
		return newToolResultError("name is required"), nil
	}
	packageName, ok := request.GetArguments()["package_name"].(string)
	if !ok || packageName == "" {
		return newToolResultError("package_name is required"), nil
	}
	parentName := ""
	if p, ok := request.GetArguments()["parent_name"].(string); ok {
		parentName = p
	}
	transport := ""
	if t, ok := request.GetArguments()["transport"].(string); ok {
		transport = t
	}

	opts := adt.CreateObjectOptions{
		ObjectType:  adt.CreatableObjectType(objectType),
		Name:        name,
		PackageName: packageName,
		ParentName:  parentName,
		Transport:   transport,
	}

	pce := s.adtClient.RecoverFailedCreate(ctx, opts)

	// Classify the outcome for the UI layer. The four cases correspond
	// to the four shapes RecoverFailedCreate returns:
	//
	//   cleaned        → probe found the object, cleanup succeeded
	//   already_clean  → probe found nothing, idempotent no-op
	//   partial        → probe found the object, cleanup could not finish
	//   probe_failed   → could not even determine whether the object exists
	status := "partial"
	switch {
	case pce.CleanupOK && len(pce.CleanupActions) == 1 &&
		strings.Contains(pce.CleanupActions[0], "nothing to recover"):
		status = "already_clean"
	case pce.CleanupOK:
		status = "cleaned"
	case pce.OriginalErr != nil && strings.Contains(pce.OriginalErr.Error(), "existence probe failed"):
		status = "probe_failed"
	}

	result := map[string]any{
		"status":          status,
		"object_url":      pce.ObjectURL,
		"package":         pce.Package,
		"transport":       pce.Transport,
		"cleanup_actions": pce.CleanupActions,
	}
	if len(pce.ManualSteps) > 0 {
		result["manual_steps"] = pce.ManualSteps
	}
	if pce.OriginalErr != nil {
		result["last_error"] = pce.OriginalErr.Error()
	}

	output, _ := json.MarshalIndent(result, "", "  ")
	return mcp.NewToolResultText(string(output)), nil
}

func (s *Server) handleDeleteObject(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	objectURL, ok := request.GetArguments()["object_url"].(string)
	if !ok || objectURL == "" {
		return newToolResultError("object_url is required"), nil
	}

	// Optional, as everywhere else: left empty this takes its own lock.
	lockHandle := ""
	if lh, ok := request.GetArguments()["lock_handle"].(string); ok {
		lockHandle = lh
	}

	transport := ""
	if t, ok := request.GetArguments()["transport"].(string); ok {
		transport = t
	}

	if lockHandle != "" {
		// A supplied handle was taken in an earlier call, so the caller owns
		// the lock and its release; that cross-call window is #169.
		if err := s.adtClient.DeleteObject(ctx, objectURL, lockHandle, transport); err != nil {
			return newToolResultError(fmt.Sprintf("Failed to delete object: %v", err)), nil
		}
		return mcp.NewToolResultText("Object deleted successfully"), nil
	}

	// The handler's own lock: DeleteObject's gate runs before the LOCK, since
	// under the lock its package lookup is a stateless request that retires
	// the session the handle belongs to and the DELETE comes back 423 (issue
	// #238). The UNLOCK goes out after a successful DELETE too: the DELETE
	// does not release the ENQUEUE, and the entry stays in SM12 for as long
	// as the ADT session lives.
	note, err := s.adtClient.DeleteObjectGated(ctx, objectURL, transport)
	if err != nil {
		return newToolResultError(fmt.Sprintf("Failed to delete object: %v", err)), nil
	}
	if note != "" {
		return mcp.NewToolResultText("Object deleted successfully; " + note), nil
	}
	return mcp.NewToolResultText("Object deleted successfully"), nil
}

// deleteNameTypes are the target types delete <TYPE> <NAME> resolves to an
// ADT URL without a lookup (FUNC looks up its group when none is given).
//
// DEVC is left out on purpose: whether --allowed-packages judges a package
// by its own name or by its parent's has not been pinned, and a delete gate
// that may be checking the wrong package is not one to hand a model. A
// package is still deleted by URL (delete OBJECT), or by git_delete_objects
// when empty.
var deleteNameTypes = []string{
	"PROG", "INCL", "CLAS", "INTF", "FUGR", "FUNC", "TABL", "STRUCT", "DTEL", "DOMA",
	"TTYP", "DDLS", "DCLS", "BDEF", "SRVD", "SRVB", "MSAG", "XSLT",
}

// deleteNameTypesLine is deleteNameTypes as help prints it.
var deleteNameTypesLine = strings.Join(deleteNameTypes, ", ")

// deletableByName reports whether delete <TYPE> <NAME> knows objectType.
func deletableByName(objectType string) bool {
	for _, t := range deleteNameTypes {
		if t == objectType {
			return true
		}
	}
	return false
}

// deleteNameRe is what an object name delete <TYPE> <NAME> accepts: letters,
// digits, _, $ and the / of a namespace. Nothing else -- not a dot, a space or
// a percent sign -- so the name can only ever be one path segment's worth of
// object, never a way to address something else.
var deleteNameRe = regexp.MustCompile(`^[A-Z0-9_/$]*[A-Z0-9][A-Z0-9_/$]*$`)

// validDeleteName reports whether name may go into a delete URL.
func validDeleteName(name string) bool {
	return deleteNameRe.MatchString(strings.ToUpper(name))
}

// deleteURLByName is the ADT URL delete <TYPE> <NAME> deletes at. parent is
// the function group of a FUNC; the other types ignore it.
func deleteURLByName(objectType, name, parent string) (string, bool) {
	switch objectType {
	case "INCL":
		return adt.GetObjectURL(adt.ObjectTypeInclude, name, ""), true
	case "FUNC":
		if parent == "" {
			return "", false
		}
		return adt.GetObjectURL(adt.ObjectTypeFunctionMod, name, parent), true
	case "STRUCT":
		return adt.StructureURL(name), true
	}
	return adt.GitObjectURL(objectType, name)
}

// handleDeleteByName is delete <TYPE> <NAME>: the object's URL is built from
// its type and name, and it is deleted the way handleDeleteObject deletes a
// URL. Without lock_handle it locks, deletes and unlocks in this one call.
func (s *Server) handleDeleteByName(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	args := request.GetArguments()
	objectType := strings.ToUpper(strings.TrimSpace(getStringParam(args, "object_type")))
	name := strings.TrimSpace(getStringParam(args, "object_name"))
	if objectType == "" || name == "" {
		return newToolResultError("delete needs a target \"<TYPE> <NAME>\", e.g. target=\"PROG ZDEMO\""), nil
	}
	// Refused before anything is looked up: a FUNC's group is found with a
	// search, and under --read-only that search is a request for nothing.
	if err := s.adtClient.Safety().CheckOperation(adt.OpDelete, "DeleteObject"); err != nil {
		return newToolResultError(fmt.Sprintf("Failed to delete object: %v", err)), nil
	}
	parent := firstParam(args, "parent", "parent_name", "function_group")
	for _, n := range []string{name, parent} {
		if n != "" && !validDeleteName(n) {
			return newToolResultError(fmt.Sprintf("delete: %q is not an object name (letters, digits, _, $ and a namespace's /)", n)), nil
		}
	}
	if objectType == "FUNC" && parent == "" {
		group, err := s.adtClient.ResolveFunctionGroup(ctx, name)
		if err != nil {
			return newToolResultError(fmt.Sprintf("Failed to delete object: %v; pass params={\"parent\": \"<group>\"}", err)), nil
		}
		parent = group
	}
	objectURL, ok := deleteURLByName(objectType, name, parent)
	if !ok || objectURL == "" {
		return newToolResultError(fmt.Sprintf("delete: no ADT delete for type %s. Supported: %s; or pass target=\"OBJECT\" with params.object_url",
			objectType, strings.Join(deleteNameTypes, ", "))), nil
	}
	out := copyParams(args)
	delete(out, "object_type")
	delete(out, "object_name")
	out["object_url"] = objectURL
	return s.handleDeleteObject(ctx, newRequest(out))
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

	// Reassigning an object's package changes TADIR: an object change,
	// refused under --read-only before the WebSocket connects. The
	// WebSocket client carries no safety config of its own, so the gate is
	// here, where both routes (edit MOVE, debug MOVE) and the tool meet.
	if err := s.adtClient.Safety().CheckOperation(adt.OpUpdate, "MoveObject"); err != nil {
		return newToolResultError(err.Error()), nil
	}
	// --allowed-packages covers both ends of the move: the target package,
	// known without a request, and the package the object is in now, looked
	// up through the repository search. Without the second, an object could
	// be moved out of a package the server may not touch into one it may,
	// and then edited.
	if err := s.adtClient.Safety().CheckPackage(newPackage); err != nil {
		return newToolResultError(err.Error()), nil
	}
	if err := s.adtClient.CheckObjectPackageByName(ctx, objectType, objectName); err != nil {
		return newToolResultError(err.Error()), nil
	}

	// A transport names the request the moved object is recorded in. It is
	// checked before anything moves: a move that cannot be recorded is not
	// started.
	transport := strings.ToUpper(transportParam(request.GetArguments()))
	if transport != "" {
		if err := s.checkTransportWrite("MoveObject", transport); err != nil {
			return newToolResultError(err.Error()), nil
		}
	}

	// Ensure WebSocket client is connected
	if err := s.ensureDebugWSClient(ctx); err != nil {
		return newToolResultError(fmt.Sprintf("Failed to connect to ZADT_VSP WebSocket: %v. Ensure ZADT_VSP is deployed and SAPC/SICF are configured.", err)), nil
	}

	result, err := s.debugWSClient.MoveObject(ctx, objectType, objectName, newPackage)
	if err != nil {
		return newToolResultError(fmt.Sprintf("MoveObject failed: %v%s", err, moveTimeoutHint(err))), nil
	}
	if !result.Success {
		return newToolResultError(fmt.Sprintf("Move failed: %s", result.Message)), nil
	}

	text := fmt.Sprintf("Object moved successfully.\n\nObject: %s %s\nNew Package: %s\nMessage: %s",
		result.Object, result.ObjName, result.NewPackage, result.Message)
	switch {
	case transport != "":
		entry := adt.TransportEntry{TransportObjectKey: adt.TransportObjectKey{
			PgmID: "R3TR", Object: strings.ToUpper(objectType), Name: strings.ToUpper(objectName)}}
		rec, err := s.adtClient.AddTransportObjects(ctx, s.debugWSClient, transport, []adt.TransportEntry{entry})
		if err != nil {
			return newToolResultError(fmt.Sprintf("%s\n\nThe object was moved, but recording it in %s failed: %v. "+
				`Record it with SAP(action="system", params={"type": "add_transport_object", "transport": %q, "objects": [%q]}).`,
				text, transport, err, transport, entry.String())), nil
		}
		text += fmt.Sprintf("\nRecorded in: %s (task %s)", rec.Request, rec.Task)
	case !strings.HasPrefix(newPackage, "$"):
		text += fmt.Sprintf("\nNot recorded in a request: %s is transportable; pass \"transport\" to record the object, "+
			`or add it with SAP(action="system", params={"type": "add_transport_object", ...}).`, newPackage)
	}
	return mcp.NewToolResultText(text), nil
}

// moveTimeoutHint explains a move that got no answer. The ZADT_VSP session
// ends without one when the move dumps in the push channel, and the dump has
// rolled the move back.
func moveTimeoutHint(err error) string {
	if err == nil || !strings.Contains(err.Error(), "timeout") {
		return ""
	}
	return ". No answer usually means the ZADT_VSP session ended with a runtime error, which rolled the move back; " +
		`see SAP(action="analyze", params={"type": "list_dumps", "since": "<today>"}). The next call opens a new session`
}

// withName fills params["name"] from the target ("STRUCT ZDEMO") when the call
// did not pass it.
func withName(params map[string]any, objectName string) map[string]any {
	if objectName == "" || getStringParam(params, "name") != "" {
		return params
	}
	out := make(map[string]any, len(params)+1)
	for k, v := range params {
		out[k] = v
	}
	out["name"] = objectName
	return out
}
