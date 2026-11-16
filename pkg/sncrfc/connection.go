package sncrfc

import (
	"errors"
	"fmt"
	"regexp"
	"strings"
	"sync"
)

// Identity contains only the authenticated user and approved logical target.
type Identity struct {
	System string `json:"system"`
	Client string `json:"client"`
	User   string `json:"user"`
}

// Options are kept in memory. Never serialize Parameters or include them in logs.
// This first client supports SNC SSO only; no credentials, fallback or retries.
type Options struct {
	DLL                                      string
	System, Client, Connection, ExpectedUser string
	Parameters                               map[string]string
}

var sidPattern = regexp.MustCompile(`^[A-Z0-9]{3}$`)
var clientPattern = regexp.MustCompile(`^[0-9]{3}$`)
var userPattern = regexp.MustCompile(`^[A-Za-z0-9_@.\-]{1,12}$`)

func (o Options) validate() error {
	if !sidPattern.MatchString(o.System) || !clientPattern.MatchString(o.Client) || strings.TrimSpace(o.Connection) == "" || !userPattern.MatchString(o.ExpectedUser) {
		return errors.New("require exact system, client, connection and expected SAP user")
	}
	allowed := map[string]bool{"ashost": true, "sysnr": true, "mshost": true, "msserv": true, "r3name": true, "group": true, "saprouter": true, "client": true, "snc_mode": true, "snc_sso": true, "snc_qop": true, "snc_partnername": true, "snc_lib": true, "trace": true}
	for k, v := range o.Parameters {
		if !allowed[k] || strings.ContainsRune(v, 0) || len(v) > 4096 {
			return errors.New("unsupported or invalid RFC parameter")
		}
	}
	p := o.Parameters
	if p["client"] != o.Client || p["snc_mode"] != "1" || p["snc_sso"] != "1" || p["trace"] != "0" || strings.TrimSpace(p["snc_partnername"]) == "" || strings.TrimSpace(p["snc_lib"]) == "" {
		return errors.New("SNC SSO, matching client and disabled tracing are required")
	}
	switch p["snc_qop"] {
	case "1", "2", "3", "9":
	default:
		return errors.New("require explicit SNC quality of protection from the selected profile")
	}
	direct := p["ashost"] != "" && regexp.MustCompile(`^[0-9]{2}$`).MatchString(p["sysnr"])
	balanced := p["mshost"] != "" && p["msserv"] != "" && p["r3name"] == o.System && p["group"] != ""
	if direct == balanced || (direct && (p["mshost"] != "" || p["msserv"] != "" || p["group"] != "" || p["r3name"] != "")) || (balanced && (p["ashost"] != "" || p["sysnr"] != "")) {
		return errors.New("require one exact direct or message-server connection")
	}
	return nil
}

type nativeAPI interface {
	open(map[string]string) (uintptr, error)
	attributes(uintptr) (Identity, error)
	close(uintptr) error
	release() error
}

// Connection is returned only after checking the actual post-logon identity.
// Function calls are restricted to the bounded read-only bootstrap/discovery API.
type Connection struct {
	mu          sync.Mutex
	api         nativeAPI
	handle      uintptr
	identity    Identity
	closeError  error
	callError   error
	dataPreview bool
}

// EnableDataPreview lets Call forward bounded ADT data preview POSTs (SELECT
// only). It is off unless the caller asks for it.
func (c *Connection) EnableDataPreview() {
	c.mu.Lock()
	c.dataPreview = true
	c.mu.Unlock()
}

func Open(o Options) (*Connection, error) { return openWith(o, loadNative) }

func openWith(o Options, load func(string) (nativeAPI, error)) (*Connection, error) {
	if err := o.validate(); err != nil {
		return nil, err
	}
	a, err := load(o.DLL)
	if err != nil {
		return nil, safeError("load RFC library", err)
	}
	h, err := a.open(o.Parameters)
	if err != nil || h == 0 {
		if h != 0 {
			_ = a.close(h)
		}
		_ = a.release()
		return nil, safeError("SNC logon", err)
	}
	c := &Connection{api: a, handle: h}
	id, err := a.attributes(h)
	if err != nil {
		cleanup := c.Close()
		return nil, errors.Join(safeError("read authenticated identity", err), cleanup)
	}
	if id.System != o.System || id.Client != o.Client || id.User == "" || !strings.EqualFold(id.User, o.ExpectedUser) {
		cleanup := c.Close()
		return nil, errors.Join(errors.New("authenticated SAP identity does not match the approved target/user"), cleanup)
	}
	c.identity = id
	return c, nil
}

func (c *Connection) Identity() Identity { return c.identity }

func (c *Connection) Close() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.handle == 0 {
		return c.closeError
	}
	h := c.handle
	c.handle = 0
	err := c.api.close(h)
	if err != nil {
		// A failed close may leave SDK activity alive. Retain the DLL until
		// process exit; never unload code beneath it or retry the denied call.
		c.closeError = errors.Join(c.closeError, safeError("close RFC connection", err))
		return c.closeError
	}
	if c.closeError != nil {
		// An earlier close failed; the DLL must stay loaded until process exit.
		return c.closeError
	}
	releaseErr := c.api.release()
	if releaseErr != nil {
		c.closeError = safeError("release RFC library", releaseErr)
		return c.closeError
	}
	return nil
}

// Native messages can contain hosts, partners and SAP content. Only numeric
// RFC codes/groups leave this boundary; arbitrary wrapped errors are discarded.
type NativeError struct{ Code, Group uint32 }

func (e *NativeError) Error() string { return fmt.Sprintf("RFC code=%d group=%d", e.Code, e.Group) }
func safeError(stage string, err error) error {
	var native *NativeError
	if errors.As(err, &native) {
		return fmt.Errorf("%s: %w", stage, &NativeError{Code: native.Code, Group: native.Group})
	}
	return fmt.Errorf("%s failed", stage)
}
