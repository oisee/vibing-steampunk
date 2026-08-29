package adt

// IAM app (ADT object type SIA6) support.
//
// Publishing a RAP service binding on S/4HANA Cloud does NOT create an IAM app.
// It creates an IAM *business service* — an SIA6 whose appType is IBS and whose
// secondaryID names the generated communication scenario — plus the scenario
// itself (SCO2). The IAM app that a business catalog can hold is a separate
// SIA6 that a developer creates, normally with appType EXT for an application
// served outside the system, such as a Fiori app on BTP Cloud Foundry.
//
// That distinction is the reason the Eclipse "Business Catalog IAM App
// Assignment" wizard answers
//
//	IAM App 'ZUI_..._O4_0001_G4BA_IBS' does not exist
//
// for an object that plainly exists and that the same tree lists under "IAM
// Apps": the wizard wants an assignable app, and a generated business service
// is not one.
//
// The payload below was read from a live tenant (a live S/4HANA Cloud tenant, 2026-08-13) rather
// than guessed:
//
//	GET /sap/bc/adt/aps/cloud/iam/sia6/<name>
//	-> root sia6:sia6, namespace http://www.sap.com/iam/sia6, adtcore header,
//	   adtcore:packageRef, then sia6:content with appID, appType, ui5AppId,
//	   transactionCode, scopeDependent, providerClass, secondaryID, services,
//	   commonAuthorization:auths/restrictions, appconfigs and pluginConfigs.
//
// Only the identifying subset is written on create. The workbench fills
// pluginConfigs itself, and both a generated IBS app and a hand-made EXT app
// were observed with an empty <sia6:services/>, so services is not required to
// create the object.

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strings"
)

// ObjectTypeIAMApp is the IAM app, ADT type SIA6.
const ObjectTypeIAMApp CreatableObjectType = "SIA6/AP"

// IAMAppPath is the ADT collection holding IAM apps.
const IAMAppPath = "/sap/bc/adt/aps/cloud/iam/sia6"

// IAMAppTypeExternal is the app type for an application served outside this
// system, which is what a Fiori app deployed to BTP is.
const IAMAppTypeExternal = "EXT"

// IAMPublishContentType is the media type the publish resource declares in the
// ADT discovery document.
const IAMPublishContentType = "application/vnd.sap.adt.iam.publishing+xml"

// Publishing status values returned in sia6:publishingStatusText.
const (
	IAMPublishStatusPublished   = "p"
	IAMPublishStatusUnpublished = "u"
)

func init() {
	objectTypes[ObjectTypeIAMApp] = objectTypeInfo{
		creationPath: IAMAppPath,
		rootName:     "sia6:sia6",
		namespace:    `xmlns:sia6="http://www.sap.com/iam/sia6"`,
		bodyBuilder:  buildIAMAppBody,
	}
}

// IAMAppURL returns the ADT URL of an IAM app. ADT lowercases the object name
// in the path.
func IAMAppURL(name string) string {
	return IAMAppPath + "/" + strings.ToLower(name)
}

// IAMAppExists reports whether an IAM app is already present.
//
// Accept is */* for the same reason as BusinessCatalogExists: an ADT resource
// serves its own vnd.sap.adt media type and answers 406 to anything else, so a
// narrower Accept turns an existing object into a false negative.
func (c *Client) IAMAppExists(ctx context.Context, name string) bool {
	resp, err := c.RawGet(ctx, IAMAppURL(name), "*/*")
	if err != nil {
		return false
	}
	return resp.StatusCode >= 200 && resp.StatusCode < 300
}

func buildIAMAppBody(opts CreateObjectOptions, typeInfo objectTypeInfo, responsible string) string {
	appType := strings.ToUpper(opts.AppType)
	if appType == "" {
		appType = IAMAppTypeExternal
	}
	name := strings.ToUpper(opts.Name)

	secondary := ""
	if opts.SecondaryID != "" {
		secondary = fmt.Sprintf("\n    <sia6:secondaryID>%s</sia6:secondaryID>",
			escapeXML(strings.ToUpper(opts.SecondaryID)))
	}

	return fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?>
<%s %s xmlns:adtcore="http://www.sap.com/adt/core"
  adtcore:description="%s"
  adtcore:name="%s"
  adtcore:type="SIA6"
  adtcore:responsible="%s"
  adtcore:masterLanguage="EN"
  adtcore:language="EN">
  <adtcore:packageRef adtcore:name="%s"/>
  <sia6:content>
    <sia6:appID>%s</sia6:appID>
    <sia6:appType>%s</sia6:appType>
    <sia6:scopeDependent>false</sia6:scopeDependent>%s
    <sia6:services/>
  </sia6:content>
</%s>`,
		typeInfo.rootName, typeInfo.namespace,
		escapeXML(opts.Description),
		name,
		responsible,
		strings.ToUpper(opts.PackageName),
		name,
		appType,
		secondary,
		typeInfo.rootName)
}

// PublishIAMApp runs the "Publish Locally" action for an IAM app and returns
// the publishing status ADT reports.
//
// The request form was read out of the ADT discovery document rather than
// guessed, after an earlier version modelled on the SIA1 catalog resource
// returned HTTP 500 (SY/530). Three things differ from SIA1:
//
//	path        .../iam/sia6/publish     not $publish
//	parameter   ?application=<NAME>      not ?name=<NAME>
//	content     application/vnd.sap.adt.iam.publishing+xml, body empty
//
// Discovery registers it as:
//
//	<app:collection href="/sap/bc/adt/aps/cloud/iam/sia6/publish">
//	  <atom:title>Publish Locally</atom:title>
//	  <app:accept>application/vnd.sap.adt.iam.publishing+xml</app:accept>
//
// with no template links, which is why the object goes in the query string and
// not in the body. Getting the parameter name wrong is worth recognising: ADT
// answers "Parameter application could not be found", which names the parameter
// it wanted.
//
// Returns the status text: "p" published, "u" unpublished. A 200 does NOT mean
// the app is published — an app that has nothing to publish comes back "u", so
// callers should report the status rather than assume success. Observed on the tenant
// 080, 2026-08-13: an app published through Eclipse returned "p", while a
// freshly created one with no services returned "u" on repeated calls.
//
// The call can be slow; a second consecutive publish was seen to hang past
// three minutes.
func (c *Client) PublishIAMApp(ctx context.Context, appName string) (string, error) {
	if appName == "" {
		return "", fmt.Errorf("IAM app name is required")
	}

	if err := c.checkMutation(ctx, MutationContext{
		Op:        OpUpdate,
		OpName:    "PublishIAMApp",
		ObjectURL: IAMAppURL(appName),
	}); err != nil {
		return "", err
	}

	params := url.Values{}
	params.Set("application", strings.ToUpper(appName))

	resp, err := c.transport.Request(ctx, IAMAppPath+"/publish", &RequestOptions{
		Method:      http.MethodPost,
		Query:       params,
		ContentType: IAMPublishContentType,
		Accept:      "*/*",
	})
	if err != nil {
		return "", fmt.Errorf("publishing IAM app %s: %w", strings.ToUpper(appName), err)
	}
	return firstXMLValue(string(resp.Body), "sia6:publishingStatusText"), nil
}
