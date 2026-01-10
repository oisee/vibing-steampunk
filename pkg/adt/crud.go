package adt

import (
	"context"
	"encoding/xml"
	"fmt"
	"net/http"
	"net/url"
	"strings"
)

// --- Lock/Unlock Operations ---

// LockResult represents the result of locking an object.
type LockResult struct {
	LockHandle          string `json:"lockHandle"`
	CorrNr              string `json:"corrNr,omitempty"`
	CorrUser            string `json:"corrUser,omitempty"`
	CorrText            string `json:"corrText,omitempty"`
	IsLocal             bool   `json:"isLocal"`
	IsLinkUp            bool   `json:"isLinkUp"`
	ModificationSupport string `json:"modificationSupport,omitempty"`
}

// LockObject acquires an edit lock on an ABAP object.
// objectURL is the ADT URL of the object (e.g., "/sap/bc/adt/programs/programs/ZTEST")
// accessMode is typically "MODIFY" for editing
func (c *Client) LockObject(ctx context.Context, objectURL string, accessMode string) (*LockResult, error) {
	// Safety check - only check for MODIFY locks, READ locks are safe
	if accessMode == "" || accessMode == "MODIFY" {
		if err := c.checkSafety(OpLock, "LockObject"); err != nil {
			return nil, err
		}
	}

	if accessMode == "" {
		accessMode = "MODIFY"
	}

	params := url.Values{}
	params.Set("_action", "LOCK")
	params.Set("accessMode", accessMode)

	resp, err := c.transport.Request(ctx, objectURL, &RequestOptions{
		Method: http.MethodPost,
		Query:  params,
		Accept: "application/vnd.sap.as+xml;charset=UTF-8;dataname=com.sap.adt.lock.result",
	})
	if err != nil {
		return nil, fmt.Errorf("locking object: %w", err)
	}

	return parseLockResult(resp.Body)
}

func parseLockResult(data []byte) (*LockResult, error) {
	// Parse the ABAP serialization XML format
	type lockData struct {
		LockHandle string `xml:"LOCK_HANDLE"`
		CorrNr     string `xml:"CORRNR"`
		CorrUser   string `xml:"CORRUSER"`
		CorrText   string `xml:"CORRTEXT"`
		IsLocal    string `xml:"IS_LOCAL"`
		IsLinkUp   string `xml:"IS_LINK_UP"`
		ModSupport string `xml:"MODIFICATION_SUPPORT"`
	}
	type values struct {
		Data lockData `xml:"DATA"`
	}
	type abapResponse struct {
		Values values `xml:"values"`
	}

	var resp abapResponse
	if err := xml.Unmarshal(data, &resp); err != nil {
		return nil, fmt.Errorf("parsing lock response: %w", err)
	}

	return &LockResult{
		LockHandle:          resp.Values.Data.LockHandle,
		CorrNr:              resp.Values.Data.CorrNr,
		CorrUser:            resp.Values.Data.CorrUser,
		CorrText:            resp.Values.Data.CorrText,
		IsLocal:             resp.Values.Data.IsLocal == "X",
		IsLinkUp:            resp.Values.Data.IsLinkUp == "X",
		ModificationSupport: resp.Values.Data.ModSupport,
	}, nil
}

// UnlockObject releases an edit lock on an ABAP object.
func (c *Client) UnlockObject(ctx context.Context, objectURL string, lockHandle string) error {
	params := url.Values{}
	params.Set("_action", "UNLOCK")
	params.Set("lockHandle", lockHandle)

	_, err := c.transport.Request(ctx, objectURL, &RequestOptions{
		Method: http.MethodPost,
		Query:  params,
	})
	if err != nil {
		return fmt.Errorf("unlocking object: %w", err)
	}

	return nil
}

// --- Update Source Operations ---

// UpdateSource writes source code to an ABAP object.
// objectSourceURL is the source URL (e.g., "/sap/bc/adt/programs/programs/ZTEST/source/main")
// lockHandle is required (from LockObject)
// transport is optional (for transportable objects)
func (c *Client) UpdateSource(ctx context.Context, objectSourceURL string, source string, lockHandle string, transport string) error {
	// Safety check
	if err := c.checkSafety(OpUpdate, "UpdateSource"); err != nil {
		return err
	}

	params := url.Values{}
	params.Set("lockHandle", lockHandle)
	if transport != "" {
		params.Set("corrNr", transport)
	}

	// Determine content type based on source content
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

// --- Create Object Operations ---

// CreatableObjectType defines types of ABAP objects that can be created.
type CreatableObjectType string

const (
	ObjectTypeProgram       CreatableObjectType = "PROG/P"
	ObjectTypeInclude       CreatableObjectType = "PROG/I"
	ObjectTypeClass         CreatableObjectType = "CLAS/OC"
	ObjectTypeInterface     CreatableObjectType = "INTF/OI"
	ObjectTypeFunctionGroup CreatableObjectType = "FUGR/F"
	ObjectTypeFunctionMod   CreatableObjectType = "FUGR/FF"
	ObjectTypeTable         CreatableObjectType = "TABL/DT"
	ObjectTypePackage       CreatableObjectType = "DEVC/K"
	// RAP object types (read-only via ADT, created via RAP generators)
	ObjectTypeDDLS CreatableObjectType = "DDLS/DF"  // CDS DDL Source
	ObjectTypeBDEF CreatableObjectType = "BDEF/BDO" // Behavior Definition
	ObjectTypeSRVD CreatableObjectType = "SRVD/SRV" // Service Definition
	ObjectTypeSRVB CreatableObjectType = "SRVB/SVB" // Service Binding
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
	// For BDEF: the root CDS entity name (e.g., "ZTRAVEL" for define behavior for ZTRAVEL)
	RootEntity string `json:"rootEntity,omitempty"`
	// For BDEF: implementation type (managed, unmanaged, projection, interface, abstract)
	ImplementationType string `json:"implementationType,omitempty"`
	// For SRVB: the service definition name to bind
	ServiceDefinition string `json:"serviceDefinition,omitempty"`
	// For SRVB: binding type ("ODATA" for both V2 and V4)
	BindingType string `json:"bindingType,omitempty"`
	// For SRVB: binding version ("V2" or "V4")
	BindingVersion string `json:"bindingVersion,omitempty"`
	// For SRVB: category ("0" for Web API, "1" for UI)
	BindingCategory string `json:"bindingCategory,omitempty"`

	// For BDEF: source code (required for creation - ADT API embeds source in creation request)
	Source string `json:"source,omitempty"`
}

// objectTypeInfo contains metadata for creating object types.
type objectTypeInfo struct {
	creationPath string
	rootName     string
	namespace    string
}

var objectTypes = map[CreatableObjectType]objectTypeInfo{
	ObjectTypeProgram: {
		creationPath: "/sap/bc/adt/programs/programs",
		rootName:     "program:abapProgram",
		namespace:    `xmlns:program="http://www.sap.com/adt/programs/programs"`,
	},
	ObjectTypeInclude: {
		creationPath: "/sap/bc/adt/programs/includes",
		rootName:     "include:abapInclude",
		namespace:    `xmlns:include="http://www.sap.com/adt/programs/includes"`,
	},
	ObjectTypeClass: {
		creationPath: "/sap/bc/adt/oo/classes",
		rootName:     "class:abapClass",
		namespace:    `xmlns:class="http://www.sap.com/adt/oo/classes"`,
	},
	ObjectTypeInterface: {
		creationPath: "/sap/bc/adt/oo/interfaces",
		rootName:     "intf:abapInterface",
		namespace:    `xmlns:intf="http://www.sap.com/adt/oo/interfaces"`,
	},
	ObjectTypeFunctionGroup: {
		creationPath: "/sap/bc/adt/functions/groups",
		rootName:     "group:abapFunctionGroup",
		namespace:    `xmlns:group="http://www.sap.com/adt/functions/groups"`,
	},
	ObjectTypeFunctionMod: {
		creationPath: "/sap/bc/adt/functions/groups/%s/fmodules",
		rootName:     "fmodule:abapFunctionModule",
		namespace:    `xmlns:fmodule="http://www.sap.com/adt/functions/fmodules"`,
	},
	ObjectTypePackage: {
		creationPath: "/sap/bc/adt/packages",
		rootName:     "pack:package",
		namespace:    `xmlns:pack="http://www.sap.com/adt/packages"`,
	},
	// RAP object types
	ObjectTypeDDLS: {
		creationPath: "/sap/bc/adt/ddic/ddl/sources",
		rootName:     "ddl:ddlSource",
		namespace:    `xmlns:ddl="http://www.sap.com/adt/ddic/ddlsources"`,
	},
	ObjectTypeBDEF: {
		creationPath: "/sap/bc/adt/bo/behaviordefinitions",
		rootName:     "bdef:behaviorDefinition",
		namespace:    `xmlns:bdef="http://www.sap.com/adt/bo/behaviordefinitions"`,
	},
	ObjectTypeSRVD: {
		creationPath: "/sap/bc/adt/ddic/srvd/sources",
		rootName:     "srvd:srvdSource",
		namespace:    `xmlns:srvd="http://www.sap.com/adt/ddic/srvdsources"`,
	},
	ObjectTypeSRVB: {
		creationPath: "/sap/bc/adt/businessservices/bindings",
		rootName:     "srvb:serviceBinding",
		namespace:    `xmlns:srvb="http://www.sap.com/adt/ddic/ServiceBindings"`,
	},
}

// tryCleanupOrphanLock attempts to clear an orphan lock left behind by a failed creation.
// SAP ADT sometimes creates ENQUEUE locks during object creation that aren't released on failure.
// This function tries to acquire and immediately release such locks.
func (c *Client) tryCleanupOrphanLock(ctx context.Context, objectURL string) {
	// Try to acquire the lock - this may succeed if it's our own orphan lock
	lock, err := c.LockObject(ctx, objectURL, "MODIFY")
	if err != nil {
		// Lock acquisition failed - lock might be held by another user or doesn't exist
		return
	}
	// Successfully acquired - release it immediately
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
// Returns (true, nil) if package exists, (false, nil) if it doesn't exist (404).
// Returns (false, error) for server errors (5xx) to distinguish from "not found".
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

	// 404 means package doesn't exist - this is a definitive "no"
	if IsNotFoundError(err) {
		return false, nil
	}

	// For other errors (5xx, network, etc.), we can't determine if package exists
	return false, fmt.Errorf("checking package %s: %w", packageName, err)
}

// CreateObject creates a new ABAP object.
// IMPORTANT: This function validates package existence BEFORE calling SAP ADT CreateObject API.
// This prevents orphan ENQUEUE locks that SAP creates internally during CreateObject
// before validating the request. These orphan locks can only be cleared via SM12.
func (c *Client) CreateObject(ctx context.Context, opts CreateObjectOptions) error {
	// Safety check
	if err := c.checkSafety(OpCreate, "CreateObject"); err != nil {
		return err
	}

	typeInfo, ok := objectTypes[opts.ObjectType]
	if !ok {
		return fmt.Errorf("unsupported object type: %s", opts.ObjectType)
	}

	opts.Name = strings.ToUpper(opts.Name)
	opts.PackageName = strings.ToUpper(opts.PackageName)

	// Check package restrictions
	if err := c.checkPackageSafety(opts.PackageName); err != nil {
		return err
	}

	// Additional validation for package creation: only local packages are supported
	if opts.ObjectType == ObjectTypePackage && !strings.HasPrefix(opts.Name, "$") {
		return fmt.Errorf("only local packages (starting with $) are supported for creation, got: %s", opts.Name)
	}

	// CRITICAL: Validate package exists BEFORE calling SAP ADT CreateObject API.
	// SAP ADT creates ENQUEUE locks internally BEFORE validating the request.
	// If package doesn't exist, SAP fails but leaves the lock orphaned.
	// These orphan locks can only be cleared via SM12 transaction.
	// By checking first, we prevent this scenario entirely.
	if opts.ObjectType != ObjectTypePackage && opts.PackageName != "" {
		exists, err := c.packageExists(ctx, opts.PackageName)
		if err != nil {
			return fmt.Errorf("validating package: %w", err)
		}
		if !exists {
			return fmt.Errorf("package %s does not exist - create it first to avoid orphan locks", opts.PackageName)
		}
	}

	// Build creation URL
	creationURL := typeInfo.creationPath
	if opts.ObjectType == ObjectTypeFunctionMod && opts.ParentName != "" {
		creationURL = fmt.Sprintf(typeInfo.creationPath, strings.ToUpper(opts.ParentName))
	}

	// Build request body with current user as default responsible
	defaultResponsible := c.config.Username
	if defaultResponsible == "" {
		defaultResponsible = "DDIC" // Fallback to standard development user
	}
	body := buildCreateObjectBody(opts, typeInfo, defaultResponsible)

	params := url.Values{}
	if opts.Transport != "" {
		params.Set("corrNr", opts.Transport)
	}

	// BDEF requires specific content type
	contentType := "application/*"
	if opts.ObjectType == ObjectTypeBDEF {
		contentType = "application/vnd.sap.adt.blues.v1+xml"
	}

	// First attempt
	_, err := c.transport.Request(ctx, creationURL, &RequestOptions{
		Method:      http.MethodPost,
		Query:       params,
		Body:        []byte(body),
		ContentType: contentType,
	})

	// If we hit a lock conflict, try to clean up orphan lock and retry once
	if isLockConflictError(err) {
		// Get the object URL for lock cleanup
		objectURL := GetObjectURL(opts.ObjectType, opts.Name, opts.ParentName)
		if objectURL != "" {
			c.tryCleanupOrphanLock(ctx, objectURL)

			// Retry creation
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

func buildCreateObjectBody(opts CreateObjectOptions, typeInfo objectTypeInfo, defaultResponsible string) string {
	responsible := opts.Responsible
	if responsible == "" {
		responsible = defaultResponsible
	}

	// For packages, use special structure with attributes element
	// Note: Only local packages (starting with $) are supported (validated in CreateObject)
	if opts.ObjectType == ObjectTypePackage {
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

	// For function modules, reference the function group
	if opts.ObjectType == ObjectTypeFunctionMod {
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

	// For SRVD (Service Definition), include the srvdSourceType attribute
	if opts.ObjectType == ObjectTypeSRVD {
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

	// For SRVB (Service Binding), include service definition and binding info
	if opts.ObjectType == ObjectTypeSRVB {
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
			bindingCategory = "0" // Web API
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

	// For BDEF (Behavior Definition), use blue:blueSource as root element
	// ADT API expects this specific format (discovered from existing BDEFs)
	if opts.ObjectType == ObjectTypeBDEF {
		// BDEF creation uses blue:blueSource as root, source is set separately via PUT
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

	// Standard object creation (DDLS uses standard body)
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

func escapeXML(s string) string {
	s = strings.ReplaceAll(s, "&", "&amp;")
	s = strings.ReplaceAll(s, "<", "&lt;")
	s = strings.ReplaceAll(s, ">", "&gt;")
	s = strings.ReplaceAll(s, "\"", "&quot;")
	s = strings.ReplaceAll(s, "'", "&apos;")
	return s
}

// --- Delete Object Operations ---

// DeleteObject deletes an ABAP object.
// objectURL is the ADT URL of the object (e.g., "/sap/bc/adt/programs/programs/ZTEST")
// lockHandle is required (from LockObject)
// transport is optional (for transportable objects)
func (c *Client) DeleteObject(ctx context.Context, objectURL string, lockHandle string, transport string) error {
	// Safety check
	if err := c.checkSafety(OpDelete, "DeleteObject"); err != nil {
		return err
	}

	params := url.Values{}
	params.Set("lockHandle", lockHandle)
	if transport != "" {
		params.Set("corrNr", transport)
	}

	_, err := c.transport.Request(ctx, objectURL, &RequestOptions{
		Method: http.MethodDelete,
		Query:  params,
	})
	if err != nil {
		return fmt.Errorf("deleting object: %w", err)
	}

	return nil
}

// --- Helper to get object URLs ---

// GetObjectURL returns the ADT URL for an object based on its type and name.
// All names are URL-encoded to support namespaced objects like /UI5/CL_REPOSITORY_LOAD.
func GetObjectURL(objectType CreatableObjectType, name string, parentName string) string {
	name = strings.ToUpper(name)
	encodedName := url.PathEscape(name)

	switch objectType {
	case ObjectTypeProgram:
		return fmt.Sprintf("/sap/bc/adt/programs/programs/%s", encodedName)
	case ObjectTypeInclude:
		return fmt.Sprintf("/sap/bc/adt/programs/includes/%s", encodedName)
	case ObjectTypeClass:
		return fmt.Sprintf("/sap/bc/adt/oo/classes/%s", encodedName)
	case ObjectTypeInterface:
		return fmt.Sprintf("/sap/bc/adt/oo/interfaces/%s", encodedName)
	case ObjectTypeFunctionGroup:
		return fmt.Sprintf("/sap/bc/adt/functions/groups/%s", encodedName)
	case ObjectTypeFunctionMod:
		parentName = strings.ToUpper(parentName)
		encodedParent := url.PathEscape(parentName)
		return fmt.Sprintf("/sap/bc/adt/functions/groups/%s/fmodules/%s", encodedParent, encodedName)
	case ObjectTypePackage:
		return fmt.Sprintf("/sap/bc/adt/packages/%s", encodedName)
	// RAP object types - use lowercase for CDS objects
	case ObjectTypeDDLS:
		return fmt.Sprintf("/sap/bc/adt/ddic/ddl/sources/%s", url.PathEscape(strings.ToLower(name)))
	case ObjectTypeBDEF:
		return fmt.Sprintf("/sap/bc/adt/bo/behaviordefinitions/%s", url.PathEscape(strings.ToLower(name)))
	case ObjectTypeSRVD:
		return fmt.Sprintf("/sap/bc/adt/ddic/srvd/sources/%s", url.PathEscape(strings.ToLower(name)))
	case ObjectTypeSRVB:
		return fmt.Sprintf("/sap/bc/adt/businessservices/bindings/%s", url.PathEscape(strings.ToLower(name)))
	default:
		return ""
	}
}

// GetSourceURL returns the source URL for an object.
func GetSourceURL(objectType CreatableObjectType, name string, parentName string) string {
	objectURL := GetObjectURL(objectType, name, parentName)
	if objectURL == "" {
		return ""
	}
	return objectURL + "/source/main"
}

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
// Note: For includes other than main, the URL does NOT have /source/main suffix
// Supports namespaced classes like /UI5/CL_REPOSITORY_LOAD.
func GetClassIncludeSourceURL(className string, includeType ClassIncludeType) string {
	className = strings.ToUpper(className)
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

	// URL encode for namespaced objects
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

// --- Service Binding Publish/Unpublish Operations ---

// PublishResult represents the result of a publish/unpublish operation.
type PublishResult struct {
	Severity  string `json:"severity"`
	ShortText string `json:"shortText"`
	LongText  string `json:"longText"`
}

// PublishServiceBinding publishes a service binding to make it available as OData service.
// serviceName is the service binding name (e.g., "ZTRAVEL_SB")
// serviceVersion is typically "0001"
func (c *Client) PublishServiceBinding(ctx context.Context, serviceName string, serviceVersion string) (*PublishResult, error) {
	return c.publishUnpublishServiceBinding(ctx, "publishjobs", serviceName, serviceVersion)
}

// UnpublishServiceBinding unpublishes a service binding.
func (c *Client) UnpublishServiceBinding(ctx context.Context, serviceName string, serviceVersion string) (*PublishResult, error) {
	return c.publishUnpublishServiceBinding(ctx, "unpublishjobs", serviceName, serviceVersion)
}

func (c *Client) publishUnpublishServiceBinding(ctx context.Context, action, serviceName, serviceVersion string) (*PublishResult, error) {
	if serviceVersion == "" {
		serviceVersion = "0001"
	}

	params := url.Values{}
	params.Set("servicename", serviceName)
	params.Set("serviceversion", serviceVersion)

	body := fmt.Sprintf(`<adtcore:objectReferences xmlns:adtcore="http://www.sap.com/adt/core">
  <adtcore:objectReference adtcore:name="%s"/>
</adtcore:objectReferences>`, serviceName)

	path := fmt.Sprintf("/sap/bc/adt/businessservices/odatav2/%s", action)

	resp, err := c.transport.Request(ctx, path, &RequestOptions{
		Method:      http.MethodPost,
		Query:       params,
		Body:        []byte(body),
		ContentType: "application/*",
		Accept:      "application/*",
	})
	if err != nil {
		return nil, fmt.Errorf("%s service binding: %w", action, err)
	}

	return parsePublishResult(resp.Body)
}

func parsePublishResult(data []byte) (*PublishResult, error) {
	type publishData struct {
		Severity  string `xml:"SEVERITY"`
		ShortText string `xml:"SHORT_TEXT"`
		LongText  string `xml:"LONG_TEXT"`
	}
	type values struct {
		Data publishData `xml:"DATA"`
	}
	type abapResponse struct {
		Values values `xml:"values"`
	}

	var resp abapResponse
	if err := xml.Unmarshal(data, &resp); err != nil {
		return nil, fmt.Errorf("parsing publish response: %w", err)
	}

	return &PublishResult{
		Severity:  resp.Values.Data.Severity,
		ShortText: resp.Values.Data.ShortText,
		LongText:  resp.Values.Data.LongText,
	}, nil
}

// --- DDIC Table/Structure Operations ---

// CreateTableOptions defines options for creating a DDIC table.
type CreateTableOptions struct {
	Name          string       `json:"name"`          // Table name (uppercase, max 30 chars, must start with Z/Y)
	Description   string       `json:"description"`   // Short description
	Package       string       `json:"package"`       // Target package
	Fields        []TableField `json:"fields"`        // Field definitions
	Transport     string       `json:"transport,omitempty"` // Transport request (optional for $TMP)
	DeliveryClass string       `json:"deliveryClass,omitempty"` // A=Application, C=Customizing, L=Temp, etc. (default: A)
	TableCategory string       `json:"tableCategory,omitempty"` // TRANSPARENT (default), STRUCTURE, etc.
}

// CreateTable creates a new DDIC transparent table from JSON-like options.
// This is a high-level tool that handles the full workflow: create → set source → activate.
func (c *Client) CreateTable(ctx context.Context, opts CreateTableOptions) error {
	if err := c.checkSafety(OpCreate, "CreateTable"); err != nil {
		return err
	}

	// Validate input
	opts.Name = strings.ToUpper(opts.Name)
	if opts.Name == "" || len(opts.Name) > 30 {
		return fmt.Errorf("table name must be 1-30 characters")
	}
	if len(opts.Fields) == 0 {
		return fmt.Errorf("at least one field is required")
	}
	if opts.Package == "" {
		opts.Package = "$TMP"
	}
	if opts.DeliveryClass == "" {
		opts.DeliveryClass = "A"
	}
	if opts.TableCategory == "" {
		opts.TableCategory = "TRANSPARENT"
	}

	// Generate DDL source
	ddlSource := generateTableDDL(opts)

	// Step 1: Create table object
	createBody := fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?>
<blue:blueSource xmlns:blue="http://www.sap.com/wbobj/blue"
                 xmlns:adtcore="http://www.sap.com/adt/core"
                 adtcore:name="%s"
                 adtcore:type="TABL/DT"
                 adtcore:description="%s">
  <adtcore:packageRef adtcore:name="%s"/>
</blue:blueSource>`, opts.Name, escapeXML(opts.Description), opts.Package)

	params := url.Values{}
	if opts.Transport != "" {
		params.Set("corrNr", opts.Transport)
	}

	_, err := c.transport.Request(ctx, "/sap/bc/adt/ddic/tables", &RequestOptions{
		Method:      http.MethodPost,
		Query:       params,
		Body:        []byte(createBody),
		ContentType: "application/vnd.sap.adt.tables.v2+xml",
		Accept:      "application/vnd.sap.adt.tables.v2+xml",
	})
	if err != nil {
		return fmt.Errorf("creating table object: %w", err)
	}

	// Step 2: Lock, update source, unlock
	tableURL := fmt.Sprintf("/sap/bc/adt/ddic/tables/%s", strings.ToLower(opts.Name))
	sourceURL := tableURL + "/source/main"

	lock, err := c.LockObject(ctx, tableURL, "MODIFY")
	if err != nil {
		return fmt.Errorf("locking table: %w", err)
	}

	params = url.Values{}
	params.Set("lockHandle", lock.LockHandle)
	if opts.Transport != "" {
		params.Set("corrNr", opts.Transport)
	}

	_, err = c.transport.Request(ctx, sourceURL, &RequestOptions{
		Method:      http.MethodPut,
		Query:       params,
		Body:        []byte(ddlSource),
		ContentType: "text/plain",
	})
	if err != nil {
		c.UnlockObject(ctx, tableURL, lock.LockHandle)
		return fmt.Errorf("updating table source: %w", err)
	}

	// Unlock BEFORE activation
	c.UnlockObject(ctx, tableURL, lock.LockHandle)

	// Step 3: Activate
	if _, err := c.Activate(ctx, tableURL, opts.Name); err != nil {
		return fmt.Errorf("activating table: %w", err)
	}

	return nil
}

// generateTableDDL converts CreateTableOptions to CDS-style DDL source.
func generateTableDDL(opts CreateTableOptions) string {
	var sb strings.Builder

	// Annotations - must match SAP's expected format
	sb.WriteString(fmt.Sprintf("@EndUserText.label : '%s'\n", escapeQuote(opts.Description)))
	sb.WriteString("@AbapCatalog.enhancement.category : #NOT_EXTENSIBLE\n")
	sb.WriteString(fmt.Sprintf("@AbapCatalog.tableCategory : #%s\n", opts.TableCategory))
	sb.WriteString(fmt.Sprintf("@AbapCatalog.deliveryClass : #%s\n", opts.DeliveryClass))
	sb.WriteString("@AbapCatalog.dataMaintenance : #ALLOWED\n")
	sb.WriteString(fmt.Sprintf("define table %s {\n\n", strings.ToLower(opts.Name)))

	// Auto-add MANDT as first key field (standard SAP practice)
	sb.WriteString("  key client : abap.clnt not null;\n")

	// User-defined fields
	for _, f := range opts.Fields {
		fieldName := strings.ToLower(f.Name)
		fieldType := mapFieldType(f)

		if f.IsKey {
			sb.WriteString(fmt.Sprintf("  key %s : %s not null;\n", fieldName, fieldType))
		} else if f.NotNull {
			sb.WriteString(fmt.Sprintf("  %s : %s not null;\n", fieldName, fieldType))
		} else {
			sb.WriteString(fmt.Sprintf("  %s : %s;\n", fieldName, fieldType))
		}
	}

	sb.WriteString("\n}\n")
	return sb.String()
}

// mapFieldType converts a simple type spec to ABAP DDL type.
func mapFieldType(f TableField) string {
	t := strings.ToUpper(f.Type)

	// Handle built-in types with length
	switch t {
	case "CHAR":
		if f.Length > 0 {
			return fmt.Sprintf("abap.char(%d)", f.Length)
		}
		return "abap.char(1)"
	case "NUMC":
		if f.Length > 0 {
			return fmt.Sprintf("abap.numc(%d)", f.Length)
		}
		return "abap.numc(10)"
	case "RAW":
		if f.Length > 0 {
			return fmt.Sprintf("abap.raw(%d)", f.Length)
		}
		return "abap.raw(16)"
	case "DEC", "CURR", "QUAN":
		l := f.Length
		if l == 0 {
			l = 15
		}
		d := f.Decimals
		return fmt.Sprintf("abap.dec(%d,%d)", l, d)
	case "INT1":
		return "abap.int1"
	case "INT2":
		return "abap.int2"
	case "INT4":
		return "abap.int4"
	case "INT8":
		return "abap.int8"
	case "FLTP":
		return "abap.fltp"
	case "STRING":
		return "abap.string(0)"
	case "RAWSTRING":
		return "abap.rawstring(0)"
	case "DATS", "DATE":
		return "abap.dats"
	case "TIMS", "TIME":
		return "abap.tims"
	case "TIMESTAMPL":
		return "timestampl"
	case "UTCLONG":
		return "abap.utclong"
	case "SYSUUID_X16", "UUID":
		return "sysuuid_x16"
	case "MANDT", "CLIENT":
		return "mandt"
	}

	// Check for CHARnn, NUMCnn shorthand (e.g., CHAR32, NUMC10)
	if strings.HasPrefix(t, "CHAR") && len(t) > 4 {
		return fmt.Sprintf("abap.char(%s)", t[4:])
	}
	if strings.HasPrefix(t, "NUMC") && len(t) > 4 {
		return fmt.Sprintf("abap.numc(%s)", t[4:])
	}

	// Assume it's a data element name
	return strings.ToLower(t)
}

func escapeQuote(s string) string {
	return strings.ReplaceAll(s, "'", "''")
}
