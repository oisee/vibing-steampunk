package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/oisee/vibing-steampunk/pkg/adt"
	"github.com/spf13/cobra"
)

var appLogCmd = &cobra.Command{
	Use:   "applog",
	Short: "Read the application log (SLG1) — who logged what, and from which program",
	Long: `Read application log headers over plain ADT.

SAP's own way in is the BAL_* function group, which cannot be called remotely by
any transport. The header table is an ordinary table, so this reads it with free
SQL instead — no RFC, no gateway, no Z code.

The headers are enough to answer which program logged what, for which log
object, and when — the part that connects a log to a dump. The messages live
in BALDAT as a compressed data cluster; --messages reads that with the same
free SQL and decodes it here, class and number and variables, with the text
from T100 in the system's language.

  vsp applog --program ZCL_ORDER_POST --top 20
  vsp applog --user TESTUSER --since 2026-08-01
  vsp applog --object ZDEMO_LOG --json
  vsp applog --object ZDEMO_LOG --since 2026-09-01 --messages`,
	Args: cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		params, err := resolveSystemParams(cmd)
		if err != nil {
			return err
		}
		client, err := getClient(params)
		if err != nil {
			return err
		}

		filter := adt.AppLogFilter{}
		filter.Program, _ = cmd.Flags().GetString("program")
		filter.User, _ = cmd.Flags().GetString("user")
		filter.Object, _ = cmd.Flags().GetString("object")
		filter.SubObject, _ = cmd.Flags().GetString("subobject")
		filter.Limit, _ = cmd.Flags().GetInt("top")

		for _, spec := range []struct {
			flag string
			into *time.Time
		}{{"since", &filter.From}, {"until", &filter.To}} {
			raw, _ := cmd.Flags().GetString(spec.flag)
			if strings.TrimSpace(raw) == "" {
				continue
			}
			when, perr := time.Parse("2006-01-02", strings.TrimSpace(raw))
			if perr != nil {
				return fmt.Errorf("--%s wants a date as YYYY-MM-DD, got %q", spec.flag, raw)
			}
			*spec.into = when
		}

		ctx := context.Background()
		entries, err := client.ApplicationLog(ctx, filter)
		if err != nil {
			return err
		}
		withMessages, _ := cmd.Flags().GetBool("messages")
		if withMessages {
			if err := client.AttachAppLogMessages(ctx, params.Language, entries); err != nil {
				return err
			}
		}

		if asJSON, _ := cmd.Flags().GetBool("json"); asJSON {
			out, merr := json.MarshalIndent(entries, "", "  ")
			if merr != nil {
				return merr
			}
			fmt.Println(string(out))
			return nil
		}

		if len(entries) == 0 {
			fmt.Fprintln(os.Stderr, "no log entries match")
			return nil
		}
		if !withMessages {
			fmt.Printf("%-19s %-20s %-20s %-14s %s\n", "WHEN", "OBJECT", "SUBOBJECT", "USER", "PROGRAM")
			fmt.Println(strings.Repeat("-", 100))
		}
		messages := 0
		for _, e := range entries {
			when := "-"
			if !e.At.IsZero() {
				when = e.At.Format("2006-01-02 15:04:05")
			}
			if !withMessages {
				fmt.Printf("%-19s %-20s %-20s %-14s %s\n", when, e.Object, e.SubObject, e.User, e.Program)
				continue
			}
			fmt.Printf("%s  %s/%s  log %s  %s  %s", when, e.Object, e.SubObject, strings.TrimLeft(e.LogNumber, "0"), e.User, e.Program)
			if e.External != "" {
				fmt.Printf("  ext=%s", e.External)
			}
			fmt.Println()
			if len(e.Messages) == 0 {
				fmt.Printf("    (no messages stored; header counts %d)\n", e.MessageCount)
				continue
			}
			for _, m := range e.Messages {
				messages++
				printAppLogMessage(m)
			}
		}
		if withMessages {
			fmt.Fprintf(os.Stderr, "\n%d entries, %d messages\n", len(entries), messages)
		} else {
			fmt.Fprintf(os.Stderr, "\n%d entries\n", len(entries))
		}
		return nil
	},
}

func init() {
	appLogCmd.Flags().String("program", "", "Only entries written by this program (ALPROG)")
	appLogCmd.Flags().String("user", "", "Only entries logged by this user")
	appLogCmd.Flags().String("object", "", "Log object (SLG0)")
	appLogCmd.Flags().String("subobject", "", "Log subobject")
	appLogCmd.Flags().String("since", "", "Earliest date, YYYY-MM-DD")
	appLogCmd.Flags().String("until", "", "Latest date, YYYY-MM-DD")
	appLogCmd.Flags().Int("top", 100, "Maximum entries to read")
	appLogCmd.Flags().Bool("messages", false, "Read the messages too (BALDAT, decoded here)")
	appLogCmd.Flags().Bool("json", false, "Emit JSON")
	rootCmd.AddCommand(appLogCmd)
}

// printAppLogMessage prints one message the way SLG1 shows it: type, class
// and number, then the text or, when T100 has none, the variables.
func printAppLogMessage(m adt.AppLogMessage) {
	indent := "    "
	if level := strings.TrimSpace(m.DetailLevel); level != "" && level != "1" {
		if n, err := strconv.Atoi(level); err == nil && n > 1 {
			indent += strings.Repeat("  ", n-1)
		}
	}
	text := m.Text
	if text == "" {
		text = strings.TrimSpace(strings.Join([]string{m.V1, m.V2, m.V3, m.V4}, " "))
	}
	fmt.Printf("%s%s %s %-20s %s  %s\n", indent, m.Number, m.Type, m.ID+" "+m.No, m.Timestamp, text)
	if m.Context != nil && (m.Context.Table != "" || m.Context.Value != "") {
		fmt.Printf("%s       context %s: %s\n", indent, m.Context.Table, m.Context.Value)
	}
	for _, p := range m.Params {
		fmt.Printf("%s       %s = %s\n", indent, p.Name, p.Value)
	}
}
