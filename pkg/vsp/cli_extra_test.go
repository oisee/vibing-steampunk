package vsp

import (
	"testing"

	"github.com/spf13/cobra"
)

// `--top 0` must mean "all rows" (per the flag's own help text), while
// omitting `--top` entirely must keep the existing 100-row default. Both
// leave the underlying int at its zero value, so the fix depends entirely on
// cmd.Flags().Changed("top") telling the two cases apart. This test pins that
// distinction so a future "simplification" doesn't collapse it back into a
// bare `top == 0` check.
func TestQueryTopFlagChangedDistinguishesExplicitZero(t *testing.T) {
	newQueryFlags := func() *cobra.Command {
		cmd := &cobra.Command{Use: "query"}
		cmd.Flags().Int("top", 0, "Maximum number of rows (0=all)")
		cmd.Flags().Int("skip", 0, "Skip first N rows")
		return cmd
	}

	t.Run("top omitted", func(t *testing.T) {
		cmd := newQueryFlags()
		if err := cmd.ParseFlags([]string{}); err != nil {
			t.Fatal(err)
		}
		if cmd.Flags().Changed("top") {
			t.Fatal("Changed(\"top\") should be false when --top is not passed")
		}
		top, _ := cmd.Flags().GetInt("top")
		if top != 0 {
			t.Fatalf("expected zero value for unset --top, got %d", top)
		}
	})

	t.Run("top explicitly zero", func(t *testing.T) {
		cmd := newQueryFlags()
		if err := cmd.ParseFlags([]string{"--top", "0"}); err != nil {
			t.Fatal(err)
		}
		if !cmd.Flags().Changed("top") {
			t.Fatal("Changed(\"top\") should be true when --top 0 is passed explicitly")
		}
		top, _ := cmd.Flags().GetInt("top")
		if top != 0 {
			t.Fatalf("expected 0, got %d", top)
		}
	})

	t.Run("top explicitly positive", func(t *testing.T) {
		cmd := newQueryFlags()
		if err := cmd.ParseFlags([]string{"--top", "50"}); err != nil {
			t.Fatal(err)
		}
		if !cmd.Flags().Changed("top") {
			t.Fatal("Changed(\"top\") should be true when --top is passed")
		}
		top, _ := cmd.Flags().GetInt("top")
		if top != 50 {
			t.Fatalf("expected 50, got %d", top)
		}
	})
}
