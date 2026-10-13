package saprfc

import "testing"

// A configured terminal id must reach the breakpoints a session sets before it
// listens, or a SAP GUI session (which runs under its own terminal) never stops.
func TestUseTerminalSetsBreakpointTerminal(t *testing.T) {
	const gui = "0123456789ABCDEF0123456789ABCDEF"

	d := NewADTDebugger(nil, "developer")
	if _, term := d.ideIDs(); term != TerminalID {
		t.Fatalf("default terminal = %q, want vsp's own %q", term, TerminalID)
	}

	d.UseTerminal(" " + gui + " ")
	ide, term := d.ideIDs()
	if term != gui {
		t.Errorf("terminal after UseTerminal = %q, want %q", term, gui)
	}
	if ide != IDEID {
		t.Errorf("ide id changed to %q, want %q", ide, IDEID)
	}

	d.UseTerminal("")
	if _, term := d.ideIDs(); term != TerminalID {
		t.Errorf("terminal after UseTerminal(\"\") = %q, want vsp's own %q", term, TerminalID)
	}
}
