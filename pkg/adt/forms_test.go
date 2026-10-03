package adt

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"
)

// fakeFormBridge answers form service actions from queues, one per action, and
// records what it was sent.
type fakeFormBridge struct {
	answers map[string][]string // action -> JSON answers, in order
	calls   []fakeFormCall
}

type fakeFormCall struct {
	action string
	params map[string]any
}

func (f *fakeFormBridge) FormRequest(_ context.Context, action string, params map[string]any) (json.RawMessage, error) {
	f.calls = append(f.calls, fakeFormCall{action: action, params: params})
	queue := f.answers[action]
	if len(queue) == 0 {
		return nil, fmt.Errorf("no answer queued for %s", action)
	}
	f.answers[action] = queue[1:]
	if strings.HasPrefix(queue[0], "ERR:") {
		return nil, fmt.Errorf("%s", strings.TrimPrefix(queue[0], "ERR:"))
	}
	return json.RawMessage(queue[0]), nil
}

func (f *fakeFormBridge) count(action string) int {
	n := 0
	for _, c := range f.calls {
		if c.action == action {
			n++
		}
	}
	return n
}

const activationOK = `<?xml version="1.0" encoding="utf-8"?><chkl:messages xmlns:chkl="http://www.sap.com/abapxml/checklist"><chkl:properties checkExecuted="true" activationExecuted="true" generationExecuted="false"/></chkl:messages>`

// fakeADT answers the CSRF fetch and activations, and records the requests.
type fakeADT struct {
	activations []string // activation answers, in order
	requests    []*http.Request
	bodies      []string
}

func (f *fakeADT) Do(req *http.Request) (*http.Response, error) {
	body := ""
	if req.Body != nil {
		b, _ := io.ReadAll(req.Body)
		body = string(b)
	}
	f.requests = append(f.requests, req)
	f.bodies = append(f.bodies, body)
	h := http.Header{}
	h.Set("X-CSRF-Token", "token")
	h.Set("Content-Type", "application/xml")
	answer := ""
	if req.URL.Path == "/sap/bc/adt/activation" {
		if len(f.activations) == 0 {
			return &http.Response{StatusCode: http.StatusInternalServerError, Header: h, Body: io.NopCloser(strings.NewReader(""))}, nil
		}
		answer, f.activations = f.activations[0], f.activations[1:]
	}
	return &http.Response{StatusCode: http.StatusOK, Header: h, Body: io.NopCloser(strings.NewReader(answer))}, nil
}

func (f *fakeADT) activationBodies() []string {
	var out []string
	for i, r := range f.requests {
		if r.URL.Path == "/sap/bc/adt/activation" {
			out = append(out, f.bodies[i])
		}
	}
	return out
}

// formTestClient answers the given number of activations and allows
// transportable edits, as a system writing forms in a request does.
func formTestClient(t *testing.T, activations int) (*Client, *fakeADT) {
	t.Helper()
	adt := &fakeADT{}
	for i := 0; i < activations; i++ {
		adt.activations = append(adt.activations, activationOK)
	}
	cfg := NewConfig("https://sap.example.com", "developer", "secret")
	cfg.Safety.EnableTransports = true
	cfg.Safety.AllowTransportableEdits = true
	return NewClientWithTransport(cfg, NewTransportWithClient(cfg, adt)), adt
}

func formInfoJSON(exists bool, pkg string, inactive bool) string {
	return fmt.Sprintf(`{"type":"SFPF","name":"ZDEMO","exists":%t,"package":%q,"masterLanguage":"DE","languages":["DE","ES"],"inactive":%t}`, exists, pkg, inactive)
}

func TestNormalizeFormType(t *testing.T) {
	for in, want := range map[string]string{"ssfo": "SSFO", "SmartForm": "SSFO", "sapscript": "FORM", "FORM": "FORM", "adobe": "SFPF", "SFPI": "SFPI"} {
		if got, ok := NormalizeFormType(in); !ok || got != want {
			t.Errorf("NormalizeFormType(%q) = %q, %t; want %q", in, got, ok, want)
		}
	}
	if _, ok := NormalizeFormType("PROG"); ok {
		t.Error("PROG taken for a form type")
	}
}

func TestFormObjectURL(t *testing.T) {
	if got := FormObjectURL(FormTypeAdobeForm, "/par/load_instr"); got != "/sap/bc/adt/vit/wb/object_type/sfpf5f/object_name/%2FPAR%2FLOAD_INSTR" {
		t.Errorf("form URL %s", got)
	}
	if got := FormObjectURL(FormTypeAdobeIntf, "ZDEMO_IF"); got != "/sap/bc/adt/vit/wb/object_type/sfpi5i/object_name/ZDEMO_IF" {
		t.Errorf("interface URL %s", got)
	}
}

func TestReadForm(t *testing.T) {
	c, _ := newTransportTestClient(t, nil)
	b64 := base64.StdEncoding.EncodeToString([]byte("<xdp/>"))
	ws := &fakeFormBridge{answers: map[string][]string{"read": {
		`{"masterLanguage":"DE","mimeType":"application/vnd.adobe.xdp+xml","contentBase64":"` + b64 + `"}`,
		`{"masterLanguage":"DE","mimeType":"application/xml","contentBase64":"` + b64 + `"}`,
	}}}
	got, err := c.ReadForm(context.Background(), ws, "adobe", "zdemo", "es")
	if err != nil {
		t.Fatal(err)
	}
	if got.Type != "SFPF" || got.Name != "ZDEMO" || got.Language != "ES" || got.Content != "<xdp/>" {
		t.Errorf("read %+v", got)
	}
	if p := ws.calls[0].params; p["language"] != "ES" || p["name"] != "ZDEMO" {
		t.Errorf("sent %v", p)
	}
	// A language means a layout only for an Adobe form.
	got, err = c.ReadForm(context.Background(), ws, "FORM", "ZDEMO", "DE")
	if err != nil || got.Language != "" {
		t.Errorf("SAPscript read %+v %v", got, err)
	}
}

func TestWriteFormRefusesBeforeSending(t *testing.T) {
	c, _ := newTransportTestClient(t, nil)
	c.config.Safety.AllowedPackages = []string{"Z*", "$TMP"}
	c.config.Safety.AllowedTransports = []string{"A4HK*"}
	ctx := context.Background()

	for name, tc := range map[string]struct {
		info string
		opts FormWriteOptions
		want string
	}{
		"package outside the list": {formInfoJSON(true, "/PAR/GENERAL", false),
			FormWriteOptions{Type: "SFPF", Name: "ZDEMO", Content: "<x/>"}, "/PAR/GENERAL"},
		"request outside the list": {formInfoJSON(true, "ZDEMO", false),
			FormWriteOptions{Type: "SFPF", Name: "ZDEMO", Content: "<x/>", Transport: "B4HK900001"}, "B4HK900001"},
		"create without package": {formInfoJSON(false, "", false),
			FormWriteOptions{Type: "SSFO", Name: "ZDEMO", Content: "<x/>"}, "package is required"},
		"create into a package outside the list": {formInfoJSON(false, "", false),
			FormWriteOptions{Type: "FORM", Name: "ZDEMO", Content: "<x/>", Package: "YOTHER"}, "YOTHER"},
		"create an Adobe form": {formInfoJSON(false, "", false),
			FormWriteOptions{Type: "SFPF", Name: "ZDEMO", Content: "<x/>", Package: "$TMP"}, "not created"},
	} {
		ws := &fakeFormBridge{answers: map[string][]string{"info": {tc.info}}}
		_, err := c.WriteForm(ctx, ws, tc.opts)
		if err == nil || !strings.Contains(err.Error(), tc.want) {
			t.Errorf("%s: err = %v, want it to name %q", name, err, tc.want)
		}
		if ws.count("write") != 0 {
			t.Errorf("%s: written although refused", name)
		}
	}

	c.config.Safety.ReadOnly = true
	ws := &fakeFormBridge{answers: map[string][]string{"info": {formInfoJSON(true, "ZDEMO", false)}}}
	if _, err := c.WriteForm(ctx, ws, FormWriteOptions{Type: "SSFO", Name: "ZDEMO", Content: "<x/>"}); err == nil || ws.count("write") != 0 {
		t.Errorf("read-only mode wrote a form: %v", err)
	}
}

func TestWriteFormCreatesWithPackage(t *testing.T) {
	c, mock := newTransportTestClient(t, nil)
	ws := &fakeFormBridge{answers: map[string][]string{
		"info":  {formInfoJSON(false, "", false)},
		"write": {`{"saved":true,"created":true,"activationRequired":false}`},
	}}
	res, err := c.WriteForm(context.Background(), ws, FormWriteOptions{Type: "SAPSCRIPT", Name: "zdemo", Content: "<form/>", Package: "$tmp"})
	if err != nil || !res.Created || !res.Saved || res.Activated {
		t.Fatalf("create %+v %v", res, err)
	}
	p := ws.calls[1].params
	content, _ := base64.StdEncoding.DecodeString(p["contentBase64"].(string))
	if p["type"] != "FORM" || p["package"] != "$TMP" || string(content) != "<form/>" {
		t.Errorf("sent %v", p)
	}
	if len(mock.requests) != 0 {
		t.Errorf("%d ADT requests for a SAPscript form, which needs no activation", len(mock.requests))
	}
}

func TestWriteFormActivatesAdobeOverADT(t *testing.T) {
	c, mock := formTestClient(t, 1)
	ws := &fakeFormBridge{answers: map[string][]string{
		"info":  {formInfoJSON(true, "ZDEMO", false), formInfoJSON(true, "ZDEMO", false)},
		"write": {`{"saved":true,"activationRequired":true}`},
	}}
	res, err := c.WriteForm(context.Background(), ws, FormWriteOptions{Type: "SFPF", Name: "ZDEMO", Content: "<xdp/>", Language: "DE", Transport: "A4HK900001"})
	if err != nil || !res.Activated || res.Language != "DE" {
		t.Fatalf("write %+v %v", res, err)
	}
	if ws.calls[1].params["transport"] != "A4HK900001" || ws.calls[1].params["language"] != "DE" {
		t.Errorf("sent %v", ws.calls[1].params)
	}
	bodies := mock.activationBodies()
	if len(bodies) != 1 || !strings.Contains(bodies[0], `/sap/bc/adt/vit/wb/object_type/sfpf5f/object_name/ZDEMO`) {
		t.Errorf("activation requests: %q", bodies)
	}
}

func TestWriteFormPutsBackTheOldFormWhenActivationFails(t *testing.T) {
	c, _ := formTestClient(t, 2)
	backup := base64.StdEncoding.EncodeToString([]byte("<old/>"))
	ws := &fakeFormBridge{answers: map[string][]string{
		// before the write; after the first activation (still inactive);
		// after the activation of the old form (active again)
		"info":  {formInfoJSON(true, "ZDEMO", false), formInfoJSON(true, "ZDEMO", true), formInfoJSON(true, "ZDEMO", false)},
		"write": {`{"saved":true,"activationRequired":true,"backupBase64":"` + backup + `"}`, `{"saved":true,"activationRequired":true}`},
	}}
	res, err := c.WriteForm(context.Background(), ws, FormWriteOptions{Type: "SFPF", Name: "ZDEMO", Content: "<new/>"})
	if err == nil || !strings.Contains(err.Error(), "put back") {
		t.Fatalf("err = %v, want the failed activation and the restore named", err)
	}
	if res == nil || !res.Restored || res.Activated {
		t.Fatalf("result %+v", res)
	}
	if ws.count("write") != 2 || ws.calls[3].params["contentBase64"] != backup {
		t.Errorf("the old form was not written back: %v", ws.calls)
	}
}

func TestWriteFormLayoutLeftInactiveIsNotRestored(t *testing.T) {
	c, _ := formTestClient(t, 1)
	ws := &fakeFormBridge{answers: map[string][]string{
		"info":  {formInfoJSON(true, "ZDEMO", false), formInfoJSON(true, "ZDEMO", true)},
		"write": {`{"saved":true,"activationRequired":true}`},
	}}
	_, err := c.WriteForm(context.Background(), ws, FormWriteOptions{Type: "SFPF", Name: "ZDEMO", Content: "<xdp/>", Language: "DE"})
	if err == nil || !strings.Contains(err.Error(), "active one is unchanged") {
		t.Fatalf("err = %v", err)
	}
	if ws.count("write") != 1 {
		t.Errorf("%d writes; a layout has no backup to write", ws.count("write"))
	}
}

func TestWriteFormTestRunSavesNothing(t *testing.T) {
	c, mock := newTransportTestClient(t, nil)
	ws := &fakeFormBridge{answers: map[string][]string{
		"info":  {formInfoJSON(true, "ZDEMO", false)},
		"write": {`{"saved":false,"testRun":true,"activationRequired":false}`},
	}}
	res, err := c.WriteForm(context.Background(), ws, FormWriteOptions{Type: "SFPF", Name: "ZDEMO", Content: "<x/>", TestRun: true})
	if err != nil || res.Saved || !res.TestRun {
		t.Fatalf("test run %+v %v", res, err)
	}
	if ws.calls[1].params["testRun"] != "X" || len(mock.requests) != 0 {
		t.Errorf("test run sent %v and %d ADT requests", ws.calls[1].params, len(mock.requests))
	}
}
