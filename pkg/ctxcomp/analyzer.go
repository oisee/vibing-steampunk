package ctxcomp

import (
	"context"
	"sort"
	"strings"
	"time"
)

// AnalysisLayer identifies which analysis method found a dependency.
type AnalysisLayer int

const (
	LayerRegex    AnalysisLayer = iota // 3b: Go regex (ctxcomp) — filesystem, fastest
	LayerParser                        // 3:  abaplint parser — filesystem or SAP, deep
	LayerScan                          // 2:  SCAN ABAP-SOURCE — SAP kernel tokenizer
	LayerCross                         // 1b: CROSS/WBCROSSGT — SAP index, instant
	LayerWhereUsed                     // 1:  ADT Where-Used — SAP full cross-ref
)

func (l AnalysisLayer) String() string {
	switch l {
	case LayerRegex:
		return "regex"
	case LayerParser:
		return "parser"
	case LayerScan:
		return "scan"
	case LayerCross:
		return "cross"
	case LayerWhereUsed:
		return "where-used"
	}
	return "unknown"
}

// AnalyzedDep is a dependency found by one or more layers.
type AnalyzedDep struct {
	Name       string
	Kind       DependencyKind
	Line       int
	FoundBy    []AnalysisLayer // which layers found it
	Confidence float64         // 0.0-1.0, higher = more certain
	InString   bool            // true if found inside a string literal (false positive)
	InComment  bool            // true if found inside a comment (false positive)
}

// AnalysisResult holds the combined output.
type AnalysisResult struct {
	ObjectName    string
	TotalLines    int
	Dependencies  []AnalyzedDep
	TrueDeps      int // confirmed by 2+ layers or high confidence
	FalsePositives int
	Layers        []AnalysisLayer // which layers were used
	Duration      time.Duration
	LayerDurations map[AnalysisLayer]time.Duration
}

// ADTProvider abstracts SAP-side operations (layers 1, 1b, 2).
type ADTProvider interface {
	// ScanSource runs SCAN ABAP-SOURCE on SAP (layer 2)
	ScanSource(ctx context.Context, source string) ([]ScanToken, error)
	// GetCrossReferences queries CROSS/WBCROSSGT (layer 1b)
	GetCrossReferences(ctx context.Context, objectName string) ([]string, error)
	// GetWhereUsed runs ADT where-used (layer 1)
	GetWhereUsed(ctx context.Context, objectURL string) ([]string, error)
}

// ScanToken represents a token from SCAN ABAP-SOURCE.
type ScanToken struct {
	Type string // I=identifier, S=string, C=comment, K=keyword
	Str  string
	Row  int
	Col  int
}

// Analyzer combines all layers for comprehensive code intelligence.
//
// Confidence model:
//   1.0  — parser + SAP layer (scan or cross) agree
//   0.95 — parser + regex agree (no SAP needed)
//   0.9  — parser only (authoritative — reads actual source)
//   0.85 — SCAN ABAP-SOURCE only (SAP kernel, reliable)
//   0.8  — CROSS index + regex agree
//   0.6  — CROSS index only (may be stale — only updated on activation)
//   0.3  — regex only (likely false positive — found in string/comment)
//
// Key insight: CROSS/WBCROSSGT tables can be stale (inactive objects,
// $TMP, unactivated changes). The abaplint parser is the real-time
// ground truth — it parses actual source, not an index snapshot.
// Use parser as primary harness, CROSS as supplementary confirmation.
type Analyzer struct {
	adtProvider ADTProvider // nil = offline mode (layers 3, 3b only)
}

// NewAnalyzer creates a new analyzer. If adtProvider is nil, only offline layers are used.
func NewAnalyzer(adtProvider ADTProvider) *Analyzer {
	return &Analyzer{adtProvider: adtProvider}
}

// Analyze runs all available layers and combines results.
func (a *Analyzer) Analyze(ctx context.Context, source, objectName string) *AnalysisResult {
	start := time.Now()
	result := &AnalysisResult{
		ObjectName:     objectName,
		TotalLines:     strings.Count(source, "\n") + 1,
		LayerDurations: make(map[AnalysisLayer]time.Duration),
	}

	// Merge map: name → AnalyzedDep
	merged := make(map[string]*AnalyzedDep)

	// Layer 3b: Go Regex (always available, fastest)
	t0 := time.Now()
	regexDeps := ExtractDependencies(source)
	result.LayerDurations[LayerRegex] = time.Since(t0)
	result.Layers = append(result.Layers, LayerRegex)

	for _, d := range regexDeps {
		addOrMerge(merged, d.Name, d.Kind, d.Line, LayerRegex)
	}

	// Layer 3: Parser-based (runs in Go — simulate by checking token context)
	// Uses the same regex scan but validates against string/comment patterns
	t0 = time.Now()
	parserDeps := extractWithValidation(source)
	result.LayerDurations[LayerParser] = time.Since(t0)
	result.Layers = append(result.Layers, LayerParser)

	for _, d := range parserDeps {
		addOrMerge(merged, d.name, d.kind, d.line, LayerParser)
	}

	// Mark false positives: found by regex but NOT by parser
	for name, dep := range merged {
		foundByRegex := containsLayer(dep.FoundBy, LayerRegex)
		foundByParser := containsLayer(dep.FoundBy, LayerParser)
		if foundByRegex && !foundByParser {
			dep.InString = true // likely in string or comment
			dep.Confidence = 0.3
			result.FalsePositives++
		}
		_ = name
	}

	// Layer 2: SCAN ABAP-SOURCE (if SAP connected)
	if a.adtProvider != nil {
		t0 = time.Now()
		scanTokens, err := a.adtProvider.ScanSource(ctx, source)
		result.LayerDurations[LayerScan] = time.Since(t0)
		if err == nil {
			result.Layers = append(result.Layers, LayerScan)
			scanDeps := extractFromScanTokens(scanTokens)
			for _, d := range scanDeps {
				addOrMerge(merged, d.name, d.kind, 0, LayerScan)
			}
		}
	}

	// Layer 1b: CROSS/WBCROSSGT (if SAP connected)
	if a.adtProvider != nil && objectName != "" {
		t0 = time.Now()
		crossRefs, err := a.adtProvider.GetCrossReferences(ctx, objectName)
		result.LayerDurations[LayerCross] = time.Since(t0)
		if err == nil {
			result.Layers = append(result.Layers, LayerCross)
			for _, ref := range crossRefs {
				addOrMerge(merged, ref, inferKind(ref), 0, LayerCross)
			}
		}
	}

	// Calculate confidence — parser is the authority, CROSS is supplementary
	// Parser-confirmed = real. Regex-only = suspect. CROSS-only = stale but notable.
	for _, dep := range merged {
		if dep.Confidence > 0 {
			continue // already set (e.g., false positive)
		}

		hasParser := containsLayer(dep.FoundBy, LayerParser)
		hasRegex := containsLayer(dep.FoundBy, LayerRegex)
		hasScan := containsLayer(dep.FoundBy, LayerScan)
		hasCross := containsLayer(dep.FoundBy, LayerCross)

		switch {
		case hasParser && (hasScan || hasCross):
			dep.Confidence = 1.0 // confirmed by parser + SAP layer
		case hasParser && hasRegex:
			dep.Confidence = 0.95 // confirmed by parser (regex agrees)
		case hasParser:
			dep.Confidence = 0.9 // parser says yes (authoritative)
		case hasScan:
			dep.Confidence = 0.85 // SAP kernel tokenizer says yes
		case hasCross && hasRegex:
			dep.Confidence = 0.8 // CROSS index + regex agree
		case hasCross:
			dep.Confidence = 0.6 // CROSS only — might be stale but notable
			dep.InComment = false // not a false positive, just from index
		case hasRegex:
			dep.Confidence = 0.3 // regex only — likely false positive
			dep.InString = true
		default:
			dep.Confidence = 0.1
		}

		if dep.Confidence >= 0.5 {
			result.TrueDeps++
		}
	}

	// Sort by confidence (highest first), then name
	deps := make([]AnalyzedDep, 0, len(merged))
	for _, d := range merged {
		deps = append(deps, *d)
	}
	sort.Slice(deps, func(i, j int) bool {
		if deps[i].Confidence != deps[j].Confidence {
			return deps[i].Confidence > deps[j].Confidence
		}
		return deps[i].Name < deps[j].Name
	})

	result.Dependencies = deps
	result.Duration = time.Since(start)
	return result
}

// --- Internal helpers ---

type parsedDep struct {
	name string
	kind DependencyKind
	line int
}

// extractWithValidation does regex extraction but skips matches inside strings and comments.
func extractWithValidation(source string) []parsedDep {
	var deps []parsedDep
	seen := make(map[string]bool)
	lines := strings.Split(source, "\n")

	for lineIdx, line := range lines {
		lineNum := lineIdx + 1
		trimmed := strings.TrimSpace(line)

		// Skip full-line comments
		if strings.HasPrefix(trimmed, "*") || strings.HasPrefix(trimmed, "\"") {
			continue
		}

		// Remove inline comments (everything after unquoted ")
		cleanLine := removeInlineComment(line)

		// Remove string literals (content between ' ' and ` `)
		// But preserve CALL FUNCTION 'name' pattern first
		cleanLine = removeStringLiterals(cleanLine)

		// CALL FUNCTION needs the original line (name is in string)
		for _, m := range reCallFunction.FindAllStringSubmatch(line, -1) {
			addParsedDep(&deps, seen, m[1], KindFunction, lineNum)
		}

		// Now extract from cleaned line
		for _, m := range reTypeRefTo.FindAllStringSubmatch(cleanLine, -1) {
			addParsedDep(&deps, seen, m[1], inferKind(m[1]), lineNum)
		}
		for _, m := range reNew.FindAllStringSubmatch(cleanLine, -1) {
			addParsedDep(&deps, seen, m[1], KindClass, lineNum)
		}
		for _, m := range reStaticCall.FindAllStringSubmatch(cleanLine, -1) {
			addParsedDep(&deps, seen, m[1], inferKind(m[1]), lineNum)
		}
		for _, m := range reIntfMethod.FindAllStringSubmatch(cleanLine, -1) {
			addParsedDep(&deps, seen, m[1], KindInterface, lineNum)
		}
		for _, m := range reInheriting.FindAllStringSubmatch(cleanLine, -1) {
			addParsedDep(&deps, seen, m[1], KindClass, lineNum)
		}
		for _, m := range reInterfaces.FindAllStringSubmatch(cleanLine, -1) {
			addParsedDep(&deps, seen, m[1], KindInterface, lineNum)
		}
		for _, m := range reCallFunction.FindAllStringSubmatch(cleanLine, -1) {
			addParsedDep(&deps, seen, m[1], KindFunction, lineNum)
		}
		for _, m := range reCast.FindAllStringSubmatch(cleanLine, -1) {
			addParsedDep(&deps, seen, m[1], inferKind(m[1]), lineNum)
		}
		for _, m := range reRaising.FindAllStringSubmatch(cleanLine, -1) {
			addParsedDep(&deps, seen, m[1], KindClass, lineNum)
		}
	}

	return deps
}

func removeInlineComment(line string) string {
	inString := false
	for i := 0; i < len(line); i++ {
		if line[i] == '\'' {
			inString = !inString
		}
		if !inString && line[i] == '"' {
			return line[:i]
		}
	}
	return line
}

func removeStringLiterals(line string) string {
	var result strings.Builder
	inSingle := false
	inBacktick := false

	for i := 0; i < len(line); i++ {
		ch := line[i]
		if ch == '\'' && !inBacktick {
			inSingle = !inSingle
			result.WriteByte(ch)
			continue
		}
		if ch == '`' && !inSingle {
			inBacktick = !inBacktick
			result.WriteByte(ch)
			continue
		}
		if inSingle || inBacktick {
			result.WriteByte('_') // replace content with placeholder
		} else {
			result.WriteByte(ch)
		}
	}
	return result.String()
}

func addParsedDep(deps *[]parsedDep, seen map[string]bool, name string, kind DependencyKind, line int) {
	upper := strings.ToUpper(name)
	if shouldSkip(upper) || seen[upper] {
		return
	}
	// Skip placeholder strings (from removeStringLiterals)
	if strings.Trim(upper, "_") == "" {
		return
	}
	seen[upper] = true
	*deps = append(*deps, parsedDep{name: upper, kind: kind, line: line})
}

func addOrMerge(merged map[string]*AnalyzedDep, name string, kind DependencyKind, line int, layer AnalysisLayer) {
	upper := strings.ToUpper(name)
	if shouldSkip(upper) {
		return
	}
	if existing, ok := merged[upper]; ok {
		if !containsLayer(existing.FoundBy, layer) {
			existing.FoundBy = append(existing.FoundBy, layer)
		}
		if kind == KindInterface && existing.Kind == KindClass {
			existing.Kind = KindInterface
		}
	} else {
		merged[upper] = &AnalyzedDep{
			Name:    upper,
			Kind:    kind,
			Line:    line,
			FoundBy: []AnalysisLayer{layer},
		}
	}
}

func containsLayer(layers []AnalysisLayer, target AnalysisLayer) bool {
	for _, l := range layers {
		if l == target {
			return true
		}
	}
	return false
}

// extractFromScanTokens extracts dependencies from SCAN ABAP-SOURCE tokens.
func extractFromScanTokens(tokens []ScanToken) []parsedDep {
	var deps []parsedDep
	seen := make(map[string]bool)
	prev := ""
	prevPrev := ""

	for _, tok := range tokens {
		if tok.Type != "I" { // only identifiers
			prevPrev = prev
			prev = ""
			continue
		}

		upper := strings.ToUpper(tok.Str)

		// TYPE REF TO <name>
		if prev == "TO" && prevPrev == "REF" {
			addParsedDep(&deps, seen, upper, inferKind(upper), tok.Row)
		}

		// <name>=>
		if strings.Contains(upper, "=>") {
			parts := strings.SplitN(upper, "=>", 2)
			if len(parts[0]) > 0 {
				addParsedDep(&deps, seen, parts[0], inferKind(parts[0]), tok.Row)
			}
		}

		// NEW <name>
		if prev == "NEW" && upper != "(" {
			// Remove trailing ( if present
			clean := strings.TrimSuffix(upper, "(")
			if len(clean) > 0 {
				addParsedDep(&deps, seen, clean, KindClass, tok.Row)
			}
		}

		prevPrev = prev
		prev = upper
	}

	return deps
}
