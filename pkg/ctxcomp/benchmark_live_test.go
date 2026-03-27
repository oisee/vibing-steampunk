package ctxcomp

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/oisee/vibing-steampunk/pkg/adt"
)

// TestBenchmarkLive runs the benchmark against a real SAP system.
// Requires SAP_URL, SAP_USER, SAP_PASSWORD env vars.
func TestBenchmarkLive(t *testing.T) {
	url := os.Getenv("SAP_URL")
	user := os.Getenv("SAP_USER")
	pass := os.Getenv("SAP_PASSWORD")
	if url == "" || user == "" {
		t.Skip("SAP_URL/SAP_USER not set — skipping live benchmark")
	}

	client := adt.NewClient(url, user, pass, adt.WithInsecureSkipVerify())
	var totalCompressMs float64
	_ = totalCompressMs

	ctx := context.Background()

	// Search for abapGit classes
	results, err := client.SearchObject(ctx, "ZCL_ABAPGIT*", 50)
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	t.Logf("Found %d abapGit classes", len(results))

	type result struct {
		name     string
		lines    int
		regexDeps int
		regexFP   int
		regexMs  float64
		compressRatio float64
	}

	var totalLines, totalRegexDeps, totalFP int
	var totalRegexMs float64
	var details []result

	for i, r := range results {
		if i >= 30 { break } // limit to 30

		// Read source via ADT (same as GetSource)
		source, err := client.GetClassSource(ctx, r.Name)
		if err != nil {
			continue
		}

		lines := strings.Count(source, "\n") + 1
		if lines < 10 { continue }

		// Go Regex benchmark
		start := time.Now()
		deps := ExtractDependencies(source)
		regexMs := float64(time.Since(start).Microseconds()) / 1000.0

		// Contract compression
		compStart := time.Now()
		contract := ExtractContract(source, KindClass)
		_ = time.Since(compStart)

		compressRatio := 0.0
		if len(contract) > 0 {
			compressRatio = float64(len(source)) / float64(len(contract))
		}

		// Check for false positives (deps found in strings/comments)
		fp := 0
		for _, d := range deps {
			// Simple heuristic: check if the name appears only inside quotes or after *
			nameInStr := strings.Contains(source, "'"+strings.ToLower(d.Name))
			nameInComment := false
			for _, line := range strings.Split(source, "\n") {
				trimmed := strings.TrimSpace(line)
				if strings.HasPrefix(trimmed, "*") && strings.Contains(strings.ToUpper(trimmed), d.Name) {
					nameInComment = true
				}
			}
			if nameInStr && !strings.Contains(source, "TYPE REF TO "+strings.ToLower(d.Name)) {
				fp++
			}
			_ = nameInComment
		}

		totalLines += lines
		totalRegexDeps += len(deps)
		totalFP += fp
		totalRegexMs += regexMs

		details = append(details, result{
			name: r.Name, lines: lines,
			regexDeps: len(deps), regexFP: fp,
			regexMs: regexMs, compressRatio: compressRatio,
		})
	}

	t.Logf("\n=== LIVE BENCHMARK: Go Regex (ctxcomp) on abapGit ===")
	t.Logf("Classes analyzed: %d", len(details))
	t.Logf("Total lines: %d", totalLines)
	t.Logf("Total dependencies: %d", totalRegexDeps)
	t.Logf("Potential false positives: %d", totalFP)
	t.Logf("Total regex time: %.1f ms", totalRegexMs)
	t.Logf("Avg per class: %.2f ms", totalRegexMs/float64(len(details)))
	t.Logf("Speed: %.0f lines/ms", float64(totalLines)/totalRegexMs)

	t.Logf("\n--- Top 10 by dependency count ---")
	for i, d := range details {
		if i >= 10 { break }
		t.Logf("  %-35s %5d lines  %3d deps  %.1fms  compress=%.1fx",
			d.name, d.lines, d.regexDeps, d.regexMs, d.compressRatio)
	}
}
