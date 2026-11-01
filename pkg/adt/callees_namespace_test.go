package adt

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// In a namespace the function pool prefixes come after the namespace:
// /DEMO/LOG's main program is /DEMO/SAPLLOG and its module includes are
// /DEMO/LLOGU01 and so on. Building L/DEMO/... or stripping SAPL from the front
// of /DEMO/SAPLLOG gives an include that does not exist, and the empty answer
// reads as "this calls nothing".

func TestIncludePredicateNamespacedGroup(t *testing.T) {
	c := &Client{}
	got, err := c.includePredicate(context.Background(), calleeTarget{Name: "/DEMO/LOG", Type: "FUGR"})
	if err != nil {
		t.Fatalf("includePredicate: %v", err)
	}
	if want := "INCLUDE LIKE '/DEMO/LLOG%'"; got != want {
		t.Errorf("includePredicate = %q, want %q", got, want)
	}
}

func TestPoolIncludeForKeepsTheNamespaceInFront(t *testing.T) {
	if got := poolIncludeFor("/DEMO/LOG", "3"); got != "/DEMO/LLOGU03" {
		t.Errorf("poolIncludeFor(/DEMO/LOG, 3) = %q, want /DEMO/LLOGU03", got)
	}
}

func TestIncludeBelongsToNamespacedGroup(t *testing.T) {
	group := calleeTarget{Name: "/DEMO/LOG", Type: "FUGR"}
	if !includeBelongsTo("/DEMO/LLOGU01", group) {
		t.Error("a namespaced function pool section was rejected")
	}
	if !includeBelongsTo("/DEMO/LLOGTOP", group) {
		t.Error("a namespaced TOP include was rejected")
	}
	if includeBelongsTo("/DEMO/LLOGGINGU01", group) {
		t.Error("another group's include was accepted because the names share a prefix")
	}
	if includeBelongsTo("/OTHER/LLOGU01", group) {
		t.Error("the same group name in another namespace was accepted")
	}
}

// The whole path, TFDIR included: the module's include has to be the one the
// tables are keyed by, or both queries come back empty.
func TestCalleesOfANamespacedFunctionModule(t *testing.T) {
	var asked []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("x-csrf-token", "test-token")
		if r.Method == http.MethodHead || r.Method == http.MethodGet {
			w.WriteHeader(http.StatusOK)
			return
		}
		body, _ := io.ReadAll(r.Body)
		sql := string(body)
		asked = append(asked, sql)
		w.Header().Set("Content-Type", "application/xml")
		const own = "INCLUDE = '/DEMO/LLOGU03'"
		switch {
		case strings.Contains(sql, " FROM TFDIR "):
			w.Write([]byte(tableXML(col("PNAME", "/DEMO/SAPLLOG"), col("INCLUDE", "3"))))
		case strings.Contains(sql, " FROM WBCROSSGT ") && strings.Contains(sql, own):
			w.Write([]byte(tableXML(
				col("INCLUDE", "/DEMO/LLOGU03"),
				col("OTYPE", "ME"),
				col("NAME", `/DEMO/CL_LOG_STORE\ME:SAVE`),
				col("DIRECT", "X"),
			)))
		case strings.Contains(sql, " FROM CROSS ") && strings.Contains(sql, own):
			w.Write([]byte(tableXML(
				col("INCLUDE", "/DEMO/LLOGU03"),
				col("TYPE", "F"),
				col("NAME", "/DEMO/LOG_FLUSH"),
				col("PROG", ""),
			)))
		default:
			w.Write([]byte(tableXML()))
		}
	}))
	defer srv.Close()

	client := NewClient(srv.URL, "user", "pass")
	callees, _, err := client.Callees(context.Background(),
		"/sap/bc/adt/functions/groups/%2fdemo%2flog/fmodules/%2fdemo%2flog_write")
	if err != nil {
		t.Fatalf("Callees: %v", err)
	}
	names := map[string]bool{}
	for _, c := range callees {
		names[c.Name] = true
	}
	if !names["/DEMO/CL_LOG_STORE"] || !names["/DEMO/LOG_FLUSH"] {
		t.Errorf("both tables hold rows for /DEMO/LLOGU03, got %+v; queries sent: %q", callees, asked)
	}
}
