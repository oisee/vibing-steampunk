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

	"github.com/mark3labs/mcp-go/mcp"
	openrfc "github.com/oisee/open-rfc-go/rfc"

	"github.com/oisee/vibing-steampunk/pkg/adt"
)

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
