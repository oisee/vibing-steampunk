package adt

import (
	"strings"
	"testing"
)

func TestDDICObjectTypesAndURLs(t *testing.T) {
	if _, ok := objectTypes[ObjectTypeDomain]; !ok {
		t.Fatal("DDIC domain is not registered as creatable")
	}
	if _, ok := objectTypes[ObjectTypeDataElement]; !ok {
		t.Fatal("DDIC data element is not registered as creatable")
	}
	if got := GetObjectURL(ObjectTypeDomain, "ZORDER_STATUS", ""); got != "/sap/bc/adt/ddic/domains/zorder_status" {
		t.Fatalf("domain URL = %q", got)
	}
	if got := GetObjectURL(ObjectTypeDataElement, "ZORDER_STATUS", ""); got != "/sap/bc/adt/ddic/dataelements/zorder_status" {
		t.Fatalf("data element URL = %q", got)
	}
}

func TestDDICPropertyXMLContainsReviewedFields(t *testing.T) {
	domain := DomainCreateOptions{
		Name: "ZORDER_STATUS", Description: "Order status", PackageName: "$TMP", DataType: "CHAR", Length: 1, Decimals: 0,
		OutputLength: 1, OutputStyle: "RIGHTJUSTIFIED", ValueTableRef: "ZSTATUS",
		AppendExists: true, FixValues: []DomainFixValue{{Low: "A", Text: "Active"}},
	}
	body := ""
	// Exercise the pure XML fragment helper without requiring an SAP session.
	body = domainValueXML(domain)
	for _, want := range []string{"valueTableRef", "ZSTATUS", "appendExists", "fixValues", "Active"} {
		if !strings.Contains(body, want) {
			t.Fatalf("domain value XML missing %q: %s", want, body)
		}
	}

	if err := validateDomainCreateOptions(&domain); err != nil {
		t.Fatalf("valid domain rejected: %v", err)
	}
	dataElement := DataElementCreateOptions{
		Name: "ZORDER_STATUS", Description: "Order status", PackageName: "$TMP",
		DataType: "CHAR", Length: 1,
		Labels: DataElementFieldLabels{Short: "Status", Medium: "Order status", Long: "Order status", Heading: "Status"},
	}
	if err := validateDataElementCreateOptions(&dataElement); err != nil {
		t.Fatalf("valid data element rejected: %v", err)
	}
}
