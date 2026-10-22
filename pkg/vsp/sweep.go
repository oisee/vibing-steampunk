package vsp

// `vsp sweep` calls everything the product advertises and reports what did not
// answer.
//
// It exists because ten capabilities were found dead in one week — advertised,
// registered, reachable, and never once correct — and every one was found by
// hand. A person looking is not a mechanism, and the eleventh would ship the
// same way. This is the mechanism.
//
// It is deliberately a sibling of `vsp compat` rather than part of it. compat
// asks the *system* what it supports; sweep asks *us* whether what we claim to
// support answers. They fail differently and a reader needs to know which one
// is talking.

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/oisee/vibing-steampunk/internal/mcp"
	"github.com/oisee/vibing-steampunk/pkg/adt"
	"github.com/oisee/vibing-steampunk/pkg/graph"
	"github.com/spf13/cobra"
)

func init() {
	sweepCmd.Flags().Bool("json", false, "Emit the report as JSON, for diffing or for a record")
	sweepCmd.Flags().Bool("reach-only", false, "Run only the offline pass: is every advertised capability registered and routed")
	sweepCmd.Flags().String("only", "", "Run probes whose id or capability contains this")
	sweepCmd.Flags().String("class", "CL_ABAP_TYPEDESCR", "Class to probe reads and structure with")
	sweepCmd.Flags().String("program", "", "Program to probe with (default: found automatically)")
	sweepCmd.Flags().String("package", "", "Package to probe with (default: found automatically)")
	sweepCmd.Flags().String("table", "T000", "Table to probe free SQL with")
	sweepCmd.Flags().String("referenced", "CL_ABAP_TYPEDESCR", "An object known to be referenced by other code — the input for every caller probe")
	sweepCmd.Flags().String("references", "CL_ABAP_TYPEDESCR", "An object known to reference other code — the input for every callee probe")
	sweepCmd.Flags().Duration("timeout", 45*time.Second, "Cap one probe; a capability that exceeds it is reported, not waited on")
	sweepCmd.Flags().Bool("strict", false, "Exit non-zero when there is any finding, for CI")
	rootCmd.AddCommand(sweepCmd)
}

var sweepCmd = &cobra.Command{
	Use:   "sweep",
	Short: "Call everything this tool advertises, and report what did not answer",
	Long: `Walk the advertised capability surface and say which parts of it work.

Two passes, because a capability fails in two different ways:

  reach   Is it registered and routed at all? Needs no system, runs in CI, and
          is the check that finds a tool whitelisted behind a registration
          function nobody calls.

  answer  Called against a live system with an input that has an answer, does
          it produce one? This is the pass that finds a feature which has been
          returning an empty list since the day it shipped.

The distinction that matters is between an empty answer that is true and an
empty answer that is a failure wearing a truthful face. Probes that could not
tell the difference on their own carry an oracle — an independent second route
to the same fact — and when the oracle says there are twelve and the capability
says none, the report says "dead" rather than "no results".

The sweep never writes. Every probe is a read.

  vsp sweep --reach-only          # offline; no system needed
  vsp -s dev sweep                # the full pass
  vsp -s dev sweep --only graph   # one area
  vsp -s dev sweep --json         # a record to keep or to diff
  vsp -s dev sweep --strict       # non-zero exit on any finding`,
	RunE: runSweep,
}

func runSweep(cmd *cobra.Command, args []string) error {
	asJSON, _ := cmd.Flags().GetBool("json")
	reachOnly, _ := cmd.Flags().GetBool("reach-only")
	strict, _ := cmd.Flags().GetBool("strict")
	only, _ := cmd.Flags().GetString("only")

	if reachOnly {
		report := &mcp.SweepReport{
			System:       "(no system)",
			Reach:        mcp.SweepReach(),
			ReachChecked: mcp.ReachChecked(),
			Build:        sweepBuild(),
		}
		if asJSON {
			return emitJSON(report)
		}
		fmt.Print(report.Text())
		return sweepExit(report, strict)
	}

	params, err := resolveSystemParams(cmd)
	if err != nil {
		return err
	}
	client, err := getClient(params)
	if err != nil {
		return err
	}

	targets := mcp.SweepTargets{}
	targets.Class, _ = cmd.Flags().GetString("class")
	targets.Program, _ = cmd.Flags().GetString("program")
	targets.Package, _ = cmd.Flags().GetString("package")
	targets.Table, _ = cmd.Flags().GetString("table")
	targets.Referenced, _ = cmd.Flags().GetString("referenced")
	targets.References, _ = cmd.Flags().GetString("references")
	missed := fillSweepTargets(cmd, client, &targets)

	// A probe skipped because nothing was found to run it against says nothing
	// about the capability, and a reader who takes it for a clean result has
	// been misled by the report rather than by the code.
	if note := adt.UnsearchedNote(missed, len(missed)+4, "probe target"); note != "" {
		fmt.Fprintln(os.Stderr, note)
	}

	srv := mcp.NewServerWithClient(sweepConfig(), client)
	fmt.Fprintf(os.Stderr, "sweeping %s...\n", params.Name)
	perProbe, _ := cmd.Flags().GetDuration("timeout")
	report := srv.RunSweep(cmd.Context(), params.Name, targets, mcp.SweepOptions{
		Only:     only,
		PerProbe: perProbe,
		Progress: func(p mcp.Probe) { fmt.Fprintf(os.Stderr, "  %-28s %s\n", p.ID, p.Capability) },
	})
	report.Missed = missed
	report.Build = sweepBuild()
	report.Targets = targets

	if asJSON {
		if err := emitJSON(report); err != nil {
			return err
		}
		return sweepExit(report, strict)
	}
	fmt.Print(report.Text())
	return sweepExit(report, strict)
}

// sweepExit turns findings into an exit code, but only when asked.
//
// The default is to exit zero with the findings printed, because a person
// running this by hand wants to read it. --strict is for the build, where a
// finding must stop something.
func sweepExit(report *mcp.SweepReport, strict bool) error {
	findings := report.Findings()
	if strict && len(findings) > 0 {
		return fmt.Errorf("%d capability finding(s); see the report above", len(findings))
	}
	return nil
}

// fillSweepTargets finds objects to probe with.
//
// This is most of the value of the sweep, and it is where one can quietly
// become useless. Ask for the callers of a class nobody calls and the empty
// answer is true, the probe passes, and a dead capability stays dead.
//
// The inverse cost is worse and was paid on the first run: a target chosen for
// convenience produced a "dead" verdict that was wrong. `CL_ABAP_TYPEDESCR`
// exists on every system, so it looked like a safe default for the callee
// probe — but WBCROSSGT holds no rows for its includes at all, so the parser
// found dependencies in its source, the tables honestly had none, and the sweep
// read the disagreement as a defect. Two routes that answer *different*
// questions are not an oracle. A sweep that invents findings is the thing it
// was built to catch.
//
// So the reference targets are resolved by asking the tables which object they
// have rows for, rather than by naming one and hoping.
func fillSweepTargets(cmd *cobra.Command, client *adt.Client, targets *mcp.SweepTargets) []adt.Unsearched {
	var missed []adt.Unsearched
	ctx := cmd.Context()

	find := func(objType string, patterns ...string) string {
		var lastErr error
		for _, pattern := range patterns {
			results, err := client.SearchObject(ctx, pattern, 100)
			if err != nil {
				lastErr = err
				continue
			}
			for _, r := range results {
				if strings.EqualFold(r.Type, objType) {
					return r.Name
				}
			}
			lastErr = nil
		}
		if lastErr != nil {
			missed = append(missed, adt.Unsearched{Object: objType, Reason: lastErr.Error()})
		}
		return ""
	}
	if targets.Program == "" {
		targets.Program = find("PROG/P", "Z*", "R*")
	}

	// A package with no source-bearing object cannot answer a boundary
	// question, and the handler correctly refuses to give it a verdict. Probing
	// with one tests nothing and reads as a failure.
	if targets.Package == "" {
		if pkg, err := packageWithSource(ctx, client); err != nil {
			missed = append(missed, adt.Unsearched{Object: "package with source", Reason: err.Error()})
		} else {
			targets.Package = pkg
		}
	}

	// The post-mortem types default to "latest" and refuse when the feed is
	// empty. That refusal is correct — a system with no dumps has no dumps —
	// so the probes are skipped rather than run, and the report says which.
	// Resolving here rather than inside a probe keeps "there was nothing to
	// ask about" distinct from "asking failed".
	if targets.Dump == "" {
		dumps, err := client.Dumps(ctx, adt.DumpFilter{Limit: 1})
		switch {
		case err != nil:
			missed = append(missed, adt.Unsearched{Object: "runtime dump", Reason: err.Error()})
		case len(dumps) == 0:
			missed = append(missed, adt.Unsearched{Object: "runtime dump",
				Reason: "the dump feed is empty on this system, which is a fact about the system rather than about the capability"})
		default:
			targets.Dump = dumps[0].ID
		}
	}
	// The CR types group transports by a landscape-specific attribute. Without
	// it configured they refuse, clearly and correctly — so the probe is skipped
	// rather than run, and the reason is the configuration rather than the code.
	if targets.CRAttribute == "" {
		attr := ""
		if params, perr := resolveSystemParams(cmd); perr == nil {
			attr = params.TransportAttribute
		}
		if attr != "" {
			targets.CRAttribute = attr
		} else {
			missed = append(missed, adt.Unsearched{Object: "CR attribute",
				Reason: "no transport_attribute is configured for this system, so the change-request types have nothing to group by"})
		}
	}

	if targets.Trace == "" {
		traces, err := client.ListTraces(ctx, &adt.TraceQueryOptions{MaxResults: 1})
		switch {
		case err != nil:
			missed = append(missed, adt.Unsearched{Object: "ABAP trace", Reason: err.Error()})
		case len(traces) == 0:
			missed = append(missed, adt.Unsearched{Object: "ABAP trace",
				Reason: "no trace has been recorded on this system; nothing to ask get_trace about"})
		default:
			targets.Trace = traces[0].ID
		}
	}

	// A data element the dictionary demonstrably holds English labels for.
	// Asking about one with none returns four empty strings, which is a true
	// answer and tells nobody whether the capability works.
	if targets.DataElement == "" {
		if name, err := dataElementWithLabels(ctx, client); err != nil {
			missed = append(missed, adt.Unsearched{Object: "data element with labels", Reason: err.Error()})
		} else {
			targets.DataElement = name
		}
	}

	// The same for a message class: T100 says which ones have English texts.
	if targets.MessageClass == "" {
		if name, err := messageClassWithTexts(ctx, client); err != nil {
			missed = append(missed, adt.Unsearched{Object: "message class with texts", Reason: err.Error()})
		} else {
			targets.MessageClass = name
		}
	}

	// A text pool is not in the source and most programs have none, so the
	// program used for reading source is the wrong one to ask.
	if targets.TextPoolProgram == "" {
		if name, err := programWithTextPool(ctx, client); err != nil {
			missed = append(missed, adt.Unsearched{Object: "program with a text pool", Reason: err.Error()})
		} else {
			targets.TextPoolProgram = name
		}
	}

	// An object with version history, and two of its versions. The URIs are
	// issued by the server and cannot be built by hand, which is why the two
	// capabilities that consume one had never been probed: there was no way to
	// write the input down in a static table.
	if targets.Versioned == "" {
		name, objType, err := objectWithVersionHistory(ctx, client)
		switch {
		case err != nil:
			missed = append(missed, adt.Unsearched{Object: "object with version history", Reason: err.Error()})
		default:
			targets.Versioned, targets.VersionedType = name, objType
			revisions, rerr := client.GetRevisions(ctx, objType, name, nil)
			switch {
			case rerr != nil:
				missed = append(missed, adt.Unsearched{Object: "a version URI", Reason: rerr.Error()})
			case len(revisions) < 2:
				// One version is enough to read and not enough to compare, and
				// saying so is better than probing compare with the same URI
				// twice and calling an empty diff an answer.
				missed = append(missed, adt.Unsearched{Object: "two version URIs",
					Reason: fmt.Sprintf("%s %s has %d version(s) in the feed; comparing needs two", objType, name, len(revisions))})
				if len(revisions) == 1 {
					targets.VersionURI = revisions[0].URI
				}
			default:
				targets.VersionURI = revisions[0].URI
				targets.VersionURI2 = revisions[1].URI
			}
		}
	}

	// An object the cross-reference tables demonstrably have rows for. Only for
	// such an object is an empty callee list impossible, which is the whole
	// premise of that probe.
	if !cmd.Flags().Changed("references") {
		if obj, objType, err := objectWithCrossReferences(ctx, client); err != nil {
			missed = append(missed, adt.Unsearched{Object: "object with cross-references", Reason: err.Error()})
			targets.References, targets.ReferencesType = "", ""
		} else if obj != "" {
			targets.References, targets.ReferencesType = obj, objType
		}
	}
	return missed
}

// packageWithSource returns a package that holds at least one class or program.
func packageWithSource(ctx context.Context, client *adt.Client) (string, error) {
	// Filtered server-side, so fifty rows are fifty candidates.
	//
	// The unfiltered query took whatever TADIR returned first and then skipped
	// local packages here. On one system the first fifty classes were all in
	// `$` packages, the loop found nothing, and four capabilities went
	// unverified on the release where verification matters most — for want of a
	// package that system has thousands of. A filter the client applies after
	// the fact is a filter the row limit can starve.
	res, err := client.GetTableContents(ctx, "TADIR", 50,
		"SELECT * FROM TADIR WHERE OBJECT = 'CLAS' AND DELFLAG = '' AND DEVCLASS NOT LIKE '$%'")
	if err != nil {
		return "", err
	}
	// rowStringOf, not a type assertion on any.
	//
	// This read `row["DEVCLASS"].(string)` with the ok discarded. A data-preview
	// value that arrives as anything but a plain string then leaves pkg empty,
	// the loop finds nothing, and the function reports "no package with a class
	// was found in TADIR" — a claim about the system, manufactured by a type
	// conversion. On 7.50 that is exactly what happened, and three probes were
	// skipped for want of a package the system has thousands of.
	//
	// The shape is the one this project keeps finding: a failure that produces
	// a plausible sentence rather than an error. It is the only such assertion
	// left in the tree, which is why it survived.
	for _, row := range res.Rows {
		if pkg := rowStringOf(row, "DEVCLASS"); pkg != "" {
			return pkg, nil
		}
	}

	// A local package is a worse probe target and a much better one than none:
	// a boundary question about $TMP is still a boundary question, and skipping
	// the probe answers nothing at all. Preference, not requirement.
	local, lerr := client.GetTableContents(ctx, "TADIR", 20,
		"SELECT * FROM TADIR WHERE OBJECT = 'CLAS' AND DELFLAG = ''")
	if lerr == nil {
		for _, row := range local.Rows {
			if pkg := rowStringOf(row, "DEVCLASS"); pkg != "" {
				return pkg, nil
			}
		}
	}
	return "", fmt.Errorf("TADIR named no package holding a class, transportable or local (%d rows, then %d)",
		len(res.Rows), rowCountOf(local))
}

// objectWithCrossReferences returns an object whose includes appear in
// WBCROSSGT — name **and type** — so that "this object references nothing" is
// known to be false before the question is put.
//
// The type is not decoration. The first version returned a name alone and every
// probe then asserted CLAS, which is right for a class pool and wrong for an
// ordinary include: on one system the first WBCROSSGT row happened to be a
// class and the sweep passed, on another it was a program include and the probe
// asked for it at /oo/classes/…, got a 404, and reported the capability
// **absent on that release**. A verdict about the release, produced by a
// mistake of ours, in the tool whose whole job is telling those apart.
//
// NormalizeInclude has returned the type all along and this function was not
// asking for it — the same silhouette as the include section and the edge line
// number, both of which were computed and discarded.
func objectWithCrossReferences(ctx context.Context, client *adt.Client) (name, objType string, err error) {
	res, err := client.GetTableContents(ctx, "WBCROSSGT", 50, "")
	if err != nil {
		return "", "", err
	}
	for _, row := range res.Rows {
		include := strings.TrimSpace(rowStringOf(row, "INCLUDE"))
		if include == "" {
			continue
		}
		_, t, n := graph.NormalizeInclude(include)
		// A target whose type could not be established is not returned at all.
		// Guessing one puts the probe at an address the object does not live
		// at, and the 404 that follows is indistinguishable from a release
		// difference.
		if t == "" || n == "" {
			continue
		}
		if repositoryNameFromInclude(include) == "" {
			continue // generated or internal — not a repository object
		}
		return n, t, nil
	}
	return "", "", fmt.Errorf("WBCROSSGT returned no include that names a repository object of a known type")
}

// rowStringOf reads a column from a data-preview row.
func rowStringOf(row map[string]interface{}, column string) string {
	v, ok := row[column]
	if !ok || v == nil {
		return ""
	}
	return strings.TrimSpace(fmt.Sprintf("%v", v))
}

// sweepConfig supplies the settings that shape registration, and nothing about
// the connection: the client is handed over already built, with this system's
// authentication and safety on it.
func sweepConfig() *mcp.Config {
	c := *cfg
	// Expert registers the widest surface, which is what a sweep should walk.
	// The universal router is reached directly whatever the mode, so this
	// affects only the reach pass's view of tool registration.
	c.Mode = "expert"
	return &c
}

// sweepBuild names the binary being exercised, for the report to carry.
func sweepBuild() string {
	return fmt.Sprintf("%s (commit %s, built %s)", Version, Commit, BuildDate)
}

// repositoryNameFromInclude recovers the object name from an include name, or
// returns empty when the include does not name one.
//
// Kept as a filter for generated and internal includes — %_CCRMB and the like —
// which NormalizeInclude will happily turn into a plausible-looking name. The
// type now comes from NormalizeInclude; this only answers "is this a repository
// object at all".
//
// A class include is the class name padded with '=' and a two-letter section,
// so cutting at the first '=' is the whole trick. An earlier version trimmed
// padding and section letters from the right, which ate real characters and
// handed the sweep "%_CCRMB" as an object to ask about; the handler refused it
// correctly and the sweep filed the refusal as a defect in the handler.
func repositoryNameFromInclude(include string) string {
	name := strings.TrimSpace(strings.Split(include, "=")[0])
	if len(name) < 4 {
		return ""
	}
	first := rune(name[0])
	if first != '/' && !(first >= 'A' && first <= 'Z') {
		return ""
	}
	for _, r := range name {
		ok := (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '_' || r == '/'
		if !ok {
			return ""
		}
	}
	return name
}

// rowCountOf is nil-safe, so an error path can still say how much it saw.
func rowCountOf(res *adt.TableContentsResult) int {
	if res == nil {
		return 0
	}
	return len(res.Rows)
}

// dataElementWithLabels returns a data element the dictionary holds English
// labels for.
//
// Read from DD04T rather than named as a constant. MANDT would work on every
// system anybody has ever run this against, and that is the argument that put a
// hardcoded package in the boundary probe and made it skip on the one landscape
// that mattered.
func dataElementWithLabels(ctx context.Context, client *adt.Client) (string, error) {
	res, err := client.GetTableContents(ctx, "DD04T", 20,
		"SELECT ROLLNAME FROM DD04T WHERE DDLANGUAGE = 'E' AND AS4LOCAL = 'A' AND DDTEXT <> ''")
	if err != nil {
		return "", err
	}
	for _, row := range res.Rows {
		if name := strings.TrimSpace(rowStringOf(row, "ROLLNAME")); name != "" {
			return name, nil
		}
	}
	return "", fmt.Errorf("DD04T returned no data element with an English label")
}

// messageClassWithTexts returns a message class that has English texts.
func messageClassWithTexts(ctx context.Context, client *adt.Client) (string, error) {
	res, err := client.GetTableContents(ctx, "T100", 20,
		"SELECT ARBGB FROM T100 WHERE SPRSL = 'E' AND TEXT <> ''")
	if err != nil {
		return "", err
	}
	for _, row := range res.Rows {
		if name := strings.TrimSpace(rowStringOf(row, "ARBGB")); name != "" {
			return name, nil
		}
	}
	return "", fmt.Errorf("T100 returned no message class with an English text")
}

// programWithTextPool returns a program that has a text pool.
//
// TEXTPOOL itself is not readable through the data preview — it answers "Cannot
// find 'TEXTPOOL'" — so the question is asked of the report source directory
// instead: a program with selection texts is a program with a SELECT-OPTIONS or
// PARAMETERS statement, and TRDIR does not record that. What it does record is
// which programs are reports at all, and a report without a text pool returns an
// empty one, so this is the weakest of the four resolutions and says so by
// preferring a program the system generated texts for.
func programWithTextPool(ctx context.Context, client *adt.Client) (string, error) {
	res, err := client.GetTableContents(ctx, "TRDIRT", 20,
		"SELECT NAME FROM TRDIRT WHERE SPRSL = 'E' AND TEXT <> ''")
	if err != nil {
		return "", err
	}
	for _, row := range res.Rows {
		if name := strings.TrimSpace(rowStringOf(row, "NAME")); name != "" {
			return name, nil
		}
	}
	return "", fmt.Errorf("TRDIRT returned no program with an English title")
}

// objectWithVersionHistory returns an object the version directory holds more
// than one version for, and its repository type.
//
// VRSD records versions per source unit, so a class appears under its include
// names — CL_FOO========CPUB — and the repository object has to be recovered
// from that. The same normalisation the cross-reference resolution uses, for
// the same reason: an include name asked about at the object's address is a 404
// that reads like a missing capability.
func objectWithVersionHistory(ctx context.Context, client *adt.Client) (name, objType string, err error) {
	res, err := client.GetTableContents(ctx, "VRSD", 200,
		"SELECT OBJTYPE, OBJNAME, VERSNO FROM VRSD WHERE VERSNO > '00000'")
	if err != nil {
		return "", "", err
	}
	for _, row := range res.Rows {
		raw := strings.TrimSpace(rowStringOf(row, "OBJNAME"))
		if raw == "" {
			continue
		}
		_, t, n := graph.NormalizeInclude(raw)
		if t == "" || n == "" {
			continue
		}
		if repositoryNameFromInclude(raw) == "" {
			continue
		}
		return n, t, nil
	}
	return "", "", fmt.Errorf("VRSD returned no versioned source unit that names a repository object of a known type")
}
