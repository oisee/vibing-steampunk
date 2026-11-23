package adt

import (
	"context"
	"net/http"
	"strings"
	"testing"
)

// EditSource passes the caller's object URL to the syntax pre-check as is, and
// callers often give the source URL (.../source/main) — it is the URL ADT
// itself shows for a source. postCheckRun appended /source/main again, and SAP
// answered 500 "URI-Mapping cannot be performed" (seen on S/4HANA 2023), so an
// edit with syntax_check=true could not be saved at all.
func TestCheckRunDoesNotDoubleSourceMain(t *testing.T) {
	for _, url := range []string{
		"/sap/bc/adt/programs/programs/zdemo/source/main",
		"/sap/bc/adt/programs/programs/zdemo",
	} {
		m := &activateHTTP{}
		_, _ = m.client("dev").SyntaxCheck(context.Background(), url, "REPORT zdemo.")

		var body string
		for _, c := range m.calls {
			if c.method == http.MethodPost && strings.Contains(c.path, "/checkruns") {
				body = c.body
			}
		}
		if body == "" {
			t.Fatalf("%s: no check run was posted", url)
		}
		if strings.Contains(body, "/source/main/source/main") {
			t.Errorf("%s: /source/main doubled in the check run: %s", url, body)
		}
		if !strings.Contains(body, `adtcore:uri="/sap/bc/adt/programs/programs/zdemo"`) {
			t.Errorf("%s: checkObject should be the object URL without /source/main: %s", url, body)
		}
		if !strings.Contains(body, `chkrun:uri="/sap/bc/adt/programs/programs/zdemo/source/main"`) {
			t.Errorf("%s: artifact should be the source URL: %s", url, body)
		}
	}
}
