package vsp

import (
	"context"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/oisee/vibing-steampunk/internal/fakesap"
	"github.com/oisee/vibing-steampunk/pkg/graph"
	"github.com/spf13/cobra"
)

// The golden files under testdata/golden/adtsource pin what the commands built
// on the shared ADT/SQL source (pkg/graph/adtsource) print against a scripted
// SAP. They were captured before that package existed, so a diff here is a
// change in what a user sees, not a refactoring detail.
//
//	go test ./cmd/vsp -run TestGoldenADTSource -update-adtsource-golden

var updateADTSourceGolden = flag.Bool("update-adtsource-golden", false, "rewrite testdata/golden/adtsource")

// goldenADTSourceDir is fixed before any test moves the working directory.
var goldenADTSourceDir = func() string {
	wd, err := os.Getwd()
	if err != nil {
		panic(err)
	}
	return filepath.Join(wd, "testdata", "golden", "adtsource")
}()

// goldenCase is one command run against one world.
type goldenCase struct {
	name  string
	world func() fakesap.World
	cmd   *cobra.Command
	run   func(*cobra.Command, []string) error
	args  []string
	flags map[string]string
}

// withFlags sets a command's flags for one run and puts them back afterwards.
func withFlags(t *testing.T, cmd *cobra.Command, flags map[string]string) {
	t.Helper()
	for name, value := range flags {
		f := cmd.Flags().Lookup(name)
		if f == nil {
			t.Fatalf("%s has no flag --%s", cmd.Name(), name)
		}
		saved, changed := f.Value.String(), f.Changed
		if err := cmd.Flags().Set(name, value); err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() {
			_ = cmd.Flags().Set(name, saved)
			f.Changed = changed
		})
	}
}

func goldenCompare(t *testing.T, name, got string) {
	t.Helper()
	// Absolute, because the run under test has moved the working directory.
	path := filepath.Join(goldenADTSourceDir, name+".golden")
	if *updateADTSourceGolden {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(got), 0o644); err != nil {
			t.Fatal(err)
		}
		return
	}
	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("%v (run with -update-adtsource-golden to create it)", err)
	}
	if string(want) != got {
		t.Errorf("%s changed.\n--- want\n%s\n--- got\n%s", name, want, got)
	}
}

func cliGoldenCases() []goldenCase {
	gold, narrow := fakesap.Gold, fakesap.Narrow
	return []goldenCase{
		{name: "boundaries_gold_text", world: gold, cmd: boundariesCmd, run: runBoundaries, args: []string{"$ZGOLD"}},
		{name: "boundaries_gold_json", world: gold, cmd: boundariesCmd, run: runBoundaries, args: []string{"$ZGOLD"}, flags: map[string]string{"format": "json"}},
		{name: "boundaries_gold_md", world: gold, cmd: boundariesCmd, run: runBoundaries, args: []string{"$ZGOLD"}, flags: map[string]string{"format": "md"}},
		{name: "boundaries_narrow_text", world: narrow, cmd: boundariesCmd, run: runBoundaries, args: []string{"$ZNARROW"}},
		{name: "boundaries_narrow_tadir_down_text", world: fakesap.NarrowTADIRDown, cmd: boundariesCmd, run: runBoundaries, args: []string{"$ZNARROW"}},
		{name: "boundaries_narrow_tadir_down_json", world: fakesap.NarrowTADIRDown, cmd: boundariesCmd, run: runBoundaries, args: []string{"$ZNARROW"}, flags: map[string]string{"format": "json"}},
		{name: "boundaries_narrow_tfdir_down_text", world: fakesap.NarrowTFDIRDown, cmd: boundariesCmd, run: runBoundaries, args: []string{"$ZNARROW"}},
		{name: "boundaries_narrow_fugr_down_text", world: fakesap.NarrowFUGRDown, cmd: boundariesCmd, run: runBoundaries, args: []string{"$ZNARROW"}},
		{name: "boundaries_narrow_all_down_json", world: fakesap.NarrowAllDown, cmd: boundariesCmd, run: runBoundaries, args: []string{"$ZNARROW"}, flags: map[string]string{"format": "json"}},

		{name: "health_gold_package_text", world: gold, cmd: healthCmd, run: runHealth, flags: map[string]string{"package": "$ZGOLD"}},
		{name: "health_gold_package_details", world: gold, cmd: healthCmd, run: runHealth, flags: map[string]string{"package": "$ZGOLD", "details": "true"}},
		{name: "health_gold_package_json", world: gold, cmd: healthCmd, run: runHealth, flags: map[string]string{"package": "$ZGOLD", "format": "json"}},
		{name: "health_gold_package_md", world: gold, cmd: healthCmd, run: runHealth, flags: map[string]string{"package": "$ZGOLD", "format": "md"}},
		{name: "health_gold_package_fast_json", world: gold, cmd: healthCmd, run: runHealth, flags: map[string]string{"package": "$ZGOLD", "fast": "true", "format": "json"}},
		{name: "health_gold_object_json", world: gold, cmd: healthCmd, run: runHealth, args: []string{"CLAS", "ZCL_GOLD_A"}, flags: map[string]string{"format": "json"}},
		{name: "health_gold_object_text", world: gold, cmd: healthCmd, run: runHealth, args: []string{"CLAS", "ZCL_GOLD_A"}},
		{name: "health_narrow_package_tadir_down_json", world: fakesap.NarrowTADIRDown, cmd: healthCmd, run: runHealth, flags: map[string]string{"package": "$ZNARROW", "format": "json"}},
		{name: "health_narrow_object_tadir_down_json", world: fakesap.NarrowTADIRDown, cmd: healthCmd, run: runHealth, args: []string{"CLAS", "ZCL_NARROW"}, flags: map[string]string{"format": "json"}},
		{name: "health_narrow_object_tfdir_down_json", world: fakesap.NarrowTFDIRDown, cmd: healthCmd, run: runHealth, args: []string{"CLAS", "ZCL_NARROW"}, flags: map[string]string{"format": "json"}},
		{name: "health_narrow_object_fugr_down_json", world: fakesap.NarrowFUGRDown, cmd: healthCmd, run: runHealth, args: []string{"CLAS", "ZCL_NARROW"}, flags: map[string]string{"format": "json"}},
		{name: "health_narrow_object_all_down_text", world: fakesap.NarrowAllDown, cmd: healthCmd, run: runHealth, args: []string{"CLAS", "ZCL_NARROW"}},
		{name: "health_narrow_package_all_down_text", world: fakesap.NarrowAllDown, cmd: healthCmd, run: runHealth, flags: map[string]string{"package": "$ZNARROW"}},

		{name: "where_used_config_gold_text", world: gold, cmd: graphWhereUsedConfigCmd, run: runGraphWhereUsedConfig, args: []string{"ZGOLD_VAR"}},
		{name: "where_used_config_gold_json", world: gold, cmd: graphWhereUsedConfigCmd, run: runGraphWhereUsedConfig, args: []string{"ZGOLD_VAR"}, flags: map[string]string{"format": "json"}},
		{name: "where_used_config_wbcrossgt_down_text", world: func() fakesap.World { return fakesap.CrossDown(false) }, cmd: graphWhereUsedConfigCmd, run: runGraphWhereUsedConfig, args: []string{"ZGOLD_VAR"}},
		{name: "where_used_config_wbcrossgt_down_json", world: func() fakesap.World { return fakesap.CrossDown(false) }, cmd: graphWhereUsedConfigCmd, run: runGraphWhereUsedConfig, args: []string{"ZGOLD_VAR"}, flags: map[string]string{"format": "json"}},
		{name: "where_used_config_both_down_text", world: func() fakesap.World { return fakesap.CrossDown(true) }, cmd: graphWhereUsedConfigCmd, run: runGraphWhereUsedConfig, args: []string{"ZGOLD_VAR"}},

		{name: "loads_gold_both_text", world: gold, cmd: loadsCmd, run: runLoads, args: []string{"ZCL_GOLD_A"}, flags: map[string]string{"direction": "both"}},
		{name: "loads_gold_both_json", world: gold, cmd: loadsCmd, run: runLoads, args: []string{"ZCL_GOLD_A"}, flags: map[string]string{"direction": "both", "json": "true"}},
		{name: "loads_report_loads_text", world: gold, cmd: loadsCmd, run: runLoads, args: []string{"ZGOLD_REPORT"}},
	}
}

func TestGoldenADTSource(t *testing.T) {
	for _, c := range cliGoldenCases() {
		t.Run(c.name, func(t *testing.T) {
			srv := fakesap.New(t, c.world())
			examplesAgainst(t, srv.Server)
			withFlags(t, c.cmd, c.flags)
			c.cmd.SetContext(context.Background())

			var stdout, stderr string
			var err error
			stdout = captureStdout(t, func() {
				stderr = captureStderr(t, func() {
					err = c.run(c.cmd, c.args)
				})
			})
			var b strings.Builder
			fmt.Fprintf(&b, "=== stdout\n%s\n=== stderr\n%s\n=== error\n%v\n=== requests\n%s\n",
				stdout, stderr, err, strings.Join(srv.Log(), "\n"))
			goldenCompare(t, "cli_"+c.name, fakesap.Normalise(b.String()))
		})
	}
}

// The package resolver on its own, against each way the lookups can fail: what
// ends up placed, and what is reported as unplaced.
func TestGoldenADTSourceResolvePackages(t *testing.T) {
	worlds := map[string]func() fakesap.World{
		"narrow":            fakesap.Narrow,
		"narrow_tadir_down": fakesap.NarrowTADIRDown,
		"narrow_tfdir_down": fakesap.NarrowTFDIRDown,
		"narrow_fugr_down":  fakesap.NarrowFUGRDown,
		"narrow_all_down":   fakesap.NarrowAllDown,
		"gold":              fakesap.Gold,
	}
	for name, world := range worlds {
		t.Run(name, func(t *testing.T) {
			srv := fakesap.New(t, world())
			examplesAgainst(t, srv.Server)
			client, err := getClient(mustSystemParams(t))
			if err != nil {
				t.Fatal(err)
			}
			g := goldenResolveGraph()
			var missedText string
			stderr := captureStderr(t, func() {
				missed := resolvePackagesCLI(context.Background(), client, g)
				for _, m := range missed {
					missedText += fmt.Sprintf("%s: %s\n", m.Object, m.Reason)
				}
			})
			goldenCompare(t, "cli_resolve_"+name, fakesap.Normalise(fmt.Sprintf(
				"=== nodes\n%s=== missed\n%s=== stderr\n%s=== requests\n%s\n",
				describeNodes(g), missedText, stderr, strings.Join(srv.Log(), "\n"))))
		})
	}
}

// goldenResolveGraph is the graph a scan of ZCL_NARROW produces: one placed
// object and five targets, so every lookup is a single batch.
func goldenResolveGraph() *graph.Graph {
	g := graph.New()
	g.AddNode(&graph.Node{ID: "CLAS:ZCL_NARROW", Name: "ZCL_NARROW", Type: "CLAS", Package: "$ZNARROW"})
	for _, id := range []string{"CLAS:ZCL_FOREIGN", "FUGR:Z_GOLD_FM", "FUGR:Z_LOST_FM", "CLAS:ZCL_GOLD_A", "CLAS:CL_STANDARD", "DYNAMIC:LV_FM"} {
		parts := strings.SplitN(id, ":", 2)
		g.AddNode(&graph.Node{ID: id, Name: parts[1], Type: parts[0]})
	}
	return g
}

func describeNodes(g *graph.Graph) string {
	var lines []string
	for _, n := range g.Nodes() {
		lines = append(lines, fmt.Sprintf("%s type=%s package=%s\n", n.ID, n.Type, n.Package))
	}
	sort.Strings(lines)
	return strings.Join(lines, "")
}

func mustSystemParams(t *testing.T) *systemParams {
	t.Helper()
	params, err := resolveSystemParams(healthCmd)
	if err != nil {
		t.Fatal(err)
	}
	return params
}
