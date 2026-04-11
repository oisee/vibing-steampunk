// Package main provides shared helpers for package acquisition across CLI commands.
// Used by: slim, api-surface, deps, health.
package main

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/oisee/vibing-steampunk/pkg/adt"
	"github.com/oisee/vibing-steampunk/pkg/graph"
)

// PackageObject represents a repository object from TADIR.
type PackageObject struct {
	Name    string
	Type    string
	Package string
}

// AcquirePackageScope resolves the set of packages for analysis using TDEVC hierarchy.
// Falls back to LIKE prefix if TDEVC is unavailable or root is not in TDEVC.
// If exact is true, returns only the given package.
func AcquirePackageScope(ctx context.Context, client *adt.Client, pkg string, inclSub bool) (*graph.PackageScope, error) {
	if !inclSub {
		return graph.ResolvePackageScope(pkg, true, nil), nil
	}

	tdevcResult, err := client.RunQuery(ctx,
		fmt.Sprintf("SELECT DEVCLASS, PARENTCL FROM TDEVC WHERE DEVCLASS LIKE '%s%%'", pkg), 500)
	if err == nil && tdevcResult != nil && len(tdevcResult.Rows) > 0 {
		var rows []graph.TDEVCRow
		for _, row := range tdevcResult.Rows {
			rows = append(rows, graph.TDEVCRow{
				DevClass: strings.TrimSpace(fmt.Sprintf("%v", row["DEVCLASS"])),
				ParentCL: strings.TrimSpace(fmt.Sprintf("%v", row["PARENTCL"])),
			})
		}
		scope := graph.ResolvePackageScope(pkg, false, rows)
		return scope, nil
	}

	// Fallback: no TDEVC or query failed
	return graph.ResolvePackageScope(pkg, false, nil), nil
}

// ScopeToWhere converts a PackageScope into a SQL WHERE clause for TADIR.
func ScopeToWhere(scope *graph.PackageScope) string {
	if len(scope.Packages) == 0 {
		return fmt.Sprintf("DEVCLASS = '%s'", scope.RootPackage)
	}
	if len(scope.Packages) == 1 {
		return fmt.Sprintf("DEVCLASS = '%s'", scope.Packages[0])
	}
	quoted := make([]string, len(scope.Packages))
	for i, p := range scope.Packages {
		quoted[i] = fmt.Sprintf("'%s'", p)
	}
	return fmt.Sprintf("DEVCLASS IN (%s)", strings.Join(quoted, ","))
}

// AcquirePackageObjects fetches objects from TADIR for the given scope WHERE clause.
func AcquirePackageObjects(ctx context.Context, client *adt.Client, packWhere string) ([]PackageObject, error) {
	result, err := client.RunQuery(ctx,
		fmt.Sprintf("SELECT OBJECT, OBJ_NAME, DEVCLASS FROM TADIR WHERE %s AND PGMID = 'R3TR'", packWhere), 1000)
	if err != nil {
		return nil, fmt.Errorf("TADIR query failed: %w", err)
	}
	if result == nil || len(result.Rows) == 0 {
		return nil, nil
	}

	var objects []PackageObject
	for _, row := range result.Rows {
		nm := strings.TrimSpace(fmt.Sprintf("%v", row["OBJ_NAME"]))
		ot := strings.TrimSpace(fmt.Sprintf("%v", row["OBJECT"]))
		dv := strings.TrimSpace(fmt.Sprintf("%v", row["DEVCLASS"]))
		if nm == "" {
			continue
		}
		objects = append(objects, PackageObject{
			Name:    strings.ToUpper(nm),
			Type:    strings.ToUpper(ot),
			Package: strings.ToUpper(dv),
		})
	}
	return objects, nil
}

// AcquireReverseRefs fetches reverse cross-references for a list of object names.
// Returns SlimRefRow format (CallerInclude, TargetName, Source).
// Queries WBCROSSGT and CROSS per object (ADT freestyle doesn't support OR+LIKE).
func AcquireReverseRefs(ctx context.Context, client *adt.Client, names []string, progress bool) []graph.SlimRefRow {
	var allRefs []graph.SlimRefRow
	for idx, nm := range names {
		if progress {
			fmt.Fprintf(os.Stderr, "\r  [%d/%d] %-40s", idx+1, len(names), nm)
		}

		wbQuery := fmt.Sprintf("SELECT INCLUDE, NAME FROM WBCROSSGT WHERE NAME LIKE '%s%%'", nm)
		wbResult, err := client.RunQuery(ctx, wbQuery, 500)
		if err == nil && wbResult != nil {
			for _, row := range wbResult.Rows {
				allRefs = append(allRefs, graph.SlimRefRow{
					CallerInclude: strings.TrimSpace(fmt.Sprintf("%v", row["INCLUDE"])),
					TargetName:    strings.TrimSpace(fmt.Sprintf("%v", row["NAME"])),
					Source:        "WBCROSSGT",
				})
			}
		}

		crossQuery := fmt.Sprintf("SELECT INCLUDE, NAME FROM CROSS WHERE NAME LIKE '%s%%'", nm)
		crossResult, err := client.RunQuery(ctx, crossQuery, 500)
		if err == nil && crossResult != nil {
			for _, row := range crossResult.Rows {
				allRefs = append(allRefs, graph.SlimRefRow{
					CallerInclude: strings.TrimSpace(fmt.Sprintf("%v", row["INCLUDE"])),
					TargetName:    strings.TrimSpace(fmt.Sprintf("%v", row["NAME"])),
					Source:        "CROSS",
				})
			}
		}
	}
	if progress && len(names) > 0 {
		fmt.Fprintf(os.Stderr, "\r")
	}
	return allRefs
}

// IsSourceBearing returns true for object types that contain ABAP source code.
func IsSourceBearing(objType string) bool {
	switch strings.ToUpper(objType) {
	case "CLAS", "PROG", "INTF", "FUGR":
		return true
	default:
		return false
	}
}
