// Package saprfc bridges vsp's system configuration to classic SAP RFC, using
// the SDK-free open-rfc-go client. An ADT system is described by an HTTP URL;
// RFC needs a gateway host and port instead, so the host defaults to the URL's
// host and the port to the gateway of the instance (3300 + system number). Both
// can be overridden per system (rfc_host / rfc_sysnr / rfc_port) or per command.
package saprfc

import (
	"context"
	"fmt"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/oisee/open-rfc-go/rfc"

	"github.com/oisee/vibing-steampunk/pkg/adt"
)

// Params is a resolved RFC destination.
type Params struct {
	Host     string // gateway host
	Sysnr    string // two-digit instance number
	Port     int    // gateway port (3300 + sysnr unless overridden)
	Client   string
	User     string
	Password Secret
	Language string

	// Expect is the identity pin (see adt.IdentityPin). With one, Open
	// refuses a logon user or client that contradicts it before dialling,
	// and after logon refuses a system whose RFC_SYSTEM_INFO names another
	// SID. Nil: no check and no extra call.
	Expect *adt.IdentityPin
}

// Secret is a string that will not print itself. A logon password reaches a log
// or an error message by accident, never on purpose: one %v on a struct that
// happens to contain it is enough, and the struct grows the field long after
// the format string was written. Making the type refuse to render closes that
// whole class at compile time, and costs a conversion at the two places where
// the value is genuinely needed.
type Secret string

const redacted = "[redacted]"

func (Secret) String() string               { return redacted }
func (Secret) GoString() string             { return redacted }
func (Secret) MarshalText() ([]byte, error) { return []byte(redacted), nil }
func (Secret) MarshalJSON() ([]byte, error) { return []byte(`"` + redacted + `"`), nil }

// Reveal returns the secret itself. Call it only where the value is handed to
// the protocol — never into a log, an error, or a rendered structure.
func (s Secret) Reveal() string { return string(s) }

// Input carries what vsp knows about a system plus any explicit overrides.
// Overrides win over the system's RFC settings, which win over the URL.
type Input struct {
	URL      string // ADT base URL, e.g. http://a4h.example:50000
	User     string
	Password string
	Client   string
	Language string

	// Per-system RFC settings (.vsp.json), including credentials that already
	// resolve from the RFC environment (SAP_USER/SAP_PASSWORD).
	RFCHost     string
	RFCSysnr    string
	RFCPort     int
	RFCUser     string
	RFCPassword string

	// Per-command overrides (flags).
	HostFlag  string
	SysnrFlag string
	PortFlag  int
	UserFlag  string

	// Expect is carried into Params.Expect.
	Expect *adt.IdentityPin
}

// Resolve turns an Input into RFC destination parameters.
func Resolve(in Input) (Params, error) {
	host := firstNonEmpty(in.HostFlag, in.RFCHost)
	sysnr := firstNonEmpty(in.SysnrFlag, in.RFCSysnr)

	if host == "" || sysnr == "" {
		uHost, uSysnr := fromURL(in.URL)
		if host == "" {
			host = uHost
		}
		if sysnr == "" {
			sysnr = uSysnr
		}
	}
	if host == "" {
		return Params{}, fmt.Errorf("no RFC host: set rfc_host in .vsp.json, pass --rfc-host, or configure the system URL")
	}
	if sysnr == "" {
		sysnr = "00"
	}
	n, err := strconv.Atoi(sysnr)
	if err != nil || n < 0 || n > 99 {
		return Params{}, fmt.Errorf("invalid system number %q (expected 00..99)", sysnr)
	}

	port := in.PortFlag
	if port == 0 {
		port = in.RFCPort
	}
	if port == 0 {
		port = 3300 + n // the instance's gateway
	}

	lang := strings.ToUpper(firstNonEmpty(in.Language, "EN"))
	// RFC logon may differ from the ADT logon: an explicit flag wins, then the
	// system's rfc_user/rfc_password (which already fall back to SAP_USER/
	// SAP_PASSWORD), then the ADT credentials for the same system.
	user := firstNonEmpty(in.UserFlag, in.RFCUser, in.User)
	password := firstNonEmpty(in.RFCPassword, in.Password)
	if user == "" || password == "" {
		return Params{}, fmt.Errorf("no RFC credentials: set rfc_user/rfc_password in .vsp.json, SAP_USER/SAP_PASSWORD, or the system's user/password")
	}
	return Params{
		Host:     host,
		Sysnr:    fmt.Sprintf("%02d", n),
		Port:     port,
		Client:   firstNonEmpty(in.Client, "001"),
		User:     user,
		Password: Secret(password),
		Language: lang[:1],
		Expect:   in.Expect,
	}, nil
}

// Open dials an RFC client for the resolved parameters.
func Open(ctx context.Context, p Params) (*rfc.Client, error) {
	return OpenWithTimeout(ctx, p, 0)
}

// OpenWithTimeout dials an RFC client that allows a single call to take up to
// timeout (zero keeps the library default). Raise it for calls that block
// server-side on purpose — a debugger listener holds its conversation for as
// long as it waits, and the client must not give up before the server does.
func OpenWithTimeout(ctx context.Context, p Params, timeout time.Duration) (*rfc.Client, error) {
	// The identity pin, before dialling: an RFC user or client that is not
	// the pinned one (an rfc_user, --rfc-user or a per-call user) never logs on.
	if p.Expect != nil {
		if err := p.Expect.CheckConfigured(p.User, p.Client); err != nil {
			return nil, fmt.Errorf("RFC logon: %w", err)
		}
	}
	c, err := dialRFC(ctx, p, timeout)
	if err != nil || p.Expect == nil {
		return c, err
	}
	// After logon, before any other call: the gateway may belong to another
	// system (another host or instance than the pin's). The logon itself
	// fixes client and user; RFC_SYSTEM_INFO says which system this is.
	sid, err := rfcSystemID(ctx, c)
	if err != nil {
		closeRFC(ctx, c)
		return nil, fmt.Errorf("identity pin %s: RFC_SYSTEM_INFO after logon failed, nothing else was called: %w", p.Expect, err)
	}
	id := adt.Identity{SID: strings.ToUpper(strings.TrimSpace(sid)), Client: p.Client, User: strings.ToUpper(p.User), Source: "RFC_SYSTEM_INFO"}
	if err := p.Expect.Check(id); err != nil {
		closeRFC(ctx, c)
		return nil, fmt.Errorf("RFC %s:%d: %w", p.Host, p.Port, err)
	}
	return c, nil
}

// The steps of a pinned logon, as variables so a test can stand in for a
// gateway.
var (
	dialRFC     = openRFC
	rfcSystemID = func(ctx context.Context, c *rfc.Client) (string, error) {
		r, err := c.Call(ctx, "RFC_SYSTEM_INFO", nil)
		if err != nil {
			return "", err
		}
		m, _ := r.Get("RFCSI_EXPORT").(map[string]any)
		return str(m["RFCSYSID"]), nil
	}
	closeRFC = func(ctx context.Context, c *rfc.Client) { _ = c.Close(ctx) }
)

func openRFC(ctx context.Context, p Params, timeout time.Duration) (*rfc.Client, error) {
	n, _ := strconv.Atoi(p.Sysnr)
	return rfc.Open(ctx, rfc.Destination{
		OperationTimeout: timeout,
		Host:             p.Host,
		Port:             p.Port,
		Service:          fmt.Sprintf("sapdp%02d", n),
		Client:           p.Client,
		User:             p.User,
		Password:         p.Password.Reveal(),
		Language:         p.Language,
	})
}

// fromURL extracts the host and, where the port follows a standard AS ABAP
// convention, the instance number: 80NN and 443NN (ICM HTTP/HTTPS) and 5NN00
// (the port layout used by the ABAP developer editions).
// SysnrFromURL derives the host and system number from an ADT base URL.
//
// Exported because two callers need the same derivation and the second one
// nearly reimplemented it: the rule is a convention about ICM ports, not a
// fact the system told us, and a second copy would be a second place to get
// the ranges wrong. Callers that show the number to a person should say it was
// derived.
func SysnrFromURL(raw string) (host, sysnr string) { return fromURL(raw) }

func fromURL(raw string) (host, sysnr string) {
	if raw == "" {
		return "", ""
	}
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" {
		return "", ""
	}
	host = u.Hostname()
	port, err := strconv.Atoi(u.Port())
	if err != nil {
		return host, ""
	}
	switch {
	case port >= 8000 && port <= 8099:
		sysnr = fmt.Sprintf("%02d", port-8000)
	case port >= 44300 && port <= 44399:
		sysnr = fmt.Sprintf("%02d", port-44300)
	case port >= 50000 && port <= 59999 && port%100 == 0:
		sysnr = fmt.Sprintf("%02d", (port/100)%100)
	}
	return host, sysnr
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if v != "" {
			return v
		}
	}
	return ""
}
