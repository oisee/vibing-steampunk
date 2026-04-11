package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strings"

	"github.com/oisee/vibing-steampunk/pkg/adt"
	"github.com/oisee/vibing-steampunk/pkg/graph"
	"github.com/spf13/cobra"
)

var changelogCmd = &cobra.Command{
	Use:   "changelog <package>",
	Short: "Show transport-based change history for a package",
	Long: `Show a transport-based changelog for a package.

The result is aggregated by transport request, not by individual object revision.
This gives a practical answer to "what changed in this package recently?"
without opening SE10/SE03.

Examples:
  vsp changelog '$ZDEV'
  vsp changelog '$ZDEV' --since 20260101
  vsp changelog '$ZDEV' --top 20
  vsp changelog '$ZDEV' --include-subpackages --format json`,
	Args: cobra.ExactArgs(1),
	RunE: runChangelog,
}

func init() {
	changelogCmd.Flags().Bool("include-subpackages", false, "Include subpackages")
	changelogCmd.Flags().String("since", "", "Only include transports on or after YYYYMMDD")
	changelogCmd.Flags().Int("top", 20, "Maximum transport entries to show")
	changelogCmd.Flags().String("format", "text", "Output format: text or json")
	rootCmd.AddCommand(changelogCmd)
}

type changelogObject struct {
	Type string `json:"type"`
	Name string `json:"name"`
}

type changelogEntry struct {
	Transport   string            `json:"transport"`
	Date        string            `json:"date"`
	User        string            `json:"user"`
	Description string            `json:"description,omitempty"`
	Type        string            `json:"type,omitempty"`
	Status      string            `json:"status,omitempty"`
	Objects     []changelogObject `json:"objects"`
}

type changelogResult struct {
	Package         string           `json:"package"`
	ScopePackages   []string         `json:"scopePackages,omitempty"`
	Since           string           `json:"since,omitempty"`
	TotalObjects    int              `json:"totalObjects"`
	TotalTransports int              `json:"totalTransports"`
	Entries         []changelogEntry `json:"entries"`
}

type transportRef struct {
	TRKORR  string
	PGMID   string
	Object  string
	ObjName string
}

func runChangelog(cmd *cobra.Command, args []string) error {
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
	ctx := context.Background()

	scope, err := AcquirePackageScope(ctx, client, pkg, includeSub)
	if err != nil {
		return fmt.Errorf("scope resolution failed: %w", err)
	}

	fmt.Fprintf(os.Stderr, "Loading package %s (%d packages in scope)...\n", pkg, len(scope.Packages))
	objects, err := AcquirePackageObjects(ctx, client, ScopeToWhere(scope))
	if err != nil {
		return err
	}
	if len(objects) == 0 {
		return fmt.Errorf("package %s is empty or not found", pkg)
	}

	fmt.Fprintf(os.Stderr, "Collecting transport history for %d objects...\n", len(objects))
	refs := fetchChangelogRefs(ctx, client, objects)
	result, err := buildChangelogResult(pkg, scope.Packages, since, topN, objects, refs, client, ctx)
	if err != nil {
		return err
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
		printChangelogText(result)
		return nil
	default:
		return fmt.Errorf("unsupported format %q (want text or json)", format)
	}
}

func fetchChangelogRefs(ctx context.Context, client *adt.Client, objects []PackageObject) []transportRef {
	var refs []transportRef
	for idx, obj := range objects {
		fmt.Fprintf(os.Stderr, "\r  [%d/%d] %s %-40s", idx+1, len(objects), obj.Type, obj.Name)
		query := fmt.Sprintf(
			"SELECT TRKORR, PGMID, OBJECT, OBJ_NAME FROM E071 WHERE PGMID = 'R3TR' AND OBJECT = '%s' AND OBJ_NAME = '%s'",
			obj.Type, obj.Name,
		)
		result, err := client.RunQuery(ctx, query, 200)
		if err != nil || result == nil {
			continue
		}
		for _, row := range result.Rows {
			ref := transportRef{
				TRKORR:  strings.TrimSpace(fmt.Sprintf("%v", row["TRKORR"])),
				PGMID:   strings.TrimSpace(fmt.Sprintf("%v", row["PGMID"])),
				Object:  strings.TrimSpace(fmt.Sprintf("%v", row["OBJECT"])),
				ObjName: strings.TrimSpace(fmt.Sprintf("%v", row["OBJ_NAME"])),
			}
			if ref.TRKORR == "" || ref.Object == "" || ref.ObjName == "" {
				continue
			}
			refs = append(refs, ref)
		}
	}
	if len(objects) > 0 {
		fmt.Fprintf(os.Stderr, "\r")
	}
	return refs
}

func buildChangelogResult(pkg string, scopePkgs []string, since string, topN int, objects []PackageObject, refs []transportRef, client *adt.Client, ctx context.Context) (*changelogResult, error) {
	result := &changelogResult{
		Package:       pkg,
		ScopePackages: append([]string(nil), scopePkgs...),
		Since:         since,
		TotalObjects:  len(objects),
	}

	if len(refs) == 0 {
		return result, nil
	}

	headers, err := loadTransportHeaders(ctx, client, refs)
	if err != nil {
		return nil, err
	}

	entries := aggregateChangelogEntries(headers, refs, since)
	sort.Slice(entries, func(i, j int) bool {
		if entries[i].Date != entries[j].Date {
			return entries[i].Date > entries[j].Date
		}
		return entries[i].Transport > entries[j].Transport
	})

	result.TotalTransports = len(entries)
	if topN > 0 && len(entries) > topN {
		entries = entries[:topN]
	}
	result.Entries = entries
	return result, nil
}

func loadTransportHeaders(ctx context.Context, client *adt.Client, refs []transportRef) (map[string]graph.TransportHeader, error) {
	seen := make(map[string]bool)
	trs := make([]string, 0, len(refs))
	for _, ref := range refs {
		if ref.TRKORR == "" || seen[ref.TRKORR] {
			continue
		}
		seen[ref.TRKORR] = true
		trs = append(trs, ref.TRKORR)
	}
	if len(trs) == 0 {
		return map[string]graph.TransportHeader{}, nil
	}

	headers := make(map[string]graph.TransportHeader, len(trs))
	for start := 0; start < len(trs); start += 100 {
		end := start + 100
		if end > len(trs) {
			end = len(trs)
		}
		chunk := trs[start:end]
		quoted := make([]string, len(chunk))
		for i, tr := range chunk {
			quoted[i] = fmt.Sprintf("'%s'", tr)
		}
		query := fmt.Sprintf(
			"SELECT TRKORR, STRKORR, TRFUNCTION, TRSTATUS, AS4USER, AS4DATE FROM E070 WHERE TRKORR IN (%s)",
			strings.Join(quoted, ","),
		)
		result, err := client.RunQuery(ctx, query, len(chunk)+20)
		if err != nil {
			return nil, fmt.Errorf("E070 query failed: %w", err)
		}
		if result == nil {
			continue
		}
		for _, row := range result.Rows {
			h := graph.TransportHeader{
				TRKORR:     strings.TrimSpace(fmt.Sprintf("%v", row["TRKORR"])),
				STRKORR:    strings.TrimSpace(fmt.Sprintf("%v", row["STRKORR"])),
				TRFUNCTION: strings.TrimSpace(fmt.Sprintf("%v", row["TRFUNCTION"])),
				TRSTATUS:   strings.TrimSpace(fmt.Sprintf("%v", row["TRSTATUS"])),
				AS4USER:    strings.TrimSpace(fmt.Sprintf("%v", row["AS4USER"])),
				AS4DATE:    strings.TrimSpace(fmt.Sprintf("%v", row["AS4DATE"])),
			}
			if h.TRKORR != "" {
				headers[h.TRKORR] = h
			}
		}

		// AS4TEXT lives in E07T, not E070 — fetch descriptions separately
		textQuery := fmt.Sprintf(
			"SELECT TRKORR, AS4TEXT FROM E07T WHERE TRKORR IN (%s) AND LANGU = 'E'",
			strings.Join(quoted, ","),
		)
		textResult, err := client.RunQuery(ctx, textQuery, len(chunk)+20)
		if err == nil && textResult != nil {
			for _, row := range textResult.Rows {
				trkorr := strings.TrimSpace(fmt.Sprintf("%v", row["TRKORR"]))
				if h, ok := headers[trkorr]; ok {
					h.AS4TEXT = strings.TrimSpace(fmt.Sprintf("%v", row["AS4TEXT"]))
					headers[trkorr] = h
				}
			}
		}
	}

	return headers, nil
}

func aggregateChangelogEntries(headers map[string]graph.TransportHeader, refs []transportRef, since string) []changelogEntry {
	taskToRequest := make(map[string]string)
	for _, h := range headers {
		if h.STRKORR != "" {
			taskToRequest[h.TRKORR] = h.STRKORR
		}
	}

	type agg struct {
		entry changelogEntry
		seen  map[string]bool
	}
	grouped := make(map[string]*agg)

	for _, ref := range refs {
		requestNr := ref.TRKORR
		if parent, ok := taskToRequest[requestNr]; ok && parent != "" {
			requestNr = parent
		}
		header, ok := headers[requestNr]
		if !ok {
			header, ok = headers[ref.TRKORR]
			if ok {
				requestNr = header.TRKORR
			}
		}
		if !ok || requestNr == "" {
			continue
		}
		if since != "" && header.AS4DATE != "" && header.AS4DATE < since {
			continue
		}

		a, exists := grouped[requestNr]
		if !exists {
			a = &agg{
				entry: changelogEntry{
					Transport:   requestNr,
					Date:        header.AS4DATE,
					User:        header.AS4USER,
					Description: header.AS4TEXT,
					Type:        header.TRFUNCTION,
					Status:      header.TRSTATUS,
				},
				seen: make(map[string]bool),
			}
			grouped[requestNr] = a
		}
		key := ref.Object + ":" + ref.ObjName
		if a.seen[key] {
			continue
		}
		a.seen[key] = true
		a.entry.Objects = append(a.entry.Objects, changelogObject{
			Type: ref.Object,
			Name: ref.ObjName,
		})
	}

	entries := make([]changelogEntry, 0, len(grouped))
	for _, a := range grouped {
		sort.Slice(a.entry.Objects, func(i, j int) bool {
			if a.entry.Objects[i].Type != a.entry.Objects[j].Type {
				return a.entry.Objects[i].Type < a.entry.Objects[j].Type
			}
			return a.entry.Objects[i].Name < a.entry.Objects[j].Name
		})
		entries = append(entries, a.entry)
	}
	return entries
}

func printChangelogText(result *changelogResult) {
	headline := fmt.Sprintf("Changelog: %s", result.Package)
	if result.Since != "" {
		headline += fmt.Sprintf(" (since %s)", result.Since)
	}
	fmt.Println(headline)
	fmt.Println()

	if len(result.Entries) == 0 {
		fmt.Println("No transports found.")
		return
	}

	for _, entry := range result.Entries {
		fmt.Printf("%s  %s  %s", entry.Date, entry.Transport, entry.User)
		if entry.Description != "" {
			fmt.Printf("  %q", entry.Description)
		}
		fmt.Println()
		names := make([]string, 0, len(entry.Objects))
		for _, obj := range entry.Objects {
			names = append(names, obj.Type+" "+obj.Name)
		}
		fmt.Printf("  %s\n\n", strings.Join(names, ", "))
	}

	totalObjects := 0
	for _, entry := range result.Entries {
		totalObjects += len(entry.Objects)
	}
	fmt.Printf("%d transports, %d object changes\n", len(result.Entries), totalObjects)
}
