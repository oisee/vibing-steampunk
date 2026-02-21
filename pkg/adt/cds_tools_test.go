package adt

import (
	"context"
	"net/http"
	"testing"
)

// --- CDS Impact Analysis Tests ---

func TestParseCDSImpactAnalysis(t *testing.T) {
	xmlResponse := `<?xml version="1.0" encoding="UTF-8"?>
<usageReferences:usageReferenceResult xmlns:usageReferences="http://www.sap.com/adt/ris/usageReferences" xmlns:adtcore="http://www.sap.com/adt/core">
  <usageReferences:referencedObjects>
    <usageReferences:referencedObject usageReferences:uri="/sap/bc/adt/ddic/ddl/sources/ZCONSUMER_VIEW" usageReferences:isResult="true">
      <adtcore:adtObject adtcore:uri="/sap/bc/adt/ddic/ddl/sources/zconsumer_view" adtcore:type="DDLS/DF" adtcore:name="ZCONSUMER_VIEW" adtcore:description="Consumer CDS view">
        <adtcore:packageRef adtcore:name="$TMP"/>
      </adtcore:adtObject>
    </usageReferences:referencedObject>
    <usageReferences:referencedObject usageReferences:uri="/sap/bc/adt/programs/programs/ZREPORT" usageReferences:isResult="true">
      <adtcore:adtObject adtcore:uri="/sap/bc/adt/programs/programs/zreport" adtcore:type="PROG/P" adtcore:name="ZREPORT" adtcore:description="Report using CDS view">
        <adtcore:packageRef adtcore:name="ZTEST_PKG"/>
      </adtcore:adtObject>
    </usageReferences:referencedObject>
    <usageReferences:referencedObject usageReferences:uri="/sap/bc/adt/oo/classes/ZCL_HELPER" usageReferences:isResult="false">
      <adtcore:adtObject adtcore:uri="/sap/bc/adt/oo/classes/zcl_helper" adtcore:type="CLAS/OC" adtcore:name="ZCL_HELPER"/>
    </usageReferences:referencedObject>
  </usageReferences:referencedObjects>
</usageReferences:usageReferenceResult>`

	result, err := parseCDSImpactAnalysis([]byte(xmlResponse), "ZBASE_VIEW")
	if err != nil {
		t.Fatalf("parseCDSImpactAnalysis failed: %v", err)
	}

	if result.ViewName != "ZBASE_VIEW" {
		t.Errorf("ViewName = %q, want %q", result.ViewName, "ZBASE_VIEW")
	}
	// Only isResult=true objects should be included
	if result.TotalCount != 2 {
		t.Fatalf("TotalCount = %d, want 2", result.TotalCount)
	}
	if result.ImpactedObjects[0].Name != "ZCONSUMER_VIEW" {
		t.Errorf("Object[0].Name = %q, want %q", result.ImpactedObjects[0].Name, "ZCONSUMER_VIEW")
	}
	if result.ImpactedObjects[0].Type != "DDLS/DF" {
		t.Errorf("Object[0].Type = %q, want %q", result.ImpactedObjects[0].Type, "DDLS/DF")
	}
	if result.ImpactedObjects[0].Package != "$TMP" {
		t.Errorf("Object[0].Package = %q, want %q", result.ImpactedObjects[0].Package, "$TMP")
	}
	if result.ImpactedObjects[1].Name != "ZREPORT" {
		t.Errorf("Object[1].Name = %q, want %q", result.ImpactedObjects[1].Name, "ZREPORT")
	}
}

func TestParseCDSImpactAnalysis_Empty(t *testing.T) {
	xmlResponse := `<?xml version="1.0" encoding="UTF-8"?>
<usageReferences:usageReferenceResult xmlns:usageReferences="http://www.sap.com/adt/ris/usageReferences">
  <usageReferences:referencedObjects/>
</usageReferences:usageReferenceResult>`

	result, err := parseCDSImpactAnalysis([]byte(xmlResponse), "ZUNUSED_VIEW")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.TotalCount != 0 {
		t.Errorf("TotalCount = %d, want 0", result.TotalCount)
	}
}

func TestParseCDSImpactAnalysis_InvalidXML(t *testing.T) {
	result, err := parseCDSImpactAnalysis([]byte("not xml"), "ZVIEW")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.ViewName != "ZVIEW" {
		t.Errorf("ViewName = %q, want %q", result.ViewName, "ZVIEW")
	}
	if result.TotalCount != 0 {
		t.Errorf("TotalCount = %d, want 0", result.TotalCount)
	}
}

func TestClient_GetCDSImpactAnalysis(t *testing.T) {
	impactXML := `<?xml version="1.0" encoding="UTF-8"?>
<usageReferences:usageReferenceResult xmlns:usageReferences="http://www.sap.com/adt/ris/usageReferences" xmlns:adtcore="http://www.sap.com/adt/core">
  <usageReferences:referencedObjects>
    <usageReferences:referencedObject usageReferences:isResult="true">
      <adtcore:adtObject adtcore:uri="/sap/bc/adt/ddic/ddl/sources/zconsumer" adtcore:type="DDLS/DF" adtcore:name="ZCONSUMER"/>
    </usageReferences:referencedObject>
  </usageReferences:referencedObjects>
</usageReferences:usageReferenceResult>`

	mock := &mockTransportClient{
		responses: map[string]*http.Response{
			"/sap/bc/adt/core/discovery":                               newCSRFResponse(),
			"/sap/bc/adt/repository/informationsystem/usageReferences": newTestResponse(impactXML),
		},
	}

	cfg := NewConfig("https://sap.example.com:44300", "user", "pass")
	transport := NewTransportWithClient(cfg, mock)
	client := NewClientWithTransport(cfg, transport)

	result, err := client.GetCDSImpactAnalysis(context.Background(), "ZBASE_VIEW")
	if err != nil {
		t.Fatalf("GetCDSImpactAnalysis failed: %v", err)
	}

	if result.TotalCount != 1 {
		t.Errorf("TotalCount = %d, want 1", result.TotalCount)
	}
	if result.ImpactedObjects[0].Name != "ZCONSUMER" {
		t.Errorf("Object[0].Name = %q, want %q", result.ImpactedObjects[0].Name, "ZCONSUMER")
	}
}

func TestClient_GetCDSImpactAnalysis_ReadOnly(t *testing.T) {
	cfg := NewConfig("https://sap.example.com:44300", "user", "pass")
	cfg.Safety.ReadOnly = true
	transport := NewTransportWithClient(cfg, &mockTransportClient{responses: map[string]*http.Response{}})
	client := NewClientWithTransport(cfg, transport)

	_, err := client.GetCDSImpactAnalysis(context.Background(), "ZVIEW")
	if err == nil {
		t.Error("Expected error for read-only mode")
	}
}

// --- CDS Element Info Tests ---

func TestParseCDSElementInfo(t *testing.T) {
	xmlResponse := `<?xml version="1.0" encoding="UTF-8"?>
<ddl:ddlSource xmlns:ddl="http://www.sap.com/adt/ddic/ddlsources" xmlns:adtcore="http://www.sap.com/adt/core"
  ddl:name="ZTRAVEL" adtcore:description="Travel CDS view">
  <ddl:content>
    <ddl:element ddl:name="TravelId" ddl:type="NUMC(8)" ddl:description="Travel ID" ddl:semantics="objectId">
      <ddl:annotation ddl:name="@ObjectModel.text.element" ddl:value="Description"/>
      <ddl:annotation ddl:name="@UI.lineItem" ddl:value="[{position: 10}]"/>
    </ddl:element>
    <ddl:element ddl:name="AgencyId" ddl:type="NUMC(6)" ddl:description="Agency ID"/>
    <ddl:element ddl:name="BeginDate" ddl:type="DATS" ddl:description="Begin Date" ddl:semantics="dateRange.begin"/>
  </ddl:content>
</ddl:ddlSource>`

	result, err := parseCDSElementInfo([]byte(xmlResponse), "ZTRAVEL")
	if err != nil {
		t.Fatalf("parseCDSElementInfo failed: %v", err)
	}

	if result.ViewName != "ZTRAVEL" {
		t.Errorf("ViewName = %q, want %q", result.ViewName, "ZTRAVEL")
	}
	if len(result.Elements) != 3 {
		t.Fatalf("Expected 3 elements, got %d", len(result.Elements))
	}

	// First element should have annotations
	elem := result.Elements[0]
	if elem.Name != "TravelId" {
		t.Errorf("Element[0].Name = %q, want %q", elem.Name, "TravelId")
	}
	if elem.Type != "NUMC(8)" {
		t.Errorf("Element[0].Type = %q, want %q", elem.Type, "NUMC(8)")
	}
	if elem.Semantics != "objectId" {
		t.Errorf("Element[0].Semantics = %q, want %q", elem.Semantics, "objectId")
	}
	if len(elem.Annotations) != 2 {
		t.Fatalf("Element[0] should have 2 annotations, got %d", len(elem.Annotations))
	}
	if elem.Annotations["@ObjectModel.text.element"] != "Description" {
		t.Errorf("Annotation value = %q, want %q", elem.Annotations["@ObjectModel.text.element"], "Description")
	}

	// Third element should have semantics but no annotations
	if result.Elements[2].Semantics != "dateRange.begin" {
		t.Errorf("Element[2].Semantics = %q, want %q", result.Elements[2].Semantics, "dateRange.begin")
	}
}

func TestParseCDSElementInfo_Empty(t *testing.T) {
	xmlResponse := `<?xml version="1.0" encoding="UTF-8"?>
<ddl:ddlSource xmlns:ddl="http://www.sap.com/adt/ddic/ddlsources"
  ddl:name="ZEMPTY_VIEW"/>` // No content element

	result, err := parseCDSElementInfo([]byte(xmlResponse), "ZEMPTY_VIEW")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(result.Elements) != 0 {
		t.Errorf("Expected 0 elements, got %d", len(result.Elements))
	}
}

func TestParseCDSElementInfo_InvalidXML(t *testing.T) {
	result, err := parseCDSElementInfo([]byte("not xml at all"), "ZVIEW")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.ViewName != "ZVIEW" {
		t.Errorf("ViewName = %q, want %q", result.ViewName, "ZVIEW")
	}
	if len(result.Elements) != 0 {
		t.Errorf("Expected 0 elements for invalid XML, got %d", len(result.Elements))
	}
}

func TestClient_GetCDSElementInfo(t *testing.T) {
	elementXML := `<?xml version="1.0" encoding="UTF-8"?>
<ddl:ddlSource xmlns:ddl="http://www.sap.com/adt/ddic/ddlsources"
  ddl:name="ZTRAVEL">
  <ddl:content>
    <ddl:element ddl:name="TravelId" ddl:type="NUMC(8)"/>
  </ddl:content>
</ddl:ddlSource>`

	mock := &mockTransportClient{
		responses: map[string]*http.Response{
			"/sap/bc/adt/ddic/ddl/sources/ZTRAVEL": newTestResponse(elementXML),
		},
	}

	cfg := NewConfig("https://sap.example.com:44300", "user", "pass")
	transport := NewTransportWithClient(cfg, mock)
	client := NewClientWithTransport(cfg, transport)

	result, err := client.GetCDSElementInfo(context.Background(), "ZTRAVEL")
	if err != nil {
		t.Fatalf("GetCDSElementInfo failed: %v", err)
	}

	if len(result.Elements) != 1 {
		t.Fatalf("Expected 1 element, got %d", len(result.Elements))
	}
	if result.Elements[0].Name != "TravelId" {
		t.Errorf("Element.Name = %q, want %q", result.Elements[0].Name, "TravelId")
	}
}

func TestClient_GetCDSElementInfo_ReadOnly(t *testing.T) {
	cfg := NewConfig("https://sap.example.com:44300", "user", "pass")
	cfg.Safety.ReadOnly = true
	transport := NewTransportWithClient(cfg, &mockTransportClient{responses: map[string]*http.Response{}})
	client := NewClientWithTransport(cfg, transport)

	_, err := client.GetCDSElementInfo(context.Background(), "ZVIEW")
	if err == nil {
		t.Error("Expected error for read-only mode")
	}
}
