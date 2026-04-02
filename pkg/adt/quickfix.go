package adt

import (
	"context"
	"encoding/xml"
	"fmt"
	"net/http"
	"net/url"
	"strings"
)

// --- Quick Fix Proposals ---

// QuickFixProposal represents a single auto-fix suggestion from the ADT quickfix engine.
type QuickFixProposal struct {
	ID          string `json:"id" xml:"id,attr"`
	Title       string `json:"title" xml:"title,attr"`
	Description string `json:"description,omitempty" xml:"description,attr"`
}

// QuickFixProposalsResult is the result of querying quick fix proposals.
type QuickFixProposalsResult struct {
	Proposals []QuickFixProposal `json:"proposals"`
}

// QuickFixApplyResult is the result of applying a quick fix.
type QuickFixApplyResult struct {
	Success   bool   `json:"success"`
	NewSource string `json:"newSource,omitempty"`
	Message   string `json:"message,omitempty"`
}

// GetQuickFixProposals retrieves available quick fix suggestions at a given position.
// objectURI is the ADT object URI (e.g., "/sap/bc/adt/programs/programs/ZTEST")
// line and col are 1-based position of the error/warning.
// source is the current source code.
func (c *Client) GetQuickFixProposals(ctx context.Context, objectURI string, line, col int, source string) (*QuickFixProposalsResult, error) {
	if err := c.checkSafety(OpRead, "GetQuickFixProposals"); err != nil {
		return nil, err
	}

	q := url.Values{}
	q.Set("uri", objectURI)
	q.Set("line", fmt.Sprintf("%d", line))
	q.Set("column", fmt.Sprintf("%d", col))

	resp, err := c.transport.Request(ctx, "/sap/bc/adt/quickfix/proposals", &RequestOptions{
		Method:      http.MethodPost,
		Query:       q,
		Body:        []byte(source),
		ContentType: "text/plain",
		Accept:      "application/xml",
	})
	if err != nil {
		return nil, fmt.Errorf("get quickfix proposals failed: %w", err)
	}

	return parseQuickFixProposals(resp.Body)
}

func parseQuickFixProposals(data []byte) (*QuickFixProposalsResult, error) {
	xmlStr := string(data)
	xmlStr = strings.ReplaceAll(xmlStr, "quickfix:", "")

	type proposal struct {
		ID          string `xml:"id,attr"`
		Title       string `xml:"title,attr"`
		Description string `xml:"description,attr"`
	}
	type proposals struct {
		XMLName xml.Name   `xml:"proposals"`
		Items   []proposal `xml:"proposal"`
	}

	var resp proposals
	if err := xml.Unmarshal([]byte(xmlStr), &resp); err != nil {
		return nil, fmt.Errorf("parsing quickfix proposals: %w", err)
	}

	result := &QuickFixProposalsResult{}
	for _, p := range resp.Items {
		result.Proposals = append(result.Proposals, QuickFixProposal{
			ID:          p.ID,
			Title:       p.Title,
			Description: p.Description,
		})
	}
	return result, nil
}

// ApplyQuickFix applies a specific quick fix proposal.
// proposalID is the ID from GetQuickFixProposals.
// source is the current source code.
func (c *Client) ApplyQuickFix(ctx context.Context, objectURI string, proposalID string, line, col int, source string) (*QuickFixApplyResult, error) {
	if err := c.checkSafety(OpUpdate, "ApplyQuickFix"); err != nil {
		return nil, err
	}

	q := url.Values{}
	q.Set("uri", objectURI)
	q.Set("proposalId", proposalID)
	q.Set("line", fmt.Sprintf("%d", line))
	q.Set("column", fmt.Sprintf("%d", col))

	resp, err := c.transport.Request(ctx, "/sap/bc/adt/quickfix/apply", &RequestOptions{
		Method:      http.MethodPost,
		Query:       q,
		Body:        []byte(source),
		ContentType: "text/plain",
		Accept:      "application/xml, text/plain",
	})
	if err != nil {
		return nil, fmt.Errorf("apply quickfix failed: %w", err)
	}

	return parseQuickFixApplyResult(resp.Body)
}

func parseQuickFixApplyResult(data []byte) (*QuickFixApplyResult, error) {
	body := string(data)

	// If response is plain text, it's the new source code
	if !strings.Contains(body, "<?xml") && !strings.Contains(body, "<quickfix") {
		return &QuickFixApplyResult{
			Success:   true,
			NewSource: body,
		}, nil
	}

	// XML response
	xmlStr := strings.ReplaceAll(body, "quickfix:", "")

	type result struct {
		XMLName   xml.Name `xml:"result"`
		Status    string   `xml:"status,attr"`
		NewSource string   `xml:"newSource"`
		Message   string   `xml:"message"`
	}

	var resp result
	if err := xml.Unmarshal([]byte(xmlStr), &resp); err != nil {
		return nil, fmt.Errorf("parsing quickfix apply result: %w", err)
	}

	return &QuickFixApplyResult{
		Success:   resp.Status == "OK" || resp.Status == "success",
		NewSource: resp.NewSource,
		Message:   resp.Message,
	}, nil
}

// --- ATC Quick Fix ---

// ATCQuickFixDetails contains the details of an ATC finding's available quick fix.
type ATCQuickFixDetails struct {
	FindingID   string `json:"findingId"`
	Title       string `json:"title,omitempty"`
	Description string `json:"description,omitempty"`
	CanAutoFix  bool   `json:"canAutoFix"`
}

// ATCQuickFixApplyResult is the result of applying an ATC quick fix.
type ATCQuickFixApplyResult struct {
	Success bool   `json:"success"`
	Message string `json:"message,omitempty"`
}

// GetATCQuickFixDetails retrieves quick fix details for a specific ATC finding.
// findingID is from the ATCFinding.QuickfixInfo field.
func (c *Client) GetATCQuickFixDetails(ctx context.Context, findingID string) (*ATCQuickFixDetails, error) {
	path := fmt.Sprintf("/sap/bc/adt/atc/quickfix/%s", url.PathEscape(findingID))

	resp, err := c.transport.Request(ctx, path, &RequestOptions{
		Method: http.MethodGet,
		Accept: "application/xml",
	})
	if err != nil {
		return nil, fmt.Errorf("get ATC quickfix details failed: %w", err)
	}

	return parseATCQuickFixDetails(resp.Body, findingID)
}

func parseATCQuickFixDetails(data []byte, findingID string) (*ATCQuickFixDetails, error) {
	xmlStr := string(data)
	xmlStr = strings.ReplaceAll(xmlStr, "atcquickfix:", "")

	type details struct {
		XMLName     xml.Name `xml:"quickfixDetails"`
		Title       string   `xml:"title,attr"`
		Description string   `xml:"description,attr"`
		CanAutoFix  bool     `xml:"canAutoFix,attr"`
	}

	var resp details
	if err := xml.Unmarshal([]byte(xmlStr), &resp); err != nil {
		// If parsing fails, return basic info
		return &ATCQuickFixDetails{
			FindingID:  findingID,
			CanAutoFix: false,
		}, nil
	}

	return &ATCQuickFixDetails{
		FindingID:   findingID,
		Title:       resp.Title,
		Description: resp.Description,
		CanAutoFix:  resp.CanAutoFix,
	}, nil
}

// ApplyATCQuickFix applies the quick fix for a specific ATC finding.
func (c *Client) ApplyATCQuickFix(ctx context.Context, findingID string) (*ATCQuickFixApplyResult, error) {
	if err := c.checkSafety(OpUpdate, "ApplyATCQuickFix"); err != nil {
		return nil, err
	}

	path := fmt.Sprintf("/sap/bc/adt/atc/quickfix/%s/apply", url.PathEscape(findingID))

	resp, err := c.transport.Request(ctx, path, &RequestOptions{
		Method: http.MethodPost,
		Accept: "application/xml",
	})
	if err != nil {
		return nil, fmt.Errorf("apply ATC quickfix failed: %w", err)
	}

	return parseATCQuickFixApplyResult(resp.Body)
}

func parseATCQuickFixApplyResult(data []byte) (*ATCQuickFixApplyResult, error) {
	xmlStr := string(data)
	xmlStr = strings.ReplaceAll(xmlStr, "atcquickfix:", "")

	type result struct {
		XMLName xml.Name `xml:"applyResult"`
		Success bool     `xml:"success,attr"`
		Message string   `xml:"message,attr"`
	}

	var resp result
	if err := xml.Unmarshal([]byte(xmlStr), &resp); err != nil {
		// SAP ADT may return non-XML on success. Include raw response for debugging.
		return &ATCQuickFixApplyResult{
			Success: true,
			Message: fmt.Sprintf("ATC quickfix applied (non-XML response: %s)", truncate(string(data), 200)),
		}, nil
	}

	return &ATCQuickFixApplyResult{
		Success: resp.Success,
		Message: resp.Message,
	}, nil
}
