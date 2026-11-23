package adt

import (
	"context"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// The dataelements endpoint refuses application/xml with 406 on every name, so
// both callers of it must send the versioned vocabulary type. GetDataElementLabels
// was fixed for this once; GetTypeInfo was missed and kept the generic type, which
// meant it had never returned anything to anybody. These two tests pin the header
// on BOTH callers, because the bug was a twin drifting out of step with its twin —
// pinning only the one that broke would let the next divergence through.
//
// v2 first, v1 at a lower q: 7.50 knows only v1 and answers a v2-only Accept
// with 406 too.
const wantDataElementsAccept = "application/vnd.sap.adt.dataelements.v2+xml, application/vnd.sap.adt.dataelements.v1+xml;q=0.9"

func TestGetTypeInfo_SendsVersionedAccept(t *testing.T) {
	xmlResp := `<?xml version="1.0" encoding="utf-8"?><blue:wbobj adtcore:name="APC_CONNECTION_ID" adtcore:type="DTEL/DE" adtcore:description="Connection ID" xmlns:blue="http://www.sap.com/wbobj/dictionary/dtel" xmlns:adtcore="http://www.sap.com/adt/core"><dtel:dataElement xmlns:dtel="http://www.sap.com/adt/dictionary/dataelements"><dtel:typeKind>domain</dtel:typeKind></dtel:dataElement></blue:wbobj>`

	mock := &mockTransportClient{
		responses: map[string]*http.Response{
			"/sap/bc/adt/ddic/dataelements/APC_CONNECTION_ID": newTestResponse(xmlResp),
			"discovery": newTestResponse("OK"),
		},
	}

	cfg := NewConfig("https://sap.example.com:44300", "user", "pass")
	client := NewClientWithTransport(cfg, NewTransportWithClient(cfg, mock))

	if _, err := client.GetTypeInfo(context.Background(), "APC_CONNECTION_ID"); err != nil {
		t.Fatalf("GetTypeInfo failed: %v", err)
	}

	assertDataElementsAccept(t, mock, "GetTypeInfo")
}

func TestGetDataElementLabels_SendsVersionedAccept(t *testing.T) {
	xmlResp := `<?xml version="1.0" encoding="utf-8"?><blue:wbobj xmlns:blue="http://www.sap.com/wbobj/dictionary/dtel"><dtel:dataElement xmlns:dtel="http://www.sap.com/adt/dictionary/dataelements"><dtel:shortFieldLabel>X</dtel:shortFieldLabel></dtel:dataElement></blue:wbobj>`

	mock := &mockTransportClient{
		responses: map[string]*http.Response{
			"/sap/bc/adt/ddic/dataelements/ZDEMO_ORDER_ID": newTestResponse(xmlResp),
			"discovery": newTestResponse("OK"),
		},
	}

	cfg := NewConfig("https://sap.example.com:44300", "user", "pass")
	client := NewClientWithTransport(cfg, NewTransportWithClient(cfg, mock))

	if _, err := client.GetDataElementLabels(context.Background(), "ZDEMO_ORDER_ID", "EN"); err != nil {
		t.Fatalf("GetDataElementLabels failed: %v", err)
	}

	assertDataElementsAccept(t, mock, "GetDataElementLabels")
}

// assertDataElementsAccept checks the request that actually went to the
// dataelements endpoint, not merely the last request made, so a CSRF or
// discovery hop in between cannot make the assertion pass by accident.
func assertDataElementsAccept(t *testing.T, mock *mockTransportClient, caller string) {
	t.Helper()

	found := false
	for _, req := range mock.requests {
		if !strings.Contains(req.URL.Path, "/sap/bc/adt/ddic/dataelements/") {
			continue
		}
		found = true
		if got := req.Header.Get("Accept"); got != wantDataElementsAccept {
			t.Errorf("%s sent Accept %q to the dataelements endpoint, want %q\n"+
				"A generic type is refused there with 406 on every name, so this "+
				"call would return nothing to anybody.", caller, got, wantDataElementsAccept)
		}
	}
	if !found {
		t.Fatalf("%s made no request to the dataelements endpoint", caller)
	}
}

// The parser reads child elements of dtel:dataElement. The previous version read
// them as attributes of the root and returned zero for every element, which the
// 406 hid. These fixtures are real documents read off a system, trimmed to the
// dataElement block plus the root attributes, and they cover both typeKinds and
// non-zero decimals — one specimen would not have caught a domain-typed element
// or a padded decimal.
func TestGetTypeInfo_ParsesRealDocuments(t *testing.T) {
	doc := func(name, objType, desc, kind, domain, dataType, length, decimals string) string {
		return `<?xml version="1.0" encoding="utf-8"?><blue:wbobj adtcore:name="` + name +
			`" adtcore:type="` + objType + `" adtcore:description="` + desc +
			`" xmlns:blue="http://www.sap.com/wbobj/dictionary/dtel" xmlns:adtcore="http://www.sap.com/adt/core">` +
			`<adtcore:packageRef adtcore:uri="/sap/bc/adt/packages/x" adtcore:type="DEVC/K" adtcore:name="X" adtcore:description="pkg"/>` +
			`<dtel:dataElement xmlns:dtel="http://www.sap.com/adt/dictionary/dataelements">` +
			`<dtel:typeKind>` + kind + `</dtel:typeKind><dtel:typeName>` + domain + `</dtel:typeName>` +
			`<dtel:dataType>` + dataType + `</dtel:dataType>` +
			`<dtel:dataTypeLength>` + length + `</dtel:dataTypeLength>` +
			`<dtel:dataTypeDecimals>` + decimals + `</dtel:dataTypeDecimals>` +
			`<dtel:shortFieldLabel>ID</dtel:shortFieldLabel></dtel:dataElement></blue:wbobj>`
	}

	cases := []struct {
		name, kind, domain, dataType string
		length, decimals             int
		body                         string
	}{
		{"APC_CONNECTION_ID", "predefinedAbapType", "", "CHAR", 32, 0,
			doc("APC_CONNECTION_ID", "DTEL/DE", "APC connection id", "predefinedAbapType", "", "CHAR", "000032", "000000")},
		{"AMC_CHANNEL_ID", "domain", "AMC_CHANNEL_ID", "SSTRING", 140, 0,
			doc("AMC_CHANNEL_ID", "DTEL/DE", "Identifier of the ABAP Messaging Channel", "domain", "AMC_CHANNEL_ID", "SSTRING", "000140", "000000")},
		{"DMBTR", "domain", "AFLE13D2O16N_TO_23D2O30N", "CURR", 23, 2,
			doc("DMBTR", "DTEL/DE", "Amount", "domain", "AFLE13D2O16N_TO_23D2O30N", "CURR", "000023", "000002")},
		{"MENGE_D", "domain", "MENG13", "QUAN", 13, 3,
			doc("MENGE_D", "DTEL/DE", "Quantity", "domain", "MENG13", "QUAN", "000013", "000003")},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			mock := &mockTransportClient{responses: map[string]*http.Response{
				"/sap/bc/adt/ddic/dataelements/" + tc.name: newTestResponse(tc.body),
				"discovery": newTestResponse("OK"),
			}}
			cfg := NewConfig("https://sap.example.com:44300", "user", "pass")
			client := NewClientWithTransport(cfg, NewTransportWithClient(cfg, mock))

			got, err := client.GetTypeInfo(context.Background(), tc.name)
			if err != nil {
				t.Fatalf("GetTypeInfo: %v", err)
			}
			for _, c := range []struct {
				field     string
				got, want any
			}{
				{"Name", got.Name, tc.name},
				{"Type", got.Type, tc.dataType},
				{"Length", got.Length, tc.length},
				{"Decimals", got.Decimals, tc.decimals},
				{"TypeKind", got.TypeKind, tc.kind},
				{"DomainName", got.DomainName, tc.domain},
				{"ObjectType", got.ObjectType, "DTEL/DE"},
			} {
				if c.got != c.want {
					t.Errorf("%s = %v, want %v", c.field, c.got, c.want)
				}
			}
		})
	}
}

// Parsing the FULL documents as a system actually sends them, byte for byte,
// rather than the trimmed fixtures above. The trimmed ones are my transcription
// and could agree with the parser while both disagree with SAP; these cannot.
// They carry the whole root attribute set, the atom:links and the packageRef,
// which is where a naive attribute mapping goes wrong: packageRef also has an
// adtcore:type, so anything matching attributes loosely picks up "DEVC/K".
func TestGetTypeInfo_ParsesUnmodifiedSystemDocuments(t *testing.T) {
	cases := []struct {
		file, name, kind, domain, dataType string
		length, decimals                   int
	}{
		{"dataelement-apc_connection_id.v2.xml", "APC_CONNECTION_ID", "predefinedAbapType", "", "CHAR", 32, 0},
		{"dataelement-dmbtr.v2.xml", "DMBTR", "domain", "AFLE13D2O16N_TO_23D2O30N", "CURR", 23, 2},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			body, err := os.ReadFile(filepath.Join("testdata", tc.file))
			if err != nil {
				t.Fatalf("reading fixture: %v", err)
			}

			mock := &mockTransportClient{responses: map[string]*http.Response{
				"/sap/bc/adt/ddic/dataelements/" + tc.name: newTestResponse(string(body)),
				"discovery": newTestResponse("OK"),
			}}
			cfg := NewConfig("https://sap.example.com:44300", "user", "pass")
			client := NewClientWithTransport(cfg, NewTransportWithClient(cfg, mock))

			got, err := client.GetTypeInfo(context.Background(), tc.name)
			if err != nil {
				t.Fatalf("GetTypeInfo: %v", err)
			}
			if got.Name != tc.name || got.Type != tc.dataType ||
				got.Length != tc.length || got.Decimals != tc.decimals ||
				got.TypeKind != tc.kind || got.DomainName != tc.domain {
				t.Errorf("got %+v\nwant Name=%s Type=%s Length=%d Decimals=%d TypeKind=%s DomainName=%s",
					got, tc.name, tc.dataType, tc.length, tc.decimals, tc.kind, tc.domain)
			}
			if got.ObjectType != "DTEL/DE" {
				t.Errorf("ObjectType = %q, want DTEL/DE (the packageRef's DEVC/K must not win)", got.ObjectType)
			}
		})
	}
}
