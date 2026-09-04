package main

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"

	"github.com/oisee/vibing-steampunk/pkg/adt"
)

// Variants, function test data, documentation: the parts of a system that
// say what it is configured to do, read from their tables and rendered for
// a person or a machine.

var variantsCmd = &cobra.Command{
	Use:   "variants <REPORT> [VARIANT]",
	Short: "Report variants (SE38): list them, or show one with every field, its label and value",
	Long: `Report variants over plain ADT: VARID and VARIT for the list, the VARI
cluster for the values and the selection screen's shape, the program's
selection texts for the labels. What a job's variant selects, without
RS_VARIANT_CONTENTS.

  vsp variants ZDEMO_NIGHTLY_RUN                 # the variants
  vsp variants ZDEMO_NIGHTLY_RUN MONTH_END       # one, as a Markdown table
  vsp variants ZDEMO_NIGHTLY_RUN MONTH_END --json`,
	Args: cobra.RangeArgs(1, 2),
	RunE: func(cmd *cobra.Command, args []string) error {
		params, err := resolveSystemParams(cmd)
		if err != nil {
			return err
		}
		client, err := getClient(params)
		if err != nil {
			return err
		}
		asJSON, _ := cmd.Flags().GetBool("json")
		ctx := context.Background()
		if len(args) == 1 {
			list, err := client.Variants(ctx, args[0], params.Language)
			if err != nil {
				return err
			}
			if asJSON {
				return printJSON(list)
			}
			if len(list) == 0 {
				fmt.Fprintf(os.Stderr, "%s has no variants\n", strings.ToUpper(args[0]))
				return nil
			}
			for _, v := range list {
				line := fmt.Sprintf("%-14s %-40s %s %s", v.Name, v.Text, v.CreatedBy, v.Created.Format("2006-01-02"))
				if v.Protected {
					line += "  protected"
				}
				fmt.Println(line)
			}
			return nil
		}
		v, err := client.Variant(ctx, args[0], args[1], params.Language)
		if err != nil {
			return err
		}
		if asJSON {
			return printJSON(v)
		}
		fmt.Print(adt.VariantMarkdown(v))
		return nil
	},
}

var fmtestCmd = &cobra.Command{
	Use:   "fmtest <FUNCTION_MODULE>",
	Short: "The Function Builder's saved test data (SE37): interface, every set's inputs, outputs, runtime",
	Long: `Test data a developer saved in SE37, read from the EUFUNC cluster table:
the directory of sets with their titles and dates, the interface as it was
then, and each set's inputs (%_I), what came back (%_V), the runtime, the
return code and the exception.

  vsp fmtest ZDEMO_CALCULATE_TAX
  vsp fmtest ZDEMO_CALCULATE_TAX --json

A leading FM or FUNC, as the other commands take a type first, is accepted.`,
	Args: cobra.RangeArgs(1, 2),
	RunE: func(cmd *cobra.Command, args []string) error {
		name := args[len(args)-1]
		if len(args) == 2 {
			if t := strings.ToUpper(args[0]); t != "FM" && t != "FUNC" && t != "FUGR/FF" {
				return fmt.Errorf("fmtest takes a function module name; %q is not a type it knows (FM, FUNC)", args[0])
			}
		}
		params, err := resolveSystemParams(cmd)
		if err != nil {
			return err
		}
		client, err := getClient(params)
		if err != nil {
			return err
		}
		data, err := client.FunctionTestData(context.Background(), name)
		if err != nil {
			return err
		}
		if asJSON, _ := cmd.Flags().GetBool("json"); asJSON {
			return printJSON(data)
		}
		fmt.Print(adt.FunctionTestMarkdown(data))
		return nil
	},
}

var docsCmd = &cobra.Command{
	Use:   "docs",
	Short: "Documentation (SE61) and the IMG as Markdown: objects, activities, where a setting lives",
	Long: `SAP's documentation is two tables, DOKIL and DOKTL, and the IMG hangs off
them. This reads the ITF text and renders it as Markdown, includes resolved.

  vsp docs read DE BALLEVEL                      # a data element's documentation
  vsp docs read FU BAL_LOG_CREATE                # a function module's
  vsp docs read RE RSPARAM --lang D              # a report's, in German
  vsp docs index BAL_LOG_CREATE                  # every class and language that has one
  vsp docs img "delta link"                      # IMG nodes whose text matches, with the path to each
  vsp docs activity /IWBEP/CP_DELETE_JOB         # one activity: transaction, path, documentation

Classes: DE data element, DT domain, TB table, RE report, FU function module,
CL class, IF interface, NA message, TX general text, HY IMG activity.`,
}

var docsReadCmd = &cobra.Command{
	Use:   "read <CLASS> <OBJECT>",
	Short: "Render one documentation text as Markdown",
	Args:  cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		client, lang, err := docsClient(cmd)
		if err != nil {
			return err
		}
		doc, err := client.Documentation(context.Background(), args[0], args[1], lang)
		if err != nil {
			return err
		}
		if asJSON, _ := cmd.Flags().GetBool("json"); asJSON {
			return printJSON(doc)
		}
		fmt.Printf("# %s %s\n\n%s", doc.ID, doc.Object, doc.Markdown)
		return nil
	},
}

var docsIndexCmd = &cobra.Command{
	Use:   "index <OBJECT>",
	Short: "List the documentation texts that exist for an object name",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		client, _, err := docsClient(cmd)
		if err != nil {
			return err
		}
		list, err := client.DocumentationIndex(context.Background(), args[0])
		if err != nil {
			return err
		}
		if asJSON, _ := cmd.Flags().GetBool("json"); asJSON {
			return printJSON(list)
		}
		for _, d := range list {
			fmt.Printf("%-3s %-8s %-30s %s\n", d.ID, d.Language, d.Object, adt.DocumentationClasses[d.ID])
		}
		if len(list) == 0 {
			fmt.Fprintln(os.Stderr, "no documentation")
		}
		return nil
	},
}

var docsIMGCmd = &cobra.Command{
	Use:   "img <TEXT>",
	Short: "Find IMG nodes by their text, with the path to each and what it opens",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		client, lang, err := docsClient(cmd)
		if err != nil {
			return err
		}
		top, _ := cmd.Flags().GetInt("top")
		nodes, err := client.IMGSearch(context.Background(), args[0], lang, top)
		if err != nil {
			return err
		}
		if asJSON, _ := cmd.Flags().GetBool("json"); asJSON {
			return printJSON(nodes)
		}
		if len(nodes) == 0 {
			fmt.Fprintln(os.Stderr, "no IMG node matches")
			return nil
		}
		for _, n := range nodes {
			fmt.Printf("%s\n", n.Text)
			if n.Path != "" {
				fmt.Printf("    %s\n", n.Path)
			}
			if n.RefObject != "" {
				fmt.Printf("    %s %s", n.RefType, n.RefObject)
				if n.Transaction != "" {
					fmt.Printf("  (transaction %s)", n.Transaction)
				}
				fmt.Println()
			}
		}
		return nil
	},
}

var docsActivityCmd = &cobra.Command{
	Use:   "activity <ACTIVITY>",
	Short: "One IMG activity: transaction, the paths to it, its documentation",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		client, lang, err := docsClient(cmd)
		if err != nil {
			return err
		}
		a, err := client.IMGActivity(context.Background(), args[0], lang)
		if err != nil {
			return err
		}
		if asJSON, _ := cmd.Flags().GetBool("json"); asJSON {
			return printJSON(a)
		}
		fmt.Printf("# IMG activity %s\n\n", a.Activity)
		if a.Text != "" {
			fmt.Printf("%s\n\n", a.Text)
		}
		if a.Transaction != "" {
			fmt.Printf("Transaction: %s\n\n", a.Transaction)
		}
		for _, p := range a.Paths {
			fmt.Printf("- %s\n", p)
		}
		if len(a.Paths) > 0 {
			fmt.Println()
		}
		fmt.Print(a.Documentation)
		return nil
	},
}

func docsClient(cmd *cobra.Command) (*adt.Client, string, error) {
	params, err := resolveSystemParams(cmd)
	if err != nil {
		return nil, "", err
	}
	client, err := getClient(params)
	if err != nil {
		return nil, "", err
	}
	lang, _ := cmd.Flags().GetString("lang")
	if lang == "" {
		lang = params.Language
	}
	return client, lang, nil
}

func init() {
	variantsCmd.Flags().Bool("json", false, "Emit JSON")
	fmtestCmd.Flags().Bool("json", false, "Emit JSON")
	for _, c := range []*cobra.Command{docsReadCmd, docsIndexCmd, docsIMGCmd, docsActivityCmd} {
		c.Flags().Bool("json", false, "Emit JSON")
		c.Flags().String("lang", "", "Language (ISO code or SAP key); the system's logon language by default")
	}
	docsIMGCmd.Flags().Int("top", 40, "Maximum nodes")
	docsCmd.AddCommand(docsReadCmd, docsIndexCmd, docsIMGCmd, docsActivityCmd)
	rootCmd.AddCommand(variantsCmd, fmtestCmd, docsCmd)
}
