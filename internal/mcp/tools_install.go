// Package mcp provides the MCP server implementation for ABAP ADT tools.
// tools_install.go registers install/setup tools.
package mcp

import (
	"github.com/mark3labs/mcp-go/mcp"
)

// registerInstallTools registers install/setup tools.
func (s *Server) registerInstallTools(shouldRegister func(string) bool) {
	if shouldRegister("InstallZADTVSP") {
		s.mcpServer.AddTool(mcp.NewTool("InstallZADTVSP",
			mcp.WithDescription("Deploy ZADT_VSP WebSocket handler to SAP system. Creates package and deploys 6 ABAP objects (interface + 5 classes) that enable WebSocket debugging, RFC calls, and abapGit export. After deployment, manual SAPC and SICF setup is required."),
			mcp.WithString("package",
				mcp.Description("Target package name (default: $ZADT_VSP). Must be local package starting with $."),
			),
			mcp.WithBoolean("skip_git_service",
				mcp.Description("Skip ZCL_VSP_GIT_SERVICE deployment if abapGit is not installed (default: false, auto-detected)"),
			),
			mcp.WithBoolean("check_only",
				mcp.Description("Only check prerequisites without deploying (default: false)"),
			),
		), s.handleInstallZADTVSP)
	}

	if shouldRegister("ListDependencies") {
		s.mcpServer.AddTool(mcp.NewTool("ListDependencies",
			mcp.WithDescription("List embedded dependency packages (abapGit editions and other optional dependencies) and whether each is available for DeployZip or `vsp install abapgit`."),
		), s.handleListDependencies)
	}

	if shouldRegister("DeployZip") {
		s.mcpServer.AddTool(mcp.NewTool("DeployZip",
			mcp.WithDescription("Deploy objects from an embedded abapGit-format ZIP to a SAP package. Uses ADT native deployment (PROG, CLAS, INTF, DDLS, BDEF, SRVD). For full 158 object type support, install ZADT_VSP first."),
			mcp.WithString("source",
				mcp.Required(),
				mcp.Description("Embedded dependency name: 'abapgit-standalone' (single ZABAPGIT program) or 'abapgit-full' (564 objects - full developer edition)"),
			),
			mcp.WithString("package",
				mcp.Required(),
				mcp.Description("Target SAP package name (e.g., '$ZGIT'). Package will be created if it doesn't exist."),
			),
			mcp.WithBoolean("dry_run",
				mcp.Description("Show deployment plan without deploying (default: false)"),
			),
			mcp.WithString("type_filter",
				mcp.Description("Deploy only objects of this type (e.g., 'PROG', 'CLAS', 'INTF')"),
			),
			mcp.WithString("name_filter",
				mcp.Description("Deploy only objects matching this name pattern (e.g., 'ZCL_ABAPGIT_*')"),
			),
			mcp.WithNumber("timeout",
				mcp.Description(callTimeoutDescription),
			),
		), s.handleDeployZip)
	}
}
