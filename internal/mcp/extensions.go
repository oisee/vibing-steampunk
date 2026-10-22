package mcp

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/mark3labs/mcp-go/mcp"
	openrfc "github.com/oisee/open-rfc-go/rfc"

	"github.com/oisee/vibing-steampunk/pkg/adt"
	"github.com/oisee/vibing-steampunk/pkg/mcpext"
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
	res, err := ea.action.Handler(ctx, serverEnv{s: s}, objectName, params)
	return res, true, err
}

// serverEnv is what an extension handler gets: the server's own clients.
type serverEnv struct{ s *Server }

func (e serverEnv) ADT() *adt.Client { return e.s.adtClient }

func (e serverEnv) RFC(ctx context.Context) (*openrfc.Client, func(), error) {
	// No params: the server's own gateway and logon, never a per-call host.
	return e.s.rfcClientFor(ctx, map[string]any{})
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
