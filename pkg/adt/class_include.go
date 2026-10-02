package adt

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strings"
)

// --- Class Include Operations ---

// ClassIncludeType represents the type of class include.
type ClassIncludeType string

const (
	ClassIncludeMain            ClassIncludeType = "main"
	ClassIncludeDefinitions     ClassIncludeType = "definitions"
	ClassIncludeImplementations ClassIncludeType = "implementations"
	ClassIncludeMacros          ClassIncludeType = "macros"
	ClassIncludeTestClasses     ClassIncludeType = "testclasses"
)

// ClassIncludeForSection maps the suffix a cross-reference row carries to the
// ADT address of that part of the class.
//
// The pairs are measured against a live 7.58, not inferred, and the set is
// exactly this: includes/main, includes/localtypes and
// includes/localimplementations do not answer — 404, 404 and 400 — so an
// address cannot be invented from a suffix by pattern.
//
// The second return says whether the section has an address of its own at all.
// CP, CU, CO, CI and the CM### method includes do not: their source is the main
// source, and a caller must read that rather than guess a path.
func ClassIncludeForSection(section string) (ClassIncludeType, bool) {
	switch strings.ToUpper(strings.TrimSpace(section)) {
	case "CCAU":
		return ClassIncludeTestClasses, true
	case "CCDEF":
		return ClassIncludeDefinitions, true
	case "CCIMP":
		return ClassIncludeImplementations, true
	case "CCMAC":
		return ClassIncludeMacros, true
	}
	return ClassIncludeMain, false
}

// unescapeObjectName returns the raw object name for a name that may arrive
// already escaped from a URL (%2FDMO%2FCL_FLIGHT -> /DMO/CL_FLIGHT), so the
// caller can escape it exactly once. A name that is not a valid escape
// sequence is returned unchanged.
func unescapeObjectName(name string) string {
	if raw, err := url.PathUnescape(name); err == nil {
		return raw
	}
	return name
}

// GetClassIncludeURL returns the URL for a class include.
// Supports namespaced classes like /UI5/CL_REPOSITORY_LOAD.
func GetClassIncludeURL(className string, includeType ClassIncludeType) string {
	className = strings.ToUpper(unescapeObjectName(className))
	encodedName := url.PathEscape(className)
	if includeType == ClassIncludeMain {
		return fmt.Sprintf("/sap/bc/adt/oo/classes/%s/source/main", encodedName)
	}
	return fmt.Sprintf("/sap/bc/adt/oo/classes/%s/includes/%s", encodedName, includeType)
}

// GetClassIncludeSourceURL returns the source URL for a class include.
// Note: For includes other than main, the URL does NOT have /source/main suffix
// Supports namespaced classes like /UI5/CL_REPOSITORY_LOAD.
//
// The name may be raw (/DMO/CL_FLIGHT) or already escaped from a URL
// (%2FDMO%2FCL_FLIGHT); either way it is escaped exactly once.
func GetClassIncludeSourceURL(className string, includeType ClassIncludeType) string {
	className = strings.ToUpper(unescapeObjectName(className))
	encodedName := url.PathEscape(className)
	if includeType == ClassIncludeMain {
		return fmt.Sprintf("/sap/bc/adt/oo/classes/%s/source/main", encodedName)
	}
	// For other includes (definitions, implementations, macros, testclasses),
	// the source is accessed directly at the include URL without /source/main
	return fmt.Sprintf("/sap/bc/adt/oo/classes/%s/includes/%s", encodedName, includeType)
}

// CreateTestInclude creates the test classes include for a class.
// This must be called before you can write test class code.
// Requires a lock on the parent class.
// Supports namespaced classes.
func (c *Client) CreateTestInclude(ctx context.Context, className string, lockHandle string, transport string) error {
	// The name may arrive already escaped from a URL; normalize to the raw
	// name so it is escaped exactly once below.
	className = strings.ToUpper(unescapeObjectName(className))

	// Unified mutation policy gate (op type + parent class package + transport)
	if err := c.checkMutation(ctx, MutationContext{
		Op:         OpCreate,
		OpName:     "CreateTestInclude",
		ObjectURL:  GetObjectURL(ObjectTypeClass, className, ""),
		Transport:  transport,
		LockHandle: lockHandle,
	}); err != nil {
		return err
	}

	body := `<?xml version="1.0" encoding="UTF-8"?>
<class:abapClassInclude xmlns:class="http://www.sap.com/adt/oo/classes"
  xmlns:adtcore="http://www.sap.com/adt/core"
  adtcore:name="dummy" class:includeType="testclasses"/>`

	params := url.Values{}
	params.Set("lockHandle", lockHandle)
	if transport != "" {
		params.Set("corrNr", transport)
	}

	// URL encode for namespaced objects
	includesURL := fmt.Sprintf("/sap/bc/adt/oo/classes/%s/includes", url.PathEscape(className))
	_, err := c.transport.Request(ctx, includesURL, &RequestOptions{
		Method:      http.MethodPost,
		Query:       params,
		Body:        []byte(body),
		ContentType: "application/*",
		Stateful:    true, // Must match lock session — the lock was acquired statefully (issues #88/#92/#98)
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

	// Unified mutation policy gate (op type + package + transport)
	if err := c.checkMutation(ctx, MutationContext{
		Op:         OpUpdate,
		OpName:     "UpdateClassInclude",
		ObjectURL:  sourceURL,
		Transport:  transport,
		LockHandle: lockHandle,
	}); err != nil {
		return err
	}
	if err := c.verifyExpectedSourceHash(ctx, sourceURL); err != nil {
		return err
	}

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
		Stateful:    true, // Must match lock session — the lock was acquired statefully (issues #88/#92/#98)
	})
	if err != nil {
		return &classIncludePutError{err: err}
	}

	return nil
}
