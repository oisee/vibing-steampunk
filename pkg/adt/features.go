// Package adt provides a Go client for SAP ABAP Development Tools (ADT) REST API.
package adt

import (
	"context"
	"fmt"
	"io"
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

// allFeatures defines the canonical order for probing (HANA first, others depend on it)
var allFeatures = []FeatureID{
	FeatureHANA, // Probe first - other features may depend on it
	FeatureAbapGit,
	FeatureRAP,
	FeatureAMDP,
	FeatureUI5,
	FeatureTransport,
}

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
	ID        FeatureID   `json:"id"`
	Available bool        `json:"available"`
	Mode      FeatureMode `json:"mode"`
	Message   string      `json:"message,omitempty"`
	ProbedAt  time.Time   `json:"probed_at,omitempty"`
}

// FeatureConfig controls which optional features are enabled
type FeatureConfig struct {
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
	results := make(map[FeatureID]*FeatureStatus)
	for _, id := range allFeatures {
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
	// Use shared GetDatabaseInfo which queries DB6NAVSYST
	dbInfo, err := p.client.GetDatabaseInfo(ctx)
	if err != nil {
		return false, "", fmt.Errorf("database info query failed: %w", err)
	}
	if dbInfo == nil {
		return false, "no database info available", nil
	}

	if strings.ToUpper(dbInfo.System) == "HDB" {
		msg := fmt.Sprintf("HANA %s", dbInfo.Release)
		if dbInfo.Host != "" {
			msg += fmt.Sprintf(" (host: %s)", dbInfo.Host)
		}
		return true, msg, nil
	}

	return false, fmt.Sprintf("non-HANA database: %s", dbInfo.System), nil
}

// probeAbapGit checks if abapGit is installed
// abapGit is a community add-on - it doesn't register in ADT discovery,
// so we use object search directly. Checks for:
// - Developer edition: ZIF_ABAPGIT* interfaces (full package structure)
// - Standalone edition: ZABAPGIT program (single file)
func (p *FeatureProber) probeAbapGit(ctx context.Context) (bool, string, error) {
	// Check for developer edition first (full package with interfaces)
	// This takes priority because systems with dev edition also have ZABAPGIT program
	results, err := p.client.SearchObject(ctx, "ZIF_ABAPGIT*", 1)
	if err != nil {
		return false, "", err
	}
	if len(results) > 0 {
		return true, "abapGit installed (developer edition)", nil
	}

	// Check for standalone edition (single program, no interfaces)
	results, err = p.client.SearchObject(ctx, "ZABAPGIT*", 1)
	if err != nil {
		return false, "", err
	}
	if len(results) > 0 {
		return true, "abapGit installed (standalone)", nil
	}

	return false, "abapGit not found", nil
}

// probeRAP checks if RAP development tools are available
// Uses discovery endpoint check - checks for CDS/DDLS, BDEF, SRVD endpoints
func (p *FeatureProber) probeRAP(ctx context.Context) (bool, string, error) {
	// Check discovery first - fastest and most reliable
	discovery, err := p.client.GetDiscovery(ctx)
	if err == nil {
		// Check for CDS views endpoint (primary RAP indicator)
		if discovery.HasCollection(EndpointCDSViews) {
			return true, "CDS views endpoint available", nil
		}
		// Also check for behavior definitions
		if discovery.HasCollection(EndpointBehaviorDef) {
			return true, "BDEF endpoint available", nil
		}
	}

	// Fall back to object search
	results, err := p.client.SearchObject(ctx, "I_*", 3)
	if err != nil {
		return false, "", fmt.Errorf("search failed: %w", err)
	}
	for _, r := range results {
		if strings.HasPrefix(r.Type, "DDLS") {
			return true, fmt.Sprintf("CDS views available (found %s)", r.Name), nil
		}
	}

	return false, "RAP endpoints not found", nil
}

// probeAMDP checks if AMDP debugging is available
// AMDP is a HANA feature - if HANA is available, AMDP is supported
func (p *FeatureProber) probeAMDP(ctx context.Context) (bool, string, error) {
	// AMDP requires HANA - that's the only check needed
	hanaStatus := p.Probe(ctx, FeatureHANA)
	if !hanaStatus.Available {
		return false, "AMDP requires HANA database", nil
	}

	return true, "AMDP supported (HANA detected)", nil
}

// probeUI5 checks if UI5/Fiori BSP management is available
// Uses discovery endpoint check first, falls back to functional probe
func (p *FeatureProber) probeUI5(ctx context.Context) (bool, string, error) {
	// Check discovery first - fastest and most reliable
	discovery, err := p.client.GetDiscovery(ctx)
	if err == nil && discovery.HasCollection(EndpointUI5Repository) {
		return true, "UI5 BSP endpoint available", nil
	}

	// Fall back to functional probe
	apps, err := p.client.UI5ListApps(ctx, "Z*", 1)
	if err != nil {
		if strings.Contains(err.Error(), "404") {
			return false, "UI5 BSP repository not available", nil
		}
		return false, fmt.Sprintf("UI5 probe failed: %v", err), nil
	}

	if len(apps) > 0 {
		return true, fmt.Sprintf("UI5 BSP available (%d apps found)", len(apps)), nil
	}

	return true, "UI5 BSP repository available", nil
}

// probeTransport checks if CTS transport management is available
// Uses discovery endpoint check first, falls back to functional probe
func (p *FeatureProber) probeTransport(ctx context.Context) (bool, string, error) {
	// Check discovery first - fastest and most reliable
	discovery, err := p.client.GetDiscovery(ctx)
	if err == nil && discovery.HasCollection(EndpointTransports) {
		return true, "CTS endpoint available", nil
	}

	// Fall back to functional probe
	transports, err := p.client.ListTransports(ctx, "*")
	if err != nil {
		errStr := err.Error()
		if strings.Contains(errStr, "not enabled") {
			return true, "CTS available (disabled by config)", nil
		}
		if strings.Contains(errStr, "404") {
			return false, "CTS endpoint not available", nil
		}
		return true, "CTS available (access restricted)", nil
	}

	return true, fmt.Sprintf("CTS available (%d transports)", len(transports)), nil
}

// FeatureSummary returns a human-readable summary of all features
func (p *FeatureProber) FeatureSummary(ctx context.Context) string {
	results := p.ProbeAll(ctx)
	var parts []string

	for _, id := range allFeatures {
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
