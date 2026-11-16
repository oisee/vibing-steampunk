package adt

import (
	"context"
	"encoding/xml"
	"io"
	"net/http"
	"strings"
	"sync"
	"testing"
)

func TestMessageClassCreateBody(t *testing.T) {
	body := buildCreateObjectBody(CreateObjectOptions{
		ObjectType: ObjectTypeMessageClass, Name: "zdemo", Description: "Demo & more",
		PackageName: "$tmp", MasterLanguage: "de",
	}, objectTypes[ObjectTypeMessageClass], "DEVELOPER")
	for _, want := range []string{
		`<mc:messageClass xmlns:mc="http://www.sap.com/adt/MessageClass"`,
		`adtcore:name="ZDEMO"`, `adtcore:type="MSAG/N"`,
		`adtcore:masterLanguage="DE"`, `adtcore:description="Demo &amp; more"`,
		`<adtcore:packageRef adtcore:name="$TMP"/>`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("body lacks %s:\n%s", want, body)
		}
	}
	if GetObjectURL(ObjectTypeMessageClass, "ZDEMO", "") != "/sap/bc/adt/messageclass/zdemo" {
		t.Error("object URL")
	}
}

// Without adtcore:language the messages were stored with an empty SPRSL:
// written, reported as written, and never found at runtime.
func TestMessageClassWriteCarriesTheLanguage(t *testing.T) {
	b, err := xml.Marshal(messageClassWrite{XMLNSmc: "m", XMLNSadtcore: "a", Name: "ZDEMO", Language: "DE",
		Messages: []messageWrite{{Number: "001", Text: "Text"}}})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(b), `adtcore:language="DE"`) {
		t.Errorf("no language in %s", b)
	}
}

// The same through WriteMessageClassTexts: the PUT body carries the language
// asked for (uppercased), not the struct default.
func TestWriteMessageClassTexts_PutBodyCarriesTheLanguage(t *testing.T) {
	var mu sync.Mutex
	var putBody string
	client := newStubbedClient(t, &adtRecorder{}, func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPut && strings.Contains(r.URL.Path, "/messageclass/") {
			b, _ := io.ReadAll(r.Body)
			mu.Lock()
			putBody = string(b)
			mu.Unlock()
		}
		// The class is read before the PUT, for its short text and package.
		if r.Method == http.MethodGet && strings.Contains(r.URL.Path, "/messageclass/") {
			_, _ = io.WriteString(w, `<mc:messageClass xmlns:mc="http://www.sap.com/adt/MessageClass" xmlns:adtcore="http://www.sap.com/adt/core" adtcore:name="ZDEMO_MC" adtcore:description="demo" adtcore:masterLanguage="DE"><adtcore:packageRef adtcore:name="$TMP"/></mc:messageClass>`)
			return
		}
		w.WriteHeader(http.StatusOK)
	})
	if err := client.WriteMessageClassTexts(context.Background(), "ZDEMO_MC", "de",
		[]MessageClassMessage{{Number: "001", Text: "Hallo"}}, "HANDLE", ""); err != nil {
		t.Fatalf("WriteMessageClassTexts: %v", err)
	}
	mu.Lock()
	defer mu.Unlock()
	if putBody == "" {
		t.Fatal("no PUT of the message class reached the server")
	}
	if !strings.Contains(putBody, `adtcore:language="DE"`) {
		t.Errorf("PUT body has no adtcore:language=\"DE\":\n%s", putBody)
	}
}
