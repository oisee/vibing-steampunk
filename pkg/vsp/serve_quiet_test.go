package vsp

import (
	"bytes"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/spf13/cobra"
)

// An error from a running MCP server must not bring the CLI usage text with
// it; the MCP client shows stderr as the server's log.
func TestServeMCPErrorPrintsNoUsage(t *testing.T) {
	var stderr bytes.Buffer
	cmd := &cobra.Command{
		Use: "vsp",
		RunE: func(cmd *cobra.Command, args []string) error {
			return serveMCP(cmd, func() error { return errors.New("server stopped") })
		},
	}
	cmd.Flags().Bool("read-only", false, "a flag the usage text would list")
	cmd.SetOut(&stderr)
	cmd.SetErr(&stderr)
	cmd.SetArgs(nil)

	if err := cmd.Execute(); err == nil {
		t.Fatal("want the server's error back")
	}
	if strings.Contains(stderr.String(), "Usage:") || strings.Contains(stderr.String(), "--read-only") {
		t.Fatalf("usage printed for a server error:\n%s", stderr.String())
	}
}

func TestResolveCallTimeout(t *testing.T) {
	newCmd := func() *cobra.Command {
		cmd := &cobra.Command{Use: "vsp"}
		cmd.Flags().Int("call-timeout", 0, "")
		return cmd
	}
	good := []struct {
		flag, env string
		want      time.Duration
	}{
		{"", "", 0},
		{"", "0", 0},
		{"", "300", 300 * time.Second},
		{"", "5m", 5 * time.Minute},
		{"", "1.5", 1500 * time.Millisecond},
		{"", "100000", time.Hour}, // capped
		{"", "3h", time.Hour},     // capped
		{"90", "300", 90 * time.Second},
		{"0", "300", 0}, // an explicit 0 on the flag wins: none
		{"7200", "", time.Hour},
	}
	for _, c := range good {
		t.Setenv("SAP_CALL_TIMEOUT", c.env)
		cmd := newCmd()
		if c.flag != "" {
			_ = cmd.Flags().Set("call-timeout", c.flag)
		}
		got, err := resolveCallTimeout(cmd)
		if err != nil || got != c.want {
			t.Errorf("flag %q env %q: got %v, %v; want %v", c.flag, c.env, got, err, c.want)
		}
	}
	bad := []struct{ flag, env string }{
		{"", "soon"},
		{"", "-5"},
		{"", "-1m"},
		{"", "500ms"},
		{"", "0.5"},
		{"", "NaN"},
		{"", "+Inf"},
		{"", "Inf"},
		{"-1", ""},
	}
	for _, c := range bad {
		t.Setenv("SAP_CALL_TIMEOUT", c.env)
		cmd := newCmd()
		if c.flag != "" {
			_ = cmd.Flags().Set("call-timeout", c.flag)
		}
		if got, err := resolveCallTimeout(cmd); err == nil {
			t.Errorf("flag %q env %q: got %v, want a startup error", c.flag, c.env, got)
		}
	}
}
