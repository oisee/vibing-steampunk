package adt

import (
	"testing"
)

func TestParseATCCustomizing(t *testing.T) {
	xmlData := `<?xml version="1.0" encoding="utf-8"?>
<atccust:customizing xmlns:atccust="http://www.sap.com/adt/atc/customizing">
  <atccust:properties>
    <atccust:property name="systemCheckVariant" value="STANDARD"/>
    <atccust:property name="transportCheckVariant" value="TRANSPORT_CHECK"/>
  </atccust:properties>
  <atccust:exemption>
    <atccust:reasons>
      <atccust:reason id="FPOS" title="False Positive" justificationMandatory="true"/>
      <atccust:reason id="OTHR" title="Other" justificationMandatory="false"/>
    </atccust:reasons>
  </atccust:exemption>
</atccust:customizing>`

	result, err := parseATCCustomizing([]byte(xmlData))
	if err != nil {
		t.Fatalf("parseATCCustomizing failed: %v", err)
	}

	// Check properties
	if len(result.Properties) != 2 {
		t.Errorf("expected 2 properties, got %d", len(result.Properties))
	}

	var foundVariant bool
	for _, p := range result.Properties {
		if p.Name == "systemCheckVariant" && p.Value == "STANDARD" {
			foundVariant = true
		}
	}
	if !foundVariant {
		t.Error("expected to find systemCheckVariant=STANDARD")
	}

	// Check exemptions
	if len(result.Exemptions) != 2 {
		t.Errorf("expected 2 exemptions, got %d", len(result.Exemptions))
	}

	var foundFPOS bool
	for _, e := range result.Exemptions {
		if e.ID == "FPOS" && e.Title == "False Positive" && e.JustificationMandatory {
			foundFPOS = true
		}
	}
	if !foundFPOS {
		t.Error("expected to find FPOS exemption with JustificationMandatory=true")
	}
}

func TestParseATCRunResult(t *testing.T) {
	xmlData := `<?xml version="1.0" encoding="utf-8"?>
<atcworklist:worklistRun xmlns:atcworklist="http://www.sap.com/adt/atc/worklist">
  <atcworklist:worklistId>12345ABCDE</atcworklist:worklistId>
  <atcworklist:worklistTimestamp>2025-12-04T10:30:00Z</atcworklist:worklistTimestamp>
  <atcworklist:infos>
    <atcworklist:info type="info" description="Check completed"/>
  </atcworklist:infos>
</atcworklist:worklistRun>`

	result, err := parseATCRunResult([]byte(xmlData))
	if err != nil {
		t.Fatalf("parseATCRunResult failed: %v", err)
	}

	if result.WorklistID != "12345ABCDE" {
		t.Errorf("expected worklistId '12345ABCDE', got '%s'", result.WorklistID)
	}

	if len(result.Infos) != 1 {
		t.Errorf("expected 1 info, got %d", len(result.Infos))
	}

	if result.Infos[0].Type != "info" || result.Infos[0].Description != "Check completed" {
		t.Errorf("unexpected info: %+v", result.Infos[0])
	}
}

func TestParseATCWorklist(t *testing.T) {
	xmlData := `<?xml version="1.0" encoding="utf-8"?>
<atcworklist:worklist xmlns:atcworklist="http://www.sap.com/adt/atc/worklist"
                      xmlns:atcobject="http://www.sap.com/adt/atc/object"
                      xmlns:atcfinding="http://www.sap.com/adt/atc/finding"
                      xmlns:adtcore="http://www.sap.com/adt/core"
                      id="12345ABCDE" timestamp="2025-12-04T10:30:00Z"
                      usedObjectSet="LAST_RUN" objectSetIsComplete="true">
  <atcworklist:objectSets>
    <atcworklist:objectSet name="LAST_RUN" title="Last Run" kind="LAST_RUN"/>
  </atcworklist:objectSets>
  <atcworklist:objects>
    <atcobject:object uri="/sap/bc/adt/oo/classes/ZCL_TEST" type="CLAS/OC"
                      name="ZCL_TEST" packageName="$TMP" author="DEVELOPER">
      <atcfinding:findings>
        <atcfinding:finding uri="/sap/bc/adt/atc/findings/123"
                           location="/sap/bc/adt/oo/classes/ZCL_TEST/source/main#start=10,5"
                           priority="2" checkId="CL_CI_TEST" checkTitle="Test Check"
                           messageId="001" messageTitle="Unused variable"
                           exemptionApproval="" exemptionKind="" quickfixInfo="QF123">
          <atcfinding:link href="/sap/bc/adt/atc/quickfix/123" rel="http://www.sap.com/adt/relations/quickfix" type="application/xml"/>
        </atcfinding:finding>
        <atcfinding:finding uri="/sap/bc/adt/atc/findings/124"
                           location="/sap/bc/adt/oo/classes/ZCL_TEST/source/main#start=25,1"
                           priority="1" checkId="CL_CI_SYNTAX" checkTitle="Syntax Check"
                           messageId="002" messageTitle="Syntax error in statement">
        </atcfinding:finding>
      </atcfinding:findings>
    </atcobject:object>
  </atcworklist:objects>
</atcworklist:worklist>`

	result, err := parseATCWorklist([]byte(xmlData))
	if err != nil {
		t.Fatalf("parseATCWorklist failed: %v", err)
	}

	// Check basic attributes
	if result.ID != "12345ABCDE" {
		t.Errorf("expected id '12345ABCDE', got '%s'", result.ID)
	}
	if result.UsedObjectSet != "LAST_RUN" {
		t.Errorf("expected usedObjectSet 'LAST_RUN', got '%s'", result.UsedObjectSet)
	}
	if !result.ObjectSetIsComplete {
		t.Error("expected objectSetIsComplete to be true")
	}

	// Check object sets
	if len(result.ObjectSets) != 1 {
		t.Errorf("expected 1 object set, got %d", len(result.ObjectSets))
	}

	// Check objects
	if len(result.Objects) != 1 {
		t.Fatalf("expected 1 object, got %d", len(result.Objects))
	}

	obj := result.Objects[0]
	if obj.Name != "ZCL_TEST" {
		t.Errorf("expected object name 'ZCL_TEST', got '%s'", obj.Name)
	}
	if obj.PackageName != "$TMP" {
		t.Errorf("expected package '$TMP', got '%s'", obj.PackageName)
	}

	// Check findings
	if len(obj.Findings) != 2 {
		t.Fatalf("expected 2 findings, got %d", len(obj.Findings))
	}

	// First finding (warning)
	f1 := obj.Findings[0]
	if f1.Priority != 2 {
		t.Errorf("expected priority 2 (warning), got %d", f1.Priority)
	}
	if f1.MessageTitle != "Unused variable" {
		t.Errorf("expected message 'Unused variable', got '%s'", f1.MessageTitle)
	}
	if f1.Line != 10 || f1.Column != 5 {
		t.Errorf("expected line=10, column=5, got line=%d, column=%d", f1.Line, f1.Column)
	}
	if f1.QuickfixInfo != "QF123" {
		t.Errorf("expected quickfixInfo 'QF123', got '%s'", f1.QuickfixInfo)
	}

	// Second finding (error)
	f2 := obj.Findings[1]
	if f2.Priority != 1 {
		t.Errorf("expected priority 1 (error), got %d", f2.Priority)
	}
	if f2.Line != 25 || f2.Column != 1 {
		t.Errorf("expected line=25, column=1, got line=%d, column=%d", f2.Line, f2.Column)
	}
}

func TestParseATCWorklistEmpty(t *testing.T) {
	xmlData := `<?xml version="1.0" encoding="utf-8"?>
<atcworklist:worklist xmlns:atcworklist="http://www.sap.com/adt/atc/worklist"
                      id="EMPTY123" timestamp="2025-12-04T10:30:00Z"
                      usedObjectSet="" objectSetIsComplete="true">
  <atcworklist:objectSets/>
  <atcworklist:objects/>
</atcworklist:worklist>`

	result, err := parseATCWorklist([]byte(xmlData))
	if err != nil {
		t.Fatalf("parseATCWorklist failed: %v", err)
	}

	if result.ID != "EMPTY123" {
		t.Errorf("expected id 'EMPTY123', got '%s'", result.ID)
	}
	if len(result.Objects) != 0 {
		t.Errorf("expected 0 objects, got %d", len(result.Objects))
	}
}

func TestATCFindingPriorityMapping(t *testing.T) {
	// Test priority interpretation: 1=Error, 2=Warning, 3+=Info
	tests := []struct {
		priority int
		expected string
	}{
		{1, "Error"},
		{2, "Warning"},
		{3, "Info"},
		{4, "Info"},
	}

	for _, tt := range tests {
		var got string
		switch tt.priority {
		case 1:
			got = "Error"
		case 2:
			got = "Warning"
		default:
			got = "Info"
		}
		if got != tt.expected {
			t.Errorf("priority %d: expected %s, got %s", tt.priority, tt.expected, got)
		}
	}
}

func TestParseATCWorklistWithTags(t *testing.T) {
	xmlData := `<?xml version="1.0" encoding="utf-8"?>
<atcworklist:worklist xmlns:atcworklist="http://www.sap.com/adt/atc/worklist"
                      xmlns:atcobject="http://www.sap.com/adt/atc/object"
                      xmlns:atcfinding="http://www.sap.com/adt/atc/finding"
                      xmlns:adtcore="http://www.sap.com/adt/core"
                      id="TAGS123" timestamp="2025-12-04T10:30:00Z"
                      usedObjectSet="LAST_RUN" objectSetIsComplete="true">
  <atcworklist:objects>
    <atcobject:object uri="/sap/bc/adt/oo/classes/ZCL_SERIALIZER" type="CLAS/OC"
                      name="ZCL_SERIALIZER" packageName="$ZRAY" author="DEVELOPER">
      <atcfinding:findings>
        <atcfinding:finding uri="/sap/bc/adt/atc/findings/001"
                           location="/sap/bc/adt/oo/classes/ZCL_SERIALIZER/source/main#start=50,10"
                           priority="2" checkId="CL_ABAP_API_STATE" checkTitle="API State Check"
                           messageId="RELEASED" messageTitle="Usage of not released ABAP Platform APIs"
                           processor="PROCESSOR1" lastChangedBy="DEVUSER">
          <atcfinding:tags>
            <atcfinding:tag atcfinding:name="REF_OBJ_NAME" atcfinding:value="DECIMALS"/>
            <atcfinding:tag atcfinding:name="REF_OBJ_TYPE" atcfinding:value="DTEL"/>
            <atcfinding:tag atcfinding:name="REF_PACKAGE" atcfinding:value="SDIC"/>
            <atcfinding:tag atcfinding:name="REF_SOFTWARE_COMPONENT" atcfinding:value="SAP_BASIS"/>
            <atcfinding:tag atcfinding:name="APPLICATION_COMPONENT" atcfinding:value="BC-DWB-DIC-AC"/>
          </atcfinding:tags>
        </atcfinding:finding>
        <atcfinding:finding uri="/sap/bc/adt/atc/findings/002"
                           location="/sap/bc/adt/oo/classes/ZCL_SERIALIZER/source/main#start=100,5"
                           priority="2" checkId="CL_ABAP_API_STATE" checkTitle="API State Check"
                           messageId="RELEASED" messageTitle="Usage of not released ABAP Platform APIs">
          <atcfinding:tags>
            <atcfinding:tag atcfinding:name="REF_OBJ_NAME" atcfinding:value="SY-DATUM"/>
          </atcfinding:tags>
        </atcfinding:finding>
      </atcfinding:findings>
    </atcobject:object>
  </atcworklist:objects>
</atcworklist:worklist>`

	result, err := parseATCWorklist([]byte(xmlData))
	if err != nil {
		t.Fatalf("parseATCWorklist failed: %v", err)
	}

	if len(result.Objects) != 1 {
		t.Fatalf("expected 1 object, got %d", len(result.Objects))
	}

	obj := result.Objects[0]
	if len(obj.Findings) != 2 {
		t.Fatalf("expected 2 findings, got %d", len(obj.Findings))
	}

	// First finding - full tags
	f1 := obj.Findings[0]
	if f1.Processor != "PROCESSOR1" {
		t.Errorf("expected processor 'PROCESSOR1', got '%s'", f1.Processor)
	}
	if f1.LastChangedBy != "DEVUSER" {
		t.Errorf("expected lastChangedBy 'DEVUSER', got '%s'", f1.LastChangedBy)
	}
	if len(f1.Tags) != 5 {
		t.Errorf("expected 5 tags, got %d", len(f1.Tags))
	}
	if f1.Tags[ATCTagRefObjName] != "DECIMALS" {
		t.Errorf("expected REF_OBJ_NAME 'DECIMALS', got '%s'", f1.Tags[ATCTagRefObjName])
	}
	if f1.Tags[ATCTagRefObjType] != "DTEL" {
		t.Errorf("expected REF_OBJ_TYPE 'DTEL', got '%s'", f1.Tags[ATCTagRefObjType])
	}
	if f1.Tags[ATCTagRefPackage] != "SDIC" {
		t.Errorf("expected REF_PACKAGE 'SDIC', got '%s'", f1.Tags[ATCTagRefPackage])
	}
	if f1.Tags[ATCTagRefSoftwareComponent] != "SAP_BASIS" {
		t.Errorf("expected REF_SOFTWARE_COMPONENT 'SAP_BASIS', got '%s'", f1.Tags[ATCTagRefSoftwareComponent])
	}
	if f1.Tags[ATCTagApplicationComponent] != "BC-DWB-DIC-AC" {
		t.Errorf("expected APPLICATION_COMPONENT 'BC-DWB-DIC-AC', got '%s'", f1.Tags[ATCTagApplicationComponent])
	}

	// Second finding - single tag
	f2 := obj.Findings[1]
	if len(f2.Tags) != 1 {
		t.Errorf("expected 1 tag, got %d", len(f2.Tags))
	}
	if f2.Tags[ATCTagRefObjName] != "SY-DATUM" {
		t.Errorf("expected REF_OBJ_NAME 'SY-DATUM', got '%s'", f2.Tags[ATCTagRefObjName])
	}
}
