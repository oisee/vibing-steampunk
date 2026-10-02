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

// DataElementLabels holds the text labels of a data element in a specific
// language.
//
// The xml tags this carried — shortDescription, mediumDescription,
// longDescription and heading, all as attributes of the root element —
// described a response that does not exist. Checked against a live 7.58: the
// labels are child elements of dtel:dataElement, named shortFieldLabel,
// mediumFieldLabel, longFieldLabel and headingFieldLabel. Nothing had ever
// matched, and nothing could: the request that would have carried the answer
// was refused with 406 before any parsing happened.
//
// The tags are gone rather than corrected, because this is the type a caller
// holds and the wire format is not its business. dataElementDoc below is the
// wire format, and it is the only thing that knows the element names.
type DataElementLabels struct {
	Short   string `json:"short"`
	Medium  string `json:"medium"`
	Long    string `json:"long"`
	Heading string `json:"heading"`
}

// dataElementDoc is the representation ADT actually serves for a data element.
//
// Only the four labels are mapped. The document also carries the domain, the
// type, the field lengths and a dozen flags, and mapping those would be
// inventing a feature under cover of a bug fix.
type dataElementDoc struct {
	XMLName     xml.Name `xml:"wbobj"`
	DataElement struct {
		Short   string `xml:"shortFieldLabel"`
		Medium  string `xml:"mediumFieldLabel"`
		Long    string `xml:"longFieldLabel"`
		Heading string `xml:"headingFieldLabel"`
	} `xml:"dataElement"`
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
		Method: http.MethodGet,
		// The versioned vocabulary type, not application/xml. The generic one
		// is refused with 406 "The message content is not acceptable" on every
		// name, namespaced or not — so this call had never returned a label to
		// anybody. Checked against 7.58 with both a plain and a namespaced
		// element, because a 406 on one name and a 406 on all names are
		// different bugs.
		Accept:           "application/vnd.sap.adt.dataelements.v2+xml",
		OverrideLanguage: lang,
	})
	if err != nil {
		return nil, fmt.Errorf("get data element labels: %w", err)
	}

	var doc dataElementDoc
	if err := xml.Unmarshal(resp.Body, &doc); err != nil {
		return nil, fmt.Errorf("parse data element labels: %w", err)
	}

	// An element with no translation in the requested language answers in its
	// master language rather than empty, so a caller cannot read "these are the
	// English labels" out of a successful call. That is ADT's behaviour and not
	// something to paper over here.
	return &DataElementLabels{
		Short:   doc.DataElement.Short,
		Medium:  doc.DataElement.Medium,
		Long:    doc.DataElement.Long,
		Heading: doc.DataElement.Heading,
	}, nil
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

// WriteMessageClassTexts writes messages of a message class in one language,
// adding the numbers that do not exist yet; messages not named stay as they
// are. With lockHandle empty it takes and releases the lock itself, after the
// package gate -- the gate's lookup inside a lock window would retire the
// session the lock belongs to (#91).
func (c *Client) WriteMessageClassTexts(ctx context.Context, name, lang string, texts []MessageClassMessage, lockHandle, transport string) (err error) {
	name = strings.ToUpper(name)
	lang = strings.ToUpper(lang)
	objectURL := fmt.Sprintf("/sap/bc/adt/messageclass/%s", url.PathEscape(strings.ToLower(name)))

	// Unified mutation policy gate (op type + package + transport)
	ctx, err = c.gateAndMark(ctx, MutationContext{
		Op:        OpUpdate,
		OpName:    "WriteMessageClassTexts",
		ObjectURL: objectURL,
		Transport: transport,
		// The caller's handle, if any: the gate then runs after that LOCK and
		// its lookup joins the lock's session. Empty when this call locks
		// itself, below the gate.
		LockHandle: lockHandle,
	})
	if err != nil {
		return err
	}
	if lockHandle == "" {
		lock, lerr := c.LockObject(ctx, objectURL, "MODIFY")
		if lerr != nil {
			return fmt.Errorf("locking message class %s: %w", name, lerr)
		}
		// The lock this call took is released on a context of its own -- a
		// write that failed because ctx was cancelled must not strand it --
		// and a release that fails is reported, also after a good write.
		defer func() {
			if uerr := c.releaseLockAfterFailure(ctx, objectURL, lock.LockHandle); uerr != nil {
				if err == nil {
					err = fmt.Errorf("message class %s written, but %s", name, strandedLockAdvice(objectURL, uerr))
				} else {
					err = fmt.Errorf("%w; %s", err, strandedLockAdvice(objectURL, uerr))
				}
			}
		}()
		lockHandle = lock.LockHandle
	}

	// Build the XML body in the shape ADT actually serves and expects: a
	// namespaced <mc:messageClass> root whose messages carry mc:msgno/mc:msgtext
	// as ATTRIBUTES. MessageClass itself is the read model — Go's encoding/xml
	// cannot express prefixed names for writing and namespace-agnostic matching
	// for reading in one struct — so the request has its own type.
	mc := messageClassWrite{
		XMLNSmc:      "http://www.sap.com/adt/MessageClass",
		XMLNSadtcore: "http://www.sap.com/adt/core",
		Name:         name,
		Language:     lang,
	}
	for _, t := range texts {
		mc.Messages = append(mc.Messages, messageWrite{Number: t.Number, Text: t.Text})
	}
	body, err := xml.Marshal(mc)
	if err != nil {
		return fmt.Errorf("marshal message class XML: %w", err)
	}
	body = append([]byte(xml.Header), body...)

	path := objectURL

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
		// The lockHandle in the query above came from a stateful LOCK and is
		// bound to that session. Without this the PUT that consumes it went out
		// explicitly stateless and could never match its own lock — the same
		// defect as CreateTable's source PUT (issue #91).
		Stateful: true,
	})
	if err != nil {
		return fmt.Errorf("write message class texts: %w", err)
	}

	return nil
}

// WriteDataElementLabels updates data element labels in a specific language.
// Requires a lock handle from LockObject and optionally a transport request number.
func (c *Client) WriteDataElementLabels(ctx context.Context, name, lang string, labels *DataElementLabels, lockHandle, transport string) error {
	name = strings.ToUpper(name)
	lang = strings.ToUpper(lang)

	// Unified mutation policy gate (op type + package + transport)
	if err := c.checkMutation(ctx, MutationContext{
		Op:         OpUpdate,
		OpName:     "WriteDataElementLabels",
		ObjectURL:  fmt.Sprintf("/sap/bc/adt/ddic/dataelements/%s", url.PathEscape(name)),
		Transport:  transport,
		LockHandle: lockHandle,
	}); err != nil {
		return err
	}

	// This marshalled the read model straight to XML and PUT it as
	// application/xml. Neither half of that is the resource: what ADT serves
	// and takes at this address is a blue:wbobj document carrying the whole
	// data element — domain, type, lengths, a dozen flags — with the labels as
	// dtel:*FieldLabel children, under the versioned vocabulary type. A
	// four-field document is not a subset of it; it is a different document,
	// and a PUT of one would either be rejected or, worse, accepted as a
	// replacement for everything the element has.
	//
	// The reader beside this was broken in exactly the same way and could be
	// fixed, because reading is verifiable against a live system without
	// changing it. This cannot: the only way to know what the resource accepts
	// is to write, and a guess that turns out to be a whole-object replacement
	// costs the object.
	//
	// So it refuses, and says what the fix is rather than pretending the fix is
	// in. The correct implementation is read-modify-write — GET the current
	// document, substitute the four labels, PUT it back under the same
	// Content-Type — and it needs a scratch data element on a system somebody
	// is willing to have written to.
	_ = labels
	_ = lockHandle
	return fmt.Errorf("writing data element labels is not implemented: the request this used to send " +
		"was a four-field document at a resource that takes the element's whole representation, so it " +
		"could not have worked; use SE11 for now, or see WriteDataElementLabels for what a correct " +
		"implementation has to do")
}

// GetTextPoolInLanguage retrieves the text pool (text elements/symbols) of a program in a specific language.
func (c *Client) GetTextPoolInLanguage(ctx context.Context, programName, lang string) ([]TextPoolEntry, error) {
	if err := c.checkSafety(OpRead, "GetTextPoolInLanguage"); err != nil {
		return nil, err
	}

	programName = strings.ToUpper(programName)
	lang = strings.ToUpper(lang)

	// The address was /programs/programs/{name}/textelements, which answers 404
	// "No suitable resource found" on 7.58 — so this had never returned a text
	// to anybody. The text pool is not a sub-resource of the program; it is its
	// own resource, and it is a container of three:
	//
	//	/sap/bc/adt/textelements/programs/{name}/source/symbols
	//	                                        /source/selections
	//	                                        /source/headings
	//
	// Each answers plain text — `key=value` per line, with an @MaxLength
	// directive at the top of the symbols one — under its own vocabulary type.
	// Asking with */* gets an HTML rendering of the same thing, which parses to
	// nothing and would read as an empty text pool.
	var entries []TextPoolEntry
	for _, sub := range []struct {
		name string
		// ID is the classic READ TEXTPOOL id, so a caller who knows ABAP reads
		// these without a translation table: I for text symbols, S for
		// selection texts, H for column and list headings.
		id string
	}{
		{"symbols", "I"},
		{"selections", "S"},
		{"headings", "H"},
	} {
		path := fmt.Sprintf("/sap/bc/adt/textelements/programs/%s/source/%s",
			url.PathEscape(programName), sub.name)
		resp, err := c.transport.Request(ctx, path, &RequestOptions{
			Method:           http.MethodGet,
			Accept:           "application/vnd.sap.adt.textelements." + sub.name + ".v1",
			OverrideLanguage: lang,
		})
		if err != nil {
			// One missing kind is not a missing text pool: a report with no
			// selection screen has no selection texts, and that is an answer.
			// A caller that treated the first 404 as fatal would lose the two
			// kinds that did answer.
			if IsNotFoundError(err) {
				continue
			}
			return nil, fmt.Errorf("get text pool (%s): %w", sub.name, err)
		}
		entries = append(entries, parseTextPoolSource(sub.id, string(resp.Body))...)
	}

	return entries, nil
}

// parseTextPoolSource turns one text-element document into entries.
//
// Empty values are kept. "columnHeader_1=" means the heading exists and is
// untranslated, which is the single most useful thing a translation report can
// say; dropping it would turn a gap into a silence.
func parseTextPoolSource(id, body string) []TextPoolEntry {
	var out []TextPoolEntry
	for _, line := range strings.Split(body, "\n") {
		line = strings.TrimRight(line, "\r")
		if strings.TrimSpace(line) == "" {
			continue
		}
		// @MaxLength:8 and friends are directives about the document, not
		// entries in it.
		if strings.HasPrefix(strings.TrimSpace(line), "@") {
			continue
		}
		eq := strings.Index(line, "=")
		if eq < 0 {
			continue
		}
		key := strings.TrimSpace(line[:eq])
		if key == "" {
			continue
		}
		out = append(out, TextPoolEntry{ID: id, Key: key, Text: line[eq+1:]})
	}
	return out
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

// messageClassWrite is the request body for updating a message class. ADT serves
// and accepts <mc:messageClass> with the messages' number and text as attributes;
// marshalling the read model produced a bare <MessageClass> that the server could
// not interpret.
type messageClassWrite struct {
	XMLName      xml.Name       `xml:"mc:messageClass"`
	XMLNSmc      string         `xml:"xmlns:mc,attr"`
	XMLNSadtcore string         `xml:"xmlns:adtcore,attr"`
	Name         string         `xml:"adtcore:name,attr"`
	Language     string         `xml:"adtcore:language,attr,omitempty"`
	Messages     []messageWrite `xml:"mc:messages"`
}

type messageWrite struct {
	Number string `xml:"mc:msgno,attr"`
	Text   string `xml:"mc:msgtext,attr"`
}
