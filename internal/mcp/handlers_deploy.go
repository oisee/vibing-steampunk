// Package mcp provides the MCP server implementation for ABAP ADT tools.
// handlers_deploy.go contains handlers for deploying abapGit-format ZIPs to SAP packages.
package mcp

import (
	"context"
	"fmt"
	"net/url"
	"strings"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/oisee/vibing-steampunk/embedded/deps"
	"github.com/oisee/vibing-steampunk/pkg/adt"
)

// objectTypeMapping maps deployment type strings to CreatableObjectType and URL patterns
var objectTypeMapping = map[string]struct {
	creatableType adt.CreatableObjectType
	urlPattern    string // format string with %s for name
}{
	"PROG": {adt.ObjectTypeProgram, "/sap/bc/adt/programs/programs/%s"},
	"CLAS": {adt.ObjectTypeClass, "/sap/bc/adt/oo/classes/%s"},
	"INTF": {adt.ObjectTypeInterface, "/sap/bc/adt/oo/interfaces/%s"},
	"DDLS": {adt.ObjectTypeDDLS, "/sap/bc/adt/ddic/ddl/sources/%s"},
	"BDEF": {adt.ObjectTypeBDEF, "/sap/bc/adt/bo/behaviordefinitions/%s"},
	"SRVD": {adt.ObjectTypeSRVD, "/sap/bc/adt/ddic/srvd/sources/%s"},
}

// handleDeployZip deploys objects from an embedded abapGit-format ZIP to a SAP package.
// Uses a 3-phase approach for bulk deployment:
//
//	Phase 1: Create all objects (empty shells)
//	Phase 2: Upload source code (Lock → UpdateSource → Unlock, NO syntax check)
//	Phase 3: Mass activate all objects
func (s *Server) handleDeployZip(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	// Parse parameters
	source, _ := request.Params.Arguments["source"].(string)
	if source == "" {
		return newToolResultError("source is required (e.g., 'abapgit-standalone', 'abapgit-full')"), nil
	}

	packageName, _ := request.Params.Arguments["package"].(string)
	if packageName == "" {
		return newToolResultError("package is required (e.g., '$ZGIT')"), nil
	}
	packageName = strings.ToUpper(packageName)

	dryRun := false
	if dr, ok := request.Params.Arguments["dry_run"].(bool); ok {
		dryRun = dr
	}

	typeFilter := ""
	if tf, ok := request.Params.Arguments["type_filter"].(string); ok {
		typeFilter = strings.ToUpper(tf)
	}

	nameFilter := ""
	if nf, ok := request.Params.Arguments["name_filter"].(string); ok {
		nameFilter = strings.ToUpper(nf)
	}

	// Get ZIP data
	zipData := deps.GetDependencyZIP(source)
	if zipData == nil {
		available := deps.GetAvailableDependencies()
		var names []string
		for _, d := range available {
			if d.Available {
				names = append(names, d.Name)
			}
		}
		return newToolResultError(fmt.Sprintf("Source '%s' not found. Available: %s", source, strings.Join(names, ", "))), nil
	}

	var sb strings.Builder
	fmt.Fprintf(&sb, "DeployZip: %s → %s\n", source, packageName)
	sb.WriteString(strings.Repeat("=", 60) + "\n\n")
	fmt.Fprintf(&sb, "Source: %s (%d bytes)\n", source, len(zipData))
	fmt.Fprintf(&sb, "Target: %s\n\n", packageName)

	// Parse ZIP
	files, err := deps.UnzipInMemory(zipData)
	if err != nil {
		return newToolResultError(fmt.Sprintf("Failed to parse ZIP: %v", err)), nil
	}

	// Create deployment plan
	plan := deps.CreateDeploymentPlan(source, packageName, files)
	fmt.Fprintf(&sb, "Found %d objects in %d files\n\n", plan.TotalObjects, plan.TotalFiles)

	// Filter and classify objects
	var deployable, skipped []deps.DeploymentObject
	for _, obj := range plan.Objects {
		// Type filter
		if typeFilter != "" && obj.Type != typeFilter {
			continue
		}
		// Name filter (simple wildcard)
		if nameFilter != "" {
			pattern := strings.ReplaceAll(nameFilter, "*", "")
			if !strings.Contains(obj.Name, pattern) {
				continue
			}
		}

		if _, ok := objectTypeMapping[obj.Type]; ok {
			deployable = append(deployable, obj)
		} else {
			skipped = append(skipped, obj)
		}
	}

	// Show deployment plan
	fmt.Fprintf(&sb, "Deployment Plan (%d deployable, %d skipped):\n", len(deployable), len(skipped))
	sb.WriteString(strings.Repeat("-", 60) + "\n")

	for _, obj := range deployable {
		includeInfo := ""
		if len(obj.Includes) > 0 {
			var incTypes []string
			for t := range obj.Includes {
				incTypes = append(incTypes, t)
			}
			includeInfo = fmt.Sprintf(" [+%s]", strings.Join(incTypes, ","))
		}
		fmt.Fprintf(&sb, "  ✓ %-6s %-40s%s\n", obj.Type, obj.Name, includeInfo)
	}

	if len(skipped) > 0 {
		sb.WriteString("\n  Skipped (requires ZADT_VSP):\n")
		for _, obj := range skipped {
			fmt.Fprintf(&sb, "  ⊘ %-6s %s\n", obj.Type, obj.Name)
		}
	}

	sb.WriteString(strings.Repeat("-", 60) + "\n\n")

	if dryRun {
		sb.WriteString("Dry run - no changes made.\n")
		return mcp.NewToolResultText(sb.String()), nil
	}

	if len(deployable) == 0 {
		sb.WriteString("Nothing to deploy.\n")
		return mcp.NewToolResultText(sb.String()), nil
	}

	// Ensure package exists
	fmt.Fprintf(&sb, "Checking package %s...\n", packageName)
	pkg, pkgErr := s.adtClient.GetPackage(ctx, packageName)
	if pkgErr != nil || pkg.URI == "" {
		fmt.Fprintf(&sb, "Creating package %s...\n", packageName)
		err = s.adtClient.CreateObject(ctx, adt.CreateObjectOptions{
			ObjectType:  adt.ObjectTypePackage,
			Name:        packageName,
			Description: fmt.Sprintf("Deployed from %s", source),
		})
		if err != nil {
			// Tolerate "already exists" - GetPackage may fail for $ packages
			if strings.Contains(err.Error(), "AlreadyExists") || strings.Contains(err.Error(), "already exist") {
				fmt.Fprintf(&sb, "  ✓ Package already exists\n")
			} else {
				return newToolResultError(fmt.Sprintf("Failed to create package: %v", err)), nil
			}
		} else {
			fmt.Fprintf(&sb, "  ✓ Package created\n")
		}
	} else {
		fmt.Fprintf(&sb, "  ✓ Package exists\n")
	}
	sb.WriteString("\n")

	// ================================================================
	// PHASE 1: Create all objects (empty shells)
	// ================================================================
	sb.WriteString("Phase 1: Creating objects...\n")
	var createSuccess, createSkipped, createFailed int

	for i, obj := range deployable {
		typeInfo, ok := objectTypeMapping[obj.Type]
		if !ok {
			continue
		}

		desc := obj.Description
		if desc == "" {
			desc = obj.Name
		}

		fmt.Fprintf(&sb, "  [%d/%d] Create %s %s... ", i+1, len(deployable), obj.Type, obj.Name)

		err := s.adtClient.CreateObject(ctx, adt.CreateObjectOptions{
			ObjectType:  typeInfo.creatableType,
			Name:        obj.Name,
			Description: desc,
			PackageName: packageName,
		})
		if err != nil {
			if strings.Contains(err.Error(), "AlreadyExists") || strings.Contains(err.Error(), "already exist") {
				fmt.Fprintf(&sb, "exists\n")
				createSkipped++
			} else {
				fmt.Fprintf(&sb, "FAIL: %v\n", err)
				createFailed++
			}
		} else {
			fmt.Fprintf(&sb, "ok\n")
			createSuccess++
		}
	}

	fmt.Fprintf(&sb, "\n  Phase 1 summary: %d created, %d existed, %d failed\n\n", createSuccess, createSkipped, createFailed)

	// ================================================================
	// PHASE 2: Upload source (Lock → UpdateSource → Unlock, NO syntax check)
	// ================================================================
	sb.WriteString("Phase 2: Uploading source code...\n")
	var uploadSuccess, uploadFailed int
	var uploadFailures []string

	for i, obj := range deployable {
		typeInfo, ok := objectTypeMapping[obj.Type]
		if !ok {
			continue
		}

		encodedName := url.PathEscape(strings.ToLower(obj.Name))
		objectURL := fmt.Sprintf(typeInfo.urlPattern, encodedName)
		sourceURL := objectURL + "/source/main"

		fmt.Fprintf(&sb, "  [%d/%d] Upload %s %s... ", i+1, len(deployable), obj.Type, obj.Name)

		// Lock
		lockResult, err := s.adtClient.LockObject(ctx, objectURL, "MODIFY")
		if err != nil {
			fmt.Fprintf(&sb, "LOCK FAIL: %v\n", err)
			uploadFailed++
			uploadFailures = append(uploadFailures, fmt.Sprintf("%s %s: lock failed: %v", obj.Type, obj.Name, err))
			continue
		}

		// Upload source (no syntax check!)
		err = s.adtClient.UpdateSource(ctx, sourceURL, obj.MainSource, lockResult.LockHandle, "")
		if err != nil {
			// Always try to unlock even if upload fails
			_ = s.adtClient.UnlockObject(ctx, objectURL, lockResult.LockHandle)
			fmt.Fprintf(&sb, "UPLOAD FAIL: %v\n", err)
			uploadFailed++
			uploadFailures = append(uploadFailures, fmt.Sprintf("%s %s: upload failed: %v", obj.Type, obj.Name, err))
			continue
		}

		// Unlock
		err = s.adtClient.UnlockObject(ctx, objectURL, lockResult.LockHandle)
		if err != nil {
			fmt.Fprintf(&sb, "UNLOCK FAIL: %v\n", err)
			// Source was uploaded, just couldn't unlock - not fatal
		}

		fmt.Fprintf(&sb, "ok\n")
		uploadSuccess++
	}

	fmt.Fprintf(&sb, "\n  Phase 2 summary: %d uploaded, %d failed\n\n", uploadSuccess, uploadFailed)

	// ================================================================
	// PHASE 3: Iterative activation
	// ================================================================
	sb.WriteString("Phase 3: Iterative activation...\n")
	iterResult, err := s.adtClient.ActivatePackageIterative(ctx, packageName, 5)
	if err != nil {
		fmt.Fprintf(&sb, "  Activation error: %v\n", err)
	} else {
		for i, pass := range iterResult.Passes {
			fmt.Fprintf(&sb, "  Pass %d: %d activated, %d failed\n", i+1, len(pass.Activated), len(pass.Failed))
		}
		fmt.Fprintf(&sb, "  Total: %d activated across %d pass(es)\n", iterResult.TotalActivated, iterResult.Iterations)
		if iterResult.StillInactive == 0 {
			sb.WriteString("  All objects active — deployment verified.\n")
		} else {
			fmt.Fprintf(&sb, "  WARNING: %d object(s) still inactive in %s\n", iterResult.StillInactive, packageName)
		}
	}

	// ================================================================
	// SUMMARY
	// ================================================================
	sb.WriteString("\n" + strings.Repeat("=", 60) + "\n")
	fmt.Fprintf(&sb, "Deployment complete:\n")
	fmt.Fprintf(&sb, "  Phase 1 (Create):   %d ok, %d existed, %d failed\n", createSuccess, createSkipped, createFailed)
	fmt.Fprintf(&sb, "  Phase 2 (Upload):   %d ok, %d failed\n", uploadSuccess, uploadFailed)
	if iterResult != nil {
		fmt.Fprintf(&sb, "  Phase 3 (Activate): %s\n", iterResult.Summary)
	}

	if len(uploadFailures) > 0 {
		sb.WriteString("\nUpload failures:\n")
		for _, f := range uploadFailures {
			fmt.Fprintf(&sb, "  • %s\n", f)
		}
	}

	if len(skipped) > 0 {
		fmt.Fprintf(&sb, "\n%d objects skipped (not supported via ADT native).\n", len(skipped))
		sb.WriteString("Install ZADT_VSP to enable full object type support.\n")
	}

	return mcp.NewToolResultText(sb.String()), nil
}
