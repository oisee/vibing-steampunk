// ABOUTME: URL generation helpers for ABAP object types.
// ABOUTME: Provides GetObjectURL, GetSourceURL, and object type metadata.

package adt

import (
	"fmt"
	"net/url"
	"strings"
)

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
	ObjectTypeDDLS          CreatableObjectType = "DDLS/DF"  // CDS DDL Source
	ObjectTypeBDEF          CreatableObjectType = "BDEF/BDO" // Behavior Definition
	ObjectTypeSRVD          CreatableObjectType = "SRVD/SRV" // Service Definition
	ObjectTypeSRVB          CreatableObjectType = "SRVB/SVB" // Service Binding
)

// objectTypeInfo contains metadata for object types - single source of truth.
type objectTypeInfo struct {
	basePath    string // Base path for the object type (used for both creation and retrieval)
	rootName    string // XML root element name for creation
	namespace   string // XML namespace for creation
	useLowerURL bool   // Whether URL uses lowercase name (CDS objects)
}

// objectTypes maps object types to their metadata.
var objectTypes = map[CreatableObjectType]objectTypeInfo{
	ObjectTypeProgram: {
		basePath:  "/sap/bc/adt/programs/programs",
		rootName:  "program:abapProgram",
		namespace: `xmlns:program="http://www.sap.com/adt/programs/programs"`,
	},
	ObjectTypeInclude: {
		basePath:  "/sap/bc/adt/programs/includes",
		rootName:  "include:abapInclude",
		namespace: `xmlns:include="http://www.sap.com/adt/programs/includes"`,
	},
	ObjectTypeClass: {
		basePath:  "/sap/bc/adt/oo/classes",
		rootName:  "class:abapClass",
		namespace: `xmlns:class="http://www.sap.com/adt/oo/classes"`,
	},
	ObjectTypeInterface: {
		basePath:  "/sap/bc/adt/oo/interfaces",
		rootName:  "intf:abapInterface",
		namespace: `xmlns:intf="http://www.sap.com/adt/oo/interfaces"`,
	},
	ObjectTypeFunctionGroup: {
		basePath:  "/sap/bc/adt/functions/groups",
		rootName:  "group:abapFunctionGroup",
		namespace: `xmlns:group="http://www.sap.com/adt/functions/groups"`,
	},
	ObjectTypeFunctionMod: {
		basePath:  "/sap/bc/adt/functions/groups", // Parent path, requires parentName
		rootName:  "fmodule:abapFunctionModule",
		namespace: `xmlns:fmodule="http://www.sap.com/adt/functions/fmodules"`,
	},
	ObjectTypePackage: {
		basePath:  "/sap/bc/adt/packages",
		rootName:  "pack:package",
		namespace: `xmlns:pack="http://www.sap.com/adt/packages"`,
	},
	ObjectTypeDDLS: {
		basePath:    "/sap/bc/adt/ddic/ddl/sources",
		rootName:    "ddl:ddlSource",
		namespace:   `xmlns:ddl="http://www.sap.com/adt/ddic/ddlsources"`,
		useLowerURL: true,
	},
	ObjectTypeBDEF: {
		basePath:    "/sap/bc/adt/bo/behaviordefinitions",
		rootName:    "bdef:behaviorDefinition",
		namespace:   `xmlns:bdef="http://www.sap.com/adt/bo/behaviordefinitions"`,
		useLowerURL: true,
	},
	ObjectTypeSRVD: {
		basePath:    "/sap/bc/adt/ddic/srvd/sources",
		rootName:    "srvd:srvdSource",
		namespace:   `xmlns:srvd="http://www.sap.com/adt/ddic/srvdsources"`,
		useLowerURL: true,
	},
	ObjectTypeSRVB: {
		basePath:    "/sap/bc/adt/businessservices/bindings",
		rootName:    "srvb:serviceBinding",
		namespace:   `xmlns:srvb="http://www.sap.com/adt/ddic/ServiceBindings"`,
		useLowerURL: true,
	},
}

// GetObjectURL returns the ADT URL for an object based on its type and name.
// All names are URL-encoded to support namespaced objects like /UI5/CL_REPOSITORY_LOAD.
func GetObjectURL(objectType CreatableObjectType, name string, parentName string) string {
	info, ok := objectTypes[objectType]
	if !ok {
		return ""
	}

	name = strings.ToUpper(name)

	// Handle function modules specially - they're nested under function groups
	if objectType == ObjectTypeFunctionMod {
		parentName = strings.ToUpper(parentName)
		return fmt.Sprintf("%s/%s/fmodules/%s",
			info.basePath,
			url.PathEscape(parentName),
			url.PathEscape(name))
	}

	// CDS-based objects use lowercase in URLs
	if info.useLowerURL {
		return fmt.Sprintf("%s/%s", info.basePath, url.PathEscape(strings.ToLower(name)))
	}

	return fmt.Sprintf("%s/%s", info.basePath, url.PathEscape(name))
}

// GetSourceURL returns the source URL for an object.
func GetSourceURL(objectType CreatableObjectType, name string, parentName string) string {
	objectURL := GetObjectURL(objectType, name, parentName)
	if objectURL == "" {
		return ""
	}
	return objectURL + "/source/main"
}

// getCreationURL returns the URL for creating an object of the given type.
func getCreationURL(objectType CreatableObjectType, parentName string) string {
	info, ok := objectTypes[objectType]
	if !ok {
		return ""
	}

	// Function modules are created under their function group
	if objectType == ObjectTypeFunctionMod && parentName != "" {
		return fmt.Sprintf("%s/%s/fmodules", info.basePath, strings.ToUpper(parentName))
	}

	return info.basePath
}

// getObjectTypeInfo returns the metadata for an object type.
func getObjectTypeInfo(objectType CreatableObjectType) (objectTypeInfo, bool) {
	info, ok := objectTypes[objectType]
	return info, ok
}
