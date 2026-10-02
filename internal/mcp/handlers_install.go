// Package mcp provides the MCP server implementation for ABAP ADT tools.
// handlers_install.go contains handlers for installing ZADT_VSP and deploying embedded ZIPs.
package mcp

import (
	"context"
	"fmt"
	"strings"

	"github.com/mark3labs/mcp-go/mcp"
	embedded "github.com/oisee/vibing-steampunk/embedded/abap"
	"github.com/oisee/vibing-steampunk/embedded/deps"
	installer "github.com/oisee/vibing-steampunk/internal/install"
	"github.com/oisee/vibing-steampunk/pkg/adt"
)

// routeInstallAction routes "system" with install-related types.
func (s *Server) routeInstallAction(ctx context.Context, action, objectType, objectName string, params map[string]any) (*mcp.CallToolResult, bool, error) {
	if action != "system" {
		return nil, false, nil
	}
	installType := getStringParam(params, "type")
	switch installType {
	case "install_zadt_vsp":
		return s.callHandler(ctx, s.handleInstallZADTVSP, params)
	case "list_dependencies":
		return s.callHandler(ctx, s.handleListDependencies, params)
	case "deploy_zip":
		return s.callHandler(ctx, s.handleDeployZip, params)
	}
	return nil, false, nil
}

// --- Install Handlers ---

func (s *Server) handleInstallZADTVSP(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	// Parse parameters
	packageName := "$ZADT_VSP"
	if pkg, ok := request.GetArguments()["package"].(string); ok && pkg != "" {
		packageName = strings.ToUpper(pkg)
	}

	skipGitService := false
	if skip, ok := request.GetArguments()["skip_git_service"].(bool); ok {
		skipGitService = skip
	}

	checkOnly := false
	if check, ok := request.GetArguments()["check_only"].(bool); ok {
		checkOnly = check
	}

	// Validate package name
	if !strings.HasPrefix(packageName, "$") {
		return newToolResultError("Package name must start with $ (local package)"), nil
	}

	var sb strings.Builder
	sb.WriteString("ZADT_VSP Installation\n")
	sb.WriteString("=====================\n\n")

	// Phase 1: Check prerequisites
	sb.WriteString("Checking prerequisites...\n")

	packageExists, err := s.adtClient.PackageExists(ctx, packageName)
	if err != nil {
		return newToolResultError(fmt.Sprintf("Failed to check package %s: %v", packageName, err)), nil
	}
	if packageExists {
		fmt.Fprintf(&sb, "  ✓ Package %s exists\n", packageName)
	} else {
		fmt.Fprintf(&sb, "  → Package %s will be created\n", packageName)
	}

	// Check for abapGit (for Git service)
	hasAbapGit := false
	results, err := s.adtClient.SearchObject(ctx, "ZCL_ABAPGIT_OBJECTS", 1)
	if err == nil && len(results) > 0 {
		hasAbapGit = true
		sb.WriteString("  ✓ abapGit detected → Git service will be deployed\n")
	} else {
		sb.WriteString("  ⚠ abapGit not detected → Git service will be skipped\n")
		skipGitService = true
	}

	// Check existing objects
	objects := embedded.GetObjects()
	existingObjects := []string{}
	for _, obj := range objects {
		results, err := s.adtClient.SearchObject(ctx, obj.Name, 1)
		if err == nil && len(results) > 0 {
			existingObjects = append(existingObjects, obj.Name)
		}
	}
	if len(existingObjects) > 0 {
		fmt.Fprintf(&sb, "  ⚠ Existing objects will be updated: %s\n", strings.Join(existingObjects, ", "))
	}

	sb.WriteString("\n")

	// If check_only, stop here
	if checkOnly {
		sb.WriteString("Check complete (--check_only mode, no changes made).\n\n")
		sb.WriteString("Objects to deploy:\n")
		for i, obj := range objects {
			if obj.RequiresAbapGit && skipGitService {
				fmt.Fprintf(&sb, "  [%d/%d] %s - SKIP (no abapGit)\n", i+1, len(objects), obj.Name)
			} else {
				fmt.Fprintf(&sb, "  [%d/%d] %s - %s\n", i+1, len(objects), obj.Name, obj.Description)
			}
		}
		return mcp.NewToolResultText(sb.String()), nil
	}

	// Temporarily allow the install target package to bypass SAP_ALLOWED_PACKAGES restrictions.
	// Install operations are self-contained bootstrap operations that should not be blocked.
	cleanupPkgSafety := s.adtClient.AllowPackageTemporarily(packageName)
	defer cleanupPkgSafety()

	// Phase 2: Create package if needed
	created, err := installer.EnsurePackage(ctx, s.adtClient, packageName, "VSP WebSocket Handler")
	if err != nil {
		return newToolResultError(fmt.Sprintf("Failed to ensure package %s: %v", packageName, err)), nil
	}
	if created {
		fmt.Fprintf(&sb, "Package %s created and verified\n\n", packageName)
	} else {
		fmt.Fprintf(&sb, "Using existing package %s\n\n", packageName)
	}

	// Phase 3: Deploy objects
	sb.WriteString("Deploying ABAP objects...\n")

	deployed := []string{}
	skipped := []string{}
	failed := []string{}

	for i, obj := range objects {
		// Skip the git service and its job program if no abapGit
		if obj.RequiresAbapGit && skipGitService {
			fmt.Fprintf(&sb, "  [%d/%d] %s ⊘ Skipped (no abapGit)\n", i+1, len(objects), obj.Name)
			skipped = append(skipped, obj.Name)
			continue
		}

		fmt.Fprintf(&sb, "  [%d/%d] %s ", i+1, len(objects), obj.Name)

		// Use WriteSource to create/update
		opts := &adt.WriteSourceOptions{
			Package:     packageName,
			Description: obj.Description,
			Mode:        adt.WriteModeUpsert,
		}
		_, err := installer.DeploySource(ctx, s.adtClient, obj.Type, obj.Name, obj.Source, opts)
		if err != nil {
			fmt.Fprintf(&sb, "✗ Failed: %v\n", err)
			failed = append(failed, obj.Name+": "+err.Error())
		} else {
			sb.WriteString("✓ Deployed\n")
			deployed = append(deployed, obj.Name)
		}
	}

	// The transport service's push channel. Without it uploads still work;
	// their outcome is read with transport_status.
	fmt.Fprintf(&sb, "  AMC %s (transport push) ", embedded.AMCApplicationName)
	if err := s.adtClient.UpsertAMCApplication(ctx, embedded.AMCApplicationName, embedded.AMCApplicationDescription,
		packageName, embedded.AMCApplicationDefinition); err != nil {
		fmt.Fprintf(&sb, "– not set up (%v); upload outcomes are read with transport_status\n", err)
	} else {
		sb.WriteString("✓ Deployed\n")
	}
	// The git import's push channel, only with the git service it names.
	if !skipGitService {
		fmt.Fprintf(&sb, "  AMC %s (git import push) ", embedded.AMCGitApplicationName)
		if err := s.adtClient.UpsertAMCApplication(ctx, embedded.AMCGitApplicationName, embedded.AMCGitApplicationDescription,
			packageName, embedded.AMCGitApplicationDefinition); err != nil {
			fmt.Fprintf(&sb, "– not set up (%v); import outcomes are read with git_import_status\n", err)
		} else {
			sb.WriteString("✓ Deployed\n")
		}
	}

	sb.WriteString("\n")

	// Summary
	sb.WriteString("═══════════════════════════════════════════════════════════════════════════════\n")
	if len(failed) > 0 {
		sb.WriteString("  DEPLOYMENT PARTIALLY FAILED\n")
		sb.WriteString("═══════════════════════════════════════════════════════════════════════════════\n\n")
		sb.WriteString("Failed objects:\n")
		for _, f := range failed {
			fmt.Fprintf(&sb, "  • %s\n", f)
		}
	} else {
		sb.WriteString("  DEPLOYMENT COMPLETE - Manual Steps Required\n")
		sb.WriteString("═══════════════════════════════════════════════════════════════════════════════\n")
	}

	fmt.Fprintf(&sb, "\nDeployed: %d, Skipped: %d, Failed: %d\n\n", len(deployed), len(skipped), len(failed))
	if len(failed) > 0 {
		return newToolResultError(sb.String()), nil
	}

	// Post-deployment instructions
	sb.WriteString(embedded.PostDeploymentInstructions())

	// Features unlocked
	sb.WriteString("\nFeatures unlocked:\n")
	sb.WriteString("  ✓ WebSocket debugging (TPDAPI)\n")
	sb.WriteString("  ✓ RFC/BAPI execution\n")
	sb.WriteString("  ✓ AMDP debugging (experimental)\n")
	if hasAbapGit && !skipGitService {
		sb.WriteString("  ✓ abapGit export and zip import (git_import_zip)\n")
	} else {
		sb.WriteString("  ✗ abapGit export and import (install abapGit first)\n")
	}

	return mcp.NewToolResultText(sb.String()), nil
}

func (s *Server) handleListDependencies(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	var sb strings.Builder
	sb.WriteString("Available Dependencies\n")
	sb.WriteString("======================\n\n")

	dependencies := deps.GetAvailableDependencies()
	for _, dep := range dependencies {
		status := "⚠ Not embedded (placeholder)"
		if dep.Available {
			status = "✓ Available"
		}
		fmt.Fprintf(&sb, "• %s\n", dep.Name)
		fmt.Fprintf(&sb, "  Description: %s\n", dep.Description)
		fmt.Fprintf(&sb, "  Package: %s\n", dep.Package)
		fmt.Fprintf(&sb, "  Status: %s\n", status)
		sb.WriteString("\n")
	}

	sb.WriteString("Usage:\n")
	sb.WriteString("  vsp install abapgit --edition standalone  # CLI: single program ZABAPGIT\n")
	sb.WriteString("  DeployZip source=abapgit-standalone package=$ABAPGIT\n")
	sb.WriteString("\nabapGit install is CLI-only (standalone edition) for now; the MCP install\n")
	sb.WriteString("tool is being rebuilt (#277). The developer edition is not installable yet.\n")

	return mcp.NewToolResultText(sb.String()), nil
}
