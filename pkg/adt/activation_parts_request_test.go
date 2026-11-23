package adt

import (
	"context"
	"io"
	"net/http"
	"strings"
	"sync"
	"testing"
)

// activateCall is one request the activation flow sent.
type activateCall struct {
	method, path, body string
}

// activateHTTP stands in for SAP below the transport: every activation POST is
// answered by activate (in call order), and the inactive list by inactive.
type activateHTTP struct {
	activate []string
	inactive func() (int, string)
	missing  map[string]bool // GET paths that answer 404

	mu    sync.Mutex
	calls []activateCall
}

func (m *activateHTTP) Do(req *http.Request) (*http.Response, error) {
	var body string
	if req.Body != nil {
		b, _ := io.ReadAll(req.Body)
		body = string(b)
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.calls = append(m.calls, activateCall{req.Method, req.URL.Path, body})

	status, answer := http.StatusOK, ""
	switch {
	case strings.HasSuffix(req.URL.Path, "/activation/inactiveobjects"):
		status, answer = m.inactive()
	case req.Method == http.MethodGet && m.missing[req.URL.Path]:
		status = http.StatusNotFound
	case strings.HasSuffix(req.URL.Path, "/activation") && req.Method == http.MethodPost:
		n := len(m.activations()) - 1
		if n < len(m.activate) {
			answer = m.activate[n]
		}
	}
	h := http.Header{}
	h.Set("X-CSRF-Token", "TOKEN")
	return &http.Response{StatusCode: status, Body: io.NopCloser(strings.NewReader(answer)), Header: h}, nil
}

// activations are the activation POSTs, in order. The caller holds mu.
func (m *activateHTTP) activations() []activateCall {
	var out []activateCall
	for _, c := range m.calls {
		if c.method == http.MethodPost && strings.HasSuffix(c.path, "/activation") {
			out = append(out, c)
		}
	}
	return out
}

func (m *activateHTTP) posts() []activateCall {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.activations()
}

func (m *activateHTTP) client(user string) *Client {
	cfg := NewConfig("https://sap.example.com:44300", user, "secret")
	return NewClientWithTransport(cfg, NewTransportWithClient(cfg, m))
}

// inactiveList builds the inactive objects feed; a part with user "" has no
// ioc:user attribute.
func inactiveList(parts ...[2]string) string {
	var b strings.Builder
	b.WriteString(`<?xml version="1.0" encoding="UTF-8"?><ioc:inactiveObjects xmlns:ioc="http://www.sap.com/adt/inactivectsobjects">`)
	for _, p := range parts {
		name, user := p[0], p[1]
		uri := "/sap/bc/adt/functions/groups/zdemo/includes/" + strings.ToLower(name)
		if name == "ZDEMO" {
			uri = "/sap/bc/adt/functions/groups/zdemo"
		}
		b.WriteString(`<ioc:entry><ioc:object`)
		if user != "" {
			b.WriteString(` ioc:user="` + user + `"`)
		}
		b.WriteString(`><ioc:ref adtcore:uri="` + uri + `" adtcore:type="FUGR/I" adtcore:name="` + name +
			`" xmlns:adtcore="http://www.sap.com/adt/core"/></ioc:object></ioc:entry>`)
	}
	b.WriteString(`</ioc:inactiveObjects>`)
	return b.String()
}

const groupURL = "/sap/bc/adt/functions/groups/zdemo"

// The silent refusal is retried as one batch: the group and the caller's own
// inactive parts (and parts that name no user), never a colleague's.
func TestActivate_RetriesWithOwnInactiveParts(t *testing.T) {
	m := &activateHTTP{
		activate: []string{silentRefusalXML, ""},
		inactive: func() (int, string) {
			return http.StatusOK, inactiveList(
				[2]string{"LZDEMOTOP", "DEV"},
				[2]string{"LZDEMOF01", "OTHER"},
				[2]string{"LZDEMOF02", ""},
			)
		},
	}
	res, err := m.client("dev").Activate(context.Background(), groupURL, "ZDEMO")
	if err != nil {
		t.Fatalf("Activate: %v", err)
	}
	posts := m.posts()
	if len(posts) != 2 {
		t.Fatalf("activation POSTs = %d, want the refused one and the retry", len(posts))
	}
	retry := posts[1].body
	for _, want := range []string{`adtcore:name="ZDEMO"`, `adtcore:name="LZDEMOTOP"`, `adtcore:name="LZDEMOF02"`} {
		if !strings.Contains(retry, want) {
			t.Errorf("retry body lacks %s:\n%s", want, retry)
		}
	}
	if strings.Contains(retry, "LZDEMOF01") {
		t.Errorf("retry activates another user's part:\n%s", retry)
	}
	if !res.Success {
		t.Errorf("the retry went through, result = %+v", res)
	}
}

// Without a user name (cookie/SSO logon) the owner of a part cannot be
// checked, so no part that carries a user name is activated along.
func TestActivate_UnknownUserDoesNotTakeOthersParts(t *testing.T) {
	m := &activateHTTP{
		activate: []string{silentRefusalXML, ""},
		inactive: func() (int, string) {
			return http.StatusOK, inactiveList([2]string{"LZDEMOTOP", "DEV"}, [2]string{"LZDEMOF01", "OTHER"})
		},
	}
	res, err := m.client("").Activate(context.Background(), groupURL, "ZDEMO")
	if err != nil {
		t.Fatalf("Activate: %v", err)
	}
	for _, p := range m.posts()[1:] {
		if strings.Contains(p.body, "LZDEMOF01") || strings.Contains(p.body, "LZDEMOTOP") {
			t.Errorf("parts activated without knowing whose they are:\n%s", p.body)
		}
	}
	if res.Success {
		t.Errorf("the refusal became a success: %+v", res)
	}
}

// When the inactive list cannot be read, the original refusal comes back and
// nothing else is sent.
func TestActivate_LookupFailureKeepsTheRefusal(t *testing.T) {
	m := &activateHTTP{
		activate: []string{silentRefusalXML, ""},
		inactive: func() (int, string) { return http.StatusInternalServerError, "boom" },
	}
	res, err := m.client("dev").Activate(context.Background(), groupURL, "ZDEMO")
	if err != nil {
		t.Fatalf("Activate: %v", err)
	}
	if n := len(m.posts()); n != 1 {
		t.Errorf("activation POSTs = %d, want only the refused one", n)
	}
	if res.Success || len(res.Messages) != 0 || len(res.Inactive) != 0 {
		t.Errorf("result = %+v, want the original empty refusal", res)
	}
}

// Nothing of the object inactive: the refusal meant there was nothing to do.
func TestActivate_NothingInactiveIsNothingToActivate(t *testing.T) {
	m := &activateHTTP{
		activate: []string{silentRefusalXML},
		inactive: func() (int, string) { return http.StatusOK, inactiveList() },
	}
	res, err := m.client("dev").Activate(context.Background(), groupURL, "ZDEMO")
	if err != nil {
		t.Fatalf("Activate: %v", err)
	}
	if !res.Success || len(res.Messages) != 1 || !strings.Contains(res.Messages[0].ShortText, "Nothing to activate") {
		t.Errorf("result = %+v, want the 'Nothing to activate' verdict", res)
	}
	if n := len(m.posts()); n != 1 {
		t.Errorf("activation POSTs = %d, want 1", n)
	}
}

// SAP refuses without a word for an object that does not exist, too, and the
// inactive list then has nothing of it. "Nothing to activate" was the verdict,
// success:true, for an include name with a typo (seen on S/4HANA 2023). When
// the object itself answers 404, the refusal stands and says why.
func TestActivate_MissingObjectIsNotNothingToActivate(t *testing.T) {
	const incl = "/sap/bc/adt/programs/includes/zdemo_typo_f01"
	m := &activateHTTP{
		activate: []string{silentRefusalXML},
		inactive: func() (int, string) { return http.StatusOK, inactiveList() },
		missing:  map[string]bool{incl: true},
	}
	res, err := m.client("dev").Activate(context.Background(), incl+"/source/main", "ZDEMO_TYPO_F01")
	if err != nil {
		t.Fatalf("Activate: %v", err)
	}
	if res.Success {
		t.Fatalf("activating an object that does not exist was reported as success: %+v", res)
	}
	if len(res.Messages) == 0 || !strings.Contains(res.Messages[0].ShortText, "does not exist") {
		t.Errorf("result = %+v, want a message saying the object does not exist", res)
	}
}
