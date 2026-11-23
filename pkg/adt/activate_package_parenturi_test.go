package adt

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
)

// Some systems (S/4HANA 2023 seen) send the inactive-objects feed without
// adtcore:parentUri. The package filter read only that attribute, so every
// object looked like it belonged to no package: ActivatePackage("ZDEMO_PKG")
// reported "Activated 0 objects, 0 failed" and ActivatePackageIterative
// reported "All objects active — deployment verified" with the package's
// objects still inactive. The package is resolved from the repository search
// instead, and an object whose package cannot be resolved is reported, not
// silently dropped.

const inactiveFeedWithoutParentURI = `<?xml version="1.0" encoding="UTF-8"?>
<ioc:inactiveObjects xmlns:ioc="http://www.sap.com/adt/inactivectsobjects">
  <ioc:entry><ioc:object ioc:user="TESTUSER">
    <ioc:ref adtcore:uri="/sap/bc/adt/programs/includes/zdemo_f01" adtcore:type="PROG/I" adtcore:name="ZDEMO_F01"
             xmlns:adtcore="http://www.sap.com/adt/core"/>
  </ioc:object></ioc:entry>
  <ioc:entry><ioc:object ioc:user="TESTUSER">
    <ioc:ref adtcore:uri="/sap/bc/adt/programs/programs/zother" adtcore:type="PROG/P" adtcore:name="ZOTHER"
             xmlns:adtcore="http://www.sap.com/adt/core"/>
  </ioc:object></ioc:entry>
  <ioc:entry><ioc:object ioc:user="TESTUSER">
    <ioc:ref adtcore:uri="/sap/bc/adt/programs/programs/zghost" adtcore:type="PROG/P" adtcore:name="ZGHOST"
             xmlns:adtcore="http://www.sap.com/adt/core"/>
  </ioc:object></ioc:entry>
</ioc:inactiveObjects>`

type parentURIServer struct {
	mu        sync.Mutex
	activated []string
}

func (s *parentURIServer) start(t *testing.T) *Client {
	t.Helper()
	packages := map[string]string{ // search answer per object name; ZGHOST has none
		"ZDEMO_F01": `<adtcore:objectReference adtcore:uri="/sap/bc/adt/programs/includes/zdemo_f01" adtcore:type="PROG/I" adtcore:name="ZDEMO_F01" adtcore:packageName="ZDEMO_PKG"/>`,
		"ZOTHER":   `<adtcore:objectReference adtcore:uri="/sap/bc/adt/programs/programs/zother" adtcore:type="PROG/P" adtcore:name="ZOTHER" adtcore:packageName="ZOTHERPKG"/>`,
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.Path
		switch {
		case strings.Contains(path, "/discovery"):
			w.Header().Set("X-CSRF-Token", "TOKEN")
			w.WriteHeader(http.StatusOK)
		case strings.Contains(path, "informationsystem/search"):
			w.Header().Set("Content-Type", "application/xml")
			name := strings.ToUpper(strings.TrimSuffix(r.URL.Query().Get("query"), "*"))
			_, _ = w.Write([]byte(`<?xml version="1.0" encoding="UTF-8"?>
<adtcore:objectReferences xmlns:adtcore="http://www.sap.com/adt/core">` + packages[name] + `</adtcore:objectReferences>`))
		case strings.Contains(path, "/activation/inactiveobjects"):
			w.Header().Set("Content-Type", "application/xml")
			_, _ = w.Write([]byte(inactiveFeedWithoutParentURI))
		case strings.Contains(path, "/activation"):
			s.mu.Lock()
			s.activated = append(s.activated, r.Method+" "+path)
			s.mu.Unlock()
			w.WriteHeader(http.StatusOK) // empty body: activated
		default:
			w.WriteHeader(http.StatusOK)
		}
	}))
	t.Cleanup(srv.Close)
	cfg := NewConfig(srv.URL, "TESTUSER", "secret")
	return NewClientWithTransport(cfg, NewTransport(cfg))
}

func TestActivatePackageResolvesPackageWhenParentURIIsMissing(t *testing.T) {
	srv := &parentURIServer{}
	client := srv.start(t)

	result, err := client.ActivatePackage(context.Background(), "zdemo_pkg", 0)
	if err != nil {
		t.Fatalf("ActivatePackage: %v", err)
	}
	if len(result.Activated) != 1 || result.Activated[0].Name != "ZDEMO_F01" {
		t.Fatalf("want only ZDEMO_F01 activated, got %+v (failed %+v)", result.Activated, result.Failed)
	}
	if len(result.Skipped) != 1 || !strings.Contains(result.Skipped[0], "ZGHOST") {
		t.Fatalf("an object of unknown package must be reported in Skipped, got %+v", result.Skipped)
	}
	if !strings.Contains(result.Summary, "unknown package") {
		t.Fatalf("summary must say objects were skipped for unknown package: %q", result.Summary)
	}
}

func TestActivatePackageIterativeDoesNotClaimVerifiedWhenParentURIIsMissing(t *testing.T) {
	srv := &parentURIServer{} // the feed never empties: ZDEMO_F01 stays inactive
	client := srv.start(t)

	result, err := client.ActivatePackageIterative(context.Background(), "ZDEMO_PKG", 2)
	if err != nil {
		t.Fatalf("ActivatePackageIterative: %v", err)
	}
	if result.StillInactive == 0 || strings.Contains(result.Summary, "deployment verified") {
		t.Fatalf("an object still inactive in the package was not counted: %+v", result)
	}
}
