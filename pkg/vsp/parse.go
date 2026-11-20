package vsp

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/oisee/vibing-steampunk/pkg/abaplint"
	"github.com/spf13/cobra"
)

// --- parse command (ABAP analysis) ---

var parseCmd = &cobra.Command{
	Use:   "parse <type> <name>",
	Short: "Parse ABAP source and show structure",
	Long: `Parse ABAP source into tokens and statements.
Works offline with --file/--stdin, or fetches from SAP.

Examples:
  vsp parse CLAS ZCL_TEST
  vsp parse --file myclass.clas.abap
  echo "DATA lv_x TYPE i." | vsp parse --stdin
  vsp parse --file source.abap --format json`,
	RunE: runParse,
}

func init() {
	parseCmd.Flags().String("file", "", "Parse local file")
	parseCmd.Flags().Bool("stdin", false, "Read from stdin")
	parseCmd.Flags().String("format", "text", "Output format: text, json, summary")

	rootCmd.AddCommand(parseCmd)
}

func runParse(cmd *cobra.Command, args []string) error {
	file, _ := cmd.Flags().GetString("file")
	stdin, _ := cmd.Flags().GetBool("stdin")
	format, _ := cmd.Flags().GetString("format")

	var source string
	var filename string

	if stdin {
		data, _ := readStdin()
		source = string(data)
		filename = "stdin"
	} else if file != "" {
		data, err := os.ReadFile(file)
		if err != nil {
			return err
		}
		source = string(data)
		filename = file
	} else if len(args) >= 2 {
		params, err := resolveSystemParams(cmd)
		if err != nil {
			return err
		}
		client, err := getClient(params)
		if err != nil {
			return err
		}
		src, err := client.GetSource(context.Background(), args[0], args[1], nil)
		if err != nil {
			return err
		}
		source = src
		filename = args[1]
	} else {
		return fmt.Errorf("usage: vsp parse <type> <name>, or --file/--stdin")
	}

	lex := &abaplint.Lexer{}
	tokens := lex.Run(source)

	parser := &abaplint.StatementParser{}
	stmts := parser.Parse(tokens)

	matcher := abaplint.NewStatementMatcher()
	matcher.ClassifyStatements(stmts)

	switch format {
	case "summary":
		typeCount := map[string]int{}
		for _, s := range stmts {
			typeCount[s.Type]++
		}
		fmt.Printf("File: %s\n", filename)
		fmt.Printf("Tokens: %d\n", len(tokens))
		fmt.Printf("Statements: %d\n", len(stmts))
		fmt.Println("---")
		for t, c := range typeCount {
			fmt.Printf("  %-25s %d\n", t, c)
		}
	case "json":
		fmt.Print("[")
		for i, s := range stmts {
			if i > 0 {
				fmt.Print(",")
			}
			toks := make([]string, len(s.Tokens))
			for j, t := range s.Tokens {
				toks[j] = t.Str
			}
			fmt.Printf(`{"type":"%s","tokens":["%s"]}`, s.Type, strings.Join(toks, `","`))
		}
		fmt.Println("]")
	default:
		for _, s := range stmts {
			fmt.Printf("%-20s %s\n", s.Type, s.ConcatTokens())
		}
	}
	return nil
}
