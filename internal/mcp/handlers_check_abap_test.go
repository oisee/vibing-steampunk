package mcp

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"testing"

	"github.com/mark3labs/mcp-go/mcp"
)

// A check that worked but left its temporary program in $TMP is a failed
// call: the caller has to learn the program's name and delete it.
func TestHandleCheckABAPLeftoverProgramIsAnError(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-CSRF-Token", "TOKEN")
		switch {
		case strings.Contains(r.URL.Path, "/checkruns"):
			w.Header().Set("Content-Type", "application/xml")
			_, _ = io.WriteString(w, `<?xml version="1.0" encoding="utf-8"?><chkrun:checkRunReports xmlns:chkrun="http://www.sap.com/adt/checkrun"><chkrun:checkReport chkrun:reporter="abapCheckRun" chkrun:status="processed" chkrun:statusText="checked"/></chkrun:checkRunReports>`)
		case r.Method == http.MethodPost && r.URL.Query().Get("_action") == "LOCK":
			w.Header().Set("Content-Type", "application/vnd.sap.as+xml")
			_, _ = io.WriteString(w, `<?xml version="1.0" encoding="UTF-8"?><asx:abap xmlns:asx="http://www.sap.com/abapxml"><asx:values><DATA><LOCK_HANDLE>HANDLE-1</LOCK_HANDLE></DATA></asx:values></asx:abap>`)
		case r.Method == http.MethodDelete:
			w.WriteHeader(http.StatusInternalServerError)
		default:
			w.WriteHeader(http.StatusOK)
		}
	}))
	t.Cleanup(ts.Close)

	server := NewServer(&Config{BaseURL: ts.URL, Username: "u", Password: "p", Client: "001", Language: "EN"})
	res, err := server.handleCheckABAP(context.Background(), newRequest(map[string]any{"code": "DATA lv TYPE i."}))
	if err != nil {
		t.Fatalf("handleCheckABAP: %v", err)
	}
	text := res.Content[0].(mcp.TextContent).Text
	if !res.IsError {
		t.Fatalf("a leftover temporary program was reported as success:\n%s", text)
	}
	name := regexp.MustCompile(`ZVSP_CHK_U_[0-9]`).FindString(text)
	if name == "" || !strings.HasPrefix(text, "check_abap left the temporary program "+name+" in $TMP") {
		t.Fatalf("error does not lead with the program's name:\n%s", text)
	}
	// The check's own result is still there.
	if !strings.Contains(text, `"ok": true`) || !strings.Contains(text, `"cleanedUp": false`) {
		t.Fatalf("result payload missing:\n%s", text)
	}
}
