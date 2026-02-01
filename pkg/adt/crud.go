// ABOUTME: Source update operations for ABAP objects.
// ABOUTME: Provides UpdateSource and UpdateObjectDescription for ABAP objects.

package adt

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strings"
)

// UpdateSource writes source code to an ABAP object.
// objectSourceURL is the source URL (e.g., "/sap/bc/adt/programs/programs/ZTEST/source/main")
// lockHandle is required (from LockObject)
// transport is optional (for transportable objects)
func (c *Client) UpdateSource(ctx context.Context, objectSourceURL string, source string, lockHandle string, transport string) error {
	if err := c.checkSafety(OpUpdate, "UpdateSource"); err != nil {
		return err
	}

	params := url.Values{}
	params.Set("lockHandle", lockHandle)
	if transport != "" {
		params.Set("corrNr", transport)
	}

	contentType := "text/plain; charset=utf-8"
	if strings.HasPrefix(strings.TrimSpace(source), "<?xml") {
		contentType = "application/*"
	}

	_, err := c.transport.Request(ctx, objectSourceURL, &RequestOptions{
		Method:      http.MethodPut,
		Query:       params,
		Body:        []byte(source),
		ContentType: contentType,
	})
	if err != nil {
		return fmt.Errorf("updating source: %w", err)
	}

	return nil
}

// UpdateObjectDescription updates the description of an ABAP object.
// objectURL is the object URL (e.g., "/sap/bc/adt/oo/classes/ZCL_TEST")
// objectType is the CreatableObjectType (e.g., ObjectTypeClass)
// name is the object name
// description is the new description
// lockHandle is required (from LockObject)
// transport is optional (for transportable objects)
func (c *Client) UpdateObjectDescription(ctx context.Context, objectURL string, objectType CreatableObjectType, name, description, lockHandle, transport string) error {
	if err := c.checkSafety(OpUpdate, "UpdateObjectDescription"); err != nil {
		return err
	}

	typeInfo, ok := getObjectTypeInfo(objectType)
	if !ok {
		return fmt.Errorf("unknown object type: %s", objectType)
	}

	// Build minimal XML body with updated description
	body := fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?>
<%s %s xmlns:adtcore="http://www.sap.com/adt/core"
  adtcore:description="%s"
  adtcore:name="%s">
</%s>`,
		typeInfo.rootName, typeInfo.namespace,
		escapeXML(description),
		name,
		typeInfo.rootName)

	params := url.Values{}
	params.Set("lockHandle", lockHandle)
	if transport != "" {
		params.Set("corrNr", transport)
	}

	// Use appropriate content type based on object type
	contentType := "application/*"
	switch objectType {
	case ObjectTypeClass:
		contentType = "application/vnd.sap.adt.oo.classes.v2+xml"
	case ObjectTypeInterface:
		contentType = "application/vnd.sap.adt.oo.interfaces.v2+xml"
	case ObjectTypeProgram:
		contentType = "application/vnd.sap.adt.programs.programs.v2+xml"
	}

	_, err := c.transport.Request(ctx, objectURL, &RequestOptions{
		Method:      http.MethodPut,
		Query:       params,
		Body:        []byte(body),
		ContentType: contentType,
	})
	if err != nil {
		return fmt.Errorf("updating object description: %w", err)
	}

	return nil
}
