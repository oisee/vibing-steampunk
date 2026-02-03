package adt

import (
	"context"
	"fmt"
	"strings"
)

// AbapHelpResult contains ABAP keyword documentation info.
type AbapHelpResult struct {
	Keyword       string `json:"keyword"`
	URL           string `json:"url"`
	SearchQuery   string `json:"search_query"`
	Documentation string `json:"documentation,omitempty"` // Only if ZADT_VSP available
}

// GetAbapHelpURL returns the SAP Help Portal URL for an ABAP keyword.
// The URL pattern follows SAP's standard documentation structure.
func GetAbapHelpURL(keyword string) string {
	keyword = strings.ToLower(strings.TrimSpace(keyword))
	if keyword == "" {
		return ""
	}
	// SAP Help Portal URL pattern for ABAP keywords
	// Example: https://help.sap.com/doc/abapdocu_latest_index_htm/latest/en-US/abapselect.htm
	return fmt.Sprintf("https://help.sap.com/doc/abapdocu_latest_index_htm/latest/en-US/abap%s.htm", keyword)
}

// FormatAbapHelpQuery formats a search query for ABAP keyword documentation.
func FormatAbapHelpQuery(keyword string) string {
	keyword = strings.ToUpper(strings.TrimSpace(keyword))
	if keyword == "" {
		return ""
	}
	return fmt.Sprintf("SAP ABAP %s statement syntax documentation site:help.sap.com", keyword)
}

// GetAbapHelp returns ABAP keyword documentation.
// Level 1: Always returns URL and search query.
// Level 2: If ZADT_VSP WebSocket is available, also returns real documentation from SAP system.
func (c *Client) GetAbapHelp(ctx context.Context, keyword string) (*AbapHelpResult, error) {
	keyword = strings.TrimSpace(keyword)
	if keyword == "" {
		return nil, fmt.Errorf("keyword is required")
	}

	result := &AbapHelpResult{
		Keyword:     strings.ToUpper(keyword),
		URL:         GetAbapHelpURL(keyword),
		SearchQuery: FormatAbapHelpQuery(keyword),
	}

	// Level 2: Try ZADT_VSP for real documentation (if available)
	// TODO: Implement WebSocket call to get documentation from CL_ABAP_DOCU
	// if c.wsClient != nil && c.wsClient.IsConnected() {
	//     html, err := c.wsClient.GetAbapDocumentation(ctx, keyword)
	//     if err == nil && html != "" {
	//         result.Documentation = html
	//     }
	// }

	return result, nil
}
