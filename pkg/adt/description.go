package adt

import (
	"context"
	"fmt"
	"html"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
)

// An object's description is the short text SE38 shows as the title, the
// one a list prints in its header, TRDIRT for a program. Creating over ADT
// sets it once; changing it afterwards meant SE38 or the Eclipse properties
// page. It is an attribute of the object's metadata document, and that
// document is what a PUT of the object under a lock replaces.

// DescriptionResult says what changed.
type DescriptionResult struct {
	ObjectURL string `json:"objectUrl"`
	Old       string `json:"old"`
	New       string `json:"new"`
	Changed   bool   `json:"changed"`
	Transport string `json:"transport,omitempty"`
	Limit     int    `json:"limit,omitempty"`
}

var (
	descriptionAttr  = regexp.MustCompile(`\sadtcore:description="([^"]*)"`)
	descriptionLimit = regexp.MustCompile(`\sadtcore:descriptionTextLimit="(\d+)"`)
)

// DescriptionObjectURL is the metadata document of an object a description
// can be set on. parent is the function group of a function module.
func DescriptionObjectURL(objectType, name, parent string) (string, error) {
	enc := url.PathEscape(strings.ToLower(strings.TrimSpace(name)))
	switch strings.ToUpper(strings.TrimSpace(objectType)) {
	case "PROG", "PROG/P", "PROGRAM", "":
		return "/sap/bc/adt/programs/programs/" + enc, nil
	case "INCL", "PROG/I", "INCLUDE":
		return "/sap/bc/adt/programs/includes/" + enc, nil
	case "CLAS", "CLAS/OC", "CLASS":
		return "/sap/bc/adt/oo/classes/" + enc, nil
	case "INTF", "INTF/OI", "INTERFACE":
		return "/sap/bc/adt/oo/interfaces/" + enc, nil
	case "FUGR", "FUGR/F":
		return "/sap/bc/adt/functions/groups/" + enc, nil
	case "FUNC", "FUGR/FF":
		if parent == "" {
			return "", fmt.Errorf("a function module needs its function group (parent)")
		}
		return "/sap/bc/adt/functions/groups/" + url.PathEscape(strings.ToLower(parent)) + "/fmodules/" + enc, nil
	case "TABL", "TABL/DT":
		return "/sap/bc/adt/ddic/tables/" + enc, nil
	case "DDLS", "DDLS/DF":
		return "/sap/bc/adt/ddic/ddl/sources/" + enc, nil
	}
	return "", fmt.Errorf("no description handling for object type %q (PROG, INCL, CLAS, INTF, FUGR, FUNC, TABL, DDLS)", objectType)
}

// GetDescription reads the description from the object's metadata.
func (c *Client) GetDescription(ctx context.Context, objectType, name, parent string) (*DescriptionResult, error) {
	objectURL, err := DescriptionObjectURL(objectType, name, parent)
	if err != nil {
		return nil, err
	}
	body, _, err := c.readMetadata(ctx, objectURL, false)
	if err != nil {
		return nil, err
	}
	old, limit, err := descriptionOf(body)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", objectURL, err)
	}
	return &DescriptionResult{ObjectURL: objectURL, Old: old, New: old, Limit: limit}, nil
}

// SetDescription changes the description and nothing else. Nothing is
// locked when the text is already so.
func (c *Client) SetDescription(ctx context.Context, objectType, name, parent, description, transport string) (*DescriptionResult, error) {
	objectURL, err := DescriptionObjectURL(objectType, name, parent)
	if err != nil {
		return nil, err
	}
	description = strings.TrimSpace(description)
	if description == "" {
		return nil, fmt.Errorf("an empty description is not written")
	}
	if err = c.checkMutation(ctx, MutationContext{Op: OpUpdate, OpName: "SetDescription", ObjectURL: objectURL, Transport: transport}); err != nil {
		return nil, err
	}
	body, _, err := c.readMetadata(ctx, objectURL, false)
	if err != nil {
		return nil, err
	}
	old, limit, err := descriptionOf(body)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", objectURL, err)
	}
	res := &DescriptionResult{ObjectURL: objectURL, Old: old, New: description, Limit: limit}
	if limit > 0 && len([]rune(description)) > limit {
		return res, fmt.Errorf("the description is %d characters; %s allows %d", len([]rune(description)), objectURL, limit)
	}
	if old == description {
		return res, nil
	}

	lock, err := c.LockObject(ctx, objectURL, "MODIFY")
	if err != nil {
		return res, fmt.Errorf("locking %s: %w", objectURL, err)
	}
	unlockCtx := context.WithoutCancel(ctx)
	defer func() { _ = c.UnlockObject(unlockCtx, objectURL, lock.LockHandle) }()
	if res.Transport, err = c.resolveWriteTransport(transport, lock.CorrNr, "SetDescription"); err != nil {
		return res, err
	}
	// Read again under the lock: the document is put back whole, so it has
	// to be the current one.
	body, contentType, err := c.readMetadata(ctx, objectURL, true)
	if err != nil {
		return res, err
	}
	if res.Old, _, err = descriptionOf(body); err != nil {
		return res, fmt.Errorf("%s: %w", objectURL, err)
	}
	updated := descriptionAttr.ReplaceAllLiteralString(body, ` adtcore:description="`+html.EscapeString(description)+`"`)
	params := url.Values{}
	params.Set("lockHandle", lock.LockHandle)
	if res.Transport != "" {
		params.Set("corrNr", res.Transport)
	}
	if _, err := c.transport.Request(ctx, objectURL, &RequestOptions{
		Method:      http.MethodPut,
		Query:       params,
		Body:        []byte(updated),
		ContentType: contentType,
		Accept:      contentType,
		Stateful:    true,
	}); err != nil {
		return res, fmt.Errorf("writing the description of %s: %w", objectURL, err)
	}
	res.Changed = true
	return res, nil
}

// readMetadata is the object's own document, in whatever vocabulary the
// server speaks for it; the content type comes back so a PUT can use it.
func (c *Client) readMetadata(ctx context.Context, objectURL string, stateful bool) (string, string, error) {
	resp, err := c.transport.Request(ctx, objectURL, &RequestOptions{Method: http.MethodGet, Accept: "*/*", Stateful: stateful})
	if err != nil {
		return "", "", fmt.Errorf("reading %s: %w", objectURL, err)
	}
	ct := resp.Headers.Get("Content-Type")
	if i := strings.Index(ct, ";"); i >= 0 {
		ct = strings.TrimSpace(ct[:i])
	}
	return string(resp.Body), ct, nil
}

func descriptionOf(body string) (string, int, error) {
	m := descriptionAttr.FindStringSubmatch(body)
	if m == nil {
		return "", 0, fmt.Errorf("the metadata carries no adtcore:description")
	}
	limit := 0
	if l := descriptionLimit.FindStringSubmatch(body); l != nil {
		limit, _ = strconv.Atoi(l[1])
	}
	return html.UnescapeString(m[1]), limit, nil
}
