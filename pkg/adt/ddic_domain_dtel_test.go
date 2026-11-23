package adt

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
)

func TestDomainBody(t *testing.T) {
	b := domainBody(DomainOptions{Name: "ZDEMO", Description: "Demo & co", Package: "$TMP", DataType: "char",
		Length: 2, OutputLength: 2, Lowercase: true, FixedValues: []FixedValue{{Low: "A", Text: "Alpha"}, {Low: "B", High: "C", Text: "B<C"}}}, "DE")
	for _, want := range []string{
		`adtcore:name="ZDEMO"`, `adtcore:description="Demo &amp; co"`, `adtcore:masterLanguage="DE"`,
		`<doma:datatype>CHAR</doma:datatype><doma:length>000002</doma:length>`,
		`<doma:lowercase>true</doma:lowercase>`,
		`<doma:position>0002</doma:position><doma:low>B</doma:low><doma:high>C</doma:high><doma:text>B&lt;C</doma:text>`,
	} {
		if !strings.Contains(b, want) {
			t.Errorf("domain body lacks %s", want)
		}
	}
}

func TestDataElementBody(t *testing.T) {
	b := dataElementBody(DataElementOptions{Name: "ZDEMO", Description: "Demo", Package: "$TMP", Domain: "zdom",
		ShortLabel: "Demo", LongLabel: "Demo field"}, "EN")
	for _, want := range []string{
		`<dtel:typeKind>domain</dtel:typeKind><dtel:typeName>ZDOM</dtel:typeName>`,
		`<dtel:shortFieldLabel>Demo</dtel:shortFieldLabel><dtel:shortFieldLength>10</dtel:shortFieldLength><dtel:shortFieldMaxLength>10</dtel:shortFieldMaxLength>`,
		`<dtel:headingFieldMaxLength>55</dtel:headingFieldMaxLength>`,
	} {
		if !strings.Contains(b, want) {
			t.Errorf("data element body lacks %s", want)
		}
	}
	b = dataElementBody(DataElementOptions{Name: "ZDEMO", DataType: "char", Length: 15}, "EN")
	if !strings.Contains(b, `<dtel:typeKind>predefinedAbapType</dtel:typeKind><dtel:typeName></dtel:typeName><dtel:dataType>CHAR</dtel:dataType><dtel:dataTypeLength>000015</dtel:dataTypeLength>`) {
		t.Errorf("predefined type: %s", b)
	}
	b = dataElementBody(DataElementOptions{Name: "ZDEMO", Domain: "ZD"}, `E"&`)
	if !strings.Contains(b, `adtcore:language="E&quot;&amp;" adtcore:masterLanguage="E&quot;&amp;"`) {
		t.Errorf("language not escaped: %s", b)
	}
}

func TestDDICCreateRefusesBeforeSending(t *testing.T) {
	c, mock := newTransportTestClient(t, nil)
	for name, err := range map[string]error{
		"no type": func() error {
			_, e := c.CreateDomain(context.Background(), DomainOptions{Name: "ZD", Description: "d"})
			return e
		}(),
		"domain and type": func() error {
			_, e := c.CreateDataElement(context.Background(), DataElementOptions{Name: "ZE", Domain: "ZD", DataType: "CHAR"})
			return e
		}(),
		"neither": func() error {
			_, e := c.CreateDataElement(context.Background(), DataElementOptions{Name: "ZE"})
			return e
		}(),
		"short label long": func() error {
			_, e := c.CreateDataElement(context.Background(), DataElementOptions{Name: "ZE", Domain: "ZD", ShortLabel: "more than ten"})
			return e
		}(),
		"fixed value text": func() error {
			_, e := c.CreateDomain(context.Background(), DomainOptions{Name: "ZD", DataType: "CHAR", Length: 1, FixedValues: []FixedValue{{Low: "A", Text: strings.Repeat("x", 61)}}})
			return e
		}(),
	} {
		if err == nil {
			t.Errorf("%s: accepted", name)
		}
	}
	if len(mock.requests) != 0 {
		t.Errorf("%d requests sent for invalid input", len(mock.requests))
	}
}

// ddicCreateServer answers a DDIC create POST with the status refuse gives its
// Content-Type (200 when none), activates without a word and serves an empty
// inactive list. It returns the Content-Type of every POST, in order.
func ddicCreateServer(t *testing.T, refuse map[string]int) (*Client, func() []string) {
	t.Helper()
	var mu sync.Mutex
	var posted []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		w.Header().Set("X-CSRF-Token", "TOKEN")
		switch {
		case strings.Contains(r.URL.Path, "/activation/inactiveobjects"):
			_, _ = w.Write([]byte(inactiveFeed()))
		case r.Method == http.MethodPost && strings.HasPrefix(r.URL.Path, "/sap/bc/adt/ddic/"):
			ct := r.Header.Get("Content-Type")
			posted = append(posted, ct)
			if status := refuse[ct]; status != 0 {
				w.WriteHeader(status)
			}
		}
	}))
	t.Cleanup(srv.Close)
	cfg := NewConfig(srv.URL, "TESTUSER", "secret")
	return NewClientWithTransport(cfg, NewTransport(cfg)), func() []string {
		mu.Lock()
		defer mu.Unlock()
		return append([]string(nil), posted...)
	}
}

type ddicCreateCase struct {
	name   string
	v2, v1 string
	create func(*Client) (*DDICObjectResult, error)
}

func ddicCreateCases() []ddicCreateCase {
	return []ddicCreateCase{
		{"domain", "application/vnd.sap.adt.domains.v2+xml", "application/vnd.sap.adt.domains.v1+xml",
			func(c *Client) (*DDICObjectResult, error) {
				return c.CreateDomain(context.Background(), DomainOptions{Name: "ZDEMO_D", Description: "d", DataType: "CHAR", Length: 1})
			}},
		{"data element", "application/vnd.sap.adt.dataelements.v2+xml", "application/vnd.sap.adt.dataelements.v1+xml",
			func(c *Client) (*DDICObjectResult, error) {
				return c.CreateDataElement(context.Background(), DataElementOptions{Name: "ZDEMO_E", Description: "e", Domain: "ZDEMO_D"})
			}},
	}
}

// SAP_BASIS 7.50 knows only v1: it refuses the v2 document (415 for the
// Content-Type, 406 for the Accept) and creates it as v1.
func TestDDICCreateFallsBackToV1(t *testing.T) {
	for _, tc := range ddicCreateCases() {
		for _, status := range []int{http.StatusUnsupportedMediaType, http.StatusNotAcceptable} {
			c, posted := ddicCreateServer(t, map[string]int{tc.v2: status})
			res, err := tc.create(c)
			if err != nil || !res.Active {
				t.Fatalf("%s, v2 refused with %d: %+v, %v", tc.name, status, res, err)
			}
			if got := posted(); len(got) != 2 || got[0] != tc.v2 || got[1] != tc.v1 {
				t.Errorf("%s, v2 refused with %d: posted %q, want v2 then v1", tc.name, status, got)
			}
		}
	}
}

// A system that takes v2 gets one POST, as before.
func TestDDICCreateSendsV2Once(t *testing.T) {
	for _, tc := range ddicCreateCases() {
		c, posted := ddicCreateServer(t, nil)
		if res, err := tc.create(c); err != nil || !res.Active {
			t.Fatalf("%s: %+v, %v", tc.name, res, err)
		}
		if got := posted(); len(got) != 1 || got[0] != tc.v2 {
			t.Errorf("%s: posted %q, want v2 once", tc.name, got)
		}
	}
}

// Any other refusal is about the document or the system, not its version: it
// is reported, and the create is not sent a second time.
func TestDDICCreateDoesNotRetryOtherErrors(t *testing.T) {
	for _, tc := range ddicCreateCases() {
		for _, status := range []int{http.StatusBadRequest, http.StatusInternalServerError} {
			c, posted := ddicCreateServer(t, map[string]int{tc.v2: status})
			if _, err := tc.create(c); err == nil {
				t.Errorf("%s, %d: accepted", tc.name, status)
			}
			if got := posted(); len(got) != 1 {
				t.Errorf("%s, %d: posted %q, want a single POST", tc.name, status, got)
			}
		}
	}
}

// When neither version is accepted, the last refusal is what the caller sees.
func TestDDICCreateReportsWhenNoVersionIsAccepted(t *testing.T) {
	for _, tc := range ddicCreateCases() {
		c, posted := ddicCreateServer(t, map[string]int{tc.v2: http.StatusUnsupportedMediaType, tc.v1: http.StatusUnsupportedMediaType})
		_, err := tc.create(c)
		if err == nil || !strings.Contains(err.Error(), "415") {
			t.Errorf("%s: got %v, want the 415", tc.name, err)
		}
		if got := posted(); len(got) != 2 {
			t.Errorf("%s: posted %q, want v2 then v1", tc.name, got)
		}
	}
}
