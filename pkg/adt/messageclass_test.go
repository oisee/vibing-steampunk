package adt

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"
)

const testMessageClassXML = `<?xml version="1.0" encoding="utf-8"?><mc:messageClass adtcore:masterLanguage="RU" adtcore:name="ZDEMO_MC" adtcore:type="MSAG/N" adtcore:description="Demo messages" adtcore:language="RU" xmlns:mc="http://www.sap.com/adt/MessageClass" xmlns:adtcore="http://www.sap.com/adt/core">` +
	`<adtcore:packageRef adtcore:uri="/sap/bc/adt/vit/wb/object_type/devck/object_name/ZDEMO" adtcore:type="DEVC/K" adtcore:name="ZDEMO"/>` +
	`<mc:messages mc:msgno="001" mc:msgtext="First" mc:selfexplainatory="true" mc:documented="false" adtcore:name=""/>` +
	`<mc:messages mc:msgno="002" mc:msgtext="Second" mc:selfexplainatory="true" mc:documented="false" adtcore:name=""/>` +
	`</mc:messageClass>`

// The PUT takes the language of the texts, the package and the short text
// from the body. Without them SAP answered 200, wrote T100 under an empty
// language and blanked the short text (7.50).
func TestMessageClassBodyCarriesWhatThePutReads(t *testing.T) {
	cur := &MessageClass{Name: "zdemo_mc", Description: "Demo & messages", Package: "ZDEMO", MasterLanguage: "RU"}
	body, err := messageClassBody(cur, "ru", []MessageClassMessage{{Number: "003", Text: "Third <&1>"}})
	if err != nil {
		t.Fatal(err)
	}
	s := string(body)
	for _, want := range []string{
		`adtcore:name="ZDEMO_MC"`,
		`adtcore:language="RU"`,
		`adtcore:masterLanguage="RU"`, // 7.40 saves nothing without it
		`adtcore:description="Demo &amp; messages"`,
		`<adtcore:packageRef adtcore:name="ZDEMO">`,
		`<mc:messages mc:msgno="003" mc:msgtext="Third &lt;&amp;1&gt;">`,
	} {
		if !strings.Contains(s, want) {
			t.Errorf("body lacks %s:\n%s", want, s)
		}
	}
}

func TestReadMessageClassKeepsPackageAndLanguages(t *testing.T) {
	rec := &adtRecorder{}
	client := newStubbedClient(t, rec, func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, testMessageClassXML)
	})
	mc, err := client.GetMessageClass(context.Background(), "ZDEMO_MC")
	if err != nil {
		t.Fatal(err)
	}
	if mc.Package != "ZDEMO" || mc.MasterLanguage != "RU" || mc.Description != "Demo messages" || len(mc.Messages) != 2 {
		t.Errorf("read %+v", mc)
	}
}
