package adt

import (
	"context"
	"net/http"
	"strings"
	"testing"
)

func TestGetObjectTextsInLanguage(t *testing.T) {
	mock := &mockTransportClient{
		responses: map[string]*http.Response{
			"/sap/bc/adt/programs/programs/ZTEST/source/main": newTestResponse("REPORT ztest.\nWRITE 'Bonjour'."),
			"discovery": newTestResponse("OK"),
		},
	}

	cfg := NewConfig("https://sap.example.com:44300", "user", "pass")
	transport := NewTransportWithClient(cfg, mock)
	client := NewClientWithTransport(cfg, transport)

	content, err := client.GetObjectTextsInLanguage(context.Background(), "/sap/bc/adt/programs/programs/ZTEST/source/main", "FR")
	if err != nil {
		t.Fatalf("GetObjectTextsInLanguage failed: %v", err)
	}

	if !strings.Contains(content, "Bonjour") {
		t.Errorf("Expected content to contain 'Bonjour', got: %s", content)
	}
}

func TestGetDataElementLabels(t *testing.T) {
	xmlResp := `<?xml version="1.0" encoding="UTF-8"?>
<dataElement shortDescription="Court" mediumDescription="Moyen" longDescription="Long texte" heading="En-tête"/>`

	mock := &mockTransportClient{
		responses: map[string]*http.Response{
			"/sap/bc/adt/ddic/dataelements/ZTEST_DTEL": newTestResponse(xmlResp),
			"discovery": newTestResponse("OK"),
		},
	}

	cfg := NewConfig("https://sap.example.com:44300", "user", "pass")
	transport := NewTransportWithClient(cfg, mock)
	client := NewClientWithTransport(cfg, transport)

	labels, err := client.GetDataElementLabels(context.Background(), "ZTEST_DTEL", "FR")
	if err != nil {
		t.Fatalf("GetDataElementLabels failed: %v", err)
	}

	if labels.Short != "Court" {
		t.Errorf("Short = %v, want Court", labels.Short)
	}
	if labels.Medium != "Moyen" {
		t.Errorf("Medium = %v, want Moyen", labels.Medium)
	}
	if labels.Long != "Long texte" {
		t.Errorf("Long = %v, want Long texte", labels.Long)
	}
	if labels.Heading != "En-tête" {
		t.Errorf("Heading = %v, want En-tête", labels.Heading)
	}
}

func TestGetMessageClassTexts(t *testing.T) {
	xmlResp := `<?xml version="1.0" encoding="UTF-8"?>
<mc:messageclass xmlns:mc="http://www.sap.com/adt/mc" name="ZTEST_MC">
  <mc:messages msgno="001" msgtext="Message un"/>
  <mc:messages msgno="002" msgtext="Message deux"/>
</mc:messageclass>`

	mock := &mockTransportClient{
		responses: map[string]*http.Response{
			"/sap/bc/adt/messageclass/ztest_mc": newTestResponse(xmlResp),
			"discovery":                         newTestResponse("OK"),
		},
	}

	cfg := NewConfig("https://sap.example.com:44300", "user", "pass")
	transport := NewTransportWithClient(cfg, mock)
	client := NewClientWithTransport(cfg, transport)

	texts, err := client.GetMessageClassTexts(context.Background(), "ZTEST_MC", "FR")
	if err != nil {
		t.Fatalf("GetMessageClassTexts failed: %v", err)
	}

	if len(texts) != 2 {
		t.Fatalf("Expected 2 messages, got %d", len(texts))
	}

	if texts[0].Number != "001" {
		t.Errorf("Number = %v, want 001", texts[0].Number)
	}
	if texts[0].Text != "Message un" {
		t.Errorf("Text = %v, want 'Message un'", texts[0].Text)
	}
}

func TestGetTextPoolInLanguage(t *testing.T) {
	xmlResp := `<?xml version="1.0" encoding="UTF-8"?>
<textPool>
  <entry id="I" key="001" entry="Texte un"/>
  <entry id="I" key="002" entry="Texte deux"/>
</textPool>`

	mock := &mockTransportClient{
		responses: map[string]*http.Response{
			"/sap/bc/adt/programs/programs/ZTEST/textelements": newTestResponse(xmlResp),
			"discovery": newTestResponse("OK"),
		},
	}

	cfg := NewConfig("https://sap.example.com:44300", "user", "pass")
	transport := NewTransportWithClient(cfg, mock)
	client := NewClientWithTransport(cfg, transport)

	entries, err := client.GetTextPoolInLanguage(context.Background(), "ZTEST", "FR")
	if err != nil {
		t.Fatalf("GetTextPoolInLanguage failed: %v", err)
	}

	if len(entries) != 2 {
		t.Fatalf("Expected 2 entries, got %d", len(entries))
	}

	if entries[0].Key != "001" {
		t.Errorf("Key = %v, want 001", entries[0].Key)
	}
	if entries[0].Text != "Texte un" {
		t.Errorf("Text = %v, want 'Texte un'", entries[0].Text)
	}
}

func TestCompareObjectLanguages(t *testing.T) {
	// Use a func-based mock that returns different content based on sap-language query param
	mock := &funcMockClient{
		doFunc: func(req *http.Request) (*http.Response, error) {
			if strings.Contains(req.URL.Path, "discovery") {
				return newTestResponse("OK"), nil
			}
			lang := req.URL.Query().Get("sap-language")
			if lang == "FR" {
				return newTestResponse("REPORT ztest.\nWRITE 'Bonjour'."), nil
			}
			return newTestResponse("REPORT ztest.\nWRITE 'Hello'."), nil
		},
	}

	cfg := NewConfig("https://sap.example.com:44300", "user", "pass")
	transport := NewTransportWithClient(cfg, mock)
	client := NewClientWithTransport(cfg, transport)

	comparison, err := client.CompareObjectLanguages(context.Background(),
		"/sap/bc/adt/programs/programs/ZTEST/source/main", "EN", "FR")
	if err != nil {
		t.Fatalf("CompareObjectLanguages failed: %v", err)
	}

	if comparison.SourceLang != "EN" {
		t.Errorf("SourceLang = %v, want EN", comparison.SourceLang)
	}
	if comparison.TargetLang != "FR" {
		t.Errorf("TargetLang = %v, want FR", comparison.TargetLang)
	}

	// Line 2 differs: 'Hello' vs 'Bonjour'
	if len(comparison.Entries) == 0 {
		t.Error("Expected at least 1 diff entry for differing content")
	}
}

// funcMockClient is a mock HTTP client that uses a function for responses.
type funcMockClient struct {
	doFunc func(req *http.Request) (*http.Response, error)
}

func (m *funcMockClient) Do(req *http.Request) (*http.Response, error) {
	return m.doFunc(req)
}

func TestOverrideLanguageInRequest(t *testing.T) {
	mock := &mockTransportClient{
		responses: map[string]*http.Response{
			"/sap/bc/adt/programs/programs/ZTEST/source/main": newTestResponse("OK"),
			"discovery": newTestResponse("OK"),
		},
	}

	cfg := NewConfig("https://sap.example.com:44300", "user", "pass")
	cfg.Language = "EN"
	transport := NewTransportWithClient(cfg, mock)
	client := NewClientWithTransport(cfg, transport)

	_, err := client.GetObjectTextsInLanguage(context.Background(), "/sap/bc/adt/programs/programs/ZTEST/source/main", "FR")
	if err != nil {
		t.Fatalf("GetObjectTextsInLanguage failed: %v", err)
	}

	// Verify that the request URL contains sap-language=FR (override)
	if len(mock.requests) < 1 {
		t.Fatal("Expected at least 1 request")
	}

	lastReq := mock.requests[len(mock.requests)-1]
	sapLang := lastReq.URL.Query().Get("sap-language")
	if sapLang != "FR" {
		t.Errorf("Expected sap-language=FR in URL, got sap-language=%s (full URL: %s)", sapLang, lastReq.URL.String())
	}
}
