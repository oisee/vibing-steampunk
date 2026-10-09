package vsp

import (
	"context"
	"fmt"
	"os"
	"strings"

	embedded "github.com/oisee/vibing-steampunk/embedded/abap"
	"github.com/oisee/vibing-steampunk/embedded/deps"
	installer "github.com/oisee/vibing-steampunk/internal/install"
	"github.com/oisee/vibing-steampunk/pkg/adt"
	"github.com/spf13/cobra"
)

// --- install command ---

var installCmd = &cobra.Command{
	Use:   "install",
	Short: "Install components to SAP system",
	Long: `Install software components to a SAP system.

Subcommands:
  zadt-vsp    Install ZADT_VSP WebSocket handler (12 ABAP objects)
  abapgit     Install abapGit (standalone edition)
  list        List available installable components

Examples:
  vsp -s a4h install zadt-vsp
  vsp -s a4h install abapgit
  vsp -s a4h install list
  vsp -s a4h install zadt-vsp --dry-run`,
}

var installZadtVspCmd = &cobra.Command{
	Use:   "zadt-vsp",
	Short: "Install ZADT_VSP WebSocket handler",
	Long: `Install the ZADT_VSP WebSocket handler to enable advanced features.

Deploys 12 ABAP objects (1 interface, 9 classes, 2 programs) in dependency order:
  ZIF_VSP_SERVICE, ZCL_VSP_UTILS, ZCL_VSP_TADIR_MOVE, ZCL_VSP_RFC_SERVICE,
  ZCL_VSP_DEBUG_SERVICE, ZCL_VSP_AMDP_SERVICE, ZCL_VSP_GIT_SERVICE,
  ZCL_VSP_REPORT_SERVICE, ZCL_VSP_TRANSPORT_SERVICE, ZVSP_TRANSPORT_BUFFER,
  ZVSP_GIT_IMPORT, ZCL_VSP_APC_HANDLER

ZCL_VSP_GIT_SERVICE and ZVSP_GIT_IMPORT (and AMC application ZVSP_GIT) need
abapGit on the system and are skipped without it.

Features unlocked after install:
  - WebSocket debugging (TPDAPI)
  - RFC/BAPI execution
  - AMDP debugging (experimental)
  - abapGit export and zip import (requires abapGit)

Examples:
  vsp -s a4h install zadt-vsp
  vsp -s a4h install zadt-vsp --package '$ZADT_CUSTOM'
  vsp -s a4h install zadt-vsp --dry-run`,
	RunE: runInstallZadtVsp,
}

var installAbapGitCmd = &cobra.Command{
	Use:   "abapgit",
	Short: "Install abapGit from embedded ZIP",
	Long: `Install abapGit to a SAP system from the embedded ZIP archive.

Editions:
  standalone  Single program ZABAPGIT (default)
  full        The developer edition. Not installable yet: this build carries
              no developer-edition ZIP, so it is refused (see #277).

Examples:
  vsp -s a4h install abapgit
  vsp -s a4h install abapgit --package '$ZGIT_CUSTOM'
  vsp -s a4h install abapgit --dry-run`,
	RunE: runInstallAbapGit,
}

var installListCmd = &cobra.Command{
	Use:   "list",
	Short: "List available installable components",
	Long: `List all components that can be installed to a SAP system.

Shows embedded dependencies, their availability status, and target packages.`,
	RunE: runInstallList,
}

// --- install handler implementations ---

func runInstallZadtVsp(cmd *cobra.Command, args []string) error {
	params, err := resolveSystemParams(cmd)
	if err != nil {
		return err
	}

	client, err := getClient(params)
	if err != nil {
		return err
	}

	packageName, _ := cmd.Flags().GetString("package")
	packageName = strings.ToUpper(packageName)
	dryRun, _ := cmd.Flags().GetBool("dry-run")
	skipGitService, _ := cmd.Flags().GetBool("skip-git-service")
	budget, err := resolveCallTimeout(cmd)
	if err != nil {
		return err
	}

	// Validate package name
	if !strings.HasPrefix(packageName, "$") {
		return fmt.Errorf("package name must start with $ (local package): %s", packageName)
	}

	ctx := context.Background()

	fmt.Fprintf(os.Stderr, "ZADT_VSP Installation\n")
	fmt.Fprintf(os.Stderr, "=====================\n\n")

	// Phase 1: Check prerequisites
	fmt.Fprintf(os.Stderr, "Checking prerequisites...\n")

	// Check if package exists.
	// GetPackage reads the nodestructure API and cannot distinguish
	// "package does not exist" from "package exists but has no children",
	// so we use the direct PackageExists probe here. An inconclusive probe
	// must not be treated as absence, because that could turn a network or
	// authorization failure into an unintended create attempt.
	packageExists, err := client.PackageExists(ctx, packageName)
	if err != nil {
		return fmt.Errorf("failed to check package %s: %w", packageName, err)
	}
	if packageExists {
		fmt.Fprintf(os.Stderr, "  Package %s exists\n", packageName)
	} else {
		fmt.Fprintf(os.Stderr, "  Package %s will be created\n", packageName)
	}

	// Check for abapGit (for Git service)
	hasAbapGit := false
	if !skipGitService {
		results, err := client.SearchObject(ctx, "ZCL_ABAPGIT_OBJECTS", 1)
		if err == nil && len(results) > 0 {
			hasAbapGit = true
			fmt.Fprintf(os.Stderr, "  abapGit detected -> Git service will be deployed\n")
		} else {
			fmt.Fprintf(os.Stderr, "  abapGit not detected -> Git service will be skipped\n")
			skipGitService = true
		}
	} else {
		fmt.Fprintf(os.Stderr, "  Git service skipped (--skip-git-service)\n")
	}

	// Get objects to deploy
	objects := embedded.GetObjects()

	// Check existing objects
	existingObjects := []string{}
	for _, obj := range objects {
		results, err := client.SearchObject(ctx, obj.Name, 1)
		if err == nil && len(results) > 0 && results[0].Name == obj.Name {
			existingObjects = append(existingObjects, obj.Name)
		}
	}
	if len(existingObjects) > 0 {
		fmt.Fprintf(os.Stderr, "  Existing objects will be updated: %s\n", strings.Join(existingObjects, ", "))
	}

	fmt.Fprintf(os.Stderr, "\n")

	// Show deployment plan
	fmt.Fprintf(os.Stderr, "Deployment Plan (%d objects):\n", len(objects))
	fmt.Fprintf(os.Stderr, "%s\n", strings.Repeat("-", 60))
	for i, obj := range objects {
		if obj.RequiresAbapGit && skipGitService {
			fmt.Fprintf(os.Stderr, "  [%d/%d] %-30s SKIP (no abapGit)\n", i+1, len(objects), obj.Name)
		} else {
			action := "CREATE"
			for _, existing := range existingObjects {
				if existing == obj.Name {
					action = "UPDATE"
					break
				}
			}
			fmt.Fprintf(os.Stderr, "  [%d/%d] %-30s %s - %s\n", i+1, len(objects), obj.Name, action, obj.Description)
		}
	}
	fmt.Fprintf(os.Stderr, "%s\n\n", strings.Repeat("-", 60))

	if dryRun {
		fmt.Fprintf(os.Stderr, "Dry run - no changes made.\n")
		return nil
	}

	// Phase 2: Create package if needed
	created, err := installer.EnsurePackage(ctx, client, packageName, "VSP WebSocket Handler")
	if err != nil {
		return fmt.Errorf("failed to ensure package %s: %w", packageName, err)
	}
	if created {
		fmt.Fprintf(os.Stderr, "Package %s created and verified\n\n", packageName)
	} else {
		fmt.Fprintf(os.Stderr, "Using existing package %s\n\n", packageName)
	}

	// Phase 3: Deploy objects
	fmt.Fprintf(os.Stderr, "Deploying ABAP objects...\n")

	deployed := 0
	skipped := 0
	failed := 0

	for i, obj := range objects {
		// Skip the git service and its job program if no abapGit
		if obj.RequiresAbapGit && skipGitService {
			fmt.Fprintf(os.Stderr, "  [%d/%d] %s ... SKIPPED (no abapGit)\n", i+1, len(objects), obj.Name)
			skipped++
			continue
		}

		fmt.Fprintf(os.Stderr, "  [%d/%d] %s ... ", i+1, len(objects), obj.Name)

		opts := &adt.WriteSourceOptions{
			Package:     packageName,
			Description: obj.Description,
			Mode:        adt.WriteModeUpsert,
		}
		objCtx, cancel := withWriteBudget(ctx, budget)
		_, err := installer.DeploySource(objCtx, client, obj.Type, obj.Name, obj.Source, opts)
		cancel()
		if err != nil {
			fmt.Fprintf(os.Stderr, "FAILED: %v\n", err)
			failed++
		} else {
			fmt.Fprintf(os.Stderr, "OK\n")
			deployed++
		}
	}

	// The transport service's push channel. Without it uploads still work;
	// their outcome is read with vsp transport status.
	fmt.Fprintf(os.Stderr, "  AMC %s (transport push) ... ", embedded.AMCApplicationName)
	// Each AMC application is one write and activation, under the same
	// per-object budget as the classes above.
	amcCtx, amcCancel := withWriteBudget(ctx, budget)
	err = client.UpsertAMCApplication(amcCtx, embedded.AMCApplicationName, embedded.AMCApplicationDescription,
		packageName, embedded.AMCApplicationDefinition)
	amcCancel()
	if err != nil {
		fmt.Fprintf(os.Stderr, "not set up (%v); upload outcomes are read with vsp transport status\n", err)
	} else {
		fmt.Fprintf(os.Stderr, "OK\n")
	}
	// The git import's push channel, only with the git service it names.
	if !skipGitService {
		fmt.Fprintf(os.Stderr, "  AMC %s (git import push) ... ", embedded.AMCGitApplicationName)
		amcCtx, amcCancel := withWriteBudget(ctx, budget)
		err = client.UpsertAMCApplication(amcCtx, embedded.AMCGitApplicationName, embedded.AMCGitApplicationDescription,
			packageName, embedded.AMCGitApplicationDefinition)
		amcCancel()
		if err != nil {
			fmt.Fprintf(os.Stderr, "not set up (%v); import outcomes are read with vsp git import-status\n", err)
		} else {
			fmt.Fprintf(os.Stderr, "OK\n")
		}
	}

	fmt.Fprintf(os.Stderr, "\n")

	// Summary
	if failed > 0 {
		fmt.Fprintf(os.Stderr, "DEPLOYMENT PARTIALLY FAILED\n")
		fmt.Fprintf(os.Stderr, "Deployed: %d, Skipped: %d, Failed: %d\n\n", deployed, skipped, failed)
		return fmt.Errorf("%d object(s) failed to deploy; post-deployment features were not declared ready", failed)
	} else {
		fmt.Fprintf(os.Stderr, "DEPLOYMENT COMPLETE\n")
		fmt.Fprintf(os.Stderr, "Deployed: %d, Skipped: %d\n\n", deployed, skipped)
	}

	// Post-deployment instructions
	fmt.Fprint(os.Stderr, embedded.PostDeploymentInstructions())

	// Features summary
	fmt.Fprintf(os.Stderr, "\nFeatures unlocked:\n")
	fmt.Fprintf(os.Stderr, "  WebSocket debugging (TPDAPI)\n")
	fmt.Fprintf(os.Stderr, "  RFC/BAPI execution\n")
	fmt.Fprintf(os.Stderr, "  AMDP debugging (experimental)\n")
	if hasAbapGit && !skipGitService {
		fmt.Fprintf(os.Stderr, "  abapGit export and zip import (vsp git import-zip)\n")
	} else {
		fmt.Fprintf(os.Stderr, "  abapGit export and import NOT available (install abapGit first)\n")
	}

	return nil
}

func runInstallAbapGit(cmd *cobra.Command, args []string) error {
	edition, _ := cmd.Flags().GetString("edition")
	edition = strings.ToLower(edition)
	packageName, _ := cmd.Flags().GetString("package")
	dryRun, _ := cmd.Flags().GetBool("dry-run")

	// Validate edition before touching the system. Only the standalone edition
	// is installable: the developer-edition ZIP this build embeds is empty.
	switch edition {
	case "standalone":
	case "full", "dev":
		return fmt.Errorf("the developer edition is not installable yet; see #277. " +
			"Use --edition standalone, or install the developer version from https://github.com/abapGit/abapGit")
	default:
		return fmt.Errorf("invalid edition '%s'. Use 'standalone'", edition)
	}

	params, err := resolveSystemParams(cmd)
	if err != nil {
		return err
	}

	client, err := getClient(params)
	if err != nil {
		return err
	}

	budget, err := resolveCallTimeout(cmd)
	if err != nil {
		return err
	}

	depName := "abapgit-standalone"
	if packageName == "" {
		packageName = "$ABAPGIT"
	}
	packageName = strings.ToUpper(packageName)

	// Validate package name
	if !strings.HasPrefix(packageName, "$") {
		return fmt.Errorf("package name must start with $ (local package): %s", packageName)
	}

	fmt.Fprintf(os.Stderr, "Install abapGit (%s edition)\n", edition)
	fmt.Fprintf(os.Stderr, "============================\n\n")

	// Get ZIP data
	zipData, depErr := deps.RequireDependencyZIP(depName)
	if depErr != nil {
		fmt.Fprintf(os.Stderr, "ZIP not embedded for edition '%s'\n\n", edition)
		fmt.Fprintf(os.Stderr, "To embed abapGit:\n")
		fmt.Fprintf(os.Stderr, "1. On a system with abapGit installed, run:\n")
		fmt.Fprintf(os.Stderr, "   vsp export '$ABAPGIT' -o abapgit-standalone.zip\n")
		fmt.Fprintf(os.Stderr, "\n2. Place ZIP in embedded/deps/\n")
		fmt.Fprintf(os.Stderr, "3. Update embedded/deps/embed.go with go:embed directive\n")
		fmt.Fprintf(os.Stderr, "4. Rebuild vsp\n\n")
		fmt.Fprintf(os.Stderr, "Alternative: Download from https://github.com/abapGit/abapGit\n")
		return depErr
	}

	fmt.Fprintf(os.Stderr, "Source: %s (embedded, %d bytes)\n", depName, len(zipData))
	fmt.Fprintf(os.Stderr, "Target: %s\n\n", packageName)

	// Parse ZIP
	files, err := deps.UnzipInMemory(zipData)
	if err != nil {
		return fmt.Errorf("failed to parse ZIP: %w", err)
	}

	// Create deployment plan
	plan := deps.CreateDeploymentPlan(depName, packageName, files)
	fmt.Fprintf(os.Stderr, "Found %d objects in %d files\n\n", plan.TotalObjects, plan.TotalFiles)

	// Show deployment plan
	fmt.Fprintf(os.Stderr, "Deployment Plan (%d objects):\n", plan.TotalObjects)
	fmt.Fprintf(os.Stderr, "%s\n", strings.Repeat("-", 60))
	for i, obj := range plan.Objects {
		includeInfo := ""
		if len(obj.Includes) > 0 {
			var incTypes []string
			for t := range obj.Includes {
				incTypes = append(incTypes, t)
			}
			includeInfo = fmt.Sprintf(" [+%s]", strings.Join(incTypes, ","))
		}
		fmt.Fprintf(os.Stderr, "  [%d/%d] %-6s %-40s%s\n", i+1, plan.TotalObjects, obj.Type, obj.Name, includeInfo)
	}
	fmt.Fprintf(os.Stderr, "%s\n\n", strings.Repeat("-", 60))

	if dryRun {
		fmt.Fprintf(os.Stderr, "Dry run - no changes made.\n")
		return nil
	}

	ctx := context.Background()

	// Ensure package exists. PackageExists probes /packages/{name} directly;
	// a GetPackage-based check cannot distinguish "absent" from "present but
	// empty" because nodestructure returns an empty tree in both cases.
	fmt.Fprintf(os.Stderr, "Checking package %s...\n", packageName)
	created, pkgErr := installer.EnsurePackage(ctx, client, packageName, fmt.Sprintf("abapGit %s edition", edition))
	if pkgErr != nil {
		return fmt.Errorf("failed to ensure package: %w", pkgErr)
	}
	if created {
		fmt.Fprintf(os.Stderr, "  Package created and verified\n")
	} else {
		fmt.Fprintf(os.Stderr, "  Package exists\n")
	}

	// Deploy objects
	fmt.Fprintf(os.Stderr, "\nDeploying objects...\n")
	success := 0
	failCount := 0

	for i, obj := range plan.Objects {
		if obj.MainSource == "" {
			continue // skip XML-only entries
		}

		fmt.Fprintf(os.Stderr, "  [%d/%d] %s %s ... ", i+1, plan.TotalObjects, obj.Type, obj.Name)

		desc := obj.Description
		if desc == "" {
			desc = fmt.Sprintf("Deployed: %s", obj.Name)
		}

		wopts := &adt.WriteSourceOptions{
			Package:     packageName,
			Description: desc,
			Mode:        adt.WriteModeUpsert,
		}
		objCtx, cancel := withWriteBudget(ctx, budget)
		_, err := installer.DeploySource(objCtx, client, obj.Type, obj.Name, obj.MainSource, wopts)
		cancel()
		if err != nil {
			fmt.Fprintf(os.Stderr, "FAILED: %v\n", err)
			failCount++
		} else {
			fmt.Fprintf(os.Stderr, "OK\n")
			success++
		}
	}

	fmt.Fprintf(os.Stderr, "\nDeployment complete: %d success, %d failed\n", success, failCount)

	if failCount > 0 {
		return fmt.Errorf("%d object(s) failed to deploy", failCount)
	}
	return nil
}

// objectKindSummary counts the objects by kind, as "1 interface, 9 classes,
// 1 program", so that the count cannot drift from the list.
func objectKindSummary(objects []embedded.ObjectInfo) string {
	names := []struct{ typ, one, many string }{
		{"INTF", "interface", "interfaces"}, {"CLAS", "class", "classes"}, {"PROG", "program", "programs"},
	}
	var parts []string
	for _, n := range names {
		c := 0
		for _, o := range objects {
			if o.Type == n.typ {
				c++
			}
		}
		switch {
		case c == 1:
			parts = append(parts, "1 "+n.one)
		case c > 1:
			parts = append(parts, fmt.Sprintf("%d %s", c, n.many))
		}
	}
	return strings.Join(parts, ", ")
}

func runInstallList(_ *cobra.Command, _ []string) error {
	fmt.Println("Available Installable Components")
	fmt.Println("================================")
	fmt.Println()

	// ZADT_VSP
	objects := embedded.GetObjects()
	fmt.Printf("1. zadt-vsp\n")
	fmt.Printf("   Description: ZADT_VSP WebSocket handler for advanced features\n")
	fmt.Printf("   Default package: $ZADT_VSP\n")
	fmt.Printf("   Objects: %d (%s)\n", len(objects), objectKindSummary(objects))
	fmt.Printf("   Status: Embedded (always available)\n")
	fmt.Printf("   Install: vsp install zadt-vsp\n")
	fmt.Println()

	// Embedded dependencies (abapGit editions)
	dependencies := deps.GetAvailableDependencies()
	for i, dep := range dependencies {
		status := "Not embedded (placeholder)"
		if dep.Available {
			status = "Available"
		}
		fmt.Printf("%d. %s\n", i+2, dep.Name)
		fmt.Printf("   Description: %s\n", dep.Description)
		fmt.Printf("   Default package: %s\n", dep.Package)
		fmt.Printf("   Status: %s\n", status)
		if dep.Available {
			edition := "standalone"
			if strings.Contains(dep.Name, "full") {
				edition = "full"
			}
			fmt.Printf("   Install: vsp install abapgit --edition %s\n", edition)
		}
		fmt.Println()
	}

	return nil
}

func init() {
	// Install flags
	installZadtVspCmd.Flags().String("package", "$ZADT_VSP", "Target package for ZADT_VSP objects")
	installZadtVspCmd.Flags().Bool("dry-run", false, "Show what would be deployed without deploying")
	installZadtVspCmd.Flags().Bool("skip-git-service", false, "Skip ZCL_VSP_GIT_SERVICE even if abapGit is detected")

	installAbapGitCmd.Flags().String("edition", "standalone", "abapGit edition: standalone (the developer edition is not installable yet, #277)")
	installAbapGitCmd.Flags().String("package", "", "Target package (default: $ABAPGIT)")
	installAbapGitCmd.Flags().Bool("dry-run", false, "Show what would be deployed without deploying")
	addCallTimeoutFlag(installZadtVspCmd, installAbapGitCmd)

	// Install subcommands
	installCmd.AddCommand(installZadtVspCmd)
	installCmd.AddCommand(installAbapGitCmd)
	installCmd.AddCommand(installListCmd)

	rootCmd.AddCommand(installCmd)
}
