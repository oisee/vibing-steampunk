// ABOUTME: Class include operations for ABAP classes.
// ABOUTME: Provides GetClassIncludeURL, CreateTestInclude, GetClassInclude, UpdateClassInclude.

package adt

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strings"
)

// ClassIncludeType represents the type of class include.
type ClassIncludeType string

const (
	ClassIncludeMain            ClassIncludeType = "main"
	ClassIncludeDefinitions     ClassIncludeType = "definitions"
	ClassIncludeImplementations ClassIncludeType = "implementations"
	ClassIncludeMacros          ClassIncludeType = "macros"
	ClassIncludeTestClasses     ClassIncludeType = "testclasses"
)

// GetClassIncludeURL returns the URL for a class include.
// Supports namespaced classes like /UI5/CL_REPOSITORY_LOAD.
func GetClassIncludeURL(className string, includeType ClassIncludeType) string {
	className = strings.ToUpper(className)
	encodedName := url.PathEscape(className)
	if includeType == ClassIncludeMain {
		return fmt.Sprintf("/sap/bc/adt/oo/classes/%s/source/main", encodedName)
	}
	return fmt.Sprintf("/sap/bc/adt/oo/classes/%s/includes/%s", encodedName, includeType)
}

// GetClassIncludeSourceURL returns the source URL for a class include.
// Note: For includes other than main, the URL does NOT have /source/main suffix.
// Supports namespaced classes like /UI5/CL_REPOSITORY_LOAD.
func GetClassIncludeSourceURL(className string, includeType ClassIncludeType) string {
	className = strings.ToUpper(className)
	encodedName := url.PathEscape(className)
	if includeType == ClassIncludeMain {
		return fmt.Sprintf("/sap/bc/adt/oo/classes/%s/source/main", encodedName)
	}
	return fmt.Sprintf("/sap/bc/adt/oo/classes/%s/includes/%s", encodedName, includeType)
}

// CreateTestInclude creates the test classes include for a class.
// This must be called before you can write test class code.
// Requires a lock on the parent class.
func (c *Client) CreateTestInclude(ctx context.Context, className string, lockHandle string, transport string) error {
	className = strings.ToUpper(className)

	body := `<?xml version="1.0" encoding="UTF-8"?>
<class:abapClassInclude xmlns:class="http://www.sap.com/adt/oo/classes"
  xmlns:adtcore="http://www.sap.com/adt/core"
  adtcore:name="dummy" class:includeType="testclasses"/>`

	params := url.Values{}
	params.Set("lockHandle", lockHandle)
	if transport != "" {
		params.Set("corrNr", transport)
	}

	includesURL := fmt.Sprintf("/sap/bc/adt/oo/classes/%s/includes", url.PathEscape(className))
	_, err := c.transport.Request(ctx, includesURL, &RequestOptions{
		Method:      http.MethodPost,
		Query:       params,
		Body:        []byte(body),
		ContentType: "application/*",
	})
	if err != nil {
		return fmt.Errorf("creating test include: %w", err)
	}

	return nil
}

// GetClassInclude retrieves the source code of a class include.
func (c *Client) GetClassInclude(ctx context.Context, className string, includeType ClassIncludeType) (string, error) {
	sourceURL := GetClassIncludeSourceURL(className, includeType)

	resp, err := c.transport.Request(ctx, sourceURL, &RequestOptions{
		Method: http.MethodGet,
	})
	if err != nil {
		return "", fmt.Errorf("getting class include: %w", err)
	}

	return string(resp.Body), nil
}

// UpdateClassInclude updates the source code of a class include.
// Requires a lock on the parent class.
func (c *Client) UpdateClassInclude(ctx context.Context, className string, includeType ClassIncludeType, source string, lockHandle string, transport string) error {
	sourceURL := GetClassIncludeSourceURL(className, includeType)

	params := url.Values{}
	params.Set("lockHandle", lockHandle)
	if transport != "" {
		params.Set("corrNr", transport)
	}

	_, err := c.transport.Request(ctx, sourceURL, &RequestOptions{
		Method:      http.MethodPut,
		Query:       params,
		Body:        []byte(source),
		ContentType: "text/plain; charset=utf-8",
	})
	if err != nil {
		return fmt.Errorf("updating class include: %w", err)
	}

	return nil
}
