package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/oisee/vibing-steampunk/pkg/adt"
)

// registerIAMTools registers the identity and access management tools.
//
// Called from registerTools alongside the other register* functions.
func (s *Server) registerIAMTools(shouldRegister func(string) bool) {
	if shouldRegister("CreateBusinessCatalog") {
		s.mcpServer.AddTool(mcp.NewTool("CreateBusinessCatalog",
			mcp.WithDescription("Create an IAM business catalog (ADT type SIA1) and assign IAM apps to it. "+
				"This is the step that authorizes a published RAP service: publishing a service binding "+
				"generates an IAM app, which grants nothing until a business catalog holds it and a business "+
				"role includes that catalog. Creates, activates and publishes the catalog. "+
				"Assigning apps is a separate step: use CreateIAMApp then AssignIAMAppToCatalog, which "+
				"creates the SIA7 assignment object. (The SIA1 $bucapps sub-resource is read-only and "+
				"answers 405; the assignment is a first-class object, which is what Eclipse writes.) "+
				"Finish by adding the catalog to a business role in Maintain Business Roles."),
			mcp.WithString("name",
				mcp.Required(),
				mcp.Description("Business catalog name, e.g. ZBC_MY_APP"),
			),
			mcp.WithString("description",
				mcp.Required(),
				mcp.Description("Business catalog description"),
			),
			mcp.WithString("package_name",
				mcp.Required(),
				mcp.Description("Package to create the catalog in"),
			),
			mcp.WithString("iam_apps",
				mcp.Description("Comma-separated IAM app IDs this catalog is meant to hold, e.g. "+
					"ZUI_MY_SRV_O4_0001_ABCD_IBS. Recorded in the response as a reminder only: ADT cannot write "+
					"app assignments (see attempt_app_assignment)."),
			),
			mcp.WithBoolean("attempt_app_assignment",
				mcp.Description("Try to write the assignments over ADT anyway (default: false). "+
					"The $bucapps resource controller implements only GET on S/4HANA Cloud 2602, so this "+
					"returns 405. Left in for a future release that opens it for writing."),
			),
			mcp.WithString("transport",
				mcp.Description("Transport request number, required for a transportable package"),
			),
			mcp.WithBoolean("publish",
				mcp.Description("Run the local publish after activation (default: true)"),
			),
			mcp.WithBoolean("skip_activation",
				mcp.Description("Create and assign without activating (default: false)"),
			),
		), s.handleCreateBusinessCatalog)
	}

	if shouldRegister("CreateIAMApp") {
		s.mcpServer.AddTool(mcp.NewTool("CreateIAMApp",
			mcp.WithDescription("Create an IAM app (ADT type SIA6), activate and publish it locally. "+
				"Publishing a service binding does NOT create this object: it creates an IAM business "+
				"service (SIA6 with appType IBS) plus a communication scenario, and neither can be put "+
				"in a business catalog. This is the assignable app that goes in one. Use app_type EXT "+
				"for an application served outside the system, such as a Fiori app on BTP Cloud Foundry. "+
				"Until the app is published, the business catalog assignment does not see it, which "+
				"reads as the app not existing."),
			mcp.WithString("name", mcp.Required(),
				mcp.Description("IAM app name, e.g. ZIA_MY_APP")),
			mcp.WithString("description", mcp.Required(),
				mcp.Description("IAM app description")),
			mcp.WithString("package_name", mcp.Required(),
				mcp.Description("Package to create the app in")),
			mcp.WithString("app_type",
				mcp.Description("Application type: EXT (external app, the default) or IBS")),
			mcp.WithString("secondary_id",
				mcp.Description("Object the app stands for, e.g. the generated communication scenario "+
					"on an IBS app. Leave empty for a plain external app.")),
			mcp.WithString("transport",
				mcp.Description("Transport request number, required for a transportable package")),
			mcp.WithBoolean("publish",
				mcp.Description("Run the local publish after activation (default: true)")),
			mcp.WithBoolean("skip_activation",
				mcp.Description("Create without activating (default: false)")),
		), s.handleCreateIAMApp)
	}

	if shouldRegister("AssignIAMAppToCatalog") {
		s.mcpServer.AddTool(mcp.NewTool("AssignIAMAppToCatalog",
			mcp.WithDescription("Assign an IAM app to a business catalog by creating the assignment "+
				"object (ADT type SIA7), then activate it. This is what the Eclipse 'Business Catalog "+
				"IAM App Assignment' wizard creates. It is NOT the SIA1 $bucapps sub-resource, which is "+
				"read-only and answers 405 to every write verb; the assignment is a first-class object "+
				"in its own collection. Both the catalog and the app must exist and be published first."),
			mcp.WithString("business_catalog", mcp.Required(),
				mcp.Description("Business catalog ID, e.g. ZBC_MY_APP")),
			mcp.WithString("iam_app", mcp.Required(),
				mcp.Description("IAM app ID to assign, e.g. ZIA_MY_APP")),
			mcp.WithString("package_name", mcp.Required(),
				mcp.Description("Package to create the assignment in")),
			mcp.WithString("name",
				mcp.Description("Assignment object name. Defaults to <catalog>_0001, which is what "+
					"the workbench generates.")),
			mcp.WithString("transport",
				mcp.Description("Transport request number, required for a transportable package")),
			mcp.WithBoolean("skip_activation",
				mcp.Description("Create without activating (default: false)")),
		), s.handleAssignIAMAppToCatalog)
	}
}

func (s *Server) handleCreateBusinessCatalog(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	args := request.GetArguments()

	name, ok := args["name"].(string)
	if !ok || name == "" {
		return newToolResultError("name is required"), nil
	}
	description, ok := args["description"].(string)
	if !ok || description == "" {
		return newToolResultError("description is required"), nil
	}
	packageName, ok := args["package_name"].(string)
	if !ok || packageName == "" {
		return newToolResultError("package_name is required"), nil
	}
	iamApps := ""
	if ia, ok := args["iam_apps"].(string); ok {
		iamApps = strings.TrimSpace(ia)
	}
	attemptAssignment := false
	if aa, ok := args["attempt_app_assignment"].(bool); ok {
		attemptAssignment = aa
	}

	transport := ""
	if t, ok := args["transport"].(string); ok {
		transport = t
	}
	publish := true
	if p, ok := args["publish"].(bool); ok {
		publish = p
	}
	skipActivation := false
	if sa, ok := args["skip_activation"].(bool); ok {
		skipActivation = sa
	}

	var apps []adt.BusinessCatalogApp
	for _, raw := range strings.Split(iamApps, ",") {
		appID := strings.TrimSpace(raw)
		if appID == "" {
			continue
		}
		apps = append(apps, adt.BusinessCatalogApp{AppID: appID})
	}

	name = strings.ToUpper(name)
	catalogURL := adt.BusinessCatalogURL(name)

	// Steps are reported individually so a partial failure is diagnosable
	// rather than collapsing into one opaque error.
	steps := []string{}

	if err := s.adtClient.CreateObject(ctx, adt.CreateObjectOptions{
		ObjectType:  adt.ObjectTypeBusinessCatalog,
		Name:        name,
		Description: description,
		PackageName: packageName,
		Transport:   transport,
	}); err != nil {
		// A create that fails because the catalog is already there is not a
		// failure of this call. It happens on any rerun after a later step
		// broke, and demanding a manual delete first would be hostile.
		//
		// The typed ADT exception id is the reliable signal. The GET probe is
		// only a backstop for a create that failed some other way after SAP had
		// already persisted the object.
		if !adt.IsResourceAlreadyExists(err) && !s.adtClient.BusinessCatalogExists(ctx, name) {
			return newToolResultError(fmt.Sprintf("Failed to create business catalog: %v", err)), nil
		}
		steps = append(steps, "already existed, continuing")
	} else {
		steps = append(steps, "created")
	}

	// App assignment only runs when explicitly asked for, because the ADT
	// resource is read-only. Skipping it keeps the run going to activation
	// instead of aborting on a 405 and leaving an inactive catalog behind.
	if attemptAssignment && len(apps) > 0 {
		lock, err := s.adtClient.LockObject(ctx, catalogURL, "MODIFY")
		if err != nil {
			return newToolResultError(fmt.Sprintf(
				"Catalog %s exists but could not be locked: %v", name, err)), nil
		}
		steps = append(steps, "locked")

		assignVerb, assignErr := s.adtClient.AssignBusinessCatalogApps(ctx, name, lock.LockHandle, apps, transport)

		// Release the lock whatever happened, so a failed assignment does not
		// leave the catalog stuck for the next attempt.
		if unlockErr := s.adtClient.UnlockObject(ctx, catalogURL, lock.LockHandle); unlockErr != nil {
			steps = append(steps, fmt.Sprintf("unlock failed: %v", unlockErr))
		} else {
			steps = append(steps, "unlocked")
		}

		if assignErr != nil {
			steps = append(steps, fmt.Sprintf("app assignment failed via %s: %v", assignVerb, assignErr))
		} else {
			steps = append(steps, fmt.Sprintf("assigned %d app(s) via %s", len(apps), assignVerb))
		}
	}

	result := map[string]any{
		"status":     "created",
		"name":       name,
		"object_url": catalogURL,
		"apps":       iamApps,
		"steps":      steps,
	}

	if !skipActivation {
		activation, err := s.adtClient.Activate(ctx, catalogURL, name)
		if err != nil {
			result["status"] = "created_not_activated"
			result["activation_error"] = err.Error()
			result["next_step"] = "Activate the catalog, then verify with GetInactiveObjects."
			output, _ := json.MarshalIndent(result, "", "  ")
			return mcp.NewToolResultText(string(output)), nil
		}
		result["activation"] = activation
		steps = append(steps, "activated")
	}

	if publish && !skipActivation {
		err := s.adtClient.PublishBusinessCatalog(ctx, name)
		switch {
		case err != nil && isTimeout(err):
			// Publishing is a job, and it routinely outlives the client
			// deadline. Reporting that as an error makes a probably-successful
			// publish look broken.
			result["publish_status"] = "unknown"
			result["publish_warning"] = "The publish request timed out waiting for a " +
				"response. Publishing is asynchronous, so the job may still complete. " +
				"Do not read this as a failure; re-read the catalog to see its status."
			steps = append(steps, "publish submitted, response timed out")
		case err != nil:
			result["publish_error"] = err.Error()
		default:
			steps = append(steps, "published")
		}
	}

	result["steps"] = steps
	if len(apps) > 0 {
		result["next_step_apps"] = fmt.Sprintf(
			"Assign the app(s) with AssignIAMAppToCatalog(business_catalog=%s, iam_app=...): %s. "+
				"The app must exist and be published first — CreateIAMApp does both.", name, iamApps)
	}
	result["next_step_role"] = fmt.Sprintf(
		"Then add catalog %s to a business role in Maintain Business Roles and assign the role to a business user.", name)

	output, _ := json.MarshalIndent(result, "", "  ")
	return mcp.NewToolResultText(string(output)), nil
}

// handleCreateIAMApp creates an SIA6 IAM app, activates and publishes it.
//
// Publishing matters more than it looks: an unpublished app is invisible to
// business catalog maintenance, and the Eclipse assignment wizard reports it as
// not existing rather than as not published.
func (s *Server) handleCreateIAMApp(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	args := request.GetArguments()

	name, ok := args["name"].(string)
	if !ok || name == "" {
		return newToolResultError("name is required"), nil
	}
	description, ok := args["description"].(string)
	if !ok || description == "" {
		return newToolResultError("description is required"), nil
	}
	packageName, ok := args["package_name"].(string)
	if !ok || packageName == "" {
		return newToolResultError("package_name is required"), nil
	}

	appType, _ := args["app_type"].(string)
	secondaryID, _ := args["secondary_id"].(string)
	transport, _ := args["transport"].(string)
	publish := true
	if p, ok := args["publish"].(bool); ok {
		publish = p
	}
	skipActivation := false
	if sa, ok := args["skip_activation"].(bool); ok {
		skipActivation = sa
	}

	name = strings.ToUpper(name)
	appURL := adt.IAMAppURL(name)
	steps := []string{}

	if err := s.adtClient.CreateObject(ctx, adt.CreateObjectOptions{
		ObjectType:  adt.ObjectTypeIAMApp,
		Name:        name,
		Description: description,
		PackageName: packageName,
		Transport:   transport,
		AppType:     appType,
		SecondaryID: secondaryID,
	}); err != nil {
		// Same reasoning as the catalog: a rerun after a later step failed must
		// not be blocked by the object already being there.
		if !adt.IsResourceAlreadyExists(err) && !s.adtClient.IAMAppExists(ctx, name) {
			return newToolResultError(fmt.Sprintf("Failed to create IAM app: %v", err)), nil
		}
		steps = append(steps, "already existed, continuing")
	} else {
		steps = append(steps, "created")
	}

	result := map[string]any{
		"status":     "created",
		"name":       name,
		"object_url": appURL,
		"app_type":   strings.ToUpper(appType),
	}

	if !skipActivation {
		activation, err := s.adtClient.Activate(ctx, appURL, name)
		if err != nil {
			result["status"] = "created_not_activated"
			result["activation_error"] = err.Error()
			result["next_step"] = "Activate the app, then verify with GetInactiveObjects."
			result["steps"] = steps
			output, _ := json.MarshalIndent(result, "", "  ")
			return mcp.NewToolResultText(string(output)), nil
		}
		result["activation"] = activation
		steps = append(steps, "activated")
	}

	if publish && !skipActivation {
		status, err := s.adtClient.PublishIAMApp(ctx, name)
		switch {
		case err != nil && isTimeout(err):
			// Publishing is a job. The client giving up on the response does not
			// mean the job failed — PublishServiceBinding behaves the same way,
			// and its timeout is routinely mistaken for an error. Observed on
			// a live S/4HANA Cloud tenant: the first publish of an app answers promptly, a repeat
			// one blocks past three minutes.
			result["publish_status"] = "unknown"
			result["publish_warning"] = "The publish request timed out waiting for a " +
				"response. Publishing is asynchronous, so the job may still complete. " +
				"Do not read this as a failure; re-read the app to see its status."
			steps = append(steps, "publish submitted, response timed out")
		case err != nil:
			result["publish_error"] = err.Error()
		case status == adt.IAMPublishStatusPublished:
			steps = append(steps, "published")
			result["publish_status"] = status
		default:
			// A 200 does not mean published. Report the status rather than
			// letting an unpublished app look like a completed step.
			result["publish_status"] = status
			result["publish_warning"] = fmt.Sprintf(
				"Publish returned status %q, not %q (published). The call succeeded but the "+
					"app is not published. An app with no services or authorizations has "+
					"nothing to publish.", status, adt.IAMPublishStatusPublished)
			steps = append(steps, "publish returned "+status)
		}
	}

	result["steps"] = steps
	result["next_step_assign"] = fmt.Sprintf(
		"Assign it with AssignIAMAppToCatalog(business_catalog=<catalog>, iam_app=%s).", name)

	output, _ := json.MarshalIndent(result, "", "  ")
	return mcp.NewToolResultText(string(output)), nil
}

// handleAssignIAMAppToCatalog creates the SIA7 assignment object linking a
// business catalog to an IAM app.
func (s *Server) handleAssignIAMAppToCatalog(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	args := request.GetArguments()

	catalog, ok := args["business_catalog"].(string)
	if !ok || catalog == "" {
		return newToolResultError("business_catalog is required"), nil
	}
	app, ok := args["iam_app"].(string)
	if !ok || app == "" {
		return newToolResultError("iam_app is required"), nil
	}
	packageName, ok := args["package_name"].(string)
	if !ok || packageName == "" {
		return newToolResultError("package_name is required"), nil
	}

	catalog = strings.ToUpper(catalog)
	app = strings.ToUpper(app)

	name, _ := args["name"].(string)
	if name == "" {
		name = adt.DefaultAssignmentName(catalog, 1)
	}
	name = strings.ToUpper(name)

	transport, _ := args["transport"].(string)
	skipActivation := false
	if sa, ok := args["skip_activation"].(bool); ok {
		skipActivation = sa
	}

	assignURL := adt.CatalogAppAssignmentURL(name)
	steps := []string{}

	if err := s.adtClient.CreateObject(ctx, adt.CreateObjectOptions{
		ObjectType:        adt.ObjectTypeCatalogAppAssignment,
		Name:              name,
		Description:       "Business Catalog to IAM App assignment",
		PackageName:       packageName,
		Transport:         transport,
		BusinessCatalogID: catalog,
		AppID:             app,
	}); err != nil {
		if !adt.IsResourceAlreadyExists(err) && !s.adtClient.CatalogAppAssignmentExists(ctx, name) {
			return newToolResultError(fmt.Sprintf(
				"Failed to create assignment %s (%s -> %s): %v", name, catalog, app, err)), nil
		}
		steps = append(steps, "already existed, continuing")
	} else {
		steps = append(steps, "created")
	}

	result := map[string]any{
		"status":     "created",
		"name":       name,
		"object_url": assignURL,
		// Deliberately NOT the requested catalog and app. ADT does not merge a
		// payload into an object that already exists, so echoing the request
		// back would describe an object this call never wrote. Read it instead.
		"requested_business_catalog": catalog,
		"requested_iam_app":          app,
	}

	if stored, err := s.adtClient.ReadCatalogAppAssignment(ctx, name); err != nil {
		result["read_back_error"] = err.Error()
		result["warning"] = "Could not read the assignment back. Do not assume it holds the requested app."
	} else {
		result["business_catalog"] = stored.CatalogID
		result["iam_app"] = stored.AppID
		if !strings.EqualFold(stored.AppID, app) || !strings.EqualFold(stored.CatalogID, catalog) {
			result["status"] = "exists_with_different_content"
			result["warning"] = fmt.Sprintf(
				"Assignment %s already existed and holds catalog %s -> app %s, NOT the requested %s -> %s. "+
					"ADT create does not overwrite. Delete the object and re-run, or pass a different name.",
				name, stored.CatalogID, stored.AppID, catalog, app)
		}
	}

	if !skipActivation {
		activation, err := s.adtClient.Activate(ctx, assignURL, name)
		if err != nil {
			result["status"] = "created_not_activated"
			result["activation_error"] = err.Error()
			result["steps"] = steps
			output, _ := json.MarshalIndent(result, "", "  ")
			return mcp.NewToolResultText(string(output)), nil
		}
		result["activation"] = activation
		steps = append(steps, "activated")
	}

	result["steps"] = steps
	result["next_step_role"] = fmt.Sprintf(
		"Add catalog %s to a business role in Maintain Business Roles, then assign that role to a "+
			"business user in Maintain Business Users. Neither step is reachable over ADT.", catalog)

	output, _ := json.MarshalIndent(result, "", "  ")
	return mcp.NewToolResultText(string(output)), nil
}

// isTimeout reports whether an error is a client-side deadline rather than a
// refusal by the backend. Kept deliberately simple: the transport wraps the
// underlying error, so matching the text is more robust here than unwrapping to
// a concrete type that may change.
func isTimeout(err error) bool {
	if err == nil {
		return false
	}
	s := err.Error()
	return strings.Contains(s, "context deadline exceeded") ||
		strings.Contains(s, "Client.Timeout") ||
		strings.Contains(s, "timeout")
}
