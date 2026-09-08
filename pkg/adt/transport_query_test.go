package adt

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"
)

// The organizer tree as ABAP 7.57 renders it with targets: a tm:project level
// between target and bucket, both buckets present, workbench and customizing.
const treeWithProjects = `<?xml version="1.0" encoding="utf-8"?>
<tm:root xmlns:tm="http://www.sap.com/cts/adt/tm" xmlns:adtcore="http://www.sap.com/adt/core" adtcore:name="DEVELOPER">
  <tm:workbench tm:category="Workbench">
    <tm:target tm:name="/TARGET/" tm:desc="Target group">
      <tm:project tm:projectId="PRJ1" tm:name="PROJECT_1" tm:desc="First project">
        <tm:modifiable tm:status="Modifiable">
          <tm:request tm:number="TR-MOD-1" tm:owner="DEVELOPER" tm:desc="Modifiable one" tm:type="K" tm:status="D">
            <tm:task tm:number="TR-MOD-1-T" tm:parent="TR-MOD-1" tm:owner="DEVELOPER" tm:desc="Task" tm:type="Development" tm:status="D">
              <tm:abap_object tm:pgmid="R3TR" tm:type="PROG" tm:name="ZPROG" tm:wbtype="PROG/P"/>
            </tm:task>
          </tm:request>
        </tm:modifiable>
        <tm:released tm:status="Released (last 2 weeks)">
          <tm:request tm:number="TR-REL-1" tm:owner="OTHER" tm:desc="Released one" tm:type="K" tm:status="R"/>
        </tm:released>
      </tm:project>
      <tm:project tm:projectId="Not_assigned_to_a_project" tm:name="" tm:desc="Requests not assigned to any project">
        <tm:modifiable tm:status="Modifiable">
          <tm:request tm:number="TR-MOD-2" tm:owner="DEVELOPER" tm:desc="Modifiable two" tm:type="K" tm:status="D"/>
        </tm:modifiable>
      </tm:project>
    </tm:target>
  </tm:workbench>
  <tm:customizing tm:category="Customizing">
    <tm:target tm:name="/TARGET/" tm:desc="Target group">
      <tm:project tm:projectId="PRJ1" tm:name="PROJECT_1" tm:desc="First project">
        <tm:modifiable tm:status="Modifiable">
          <tm:request tm:number="TR-CUS-1" tm:owner="OTHER" tm:desc="Customizing one" tm:type="W" tm:status="D"/>
        </tm:modifiable>
      </tm:project>
    </tm:target>
  </tm:customizing>
</tm:root>`

const emptyTree = `<?xml version="1.0" encoding="utf-8"?>
<tm:root xmlns:tm="http://www.sap.com/cts/adt/tm"></tm:root>`

const configurationsList = `<?xml version="1.0" encoding="utf-8"?>
<configurations:configurations xmlns:configurations="http://www.sap.com/adt/configurations">
  <configuration:configuration createdBy="DEVELOPER" xmlns:configuration="http://www.sap.com/adt/configuration">
    <atom:link href="/sap/bc/adt/cts/transportrequests/searchconfiguration/configurations?sap-client=001&amp;sap-language=EN/CFG-DEVELOPER" rel="http://www.sap.com/adt/categories/configurations" type="application/vnd.sap.adt.configuration.v1+xml" xmlns:atom="http://www.w3.org/2005/Atom"/>
  </configuration:configuration>
  <configuration:configuration createdBy="OTHER" xmlns:configuration="http://www.sap.com/adt/configuration">
    <atom:link href="/sap/bc/adt/cts/transportrequests/searchconfiguration/configurations/CFG-OTHER" rel="http://www.sap.com/adt/categories/configurations" type="application/vnd.sap.adt.configuration.v1+xml" xmlns:atom="http://www.w3.org/2005/Atom"/>
  </configuration:configuration>
</configurations:configurations>`

func configurationFor(user string) string {
	return `<?xml version="1.0" encoding="utf-8"?>
<configuration:configuration createdBy="` + user + `" changedAt="2025-03-21T06:46:52Z" client="001" xmlns:configuration="http://www.sap.com/adt/configuration">
  <configuration:properties>
    <configuration:property key="WorkbenchRequests" isMandatory="true">true</configuration:property>
    <configuration:property key="CustomizingRequests" isMandatory="true">true</configuration:property>
    <configuration:property key="TransportOfCopies" isMandatory="true">true</configuration:property>
    <configuration:property key="Modifiable" isMandatory="true">true</configuration:property>
    <configuration:property key="Released" isMandatory="true">false</configuration:property>
    <configuration:property key="User" isMandatory="true">` + user + `</configuration:property>
    <configuration:property key="DateFilter" isMandatory="true">1</configuration:property>
  </configuration:properties>
</configuration:configuration>`
}

// sequencedClient answers by path and query, one canned response per call,
// so a fallback chain can be scripted request by request.
type sequencedClient struct {
	t        *testing.T
	answers  map[string][]string // key: path, or path + "?" + a query substring
	requests []*http.Request
}

func (m *sequencedClient) Do(req *http.Request) (*http.Response, error) {
	m.requests = append(m.requests, req)
	key := req.URL.Path
	for k := range m.answers {
		if strings.HasPrefix(k, key+"?") && strings.Contains(req.URL.RawQuery, strings.TrimPrefix(k, key+"?")) {
			key = k
			break
		}
	}
	bodies, ok := m.answers[key]
	if !ok || len(bodies) == 0 {
		m.t.Logf("no canned answer for %s?%s", req.URL.Path, req.URL.RawQuery)
		return &http.Response{StatusCode: http.StatusNotFound, Body: io.NopCloser(strings.NewReader("")), Header: http.Header{}}, nil
	}
	body := bodies[0]
	m.answers[key] = bodies[1:]
	return &http.Response{
		StatusCode: http.StatusOK,
		Body:       io.NopCloser(strings.NewReader(body)),
		Header:     http.Header{"X-CSRF-Token": []string{"token"}, "Content-Type": []string{"application/xml"}},
	}, nil
}

func newTransportTestClient(t *testing.T, answers map[string][]string) (*Client, *sequencedClient) {
	mock := &sequencedClient{t: t, answers: answers}
	cfg := NewConfig("https://sap.example.com", "developer", "secret")
	cfg.Safety.EnableTransports = true
	tr := NewTransportWithClient(cfg, mock)
	return NewClientWithTransport(cfg, tr), mock
}

func TestParseUserTransportsProjectLevel(t *testing.T) {
	result, err := parseUserTransports([]byte(treeWithProjects))
	if err != nil {
		t.Fatalf("parseUserTransports: %v", err)
	}
	if len(result.Workbench) != 3 {
		t.Fatalf("workbench requests = %d, want 3", len(result.Workbench))
	}
	if len(result.Customizing) != 1 {
		t.Fatalf("customizing requests = %d, want 1", len(result.Customizing))
	}
	byNumber := map[string]TransportRequest{}
	for _, r := range append(result.Workbench, result.Customizing...) {
		byNumber[r.Number] = r
	}
	if got := byNumber["TR-MOD-1"]; got.Bucket != "modifiable" || got.Project != "PROJECT_1" || got.ProjectID != "PRJ1" {
		t.Errorf("TR-MOD-1 = bucket %q project %q/%q, want modifiable PROJECT_1/PRJ1", got.Bucket, got.Project, got.ProjectID)
	}
	if got := byNumber["TR-REL-1"]; got.Bucket != "released" || got.Status != "R" {
		t.Errorf("TR-REL-1 = bucket %q status %q, want released R", got.Bucket, got.Status)
	}
	if got := byNumber["TR-MOD-2"]; got.Bucket != "modifiable" || got.ProjectID != "Not_assigned_to_a_project" {
		t.Errorf("TR-MOD-2 = bucket %q projectId %q", got.Bucket, got.ProjectID)
	}
	if got := byNumber["TR-MOD-1"]; len(got.Tasks) != 1 || len(got.Tasks[0].Objects) != 1 {
		t.Errorf("TR-MOD-1 tasks/objects not carried through the project level: %+v", got.Tasks)
	}
}

func TestTransportQueryDefaults(t *testing.T) {
	q, err := TransportQuery{}.normalized("developer")
	if err != nil {
		t.Fatal(err)
	}
	if q.User != "DEVELOPER" || q.RequestTypes != "KWT" || q.RequestStatuses != "DR" || q.Source != TransportSourceAuto {
		t.Errorf("defaults = %+v", q)
	}

	q, err = TransportQuery{ConfigURI: "/sap/bc/adt/cts/transportrequests/searchconfiguration/configurations/X"}.normalized("developer")
	if err != nil || q.Source != TransportSourceConfig {
		t.Errorf("config_uri alone should select source config, got %q (%v)", q.Source, err)
	}

	for _, bad := range []TransportQuery{
		{RequestTypes: "KX"},
		{RequestStatuses: "Q"},
		{ReleasedFrom: "2026-01-01", ReleasedTo: "20261231"},
		{ReleasedFrom: "20260101"},
		{Source: "eclipse"},
	} {
		if _, err := bad.normalized("developer"); err == nil {
			t.Errorf("%+v should be rejected", bad)
		}
	}
}

func TestQueryTransportsParamsSendsTypeAndStatus(t *testing.T) {
	client, mock := newTransportTestClient(t, map[string][]string{
		transportRequestsPath: {treeWithProjects},
	})

	res, err := client.QueryTransports(context.Background(), TransportQuery{
		User: "developer", Targets: true, Source: TransportSourceParams,
		RequestStatuses: "D", ReleasedFrom: "20260101", ReleasedTo: "20261231",
	})
	if err != nil {
		t.Fatalf("QueryTransports: %v", err)
	}
	if res.Source != TransportSourceParams {
		t.Errorf("source = %q, want params", res.Source)
	}

	query := mock.requests[len(mock.requests)-1].URL.Query()
	for key, want := range map[string]string{
		"user": "DEVELOPER", "targets": "true", "requestType": "KWT", "requestStatus": "D",
		"releasedFromDate": "20260101", "releasedToDate": "20261231",
	} {
		if got := query.Get(key); got != want {
			t.Errorf("query %s = %q, want %q", key, got, want)
		}
	}
	if query.Has("configUri") {
		t.Errorf("params source must not send configUri")
	}
}

func TestQueryTransportsAutoFallsBackToConfigThenSQL(t *testing.T) {
	// The tree answers nothing for the explicit parameters, the saved
	// configuration of another user answers the full tree.
	client, mock := newTransportTestClient(t, map[string][]string{
		transportRequestsPath + "?requestStatus":      {emptyTree},
		transportRequestsPath + "?configUri":          {treeWithProjects},
		transportSearchConfigsPath:                    {configurationsList},
		transportSearchConfigsPath + "/CFG-DEVELOPER": {configurationFor("DEVELOPER")},
		transportSearchConfigsPath + "/CFG-OTHER":     {configurationFor("OTHER")},
	})

	res, err := client.QueryTransports(context.Background(), TransportQuery{User: "SOMEONE", Targets: true})
	if err != nil {
		t.Fatalf("QueryTransports: %v", err)
	}
	if res.Source != TransportSourceConfig {
		t.Fatalf("source = %q, want config (notes: %v)", res.Source, res.Notes)
	}
	if res.ConfigURI != transportSearchConfigsPath+"/CFG-DEVELOPER" {
		t.Errorf("configUri = %q, want the first configuration rebuilt from its id", res.ConfigURI)
	}
	if len(res.Transports.Workbench) != 3 {
		t.Errorf("workbench = %d, want 3", len(res.Transports.Workbench))
	}

	var sawParams, sawConfig bool
	for _, r := range mock.requests {
		if r.URL.Path != transportRequestsPath {
			continue
		}
		if r.URL.Query().Get("requestStatus") == "DR" {
			sawParams = true
		}
		if r.URL.Query().Get("configUri") != "" {
			sawConfig = true
		}
	}
	if !sawParams || !sawConfig {
		t.Errorf("expected the tree to be asked with parameters first and via configUri second (params=%v config=%v)", sawParams, sawConfig)
	}

	joined := strings.Join(res.Notes, "\n")
	for _, want := range []string{"returned no requests", "no search configuration names user SOMEONE", "excludes released requests", "were not applied"} {
		if !strings.Contains(joined, want) {
			t.Errorf("notes should mention %q, got:\n%s", want, joined)
		}
	}
}

func TestQueryTransportsConfigMatchesUser(t *testing.T) {
	client, _ := newTransportTestClient(t, map[string][]string{
		transportRequestsPath + "?configUri":          {treeWithProjects},
		transportSearchConfigsPath:                    {configurationsList},
		transportSearchConfigsPath + "/CFG-DEVELOPER": {configurationFor("DEVELOPER")},
		transportSearchConfigsPath + "/CFG-OTHER":     {configurationFor("OTHER")},
	})

	res, err := client.QueryTransports(context.Background(), TransportQuery{User: "other", Source: TransportSourceConfig, Targets: true})
	if err != nil {
		t.Fatalf("QueryTransports: %v", err)
	}
	if res.ConfigURI != transportSearchConfigsPath+"/CFG-OTHER" {
		t.Errorf("configUri = %q, want the configuration saved for OTHER", res.ConfigURI)
	}
	for _, n := range res.Notes {
		if strings.Contains(n, "no search configuration names user") {
			t.Errorf("a matching configuration must not be reported as foreign: %s", n)
		}
	}
}

func TestQueryTransportsConfigWithoutAnyConfiguration(t *testing.T) {
	client, _ := newTransportTestClient(t, map[string][]string{
		transportSearchConfigsPath: {`<?xml version="1.0"?><configurations:configurations xmlns:configurations="http://www.sap.com/adt/configurations"/>`},
	})
	_, err := client.QueryTransports(context.Background(), TransportQuery{Source: TransportSourceConfig, Targets: true})
	if err == nil || !strings.Contains(err.Error(), "Configure Tree") {
		t.Errorf("expected an error pointing at Eclipse's Configure Tree, got %v", err)
	}
}

func TestListTransportsKeepsReleasedRows(t *testing.T) {
	client, _ := newTransportTestClient(t, map[string][]string{
		transportRequestsPath: {treeWithProjects},
	})
	rows, err := client.ListTransports(context.Background(), "developer")
	if err != nil {
		t.Fatalf("ListTransports: %v", err)
	}
	buckets := make([]string, 0, len(rows))
	for _, r := range rows {
		buckets = append(buckets, r.Number+"="+r.Bucket)
	}
	if len(rows) != 4 {
		t.Fatalf("rows = %d (%v), want 4 incl. the released one", len(rows), buckets)
	}
	for _, r := range rows {
		if r.Number == "TR-REL-1" && (r.Bucket != "released" || r.StatusText != "Released") {
			t.Errorf("released row not marked: %+v", r)
		}
		if r.Number == "TR-CUS-1" && r.Type != "W" {
			t.Errorf("customizing row type = %q, want W", r.Type)
		}
	}
}

func TestSQLLetterList(t *testing.T) {
	if got := sqlLetterList("KW", nil); got != "'K', 'W'" {
		t.Errorf("got %q", got)
	}
	if got := sqlLetterList("DR", map[string]string{"D": "'D', 'L'", "R": "'R', 'N'"}); got != "'D', 'L', 'R', 'N'" {
		t.Errorf("got %q", got)
	}
	if got := sapLanguageKey("DE"); got != "D" {
		t.Errorf("DE -> %q", got)
	}
	if got := sapLanguageKey(""); got != "E" {
		t.Errorf("empty -> %q", got)
	}
}
