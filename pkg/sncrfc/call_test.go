package sncrfc

import (
	"errors"
	"testing"
)

type functionFake struct {
	fakeAPI
	describes, invokes int
	failure            error
}

func (f *functionFake) describe(uintptr, string) (Description, error) {
	f.describes++
	return Description{}, nil
}
func (f *functionFake) invoke(uintptr, Description, map[string]any) (map[string]any, error) {
	f.invokes++
	return nil, f.failure
}
func fixtureRequest() map[string]any {
	return map[string]any{"REQUEST": map[string]any{"REQUEST_LINE": map[string]any{"METHOD": "GET", "URI": "/sap/bc/adt/discovery", "VERSION": "HTTP/1.1"}, "HEADER_FIELDS": []map[string]any{{"NAME": "Accept", "VALUE": "application/atomsvc+xml"}}, "MESSAGE_BODY": []byte{}}}
}
func TestCallDenialIsTerminalAndCloseStillWorks(t *testing.T) {
	f := &functionFake{failure: &NativeError{29, 7}}
	c := &Connection{api: f, handle: 42}
	_, e := c.Call("SADT_REST_RFC_ENDPOINT", fixtureRequest())
	var n *NativeError
	if !errors.As(e, &n) || n.Code != 29 {
		t.Fatal("denial lost")
	}
	_, _ = c.Call("SADT_REST_RFC_ENDPOINT", fixtureRequest())
	_, _ = c.Describe("SADT_REST_RFC_ENDPOINT")
	if f.describes != 1 || f.invokes != 1 {
		t.Fatal("denied connection reused")
	}
	if c.Close() != nil || f.closes != 1 {
		t.Fatal("denial prevents cleanup")
	}
}
func TestDiscoveryRejectsWritesAndExtraHeadersBeforeSDK(t *testing.T) {
	for _, change := range []func(map[string]any){func(r map[string]any) { r["REQUEST_LINE"].(map[string]any)["METHOD"] = "POST" }, func(r map[string]any) { r["REQUEST_LINE"].(map[string]any)["URI"] = "/sap/bc/adt/other" }, func(r map[string]any) { r["HEADER_FIELDS"] = []map[string]any{{"NAME": "Cookie", "VALUE": "fixture"}} }, func(r map[string]any) { r["EXTRA"] = "fixture" }} {
		f := &functionFake{}
		c := &Connection{api: f, handle: 42}
		input := fixtureRequest()
		change(input["REQUEST"].(map[string]any))
		if _, e := c.Call("SADT_REST_RFC_ENDPOINT", input); e == nil {
			t.Fatal("unsafe discovery accepted")
		}
		if f.describes != 0 || f.invokes != 0 {
			t.Fatal("rejection after SDK call")
		}
	}
}

// vsp puts sap-client and sap-language on every request: a data preview and
// the identity check must pass with them, and nothing else may ride along.
func TestPreviewAndIdentityAcceptSessionParams(t *testing.T) {
	const fs = "/sap/bc/adt/datapreview/freestyle"
	id := []byte("SELECT MANDT, LOGSYS FROM T000 WHERE MANDT = '122'")
	for _, c := range []struct {
		uri  string
		body []byte
		id   bool
		prev bool
	}{
		{fs + "?rowNumber=1&sap-client=122&sap-language=EN", id, true, true},
		{fs + "?rowNumber=1", id, true, true},
		{fs + "?rowNumber=5&sap-client=122", id, false, true},
		{fs + "?rowNumber=1&sap-client=122&evil=1", id, false, false},
		{fs + "?rowNumber=1&sap-client=12x", id, false, false},
		{fs + "?rowNumber=1&sap-client=122", []byte("SELECT * FROM T000"), false, true},
		{fs + "?rowNumber=1&sap-client=122", []byte("SELECT MANDT, LOGSYS FROM T000 WHERE MANDT = '122' OR 1 = 1"), false, true},
		{fs + "?rowNumber=1&sap-client=122", []byte("SELECT BNAME, PASSCODE FROM USR02"), false, false},
	} {
		if got := ADTIdentityQueryAllowed(c.uri, c.body); got != c.id {
			t.Errorf("identity %q %q = %v, want %v", c.uri, c.body, got, c.id)
		}
		if got := ADTDataPreviewAllowed(c.uri, c.body); got != c.prev {
			t.Errorf("preview %q %q = %v, want %v", c.uri, c.body, got, c.prev)
		}
	}
}
