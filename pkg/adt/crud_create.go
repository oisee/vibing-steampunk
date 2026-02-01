// ABOUTME: Object creation operations for ABAP objects.
// ABOUTME: Provides CreateObject and XML body building for various object types.

package adt

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strings"
)

// CreateObjectOptions contains options for creating a new ABAP object.
type CreateObjectOptions struct {
	ObjectType  CreatableObjectType `json:"objectType"`
	Name        string              `json:"name"`
	Description string              `json:"description"`
	PackageName string              `json:"packageName"`
	Transport   string              `json:"transport,omitempty"`
	Responsible string              `json:"responsible,omitempty"`
	// For function modules - the function group name
	ParentName string `json:"parentName,omitempty"`

	// RAP-specific options
	RootEntity         string `json:"rootEntity,omitempty"`         // For BDEF: root CDS entity name
	ImplementationType string `json:"implementationType,omitempty"` // For BDEF: managed, unmanaged, etc.
	ServiceDefinition  string `json:"serviceDefinition,omitempty"`  // For SRVB: service definition name
	BindingType        string `json:"bindingType,omitempty"`        // For SRVB: "ODATA"
	BindingVersion     string `json:"bindingVersion,omitempty"`     // For SRVB: "V2" or "V4"
	BindingCategory    string `json:"bindingCategory,omitempty"`    // For SRVB: "0" for Web API, "1" for UI

	// For BDEF: source code (ADT API embeds source in creation request)
	Source string `json:"source,omitempty"`
}

// CreateObject creates a new ABAP object.
func (c *Client) CreateObject(ctx context.Context, opts CreateObjectOptions) error {
	if err := c.checkSafety(OpCreate, "CreateObject"); err != nil {
		return err
	}

	typeInfo, ok := getObjectTypeInfo(opts.ObjectType)
	if !ok {
		return fmt.Errorf("unsupported object type: %s", opts.ObjectType)
	}

	opts.Name = strings.ToUpper(opts.Name)
	opts.PackageName = strings.ToUpper(opts.PackageName)

	if err := c.checkPackageSafety(opts.PackageName); err != nil {
		return err
	}

	// Only local packages are supported for creation
	if opts.ObjectType == ObjectTypePackage && !strings.HasPrefix(opts.Name, "$") {
		return fmt.Errorf("only local packages (starting with $) are supported for creation, got: %s", opts.Name)
	}

	// Validate package exists before calling SAP ADT to prevent orphan locks
	if opts.ObjectType != ObjectTypePackage && opts.PackageName != "" {
		exists, err := c.packageExists(ctx, opts.PackageName)
		if err != nil {
			return fmt.Errorf("validating package: %w", err)
		}
		if !exists {
			return fmt.Errorf("package %s does not exist - create it first to avoid orphan locks", opts.PackageName)
		}
	}

	creationURL := getCreationURL(opts.ObjectType, opts.ParentName)
	body := buildCreateObjectBody(opts, typeInfo, c.getDefaultResponsible())

	params := url.Values{}
	if opts.Transport != "" {
		params.Set("corrNr", opts.Transport)
	}

	contentType := "application/*"
	if opts.ObjectType == ObjectTypeBDEF {
		contentType = "application/vnd.sap.adt.blues.v1+xml"
	}

	_, err := c.transport.Request(ctx, creationURL, &RequestOptions{
		Method:      http.MethodPost,
		Query:       params,
		Body:        []byte(body),
		ContentType: contentType,
	})

	// If lock conflict, try cleanup and retry once
	if isLockConflictError(err) {
		objectURL := GetObjectURL(opts.ObjectType, opts.Name, opts.ParentName)
		if objectURL != "" {
			c.tryCleanupOrphanLock(ctx, objectURL)
			_, err = c.transport.Request(ctx, creationURL, &RequestOptions{
				Method:      http.MethodPost,
				Query:       params,
				Body:        []byte(body),
				ContentType: contentType,
			})
		}
	}

	if err != nil {
		return fmt.Errorf("creating object: %w", err)
	}

	return nil
}

func (c *Client) getDefaultResponsible() string {
	if c.config.Username != "" {
		return c.config.Username
	}
	return "DDIC"
}

// tryCleanupOrphanLock attempts to clear an orphan lock left behind by a failed creation.
func (c *Client) tryCleanupOrphanLock(ctx context.Context, objectURL string) {
	lock, err := c.LockObject(ctx, objectURL, "MODIFY")
	if err != nil {
		return
	}
	_ = c.UnlockObject(ctx, objectURL, lock.LockHandle)
}

// isLockConflictError checks if an error is a lock conflict (HTTP 403 "is currently editing")
func isLockConflictError(err error) bool {
	if err == nil {
		return false
	}
	errStr := err.Error()
	return strings.Contains(errStr, "403") && strings.Contains(errStr, "currently editing")
}

// packageExists checks if a package exists in the system.
func (c *Client) packageExists(ctx context.Context, packageName string) (bool, error) {
	packageName = strings.ToUpper(packageName)
	url := fmt.Sprintf("/sap/bc/adt/packages/%s", strings.ToLower(packageName))

	_, err := c.transport.Request(ctx, url, &RequestOptions{
		Method: http.MethodGet,
		Accept: "application/vnd.sap.adt.packages.v1+xml",
	})

	if err == nil {
		return true, nil
	}
	if IsNotFoundError(err) {
		return false, nil
	}
	return false, fmt.Errorf("checking package %s: %w", packageName, err)
}

// buildCreateObjectBody generates the XML body for object creation.
func buildCreateObjectBody(opts CreateObjectOptions, typeInfo objectTypeInfo, responsible string) string {
	if opts.Responsible != "" {
		responsible = opts.Responsible
	}

	switch opts.ObjectType {
	case ObjectTypePackage:
		return buildPackageBody(opts, typeInfo, responsible)
	case ObjectTypeFunctionMod:
		return buildFunctionModuleBody(opts, typeInfo, responsible)
	case ObjectTypeSRVD:
		return buildServiceDefinitionBody(opts, typeInfo, responsible)
	case ObjectTypeSRVB:
		return buildServiceBindingBody(opts, typeInfo, responsible)
	case ObjectTypeBDEF:
		return buildBehaviorDefinitionBody(opts, typeInfo, responsible)
	default:
		return buildStandardBody(opts, typeInfo, responsible)
	}
}

func buildStandardBody(opts CreateObjectOptions, typeInfo objectTypeInfo, responsible string) string {
	return fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?>
<%s %s xmlns:adtcore="http://www.sap.com/adt/core"
  adtcore:description="%s"
  adtcore:name="%s"
  adtcore:type="%s"
  adtcore:responsible="%s">
  <adtcore:packageRef adtcore:name="%s"/>
</%s>`,
		typeInfo.rootName, typeInfo.namespace,
		escapeXML(opts.Description),
		opts.Name,
		opts.ObjectType,
		responsible,
		opts.PackageName,
		typeInfo.rootName)
}

func buildPackageBody(opts CreateObjectOptions, typeInfo objectTypeInfo, responsible string) string {
	return fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?>
<%s %s xmlns:adtcore="http://www.sap.com/adt/core"
  adtcore:description="%s"
  adtcore:name="%s"
  adtcore:type="%s"
  adtcore:responsible="%s">
  <pack:attributes pack:packageType="development"/>
  <pack:superPackage adtcore:name="%s" adtcore:type="DEVC/K"/>
  <pack:applicationComponent/>
  <pack:transport>
    <pack:softwareComponent pack:name="LOCAL"/>
    <pack:transportLayer pack:name=""/>
  </pack:transport>
  <pack:translation/>
  <pack:useAccesses/>
  <pack:packageInterfaces/>
  <pack:subPackages/>
</%s>`,
		typeInfo.rootName, typeInfo.namespace,
		escapeXML(opts.Description),
		opts.Name,
		opts.ObjectType,
		responsible,
		opts.PackageName,
		typeInfo.rootName)
}

func buildFunctionModuleBody(opts CreateObjectOptions, typeInfo objectTypeInfo, responsible string) string {
	return fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?>
<%s %s xmlns:adtcore="http://www.sap.com/adt/core"
  adtcore:description="%s"
  adtcore:name="%s"
  adtcore:type="%s"
  adtcore:responsible="%s">
  <adtcore:containerRef adtcore:name="%s" adtcore:type="FUGR/F"
    adtcore:uri="/sap/bc/adt/functions/groups/%s"/>
</%s>`,
		typeInfo.rootName, typeInfo.namespace,
		escapeXML(opts.Description),
		opts.Name,
		opts.ObjectType,
		responsible,
		strings.ToUpper(opts.ParentName),
		strings.ToLower(opts.ParentName),
		typeInfo.rootName)
}

func buildServiceDefinitionBody(opts CreateObjectOptions, typeInfo objectTypeInfo, responsible string) string {
	return fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?>
<%s %s xmlns:adtcore="http://www.sap.com/adt/core"
  adtcore:description="%s"
  adtcore:name="%s"
  adtcore:type="%s"
  adtcore:responsible="%s"
  srvd:srvdSourceType="S">
  <adtcore:packageRef adtcore:name="%s"/>
</%s>`,
		typeInfo.rootName, typeInfo.namespace,
		escapeXML(opts.Description),
		opts.Name,
		opts.ObjectType,
		responsible,
		opts.PackageName,
		typeInfo.rootName)
}

func buildServiceBindingBody(opts CreateObjectOptions, typeInfo objectTypeInfo, responsible string) string {
	bindingType := opts.BindingType
	if bindingType == "" {
		bindingType = "ODATA"
	}
	bindingVersion := opts.BindingVersion
	if bindingVersion == "" {
		bindingVersion = "V2"
	}
	bindingCategory := opts.BindingCategory
	if bindingCategory == "" {
		bindingCategory = "0"
	}

	return fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?>
<%s %s xmlns:adtcore="http://www.sap.com/adt/core"
  adtcore:description="%s"
  adtcore:name="%s"
  adtcore:type="%s"
  adtcore:responsible="%s">
  <adtcore:packageRef adtcore:name="%s"/>
  <srvb:services srvb:name="%s">
    <srvb:content srvb:version="0001">
      <srvb:serviceDefinition adtcore:name="%s"/>
    </srvb:content>
  </srvb:services>
  <srvb:binding srvb:category="%s" srvb:type="%s" srvb:version="%s">
    <srvb:implementation adtcore:name=""/>
  </srvb:binding>
</%s>`,
		typeInfo.rootName, typeInfo.namespace,
		escapeXML(opts.Description),
		opts.Name,
		opts.ObjectType,
		responsible,
		opts.PackageName,
		opts.Name,
		strings.ToUpper(opts.ServiceDefinition),
		bindingCategory,
		bindingType,
		bindingVersion,
		typeInfo.rootName)
}

func buildBehaviorDefinitionBody(opts CreateObjectOptions, _ objectTypeInfo, responsible string) string {
	return fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?>
<blue:blueSource xmlns:blue="http://www.sap.com/wbobj/blue" xmlns:adtcore="http://www.sap.com/adt/core"
  adtcore:description="%s"
  adtcore:name="%s"
  adtcore:type="%s"
  adtcore:responsible="%s">
  <adtcore:packageRef adtcore:name="%s"/>
</blue:blueSource>`,
		escapeXML(opts.Description),
		opts.Name,
		opts.ObjectType,
		responsible,
		opts.PackageName)
}

func escapeXML(s string) string {
	s = strings.ReplaceAll(s, "&", "&amp;")
	s = strings.ReplaceAll(s, "<", "&lt;")
	s = strings.ReplaceAll(s, ">", "&gt;")
	s = strings.ReplaceAll(s, "\"", "&quot;")
	s = strings.ReplaceAll(s, "'", "&apos;")
	return s
}
