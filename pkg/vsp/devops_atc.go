package vsp

import (
	"context"
	"fmt"
	"strings"

	"github.com/spf13/cobra"
)

// --- atc command ---

var atcCmd = &cobra.Command{
	Use:   "atc <type> <name>",
	Short: "Run ATC checks",
	Long: `Run ABAP Test Cockpit (ATC) checks on an object.

Examples:
  vsp atc CLAS ZCL_MY_CLASS
  vsp atc PROG ZTEST_REPORT
  vsp atc CLAS ZCL_FOO --variant MY_VARIANT`,
	Args: cobra.ExactArgs(2),
	RunE: runATC,
}

func runATC(cmd *cobra.Command, args []string) error {
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
	variant, _ := cmd.Flags().GetString("variant")
	maxFindings, _ := cmd.Flags().GetInt("max-findings")

	objectURL := buildObjectURL(objType, name)
	if objectURL == "" {
		return fmt.Errorf("unsupported object type: %s (supported: CLAS, PROG, INCL, INTF)", objType)
	}

	ctx := context.Background()
	worklist, err := client.RunATCCheck(ctx, objectURL, variant, maxFindings)
	if err != nil {
		return fmt.Errorf("ATC check failed: %w", err)
	}

	// Format output
	totalFindings := 0
	for _, obj := range worklist.Objects {
		if len(obj.Findings) == 0 {
			continue
		}
		fmt.Printf("%s %s (%s)\n", obj.Type, obj.Name, obj.PackageName)
		for _, f := range obj.Findings {
			priority := "INFO"
			switch f.Priority {
			case 1:
				priority = "ERROR"
			case 2:
				priority = "WARN"
			}
			location := ""
			if f.Line > 0 {
				location = fmt.Sprintf(" [line %d]", f.Line)
			}
			fmt.Printf("  %s%s %s — %s\n", priority, location, f.CheckTitle, f.MessageTitle)
			totalFindings++
		}
	}

	if totalFindings == 0 {
		fmt.Println("No findings.")
	} else {
		fmt.Printf("\nTotal: %d finding(s)\n", totalFindings)
	}
	return nil
}

func init() {
	// ATC flags
	atcCmd.Flags().String("variant", "", "ATC check variant (empty for system default)")
	atcCmd.Flags().Int("max-findings", 100, "Maximum number of findings")

	rootCmd.AddCommand(atcCmd)
}
