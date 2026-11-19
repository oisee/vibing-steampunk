package vsp

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/oisee/vibing-steampunk/pkg/adt"
	"github.com/oisee/vibing-steampunk/pkg/graph"
)

func printCLIHealth(result *cliHealthResult, details bool) {
	switch result.Scope.Kind {
	case "package":
		fmt.Printf("Health: package %s\n", result.Scope.Package)
	default:
		fmt.Printf("Health: %s %s\n", result.Scope.ObjectType, result.Scope.ObjectName)
	}
	fmt.Printf("Summary: %s — %s\n\n", result.Summary.Status, result.Summary.Headline)
	for _, key := range []string{"tests", "atc", "boundaries", "staleness"} {
		sig, ok := result.Signals[key]
		if !ok {
			continue
		}
		fmt.Printf("%-11s %s", key+":", sig.Status)
		if len(sig.Details) > 0 {
			data, _ := json.Marshal(sig.Details)
			fmt.Printf(" %s", string(data))
		}
		fmt.Println()
		// Beneath the signal it qualifies, because that is where the number it
		// contradicts is. --format json carries the same list as a field.
		if note := adt.UnsearchedNote(sig.Unsearched, len(sig.Unsearched)+searchedCount(sig), "item"); note != "" {
			for _, line := range strings.Split(note, "\n") {
				fmt.Printf("            %s\n", strings.TrimLeft(line, " "))
			}
		}
	}

	if !details {
		return
	}

	// Detailed test results — grouped by parent object
	if result.TestDetails != nil && len(result.TestDetails.Classes) > 0 {
		fmt.Printf("\n--- Test Details ---\n\n")
		groups := groupTestsByParent(result.TestDetails.Classes)
		for _, g := range groups {
			fmt.Printf("  %s\n", g.label)
			for _, class := range g.classes {
				className := class.Name
				if className == "" {
					className = "(anonymous)"
				}
				fmt.Printf("    %s\n", className)
				for _, method := range class.TestMethods {
					status := "PASS"
					if len(method.Alerts) > 0 {
						status = "FAIL"
					}
					fmt.Printf("      %-4s  %s (%.3fs)\n", status, method.Name, method.ExecutionTime)
					for _, alert := range method.Alerts {
						fmt.Printf("            %s: %s\n", alert.Kind, alert.Title)
						for _, d := range alert.Details {
							fmt.Printf("              %s\n", d)
						}
					}
				}
				for _, alert := range class.Alerts {
					fmt.Printf("      ALERT %s: %s\n", alert.Kind, alert.Title)
				}
			}
			fmt.Println()
		}
	}

	// Detailed ATC findings
	if result.ATCDetails != nil && len(result.ATCDetails.Objects) > 0 {
		fmt.Printf("\n--- ATC Findings ---\n\n")
		for _, obj := range result.ATCDetails.Objects {
			if len(obj.Findings) == 0 {
				continue
			}
			fmt.Printf("  %s %s (%d findings)\n", obj.Type, obj.Name, len(obj.Findings))
			for _, f := range obj.Findings {
				prio := "INFO"
				switch f.Priority {
				case 1:
					prio = "ERROR"
				case 2:
					prio = "WARN"
				}
				loc := ""
				if f.Location != "" {
					loc = " @ " + f.Location
				}
				fmt.Printf("    %-5s  %s — %s%s\n", prio, f.CheckTitle, f.MessageTitle, loc)
			}
		}
	}

	// Detailed crossing entries
	if result.CrossingDetails != nil && len(result.CrossingDetails.Entries) > 0 {
		fmt.Printf("\n--- Boundary Crossings ---\n\n")

		// Group by direction, show violations first
		for _, dir := range graph.CrossingDirectionOrder {
			var entries []graph.CrossingEntry
			for _, e := range result.CrossingDetails.Entries {
				if e.Direction == dir {
					entries = append(entries, e)
				}
			}
			if len(entries) == 0 {
				continue
			}
			marker := "OK"
			if dir == graph.CrossSibling || dir == graph.CrossDownward || dir == graph.CrossCommonDown {
				marker = "BAD"
			}
			if dir == graph.CrossExternal {
				marker = "WARN"
			}
			fmt.Printf("  %s  %s (%d)\n", marker, dir, len(entries))
			for _, e := range entries {
				ref := e.EdgeKind
				if e.RefDetail != "" {
					ref += " " + e.RefDetail
				}
				fmt.Printf("    %s → %s  %s %s → %s %s  [%s]\n",
					e.SourcePackage, e.TargetPackage, e.SourceType, e.SourceObject, e.TargetType, e.TargetObject, ref)
			}
		}

		if len(result.CrossingDetails.Circular) > 0 {
			fmt.Printf("\n  CIRCULAR dependencies:\n")
			for _, c := range result.CrossingDetails.Circular {
				fmt.Printf("    %s\n", c)
			}
		}
	}
}

type testGroup struct {
	label   string
	classes []adt.UnitTestClass
}

func groupTestsByParent(classes []adt.UnitTestClass) []testGroup {
	order := []string{}
	groups := map[string]*testGroup{}
	for _, c := range classes {
		key := c.ParentName
		if key == "" {
			key = "(unknown)"
		}
		g, ok := groups[key]
		if !ok {
			label := key
			if c.ParentType != "" {
				label = c.ParentType + " " + key
			}
			g = &testGroup{label: label}
			groups[key] = g
			order = append(order, key)
		}
		g.classes = append(g.classes, c)
	}
	result := make([]testGroup, 0, len(order))
	for _, k := range order {
		result = append(result, *groups[k])
	}
	return result
}

func printCLIHealthMD(result *cliHealthResult) {
	scope := result.Scope.Package
	if scope == "" {
		scope = result.Scope.ObjectType + " " + result.Scope.ObjectName
	}
	fmt.Printf("# Health Report: %s\n\n", scope)
	fmt.Printf("**%s** — %s\n\n", result.Summary.Status, result.Summary.Headline)

	fmt.Print("## Signals\n\n")
	fmt.Println("| Signal | Status | Details |")
	fmt.Println("|--------|--------|---------|")
	for _, key := range []string{"tests", "atc", "boundaries", "staleness"} {
		sig, ok := result.Signals[key]
		if !ok {
			continue
		}
		detailStr := ""
		if len(sig.Details) > 0 {
			parts := make([]string, 0, len(sig.Details))
			for k, v := range sig.Details {
				parts = append(parts, fmt.Sprintf("%s: %v", k, v))
			}
			sort.Strings(parts)
			detailStr = strings.Join(parts, ", ")
		}
		fmt.Printf("| %s | %s | %s |\n", key, sig.Status, detailStr)
	}

	if result.TestDetails != nil && len(result.TestDetails.Classes) > 0 {
		fmt.Print("\n## Test Details\n\n")
		groups := groupTestsByParent(result.TestDetails.Classes)
		for _, g := range groups {
			fmt.Printf("### %s\n\n", g.label)
			for _, class := range g.classes {
				className := class.Name
				if className == "" {
					className = "(anonymous)"
				}
				fmt.Printf("#### %s\n\n", className)
				fmt.Println("| Method | Status | Time | Details |")
				fmt.Println("|--------|--------|------|---------|")
				for _, method := range class.TestMethods {
					status := "PASS"
					detail := ""
					if len(method.Alerts) > 0 {
						status = "FAIL"
						parts := make([]string, 0)
						for _, a := range method.Alerts {
							parts = append(parts, fmt.Sprintf("%s: %s", a.Kind, a.Title))
							parts = append(parts, a.Details...)
						}
						detail = strings.Join(parts, "; ")
					}
					fmt.Printf("| %s | %s | %.3fs | %s |\n", method.Name, status, method.ExecutionTime, detail)
				}
				fmt.Println()
			}
		}
	}

	if result.ATCDetails != nil && len(result.ATCDetails.Objects) > 0 {
		fmt.Print("\n## ATC Findings\n\n")
		for _, obj := range result.ATCDetails.Objects {
			if len(obj.Findings) == 0 {
				continue
			}
			fmt.Printf("### %s %s\n\n", obj.Type, obj.Name)
			fmt.Println("| Priority | Check | Message | Location |")
			fmt.Println("|----------|-------|---------|----------|")
			for _, f := range obj.Findings {
				prio := "Info"
				switch f.Priority {
				case 1:
					prio = "Error"
				case 2:
					prio = "Warning"
				}
				fmt.Printf("| %s | %s | %s | %s |\n", prio, f.CheckTitle, f.MessageTitle, f.Location)
			}
			fmt.Println()
		}
	}

	printCrossingsMD(result.CrossingDetails)
}

func printCLIHealthHTML(result *cliHealthResult, details bool) {
	scope := result.Scope.Package
	if scope == "" {
		scope = result.Scope.ObjectType + " " + result.Scope.ObjectName
	}

	fmt.Println(`<!DOCTYPE html>
<html><head><meta charset="UTF-8">
<title>Health Report</title>
<style>
  body { font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', sans-serif; max-width: 960px; margin: 2em auto; padding: 0 1em; color: #333; }
  h1 { border-bottom: 2px solid #ddd; padding-bottom: 0.3em; }
  h2 { margin-top: 1.5em; color: #555; }
  h3 { color: #666; }
  table { border-collapse: collapse; width: 100%; margin: 1em 0; }
  th, td { border: 1px solid #ddd; padding: 8px; text-align: left; }
  th { background: #f5f5f5; }
  .PASS, .CLEAN, .GOOD { color: #2e7d32; }
  .FAIL, .BAD, .ERROR { color: #c62828; }
  .WARN, .FINDINGS { color: #ef6c00; }
  .NONE, .UNKNOWN, .SKIPPED { color: #757575; }
  .prio-error { color: #c62828; font-weight: bold; }
  .prio-warn { color: #ef6c00; }
  .prio-info { color: #1565c0; }
  nav.toc { background: #f9f9f9; border: 1px solid #ddd; border-radius: 6px; padding: 0.8em 1.2em; margin-bottom: 1.5em; }
  nav.toc summary { font-weight: bold; cursor: pointer; }
  nav.toc ul { margin: 0.5em 0 0; padding-left: 1.5em; }
  nav.toc li { margin: 0.2em 0; }
  nav.toc a { text-decoration: none; color: #1565c0; }
  nav.toc a:hover { text-decoration: underline; }
</style>
</head><body>`)

	fmt.Printf("<h1>Health Report: %s</h1>\n", scope)
	fmt.Printf("<p><strong class=%q>%s</strong> — %s</p>\n", result.Summary.Status, result.Summary.Status, result.Summary.Headline)

	// Table of contents
	fmt.Println(`<nav class="toc"><details open><summary>Contents</summary><ul>`)
	fmt.Println(`<li><a href="#signals">Signals</a></li>`)
	if result.TestDetails != nil && len(result.TestDetails.Classes) > 0 {
		fmt.Println(`<li><a href="#tests">Test Details</a></li>`)
	}
	if result.ATCDetails != nil && len(result.ATCDetails.Objects) > 0 {
		fmt.Println(`<li><a href="#atc">ATC Findings</a></li>`)
	}
	if result.CrossingDetails != nil && len(result.CrossingDetails.Entries) > 0 {
		fmt.Println(`<li><a href="#boundaries">Boundary Crossings</a></li>`)
	}
	fmt.Println(`</ul></details></nav>`)

	fmt.Println(`<h2 id="signals">Signals</h2>`)
	fmt.Println("<table><tr><th>Signal</th><th>Status</th><th>Details</th></tr>")
	for _, key := range []string{"tests", "atc", "boundaries", "staleness"} {
		sig, ok := result.Signals[key]
		if !ok {
			continue
		}
		detailStr := ""
		if len(sig.Details) > 0 {
			parts := make([]string, 0, len(sig.Details))
			for k, v := range sig.Details {
				parts = append(parts, fmt.Sprintf("%s: %v", k, v))
			}
			sort.Strings(parts)
			detailStr = strings.Join(parts, ", ")
		}
		fmt.Printf("<tr><td>%s</td><td class=%q>%s</td><td>%s</td></tr>\n", key, sig.Status, sig.Status, detailStr)
	}
	fmt.Println("</table>")

	if result.TestDetails != nil && len(result.TestDetails.Classes) > 0 {
		if details {
			fmt.Println(`<h2 id="tests">Test Details</h2>`)
		} else {
			fmt.Println(`<h2 id="tests">Failing Tests</h2>`)
		}
		groups := groupTestsByParent(result.TestDetails.Classes)
		for _, g := range groups {
			// Without --details, skip groups that have no failures
			if !details {
				hasFailures := false
				for _, class := range g.classes {
					for _, method := range class.TestMethods {
						if len(method.Alerts) > 0 {
							hasFailures = true
							break
						}
					}
					if hasFailures {
						break
					}
				}
				if !hasFailures {
					continue
				}
			}
			fmt.Printf("<h3>%s</h3>\n", g.label)
			for _, class := range g.classes {
				// Without --details, skip classes with no failures
				if !details {
					hasClassFailures := false
					for _, method := range class.TestMethods {
						if len(method.Alerts) > 0 {
							hasClassFailures = true
							break
						}
					}
					if !hasClassFailures {
						continue
					}
				}
				className := class.Name
				if className == "" {
					className = "(anonymous)"
				}
				fmt.Printf("<h4>%s</h4>\n", className)
				fmt.Println("<table><tr><th>Method</th><th>Status</th><th>Time</th><th>Details</th></tr>")
				for _, method := range class.TestMethods {
					status := "PASS"
					detail := ""
					if len(method.Alerts) > 0 {
						status = "FAIL"
						parts := make([]string, 0)
						for _, a := range method.Alerts {
							parts = append(parts, fmt.Sprintf("<strong>%s:</strong> %s", a.Kind, a.Title))
							for _, d := range a.Details {
								parts = append(parts, d)
							}
						}
						detail = strings.Join(parts, "<br>")
					}
					// Without --details, only show failing methods
					if !details && status == "PASS" {
						continue
					}
					fmt.Printf("<tr><td>%s</td><td class=%q>%s</td><td>%.3fs</td><td>%s</td></tr>\n", method.Name, status, status, method.ExecutionTime, detail)
				}
				fmt.Println("</table>")
			}
		}
	}

	if result.ATCDetails != nil && len(result.ATCDetails.Objects) > 0 {
		fmt.Println(`<h2 id="atc">ATC Findings</h2>`)
		for _, obj := range result.ATCDetails.Objects {
			if len(obj.Findings) == 0 {
				continue
			}
			fmt.Printf("<h3>%s %s (%d findings)</h3>\n", obj.Type, obj.Name, len(obj.Findings))
			fmt.Println("<table><tr><th>Priority</th><th>Check</th><th>Message</th><th>Location</th></tr>")
			for _, f := range obj.Findings {
				prio := "Info"
				prioClass := "prio-info"
				switch f.Priority {
				case 1:
					prio = "Error"
					prioClass = "prio-error"
				case 2:
					prio = "Warning"
					prioClass = "prio-warn"
				}
				fmt.Printf("<tr><td class=%q>%s</td><td>%s</td><td>%s</td><td>%s</td></tr>\n", prioClass, prio, f.CheckTitle, f.MessageTitle, f.Location)
			}
			fmt.Println("</table>")
		}
	}

	// Crossing details
	if result.CrossingDetails != nil && len(result.CrossingDetails.Entries) > 0 {
		fmt.Println(`<h2 id="boundaries">Boundary Crossings</h2>`)
		for _, dir := range graph.CrossingDirectionOrder {
			var entries []graph.CrossingEntry
			for _, e := range result.CrossingDetails.Entries {
				if e.Direction == dir {
					entries = append(entries, e)
				}
			}
			if len(entries) == 0 {
				continue
			}
			cssClass := "PASS"
			if dir == graph.CrossSibling || dir == graph.CrossDownward || dir == graph.CrossCommonDown {
				cssClass = "FAIL"
			}
			if dir == graph.CrossExternal {
				cssClass = "WARN"
			}
			fmt.Printf("<h3 class=%q>%s (%d)</h3>\n", cssClass, dir, len(entries))
			fmt.Println("<table><tr><th>From Pkg</th><th>Source</th><th>To Pkg</th><th>Target</th><th>Edge</th><th>Detail</th></tr>")
			for _, e := range entries {
				fmt.Printf("<tr><td>%s</td><td>%s %s</td><td>%s</td><td>%s %s</td><td>%s</td><td>%s</td></tr>\n",
					e.SourcePackage, e.SourceType, e.SourceObject,
					e.TargetPackage, e.TargetType, e.TargetObject,
					e.EdgeKind, e.RefDetail)
			}
			fmt.Println("</table>")
		}
		if len(result.CrossingDetails.Circular) > 0 {
			fmt.Println("<h3 class=\"FAIL\">Circular Dependencies</h3><ul>")
			for _, c := range result.CrossingDetails.Circular {
				fmt.Printf("<li>%s</li>\n", c)
			}
			fmt.Println("</ul>")
		}
	}

	fmt.Println("</body></html>")
}
