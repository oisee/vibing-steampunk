package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/oisee/open-rfc-go/rfc"
	"github.com/oisee/vibing-steampunk/pkg/adt"
	"github.com/oisee/vibing-steampunk/pkg/saprfc"
)

var spoolCmd = &cobra.Command{
	Use:   "spool",
	Short: "Spool requests (SP01): list them, read one, export many — over ADT, XBP when TemSe is in files",
	Long: `Spool requests over plain ADT.

TSP01 is the request, TST01 the TemSe object it points at, TST03 the content
when the system keeps spool in the database (the default). A list spool is
decoded here: the TemSe records, the print controls, the format escapes. When
a system keeps spool in files, TST03 is empty and the content comes over RFC
from the XBP interface instead (--via rfc, or automatically when ADT cannot).

  vsp spool list --since 2026-09-01 --user TESTUSER
  vsp spool list --job ZDEMO_NIGHTLY            # what that job's steps wrote
  vsp spool read 27302
  vsp spool export --since 2026-09-01 --out ./spool   # one file per request, and an index`,
}

var spoolListCmd = &cobra.Command{
	Use:   "list",
	Short: "List spool requests, newest first, with the job step that wrote each",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		client, filter, err := spoolClientAndFilter(cmd)
		if err != nil {
			return err
		}
		reqs, err := client.SpoolRequests(context.Background(), filter)
		if err != nil {
			return err
		}
		if asJSON, _ := cmd.Flags().GetBool("json"); asJSON {
			return printJSON(reqs)
		}
		if len(reqs) == 0 {
			fmt.Fprintln(os.Stderr, "no spool requests match")
			return nil
		}
		fmt.Printf("%-8s %-19s %-12s %-5s %6s %5s %-30s %s\n", "NUMBER", "CREATED", "OWNER", "TYPE", "LINES", "STORE", "TITLE", "JOB")
		fmt.Println(strings.Repeat("-", 110))
		for _, r := range reqs {
			job := ""
			if r.Job != nil {
				job = fmt.Sprintf("%s/%s step %d %s", r.Job.Name, r.Job.Count, r.Job.Step, r.Job.Program)
			}
			fmt.Printf("%-8d %-19s %-12s %-5s %6d %5s %-30.30s %s\n", r.Number, r.Created.Format("2006-01-02 15:04:05"), r.Owner, r.DocType, r.Lines, r.Storage, r.Title, job)
		}
		fmt.Fprintf(os.Stderr, "\n%d requests\n", len(reqs))
		return nil
	},
}

var spoolReadCmd = &cobra.Command{
	Use:   "read <NUMBER>",
	Short: "Read one spool request's content",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		number, err := strconv.Atoi(strings.TrimSpace(args[0]))
		if err != nil {
			return fmt.Errorf("spool number %q is not a number", args[0])
		}
		via, _ := cmd.Flags().GetString("via")
		asJSON, _ := cmd.Flags().GetBool("json")
		if via == "rfc" {
			return withRFC(cmd, func(ctx context.Context, c *rfc.Client) error {
				lines, err := saprfc.ReadSpoolRequest(ctx, c, number)
				if err != nil {
					return err
				}
				if asJSON {
					return printJSON(map[string]any{"number": number, "via": "rfc", "lines": lines})
				}
				for _, l := range lines {
					fmt.Println(l)
				}
				return nil
			})
		}
		params, err := resolveSystemParams(cmd)
		if err != nil {
			return err
		}
		client, err := getClient(params)
		if err != nil {
			return err
		}
		content, err := client.Spool(context.Background(), number)
		if err != nil {
			if content != nil && content.Request.Storage != "" && content.Request.Storage != "D" && via == "auto" {
				fmt.Fprintf(os.Stderr, "%v; reading over RFC\n", err)
				return withRFC(cmd, func(ctx context.Context, c *rfc.Client) error {
					lines, err := saprfc.ReadSpoolRequest(ctx, c, number)
					if err != nil {
						return err
					}
					for _, l := range lines {
						fmt.Println(l)
					}
					return nil
				})
			}
			return err
		}
		if asJSON {
			return printJSON(content)
		}
		if content.List != nil {
			fmt.Print(content.List.Text())
			return nil
		}
		if raw, _ := cmd.Flags().GetBool("raw"); raw || content.Raw != nil {
			_, err = os.Stdout.Write(content.Raw)
			return err
		}
		return nil
	},
}

var spoolExportCmd = &cobra.Command{
	Use:   "export",
	Short: "Write every matching spool request to a directory, one file each, with an index",
	Long: `Export spool requests: <number>.txt per request (the decoded list, or the raw
content with the document type as extension), index.json with every request's
header and job step, and index.md as a table. Requests whose content ADT
cannot read — TemSe in files — are listed in the index with the reason, and
read over RFC when --via rfc is given.`,
	Args: cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		client, filter, err := spoolClientAndFilter(cmd)
		if err != nil {
			return err
		}
		out, _ := cmd.Flags().GetString("out")
		if out == "" {
			return fmt.Errorf("--out DIR is required")
		}
		if err := os.MkdirAll(out, 0o755); err != nil {
			return err
		}
		via, _ := cmd.Flags().GetString("via")
		ctx := context.Background()
		reqs, err := client.SpoolRequests(ctx, filter)
		if err != nil {
			return err
		}
		type entry struct {
			adt.SpoolRequest
			File  string `json:"file,omitempty"`
			Error string `json:"error,omitempty"`
		}
		var index []entry
		written := 0
		var rfcClient *rfc.Client
		for _, r := range reqs {
			e := entry{SpoolRequest: r}
			content, cerr := client.Spool(ctx, r.Number)
			switch {
			case cerr == nil && content.List != nil:
				e.File = fmt.Sprintf("%d.txt", r.Number)
				cerr = os.WriteFile(filepath.Join(out, e.File), []byte(content.List.Text()), 0o644)
			case cerr == nil && content.Raw != nil:
				e.File = fmt.Sprintf("%d.%s", r.Number, strings.ToLower(nonEmpty(r.DocType, "bin")))
				cerr = os.WriteFile(filepath.Join(out, e.File), content.Raw, 0o644)
			case cerr != nil && via == "rfc":
				if rfcClient == nil {
					if rfcClient, err = rfcClientFor(cmd); err != nil {
						return err
					}
				}
				lines, rerr := saprfc.ReadSpoolRequest(ctx, rfcClient, r.Number)
				if rerr != nil {
					cerr = fmt.Errorf("%v; over RFC: %v", cerr, rerr)
					break
				}
				e.File = fmt.Sprintf("%d.txt", r.Number)
				cerr = os.WriteFile(filepath.Join(out, e.File), []byte(strings.Join(lines, "\n")+"\n"), 0o644)
			}
			if cerr != nil {
				e.Error = cerr.Error()
				e.File = ""
			} else {
				written++
			}
			index = append(index, e)
		}
		if rfcClient != nil {
			rfcClient.Close(ctx)
		}
		data, _ := json.MarshalIndent(index, "", "  ")
		if err := os.WriteFile(filepath.Join(out, "index.json"), data, 0o644); err != nil {
			return err
		}
		var md strings.Builder
		md.WriteString("# Spool requests\n\n| Number | Created | Owner | Type | Lines | Title | Job | File |\n|---|---|---|---|---:|---|---|---|\n")
		for _, e := range index {
			job := ""
			if e.Job != nil {
				job = fmt.Sprintf("%s/%s step %d (%s)", e.Job.Name, e.Job.Count, e.Job.Step, e.Job.Program)
			}
			file := e.File
			if e.Error != "" {
				file = "not read: " + e.Error
			}
			fmt.Fprintf(&md, "| %d | %s | %s | %s | %d | %s | %s | %s |\n", e.Number, e.Created.Format("2006-01-02 15:04:05"), e.Owner, e.DocType, e.Lines, strings.ReplaceAll(e.Title, "|", "\\|"), job, file)
		}
		if err := os.WriteFile(filepath.Join(out, "index.md"), []byte(md.String()), 0o644); err != nil {
			return err
		}
		fmt.Fprintf(os.Stderr, "%d of %d spool requests written to %s\n", written, len(index), out)
		return nil
	},
}

func spoolClientAndFilter(cmd *cobra.Command) (*adt.Client, adt.SpoolFilter, error) {
	params, err := resolveSystemParams(cmd)
	if err != nil {
		return nil, adt.SpoolFilter{}, err
	}
	client, err := getClient(params)
	if err != nil {
		return nil, adt.SpoolFilter{}, err
	}
	f := adt.SpoolFilter{}
	f.Owner, _ = cmd.Flags().GetString("user")
	f.Title, _ = cmd.Flags().GetString("title")
	f.Program, _ = cmd.Flags().GetString("program")
	f.Job, _ = cmd.Flags().GetString("job")
	f.Limit, _ = cmd.Flags().GetInt("top")
	if f.From, f.To, err = dateWindow(cmd); err != nil {
		return nil, adt.SpoolFilter{}, err
	}
	return client, f, nil
}

// dateWindow reads --since and --until as YYYY-MM-DD, the end inclusive.
func dateWindow(cmd *cobra.Command) (time.Time, time.Time, error) {
	var from, to time.Time
	for _, spec := range []struct {
		flag string
		into *time.Time
		end  bool
	}{{"since", &from, false}, {"until", &to, true}} {
		raw, _ := cmd.Flags().GetString(spec.flag)
		if strings.TrimSpace(raw) == "" {
			continue
		}
		when, err := time.Parse("2006-01-02", strings.TrimSpace(raw))
		if err != nil {
			return from, to, fmt.Errorf("--%s wants a date as YYYY-MM-DD, got %q", spec.flag, raw)
		}
		if spec.end {
			when = when.Add(24*time.Hour - time.Second)
		}
		*spec.into = when
	}
	return from, to, nil
}

func nonEmpty(s, fallback string) string {
	if strings.TrimSpace(s) == "" {
		return fallback
	}
	return s
}

func printJSON(v any) error {
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	return enc.Encode(v)
}

func init() {
	for _, c := range []*cobra.Command{spoolListCmd, spoolExportCmd} {
		c.Flags().String("user", "", "Only requests owned by this user")
		c.Flags().String("title", "", "Title pattern, * as wildcard")
		c.Flags().String("program", "", "Only requests whose third name part is this program")
		c.Flags().String("job", "", "Only requests written by this background job's steps")
		c.Flags().String("since", "", "Earliest creation date, YYYY-MM-DD")
		c.Flags().String("until", "", "Latest creation date, YYYY-MM-DD")
		c.Flags().Int("top", 50, "Maximum requests")
	}
	spoolListCmd.Flags().Bool("json", false, "Emit JSON")
	spoolReadCmd.Flags().Bool("json", false, "Emit JSON: request, TemSe header, lines with page and formats")
	spoolReadCmd.Flags().Bool("raw", false, "Write the stored bytes for a non-list document")
	spoolReadCmd.Flags().String("via", "auto", "adt, rfc, or auto (ADT, then RFC when the content is not in the database)")
	spoolExportCmd.Flags().String("out", "", "Directory to write into")
	spoolExportCmd.Flags().String("via", "adt", "adt, or rfc to also fetch content ADT cannot read")
	spoolCmd.AddCommand(spoolListCmd, spoolReadCmd, spoolExportCmd)
	rootCmd.AddCommand(spoolCmd)
}

// rfcClientFor opens an RFC connection for a command that mostly works over
// ADT and only sometimes needs the gateway. The caller closes it.
func rfcClientFor(cmd *cobra.Command) (*rfc.Client, error) {
	dest, err := rfcDestinationFor(cmd)
	if err != nil {
		return nil, err
	}
	c, err := saprfc.OpenWithTimeout(context.Background(), dest, 0)
	if err != nil {
		return nil, fmt.Errorf("RFC logon to %s:%d failed: %w", dest.Host, dest.Port, err)
	}
	return c, nil
}
