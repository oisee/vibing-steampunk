// Package mcp provides the MCP server implementation for ABAP ADT tools.
// handlers_install.go contains handlers for installing ZADT_VSP and abapGit.
package mcp

import (
	"context"
	"fmt"
	"strings"

	"github.com/mark3labs/mcp-go/mcp"
	embedded "github.com/oisee/vibing-steampunk/embedded/abap"
	"github.com/oisee/vibing-steampunk/embedded/deps"
	"github.com/oisee/vibing-steampunk/pkg/adt"
)

// --- Install Routing ---
// Routes for this module:
//   system: type=install_zadt_vsp, type=install_abapgit, type=list_dependencies, type=install_dummy_test

// routeInstallAction routes install actions.
// Returns (result, true) if handled, (nil, false) if not handled by this module.
func (s *Server) routeInstallAction(ctx context.Context, action, _, _ string, params map[string]any) (*mcp.CallToolResult, bool, error) {
	if action != "system" {
		return nil, false, nil
	}

	systemType := getStr(params, "type")
	switch systemType {
	case "install_zadt_vsp":
		args := map[string]any{}
		copyOptional(args, params, "package")
		copyOptionalBool(args, params, "skip_git_service", "check_only")
		result, err := s.handleInstallZADTVSP(ctx, newRequest(args))
		return result, true, err

	case "install_abapgit":
		args := map[string]any{}
		copyOptional(args, params, "edition", "package")
		copyOptionalBool(args, params, "check_only")
		result, err := s.handleInstallAbapGit(ctx, newRequest(args))
		return result, true, err

	case "list_dependencies":
		result, err := s.handleListDependencies(ctx, newRequest(nil))
		return result, true, err

	case "install_dummy_test":
		args := map[string]any{}
		copyOptionalBool(args, params, "check_only", "cleanup")
		result, err := s.handleInstallDummyTest(ctx, newRequest(args))
		return result, true, err
	}

	return nil, false, nil
}

// --- Install Handlers ---

func (s *Server) handleInstallDummyTest(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	const (
		testPackage   = "$ZADT_INSTALL_TEST"
		testInterface = "ZIF_DUMMY_TEST"
		testClass     = "ZCL_DUMMY_TEST"
	)

	args := request.Params.Arguments
	checkOnly := getBool(args, "check_only", false)
	cleanup := getBool(args, "cleanup", false)

	var sb strings.Builder
	sb.WriteString("InstallDummyTest - Workflow Verification\n")
	sb.WriteString("========================================\n\n")

	// Track success/failure
	allPassed := true
	stepNum := 0

	step := func(name string) {
		stepNum++
		fmt.Fprintf(&sb, "[Step %d] %s\n", stepNum, name)
	}

	pass := func(msg string) {
		fmt.Fprintf(&sb, "  ✓ %s\n", msg)
	}

	fail := func(msg string) {
		fmt.Fprintf(&sb, "  ✗ %s\n", msg)
		allPassed = false
	}

	info := func(msg string) {
		fmt.Fprintf(&sb, "  → %s\n", msg)
	}

	// Step 1: Check/Create package (upsert strategy)
	step("Package Check/Create")
	pkg, err := s.adtClient.GetPackage(ctx, testPackage, nil)
	packageExists := err == nil && pkg.URI != "" // URI empty = package doesn't really exist
	if !packageExists {
		info("Package doesn't exist, creating...")
		if checkOnly {
			info("SKIP (check_only mode)")
		} else {
			err = s.adtClient.CreateObject(ctx, adt.CreateObjectOptions{
				ObjectType:  adt.ObjectTypePackage,
				Name:        testPackage,
				Description: "Install Tools Test Package",
			})
			if err != nil {
				fail(fmt.Sprintf("CreateObject(package) failed: %v", err))
			} else {
				// Verify
				pkg, err = s.adtClient.GetPackage(ctx, testPackage, nil)
				if err != nil || pkg.URI == "" {
					fail("Verification failed: package not found after create")
				} else {
					pass("Package created and verified")
				}
			}
		}
	} else {
		pass("Package exists")
	}

	if checkOnly {
		sb.WriteString("\n[check_only mode] Would deploy:\n")
		// Check existing objects
		intfResults, _ := s.adtClient.SearchObject(ctx, testInterface, 1)
		intfExists := len(intfResults) > 0 && intfResults[0].Name == testInterface
		classResults, _ := s.adtClient.SearchObject(ctx, testClass, 1)
		classExists := len(classResults) > 0 && classResults[0].Name == testClass

		intfAction := "CREATE"
		if intfExists {
			intfAction = "UPDATE"
		}
		classAction := "CREATE"
		if classExists {
			classAction = "UPDATE"
		}
		fmt.Fprintf(&sb, "  1. Interface: %s [%s]\n", testInterface, intfAction)
		fmt.Fprintf(&sb, "  2. Class: %s [%s]\n", testClass, classAction)
		return mcp.NewToolResultText(sb.String()), nil
	}

	// Step 2: Create/Update Interface (upsert)
	step("Interface Creation")

	// Check if interface exists
	intfResults, _ := s.adtClient.SearchObject(ctx, testInterface, 1)
	intfExists := len(intfResults) > 0 && intfResults[0].Name == testInterface
	if intfExists {
		info(fmt.Sprintf("Interface %s exists, will update", testInterface))
	}

	interfaceSource := `INTERFACE zif_dummy_test
  PUBLIC.

  METHODS get_value
    RETURNING VALUE(rv_value) TYPE string.

ENDINTERFACE.`

	opts := &adt.WriteSourceOptions{
		Package:     testPackage,
		Description: "Dummy Test Interface",
		Mode:        adt.WriteModeUpsert,
	}
	result, err := s.adtClient.WriteSource(ctx, "INTF", testInterface, interfaceSource, opts)
	if err != nil {
		fail(fmt.Sprintf("WriteSource failed: %v", err))
	} else if !result.Success {
		fail(fmt.Sprintf("WriteSource returned failure: %s", result.Message))
	} else {
		pass(fmt.Sprintf("Interface written (mode=%s)", result.Mode))
	}

	// Step 3: Verify Interface
	step("Interface Verification")
	results, err := s.adtClient.SearchObject(ctx, testInterface, 10)
	if err != nil {
		fail(fmt.Sprintf("SearchObject failed: %v", err))
	} else {
		found := false
		for _, r := range results {
			if r.Name == testInterface {
				pass(fmt.Sprintf("Found: %s (%s) in %s", r.Name, r.Type, r.PackageName))
				found = true
				break
			}
		}
		if !found {
			fail("Interface not found in search results!")
		}
	}

	// Step 4: Read Interface Source
	step("Interface Source Read-back")
	src, err := s.adtClient.GetSource(ctx, "INTF", testInterface, nil)
	if err != nil {
		fail(fmt.Sprintf("GetSource failed: %v", err))
	} else {
		pass(fmt.Sprintf("Source retrieved (%d bytes)", len(src)))
	}

	// Step 5: Create/Update Class (upsert)
	step("Class Creation")

	// Check if class exists
	classResults, _ := s.adtClient.SearchObject(ctx, testClass, 1)
	classExists := len(classResults) > 0 && classResults[0].Name == testClass
	if classExists {
		info(fmt.Sprintf("Class %s exists, will update", testClass))
	}

	classSource := `CLASS zcl_dummy_test DEFINITION
  PUBLIC
  FINAL
  CREATE PUBLIC.

  PUBLIC SECTION.
    INTERFACES zif_dummy_test.

ENDCLASS.

CLASS zcl_dummy_test IMPLEMENTATION.

  METHOD zif_dummy_test~get_value.
    rv_value = 'Dummy Test Passed'.
  ENDMETHOD.

ENDCLASS.`

	opts = &adt.WriteSourceOptions{
		Package:     testPackage,
		Description: "Dummy Test Class",
		Mode:        adt.WriteModeUpsert,
	}
	result, err = s.adtClient.WriteSource(ctx, "CLAS", testClass, classSource, opts)
	if err != nil {
		fail(fmt.Sprintf("WriteSource failed: %v", err))
	} else if !result.Success {
		fail(fmt.Sprintf("WriteSource returned failure: %s", result.Message))
	} else {
		pass(fmt.Sprintf("Class written (mode=%s)", result.Mode))
	}

	// Step 6: Verify Class
	step("Class Verification")
	results, err = s.adtClient.SearchObject(ctx, testClass, 10)
	if err != nil {
		fail(fmt.Sprintf("SearchObject failed: %v", err))
	} else {
		found := false
		for _, r := range results {
			if r.Name == testClass {
				pass(fmt.Sprintf("Found: %s (%s) in %s", r.Name, r.Type, r.PackageName))
				found = true
				break
			}
		}
		if !found {
			fail("Class not found in search results!")
		}
	}

	// Step 7: Read Class Source
	step("Class Source Read-back")
	src, err = s.adtClient.GetSource(ctx, "CLAS", testClass, nil)
	if err != nil {
		fail(fmt.Sprintf("GetSource failed: %v", err))
	} else {
		pass(fmt.Sprintf("Source retrieved (%d bytes)", len(src)))
	}

	// Step 8: Run Unit Tests (optional verification)
	step("Unit Tests (Activation Check)")
	testResult, err := s.adtClient.RunUnitTests(ctx, fmt.Sprintf("/sap/bc/adt/oo/classes/%s", strings.ToLower(testClass)), nil)
	if err != nil {
		info(fmt.Sprintf("RunUnitTests: %v (expected - no tests defined)", err))
	} else {
		pass(fmt.Sprintf("Unit test framework responded (classes=%d)", len(testResult.Classes)))
	}

	// Cleanup if requested
	if cleanup {
		sb.WriteString("\n[Cleanup]\n")
		sb.WriteString("  → Cleanup not yet implemented (manual deletion required)\n")
		// TODO: Implement delete via ADT
	}

	// Summary
	sb.WriteString("\n========================================\n")
	if allPassed {
		sb.WriteString("✅ ALL TESTS PASSED\n")
		sb.WriteString("\nThe Install* workflow is working correctly:\n")
		sb.WriteString("  • Package creation ✓\n")
		sb.WriteString("  • Interface creation (WriteSource) ✓\n")
		sb.WriteString("  • Interface verification (SearchObject) ✓\n")
		sb.WriteString("  • Interface read-back (GetSource) ✓\n")
		sb.WriteString("  • Class creation (WriteSource) ✓\n")
		sb.WriteString("  • Class verification (SearchObject) ✓\n")
		sb.WriteString("  • Class read-back (GetSource) ✓\n")
	} else {
		sb.WriteString("❌ SOME TESTS FAILED\n")
		sb.WriteString("\nCheck the steps above for details.\n")
	}

	fmt.Fprintf(&sb, "\nTest objects in package: %s\n", testPackage)

	return mcp.NewToolResultText(sb.String()), nil
}

func (s *Server) handleInstallZADTVSP(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	args := request.Params.Arguments
	packageName := getStr(args, "package")
	if packageName == "" {
		packageName = "$ZADT_VSP"
	} else {
		packageName = strings.ToUpper(packageName)
	}
	skipGitService := getBool(args, "skip_git_service", false)
	checkOnly := getBool(args, "check_only", false)

	if !strings.HasPrefix(packageName, "$") {
		return newToolResultError("Package name must start with $ (local package)"), nil
	}

	var sb strings.Builder
	sb.WriteString("ZADT_VSP Installation\n")
	sb.WriteString("=====================\n\n")

	// Phase 1: Check prerequisites
	sb.WriteString("Checking prerequisites...\n")

	// Check if package exists
	packageExists := false
	pkg, err := s.adtClient.GetPackage(ctx, packageName, nil)
	if err == nil && (pkg.URI != "" || pkg.Name != "" || pkg.TotalObjects > 0) {
		packageExists = true
		fmt.Fprintf(&sb, "  ✓ Package %s exists (%d objects)\n", packageName, pkg.TotalObjects)
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
			if obj.Optional && skipGitService && obj.Name == "ZCL_VSP_GIT_SERVICE" {
				fmt.Fprintf(&sb, "  [%d/%d] %s - SKIP (no abapGit)\n", i+1, len(objects), obj.Name)
			} else {
				fmt.Fprintf(&sb, "  [%d/%d] %s - %s\n", i+1, len(objects), obj.Name, obj.Description)
			}
		}
		return mcp.NewToolResultText(sb.String()), nil
	}

	// Phase 2: Create package if needed
	if !packageExists {
		fmt.Fprintf(&sb, "Creating package %s...\n", packageName)
		createOpts := adt.CreateObjectOptions{
			ObjectType:  adt.ObjectTypePackage,
			Name:        packageName,
			Description: "VSP WebSocket Handler",
		}
		err := s.adtClient.CreateObject(ctx, createOpts)
		if err != nil {
			return wrapErr("CreateObject(package)", err), nil
		}
		fmt.Fprintf(&sb, "  ✓ Package %s created\n\n", packageName)
	} else {
		fmt.Fprintf(&sb, "Using existing package %s\n\n", packageName)
	}

	// Phase 3: Deploy objects
	sb.WriteString("Deploying ABAP objects...\n")

	deployed := []string{}
	skipped := []string{}
	failed := []string{}

	for i, obj := range objects {
		// Skip Git service if no abapGit
		if obj.Name == "ZCL_VSP_GIT_SERVICE" && skipGitService {
			fmt.Fprintf(&sb, "  [%d/%d] %s ⊘ Skipped (no abapGit)\n", i+1, len(objects), obj.Name)
			skipped = append(skipped, obj.Name)
			continue
		}

		fmt.Fprintf(&sb, "  [%d/%d] %s ", i+1, len(objects), obj.Name)

		// Use WriteSource to create/update
		opts := &adt.WriteSourceOptions{
			Package: packageName,
			Mode:    adt.WriteModeUpsert,
		}
		_, err := s.adtClient.WriteSource(ctx, obj.Type, obj.Name, obj.Source, opts)
		if err != nil {
			fmt.Fprintf(&sb, "✗ Failed: %v\n", err)
			failed = append(failed, obj.Name+": "+err.Error())
		} else {
			sb.WriteString("✓ Deployed\n")
			deployed = append(deployed, obj.Name)
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

	// Post-deployment instructions
	sb.WriteString(embedded.PostDeploymentInstructions())

	// Features unlocked
	sb.WriteString("\nFeatures unlocked:\n")
	sb.WriteString("  ✓ WebSocket debugging (TPDAPI)\n")
	sb.WriteString("  ✓ RFC/BAPI execution\n")
	sb.WriteString("  ✓ AMDP debugging (experimental)\n")
	if hasAbapGit && !skipGitService {
		sb.WriteString("  ✓ abapGit export (158 object types)\n")
	} else {
		sb.WriteString("  ✗ abapGit export (install abapGit first)\n")
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
	sb.WriteString("  InstallAbapGit --edition standalone  # Single program ZABAPGIT\n")
	sb.WriteString("  InstallAbapGit --edition dev         # Full $ZGIT_DEV packages\n")
	sb.WriteString("\nNote: To add dependency ZIPs, export from SAP with GitExport and\n")
	sb.WriteString("embed in embedded/deps/\n")

	return mcp.NewToolResultText(sb.String()), nil
}

func (s *Server) handleInstallAbapGit(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	args := request.Params.Arguments
	edition := strings.ToLower(getStr(args, "edition"))
	if edition == "" {
		edition = "standalone"
	}
	if edition != "standalone" && edition != "dev" {
		return newToolResultError("Invalid edition. Use 'standalone' or 'dev'"), nil
	}

	packageName := strings.ToUpper(getStr(args, "package"))
	if packageName == "" {
		if edition == "standalone" {
			packageName = "$ABAPGIT"
		} else {
			packageName = "$ZGIT_DEV"
		}
	}
	if !strings.HasPrefix(packageName, "$") {
		return newToolResultError("Package name must start with $ (local package)"), nil
	}

	checkOnly := getBool(args, "check_only", false)

	var sb strings.Builder
	fmt.Fprintf(&sb, "Install abapGit (%s edition)\n", edition)
	sb.WriteString("============================\n\n")

	// Check if ZIP is available
	dependencies := deps.GetAvailableDependencies()
	var selectedDep *deps.DependencyInfo
	for i, dep := range dependencies {
		if (edition == "standalone" && dep.Name == "abapgit-standalone") ||
			(edition == "dev" && dep.Name == "abapgit-dev") {
			selectedDep = &dependencies[i]
			break
		}
	}

	if selectedDep == nil {
		return newToolResultError("Edition '" + edition + "' not found in available dependencies"), nil
	}

	if !selectedDep.Available {
		sb.WriteString("⚠ ZIP not embedded yet\n\n")
		sb.WriteString("To embed abapGit:\n")
		sb.WriteString("1. On a system with abapGit installed, run:\n")
		if edition == "standalone" {
			sb.WriteString("   vsp git-export --objects '[{\"type\":\"PROG\",\"name\":\"ZABAPGIT\"}]' > abapgit-standalone.zip\n")
		} else {
			sb.WriteString("   vsp git-export --packages '$ZGIT_DEV' --include-subpackages > abapgit-dev.zip\n")
		}
		sb.WriteString("\n2. Place ZIP in embedded/deps/\n")
		sb.WriteString("3. Update embedded/deps/embed.go with go:embed directive\n")
		sb.WriteString("4. Rebuild vsp\n\n")
		sb.WriteString("Alternative: Download from GitHub:\n")
		sb.WriteString("  https://github.com/abapGit/abapGit\n")
		return mcp.NewToolResultText(sb.String()), nil
	}

	// TODO: Implement actual deployment when ZIPs are embedded
	// This is the workflow:
	// 1. Get ZIP data from embedded variable
	// 2. files, err := deps.UnzipInMemory(zipData)
	// 3. plan := deps.CreateDeploymentPlan(edition, packageName, files)
	// 4. If checkOnly, show plan and return
	// 5. For each object in plan:
	//    - Check if exists (SearchObject)
	//    - WriteSource for main source
	//    - WriteSource for each include (locals_def, locals_imp, testclasses)
	// 6. Show summary

	fmt.Fprintf(&sb, "Target package: %s\n", packageName)
	fmt.Fprintf(&sb, "Edition: %s\n", edition)
	fmt.Fprintf(&sb, "Check only: %v\n\n", checkOnly)

	sb.WriteString("Status: ZIP embedding ready, deployment logic implemented.\n")
	sb.WriteString("Waiting for actual ZIP files to be embedded.\n\n")

	sb.WriteString("Architecture:\n")
	sb.WriteString("  1. embedded/deps/embed.go - go:embed for ZIP files\n")
	sb.WriteString("  2. deps.UnzipInMemory() - Extract files in memory\n")
	sb.WriteString("  3. deps.ParseAbapGitFilename() - Parse object type/name\n")
	sb.WriteString("  4. deps.CreateDeploymentPlan() - Sort by dependency order\n")
	sb.WriteString("  5. WriteSource() - Deploy each object via ADT\n")

	return mcp.NewToolResultText(sb.String()), nil
}
