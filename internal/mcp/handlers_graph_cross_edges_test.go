package mcp

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/oisee/vibing-steampunk/pkg/adt"
	"github.com/oisee/vibing-steampunk/pkg/graph"
)

const previewXMLHeader = `<?xml version="1.0" encoding="utf-8"?>` +
	`<dataPreview:tableData xmlns:dataPreview="http://www.sap.com/adt/dataPreview">`

// crossEdgesSAP answers the CROSS query with one row per type code. WBCROSSGT
// answers with no rows, so every edge in the graph comes from the CROSS branch.
func crossEdgesSAP(t *testing.T, codes ...string) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-CSRF-Token", "test-token")
		w.Header().Set("Content-Type", "application/xml")
		if r.Method != http.MethodPost {
			w.WriteHeader(http.StatusOK)
			return
		}
		body, _ := io.ReadAll(r.Body)
		if !strings.Contains(string(body), " FROM CROSS ") {
			_, _ = w.Write([]byte(previewXMLHeader +
				`<dataPreview:columns><dataPreview:metadata dataPreview:name="INCLUDE" ` +
				`dataPreview:type="C" dataPreview:length="40"/><dataPreview:dataSet></dataPreview:dataSet>` +
				`</dataPreview:columns></dataPreview:tableData>`))
			return
		}
		_, _ = w.Write([]byte(crossRowsXML(codes)))
	}))
	t.Cleanup(srv.Close)
	return srv
}

func crossRowsXML(codes []string) string {
	var b strings.Builder
	b.WriteString(previewXMLHeader)
	b.WriteString(`<dataPreview:columns><dataPreview:metadata dataPreview:name="INCLUDE" ` +
		`dataPreview:type="C" dataPreview:length="40"/><dataPreview:dataSet>`)
	for i := range codes {
		fmt.Fprintf(&b, `<dataPreview:data>ZDEMO_CALLER%d</dataPreview:data>`, i)
	}
	b.WriteString(`</dataPreview:dataSet></dataPreview:columns>`)
	b.WriteString(`<dataPreview:columns><dataPreview:metadata dataPreview:name="TYPE" ` +
		`dataPreview:type="C" dataPreview:length="1"/><dataPreview:dataSet>`)
	for _, code := range codes {
		fmt.Fprintf(&b, `<dataPreview:data>%s</dataPreview:data>`, code)
	}
	b.WriteString(`</dataPreview:dataSet></dataPreview:columns></dataPreview:tableData>`)
	return b.String()
}

// F and R are call sites; every other CROSS type is a reference. The codes are
// written as literals from the recorded CROSS vocabulary, so a changed
// production constant is caught here rather than followed.
func TestCrossEdgesAreCallsOnlyForCallAndReportCodes(t *testing.T) {
	srv := crossEdgesSAP(t, "F", "R", "S")
	s := &Server{adtClient: adt.NewClient(srv.URL, "TESTUSER", "secret")}

	g, err := s.fetchReverseDeps(context.Background(), "FUNC", "ZDEMO_FM", 1)
	if err != nil {
		t.Fatalf("fetchReverseDeps: %v", err)
	}

	want := map[string]graph.EdgeKind{
		"F": graph.EdgeCalls,
		"R": graph.EdgeCalls,
		"S": graph.EdgeReferences,
	}
	seen := map[string]bool{}
	for _, e := range g.Edges() {
		code, ok := strings.CutPrefix(e.RefDetail, "TYPE:")
		if !ok || e.Source != graph.SourceCROSS {
			continue
		}
		seen[code] = true
		if e.Kind != want[code] {
			t.Errorf("TYPE:%s → %s, want %s", code, e.Kind, want[code])
		}
	}
	for code := range want {
		if !seen[code] {
			t.Errorf("no CROSS edge for TYPE:%s", code)
		}
	}
}
