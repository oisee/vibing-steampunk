package main

import (
	"context"
	"fmt"
	"os"
	"sort"
	"strings"

	"github.com/spf13/cobra"
)

// adtGetCmd is a read-only window onto ADT paths that vsp does not model.
//
// Adding a new creatable object type to the objectTypes registry needs three
// facts SAP does not publish: the creation collection, the XML root element
// and the namespace. All three are readable from a live system, so read them
// rather than guess them.
var adtGetCmd = &cobra.Command{
	Use:   "adt-get <path>",
	Short: "Read an arbitrary ADT path (GET only)",
	Long: `Perform an authenticated read-only GET against an ADT path and print the response.

Use it to learn the contract for an object type vsp does not support yet.

Examples:
  # Every collection on the system, with the content types each one accepts
  vsp adt-get /sap/bc/adt/discovery

  # Only the collections whose href matches a substring
  vsp adt-get /sap/bc/adt/discovery --grep iam

  # One instance of an unmodelled type, to see its root element and namespace
  vsp adt-get /sap/bc/adt/aps/cloud/iam/sia1/sap_core_bc_a4c_dic

  # Against a named system from .vsp.json
  vsp -s n4d adt-get /sap/bc/adt/discovery`,
	Args: cobra.ExactArgs(1),
	RunE: runAdtGet,
}

func init() {
	adtGetCmd.Flags().String("accept", "", "Accept header. Defaults to a permissive ADT value.")
	adtGetCmd.Flags().String("grep", "", "Print only lines containing this substring, case insensitive.")
	adtGetCmd.Flags().String("out", "", "Write the body to this file instead of stdout.")
	adtGetCmd.Flags().Bool("headers", false, "Also print the response status and headers.")
	rootCmd.AddCommand(adtGetCmd)
}

func runAdtGet(cmd *cobra.Command, args []string) error {
	path := args[0]

	accept, _ := cmd.Flags().GetString("accept")
	grep, _ := cmd.Flags().GetString("grep")
	outFile, _ := cmd.Flags().GetString("out")
	showHeaders, _ := cmd.Flags().GetBool("headers")

	if accept == "" {
		// Permissive on purpose. ADT answers most resources with their own
		// vnd.sap.adt type, and asking for a specific one you guessed wrong
		// returns 406 and teaches you nothing.
		accept = "application/atomsvc+xml, application/atom+xml, application/xml, application/*, */*"
	}

	params, err := resolveSystemParams(cmd)
	if err != nil {
		return err
	}

	client, err := getClient(params)
	if err != nil {
		return err
	}

	resp, err := client.RawGet(context.Background(), path, accept)
	if err != nil {
		return fmt.Errorf("GET %s: %w", path, err)
	}

	if showHeaders {
		fmt.Fprintf(os.Stderr, "status: %d\n", resp.StatusCode)
		keys := make([]string, 0, len(resp.Headers))
		for k := range resp.Headers {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			fmt.Fprintf(os.Stderr, "%s: %s\n", k, strings.Join(resp.Headers[k], ", "))
		}
		fmt.Fprintln(os.Stderr, "---")
	}

	body := string(resp.Body)

	if grep != "" {
		needle := strings.ToLower(grep)
		var kept []string
		// ADT often returns one very long line, so split on tag boundaries
		// first to make substring filtering useful.
		normalised := strings.ReplaceAll(body, "><", ">\n<")
		for _, line := range strings.Split(normalised, "\n") {
			if strings.Contains(strings.ToLower(line), needle) {
				kept = append(kept, strings.TrimSpace(line))
			}
		}
		body = strings.Join(kept, "\n")
		if body != "" {
			body += "\n"
		}
	}

	if outFile != "" {
		if err := os.WriteFile(outFile, []byte(body), 0o600); err != nil {
			return fmt.Errorf("writing %s: %w", outFile, err)
		}
		fmt.Fprintf(os.Stderr, "wrote %d bytes to %s\n", len(body), outFile)
		return nil
	}

	fmt.Print(body)
	if !strings.HasSuffix(body, "\n") {
		fmt.Println()
	}
	return nil
}
