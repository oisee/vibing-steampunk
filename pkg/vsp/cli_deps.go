package vsp

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
Shows internal vs external references,
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
	// An object whose references could not be read contributes none, and none
	// is what an object with no dependencies looks like. This report's whole
	// subject is what depends on what.
	var unreadable []adt.Unsearched

	for _, obj := range objects {
		if obj.objType != "CLAS" && obj.objType != "PROG" && obj.objType != "INTF" && obj.objType != "FUGR" {
			continue // skip non-code objects for cross-ref
		}

		di := depInfo{Name: obj.name, Type: obj.objType, Package: obj.pkg}

		// Query WBCROSSGT for this object's references
		refs, gaps := queryObjectRefs(ctx, client, obj.name, obj.objType)
		unreadable = append(unreadable, gaps...)

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
	if note := adt.UnsearchedNote(unreadable, len(objects), "object"); note != "" {
		fmt.Fprintf(os.Stderr, "%s\nDependencies of the objects above are absent from what follows.\n\n", note)
	}

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

// queryObjectRefs reads what one object references.
//
// It used to run its own pair of queries, and carried every defect that was
// fixed in the shared reader over the past week: both queries failed silently,
// so a blocked table read was indistinguishable from an object that references
// nothing; the include filter was a bare prefix, so ZCL_ORDER collected
// ZCL_ORDER_ITEM's references as its own; INDIRECT rows were not excluded, so
// types implied by types arrived as dependencies; and a name too long for
// WBCROSSGT's NAME column arrived as the SHA-1 it is stored under and would
// have been printed as the name of a dependency.
//
// Callees has all four guards and one place to keep them. The gaps it reports
// are returned rather than dropped, because "this object references nothing"
// and "the tables could not be read" are the two answers this command exists to
// tell apart.
func queryObjectRefs(ctx context.Context, client *adt.Client, name, objType string) ([]crossRef, []adt.Unsearched) {
	uri := adt.GetObjectURL(objectTypeForDeps(objType), name, "")
	if uri == "" {
		return nil, []adt.Unsearched{{Object: objType + " " + name,
			Reason: "no ADT path is known for this object type, so its references cannot be read"}}
	}

	callees, gaps, err := client.Callees(ctx, uri)
	if err != nil {
		return nil, append(gaps, adt.Unsearched{Object: objType + " " + name, Reason: err.Error()})
	}

	refs := make([]crossRef, 0, len(callees))
	for _, c := range callees {
		refs = append(refs, crossRef{c.Name, crossTypeCodeOf(c)})
	}
	return refs, gaps
}

// objectTypeForDeps maps the package listing's short kind to the creatable type
// the URL builder speaks.
func objectTypeForDeps(objType string) adt.CreatableObjectType {
	switch strings.ToUpper(objType) {
	case "CLAS":
		return adt.ObjectTypeClass
	case "INTF":
		return adt.ObjectTypeInterface
	case "PROG":
		return adt.ObjectTypeProgram
	case "FUGR":
		return adt.ObjectTypeFunctionGroup
	}
	return ""
}

// crossTypeCodeOf turns the decoded kind back into the one-or-two letter code
// this command's classifier reads.
//
// Going back to the raw code is deliberate rather than lazy: isDDIC below asks
// whether a reference is a type or a data object, and those are exactly the
// codes WBCROSSGT uses. Rewriting the classifier to speak the decoded kinds
// would change what "DDIC" means in the report as a side effect of a refactor,
// which is the kind of quiet change this week has been about not making.
func crossTypeCodeOf(c adt.Callee) string {
	switch c.Kind {
	case "type":
		return "TY"
	case "data":
		return "DA"
	case "method":
		return "ME"
	case "function module":
		return "F"
	}
	return strings.ToUpper(c.Kind)
}

// isDDIC is not implemented, and returns false on every path.
//
// Left as it is rather than guessed at: telling a DDIC type from an ordinary
// one needs a TADIR lookup, and the codes available here — DA for a data
// object, TY for a type — say nothing about where the type lives. Reporting
// them as DDIC would be inventing a classification out of a code that does not
// carry it.
//
// The DDIC field it feeds is never printed either, so nothing in the output
// claims a classification that is not happening. The command's own description
// did claim it, and no longer does.
func isDDIC(otype string) bool {
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

// queryObjectRefsOnly is queryObjectRefs for a caller that cannot yet carry the
// gaps. It exists so the dropping is visible at the call site and greppable,
// rather than hidden inside a function that silently returns less than it knows.
func queryObjectRefsOnly(ctx context.Context, client *adt.Client, name, objType string) []crossRef {
	refs, _ := queryObjectRefs(ctx, client, name, objType)
	return refs
}
