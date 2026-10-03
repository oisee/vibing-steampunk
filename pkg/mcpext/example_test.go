package mcpext_test

import (
	"context"
	"fmt"

	"github.com/mark3labs/mcp-go/mcp"

	"github.com/oisee/vibing-steampunk/pkg/adt"
	"github.com/oisee/vibing-steampunk/pkg/mcpext"
	"github.com/oisee/vibing-steampunk/pkg/vsp"
)

// greeter is a whole extension: one read action on a target type of its own.
type greeter struct{}

func (greeter) Name() string { return "greeter" }

func (greeter) Help() string {
	return `SAP(action="read", target="GREETING <name>") answers with a greeting.`
}

func (greeter) Actions() []mcpext.Action {
	return []mcpext.Action{{
		Action: "read",
		Type:   "GREETING",
		Class:  mcpext.Read,
		Op:     adt.OpRead,
		Handler: func(ctx context.Context, env mcpext.Env, name string, params map[string]any) (*mcp.CallToolResult, error) {
			// env.ADT() is the server's own client; a write through it passes
			// the same mutation gate as any built-in write.
			return mcp.NewToolResultText(fmt.Sprintf("hello, %s", name)), nil
		},
	}}
}

// A downstream binary is a main that passes its extensions to vsp.Run. It
// gets every built-in command and action, plus SAP(action="read",
// target="GREETING ...") and SAP(action="help", target="greeter").
func Example() {
	main := func() { vsp.Run(greeter{}) }
	_ = main // in a real binary: func main() { vsp.Run(greeter{}) }
}
