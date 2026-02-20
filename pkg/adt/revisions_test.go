package adt

import (
	"context"
	"net/http"
	"strings"
	"testing"
)

func TestParseRevisionFeed(t *testing.T) {
	feedXML := `<?xml version="1.0" encoding="UTF-8"?>
<atom:feed xmlns:atom="http://www.w3.org/2005/Atom"
           xmlns:adtcore="http://www.sap.com/adt/core">
  <atom:title>Versions of ZTEST_PROGRAM</atom:title>
  <atom:entry>
    <atom:id>5</atom:id>
    <atom:title>Active Version</atom:title>
    <atom:updated>2025-06-15T14:30:00Z</atom:updated>
    <atom:author><atom:name>DEVELOPER1</atom:name></atom:author>
    <atom:content src="/sap/bc/adt/programs/programs/ztest/source/main?version=5" type="text/plain"/>
    <atom:link href="/sap/bc/adt/cts/transportrequests/K900123" rel="http://www.sap.com/adt/relations/transport"
               type="application/vnd.sap.adt.transportrequests.v1+xml" adtcore:name="K900123"/>
  </atom:entry>
  <atom:entry>
    <atom:id>4</atom:id>
    <atom:title>Previous Version</atom:title>
    <atom:updated>2025-06-10T09:15:00Z</atom:updated>
    <atom:author><atom:name>DEVELOPER2</atom:name></atom:author>
    <atom:content src="/sap/bc/adt/programs/programs/ztest/source/main?version=4" type="text/plain"/>
  </atom:entry>
  <atom:entry>
    <atom:id>3</atom:id>
    <atom:title>Initial Version</atom:title>
    <atom:updated>2025-06-01T08:00:00Z</atom:updated>
    <atom:author><atom:name>DEVELOPER1</atom:name></atom:author>
    <atom:content src="/sap/bc/adt/programs/programs/ztest/source/main?version=3" type="text/plain"/>
    <atom:link href="/sap/bc/adt/cts/transportrequests/K900100" rel="http://www.sap.com/adt/relations/transport"
               type="application/vnd.sap.adt.transportrequests.v1+xml" adtcore:name="K900100"/>
  </atom:entry>
</atom:feed>`

	revisions, err := ParseRevisionFeed([]byte(feedXML))
	if err != nil {
		t.Fatalf("ParseRevisionFeed failed: %v", err)
	}

	if len(revisions) != 3 {
		t.Fatalf("Expected 3 revisions, got %d", len(revisions))
	}

	// Check first entry
	if revisions[0].Version != "5" {
		t.Errorf("revisions[0].Version = %q, want %q", revisions[0].Version, "5")
	}
	if revisions[0].VersionTitle != "Active Version" {
		t.Errorf("revisions[0].VersionTitle = %q, want %q", revisions[0].VersionTitle, "Active Version")
	}
	if revisions[0].Author != "DEVELOPER1" {
		t.Errorf("revisions[0].Author = %q, want %q", revisions[0].Author, "DEVELOPER1")
	}
	if revisions[0].Date != "2025-06-15T14:30:00Z" {
		t.Errorf("revisions[0].Date = %q, want %q", revisions[0].Date, "2025-06-15T14:30:00Z")
	}
	if revisions[0].URI != "/sap/bc/adt/programs/programs/ztest/source/main?version=5" {
		t.Errorf("revisions[0].URI = %q", revisions[0].URI)
	}
	if revisions[0].Transport != "K900123" {
		t.Errorf("revisions[0].Transport = %q, want %q", revisions[0].Transport, "K900123")
	}

	// Check second entry (no transport)
	if revisions[1].Transport != "" {
		t.Errorf("revisions[1].Transport = %q, want empty", revisions[1].Transport)
	}
	if revisions[1].Author != "DEVELOPER2" {
		t.Errorf("revisions[1].Author = %q, want %q", revisions[1].Author, "DEVELOPER2")
	}

	// Check third entry
	if revisions[2].Transport != "K900100" {
		t.Errorf("revisions[2].Transport = %q, want %q", revisions[2].Transport, "K900100")
	}
}

func TestParseRevisionFeed_Empty(t *testing.T) {
	feedXML := `<?xml version="1.0" encoding="UTF-8"?>
<atom:feed xmlns:atom="http://www.w3.org/2005/Atom">
  <atom:title>No versions</atom:title>
</atom:feed>`

	revisions, err := ParseRevisionFeed([]byte(feedXML))
	if err != nil {
		t.Fatalf("ParseRevisionFeed failed: %v", err)
	}
	if len(revisions) != 0 {
		t.Errorf("Expected 0 revisions, got %d", len(revisions))
	}
}

func TestParseRevisionFeed_InvalidXML(t *testing.T) {
	_, err := ParseRevisionFeed([]byte("not xml"))
	if err == nil {
		t.Error("Expected error for invalid XML")
	}
}

func TestResolveRevisionURL(t *testing.T) {
	cfg := NewConfig("https://sap.example.com:44300", "user", "pass")
	client := &Client{config: cfg}

	tests := []struct {
		name       string
		objectType string
		objName    string
		opts       *GetSourceOptions
		wantURL    string
		wantErr    bool
	}{
		{
			name:       "PROG",
			objectType: "PROG",
			objName:    "ZTEST",
			opts:       &GetSourceOptions{},
			wantURL:    "/sap/bc/adt/programs/programs/ZTEST/source/main/versions",
		},
		{
			name:       "CLAS main",
			objectType: "CLAS",
			objName:    "ZCL_TEST",
			opts:       &GetSourceOptions{},
			wantURL:    "/sap/bc/adt/oo/classes/ZCL_TEST/includes/main/versions",
		},
		{
			name:       "CLAS explicit main",
			objectType: "CLAS",
			objName:    "ZCL_TEST",
			opts:       &GetSourceOptions{Include: "main"},
			wantURL:    "/sap/bc/adt/oo/classes/ZCL_TEST/includes/main/versions",
		},
		{
			name:       "CLAS testclasses",
			objectType: "CLAS",
			objName:    "ZCL_TEST",
			opts:       &GetSourceOptions{Include: "testclasses"},
			wantURL:    "/sap/bc/adt/oo/classes/ZCL_TEST/includes/testclasses/versions",
		},
		{
			name:       "INTF",
			objectType: "INTF",
			objName:    "ZIF_TEST",
			opts:       &GetSourceOptions{},
			wantURL:    "/sap/bc/adt/oo/interfaces/ZIF_TEST/includes/main/versions",
		},
		{
			name:       "FUNC",
			objectType: "FUNC",
			objName:    "Z_TEST_FM",
			opts:       &GetSourceOptions{Parent: "ZFUGR"},
			wantURL:    "/sap/bc/adt/functions/groups/ZFUGR/fmodules/Z_TEST_FM/source/main/versions",
		},
		{
			name:       "FUNC without parent",
			objectType: "FUNC",
			objName:    "Z_TEST_FM",
			opts:       &GetSourceOptions{},
			wantErr:    true,
		},
		{
			name:       "INCL",
			objectType: "INCL",
			objName:    "ZTEST_INCL",
			opts:       &GetSourceOptions{},
			wantURL:    "/sap/bc/adt/programs/includes/ZTEST_INCL/source/main/versions",
		},
		{
			name:       "DDLS",
			objectType: "DDLS",
			objName:    "ZTEST_CDS",
			opts:       &GetSourceOptions{},
			wantURL:    "/sap/bc/adt/ddic/ddl/sources/ZTEST_CDS/source/main/versions",
		},
		{
			name:       "BDEF",
			objectType: "BDEF",
			objName:    "ZTEST_BDEF",
			opts:       &GetSourceOptions{},
			wantURL:    "/sap/bc/adt/bo/behaviordefinitions/ZTEST_BDEF/source/main/versions",
		},
		{
			name:       "SRVD",
			objectType: "SRVD",
			objName:    "ZTEST_SRVD",
			opts:       &GetSourceOptions{},
			wantURL:    "/sap/bc/adt/ddic/srvd/sources/ZTEST_SRVD/source/main/versions",
		},
		{
			name:       "unsupported type",
			objectType: "TABL",
			objName:    "ZTEST",
			opts:       &GetSourceOptions{},
			wantErr:    true,
		},
		{
			name:       "namespace object",
			objectType: "CLAS",
			objName:    "/UI5/CL_REPOSITORY",
			opts:       &GetSourceOptions{},
			wantURL:    "/sap/bc/adt/oo/classes/%2FUI5%2FCL_REPOSITORY/includes/main/versions",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := client.resolveRevisionURL(tt.objectType, tt.objName, tt.opts)
			if tt.wantErr {
				if err == nil {
					t.Error("Expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("Unexpected error: %v", err)
			}
			if got != tt.wantURL {
				t.Errorf("got %q, want %q", got, tt.wantURL)
			}
		})
	}
}

func TestExtractVersionLabel(t *testing.T) {
	tests := []struct {
		uri    string
		expect string
	}{
		{"current", "current"},
		{"/sap/bc/adt/programs/programs/ztest/source/main?version=42", "v42"},
		{"/sap/bc/adt/programs/programs/ztest/source/main?version=5&format=text", "v5"},
		// Class format: .../versions/{timestamp}/{version}/content
		{"/sap/bc/adt/oo/classes/%2FCOCKPIT%2FCL_TOOLS/includes/main/versions/20161212091747/00000/content", "v00000"},
		{"/sap/bc/adt/oo/classes/ZCL_TEST/includes/main/versions/20250101120000/00003/content", "v00003"},
		{"/some/path/123", "123"},
		{"", ""},
	}

	for _, tt := range tests {
		t.Run(tt.uri, func(t *testing.T) {
			got := extractVersionLabel(tt.uri)
			if got != tt.expect {
				t.Errorf("extractVersionLabel(%q) = %q, want %q", tt.uri, got, tt.expect)
			}
		})
	}
}

func TestClient_GetRevisions(t *testing.T) {
	feedXML := `<?xml version="1.0" encoding="UTF-8"?>
<atom:feed xmlns:atom="http://www.w3.org/2005/Atom"
           xmlns:adtcore="http://www.sap.com/adt/core">
  <atom:title>Versions</atom:title>
  <atom:entry>
    <atom:id>1</atom:id>
    <atom:title>Version 1</atom:title>
    <atom:updated>2025-01-01T00:00:00Z</atom:updated>
    <atom:author><atom:name>DEV</atom:name></atom:author>
    <atom:content src="/sap/bc/adt/programs/programs/ZTEST/source/main?version=1" type="text/plain"/>
  </atom:entry>
</atom:feed>`

	mock := &mockTransportClient{
		responses: map[string]*http.Response{
			// PROG uses /source/main/versions
			"/sap/bc/adt/programs/programs/ZTEST/source/main/versions": newTestResponse(feedXML),
		},
	}

	cfg := NewConfig("https://sap.example.com:44300", "user", "pass")
	transport := NewTransportWithClient(cfg, mock)
	client := NewClientWithTransport(cfg, transport)

	revisions, err := client.GetRevisions(context.Background(), "PROG", "ZTEST", nil)
	if err != nil {
		t.Fatalf("GetRevisions failed: %v", err)
	}

	if len(revisions) != 1 {
		t.Fatalf("Expected 1 revision, got %d", len(revisions))
	}
	if revisions[0].Author != "DEV" {
		t.Errorf("Author = %q, want %q", revisions[0].Author, "DEV")
	}

	// Verify the correct URL was requested
	if len(mock.requests) == 0 {
		t.Fatal("No requests made")
	}
	requestPath := mock.requests[0].URL.Path
	if !strings.HasSuffix(requestPath, "/versions") {
		t.Errorf("Request path %q should end with /versions", requestPath)
	}
}

func TestClient_GetRevisionSource(t *testing.T) {
	sourceCode := "REPORT ztest.\nWRITE 'Old version'."

	mock := &mockTransportClient{
		responses: map[string]*http.Response{
			"/sap/bc/adt/programs/programs/ZTEST/source/main": newTestResponse(sourceCode),
		},
	}

	cfg := NewConfig("https://sap.example.com:44300", "user", "pass")
	transport := NewTransportWithClient(cfg, mock)
	client := NewClientWithTransport(cfg, transport)

	source, err := client.GetRevisionSource(context.Background(), "/sap/bc/adt/programs/programs/ZTEST/source/main?version=1")
	if err != nil {
		t.Fatalf("GetRevisionSource failed: %v", err)
	}

	if !strings.Contains(source, "REPORT ztest") {
		t.Error("Source should contain REPORT statement")
	}
}

func TestClient_GetRevisionSource_EmptyURI(t *testing.T) {
	cfg := NewConfig("https://sap.example.com:44300", "user", "pass")
	transport := NewTransportWithClient(cfg, &mockTransportClient{responses: map[string]*http.Response{}})
	client := NewClientWithTransport(cfg, transport)

	_, err := client.GetRevisionSource(context.Background(), "")
	if err == nil {
		t.Error("Expected error for empty URI")
	}
}
