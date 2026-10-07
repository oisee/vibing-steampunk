package saprfc

import (
	"strings"
	"testing"
)

func routeInput() Input {
	return Input{URL: "https://gw.example.local:44300", User: "TESTUSER", Password: "p", Client: "100"}
}

// A gateway behind a SAProuter, named the way SAP Logon writes it: the route
// prefix goes to the transport, and only the last hop's host is dialled.
func TestResolve_RouteInRFCHost(t *testing.T) {
	in := routeInput()
	in.RFCHost = "/H/router.example.local/H/gw.example.local"
	in.RFCSysnr = "00"
	p, err := Resolve(in)
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if p.Router != "/H/router.example.local/H/" || p.Host != "gw.example.local" || p.Port != 3300 {
		t.Errorf("got router=%q host=%q port=%d, want /H/router.example.local/H/, gw.example.local, 3300", p.Router, p.Host, p.Port)
	}
}

// Router hops may carry their own service; the last hop's /S/ is the gateway port.
func TestResolve_RouteWithServicePorts(t *testing.T) {
	in := routeInput()
	in.RFCHost = "/H/r1.example.local/S/3299/H/r2.example.local/H/gw.example.local/S/3301"
	p, err := Resolve(in)
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if p.Router != "/H/r1.example.local/S/3299/H/r2.example.local/H/" || p.Host != "gw.example.local" || p.Port != 3301 {
		t.Errorf("got router=%q host=%q port=%d", p.Router, p.Host, p.Port)
	}
}

// rfc_router beside a plain rfc_host, with or without the trailing /H/.
func TestResolve_RouterSetting(t *testing.T) {
	for _, r := range []string{"/H/router.example.local/H/", "/H/router.example.local", " /H/router.example.local/ "} {
		in := routeInput()
		in.RFCHost, in.RFCRouter, in.RFCSysnr = "gw.example.local", r, "00"
		p, err := Resolve(in)
		if err != nil {
			t.Fatalf("Resolve(%q): %v", r, err)
		}
		if p.Router != "/H/router.example.local/H/" || p.Host != "gw.example.local" {
			t.Errorf("rfc_router %q: got router=%q host=%q", r, p.Router, p.Host)
		}
	}
}

// The flag wins over the system's rfc_router, as the other flags do.
func TestResolve_RouterFlagWins(t *testing.T) {
	in := routeInput()
	in.RFCHost, in.RFCSysnr = "gw.example.local", "00"
	in.RFCRouter, in.RouterFlag = "/H/old.example.local/H/", "/H/new.example.local/H/"
	p, err := Resolve(in)
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if p.Router != "/H/new.example.local/H/" {
		t.Errorf("router = %q, want the flag's", p.Router)
	}
}

// Without a route nothing changes: direct dial, as before.
func TestResolve_NoRouteIsDirect(t *testing.T) {
	in := routeInput()
	in.RFCSysnr = "00"
	p, err := Resolve(in)
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if p.Router != "" || p.Host != "gw.example.local" {
		t.Errorf("got router=%q host=%q, want no router and the URL host", p.Router, p.Host)
	}
}

func TestResolve_RouteRefusals(t *testing.T) {
	for _, tc := range []struct{ host, router, want string }{
		{"/H/router.example.local/P/secret/H/gw.example.local", "", "password"},
		{"/H/router.example.local/H/gw.example.local", "/H/other.example.local/H/", "give the route once"},
		{"/H/router.example.local/H/gw.example.local/S/notaport", "", "invalid port"},
	} {
		in := routeInput()
		in.RFCHost, in.RFCRouter = tc.host, tc.router
		_, err := Resolve(in)
		if err == nil || !strings.Contains(err.Error(), tc.want) {
			t.Errorf("Resolve(host=%q router=%q) err = %v, want one mentioning %q", tc.host, tc.router, err, tc.want)
		}
	}
}

// A route repeated in both places is fine when it's the same route.
func TestResolve_SameRouteTwiceIsFine(t *testing.T) {
	in := routeInput()
	in.RFCHost, in.RFCRouter, in.RFCSysnr = "/H/router.example.local/H/gw.example.local", "/H/router.example.local", "00"
	if _, err := Resolve(in); err != nil {
		t.Errorf("same route twice: %v", err)
	}
}
