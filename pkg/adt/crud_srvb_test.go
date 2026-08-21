package adt

import (
	"strings"
	"testing"
)

// srvbTypeInfo mirrors what CreateObject resolves for ObjectTypeSRVB.
func srvbTestTypeInfo(t *testing.T) objectTypeInfo {
	t.Helper()
	ti, ok := objectTypes[ObjectTypeSRVB]
	if !ok {
		t.Fatalf("ObjectTypeSRVB not present in objectTypes registry")
	}
	return ti
}

func buildSRVB(t *testing.T, opts CreateObjectOptions) string {
	t.Helper()
	opts.ObjectType = ObjectTypeSRVB
	if opts.Name == "" {
		opts.Name = "ZUI_TEST_O4"
	}
	if opts.PackageName == "" {
		opts.PackageName = "$TMP"
	}
	if opts.ServiceDefinition == "" {
		opts.ServiceDefinition = "ZUI_TEST"
	}
	return buildCreateObjectBody(opts, srvbTestTypeInfo(t), "DEVELOPER")
}

// The binding category values come from SAP domain SRVB_BND_CATEGORY:
//
//	0 = UI (User Interface)
//	1 = A2X (Application to X users) i.e. Web API
//
// The default must be UI ("0"), and an explicitly requested category must survive.
func TestBuildCreateObjectBody_SRVBCategory(t *testing.T) {
	tests := []struct {
		name         string
		category     string
		wantCategory string
	}{
		{name: "default is UI", category: "", wantCategory: `srvb:category="0"`},
		{name: "explicit UI", category: "0", wantCategory: `srvb:category="0"`},
		{name: "explicit A2X/Web API", category: "1", wantCategory: `srvb:category="1"`},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			body := buildSRVB(t, CreateObjectOptions{BindingCategory: tc.category})
			if !strings.Contains(body, tc.wantCategory) {
				t.Errorf("want %s in body, got:\n%s", tc.wantCategory, body)
			}
		})
	}
}

// Regression: passing binding_version must actually reach the ADT payload.
// A silently-defaulted V2 binding for a caller that asked for V4 produces a
// service that cannot drive a Fiori Elements V4 app.
func TestBuildCreateObjectBody_SRVBVersion(t *testing.T) {
	tests := []struct {
		name        string
		version     string
		wantVersion string
	}{
		{name: "default is V2", version: "", wantVersion: `srvb:version="V2"`},
		{name: "explicit V2", version: "V2", wantVersion: `srvb:version="V2"`},
		{name: "explicit V4", version: "V4", wantVersion: `srvb:version="V4"`},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			body := buildSRVB(t, CreateObjectOptions{BindingVersion: tc.version})
			if !strings.Contains(body, tc.wantVersion) {
				t.Errorf("want %s in body, got:\n%s", tc.wantVersion, body)
			}
		})
	}
}

// The bound service definition must be upper-cased and present.
func TestBuildCreateObjectBody_SRVBServiceDefinition(t *testing.T) {
	body := buildSRVB(t, CreateObjectOptions{ServiceDefinition: "zui_flight_o4"})
	if !strings.Contains(body, `<srvb:serviceDefinition adtcore:name="ZUI_FLIGHT_O4"/>`) {
		t.Errorf("service definition not bound/upper-cased, got:\n%s", body)
	}
	if !strings.Contains(body, `srvb:type="ODATA"`) {
		t.Errorf(`want srvb:type="ODATA", got:\n%s`, body)
	}
}
