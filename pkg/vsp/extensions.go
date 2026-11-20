package vsp

import (
	"context"
	"fmt"
	"os"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/oisee/open-rfc-go/rfc"
	"github.com/spf13/cobra"

	"github.com/oisee/vibing-steampunk/pkg/adt"
	"github.com/oisee/vibing-steampunk/pkg/config"
	"github.com/oisee/vibing-steampunk/pkg/mcpext"
	"github.com/oisee/vibing-steampunk/pkg/saprfc"
)

// attachExtensionCommands adds the extensions' command-line commands, under
// the built-in command each names as its parent or at the top level. A name
// already taken there, or a parent that does not exist, stops Run: a command
// that silently replaced a built-in, or went nowhere, would be found by
// nobody.
func attachExtensionCommands(exts []mcpext.Extension) error {
	var problems []string
	for _, ext := range exts {
		cp, ok := ext.(mcpext.CommandProvider)
		if !ok {
			continue
		}
		name := ext.Name()
		for _, c := range cp.Commands(func(cmd *cobra.Command) (mcpext.Env, error) { return newCLIEnv(cmd, name) }) {
			if c.Command == nil {
				problems = append(problems, fmt.Sprintf("extension %s: a command without a cobra.Command", name))
				continue
			}
			parent := rootCmd
			if c.Parent != "" {
				parent = findSubcommand(rootCmd, c.Parent)
				if parent == nil {
					problems = append(problems, fmt.Sprintf("extension %s: command %q goes under %q, which vsp does not have", name, c.Command.Name(), c.Parent))
					continue
				}
			}
			if findSubcommand(parent, c.Command.Name()) != nil {
				problems = append(problems, fmt.Sprintf("extension %s: %q already exists", name, strings.TrimSpace(parent.CommandPath()+" "+c.Command.Name())))
				continue
			}
			parent.AddCommand(c.Command)
		}
	}
	if len(problems) > 0 {
		return fmt.Errorf("extension commands refused:\n  %s", strings.Join(problems, "\n  "))
	}
	return nil
}

func findSubcommand(parent *cobra.Command, name string) *cobra.Command {
	for _, c := range parent.Commands() {
		if c.Name() == name || c.HasAlias(name) {
			return c
		}
	}
	return nil
}

// extensionVersions lists the extensions with their versions, for
// `vsp --version`.
func extensionVersions(exts []mcpext.Extension) string {
	if len(exts) == 0 {
		return ""
	}
	var parts []string
	for _, ext := range exts {
		v := "unversioned"
		if ver, ok := ext.(mcpext.Versioned); ok && ver.Version() != "" {
			v = ver.Version()
		}
		parts = append(parts, ext.Name()+" "+v)
	}
	sort.Strings(parts)
	return "; extensions: " + strings.Join(parts, ", ")
}

// cliEnv is an extension's Env on the command line: the system, credentials
// and flags the command resolved, connections opened on demand.
type cliEnv struct {
	cmd    *cobra.Command
	ext    string
	sys    *systemParams
	client *adt.Client

	wsMu sync.Mutex
	ws   *adt.DebugWebSocketClient
}

func newCLIEnv(cmd *cobra.Command, ext string) (mcpext.Env, error) {
	sys, err := resolveSystemParams(cmd)
	if err != nil {
		return nil, err
	}
	client, err := createADTClientFor(cmd)
	if err != nil {
		return nil, err
	}
	return &cliEnv{cmd: cmd, ext: ext, sys: sys, client: client}, nil
}

func (e *cliEnv) ADT() *adt.Client { return e.client }

func (e *cliEnv) RFC(ctx context.Context) (*rfc.Client, func(), error) {
	c, err := e.RFCDedicated(ctx, 0)
	if err != nil {
		return nil, nil, err
	}
	return c, func() { _ = c.Close(context.Background()) }, nil
}

func (e *cliEnv) RFCDedicated(ctx context.Context, timeout time.Duration) (*rfc.Client, error) {
	dest, err := rfcDestinationFor(e.cmd)
	if err != nil {
		return nil, err
	}
	c, err := saprfc.OpenWithTimeout(ctx, dest, timeout)
	if err != nil {
		return nil, fmt.Errorf("RFC logon to %s:%d failed: %w", dest.Host, dest.Port, err)
	}
	return c, nil
}

// DropRFC has nothing to drop: every RFC of a command is its own connection.
func (e *cliEnv) DropRFC(context.Context) {}

func (e *cliEnv) ZADTVSP(ctx context.Context) (*adt.DebugWebSocketClient, error) {
	e.wsMu.Lock()
	defer e.wsMu.Unlock()
	if e.ws != nil && e.ws.IsConnected() {
		return e.ws, nil
	}
	ws := e.client.NewDebugWebSocketClient()
	if err := ws.Connect(ctx); err != nil {
		return nil, fmt.Errorf("ZADT_VSP (WebSocket) is not reachable: %w", err)
	}
	e.ws = ws
	return ws, nil
}

func (e *cliEnv) System() mcpext.System {
	return mcpext.System{Name: e.sys.Name, URL: e.sys.URL, Client: e.sys.Client, User: e.sys.User,
		Language: e.sys.Language, ReadOnly: e.client.Safety().ReadOnly}
}

func (e *cliEnv) Setting(key string) (any, bool) {
	if e.sys.Name == "" {
		return nil, false
	}
	cfg, _, err := config.LoadSystems()
	if err != nil || cfg == nil {
		return nil, false
	}
	sys, ok := cfg.Systems[e.sys.Name]
	if !ok {
		return nil, false
	}
	return sys.ExtensionSetting(e.ext, key)
}

func (e *cliEnv) StartAsync(string, func(context.Context) (any, error)) (string, error) {
	return "", fmt.Errorf("background tasks need the MCP server; a command line ends with the command")
}

func (e *cliEnv) Logf(format string, args ...any) {
	if verbose, _ := e.cmd.Flags().GetBool("verbose"); verbose || os.Getenv("VSP_VERBOSE") == "true" {
		fmt.Fprintf(os.Stderr, "[%s] %s\n", e.ext, fmt.Sprintf(format, args...))
	}
}
