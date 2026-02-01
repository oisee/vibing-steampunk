package adt

import (
	"context"
	"encoding/xml"
	"fmt"
	"net/http"
)

// GetDiscovery retrieves and parses the ADT discovery document.
// Results are cached for the lifetime of the client.
// This is the fastest and most robust way to probe for available features.
func (c *Client) GetDiscovery(ctx context.Context) (*Discovery, error) {
	// Check cache first
	c.discoveryCacheMu.RLock()
	if c.discoveryCache != nil {
		c.discoveryCacheMu.RUnlock()
		return c.discoveryCache, nil
	}
	c.discoveryCacheMu.RUnlock()

	// Fetch discovery document
	resp, err := c.transport.Request(ctx, "/sap/bc/adt/discovery", &RequestOptions{
		Method: http.MethodGet,
		Accept: "application/atomsvc+xml",
	})
	if err != nil {
		return nil, fmt.Errorf("failed to fetch discovery: %w", err)
	}

	// Parse APP service document
	var service APPService
	if err := xml.Unmarshal(resp.Body, &service); err != nil {
		return nil, fmt.Errorf("failed to parse discovery: %w", err)
	}

	// Build discovery object with index
	discovery := &Discovery{
		Workspaces: service.Workspaces,
	}
	discovery.buildCollectionIndex()

	// Cache and return
	c.discoveryCacheMu.Lock()
	c.discoveryCache = discovery
	c.discoveryCacheMu.Unlock()

	return discovery, nil
}

// ClearDiscoveryCache clears the cached discovery document.
// Use this if you need to re-probe after system changes.
func (c *Client) ClearDiscoveryCache() {
	c.discoveryCacheMu.Lock()
	c.discoveryCache = nil
	c.discoveryCacheMu.Unlock()
}

// HasFeatureEndpoint checks if a specific ADT endpoint is available.
// This is a convenience wrapper around GetDiscovery + HasCollection.
func (c *Client) HasFeatureEndpoint(ctx context.Context, uriPath string) (bool, error) {
	discovery, err := c.GetDiscovery(ctx)
	if err != nil {
		return false, err
	}
	return discovery.HasCollection(uriPath), nil
}

// ListDiscoveryCollections returns all collection hrefs from the discovery document.
// Useful for debugging which endpoints are available.
func (c *Client) ListDiscoveryCollections(ctx context.Context) ([]string, error) {
	discovery, err := c.GetDiscovery(ctx)
	if err != nil {
		return nil, err
	}
	var hrefs []string
	for href := range discovery.collections {
		hrefs = append(hrefs, href)
	}
	return hrefs, nil
}

// Well-known ADT endpoints for feature detection.
// These are base paths that should match discovery document collections.
// HasCollection uses prefix matching, so "/sap/bc/adt/ddic/ddl" matches
// discovery entries like "/sap/bc/adt/ddic/ddl/sources".
const (
	// EndpointUI5Repository is the UI5/BSP repository endpoint
	EndpointUI5Repository = "/sap/bc/adt/filestore/ui5-bsp"
	// EndpointTransports is the CTS transport management endpoint
	EndpointTransports = "/sap/bc/adt/cts/transportrequests"
	// EndpointCDSViews is the CDS/DDLS endpoint (discovery may list /ddic/ddl/sources or similar)
	EndpointCDSViews = "/sap/bc/adt/ddic/ddl"
	// EndpointBehaviorDef is the behavior definition endpoint (BDEF)
	EndpointBehaviorDef = "/sap/bc/adt/bo/behavior"
	// EndpointServiceDef is the service definition endpoint (SRVD)
	// Discovery lists /sap/bc/adt/ddic/srvd/srvd
	EndpointServiceDef = "/sap/bc/adt/ddic/srvd"
	// EndpointServiceBinding is the service binding endpoint (SRVB)
	// Discovery lists /sap/bc/adt/ddic/srvb/srvb
	EndpointServiceBinding = "/sap/bc/adt/ddic/srvb"
)
