package adt

import (
	"context"
	"encoding/xml"
	"fmt"
	"net/http"
	"net/url"
	"regexp"
	"strings"
)

// The transport organizer tree: GET /sap/bc/adt/cts/transportrequests
//
// ADT discovery advertises this collection with a single template parameter,
// {?targets}. The handler reads far more, and what it does without the rest is
// surprising enough that the whole contract is written down here. Verified on
// ABAP 7.57 against the backend classes: CL_CTS_ADT_RES_APP registers the
// collection, CL_CTS_ADT_TM_RES_COLL_CONT answers GET.
//
// Two ways to steer the listing:
//
//  1. Explicit query parameters (the pre-1908 contract, still served):
//
//     requestType      letters of K (workbench), W (customizing), T (transport of copies)
//     requestStatus    letters of D (modifiable), R (released)
//     user             SAP user name; the backend defaults to sy-uname
//     targetuser       second user filter the organizer view offers
//     targets          "true" groups requests by transport target and CTS project
//     releasedFromDate YYYYMMDD; with releasedToDate bounds the released bucket
//     releasedToDate   YYYYMMDD; both unset means the last 14 days
//     req_cat, status  deprecated aliases of requestType and requestStatus
//
//  2. A saved search configuration (what Eclipse ADT sends since 1908):
//
//     configUri=/sap/bc/adt/cts/transportrequests/searchconfiguration/configurations/<id>
//
//     Every criterion — request types, statuses, the user, the date window —
//     is read from that configuration (properties WorkbenchRequests,
//     CustomizingRequests, TransportOfCopies, Modifiable, Released, User,
//     DateFilter, FromDate, ToDate). A user parameter on the same request is
//     ignored. Configurations are maintained in Eclipse (Transport Organizer
//     view, "Configure Tree") and listed under
//     /sap/bc/adt/cts/transportrequests/searchconfiguration/configurations.
//
// The pitfall that motivates all of this: without requestStatus the handler
// assembles the selection criteria for modifiable requests but only processes
// the hits inside a block that requires a D in the status list, so the answer
// holds nothing but released requests of the last two weeks. A client that
// sends user and targets alone therefore sees "no modifiable transports" on a
// system full of them. Always send requestType and requestStatus.
//
// The same collection serves, per request number:
//
//	/{trnumber}                                 request or task
//	/{trnumber}/{traction}                      releasejobs, newreleasejobs, relwithignlock,
//	                                            relObjigchkatc, tasks, merge, sortandcompress,
//	                                            consistencychecks, moveobjects, reassign
//	/{trnumber}/checkruns                       transport editor checks
//	/{trnumber}/transportlogs[/independentlogs] logs
//	/{trnumber}/objectkeys[/checkruns]          object key editor
//	/{trnumber}/transportchecks                 transport checks
//	/valuehelp/{attribute|target|ctsproject|object/{field}}
//	/reference                                  VIT to native URI mapping
//	/searchconfiguration/{configurations|metadata}
//
// Neighbours: /sap/bc/adt/cts/transports (create and query, CL_CTS_ADT_RES_OBJ_RECORD)
// and /sap/bc/adt/cts/transportchecks (CL_CTS_ADT_RES_CHECK).

// Sources a transport listing can come from.
const (
	// TransportSourceAuto tries the organizer tree with explicit parameters,
	// then the saved search configuration, then the E070/E07T tables, and
	// stops at the first source that returns requests.
	TransportSourceAuto = "auto"
	// TransportSourceParams asks the organizer tree with explicit parameters.
	TransportSourceParams = "params"
	// TransportSourceConfig asks the organizer tree through a saved search
	// configuration, the way Eclipse ADT does.
	TransportSourceConfig = "config"
	// TransportSourceSQL reads E070/E07T directly. The only source that
	// understands User "*".
	TransportSourceSQL = "sql"
)

// Defaults applied when a caller leaves the filters empty.
const (
	DefaultTransportRequestTypes    = "KWT"
	DefaultTransportRequestStatuses = "DR"
)

const (
	transportRequestsPath      = "/sap/bc/adt/cts/transportrequests"
	transportSearchConfigsPath = transportRequestsPath + "/searchconfiguration/configurations"

	acceptConfigurationsV1 = "application/vnd.sap.adt.configurations.v1+xml"
	acceptConfigurationV1  = "application/vnd.sap.adt.configuration.v1+xml"
)

// TransportQuery describes one listing of the transport organizer.
type TransportQuery struct {
	// User is the SAP user whose requests are wanted. Empty means the
	// connection's user (the backend falls back to sy-uname). "*" means every
	// user and is served by the SQL source only.
	User string `json:"user,omitempty"`
	// RequestTypes holds letters of K (workbench), W (customizing) and
	// T (transport of copies). Empty means DefaultTransportRequestTypes.
	RequestTypes string `json:"requestTypes,omitempty"`
	// RequestStatuses holds letters of D (modifiable) and R (released). Empty
	// means DefaultTransportRequestStatuses.
	RequestStatuses string `json:"requestStatuses,omitempty"`
	// ReleasedFrom and ReleasedTo (YYYYMMDD) bound the released bucket. Both
	// empty leaves the backend default, the last 14 days.
	ReleasedFrom string `json:"releasedFrom,omitempty"`
	ReleasedTo   string `json:"releasedTo,omitempty"`
	// Targets groups the requests by transport target and CTS project.
	Targets bool `json:"targets"`
	// Source is one of the TransportSource* constants; empty means auto.
	Source string `json:"source,omitempty"`
	// ConfigURI names a saved search configuration explicitly. Setting it
	// selects the config source.
	ConfigURI string `json:"configUri,omitempty"`
}

// TransportQueryResult is a listing together with where it came from.
type TransportQueryResult struct {
	Transports *UserTransports `json:"transports"`
	// Source names the source that produced Transports.
	Source string `json:"source"`
	// ConfigURI is the search configuration used, when Source is config.
	ConfigURI string `json:"configUri,omitempty"`
	// Query is the query after defaults were applied.
	Query TransportQuery `json:"query"`
	// Notes explain fallbacks and deviations, for the caller to pass on.
	Notes []string `json:"notes,omitempty"`
}

// Empty reports whether the listing holds no request at all.
func (r *TransportQueryResult) Empty() bool {
	return r == nil || r.Transports == nil ||
		(len(r.Transports.Workbench) == 0 && len(r.Transports.Customizing) == 0)
}

var yyyymmdd = regexp.MustCompile(`^\d{8}$`)

// normalized applies defaults and validates the query.
func (q TransportQuery) normalized(connectionUser string) (TransportQuery, error) {
	q.User = strings.ToUpper(strings.TrimSpace(q.User))
	if q.User == "" {
		q.User = strings.ToUpper(strings.TrimSpace(connectionUser))
	}

	q.RequestTypes = strings.ToUpper(strings.TrimSpace(q.RequestTypes))
	if q.RequestTypes == "" {
		q.RequestTypes = DefaultTransportRequestTypes
	}
	if err := onlyLetters(q.RequestTypes, "KWT", "request type"); err != nil {
		return q, err
	}

	q.RequestStatuses = strings.ToUpper(strings.TrimSpace(q.RequestStatuses))
	if q.RequestStatuses == "" {
		q.RequestStatuses = DefaultTransportRequestStatuses
	}
	if err := onlyLetters(q.RequestStatuses, "DR", "request status"); err != nil {
		return q, err
	}

	for name, v := range map[string]string{"released_from": q.ReleasedFrom, "released_to": q.ReleasedTo} {
		if v != "" && !yyyymmdd.MatchString(v) {
			return q, fmt.Errorf("%s must be a date as YYYYMMDD, got %q", name, v)
		}
	}
	if (q.ReleasedFrom == "") != (q.ReleasedTo == "") {
		return q, fmt.Errorf("released_from and released_to go together; give both or neither")
	}

	q.ConfigURI = strings.TrimSpace(q.ConfigURI)
	q.Source = strings.ToLower(strings.TrimSpace(q.Source))
	switch q.Source {
	case "":
		if q.ConfigURI != "" {
			q.Source = TransportSourceConfig
		} else {
			q.Source = TransportSourceAuto
		}
	case TransportSourceAuto, TransportSourceParams, TransportSourceConfig, TransportSourceSQL:
	default:
		return q, fmt.Errorf("source must be one of auto, params, config, sql; got %q", q.Source)
	}
	if q.ConfigURI != "" && q.Source != TransportSourceConfig && q.Source != TransportSourceAuto {
		return q, fmt.Errorf("config_uri only applies to source config or auto, not %q", q.Source)
	}
	return q, nil
}

func onlyLetters(value, allowed, what string) error {
	for _, r := range value {
		if !strings.ContainsRune(allowed, r) {
			return fmt.Errorf("%s %q: only letters of %s are allowed", what, value, allowed)
		}
	}
	return nil
}

// Describe renders the effective filters in one line, for tool output.
func (q TransportQuery) Describe() string {
	parts := []string{"type " + q.RequestTypes, "status " + q.RequestStatuses}
	if q.ReleasedFrom != "" {
		parts = append(parts, fmt.Sprintf("released %s..%s", q.ReleasedFrom, q.ReleasedTo))
	} else if strings.Contains(q.RequestStatuses, "R") {
		parts = append(parts, "released within the last 14 days (backend default)")
	}
	if !q.Targets {
		parts = append(parts, "no target grouping")
	}
	return strings.Join(parts, ", ")
}

// QueryUserTransports lists transport requests the way GetUserTransports
// does, with the query spelled out. It needs transport management enabled.
func (c *Client) QueryUserTransports(ctx context.Context, q TransportQuery) (*TransportQueryResult, error) {
	if err := c.checkSafety(OpTransport, "GetUserTransports"); err != nil {
		return nil, err
	}
	return c.queryTransports(ctx, q)
}

// QueryTransports lists transport requests the way ListTransports does, with
// the query spelled out. Reading is allowed whenever transportable edits are.
func (c *Client) QueryTransports(ctx context.Context, q TransportQuery) (*TransportQueryResult, error) {
	if err := c.config.Safety.CheckTransport("", "ListTransports", false); err != nil {
		return nil, err
	}
	return c.queryTransports(ctx, q)
}

func (c *Client) queryTransports(ctx context.Context, q TransportQuery) (*TransportQueryResult, error) {
	q, err := q.normalized(c.config.Username)
	if err != nil {
		return nil, err
	}
	res := &TransportQueryResult{Query: q}

	switch q.Source {
	case TransportSourceParams:
		return c.transportsViaParams(ctx, res)
	case TransportSourceConfig:
		return c.transportsViaConfig(ctx, res)
	case TransportSourceSQL:
		return c.transportsViaSQL(ctx, res)
	}

	// auto: first source with requests wins; a source that fails is noted
	// and the next one is tried, so one broken path never hides the others.
	if r, err := c.transportsViaParams(ctx, res); err != nil {
		res.note("organizer tree with explicit parameters failed: %v", err)
	} else if !r.Empty() {
		return r, nil
	} else {
		res.note("organizer tree with explicit parameters returned no requests")
	}

	if r, err := c.transportsViaConfig(ctx, res); err != nil {
		res.note("saved search configuration not usable: %v", err)
	} else if !r.Empty() {
		return r, nil
	} else {
		res.note("organizer tree via search configuration %s returned no requests", r.ConfigURI)
	}

	return c.transportsViaSQL(ctx, res)
}

func (r *TransportQueryResult) note(format string, args ...any) {
	r.Notes = append(r.Notes, fmt.Sprintf(format, args...))
}

// transportsViaParams asks the organizer tree with explicit parameters.
func (c *Client) transportsViaParams(ctx context.Context, res *TransportQueryResult) (*TransportQueryResult, error) {
	q := res.Query
	values := url.Values{}
	if q.User != "" {
		values.Set("user", q.User)
	}
	values.Set("targets", boolString(q.Targets))
	values.Set("requestType", q.RequestTypes)
	values.Set("requestStatus", q.RequestStatuses)
	if q.ReleasedFrom != "" {
		values.Set("releasedFromDate", q.ReleasedFrom)
		values.Set("releasedToDate", q.ReleasedTo)
	}

	transports, err := c.fetchTransportTree(ctx, values)
	if err != nil {
		return nil, err
	}
	res.Transports = transports
	res.Source = TransportSourceParams
	res.ConfigURI = ""
	return res, nil
}

// transportsViaConfig asks the organizer tree through a saved search
// configuration. The configuration decides the filters, including the user.
func (c *Client) transportsViaConfig(ctx context.Context, res *TransportQueryResult) (*TransportQueryResult, error) {
	q := res.Query
	if q.User == "*" {
		return nil, fmt.Errorf("a search configuration always names one user; use source sql for every user")
	}

	configURI := q.ConfigURI
	if configURI == "" {
		configs, err := c.TransportSearchConfigurations(ctx)
		if err != nil {
			return nil, err
		}
		if len(configs) == 0 {
			return nil, fmt.Errorf("no transport search configuration is saved on the server; " +
				"create one in Eclipse (Transport Organizer view, Configure Tree) or use source params")
		}
		chosen := configs[0]
		matched := false
		for _, cfg := range configs {
			if strings.EqualFold(cfg.User, q.User) {
				chosen, matched = cfg, true
				break
			}
		}
		if !matched && q.User != "" {
			res.note("no search configuration names user %s; using %s, which belongs to %s — "+
				"the listing follows that configuration, not the user asked for",
				q.User, chosen.URI, chosen.User)
		}
		if !chosen.Modifiable && strings.Contains(q.RequestStatuses, "D") {
			res.note("search configuration %s excludes modifiable requests", chosen.URI)
		}
		if !chosen.Released && strings.Contains(q.RequestStatuses, "R") {
			res.note("search configuration %s excludes released requests", chosen.URI)
		}
		configURI = chosen.URI
	}

	res.note("the search configuration decides request types, statuses, user and date window; "+
		"request_type %s, request_status %s and any released window were not applied", q.RequestTypes, q.RequestStatuses)

	values := url.Values{}
	values.Set("targets", boolString(q.Targets))
	values.Set("configUri", configURI)

	transports, err := c.fetchTransportTree(ctx, values)
	if err != nil {
		return nil, err
	}
	res.Transports = transports
	res.Source = TransportSourceConfig
	res.ConfigURI = configURI
	return res, nil
}

// transportsViaSQL reads E070/E07T directly.
func (c *Client) transportsViaSQL(ctx context.Context, res *TransportQueryResult) (*TransportQueryResult, error) {
	q := res.Query
	rows, err := c.listTransportsViaSQLQuery(ctx, q.User, q.RequestTypes, q.RequestStatuses)
	if err != nil {
		return nil, err
	}
	res.Transports = summariesToUserTransports(rows)
	res.Source = TransportSourceSQL
	res.ConfigURI = ""
	return res, nil
}

// fetchTransportTree runs one GET against the organizer tree and parses it.
func (c *Client) fetchTransportTree(ctx context.Context, values url.Values) (*UserTransports, error) {
	resp, err := c.transport.Request(ctx, transportRequestsPath, &RequestOptions{
		Method: http.MethodGet,
		Query:  values,
		Accept: acceptTransportOrganizerTreeV1 + ", " + acceptTransportOrganizerV1 + ";q=0.9",
	})
	if err != nil {
		return nil, fmt.Errorf("reading the transport organizer tree: %w", err)
	}
	return parseUserTransports(resp.Body)
}

func boolString(b bool) string {
	if b {
		return "true"
	}
	return "false"
}

// TransportSearchConfiguration is one saved Transport Organizer search
// configuration, as Eclipse ADT keeps it on the server.
type TransportSearchConfiguration struct {
	URI         string            `json:"uri"`
	User        string            `json:"user"`
	Workbench   bool              `json:"workbench"`
	Customizing bool              `json:"customizing"`
	Copies      bool              `json:"transportOfCopies"`
	Modifiable  bool              `json:"modifiable"`
	Released    bool              `json:"released"`
	DateFilter  string            `json:"dateFilter"`
	FromDate    string            `json:"fromDate,omitempty"`
	ToDate      string            `json:"toDate,omitempty"`
	CreatedBy   string            `json:"createdBy,omitempty"`
	ChangedAt   string            `json:"changedAt,omitempty"`
	Properties  map[string]string `json:"properties"`
}

// TransportSearchConfigurations lists the saved search configurations with
// their properties.
func (c *Client) TransportSearchConfigurations(ctx context.Context) ([]TransportSearchConfiguration, error) {
	resp, err := c.transport.Request(ctx, transportSearchConfigsPath, &RequestOptions{
		Method: http.MethodGet,
		Accept: acceptConfigurationsV1,
	})
	if err != nil {
		return nil, fmt.Errorf("listing transport search configurations: %w", err)
	}
	uris, err := parseConfigurationURIs(resp.Body)
	if err != nil {
		return nil, err
	}

	var out []TransportSearchConfiguration
	for _, uri := range uris {
		detail, err := c.transport.Request(ctx, uri, &RequestOptions{
			Method: http.MethodGet,
			Accept: acceptConfigurationV1,
		})
		if err != nil {
			return nil, fmt.Errorf("reading transport search configuration %s: %w", uri, err)
		}
		cfg, err := parseSearchConfiguration(detail.Body)
		if err != nil {
			return nil, fmt.Errorf("parsing transport search configuration %s: %w", uri, err)
		}
		cfg.URI = uri
		out = append(out, cfg)
	}
	return out, nil
}

// parseConfigurationURIs extracts the configuration URIs from the collection.
//
// The server builds the atom:link hrefs from the request URL it was called
// with, so a call that carried sap-client and sap-language gets them back in
// the middle of the href. Only the trailing id is trusted; the URI is rebuilt
// from it.
func parseConfigurationURIs(data []byte) ([]string, error) {
	type link struct {
		Href string `xml:"href,attr"`
	}
	type configuration struct {
		Links []link `xml:"link"`
	}
	var doc struct {
		Configurations []configuration `xml:"configuration"`
	}
	if err := xml.Unmarshal(data, &doc); err != nil {
		return nil, fmt.Errorf("parsing transport search configurations: %w", err)
	}

	var uris []string
	for _, cfg := range doc.Configurations {
		for _, l := range cfg.Links {
			id := l.Href[strings.LastIndex(l.Href, "/")+1:]
			if id == "" || strings.ContainsAny(id, "?&=") {
				continue
			}
			uris = append(uris, transportSearchConfigsPath+"/"+id)
			break
		}
	}
	return uris, nil
}

// parseSearchConfiguration reads one configuration document.
func parseSearchConfiguration(data []byte) (TransportSearchConfiguration, error) {
	type property struct {
		Key   string `xml:"key,attr"`
		Value string `xml:",chardata"`
	}
	var doc struct {
		CreatedBy  string     `xml:"createdBy,attr"`
		ChangedAt  string     `xml:"changedAt,attr"`
		Properties []property `xml:"properties>property"`
	}
	if err := xml.Unmarshal(data, &doc); err != nil {
		return TransportSearchConfiguration{}, err
	}
	cfg := TransportSearchConfiguration{
		CreatedBy:  doc.CreatedBy,
		ChangedAt:  doc.ChangedAt,
		Properties: map[string]string{},
	}
	for _, p := range doc.Properties {
		cfg.Properties[p.Key] = strings.TrimSpace(p.Value)
	}
	cfg.User = strings.ToUpper(cfg.Properties["User"])
	cfg.Workbench = cfg.Properties["WorkbenchRequests"] == "true"
	cfg.Customizing = cfg.Properties["CustomizingRequests"] == "true"
	cfg.Copies = cfg.Properties["TransportOfCopies"] == "true"
	cfg.Modifiable = cfg.Properties["Modifiable"] == "true"
	cfg.Released = cfg.Properties["Released"] == "true"
	cfg.DateFilter = cfg.Properties["DateFilter"]
	cfg.FromDate = cfg.Properties["FromDate"]
	cfg.ToDate = cfg.Properties["ToDate"]
	return cfg, nil
}

// summariesToUserTransports groups flat rows the way the organizer tree would.
func summariesToUserTransports(rows []TransportSummary) *UserTransports {
	result := &UserTransports{}
	for _, row := range rows {
		tr := TransportRequest{
			Number:      row.Number,
			Owner:       row.Owner,
			Description: row.Description,
			Status:      row.Status,
			Target:      row.Target,
			Bucket:      bucketForStatus(row.Status),
		}
		if row.Type == "W" {
			tr.Type = "customizing"
			result.Customizing = append(result.Customizing, tr)
		} else {
			tr.Type = "workbench"
			result.Workbench = append(result.Workbench, tr)
		}
	}
	return result
}

// FlattenTransports turns a grouped listing into the flat rows ListTransports
// returns.
func FlattenTransports(t *UserTransports) []TransportSummary {
	if t == nil {
		return nil
	}
	var out []TransportSummary
	add := func(reqs []TransportRequest, typ string) {
		for _, r := range reqs {
			out = append(out, TransportSummary{
				Number:      r.Number,
				Owner:       r.Owner,
				Description: r.Description,
				Type:        typ,
				Status:      r.Status,
				StatusText:  transportStatusText(r.Status),
				Target:      r.Target,
				Bucket:      r.Bucket,
				Project:     r.Project,
			})
		}
	}
	add(t.Workbench, "K")
	add(t.Customizing, "W")
	return out
}

func bucketForStatus(status string) string {
	switch status {
	case "D", "L":
		return "modifiable"
	case "R", "N", "O":
		return "released"
	}
	return ""
}

func transportStatusText(status string) string {
	switch status {
	case "D":
		return "Modifiable"
	case "L":
		return "Modifiable, protected"
	case "R":
		return "Released"
	case "N":
		return "Released (import started)"
	case "O":
		return "Release started"
	}
	return ""
}

// sapLanguageKey maps a session language (ISO code or SAP key) to the
// one-character key E07T stores.
func sapLanguageKey(lang string) string {
	l := strings.ToUpper(strings.TrimSpace(lang))
	switch l {
	case "":
		return "E"
	case "DE":
		return "D"
	case "EN":
		return "E"
	case "FR":
		return "F"
	case "IT":
		return "I"
	case "ES":
		return "S"
	case "NL":
		return "N"
	case "PT":
		return "P"
	case "RU":
		return "R"
	case "JA":
		return "J"
	case "ZH":
		return "1"
	}
	if len(l) == 1 {
		return l
	}
	return l[:1]
}
