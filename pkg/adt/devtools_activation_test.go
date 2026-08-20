package adt

import (
	"strings"
	"testing"
)

// Real response captured from POST /sap/bc/adt/activation?method=activate&preauditRequested=true
// for a program containing a syntax error. Note the ROOT element is <chkl:messages>.
const activationSyntaxErrorPayload = `<?xml version="1.0" encoding="utf-8"?><chkl:messages xmlns:chkl="http://www.sap.com/abapxml/checklist"><msg objDescr="Program Z_VSP_ACT_TEST" type="E" line="1" href="/sap/bc/adt/programs/programs/z_vsp_act_test/source/main#start=2,6" forceSupported="true"><shortText><txt>Field "LV_UNDEFINED_VARIABLE" is unknown.</txt></shortText><atom:link href="art.syntax:GTU" rel="http://www.sap.com/adt/categories/quickfixes" xmlns:atom="http://www.w3.org/2005/Atom"/></msg></chkl:messages>`

func TestParseActivationResultSyntaxError(t *testing.T) {
	res, err := parseActivationResult([]byte(activationSyntaxErrorPayload))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.Success {
		t.Fatal("expected Success=false for activation response with type=E message")
	}
	if len(res.Messages) != 1 {
		t.Fatalf("expected 1 message, got %d", len(res.Messages))
	}
	if res.Messages[0].Type != "E" {
		t.Fatalf("expected type E, got %q", res.Messages[0].Type)
	}
	if !strings.Contains(res.Messages[0].ShortText, "LV_UNDEFINED_VARIABLE") {
		t.Fatalf("unexpected message text: %q", res.Messages[0].ShortText)
	}
}

func TestParseActivationResultEmptyBodySuccess(t *testing.T) {
	res, err := parseActivationResult(nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !res.Success {
		t.Fatal("expected Success=true for empty body")
	}
}

func TestParseActivationResultInactiveObjects(t *testing.T) {
	payload := `<?xml version="1.0" encoding="utf-8"?><ioc:inactiveObjects xmlns:ioc="http://www.sap.com/abapxml/inactiveCtsObjects" xmlns:adtcore="http://www.sap.com/adt/core"><ioc:entry><ioc:object><ioc:ref adtcore:uri="/sap/bc/adt/programs/programs/ztest" adtcore:type="PROG/P" adtcore:name="ZTEST"/></ioc:object><ioc:transport/></ioc:entry></ioc:inactiveObjects>`
	res, err := parseActivationResult([]byte(payload))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.Success {
		t.Fatal("expected Success=false when inactive objects are returned")
	}
	if len(res.Inactive) != 1 || res.Inactive[0].Name != "ZTEST" {
		t.Fatalf("unexpected inactive list: %+v", res.Inactive)
	}
}
