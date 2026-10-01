package mcp

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/oisee/vibing-steampunk/pkg/adt"
)

func (s *Server) registerWriteMetadataExtension() {
	s.mcpServer.AddTool(mcp.NewTool("WriteMetadataExtension",
		mcp.WithDescription("Create or update a CDS metadata extension (DDLX/EX) with automatic create/update detection and activation."),
		mcp.WithString("name",
			mcp.Required(),
			mcp.Description("Metadata extension name, usually matching the annotated projection view (for example ZC_EMPLOYEE)."),
		),
		mcp.WithString("source_code",
			mcp.Required(),
			mcp.Description("DDLX source code."),
		),
		mcp.WithString("mode",
			mcp.Description("Operation mode: upsert (default, auto-detect), create (new only), update (existing only)"),
		),
		mcp.WithString("description",
			mcp.Description("Object description (required for create mode)"),
		),
		mcp.WithString("package_name",
			mcp.Description("Package name (required for create mode)"),
		),
		mcp.WithString("transport_request",
			mcp.Description("Transport request number"),
		),
	), s.handleWriteMetadataExtension)
}

func (s *Server) handleWriteMetadataExtension(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	name, ok := request.GetArguments()["name"].(string)
	if !ok || name == "" {
		return newToolResultError("name is required"), nil
	}

	sourceCode, ok := request.GetArguments()["source_code"].(string)
	if !ok || sourceCode == "" {
		return newToolResultError("source_code is required"), nil
	}

	mode, _ := request.GetArguments()["mode"].(string)
	description, _ := request.GetArguments()["description"].(string)
	packageName, _ := request.GetArguments()["package_name"].(string)
	transport, _ := request.GetArguments()["transport_request"].(string)

	opts := &adt.WriteMetadataExtensionOptions{
		Mode:        adt.WriteSourceMode(mode),
		Description: description,
		Package:     packageName,
		Transport:   transport,
	}

	result, err := s.adtClient.WriteMetadataExtension(ctx, name, sourceCode, opts)
	if err != nil {
		return newToolResultError(fmt.Sprintf("WriteMetadataExtension failed: %v", err)), nil
	}

	output, _ := json.MarshalIndent(result, "", "  ")
	return mcp.NewToolResultText(string(output)), nil
}
