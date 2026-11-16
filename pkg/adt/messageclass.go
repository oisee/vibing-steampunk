package adt

import (
	"context"
	"encoding/xml"
	"fmt"
	"net/http"
	"net/url"
	"strings"
)

// Message classes over /sap/bc/adt/messageclass (SMESSAGE_ADT). The PUT
// takes the whole class document and reads from it what the editor sends:
// the language the texts go in (adtcore:language, straight into
// T100-SPRSL — not the session's language), the package (packageRef) and
// the short text (adtcore:description, rewritten whenever it differs).
// A body with the messages alone was answered 200 and wrote the texts
// under an empty language, where no logon shows them, and blanked the
// class's short text (7.50, 2026-09-29). 7.40 SP06 takes the language
// from the session instead and saves only when adtcore:masterLanguage
// equals it — any other body is answered 200 and nothing is written; a
// translation cannot be written there at all. Messages the body does not
// name are left as they are. A message class has no activation.

const messageClassNS = "http://www.sap.com/adt/MessageClass"

func messageClassURL(name string) string {
	return "/sap/bc/adt/messageclass/" + url.PathEscape(strings.ToLower(name))
}

// readMessageClass reads the class in one language. Stateful keeps the
// read on the session of a lock already taken: a stateless request between
// LOCK and PUT retires the lock handle (#91).
func (c *Client) readMessageClass(ctx context.Context, name, lang string, stateful bool) (*MessageClass, error) {
	resp, err := c.transport.Request(ctx, messageClassURL(name), &RequestOptions{
		Method:           http.MethodGet,
		Accept:           "application/vnd.sap.adt.mc.messageclass+xml",
		OverrideLanguage: lang,
		Stateful:         stateful,
	})
	if err != nil {
		return nil, fmt.Errorf("reading message class %s: %w", name, err)
	}
	var mc MessageClass
	if err := xml.Unmarshal(resp.Body, &mc); err != nil {
		return nil, fmt.Errorf("parsing message class %s: %w", name, err)
	}
	mc.Name = strings.ToUpper(name)
	mc.Package = mc.PackageRef.Name
	return &mc, nil
}

type messageClassWrite struct {
	XMLName      xml.Name            `xml:"mc:messageClass"`
	XMLNSmc      string              `xml:"xmlns:mc,attr"`
	XMLNSadtcore string              `xml:"xmlns:adtcore,attr"`
	Name         string              `xml:"adtcore:name,attr"`
	Type         string              `xml:"adtcore:type,attr"`
	Description  string              `xml:"adtcore:description,attr"`
	Language     string              `xml:"adtcore:language,attr"`
	Master       string              `xml:"adtcore:masterLanguage,attr,omitempty"`
	PackageRef   *messageClassPkgRef `xml:"adtcore:packageRef,omitempty"`
	Messages     []messageWrite      `xml:"mc:messages"`
}

type messageClassPkgRef struct {
	Name string `xml:"adtcore:name,attr"`
}

type messageWrite struct {
	Number          string `xml:"mc:msgno,attr"`
	Text            string `xml:"mc:msgtext,attr,omitempty"`
	SelfExplanatory string `xml:"mc:selfexplainatory,attr,omitempty"`
}

// messageClassBody is the PUT document. cur is the class as read: its
// short text and package go back unchanged, so the write touches only the
// messages named. lang is the ISO language the texts are written in.
func messageClassBody(cur *MessageClass, lang string, msgs []MessageClassMessage) ([]byte, error) {
	w := messageClassWrite{
		XMLNSmc:      messageClassNS,
		XMLNSadtcore: "http://www.sap.com/adt/core",
		Name:         strings.ToUpper(cur.Name),
		Type:         string(ObjectTypeMessageClass),
		Description:  cur.Description,
		Language:     strings.ToUpper(lang),
		Master:       strings.ToUpper(cur.MasterLanguage),
	}
	if cur.Package != "" {
		w.PackageRef = &messageClassPkgRef{Name: cur.Package}
	}
	for _, m := range msgs {
		w.Messages = append(w.Messages, messageWrite{Number: m.Number, Text: m.Text, SelfExplanatory: m.SelfExplanatory})
	}
	body, err := xml.Marshal(w)
	if err != nil {
		return nil, fmt.Errorf("marshal message class XML: %w", err)
	}
	return append([]byte(xml.Header), body...), nil
}
