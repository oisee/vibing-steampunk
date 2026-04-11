package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/oisee/vibing-steampunk/pkg/adt"
	"github.com/oisee/vibing-steampunk/pkg/graph"
	"github.com/spf13/cobra"
)

var apiSurfaceCmd = &cobra.Command{
	Use:   "api-surface <package>",
	Short: "Show top standard SAP APIs used by custom code in a package",
	Long: `Scan a custom package and show which standard SAP APIs it depends on,
ranked by how many custom objects reference each API.

Classification: Z*, Y*, /Z.../, /Y.../ = custom. Everything else = standard by default.

Examples:
  vsp api-surface '$ZDEV'
  vsp api-surface '$ZDEV' --include-subpackages --top 20
  vsp api-surface '$ZDEV' --with-release-state --format json`,
	Args: cobra.ExactArgs(1),
	RunE: runAPISurface,
}

func init() {
	apiSurfaceCmd.Flags().Bool("include-subpackages", false, "Include subpackages")
	apiSurfaceCmd.Flags().Int("top", 50, "Maximum APIs to show")
	apiSurfaceCmd.Flags().Bool("with-release-state", false, "Check API release state for top results (slower)")
	apiSurfaceCmd.Flags().String("format", "text", "Output format: text or json")
	rootCmd.AddCommand(apiSurfaceCmd)
}

type apiSurfacePkgObj struct {
	objType string
	name    string
}

func runAPISurface(cmd *cobra.Command, args []string) error {
	params, err := resolveSystemParams(cmd)
	if err != nil {
		return err
	}
	client, err := getClient(params)
	if err != nil {
		return err
	}

	pkg := strings.ToUpper(strings.TrimSpace(args[0]))
	includeSub, _ := cmd.Flags().GetBool("include-subpackages")
	topN, _ := cmd.Flags().GetInt("top")
	withRelease, _ := cmd.Flags().GetBool("with-release-state")
	format, _ := cmd.Flags().GetString("format")
	ctx := context.Background()

	customObjects, objects, err := loadAPISurfacePackageObjects(ctx, client, pkg, includeSub)
	if err != nil {
		return err
	}

	fmt.Fprintf(os.Stderr, "Found %d scoped custom objects (%d source-bearing).\n", len(customObjects), len(objects))
	rows, err := fetchAPISurfaceRows(ctx, client, objects)
	if err != nil {
		return err
	}
	fmt.Fprintf(os.Stderr, "Collected %d cross-references.\n", len(rows))

	result := graph.ComputeAPISurface(rows, customObjects, topN)
	result.Scope = pkg

	if withRelease && len(result.TopAPIs) > 0 {
		fmt.Fprintf(os.Stderr, "Checking release state for top %d APIs...\n", len(result.TopAPIs))
		result.ByReleaseState = make(map[string]int)
		for i := range result.TopAPIs {
			objURL := cliADTObjectURL(result.TopAPIs[i].Type, result.TopAPIs[i].Name)
			if objURL == "" {
				continue
			}
			state, err := client.GetAPIReleaseState(ctx, objURL)
			if err != nil || state == nil {
				continue
			}
			releaseState := ""
			if state.C1 != nil && state.C1.Status.State != "" {
				releaseState = strings.ToUpper(strings.TrimSpace(state.C1.Status.State))
			}
			if releaseState != "RELEASED" {
				continue
			}

			result.TopAPIs[i].ReleaseState = releaseState
			result.ByReleaseState[result.TopAPIs[i].ReleaseState]++
		}
	}

	switch format {
	case "json":
		data, err := json.MarshalIndent(result, "", "  ")
		if err != nil {
			return fmt.Errorf("marshal result: %w", err)
		}
		fmt.Println(string(data))
		return nil
	case "text":
		printAPISurfaceText(result, withRelease)
		return nil
	default:
		return fmt.Errorf("unsupported format %q (want text or json)", format)
	}
}

func loadAPISurfacePackageObjects(ctx context.Context, client *adt.Client, pkg string, includeSub bool) (map[string]bool, []apiSurfacePkgObj, error) {
	// Use shared scope + object acquisition
	scope, err := AcquirePackageScope(ctx, client, pkg, includeSub)
	if err != nil {
		return nil, nil, fmt.Errorf("scope resolution failed: %w", err)
	}

	fmt.Fprintf(os.Stderr, "Loading package %s (%d packages in scope)...\n", pkg, len(scope.Packages))
	pkgObjects, err := AcquirePackageObjects(ctx, client, ScopeToWhere(scope))
	if err != nil {
		return nil, nil, err
	}
	if len(pkgObjects) == 0 {
		return nil, nil, fmt.Errorf("package %s is empty or not found", pkg)
	}

	customObjects := make(map[string]bool, len(pkgObjects))
	objects := make([]apiSurfacePkgObj, 0, len(pkgObjects))
	for _, obj := range pkgObjects {
		customObjects[obj.Name] = true
		if IsSourceBearing(obj.Type) {
			objects = append(objects, apiSurfacePkgObj{objType: obj.Type, name: obj.Name})
		}
	}
	return customObjects, objects, nil
}

func fetchAPISurfaceRows(ctx context.Context, client *adt.Client, objects []apiSurfacePkgObj) ([]graph.APISurfaceRow, error) {
	var rows []graph.APISurfaceRow

	fmt.Fprintf(os.Stderr, "Querying object references...\n")
	for _, obj := range objects {
		include := apiSurfaceCallerInclude(obj)
		for _, ref := range queryObjectRefs(ctx, client, obj.name, obj.objType) {
			rows = append(rows, graph.APISurfaceRow{
				Include: include,
				RefName: ref.name,
				RefType: ref.otype,
				Source:  "MIXED",
			})
		}
	}

	return rows, nil
}

// isSourceBearingAPISurfaceObject removed — use shared IsSourceBearing() from acquire.go

func apiSurfaceCallerInclude(obj apiSurfacePkgObj) string {
	switch obj.objType {
	case "CLAS":
		return obj.name + "========CP"
	case "INTF":
		return obj.name + "========IP"
	case "FUGR":
		return "SAPL" + obj.name
	default:
		return obj.name
	}
}

func printAPISurfaceText(result *graph.APISurfaceResult, withRelease bool) {
	fmt.Printf("API Surface: %s (%d custom objects → %d standard APIs, %d crossings)\n\n",
		result.Scope, result.TotalCustomObjects, result.UniqueStandardAPIs, result.TotalCrossings)

	if len(result.TopAPIs) == 0 {
		fmt.Println("No standard API dependencies found.")
		return
	}

	rows := make([][]string, 0, len(result.TopAPIs))
	for _, api := range result.TopAPIs {
		callers := strings.Join(api.Callers, ", ")
		if len(callers) > 50 {
			callers = callers[:47] + "..."
		}
		row := []string{
			fmt.Sprintf("%d", api.CallerCount),
			fmt.Sprintf("%d", api.UsageCount),
			api.Type,
			api.Name,
		}
		if withRelease {
			state := api.ReleaseState
			if state == "" {
				state = "-"
			}
			row = append(row, state)
		}
		row = append(row, callers)
		rows = append(rows, row)
	}

	headers := []string{"Callers", "Refs", "Type", "API"}
	if withRelease {
		headers = append(headers, "Release")
	}
	headers = append(headers, "Used By")
	fmt.Print(formatTable(headers, rows))

	if len(result.ByReleaseState) > 0 {
		fmt.Fprintf(os.Stderr, "\nRelease state summary:")
		for state, count := range result.ByReleaseState {
			fmt.Fprintf(os.Stderr, " %s=%d", state, count)
		}
		fmt.Fprintln(os.Stderr)
	}
	fmt.Fprintf(os.Stderr, "%d standard APIs shown (of %d total)\n", len(result.TopAPIs), result.UniqueStandardAPIs)
}
