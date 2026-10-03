package vsp

import (
	"context"
	"strings"
	"testing"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/spf13/cobra"

	"github.com/oisee/vibing-steampunk/pkg/adt"
	"github.com/oisee/vibing-steampunk/pkg/mcpext"
)

type cmdExtension struct {
	name     string
	commands []mcpext.Command
}

func (e cmdExtension) Name() string    { return e.name }
func (e cmdExtension) Help() string    { return "" }
func (e cmdExtension) Version() string { return "0.1.0" }
func (e cmdExtension) Actions() []mcpext.Action {
	return []mcpext.Action{{Action: "read", Type: "CMDX", Class: mcpext.Read, Op: adt.OpRead,
		Handler: func(context.Context, mcpext.Env, string, map[string]any) (*mcp.CallToolResult, error) {
			return nil, nil
		}}}
}
func (e cmdExtension) Commands(mcpext.EnvFunc) []mcpext.Command { return e.commands }

func TestExtensionCommandGoesUnderItsParent(t *testing.T) {
	sub := &cobra.Command{Use: "import-demo"}
	top := &cobra.Command{Use: "demo-top"}
	ext := cmdExtension{name: "cmds", commands: []mcpext.Command{{Parent: "transport", Command: sub}, {Command: top}}}
	if err := attachExtensionCommands([]mcpext.Extension{ext}); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		sub.Parent().RemoveCommand(sub)
		rootCmd.RemoveCommand(top)
	})
	if got := sub.CommandPath(); !strings.HasSuffix(got, "transport import-demo") {
		t.Errorf("sub-command path %q", got)
	}
	if top.Parent() != rootCmd {
		t.Error("a top-level command was not put at the top level")
	}
}

func TestExtensionCommandsThatCollideOrGoNowhereAreRefused(t *testing.T) {
	ext := cmdExtension{name: "cmds", commands: []mcpext.Command{
		{Command: &cobra.Command{Use: "transport"}},                      // a built-in name
		{Parent: "nosuchparent", Command: &cobra.Command{Use: "orphan"}}, // no such parent
		{Parent: "transport"}, // no command
	}}
	err := attachExtensionCommands([]mcpext.Extension{ext})
	if err == nil {
		t.Fatal("conflicting commands were accepted")
	}
	for _, want := range []string{`transport" already exists`, `under "nosuchparent"`, "without a cobra.Command"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error does not say %q:\n%v", want, err)
		}
	}
}

func TestExtensionVersionsLine(t *testing.T) {
	if got := extensionVersions(nil); got != "" {
		t.Errorf("no extensions: %q", got)
	}
	if got := extensionVersions([]mcpext.Extension{cmdExtension{name: "cmds"}}); got != "; extensions: cmds 0.1.0" {
		t.Errorf("versions line %q", got)
	}
}
