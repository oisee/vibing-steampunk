package saprfc

import (
	"context"
	"fmt"
	"net/url"
	"strings"

	"github.com/oisee/vibing-steampunk/pkg/adt"
)

// Breakpoints through SAP's own ADT resource, on whichever transport this
// session speaks.
//
// vsp used to reach them two other ways — the deprecated stateless REST client,
// which answered 403 because a CSRF token cannot survive a session it does not
// have, and the ZADT_VSP WebSocket, which existed to provide the session the
// stateless client lacked. Both are unnecessary here: the tunnel's roll area and
// the stateful ICF session are each a session, and /sap/bc/adt/debugger/breakpoints
// answers 200 on both. Verified live on A4H, 2026-08-21, over each in turn.
//
// The resource is a *set*, not a list of rows: a POST replaces every breakpoint
// registered for this (user, ideId, terminalId, scope). Deleting one therefore
// means posting the others, which is what Eclipse does and what DropBreakpoint
// does below.

// SystemDebugging decides whether breakpoints in SAP's own code can fire.
//
// Off — the default — is not merely a policy of ours: a breakpoint set in a
// system program is accepted, is given an id, and then never stops anything.
// Measured on A4H: a breakpoint on SAPMSSY0's %_BEFORE_COMMIT, which a trace
// proves runs on every COMMIT WORK, did not fire until this flag was set, and
// fired immediately once it was. So customer code is where the debugger lives
// by default, and standard code is one deliberate switch away.
//
// Staying out of standard costs less than it sounds. A breakpoint on the line
// that calls a standard function module captures what was passed in, and one on
// the line after captures what came back — the contract, without stepping
// through somebody else's implementation.
func (d *Debugger) SystemDebugging(on bool) { d.systemDebugging = on }

// IDEID identifies vsp to SAP's breakpoint and listener registries.
const IDEID = "vsp"

// TerminalID is the 32-character terminal this client registers as. SAP wants an
// id of that shape; it only has to be stable and distinct, not meaningful — but
// stable matters: breakpoints are registered against it, and a client that
// changes it cannot see its own breakpoints.
const TerminalID = "56535000000000000000000000006462"

// ideID returns the registry identity of this session, defaulting to vsp's.
func (d *Debugger) ideIDs() (ide, term string) {
	ide, term = d.ideID, d.terminalID
	if ide == "" {
		ide = IDEID
	}
	if term == "" {
		term = TerminalID
	}
	return ide, term
}

// requestUser is whose code the breakpoints and the listener apply to.
func (d *Debugger) requestUser() string {
	if d.listenUser != "" {
		return d.listenUser
	}
	return d.user
}

// ADTSetBreakpoints registers exactly this set of breakpoints, replacing
// whatever this client had registered before, and returns them as SAP resolved
// them — with the ids, the adjusted lines and the object each landed in.
func (d *Debugger) ADTSetBreakpoints(ctx context.Context, bps []adt.Breakpoint) ([]adt.Breakpoint, error) {
	ide, term := d.ideIDs()
	user := d.requestUser()
	if user == "" {
		return nil, fmt.Errorf("a breakpoint needs the user it applies to")
	}
	for i := range bps {
		if bps[i].Kind == "" {
			bps[i].Kind = adt.BreakpointKindLine
		}
		bps[i].Enabled = true
	}
	body, err := adt.BuildBreakpointRequestXML(&adt.BreakpointRequest{
		Scope:           adt.BreakpointScopeExternal,
		DebuggingMode:   adt.DebuggingModeUser,
		User:            user,
		IdeID:           ide,
		TerminalID:      term,
		SystemDebugging: d.systemDebugging,
		Breakpoints:     bps,
	})
	if err != nil {
		return nil, err
	}
	res, err := d.ADT(ctx, "POST", "/sap/bc/adt/debugger/breakpoints",
		[]ADTHeader{{Name: "Content-Type", Value: "application/xml"},
			{Name: "Accept", Value: acceptAnything}}, []byte(body))
	if err != nil {
		return nil, err
	}
	if res.Status != 200 {
		return nil, adtError("breakpoints", res)
	}
	parsed, err := adt.ParseBreakpointResponseXML(res.Body)
	if err != nil {
		return nil, err
	}
	// SAP answers per requested breakpoint, not per request: a line it cannot
	// place comes back with an errorMessage and no id. Dropping those silently
	// is how a caller ends up believing it instrumented a unit it did not —
	// posting sixty candidate lines and receiving six is a normal outcome, since
	// declarations and comments carry no statement to stop at.
	var placed []adt.Breakpoint
	d.bpRejects = nil
	for _, bp := range parsed.Breakpoints {
		if bp.ID == "" {
			d.bpRejects = append(d.bpRejects, bp)
			continue
		}
		placed = append(placed, bp)
	}
	d.bpSet = placed
	return placed, nil
}

// Rejected returns the breakpoints SAP refused to place in the last set, with
// its reason on each. It is the diagnostic that makes blind placement viable:
// send every line that might be a call, keep what stuck, and report the rest.
func (d *Debugger) Rejected() []adt.Breakpoint { return d.bpRejects }

// ADTBreakpoints reads the breakpoints this client has registered.
//
// SAP answers the GET with 200 and an empty body — with a document uri and
// without, on both transports (A4H, 2026-08-21). That is not a bug to work
// around: in ADT the breakpoint set is the IDE's state, and Eclipse posts the
// whole set rather than asking what the server holds. So this returns what this
// session registered, and the GET is still issued in case a release answers it.
//
// The server's own truth is readable, but only over RFC and only with the
// facade: Breakpoints() reads ABDBG_EXTDBPS, which is where these land too.
func (d *Debugger) ADTBreakpoints(ctx context.Context) ([]adt.Breakpoint, error) {
	ide, term := d.ideIDs()
	q := url.Values{}
	q.Set("scope", string(adt.BreakpointScopeExternal))
	q.Set("debuggingMode", string(adt.DebuggingModeUser))
	q.Set("requestUser", d.requestUser())
	q.Set("ideId", ide)
	q.Set("terminalId", term)

	res, err := d.ADT(ctx, "GET", "/sap/bc/adt/debugger/breakpoints?"+q.Encode(),
		[]ADTHeader{{Name: "Accept", Value: acceptAnything}}, nil)
	if err != nil {
		return nil, err
	}
	if res.Status != 200 {
		return nil, adtError("breakpoints", res)
	}
	if len(res.Body) == 0 {
		return d.bpSet, nil
	}
	parsed, err := adt.ParseBreakpointResponseXML(res.Body)
	if err != nil {
		return nil, err
	}
	if len(parsed.Breakpoints) == 0 {
		return d.bpSet, nil
	}
	d.bpSet = parsed.Breakpoints
	return parsed.Breakpoints, nil
}

// ADTDropBreakpoint removes one breakpoint by id. Since the resource is a set,
// it does so by registering everything except that one — a DELETE on the
// individual id is accepted by some releases and ignored by others, and the
// difference is not worth carrying.
func (d *Debugger) ADTDropBreakpoint(ctx context.Context, id string) ([]adt.Breakpoint, error) {
	current, err := d.ADTBreakpoints(ctx)
	if err != nil {
		return nil, err
	}
	var keep []adt.Breakpoint
	found := false
	for _, bp := range current {
		if strings.EqualFold(bp.ID, id) {
			found = true
			continue
		}
		keep = append(keep, bp)
	}
	if !found {
		return current, fmt.Errorf("no breakpoint with id %q is registered", id)
	}
	return d.ADTSetBreakpoints(ctx, keep)
}

// ADTClearBreakpoints removes every breakpoint this client registered.
func (d *Debugger) ADTClearBreakpoints(ctx context.Context) error {
	_, err := d.ADTSetBreakpoints(ctx, nil)
	d.bpSet = nil
	return err
}

// ResolveSourceURI turns an object name into the ADT source URI a line
// breakpoint is addressed by. A breakpoint names a *source document*, and the
// path differs per object type — a function module lives under its group, a
// class under oo/classes — so asking the repository is more reliable than
// guessing from the name, and it also catches the typo case with a clear error.
func (d *Debugger) ResolveSourceURI(ctx context.Context, name string) (string, error) {
	name = strings.ToUpper(strings.TrimSpace(name))
	if strings.HasPrefix(name, "/SAP/BC/ADT/") {
		return name, nil // already a URI
	}
	q := url.Values{}
	q.Set("operation", "quickSearch")
	q.Set("query", name)
	q.Set("maxResults", "20")

	res, err := d.ADT(ctx, "GET", "/sap/bc/adt/repository/informationsystem/search?"+q.Encode(),
		[]ADTHeader{{Name: "Accept", Value: acceptAnything}}, nil)
	if err != nil {
		return "", err
	}
	if res.Status != 200 {
		return "", adtError("search", res)
	}
	results, err := adt.ParseSearchResults(res.Body)
	if err != nil {
		return "", err
	}
	// A name is not unique across object types, and the first match is not
	// necessarily the one with source in it. RFC_SYSTEM_INFO, for instance, is
	// both a DDIC structure and a function module; resolving to the structure
	// yields a URI a breakpoint cannot be placed on, and SAP reports that far
	// away as "Parameter I_MAIN_PROGRAM ... is initial, and therefore invalid".
	//
	// So prefer a result that can actually carry a line breakpoint, and only
	// fall back to a non-source match when there is nothing better.
	var fallback string
	for _, r := range results {
		if !strings.EqualFold(strings.TrimSpace(r.Name), name) {
			continue
		}
		uri := r.URI
		if i := strings.Index(uri, "#"); i >= 0 {
			uri = uri[:i]
		}
		if !strings.Contains(uri, "/source/main") {
			uri = strings.TrimSuffix(uri, "/") + "/source/main"
		}
		if sourceBearingURI(uri) {
			return uri, nil
		}
		if fallback == "" {
			fallback = uri
		}
	}
	if fallback != "" {
		return fallback, nil
	}
	return "", fmt.Errorf("no object named %s in the repository", name)
}

// ADTAdd adds one breakpoint to the set this client already has. The
// read-modify-write is not an optimisation to skip: a bare POST would silently
// drop every other breakpoint the session had registered.
func (d *Debugger) ADTAdd(ctx context.Context, bp adt.Breakpoint) ([]adt.Breakpoint, error) {
	current, err := d.ADTBreakpoints(ctx)
	if err != nil {
		return nil, err
	}
	return d.ADTSetBreakpoints(ctx, append(current, bp))
}

// ADTAddBreakpoint adds a line breakpoint, naming the object rather than its
// ADT URI.
func (d *Debugger) ADTAddBreakpoint(ctx context.Context, object string, line int, condition string) ([]adt.Breakpoint, error) {
	uri, err := d.ResolveSourceURI(ctx, object)
	if err != nil {
		return nil, err
	}
	return d.ADTAdd(ctx, adt.Breakpoint{
		Kind:      adt.BreakpointKindLine,
		URI:       uri,
		Line:      line,
		Condition: condition,
	})
}

// FormatBreakpoints renders a breakpoint set for a terminal.
func FormatBreakpoints(bps []adt.Breakpoint) string {
	if len(bps) == 0 {
		return "no breakpoints registered by this client\n"
	}
	var sb strings.Builder
	for _, bp := range bps {
		where := bp.ObjectName
		if where == "" {
			where = bp.URI
		}
		fmt.Fprintf(&sb, "%-10s %-28s %s\n", bp.Kind, where, bp.ID)
	}
	return sb.String()
}

// sourceBearingURI reports whether an ADT URI names something a line
// breakpoint can sit in. The list is the set of source-bearing resources this
// client already knows how to read; anything else — a DDIC type, a domain, a
// table — has no line to stop on.
func sourceBearingURI(uri string) bool {
	lower := strings.ToLower(uri)
	for _, path := range []string{
		"/functions/groups/",
		"/programs/programs/",
		"/programs/includes/",
		"/oo/classes/",
		"/oo/interfaces/",
		"/behaviordefinitions/",
	} {
		if strings.Contains(lower, path) {
			return true
		}
	}
	return false
}
