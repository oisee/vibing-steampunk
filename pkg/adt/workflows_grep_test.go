package adt

import (
	"context"
	"net/http"
	"strings"
	"testing"
)

// TestGrepObjectWithEnhancements_HitInsideEnhancementBody: the host include
// has no match in its raw source, but an attached ENHO body contains the
// pattern. The grep result must include the hit, prefixed with the ENHO
// tag so callers can tell it came from an enhancement.
func TestGrepObjectWithEnhancements_HitInsideEnhancementBody(t *testing.T) {
	baseSource := `FORM foo.
  WRITE 'no pattern here'.
ENDFORM.
`
	enhSource := `ENHANCEMENT 2 Y3EI_TEST.
  LOOP AT lt_xfplt INTO DATA(ls_xfplt).
    lv_sum = lv_sum + ls_xfplt-fakwr.
  ENDLOOP.
ENDENHANCEMENT.
`
	browserResp := `<?xml version="1.0" encoding="UTF-8"?>
<adtcore:objectReferences xmlns:adtcore="http://www.sap.com/adt/core">
  <adtcore:objectReference adtcore:uri="/sap/bc/adt/enhancements/enhoxh/y3ei_test" adtcore:type="ENHO/XH" adtcore:name="Y3EI_TEST" adtcore:packageName="YSD"/>
</adtcore:objectReferences>`

	mock := &routedMock{
		byPath: map[string]*http.Response{
			"/sap/bc/adt/programs/includes/RVKMP901/source/main":          newBody(baseSource),
			"/sap/bc/adt/enhancements/enhoxhs":                            newBody(browserResp),
			"/sap/bc/adt/enhancements/enhoxh/y3ei_test/source/main":       newBody(enhSource),
			"/sap/bc/adt/discovery":                                       newBody("OK"),
		},
	}
	cfg := NewConfig("https://sap.example.com:44300", "u", "p")
	transport := NewTransportWithClient(cfg, mock)
	client := NewClientWithTransport(cfg, transport)

	result, err := client.GrepObjectWithEnhancements(
		context.Background(),
		"/sap/bc/adt/programs/includes/RVKMP901",
		`lv_sum\s*=\s*lv_sum\s*\+\s*ls_xfplt-fakwr`,
		false, 0, nil,
	)
	if err != nil {
		t.Fatalf("GrepObjectWithEnhancements failed: %v", err)
	}
	if result.MatchCount == 0 {
		t.Fatalf("expected ENHO hit; got 0 matches. Result: %+v", result)
	}

	found := false
	for _, m := range result.Matches {
		if strings.Contains(m.MatchedLine, "[ENHO Y3EI_TEST @ RVKMP901]") &&
			strings.Contains(m.MatchedLine, "lv_sum = lv_sum + ls_xfplt-fakwr") {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected ENHO-tagged hit; got matches:\n")
		for _, m := range result.Matches {
			t.Errorf("  %d: %s", m.LineNumber, m.MatchedLine)
		}
	}
}

// TestGrepObjectWithEnhancements_BridgeDown_DegradesGracefully: the ENHO is
// listed but its body fetch fails (REST 404 + RFC bridge dead). The grep
// must continue, surface a warning hit per ref, and not error the call.
func TestGrepObjectWithEnhancements_BridgeDown_DegradesGracefully(t *testing.T) {
	baseSource := `FORM foo.
ENDFORM.
`
	browserResp := `<?xml version="1.0" encoding="UTF-8"?>
<adtcore:objectReferences xmlns:adtcore="http://www.sap.com/adt/core">
  <adtcore:objectReference adtcore:uri="/sap/bc/adt/enhancements/enhoxh/y_lost" adtcore:type="ENHO/XH" adtcore:name="Y_LOST" adtcore:packageName="YSD"/>
</adtcore:objectReferences>`

	// No ENHO source endpoint registered → REST steps both 404.
	mock := &routedMock{
		byPath: map[string]*http.Response{
			"/sap/bc/adt/programs/includes/RVKMP901/source/main": newBody(baseSource),
			"/sap/bc/adt/enhancements/enhoxhs":                   newBody(browserResp),
			"/sap/bc/adt/discovery":                              newBody("OK"),
		},
	}
	cfg := NewConfig("https://sap.example.com:44300", "u", "p")
	transport := NewTransportWithClient(cfg, mock)
	client := NewClientWithTransport(cfg, transport)

	// Force the RFC fallback to fail too — simulates ZADT_VSP bridge down.
	client.rfcFetcherFactory = func(ctx context.Context) (rfcSourceFetcher, error) {
		return nil, fmtError("WebSocket connection failed (HTTP 403)")
	}

	result, err := client.GrepObjectWithEnhancements(
		context.Background(),
		"/sap/bc/adt/programs/includes/RVKMP901",
		`lv_sum`,
		false, 0, nil,
	)
	if err != nil {
		t.Fatalf("GrepObjectWithEnhancements should soft-fail on bridge-down, got: %v", err)
	}

	foundWarning := false
	for _, m := range result.Matches {
		if strings.Contains(m.MatchedLine, "ENHO/XH Y_LOST @ RVKMP901") &&
			strings.Contains(m.MatchedLine, "body unavailable") {
			foundWarning = true
			break
		}
	}
	if !foundWarning {
		t.Errorf("expected warning hit for unfetchable ENHO body; got matches:\n")
		for _, m := range result.Matches {
			t.Errorf("  %d: %s", m.LineNumber, m.MatchedLine)
		}
	}
}

// TestGrepObjectWithEnhancements_DedupesViaWalkState: when two GrepObject
// calls share a walk state and the same ENHO is listed for both objects,
// the body is fetched at most once.
func TestGrepObjectWithEnhancements_DedupesViaWalkState(t *testing.T) {
	baseSource := `FORM foo.
ENDFORM.
`
	enhSource := `ENHANCEMENT 2 Y_SHARED.
  WRITE 'shared'.
ENDENHANCEMENT.
`
	// Same ENHO listed for both includes — typical of a function group's
	// CXX/EXX/F0X includes all sharing one HOOK_IMPL plug-in.
	browserResp := `<?xml version="1.0" encoding="UTF-8"?>
<adtcore:objectReferences xmlns:adtcore="http://www.sap.com/adt/core">
  <adtcore:objectReference adtcore:uri="/sap/bc/adt/enhancements/enhoxh/y_shared" adtcore:type="ENHO/XH" adtcore:name="Y_SHARED" adtcore:packageName="YSD"/>
</adtcore:objectReferences>`

	// Counter to assert how many times the ENHO body was fetched.
	fetchCount := 0
	mock := &routedMock{
		byPath: map[string]*http.Response{
			"/sap/bc/adt/programs/includes/INC_A/source/main": newBody(baseSource),
			"/sap/bc/adt/programs/includes/INC_B/source/main": newBody(baseSource),
			"/sap/bc/adt/enhancements/enhoxhs":                newBody(browserResp),
			"/sap/bc/adt/discovery":                           newBody("OK"),
		},
	}
	// Wrap the mock to count source/main fetches on the ENHO URL.
	countingMock := &countingTransportClient{
		inner:    mock,
		matchURL: "/sap/bc/adt/enhancements/enhoxh/y_shared/source/main",
		body:     enhSource,
		count:    &fetchCount,
	}
	cfg := NewConfig("https://sap.example.com:44300", "u", "p")
	transport := NewTransportWithClient(cfg, countingMock)
	client := NewClientWithTransport(cfg, transport)

	walk := NewGrepEnhancementsState(0) // default cap

	if _, err := client.GrepObjectWithEnhancements(
		context.Background(),
		"/sap/bc/adt/programs/includes/INC_A",
		`shared`, false, 0, walk,
	); err != nil {
		t.Fatalf("first call failed: %v", err)
	}
	if _, err := client.GrepObjectWithEnhancements(
		context.Background(),
		"/sap/bc/adt/programs/includes/INC_B",
		`shared`, false, 0, walk,
	); err != nil {
		t.Fatalf("second call failed: %v", err)
	}

	if fetchCount != 1 {
		t.Errorf("expected ENHO body fetched once across two grep calls, got %d", fetchCount)
	}
}

// countingTransportClient wraps a mock and counts requests to a specific URL,
// returning the configured body for that URL and delegating everything else.
type countingTransportClient struct {
	inner    *routedMock
	matchURL string
	body     string
	count    *int
}

func (c *countingTransportClient) Do(req *http.Request) (*http.Response, error) {
	if req.URL.Path == c.matchURL {
		*c.count++
		return newBody(c.body), nil
	}
	return c.inner.Do(req)
}
