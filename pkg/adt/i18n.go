package adt

import (
	"context"
	"encoding/xml"
	"fmt"
	"net/http"
	"net/url"
	"strings"
)

// --- i18n Types ---

// DataElementLabels holds the text labels of a data element in a specific language.
type DataElementLabels struct {
	Short   string `json:"short" xml:"shortDescription,attr"`
	Medium  string `json:"medium" xml:"mediumDescription,attr"`
	Long    string `json:"long" xml:"longDescription,attr"`
	Heading string `json:"heading" xml:"heading,attr"`
}

// TextPoolEntry represents a single text pool entry (text element/symbol) of a program.
type TextPoolEntry struct {
	ID   string `json:"id" xml:"id,attr"`
	Key  string `json:"key" xml:"key,attr"`
	Text string `json:"text" xml:"entry,attr"`
}

// LanguageComparison holds the result of comparing an object's texts in two languages.
type LanguageComparison struct {
	SourceLang string            `json:"sourceLang"`
	TargetLang string            `json:"targetLang"`
	Entries    []ComparisonEntry `json:"entries"`
}

// ComparisonEntry represents a single text key compared between two languages.
type ComparisonEntry struct {
	Key        string `json:"key"`
	SourceText string `json:"sourceText"`
	TargetText string `json:"targetText"`
	Missing    bool   `json:"missing"`
}

// --- i18n Methods ---

// GetObjectTextsInLanguage retrieves the source/content of an object in a specific language.
// objectSourceURL is the ADT source URL (e.g., /sap/bc/adt/programs/programs/ZTEST/source/main).
func (c *Client) GetObjectTextsInLanguage(ctx context.Context, objectSourceURL, lang string) (string, error) {
	if err := c.checkSafety(OpRead, "GetObjectTextsInLanguage"); err != nil {
		return "", err
	}

	lang = strings.ToUpper(lang)

	resp, err := c.transport.Request(ctx, objectSourceURL, &RequestOptions{
		Method:           http.MethodGet,
		OverrideLanguage: lang,
	})
	if err != nil {
		return "", fmt.Errorf("get object texts in language %s: %w", lang, err)
	}

	return string(resp.Body), nil
}

// GetDataElementLabels retrieves the text labels of a data element in a specific language.
func (c *Client) GetDataElementLabels(ctx context.Context, name, lang string) (*DataElementLabels, error) {
	if err := c.checkSafety(OpRead, "GetDataElementLabels"); err != nil {
		return nil, err
	}

	name = strings.ToUpper(name)
	lang = strings.ToUpper(lang)

	path := fmt.Sprintf("/sap/bc/adt/ddic/dataelements/%s", url.PathEscape(name))
	resp, err := c.transport.Request(ctx, path, &RequestOptions{
		Method:           http.MethodGet,
		Accept:           "application/xml",
		OverrideLanguage: lang,
	})
	if err != nil {
		return nil, fmt.Errorf("get data element labels: %w", err)
	}

	// Parse the XML - data element labels are attributes on the root element
	var labels DataElementLabels
	if err := xml.Unmarshal(resp.Body, &labels); err != nil {
		return nil, fmt.Errorf("parse data element labels: %w", err)
	}

	return &labels, nil
}

// GetMessageClassTexts retrieves all messages of a message class in a specific language.
func (c *Client) GetMessageClassTexts(ctx context.Context, name, lang string) ([]MessageClassMessage, error) {
	if err := c.checkSafety(OpRead, "GetMessageClassTexts"); err != nil {
		return nil, err
	}

	name = strings.ToUpper(name)
	lang = strings.ToUpper(lang)

	path := fmt.Sprintf("/sap/bc/adt/messageclass/%s", url.PathEscape(strings.ToLower(name)))
	resp, err := c.transport.Request(ctx, path, &RequestOptions{
		Method:           http.MethodGet,
		Accept:           "application/vnd.sap.adt.mc.messageclass+xml",
		OverrideLanguage: lang,
	})
	if err != nil {
		return nil, fmt.Errorf("get message class texts: %w", err)
	}

	var mc MessageClass
	if err := xml.Unmarshal(resp.Body, &mc); err != nil {
		return nil, fmt.Errorf("parse message class XML: %w", err)
	}

	return mc.Messages, nil
}

// WriteMessageClassTexts updates message class texts in a specific language.
// Requires a lock handle from LockObject and optionally a transport request number.
func (c *Client) WriteMessageClassTexts(ctx context.Context, name, lang string, texts []MessageClassMessage, lockHandle, transport string) error {
	if err := c.checkSafety(OpUpdate, "WriteMessageClassTexts"); err != nil {
		return err
	}

	name = strings.ToUpper(name)
	lang = strings.ToUpper(lang)

	// Build XML body
	mc := MessageClass{
		Name:     name,
		Messages: texts,
	}
	body, err := xml.Marshal(mc)
	if err != nil {
		return fmt.Errorf("marshal message class XML: %w", err)
	}

	path := fmt.Sprintf("/sap/bc/adt/messageclass/%s", url.PathEscape(strings.ToLower(name)))

	params := url.Values{}
	params.Set("lockHandle", lockHandle)
	if transport != "" {
		params.Set("corrNr", transport)
	}

	_, err = c.transport.Request(ctx, path, &RequestOptions{
		Method:           http.MethodPut,
		Query:            params,
		Body:             body,
		ContentType:      "application/vnd.sap.adt.mc.messageclass+xml",
		OverrideLanguage: lang,
	})
	if err != nil {
		return fmt.Errorf("write message class texts: %w", err)
	}

	return nil
}

// WriteDataElementLabels updates data element labels in a specific language.
// Requires a lock handle from LockObject and optionally a transport request number.
func (c *Client) WriteDataElementLabels(ctx context.Context, name, lang string, labels *DataElementLabels, lockHandle, transport string) error {
	if err := c.checkSafety(OpUpdate, "WriteDataElementLabels"); err != nil {
		return err
	}

	name = strings.ToUpper(name)
	lang = strings.ToUpper(lang)

	body, err := xml.Marshal(labels)
	if err != nil {
		return fmt.Errorf("marshal data element labels: %w", err)
	}

	path := fmt.Sprintf("/sap/bc/adt/ddic/dataelements/%s", url.PathEscape(name))

	params := url.Values{}
	params.Set("lockHandle", lockHandle)
	if transport != "" {
		params.Set("corrNr", transport)
	}

	_, err = c.transport.Request(ctx, path, &RequestOptions{
		Method:           http.MethodPut,
		Query:            params,
		Body:             body,
		ContentType:      "application/xml",
		OverrideLanguage: lang,
	})
	if err != nil {
		return fmt.Errorf("write data element labels: %w", err)
	}

	return nil
}

// GetTextPoolInLanguage retrieves the text pool (text elements/symbols) of a program in a specific language.
func (c *Client) GetTextPoolInLanguage(ctx context.Context, programName, lang string) ([]TextPoolEntry, error) {
	if err := c.checkSafety(OpRead, "GetTextPoolInLanguage"); err != nil {
		return nil, err
	}

	programName = strings.ToUpper(programName)
	lang = strings.ToUpper(lang)

	path := fmt.Sprintf("/sap/bc/adt/programs/programs/%s/textelements", url.PathEscape(programName))
	resp, err := c.transport.Request(ctx, path, &RequestOptions{
		Method:           http.MethodGet,
		Accept:           "application/xml",
		OverrideLanguage: lang,
	})
	if err != nil {
		return nil, fmt.Errorf("get text pool: %w", err)
	}

	type textPool struct {
		Entries []TextPoolEntry `xml:"entry"`
	}
	var tp textPool
	if err := xml.Unmarshal(resp.Body, &tp); err != nil {
		return nil, fmt.Errorf("parse text pool XML: %w", err)
	}

	return tp.Entries, nil
}

// CompareObjectLanguages compares the text content of an object in two languages.
// Returns a comparison showing which texts differ or are missing in the target language.
func (c *Client) CompareObjectLanguages(ctx context.Context, objectSourceURL, sourceLang, targetLang string) (*LanguageComparison, error) {
	if err := c.checkSafety(OpRead, "CompareObjectLanguages"); err != nil {
		return nil, err
	}

	// Get source language content
	sourceContent, err := c.GetObjectTextsInLanguage(ctx, objectSourceURL, sourceLang)
	if err != nil {
		return nil, fmt.Errorf("get source language (%s): %w", sourceLang, err)
	}

	// Get target language content
	targetContent, err := c.GetObjectTextsInLanguage(ctx, objectSourceURL, targetLang)
	if err != nil {
		return nil, fmt.Errorf("get target language (%s): %w", targetLang, err)
	}

	// Build comparison by splitting into lines
	sourceLines := strings.Split(sourceContent, "\n")
	targetLines := strings.Split(targetContent, "\n")

	// Build target map for lookup
	targetMap := make(map[int]string)
	for i, line := range targetLines {
		targetMap[i] = line
	}

	comparison := &LanguageComparison{
		SourceLang: strings.ToUpper(sourceLang),
		TargetLang: strings.ToUpper(targetLang),
	}

	for i, sourceLine := range sourceLines {
		entry := ComparisonEntry{
			Key:        fmt.Sprintf("line-%d", i+1),
			SourceText: sourceLine,
		}
		if targetLine, ok := targetMap[i]; ok {
			entry.TargetText = targetLine
			entry.Missing = false
		} else {
			entry.Missing = true
		}
		if entry.SourceText != entry.TargetText || entry.Missing {
			comparison.Entries = append(comparison.Entries, entry)
		}
	}

	return comparison, nil
}
