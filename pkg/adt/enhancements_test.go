package adt

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"
)

func newEnhancementSearchResponse(name, subtype, pkg string) *http.Response {
	uri := "/sap/bc/adt/enhancements/enho" + strings.ToLower(subtype) + "/" + strings.ToLower(name)
	xml := `<?xml version="1.0" encoding="UTF-8"?>
<adtcore:objectReferences xmlns:adtcore="http://www.sap.com/adt/core">
  <adtcore:objectReference adtcore:uri="` + uri + `" adtcore:type="ENHO/` + subtype + `" adtcore:name="` + name + `" adtcore:packageName="` + pkg + `" adtcore:description="demo"/>
</adtcore:objectReferences>`
	return newTestResponse(xml)
}

// newRoutedMockTransport returns a mock that dispatches by exact path. Useful
// when a test needs different bodies at different URLs (e.g. search + then
// source fetch).
type routedMock struct {
	byPath   map[string]*http.Response
	requests []*http.Request
}

func (r *routedMock) Do(req *http.Request) (*http.Response, error) {
	r.requests = append(r.requests, req)
	if resp, ok := r.byPath[req.URL.Path]; ok {
		return resp, nil
	}
	for key, resp := range r.byPath {
		if strings.Contains(req.URL.Path, key) {
			return resp, nil
		}
	}
	return &http.Response{
		StatusCode: http.StatusNotFound,
		Body:       io.NopCloser(strings.NewReader("Not found")),
		Header:     http.Header{},
	}, nil
}

func newBody(s string) *http.Response {
	return &http.Response{
		StatusCode: http.StatusOK,
		Body:       io.NopCloser(strings.NewReader(s)),
		Header:     http.Header{"X-CSRF-Token": []string{"t"}},
	}
}

func TestGetEnhancement_ResolvesSubtypeViaSearch(t *testing.T) {
	sourceBody := `ENHANCEMENT 2 Y3EI_SKIP_BYPASS_CC_WITH_LIMIT.
  LOOP AT lt_xfplt INTO DATA(ls_xfplt) WHERE fpltr < 900000.
    lv_sum = lv_sum + ls_xfplt-fakwr.
  ENDLOOP.
ENDENHANCEMENT.`

	searchResp := newEnhancementSearchResponse("Y3EI_SKIP_BYPASS_CC_WITH_LIMIT", "XH", "YSD")

	mock := &routedMock{
		byPath: map[string]*http.Response{
			"/sap/bc/adt/repository/informationsystem/search":                                       searchResp,
			"/sap/bc/adt/enhancements/enhoxh/y3ei_skip_bypass_cc_with_limit/source/main":            newBody(sourceBody),
			"/sap/bc/adt/discovery":                                                                 newBody("OK"),
		},
	}

	cfg := NewConfig("https://sap.example.com:44300", "u", "p")
	transport := NewTransportWithClient(cfg, mock)
	client := NewClientWithTransport(cfg, transport)

	got, err := client.GetEnhancement(context.Background(), "Y3EI_SKIP_BYPASS_CC_WITH_LIMIT")
	if err != nil {
		t.Fatalf("GetEnhancement failed: %v", err)
	}
	if !strings.Contains(got, "lv_sum = lv_sum + ls_xfplt-fakwr") {
		t.Fatalf("expected spliced enhancement source, got:\n%s", got)
	}
}

func TestGetEnhancement_NotFound(t *testing.T) {
	emptySearch := `<?xml version="1.0" encoding="UTF-8"?>
<adtcore:objectReferences xmlns:adtcore="http://www.sap.com/adt/core"></adtcore:objectReferences>`
	mock := &mockTransportClient{
		responses: map[string]*http.Response{
			"search":    newTestResponse(emptySearch),
			"discovery": newTestResponse("OK"),
		},
	}
	cfg := NewConfig("https://sap.example.com:44300", "u", "p")
	transport := NewTransportWithClient(cfg, mock)
	client := NewClientWithTransport(cfg, transport)

	_, err := client.GetEnhancement(context.Background(), "ZDOES_NOT_EXIST")
	if err == nil {
		t.Fatal("expected not-found error, got nil")
	}
	if !strings.Contains(err.Error(), "not found") {
		t.Fatalf("expected not-found message, got: %v", err)
	}
}

func TestGetEnhancement_AmbiguousAcrossSubtypes(t *testing.T) {
	// Two ENHO hits with the same name, different subtypes.
	ambiguousSearch := `<?xml version="1.0" encoding="UTF-8"?>
<adtcore:objectReferences xmlns:adtcore="http://www.sap.com/adt/core">
  <adtcore:objectReference adtcore:uri="/sap/bc/adt/enhancements/enhoxh/yzzz" adtcore:type="ENHO/XH" adtcore:name="YZZZ" adtcore:packageName="P1"/>
  <adtcore:objectReference adtcore:uri="/sap/bc/adt/enhancements/enhoxc/yzzz" adtcore:type="ENHO/XC" adtcore:name="YZZZ" adtcore:packageName="P2"/>
</adtcore:objectReferences>`
	mock := &mockTransportClient{
		responses: map[string]*http.Response{
			"search":    newTestResponse(ambiguousSearch),
			"discovery": newTestResponse("OK"),
		},
	}
	cfg := NewConfig("https://sap.example.com:44300", "u", "p")
	transport := NewTransportWithClient(cfg, mock)
	client := NewClientWithTransport(cfg, transport)

	_, err := client.GetEnhancement(context.Background(), "YZZZ")
	if err == nil {
		t.Fatal("expected ambiguity error, got nil")
	}
	msg := err.Error()
	if !strings.Contains(msg, "ambiguous") || !strings.Contains(msg, "XC") || !strings.Contains(msg, "XH") {
		t.Fatalf("expected ambiguity message listing both subtypes, got: %v", err)
	}
}

func TestListEnhancementsForInclude_ParsesResponse(t *testing.T) {
	browserResp := `<?xml version="1.0" encoding="UTF-8"?>
<adtcore:objectReferences xmlns:adtcore="http://www.sap.com/adt/core">
  <adtcore:objectReference adtcore:uri="/sap/bc/adt/enhancements/enhoxh/y3ei_skip_bypass_cc_with_limit" adtcore:type="ENHO/XH" adtcore:name="Y3EI_SKIP_BYPASS_CC_WITH_LIMIT" adtcore:packageName="YSD" adtcore:description="Skip bypass when CC with limit-to"/>
  <adtcore:objectReference adtcore:uri="/sap/bc/adt/programs/includes/rvkmp901" adtcore:type="PROG/I" adtcore:name="RVKMP901" adtcore:packageName="VKM"/>
</adtcore:objectReferences>`

	mock := &routedMock{
		byPath: map[string]*http.Response{
			"/sap/bc/adt/enhancements/enhoxhs": newBody(browserResp),
			"/sap/bc/adt/discovery":             newBody("OK"),
		},
	}
	cfg := NewConfig("https://sap.example.com:44300", "u", "p")
	transport := NewTransportWithClient(cfg, mock)
	client := NewClientWithTransport(cfg, transport)

	refs, err := client.ListEnhancementsForInclude(context.Background(), "RVKMP901")
	if err != nil {
		t.Fatalf("ListEnhancementsForInclude failed: %v", err)
	}
	if len(refs) != 1 {
		t.Fatalf("expected 1 ENHO match, got %d: %+v", len(refs), refs)
	}
	if refs[0].Name != "Y3EI_SKIP_BYPASS_CC_WITH_LIMIT" {
		t.Errorf("wrong name: %s", refs[0].Name)
	}
	if refs[0].Kind != "XH" {
		t.Errorf("wrong kind: %s", refs[0].Kind)
	}
}

func TestGetIncludeMerged_AnchorResolvable(t *testing.T) {
	includeBody := `FORM BEDINGUNG_PRUEFEN_901.
"""""""""""""""""""""""""""""""""""""""""""""""""""""""""""""""""""""""""""""""""""""""""""""$"$\SE:(1) Form BEDINGUNG_PRUEFEN_901, Start                                                                                                                 A
*$*$-Start: (1)---------------------------------------------------------------------------------$*$*
* existing body
ENDFORM.
`
	enhSource := `ENHANCEMENT 2 Y3EI_SKIP_BYPASS_CC_WITH_LIMIT.
  DATA lv_sum TYPE fakwr.
  lv_sum = lv_sum + 1.
ENDENHANCEMENT.`

	browserResp := `<?xml version="1.0" encoding="UTF-8"?>
<adtcore:objectReferences xmlns:adtcore="http://www.sap.com/adt/core">
  <adtcore:objectReference adtcore:uri="/sap/bc/adt/enhancements/enhoxh/y3ei_skip_bypass_cc_with_limit" adtcore:type="ENHO/XH" adtcore:name="Y3EI_SKIP_BYPASS_CC_WITH_LIMIT" adtcore:packageName="YSD"/>
</adtcore:objectReferences>`

	searchResp := newEnhancementSearchResponse("Y3EI_SKIP_BYPASS_CC_WITH_LIMIT", "XH", "YSD")

	mock := &routedMock{
		byPath: map[string]*http.Response{
			"/sap/bc/adt/programs/includes/RVKMP901/source/main":                                    newBody(includeBody),
			"/sap/bc/adt/enhancements/enhoxhs":                                                      newBody(browserResp),
			"/sap/bc/adt/repository/informationsystem/search":                                       searchResp,
			"/sap/bc/adt/enhancements/enhoxh/y3ei_skip_bypass_cc_with_limit/source/main":            newBody(enhSource),
			"/sap/bc/adt/discovery":                                                                 newBody("OK"),
		},
	}
	cfg := NewConfig("https://sap.example.com:44300", "u", "p")
	transport := NewTransportWithClient(cfg, mock)
	client := NewClientWithTransport(cfg, transport)

	merged, err := client.GetIncludeMerged(context.Background(), "RVKMP901")
	if err != nil {
		t.Fatalf("GetIncludeMerged failed: %v", err)
	}

	if !strings.Contains(merged, "ENHO/XH Y3EI_SKIP_BYPASS_CC_WITH_LIMIT") {
		t.Errorf("expected ENHO banner in output, got:\n%s", merged)
	}
	if !strings.Contains(merged, "ENHANCEMENT 2 Y3EI_SKIP_BYPASS_CC_WITH_LIMIT") {
		t.Errorf("expected enhancement body in merged output, got:\n%s", merged)
	}
	if !strings.Contains(merged, "unresolved enhancements") {
		// Good — should not be in this positive case.
	} else {
		t.Errorf("positive case should not contain unresolved-enhancements banner, got:\n%s", merged)
	}

	// Anchor marker must still appear (we insert AFTER it, we don't consume it).
	if !strings.Contains(merged, "$*$\\SE:(1) Form BEDINGUNG_PRUEFEN_901, Start") {
		t.Errorf("anchor marker was dropped from output")
	}
}

func TestGetIncludeMerged_AnchorUnresolvable(t *testing.T) {
	includeBody := `FORM foo.
* no SE anchor markers in this body
ENDFORM.
`
	enhSource := `ENHANCEMENT 2 YZZZ_ENH.
  WRITE 'hi'.
ENDENHANCEMENT.`

	browserResp := `<?xml version="1.0" encoding="UTF-8"?>
<adtcore:objectReferences xmlns:adtcore="http://www.sap.com/adt/core">
  <adtcore:objectReference adtcore:uri="/sap/bc/adt/enhancements/enhoxh/yzzz_enh" adtcore:type="ENHO/XH" adtcore:name="YZZZ_ENH" adtcore:packageName="YSD"/>
</adtcore:objectReferences>`
	searchResp := newEnhancementSearchResponse("YZZZ_ENH", "XH", "YSD")

	mock := &routedMock{
		byPath: map[string]*http.Response{
			"/sap/bc/adt/programs/includes/ZINCL_NOANCHOR/source/main":                newBody(includeBody),
			"/sap/bc/adt/enhancements/enhoxhs":                                         newBody(browserResp),
			"/sap/bc/adt/repository/informationsystem/search":                          searchResp,
			"/sap/bc/adt/enhancements/enhoxh/yzzz_enh/source/main":                     newBody(enhSource),
			"/sap/bc/adt/discovery":                                                    newBody("OK"),
		},
	}
	cfg := NewConfig("https://sap.example.com:44300", "u", "p")
	transport := NewTransportWithClient(cfg, mock)
	client := NewClientWithTransport(cfg, transport)

	merged, err := client.GetIncludeMerged(context.Background(), "ZINCL_NOANCHOR")
	if err != nil {
		t.Fatalf("GetIncludeMerged should not error on unresolved anchor, got: %v", err)
	}
	if !strings.Contains(merged, "unresolved enhancements") {
		t.Errorf("expected unresolved-enhancements banner, got:\n%s", merged)
	}
	if !strings.Contains(merged, "ENHO/XH YZZZ_ENH") {
		t.Errorf("expected enhancement banner in unresolved section, got:\n%s", merged)
	}
	if !strings.Contains(merged, "anchor unresolved") {
		t.Errorf("expected 'anchor unresolved' note, got:\n%s", merged)
	}
}

// stubRFCSourceFetcher is the test boundary for GetEnhancement's RFC fallback
// path. Records the calls the production code made; returns whatever the test
// preloaded in sourceLines/err.
type stubRFCSourceFetcher struct {
	sourceLines []string
	err         error

	readCalls []string // program names the prod code asked for
	closeFn   func()
}

func (s *stubRFCSourceFetcher) CallRFC(ctx context.Context, function string, params map[string]string) (*RFCResult, error) {
	// CallRFC is no longer the production path — kept on the stub purely so it
	// satisfies the rfcSourceFetcher interface. Tests should set sourceLines
	// and observe readCalls.
	return nil, fmtError("stub.CallRFC: not used in tests")
}

func (s *stubRFCSourceFetcher) ReadSource(ctx context.Context, program string) ([]string, error) {
	s.readCalls = append(s.readCalls, program)
	return s.sourceLines, s.err
}

func (s *stubRFCSourceFetcher) Close() error {
	if s.closeFn != nil {
		s.closeFn()
	}
	return nil
}

// TestGetEnhancement_FallsBackToRFC: classic ECC, both REST source URLs 404,
// the RPY_PROGRAM_READ bridge returns the body. GetEnhancement should return
// the spliced source instead of the metadata-only error.
func TestGetEnhancement_FallsBackToRFC(t *testing.T) {
	searchResp := newEnhancementSearchResponse("Y3EI_SKIP_BYPASS_CC_WITH_LIMIT", "XH", "YSD")

	mock := &routedMock{
		byPath: map[string]*http.Response{
			"/sap/bc/adt/repository/informationsystem/search": searchResp,
			"/sap/bc/adt/discovery":                           newBody("OK"),
			// No source/main endpoint registered — both candidate URLs 404.
		},
	}
	cfg := NewConfig("https://sap.example.com:44300", "u", "p")
	transport := NewTransportWithClient(cfg, mock)
	client := NewClientWithTransport(cfg, transport)

	stub := &stubRFCSourceFetcher{
		sourceLines: []string{
			"ENHANCEMENT 2 Y3EI_SKIP_BYPASS_CC_WITH_LIMIT.",
			"  lv_sum = lv_sum + ls_xfplt-fakwr.",
			"ENDENHANCEMENT.",
		},
	}
	closed := false
	stub.closeFn = func() { closed = true }
	client.rfcFetcherFactory = func(ctx context.Context) (rfcSourceFetcher, error) {
		return stub, nil
	}

	got, err := client.GetEnhancement(context.Background(), "Y3EI_SKIP_BYPASS_CC_WITH_LIMIT")
	if err != nil {
		t.Fatalf("GetEnhancement should fall through to RFC, got error: %v", err)
	}
	if !strings.Contains(got, "lv_sum = lv_sum + ls_xfplt-fakwr") {
		t.Fatalf("expected RFC body in result, got: %q", got)
	}
	if !strings.Contains(got, "ENDENHANCEMENT.") {
		t.Fatalf("expected RFC source lines joined with newline, got: %q", got)
	}
	if len(stub.readCalls) != 1 {
		t.Fatalf("expected exactly one ReadSource call, got %d", len(stub.readCalls))
	}
	// Convention says <ENHNAME>E unless table fallback set ref.EnhInclude.
	// Search-based discovery used here doesn't populate EnhInclude, so the
	// derived program name is the convention-based fallback.
	if got := stub.readCalls[0]; got != "Y3EI_SKIP_BYPASS_CC_WITH_LIMITE" {
		t.Errorf("wrong ReadSource program: %q", got)
	}
	if !closed {
		t.Errorf("Close() was not called on the RFC fetcher")
	}
}

// TestGetEnhancementByRef_PreservesEnhInclude: when the caller already has
// a table-discovered ref with EnhInclude populated (e.g. the include-footer
// renderer for HOOK_IMPL ENHOs whose REPOSRC entry uses `=`-padding rather
// than the simple <NAME>E convention), the RFC fallback must call ReadSource
// with that exact entry name — not the resolver's guess.
//
// Regression target: pre-fix, the include footer called GetEnhancement(name)
// which routed through resolveEnhancement, dropping EnhInclude and forcing
// the RFC step to guess "ISM_SAPLVKMPE" — which doesn't exist as a REPOSRC
// row, so all 7 non-LEGO HOOK_IMPL ENHOs on RVKMP901 rendered as
// "[source body unavailable]" even though the bridge worked.
func TestGetEnhancementByRef_PreservesEnhInclude(t *testing.T) {
	mock := &routedMock{
		byPath: map[string]*http.Response{
			// REST steps both fail: no source/main endpoint registered, and
			// the URI on the ref is empty so step 1 is skipped entirely.
			"/sap/bc/adt/discovery": newBody("OK"),
		},
	}
	cfg := NewConfig("https://sap.example.com:44300", "u", "p")
	transport := NewTransportWithClient(cfg, mock)
	client := NewClientWithTransport(cfg, transport)

	stub := &stubRFCSourceFetcher{
		sourceLines: []string{
			"ENHANCEMENT 1 ISM_SAPLVKMP.",
			"  WRITE 'hi'.",
			"ENDENHANCEMENT.",
		},
	}
	client.rfcFetcherFactory = func(ctx context.Context) (rfcSourceFetcher, error) {
		return stub, nil
	}

	// Synthetic ENHINCINX-discovered ref: short ENHNAME, padded ENHINCLUDE.
	ref := &EnhancementRef{
		Name:        "ISM_SAPLVKMP",
		Kind:        "XH",
		PackageName: "JAS_MODIF",
		HostProgram: "SAPLVKMP",
		EnhInclude:  "ISM_SAPLVKMP==================E",
	}

	got, err := client.GetEnhancementByRef(context.Background(), ref)
	if err != nil {
		t.Fatalf("GetEnhancementByRef should succeed via RFC, got error: %v", err)
	}
	if !strings.Contains(got, "WRITE 'hi'") {
		t.Fatalf("expected RFC body in result, got: %q", got)
	}
	if len(stub.readCalls) != 1 {
		t.Fatalf("expected exactly one ReadSource call, got %d", len(stub.readCalls))
	}
	// The whole point of the fix: the padded REPOSRC name from EnhInclude
	// must reach the RFC fetcher verbatim. <NAME>E ("ISM_SAPLVKMPE") would
	// be the pre-fix behaviour and is wrong.
	if got := stub.readCalls[0]; got != "ISM_SAPLVKMP==================E" {
		t.Errorf("RFC fetcher got wrong program name: %q (expected the padded EnhInclude verbatim)", got)
	}
}

// TestGetEnhancementByRef_ErrorMessageUsesEnhInclude: when all source paths
// fail and we fall back to the metadata-only error, the SE80 hint should
// point at ref.EnhInclude when available, not the convention-based guess.
func TestGetEnhancementByRef_ErrorMessageUsesEnhInclude(t *testing.T) {
	mock := &routedMock{
		byPath: map[string]*http.Response{
			"/sap/bc/adt/discovery": newBody("OK"),
		},
	}
	cfg := NewConfig("https://sap.example.com:44300", "u", "p")
	transport := NewTransportWithClient(cfg, mock)
	client := NewClientWithTransport(cfg, transport)

	client.rfcFetcherFactory = func(ctx context.Context) (rfcSourceFetcher, error) {
		return nil, fmtError("RFC unreachable")
	}

	ref := &EnhancementRef{
		Name:        "ISM_SAPLVKMP",
		Kind:        "XH",
		PackageName: "JAS_MODIF",
		EnhInclude:  "ISM_SAPLVKMP==================E",
	}

	_, err := client.GetEnhancementByRef(context.Background(), ref)
	if err == nil {
		t.Fatal("expected metadata-only error when all paths fail, got nil")
	}
	if !strings.Contains(err.Error(), "ISM_SAPLVKMP==================E") {
		t.Errorf("expected SE80 hint to use EnhInclude verbatim, got: %v", err)
	}
}

// TestGetEnhancement_RFCFails_FallsThroughToMetadataError: when the RFC path
// errors (no ZADT_VSP installed, FM not remote-callable, etc.), the user
// should still see the structured metadata-only error pointing at SE80,
// not the raw RFC error.
func TestGetEnhancement_RFCFails_FallsThroughToMetadataError(t *testing.T) {
	searchResp := newEnhancementSearchResponse("Y3EI_SKIP_BYPASS_CC_WITH_LIMIT", "XH", "YSD")

	mock := &routedMock{
		byPath: map[string]*http.Response{
			"/sap/bc/adt/repository/informationsystem/search": searchResp,
			"/sap/bc/adt/discovery":                           newBody("OK"),
		},
	}
	cfg := NewConfig("https://sap.example.com:44300", "u", "p")
	transport := NewTransportWithClient(cfg, mock)
	client := NewClientWithTransport(cfg, transport)

	client.rfcFetcherFactory = func(ctx context.Context) (rfcSourceFetcher, error) {
		// Simulate WebSocket 403 (ZADT_VSP not installed).
		return nil, fmtError("WebSocket connection failed (HTTP 403)")
	}

	_, err := client.GetEnhancement(context.Background(), "Y3EI_SKIP_BYPASS_CC_WITH_LIMIT")
	if err == nil {
		t.Fatal("expected metadata error when RFC fallback fails, got nil")
	}
	for _, want := range []string{
		"Y3EI_SKIP_BYPASS_CC_WITH_LIMIT",
		"source body unavailable",
		"SE80",
	} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("expected metadata error to contain %q, got: %v", want, err)
		}
	}
}

// fmtError is a small helper to build an error with a fixed message —
// avoids importing fmt just for one Errorf in the tests.
func fmtError(msg string) error { return errString(msg) }

type errString string

func (e errString) Error() string { return string(e) }

// TestGetEnhancement_NoSourceEndpoint_MetadataError: classic ECC behaviour.
// Search returns the ENHO ref; both candidate source URLs (singular/plural)
// 404. GetEnhancement should return a structured error that names the ENHO,
// its kind/package, and points at SE80 — instead of the cryptic 404.
func TestGetEnhancement_NoSourceEndpoint_MetadataError(t *testing.T) {
	searchResp := newEnhancementSearchResponse("Y3EI_SKIP_BYPASS_CC_WITH_LIMIT", "XH", "YSD")

	mock := &routedMock{
		byPath: map[string]*http.Response{
			"/sap/bc/adt/repository/informationsystem/search": searchResp,
			"/sap/bc/adt/discovery":                           newBody("OK"),
			// No source/main endpoint registered — both candidate URLs 404.
		},
	}
	cfg := NewConfig("https://sap.example.com:44300", "u", "p")
	transport := NewTransportWithClient(cfg, mock)
	client := NewClientWithTransport(cfg, transport)
	// Suppress the RFC fallback so the test doesn't open a real WebSocket
	// against sap.example.com (would be a 30s handshake timeout).
	client.rfcFetcherFactory = func(ctx context.Context) (rfcSourceFetcher, error) {
		return nil, fmtError("RFC fallback disabled for this test")
	}

	_, err := client.GetEnhancement(context.Background(), "Y3EI_SKIP_BYPASS_CC_WITH_LIMIT")
	if err == nil {
		t.Fatal("expected error when source endpoint is missing, got nil")
	}
	msg := err.Error()
	for _, want := range []string{
		"Y3EI_SKIP_BYPASS_CC_WITH_LIMIT",
		"XH",
		"YSD",
		"source body unavailable",
		"SE80",
	} {
		if !strings.Contains(msg, want) {
			t.Errorf("expected error to contain %q, got: %v", want, err)
		}
	}
}

// queryRoutedMock dispatches by path AND by ddicEntityName query parameter,
// so a single endpoint (datapreview/ddic) can return different bodies for
// different tables. Builds a fresh response per call so a path can be hit
// multiple times (e.g. CSRF preflight + actual POST) without body exhaustion.
type queryRoutedMock struct {
	byPathBody          map[string]string // path → body template
	byDdicEntityKeyBody map[string]string // ddicEntityName → body template
	requests            []*http.Request
}

func freshResponse(body string) *http.Response {
	h := http.Header{}
	h.Set("X-CSRF-Token", "t")
	return &http.Response{
		StatusCode: http.StatusOK,
		Body:       io.NopCloser(strings.NewReader(body)),
		Header:     h,
	}
}

func (q *queryRoutedMock) Do(req *http.Request) (*http.Response, error) {
	q.requests = append(q.requests, req)
	if entity := req.URL.Query().Get("ddicEntityName"); entity != "" {
		if body, ok := q.byDdicEntityKeyBody[strings.ToUpper(entity)]; ok {
			return freshResponse(body), nil
		}
	}
	if body, ok := q.byPathBody[req.URL.Path]; ok {
		return freshResponse(body), nil
	}
	for key, body := range q.byPathBody {
		if strings.Contains(req.URL.Path, key) {
			return freshResponse(body), nil
		}
	}
	return &http.Response{
		StatusCode: http.StatusNotFound,
		Body:       io.NopCloser(strings.NewReader("Not found")),
		Header:     http.Header{},
	}, nil
}

// dataPreviewBody renders an XML body string in the shape parseTableContents
// expects, given a list of columns and rows in declaration order.
func dataPreviewBody(columns []string, rows [][]string) string {
	var sb strings.Builder
	sb.WriteString(`<?xml version="1.0" encoding="UTF-8"?>` + "\n")
	sb.WriteString("<dataPreview>\n")
	for ci, name := range columns {
		sb.WriteString(`  <columns>` + "\n")
		sb.WriteString(`    <metadata name="` + name + `" type="C" description="" length="30" keyAttribute="false"/>` + "\n")
		sb.WriteString(`    <dataSet>` + "\n")
		for _, row := range rows {
			val := ""
			if ci < len(row) {
				val = row[ci]
			}
			sb.WriteString(`      <data>` + val + `</data>` + "\n")
		}
		sb.WriteString(`    </dataSet>` + "\n")
		sb.WriteString(`  </columns>` + "\n")
	}
	sb.WriteString("</dataPreview>")
	return sb.String()
}

// TestListEnhancementsForInclude_TablePathFallback: both REST endpoints fail;
// the D010INC ⨝ ENHINCINX fallback joins the data and returns the ENHO ref.
func TestListEnhancementsForInclude_TablePathFallback(t *testing.T) {
	d010Body := dataPreviewBody(
		[]string{"MASTER", "INCLUDE"},
		[][]string{{"SAPLVKMP", "RVKMP901"}},
	)
	enhincinxBody := dataPreviewBody(
		[]string{"ENHNAME", "PROGRAMNAME", "FULL_NAME", "ENHINCLUDE"},
		[][]string{
			{
				"Y3EI_SKIP_BYPASS_CC_WITH_LIMIT",
				"SAPLVKMP",
				`\PR:SAPLVKMP\FO:BEDINGUNG_PRUEFEN_901\SE:BEGIN\EI`,
				"Y3EI_SKIP_BYPASS_CC_WITH_LIMITE",
			},
		},
	)
	enhheaderBody := dataPreviewBody(
		[]string{"ENHNAME", "VERSION"},
		[][]string{{"Y3EI_SKIP_BYPASS_CC_WITH_LIMIT", "A"}},
	)
	searchBody := `<?xml version="1.0" encoding="UTF-8"?>
<adtcore:objectReferences xmlns:adtcore="http://www.sap.com/adt/core">
  <adtcore:objectReference adtcore:uri="/sap/bc/adt/enhancements/enhoxh/y3ei_skip_bypass_cc_with_limit" adtcore:type="ENHO/XH" adtcore:name="Y3EI_SKIP_BYPASS_CC_WITH_LIMIT" adtcore:packageName="YSD" adtcore:description="Skip bypass when CC with limit-to"/>
</adtcore:objectReferences>`

	mock := &queryRoutedMock{
		byPathBody: map[string]string{
			"/sap/bc/adt/repository/informationsystem/search": searchBody,
			"/sap/bc/adt/discovery":                           "OK",
			"/sap/bc/adt/core/discovery":                      "OK",
		},
		byDdicEntityKeyBody: map[string]string{
			"D010INC":   d010Body,
			"ENHINCINX": enhincinxBody,
			"ENHHEADER": enhheaderBody,
		},
	}
	cfg := NewConfig("https://sap.example.com:44300", "u", "p")
	transport := NewTransportWithClient(cfg, mock)
	client := NewClientWithTransport(cfg, transport)

	refs, err := client.ListEnhancementsForInclude(context.Background(), "RVKMP901")
	if err != nil {
		t.Fatalf("ListEnhancementsForInclude (table fallback) failed: %v", err)
	}
	if len(refs) != 1 {
		t.Fatalf("expected 1 ENHO match via table fallback, got %d: %+v", len(refs), refs)
	}
	got := refs[0]
	if got.Name != "Y3EI_SKIP_BYPASS_CC_WITH_LIMIT" {
		t.Errorf("wrong Name: %q", got.Name)
	}
	if got.Kind != "XH" {
		t.Errorf("wrong Kind: %q", got.Kind)
	}
	if got.HostProgram != "SAPLVKMP" {
		t.Errorf("wrong HostProgram: %q", got.HostProgram)
	}
	if got.EnhInclude != "Y3EI_SKIP_BYPASS_CC_WITH_LIMITE" {
		t.Errorf("wrong EnhInclude: %q", got.EnhInclude)
	}
	if !strings.Contains(got.FullName, "BEDINGUNG_PRUEFEN_901") {
		t.Errorf("expected FullName to mention the FORM, got: %q", got.FullName)
	}
}

func TestGetSource_DispatchesENHO(t *testing.T) {
	// End-to-end: GetSource(ctx, "ENHO", name) must take the ENHO branch.
	sourceBody := `ENHANCEMENT 2 Y_TEST.
ENDENHANCEMENT.`
	searchResp := newEnhancementSearchResponse("Y_TEST", "XH", "YSD")

	mock := &routedMock{
		byPath: map[string]*http.Response{
			"/sap/bc/adt/repository/informationsystem/search":  searchResp,
			"/sap/bc/adt/enhancements/enhoxh/y_test/source/main": newBody(sourceBody),
			"/sap/bc/adt/discovery":                              newBody("OK"),
		},
	}
	cfg := NewConfig("https://sap.example.com:44300", "u", "p")
	transport := NewTransportWithClient(cfg, mock)
	client := NewClientWithTransport(cfg, transport)

	got, err := client.GetSource(context.Background(), "ENHO", "Y_TEST", nil)
	if err != nil {
		t.Fatalf("GetSource(ENHO) failed: %v", err)
	}
	if !strings.Contains(got, "ENHANCEMENT 2 Y_TEST") {
		t.Fatalf("GetSource(ENHO) did not return enhancement body, got: %q", got)
	}
}
