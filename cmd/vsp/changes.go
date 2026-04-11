package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strings"

	"github.com/oisee/vibing-steampunk/pkg/adt"
	"github.com/spf13/cobra"
)

var changesCmd = &cobra.Command{
	Use:   "changes <package>",
	Short: "Group transport history by CR attribute",
	Long: `Group transport requests by a configured transport attribute (E070A).

This answers "what logical changes happened in this package?" by correlating
transports that share the same attribute value (e.g. a change request number).

The attribute name is taken from transport_attribute in .vsp.json or
VSP_TRANSPORT_ATTRIBUTE env var. Transports without the attribute appear
in an "(untagged)" group.

Examples:
  vsp changes '$ZDEV' --include-subpackages
  vsp changes '$ZDEV' --since 20260101 --format json
  vsp changes '$ZDEV' --attribute SAPNOTE`,
	Args: cobra.ExactArgs(1),
	RunE: runChanges,
}

func init() {
	changesCmd.Flags().Bool("include-subpackages", false, "Include subpackages")
	changesCmd.Flags().String("since", "", "Only include transports on or after YYYYMMDD")
	changesCmd.Flags().Int("top", 0, "Maximum change groups to show (0=all)")
	changesCmd.Flags().String("format", "text", "Output format: text or json")
	changesCmd.Flags().String("attribute", "", "Override transport attribute name")
	rootCmd.AddCommand(changesCmd)
}

// changeGroup represents a set of transports sharing the same attribute value.
type changeGroup struct {
	Reference  string           `json:"reference"`
	Transports []changelogEntry `json:"transports"`
}

type changesResult struct {
	Package         string           `json:"package"`
	Attribute       string           `json:"attribute"`
	Since           string           `json:"since,omitempty"`
	TotalObjects    int              `json:"totalObjects"`
	TotalTransports int              `json:"totalTransports"`
	Groups          []changeGroup    `json:"groups"`
	Untagged        []changelogEntry `json:"untagged,omitempty"`
	TotalGroups     int              `json:"totalGroups"`
	TotalUntagged   int              `json:"totalUntagged"`
}

func runChanges(cmd *cobra.Command, args []string) error {
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
	since, _ := cmd.Flags().GetString("since")
	topN, _ := cmd.Flags().GetInt("top")
	format, _ := cmd.Flags().GetString("format")
	attrOverride, _ := cmd.Flags().GetString("attribute")
	ctx := context.Background()

	attribute := strings.ToUpper(strings.TrimSpace(attrOverride))
	if attribute == "" {
		attribute = params.TransportAttribute
	}
	if attribute == "" {
		return fmt.Errorf("no transport attribute configured. Set transport_attribute in .vsp.json, VSP_TRANSPORT_ATTRIBUTE env var, or use --attribute flag")
	}

	scope, err := AcquirePackageScope(ctx, client, pkg, includeSub)
	if err != nil {
		return fmt.Errorf("scope resolution failed: %w", err)
	}

	fmt.Fprintf(os.Stderr, "Loading package %s (%d packages in scope, attribute %s)...\n", pkg, len(scope.Packages), attribute)
	objects, err := AcquirePackageObjects(ctx, client, ScopeToWhere(scope))
	if err != nil {
		return err
	}
	if len(objects) == 0 {
		return fmt.Errorf("package %s is empty or not found", pkg)
	}

	fmt.Fprintf(os.Stderr, "Collecting transport history for %d objects...\n", len(objects))
	refs := fetchChangelogRefs(ctx, client, objects)
	clResult, err := buildChangelogResult(pkg, scope.Packages, since, 0, objects, refs, client, ctx)
	if err != nil {
		return err
	}

	fmt.Fprintf(os.Stderr, "Loading attribute %s from E070A...\n", attribute)
	attrMap, err := loadTransportAttributes(ctx, client, clResult.Entries, attribute)
	if err != nil {
		return err
	}

	result := buildChangesResult(pkg, attribute, since, len(objects), clResult.Entries, attrMap, topN)

	switch format {
	case "json":
		data, err := json.MarshalIndent(result, "", "  ")
		if err != nil {
			return fmt.Errorf("marshal result: %w", err)
		}
		fmt.Println(string(data))
		return nil
	case "text":
		printChangesText(result)
		return nil
	default:
		return fmt.Errorf("unsupported format %q (want text or json)", format)
	}
}

// loadTransportAttributes queries E070A for the given attribute on a set of transports.
// Returns transport → []reference values.
func loadTransportAttributes(ctx context.Context, client *adt.Client, entries []changelogEntry, attribute string) (map[string][]string, error) {
	if len(entries) == 0 {
		return map[string][]string{}, nil
	}

	trs := make([]string, len(entries))
	for i, e := range entries {
		trs[i] = e.Transport
	}

	result := make(map[string][]string)
	for start := 0; start < len(trs); start += 100 {
		end := start + 100
		if end > len(trs) {
			end = len(trs)
		}
		quoted := make([]string, end-start)
		for i, tr := range trs[start:end] {
			quoted[i] = fmt.Sprintf("'%s'", tr)
		}
		query := fmt.Sprintf(
			"SELECT TRKORR, REFERENCE FROM E070A WHERE ATTRIBUTE = '%s' AND TRKORR IN (%s)",
			attribute, strings.Join(quoted, ","),
		)
		qr, err := client.RunQuery(ctx, query, (end-start)*5)
		if err != nil {
			return nil, fmt.Errorf("E070A query failed: %w", err)
		}
		if qr == nil {
			continue
		}
		for _, row := range qr.Rows {
			trkorr := strings.TrimSpace(fmt.Sprintf("%v", row["TRKORR"]))
			ref := strings.TrimSpace(fmt.Sprintf("%v", row["REFERENCE"]))
			if trkorr != "" && ref != "" {
				result[trkorr] = append(result[trkorr], ref)
			}
		}
	}
	return result, nil
}

// buildChangesResult groups changelog entries by attribute reference value.
func buildChangesResult(pkg, attribute, since string, totalObjects int, entries []changelogEntry, attrMap map[string][]string, topN int) *changesResult {
	grouped := make(map[string]*changeGroup)
	var untagged []changelogEntry

	for _, entry := range entries {
		refs := attrMap[entry.Transport]
		if len(refs) == 0 {
			untagged = append(untagged, entry)
			continue
		}
		for _, ref := range refs {
			g, ok := grouped[ref]
			if !ok {
				g = &changeGroup{Reference: ref}
				grouped[ref] = g
			}
			g.Transports = append(g.Transports, entry)
		}
	}

	groups := make([]changeGroup, 0, len(grouped))
	for _, g := range grouped {
		sort.Slice(g.Transports, func(i, j int) bool {
			if g.Transports[i].Date != g.Transports[j].Date {
				return g.Transports[i].Date > g.Transports[j].Date
			}
			return g.Transports[i].Transport > g.Transports[j].Transport
		})
		groups = append(groups, *g)
	}
	sort.Slice(groups, func(i, j int) bool {
		iDate := ""
		if len(groups[i].Transports) > 0 {
			iDate = groups[i].Transports[0].Date
		}
		jDate := ""
		if len(groups[j].Transports) > 0 {
			jDate = groups[j].Transports[0].Date
		}
		if iDate != jDate {
			return iDate > jDate
		}
		return groups[i].Reference < groups[j].Reference
	})

	if topN > 0 && len(groups) > topN {
		groups = groups[:topN]
	}

	return &changesResult{
		Package:         pkg,
		Attribute:       attribute,
		Since:           since,
		TotalObjects:    totalObjects,
		TotalTransports: len(entries),
		Groups:          groups,
		Untagged:        untagged,
		TotalGroups:     len(grouped),
		TotalUntagged:   len(untagged),
	}
}

func printChangesText(result *changesResult) {
	headline := fmt.Sprintf("Changes: %s (attribute: %s)", result.Package, result.Attribute)
	if result.Since != "" {
		headline += fmt.Sprintf(" since %s", result.Since)
	}
	fmt.Println(headline)
	fmt.Println()

	if len(result.Groups) == 0 && len(result.Untagged) == 0 {
		fmt.Println("No transports found.")
		return
	}

	for _, g := range result.Groups {
		fmt.Printf("[%s]\n", g.Reference)
		for _, entry := range g.Transports {
			fmt.Printf("  %s  %s  %s", entry.Date, entry.Transport, entry.User)
			if entry.Description != "" {
				fmt.Printf("  %q", entry.Description)
			}
			fmt.Println()
			names := make([]string, 0, len(entry.Objects))
			for _, obj := range entry.Objects {
				names = append(names, obj.Type+" "+obj.Name)
			}
			if len(names) > 0 {
				fmt.Printf("    %s\n", strings.Join(names, ", "))
			}
		}
		fmt.Println()
	}

	if len(result.Untagged) > 0 {
		fmt.Println("(untagged)")
		for _, entry := range result.Untagged {
			fmt.Printf("  %s  %s  %s", entry.Date, entry.Transport, entry.User)
			if entry.Description != "" {
				fmt.Printf("  %q", entry.Description)
			}
			fmt.Println()
			names := make([]string, 0, len(entry.Objects))
			for _, obj := range entry.Objects {
				names = append(names, obj.Type+" "+obj.Name)
			}
			if len(names) > 0 {
				fmt.Printf("    %s\n", strings.Join(names, ", "))
			}
		}
		fmt.Println()
	}

	fmt.Printf("%d change groups, %d untagged, %d transports total\n",
		result.TotalGroups, result.TotalUntagged, result.TotalTransports)
}
