package mcpexttest_test

import (
	"context"
	"testing"

	"github.com/mark3labs/mcp-go/mcp"

	"github.com/oisee/vibing-steampunk/pkg/adt"
	"github.com/oisee/vibing-steampunk/pkg/mcpext"
	"github.com/oisee/vibing-steampunk/pkg/mcpext/mcpexttest"
)

type demo struct{}

func (demo) Name() string { return "demo" }
func (demo) Help() string { return "A demo extension." }
func (demo) Actions() []mcpext.Action {
	h := func(ctx context.Context, env mcpext.Env, name string, params map[string]any) (*mcp.CallToolResult, error) {
		if v, ok := env.Setting("greeting"); ok {
			return mcpext.Text(v.(string) + " " + name), nil
		}
		return mcpext.Text("hello " + name), nil
	}
	return []mcpext.Action{
		{Action: "read", Type: "DEMOX", Class: mcpext.Read, Op: adt.OpRead, Handler: h},
		{Action: "edit", Type: "DEMOX", Class: mcpext.Mutate, Op: adt.OpUpdate, Handler: h},
		{Action: "run", Type: "DEMOX", Class: mcpext.Execute, Op: adt.OpWorkflow, Handler: h},
	}
}

func TestCheckPassesAWellBehavedExtension(t *testing.T) {
	mcpexttest.Check(t, demo{})
}

func TestFakeEnvServesAHandler(t *testing.T) {
	env := &mcpexttest.Env{Settings: map[string]any{"greeting": "servus"}}
	res, err := demo{}.Actions()[0].Handler(context.Background(), env, "ZX", nil)
	if err != nil || res.IsError {
		t.Fatalf("handler: %v %v", res, err)
	}
	id, err := env.StartAsync("job", func(context.Context) (any, error) { return 42, nil })
	if err != nil || id == "" || len(env.Tasks) != 1 || env.Tasks[0].Result != 42 {
		t.Errorf("StartAsync: %q %v %+v", id, err, env.Tasks)
	}
	env.DropRFC(context.Background())
	if env.RFCDrops != 1 {
		t.Error("DropRFC not counted")
	}
	if _, err := env.ZADTVSP(context.Background()); err == nil {
		t.Error("an unconfigured WebSocket was handed out")
	}
}
