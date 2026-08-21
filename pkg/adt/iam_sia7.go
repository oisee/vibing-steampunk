package adt

// Business catalog IAM app assignment (ADT object type SIA7) support.
//
// This is the object that puts an IAM app into a business catalog, and it is
// the piece that makes a published RAP UI service reachable by a business user:
//
//	service binding (published)
//	  -> IAM business service   SIA6, appType IBS, generated
//	       -> IAM app           SIA6, appType EXT, created by hand, published
//	            -> catalog      SIA1
//	                 + THIS     SIA7, catalog + app
//	                      -> business role -> business user
//
// iam_sia1.go concluded that assignments "cannot be done over ADT" because the
// SIA1 $bucapps sub-resource implements only GET and answers 405 to every write
// verb. The observation is correct and the conclusion was not: Eclipse does not
// write $bucapps either. It creates a first-class SIA7 object, which is an
// ordinary ADT create against its own collection.
//
// Payload read from a live tenant (a live S/4HANA Cloud tenant, 2026-08-13):
//
//	GET /sap/bc/adt/aps/cloud/iam/sia7/<name>
//	-> root sia7:sia7, namespace http://www.sap.com/iam/sia7, adtcore header,
//	   adtcore:packageRef, then sia7:content with
//	   businessCatalogAppAssignmentID, businessCatalogID, appID, roleChange,
//	   catalogChange, groupChange and chipID.
//
// chipID is deliberately not written. The observed value carried a generated
// suffix — X-SAP-UI2-PAGE:X-SAP-UI2-CATALOGPAGE:<catalog>:<opaque> — that only
// the backend can mint, so sending a fabricated one is worse than omitting it.
// The three *Change flags were all "A" on a freshly created assignment.
//
// IMPORTANT: create against an existing assignment REPLACES it. ADT does not
// raise ExceptionResourceAlreadyExists for SIA7 the way it does for most object
// types — it overwrites the content and mints a fresh chipID. Verified on the tenant
// 080, 2026-08-13: re-creating ZBC_MY_CATALOG_0001 with a different appID
// changed both the appID and adtcore:createdAt, and issued a new chipID suffix.
//
// So this is an upsert in practice. Callers must not assume an existing
// assignment is safe from a create, and the "already existed" branch in the
// handler is effectively unreachable for this type.

import (
	"context"
	"fmt"
	"strings"
)

// ObjectTypeCatalogAppAssignment is the business catalog IAM app assignment,
// ADT type SIA7.
const ObjectTypeCatalogAppAssignment CreatableObjectType = "SIA7/AS"

// CatalogAppAssignmentPath is the ADT collection holding the assignments.
const CatalogAppAssignmentPath = "/sap/bc/adt/aps/cloud/iam/sia7"

func init() {
	objectTypes[ObjectTypeCatalogAppAssignment] = objectTypeInfo{
		creationPath: CatalogAppAssignmentPath,
		rootName:     "sia7:sia7",
		namespace:    `xmlns:sia7="http://www.sap.com/iam/sia7"`,
		bodyBuilder:  buildCatalogAppAssignmentBody,
	}
}

// CatalogAppAssignmentURL returns the ADT URL of an assignment. ADT lowercases
// the object name in the path.
func CatalogAppAssignmentURL(name string) string {
	return CatalogAppAssignmentPath + "/" + strings.ToLower(name)
}

// CatalogAppAssignmentExists reports whether an assignment is already present.
func (c *Client) CatalogAppAssignmentExists(ctx context.Context, name string) bool {
	resp, err := c.RawGet(ctx, CatalogAppAssignmentURL(name), "*/*")
	if err != nil {
		return false
	}
	return resp.StatusCode >= 200 && resp.StatusCode < 300
}

// DefaultAssignmentName returns the name the workbench gives an assignment:
// the catalog followed by a four digit sequence number.
func DefaultAssignmentName(catalogID string, sequence int) string {
	return fmt.Sprintf("%s_%04d", strings.ToUpper(catalogID), sequence)
}

func buildCatalogAppAssignmentBody(opts CreateObjectOptions, typeInfo objectTypeInfo, responsible string) string {
	name := strings.ToUpper(opts.Name)
	description := opts.Description
	if description == "" {
		description = "Business Catalog to IAM App assignment"
	}

	return fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?>
<%s %s xmlns:adtcore="http://www.sap.com/adt/core"
  adtcore:description="%s"
  adtcore:name="%s"
  adtcore:type="SIA7"
  adtcore:responsible="%s"
  adtcore:masterLanguage="EN"
  adtcore:language="EN">
  <adtcore:packageRef adtcore:name="%s"/>
  <sia7:content>
    <sia7:businessCatalogAppAssignmentID>%s</sia7:businessCatalogAppAssignmentID>
    <sia7:businessCatalogID>%s</sia7:businessCatalogID>
    <sia7:appID>%s</sia7:appID>
    <sia7:roleChange>A</sia7:roleChange>
    <sia7:catalogChange>A</sia7:catalogChange>
    <sia7:groupChange>A</sia7:groupChange>
  </sia7:content>
</%s>`,
		typeInfo.rootName, typeInfo.namespace,
		escapeXML(description),
		name,
		responsible,
		strings.ToUpper(opts.PackageName),
		name,
		escapeXML(strings.ToUpper(opts.BusinessCatalogID)),
		escapeXML(strings.ToUpper(opts.AppID)),
		typeInfo.rootName)
}

// CatalogAppAssignment is the stored content of an SIA7 object.
type CatalogAppAssignment struct {
	AssignmentID string
	CatalogID    string
	AppID        string
}

// ReadCatalogAppAssignment returns what an assignment actually contains.
//
// Needed because create is not idempotent in the way it first appears: ADT
// answers an existing object with ExceptionResourceAlreadyExists and does not
// merge the payload, so a caller that treats "already exists" as success and
// then reports its own request values is describing an object it never wrote.
// Read the object instead of trusting the request.
func (c *Client) ReadCatalogAppAssignment(ctx context.Context, name string) (*CatalogAppAssignment, error) {
	resp, err := c.RawGet(ctx, CatalogAppAssignmentURL(name), "*/*")
	if err != nil {
		return nil, err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("reading assignment %s: status %d", strings.ToUpper(name), resp.StatusCode)
	}
	body := string(resp.Body)
	return &CatalogAppAssignment{
		AssignmentID: firstXMLValue(body, "sia7:businessCatalogAppAssignmentID"),
		CatalogID:    firstXMLValue(body, "sia7:businessCatalogID"),
		AppID:        firstXMLValue(body, "sia7:appID"),
	}, nil
}

// firstXMLValue pulls the text of the first <tag>...</tag> out of an ADT
// response. Deliberately not a full parse: these resources return a large
// document with several namespaces and only three values are wanted.
func firstXMLValue(doc, tag string) string {
	open, close := "<"+tag+">", "</"+tag+">"
	i := strings.Index(doc, open)
	if i < 0 {
		return ""
	}
	rest := doc[i+len(open):]
	j := strings.Index(rest, close)
	if j < 0 {
		return ""
	}
	return rest[:j]
}
