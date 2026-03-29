package main

import (
	"fmt"
	"os"

	"github.com/vinchacho/vibing-steampunk/internal/lsp"
	"github.com/vinchacho/vibing-steampunk/pkg/adt"
	"github.com/spf13/cobra"
)

var lspCmd = &cobra.Command{
	Use:   "lsp",
	Short: "Start ABAP LSP server over stdio",
	Long: `Start an LSP (Language Server Protocol) server for ABAP files.

The LSP server provides:
  - Real-time syntax checking via SAP ADT SyntaxCheck
  - Go-to-definition via SAP ADT FindDefinition
  - Diagnostic feedback on file open and change

Requires SAP connection for online diagnostics. Without SAP credentials,
the server starts but provides no diagnostics.

Configure in Claude Code settings:
  {
    "abap": {
      "command": "vsp",
      "args": ["lsp", "--stdio"],
      "extensionToLanguage": {
        ".abap": "abap",
        ".asddls": "abap",
        ".asbdef": "abap"
      }
    }
  }

Examples:
  # Start LSP server (stdio)
  vsp lsp --stdio

  # With explicit SAP connection
  vsp --url https://host:44300 --user admin --password secret lsp --stdio`,
	RunE: runLSP,
}

var lspStdio bool

func init() {
	lspCmd.Flags().BoolVar(&lspStdio, "stdio", false, "Use stdio transport (required)")
	rootCmd.AddCommand(lspCmd)
}

func runLSP(cmd *cobra.Command, args []string) error {
	if !lspStdio {
		return fmt.Errorf("--stdio flag is required (only stdio transport is supported)")
	}

	// Try to resolve SAP config — LSP can work without it (no diagnostics)
	resolveConfig(cmd.Root())

	var client *adt.Client
	if cfg.BaseURL != "" {
		// Try to set up SAP connection for online diagnostics
		if err := processCookieAuth(cmd.Root()); err == nil {
			client = createADTClient()
			if cfg.Verbose {
				fmt.Fprintf(os.Stderr, "[LSP] Connected to SAP: %s\n", cfg.BaseURL)
			}
		} else if cfg.Verbose {
			fmt.Fprintf(os.Stderr, "[LSP] SAP auth failed, running without diagnostics: %v\n", err)
		}
	} else if cfg.Verbose {
		fmt.Fprintf(os.Stderr, "[LSP] No SAP URL configured, running without diagnostics\n")
	}

	server := lsp.NewServer(client, cfg.Verbose)
	return server.Serve(os.Stdin, os.Stdout)
}
