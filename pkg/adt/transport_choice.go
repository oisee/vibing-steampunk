package adt

import (
	"context"
	"encoding/xml"
	"errors"
	"fmt"
	"net/http"
	"strings"
)

// A write to a transportable object with no transport named used to leave
// the choice to SAP, which answers with a request of its own — "Generated
// Request for Change Recording" — one per write, so a day's work on one
// feature ended up spread over three requests beside the one the developer
// already had open. Eclipse does not do that: it asks the transport check
// which of the user's open requests fit, and offers them. This does the
// same, without the dialog: the open request that already holds this
// package's objects wins, then the only candidate, then the newest; when
// there is none, one is created and the result says so.

// TransportCheck is what /sap/bc/adt/cts/transportchecks answers for one
// object: whether a request is needed, and which of the user's open ones
// fit.
type TransportCheck struct {
	Package    string
	Recording  bool // a request is required
	ObjectType string
	ObjectName string
	// Candidates are the user's modifiable requests the object could go
	// into, as the check lists them.
	Candidates []TransportCandidate
	// LockedIn is the request the object is already locked in, when the
	// check reports one.
	LockedIn string
	Messages []string
}

// TransportCandidate is one open request from the check.
type TransportCandidate struct {
	Number      string `json:"number"`
	Owner       string `json:"owner"`
	Description string `json:"description"`
	Status      string `json:"status"`
	Function    string `json:"function"`
}

// TransportChoice is the outcome: the request to write under and why.
type TransportChoice struct {
	Transport string `json:"transport,omitempty"`
	// Reason says how the request was chosen — reused, created, or none
	// needed — in words for the result.
	Reason  string `json:"reason,omitempty"`
	Created bool   `json:"created,omitempty"`
	// Err is a creation that failed after the caller enabled it.
	Err error `json:"-"`
}

var errTransportCreate = errors.New("creating a transport request failed")

// CheckTransport runs the transport check for an object about to be
// written. devClass may be empty for an existing object.
func (c *Client) CheckTransport(ctx context.Context, objectURL, devClass, operation string) (*TransportCheck, error) {
	if operation == "" {
		operation = "I"
	}
	body := fmt.Sprintf(`<?xml version="1.0" encoding="utf-8"?>
<asx:abap xmlns:asx="http://www.sap.com/abapxml" version="1.0">
  <asx:values>
    <DATA>
      <DEVCLASS>%s</DEVCLASS>
      <OPERATION>%s</OPERATION>
      <URI>%s</URI>
    </DATA>
  </asx:values>
</asx:abap>`, escapeXMLAttr(strings.ToUpper(devClass)), operation, escapeXMLAttr(objectURL))
	resp, err := c.transport.Request(ctx, "/sap/bc/adt/cts/transportchecks", &RequestOptions{
		Method:      http.MethodPost,
		Body:        []byte(body),
		ContentType: "application/vnd.sap.as+xml; charset=UTF-8; dataname=com.sap.adt.transport.service.checkData",
		Accept:      "application/vnd.sap.as+xml;charset=UTF-8;dataname=com.sap.adt.transport.service.checkData",
	})
	if err != nil {
		return nil, fmt.Errorf("transport check for %s: %w", objectURL, err)
	}
	return parseTransportCheck(resp.Body)
}

func parseTransportCheck(data []byte) (*TransportCheck, error) {
	type header struct {
		Trkorr     string `xml:"TRKORR"`
		Function   string `xml:"TRFUNCTION"`
		Status     string `xml:"TRSTATUS"`
		User       string `xml:"AS4USER"`
		Text       string `xml:"AS4TEXT"`
		LockedTask string `xml:"LOCKED_TASK"`
	}
	type ctsRequest struct {
		Header header `xml:"REQ_HEADER"`
	}
	type lock struct {
		Header header `xml:"REQ_HEADER"`
		Task   string `xml:"TASK"`
		Trkorr string `xml:"TRKORR"`
	}
	type msg struct {
		Text string `xml:"TEXT"`
		Type string `xml:"SEVERITY"`
	}
	type dataType struct {
		Object     string       `xml:"OBJECT"`
		ObjectName string       `xml:"OBJECTNAME"`
		DevClass   string       `xml:"DEVCLASS"`
		TadirDevc  string       `xml:"TADIRDEVC"`
		Recording  string       `xml:"RECORDING"`
		Requests   []ctsRequest `xml:"REQUESTS>CTS_REQUEST"`
		Locks      []lock       `xml:"LOCKS>CTS_OBJECT_LOCK"`
		Messages   []msg        `xml:"MESSAGES>CTS_MESSAGE"`
	}
	type root struct {
		Data dataType `xml:"values>DATA"`
	}
	var r root
	if err := xml.Unmarshal([]byte(strings.ReplaceAll(string(data), "asx:", "")), &r); err != nil {
		return nil, fmt.Errorf("parsing the transport check: %w", err)
	}
	d := r.Data
	tc := &TransportCheck{Package: d.DevClass, Recording: d.Recording == "X", ObjectType: d.Object, ObjectName: d.ObjectName}
	if tc.Package == "" {
		tc.Package = d.TadirDevc
	}
	for _, q := range d.Requests {
		h := q.Header
		tc.Candidates = append(tc.Candidates, TransportCandidate{Number: h.Trkorr, Owner: h.User, Description: h.Text, Status: h.Status, Function: h.Function})
	}
	for _, l := range d.Locks {
		switch {
		case l.Header.Trkorr != "":
			tc.LockedIn = l.Header.Trkorr
		case l.Trkorr != "":
			tc.LockedIn = l.Trkorr
		}
	}
	for _, m := range d.Messages {
		if strings.TrimSpace(m.Text) != "" {
			tc.Messages = append(tc.Messages, strings.TrimSpace(m.Text))
		}
	}
	return tc, nil
}

// chooseTransport picks the request a write with no transport named goes
// under. The lock's own request comes first — an object already captured
// stays where it is. Otherwise the check's candidates: the one that
// already holds an object of this package, else the only one, else the
// newest. With no candidate and recording on, a request is created,
// named after the package and the object, and the choice says so.
func (c *Client) chooseTransport(ctx context.Context, objectURL, devClass, lockCorrNr, description string) (*TransportChoice, error) {
	if lockCorrNr != "" {
		return &TransportChoice{Transport: lockCorrNr, Reason: "the request the object is already locked in"}, nil
	}
	op := "U"
	if devClass != "" {
		op = "I"
	}
	check, err := c.CheckTransport(ctx, objectURL, devClass, op)
	if err != nil {
		return nil, err
	}
	if !check.Recording {
		return &TransportChoice{}, nil
	}
	if check.LockedIn != "" {
		return &TransportChoice{Transport: check.LockedIn, Reason: "the request the object is already locked in"}, nil
	}
	open := check.Candidates[:0:0]
	for _, cand := range check.Candidates {
		if cand.Status == "" || cand.Status == "D" || cand.Status == "L" {
			open = append(open, cand)
		}
	}
	pkg := check.Package
	if pkg == "" {
		pkg = strings.ToUpper(devClass)
	}
	switch {
	case len(open) == 1:
		return &TransportChoice{Transport: open[0].Number, Reason: fmt.Sprintf("reused %s (%s): your only open request that fits", open[0].Number, open[0].Description)}, nil
	case len(open) > 1:
		// The one already holding this package's objects.
		for _, cand := range open {
			if pkg != "" && c.transportHoldsPackage(ctx, cand.Number, pkg) {
				return &TransportChoice{Transport: cand.Number, Reason: fmt.Sprintf("reused %s (%s): it already holds objects of %s", cand.Number, cand.Description, pkg)}, nil
			}
		}
		newest := open[0]
		for _, cand := range open[1:] {
			if cand.Number > newest.Number {
				newest = cand
			}
		}
		return &TransportChoice{Transport: newest.Number, Reason: fmt.Sprintf("reused %s (%s): the newest of %d open requests that fit; name transport to choose another", newest.Number, newest.Description, len(open))}, nil
	}
	if description == "" {
		description = fmt.Sprintf("%s %s", pkg, check.ObjectName)
	}
	if !c.config.Safety.IsTransportWriteAllowed() {
		return &TransportChoice{Reason: fmt.Sprintf("no open request of yours holds %s; SAP will generate one — name transport, or run with --enable-transports to have one created here", pkg)}, nil
	}
	number, err := c.CreateTransport(ctx, objectURL, description, pkg)
	if err != nil {
		return nil, fmt.Errorf("%w: no open request of yours fits %s: %v", errTransportCreate, pkg, err)
	}
	return &TransportChoice{Transport: number, Created: true, Reason: fmt.Sprintf("created request %s (%s): no open request of yours held %s", number, description, pkg)}, nil
}

// transportHoldsPackage reports whether a request carries an object of the
// package, judged by TADIR — the request's object list names objects, not
// packages.
func (c *Client) transportHoldsPackage(ctx context.Context, number, pkg string) bool {
	details, err := c.GetTransport(ctx, number)
	if err != nil {
		return false
	}
	var names []string
	for _, o := range details.Objects {
		if o.PgmID == "R3TR" || o.PgmID == "LIMU" {
			names = append(names, "'"+sqlQuote(o.Name)+"'")
		}
	}
	for _, t := range details.Tasks {
		for _, o := range t.Objects {
			if o.PgmID == "R3TR" || o.PgmID == "LIMU" {
				names = append(names, "'"+sqlQuote(o.Name)+"'")
			}
		}
	}
	if len(names) == 0 {
		return false
	}
	if len(names) > 200 {
		names = names[:200]
	}
	res, err := c.RunQuery(ctx, fmt.Sprintf("SELECT COUNT(*) AS n FROM tadir WHERE devclass = '%s' AND obj_name IN (%s)", sqlQuote(pkg), strings.Join(names, ",")), 1)
	if err != nil || res == nil || len(res.Rows) == 0 {
		return false
	}
	return cell(res.Rows[0], "N") != "0" && cell(res.Rows[0], "N") != ""
}

// planTransport is the half of the choice that runs before the lock: the
// check and, when transports are enabled and nothing fits, the creation.
// It runs before the lock because it is a stateless request, and a
// stateless hop between LOCK and PUT retires the lock handle (#91). A
// check that fails is not a reason to refuse the write — the choice is
// then left to SAP as it was — but a creation that fails is, since the
// caller enabled it. nil when a request was named, or the choice is off.
func (c *Client) planTransport(ctx context.Context, supplied, objectURL, devClass string) *TransportChoice {
	if supplied != "" || c.config.Safety.TransportChoice == "off" {
		return nil
	}
	choice, err := c.chooseTransport(ctx, objectURL, devClass, "", "")
	if err != nil {
		if errors.Is(err, errTransportCreate) {
			return &TransportChoice{Err: err}
		}
		return &TransportChoice{Reason: "transport not chosen here (" + err.Error() + "); left to SAP"}
	}
	return choice
}

// resolveWriteTransportFor is resolveWriteTransport with the plan behind
// it: the supplied request, else the lock's own, else the planned one.
// The policy on transportable edits is applied to whatever was chosen, so
// a request found or made cannot bypass the gate the caller would have
// hit naming it. The reason comes back for the result.
func (c *Client) resolveWriteTransportFor(plan *TransportChoice, supplied, lockCorrNr, opName string) (string, string, error) {
	if supplied != "" || plan == nil {
		tr, err := c.resolveWriteTransport(supplied, lockCorrNr, opName)
		return tr, "", err
	}
	if plan.Err != nil {
		return "", "", plan.Err
	}
	if lockCorrNr != "" {
		if err := c.checkTransportableEdit(lockCorrNr, opName); err != nil {
			return "", "", err
		}
		return lockCorrNr, "", nil
	}
	if plan.Transport == "" {
		return "", plan.Reason, nil
	}
	if err := c.checkTransportableEdit(plan.Transport, opName); err != nil {
		return "", "", err
	}
	return plan.Transport, plan.Reason, nil
}
