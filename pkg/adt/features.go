// Package adt provides a Go client for SAP ABAP Development Tools (ADT) REST API.
package adt

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"
)

// FeatureID identifies a specific optional feature that can be probed
type FeatureID string

const (
	// FeatureAbapGit indicates abapGit is installed on the system
	FeatureAbapGit FeatureID = "abapgit"
	// FeatureRAP indicates RAP development is available (DDLS, BDEF, SRVD, SRVB)
	FeatureRAP FeatureID = "rap"
	// FeatureAMDP indicates AMDP/HANA debugging is available
	FeatureAMDP FeatureID = "amdp"
	// FeatureUI5 indicates UI5/Fiori BSP management is available
	FeatureUI5 FeatureID = "ui5"
	// FeatureTransport indicates CTS transport management is available
	FeatureTransport FeatureID = "transport"
	// FeatureHANA indicates HANA database (required for some AMDP features)
	FeatureHANA FeatureID = "hana"
)

// FeatureMode controls how a feature is enabled
type FeatureMode string

const (
	// FeatureModeAuto enables feature if probe succeeds (default)
	FeatureModeAuto FeatureMode = "auto"
	// FeatureModeOn forces feature enabled (skip probe)
	FeatureModeOn FeatureMode = "on"
	// FeatureModeOff forces feature disabled (skip probe)
	FeatureModeOff FeatureMode = "off"
)

// FeatureStatus represents the probed status of a feature
type FeatureStatus struct {
	ID        FeatureID `json:"id"`
	Available bool      `json:"available"`
	Mode      FeatureMode `json:"mode"`
	Message   string    `json:"message,omitempty"`
	ProbedAt  time.Time `json:"probed_at,omitempty"`
}

// FeatureConfig controls which optional features are enabled
type FeatureConfig struct {
	// HANA controls HANA database detection (default: auto)
	HANA FeatureMode
	// AbapGit controls abapGit integration (default: auto)
	AbapGit FeatureMode
	// RAP controls RAP/OData tools (default: auto, usually available)
	RAP FeatureMode
	// AMDP controls AMDP/HANA debugger (default: auto)
	AMDP FeatureMode
	// UI5 controls UI5/Fiori BSP tools (default: auto)
	UI5 FeatureMode
	// Transport controls CTS transport tools (default: auto)
	Transport FeatureMode
}

// DefaultFeatureConfig returns default feature configuration (all auto-detect)
func DefaultFeatureConfig() FeatureConfig {
	return FeatureConfig{
		HANA:      FeatureModeAuto,
		AbapGit:   FeatureModeAuto,
		RAP:       FeatureModeAuto,
		AMDP:      FeatureModeAuto,
		UI5:       FeatureModeAuto,
		Transport: FeatureModeAuto,
	}
}

// GetMode returns the mode for a specific feature
func (f *FeatureConfig) GetMode(id FeatureID) FeatureMode {
	switch id {
	case FeatureHANA:
		return f.HANA
	case FeatureAbapGit:
		return f.AbapGit
	case FeatureRAP:
		return f.RAP
	case FeatureAMDP:
		return f.AMDP
	case FeatureUI5:
		return f.UI5
	case FeatureTransport:
		return f.Transport
	default:
		return FeatureModeAuto
	}
}

// FeatureProber probes SAP system for available features
type FeatureProber struct {
	client  *Client
	config  FeatureConfig
	cache   map[FeatureID]*FeatureStatus
	mu      sync.RWMutex
	verbose bool
}

// NewFeatureProber creates a new feature prober
func NewFeatureProber(client *Client, config FeatureConfig, verbose bool) *FeatureProber {
	return &FeatureProber{
		client:  client,
		config:  config,
		cache:   make(map[FeatureID]*FeatureStatus),
		verbose: verbose,
	}
}

// ProbeAll probes all features and returns their status
func (p *FeatureProber) ProbeAll(ctx context.Context) map[FeatureID]*FeatureStatus {
	features := []FeatureID{
		FeatureHANA,     // Probe first - other features may depend on it
		FeatureAbapGit,
		FeatureRAP,
		FeatureAMDP,
		FeatureUI5,
		FeatureTransport,
	}

	results := make(map[FeatureID]*FeatureStatus)
	for _, id := range features {
		status := p.Probe(ctx, id)
		results[id] = status
	}
	return results
}

// Probe checks if a specific feature is available
func (p *FeatureProber) Probe(ctx context.Context, id FeatureID) *FeatureStatus {
	// Check cache first
	p.mu.RLock()
	if cached, ok := p.cache[id]; ok {
		p.mu.RUnlock()
		return cached
	}
	p.mu.RUnlock()

	// Get configured mode
	mode := p.config.GetMode(id)

	var status *FeatureStatus
	switch mode {
	case FeatureModeOn:
		status = &FeatureStatus{
			ID:        id,
			Available: true,
			Mode:      mode,
			Message:   "forced enabled",
			ProbedAt:  time.Now(),
		}
	case FeatureModeOff:
		status = &FeatureStatus{
			ID:        id,
			Available: false,
			Mode:      mode,
			Message:   "forced disabled",
			ProbedAt:  time.Now(),
		}
	default: // FeatureModeAuto
		status = p.probeFeature(ctx, id)
		status.Mode = mode
	}

	// Cache result
	p.mu.Lock()
	p.cache[id] = status
	p.mu.Unlock()

	if p.verbose {
		availStr := "unavailable"
		if status.Available {
			availStr = "available"
		}
		fmt.Fprintf(LogOutput, "[feature] %s: %s (%s)\n", id, availStr, status.Message)
	}

	return status
}

// IsAvailable checks if a feature is available (uses cache)
func (p *FeatureProber) IsAvailable(ctx context.Context, id FeatureID) bool {
	return p.Probe(ctx, id).Available
}

// probeFeature performs the actual probe for a specific feature
func (p *FeatureProber) probeFeature(ctx context.Context, id FeatureID) *FeatureStatus {
	status := &FeatureStatus{
		ID:       id,
		ProbedAt: time.Now(),
	}

	var err error
	switch id {
	case FeatureHANA:
		status.Available, status.Message, err = p.probeHANA(ctx)
	case FeatureAbapGit:
		status.Available, status.Message, err = p.probeAbapGit(ctx)
	case FeatureRAP:
		status.Available, status.Message, err = p.probeRAP(ctx)
	case FeatureAMDP:
		status.Available, status.Message, err = p.probeAMDP(ctx)
	case FeatureUI5:
		status.Available, status.Message, err = p.probeUI5(ctx)
	case FeatureTransport:
		status.Available, status.Message, err = p.probeTransport(ctx)
	default:
		status.Available = false
		status.Message = "unknown feature"
	}

	if err != nil {
		status.Available = false
		status.Message = fmt.Sprintf("probe failed: %v", err)
	}

	return status
}

// probeHANA checks if running on HANA database
func (p *FeatureProber) probeHANA(ctx context.Context) (bool, string, error) {
	info, err := p.client.GetSystemInfo(ctx)
	if err != nil {
		return false, "", err
	}

	dbSystem := strings.ToUpper(info.DatabaseSystem)
	if strings.Contains(dbSystem, "HDB") || strings.Contains(dbSystem, "HANA") {
		return true, fmt.Sprintf("HANA detected: %s", info.DatabaseSystem), nil
	}

	return false, fmt.Sprintf("non-HANA database: %s", info.DatabaseSystem), nil
}

// probeAbapGit checks if abapGit is installed
func (p *FeatureProber) probeAbapGit(ctx context.Context) (bool, string, error) {
	// Search for abapGit interface - it's the primary indicator
	results, err := p.client.SearchObject(ctx, "ZIF_ABAPGIT*", 1)
	if err != nil {
		return false, "", err
	}

	if len(results) > 0 {
		return true, "ZIF_ABAPGIT* found", nil
	}

	// Also check for /UI2/CL_ABAPGIT* (SAP official package)
	results, err = p.client.SearchObject(ctx, "/UI2/CL_ABAPGIT*", 1)
	if err != nil {
		return false, "", err
	}

	if len(results) > 0 {
		return true, "/UI2/CL_ABAPGIT* found (SAP package)", nil
	}

	return false, "abapGit not found", nil
}

// probeRAP checks if RAP development tools are available
func (p *FeatureProber) probeRAP(ctx context.Context) (bool, string, error) {
	// Check if DDLS endpoint exists
	resp, err := p.client.transport.Request(ctx, "/sap/bc/adt/ddic/ddl/sources", &RequestOptions{
		Method: http.MethodOptions,
	})
	if err != nil {
		// Check if it's a 404 vs connection error
		if strings.Contains(err.Error(), "404") {
			return false, "DDLS endpoint not available", nil
		}
		return false, "", err
	}

	// OPTIONS returning 200 or 405 means endpoint exists
	if resp.StatusCode == 200 || resp.StatusCode == 405 {
		return true, "RAP endpoints available", nil
	}

	return false, "RAP endpoints not responding", nil
}

// probeAMDP checks if AMDP debugging is available
func (p *FeatureProber) probeAMDP(ctx context.Context) (bool, string, error) {
	// AMDP requires HANA - check that first
	hanaStatus := p.Probe(ctx, FeatureHANA)
	if !hanaStatus.Available {
		return false, "AMDP requires HANA database", nil
	}

	// Check if AMDP debugger endpoint exists
	resp, err := p.client.transport.Request(ctx, "/sap/bc/adt/debugger/amdp/sessions", &RequestOptions{
		Method: http.MethodOptions,
	})
	if err != nil {
		if strings.Contains(err.Error(), "404") {
			return false, "AMDP debugger endpoint not available", nil
		}
		return false, "", err
	}

	if resp.StatusCode == 200 || resp.StatusCode == 405 {
		return true, "AMDP debugger available", nil
	}

	return false, "AMDP debugger not responding", nil
}

// probeUI5 checks if UI5/Fiori BSP management is available
func (p *FeatureProber) probeUI5(ctx context.Context) (bool, string, error) {
	// Check if UI5 repository endpoint exists
	resp, err := p.client.transport.Request(ctx, "/sap/bc/adt/filestore/ui5-bsp", &RequestOptions{
		Method: http.MethodOptions,
	})
	if err != nil {
		if strings.Contains(err.Error(), "404") {
			return false, "UI5 BSP endpoint not available", nil
		}
		return false, "", err
	}

	if resp.StatusCode == 200 || resp.StatusCode == 405 {
		return true, "UI5 BSP repository available", nil
	}

	return false, "UI5 BSP not responding", nil
}

// probeTransport checks if CTS transport management is available
func (p *FeatureProber) probeTransport(ctx context.Context) (bool, string, error) {
	// Check if transport endpoint exists
	resp, err := p.client.transport.Request(ctx, "/sap/bc/adt/cts/transports", &RequestOptions{
		Method: http.MethodOptions,
	})
	if err != nil {
		if strings.Contains(err.Error(), "404") {
			return false, "CTS endpoint not available", nil
		}
		return false, "", err
	}

	if resp.StatusCode == 200 || resp.StatusCode == 405 {
		return true, "CTS transport management available", nil
	}

	return false, "CTS not responding", nil
}

// FeatureSummary returns a human-readable summary of all features
func (p *FeatureProber) FeatureSummary(ctx context.Context) string {
	results := p.ProbeAll(ctx)
	var parts []string

	features := []FeatureID{FeatureHANA, FeatureAbapGit, FeatureRAP, FeatureAMDP, FeatureUI5, FeatureTransport}
	for _, id := range features {
		status := results[id]
		symbol := "✗"
		if status.Available {
			symbol = "✓"
		}
		parts = append(parts, fmt.Sprintf("%s %s", symbol, id))
	}

	return strings.Join(parts, " | ")
}

// LogOutput is the writer for verbose logging (set by main)
var LogOutput io.Writer = io.Discard

// SetLogOutput sets the writer for verbose logging
func SetLogOutput(w io.Writer) {
	LogOutput = w
}
