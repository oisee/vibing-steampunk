package main

import (
	"context"
	"fmt"
	"io"
	"net/url"
	"os"
	"strings"

	"github.com/spf13/cobra"
)

var adtRequestCmd = &cobra.Command{
	Use:   "request <METHOD> <PATH>",
	Short: "Send one ADT request as given, over HTTP, with the session and CSRF token",
	Long: `One ADT request, as given: for a resource vsp has no command for yet, or to
see exactly what a resource answers. The session, the CSRF token and the
response cache are the client's own; the status line and headers go to stderr,
the body to stdout.

  vsp adt request GET /sap/bc/adt/discovery
  vsp adt request GET /sap/bc/adt/cts/transportrequests -q user=TESTUSER -H "Accept=application/vnd.sap.adt.transportorganizer.v1+xml"
  vsp adt request POST /sap/bc/adt/cts/transportchecks --body check.xml -H "Content-Type=application/vnd.sap.as+xml; charset=UTF-8; dataname=com.sap.adt.transport.service.checkData"

Anything but GET and HEAD is a write as far as the safety gates are concerned
(--read-only refuses it), and --body - reads the body from stdin.`,
	Args:         cobra.ExactArgs(2),
	SilenceUsage: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		client, _, err := docsClient(cmd)
		if err != nil {
			return err
		}
		query := url.Values{}
		for _, kv := range mustStringArray(cmd, "query") {
			k, v, ok := strings.Cut(kv, "=")
			if !ok {
				return fmt.Errorf("query %q must be NAME=VALUE", kv)
			}
			query.Add(k, v)
		}
		accept, contentType := "", ""
		for _, kv := range mustStringArray(cmd, "header") {
			k, v, ok := strings.Cut(kv, "=")
			if !ok {
				return fmt.Errorf("header %q must be NAME=VALUE", kv)
			}
			switch strings.ToLower(strings.TrimSpace(k)) {
			case "accept":
				accept = v
			case "content-type":
				contentType = v
			default:
				return fmt.Errorf("header %q: only Accept and Content-Type can be set here", k)
			}
		}
		var body []byte
		if path, _ := cmd.Flags().GetString("body"); path == "-" {
			if body, err = io.ReadAll(os.Stdin); err != nil {
				return err
			}
		} else if path != "" {
			if body, err = os.ReadFile(path); err != nil {
				return err
			}
		}
		stateful, _ := cmd.Flags().GetBool("stateful")
		resp, err := client.RawRequest(context.Background(), strings.ToUpper(args[0]), args[1], query, accept, contentType, body, stateful)
		if err != nil {
			return err
		}
		fmt.Fprintf(os.Stderr, "HTTP %d\n", resp.StatusCode)
		for k, vs := range resp.Headers {
			for _, v := range vs {
				fmt.Fprintf(os.Stderr, "%s: %s\n", k, v)
			}
		}
		fmt.Fprintf(os.Stderr, "(%d bytes)\n", len(resp.Body))
		if out, _ := cmd.Flags().GetString("output"); out != "" {
			return os.WriteFile(out, resp.Body, 0o644)
		}
		_, err = os.Stdout.Write(resp.Body)
		return err
	},
}

func mustStringArray(cmd *cobra.Command, name string) []string {
	v, _ := cmd.Flags().GetStringArray(name)
	return v
}

func init() {
	adtRequestCmd.Flags().StringArrayP("header", "H", nil, "Accept=... or Content-Type=... (repeatable)")
	adtRequestCmd.Flags().StringArrayP("query", "q", nil, "NAME=VALUE query parameter (repeatable)")
	adtRequestCmd.Flags().String("body", "", "File with the request body, or - for stdin")
	adtRequestCmd.Flags().String("output", "", "Write the body to this file instead of stdout")
	adtRequestCmd.Flags().Bool("stateful", false, "Send the request on the stateful session")
	adtRequestCmd.Flags().String("lang", "", "")
	_ = adtRequestCmd.Flags().MarkHidden("lang")
	adtCmd.AddCommand(adtRequestCmd)
}
