package adt

import (
	"context"
	"fmt"
	"net/url"
	"strings"
)

// ObjectSourceConfig defines how to read source for an object type.
type ObjectSourceConfig struct {
	// URLPattern is the URL template with %s for the object name
	URLPattern string
	// NameCase determines how to transform the name: "upper", "lower", or "preserve"
	NameCase string
	// Accept is the Accept header value (empty = default)
	Accept string
	// Description for error messages
	Description string
}

// objectSourceConfigs maps object type codes to their source retrieval configuration.
// This is the single source of truth for ADT URL patterns.
var objectSourceConfigs = map[string]ObjectSourceConfig{
	// Programs and Includes
	"PROG": {
		URLPattern:  "/sap/bc/adt/programs/programs/%s/source/main",
		NameCase:    "upper",
		Description: "program",
	},
	"INCL": {
		URLPattern:  "/sap/bc/adt/programs/includes/%s/source/main",
		NameCase:    "upper",
		Description: "include",
	},

	// Object-Oriented
	"CLAS": {
		URLPattern:  "/sap/bc/adt/oo/classes/%s/source/main",
		NameCase:    "lower",
		Description: "class",
	},
	"INTF": {
		URLPattern:  "/sap/bc/adt/oo/interfaces/%s/source/main",
		NameCase:    "lower",
		Description: "interface",
	},

	// DDIC Objects
	"TABL": {
		URLPattern:  "/sap/bc/adt/ddic/tables/%s/source/main",
		NameCase:    "upper",
		Description: "table",
	},
	"VIEW": {
		URLPattern:  "/sap/bc/adt/ddic/views/%s",
		NameCase:    "upper",
		Description: "view",
	},
	"STRU": {
		URLPattern:  "/sap/bc/adt/ddic/structures/%s/source/main",
		NameCase:    "upper",
		Description: "structure",
	},
	"DTEL": {
		URLPattern:  "/sap/bc/adt/ddic/dataelements/%s",
		NameCase:    "upper",
		Description: "data element",
	},
	"DOMA": {
		URLPattern:  "/sap/bc/adt/ddic/domains/%s",
		NameCase:    "upper",
		Description: "domain",
	},

	// CDS and RAP
	"DDLS": {
		URLPattern:  "/sap/bc/adt/ddic/ddl/sources/%s/source/main",
		NameCase:    "lower",
		Description: "CDS DDL source",
	},
	"BDEF": {
		URLPattern:  "/sap/bc/adt/bo/behaviordefinitions/%s/source/main",
		NameCase:    "lower",
		Description: "behavior definition",
	},
	"SRVD": {
		URLPattern:  "/sap/bc/adt/ddic/srvd/sources/%s/source/main",
		NameCase:    "lower",
		Description: "service definition",
	},

	// Transformations
	"XSLT": {
		URLPattern:  "/sap/bc/adt/xslt/transformations/%s/source/main",
		NameCase:    "lower",
		Accept:      "text/plain",
		Description: "simple transformation",
	},
}

// GetObjectSourceConfig returns the source configuration for an object type.
// Returns nil if the object type is not supported for simple source reading.
func GetObjectSourceConfig(objectType string) *ObjectSourceConfig {
	objectType = strings.ToUpper(objectType)
	if cfg, ok := objectSourceConfigs[objectType]; ok {
		return &cfg
	}
	return nil
}

// SupportedSourceTypes returns a list of all object types that support simple source reading.
func SupportedSourceTypes() []string {
	types := make([]string, 0, len(objectSourceConfigs))
	for t := range objectSourceConfigs {
		types = append(types, t)
	}
	return types
}

// getObjectSourceByType is the generic source reader using the configuration map.
// This is the DRY implementation that all specific Get* methods delegate to.
func (c *Client) getObjectSourceByType(ctx context.Context, objectType, name string) (string, error) {
	cfg := GetObjectSourceConfig(objectType)
	if cfg == nil {
		return "", fmt.Errorf("unsupported object type for source reading: %s", objectType)
	}

	// Apply name case transformation
	switch cfg.NameCase {
	case "upper":
		name = strings.ToUpper(name)
	case "lower":
		name = strings.ToLower(name)
	// "preserve" or empty = no transformation
	}

	// Build URL with proper escaping for namespaced objects
	sourcePath := fmt.Sprintf(cfg.URLPattern, url.PathEscape(name))

	opts := &RequestOptions{
		Method: "GET",
	}
	if cfg.Accept != "" {
		opts.Accept = cfg.Accept
	}

	resp, err := c.transport.Request(ctx, sourcePath, opts)
	if err != nil {
		return "", fmt.Errorf("getting %s source: %w", cfg.Description, err)
	}

	return string(resp.Body), nil
}

// GetObjectSource reads source code for any supported object type.
// This is the unified entry point for simple source reading.
func (c *Client) GetObjectSource(ctx context.Context, objectType, name string) (string, error) {
	return c.getObjectSourceByType(ctx, objectType, name)
}
