package mcp

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/mark3labs/mcp-go/mcp"

	"github.com/oisee/vibing-steampunk/pkg/adt"
	"github.com/oisee/vibing-steampunk/pkg/mcpext"
)

// fakeExtension declares one action of each class on type FAKEX. Its handlers
// count their calls; the mutating and executing ones would write to SAP if
// they ran, so a leak shows in the read-only invariant as well as here.
type fakeExtension struct {
	name  string
	calls map[string]*atomic.Int32
}

func newFakeExtension() *fakeExtension {
	return &fakeExtension{name: "fakeext", calls: map[string]*atomic.Int32{
		"read": {}, "edit": {}, "run": {},
	}}
}

func (f *fakeExtension) Name() string { return f.name }

func (f *fakeExtension) Help() string { return "A fake extension for tests." }

func (f *fakeExtension) Actions() []mcpext.Action {
	return []mcpext.Action{
		{Action: "read", Type: "FAKEX", Class: mcpext.Read, Op: adt.OpRead,
			Handler: func(ctx context.Context, env mcpext.Env, name string, params map[string]any) (*mcp.CallToolResult, error) {
				f.calls["read"].Add(1)
				return mcp.NewToolResultText("read " + name), nil
			}},
		{Action: "edit", Type: "FAKEX", Class: mcpext.Mutate, Op: adt.OpUpdate,
			Handler: func(ctx context.Context, env mcpext.Env, name string, params map[string]any) (*mcp.CallToolResult, error) {
				f.calls["edit"].Add(1)
				// A write through the server's own client, the way a real
				// extension would make one.
				_, err := env.ADT().Activate(ctx, "/sap/bc/adt/programs/programs/zdemo_report", "ZDEMO_REPORT")
				return mcp.NewToolResultText("edited " + name), err
			}},
		{Action: "run", Type: "FAKEX", Class: mcpext.Execute, Op: adt.OpWorkflow,
			Handler: func(ctx context.Context, env mcpext.Env, name string, params map[string]any) (*mcp.CallToolResult, error) {
				f.calls["run"].Add(1)
				return mcp.NewToolResultText("ran " + name), nil
			}},
	}
}

// extensionCases are the SAP() calls the read-only invariant makes for the
// extensions' actions, with each action's declared class as its
// classification.
func extensionCases(exts []mcpext.Extension) ([]actionCase, map[string]surfaceClass) {
	var cases []actionCase
	classes := map[string]surfaceClass{}
	for _, ext := range exts {
		for _, a := range ext.Actions() {
			c := actionCase{Name: "ext " + ext.Name() + " " + a.Key(), Action: a.Action, Exact: true}
			if a.Type != "" {
				c.Target = a.Type + " ZDEMO_REPORT"
			}
			cases = append(cases, c)
			switch a.Class {
			case mcpext.Read:
				classes[c.Name] = clsRead
			case mcpext.Mutate:
				classes[c.Name] = clsMutate
			case mcpext.Execute:
				classes[c.Name] = clsExecute
			}
		}
	}
	return cases, classes
}

func newExtensionServer(t *testing.T, readOnly bool, exts ...mcpext.Extension) *Server {
	t.Helper()
	sap := newFakeSAP(t)
	return NewServer(&Config{BaseURL: sap.srv.URL, Username: "u", Password: "p", Client: "001",
		Mode: "hyperfocused", ReadOnly: readOnly, Extensions: exts})
}

func callSAP(t *testing.T, s *Server, action, target string) *mcp.CallToolResult {
	t.Helper()
	res, err := s.handleUniversalTool(context.Background(), newRequest(map[string]any{"action": action, "target": target}))
	if err != nil {
		t.Fatalf("SAP %s %s: %v", action, target, err)
	}
	return res
}

func TestExtensionActionIsRoutedWithItsName(t *testing.T) {
	ext := newFakeExtension()
	s := newExtensionServer(t, false, ext)
	res := callSAP(t, s, "read", "FAKEX zdemo")
	if res.IsError || resultText(res) != "read ZDEMO" || ext.calls["read"].Load() != 1 {
		t.Errorf("read FAKEX: %q (error %t), %d calls", resultText(res), res.IsError, ext.calls["read"].Load())
	}
}

func TestExtensionMutationRefusedUnderReadOnlyBeforeItsHandler(t *testing.T) {
	ext := newFakeExtension()
	s := newExtensionServer(t, true, ext)
	for _, action := range []string{"edit", "run"} {
		res := callSAP(t, s, action, "FAKEX ZDEMO")
		if !res.IsError || !strings.Contains(resultText(res), "read-only") {
			t.Errorf("%s FAKEX under --read-only: %q (error %t), want a read-only refusal", action, resultText(res), res.IsError)
		}
		if n := ext.calls[action].Load(); n != 0 {
			t.Errorf("%s FAKEX: the handler ran %d times under --read-only", action, n)
		}
	}
	// Reading is still allowed.
	if res := callSAP(t, s, "read", "FAKEX ZDEMO"); res.IsError || ext.calls["read"].Load() != 1 {
		t.Errorf("read FAKEX under --read-only: %q", resultText(res))
	}
}

func TestExtensionDisallowedOpRefusedBeforeItsHandler(t *testing.T) {
	ext := newFakeExtension()
	sap := newFakeSAP(t)
	s := NewServer(&Config{BaseURL: sap.srv.URL, Username: "u", Password: "p", Client: "001",
		Mode: "hyperfocused", DisallowedOps: "U", Extensions: []mcpext.Extension{ext}})
	res := callSAP(t, s, "edit", "FAKEX ZDEMO")
	if !res.IsError || ext.calls["edit"].Load() != 0 {
		t.Errorf("edit FAKEX with update disallowed: %q, %d calls", resultText(res), ext.calls["edit"].Load())
	}
}

// A transport action needs --enable-transports, as a built-in one does, and
// --read-only refuses it like any other change.
func TestExtensionTransportActionNeedsTransportsEnabled(t *testing.T) {
	var calls atomic.Int32
	ext := staticExtension{name: "transportext", actions: []mcpext.Action{{Action: "import", Type: "FAKETR",
		Class: mcpext.Mutate, Op: adt.OpTransport,
		Handler: func(context.Context, mcpext.Env, string, map[string]any) (*mcp.CallToolResult, error) {
			calls.Add(1)
			return mcp.NewToolResultText("imported"), nil
		}}}}
	sap := newFakeSAP(t)
	for name, tc := range map[string]struct {
		cfg  Config
		runs bool
	}{
		"transports off":           {Config{}, false},
		"transports on":            {Config{EnableTransports: true}, true},
		"transports on, read-only": {Config{EnableTransports: true, ReadOnly: true}, false},
		"transport ops disallowed": {Config{EnableTransports: true, DisallowedOps: "X"}, false},
	} {
		calls.Store(0)
		cfg := tc.cfg
		cfg.BaseURL, cfg.Username, cfg.Password, cfg.Client, cfg.Mode = sap.srv.URL, "u", "p", "001", "hyperfocused"
		cfg.Extensions = []mcpext.Extension{ext}
		res := callSAP(t, NewServer(&cfg), "import", "FAKETR X1")
		if ran := calls.Load() == 1; ran != tc.runs || res.IsError == tc.runs {
			t.Errorf("%s: %q (error %t), handler ran %t, want %t", name, resultText(res), res.IsError, ran, tc.runs)
		}
	}
}

func TestExtensionCannotShadowABuiltIn(t *testing.T) {
	for name, a := range map[string]mcpext.Action{
		"built-in type":   {Action: "read", Type: "CLAS", Class: mcpext.Read, Op: adt.OpRead},
		"built-in action": {Action: "query", Class: mcpext.Read, Op: adt.OpQuery},
	} {
		a.Handler = func(context.Context, mcpext.Env, string, map[string]any) (*mcp.CallToolResult, error) {
			return nil, nil
		}
		err := ValidateExtensions([]mcpext.Extension{staticExtension{name: "shadow", actions: []mcpext.Action{a}}})
		if err == nil || !strings.Contains(err.Error(), "built-in") {
			t.Errorf("%s: err = %v, want a refusal naming the built-in", name, err)
		}
	}
	// A built-in claims a call first, so a server given such an extension
	// anyway (without vsp.Run's check) would never route to it: refuse to
	// build it at all.
	defer func() {
		if recover() == nil {
			t.Error("a server was built with an extension that shadows a built-in")
		}
	}()
	newExtensionServer(t, false, staticExtension{name: "shadow", actions: []mcpext.Action{{Action: "read", Type: "PROG",
		Class: mcpext.Read, Op: adt.OpRead,
		Handler: func(context.Context, mcpext.Env, string, map[string]any) (*mcp.CallToolResult, error) {
			return nil, nil
		}}}})
}

func TestExtensionsThatCollideAreRefused(t *testing.T) {
	a, b := newFakeExtension(), newFakeExtension()
	err := ValidateExtensions([]mcpext.Extension{a, b})
	if err == nil || !strings.Contains(err.Error(), `both named "fakeext"`) || !strings.Contains(err.Error(), `"edit FAKEX" is declared by both`) {
		t.Errorf("err = %v, want both the name and the action collision named", err)
	}
}

func TestExtensionAppearsInHelp(t *testing.T) {
	s := newExtensionServer(t, false, newFakeExtension())
	general := resultText(callSAP(t, s, "help", ""))
	if !strings.Contains(general, "Extensions: fakeext") {
		t.Errorf("general help does not list the extension:\n%s", general)
	}
	own := resultText(callSAP(t, s, "help", "fakeext"))
	for _, want := range []string{"A fake extension for tests.", `SAP(action="edit", target="FAKEX <name>")   mutate`} {
		if !strings.Contains(own, want) {
			t.Errorf("SAP help fakeext lacks %q:\n%s", want, own)
		}
	}
	// Without extensions the help is what it always was.
	plain := newExtensionServer(t, false)
	if got, want := resultText(callSAP(t, plain, "help", "")), resultText(handleHelp("")); got != want {
		t.Error("the general help changed for a server without extensions")
	}
}

func TestBuiltinRouteLiteralsAreCurrent(t *testing.T) {
	lits, _ := routedLiterals(t)
	want := map[string]bool{}
	for _, set := range lits {
		for l := range set {
			want[l] = true
		}
	}
	var missing, extra []string
	for l := range want {
		if !builtinRouteLiterals[l] {
			missing = append(missing, l)
		}
	}
	for l := range builtinRouteLiterals {
		if !want[l] {
			extra = append(extra, l)
		}
	}
	if len(missing) == 0 && len(extra) == 0 {
		return
	}
	all := make([]string, 0, len(want))
	for l := range want {
		all = append(all, l)
	}
	sort.Strings(all)
	var sb strings.Builder
	sb.WriteString("var builtinRouteLiterals = map[string]bool{\n")
	for _, l := range all {
		fmt.Fprintf(&sb, "\t%q: true,\n", l)
	}
	sb.WriteString("}\n")
	sort.Strings(missing)
	sort.Strings(extra)
	t.Errorf("builtinRouteLiterals is out of date (missing %v, extra %v); replace it in builtin_routes.go with:\n%s", missing, extra, sb.String())
}

// staticExtension is an extension with fixed actions.
type staticExtension struct {
	name    string
	actions []mcpext.Action
}

func (e staticExtension) Name() string             { return e.name }
func (e staticExtension) Help() string             { return "" }
func (e staticExtension) Actions() []mcpext.Action { return e.actions }
