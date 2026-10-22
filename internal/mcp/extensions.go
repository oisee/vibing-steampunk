package mcp

import (
	"context"
	"errors"
	"fmt"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/mark3labs/mcp-go/mcp"
	openrfc "github.com/oisee/open-rfc-go/rfc"

	"github.com/oisee/vibing-steampunk/pkg/adt"
	"github.com/oisee/vibing-steampunk/pkg/config"
	"github.com/oisee/vibing-steampunk/pkg/mcpext"
	"github.com/oisee/vibing-steampunk/pkg/saprfc"
)

// Extensions (pkg/mcpext) add SAP() actions from outside this module. They
// are tried after every built-in router, and the checks run here, before the
// extension's handler: --read-only refuses a Mutate or Execute action, and the
// action's declared operation goes through the safety configuration like a
// built-in one.

// ValidateExtensions refuses a set of extensions that conflict with each
// other or with a built-in action or type. vsp.Run calls it before anything
// starts; there is no "first one wins".
func ValidateExtensions(exts []mcpext.Extension) error {
	return mcpext.Validate(exts, isBuiltinRouteLiteral)
}

// isBuiltinRouteLiteral reports whether a built-in router matches the literal
// as an action, target type, type or op. builtinRouteLiterals is checked
// against the routers' source by TestBuiltinRouteLiteralsAreCurrent.
func isBuiltinRouteLiteral(lit string) bool {
	return builtinRouteLiterals[lit] || builtinRouteLiterals[strings.ToUpper(lit)] || builtinRouteLiterals[strings.ToLower(lit)]
}

type extAction struct {
	ext    mcpext.Extension
	action mcpext.Action
}

// indexExtensions maps "action TYPE" (or "action" for an action without a
// target) to the extension action serving it.
func indexExtensions(exts []mcpext.Extension) map[string]extAction {
	if len(exts) == 0 {
		return nil
	}
	if err := ValidateExtensions(exts); err != nil {
		// vsp.Run validates before serving; a caller that skipped it built
		// the server wrong.
		panic(err)
	}
	idx := map[string]extAction{}
	for _, ext := range exts {
		for _, a := range ext.Actions() {
			idx[a.Key()] = extAction{ext: ext, action: a}
		}
	}
	return idx
}

// routeExtensionAction is the last router: an action no built-in router took,
// served by an extension that declared it.
func (s *Server) routeExtensionAction(ctx context.Context, action, objectType, objectName string, params map[string]any) (*mcp.CallToolResult, bool, error) {
	if len(s.extActions) == 0 {
		return nil, false, nil
	}
	key := action
	if objectType != "" {
		key = action + " " + objectType
	}
	ea, ok := s.extActions[key]
	if !ok {
		return nil, false, nil
	}
	name := ea.ext.Name() + " " + key
	safety := s.adtClient.Safety()
	if ea.action.Class != mcpext.Read && safety.ReadOnly {
		return newToolResultError(fmt.Sprintf("%s is blocked: read-only mode refuses a %s action", name, ea.action.Class)), true, nil
	}
	if err := safety.CheckOperation(ea.action.Op, name); err != nil {
		return newToolResultError(err.Error()), true, nil
	}
	res, err := ea.action.Handler(ctx, s.extensionEnv(ea.ext), objectName, params)
	return res, true, err
}

// serverEnv is what an extension gets on a server: the server's own clients,
// connections and task registry, and its own settings.
type serverEnv struct {
	s   *Server
	ext string // the extension's name, for its settings and log lines
}

func (s *Server) extensionEnv(ext mcpext.Extension) serverEnv {
	return serverEnv{s: s, ext: ext.Name()}
}

func (e serverEnv) ADT() *adt.Client { return e.s.adtClient }

func (e serverEnv) RFC(ctx context.Context) (*openrfc.Client, func(), error) {
	// No params: the server's own gateway and logon, never a per-call host.
	return e.s.rfcClientFor(ctx, map[string]any{})
}

func (e serverEnv) RFCDedicated(ctx context.Context, timeout time.Duration) (*openrfc.Client, error) {
	dest, err := e.s.rfcDestination(map[string]any{})
	if err != nil {
		return nil, err
	}
	c, err := saprfc.OpenWithTimeout(ctx, dest, timeout)
	if err != nil {
		return nil, fmt.Errorf("RFC logon to %s:%d failed: %w", dest.Host, dest.Port, err)
	}
	return c, nil
}

func (e serverEnv) DropRFC(ctx context.Context) { e.s.dropSharedRFC(ctx) }

func (e serverEnv) ZADTVSP(ctx context.Context) (*adt.DebugWebSocketClient, error) {
	if err := e.s.ensureDebugWSClient(ctx); err != nil {
		return nil, err
	}
	return e.s.debugWSClient, nil
}

func (e serverEnv) System() mcpext.System {
	sys := mcpext.System{URL: e.s.config.BaseURL, Client: e.s.config.Client, User: e.s.config.Username,
		Language: e.s.config.Language, ReadOnly: e.s.adtClient.Safety().ReadOnly}
	if cfg, _, err := config.LoadSystems(); err == nil && cfg != nil {
		if name, _, ok, oerr := e.s.ownSystem(cfg); oerr == nil && ok {
			sys.Name = name
		}
	}
	return sys
}

func (e serverEnv) Setting(key string) (any, bool) {
	cfg, _, err := config.LoadSystems()
	if err != nil || cfg == nil {
		return nil, false
	}
	_, sys, ok, oerr := e.s.ownSystem(cfg)
	if oerr != nil || !ok {
		return nil, false
	}
	return sys.ExtensionSetting(e.ext, key)
}

func (e serverEnv) StartAsync(kind string, fn func(ctx context.Context) (any, error)) (string, error) {
	return e.s.startAsyncTask(kind, fn), nil
}

func (e serverEnv) Logf(format string, args ...any) {
	if e.s.config.Verbose {
		fmt.Fprintf(os.Stderr, "[%s] %s\n", e.ext, fmt.Sprintf(format, args...))
	}
}

// startAsyncTask runs fn in the background under the registry
// GET_ASYNC_RESULT reads.
func (s *Server) startAsyncTask(kind string, fn func(ctx context.Context) (any, error)) string {
	s.asyncTasksMu.Lock()
	s.asyncTaskID++
	id := fmt.Sprintf("%s_%d_%d", kind, time.Now().Unix(), s.asyncTaskID)
	task := &AsyncTask{ID: id, Type: kind, Status: "running", StartedAt: time.Now()}
	s.asyncTasks[id] = task
	s.asyncTasksMu.Unlock()
	go func() {
		var (
			result any
			err    error
		)
		func() {
			defer func() {
				if r := recover(); r != nil {
					err = fmt.Errorf("task panicked: %v", r)
				}
			}()
			result, err = fn(context.Background())
		}()
		s.asyncTasksMu.Lock()
		defer s.asyncTasksMu.Unlock()
		now := time.Now()
		task.EndedAt = &now
		// A failed task keeps what it returned besides the error: an import
		// that TMS refused still reports the steps tp ran.
		task.Result = result
		if err != nil {
			task.Status, task.Error = "error", err.Error()
			return
		}
		task.Status = "completed"
	}()
	return id
}

// StartExtensions tells the extensions that implement mcpext.Starter that the
// server starts serving. vsp.Run calls it before serving; an error stops it.
func (s *Server) StartExtensions(ctx context.Context) error {
	for _, ext := range s.config.Extensions {
		if st, ok := ext.(mcpext.Starter); ok {
			if err := st.Start(ctx, s.extensionEnv(ext)); err != nil {
				return fmt.Errorf("extension %s: %w", ext.Name(), err)
			}
		}
	}
	return nil
}

// CloseExtensions tells the extensions that implement mcpext.Closer that the
// server stops. Every one is told; the errors are joined.
func (s *Server) CloseExtensions(ctx context.Context) error {
	var errs []error
	for _, ext := range s.config.Extensions {
		if c, ok := ext.(mcpext.Closer); ok {
			if err := c.Close(ctx); err != nil {
				errs = append(errs, fmt.Errorf("extension %s: %w", ext.Name(), err))
			}
		}
	}
	return errors.Join(errs...)
}

// extensionHelp answers SAP(action="help", target=<extension name>).
func (s *Server) extensionHelp(target string) (*mcp.CallToolResult, bool) {
	target = strings.TrimSpace(target)
	if target == "" {
		return nil, false
	}
	for _, ext := range s.config.Extensions {
		if !strings.EqualFold(ext.Name(), target) {
			continue
		}
		var sb strings.Builder
		sb.WriteString(strings.TrimRight(ext.Help(), "\n"))
		sb.WriteString("\n\nActions (extension " + ext.Name() + "):\n")
		for _, line := range extensionActionLines(ext) {
			sb.WriteString("  " + line + "\n")
		}
		return mcp.NewToolResultText(sb.String()), true
	}
	return nil, false
}

// extensionsHelpSuffix lists the extensions under the general help. Empty
// without extensions, so the release binary's help is unchanged.
func (s *Server) extensionsHelpSuffix() string {
	if len(s.config.Extensions) == 0 {
		return ""
	}
	var names []string
	for _, ext := range s.config.Extensions {
		names = append(names, ext.Name())
	}
	sort.Strings(names)
	return "\n\nExtensions: " + strings.Join(names, ", ") +
		` — SAP(action="help", target="<name>") for each.`
}

// extensionsInfoLine is SAP(action="info")'s line about the extensions, with
// their versions; empty without extensions.
func (s *Server) extensionsInfoLine() string {
	if len(s.config.Extensions) == 0 {
		return ""
	}
	var parts []string
	for _, ext := range s.config.Extensions {
		v := "unversioned"
		if ver, ok := ext.(mcpext.Versioned); ok && ver.Version() != "" {
			v = ver.Version()
		}
		parts = append(parts, ext.Name()+" "+v)
	}
	sort.Strings(parts)
	return "  extensions   " + strings.Join(parts, ", ") + "\n"
}

// withHelpSuffix appends suffix to the general help (no target); any other
// help, and a help without extensions, comes back unchanged.
func withHelpSuffix(res *mcp.CallToolResult, target, suffix string) *mcp.CallToolResult {
	if suffix == "" || strings.TrimSpace(target) != "" || res == nil || len(res.Content) == 0 {
		return res
	}
	if text, ok := res.Content[0].(mcp.TextContent); ok {
		text.Text += suffix
		res.Content[0] = text
	}
	return res
}

func extensionActionLines(ext mcpext.Extension) []string {
	var out []string
	for _, a := range ext.Actions() {
		target := ""
		if a.Type != "" {
			target = fmt.Sprintf(`, target="%s <name>"`, a.Type)
		}
		out = append(out, fmt.Sprintf(`SAP(action="%s"%s)   %s`, a.Action, target, a.Class))
	}
	sort.Strings(out)
	return out
}
