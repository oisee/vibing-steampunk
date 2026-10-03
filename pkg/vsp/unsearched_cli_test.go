package vsp

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/oisee/vibing-steampunk/pkg/adt"
	"github.com/oisee/vibing-steampunk/pkg/graph"
	"github.com/spf13/cobra"
)

// deadSAP is a system that answers every request with a failure. It stands in
// for the situations this sprint exists for — an expired session, a blocked
// freestyle query, an authorisation the user does not have — all of which used
// to arrive at the caller as an empty result.
func deadSAP(t *testing.T) *adt.Client {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "Not authorised", http.StatusForbidden)
	}))
	t.Cleanup(srv.Close)
	return adt.NewClient(srv.URL, "TESTUSER", "secret")
}

// captureStdout runs f and returns what it printed. The CLI's verdicts are
// print statements, and the verdict is the thing under test.
func captureStdout(t *testing.T, f func()) string {
	t.Helper()
	orig := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	os.Stdout = w
	done := make(chan string, 1)
	go func() {
		var b bytes.Buffer
		_, _ = io.Copy(&b, r)
		done <- b.String()
	}()
	f()
	_ = w.Close()
	os.Stdout = orig
	return <-done
}

// --- the health report -------------------------------------------------

// The worst shape of this defect: a report someone reads as a verdict, which
// omits what it could not check and therefore reports better health than the
// truth.
func TestHealthIsNotGoodWhenSomethingCouldNotBeChecked(t *testing.T) {
	signals := map[string]cliHealthSignal{
		"tests":      {Status: "PASS", Details: map[string]any{"classes": 3, "packages_scanned": 1}},
		"atc":        {Status: "CLEAN"},
		"boundaries": {Status: "CLEAN", Details: map[string]any{"objects_scanned": 4}},
		"staleness":  {Status: "ACTIVE"},
	}
	if got := summarizeCLIHealth(signals); got.Status != "GOOD" {
		t.Fatalf("a complete sweep with nothing wrong is GOOD, got %q (%s)", got.Status, got.Headline)
	}

	// Same signals, but one package could not be reached.
	sig := signals["tests"]
	sig.Unsearched = []adt.Unsearched{{Object: "$ZDEMO_SUB", Reason: "403 Not authorised"}}
	signals["tests"] = sig

	got := summarizeCLIHealth(signals)
	if got.Status == "GOOD" {
		t.Fatalf("nothing bad was found, but not everything was looked at — GOOD is the one answer this must not give")
	}
	if got.Status != "PARTIAL" {
		t.Fatalf("status = %q, want PARTIAL; headline %q", got.Status, got.Headline)
	}
	if !strings.Contains(got.Headline, "not a clean bill of health") {
		t.Fatalf("the headline has to say why: %q", got.Headline)
	}
}

// A gap does not soften a real finding. A failing test is still the headline.
func TestARealFindingStillOutranksAGap(t *testing.T) {
	signals := map[string]cliHealthSignal{
		"tests": {
			Status:     "FAIL",
			Details:    map[string]any{"classes": 3, "alerts": 2},
			Unsearched: []adt.Unsearched{{Object: "$ZDEMO_SUB", Reason: "timed out"}},
		},
	}
	if got := summarizeCLIHealth(signals); got.Status != "BAD" {
		t.Fatalf("status = %q, want BAD — the gap does not make a failing test less true", got.Status)
	}
}

// "NONE" over a package whose tests nobody could run is the sentence that
// invites "there are no tests here", and packages_scanned counted the packages
// the runner refused.
func TestPackageTestsDoNotCountWhatTheyCouldNotRun(t *testing.T) {
	sig, _ := collectPackageTestsWithDetails(context.Background(), deadSAP(t), "$ZDEMO")

	if sig.Status == "NONE" {
		t.Fatal(`nothing ran, so "NONE" is a claim about tests nobody looked for`)
	}
	if sig.Status != "UNKNOWN" {
		t.Fatalf("status = %q, want UNKNOWN", sig.Status)
	}
	if scanned, _ := sig.Details["packages_scanned"].(int); scanned != 0 {
		t.Fatalf("packages_scanned = %d, want 0 — the count is the lie, not only the missing note", scanned)
	}
	if len(sig.Unsearched) == 0 {
		t.Fatal("the packages that could not be reached have to be named")
	}
	if !strings.Contains(sig.Unsearched[0].Reason, "403") {
		t.Fatalf("the reason is carried verbatim, got %q", sig.Unsearched[0].Reason)
	}
}

// Staleness is the newest change anyone made. A revision list that never
// arrived can only make a package look older than it is, and an object with no
// history at all is a different thing entirely.
func TestPackageStalenessNeverClaimsAnAgeItCouldNotRead(t *testing.T) {
	sig := collectPackageStalenessCLI(context.Background(), deadSAP(t), "$ZDEMO")
	if sig.Status == "ACTIVE" || sig.Status == "AGING" || sig.Status == "STALE" {
		t.Fatalf("status = %q — no revision was read, so no age can be claimed", sig.Status)
	}
}

// The text renderer has to show the caveat next to the number it contradicts,
// or --format json is the only way to find out the answer is partial.
func TestHealthTextPrintsTheCaveat(t *testing.T) {
	result := &cliHealthResult{
		Scope:   cliHealthScope{Kind: "package", Package: "$ZDEMO"},
		Summary: cliHealthSummary{Status: "PARTIAL", Headline: "partial"},
		Signals: map[string]cliHealthSignal{
			"tests": {
				Status:     "NONE",
				Details:    map[string]any{"packages_scanned": 2},
				Unsearched: []adt.Unsearched{{Object: "$ZDEMO_SUB", Reason: "403 Not authorised"}},
			},
		},
	}
	out := captureStdout(t, func() { printCLIHealth(result, false) })
	if !strings.Contains(out, "1 of 3") {
		t.Fatalf("the caveat should count against what was reached:\n%s", out)
	}
	if !strings.Contains(out, "$ZDEMO_SUB") || !strings.Contains(out, "403") {
		t.Fatalf("the caveat should name what was missed and why:\n%s", out)
	}
}

// --- package resolution, which is what a boundary report is made of ----

// A blocked or failing TADIR query leaves every target object without a
// package, and AnalyzeCrossings drops an edge whose source package is empty.
// The report comes back clean because it could not look.
func TestUnresolvablePackagesAreReportedAsGaps(t *testing.T) {
	g := graph.New()
	g.AddNode(&graph.Node{ID: "CLAS:ZCL_DEMO_A", Name: "ZCL_DEMO_A", Type: "CLAS", Package: "$ZDEMO"})
	g.AddNode(&graph.Node{ID: "CLAS:ZCL_DEMO_B", Name: "ZCL_DEMO_B", Type: "CLAS"})

	missed := resolvePackagesCLI(context.Background(), deadSAP(t), g)
	if len(missed) != 1 {
		t.Fatalf("missed = %+v, want the one unresolved object", missed)
	}
	if missed[0].Object != "ZCL_DEMO_B" {
		t.Fatalf("missed[0] = %q, want ZCL_DEMO_B — the node that already had a package is not a gap", missed[0].Object)
	}
	if !strings.Contains(missed[0].Reason, "403") {
		t.Fatalf("reason = %q, want the failure as it came back", missed[0].Reason)
	}
}

// --block-free-sql is the sanctioned way to break exactly this lookup, and it
// is what a caller reaching for a locked-down system meets. Every target then
// goes unplaced, and the boundary report built from it used to come back clean.
func TestBlockedFreeSQLIsAGapNotACleanReport(t *testing.T) {
	client := adt.NewClient("http://sap.example.invalid", "TESTUSER", "secret", adt.WithBlockFreeSQL())

	g := graph.New()
	g.AddNode(&graph.Node{ID: "CLAS:ZCL_DEMO_A", Name: "ZCL_DEMO_A", Type: "CLAS", Package: "$ZDEMO"})
	g.AddNode(&graph.Node{ID: "CLAS:ZCL_DEMO_B", Name: "ZCL_DEMO_B", Type: "CLAS"})
	g.AddNode(&graph.Node{ID: "CLAS:ZCL_DEMO_C", Name: "ZCL_DEMO_C", Type: "CLAS"})

	missed := resolvePackagesCLI(context.Background(), client, g)
	if len(missed) != 2 {
		t.Fatalf("missed = %+v, want both unplaced objects", missed)
	}
	note := adt.UnsearchedNote(missed, 3, "object")
	if !strings.Contains(note, "2 of 3") || !strings.Contains(note, "not a complete answer") {
		t.Fatalf("the caveat has to contradict the count:\n%s", note)
	}
}

// --- the landscape -----------------------------------------------------

func landscapeCmdWithFile(t *testing.T, path string) *cobra.Command {
	t.Helper()
	c := &cobra.Command{}
	c.SetContext(context.Background())
	c.Flags().String("file", path, "")
	c.Flags().Bool("include", false, "")
	c.Flags().Bool("all", false, "")
	c.Flags().String("filter", "", "")
	c.Flags().Bool("probe", false, "")
	c.Flags().Bool("resolve", false, "")
	c.Flags().StringSlice("domain", []string{}, "")
	c.Flags().String("client", "", "")
	c.Flags().Bool("sso", true, "")
	c.Flags().Bool("write", false, "")
	return c
}

// The file that fails to open is very often the shared company landscape, and
// that is where the system being looked for lives. "No systems matched" over an
// unread file sends the reader to check their spelling.
func TestAnUnreadableLandscapeIsNotAnEmptyLandscape(t *testing.T) {
	path := filepath.Join(t.TempDir(), "SAPUILandscape.xml")
	if err := os.WriteFile(path, []byte("this is not xml at all"), 0o600); err != nil {
		t.Fatal(err)
	}

	systems, missed, err := loadLandscape(landscapeCmdWithFile(t, path))
	if err != nil {
		t.Fatalf("one unreadable source must not fail the whole walk: %v", err)
	}
	if len(systems) != 0 {
		t.Fatalf("systems = %+v, want none", systems)
	}
	if len(missed) != 1 {
		t.Fatalf("missed = %+v, want the file that would not parse", missed)
	}

	out := captureStdout(t, func() {
		if err := runLandscapeList(landscapeCmdWithFile(t, path), nil); err != nil {
			t.Errorf("runLandscapeList: %v", err)
		}
	})
	if strings.Contains(out, "No systems matched.") {
		t.Fatalf("nothing was read, so nothing can be said to have not matched:\n%s", out)
	}
	if !strings.Contains(out, "could not be read") {
		t.Fatalf("the reader has to be told why the list is empty:\n%s", out)
	}
}

// Importing from a landscape nobody could read must not end with a message
// about the filter.
func TestImportSaysWhenTheLandscapeWasNotReadable(t *testing.T) {
	path := filepath.Join(t.TempDir(), "SAPUILandscape.xml")
	if err := os.WriteFile(path, []byte("<<<broken"), 0o600); err != nil {
		t.Fatal(err)
	}
	err := runLandscapeImport(landscapeCmdWithFile(t, path), nil)
	if err == nil {
		t.Fatal("importing from an unreadable landscape is not a success")
	}
	if !strings.Contains(err.Error(), "could not be read") {
		t.Fatalf("error = %q, want it to name the unread file rather than blame the filter", err)
	}
}

// --- detect ------------------------------------------------------------

// A name that does not resolve produced the same empty scan as a firewalled
// host, and the advice printed underneath sent the reader hunting for a
// firewall that was never in the way.
func TestDetectSaysWhenTheHostWasNeverKnockedOn(t *testing.T) {
	result := &adt.PortScanResult{
		Host:       "nosuchsystem.invalid",
		Unsearched: []adt.Unsearched{{Object: "nosuchsystem.invalid", Reason: "the name does not resolve: no such host"}},
	}
	out := captureStdout(t, func() { printFindings(result, false) })
	if strings.Contains(out, "may be firewalled") {
		t.Fatalf("no port was reached, so the firewall advice is a guess about a scan that did not happen:\n%s", out)
	}
	if !strings.Contains(out, "does not resolve") {
		t.Fatalf("the reason has to survive to the terminal:\n%s", out)
	}
}

// A scan that did run keeps its old output exactly.
func TestDetectIsUnchangedWhenTheScanRan(t *testing.T) {
	result := &adt.PortScanResult{Host: "sap.example.com"}
	out := captureStdout(t, func() { printFindings(result, false) })
	if !strings.Contains(out, "Nothing answered on sap.example.com.") {
		t.Fatalf("a complete scan that found nothing still says so:\n%s", out)
	}
}

// --- trace -------------------------------------------------------------

// The --call goroutine cannot return an error to anyone, so its failures went
// to stderr and the command ended by saying nothing ran the object — an empty
// result standing in for a failure it had already seen.
func TestACallThatNeverWentOutChangesTheVerdict(t *testing.T) {
	var out callOutcome
	if out.note() != "" {
		t.Fatal("a call that went out adds nothing to the verdict")
	}
	out.fail(context.DeadlineExceeded)
	note := out.note()
	if !strings.Contains(note, "--call never reached it") {
		t.Fatalf("note = %q", note)
	}
	if !strings.Contains(note, context.DeadlineExceeded.Error()) {
		t.Fatalf("the reason is carried verbatim: %q", note)
	}
}

// A transport that holds nothing was reported SELF-CONSISTENT — "it carries
// everything it depends on", which is trivially true of nothing and reads as
// reassurance. A transport number that does not exist answered with it.
func TestAnEmptyTransportGetsItsOwnVerdict(t *testing.T) {
	empty := &graph.TransportBoundaryReport{
		Scope:       "TR-EXAMPLE",
		ObjectCount: 0,
		Summary:     graph.TransportBoundarySummary{SelfConsistent: true},
	}
	if got := transportBoundaryStatus(empty); got != "EMPTY" {
		t.Fatalf("nothing was analysed; the verdict should say so, got %q", got)
	}
	if note := transportBoundaryNote(empty); note == "" {
		t.Fatal("EMPTY without an explanation reads as a tool that failed silently")
	}
}

// The two real verdicts are untouched, and a clean report stays quiet — or the
// note becomes wallpaper and stops being read.
func TestRealVerdictsAreUnchangedAndQuiet(t *testing.T) {
	consistent := &graph.TransportBoundaryReport{
		ObjectCount: 12,
		Summary:     graph.TransportBoundarySummary{SelfConsistent: true},
	}
	if got := transportBoundaryStatus(consistent); got != "SELF-CONSISTENT" {
		t.Fatalf("got %q", got)
	}
	if note := transportBoundaryNote(consistent); note != "" {
		t.Fatalf("a complete report should carry no note, got %q", note)
	}

	incomplete := &graph.TransportBoundaryReport{
		ObjectCount: 12,
		Summary:     graph.TransportBoundarySummary{SelfConsistent: false, Missing: 3},
	}
	if got := transportBoundaryStatus(incomplete); got != "INCOMPLETE" {
		t.Fatalf("got %q", got)
	}
}
