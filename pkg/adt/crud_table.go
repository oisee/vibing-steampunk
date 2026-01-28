// ABOUTME: DDIC table creation operations.
// ABOUTME: Provides CreateTable for creating transparent tables from JSON-like options.

package adt

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strings"
)

// CreateTableOptions defines options for creating a DDIC table.
type CreateTableOptions struct {
	Name          string       `json:"name"`
	Description   string       `json:"description"`
	Package       string       `json:"package"`
	Fields        []TableField `json:"fields"`
	Transport     string       `json:"transport,omitempty"`
	DeliveryClass string       `json:"deliveryClass,omitempty"` // A, C, L, etc. (default: A)
	TableCategory string       `json:"tableCategory,omitempty"` // TRANSPARENT (default), STRUCTURE, etc.
}

// CreateTable creates a new DDIC transparent table from JSON-like options.
// Handles the full workflow: create → set source → activate.
func (c *Client) CreateTable(ctx context.Context, opts CreateTableOptions) error {
	if err := c.checkSafety(OpCreate, "CreateTable"); err != nil {
		return err
	}

	opts.Name = strings.ToUpper(opts.Name)
	if opts.Name == "" || len(opts.Name) > 30 {
		return fmt.Errorf("table name must be 1-30 characters")
	}
	if len(opts.Fields) == 0 {
		return fmt.Errorf("at least one field is required")
	}
	if opts.Package == "" {
		opts.Package = "$TMP"
	}
	if opts.DeliveryClass == "" {
		opts.DeliveryClass = "A"
	}
	if opts.TableCategory == "" {
		opts.TableCategory = "TRANSPARENT"
	}

	ddlSource := generateTableDDL(opts)

	createBody := fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?>
<blue:blueSource xmlns:blue="http://www.sap.com/wbobj/blue"
                 xmlns:adtcore="http://www.sap.com/adt/core"
                 adtcore:name="%s"
                 adtcore:type="TABL/DT"
                 adtcore:description="%s">
  <adtcore:packageRef adtcore:name="%s"/>
</blue:blueSource>`, opts.Name, escapeXML(opts.Description), opts.Package)

	params := url.Values{}
	if opts.Transport != "" {
		params.Set("corrNr", opts.Transport)
	}

	_, err := c.transport.Request(ctx, "/sap/bc/adt/ddic/tables", &RequestOptions{
		Method:      http.MethodPost,
		Query:       params,
		Body:        []byte(createBody),
		ContentType: "application/vnd.sap.adt.tables.v2+xml",
		Accept:      "application/vnd.sap.adt.tables.v2+xml",
	})
	if err != nil {
		return fmt.Errorf("creating table object: %w", err)
	}

	tableURL := fmt.Sprintf("/sap/bc/adt/ddic/tables/%s", strings.ToLower(opts.Name))
	sourceURL := tableURL + "/source/main"

	lock, err := c.LockObject(ctx, tableURL, "MODIFY")
	if err != nil {
		return fmt.Errorf("locking table: %w", err)
	}

	params = url.Values{}
	params.Set("lockHandle", lock.LockHandle)
	if opts.Transport != "" {
		params.Set("corrNr", opts.Transport)
	}

	_, err = c.transport.Request(ctx, sourceURL, &RequestOptions{
		Method:      http.MethodPut,
		Query:       params,
		Body:        []byte(ddlSource),
		ContentType: "text/plain",
	})
	if err != nil {
		_ = c.UnlockObject(ctx, tableURL, lock.LockHandle)
		return fmt.Errorf("updating table source: %w", err)
	}

	_ = c.UnlockObject(ctx, tableURL, lock.LockHandle)

	if _, err := c.Activate(ctx, tableURL, opts.Name); err != nil {
		return fmt.Errorf("activating table: %w", err)
	}

	return nil
}

func generateTableDDL(opts CreateTableOptions) string {
	var sb strings.Builder

	sb.WriteString(fmt.Sprintf("@EndUserText.label : '%s'\n", escapeQuote(opts.Description)))
	sb.WriteString("@AbapCatalog.enhancement.category : #NOT_EXTENSIBLE\n")
	sb.WriteString(fmt.Sprintf("@AbapCatalog.tableCategory : #%s\n", opts.TableCategory))
	sb.WriteString(fmt.Sprintf("@AbapCatalog.deliveryClass : #%s\n", opts.DeliveryClass))
	sb.WriteString("@AbapCatalog.dataMaintenance : #ALLOWED\n")
	sb.WriteString(fmt.Sprintf("define table %s {\n\n", strings.ToLower(opts.Name)))

	// Auto-add MANDT as first key field
	sb.WriteString("  key client : abap.clnt not null;\n")

	for _, f := range opts.Fields {
		fieldName := strings.ToLower(f.Name)
		fieldType := mapFieldType(f)

		if f.IsKey {
			sb.WriteString(fmt.Sprintf("  key %s : %s not null;\n", fieldName, fieldType))
		} else if f.NotNull {
			sb.WriteString(fmt.Sprintf("  %s : %s not null;\n", fieldName, fieldType))
		} else {
			sb.WriteString(fmt.Sprintf("  %s : %s;\n", fieldName, fieldType))
		}
	}

	sb.WriteString("\n}\n")
	return sb.String()
}

// fieldTypeMap maps simple type names to ABAP DDL types (without length parameters).
var fieldTypeMap = map[string]string{
	"INT1":        "abap.int1",
	"INT2":        "abap.int2",
	"INT4":        "abap.int4",
	"INT8":        "abap.int8",
	"FLTP":        "abap.fltp",
	"STRING":      "abap.string(0)",
	"RAWSTRING":   "abap.rawstring(0)",
	"DATS":        "abap.dats",
	"DATE":        "abap.dats",
	"TIMS":        "abap.tims",
	"TIME":        "abap.tims",
	"TIMESTAMPL":  "timestampl",
	"UTCLONG":     "abap.utclong",
	"SYSUUID_X16": "sysuuid_x16",
	"UUID":        "sysuuid_x16",
	"MANDT":       "mandt",
	"CLIENT":      "mandt",
}

func mapFieldType(f TableField) string {
	t := strings.ToUpper(f.Type)

	// Check simple type map first
	if mapped, ok := fieldTypeMap[t]; ok {
		return mapped
	}

	// Handle types with length parameters
	switch t {
	case "CHAR":
		if f.Length > 0 {
			return fmt.Sprintf("abap.char(%d)", f.Length)
		}
		return "abap.char(1)"
	case "NUMC":
		if f.Length > 0 {
			return fmt.Sprintf("abap.numc(%d)", f.Length)
		}
		return "abap.numc(10)"
	case "RAW":
		if f.Length > 0 {
			return fmt.Sprintf("abap.raw(%d)", f.Length)
		}
		return "abap.raw(16)"
	case "DEC", "CURR", "QUAN":
		l := f.Length
		if l == 0 {
			l = 15
		}
		return fmt.Sprintf("abap.dec(%d,%d)", l, f.Decimals)
	}

	// Check for shorthand like CHAR32, NUMC10
	if strings.HasPrefix(t, "CHAR") && len(t) > 4 {
		return fmt.Sprintf("abap.char(%s)", t[4:])
	}
	if strings.HasPrefix(t, "NUMC") && len(t) > 4 {
		return fmt.Sprintf("abap.numc(%s)", t[4:])
	}

	// Assume it's a data element name
	return strings.ToLower(t)
}

func escapeQuote(s string) string {
	return strings.ReplaceAll(s, "'", "''")
}
