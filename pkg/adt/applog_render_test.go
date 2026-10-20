package adt

import "testing"

// Message 00 001 is "&1&2&3&4&5&6&7&8", the one SAP uses to log free text
// split over the four variables. &5 to &8 have no variable: they stay empty,
// where a bare-placeholder reading repeated the first variable after the
// text and left the digits behind ("...IndexAfter Import: ...Index5678").
func TestRenderMessagePlaceholdersAboveFourStayEmpty(t *testing.T) {
	for _, c := range []struct {
		text string
		vars []string
		want string
	}{
		{"&1&2&3&4&5&6&7&8", []string{"After Import: Documentation Links Ind", "ex", "", ""}, "After Import: Documentation Links Index"},
		{"&1&2&3&4&5&6&7&8", []string{"After Import: BSP Index", "", "", ""}, "After Import: BSP Index"},
		{"Value &9", []string{"x", "", "", ""}, "Value"},
		{"Job & & started", []string{"NIGHTLY", "12345678", "", ""}, "Job NIGHTLY 12345678 started"},
		{"&1 && &2", []string{"a", "b", "", ""}, "a & b"},
	} {
		if got := renderMessage(c.text, c.vars...); got != c.want {
			t.Errorf("renderMessage(%q, %q) = %q, want %q", c.text, c.vars, got, c.want)
		}
	}
}
