package adt

import (
	"context"
	"encoding/base64"
	"encoding/xml"
	"fmt"
	"net/http"
	"strings"
)

// SyntaxCheckResult represents a single syntax check message.
type SyntaxCheckResult struct {
	URI      string `json:"uri"`
	Line     int    `json:"line"`
	Offset   int    `json:"offset"`
	Severity string `json:"severity"` // E=Error, W=Warning, I=Info
	Text     string `json:"text"`
}

// SyntaxCheck performs syntax check on ABAP source code.
// objectURL is the ADT URL of the object (e.g., "/sap/bc/adt/programs/programs/ZTEST")
// For class includes (e.g., "/sap/bc/adt/oo/classes/ZCL_FOO/includes/testclasses"),
// pass the include URL directly - no /source/main suffix will be added.
// content is the source code to check
func (c *Client) SyntaxCheck(ctx context.Context, objectURL string, content string) ([]SyntaxCheckResult, error) {
	// Build the request body
	// For class includes, the URL is used as-is (no /source/main suffix)
	sourceURL := objectURL
	if !strings.Contains(objectURL, "/includes/") {
		sourceURL = objectURL + "/source/main"
	}
	encodedContent := base64.StdEncoding.EncodeToString([]byte(content))

	body := fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?>
<chkrun:checkObjectList xmlns:chkrun="http://www.sap.com/adt/checkrun" xmlns:adtcore="http://www.sap.com/adt/core">
  <chkrun:checkObject adtcore:uri="%s" chkrun:version="active">
    <chkrun:artifacts>
      <chkrun:artifact chkrun:contentType="text/plain; charset=utf-8" chkrun:uri="%s">
        <chkrun:content>%s</chkrun:content>
      </chkrun:artifact>
    </chkrun:artifacts>
  </chkrun:checkObject>
</chkrun:checkObjectList>`, sourceURL, sourceURL, encodedContent)

	resp, err := c.transport.Request(ctx, "/sap/bc/adt/checkruns?reporters=abapCheckRun", &RequestOptions{
		Method:      http.MethodPost,
		Body:        []byte(body),
		ContentType: "application/*",
	})
	if err != nil {
		return nil, fmt.Errorf("syntax check failed: %w", err)
	}

	return parseSyntaxCheckResults(resp.Body)
}

func parseSyntaxCheckResults(data []byte) ([]SyntaxCheckResult, error) {
	// The response uses namespace prefixes like chkrun:uri, chkrun:type, etc.
	// Go's xml package doesn't handle namespaced attributes well, so we strip them
	xmlStr := StripXMLNamespacePrefixes(data)

	type checkMessage struct {
		URI       string `xml:"uri,attr"`
		Type      string `xml:"type,attr"`
		ShortText string `xml:"shortText,attr"`
	}
	type checkMessageList struct {
		Messages []checkMessage `xml:"checkMessage"`
	}
	type checkReport struct {
		MessageList checkMessageList `xml:"checkMessageList"`
	}
	type checkRunReports struct {
		Reports []checkReport `xml:"checkReport"`
	}

	var resp checkRunReports
	if err := xml.Unmarshal([]byte(xmlStr), &resp); err != nil {
		return nil, fmt.Errorf("parsing syntax check response: %w", err)
	}

	var results []SyntaxCheckResult

	for _, report := range resp.Reports {
		for _, msg := range report.MessageList.Messages {
			// Parse base URI, line and offset from URI fragment
			baseURI, line, offset := ParseLocationFragmentWithBase(msg.URI)

			results = append(results, SyntaxCheckResult{
				URI:      baseURI,
				Severity: msg.Type,
				Text:     msg.ShortText,
				Line:     line,
				Offset:   offset,
			})
		}
	}

	return results, nil
}
