package adt

import (
	"context"
	"fmt"
	"strings"
)

// --- Compare Source Tool ---

// SourceDiff represents a diff between two sources.
type SourceDiff struct {
	Object1      string `json:"object1"`
	Object2      string `json:"object2"`
	Identical    bool   `json:"identical"`
	AddedLines   int    `json:"addedLines"`
	RemovedLines int    `json:"removedLines"`
	Diff         string `json:"diff"`
}

// CompareSource compares source code of two objects and returns a unified diff.
// Supports comparing any two objects that can be read via GetSource.
func (c *Client) CompareSource(ctx context.Context, type1, name1, type2, name2 string, opts1, opts2 *GetSourceOptions) (*SourceDiff, error) {
	// Get source of first object
	source1, err := c.GetSource(ctx, type1, name1, opts1)
	if err != nil {
		return nil, fmt.Errorf("getting source for %s %s: %w", type1, name1, err)
	}

	// Get source of second object
	source2, err := c.GetSource(ctx, type2, name2, opts2)
	if err != nil {
		return nil, fmt.Errorf("getting source for %s %s: %w", type2, name2, err)
	}

	result := &SourceDiff{
		Object1:   fmt.Sprintf("%s:%s", type1, name1),
		Object2:   fmt.Sprintf("%s:%s", type2, name2),
		Identical: source1 == source2,
	}

	if result.Identical {
		result.Diff = "Sources are identical"
		return result, nil
	}

	// Generate unified diff
	lines1 := strings.Split(source1, "\n")
	lines2 := strings.Split(source2, "\n")

	diff := generateUnifiedDiff(result.Object1, result.Object2, lines1, lines2)
	result.Diff = diff

	// Count added/removed lines
	for _, line := range strings.Split(diff, "\n") {
		if strings.HasPrefix(line, "+") && !strings.HasPrefix(line, "+++") {
			result.AddedLines++
		} else if strings.HasPrefix(line, "-") && !strings.HasPrefix(line, "---") {
			result.RemovedLines++
		}
	}

	return result, nil
}

// generateUnifiedDiff creates a unified diff between two sets of lines.
// Uses LCS (Longest Common Subsequence) algorithm for accurate diff generation.
func generateUnifiedDiff(name1, name2 string, lines1, lines2 []string) string {
	var diff strings.Builder

	fmt.Fprintf(&diff, "--- %s\n", name1)
	fmt.Fprintf(&diff, "+++ %s\n", name2)

	// Simple LCS-based diff algorithm
	m, n := len(lines1), len(lines2)

	// Build LCS table
	lcs := make([][]int, m+1)
	for i := range lcs {
		lcs[i] = make([]int, n+1)
	}
	for i := 1; i <= m; i++ {
		for j := 1; j <= n; j++ {
			if lines1[i-1] == lines2[j-1] {
				lcs[i][j] = lcs[i-1][j-1] + 1
			} else if lcs[i-1][j] > lcs[i][j-1] {
				lcs[i][j] = lcs[i-1][j]
			} else {
				lcs[i][j] = lcs[i][j-1]
			}
		}
	}

	// Backtrack to generate diff
	type diffLine struct {
		op   byte // ' ', '+', '-'
		text string
	}
	var diffLines []diffLine

	i, j := m, n
	for i > 0 || j > 0 {
		if i > 0 && j > 0 && lines1[i-1] == lines2[j-1] {
			diffLines = append([]diffLine{{' ', lines1[i-1]}}, diffLines...)
			i--
			j--
		} else if j > 0 && (i == 0 || lcs[i][j-1] >= lcs[i-1][j]) {
			diffLines = append([]diffLine{{'+', lines2[j-1]}}, diffLines...)
			j--
		} else {
			diffLines = append([]diffLine{{'-', lines1[i-1]}}, diffLines...)
			i--
		}
	}

	// Output hunks with context
	const contextLines = 3
	inHunk := false
	hunkStart1, hunkStart2 := 0, 0
	hunkLen1, hunkLen2 := 0, 0
	var hunkContent strings.Builder
	contextBefore := make([]diffLine, 0, contextLines)
	pendingContext := 0

	flushHunk := func() {
		if hunkLen1 > 0 || hunkLen2 > 0 {
			fmt.Fprintf(&diff, "@@ -%d,%d +%d,%d @@\n", hunkStart1, hunkLen1, hunkStart2, hunkLen2)
			diff.WriteString(hunkContent.String())
		}
		hunkContent.Reset()
		inHunk = false
		hunkLen1, hunkLen2 = 0, 0
	}

	line1, line2 := 1, 1
	for _, dl := range diffLines {
		if dl.op == ' ' {
			if inHunk {
				pendingContext++
				fmt.Fprintf(&hunkContent, " %s\n", dl.text)
				hunkLen1++
				hunkLen2++
				if pendingContext >= contextLines*2 {
					// Too much context, close hunk
					flushHunk()
					contextBefore = contextBefore[:0]
				}
			} else {
				// Accumulate context before a hunk
				if len(contextBefore) >= contextLines {
					contextBefore = contextBefore[1:]
				}
				contextBefore = append(contextBefore, dl)
			}
			line1++
			line2++
		} else {
			pendingContext = 0
			if !inHunk {
				// Start new hunk
				inHunk = true
				hunkStart1 = line1 - len(contextBefore)
				hunkStart2 = line2 - len(contextBefore)
				if hunkStart1 < 1 {
					hunkStart1 = 1
				}
				if hunkStart2 < 1 {
					hunkStart2 = 1
				}
				// Add context before
				for _, ctx := range contextBefore {
					fmt.Fprintf(&hunkContent, " %s\n", ctx.text)
					hunkLen1++
					hunkLen2++
				}
				contextBefore = contextBefore[:0]
			}
			fmt.Fprintf(&hunkContent, "%c%s\n", dl.op, dl.text)
			if dl.op == '-' {
				hunkLen1++
				line1++
			} else {
				hunkLen2++
				line2++
			}
		}
	}
	flushHunk()

	return diff.String()
}
