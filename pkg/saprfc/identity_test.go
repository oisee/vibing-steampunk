package saprfc

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/oisee/open-rfc-go/rfc"

	"github.com/oisee/vibing-steampunk/pkg/adt"
)

// fakeLogon stands in for a gateway: it counts dials, answers RFC_SYSTEM_INFO
// with sid, and counts the connections closed.
func fakeLogon(t *testing.T, sid string) (dials, infos, closes *int) {
	t.Helper()
	dials, infos, closes = new(int), new(int), new(int)
	oldDial, oldInfo, oldClose := dialRFC, rfcSystemID, closeRFC
	t.Cleanup(func() { dialRFC, rfcSystemID, closeRFC = oldDial, oldInfo, oldClose })
	dialRFC = func(context.Context, Params, time.Duration) (*rfc.Client, error) {
		*dials++
		return &rfc.Client{}, nil
	}
	rfcSystemID = func(context.Context, *rfc.Client) (string, error) {
		*infos++
		return sid, nil
	}
	closeRFC = func(context.Context, *rfc.Client) { *closes++ }
	return
}

func pin(t *testing.T, s string) *adt.IdentityPin {
	t.Helper()
	p, err := adt.ParseIdentityPin(s)
	if err != nil {
		t.Fatal(err)
	}
	return p
}

// An RFC user or client that contradicts the pin (an rfc_user, --rfc-user or a
// per-call user) never dials the gateway.
func TestRFCPinRefusesAContradictingLogonBeforeDialling(t *testing.T) {
	dials, _, _ := fakeLogon(t, "A4H")
	for _, p := range []Params{
		{Host: "h", Client: "001", User: "OTHERUSER", Expect: pin(t, "A4H.001/TESTUSER")},
		{Host: "h", Client: "100", User: "TESTUSER", Expect: pin(t, "A4H.001/TESTUSER")},
	} {
		if _, err := Open(context.Background(), p); !adt.IsIdentityMismatch(err) {
			t.Errorf("%s@%s: want an identity refusal, got %v", p.User, p.Client, err)
		}
	}
	if *dials != 0 {
		t.Errorf("a refused logon dialled the gateway %d time(s)", *dials)
	}
}

// After logon, before anything else: the gateway's system is checked, and a
// gateway of another system is closed and refused.
func TestRFCPinChecksTheSystemAfterLogon(t *testing.T) {
	dials, infos, closes := fakeLogon(t, "B4H")
	_, err := Open(context.Background(), Params{Host: "h", Client: "001", User: "TESTUSER", Expect: pin(t, "A4H.001/TESTUSER")})
	if !adt.IsIdentityMismatch(err) || !strings.Contains(err.Error(), "connected to B4H.001 as TESTUSER") {
		t.Fatalf("want a SID refusal, got %v", err)
	}
	if *dials != 1 || *infos != 1 || *closes != 1 {
		t.Errorf("dials %d, RFC_SYSTEM_INFO %d, closes %d; want 1, 1, 1", *dials, *infos, *closes)
	}

	dials, infos, closes = fakeLogon(t, "A4H")
	if _, err := Open(context.Background(), Params{Host: "h", Client: "001", User: "testuser", Expect: pin(t, "a4h.001/testuser")}); err != nil {
		t.Fatalf("matching system refused: %v", err)
	}
	if *closes != 0 || *infos != 1 || *dials != 1 {
		t.Errorf("dials %d, RFC_SYSTEM_INFO %d, closes %d; want 1, 1, 0", *dials, *infos, *closes)
	}
}

// Without a pin there is no extra call.
func TestRFCWithoutPinMakesNoIdentityCall(t *testing.T) {
	_, infos, _ := fakeLogon(t, "B4H")
	if _, err := Open(context.Background(), Params{Host: "h", Client: "001", User: "TESTUSER"}); err != nil {
		t.Fatal(err)
	}
	if *infos != 0 {
		t.Errorf("RFC_SYSTEM_INFO called %d time(s) without a pin", *infos)
	}
}

// Resolve carries the pin into the destination.
func TestResolveCarriesThePin(t *testing.T) {
	p := pin(t, "A4H.001/TESTUSER")
	dest, err := Resolve(Input{URL: "http://h:50000", User: "TESTUSER", Password: "x", Client: "001", Expect: p})
	if err != nil {
		t.Fatal(err)
	}
	if dest.Expect != p {
		t.Error("the pin was dropped by Resolve")
	}
}
