# ABAP Help Tool Design

**Date:** 2026-02-01
**Report ID:** 002
**Subject:** GetAbapHelp tool with two-level implementation
**Related Issue:** #10

---

## Overview

Implement ABAP keyword documentation lookup with graceful degradation:
- **Level 1 (always):** URL + search query for help.sap.com
- **Level 2 (if ZADT_VSP):** Real documentation from SAP system

---

## Response Structure

```json
{
  "keyword": "SELECT",
  "url": "https://help.sap.com/doc/abapdocu_latest_index_htm/latest/en-US/abapselect.htm",
  "search_query": "SAP ABAP SELECT statement syntax documentation site:help.sap.com",
  "documentation": "<html>...</html>"  // Only if ZADT_VSP available
}
```

---

## Architecture

```
┌─────────────────────────────────────────────────────────────────┐
│  GetAbapHelp(keyword="SELECT")                                  │
├─────────────────────────────────────────────────────────────────┤
│                                                                 │
│  ALWAYS RETURN:                                                 │
│  ├── url: https://help.sap.com/.../abapselect.htm              │
│  └── search_query: "SAP ABAP SELECT site:help.sap.com"         │
│                                                                 │
│  ┌─────────────────┐                                            │
│  │ ZADT_VSP        │ Yes ──► Also return:                       │
│  │ installed?      │         └── documentation: <html>...</html>│
│  └────────┬────────┘                                            │
│           │ No                                                  │
│           └──► URL + search_query only                          │
│                                                                 │
└─────────────────────────────────────────────────────────────────┘
```

---

## Implementation Plan

### Level 1: URL Helper (pkg/adt/help.go)

Cherry-pick from `one-tool-mode` branch or implement:

```go
package adt

import (
    "fmt"
    "strings"
)

// AbapHelpResult contains ABAP keyword documentation info
type AbapHelpResult struct {
    Keyword       string `json:"keyword"`
    URL           string `json:"url"`
    SearchQuery   string `json:"search_query"`
    Documentation string `json:"documentation,omitempty"` // Only if ZADT_VSP
}

// GetAbapHelpURL returns the SAP Help Portal URL for an ABAP keyword
func GetAbapHelpURL(keyword string) string {
    keyword = strings.ToLower(strings.TrimSpace(keyword))
    if keyword == "" {
        return ""
    }
    return fmt.Sprintf("https://help.sap.com/doc/abapdocu_latest_index_htm/latest/en-US/abap%s.htm", keyword)
}

// FormatAbapHelpQuery formats a search query for ABAP keyword documentation
func FormatAbapHelpQuery(keyword string) string {
    keyword = strings.ToUpper(strings.TrimSpace(keyword))
    return fmt.Sprintf("SAP ABAP %s statement syntax documentation site:help.sap.com", keyword)
}
```

### Level 2: ZADT_VSP Method (embedded/abap/zcl_vsp_service.clas.abap)

Add method to existing WebSocket service:

```abap
METHOD get_abap_documentation.
  " Input: iv_keyword TYPE string
  " Output: rv_html TYPE string

  DATA lt_html TYPE abapdocu_html_tab.

  TRY.
      cl_abap_docu=>convert_itf_to_html(
        EXPORTING
          area   = 'ABEN'
          name   = to_upper( iv_keyword )
          langu  = sy-langu
        IMPORTING
          html   = lt_html ).

      " Concatenate HTML lines
      LOOP AT lt_html INTO DATA(lv_line).
        rv_html = rv_html && lv_line.
      ENDLOOP.

    CATCH cx_abap_docu_conversion INTO DATA(lx_error).
      rv_html = |Error: { lx_error->get_text( ) }|.
  ENDTRY.
ENDMETHOD.
```

### Level 3: Go WebSocket Client (pkg/adt/help.go)

```go
// GetAbapHelp returns ABAP keyword documentation
// Uses ZADT_VSP if available, otherwise returns URL only
func (c *Client) GetAbapHelp(ctx context.Context, keyword string) (*AbapHelpResult, error) {
    result := &AbapHelpResult{
        Keyword:     strings.ToUpper(keyword),
        URL:         GetAbapHelpURL(keyword),
        SearchQuery: FormatAbapHelpQuery(keyword),
    }

    // Try ZADT_VSP for real documentation
    if c.wsClient != nil && c.wsClient.IsConnected() {
        html, err := c.wsClient.GetAbapDocumentation(ctx, keyword)
        if err == nil && html != "" {
            result.Documentation = html
        }
    }

    return result, nil
}
```

### Level 4: MCP Tool (internal/mcp/server.go)

```go
// Register tool
s.mcpServer.AddTool(mcp.NewTool("GetAbapHelp",
    mcp.WithDescription("Get ABAP keyword documentation. Returns URL to SAP Help Portal and optionally full documentation if ZADT_VSP is installed."),
    mcp.WithString("keyword",
        mcp.Required(),
        mcp.Description("ABAP keyword (e.g., SELECT, LOOP, DATA, METHOD)"),
    ),
), s.handleGetAbapHelp)

// Handler
func (s *Server) handleGetAbapHelp(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
    keyword, _ := req.Params.Arguments["keyword"].(string)
    if keyword == "" {
        return mcp.NewToolResultError("keyword is required"), nil
    }

    result, err := s.adtClient.GetAbapHelp(ctx, keyword)
    if err != nil {
        return mcp.NewToolResultError(err.Error()), nil
    }

    // Format output
    var sb strings.Builder
    fmt.Fprintf(&sb, "ABAP Keyword: %s\n\n", result.Keyword)
    fmt.Fprintf(&sb, "Documentation URL:\n  %s\n\n", result.URL)
    fmt.Fprintf(&sb, "Search Query:\n  %s\n", result.SearchQuery)

    if result.Documentation != "" {
        fmt.Fprintf(&sb, "\n---\nDocumentation from SAP system:\n\n%s", result.Documentation)
    }

    return mcp.NewToolResultText(sb.String()), nil
}
```

---

## Testing

### Unit Tests
```go
func TestGetAbapHelpURL(t *testing.T) {
    tests := []struct {
        keyword string
        want    string
    }{
        {"SELECT", "https://help.sap.com/doc/abapdocu_latest_index_htm/latest/en-US/abapselect.htm"},
        {"LOOP", "https://help.sap.com/doc/abapdocu_latest_index_htm/latest/en-US/abaploop.htm"},
        {"", ""},
    }
    // ...
}
```

### Integration Test
```go
func TestGetAbapHelp_Integration(t *testing.T) {
    // Test with ZADT_VSP
    result, err := client.GetAbapHelp(ctx, "SELECT")
    require.NoError(t, err)
    assert.NotEmpty(t, result.URL)
    assert.NotEmpty(t, result.SearchQuery)
    // Documentation may or may not be present depending on ZADT_VSP
}
```

---

## Example Usage

```
User: "Help me understand the SELECT statement in ABAP"

LLM calls: GetAbapHelp(keyword="SELECT")

Response:
ABAP Keyword: SELECT

Documentation URL:
  https://help.sap.com/doc/abapdocu_latest_index_htm/latest/en-US/abapselect.htm

Search Query:
  SAP ABAP SELECT statement syntax documentation site:help.sap.com

---
Documentation from SAP system:

<html>
<h1>SELECT</h1>
<p>The SELECT statement reads data from database tables...</p>
...
</html>
```

---

## Files to Create/Modify

| File | Action | Description |
|------|--------|-------------|
| `pkg/adt/help.go` | Create | URL helpers + GetAbapHelp method |
| `pkg/adt/help_test.go` | Create | Unit tests |
| `embedded/abap/zcl_vsp_service.clas.abap` | Modify | Add get_abap_documentation method |
| `pkg/adt/websocket_rfc.go` | Modify | Add GetAbapDocumentation call |
| `internal/mcp/server.go` | Modify | Register GetAbapHelp tool |

---

## Status

- [x] Level 1: URL helper (pkg/adt/help.go) ✅ 2026-02-01
- [x] Level 2: ABAP method in ZADT_VSP ✅ 2026-02-01
- [x] Level 3: Go WebSocket client integration ✅ 2026-02-01
- [x] Level 4: MCP tool registration ✅ 2026-02-01
- [x] Tests (15 unit tests) ✅ 2026-02-01
- [ ] Documentation

---

## Credits

Original help.go implementation from PR #16 by Filipp Gnilyak (@vitalratel)
