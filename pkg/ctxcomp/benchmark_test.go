package ctxcomp

import (
	"os"
	"strings"
	"testing"
	"time"
)

// TestBenchmarkApproaches compares regex (Go) vs parser approaches
// on real ABAP source files from the embedded test data.
func TestBenchmarkApproaches(t *testing.T) {
	// Collect all embedded ABAP source files
	sources := map[string]string{}

	// Read from embedded abap sources
	abapDir := "../ts2abap/testdata/abaplint_lexer"
	entries, err := os.ReadDir(abapDir)
	if err != nil {
		t.Skipf("No test data: %v", err)
	}

	for _, e := range entries {
		if strings.HasSuffix(e.Name(), ".clas.abap") {
			data, err := os.ReadFile(abapDir + "/" + e.Name())
			if err != nil {
				continue
			}
			sources[e.Name()] = string(data)
		}
	}

	// Also add some synthetic ABAP with edge cases
	sources["edge_cases.abap"] = `CLASS zcl_edge DEFINITION PUBLIC.
  PUBLIC SECTION.
    DATA mo_a TYPE REF TO zcl_real_dep.
    DATA mo_b TYPE REF TO zif_real_intf.
ENDCLASS.
CLASS zcl_edge IMPLEMENTATION.
  METHOD run.
    DATA(lo) = NEW zcl_factory( ).
    zcl_util=>call( ).
    CALL FUNCTION 'Z_REAL_FM'.
    * comment zcl_fake_comment=>nope
    WRITE 'zcl_fake_string=>nope'.
    DATA(lv) = |{ zcl_fake_template=>nope }|.
  ENDMETHOD.
ENDCLASS.`

	t.Logf("Testing %d ABAP sources", len(sources))

	totalLines := 0
	totalDeps := 0
	regexDuration := time.Duration(0)

	// Approach 1: Go Regex (ctxcomp)
	for name, src := range sources {
		lines := strings.Count(src, "\n") + 1
		totalLines += lines

		start := time.Now()
		deps := ExtractDependencies(src)
		regexDuration += time.Since(start)
		totalDeps += len(deps)

		if len(deps) > 0 && testing.Verbose() {
			var names []string
			for _, d := range deps {
				names = append(names, d.Name)
			}
			t.Logf("  %s: %d lines, %d deps: %s", name, lines, len(deps), strings.Join(names, ", "))
		}
	}

	t.Logf("\n=== Go Regex (ctxcomp) ===")
	t.Logf("Sources: %d, Lines: %d", len(sources), totalLines)
	t.Logf("Dependencies found: %d", totalDeps)
	t.Logf("Duration: %v", regexDuration)
	t.Logf("Speed: %.0f lines/ms", float64(totalLines)/float64(regexDuration.Milliseconds()+1))

	// Approach 2: Contract compression (how much it shrinks)
	contractDuration := time.Duration(0)
	totalOriginal := 0
	totalCompressed := 0

	for _, src := range sources {
		totalOriginal += len(src)

		start := time.Now()
		contract := ExtractContract(src, KindClass)
		contractDuration += time.Since(start)

		if contract != "" {
			totalCompressed += len(contract)
		}
	}

	ratio := float64(totalOriginal) / float64(totalCompressed+1)
	t.Logf("\n=== Contract Compression ===")
	t.Logf("Original: %d bytes", totalOriginal)
	t.Logf("Compressed: %d bytes", totalCompressed)
	t.Logf("Ratio: %.1fx", ratio)
	t.Logf("Duration: %v", contractDuration)

	// Edge case analysis
	t.Logf("\n=== Edge Case Analysis ===")
	edgeSrc := sources["edge_cases.abap"]
	deps := ExtractDependencies(edgeSrc)
	t.Logf("Edge case dependencies found by regex:")
	for _, d := range deps {
		isReal := !strings.Contains(d.Name, "FAKE")
		label := "TRUE"
		if !isReal {
			label = "FALSE POSITIVE"
		}
		t.Logf("  %s (%s) - %s", d.Name, d.Kind, label)
	}

	// Count false positives
	fp := 0
	for _, d := range deps {
		if strings.Contains(d.Name, "FAKE") {
			fp++
		}
	}
	t.Logf("False positives: %d / %d (%.0f%%)", fp, len(deps), 100*float64(fp)/float64(len(deps)))
}
