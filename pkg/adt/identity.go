package adt

// The identity pin: vsp works only where the operator said it would.
//
// A connection is three facts, and the configuration states them only
// indirectly: a URL (which system answers it is the network's business), a
// client, and credentials that may have come from a stale shell environment
// rather than from where the operator thinks. An HTTP MCP server once logged
// on as the wrong user that way — a 401 against a real person's account and a
// step toward locking it — and nothing said so until SAP did.
//
// A pin (--expect, SAP_EXPECT or "expect" in .vsp.json) names the system, the
// client and the user, as SID[.CLIENT][/USER]. Two checks enforce it:
//
//   - Before any logon: the configured user and client are compared with the
//     pin. A difference is refused without a single request, so the wrong
//     user never reaches SAP and never adds to a failed-logon count.
//   - On the first request: a preflight asks the system who and where it is,
//     before the request that triggered it is sent. A mismatch is refused, and
//     so is every request after it, for the life of the client.
//
// Without a pin none of this runs: no preflight, no extra request.

import (
	"context"
	"encoding/json"
	"encoding/xml"
	"errors"
	"fmt"
	"net/http"
	"regexp"
	"strings"
	"sync"
)

// IdentityPin is where vsp expects to be connected: a system ID, and
// optionally a client and a user. Fields are upper case; empty means "any".
type IdentityPin struct {
	SID    string
	Client string
	User   string
}

var (
	pinSIDRe    = regexp.MustCompile(`^[A-Z][A-Z0-9]{2}$`)
	pinClientRe = regexp.MustCompile(`^[0-9]{3}$`)
)

// ParseIdentityPin reads SID[.CLIENT][/USER], e.g. "A4H.001/TESTUSER",
// "A4H.001", "A4H" or "A4H/TESTUSER". Case does not matter.
func ParseIdentityPin(s string) (*IdentityPin, error) {
	raw := strings.TrimSpace(s)
	if raw == "" {
		return nil, fmt.Errorf("empty identity pin; expected SID[.CLIENT][/USER], e.g. A4H.001/DEVELOPER")
	}
	rest := strings.ToUpper(raw)
	pin := &IdentityPin{}
	if i := strings.Index(rest, "/"); i >= 0 {
		pin.User = strings.TrimSpace(rest[i+1:])
		rest = rest[:i]
		if pin.User == "" || strings.ContainsAny(pin.User, "/ ") {
			return nil, fmt.Errorf("identity pin %q: the user after '/' is empty or malformed; expected SID[.CLIENT][/USER]", raw)
		}
	}
	if i := strings.Index(rest, "."); i >= 0 {
		pin.Client = strings.TrimSpace(rest[i+1:])
		rest = rest[:i]
		if !pinClientRe.MatchString(pin.Client) {
			return nil, fmt.Errorf("identity pin %q: client %q is not three digits; expected SID[.CLIENT][/USER]", raw, pin.Client)
		}
	}
	pin.SID = strings.TrimSpace(rest)
	if !pinSIDRe.MatchString(pin.SID) {
		return nil, fmt.Errorf("identity pin %q: system ID %q is not three letters/digits starting with a letter; expected SID[.CLIENT][/USER]", raw, pin.SID)
	}
	return pin, nil
}

// String renders the pin as it is written: A4H.001/TESTUSER.
func (p IdentityPin) String() string {
	s := p.SID
	if p.Client != "" {
		s += "." + p.Client
	}
	if p.User != "" {
		s += "/" + p.User
	}
	return s
}

// CheckConfigured compares the pin with what is configured to log on, without
// contacting anything. user and client may be empty (a cookie session has no
// user name; an unset client is the system's default), and an empty value is
// not a mismatch: it is checked after logon instead.
func (p IdentityPin) CheckConfigured(user, client string) error {
	user, client = strings.TrimSpace(user), strings.TrimSpace(client)
	if p.User != "" && user != "" && !strings.EqualFold(user, p.User) {
		return &IdentityMismatchError{Pin: p, Detail: fmt.Sprintf(
			"configured to log on as %s, expected %s; no logon was attempted", strings.ToUpper(user), p)}
	}
	if p.Client != "" && client != "" && client != p.Client {
		return &IdentityMismatchError{Pin: p, Detail: fmt.Sprintf(
			"configured for client %s, expected %s; no logon was attempted", client, p)}
	}
	return nil
}

// Identity is who and where a session is, as the system reported it.
type Identity struct {
	SID    string
	Client string
	User   string
	// Source names what answered, for a person reading a refusal.
	Source string
}

// String renders the identity the way a pin is written.
func (id Identity) String() string {
	sid, client, user := id.SID, id.Client, id.User
	if sid == "" {
		sid = "?"
	}
	if client == "" {
		client = "?"
	}
	if user == "" {
		user = "?"
	}
	return fmt.Sprintf("%s.%s as %s", sid, client, user)
}

// Check compares an identity with the pin. A field the pin names but the
// system did not report is a refusal too: a pin is a promise, and one that
// could not be checked has not been kept.
func (p IdentityPin) Check(id Identity) error {
	var unknown []string
	mismatch := false
	cmp := func(want, got, what string) {
		if want == "" {
			return
		}
		if got == "" {
			unknown = append(unknown, what)
			return
		}
		if !strings.EqualFold(want, got) {
			mismatch = true
		}
	}
	cmp(p.SID, id.SID, "system ID")
	cmp(p.Client, id.Client, "client")
	cmp(p.User, id.User, "user")
	switch {
	case mismatch:
		return &IdentityMismatchError{Pin: p, Got: &id, Detail: fmt.Sprintf("connected to %s, expected %s", id, p)}
	case len(unknown) > 0:
		return &IdentityMismatchError{Pin: p, Got: &id, Detail: fmt.Sprintf(
			"could not confirm the %s (connected to %s, expected %s)", strings.Join(unknown, " and "), id, p)}
	}
	return nil
}

// IdentityMismatchError is a refusal by the identity pin. Once returned, the
// client refuses every later request with it as well.
type IdentityMismatchError struct {
	Pin    IdentityPin
	Got    *Identity
	Detail string
}

func (e *IdentityMismatchError) Error() string {
	return "identity pin refused: " + e.Detail + " — nothing further is sent; fix the connection (or the pin) and restart"
}

// IsIdentityMismatch reports whether err is a refusal by the identity pin.
func IsIdentityMismatch(err error) bool {
	var m *IdentityMismatchError
	return errors.As(err, &m)
}

// WithExpect pins the identity the client must find on the other end.
func WithExpect(pin IdentityPin) Option {
	return func(c *Config) {
		p := pin
		c.Expect = &p
	}
}

// identityGate holds the pin's verdict for one transport.
type identityGate struct {
	pin IdentityPin

	mu       sync.Mutex
	verified *Identity
	// verifiedGen is the transport's authentication generation the verdict
	// was reached under. A re-authentication (SSO refresh, a reloaded cookie
	// file, new cookies) can put another user's session behind the same
	// transport, so a verdict from an older generation is checked again.
	verifiedGen uint64
	refused     error
	probe       func(ctx context.Context) (*Identity, error)
}

// preflightKey marks the context of the preflight's own requests, so they are
// not gated by the check they implement. The value is the transport, so a
// context that strays into another transport is still checked there.
type preflightKey struct{}

// checkIdentity runs before every request the transport sends. Without a pin
// it does nothing at all.
func (t *Transport) checkIdentity(ctx context.Context) error {
	_, err := t.checkIdentityGen(ctx)
	return err
}

// inPreflight reports whether ctx belongs to this transport's own preflight.
func (t *Transport) inPreflight(ctx context.Context) bool {
	owner, _ := ctx.Value(preflightKey{}).(*Transport)
	return owner == t
}

// checkIdentityGen is checkIdentity that also returns the authentication
// generation the verdict holds for: the credentials of exactly that
// generation are the ones that may be sent.
func (t *Transport) checkIdentityGen(ctx context.Context) (uint64, error) {
	g := t.identity
	if g == nil || t.inPreflight(ctx) {
		return t.authGen.Load(), nil
	}
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.refused != nil {
		return 0, g.refused
	}
	if g.verified != nil && g.verifiedGen == t.authGen.Load() {
		return g.verifiedGen, nil
	}
	g.verified = nil
	// The configured logon first: this costs nothing and keeps a wrong user
	// from ever reaching SAP.
	if err := g.pin.CheckConfigured(t.config.Username, t.config.Client); err != nil {
		g.refused = err
		return 0, err
	}
	probe := g.probe
	if probe == nil {
		// A transport used without a Client (the debugger's stateful
		// session) still gets the full preflight, fallbacks included.
		probe = (&Client{transport: t, config: t.config}).probeIdentity
	}
	// The answer has to describe the session that is current when it
	// arrives; when the session changed under the probe (it re-authenticated)
	// it is asked again, a bounded number of times.
	for attempt := 0; attempt < 3; attempt++ {
		gen := t.authGen.Load()
		id, err := probe(context.WithValue(ctx, preflightKey{}, t))
		if err != nil {
			if IsIdentityMismatch(err) {
				g.refused = err
				return 0, err
			}
			// A password SAP refused is refused again on every retry, and each
			// one counts toward locking the account: the first 401 is final.
			var apiErr *APIError
			if errors.As(err, &apiErr) && apiErr.StatusCode == http.StatusUnauthorized && t.config.HasBasicAuth() {
				g.refused = fmt.Errorf("identity pin %s: SAP refused the logon of %s (401); not retried, because every failed logon counts toward locking the account — fix the password and restart: %w",
					g.pin, strings.ToUpper(t.config.Username), err)
				return 0, g.refused
			}
			// A network failure proves nothing about who is on the other end;
			// the next request asks again.
			return 0, fmt.Errorf("identity pin %s: preflight failed, nothing else was sent: %w", g.pin, err)
		}
		if err := g.pin.Check(*id); err != nil {
			g.refused = err
			return 0, err
		}
		if t.authGen.Load() == gen {
			g.verified, g.verifiedGen = id, gen
			return gen, nil
		}
	}
	return 0, fmt.Errorf("identity pin %s: the session kept changing during the preflight; nothing else was sent", g.pin)
}

// credentialsChanged says the transport now authenticates differently: new
// cookies, a refreshed SSO session, a reloaded cookie file, a session cookie
// the server reissued. The identity pin checks the new session before the next
// request goes out. Callers hold cookiesMu for writing, so a reader holding it
// sees cookies and generation change together.
func (t *Transport) credentialsChanged() {
	t.authGen.Add(1)
}

// admit holds req to the identity pin: the pin must hold for the current
// credentials, and req must carry exactly those. A request built before the
// credentials changed (a reauth, a cookie the preflight's answer reissued) has
// its configured cookies replaced with the verified ones before it goes out.
// Without a pin it does nothing.
func (t *Transport) admit(req *http.Request) error {
	if t.identity == nil || t.inPreflight(req.Context()) {
		return nil
	}
	for attempt := 0; attempt < 3; attempt++ {
		gen, err := t.checkIdentityGen(req.Context())
		if err != nil {
			return err
		}
		if t.syncCookies(req, gen) {
			return nil
		}
	}
	return fmt.Errorf("identity pin %s: the session kept changing; nothing was sent", t.identity.pin)
}

// syncCookies puts the configured cookies of generation gen on req, and
// reports false when the credentials have moved past gen.
func (t *Transport) syncCookies(req *http.Request, gen uint64) bool {
	t.cookiesMu.RLock()
	defer t.cookiesMu.RUnlock()
	if t.authGen.Load() != gen {
		return false
	}
	if len(t.config.Cookies) == 0 {
		return true
	}
	cookies := req.Cookies()
	seen := map[string]bool{}
	changed := false
	for _, c := range cookies {
		seen[c.Name] = true
		if v, ok := t.config.Cookies[c.Name]; ok && c.Value != "" && v != c.Value {
			c.Value, changed = v, true
		}
	}
	for name, value := range t.config.Cookies {
		if !seen[name] {
			cookies, changed = append(cookies, &http.Cookie{Name: name, Value: value}), true
		}
	}
	if changed {
		req.Header.Del("Cookie")
		for _, c := range cookies {
			req.AddCookie(c)
		}
	}
	return true
}

// verifiedCredentials is what a WebSocket built from c logs on with: the pin
// is verified, and the credentials are those of the generation it was verified
// for, read together under the lock that every change takes.
func (c *Client) verifiedCredentials(ctx context.Context) (user, password string, cookies map[string]string, err error) {
	t := c.transport
	for attempt := 0; attempt < 3; attempt++ {
		gen, verr := t.checkIdentityGen(ctx)
		if verr != nil {
			return "", "", nil, verr
		}
		t.cookiesMu.RLock()
		if t.authGen.Load() == gen {
			if len(t.config.Cookies) > 0 {
				cookies = cloneCookies(t.config.Cookies)
				user = c.config.Username
			} else {
				user, password = c.config.Username, c.config.Password
			}
			t.cookiesMu.RUnlock()
			return user, password, cookies, nil
		}
		t.cookiesMu.RUnlock()
	}
	return "", "", nil, fmt.Errorf("identity pin %s: the session kept changing; the WebSocket was not opened", t.identity.pin)
}

// IdentityStatus reports the pin and its verdict so far: pinned is false when
// there is no pin; verified is the identity found when it matched; refused is
// the refusal when it did not. All empty means not yet checked.
func (c *Client) IdentityStatus() (pin *IdentityPin, verified *Identity, refused error) {
	if c == nil || c.transport == nil || c.transport.identity == nil {
		return nil, nil, nil
	}
	g := c.transport.identity
	g.mu.Lock()
	defer g.mu.Unlock()
	p := g.pin
	if g.verified != nil && g.verifiedGen != c.transport.authGen.Load() {
		return &p, nil, g.refused
	}
	return &p, g.verified, g.refused
}

// VerifyIdentity runs the preflight now if it has not run, and returns the
// pin's verdict. It is free without a pin and after a verdict, and is what a
// caller about to reach SAP by another route (WebSocket, RFC) uses first.
func (c *Client) VerifyIdentity(ctx context.Context) error {
	if c == nil || c.transport == nil {
		return nil
	}
	return c.transport.checkIdentity(ctx)
}

// systemInformationPath answers who and where in one GET: system ID, client
// and user name, as JSON. Eclipse ADT reads it right after logon.
const systemInformationPath = "/sap/bc/adt/core/http/systeminformation"

// errNoSystemInformation says the resource is not there, as opposed to the
// connection or the logon failing.
var errNoSystemInformation = errors.New("system information resource not available")

func (t *Transport) systemInformation(ctx context.Context) (*Identity, error) {
	resp, err := t.request(ctx, systemInformationPath, &RequestOptions{
		Method: http.MethodGet,
		Accept: "application/vnd.sap.adt.core.http.systeminformation.v1+json, application/json",
		// One wrong password, one failed logon: the 401 comes straight back.
		noBasicAuthRetry: true,
	})
	if err != nil {
		var apiErr *APIError
		if errors.As(err, &apiErr) && (apiErr.StatusCode == http.StatusNotFound || apiErr.StatusCode == http.StatusNotAcceptable ||
			apiErr.StatusCode == http.StatusMethodNotAllowed || apiErr.StatusCode == http.StatusNotImplemented) {
			return nil, errNoSystemInformation
		}
		return nil, err
	}
	var raw map[string]any
	if err := json.Unmarshal(resp.Body, &raw); err != nil {
		return nil, errNoSystemInformation
	}
	field := func(names ...string) string {
		for k, v := range raw {
			for _, n := range names {
				if strings.EqualFold(k, n) {
					if s, ok := v.(string); ok {
						return strings.ToUpper(strings.TrimSpace(s))
					}
				}
			}
		}
		return ""
	}
	id := &Identity{
		SID:    field("systemID", "systemId", "sid"),
		Client: field("client"),
		User:   field("userName", "user"),
		Source: systemInformationPath,
	}
	if id.SID == "" && id.Client == "" && id.User == "" {
		return nil, errNoSystemInformation
	}
	return id, nil
}

// logsysSIDRe reads the system ID out of a logical system name that follows
// SAP's own convention, <SID>CLNT<client>. Anything else is not guessed at.
var logsysSIDRe = regexp.MustCompile(`^([A-Z][A-Z0-9]{2})CLNT[0-9]{3}$`)

// probeIdentity asks the system who and where this session is: the system
// information resource when the release has it, otherwise what SAP(info) and
// vsp info already use — T000 for the client and its logical system (whose
// name carries the SID), the transport organizer for the user of a cookie
// session, and the configured user for a password logon SAP accepted.
func (c *Client) probeIdentity(ctx context.Context) (*Identity, error) {
	id, err := c.transport.systemInformation(ctx)
	if err != nil && !errors.Is(err, errNoSystemInformation) {
		return nil, err
	}
	pin := c.transport.identity.pin
	if id == nil {
		id = &Identity{Source: "T000 and the transport organizer"}
		// vsp's own fixed statement, not caller SQL: the free-SQL safety
		// switch is about what an agent may run, so it is not consulted.
		res, qerr := c.runQueryRaw(ctx, "SELECT MANDT, LOGSYS FROM T000 WHERE MANDT = '"+c.config.Client+"'", 1)
		if qerr != nil {
			return nil, fmt.Errorf("reading T000: %w", qerr)
		}
		if len(res.Rows) > 0 {
			row := res.Rows[0]
			if v, ok := row["MANDT"].(string); ok {
				id.Client = strings.TrimSpace(v)
			}
			if v, ok := row["LOGSYS"].(string); ok {
				if m := logsysSIDRe.FindStringSubmatch(strings.ToUpper(strings.TrimSpace(v))); m != nil {
					id.SID = m[1]
				}
			}
		}
	}
	// The sap-client every request carries is the client of the session.
	if id.Client == "" {
		id.Client = strings.TrimSpace(c.config.Client)
	}
	if id.User == "" && pin.User != "" {
		if c.config.HasBasicAuth() {
			// SAP accepted this user's password: the session is this user's.
			id.User = strings.ToUpper(c.config.Username)
		} else {
			user, werr := c.whoAmI(ctx)
			if werr != nil {
				return nil, fmt.Errorf("asking the system for the session's user: %w", werr)
			}
			id.User = user
		}
	}
	return id, nil
}

// whoAmI names the user of the session through the transport organizer: the
// tree of one's own requests is rooted at one's own name, even when empty.
// saprfc.CurrentUser does the same over its own transport; this package cannot
// import that one.
func (c *Client) whoAmI(ctx context.Context) (string, error) {
	resp, err := c.transport.request(ctx, "/sap/bc/adt/cts/transportrequests", &RequestOptions{
		Method: http.MethodGet,
		Accept: "*/*",
		Query: map[string][]string{
			"_action": {"FIND"}, "trfunction": {"K"}, "trstatus": {"D"}, "targetsystem": {""},
		},
	})
	if err != nil {
		return "", err
	}
	var root struct {
		Name      string `xml:"name,attr"`
		CreatedBy string `xml:"createdBy,attr"`
		ChangedBy string `xml:"changedBy,attr"`
	}
	if err := xml.Unmarshal(resp.Body, &root); err != nil {
		return "", fmt.Errorf("reading the transport organizer: %w", err)
	}
	for _, s := range []string{root.Name, root.CreatedBy, root.ChangedBy} {
		if s = strings.ToUpper(strings.TrimSpace(s)); s != "" {
			return s, nil
		}
	}
	return "", fmt.Errorf("the transport organizer answered without naming a user")
}
