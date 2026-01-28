package adt

import (
	"context"
	"encoding/xml"
	"fmt"
	"net/http"
	"strings"
)

// --- System Information Operations ---

// SystemInfo represents SAP system information.
type SystemInfo struct {
	SystemID        string `json:"systemId"`
	Client          string `json:"client"`
	SAPRelease      string `json:"sapRelease,omitempty"`
	KernelRelease   string `json:"kernelRelease,omitempty"`
	DatabaseRelease string `json:"databaseRelease,omitempty"`
	DatabaseSystem  string `json:"databaseSystem,omitempty"`
	HostName        string `json:"hostName,omitempty"`
	InstallNumber   string `json:"installNumber,omitempty"`
	OSName          string `json:"osName,omitempty"`
	OSVersion       string `json:"osVersion,omitempty"`
}

// GetSystemInfo retrieves SAP system information from ADT endpoint.
// Uses /sap/bc/adt/system/information which is the proper ADT API.
func (c *Client) GetSystemInfo(ctx context.Context) (*SystemInfo, error) {
	resp, err := c.transport.Request(ctx, "/sap/bc/adt/system/information", &RequestOptions{
		Method: http.MethodGet,
		Accept: "application/atom+xml;type=feed",
	})
	if err != nil {
		return nil, fmt.Errorf("getting system info: %w", err)
	}

	// Parse Atom feed
	var feed AtomFeed
	if err := xml.Unmarshal(resp.Body, &feed); err != nil {
		return nil, fmt.Errorf("parsing system info: %w", err)
	}

	// Build map from entries
	data := make(map[string]string)
	for _, entry := range feed.Entries {
		data[entry.ID] = entry.Title
	}

	info := &SystemInfo{
		Client:          c.config.Client,
		DatabaseSystem:  data["DBSystem"],
		DatabaseRelease: data["DBRelease"],
		HostName:        data["DBServer"],
		KernelRelease:   data["KernelRelease"],
		InstallNumber:   data["SAPSystemNumber"],
		OSName:          data["OSName"],
		OSVersion:       data["OSVersion"],
	}

	// Extract SID from ApplicationServerName (format: hostname_SID_instance)
	// e.g., "vhcala4hci_A4H_00" -> "A4H"
	if appServer := data["ApplicationServerName"]; appServer != "" {
		parts := strings.Split(appServer, "_")
		if len(parts) >= 2 {
			info.SystemID = parts[1]
		}
	}

	// Fallback for SystemID
	if info.SystemID == "" {
		info.SystemID = "???"
	}

	return info, nil
}

// DatabaseInfo holds database system information.
type DatabaseInfo struct {
	System  string // HDB=HANA, ORA=Oracle, MSS=SQL Server, ADA=MaxDB
	Release string // Database version/release
	Host    string // Database host name
}

// GetDatabaseInfo retrieves database info from ADT system information endpoint.
func (c *Client) GetDatabaseInfo(ctx context.Context) (*DatabaseInfo, error) {
	info, err := c.GetSystemInfo(ctx)
	if err != nil {
		return nil, err
	}
	return &DatabaseInfo{
		System:  info.DatabaseSystem,
		Release: info.DatabaseRelease,
		Host:    info.HostName,
	}, nil
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
		Accept: "application/atom+xml;type=feed",
	})
	if err != nil {
		return nil, fmt.Errorf("getting installed components: %w", err)
	}

	// Parse Atom feed format
	var feed AtomFeed
	if err := xml.Unmarshal(resp.Body, &feed); err != nil {
		return nil, fmt.Errorf("parsing components: %w", err)
	}

	result := make([]InstalledComponent, 0, len(feed.Entries))
	for _, entry := range feed.Entries {
		comp := InstalledComponent{Name: entry.ID}
		// Parse title: "758;SAPK-75802INSAPBASIS;0002;SAP Basis Component"
		parts := strings.SplitN(entry.Title, ";", 4)
		if len(parts) >= 1 {
			comp.Release = strings.TrimSpace(parts[0])
		}
		if len(parts) >= 2 {
			comp.SupportPack = strings.TrimSpace(parts[1])
		}
		// parts[2] is patch level, skip
		if len(parts) >= 4 {
			comp.Description = strings.TrimSpace(parts[3])
		}
		result = append(result, comp)
	}

	return result, nil
}
