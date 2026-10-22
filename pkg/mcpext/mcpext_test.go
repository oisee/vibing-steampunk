package mcpext

import (
	"context"
	"strings"
	"testing"

	"github.com/mark3labs/mcp-go/mcp"

	"github.com/oisee/vibing-steampunk/pkg/adt"
)

type ext struct {
	name    string
	actions []Action
}

func (e ext) Name() string      { return e.name }
func (e ext) Help() string      { return "" }
func (e ext) Actions() []Action { return e.actions }

func handler(context.Context, Env, string, map[string]any) (*mcp.CallToolResult, error) {
	return nil, nil
}

func TestValidateAcceptsAConsistentSet(t *testing.T) {
	a := ext{name: "idoc", actions: []Action{
		{Action: "read", Type: "SEGM", Class: Read, Op: adt.OpRead, Handler: handler},
		{Action: "create", Type: "SEGM", Class: Mutate, Op: adt.OpCreate, Handler: handler},
	}}
	b := ext{name: "forms", actions: []Action{
		{Action: "read", Type: "SSFO", Class: Read, Op: adt.OpRead, Handler: handler},
		{Action: "importtransport", Class: Execute, Op: adt.OpWorkflow, Handler: handler},
		{Action: "import", Type: "TRANSPORT", Class: Mutate, Op: adt.OpTransport, Handler: handler},
	}}
	if err := Validate([]Extension{a, b}, func(string) bool { return false }); err != nil {
		t.Fatal(err)
	}
}

func TestValidateRefuses(t *testing.T) {
	builtin := func(l string) bool { return l == "CLAS" || l == "query" || l == "RFC" }
	for name, tc := range map[string]struct {
		exts []Extension
		want string
	}{
		"no name":            {[]Extension{ext{}}, "has no name"},
		"same name":          {[]Extension{ext{name: "a"}, ext{name: "A"}}, `both named "A"`},
		"name of a built-in": {[]Extension{ext{name: "rfc"}}, "name of a built-in"},
		"same action twice": {[]Extension{
			ext{name: "a", actions: []Action{{Action: "read", Type: "SEGM", Class: Read, Op: adt.OpRead, Handler: handler}}},
			ext{name: "b", actions: []Action{{Action: "read", Type: "SEGM", Class: Read, Op: adt.OpRead, Handler: handler}}},
		}, `"read SEGM" is declared by both a and b`},
		"built-in type": {[]Extension{ext{name: "a", actions: []Action{
			{Action: "read", Type: "CLAS", Class: Read, Op: adt.OpRead, Handler: handler}}}}, "type CLAS is routed by a built-in"},
		"built-in action": {[]Extension{ext{name: "a", actions: []Action{
			{Action: "query", Class: Read, Op: adt.OpQuery, Handler: handler}}}}, "action query is routed by a built-in"},
		"read declaring a write": {[]Extension{ext{name: "a", actions: []Action{
			{Action: "read", Type: "SEGM", Class: Read, Op: adt.OpUpdate, Handler: handler}}}}, "must declare a read operation"},
		"mutate declaring a read": {[]Extension{ext{name: "a", actions: []Action{
			{Action: "edit", Type: "SEGM", Class: Mutate, Op: adt.OpRead, Handler: handler}}}}, "--read-only blocks"},
		"execute declaring a read": {[]Extension{ext{name: "a", actions: []Action{
			{Action: "run", Type: "SEGM", Class: Execute, Op: adt.OpQuery, Handler: handler}}}}, "--read-only blocks"},
		"no class": {[]Extension{ext{name: "a", actions: []Action{
			{Action: "run", Type: "SEGM", Op: adt.OpWorkflow, Handler: handler}}}}, "unknown class"},
		"no handler": {[]Extension{ext{name: "a", actions: []Action{
			{Action: "read", Type: "SEGM", Class: Read, Op: adt.OpRead}}}}, "no handler"},
		"upper-case action": {[]Extension{ext{name: "a", actions: []Action{
			{Action: "Read", Type: "SEGM", Class: Read, Op: adt.OpRead, Handler: handler}}}}, "lower-case word"},
		"lower-case type": {[]Extension{ext{name: "a", actions: []Action{
			{Action: "read", Type: "segm", Class: Read, Op: adt.OpRead, Handler: handler}}}}, "upper-case word"},
	} {
		err := Validate(tc.exts, builtin)
		if err == nil || !strings.Contains(err.Error(), tc.want) {
			t.Errorf("%s: err = %v, want it to contain %q", name, err, tc.want)
		}
	}
}

func TestValidateNamesEveryConflict(t *testing.T) {
	err := Validate([]Extension{ext{name: "a"}, ext{name: "a"}, ext{}}, nil)
	if err == nil || !strings.Contains(err.Error(), `both named "a"`) || !strings.Contains(err.Error(), "#3 has no name") {
		t.Errorf("err = %v, want both problems", err)
	}
}
