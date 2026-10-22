package vsp

import (
	"context"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/oisee/vibing-steampunk/pkg/adt"
	"github.com/spf13/cobra"
)

// --- test command ---

var testCmd = &cobra.Command{
	Use:   "test [type] [name]",
	Short: "Run ABAP Unit tests",
	Long: `Run ABAP Unit tests for an object or package.

Examples:
  vsp test CLAS ZCL_MY_CLASS
  vsp test PROG ZTEST_PROGRAM
  vsp test --package '$TMP'
  vsp test --package '$ZADT'
  vsp test CLAS ZCL_MY_CLASS --only-failures   # failed methods + counts only
  vsp test CLAS ZCL_MY_CLASS --json            # the MCP test tool's JSON

A method fails on a failed assertion or an exception (or any critical/fatal
alert); a warning, such as a class not run for its risk level, does not fail
it. Alerts filed on the class itself (CLASS_SETUP, CLASS_TEARDOWN) are listed
under the class. The exit code is non-zero when anything failed, or when test
classes were found but no test method ran, or when ABAP Unit did not run some
test class (for example one refused for its risk level) even though others ran.`,
	// A failing test is an answer, not a mistake in the command line, and a
	// usage screen after the failures only buries them.
	SilenceUsage: true,
	RunE:         runTest,
}

func runTest(cmd *cobra.Command, args []string) error {
	params, err := resolveSystemParams(cmd)
	if err != nil {
		return err
	}

	client, err := getClient(params)
	if err != nil {
		return err
	}

	packageName, _ := cmd.Flags().GetString("package")

	var objectURL string
	if packageName != "" {
		// Package-level test
		objectURL = fmt.Sprintf("/sap/bc/adt/packages/%s", strings.ToUpper(packageName))
	} else {
		// Object-level test
		if len(args) != 2 {
			return fmt.Errorf("usage: vsp test <type> <name> or vsp test --package <package>")
		}
		objType := strings.ToUpper(args[0])
		name := strings.ToUpper(args[1])
		objectURL = buildObjectURL(objType, name)
		if objectURL == "" {
			return fmt.Errorf("unsupported object type: %s (supported: CLAS, PROG, INCL, INTF)", objType)
		}
	}

	ctx := context.Background()
	result, err := client.RunUnitTests(ctx, objectURL, nil)
	if err != nil {
		return fmt.Errorf("test run failed: %w", err)
	}

	onlyFailures, _ := cmd.Flags().GetBool("only-failures")
	asJSON, _ := cmd.Flags().GetBool("json")
	return printUnitTestReport(os.Stdout, result, onlyFailures, asJSON)
}

// printUnitTestReport prints a test run and returns the error that sets the
// exit code. The text form lists every method with PASS/FAIL and its alerts
// (only the failed ones with onlyFailures); --json prints the MCP tool's
// object. Both end on the same counts.
func printUnitTestReport(w io.Writer, result *adt.UnitTestResult, onlyFailures, asJSON bool) error {
	if result == nil {
		result = &adt.UnitTestResult{}
	}
	report := adt.NewUnitTestReport(result, onlyFailures)
	counts := report.Counts

	if asJSON {
		out, err := adt.IndentJSON(report)
		if err != nil {
			return fmt.Errorf("could not encode the result: %w", err)
		}
		fmt.Fprintln(w, string(out))
	} else {
		if counts.Classes == 0 {
			fmt.Fprintln(w, "No test classes found.")
		}
		for _, class := range result.Classes {
			var lines []string
			for _, alert := range class.Alerts {
				lines = append(lines, fmt.Sprintf("  %s (%s): %s", alert.Kind, alert.Severity, alert.Title))
				for _, detail := range alert.Details {
					lines = append(lines, "           "+detail)
				}
			}
			for _, method := range class.TestMethods {
				failed := adt.UnitTestMethodFailed(method)
				if onlyFailures && !failed {
					continue
				}
				status := "PASS"
				if failed {
					status = "FAIL"
				}
				lines = append(lines, fmt.Sprintf("  %s  %s (%.3fs)", status, method.Name, method.ExecutionTime))
				for _, alert := range method.Alerts {
					lines = append(lines, fmt.Sprintf("         %s: %s", alert.Kind, alert.Title))
					for _, detail := range alert.Details {
						lines = append(lines, "           "+detail)
					}
				}
			}
			if onlyFailures && len(lines) == 0 {
				continue
			}
			header := "Test Class: " + class.Name
			if class.ParentName != "" {
				header += " (" + class.ParentName + ")"
			}
			fmt.Fprintln(w, header)
			for _, line := range lines {
				fmt.Fprintln(w, line)
			}
		}
		if counts.Classes > 0 {
			summary := fmt.Sprintf("\nTotal: %d passed, %d failed", counts.Passed, counts.Failed)
			if counts.ClassFailures > 0 {
				summary += fmt.Sprintf(", %d class-level failure(s)", counts.ClassFailures)
			}
			if counts.Warnings > 0 {
				summary += fmt.Sprintf(", %d warning(s)", counts.Warnings)
			}
			if counts.NotRun > 0 {
				summary += fmt.Sprintf(", %d class(es) not run", counts.NotRun)
			}
			fmt.Fprintln(w, summary)
		}
		if report.Note != "" {
			fmt.Fprintln(w, report.Note)
		}
	}

	// The exit code follows report.OK in both forms: a run that is not ok,
	// including one with no test class or a class ABAP Unit did not run,
	// exits non-zero.
	switch {
	case report.OK:
		return nil
	case counts.Failed > 0 || counts.ClassFailures > 0:
		return fmt.Errorf("%d test(s) failed", counts.Failed+counts.ClassFailures)
	case counts.Classes == 0:
		return fmt.Errorf("no test class found, nothing ran")
	case counts.Methods == 0:
		return fmt.Errorf("no test method ran")
	case counts.NotRun > 0:
		return fmt.Errorf("%d test class(es) not run: %s", counts.NotRun, strings.Join(report.NotRunClasses, ", "))
	}
	return fmt.Errorf("test run is not ok")
}

func init() {
	// Test flags
	testCmd.Flags().String("package", "", "Run tests for entire package")
	testCmd.Flags().Bool("only-failures", false, "Show only failed test methods (and class-level alerts), plus the counts")
	testCmd.Flags().Bool("json", false, "Print the result as JSON, the same object the MCP test tool answers")

	rootCmd.AddCommand(testCmd)
}
