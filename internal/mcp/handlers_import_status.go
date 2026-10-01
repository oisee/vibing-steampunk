// Package mcp provides the MCP server implementation for ABAP ADT tools.
// handlers_import_status.go reports what tp did with requests in the connected
// system, from TPALOG. It reads only; vsp never imports.
package mcp

import (
	"context"
	"fmt"
	"strings"

	"github.com/mark3labs/mcp-go/mcp"

	"github.com/oisee/vibing-steampunk/pkg/saprfc"
)

// handleImportStatus reports the tp steps TPALOG holds for requests in this
// server's own system, and the worst return code of each: whether a request
// has been imported here, and how it went, without STMS.
//
//	SAP(action="system", params={"type": "import_status", "transport": "TR-EXAMPLE"})
//	SAP(action="system", params={"type": "import_status", "transport": ["TR-A", "TR-B"], "since": "20260101"})
func (s *Server) handleImportStatus(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	args := request.GetArguments()
	requests := transportList(args["transport"])
	if len(requests) == 0 {
		requests = transportList(args["transports"])
	}
	if len(requests) == 0 {
		return newToolResultError("transport (one request, a comma-separated list or an array) is required"), nil
	}
	for i, r := range requests {
		r = strings.ToUpper(r)
		if err := s.adtClient.CheckTransportBufferRead(r, "ImportStatus"); err != nil {
			return newToolResultError(err.Error()), nil
		}
		requests[i] = r
	}
	since := getStringParam(args, "since")
	if err := saprfc.CheckImportLogArgs(requests, since); err != nil {
		return newToolResultError(err.Error()), nil
	}
	if err := s.checkOwnTarget(args); err != nil {
		return newToolResultError(err.Error()), nil
	}
	c, release, err := s.rfcClientFor(ctx, args)
	if err != nil {
		return newToolResultError(fmt.Sprintf("reading TPALOG needs classic RFC to this system: %v", err)), nil
	}
	defer release()
	logs, err := saprfc.ReadImportLog(ctx, c, requests, since)
	if err != nil {
		return newToolResultError(err.Error()), nil
	}
	return newToolResultJSON(logs), nil
}

// transportList reads one request, a comma-separated list, or an array.
func transportList(v any) []string {
	var out []string
	switch t := v.(type) {
	case string:
		for _, p := range strings.Split(t, ",") {
			if p = strings.TrimSpace(p); p != "" {
				out = append(out, p)
			}
		}
	case []any:
		for _, p := range t {
			if str, ok := p.(string); ok && strings.TrimSpace(str) != "" {
				out = append(out, strings.TrimSpace(str))
			}
		}
	}
	return out
}
