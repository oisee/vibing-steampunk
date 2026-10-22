package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"

	"github.com/spf13/cobra"

	"github.com/oisee/vibing-steampunk/pkg/adt"
)

var formCmd = &cobra.Command{
	Use:   "form",
	Short: "Read and write print forms: SAPscript, Smart Forms, Adobe forms (needs ZADT_VSP)",
	Long: `Read and write print forms as documents, through ZADT_VSP's form service.

  SSFO  Smart Form, the XML of the SMARTFORMS download
  FORM  SAPscript form, abapGit's FORM structure, every language
  SFPF  Adobe form without the layout of its original language;
        with --language, the XDP layout of that language
  SFPI  Adobe form interface

A write goes through the same checks as any other: allowed packages, allowed
transport requests, read-only mode. Adobe forms and interfaces are activated
after the write; a replaced Adobe form that does not activate is put back.

Examples:
  vsp form info SFPF ZDEMO_PDF
  vsp form read SFPF ZDEMO_PDF --language ES -o zdemo_es.xdp
  vsp form write SFPF ZDEMO_PDF --language ES -f zdemo_es.xdp --transport A4HK900001
  vsp form write FORM ZDEMO_SCRIPT -f zdemo.xml --package '$TMP'      # creates it
  vsp form write SSFO ZDEMO_SF -f zdemo_sf.xml --test-run`,
}

var formInfoCmd = &cobra.Command{
	Use:   "info <TYPE> <NAME>",
	Short: "Package, original language, languages and inactive version of a form",
	Args:  cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		client, ws, closeWS, err := transportBridge(cmd)
		if err != nil {
			return err
		}
		defer closeWS()
		info, err := client.GetFormInfo(context.Background(), ws, args[0], args[1])
		if err != nil {
			return err
		}
		out, _ := json.MarshalIndent(info, "", "  ")
		fmt.Println(string(out))
		return nil
	},
}

var formReadCmd = &cobra.Command{
	Use:   "read <TYPE> <NAME>",
	Short: "Read a form as a document, to stdout or a file",
	Args:  cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		language, _ := cmd.Flags().GetString("language")
		outFile, _ := cmd.Flags().GetString("out")
		client, ws, closeWS, err := transportBridge(cmd)
		if err != nil {
			return err
		}
		defer closeWS()
		form, err := client.ReadForm(context.Background(), ws, args[0], args[1], language)
		if err != nil {
			return err
		}
		if outFile == "" {
			fmt.Print(form.Content)
			return nil
		}
		if err := os.WriteFile(outFile, []byte(form.Content), 0o644); err != nil {
			return err
		}
		fmt.Fprintf(os.Stderr, "%s %s -> %s (%d bytes, %s)\n", form.Type, form.Name, outFile, len(form.Content), form.MimeType)
		return nil
	},
}

var formWriteCmd = &cobra.Command{
	Use:   "write <TYPE> <NAME>",
	Short: "Write a form from a document (a file, or stdin with -f -)",
	Args:  cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		file, _ := cmd.Flags().GetString("file")
		var data []byte
		var err error
		switch file {
		case "":
			return fmt.Errorf("--file is required (use - for stdin)")
		case "-":
			data, err = io.ReadAll(os.Stdin)
		default:
			data, err = os.ReadFile(file)
		}
		if err != nil {
			return err
		}
		opts := adt.FormWriteOptions{Type: args[0], Name: args[1], Content: string(data)}
		opts.Language, _ = cmd.Flags().GetString("language")
		opts.Transport, _ = cmd.Flags().GetString("transport")
		opts.Package, _ = cmd.Flags().GetString("package")
		opts.TestRun, _ = cmd.Flags().GetBool("test-run")

		client, ws, closeWS, err := transportBridge(cmd)
		if err != nil {
			return err
		}
		defer closeWS()
		res, err := client.WriteForm(context.Background(), ws, opts)
		if res != nil {
			out, _ := json.MarshalIndent(res, "", "  ")
			fmt.Println(string(out))
		}
		return err
	},
}

func init() {
	formReadCmd.Flags().String("language", "", "Adobe form: read the XDP layout of this language (ISO code)")
	formReadCmd.Flags().StringP("out", "o", "", "Write the document to this file")
	formWriteCmd.Flags().StringP("file", "f", "", "The document to write (- for stdin)")
	formWriteCmd.Flags().String("language", "", "Adobe form: write the XDP layout of this language (ISO code)")
	formWriteCmd.Flags().String("transport", "", "Transport request (required for a transportable package)")
	formWriteCmd.Flags().String("package", "", "Package, to create a Smart Form or SAPscript form")
	formWriteCmd.Flags().Bool("test-run", false, "Check everything, save nothing")
	formCmd.AddCommand(formInfoCmd, formReadCmd, formWriteCmd)
	rootCmd.AddCommand(formCmd)
}
