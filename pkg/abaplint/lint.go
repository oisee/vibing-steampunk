package abaplint

import "strings"

// Issue represents a lint finding.
type Issue struct {
	Key      string // rule key, e.g. "line_length"
	Message  string
	Severity string // "Error", "Warning", "Information"
	Row      int    // 1-based
	Col      int    // 1-based
}

// ABAPFile holds parsed ABAP source for rule checking.
type ABAPFile struct {
	Filename   string
	Raw        string
	rawRows    []string
	tokens     []Token
	statements []Statement
}

// NewABAPFile parses source and creates an ABAPFile ready for linting.
func NewABAPFile(filename, source string) *ABAPFile {
	lex := &Lexer{}
	tokens := lex.Run(source)

	parser := &StatementParser{}
	stmts := parser.Parse(tokens)

	matcher := NewStatementMatcher()
	matcher.ClassifyStatements(stmts)

	return &ABAPFile{
		Filename:   filename,
		Raw:        source,
		tokens:     tokens,
		statements: stmts,
	}
}

// GetRawRows returns source lines (cached).
func (f *ABAPFile) GetRawRows() []string {
	if f.rawRows == nil {
		f.rawRows = strings.Split(f.Raw, "\n")
	}
	return f.rawRows
}

// GetTokens returns all tokens.
func (f *ABAPFile) GetTokens() []Token {
	return f.tokens
}

// GetStatements returns all statements.
func (f *ABAPFile) GetStatements() []Statement {
	return f.statements
}

// Rule is the interface for all lint rules.
type Rule interface {
	GetKey() string
	Run(file *ABAPFile) []Issue
}

// RuleConfig is configuration for a rule.
type RuleConfig struct {
	Enabled  bool
	Severity string
	Exclude  []string
}

// Linter runs a set of rules against ABAP files.
type Linter struct {
	Rules []Rule
}

// NewLinter creates a linter with default rules.
func NewLinter() *Linter {
	return &Linter{
		Rules: defaultRules(),
	}
}

// Run executes all rules on the given source.
func (l *Linter) Run(filename, source string) []Issue {
	file := NewABAPFile(filename, source)
	var issues []Issue
	for _, rule := range l.Rules {
		issues = append(issues, rule.Run(file)...)
	}
	return issues
}

// defaultRules returns all built-in rules with default config.
func defaultRules() []Rule {
	return []Rule{
		&LineLengthRule{MaxLength: 120},
		&EmptyStatementRule{},
		&ObsoleteStatementRule{
			Compute: true, Add: true, Subtract: true,
			Multiply: true, Divide: true, Move: true, Refresh: true,
		},
		&MaxOneStatementRule{},
		&PreferredCompareOperatorRule{
			BadOperators: []string{"EQ", "><", "NE", "GE", "GT", "LT", "LE"},
		},
		&ColonMissingSpaceRule{},
		&DoubleSpaceRule{AfterKeywords: true},
		&LocalVariableNamesRule{
			ExpectedData:     `^[Ll][VvSsTtRrCc]_\w+$`,
			ExpectedConstant: `^[Ll][Cc]_\w+$`,
			ExpectedFS:       `^<[Ll][VvSsTtRr]_\w+>$`,
		},
	}
}
