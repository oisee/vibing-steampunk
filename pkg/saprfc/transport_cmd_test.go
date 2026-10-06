package saprfc

import (
	"strings"
	"testing"
)

// A system behind a transport command has a placeholder URL; classic RFC must
// not take its gateway host from it.
func TestResolve_TransportCmdNeedsExplicitHost(t *testing.T) {
	base := Input{URL: "https://sidecar.invalid", User: "TESTUSER", Password: "x", Client: "001"}

	if _, err := Resolve(base); err == nil || !strings.Contains(err.Error(), "rfc_host") {
		t.Fatalf("placeholder URL host: want a refusal naming rfc_host, got %v", err)
	}

	real := base
	real.URL = "https://dev.example.local:44300"
	real.TransportCmd = true
	if _, err := Resolve(real); err == nil {
		t.Fatal("transport command without rfc_host: the URL host was dialled")
	}

	withHost := real
	withHost.RFCHost = "gw.example.local"
	p, err := Resolve(withHost)
	if err != nil || p.Host != "gw.example.local" {
		t.Fatalf("explicit rfc_host: %+v, %v", p, err)
	}

	flagHost := base
	flagHost.HostFlag = "gw.example.local"
	if _, err := Resolve(flagHost); err != nil {
		t.Fatalf("explicit --rfc-host: %v", err)
	}

	plain := base
	plain.URL = "https://dev.example.local:44300"
	if p, err := Resolve(plain); err != nil || p.Host != "dev.example.local" {
		t.Fatalf("ordinary URL: %+v, %v", p, err)
	}
}
