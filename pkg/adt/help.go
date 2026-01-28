// ABOUTME: ABAP keyword documentation helpers.
// ABOUTME: Provides URLs and search queries for ABAP keyword documentation.

package adt

import (
	"fmt"
	"strings"
)

// GetAbapHelpURL returns the SAP Help Portal URL for an ABAP keyword.
// The actual documentation should be fetched via web search (e.g., Exa)
// for better AI-consumable plain text content.
func GetAbapHelpURL(keyword string) string {
	keyword = strings.ToLower(strings.TrimSpace(keyword))
	if keyword == "" {
		return ""
	}
	// SAP Help Portal URL format for ABAP keywords
	return fmt.Sprintf("https://help.sap.com/doc/abapdocu_latest_index_htm/latest/en-US/abap%s.htm", keyword)
}

// FormatAbapHelpQuery formats a search query for ABAP keyword documentation.
// Use this with web search tools like Exa to get documentation.
func FormatAbapHelpQuery(keyword string) string {
	keyword = strings.ToUpper(strings.TrimSpace(keyword))
	return fmt.Sprintf("SAP ABAP %s statement syntax documentation site:help.sap.com", keyword)
}
