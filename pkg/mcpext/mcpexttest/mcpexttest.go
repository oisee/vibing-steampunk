// Package mcpexttest helps test an extension (pkg/mcpext): a fake Env for the
// handlers' own tests, and Check, which puts the extension through the rules
// the core enforces -- the same ones vsp.Run and the read-only invariant
// apply to it -- without a SAP system.
package mcpexttest

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	mcpgo "github.com/mark3labs/mcp-go/mcp"
	openrfc "github.com/oisee/open-rfc-go/rfc"

	"github.com/oisee/vibing-steampunk/internal/mcp"
	"github.com/oisee/vibing-steampunk/pkg/adt"
	"github.com/oisee/vibing-steampunk/pkg/mcpext"
)

// Env is a fake mcpext.Env. Set what the handler under test uses; the rest
// answers "not configured". StartAsync runs the task at once and records it.
type Env struct {
	Client     *adt.Client
	RFCFunc    func(ctx context.Context) (*openrfc.Client, error)
	WebSocket  *adt.DebugWebSocketClient
	SystemInfo mcpext.System
	Settings   map[string]any
	mu         sync.Mutex
	Tasks      []Task
	Logs       []string
	RFCDrops   int
}

// Task is a background task an extension started.
type Task struct {
	Kind   string
	Result any
	Err    error
}

func (e *Env) ADT() *adt.Client { return e.Client }

func (e *Env) RFC(ctx context.Context) (*openrfc.Client, func(), error) {
	c, err := e.RFCDedicated(ctx, 0)
	if err != nil {
		return nil, nil, err
	}
	return c, func() {}, nil
}

func (e *Env) RFCDedicated(ctx context.Context, _ time.Duration) (*openrfc.Client, error) {
	if e.RFCFunc == nil {
		return nil, fmt.Errorf("mcpexttest: no RFC configured")
	}
	return e.RFCFunc(ctx)
}

func (e *Env) DropRFC(context.Context) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.RFCDrops++
}

func (e *Env) ZADTVSP(context.Context) (*adt.DebugWebSocketClient, error) {
	if e.WebSocket == nil {
		return nil, fmt.Errorf("mcpexttest: no ZADT_VSP WebSocket configured")
	}
	return e.WebSocket, nil
}

func (e *Env) System() mcpext.System { return e.SystemInfo }

func (e *Env) Setting(key string) (any, bool) {
	v, ok := e.Settings[key]
	return v, ok
}

func (e *Env) StartAsync(kind string, fn func(ctx context.Context) (any, error)) (string, error) {
	result, err := fn(context.Background())
	e.mu.Lock()
	defer e.mu.Unlock()
	e.Tasks = append(e.Tasks, Task{Kind: kind, Result: result, Err: err})
	return fmt.Sprintf("%s_%d", kind, len(e.Tasks)), nil
}

func (e *Env) Logf(format string, args ...any) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.Logs = append(e.Logs, fmt.Sprintf(format, args...))
}

// Check puts extensions through the core's rules:
//   - they are accepted by vsp.Run: complete actions, no conflict with each
//     other or with a built-in action or type;
//   - under --read-only every Mutate and Execute action is refused before its
//     handler runs;
//   - SAP(action="help", target=<name>) answers with each action;
//   - SAP(action="help") lists them.
//
// No SAP system is needed: nothing reaches one.
func Check(t testing.TB, exts ...mcpext.Extension) {
	t.Helper()
	if err := mcp.ValidateExtensions(exts); err != nil {
		t.Fatalf("vsp.Run would refuse these extensions: %v", err)
	}

	calls := map[string]int{}
	var mu sync.Mutex
	counted := make([]mcpext.Extension, len(exts))
	for i, ext := range exts {
		counted[i] = countingExtension{Extension: ext, count: func(key string) {
			mu.Lock()
			defer mu.Unlock()
			calls[key]++
		}}
	}
	// No SAP behind this address: a request that got out would fail, not
	// change anything.
	srv := mcp.NewServer(&mcp.Config{BaseURL: "http://127.0.0.1:1", Username: "u", Password: "p", Client: "000",
		Mode: "hyperfocused", ReadOnly: true, Extensions: counted})
	call := func(args map[string]any) *mcpgo.CallToolResult {
		tool, ok := srv.GetMCPServer().ListTools()["SAP"]
		if !ok {
			t.Fatal("mcpexttest: the server has no SAP tool")
		}
		res, err := tool.Handler(context.Background(), mcpgo.CallToolRequest{Params: mcpgo.CallToolParams{Name: "SAP", Arguments: args}})
		if err != nil {
			t.Fatalf("SAP %v: %v", args, err)
		}
		return res
	}

	for _, ext := range exts {
		for _, a := range ext.Actions() {
			if a.Class == mcpext.Read {
				continue
			}
			args := map[string]any{"action": a.Action}
			if a.Type != "" {
				args["target"] = a.Type + " ZMCPEXTTEST"
			}
			res := call(args)
			mu.Lock()
			n := calls[ext.Name()+" "+a.Key()]
			mu.Unlock()
			if !res.IsError || n != 0 {
				t.Errorf("%s %s (%s): under --read-only it must be refused before the handler runs; error=%t, handler ran %d times",
					ext.Name(), a.Key(), a.Class, res.IsError, n)
			}
		}
		help := text(call(map[string]any{"action": "help", "target": ext.Name()}))
		for _, a := range ext.Actions() {
			if !strings.Contains(help, fmt.Sprintf(`SAP(action="%s"`, a.Action)) {
				t.Errorf("%s: SAP(action=\"help\", target=%q) does not show %s", ext.Name(), ext.Name(), a.Key())
			}
		}
	}
	general := text(call(map[string]any{"action": "help"}))
	for _, ext := range exts {
		if !strings.Contains(general, ext.Name()) {
			t.Errorf("SAP(action=\"help\") does not list extension %s", ext.Name())
		}
	}
}

// countingExtension is an extension whose handlers only count their calls.
type countingExtension struct {
	mcpext.Extension
	count func(key string)
}

func (c countingExtension) Actions() []mcpext.Action {
	actions := c.Extension.Actions()
	out := make([]mcpext.Action, len(actions))
	for i, a := range actions {
		key := c.Extension.Name() + " " + a.Key()
		a.Handler = func(context.Context, mcpext.Env, string, map[string]any) (*mcpgo.CallToolResult, error) {
			c.count(key)
			return mcpgo.NewToolResultText("called"), nil
		}
		out[i] = a
	}
	return out
}

func text(res *mcpgo.CallToolResult) string {
	var sb strings.Builder
	for _, c := range res.Content {
		if tc, ok := c.(mcpgo.TextContent); ok {
			sb.WriteString(tc.Text)
		}
	}
	return sb.String()
}
