package main

import (
	"context"
	"fmt"
	"os"
	"sort"
	"strings"

	"github.com/oisee/vibing-steampunk/pkg/adt"
	"github.com/spf13/cobra"
)

// --- deps command ---

var depsCmd = &cobra.Command{
	Use:   "deps <package>",
	Short: "Analyze package dependencies and transport readiness",
	Long: `Analyze dependencies of all objects in a package.
Shows internal vs external references, DDIC dependencies,
and transport readiness (can this package move autonomously?).

Uses TADIR, WBCROSSGT, CROSS, DD02L, DD03L tables via standard ADT.

Examples:
  vsp deps '$ZADT_VSP'
  vsp deps '$ZADT_VSP' --include-subpackages
  vsp deps '$TMP' --object ZCL_MY_CLASS
  vsp deps '$ZFINANCE' --format summary`,
	Args: cobra.ExactArgs(1),
	RunE: runDeps,
}

func init() {
	depsCmd.Flags().Bool("include-subpackages", false, "Include subpackages")
	depsCmd.Flags().String("object", "", "Analyze single object only")
	depsCmd.Flags().String("format", "tree", "Output: tree, summary, or json")
	rootCmd.AddCommand(depsCmd)
}

type depInfo struct {
	Name     string
	Type     string
	Package  string
	Internal []string // refs within same package
	External []string // refs to other packages
	DDIC     []string // DDIC object refs (tables, data elements)
	SAP      []string // refs to SAP standard
}

func runDeps(cmd *cobra.Command, args []string) error {
	params, err := resolveSystemParams(cmd)
	if err != nil {
		return err
	}

	client, err := getClient(params)
	if err != nil {
		return err
	}

	pkg := strings.ToUpper(args[0])
	singleObj, _ := cmd.Flags().GetString("object")
	format, _ := cmd.Flags().GetString("format")
	inclSub, _ := cmd.Flags().GetBool("include-subpackages")

	ctx := context.Background()

	// 1. Get all objects in package
	fmt.Fprintf(os.Stderr, "Loading package %s...\n", pkg)
	packWhere := fmt.Sprintf("DEVCLASS = '%s'", pkg)
	if inclSub {
		packWhere = fmt.Sprintf("DEVCLASS LIKE '%s%%'", pkg)
	}
	tadirResult, err := client.RunQuery(ctx,
		fmt.Sprintf("SELECT OBJECT, OBJ_NAME, DEVCLASS FROM TADIR WHERE %s", packWhere), 500)
	if err != nil {
		return fmt.Errorf("failed to query TADIR: %w", err)
	}

	if tadirResult == nil || len(tadirResult.Rows) == 0 {
		return fmt.Errorf("package %s is empty or not found", pkg)
	}

	// Build object set
	type pkgObj struct {
		objType string
		name    string
		pkg     string
	}
	var objects []pkgObj
	objSet := map[string]string{} // "NAME" → package

	for _, row := range tadirResult.Rows {
		ot := strings.TrimSpace(fmt.Sprintf("%v", row["OBJECT"]))
		nm := strings.TrimSpace(fmt.Sprintf("%v", row["OBJ_NAME"]))
		dv := strings.TrimSpace(fmt.Sprintf("%v", row["DEVCLASS"]))
		objSet[nm] = dv
		if singleObj == "" || strings.EqualFold(nm, singleObj) {
			objects = append(objects, pkgObj{ot, nm, dv})
		}
	}

	fmt.Fprintf(os.Stderr, "Found %d objects in %s\n", len(objects), pkg)

	// 2. For each object, get WBCROSSGT references
	var deps []depInfo

	for _, obj := range objects {
		if obj.objType != "CLAS" && obj.objType != "PROG" && obj.objType != "INTF" && obj.objType != "FUGR" {
			continue // skip non-code objects for cross-ref
		}

		di := depInfo{Name: obj.name, Type: obj.objType, Package: obj.pkg}

		// Query WBCROSSGT for this object's references
		refs := queryObjectRefs(ctx, client, obj.name, obj.objType)

		for _, ref := range refs {
			refName := ref.name
			refType := ref.otype

			// Classify: internal, external, DDIC, SAP standard
			if _, isInternal := objSet[refName]; isInternal {
				di.Internal = append(di.Internal, refType+" "+refName)
			} else if isDDIC(refType) {
				di.DDIC = append(di.DDIC, refName)
			} else if isSAPStandard(refName) {
				di.SAP = append(di.SAP, refType+" "+refName)
			} else {
				// External custom dependency
				di.External = append(di.External, refType+" "+refName)
			}
		}

		dedup(&di.Internal)
		dedup(&di.External)
		dedup(&di.DDIC)
		dedup(&di.SAP)
		deps = append(deps, di)
	}

	// 3. Output
	switch format {
	case "summary":
		printDepsSummary(deps, pkg)
	default:
		printDepsTree(deps, pkg)
	}

	return nil
}

type crossRef struct {
	name  string
	otype string
}

func queryObjectRefs(ctx context.Context, client *adt.Client, name, objType string) []crossRef {
	var refs []crossRef

	// WBCROSSGT
	sql := fmt.Sprintf("SELECT OTYPE, NAME FROM WBCROSSGT WHERE INCLUDE LIKE '%s%%'", name)
	result, err := client.RunQuery(ctx, sql, 500)
	if err == nil && result != nil {
		for _, row := range result.Rows {
			ot := strings.TrimSpace(fmt.Sprintf("%v", row["OTYPE"]))
			nm := strings.TrimSpace(fmt.Sprintf("%v", row["NAME"]))
			// Skip self-references and component refs
			if strings.Contains(nm, "\\") || nm == name {
				continue
			}
			refs = append(refs, crossRef{nm, ot})
		}
	}

	// CROSS (for procedural refs)
	var crossIncl string
	switch objType {
	case "PROG":
		crossIncl = name
	case "FUGR":
		crossIncl = "L" + name + "%"
	default:
		crossIncl = name + "%"
	}
	sql2 := fmt.Sprintf("SELECT TYPE, NAME FROM CROSS WHERE INCLUDE LIKE '%s'", crossIncl)
	result2, err := client.RunQuery(ctx, sql2, 500)
	if err == nil && result2 != nil {
		for _, row := range result2.Rows {
			ot := strings.TrimSpace(fmt.Sprintf("%v", row["TYPE"]))
			nm := strings.TrimSpace(fmt.Sprintf("%v", row["NAME"]))
			if nm == name || nm == "" {
				continue
			}
			refs = append(refs, crossRef{nm, ot})
		}
	}

	return refs
}

func isDDIC(otype string) bool {
	switch otype {
	case "DA", "TY": // data, type — could be DDIC
		return false // need further resolution
	}
	return false
}

func isSAPStandard(name string) bool {
	// Z/Y prefix = custom, everything else is SAP standard
	if strings.HasPrefix(name, "Z") || strings.HasPrefix(name, "Y") ||
		strings.HasPrefix(name, "/Z") || strings.HasPrefix(name, "/Y") {
		return false
	}
	return true
}

func dedup(s *[]string) {
	if s == nil || len(*s) == 0 {
		return
	}
	sort.Strings(*s)
	j := 0
	for i := 1; i < len(*s); i++ {
		if (*s)[i] != (*s)[j] {
			j++
			(*s)[j] = (*s)[i]
		}
	}
	*s = (*s)[:j+1]
}

func printDepsTree(deps []depInfo, pkg string) {
	totalInternal := 0
	totalExternal := 0
	totalSAP := 0
	totalDDIC := 0

	for _, d := range deps {
		totalInternal += len(d.Internal)
		totalExternal += len(d.External)
		totalSAP += len(d.SAP)
		totalDDIC += len(d.DDIC)

		fmt.Printf("%s %s\n", d.Type, d.Name)
		if len(d.Internal) > 0 {
			fmt.Printf("  Internal (%d):\n", len(d.Internal))
			for _, r := range d.Internal {
				fmt.Printf("    %s\n", r)
			}
		}
		if len(d.External) > 0 {
			fmt.Printf("  External custom (%d):\n", len(d.External))
			for _, r := range d.External {
				fmt.Printf("    ⚠ %s\n", r)
			}
		}
		if len(d.SAP) > 0 {
			fmt.Printf("  SAP standard (%d):\n", len(d.SAP))
			for _, r := range d.SAP[:min(5, len(d.SAP))] {
				fmt.Printf("    %s\n", r)
			}
			if len(d.SAP) > 5 {
				fmt.Printf("    ... +%d more\n", len(d.SAP)-5)
			}
		}
		fmt.Println()
	}

	fmt.Fprintf(os.Stderr, "\n--- Package %s ---\n", pkg)
	fmt.Fprintf(os.Stderr, "Objects: %d\n", len(deps))
	fmt.Fprintf(os.Stderr, "Internal refs: %d (within package)\n", totalInternal)
	fmt.Fprintf(os.Stderr, "External custom: %d (need transport)\n", totalExternal)
	fmt.Fprintf(os.Stderr, "SAP standard: %d (always available)\n", totalSAP)
}

func printDepsSummary(deps []depInfo, pkg string) {
	// Aggregate external dependencies
	extDeps := map[string]int{} // external object → count of refs
	for _, d := range deps {
		for _, ext := range d.External {
			extDeps[ext]++
		}
	}

	fmt.Printf("Package: %s (%d code objects)\n\n", pkg, len(deps))

	if len(extDeps) == 0 {
		fmt.Println("✓ Self-contained — no external custom dependencies")
		fmt.Println("  This package can be transported independently.")
	} else {
		fmt.Printf("⚠ External custom dependencies (%d):\n", len(extDeps))

		// Sort by count
		type extEntry struct {
			name  string
			count int
		}
		var sorted []extEntry
		for k, v := range extDeps {
			sorted = append(sorted, extEntry{k, v})
		}
		sort.Slice(sorted, func(i, j int) bool { return sorted[i].count > sorted[j].count })

		for _, e := range sorted {
			fmt.Printf("  %s (referenced by %d objects)\n", e.name, e.count)
		}
		fmt.Println("\n  These dependencies must be transported BEFORE this package.")
	}

	// SAP standard summary
	sapDeps := map[string]bool{}
	for _, d := range deps {
		for _, s := range d.SAP {
			sapDeps[s] = true
		}
	}
	fmt.Fprintf(os.Stderr, "\nSAP standard refs: %d unique (always available on target)\n", len(sapDeps))
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
