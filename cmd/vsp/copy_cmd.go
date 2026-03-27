package main

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/oisee/vibing-steampunk/embedded/deps"
	"github.com/oisee/vibing-steampunk/pkg/adt"
	"github.com/spf13/cobra"
)

var (
	copyToPackage  string
	copyEmbedded   string
	copyObjectType string
	copyObjectName string
	copyDryRun     bool
)

func init() {
	copyCmd.Flags().StringVar(&copyToPackage, "to", "", "Target package (e.g., $ZGIT, $ZADT_VSP)")
	copyCmd.Flags().StringVar(&copyEmbedded, "embedded", "", "Use embedded dependency (abapgit-standalone, abapgit-full)")
	copyCmd.Flags().StringVar(&copyObjectType, "type", "", "Filter by object type (e.g., CLAS, PROG)")
	copyCmd.Flags().StringVar(&copyObjectName, "name", "", "Filter by object name pattern (e.g., ZCL_*)")
	copyCmd.Flags().BoolVar(&copyDryRun, "dry-run", false, "Show what would be deployed without deploying")
	copyCmd.MarkFlagRequired("to")

	rootCmd.AddCommand(copyCmd)
}

var copyCmd = &cobra.Command{
	Use:   "copy <source.zip> --to <$PACKAGE>",
	Short: "Copy objects from ZIP to SAP package",
	Long: `Copy ABAP objects from an abapGit-format ZIP file to a SAP package.

If ZADT_VSP WebSocket handler is available, uses it for full 158 object type support.
Otherwise falls back to ADT native deployment (PROG, CLAS, INTF, DDLS, BDEF, SRVD).

Examples:
  # Copy from ZIP file
  vsp -s a4h copy mypackage.zip --to $ZPACKAGE

  # Copy embedded abapGit standalone
  vsp -s a4h copy --embedded abapgit-standalone --to $ZGIT

  # Copy embedded full abapGit
  vsp -s a4h copy --embedded abapgit-full --to $ZGIT

  # Dry run (show what would be deployed)
  vsp -s a4h copy package.zip --to $ZPACKAGE --dry-run

  # Filter by type
  vsp -s a4h copy package.zip --to $ZPACKAGE --type CLAS
`,
	Args: func(cmd *cobra.Command, args []string) error {
		if copyEmbedded == "" && len(args) == 0 {
			return fmt.Errorf("requires source ZIP file or --embedded flag")
		}
		return nil
	},
	RunE: runCopy,
}

func runCopy(cmd *cobra.Command, args []string) error {
	// Resolve system parameters
	params, err := resolveSystemParams(cmd)
	if err != nil {
		return err
	}

	// Create ADT client
	client, err := getClient(params)
	if err != nil {
		return err
	}

	ctx := context.Background()

	// Get ZIP data
	var zipData []byte
	var sourceName string

	if copyEmbedded != "" {
		// Use embedded dependency
		zipData = deps.GetDependencyZIP(copyEmbedded)
		if zipData == nil {
			available := deps.GetAvailableDependencies()
			var names []string
			for _, d := range available {
				if d.Available {
					names = append(names, d.Name)
				}
			}
			return fmt.Errorf("embedded dependency '%s' not found. Available: %s", copyEmbedded, strings.Join(names, ", "))
		}
		sourceName = copyEmbedded + " (embedded)"
	} else {
		// Read from file
		sourceFile := args[0]
		zipData, err = os.ReadFile(sourceFile)
		if err != nil {
			return fmt.Errorf("failed to read ZIP file: %w", err)
		}
		sourceName = sourceFile
	}

	fmt.Printf("Source: %s (%d bytes)\n", sourceName, len(zipData))
	fmt.Printf("Target: %s\n", copyToPackage)
	fmt.Println()

	// Parse ZIP
	files, err := deps.UnzipInMemory(zipData)
	if err != nil {
		return fmt.Errorf("failed to parse ZIP: %w", err)
	}

	// Create deployment plan
	plan := deps.CreateDeploymentPlan(sourceName, copyToPackage, files)
	fmt.Printf("Found %d objects in %d files\n\n", plan.TotalObjects, plan.TotalFiles)

	// Filter by type/name if specified
	var filteredObjects []deps.DeploymentObject
	for _, obj := range plan.Objects {
		if copyObjectType != "" && obj.Type != strings.ToUpper(copyObjectType) {
			continue
		}
		if copyObjectName != "" {
			pattern := strings.ToUpper(copyObjectName)
			pattern = strings.ReplaceAll(pattern, "*", "")
			if !strings.Contains(obj.Name, pattern) {
				continue
			}
		}
		filteredObjects = append(filteredObjects, obj)
	}

	if len(filteredObjects) == 0 {
		fmt.Println("No objects match the filter criteria")
		return nil
	}

	// Check if ZADT_VSP is available
	wsAvailable := checkWebSocketAvailable(client, ctx)
	if wsAvailable {
		fmt.Println("Mode: WebSocket (ZADT_VSP available - full object type support)")
	} else {
		fmt.Println("Mode: ADT Native (fallback - PROG, CLAS, INTF, DDLS, BDEF, SRVD)")
	}
	fmt.Println()

	// Show deployment plan
	fmt.Printf("Deployment Plan (%d objects):\n", len(filteredObjects))
	fmt.Println(strings.Repeat("-", 60))

	adtSupported := map[string]bool{
		"PROG": true,
		"CLAS": true,
		"INTF": true,
		"DDLS": true,
		"BDEF": true,
		"SRVD": true,
	}

	var deployable, skipped int
	for _, obj := range filteredObjects {
		supported := wsAvailable || adtSupported[obj.Type]
		status := "✓"
		if !supported {
			status = "⊘ (requires ZADT_VSP)"
			skipped++
		} else {
			deployable++
		}

		includeInfo := ""
		if len(obj.Includes) > 0 {
			var incTypes []string
			for t := range obj.Includes {
				incTypes = append(incTypes, t)
			}
			includeInfo = fmt.Sprintf(" [+%s]", strings.Join(incTypes, ","))
		}

		fmt.Printf("  %s %-6s %-40s%s\n", status, obj.Type, obj.Name, includeInfo)
	}

	fmt.Println(strings.Repeat("-", 60))
	fmt.Printf("Deployable: %d, Skipped: %d\n\n", deployable, skipped)

	if copyDryRun {
		fmt.Println("Dry run - no changes made")
		return nil
	}

	if deployable == 0 {
		fmt.Println("Nothing to deploy")
		return nil
	}

	// Ensure package exists
	fmt.Printf("Checking package %s...\n", copyToPackage)
	_, err = client.GetPackage(ctx, copyToPackage)
	if err != nil {
		fmt.Printf("Creating package %s...\n", copyToPackage)
		err = client.CreateObject(ctx, adt.CreateObjectOptions{
			ObjectType:  adt.ObjectTypePackage,
			Name:        copyToPackage,
			Description: fmt.Sprintf("Deployed from %s", sourceName),
		})
		if err != nil {
			return fmt.Errorf("failed to create package: %w", err)
		}
	}

	// Deploy objects
	fmt.Println("\nDeploying objects...")
	var success, failed int

	for _, obj := range filteredObjects {
		supported := wsAvailable || adtSupported[obj.Type]
		if !supported {
			continue
		}

		fmt.Printf("  Deploying %s %s... ", obj.Type, obj.Name)

		err := deployObject(ctx, client, obj, copyToPackage, wsAvailable)
		if err != nil {
			fmt.Printf("FAILED: %v\n", err)
			failed++
		} else {
			fmt.Printf("OK\n")
			success++
		}
	}

	fmt.Println()
	fmt.Printf("Deployment complete: %d success, %d failed\n", success, failed)

	if !wsAvailable && skipped > 0 {
		fmt.Println("\nNote: Some objects were skipped because ZADT_VSP is not installed.")
		fmt.Println("Install ZADT_VSP first to enable full object type support:")
		fmt.Println("  vsp -s <system> copy --embedded zadt-vsp --to $ZADT_VSP")
	}

	return nil
}

// checkWebSocketAvailable tests if ZADT_VSP WebSocket is available.
func checkWebSocketAvailable(client *adt.Client, ctx context.Context) bool {
	// Try to find ZCL_VSP_APC_HANDLER - if it exists, WebSocket is likely available
	results, err := client.SearchObject(ctx, "ZCL_VSP_APC_HANDLER", 1)
	if err != nil || len(results) == 0 {
		return false
	}
	return results[0].Name == "ZCL_VSP_APC_HANDLER"
}

// deployObject deploys a single object using ADT native or WebSocket.
func deployObject(ctx context.Context, client *adt.Client, obj deps.DeploymentObject, packageName string, useWebSocket bool) error {
	if useWebSocket {
		// TODO: Implement WebSocket-based deployment
		// For now, fall through to ADT native
	}

	// ADT Native deployment
	switch obj.Type {
	case "PROG":
		return deployProgram(ctx, client, obj, packageName)
	case "CLAS":
		return deployClass(ctx, client, obj, packageName)
	case "INTF":
		return deployInterface(ctx, client, obj, packageName)
	case "DDLS":
		return deployDDLS(ctx, client, obj, packageName)
	case "BDEF":
		return deployBDEF(ctx, client, obj, packageName)
	case "SRVD":
		return deploySRVD(ctx, client, obj, packageName)
	default:
		return fmt.Errorf("unsupported object type: %s", obj.Type)
	}
}

func deployProgram(ctx context.Context, client *adt.Client, obj deps.DeploymentObject, packageName string) error {
	desc := obj.Description
	if desc == "" {
		desc = fmt.Sprintf("Deployed: %s", obj.Name)
	}
	result, err := client.WriteSource(ctx, "PROG", obj.Name, obj.MainSource, &adt.WriteSourceOptions{
		Package:     packageName,
		Description: desc,
	})
	if os.Getenv("VSP_DEBUG") == "true" && result != nil {
		fmt.Fprintf(os.Stderr, "[DEBUG] WriteSource result: Success=%v, Mode=%s, Message=%s\n", result.Success, result.Mode, result.Message)
	}
	if err != nil {
		return err
	}
	if !result.Success {
		return fmt.Errorf("WriteSource failed: %s", result.Message)
	}
	return nil
}

func deployClass(ctx context.Context, client *adt.Client, obj deps.DeploymentObject, packageName string) error {
	desc := obj.Description
	if desc == "" {
		desc = fmt.Sprintf("Deployed: %s", obj.Name)
	}
	// Deploy main class
	_, err := client.WriteSource(ctx, "CLAS", obj.Name, obj.MainSource, &adt.WriteSourceOptions{
		Package:     packageName,
		Description: desc,
	})
	if err != nil {
		return err
	}

	// Deploy includes (TODO: implement include deployment)
	if len(obj.Includes) > 0 {
		var incTypes []string
		for t := range obj.Includes {
			incTypes = append(incTypes, t)
		}
		fmt.Printf("\n    Note: class includes [%s] not yet deployed (TODO)\n", strings.Join(incTypes, ","))
	}

	return nil
}

func deployInterface(ctx context.Context, client *adt.Client, obj deps.DeploymentObject, packageName string) error {
	desc := obj.Description
	if desc == "" {
		desc = fmt.Sprintf("Deployed: %s", obj.Name)
	}
	_, err := client.WriteSource(ctx, "INTF", obj.Name, obj.MainSource, &adt.WriteSourceOptions{
		Package:     packageName,
		Description: desc,
	})
	return err
}

func deployDDLS(ctx context.Context, client *adt.Client, obj deps.DeploymentObject, packageName string) error {
	desc := obj.Description
	if desc == "" {
		desc = fmt.Sprintf("Deployed: %s", obj.Name)
	}
	_, err := client.WriteSource(ctx, "DDLS", obj.Name, obj.MainSource, &adt.WriteSourceOptions{
		Package:     packageName,
		Description: desc,
	})
	return err
}

func deployBDEF(ctx context.Context, client *adt.Client, obj deps.DeploymentObject, packageName string) error {
	desc := obj.Description
	if desc == "" {
		desc = fmt.Sprintf("Deployed: %s", obj.Name)
	}
	_, err := client.WriteSource(ctx, "BDEF", obj.Name, obj.MainSource, &adt.WriteSourceOptions{
		Package:     packageName,
		Description: desc,
	})
	return err
}

func deploySRVD(ctx context.Context, client *adt.Client, obj deps.DeploymentObject, packageName string) error {
	desc := obj.Description
	if desc == "" {
		desc = fmt.Sprintf("Deployed: %s", obj.Name)
	}
	_, err := client.WriteSource(ctx, "SRVD", obj.Name, obj.MainSource, &adt.WriteSourceOptions{
		Package:     packageName,
		Description: desc,
	})
	return err
}
