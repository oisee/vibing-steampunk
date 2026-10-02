package main

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"
)

// --- deploy command ---

var deployCmd = &cobra.Command{
	Use:   "deploy <file> <package>",
	Short: "Deploy ABAP source file to SAP",
	Long: `Deploy an ABAP source file to a SAP package.

Supports abapGit-compatible file extensions:
  .clas.abap, .prog.abap, .intf.abap, .ddls.asddls, etc.

Examples:
  vsp deploy zcl_test.clas.abap '$TMP'
  vsp deploy zreport.prog.abap '$TMP' --transport A4HK900001`,
	Args: cobra.ExactArgs(2),
	RunE: runDeploy,
}

func runDeploy(cmd *cobra.Command, args []string) error {
	params, err := resolveSystemParams(cmd)
	if err != nil {
		return err
	}

	client, err := getClient(params)
	if err != nil {
		return err
	}

	filePath := args[0]
	packageName := strings.ToUpper(args[1])
	transport, _ := cmd.Flags().GetString("transport")

	budget, err := resolveCallTimeout(cmd)
	if err != nil {
		return err
	}
	ctx, cancel := withWriteBudget(context.Background(), budget)
	defer cancel()
	result, err := client.DeployFromFile(ctx, filePath, packageName, transport)
	if err != nil {
		return fmt.Errorf("deploy failed: %w", err)
	}

	if result.Success {
		action := "Updated"
		if result.Created {
			action = "Created"
		}
		fmt.Fprintf(os.Stderr, "%s %s %s\n", action, result.ObjectType, result.ObjectName)
		if result.ObjectURL != "" {
			fmt.Fprintf(os.Stderr, "URL: %s\n", result.ObjectURL)
		}
		if result.Transport != "" {
			fmt.Fprintf(os.Stderr, "Transport: %s\n", result.Transport)
		}
		if result.TransportNote != "" {
			fmt.Fprintf(os.Stderr, "  %s\n", result.TransportNote)
		}
		if result.Message != "" {
			fmt.Fprintf(os.Stderr, "%s\n", result.Message)
		}
	} else {
		fmt.Fprintf(os.Stderr, "Deploy failed for %s\n", filePath)
		if result.Message != "" {
			fmt.Fprintf(os.Stderr, "%s\n", result.Message)
		}
		for _, e := range result.Errors {
			fmt.Fprintf(os.Stderr, "  %s\n", e)
		}
		if len(result.SyntaxErrors) > 0 {
			fmt.Fprintf(os.Stderr, "Syntax errors:\n")
			for _, se := range result.SyntaxErrors {
				fmt.Fprintf(os.Stderr, "  %s\n", se)
			}
		}
		return fmt.Errorf("deploy failed")
	}

	return nil
}

func init() {
	// Deploy flags
	deployCmd.Flags().String("transport", "", "Transport request number")
	addCallTimeoutFlag(deployCmd)

	rootCmd.AddCommand(deployCmd)
}
