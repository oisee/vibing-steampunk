// Package mcpext lets a separate Go module add SAP() actions to vsp.
//
// An extension declares its actions; vsp routes to them after every built-in
// router, so an extension can neither shadow nor replace a core action, and
// the core -- not the extension -- runs the safety checks before a handler is
// called: --read-only, --allowed-ops/--disallowed-ops and the rest hold for
// extension code without trusting it. Env.ADT is the server's own client, so
// every ADT mutation an extension makes still passes the mutation gate and the
// package allowlist inside the client.
//
// Extensions are compiled in, not loaded: a downstream binary is a small main
// that passes them to vsp.Run (package pkg/vsp). Go plugins do not work on
// Windows.
//
//	func main() {
//		vsp.Run(forms.New(), idoc.New())
//	}
//
// The package is kept small and changed additively; every extension in a
// binary builds against the same vsp version.
package mcpext

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/mark3labs/mcp-go/mcp"
	openrfc "github.com/oisee/open-rfc-go/rfc"
	"github.com/spf13/cobra"

	"github.com/oisee/vibing-steampunk/pkg/adt"
)

// APIVersion is the version of this package's contract. It grows by one with
// every addition an extension may come to rely on; nothing is taken away. An
// extension that needs a later version than the binary's says so through
// Requirer and is refused at startup instead of failing on first use.
const APIVersion = 2

// Class is what an action does to the system, as the read-only invariant
// classifies the built-in actions.
type Class int

const (
	// Read changes nothing and runs nothing.
	Read Class = iota + 1
	// Mutate changes repository objects or data.
	Mutate
	// Execute runs code in the system (a report, a function module, a job).
	Execute
)

func (c Class) String() string {
	switch c {
	case Read:
		return "read"
	case Mutate:
		return "mutate"
	case Execute:
		return "execute"
	}
	return fmt.Sprintf("Class(%d)", int(c))
}

// Handler serves one action. name is the target's object name, upper-cased
// as vsp parses targets ("SEGM Z1DEMO" -> "Z1DEMO"); params are the call's
// params, as given.
type Handler func(ctx context.Context, env Env, name string, params map[string]any) (*mcp.CallToolResult, error)

// Action is one SAP(action=..., target="TYPE NAME") an extension serves.
type Action struct {
	// Action is the SAP() action, lower case: "read", "create", "edit", ...
	Action string
	// Type is the target type, upper case ("SEGM"). Empty for an action that
	// takes no target.
	Type string
	// Class says what the action does. The core refuses Mutate and Execute
	// under --read-only before the handler runs.
	Class Class
	// Op is the operation the core checks against the safety configuration
	// before the handler runs. A Read action declares a read operation
	// (adt.OpRead, OpSearch, OpQuery, OpFreeSQL, OpIntelligence); a Mutate or
	// Execute action one that changes the system (OpCreate, OpUpdate,
	// OpDelete, OpActivate, OpWorkflow, or OpTransport, which the core also
	// refuses without --enable-transports).
	Op      adt.OperationType
	Handler Handler
}

// Key is how an action is told apart: action and target type.
func (a Action) Key() string {
	if a.Type == "" {
		return a.Action
	}
	return a.Action + " " + a.Type
}

// Env is what the server gives an extension's handler.
type Env interface {
	// ADT returns the server's own ADT client, with its safety configuration
	// and mutation gate.
	ADT() *adt.Client
	// RFC returns a client on the server's own RFC gateway, under the same
	// rules as SAP(action="rfc"): no per-call host or system number, the
	// server's own logon. release gives it back.
	RFC(ctx context.Context) (client *openrfc.Client, release func(), err error)
	// RFCDedicated opens a connection of its own on the same gateway, with
	// the same logon, for a call that outlasts the shared client's timeout.
	// The caller closes it.
	RFCDedicated(ctx context.Context, timeout time.Duration) (*openrfc.Client, error)
	// DropRFC forgets the shared RFC client after its connection died
	// (openrfc.ErrTransport, openrfc.ErrClosed), so the next RFC logs on
	// again instead of failing forever.
	DropRFC(ctx context.Context)
	// ZADTVSP returns the WebSocket to the system's ZADT_VSP, connected on
	// first use. On a server it is the server's own connection, shared with
	// the built-in actions; do not close it.
	ZADTVSP(ctx context.Context) (*adt.DebugWebSocketClient, error)
	// System is the system this server or command is connected to.
	System() System
	// Setting returns one of this extension's settings for the connected
	// system, from .vsp.json: systems.<name>.extensions.<extension>.<key>.
	Setting(key string) (value any, ok bool)
	// StartAsync runs fn in the background and returns a task id that
	// SAP(action="debug", target="GET_ASYNC_RESULT") reports on, waiting up
	// to "wait_seconds" (at most 30 minutes) for it to end. A task that fails
	// keeps the result fn returned with its error. fn runs on a context of
	// its own, not the call's; bound it yourself. StartAsync is not available
	// to a command line, which ends with the command.
	StartAsync(kind string, fn func(ctx context.Context) (any, error)) (taskID string, err error)
	// Logf writes a diagnostic line when the binary runs with --verbose.
	Logf(format string, args ...any)
}

// System identifies the connected system.
type System struct {
	Name     string // the .vsp.json system name; empty without one
	URL      string
	Client   string
	User     string
	Language string
	ReadOnly bool
}

// Extension is a set of actions with a name.
type Extension interface {
	// Name identifies the extension; SAP(action="help", target=Name())
	// shows its help.
	Name() string
	// Actions lists what it serves.
	Actions() []Action
	// Help is the text SAP(action="help", target=Name()) returns.
	Help() string
}

// The interfaces below are optional: an extension implements those it needs.

// Versioned reports the extension's own version, shown by SAP(action="info")
// and `vsp version`.
type Versioned interface {
	Version() string
}

// Requirer names the APIVersion an extension needs at least.
type Requirer interface {
	RequiredAPIVersion() int
}

// Starter is told when an MCP server starts serving, with the server's Env;
// an error stops the server.
type Starter interface {
	Start(ctx context.Context, env Env) error
}

// Closer is told when the MCP server stops.
type Closer interface {
	Close(ctx context.Context) error
}

// Command is a command-line command an extension adds. Parent names the
// built-in command it goes under ("transport" for `vsp transport <cmd>`);
// empty puts it at the top level.
type Command struct {
	Parent  string
	Command *cobra.Command
}

// EnvFunc builds the Env for a running command, from the same system,
// credentials and flags every built-in command resolves.
type EnvFunc func(cmd *cobra.Command) (Env, error)

// CommandProvider adds command-line commands. Their RunE gets the Env from
// env; the command line checks the declared Op of nothing, so a command that
// writes must go through Env.ADT(), whose client applies the safety
// configuration, or check env.ADT().Safety() itself.
type CommandProvider interface {
	Commands(env EnvFunc) []Command
}

var readOps = map[adt.OperationType]bool{
	adt.OpRead: true, adt.OpSearch: true, adt.OpQuery: true, adt.OpFreeSQL: true, adt.OpIntelligence: true,
}

var writeOps = map[adt.OperationType]bool{
	adt.OpCreate: true, adt.OpUpdate: true, adt.OpDelete: true, adt.OpActivate: true, adt.OpWorkflow: true,
	adt.OpTransport: true,
}

// Validate checks a set of extensions: each action complete and consistent,
// no two extensions with the same name, no two actions with the same action
// and type, and none that a built-in router already claims. builtin reports
// whether the core routes an action or target type itself (nil skips that
// check). The error names every conflict found, both parties of each; there
// is no "first one wins", which would make the outcome depend on argument
// order.
func Validate(exts []Extension, builtin func(literal string) bool) error {
	var problems []string
	names := map[string]int{}
	owners := map[string]string{}
	for i, ext := range exts {
		if ext == nil {
			problems = append(problems, fmt.Sprintf("extension #%d is nil", i+1))
			continue
		}
		name := strings.TrimSpace(ext.Name())
		if name == "" {
			problems = append(problems, fmt.Sprintf("extension #%d has no name", i+1))
			continue
		}
		key := strings.ToLower(name)
		if j, dup := names[key]; dup {
			problems = append(problems, fmt.Sprintf("extensions #%d and #%d are both named %q", j+1, i+1, name))
		}
		names[key] = i
		if r, ok := ext.(Requirer); ok && r.RequiredAPIVersion() > APIVersion {
			problems = append(problems, fmt.Sprintf("extension %q needs mcpext API version %d, this binary has %d", name, r.RequiredAPIVersion(), APIVersion))
		}
		if builtin != nil && builtin(strings.ToUpper(name)) {
			problems = append(problems, fmt.Sprintf("extension %q has the name of a built-in action or type", name))
		}
		for _, a := range ext.Actions() {
			where := fmt.Sprintf("%s: %q", name, a.Key())
			switch {
			case a.Action == "" || a.Action != strings.ToLower(a.Action) || strings.ContainsAny(a.Action, " \t"):
				problems = append(problems, fmt.Sprintf("%s: the action must be a lower-case word", where))
			case a.Type != strings.ToUpper(a.Type) || strings.ContainsAny(a.Type, " \t"):
				problems = append(problems, fmt.Sprintf("%s: the type must be an upper-case word", where))
			case a.Handler == nil:
				problems = append(problems, fmt.Sprintf("%s: no handler", where))
			case a.Class == Read && !readOps[a.Op]:
				problems = append(problems, fmt.Sprintf("%s: a read action must declare a read operation, not %q", where, string(a.Op)))
			case (a.Class == Mutate || a.Class == Execute) && !writeOps[a.Op]:
				problems = append(problems, fmt.Sprintf("%s: a %s action must declare an operation --read-only blocks, not %q", where, a.Class, string(a.Op)))
			case a.Class != Read && a.Class != Mutate && a.Class != Execute:
				problems = append(problems, fmt.Sprintf("%s: unknown class %d", where, int(a.Class)))
			}
			if builtin != nil {
				if a.Type != "" && builtin(a.Type) {
					problems = append(problems, fmt.Sprintf("%s: type %s is routed by a built-in router", where, a.Type))
				}
				if a.Type == "" && builtin(a.Action) {
					problems = append(problems, fmt.Sprintf("%s: action %s is routed by a built-in router", where, a.Action))
				}
			}
			if prev, dup := owners[a.Key()]; dup {
				problems = append(problems, fmt.Sprintf("%q is declared by both %s and %s", a.Key(), prev, name))
			}
			owners[a.Key()] = name
		}
	}
	if len(problems) > 0 {
		return fmt.Errorf("extensions refused:\n  %s", strings.Join(problems, "\n  "))
	}
	return nil
}
