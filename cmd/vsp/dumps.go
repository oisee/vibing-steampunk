package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/oisee/vibing-steampunk/pkg/adt"
	"github.com/spf13/cobra"
)

var dumpsCmd = &cobra.Command{
	Use:   "dumps",
	Short: "Read runtime errors (ST22), group them, and see what was logged around one",
	Long: `Read ABAP runtime errors over plain ADT.

The feed carries the error type and the terminated program as structured
fields, so listing and grouping need no HTML parsing and no Z code.

  vsp dumps --since 2026-08-01                 # newest first
  vsp dumps --group                            # what keeps failing, not what failed once
  vsp dumps --program SAPLSBAL_DB
  vsp dumps --explain latest --tolerance 5m    # and what the application log said around it
  vsp dumps --similar latest                   # what else looks like this one, and how closely
  vsp dumps --impact latest                    # who else calls the code that failed

--explain ranks log entries by the argument for them, not by the clock. An
entry written by the program that dumped is connected structurally; one that is
merely nearby in time is a coincidence until something says otherwise.

--similar answers "is this new, and how often does it happen" on a ladder,
strongest rung first:

  1  same error, same program, same line          the same bug
  2  same error, same program                     the same bug or its siblings
  3  same error, same application component       a neighbourhood
  4  same error                                   a class of failure

Rungs 2 and 4 come out of the feed and cost nothing. Rungs 1 and 3 need the
failing line and the application component, which live only in the dump detail
— one fetch per candidate, bounded by --deep. Custom code is usually assigned
to no application component at all, so rung 3 mostly speaks about SAP standard
code; when a dump has no component, the rung is dropped rather than faked.

--impact asks the opposite direction and is not evidence of anything. A caller
that took part in this failure is already on the stack --explain prints; the
callers listed here did not run, which is precisely why they are worth knowing
about. It is the blast radius: who else reaches the broken code, and would.`,
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
		ctx := context.Background()

		filter := adt.DumpFilter{}
		filter.ErrorType, _ = cmd.Flags().GetString("error")
		filter.Program, _ = cmd.Flags().GetString("program")
		filter.User, _ = cmd.Flags().GetString("user")
		filter.Limit, _ = cmd.Flags().GetInt("top")
		if since, _ := cmd.Flags().GetString("since"); strings.TrimSpace(since) != "" {
			when, perr := time.Parse("2006-01-02", strings.TrimSpace(since))
			if perr != nil {
				return fmt.Errorf("--since wants a date as YYYY-MM-DD, got %q", since)
			}
			filter.From = when
		}

		dumps, err := client.Dumps(ctx, filter)
		if err != nil {
			return err
		}
		if len(dumps) == 0 {
			fmt.Fprintln(os.Stderr, "no runtime errors match")
			return nil
		}

		asJSON, _ := cmd.Flags().GetBool("json")

		if impact, _ := cmd.Flags().GetString("impact"); impact != "" {
			dump, found := adt.FindDump(dumps, impact)
			if !found {
				return fmt.Errorf("no dump in this range has an id containing %q; pass 'latest' or part of an id from the listing", impact)
			}
			return impactOfDump(ctx, client, cmd, dump, asJSON)
		}

		if explain, _ := cmd.Flags().GetString("explain"); explain != "" {
			return explainDump(ctx, client, cmd, dumps, explain, asJSON)
		}

		if similar, _ := cmd.Flags().GetString("similar"); similar != "" {
			return similarDumps(ctx, client, cmd, dumps, similar, asJSON)
		}

		if grouped, _ := cmd.Flags().GetBool("group"); grouped {
			groups := adt.GroupDumps(dumps)
			if asJSON {
				return emitJSON(groups)
			}
			fmt.Printf("%-5s %-34s %-30s %s\n", "COUNT", "RUNTIME ERROR", "PROGRAM", "LAST SEEN")
			fmt.Println(strings.Repeat("-", 100))
			for _, g := range groups {
				fmt.Printf("%-5d %-34s %-30s %s\n", g.Count, g.ErrorType, g.Program, stamp(g.Last))
			}
			fmt.Fprintf(os.Stderr, "\n%d distinct failures across %d dumps\n", len(groups), len(dumps))
			return nil
		}

		if asJSON {
			return emitJSON(dumps)
		}
		fmt.Printf("%-19s %-30s %-26s %s\n", "WHEN", "RUNTIME ERROR", "PROGRAM", "USER")
		fmt.Println(strings.Repeat("-", 100))
		for _, d := range dumps {
			fmt.Printf("%-19s %-30s %-26s %s\n", stamp(d.At), d.ErrorType, d.Program, d.User)
		}
		fmt.Fprintf(os.Stderr, "\n%d dumps\n", len(dumps))
		return nil
	},
}

// explainDump shows one dump and what the application log said around it.
func explainDump(ctx context.Context, client *adt.Client, cmd *cobra.Command, dumps []adt.Dump, which string, asJSON bool) error {
	dump, found := adt.FindDump(dumps, which)
	if !found {
		return fmt.Errorf("no dump in this range has an id containing %q; pass 'latest' or part of an id from the listing", which)
	}

	tolerance, _ := cmd.Flags().GetDuration("tolerance")
	matches, err := client.CorrelateDump(ctx, dump, tolerance, 20)
	if err != nil {
		return err
	}
	// One document carries the stack, the system fields, the source extract and
	// the chosen variables; read it once and show the causal detail, not just
	// the stack.
	detail, detailErr := client.DumpDetail(ctx, dump.ID)

	if asJSON {
		return emitJSON(struct {
			Dump    adt.Dump        `json:"dump"`
			Detail  *adt.DumpDetail `json:"detail,omitempty"`
			Matches []adt.LogMatch  `json:"matches"`
		}{dump, detail, matches})
	}

	fmt.Printf("%s  %s\n", stamp(dump.At), dump.ErrorType)
	fmt.Printf("  program %s, user %s\n", dump.Program, dump.User)
	if dump.Message != "" {
		fmt.Printf("  %s\n", dump.Message)
	}
	fmt.Println()

	switch {
	case errors.Is(detailErr, adt.ErrDumpDetailUnavailable):
		// Not a fault: the release has the feed and not the detail resource.
		fmt.Fprintf(os.Stderr, "%v\n\n", detailErr)
	case detailErr != nil:
		fmt.Fprintf(os.Stderr, "the dump detail could not be read: %v\n\n", detailErr)
	case detail != nil:
		printDumpTermination(detail)
		if len(detail.Stack) > 0 {
			fmt.Println("Call stack at the failure:")
			for _, f := range detail.Stack {
				where := f.Program
				if f.Include != "" && f.Include != f.Program {
					where += "/" + f.Include
				}
				fmt.Printf("  %3d %-12s %s:%d\n", f.Position, f.Type, where, f.Line)
				if f.Name != "" {
					fmt.Printf("      %s\n", f.Name)
				}
			}
			fmt.Println()
		}
	}

	if len(matches) == 0 {
		fmt.Println("Nothing was written to the application log in that window.")
		return nil
	}
	fmt.Println("Application log entries around it, best argument first:")
	fmt.Println()
	for _, m := range matches {
		fmt.Printf("  %s  %s/%s\n", stamp(m.Entry.At), m.Entry.Object, m.Entry.SubObject)
		fmt.Printf("      %s\n", m.Why)
	}
	fmt.Fprintln(os.Stderr, "\nRanked by the argument for each, not by nearness. A match is a candidate, not a cause.")
	if client.CalleesUnavailable(ctx) {
		// The rung itself works now — it reads the cross-reference tables.
		// What can still be missing is permission to read them, from this
		// server's own --block-free-sql or from the user's authorisations, and
		// that is worth saying because the ranking looks complete without it.
		fmt.Fprintln(os.Stderr, "One rung is missing: the cross-reference tables cannot be read here "+
			"(free SQL blocked, or no authorisation for CROSS), so \"written by something a stack frame calls\" was never asked.")
	}
	return nil
}

// printDumpTermination shows the causal core the enriched detail carries: the
// message the dump raised (SY-MSGID/MSGNO and its variables), the key system
// fields for context, and the source line it died on with a line either side.
func printDumpTermination(d *adt.DumpDetail) {
	printed := false
	if sf := d.SystemFields; len(sf) > 0 {
		if id, no := sf["SY-MSGID"], sf["SY-MSGNO"]; id != "" || no != "" {
			line := fmt.Sprintf("  message %s %s", id, no)
			if ty := sf["SY-MSGTY"]; ty != "" {
				line += " (type " + ty + ")"
			}
			var vs []string
			for _, k := range []string{"SY-MSGV1", "SY-MSGV2", "SY-MSGV3", "SY-MSGV4"} {
				if v := sf[k]; v != "" {
					vs = append(vs, v)
				}
			}
			if len(vs) > 0 {
				line += "  with " + strings.Join(vs, ", ")
			}
			fmt.Println(line)
			printed = true
		}
		var context []string
		for _, k := range []string{"SY-SUBRC", "SY-FDPOS", "SY-PFKEY", "SY-TCODE", "SY-TITLE"} {
			if v := sf[k]; v != "" {
				context = append(context, strings.TrimPrefix(k, "SY-")+"="+v)
			}
		}
		if len(context) > 0 {
			fmt.Printf("  %s\n", strings.Join(context, "  "))
			printed = true
		}
	}
	for i, s := range d.Source {
		if !s.Failed {
			continue
		}
		fmt.Println("  source:")
		lo, hi := i-1, i+1
		if lo < 0 {
			lo = 0
		}
		if hi >= len(d.Source) {
			hi = len(d.Source) - 1
		}
		for j := lo; j <= hi; j++ {
			mark := "    "
			if d.Source[j].Failed {
				mark = "  > "
			}
			fmt.Printf("  %s%5d  %s\n", mark, d.Source[j].Line, strings.TrimSpace(d.Source[j].Text))
		}
		printed = true
		break
	}
	if printed {
		fmt.Println()
	}
}

// similarDumps answers "what else looks like this dump", on a ladder.
//
// Two of the four rungs — the same failing line, the same application
// component — need the dump detail, and that is one HTTP round trip per
// candidate. The other two need nothing but the feed that has already been
// read. So the expensive half is budgeted and the cheap half always runs,
// which means a system that refuses details still answers, just with a
// shorter ladder.
func similarDumps(ctx context.Context, client *adt.Client, cmd *cobra.Command, dumps []adt.Dump, which string, asJSON bool) error {
	subject, found := adt.FindDump(dumps, which)
	if !found {
		return fmt.Errorf("no dump in this range has an id containing %q; pass 'latest' or part of an id from the listing", which)
	}

	budget, _ := cmd.Flags().GetInt("deep")
	details := map[string]*adt.DumpDetail{}
	var detailNote string

	if budget > 0 {
		// The subject's own detail is read first and outside the budget: with
		// no line and no component for the dump being asked about, no
		// candidate can reach rung 1 or rung 3 however many are read.
		detail, err := client.DumpDetail(ctx, subject.ID)
		switch {
		case errors.Is(err, adt.ErrDumpDetailUnavailable):
			// A release that has the feed and not the detail resource. Said
			// once, plainly, and then the two rungs that need no detail carry
			// the answer.
			detailNote = err.Error() + "\nRungs 1 and 3 need it, so this ranks on rungs 2 and 4 only."
			budget = 0
		case err != nil:
			detailNote = fmt.Sprintf("the detail of the dump itself could not be read (%v), so rungs 1 and 3 are unavailable", err)
			budget = 0
		default:
			details[subject.ID] = detail
		}
	}

	read := 0
	for _, candidate := range adt.DeepenOrder(subject, dumps) {
		if read >= budget {
			break
		}
		read++
		detail, err := client.DumpDetail(ctx, candidate.ID)
		if err != nil {
			// One unreadable candidate is not a reason to abandon the search;
			// it simply cannot climb past rung 2. The budget is still spent,
			// because a system answering slowly with errors is the case where
			// an unbounded retry hurts most.
			continue
		}
		details[candidate.ID] = detail
	}

	signatures := make([]adt.DumpSignature, 0, len(dumps))
	for _, d := range dumps {
		signatures = append(signatures, adt.SignatureOf(d, details[d.ID]))
	}
	matches := adt.RankSimilarDumps(adt.SignatureOf(subject, details[subject.ID]), signatures)
	summary := adt.SummarizeSimilar(matches)

	if asJSON {
		return emitJSON(struct {
			Dump    adt.Dump          `json:"dump"`
			Detail  *adt.DumpDetail   `json:"detail,omitempty"`
			Rungs   []adt.RungSummary `json:"rungs"`
			Matches []adt.DumpMatch   `json:"matches"`
		}{subject, details[subject.ID], summary, matches})
	}

	fmt.Printf("%s  %s\n", stamp(subject.At), subject.ErrorType)
	fmt.Printf("  program %s, user %s\n", subject.Program, subject.User)
	if detail := details[subject.ID]; detail != nil {
		if detail.Line > 0 {
			fmt.Printf("  terminated at %s line %d", detail.Include, detail.Line)
			if detail.Procedure != "" {
				fmt.Printf(", in %s", detail.Procedure)
			}
			fmt.Println()
		} else {
			fmt.Println("  no failing line is recorded for this dump, so rung 1 cannot apply to it")
		}
		// The component belongs to the program in the dump header, and that is
		// not always the program in the feed — on a RAISE_EXCEPTION the feed
		// names the standard class that raised and the header names the custom
		// caller. Printing which program the component describes is the
		// difference between a useful field and a puzzling one, because a
		// standard SAP class showing "no component" otherwise looks like a bug
		// in this tool.
		filedUnder := ""
		if detail.Program != "" && !strings.EqualFold(detail.Program, subject.Program) {
			filedUnder = fmt.Sprintf(" (filed under %s, not %s)", detail.Program, subject.Program)
		}
		if detail.Component != "" {
			fmt.Printf("  application component %s%s\n", detail.Component, filedUnder)
		} else {
			fmt.Printf("  no application component is assigned%s, so rung 3 cannot apply to it\n", filedUnder)
		}
		if detail.Exception != "" {
			fmt.Printf("  exception %s\n", detail.Exception)
		}
	}
	if detailNote != "" {
		fmt.Fprintf(os.Stderr, "\n%s\n", detailNote)
	}
	fmt.Println()

	if len(matches) == 0 {
		fmt.Printf("Nothing else in this range is even the same runtime error. %s has happened once here.\n", subject.ErrorType)
		return nil
	}

	fmt.Println("How much else looks like it:")
	for _, s := range summary {
		fmt.Printf("  rung %d  %-3d  %s\n", s.Rung, s.Count, s.Label)
		fmt.Printf("            %s to %s, %s\n", stamp(s.First), stamp(s.Last), phraseUsers(s.Users))
	}
	fmt.Println()

	fmt.Println("Closest first:")
	// The reason is printed when it changes, not on every row. A run of forty
	// identical sentences is how a reason stops being read, and the reason is
	// the only part of this that a person can argue with.
	previous := ""
	for _, m := range matches {
		fmt.Printf("  %s  rung %d  %s  %s\n", stamp(m.Dump.At), m.Rung, m.Dump.Program, m.Dump.User)
		if m.Why != previous {
			fmt.Printf("      %s\n", m.Why)
			previous = m.Why
		}
	}
	fmt.Fprintln(os.Stderr, "\nA rung is an argument, not a verdict. Rung 4 is the same class of failure; it is not the same bug.")
	return nil
}

// phraseUsers turns the user set into the half of "always one user" that the
// design asks for. One user across many dumps is a lead; a dozen users is a
// different investigation, and the count is what separates them.
func phraseUsers(users []string) string {
	switch len(users) {
	case 0:
		return "no user recorded"
	case 1:
		return "always " + users[0]
	default:
		return fmt.Sprintf("%d users: %s", len(users), strings.Join(users, ", "))
	}
}

// impactOfDump answers the question --explain does not: not what caused this
// failure, but who else runs into it.
//
// The two are separate flags on purpose. --explain ranks candidates for a
// cause, and every row it prints is arguable; this prints static facts about
// the repository, and none of them are. Merging the two lists would let the
// confidence of the second leak into how the first is read.
func impactOfDump(ctx context.Context, client *adt.Client, cmd *cobra.Command, dump adt.Dump, asJSON bool) error {
	frames, _ := cmd.Flags().GetInt("impact-frames")
	top, _ := cmd.Flags().GetInt("impact-top")

	result, err := client.DumpImpact(ctx, dump, adt.DumpImpactOptions{MaxUnits: frames, Limit: top})
	if err != nil {
		return err
	}
	if asJSON {
		return emitJSON(result)
	}

	fmt.Printf("%s  %s\n", stamp(dump.At), dump.ErrorType)
	fmt.Printf("  program %s, user %s\n", dump.Program, dump.User)
	if dump.Message != "" {
		fmt.Printf("  %s\n", dump.Message)
	}
	fmt.Println()

	if result.StackUnavailable {
		fmt.Fprintln(os.Stderr, "the call stack could not be read, so only the dump's own program was asked about")
		fmt.Fprintln(os.Stderr)
	}

	fmt.Println("Asked of:")
	for _, u := range result.Units {
		if u.Err != "" {
			fmt.Printf("  %-4s %-34s no where-used list: %s\n", u.Type, u.Object, u.Err)
			continue
		}
		if u.Note != "" {
			fmt.Printf("  %-4s %-34s not asked - %s\n", u.Type, u.Object, u.Note)
			continue
		}
		where := ""
		if u.Frame != nil {
			where = fmt.Sprintf("   frame %d, line %d", u.Frame.Position, u.Frame.Line)
		}
		fmt.Printf("  %-4s %-34s %4d direct callers%s\n", u.Type, u.Object, u.Total, where)
	}
	fmt.Println()

	switch {
	case !result.Answerable():
		// An empty answer and an unaskable question look the same in the
		// numbers and mean opposite things.
		fmt.Println("No unit of this dump has a where-used list that can answer. This is not a finding of zero callers.")
	case len(result.Exposed) == 0:
		fmt.Println("Nothing else calls this code. It is reachable only by the path the dump took.")
	default:
		fmt.Printf("Exposed - reaches the failing code by another path (%d):\n\n", len(result.Exposed))
		for _, e := range result.Exposed {
			marker := " "
			if e.IsTest {
				marker = "t"
			}
			fmt.Printf("  %s %-34s %-9s %-20s via %s\n", marker, e.Name, e.Type, e.Package, e.Via)
			if e.Component != "" {
				fmt.Printf("      in %s\n", e.Component)
			}
		}
	}

	if len(result.OnPath) > 0 {
		fmt.Println()
		fmt.Printf("On the dump's own stack, so not additional exposure (%d): %s\n",
			len(result.OnPath), strings.Join(callerNamesOf(result.OnPath), ", "))
	}

	fmt.Fprintln(os.Stderr, "\nWho can reach the bug, not who caused it. Object level: the where-used list resolves a method to its class, so a caller here reaches the class and not necessarily the failing method.")
	return nil
}

func callerNamesOf(callers []adt.ExposedCaller) []string {
	names := make([]string, 0, len(callers))
	for _, c := range callers {
		names = append(names, c.Name)
	}
	return names
}

func stamp(t time.Time) string {
	if t.IsZero() {
		return "-"
	}
	return t.Local().Format("2006-01-02 15:04:05")
}

func init() {
	dumpsCmd.Flags().String("since", "", "Earliest date, YYYY-MM-DD")
	dumpsCmd.Flags().String("error", "", "Only this runtime error type")
	dumpsCmd.Flags().String("program", "", "Only this terminated program")
	dumpsCmd.Flags().String("user", "", "Only this user")
	dumpsCmd.Flags().Int("top", 100, "Maximum dumps to read")
	dumpsCmd.Flags().Bool("group", false, "Group by what failed rather than when")
	dumpsCmd.Flags().String("explain", "", "Show one dump ('latest' or part of an id) with the log around it")
	dumpsCmd.Flags().String("similar", "", "Rank what else looks like one dump ('latest' or part of an id)")
	dumpsCmd.Flags().Int("deep", 25, "For --similar, how many candidate dumps to read in detail; 0 stays on the feed")
	dumpsCmd.Flags().String("impact", "", "Show who else calls the code that failed in one dump ('latest' or part of an id)")
	dumpsCmd.Flags().Int("impact-frames", 3, "How many units to walk outward from the failing statement for --impact")
	dumpsCmd.Flags().Int("impact-top", 25, "Maximum callers to list per unit for --impact")
	dumpsCmd.Flags().Duration("tolerance", 5*time.Minute, "Window on each side of the dump for --explain")
	dumpsCmd.Flags().Bool("json", false, "Emit JSON")
	rootCmd.AddCommand(dumpsCmd)
}
