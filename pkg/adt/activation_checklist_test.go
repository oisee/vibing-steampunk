package adt

import (
	"strings"
	"testing"
)

// Three ways a refused activation still reads as a success, all of them the same
// bug as #136's headline: SAP said no, in the body, and the parser threw the
// sentence away. The namespace premise in the issue is false — encoding/xml
// matches `adtcore:type="E"` against `xml:"type,attr"` — and the root-element
// half is already fixed. These are what is left.

//  1. The refusal arrives as a list of objects that are still inactive, with
//     <ioc:inactiveObjects> as the document ROOT. The parser understood the
//     wrapped form only, so the whole payload decoded to nothing and an object
//     that never activated was reported as activated.
const activationInactiveRoot = `<?xml version="1.0" encoding="utf-8"?>
<ioc:inactiveObjects xmlns:ioc="http://www.sap.com/abapxml/inactiveCtsObjects" xmlns:adtcore="http://www.sap.com/adt/core">
  <ioc:entry>
    <ioc:object>
      <ioc:ref adtcore:uri="/sap/bc/adt/programs/programs/zdemo_still_inactive" adtcore:type="PROG/P" adtcore:name="ZDEMO_STILL_INACTIVE"/>
    </ioc:object>
  </ioc:entry>
</ioc:inactiveObjects>`

//  2. SAP splits one message over several <txt> siblings. A string field takes
//     each in turn and keeps the last, so the error itself is dropped and the
//     hint that follows it is reported as the error.
const activationMultiTxt = `<?xml version="1.0" encoding="utf-8"?>
<chkl:messages xmlns:chkl="http://www.sap.com/abapxml/checklist">
  <msg objDescr="Class ZCL_DEMO" type="E" line="12" href="/sap/bc/adt/oo/classes/zcl_demo/source/main#start=12,4" forceSupported="false">
    <shortText><txt>Field "FOO" is unknown.</txt><txt>"Editing canceled" (EU 202)</txt></shortText>
  </msg>
</chkl:messages>`

//  3. The checklist's own verdict. Nothing in it is error-typed — the only
//     message is a W — but <chkl:properties activationExecuted="false"/> says the
//     activation never ran, and the object is inactive.
const activationNotExecuted = `<?xml version="1.0" encoding="utf-8"?>
<chkl:messages xmlns:chkl="http://www.sap.com/abapxml/checklist">
  <chkl:properties checkExecuted="true" activationExecuted="false" generationExecuted="false"/>
  <msg objDescr="" type="W" line="0" href="" forceSupported="true">
    <shortText><txt>Activation was cancelled.</txt></shortText>
  </msg>
</chkl:messages>`

// An activation that really ran is not a refusal, and the attribute must not
// turn every warning into a failure.
const activationExecutedWithWarning = `<?xml version="1.0" encoding="utf-8"?>
<chkl:messages xmlns:chkl="http://www.sap.com/abapxml/checklist">
  <chkl:properties checkExecuted="true" activationExecuted="true" generationExecuted="true"/>
  <msg objDescr="Class ZCL_DEMO" type="W" line="3" href="x" forceSupported="true">
    <shortText><txt>Variable is never used</txt></shortText>
  </msg>
</chkl:messages>`

func TestParseActivationResultInactiveObjectsAsRoot(t *testing.T) {
	res, err := parseActivationResult([]byte(activationInactiveRoot))
	if err != nil {
		t.Fatalf("parseActivationResult: %v", err)
	}
	if res.Success {
		t.Fatal("an object SAP listed as still inactive was reported as activated")
	}
	if len(res.Inactive) != 1 {
		t.Fatalf("inactive = %d, want 1: %+v", len(res.Inactive), res.Inactive)
	}
	got := res.Inactive[0]
	if got.Name != "ZDEMO_STILL_INACTIVE" || got.Type != "PROG/P" {
		t.Fatalf("inactive object not parsed: %+v", got)
	}
	if got.URI != "/sap/bc/adt/programs/programs/zdemo_still_inactive" {
		t.Fatalf("uri lost: %q", got.URI)
	}
}

func TestParseActivationResultKeepsEveryTxt(t *testing.T) {
	res, err := parseActivationResult([]byte(activationMultiTxt))
	if err != nil {
		t.Fatalf("parseActivationResult: %v", err)
	}
	if len(res.Messages) != 1 {
		t.Fatalf("messages = %d, want 1", len(res.Messages))
	}
	text := res.Messages[0].ShortText
	// The error is the half that was being dropped, so assert on it by name.
	if !strings.Contains(text, `Field "FOO" is unknown.`) {
		t.Fatalf("SAP's error text was dropped, kept only: %q", text)
	}
	if !strings.Contains(text, "EU 202") {
		t.Fatalf("the trailing txt was dropped: %q", text)
	}
}

func TestParseActivationResultHonoursActivationExecutedFalse(t *testing.T) {
	res, err := parseActivationResult([]byte(activationNotExecuted))
	if err != nil {
		t.Fatalf("parseActivationResult: %v", err)
	}
	if res.Success {
		t.Fatal(`activationExecuted="false" means the object is still inactive, but it was reported as a success`)
	}
	// And the reason has to survive to whoever ran the deploy. Before this, a
	// refusal with no error-typed message rendered as "SAP named no reason"
	// while the W beside it held the reason.
	lines := res.ProblemLines()
	if len(lines) != 1 || !strings.Contains(lines[0], "Activation was cancelled.") {
		t.Fatalf("SAP's reason did not reach the caller: %v", lines)
	}
}

func TestParseActivationResultActivationExecutedTrueIsNotAFailure(t *testing.T) {
	res, err := parseActivationResult([]byte(activationExecutedWithWarning))
	if err != nil {
		t.Fatalf("parseActivationResult: %v", err)
	}
	if !res.Success {
		t.Fatal("an activation that ran, with only a warning, is not a refusal")
	}
	if len(res.ProblemLines()) != 0 {
		t.Fatalf("a success has no problem lines: %v", res.ProblemLines())
	}
}

func TestActivationResultError(t *testing.T) {
	if err := ActivationResultError(&ActivationResult{Success: true}); err != nil {
		t.Fatalf("success returned error: %v", err)
	}
	if err := ActivationResultError(nil); err == nil || !strings.Contains(err.Error(), "no result") {
		t.Fatalf("nil result error = %v", err)
	}
	refused := &ActivationResult{Messages: []ActivationResultMessage{{Type: "E", ShortText: "synthetic activation failure"}}}
	if err := ActivationResultError(refused); err == nil || !strings.Contains(err.Error(), "synthetic activation failure") {
		t.Fatalf("logical failure error = %v", err)
	}
}

// The wrapped shape still has to work. PR #148 fixes the root form by declaring
// <msg> directly on the response struct and nothing else, which stops matching
// the wrapper — this is the test that catches that.
func TestParseActivationResultStillHandlesBothRootAndWrapped(t *testing.T) {
	for name, body := range map[string]string{
		"root":    activationFailedRoot,
		"wrapped": activationFailedWrapped,
	} {
		t.Run(name, func(t *testing.T) {
			res, err := parseActivationResult([]byte(body))
			if err != nil {
				t.Fatalf("parseActivationResult: %v", err)
			}
			if res.Success || len(res.Messages) != 1 {
				t.Fatalf("shape %s regressed: %+v", name, res)
			}
			if res.Messages[0].ShortText != `Field "FOO" is unknown` {
				t.Fatalf("text lost: %q", res.Messages[0].ShortText)
			}
		})
	}
}

// Some releases put the text straight in <shortText> with no <txt> at all.
// Either way the caller must not be handed an empty string.
func TestParseActivationResultReadsShortTextWithoutTxt(t *testing.T) {
	body := `<?xml version="1.0" encoding="utf-8"?>
<chkl:messages xmlns:chkl="http://www.sap.com/abapxml/checklist">
  <msg objDescr="Class ZCL_DEMO" type="E" line="4" href="x"><shortText>Field "BAR" is unknown</shortText></msg>
</chkl:messages>`
	res, err := parseActivationResult([]byte(body))
	if err != nil {
		t.Fatalf("parseActivationResult: %v", err)
	}
	if res.Success {
		t.Fatal("a type=E message is a refusal")
	}
	if len(res.Messages) != 1 || res.Messages[0].ShortText != `Field "BAR" is unknown` {
		t.Fatalf("shortText without <txt> was dropped: %+v", res.Messages)
	}
}
