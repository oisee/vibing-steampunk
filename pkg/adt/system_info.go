package adt

import (
	"context"
	"encoding/xml"
	"fmt"
	"net/http"
	"strings"
)

// --- Type Info Operations ---

// TypeInfo describes a DDIC data element.
//
// Type is the ABAP data type (CHAR, SSTRING, CURR, QUAN...), not the workbench
// object type — a value with a Length and Decimals beside it is only meaningful
// as the former. The workbench type ("DTEL/DE") is kept in ObjectType so the
// root attribute is not lost.
//
// TypeKind is "domain" when the element takes its type from a domain, whose
// name is then in DomainName, and "predefinedAbapType" when it declares the
// type itself, in which case DomainName is empty.
//
// Shapes this was written against, read off a system rather than guessed:
//
//	APC_CONNECTION_ID  predefinedAbapType  ""                        CHAR     32  0
//	AMC_CHANNEL_ID     domain              AMC_CHANNEL_ID            SSTRING 140  0
//	DMBTR              domain              AFLE13D2O16N_TO_23D2O30N  CURR     23  2
//	MENGE_D            domain              MENG13                    QUAN     13  3
type TypeInfo struct {
	Name        string
	Type        string
	Description string
	Length      int
	Decimals    int
	TypeKind    string
	DomainName  string
	ObjectType  string
}

// GetTypeInfo retrieves information about a data type.
func (c *Client) GetTypeInfo(ctx context.Context, typeName string) (*TypeInfo, error) {
	typeName = strings.ToUpper(typeName)

	resp, err := c.transport.Request(ctx, fmt.Sprintf("/sap/bc/adt/ddic/dataelements/%s", typeName), &RequestOptions{
		Method: http.MethodGet,
		// The versioned vocabulary type, not application/xml. This endpoint
		// refuses the generic one with 406 "The message content is not
		// acceptable" on every name, so this call had never returned anything
		// to anybody. GetDataElementLabels in i18n.go hit the identical bug on
		// the identical endpoint and was fixed there; this twin was missed, so
		// the same 406 survived here. Keep the two in step.
		Accept: "application/vnd.sap.adt.dataelements.v2+xml",
	})
	if err != nil {
		return nil, fmt.Errorf("getting type info: %w", err)
	}

	// The type and the lengths are CHILD ELEMENTS of dtel:dataElement, not
	// attributes of the root. The previous mapping read them as root attributes
	// and so returned zero for every element ever — invisibly, because the 406
	// above meant it never got as far as parsing. Lengths arrive zero-padded to
	// six digits ("000032", "000002"); ParseInt handles the padding.
	type typeData struct {
		Name        string `xml:"name,attr"`
		ObjectType  string `xml:"type,attr"`
		Description string `xml:"description,attr"`
		DataElement struct {
			TypeKind string `xml:"typeKind"`
			TypeName string `xml:"typeName"`
			DataType string `xml:"dataType"`
			Length   int    `xml:"dataTypeLength"`
			Decimals int    `xml:"dataTypeDecimals"`
		} `xml:"dataElement"`
	}

	var td typeData
	if err := xml.Unmarshal(resp.Body, &td); err != nil {
		return nil, fmt.Errorf("parsing type info: %w", err)
	}

	return &TypeInfo{
		Name:        td.Name,
		Type:        td.DataElement.DataType,
		Description: td.Description,
		Length:      td.DataElement.Length,
		Decimals:    td.DataElement.Decimals,
		TypeKind:    td.DataElement.TypeKind,
		DomainName:  td.DataElement.TypeName,
		ObjectType:  td.ObjectType,
	}, nil
}

// --- System Information Operations ---

// SystemInfo represents SAP system information.
type SystemInfo struct {
	SystemID        string `json:"systemId"`
	Client          string `json:"client"`
	SAPRelease      string `json:"sapRelease"`
	KernelRelease   string `json:"kernelRelease,omitempty"`
	DatabaseRelease string `json:"databaseRelease,omitempty"`
	DatabaseSystem  string `json:"databaseSystem,omitempty"`
	HostName        string `json:"hostName,omitempty"`
	InstallNumber   string `json:"installNumber,omitempty"`
	ABAPRelease     string `json:"abapRelease,omitempty"`
}

// GetSystemInfo retrieves SAP system information.
// Uses SQL queries to CVERS and T000 tables for reliable info across SAP versions.
func (c *Client) GetSystemInfo(ctx context.Context) (*SystemInfo, error) {
	info := &SystemInfo{}

	// Helper to get string from row
	getString := func(row map[string]interface{}, key string) string {
		if v, ok := row[key]; ok {
			if s, ok := v.(string); ok {
				return s
			}
		}
		return ""
	}

	// Get client info from T000 - this is the primary query, propagate errors
	clientResult, err := c.RunQuery(ctx, "SELECT MANDT, MTEXT, LOGSYS FROM T000 WHERE MANDT = '"+c.config.Client+"'", 1)
	if err != nil {
		return nil, fmt.Errorf("getting system info: %w", err)
	}
	if len(clientResult.Rows) > 0 {
		row := clientResult.Rows[0]
		info.Client = getString(row, "MANDT")
		// LOGSYS format is typically <SID>CLNT<client>, e.g., A4HCLNT001
		if logsys := getString(row, "LOGSYS"); len(logsys) >= 3 {
			info.SystemID = logsys[:3] // First 3 chars are SID
		}
	}

	// Get SAP_BASIS version from CVERS (optional - don't fail if unavailable)
	basisResult, err := c.RunQuery(ctx, "SELECT RELEASE, EXTRELEASE FROM CVERS WHERE COMPONENT = 'SAP_BASIS'", 1)
	if err == nil && len(basisResult.Rows) > 0 {
		row := basisResult.Rows[0]
		info.SAPRelease = getString(row, "RELEASE")
		info.ABAPRelease = getString(row, "RELEASE")
	}

	// Try to get kernel info from CVERS (optional)
	kernelResult, err := c.RunQuery(ctx, "SELECT RELEASE FROM CVERS WHERE COMPONENT = 'SAP_ABA'", 1)
	if err == nil && len(kernelResult.Rows) > 0 {
		info.KernelRelease = getString(kernelResult.Rows[0], "RELEASE")
	}

	// Try to detect HANA from CVERS (optional)
	hanaResult, err := c.RunQuery(ctx,
		"SELECT RELEASE FROM CVERS WHERE COMPONENT LIKE '%HDB%' OR COMPONENT LIKE '%HANA%'", 1)
	if err == nil && len(hanaResult.Rows) > 0 {
		info.DatabaseSystem = "HDB"
		info.DatabaseRelease = getString(hanaResult.Rows[0], "RELEASE")
	} else {
		// Step 2: S4CORE in CVERS — pure S/4HANA.
		// S/4HANA implies HANA database. However its version cannot be inferred from the software component.
		// Therefore DatabaseRelease is left blank
		s4Result, err := c.RunQuery(ctx,
			"SELECT COMPONENT FROM CVERS WHERE COMPONENT = 'S4CORE'", 1)
		if err == nil && len(s4Result.Rows) > 0 {
			info.DatabaseSystem = "HDB"
		}
	}

	// If we couldn't get SystemID from T000, use fallback
	if info.SystemID == "" {
		info.SystemID = "???"
	}
	if info.Client == "" {
		info.Client = c.config.Client
	}

	return info, nil
}

// InstalledComponent represents an installed software component.
type InstalledComponent struct {
	Name        string `json:"name"`
	Release     string `json:"release"`
	SupportPack string `json:"supportPack,omitempty"`
	Description string `json:"description,omitempty"`
}

// GetInstalledComponents retrieves list of installed software components.
func (c *Client) GetInstalledComponents(ctx context.Context) ([]InstalledComponent, error) {
	resp, err := c.transport.Request(ctx, "/sap/bc/adt/system/components", &RequestOptions{
		Method: http.MethodGet,
		Accept: "application/xml",
	})
	if err != nil {
		return nil, fmt.Errorf("getting installed components: %w", err)
	}

	type componentXML struct {
		Name        string `xml:"name,attr"`
		Release     string `xml:"release,attr"`
		SupportPack string `xml:"supportPack,attr"`
		Description string `xml:"description,attr"`
	}
	type componentsXML struct {
		XMLName    xml.Name       `xml:"components"`
		Components []componentXML `xml:"component"`
	}

	var comps componentsXML
	if err := xml.Unmarshal(resp.Body, &comps); err != nil {
		return nil, fmt.Errorf("parsing components: %w", err)
	}

	result := make([]InstalledComponent, len(comps.Components))
	for i, c := range comps.Components {
		result[i] = InstalledComponent{
			Name:        c.Name,
			Release:     c.Release,
			SupportPack: c.SupportPack,
			Description: c.Description,
		}
	}

	return result, nil
}
