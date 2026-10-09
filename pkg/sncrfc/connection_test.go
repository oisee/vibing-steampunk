package sncrfc

import (
	"errors"
	"strings"
	"testing"
)

type fakeAPI struct {
	identity                       Identity
	openErr, identityErr, closeErr error
	opens, reads, closes, releases int
}

func (f *fakeAPI) open(map[string]string) (uintptr, error) { f.opens++; return 42, f.openErr }
func (f *fakeAPI) attributes(uintptr) (Identity, error)    { f.reads++; return f.identity, f.identityErr }
func (f *fakeAPI) close(uintptr) error                     { f.closes++; return f.closeErr }
func (f *fakeAPI) release() error                          { f.releases++; return nil }

func testOptions() Options {
	return Options{System: "TST", Client: "123", Connection: "test", ExpectedUser: "TESTUSER", Parameters: map[string]string{
		"ashost": "fixture.invalid", "sysnr": "00", "client": "123", "snc_mode": "1", "snc_sso": "1", "snc_qop": "9", "snc_partnername": "p:fixture", "snc_lib": "fixture.dll", "trace": "0",
	}}
}

func TestOpenChecksIdentityAndCloseIsIdempotent(t *testing.T) {
	f := &fakeAPI{identity: Identity{System: "TST", Client: "123", User: "TESTUSER"}}
	c, err := openWith(testOptions(), func(string) (nativeAPI, error) { return f, nil })
	if err != nil {
		t.Fatal(err)
	}
	if c.Identity() != f.identity || f.opens != 1 || f.reads != 1 {
		t.Fatal("identity was not checked exactly once")
	}
	if err := c.Close(); err != nil {
		t.Fatal(err)
	}
	if err := c.Close(); err != nil {
		t.Fatal(err)
	}
	if f.closes != 1 || f.releases != 1 {
		t.Fatal("connection or library leaked")
	}
}

func TestIdentityFailureClosesWithoutRetry(t *testing.T) {
	for _, id := range []Identity{{System: "BAD", Client: "123", User: "TESTUSER"}, {System: "TST", Client: "999", User: "TESTUSER"}, {System: "TST", Client: "123", User: "OTHER"}, {System: "TST", Client: "123"}} {
		f := &fakeAPI{identity: id}
		if c, err := openWith(testOptions(), func(string) (nativeAPI, error) { return f, nil }); err == nil || c != nil {
			t.Fatal("accepted mismatching identity")
		}
		if f.opens != 1 || f.closes != 1 || f.releases != 1 {
			t.Fatal("retry or resource leak")
		}
	}
}

func TestNativeFailureIsSanitizedAndNeverRetried(t *testing.T) {
	for _, stage := range []string{"open", "identity", "close"} {
		secret := errors.New("host secret-partner ticket payload")
		f := &fakeAPI{identity: Identity{System: "TST", Client: "123", User: "TESTUSER"}}
		switch stage {
		case "open":
			f.openErr = secret
		case "identity":
			f.identityErr = secret
		case "close":
			f.closeErr = secret
		}
		c, err := openWith(testOptions(), func(string) (nativeAPI, error) { return f, nil })
		if stage == "close" {
			if err != nil {
				t.Fatal(err)
			}
			err = c.Close()
		}
		if err == nil || strings.Contains(err.Error(), "secret-partner") {
			t.Fatal("native error leaked or ignored")
		}
		wantReleases := 1
		if stage == "close" {
			wantReleases = 0
		}
		if f.opens != 1 || f.releases != wantReleases {
			t.Fatal("retry or library leak")
		}
	}
}

func TestUnsafeOptionsFailBeforeLoading(t *testing.T) {
	for _, mutation := range []func(*Options){
		func(o *Options) { o.ExpectedUser = "" }, func(o *Options) { o.Parameters["passwd"] = "secret" },
		func(o *Options) { o.Parameters["snc_mode"] = "0" }, func(o *Options) { o.Parameters["snc_sso"] = "0" },
		func(o *Options) { o.Parameters["trace"] = "1" }, func(o *Options) { o.Parameters["client"] = "999" },
		func(o *Options) { o.Parameters["dest"] = "fallback" }, func(o *Options) { o.Parameters["snc_qop"] = "" },
	} {
		o := testOptions()
		mutation(&o)
		loads := 0
		_, err := openWith(o, func(string) (nativeAPI, error) { loads++; return &fakeAPI{}, nil })
		if err == nil || loads != 0 {
			t.Fatal("unsafe configuration reached loader")
		}
	}
}

func TestFailedCloseIsStickyAndDoesNotUnloadLiveLibrary(t *testing.T) {
	f := &fakeAPI{identity: Identity{System: "TST", Client: "123", User: "TESTUSER"}, closeErr: errors.New("fixture cleanup failure")}
	c, err := openWith(testOptions(), func(string) (nativeAPI, error) { return f, nil })
	if err != nil {
		t.Fatal(err)
	}
	first, second := c.Close(), c.Close()
	if first == nil || second == nil {
		t.Fatal("cleanup failure was forgotten")
	}
	if f.closes != 1 || f.releases != 0 {
		t.Fatal("retried close or unloaded DLL after failed cleanup")
	}
}
