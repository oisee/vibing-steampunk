package serve

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/oisee/vibing-steampunk/pkg/sncrfc"
	"github.com/oisee/vibing-steampunk/pkg/sncrfc/landscape"
)

type probeFlags struct {
	system, client, connection, user, dll, sncLib, landscape string
	timeout                                                  time.Duration
}
type probeSession interface {
	Identity() sncrfc.Identity
	Close() error
}
type probeResult struct {
	OK         bool             `json:"ok"`
	Identity   *sncrfc.Identity `json:"identity,omitempty"`
	SNC        bool             `json:"snc,omitempty"`
	Stage      string           `json:"stage,omitempty"`
	Reason     string           `json:"reason,omitempty"`
	Code       *uint32          `json:"rfc_code,omitempty"`
	Group      *uint32          `json:"rfc_group,omitempty"`
	Diagnostic string           `json:"diagnostic_code,omitempty"`
	Include    bool             `json:"include,omitempty"`
	HTTPStatus int              `json:"http_status,omitempty"`
}
type probeFailure struct{ stage, reason string }

func (e *probeFailure) Error() string { return e.reason }

func defineProbeFlags(f *flag.FlagSet, p *probeFlags) {
	f.StringVar(&p.system, "system", "", "exact approved SAP system")
	f.StringVar(&p.client, "client", "", "exact approved SAP client")
	f.StringVar(&p.connection, "connection", "", "exact SAP Logon entry name")
	f.StringVar(&p.user, "user", "", "expected authenticated SAP user")
	f.StringVar(&p.dll, "dll", "", "absolute local sapnwrfc.dll path")
	f.StringVar(&p.sncLib, "snc-lib", "", "absolute local SNC library path")
	f.StringVar(&p.landscape, "landscape", "", "explicit SAP landscape file; otherwise SAP GUI user files")
	f.DurationVar(&p.timeout, "timeout", 45*time.Second, "total process deadline, including logon and cleanup")
}

func validateProbeFlags(p probeFlags) error {
	if !regexp.MustCompile(`^[A-Z0-9]{3}$`).MatchString(p.system) || !regexp.MustCompile(`^[0-9]{3}$`).MatchString(p.client) || strings.TrimSpace(p.connection) == "" || !regexp.MustCompile(`^[A-Za-z0-9_@.\-]{1,12}$`).MatchString(p.user) || p.dll == "" || p.sncLib == "" || p.timeout < time.Second || p.timeout > 2*time.Minute {
		return errors.New("require exact system, client, connection, expected SAP user, DLL and SNC library; timeout 1s..2m")
	}
	return nil
}

func reportProbeFailure(out io.Writer, stage string, err error) int {
	r := probeResult{Stage: stage, Reason: "operation failed; no retry performed"}
	var failure *probeFailure
	if errors.As(err, &failure) {
		r.Stage = failure.stage
		r.Reason = failure.reason
	}
	var native *sncrfc.NativeError
	if errors.As(err, &native) {
		r.Code = &native.Code
		r.Group = &native.Group
	}
	var loadErr *landscape.LoadError
	if errors.As(err, &loadErr) {
		r.Stage = "landscape"
		r.Diagnostic = loadErr.Code
		r.Include = loadErr.Include
		r.HTTPStatus = loadErr.Status
		r.Reason = loadErr.Error()
	}
	_ = json.NewEncoder(out).Encode(r)
	return 1
}

// The landscape sources: an explicit file, SAPLOGON_LSXML_FILE, or SAP GUI's user files.
func landscapePaths(explicit string) ([]string, error) {
	if explicit != "" {
		return []string{explicit}, nil
	}
	if configured := os.Getenv("SAPLOGON_LSXML_FILE"); configured != "" {
		return []string{configured}, nil
	}
	base := os.Getenv("APPDATA")
	if base == "" {
		return nil, &landscape.LoadError{Code: "no_sources"}
	}
	base = filepath.Join(base, "SAP", "Common")
	paths := []string{filepath.Join(base, "SAPUILandscape.xml")}
	global := filepath.Join(base, "SAPUILandscapeGlobal.xml")
	if _, err := os.Stat(global); err == nil {
		paths = append(paths, global)
	} else if !errors.Is(err, os.ErrNotExist) {
		return nil, &landscape.LoadError{Code: "file_access_denied", Include: true}
	}
	return paths, nil
}

func openProbe(ctx context.Context, p probeFlags) (probeSession, error) {
	paths, err := landscapePaths(p.landscape)
	if err != nil {
		return nil, err
	}
	l, err := landscape.Load(ctx, paths)
	if err != nil {
		return nil, err
	}
	params, err := l.Resolve(p.system, p.client, p.connection, p.sncLib)
	if err != nil {
		return nil, &probeFailure{"profile", err.Error()}
	}
	if err := ctx.Err(); err != nil {
		return nil, &probeFailure{"timeout", "probe deadline exceeded before logon"}
	}
	return sncrfc.Open(sncrfc.Options{DLL: p.dll, System: p.system, Client: p.client, Connection: p.connection, ExpectedUser: p.user, Parameters: params})
}

// adtSession is a logged-on session that can call SADT_REST_RFC_ENDPOINT.
type adtSession interface {
	probeSession
	Call(string, map[string]any) (map[string]any, error)
}
