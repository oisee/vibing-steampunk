package main

import (
	"context"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/oisee/vibing-steampunk/pkg/adt"
	"github.com/oisee/vibing-steampunk/pkg/ctxcomp"
	"github.com/spf13/cobra"
)

// --- source subcommands ---

var sourceReadCmd = &cobra.Command{
	Use:   "read <type> <name>",
	Short: "Read ABAP source code",
	Long: `Read source code for an ABAP object (same as 'vsp source <type> <name>').

With --summary it prints JSON metadata instead of the source: lines, bytes,
sha256 (lower-case hex SHA-256 of the exact text as read, not normalised),
sourceHash (for a guarded write) and uri. With --if-none-match <sha256> it
prints one "unchanged (sha256 ...)" line when the source still has that
digest, and the source otherwise. Either is the same single read.

Examples:
  vsp source read CLAS ZCL_MY_CLASS
  vsp source read PROG ZTEST_PROGRAM
  vsp source read CLAS ZCL_MY_CLASS --summary
  vsp source read CLAS ZCL_MY_CLASS --if-none-match 3f0a...`,
	Args: cobra.ExactArgs(2),
	RunE: runSource, // reuse existing handler
}

var sourceWriteCmd = &cobra.Command{
	Use:   "write <type> <name>",
	Short: "Write ABAP source code from stdin",
	Long: `Write source code to an ABAP object. Reads source from stdin.

Examples:
  cat myclass.abap | vsp source write CLAS ZCL_MY_CLASS
  echo "REPORT ztest." | vsp source write PROG ZTEST
  vsp source write CLAS ZCL_FOO --transport A4HK900001 < source.abap`,
	Args: cobra.ExactArgs(2),
	RunE: runSourceWrite,
}

var sourceEditCmd = &cobra.Command{
	Use:   "edit <type> <name>",
	Short: "Edit ABAP source code (string replacement)",
	Long: `Perform surgical string replacement on ABAP source code.

Examples:
  vsp source edit CLAS ZCL_FOO --old "rv_result = 1." --new "rv_result = 42."
  vsp source edit PROG ZTEST --old "old code" --new "new code" --replace-all`,
	Args: cobra.ExactArgs(2),
	RunE: runSourceEdit,
}

var sourceContextCmd = &cobra.Command{
	Use:   "context <type> <name>",
	Short: "Get source with compressed dependency contracts",
	Long: `Retrieve source code with auto-appended dependency contracts.
Dependencies are extracted from the source and their public APIs are compressed.

Examples:
  vsp source context CLAS ZCL_MY_CLASS
  vsp source context CLAS ZCL_FOO --max-deps 30`,
	Args: cobra.ExactArgs(2),
	RunE: runSourceContext,
}

// --- context top-level shortcut ---

var contextCmd = &cobra.Command{
	Use:   "context <type> <name>",
	Short: "Get source with compressed dependency contracts",
	Long: `Retrieve source code with auto-appended dependency contracts (shortcut for 'vsp source context').
Use --depth 2 or 3 to expand transitive dependencies (deps of deps).

Examples:
  vsp context CLAS ZCL_MY_CLASS
  vsp context CLAS ZCL_FOO --max-deps 30
  vsp context CLAS ZCL_DEEP --depth 2   # deps of deps`,
	Args: cobra.ExactArgs(2),
	RunE: runSourceContext,
}

func runSourceWrite(cmd *cobra.Command, args []string) error {
	params, err := resolveSystemParams(cmd)
	if err != nil {
		return err
	}

	client, err := getClient(params)
	if err != nil {
		return err
	}

	objType := strings.ToUpper(args[0])
	name := strings.ToUpper(args[1])
	transport, _ := cmd.Flags().GetString("transport")

	// Read source from stdin
	source, err := io.ReadAll(os.Stdin)
	if err != nil {
		return fmt.Errorf("failed to read stdin: %w", err)
	}
	if len(source) == 0 {
		return fmt.Errorf("no source provided on stdin")
	}

	budget, err := resolveCallTimeout(cmd)
	if err != nil {
		return err
	}
	ctx, cancel := withWriteBudget(context.Background(), budget)
	defer cancel()
	result, err := client.WriteSource(ctx, objType, name, string(source), &adt.WriteSourceOptions{
		Transport: transport,
	})
	if err != nil {
		return fmt.Errorf("write failed: %w", err)
	}

	if result.Success {
		fmt.Fprintf(os.Stderr, "%s %s %s\n", result.Mode, result.ObjectType, result.ObjectName)
		if result.ObjectURL != "" {
			fmt.Fprintf(os.Stderr, "URL: %s\n", result.ObjectURL)
		}
	} else {
		fmt.Fprintf(os.Stderr, "Write failed for %s %s\n", objType, name)
		if result.Message != "" {
			fmt.Fprintf(os.Stderr, "%s\n", result.Message)
		}
		if len(result.SyntaxErrors) > 0 {
			fmt.Fprintf(os.Stderr, "Syntax errors:\n")
			for _, se := range result.SyntaxErrors {
				fmt.Fprintf(os.Stderr, "  Line %d: %s\n", se.Line, se.Text)
			}
		}
		printActivationMessages(os.Stderr, result.Activation)
		return fmt.Errorf("write failed")
	}

	return nil
}

func runSourceEdit(cmd *cobra.Command, args []string) error {
	params, err := resolveSystemParams(cmd)
	if err != nil {
		return err
	}

	client, err := getClient(params)
	if err != nil {
		return err
	}

	objType := strings.ToUpper(args[0])
	name := strings.ToUpper(args[1])
	oldStr, _ := cmd.Flags().GetString("old")
	newStr, _ := cmd.Flags().GetString("new")
	replaceAll, _ := cmd.Flags().GetBool("replace-all")
	transport, _ := cmd.Flags().GetString("transport")

	// Build object URL from type + name
	objectURL := buildObjectURL(objType, name)
	if objectURL == "" {
		return fmt.Errorf("unsupported object type: %s (supported: CLAS, PROG, INCL, INTF)", objType)
	}

	budget, err := resolveCallTimeout(cmd)
	if err != nil {
		return err
	}
	ctx, cancel := withWriteBudget(context.Background(), budget)
	defer cancel()
	result, err := client.EditSourceWithOptions(ctx, objectURL, oldStr, newStr, &adt.EditSourceOptions{
		ReplaceAll:  replaceAll,
		SyntaxCheck: true,
		Transport:   transport,
	})
	if err != nil {
		return fmt.Errorf("edit failed: %w", err)
	}

	if result.Success {
		fmt.Fprintf(os.Stderr, "Edited %s (%d replacement(s))\n", result.ObjectName, result.MatchCount)
		if result.Activation != nil && result.Activation.Success {
			fmt.Fprintf(os.Stderr, "Activated successfully\n")
		}
	} else {
		fmt.Fprintf(os.Stderr, "Edit failed for %s\n", result.ObjectName)
		if result.Message != "" {
			fmt.Fprintf(os.Stderr, "%s\n", result.Message)
		}
		if len(result.SyntaxErrors) > 0 {
			fmt.Fprintf(os.Stderr, "Syntax errors:\n")
			for _, se := range result.SyntaxErrors {
				fmt.Fprintf(os.Stderr, "  %s\n", se)
			}
		}
		printActivationMessages(os.Stderr, result.Activation)
		return fmt.Errorf("edit failed")
	}

	return nil
}

// cliSourceAdapter adapts adt.Client to ctxcomp.ADTSourceFetcher interface.
type cliSourceAdapter struct {
	client *adt.Client
}

func (a *cliSourceAdapter) GetSource(ctx context.Context, objectType, name string, opts interface{}) (string, error) {
	return a.client.GetSource(ctx, objectType, name, nil)
}

func runSourceContext(cmd *cobra.Command, args []string) error {
	params, err := resolveSystemParams(cmd)
	if err != nil {
		return err
	}

	client, err := getClient(params)
	if err != nil {
		return err
	}

	objType := strings.ToUpper(args[0])
	name := strings.ToUpper(args[1])
	maxDeps, _ := cmd.Flags().GetInt("max-deps")

	ctx := context.Background()

	// First get the source
	source, err := client.GetSource(ctx, objType, name, nil)
	if err != nil {
		return fmt.Errorf("failed to get source: %w", err)
	}

	depth, _ := cmd.Flags().GetInt("depth")

	// Create adapter and compress
	// Who calls this — the half a source cannot tell you. Opt-in, because a hub
	// class has thousands of callers and takes four seconds to ask about, and a
	// plain read should not pay that without being asked.
	var upstream string
	if callers, _ := cmd.Flags().GetBool("callers"); callers {
		uri := adt.GetClassIncludeURL(strings.ToUpper(name), adt.ClassIncludeMain)
		if strings.ToUpper(objType) != "CLAS" {
			uri = ""
		}
		if uri != "" {
			uri = strings.TrimSuffix(uri, "/source/main")
			found, unresolved, err := client.WhereUsed(ctx, uri)
			if err != nil {
				fmt.Fprintf(os.Stderr, "  the caller list could not be read: %v\n", err)
			} else {
				if note := adt.UnresolvedIncludesNote(unresolved); note != "" {
					fmt.Fprintf(os.Stderr, "  %s\n", note)
				}
				list := make([]ctxcomp.Caller, 0, len(found))
				for _, f := range found {
					list = append(list, ctxcomp.Caller{
						Name: f.Name, Type: f.Type, Component: f.Component,
						Package: f.Package, IsTest: f.IsTest,
					})
				}
				upstream = ctxcomp.SummariseCallers(list, 6).Text()
			}
		}
	}

	provider := ctxcomp.NewMultiSourceProvider("", &cliSourceAdapter{client: client})
	compressor := ctxcomp.NewCompressor(provider, maxDeps).WithDepth(depth)
	result, err := compressor.Compress(ctx, source, name, objType)
	if err != nil {
		return fmt.Errorf("context compression failed: %w", err)
	}

	// Output: source with prologue
	if upstream != "" {
		fmt.Print(upstream)
		fmt.Println()
	}
	if result.Prologue != "" {
		fmt.Print(result.Prologue)
		fmt.Println()
	}
	fmt.Print(source)

	// Stats to stderr
	fmt.Fprintf(os.Stderr, "\n--- Context: %d deps found, %d resolved, %d failed, %d prologue lines ---\n",
		result.Stats.DepsFound, result.Stats.DepsResolved, result.Stats.DepsFailed, result.Stats.TotalLines)

	return nil
}

func init() {
	// Source subcommands
	sourceCmd.AddCommand(sourceReadCmd)
	sourceCmd.AddCommand(sourceWriteCmd)
	sourceCmd.AddCommand(sourceEditCmd)
	sourceCmd.AddCommand(sourceContextCmd)
	addSourceReadFlags(sourceReadCmd)

	// Source write flags
	sourceWriteCmd.Flags().String("transport", "", "Transport request number")
	addCallTimeoutFlag(sourceWriteCmd, sourceEditCmd)

	// Source edit flags
	sourceEditCmd.Flags().String("old", "", "String to find (required)")
	sourceEditCmd.Flags().String("new", "", "Replacement string (required)")
	sourceEditCmd.Flags().Bool("replace-all", false, "Replace all occurrences")
	sourceEditCmd.Flags().String("transport", "", "Transport request number")
	_ = sourceEditCmd.MarkFlagRequired("old")
	_ = sourceEditCmd.MarkFlagRequired("new")

	// Source context and context shortcut flags
	sourceContextCmd.Flags().Int("max-deps", 20, "Maximum number of dependencies to resolve")
	sourceContextCmd.Flags().Int("depth", 1, "Dependency expansion depth (1-3)")
	contextCmd.Flags().Int("max-deps", 20, "Maximum number of dependencies to resolve")
	contextCmd.Flags().Bool("callers", false, "Also summarise who calls this — the half the source cannot tell you")
	contextCmd.Flags().Int("depth", 1, "Dependency expansion depth (1-3)")

	rootCmd.AddCommand(contextCmd)
}
