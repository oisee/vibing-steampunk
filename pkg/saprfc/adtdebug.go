package saprfc

import (
	"context"
	"encoding/xml"
	"fmt"
	"net/url"
	"strings"
)

// Driving SAP's own ADT debugger resources over the RFC tunnel — the same
// endpoints Eclipse uses, with no Z code on the server at all.
//
// The one thing that made this look impossible from a tool like vsp is that ADT
// keeps the debug session in an ABAP roll area and selects it with a
// sap-contextid cookie, which a short-lived stateless client cannot hold. Over a
// pinned RFC conversation there is nothing to hold: the roll area IS the
// conversation. Proven live on A4H — POST /debugger/listeners returned a
// DebuggeesList through the tunnel.
//
// The window between the listener returning and the attach is small: the
// debuggee is only valid while it waits, and a human copying an id from one
// command into the next loses that race (CX_ABDBG_ACTEXT_CANNOT_ATTACH,
// subType invalidDebuggee). So listen, attach and stack run as one step here.

// ADTDebuggee is the subset of STPDA_DEBUGGEE worth carrying.
type ADTDebuggee struct {
	ID      string `xml:"DEBUGGEE_ID"`
	User    string `xml:"DEBUGGEE_USER"`
	Program string `xml:"PRG_CURR"`
	Include string `xml:"INCL_CURR"`
	Line    int    `xml:"LINE_CURR"`
	Kind    string `xml:"DBGEE_KIND"`
	Name    string `xml:"NAME"`
	Type    string `xml:"TYPE"`
	URI     string `xml:"URI"`
	// IS_ATTACH_IMPOSSIBLE is "true"/"false" text, not a boolean type.
	AttachImpossible string `xml:"IS_ATTACH_IMPOSSIBLE"`
}

// adtDebuggeeList is the DebuggeesList envelope the listener answers with.
type adtDebuggeeList struct {
	Debuggees []ADTDebuggee `xml:"values>DATA>STPDA_DEBUGGEE"`
}

// ADTListen posts the blocking listener and returns the debuggee that stopped,
// or nil when the wait timed out with nobody there.
// acceptAnything is what every debugger request asks for.
//
// Naming a concrete type looks tidier and is a portability bug. ADT matches a
// resource on the URI *and* the media type it can produce, and when nothing
// matches it reports 404 "No suitable resource found" — not 406. So a release
// whose stack resource answers only its own vendor type reads, to a caller
// asking for application/xml, as a debugger that has no stack resource at all.
// That is exactly how 7.50 presented itself until this was widened: listener
// and attach worked, and the first stack read said the resource did not exist.
const acceptAnything = "*/*"

func (d *Debugger) ADTListen(ctx context.Context, user, ideID, terminalID string, timeoutSeconds int) (*ADTDebuggee, error) {
	// An empty user means this session's own, not "everybody": ADT registers the
	// listener under the name it is given, and a listener registered under no
	// name matches nothing — it waits out its whole timeout and reports that
	// nobody stopped, which is indistinguishable from a breakpoint that did not
	// fire.
	if strings.TrimSpace(user) == "" {
		user = d.user
	}
	d.engaged = true
	// Remembered for the teardown: a listener is removed by naming the exact
	// triple it was registered with, and a row left behind blocks the next one.
	d.listenUser, d.ideID, d.terminalID = strings.ToUpper(user), ideID, terminalID
	d.listenerStopped = false
	q := url.Values{}
	q.Set("debuggingMode", "user")
	q.Set("requestUser", strings.ToUpper(user))
	q.Set("ideId", ideID)
	q.Set("terminalId", terminalID)
	q.Set("timeout", fmt.Sprint(timeoutSeconds))

	res, err := d.ADT(ctx, "POST", "/sap/bc/adt/debugger/listeners?"+q.Encode(),
		[]ADTHeader{{Name: "Accept", Value: "application/vnd.sap.as+xml"}}, nil)
	if err != nil {
		return nil, err
	}
	if res.Status != 200 {
		return nil, adtError("listen", res)
	}
	if len(res.Body) == 0 {
		return nil, nil // the listener timed out: nobody stopped
	}
	var list adtDebuggeeList
	if err := xml.Unmarshal(res.Body, &list); err != nil {
		return nil, fmt.Errorf("reading the debuggee list: %w", err)
	}
	if len(list.Debuggees) == 0 {
		return nil, nil
	}
	return &list.Debuggees[0], nil
}

// ADTAttach attaches this session to a waiting debuggee.
func (d *Debugger) ADTAttach(ctx context.Context, debuggeeID, user string) (*ADTResponse, error) {
	d.engaged = true
	q := url.Values{}
	q.Set("method", "attach")
	q.Set("debuggeeId", debuggeeID)
	q.Set("debuggingMode", "user")
	q.Set("requestUser", strings.ToUpper(user))
	q.Set("dynproDebugging", "true")

	res, err := d.ADT(ctx, "POST", "/sap/bc/adt/debugger?"+q.Encode(),
		[]ADTHeader{{Name: "Accept", Value: acceptAnything}}, nil)
	if err != nil {
		return nil, err
	}
	if res.Status != 200 {
		return res, adtError("attach", res)
	}
	return res, nil
}

// ADTStack reads the attached debuggee's call stack.
// stackShape is how a release delivers the call stack.
type stackShape int

const (
	stackShapeUnknown stackShape = iota
	// stackShapeResource is the dedicated resource, present from 7.5x on.
	stackShapeResource
	// stackShapeDispatcher is the older shape: the same getStack method, posted
	// to the debugger resource itself. 7.50 has no /debugger/stack at all.
	stackShapeDispatcher
)

// ADTStack reads the call stack of the stopped program.
//
// Two releases, two shapes. Newer ones expose /sap/bc/adt/debugger/stack; 7.50
// does not have that resource — it is absent from the discovery document and
// answers 404 — and serves the same document from the dispatcher instead. Both
// return dbg:stack with the same entries, so only the request differs, and
// which one works is remembered after the first read.
func (d *Debugger) ADTStack(ctx context.Context) (*ADTResponse, error) {
	q := url.Values{}
	q.Set("method", "getStack")
	q.Set("emode", "_")
	q.Set("semanticURIs", "true")

	if d.stackShape != stackShapeDispatcher {
		res, err := d.ADT(ctx, "GET", "/sap/bc/adt/debugger/stack?"+q.Encode(),
			[]ADTHeader{{Name: "Accept", Value: acceptAnything}}, nil)
		if err == nil && res.Status == 200 {
			d.stackShape = stackShapeResource
			return res, nil
		}
		// 404 here means the resource is not on this release, which is a
		// different thing from "the stack could not be read" and is worth one
		// retry in the older shape. Anything else is a real failure: retrying a
		// refusal or a server error in another shape only hides it.
		if !stackResourceAbsent(res) {
			if err != nil {
				return nil, err
			}
			return res, adtError("stack", res)
		}
		if d.stackShape == stackShapeResource {
			// It worked before on this very session, so a 404 now is news
			// about the session, not about the release.
			return res, adtError("stack", res)
		}
	}

	res, err := d.ADT(ctx, "POST", "/sap/bc/adt/debugger?"+q.Encode(),
		[]ADTHeader{{Name: "Accept", Value: acceptAnything}}, nil)
	if err != nil {
		return nil, err
	}
	if res.Status != 200 {
		return res, adtError("stack", res)
	}
	d.stackShape = stackShapeDispatcher
	return res, nil
}

// stackResourceAbsent reports the one answer worth retrying in the older shape:
// the resource itself is not there. A transport failure gives no response at
// all, and that is not evidence about the release.
func stackResourceAbsent(res *ADTResponse) bool {
	return res != nil && res.Status == 404
}

// ADTVariables reads named variables from the attached debuggee. The names it
// wants are the ones the stack and child-variable calls hand back — @ROOT and
// @DATAAGING are the two roots that always exist, a local is just its name.
func (d *Debugger) ADTVariables(ctx context.Context, names []string) (*ADTResponse, error) {
	if len(names) == 0 {
		names = []string{"@ROOT", "@DATAAGING"}
	}
	var items []string
	for _, n := range names {
		items = append(items, "<STPDA_ADT_VARIABLE><ID>"+xmlEsc(n)+"</ID></STPDA_ADT_VARIABLE>")
	}
	body := []byte(`<?xml version="1.0" encoding="UTF-8"?><asx:abap xmlns:asx="http://www.sap.com/abapxml" version="1.0"><asx:values><DATA>` +
		strings.Join(items, "") + `</DATA></asx:values></asx:abap>`)
	res, err := d.ADT(ctx, "POST", "/sap/bc/adt/debugger?method=getVariables",
		[]ADTHeader{{Name: "Accept", Value: "application/vnd.sap.as+xml"},
			{Name: "Content-Type", Value: "application/vnd.sap.as+xml;charset=UTF-8;dataname=com.sap.adt.debugger.Variables"}}, body)
	if err != nil {
		return nil, err
	}
	if res.Status != 200 {
		return res, adtError("variables", res)
	}
	return res, nil
}

// ADTChildVariables expands a structure or table variable by parent id.
func (d *Debugger) ADTChildVariables(ctx context.Context, parents []string) (*ADTResponse, error) {
	if len(parents) == 0 {
		parents = []string{"@ROOT", "@DATAAGING"}
	}
	var items []string
	for _, p := range parents {
		items = append(items, "<STPDA_ADT_VARIABLE_HIERARCHY><PARENT_ID>"+xmlEsc(p)+"</PARENT_ID></STPDA_ADT_VARIABLE_HIERARCHY>")
	}
	body := []byte(`<?xml version="1.0" encoding="UTF-8"?><asx:abap xmlns:asx="http://www.sap.com/abapxml" version="1.0"><asx:values><DATA><HIERARCHIES>` +
		strings.Join(items, "") + `</HIERARCHIES></DATA></asx:values></asx:abap>`)
	res, err := d.ADT(ctx, "POST", "/sap/bc/adt/debugger?method=getChildVariables",
		[]ADTHeader{{Name: "Accept", Value: "application/vnd.sap.as+xml"},
			{Name: "Content-Type", Value: "application/vnd.sap.as+xml;charset=UTF-8;dataname=com.sap.adt.debugger.ChildVariables"}}, body)
	if err != nil {
		return nil, err
	}
	if res.Status != 200 {
		return res, adtError("childVariables", res)
	}
	return res, nil
}

// xmlEsc escapes the few characters that matter inside an element body.
func xmlEsc(s string) string {
	r := strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;")
	return r.Replace(s)
}

// ADTDetach ends an ADT debug session: it releases the debuggee and removes the
// listener registration.
//
// It exists because closing the client is not enough on the HTTP route. Over RFC
// the facade's detach ends external debugging for the user and the debuggee runs
// on; over HTTPS there is no facade, so a session that simply exits leaves its
// debuggee suspended in a work process until the caller's own timeout fires —
// which is what it looked like from the other side: an RFC call that never
// answered.
func (d *Debugger) ADTDetach(ctx context.Context) error {
	if !d.engaged {
		return nil
	}
	// Whether the debuggee half was confirmed does not change what comes
	// next: the listener goes either way, and with it external debugging.
	_ = d.DetachDebuggee(ctx)
	// A listener already removed from another connection (ADTStopListenerVia)
	// is not removed twice: SAP confirmed it is gone.
	if user := d.listenUser; user != "" && !d.listenerStopped {
		// The listener is removed by naming the user and nothing else. In user
		// debugging mode SAP does not store the ideId and terminalId it was
		// given: ABDBG_LISTENER holds IDE_ID = the user and TERMINAL_ID =
		// '%_USER', so a DELETE quoting our own ids matches no row, the
		// registration survives, and it silently swallows the next debuggee —
		// a second listen on the same session then waits out its whole timeout
		// while the debuggee stops for a listener that no longer exists.
		//
		// Removing it this way ends external debugging for the user, and the
		// user's external breakpoints go with it. That is SAP's behaviour, not a
		// choice available to us: the two are the same act. So a detach leaves a
		// clean slate, and anything that wants to catch a second debuggee arms
		// its breakpoints again first.
		if _, lerr := d.ADT(ctx, "DELETE", listenerURI(user), nil, nil); lerr != nil {
			return lerr
		}
	}
	d.engaged = false
	return nil
}

// listenerURI names the user's listener registration the way ADTDetach
// removes it: by the user alone (see there for why not the ids).
func listenerURI(user string) string {
	q := url.Values{}
	q.Set("debuggingMode", "user")
	q.Set("requestUser", user)
	return "/sap/bc/adt/debugger/listeners?" + q.Encode()
}

// ADTStopListenerVia removes this session's listener registration through
// another transport — a stateless request or a second session — instead of
// this session's own.
//
// It exists for the one moment this session cannot be used: while a listener
// request is still open on it. A stateful ADT session answers one request at a
// time, and a client that stops waiting for the listener does not end it on
// the server — the request runs on until its timeout, and everything sent on
// the session meanwhile queues behind it. The registration is keyed by the
// user alone, not by the session that made it (ABDBG_LISTENER holds IDE_ID =
// the user and TERMINAL_ID = '%_USER' in user mode), so any connection of the
// same logon can remove it, and the DELETE is exactly the one ADTDetach sends.
//
// It succeeds only when SAP confirmed the removal; ADTDetach then skips the
// DELETE it would otherwise send. Nothing else is released: the breakpoints
// and any attached debuggee belong to this session and are released on it.
func (d *Debugger) ADTStopListenerVia(ctx context.Context, t ADTTransport) error {
	user := d.listenUser
	if user == "" || d.listenerStopped {
		return nil // nothing registered from here, or already removed
	}
	if t == nil {
		return fmt.Errorf("stop listener: no separate connection")
	}
	res, err := t.Do(ctx, ADTRequest{Method: "DELETE", URI: listenerURI(user)})
	if err != nil {
		return err
	}
	if res == nil {
		return fmt.Errorf("stop listener: no answer")
	}
	if res.Status < 200 || res.Status >= 300 {
		return adtError("stop listener", res)
	}
	d.listenerStopped = true
	return nil
}

// DetachDebuggee releases the debuggee attached to this session, if any, and
// leaves the listener registration and the breakpoints alone: it is the
// debuggee half of ADTDetach, for a caller that means to go on listening.
//
// SAP's own word for "let it go" is detach; a release that does not know it
// (A4H answers 400, "Unknown method") still has to release the debuggee, so
// continue is the fallback rather than terminateDebuggee, which would kill the
// user's session outright.
//
// It returns nil only when SAP confirmed that nothing is attached any more:
// the detach was accepted, or the fallback answered that the debuggee ended or
// that no session is attached. A continue that answers 200 means the program
// stopped again, at another breakpoint, and is still attached; that, and any
// failure to get an answer, is an error.
func (d *Debugger) DetachDebuggee(ctx context.Context) error {
	res, err := d.ADT(ctx, "POST", "/sap/bc/adt/debugger?method=detach",
		[]ADTHeader{{Name: "Accept", Value: acceptAnything}}, nil)
	if err == nil && res.Status >= 200 && res.Status < 300 {
		return nil
	}
	sres, serr := d.ADTStep(ctx, "stepContinue")
	switch {
	case serr == nil:
		return fmt.Errorf("detach: the debuggee ran on and stopped again; it is still attached")
	case strings.Contains(serr.Error(), "debuggeeEnded"), strings.Contains(serr.Error(), "noSessionAttached"):
		return nil
	case sres == nil:
		return fmt.Errorf("detach: no answer from SAP: %w", serr)
	default:
		return fmt.Errorf("detach: %w", serr)
	}
}

// ADTStep executes one step: stepInto, stepOver, stepReturn, stepContinue.
func (d *Debugger) ADTStep(ctx context.Context, method string) (*ADTResponse, error) {
	q := url.Values{}
	q.Set("method", method)

	res, err := d.ADT(ctx, "POST", "/sap/bc/adt/debugger?"+q.Encode(),
		[]ADTHeader{{Name: "Accept", Value: acceptAnything}}, nil)
	if err != nil {
		return nil, err
	}
	if res.Status != 200 {
		return res, adtError(method, res)
	}
	return res, nil
}

// ADTCatch is listen → attach → stack with nothing in between, because the
// debuggee stays attachable only while it waits.
func (d *Debugger) ADTCatch(ctx context.Context, user, ideID, terminalID string, timeoutSeconds int) (*ADTDebuggee, *ADTResponse, error) {
	who, err := d.ADTListen(ctx, user, ideID, terminalID, timeoutSeconds)
	if err != nil || who == nil {
		return nil, nil, err
	}
	if _, err := d.ADTAttach(ctx, who.ID, user); err != nil {
		return who, nil, err
	}
	stack, err := d.ADTStack(ctx)
	return who, stack, err
}

// adtError turns an ADT exception document into a Go error, keeping the
// subType — "invalidDebuggee" and "noSessionAttached" are the two that actually
// tell you what went wrong.
func adtError(what string, res *ADTResponse) error {
	body := string(res.Body)

	// SAP refuses in two registers at once: a sentence meant for a person
	// ("User X does not exist") and a category meant for a program
	// ("notAuthorized"). Report the sentence first — it is the part that says
	// what to do about it — and keep the category alongside, since that is what
	// callers switch on. Reporting the category alone, as this did, turns every
	// distinct refusal into the same unhelpful word.
	message := adtExceptionMessage(body)
	category := between(body, `subType">`, "<")

	switch {
	case message != "" && category != "":
		return fmt.Errorf("%s: ADT %d %s: %s (%s)", what, res.Status, res.ReasonPhrase, message, category)
	case message != "":
		return fmt.Errorf("%s: ADT %d %s: %s", what, res.Status, res.ReasonPhrase, message)
	case category != "":
		return fmt.Errorf("%s: ADT %d %s (%s)", what, res.Status, res.ReasonPhrase, category)
	default:
		return fmt.Errorf("%s: ADT %d %s", what, res.Status, res.ReasonPhrase)
	}
}

// adtExceptionMessage pulls the human sentence out of an ADT exception
// document. The element carries whatever language the session logged on in, so
// matching on lang="EN" — as this used to — silently found nothing on a system
// running in any other one, and the refusal came back as a bare status.
func adtExceptionMessage(body string) string {
	for _, open := range []string{"<message", "<localizedMessage"} {
		i := strings.Index(body, open)
		if i < 0 {
			continue
		}
		rest := body[i:]
		j := strings.Index(rest, ">")
		if j < 0 {
			continue
		}
		if text := between(rest[j:], ">", "<"); text != "" {
			return text
		}
	}
	return ""
}

// between returns what sits after the first open and before the next close.
func between(s, open, close string) string {
	i := strings.Index(s, open)
	if i < 0 {
		return ""
	}
	rest := s[i+len(open):]
	j := strings.Index(rest, close)
	if j < 0 {
		return ""
	}
	return strings.TrimSpace(rest[:j])
}
