package config

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"time"
)

// SNCSettings reaches a system over SNC (Kerberos single sign-on) through
// SAP's own sapnwrfc.dll, loaded by vsp itself on Windows. vsp then carries
// ADT over RFC (SADT_REST_RFC_ENDPOINT): it runs itself as `vsp snc-serve`,
// as the system's transport command. The channel is read-only.
//
// The block names a DLL that vsp will load, so it is honoured only where
// transport_cmd is: in ~/.vsp.json or ~/.vsp/systems.json.
type SNCSettings struct {
	// DLL is the absolute path to sapnwrfc.dll (SAP NW RFC SDK, x64). Empty
	// means sapnwrfc.dll beside vsp.exe.
	DLL string `json:"dll,omitempty"`
	// SNCLib is the absolute path to the SNC library, e.g. gx64krb5.dll.
	// Empty means SNC_LIB_64 as SAP GUI sets it, read from the user's or the
	// machine's environment in the registry, never from the process: vsp
	// loads ./.env into the process, and a project must not pick the DLL.
	// SNC_LIB is not read: it names the 32-bit library.
	SNCLib string `json:"snc_lib,omitempty"`
	// Connection is the exact name of the SAP Logon entry to connect with.
	Connection string `json:"connection"`
	// Landscape is the SAPUILandscape.xml to read; empty means SAP GUI's own
	// (SAPLOGON_LSXML_FILE, else %APPDATA%\SAP\Common).
	Landscape string `json:"landscape,omitempty"`

	// System, Client and User are where the logon must land. The connection
	// is refused unless the authenticated identity matches all three.
	System string `json:"system"`
	Client string `json:"client"`
	User   string `json:"user"`

	// LogonTimeout bounds the logon (default 45s, 1s..2m) and RequestTimeout
	// one ADT request (default 60s, 1s..10m), as Go durations.
	LogonTimeout   string `json:"logon_timeout,omitempty"`
	RequestTimeout string `json:"request_timeout,omitempty"`

	// AllowDataPreview also forwards bounded ADT data preview SELECTs.
	AllowDataPreview bool `json:"allow_data_preview,omitempty"`
	// Verbose writes one stderr line per request.
	Verbose bool `json:"verbose,omitempty"`
}

// SNCServeCommand is the hidden vsp subcommand an snc block runs.
const SNCServeCommand = "snc-serve"

// SNCPlaceholderURL is the URL an snc system gets when it names none: ADT
// paths and the Host header are built from it, nothing connects to it.
const SNCPlaceholderURL = "https://snc.invalid"

// Seams for tests.
var (
	sncGOOS       = runtime.GOOS
	sncExecutable = os.Executable
	sncLib64      = sncLib64FromRegistry
)

var (
	sncSystemRe = regexp.MustCompile(`^[A-Z0-9]{3}$`)
	sncClientRe = regexp.MustCompile(`^[0-9]{3}$`)
	sncUserRe   = regexp.MustCompile(`^[A-Za-z0-9_@.\-]{1,12}$`)
)

// applySNC checks a system's snc block and, when it is usable, turns it into
// the system's transport command: this vsp executable, run as snc-serve. It
// also pins the system (expect), fills the client and the placeholder URL,
// and marks the system read-only, because snc-serve forwards reads only.
func (c *SystemsConfig) applySNC(name string, sys *SystemConfig) error {
	s := sys.SNC
	if s == nil {
		return nil
	}
	if err := checkTrustedHomeConfig(c.source); err != nil {
		where := c.source
		if where == "" {
			where = "a config not read from a file"
		}
		return fmt.Errorf("system '%s': snc is refused in %s (%v); it is honoured only in ~/.vsp.json or ~/.vsp/systems.json, because a project file must not be able to make vsp load a DLL", name, where, err)
	}
	if sncGOOS != "windows" {
		return fmt.Errorf("system '%s': snc (sapnwrfc.dll) works only in vsp.exe on Windows; elsewhere use transport_cmd with an external helper", name)
	}
	if len(sys.TransportCmd) > 0 {
		return fmt.Errorf("system '%s': snc and transport_cmd are exclusive; snc sets the transport command itself", name)
	}
	if sys.User != "" || sys.Password != "" || sys.CookieFile != "" || sys.CookieString != "" || sys.UsesSSO() {
		return fmt.Errorf("system '%s': snc logs on with Kerberos single sign-on; remove user/password/cookies/auth", name)
	}
	exe, err := sncExecutable()
	if err != nil {
		return fmt.Errorf("system '%s': snc: cannot locate the vsp executable: %w", name, err)
	}
	filled := *s // the loaded config keeps what the file said
	if filled.DLL == "" {
		filled.DLL = filepath.Join(filepath.Dir(exe), "sapnwrfc.dll")
	}
	if filled.SNCLib == "" {
		filled.SNCLib = sncLib64()
		if filled.SNCLib == "" {
			return fmt.Errorf("system '%s': snc: snc_lib is not set and SNC_LIB_64 is not in the user or machine environment as a plain path; name the 64-bit SNC library (e.g. gx64krb5.dll)", name)
		}
	}
	argv, err := filled.argv()
	if err != nil {
		return fmt.Errorf("system '%s': snc: %w", name, err)
	}
	if sys.Client != "" && sys.Client != s.Client {
		return fmt.Errorf("system '%s': client %q differs from snc.client %q", name, sys.Client, s.Client)
	}
	sys.TransportCmd = append([]string{exe}, argv...)
	sys.Client = s.Client
	if sys.URL == "" {
		sys.URL = SNCPlaceholderURL
	}
	if sys.Expect == "" {
		sys.Expect = s.DefaultExpect()
	}
	sys.ReadOnly = true
	return nil
}

// DefaultExpect is the pin an snc system gets when it names none: the
// identity the logon must land on.
func (s *SNCSettings) DefaultExpect() string {
	return s.System + "." + s.Client + "/" + strings.ToUpper(s.User)
}

// argv validates the block and returns the snc-serve arguments (without the
// executable).
func (s *SNCSettings) argv() ([]string, error) {
	if !sncSystemRe.MatchString(s.System) {
		return nil, errors.New("system must be a three-character SAP system ID in capitals")
	}
	if !sncClientRe.MatchString(s.Client) {
		return nil, errors.New("client must be three digits")
	}
	if !sncUserRe.MatchString(s.User) {
		return nil, errors.New("user must be the SAP user the logon must land on")
	}
	if strings.TrimSpace(s.Connection) == "" {
		return nil, errors.New("connection must name the SAP Logon entry")
	}
	if !filepath.IsAbs(s.DLL) || !strings.EqualFold(filepath.Base(s.DLL), "sapnwrfc.dll") {
		return nil, errors.New("dll must be the absolute path of sapnwrfc.dll")
	}
	if !filepath.IsAbs(s.SNCLib) {
		return nil, errors.New("snc_lib must be an absolute path")
	}
	if s.Landscape != "" && !filepath.IsAbs(s.Landscape) {
		return nil, errors.New("landscape must be an absolute path")
	}
	args := []string{SNCServeCommand,
		"-system", s.System, "-client", s.Client, "-user", s.User,
		"-connection", s.Connection, "-dll", s.DLL, "-snc-lib", s.SNCLib}
	if s.Landscape != "" {
		args = append(args, "-landscape", s.Landscape)
	}
	for _, d := range []struct {
		key, flag, value string
		min, max         time.Duration
	}{
		{"logon_timeout", "-timeout", s.LogonTimeout, time.Second, 2 * time.Minute},
		{"request_timeout", "-request-timeout", s.RequestTimeout, time.Second, 10 * time.Minute},
	} {
		if d.value == "" {
			continue
		}
		v, err := time.ParseDuration(d.value)
		if err != nil || v < d.min || v > d.max {
			return nil, fmt.Errorf("%s must be a duration between %s and %s", d.key, d.min, d.max)
		}
		args = append(args, d.flag, v.String())
	}
	if s.AllowDataPreview {
		args = append(args, "-allow-data-preview")
	}
	if s.Verbose {
		args = append(args, "-verbose")
	}
	return args, nil
}

// SetSNCPlatform replaces the operating system, the vsp executable and the
// SNC_LIB_64 lookup that snc blocks are checked against, and returns a
// function that restores them. It exists for tests outside this package.
func SetSNCPlatform(goos string, executable func() (string, error), lib64 func() string) (restore func()) {
	prevGOOS, prevExe, prevLib := sncGOOS, sncExecutable, sncLib64
	sncGOOS, sncExecutable, sncLib64 = goos, executable, lib64
	return func() { sncGOOS, sncExecutable, sncLib64 = prevGOOS, prevExe, prevLib }
}
