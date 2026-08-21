package adt

// Business catalog (ADT object type SIA1) support.
//
// A published RAP service binding on S/4HANA Cloud generates an IAM app, and
// that app grants nothing until it sits in a business catalog that a business
// role includes. Without this type the last step of a RAP stack cannot be
// automated at all.
//
// Everything below was read out of a live S/4HANA Cloud tenant rather than
// guessed. The ADT serialization for SIA1 is implemented as simple
// transformations in package SR_APS_IAM_WBI_SIA1:
//
//	transformation SIA1          -> root element sia1:sia1, namespace
//	                                http://www.sap.com/iam/sia1, includes
//	                                SADT_MAIN_OBJECT for the adtcore header
//	transformation SIA1_BUCAPPS  -> sia1:bucapps / sia1:bucapp with
//	                                businessCatalogAppAssignmentID,
//	                                businessCatalogID, appID and
//	                                assignableRtypes
//
// and CL_APS_IAM_WBI_SIA1_WB_ACCESS registers the sub-resources:
//
//	/aps/cloud/iam/sia1/$bucapps{?name}   application/vnd.sap.adt.iam.bucapps+xml
//	/aps/cloud/iam/sia1/$publish{?businessCatalogID}
//	/aps/cloud/iam/sia1/$getScope{?name}
//
// The {?name} on $publish was wrong, and cost a release: the resource takes
// businessCatalogID. None of these sub-resources appear in the ADT discovery
// document at all, so the query parameter cannot be read from there — but ADT
// names the one it wants when you get it wrong:
//
//	POST .../sia1/$publish?name=X  ->  400 "Parameter businessCatalogID could not be found."
//
// which is the cheapest way to discover the signature of an unlisted resource.

import (
	"context"
	"encoding/xml"
	"fmt"
	"net/http"
	"net/url"
	"strings"
)

// ObjectTypeBusinessCatalog is the IAM business catalog, ADT type SIA1.
const ObjectTypeBusinessCatalog CreatableObjectType = "SIA1/BC"

// BusinessCatalogPath is the ADT collection holding business catalogs.
const BusinessCatalogPath = "/sap/bc/adt/aps/cloud/iam/sia1"

// BucappsContentType is the media type CL_APS_IAM_WBI_SIA1_WB_ACCESS declares
// for the $bucapps sub-resource.
const BucappsContentType = "application/vnd.sap.adt.iam.bucapps+xml"

// Registered from here rather than by editing the objectTypes literal in
// crud.go, so this file can be added to a fork without touching shared code.
//
// Note that GetObjectURL in crud.go switches over known types and will return
// an empty string for SIA1/BC. The only consequence is that CreateObject skips
// its orphan-lock cleanup retry for this type. BusinessCatalogURL below gives
// the URL for every other purpose.
func init() {
	objectTypes[ObjectTypeBusinessCatalog] = objectTypeInfo{
		creationPath: BusinessCatalogPath,
		rootName:     "sia1:sia1",
		namespace:    `xmlns:sia1="http://www.sap.com/iam/sia1"`,
	}
}

// BusinessCatalogURL returns the ADT URL of a business catalog. ADT lowercases
// the object name in the path.
func BusinessCatalogURL(name string) string {
	return BusinessCatalogPath + "/" + strings.ToLower(name)
}

// IsResourceAlreadyExists reports whether an ADT error is the backend saying
// the object is already there.
//
// ADT returns a typed exception id, ExceptionResourceAlreadyExists, alongside a
// translated message. Match the id: the message text is localised and changes
// between releases, the id does not.
func IsResourceAlreadyExists(err error) bool {
	if err == nil {
		return false
	}
	return strings.Contains(err.Error(), "ExceptionResourceAlreadyExists")
}

// BusinessCatalogExists reports whether a business catalog is already present.
//
// Secondary to IsResourceAlreadyExists, for the case where a create failed for
// some other reason after SAP had already persisted the object.
//
// Accept is */* deliberately. An ADT resource serves its own vnd.sap.adt media
// type and answers 406 for anything it does not recognise, so a narrower Accept
// turns an existing object into a false negative.
func (c *Client) BusinessCatalogExists(ctx context.Context, name string) bool {
	resp, err := c.RawGet(ctx, BusinessCatalogURL(name), "*/*")
	if err != nil {
		return false
	}
	return resp.StatusCode >= 200 && resp.StatusCode < 300
}

// requestWithVerbFallback sends a request and retries once with a different
// method if ADT answers 405.
//
// The HTTP verbs of the SIA1 sub-resources are documented nowhere and are not
// visible in the transformations or in the resource registration, which declare
// only routes and content types. A first run against N4D showed $bucapps
// rejecting POST with "Resource controller does not support method POST", so
// the verb has to be discovered rather than assumed. 405 is an unambiguous
// signal and the retry is bounded to one alternative, which beats hardcoding a
// guess that then fails the same way on the next sub-resource.
func (c *Client) requestWithVerbFallback(ctx context.Context, path string, primary, fallback string, opts *RequestOptions) (*Response, string, error) {
	opts.Method = primary
	resp, err := c.transport.Request(ctx, path, opts)
	if err == nil {
		return resp, primary, nil
	}
	if !strings.Contains(err.Error(), "status 405") {
		return nil, primary, err
	}

	retryOpts := *opts
	retryOpts.Method = fallback
	resp, fallbackErr := c.transport.Request(ctx, path, &retryOpts)
	if fallbackErr != nil {
		return nil, fallback, fmt.Errorf("%s rejected with 405, %s also failed: %w", primary, fallback, fallbackErr)
	}
	return resp, fallback, nil
}

// BusinessCatalogApp is one IAM app assignment inside a business catalog.
type BusinessCatalogApp struct {
	// AppID is the IAM app, for example a generated
	// ZUI_MY_SERVICE_O4_0001_XXXX_IBS.
	AppID string
	// AssignmentID is the app assignment ID. Leave empty to derive it from
	// the catalog and app, which is what the workbench does.
	AssignmentID string
}

// bucappsPayload mirrors transformation SIA1_BUCAPPS.
type bucappsPayload struct {
	XMLName xml.Name `xml:"sia1:bucapps"`
	NS      string   `xml:"xmlns:sia1,attr"`
	Apps    []bucapp `xml:"sia1:bucapp"`
}

type bucapp struct {
	AssignmentID string `xml:"sia1:businessCatalogAppAssignmentID"`
	CatalogID    string `xml:"sia1:businessCatalogID"`
	AppID        string `xml:"sia1:appID"`
}

// AssignBusinessCatalogApps attempts to write IAM app assignments through the
// $bucapps sub-resource.
//
// It does not work on S/4HANA Cloud 2602 and is not called by the
// CreateBusinessCatalog tool. Keep it, gated, so a release that opens the
// resource for writing needs a flag flip rather than a rewrite.
//
// The resource controller CL_APS_IAM_WBI_SIA1_BUCAPP redefines only `get`, so
// every write verb returns 405 "Resource controller does not support method".
// Both PUT and POST were confirmed rejected against the tenant. The main object
// transformation carries no app list either, so no ADT route exists: the
// Eclipse business catalog editor writes assignments through the workbench tool
// UI protocol in CL_APS_IAM_WBI_SIA1_WB_TOOL_UI, not through ADT REST.
//
// The catalog must be locked by the caller. Returns the verb ADT accepted.
func (c *Client) AssignBusinessCatalogApps(ctx context.Context, catalogName string, lockHandle string, apps []BusinessCatalogApp, transport string) (string, error) {
	if catalogName == "" {
		return "", fmt.Errorf("catalog name is required")
	}
	if lockHandle == "" {
		return "", fmt.Errorf("lock handle is required: lock the catalog before writing assignments")
	}
	if len(apps) == 0 {
		return "", fmt.Errorf("at least one app assignment is required")
	}

	catalogID := strings.ToUpper(catalogName)

	// ObjectURL rather than Package: the catalog already exists at this point,
	// so the gate resolves its package from object metadata. Omitting both
	// fails closed with "requires either ObjectURL or Package" on any
	// connection that configures AllowedPackages.
	//
	// That resolution issues a repository search between the caller's
	// LockObject and this write. The search must run inside the stateful
	// session, or ADT ends the session, drops the lock, and this POST comes
	// back 423 "is not locked".
	if err := c.checkMutation(ctx, MutationContext{
		Op:        OpUpdate,
		OpName:    "AssignBusinessCatalogApps",
		ObjectURL: BusinessCatalogURL(catalogID),
		Transport: transport,
	}); err != nil {
		return "", err
	}

	payload := bucappsPayload{
		NS: "http://www.sap.com/iam/sia1",
	}
	for _, a := range apps {
		if a.AppID == "" {
			return "", fmt.Errorf("app assignment with an empty AppID")
		}
		assignmentID := a.AssignmentID
		if assignmentID == "" {
			assignmentID = catalogID + "_" + strings.ToUpper(a.AppID)
		}
		payload.Apps = append(payload.Apps, bucapp{
			AssignmentID: assignmentID,
			CatalogID:    catalogID,
			AppID:        strings.ToUpper(a.AppID),
		})
	}

	body, err := xml.MarshalIndent(payload, "", "  ")
	if err != nil {
		return "", fmt.Errorf("marshalling bucapps payload: %w", err)
	}
	body = append([]byte(xml.Header), body...)

	params := url.Values{}
	params.Set("name", catalogID)
	params.Set("lockHandle", lockHandle)
	if transport != "" {
		params.Set("corrNr", transport)
	}

	// PUT first: the sub-resource replaces the whole assignment collection,
	// and N4D answered POST with 405.
	_, verb, err := c.requestWithVerbFallback(ctx, BusinessCatalogPath+"/$bucapps",
		http.MethodPut, http.MethodPost, &RequestOptions{
			Query:       params,
			Body:        body,
			ContentType: BucappsContentType,
		})
	if err != nil {
		return verb, fmt.Errorf("writing app assignments to catalog %s: %w", catalogID, err)
	}
	return verb, nil
}

// PublishBusinessCatalog runs the local publish that makes an activated
// catalog visible to role maintenance, the $publish sub-resource registered by
// CL_APS_IAM_WBI_SIA1_WB_ACCESS.
//
// Publishing is asynchronous in the same way service binding publication is,
// so a timeout here does not mean the job failed.
//
// PROVEN on a live S/4HANA Cloud tenant, 2026-08-13: this call timed out awaiting headers, and the
// catalog subsequently appeared in Maintain Business Roles -> Add Business
// Catalogs, where the same search had returned nothing beforehand. The job
// completed server-side. Treat the timeout as "submitted", never as an error.
func (c *Client) PublishBusinessCatalog(ctx context.Context, catalogName string) error {
	if catalogName == "" {
		return fmt.Errorf("catalog name is required")
	}

	if err := c.checkMutation(ctx, MutationContext{
		Op:        OpUpdate,
		OpName:    "PublishBusinessCatalog",
		ObjectURL: BusinessCatalogURL(catalogName),
	}); err != nil {
		return err
	}

	params := url.Values{}
	// businessCatalogID, not name. See the resource note at the top of this file.
	params.Set("businessCatalogID", strings.ToUpper(catalogName))

	// POST first: publish is an action, not a content replacement. $bucapps
	// showed these sub-resources disagree about verbs, so fall back rather
	// than fail the whole run on a 405.
	_, _, err := c.requestWithVerbFallback(ctx, BusinessCatalogPath+"/$publish",
		http.MethodPost, http.MethodPut, &RequestOptions{
			Query: params,
		})
	if err != nil {
		return fmt.Errorf("publishing catalog %s: %w", strings.ToUpper(catalogName), err)
	}
	return nil
}
