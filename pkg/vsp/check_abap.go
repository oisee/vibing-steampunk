package vsp

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/signal"
	"syscall"

	"github.com/spf13/cobra"

	"github.com/oisee/vibing-steampunk/pkg/adt"
)

var checkABAPCmd = &cobra.Command{
	Use:   "check-abap [code]",
	Short: "Type-check an ABAP snippet on SAP without running it",
	Long: `Compile-only check of an ad-hoc ABAP snippet: SAP's syntax check, which
type-checks, where a local parse does not.

The snippet is wrapped exactly as 'vsp execute' wraps it, so what passes here
is what execute would compile, and positions are reported as lines of the
snippet. Nothing runs and nothing is activated.

It does write: the check needs a real program (a program that does not exist
is checked with fixed-point arithmetic off, which breaks every ABAP SQL
statement with a host variable), so a temporary ZVSP_CHK_* program is created
in $TMP and deleted afterwards, whatever the check's outcome. For that reason
it is refused under --read-only.

Exit status: 0 when SAP found no error (warnings allowed) and the temporary
program was deleted, 1 otherwise. Ctrl-C once the program exists still
deletes it; Ctrl-C while SAP is still creating it leaves the outcome unknown,
so the program is not deleted but named in the error, to remove by hand
(vsp recover-failed-create PROG <name> --package '$TMP').

Examples:
  vsp check-abap "DATA ls TYPE t000. DATA(s) = |{ ls }|."
  vsp check-abap --file snippet.abap
  vsp check-abap --stdin --json < snippet.abap`,
	Args:          cobra.MaximumNArgs(1),
	SilenceUsage:  true,
	SilenceErrors: true,
	RunE:          runCheckABAP,
}

func init() {
	checkABAPCmd.Flags().String("file", "", "Read the snippet from a file")
	checkABAPCmd.Flags().Bool("stdin", false, "Read the snippet from stdin")
	checkABAPCmd.Flags().Bool("json", false, "Emit the result as JSON")
	rootCmd.AddCommand(checkABAPCmd)
}

func runCheckABAP(cmd *cobra.Command, args []string) error {
	file, _ := cmd.Flags().GetString("file")
	stdin, _ := cmd.Flags().GetBool("stdin")
	asJSON, _ := cmd.Flags().GetBool("json")

	sources := 0
	for _, given := range []bool{stdin, file != "", len(args) > 0} {
		if given {
			sources++
		}
	}
	if sources > 1 {
		return fmt.Errorf("give the snippet one way only: as an argument, --file or --stdin")
	}

	var code string
	switch {
	case stdin:
		data, err := readStdin()
		if err != nil {
			return fmt.Errorf("failed to read stdin: %w", err)
		}
		code = string(data)
	case file != "":
		data, err := os.ReadFile(file)
		if err != nil {
			return fmt.Errorf("failed to read file: %w", err)
		}
		code = string(data)
	case len(args) > 0:
		code = args[0]
	default:
		return fmt.Errorf("usage: vsp check-abap <code>, or use --file/--stdin")
	}

	params, err := resolveSystemParams(cmd)
	if err != nil {
		return err
	}
	client, err := getClient(params)
	if err != nil {
		return err
	}

	// Ctrl-C cancels the check rather than killing the process, so CheckABAP
	// gets to run its deferred delete of the temporary program.
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	result, err := client.CheckABAP(ctx, code)
	if err != nil {
		return fmt.Errorf("check-abap: %w", err)
	}

	if asJSON {
		out, _ := json.MarshalIndent(result, "", "  ")
		fmt.Println(string(out))
	} else {
		printCheckABAPResult(os.Stdout, result)
	}
	for _, w := range result.Warnings {
		fmt.Fprintf(os.Stderr, "! %s\n", w)
	}
	if !result.CleanedUp {
		// A leftover program is a failure whatever the check found: a script
		// reading only the exit status must not take it as a clean run.
		return fmt.Errorf("the temporary program %s may still be in $TMP; delete it", result.ProgramName)
	}
	if !result.OK {
		return fmt.Errorf("the snippet does not compile")
	}
	return nil
}

// printCheckABAPResult writes one line per finding, compiler style.
func printCheckABAPResult(w io.Writer, result *adt.CheckABAPResult) {
	if len(result.Findings) == 0 {
		fmt.Fprintln(w, "OK: no findings")
		return
	}
	for _, f := range result.Findings {
		switch {
		case f.Line > 0:
			fmt.Fprintf(w, "%d:%d: %s: %s\n", f.Line, f.Column, f.Severity, f.Message)
		case f.AfterSnippet:
			fmt.Fprintf(w, "after the snippet's last line (a statement without its period, or a block left open?): %s: %s\n", f.Severity, f.Message)
		case f.WrapperLine > 0:
			fmt.Fprintf(w, "outside the snippet (wrapper line %d): %s: %s\n", f.WrapperLine, f.Severity, f.Message)
		default:
			fmt.Fprintf(w, "%s: %s\n", f.Severity, f.Message)
		}
	}
}
