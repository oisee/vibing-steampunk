package main

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"

	"github.com/oisee/open-rfc-go/rfc"
	"github.com/oisee/vibing-steampunk/pkg/adt"
	"github.com/oisee/vibing-steampunk/pkg/saprfc"
)

var jobsCmd = &cobra.Command{
	Use:   "jobs",
	Short: "Background jobs (SM37): list them with their steps and spools, read a job log",
	Long: `Background jobs over plain ADT: TBTCO for the job, TBTCP for its steps —
program, variant, user, the spool each step wrote. The job log is a TemSe
object that most systems keep in files, so it comes over RFC from XBP.

  vsp jobs list --since 2026-09-01 --status A          # what was cancelled
  vsp jobs list --name 'ZDEMO*' --user TESTUSER
  vsp jobs list --program ZDEMO_NIGHTLY_RUN            # jobs with a step running it
  vsp jobs log ZDEMO_NIGHTLY 22554500`,
}

var jobsListCmd = &cobra.Command{
	Use:   "list",
	Short: "List jobs, latest start first, with steps and spool numbers",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		params, err := resolveSystemParams(cmd)
		if err != nil {
			return err
		}
		client, err := getClient(params)
		if err != nil {
			return err
		}
		f := adt.JobFilter{}
		f.Name, _ = cmd.Flags().GetString("name")
		f.User, _ = cmd.Flags().GetString("user")
		f.Status, _ = cmd.Flags().GetString("status")
		f.Program, _ = cmd.Flags().GetString("program")
		f.Limit, _ = cmd.Flags().GetInt("top")
		if f.From, f.To, err = dateWindow(cmd); err != nil {
			return err
		}
		jobs, err := client.Jobs(context.Background(), f)
		if err != nil {
			return err
		}
		if asJSON, _ := cmd.Flags().GetBool("json"); asJSON {
			return printJSON(jobs)
		}
		if len(jobs) == 0 {
			fmt.Fprintln(os.Stderr, "no jobs match")
			return nil
		}
		for _, j := range jobs {
			when := "-"
			if !j.Started.IsZero() {
				when = j.Started.Format("2006-01-02 15:04:05")
			} else if !j.Scheduled.IsZero() {
				when = "sched " + j.Scheduled.Format("2006-01-02 15:04")
			}
			fmt.Printf("%-19s %-10s %-12s %s/%s", when, j.StatusText, j.User, j.Name, j.Count)
			if j.Duration != "" {
				fmt.Printf("  %s", j.Duration)
			}
			if j.Periodic {
				fmt.Print("  periodic")
			}
			fmt.Println()
			for _, s := range j.Steps {
				fmt.Printf("    step %d  %s", s.Step, nonEmpty(s.Program, s.External))
				if s.Variant != "" {
					fmt.Printf("  variant %s", s.Variant)
				}
				if s.User != "" && s.User != j.User {
					fmt.Printf("  as %s", s.User)
				}
				if s.Spool != 0 {
					fmt.Printf("  spool %d", s.Spool)
				}
				fmt.Println()
			}
		}
		fmt.Fprintf(os.Stderr, "\n%d jobs\n", len(jobs))
		return nil
	},
}

var jobsLogCmd = &cobra.Command{
	Use:   "log <JOBNAME> <JOBCOUNT>",
	Short: "Read a job's log over RFC (XBP)",
	Args:  cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		asJSON, _ := cmd.Flags().GetBool("json")
		return withRFC(cmd, func(ctx context.Context, c *rfc.Client) error {
			entries, err := saprfc.ReadJobLog(ctx, c, strings.ToUpper(args[0]), args[1])
			if err != nil {
				return err
			}
			if asJSON {
				return printJSON(entries)
			}
			for _, e := range entries {
				id := strings.TrimSpace(e.ID + " " + e.No)
				fmt.Printf("%s %s %-1s %-12s %s\n", e.Date, e.Time, e.Type, id, e.Text)
			}
			return nil
		})
	},
}

func init() {
	jobsListCmd.Flags().String("name", "", "Job name, * as wildcard")
	jobsListCmd.Flags().String("user", "", "Scheduling user")
	jobsListCmd.Flags().String("status", "", "Status letters: P scheduled, S released, Y ready, R active, F finished, A cancelled")
	jobsListCmd.Flags().String("program", "", "Only jobs with a step running this program")
	jobsListCmd.Flags().String("since", "", "Earliest scheduling date, YYYY-MM-DD")
	jobsListCmd.Flags().String("until", "", "Latest scheduling date, YYYY-MM-DD")
	jobsListCmd.Flags().Int("top", 50, "Maximum jobs")
	jobsListCmd.Flags().Bool("json", false, "Emit JSON")
	jobsLogCmd.Flags().Bool("json", false, "Emit JSON")
	jobsCmd.AddCommand(jobsListCmd, jobsLogCmd)
	rootCmd.AddCommand(jobsCmd)
}
