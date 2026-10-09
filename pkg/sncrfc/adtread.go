package sncrfc

import (
	"bytes"
	"errors"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"unicode/utf8"
)

// adtReadPrefixes are the ADT resources a read-only client needs: discovery,
// system information, repository search and structure, source and metadata of
// the usual object types, packages, transports and short dumps (for RCA). Only
// GET reaches them. Everything else, including the security endpoints that
// issue tickets and every POST except an explicitly enabled data preview, is
// refused before the SDK is called.
var adtReadPrefixes = []string{
	"/sap/bc/adt/discovery",
	"/sap/bc/adt/core/discovery",
	"/sap/bc/adt/core/http/systeminformation",
	"/sap/bc/adt/compatibility/graph",
	"/sap/bc/adt/repository/",
	"/sap/bc/adt/programs/",
	"/sap/bc/adt/oo/",
	"/sap/bc/adt/functions/",
	"/sap/bc/adt/ddic/",
	"/sap/bc/adt/packages/",
	"/sap/bc/adt/system/",
	"/sap/bc/adt/cts/transportrequests",
	"/sap/bc/adt/messageclass/",
	"/sap/bc/adt/textelements/",
	"/sap/bc/adt/enhancements/",
	"/sap/bc/adt/vit/",
	"/sap/bc/adt/runtime/dumps",
	"/sap/bc/adt/runtime/dump/",
}

// adtReadHeaders may be forwarded. Cookie, Authorization and anything that
// could carry a credential or switch the session to stateful are not listed.
var adtReadHeaders = map[string]bool{
	"accept":          true,
	"accept-language": true,
	"x-csrf-token":    true,
	"cache-control":   true,
	"if-none-match":   true,
	"user-agent":      true,
	"content-type":    true,
}

// safeURI refuses anything a server might decode differently from these checks.
func safeURI(uri string) bool {
	if len(uri) > 4096 || !strings.HasPrefix(uri, "/sap/bc/adt/") || strings.ContainsAny(uri, "\\\r\n\t #") || strings.Contains(uri, "//") {
		return false
	}
	for _, r := range uri {
		if r < 0x21 || r > 0x7e {
			return false
		}
	}
	// %2f stays allowed: ADT encodes namespaced names (/ABC/CL_X) that way. So no
	// form of a dot segment may appear at all, literal or encoded, and neither
	// may an encoded backslash.
	lower := strings.ToLower(uri)
	if strings.Contains(lower, "..") || strings.Contains(lower, "%2e") || strings.Contains(lower, "%5c") || strings.Contains(lower, "/./") || strings.HasSuffix(lower, "/.") {
		return false
	}
	return true
}

// ADTReadAllowed reports whether a request URI (path and query, as sent on the
// request line) is a read the sidecar forwards.
func ADTReadAllowed(uri string) bool {
	if !safeURI(uri) {
		return false
	}
	path := strings.ToLower(uri)
	if i := strings.IndexByte(path, '?'); i >= 0 {
		path = path[:i]
	}
	for _, p := range adtReadPrefixes {
		if strings.HasSuffix(p, "/") && (path == strings.TrimSuffix(p, "/") || strings.HasPrefix(path, p)) || path == p || strings.HasPrefix(path, p+"/") {
			return true
		}
	}
	return false
}

// Data preview runs an ABAP SQL SELECT on the server: freestyle takes the
// statement as the body, ddic reads one entity with an optional WHERE text. These
// are the only POSTs the sidecar can forward, and only on a connection that
// enabled data preview explicitly. ADT itself accepts only SELECT here; the
// checks below refuse anything else before it leaves the machine.
const (
	MaxPreviewRows = 10000
	maxPreviewBody = 64 << 10
)

// secretTables hold credentials, password hashes, keys or secure-store
// entries. They are refused by name wherever they appear in a preview, so a
// SELECT cannot carry them into a model's context even where SAP authorizations
// would allow it.
var secretTables = regexp.MustCompile(`(?i)(^|[^A-Za-z0-9_/])(USR02|USH02|USRPWDHISTORY|USR40|RSECTAB|RSECACTB|SSF_PSE_D|SSF_PSE_H|STRUSTCERT|VSKOPF|SECSTORE|RFCDES|TSECSTORE|USREFUS|APQD|SNCSYSACL|USRACL)([^A-Za-z0-9_/]|$)`)

// MentionsSecretTable reports whether a preview names a credential table.
func MentionsSecretTable(s string) bool { return secretTables.MatchString(s) }

var previewEntity = regexp.MustCompile(`^[A-Za-z0-9_/]{1,40}$`)

// ADTDataPreviewAllowed reports whether a POST is a bounded data preview.
func ADTDataPreviewAllowed(uri string, body []byte) bool {
	if !safeURI(uri) || len(body) > maxPreviewBody || !utf8.Valid(body) || strings.ContainsRune(string(body), 0) {
		return false
	}
	path, query, _ := strings.Cut(uri, "?")
	q, err := url.ParseQuery(query)
	if err != nil || MentionsSecretTable(string(body)) || MentionsSecretTable(q.Get("ddicEntityName")) {
		return false
	}
	for k, v := range q {
		if len(v) != 1 || (k != "rowNumber" && k != "ddicEntityName" && !sessionParam(k, v[0])) {
			return false
		}
	}
	rows, err := strconv.Atoi(q.Get("rowNumber"))
	if err != nil || rows < 1 || rows > MaxPreviewRows {
		return false
	}
	switch path {
	case "/sap/bc/adt/datapreview/freestyle":
		head := strings.ToUpper(strings.TrimSpace(string(body)))
		return !q.Has("ddicEntityName") && len(head) > 6 && (strings.HasPrefix(head, "SELECT") || strings.HasPrefix(head, "WITH"))
	case "/sap/bc/adt/datapreview/ddic":
		return previewEntity.MatchString(q.Get("ddicEntityName"))
	}
	return false
}

// identityQuery is vsp's own identity check on releases without the system
// information resource: one row of T000 for the session's client.
var identityQuery = regexp.MustCompile(`^SELECT MANDT, LOGSYS FROM T000 WHERE MANDT = '[0-9]{3}'$`)

// ADTIdentityQueryAllowed reports whether a POST is exactly vsp's identity
// check (pkg/adt probeIdentity), which is forwarded even where data preview
// is off: without it a system older than the system information resource
// cannot be pinned, so vsp refuses to work with it at all.
func ADTIdentityQueryAllowed(uri string, body []byte) bool {
	path, query, _ := strings.Cut(uri, "?")
	q, err := url.ParseQuery(query)
	if !safeURI(uri) || path != "/sap/bc/adt/datapreview/freestyle" || err != nil || q.Get("rowNumber") != "1" {
		return false
	}
	for k, v := range q {
		if len(v) != 1 || (k != "rowNumber" && !sessionParam(k, v[0])) {
			return false
		}
	}
	return identityQuery.Match(bytes.TrimSpace(body))
}

// sessionParam reports whether a query parameter is one vsp puts on every
// request: the client and the logon language. Over RFC the session is fixed
// at logon, so they change nothing, but refusing them refused every POST.
func sessionParam(k, v string) bool {
	switch k {
	case "sap-client":
		return sessionClient.MatchString(v)
	case "sap-language":
		return sessionLanguage.MatchString(v)
	}
	return false
}

var (
	sessionClient   = regexp.MustCompile(`^[0-9]{3}$`)
	sessionLanguage = regexp.MustCompile(`^[A-Za-z0-9]{1,2}$`)
)

// ADTReadHeaderAllowed reports whether a header name may be forwarded.
func ADTReadHeaderAllowed(name string) bool { return adtReadHeaders[strings.ToLower(name)] }

func validateADTRead(input map[string]any, dataPreview bool) error {
	r, ok := input["REQUEST"].(map[string]any)
	if !ok || len(input) != 1 || len(r) != 3 {
		return errors.New("require one ADT request")
	}
	l, ok := r["REQUEST_LINE"].(map[string]any)
	if !ok || len(l) != 3 || l["VERSION"] != "HTTP/1.1" {
		return errors.New("invalid ADT request line")
	}
	uri, okURI := l["URI"].(string)
	body, okBody := r["MESSAGE_BODY"].([]byte)
	if !okURI || !okBody {
		return errors.New("invalid ADT request")
	}
	switch {
	case l["METHOD"] == "GET":
		if !ADTReadAllowed(uri) || len(body) != 0 {
			return errors.New("ADT GET outside the read allowlist")
		}
	case l["METHOD"] == "POST" && ADTIdentityQueryAllowed(uri, body):
	case l["METHOD"] == "POST" && dataPreview:
		if !ADTDataPreviewAllowed(uri, body) {
			return errors.New("POST is not a bounded data preview")
		}
	default:
		return errors.New("only ADT GET, and data preview when enabled, are allowed")
	}
	headers, ok := r["HEADER_FIELDS"].([]map[string]any)
	if !ok || len(headers) > 16 {
		return errors.New("invalid ADT headers")
	}
	for _, h := range headers {
		name, okName := h["NAME"].(string)
		value, okValue := h["VALUE"].(string)
		if len(h) != 2 || !okName || !okValue || !ADTReadHeaderAllowed(name) || len(value) > 1024 || strings.ContainsAny(value, "\r\n\x00") {
			return errors.New("ADT header outside the read allowlist")
		}
	}
	return nil
}
