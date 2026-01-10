package adt

import (
	"context"
	"encoding/xml"
	"fmt"
	"net/http"
	"net/url"
	"strings"
)

// --- Table Contents (Data Preview) ---

// TableContentsResult represents the result of a table contents query.
type TableContentsResult struct {
	Columns []TableColumn
	Rows    []map[string]any
}

// TableColumn represents a column in table contents.
type TableColumn struct {
	Name        string
	Type        string
	Description string
	Length      int
	IsKey       bool
}

// GetTableContents retrieves data from a database table.
// Optional sqlQuery can be a full SELECT statement to filter/transform results
// (e.g., "SELECT * FROM T000 WHERE MANDT = '001'").
func (c *Client) GetTableContents(ctx context.Context, tableName string, maxRows int, sqlFilter string) (*TableContentsResult, error) {
	tableName = strings.ToUpper(tableName)
	if maxRows <= 0 {
		maxRows = 100
	}

	params := url.Values{}
	params.Set("rowNumber", fmt.Sprintf("%d", maxRows))
	params.Set("ddicEntityName", tableName)

	opts := &RequestOptions{
		Method: http.MethodPost,
		Query:  params,
		Accept: "application/*",
	}

	// Add SQL filter as request body if provided
	if sqlFilter != "" {
		opts.Body = []byte(sqlFilter)
		opts.ContentType = "text/plain"
	}

	resp, err := c.transport.Request(ctx, "/sap/bc/adt/datapreview/ddic", opts)
	if err != nil {
		return nil, fmt.Errorf("getting table contents: %w", err)
	}

	return parseTableContents(resp.Body)
}

// RunQuery executes a freestyle SQL query against the SAP database.
// Example: "SELECT * FROM T000 WHERE MANDT = '001'"
func (c *Client) RunQuery(ctx context.Context, sqlQuery string, maxRows int) (*TableContentsResult, error) {
	// Safety check - free SQL can be dangerous
	if err := c.checkSafety(OpFreeSQL, "RunQuery"); err != nil {
		return nil, err
	}

	if sqlQuery == "" {
		return nil, fmt.Errorf("SQL query is required")
	}
	if maxRows <= 0 {
		maxRows = 100
	}

	params := url.Values{}
	params.Set("rowNumber", fmt.Sprintf("%d", maxRows))

	resp, err := c.transport.Request(ctx, "/sap/bc/adt/datapreview/freestyle", &RequestOptions{
		Method:      http.MethodPost,
		Query:       params,
		Accept:      "application/*",
		Body:        []byte(sqlQuery),
		ContentType: "text/plain",
	})
	if err != nil {
		return nil, fmt.Errorf("running query: %w", err)
	}

	return parseTableContents(resp.Body)
}

// parseTableContents parses the XML response for table contents.
func parseTableContents(data []byte) (*TableContentsResult, error) {
	// The ADT table data response is complex XML
	// We'll parse it into a generic structure
	type tableData struct {
		Columns []struct {
			Metadata struct {
				Name        string `xml:"name,attr"`
				Type        string `xml:"type,attr"`
				Description string `xml:"description,attr"`
				Length      int    `xml:"length,attr"`
				IsKey       bool   `xml:"keyAttribute,attr"`
			} `xml:"metadata"`
			DataSet struct {
				Data []string `xml:"data"`
			} `xml:"dataSet"`
		} `xml:"columns"`
	}

	var td tableData
	if err := xml.Unmarshal(data, &td); err != nil {
		return nil, fmt.Errorf("parsing table data: %w", err)
	}

	result := &TableContentsResult{
		Columns: make([]TableColumn, len(td.Columns)),
		Rows:    []map[string]any{},
	}

	// Extract columns
	maxRows := 0
	for i, col := range td.Columns {
		result.Columns[i] = TableColumn{
			Name:        col.Metadata.Name,
			Type:        col.Metadata.Type,
			Description: col.Metadata.Description,
			Length:      col.Metadata.Length,
			IsKey:       col.Metadata.IsKey,
		}
		if len(col.DataSet.Data) > maxRows {
			maxRows = len(col.DataSet.Data)
		}
	}

	// Build rows
	for rowIdx := 0; rowIdx < maxRows; rowIdx++ {
		row := make(map[string]any)
		for _, col := range td.Columns {
			if rowIdx < len(col.DataSet.Data) {
				row[col.Metadata.Name] = col.DataSet.Data[rowIdx]
			}
		}
		result.Rows = append(result.Rows, row)
	}

	return result, nil
}

// --- Transaction Operations ---

// Transaction represents an SAP transaction.
type Transaction struct {
	Name        string
	Description string
	Program     string
}

// GetTransaction retrieves information about a transaction.
func (c *Client) GetTransaction(ctx context.Context, tcode string) (*Transaction, error) {
	tcode = strings.ToUpper(tcode)

	resp, err := c.transport.Request(ctx, fmt.Sprintf("/sap/bc/adt/vit/wb/object_type/trant/object_name/%s", tcode), &RequestOptions{
		Method: http.MethodGet,
		Accept: "application/vnd.sap.adt.basic.object.properties+xml",
	})
	if err != nil {
		return nil, fmt.Errorf("getting transaction: %w", err)
	}

	// Parse transaction info
	type tranInfo struct {
		Name        string `xml:"name,attr"`
		Description string `xml:"description,attr"`
		Program     string `xml:"program,attr"`
	}

	var ti tranInfo
	if err := xml.Unmarshal(resp.Body, &ti); err != nil {
		return nil, fmt.Errorf("parsing transaction: %w", err)
	}

	return &Transaction{
		Name:        ti.Name,
		Description: ti.Description,
		Program:     ti.Program,
	}, nil
}

// --- Type Info Operations ---

// TypeInfo represents type information.
type TypeInfo struct {
	Name        string
	Type        string
	Description string
	Length      int
	Decimals    int
}

// GetTypeInfo retrieves information about a data type.
func (c *Client) GetTypeInfo(ctx context.Context, typeName string) (*TypeInfo, error) {
	typeName = strings.ToUpper(typeName)

	resp, err := c.transport.Request(ctx, fmt.Sprintf("/sap/bc/adt/ddic/dataelements/%s", typeName), &RequestOptions{
		Method: http.MethodGet,
		Accept: "application/xml",
	})
	if err != nil {
		return nil, fmt.Errorf("getting type info: %w", err)
	}

	type typeData struct {
		Name        string `xml:"name,attr"`
		Type        string `xml:"type,attr"`
		Description string `xml:"description,attr"`
		Length      int    `xml:"length,attr"`
		Decimals    int    `xml:"decimals,attr"`
	}

	var td typeData
	if err := xml.Unmarshal(resp.Body, &td); err != nil {
		return nil, fmt.Errorf("parsing type info: %w", err)
	}

	return &TypeInfo{
		Name:        td.Name,
		Type:        td.Type,
		Description: td.Description,
		Length:      td.Length,
		Decimals:    td.Decimals,
	}, nil
}
