package adt

import (
	"context"
	"encoding/xml"
	"fmt"
	"net/http"
	"net/url"
	"strings"
)

// --- Code Coverage ---

// CoverageResult contains line-level coverage data from a test run.
type CoverageResult struct {
	Statements    CoverageStats          `json:"statements"`
	Branches      CoverageStats          `json:"branches,omitempty"`
	Procedures    CoverageStats          `json:"procedures,omitempty"`
	SourceCoverage map[string]*SourceCoverage `json:"sourceCoverage,omitempty"`
}

// CoverageStats contains aggregate coverage statistics.
type CoverageStats struct {
	Total   int     `json:"total"`
	Covered int     `json:"covered"`
	Percent float64 `json:"percent"`
}

// SourceCoverage contains coverage data for a single source file.
type SourceCoverage struct {
	URI          string        `json:"uri"`
	Type         string        `json:"type,omitempty"`
	Name         string        `json:"name,omitempty"`
	Statements   CoverageStats `json:"statements"`
	CoveredLines []CoveredLine `json:"coveredLines,omitempty"`
}

// CoveredLine represents coverage info for a single line.
type CoveredLine struct {
	Line     int  `json:"line"`
	Executed bool `json:"executed"`
}

// GetCodeCoverage runs unit tests with coverage enabled and returns coverage data.
// objectURL is the ADT URL of the object to test.
func (c *Client) GetCodeCoverage(ctx context.Context, objectURL string, flags *UnitTestRunFlags) (*CoverageResult, error) {
	if flags == nil {
		defaultFlags := DefaultUnitTestFlags()
		flags = &defaultFlags
	}

	body := fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?>
<aunit:runConfiguration xmlns:aunit="http://www.sap.com/adt/aunit">
  <external>
    <coverage active="true"/>
  </external>
  <options>
    <uriType value="semantic"/>
    <testDeterminationStrategy sameProgram="true" assignedTests="false"/>
    <testRiskLevels harmless="%t" dangerous="%t" critical="%t"/>
    <testDurations short="%t" medium="%t" long="%t"/>
    <withNavigationUri enabled="true"/>
  </options>
  <adtcore:objectSets xmlns:adtcore="http://www.sap.com/adt/core">
    <objectSet kind="inclusive">
      <adtcore:objectReferences>
        <adtcore:objectReference adtcore:uri="%s"/>
      </adtcore:objectReferences>
    </objectSet>
  </adtcore:objectSets>
</aunit:runConfiguration>`,
		flags.Harmless, flags.Dangerous, flags.Critical,
		flags.Short, flags.Medium, flags.Long,
		objectURL)

	resp, err := c.transport.Request(ctx, "/sap/bc/adt/abapunit/testruns", &RequestOptions{
		Method:      http.MethodPost,
		Body:        []byte(body),
		ContentType: "application/*",
		Accept:      "application/*",
	})
	if err != nil {
		return nil, fmt.Errorf("running unit tests with coverage: %w", err)
	}

	return parseCoverageResult(resp.Body)
}

func parseCoverageResult(data []byte) (*CoverageResult, error) {
	if len(data) == 0 {
		return &CoverageResult{}, nil
	}

	xmlStr := string(data)
	xmlStr = strings.ReplaceAll(xmlStr, "aunit:", "")
	xmlStr = strings.ReplaceAll(xmlStr, "adtcore:", "")
	xmlStr = strings.ReplaceAll(xmlStr, `xmlns:aunit="http://www.sap.com/adt/aunit"`, "")
	xmlStr = strings.ReplaceAll(xmlStr, `xmlns:adtcore="http://www.sap.com/adt/core"`, "")

	// Parse coverage section from the response
	type coverageNode struct {
		URI        string `xml:"uri,attr"`
		Type       string `xml:"type,attr"`
		Name       string `xml:"name,attr"`
		Total      int    `xml:"total,attr"`
		Covered    int    `xml:"covered,attr"`
		Percentage string `xml:"percentage,attr"`
	}

	type coverageSection struct {
		Statement []coverageNode `xml:"statement>node"`
		Branch    []coverageNode `xml:"branch>node"`
		Procedure []coverageNode `xml:"procedure>node"`
	}

	type coverageResponse struct {
		XMLName  xml.Name        `xml:"runResult"`
		Coverage coverageSection `xml:"coverage"`
	}

	var resp coverageResponse
	if err := xml.Unmarshal([]byte(xmlStr), &resp); err != nil {
		// If coverage data isn't in expected format, return empty
		return &CoverageResult{
			Statements: CoverageStats{},
		}, nil
	}

	result := &CoverageResult{
		SourceCoverage: make(map[string]*SourceCoverage),
	}

	// Aggregate statement coverage
	for _, node := range resp.Coverage.Statement {
		result.Statements.Total += node.Total
		result.Statements.Covered += node.Covered

		if node.URI != "" {
			sc := &SourceCoverage{
				URI:  node.URI,
				Type: node.Type,
				Name: node.Name,
				Statements: CoverageStats{
					Total:   node.Total,
					Covered: node.Covered,
				},
			}
			if node.Total > 0 {
				sc.Statements.Percent = float64(node.Covered) / float64(node.Total) * 100
			}
			result.SourceCoverage[node.URI] = sc
		}
	}

	if result.Statements.Total > 0 {
		result.Statements.Percent = float64(result.Statements.Covered) / float64(result.Statements.Total) * 100
	}

	// Aggregate branch coverage
	for _, node := range resp.Coverage.Branch {
		result.Branches.Total += node.Total
		result.Branches.Covered += node.Covered
	}
	if result.Branches.Total > 0 {
		result.Branches.Percent = float64(result.Branches.Covered) / float64(result.Branches.Total) * 100
	}

	// Aggregate procedure coverage
	for _, node := range resp.Coverage.Procedure {
		result.Procedures.Total += node.Total
		result.Procedures.Covered += node.Covered
	}
	if result.Procedures.Total > 0 {
		result.Procedures.Percent = float64(result.Procedures.Covered) / float64(result.Procedures.Total) * 100
	}

	return result, nil
}

// --- SQL Explain Plan ---

// SQLExplainPlan represents the execution plan for a SQL query.
type SQLExplainPlan struct {
	Query    string          `json:"query"`
	Nodes    []SQLPlanNode   `json:"nodes"`
	TotalCost float64        `json:"totalCost,omitempty"`
}

// SQLPlanNode represents a single node in the execution plan tree.
type SQLPlanNode struct {
	ID        int           `json:"id"`
	Operator  string        `json:"operator"`
	Table     string        `json:"table,omitempty"`
	Index     string        `json:"index,omitempty"`
	Cost      float64       `json:"cost,omitempty"`
	Rows      int           `json:"rows,omitempty"`
	Children  []SQLPlanNode `json:"children,omitempty"`
}

// GetSQLExplainPlan returns the execution plan for a SQL query (HANA only).
func (c *Client) GetSQLExplainPlan(ctx context.Context, sqlQuery string) (*SQLExplainPlan, error) {
	if err := c.checkSafety(OpRead, "GetSQLExplainPlan"); err != nil {
		return nil, err
	}

	if sqlQuery == "" {
		return nil, fmt.Errorf("SQL query is required")
	}

	q := url.Values{}
	q.Set("rowNumber", "0") // We don't need actual data, just the plan

	resp, err := c.transport.Request(ctx, "/sap/bc/adt/datapreview/freestyle", &RequestOptions{
		Method:      http.MethodPost,
		Query:       q,
		Body:        []byte("EXPLAIN PLAN FOR " + sqlQuery),
		ContentType: "text/plain",
		Accept:      "application/xml",
	})
	if err != nil {
		// Fallback: try the dedicated explain endpoint
		resp, err = c.transport.Request(ctx, "/sap/bc/adt/datapreview/ddlServices/explain", &RequestOptions{
			Method:      http.MethodPost,
			Body:        []byte(sqlQuery),
			ContentType: "text/plain",
			Accept:      "application/xml",
		})
		if err != nil {
			return nil, fmt.Errorf("SQL explain plan failed: %w", err)
		}
	}

	return parseSQLExplainPlan(resp.Body, sqlQuery)
}

func parseSQLExplainPlan(data []byte, query string) (*SQLExplainPlan, error) {
	if len(data) == 0 {
		return &SQLExplainPlan{Query: query}, nil
	}

	xmlStr := string(data)
	// Strip common ADT namespace prefixes
	xmlStr = strings.ReplaceAll(xmlStr, "datapreview:", "")

	type planNode struct {
		XMLName  xml.Name   `xml:"node"`
		ID       int        `xml:"id,attr"`
		Operator string     `xml:"operator,attr"`
		Table    string     `xml:"tableName,attr"`
		Index    string     `xml:"indexName,attr"`
		Cost     float64    `xml:"cost,attr"`
		Rows     int        `xml:"outputRowCount,attr"`
		Children []planNode `xml:"node"`
	}

	type explainResult struct {
		XMLName xml.Name   `xml:"explainResult"`
		Nodes   []planNode `xml:"node"`
	}

	// Try structured XML first
	var resp explainResult
	if err := xml.Unmarshal([]byte(xmlStr), &resp); err != nil {
		// If XML parsing fails, return the raw response as a single-node plan
		return &SQLExplainPlan{
			Query: query,
			Nodes: []SQLPlanNode{
				{
					ID:       0,
					Operator: "RAW_RESPONSE",
					Table:    truncate(string(data), 500),
				},
			},
		}, nil
	}

	plan := &SQLExplainPlan{Query: query}
	var convertNodes func(nodes []planNode) []SQLPlanNode
	convertNodes = func(nodes []planNode) []SQLPlanNode {
		var result []SQLPlanNode
		for _, n := range nodes {
			node := SQLPlanNode{
				ID:       n.ID,
				Operator: n.Operator,
				Table:    n.Table,
				Index:    n.Index,
				Cost:     n.Cost,
				Rows:     n.Rows,
				Children: convertNodes(n.Children),
			}
			result = append(result, node)
		}
		return result
	}

	plan.Nodes = convertNodes(resp.Nodes)

	// Calculate total cost from root nodes
	for _, n := range plan.Nodes {
		plan.TotalCost += n.Cost
	}

	return plan, nil
}

// --- Enhanced Check Run Results ---

// CheckRunResult represents detailed results from a check run.
type CheckRunResult struct {
	CheckRunID string            `json:"checkRunId"`
	Status     string            `json:"status"`
	Messages   []CheckRunMessage `json:"messages"`
	Summary    CheckRunSummary   `json:"summary"`
}

// CheckRunMessage represents a single message from a check run.
type CheckRunMessage struct {
	URI      string `json:"uri"`
	Type     string `json:"type"`      // E=Error, W=Warning, I=Info
	Line     int    `json:"line"`
	Column   int    `json:"column"`
	Text     string `json:"text"`
	Category string `json:"category,omitempty"` // syntax, semantic, etc.
}

// CheckRunSummary contains summary counts of check results.
type CheckRunSummary struct {
	Errors   int `json:"errors"`
	Warnings int `json:"warnings"`
	Info     int `json:"info"`
	Total    int `json:"total"`
}

// GetCheckRunResults retrieves detailed results for a specific check run.
// checkRunID is typically from a SyntaxCheck or other check operation.
func (c *Client) GetCheckRunResults(ctx context.Context, checkRunID string) (*CheckRunResult, error) {
	if err := c.checkSafety(OpRead, "GetCheckRunResults"); err != nil {
		return nil, err
	}

	path := fmt.Sprintf("/sap/bc/adt/checkruns/%s", url.PathEscape(checkRunID))

	resp, err := c.transport.Request(ctx, path, &RequestOptions{
		Method: http.MethodGet,
		Accept: "application/xml",
	})
	if err != nil {
		return nil, fmt.Errorf("get check run results failed: %w", err)
	}

	return parseCheckRunResult(resp.Body, checkRunID)
}

func parseCheckRunResult(data []byte, checkRunID string) (*CheckRunResult, error) {
	if len(data) == 0 {
		return &CheckRunResult{
			CheckRunID: checkRunID,
			Status:     "empty",
		}, nil
	}

	xmlStr := string(data)
	xmlStr = strings.ReplaceAll(xmlStr, "chkrun:", "")
	xmlStr = strings.ReplaceAll(xmlStr, "adtcore:", "")

	type checkMessage struct {
		URI       string `xml:"uri,attr"`
		Type      string `xml:"type,attr"`
		Line      int    `xml:"line,attr"`
		Column    int    `xml:"column,attr"`
		ShortText string `xml:"shortText"`
		Category  string `xml:"category,attr"`
	}

	type checkMessageList struct {
		Messages []checkMessage `xml:"checkMessage"`
	}

	type checkReport struct {
		URI         string           `xml:"uri,attr"`
		Status      string           `xml:"status,attr"`
		Reporter    string           `xml:"reporter,attr"`
		MessageList checkMessageList `xml:"checkMessageList"`
	}

	type checkReports struct {
		XMLName xml.Name      `xml:"checkRunReports"`
		Reports []checkReport `xml:"checkReport"`
	}

	var resp checkReports
	if err := xml.Unmarshal([]byte(xmlStr), &resp); err != nil {
		return nil, fmt.Errorf("parsing check run results: %w", err)
	}

	result := &CheckRunResult{
		CheckRunID: checkRunID,
		Status:     "completed",
	}

	for _, report := range resp.Reports {
		if report.Status != "" {
			result.Status = report.Status
		}
		for _, msg := range report.MessageList.Messages {
			crMsg := CheckRunMessage{
				URI:      msg.URI,
				Type:     msg.Type,
				Line:     msg.Line,
				Column:   msg.Column,
				Text:     msg.ShortText,
				Category: msg.Category,
			}
			result.Messages = append(result.Messages, crMsg)

			switch msg.Type {
			case "E":
				result.Summary.Errors++
			case "W":
				result.Summary.Warnings++
			case "I":
				result.Summary.Info++
			}
			result.Summary.Total++
		}
	}

	return result, nil
}

// truncate shortens a string to maxLen characters, appending "..." if truncated.
func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	if maxLen <= 3 {
		return s[:maxLen]
	}
	return s[:maxLen-3] + "..."
}
