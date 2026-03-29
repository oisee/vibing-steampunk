package wasmcomp

import "strings"

// maxLineLen is the ABAP maximum line length.
// Technical limit is 255; we use 240 to leave margin for indentation.
const maxLineLen = 240

// linePacker collects statements and packs multiple onto one line.
type linePacker struct {
	sb        *strings.Builder
	indent    int
	pending   []string // statements waiting to be flushed
	curLen    int      // current packed line length (including indent)
}

func newLinePacker(sb *strings.Builder, indent int) *linePacker {
	return &linePacker{sb: sb, indent: indent}
}

// add adds a statement. If it fits on the current line, pack it. Otherwise flush and start new line.
func (lp *linePacker) add(stmt string) {
	// Statements that MUST be on their own line (control flow)
	if mustOwnLine(stmt) {
		lp.flush()
		lp.writeLine(stmt)
		return
	}

	indentLen := lp.indent * 2
	stmtLen := len(stmt)

	if len(lp.pending) == 0 {
		// First statement on new line
		if indentLen+stmtLen > maxLineLen {
			// Single statement exceeds limit — write as-is
			lp.writeLine(stmt)
			return
		}
		lp.pending = append(lp.pending, stmt)
		lp.curLen = indentLen + stmtLen
		return
	}

	// Try to append to current line (add 1 for the space separator)
	newLen := lp.curLen + 1 + stmtLen
	if newLen <= maxLineLen {
		lp.pending = append(lp.pending, stmt)
		lp.curLen = newLen
		return
	}

	// Doesn't fit — flush current, start new line
	lp.flush()
	if indentLen+stmtLen > maxLineLen {
		lp.writeLine(stmt)
		return
	}
	lp.pending = append(lp.pending, stmt)
	lp.curLen = indentLen + stmtLen
}

// flush writes all pending statements as one line.
func (lp *linePacker) flush() {
	if len(lp.pending) == 0 {
		return
	}
	prefix := strings.Repeat("  ", lp.indent)
	lp.sb.WriteString(prefix)
	lp.sb.WriteString(strings.Join(lp.pending, " "))
	lp.sb.WriteByte('\n')
	lp.pending = lp.pending[:0]
	lp.curLen = 0
}

func (lp *linePacker) writeLine(stmt string) {
	prefix := strings.Repeat("  ", lp.indent)
	lp.sb.WriteString(prefix)
	lp.sb.WriteString(stmt)
	lp.sb.WriteByte('\n')
}

func (lp *linePacker) setIndent(indent int) {
	if indent != lp.indent {
		lp.flush()
		lp.indent = indent
	}
}

// mustOwnLine returns true for statements that TRULY need their own line.
// For machine-generated code, only structural boundaries matter.
// IF/ELSE/ENDIF/DO/ENDDO/CASE etc. can all pack onto the same line.
func mustOwnLine(stmt string) bool {
	for _, kw := range []string{
		// Only top-level structural boundaries
		"FORM ", "ENDFORM.",
		"METHOD ", "ENDMETHOD.",
		"CLASS ", "ENDCLASS.",
		"FUNCTION ", "ENDFUNCTION.",
	} {
		if strings.HasPrefix(stmt, kw) {
			return true
		}
	}
	return false
}
