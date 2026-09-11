package adt

import (
	"encoding/xml"
	"errors"
	"strings"
	"testing"
)

// The payload has to match transformation SIA1_BUCAPPS, which expects
// sia1:bucapps holding sia1:bucapp entries with
// businessCatalogAppAssignmentID, businessCatalogID and appID. Go's XML
// encoder handles prefixed names by literal string, so assert the output
// rather than trusting it.
func TestBucappsPayloadSerialization(t *testing.T) {
	payload := bucappsPayload{
		NS: "http://www.sap.com/iam/sia1",
		Apps: []bucapp{
			{
				AssignmentID: "ZBC_MY_CATALOG_ZUI_MY_SRV_O4_0001_XXXX_IBS",
				CatalogID:    "ZBC_MY_CATALOG",
				AppID:        "ZUI_MY_SRV_O4_0001_XXXX_IBS",
			},
		},
	}

	out, err := xml.MarshalIndent(payload, "", "  ")
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	got := string(out)

	for _, want := range []string{
		`<sia1:bucapps`,
		`xmlns:sia1="http://www.sap.com/iam/sia1"`,
		`<sia1:bucapp>`,
		`<sia1:businessCatalogAppAssignmentID>ZBC_MY_CATALOG_ZUI_MY_SRV_O4_0001_XXXX_IBS</sia1:businessCatalogAppAssignmentID>`,
		`<sia1:businessCatalogID>ZBC_MY_CATALOG</sia1:businessCatalogID>`,
		`<sia1:appID>ZUI_MY_SRV_O4_0001_XXXX_IBS</sia1:appID>`,
	} {
		if !strings.Contains(got, want) {
			t.Errorf("payload missing %q\ngot:\n%s", want, got)
		}
	}
}

// SIA1/BC must land in the shared objectTypes registry via init, with the
// creation path, root element and namespace read from the live system.
func TestBusinessCatalogTypeRegistered(t *testing.T) {
	info, ok := objectTypes[ObjectTypeBusinessCatalog]
	if !ok {
		t.Fatal("SIA1/BC is not registered in objectTypes")
	}
	if info.creationPath != "/sap/bc/adt/aps/cloud/iam/sia1" {
		t.Errorf("creationPath = %q", info.creationPath)
	}
	if info.rootName != "sia1:sia1" {
		t.Errorf("rootName = %q", info.rootName)
	}
	if !strings.Contains(info.namespace, "http://www.sap.com/iam/sia1") {
		t.Errorf("namespace = %q", info.namespace)
	}
}

func TestBusinessCatalogURLLowercasesName(t *testing.T) {
	if got := BusinessCatalogURL("ZBC_MY_CATALOG"); got != "/sap/bc/adt/aps/cloud/iam/sia1/zbc_my_catalog" {
		t.Errorf("BusinessCatalogURL = %q", got)
	}
}

// A missing lock handle has to fail before any HTTP call: ADT rejects writes
// to a sub-resource of an unlocked object, and a silent attempt would look
// like a network fault instead of a caller mistake.
func TestAssignBusinessCatalogAppsValidates(t *testing.T) {
	c := NewClient("https://example.invalid", "u", "p")

	cases := []struct {
		name    string
		catalog string
		lock    string
		apps    []BusinessCatalogApp
		want    string
	}{
		{"no catalog", "", "LOCK", []BusinessCatalogApp{{AppID: "A"}}, "catalog name is required"},
		{"no lock", "ZBC", "", []BusinessCatalogApp{{AppID: "A"}}, "lock handle is required"},
		{"no apps", "ZBC", "LOCK", nil, "at least one app assignment is required"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := c.AssignBusinessCatalogApps(nil, tc.catalog, tc.lock, tc.apps, "")
			if err == nil {
				t.Fatal("expected an error")
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Errorf("error = %q, want it to contain %q", err.Error(), tc.want)
			}
		})
	}
}

// The typed exception id is what distinguishes "already there" from a real
// failure. Matching the localised message text instead would break on a
// non-English system or a release that rewords it.
func TestIsResourceAlreadyExists(t *testing.T) {
	real := errors.New(`creating object: ADT API error: status 400 at /sap/bc/adt/aps/cloud/iam/sia1: ` +
		`<exc:exception><type id="ExceptionResourceAlreadyExists"/>` +
		`<message lang="EN">Resource Business Catalog ZBC_MY_CATALOG does already exist.</message></exc:exception>`)
	if !IsResourceAlreadyExists(real) {
		t.Error("did not recognise ExceptionResourceAlreadyExists")
	}

	other := errors.New(`ADT API error: status 405: <type id="ExceptionMethodNotSupported"/>`)
	if IsResourceAlreadyExists(other) {
		t.Error("matched an unrelated ADT exception")
	}
	if IsResourceAlreadyExists(nil) {
		t.Error("matched nil")
	}
}
