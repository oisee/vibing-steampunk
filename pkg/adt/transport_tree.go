package adt

import (
	"encoding/xml"
	"fmt"
	"strings"
)

// A CTS transport listing does not have one shape.
//
// `/sap/bc/adt/cts/transportrequests` answers with a tm:root whose depth
// depends on the request and on the system: with transport targets it nests
// requests under tm:workbench > tm:target > tm:modifiable, without targets it
// drops the tm:target level, and a single-request document puts tm:request
// straight under tm:root. The two hand-written parsers each hardcoded one of
// those three, and encoding/xml answers a path that does not match with an
// empty struct and a nil error — so the mismatch surfaced as "no transports
// found" rather than as a parse failure (#111, #140).
//
// Everything below navigates the document instead of asserting its shape:
// find every tm:request wherever it sits, and remember which section
// (workbench/customizing), which target and which bucket (modifiable/released)
// it was nested in. A shape we have not seen still yields its requests; it
// only loses the grouping the extra levels would have carried.

// ctsNamespace is the namespace CTS elements live in. Attribute lookups prefer
// it, because a tm:request can also carry attributes from other namespaces
// (adtcore:name, for one) whose local part collides with a tm one.
const ctsNamespace = "http://www.sap.com/cts/adt/tm"

// ctsNode is a namespace-agnostic view of any element in a CTS response.
// Element and attribute names are read through their local part, so the
// document parses whether or not the tm: prefix is declared.
type ctsNode struct {
	XMLName xml.Name
	Attrs   []xml.Attr `xml:",any,attr"`
	Nodes   []ctsNode  `xml:",any"`
}

// attr returns the value of an attribute by local name, preferring the CTS
// namespace over any other when both are present.
func (n ctsNode) attr(name string) string {
	fallback := ""
	for _, a := range n.Attrs {
		if a.Name.Local != name {
			continue
		}
		if a.Name.Space == ctsNamespace || a.Name.Space == "tm" || a.Name.Space == "" {
			return a.Value
		}
		if fallback == "" {
			fallback = a.Value
		}
	}
	return fallback
}

// ctsObject is an abap_object entry inside a request or task.
type ctsObject struct {
	PgmID    string
	Type     string
	Name     string
	WBType   string
	Info     string
	Position string
}

// ctsRequest is one tm:request (or tm:task, which carries the same attributes)
// together with the position it was found at.
type ctsRequest struct {
	Number      string
	Parent      string
	Owner       string
	Desc        string
	Type        string
	Status      string
	StatusText  string
	Target      string
	TargetDesc  string
	Client      string
	LastChanged string

	// Section is "workbench", "customizing", or "" when the document does not
	// separate the two. Bucket is "modifiable", "released", or "".
	Section string
	Bucket  string
	// Project and ProjectID come from the tm:project level the organizer
	// inserts between target and bucket when requests carry a CTS project.
	Project   string
	ProjectID string

	Tasks   []ctsRequest
	Objects []ctsObject
}

// ctsScope carries the levels a request was nested in down the walk.
type ctsScope struct {
	section    string
	target     string
	targetDesc string
	project    string
	projectID  string
	bucket     string
}

// parseCTSRequests returns every tm:request in a CTS document, at whatever
// depth the server put it.
func parseCTSRequests(data []byte) ([]ctsRequest, error) {
	var root ctsNode
	if err := xml.Unmarshal(data, &root); err != nil {
		return nil, fmt.Errorf("parsing transport list: %w", err)
	}

	var out []ctsRequest
	collectCTSRequests(root, ctsScope{}, &out)
	return out, nil
}

func collectCTSRequests(n ctsNode, sc ctsScope, out *[]ctsRequest) {
	switch strings.ToLower(n.XMLName.Local) {
	case "workbench", "customizing":
		sc.section = strings.ToLower(n.XMLName.Local)
	case "target":
		if v := n.attr("name"); v != "" {
			sc.target = v
		}
		if v := n.attr("desc"); v != "" {
			sc.targetDesc = v
		}
	case "project":
		sc.project = n.attr("name")
		sc.projectID = n.attr("projectId")
	case "modifiable":
		sc.bucket = "modifiable"
	case "released":
		sc.bucket = "released"
	case "request":
		*out = append(*out, newCTSRequest(n, sc))
		// Tasks and objects belong to this request; nothing below a request is
		// another request.
		return
	}

	for _, c := range n.Nodes {
		collectCTSRequests(c, sc, out)
	}
}

// newCTSRequest reads one request (or task) element and its children.
func newCTSRequest(n ctsNode, sc ctsScope) ctsRequest {
	r := ctsRequest{
		Number:      n.attr("number"),
		Parent:      n.attr("parent"),
		Owner:       n.attr("owner"),
		Desc:        n.attr("desc"),
		Type:        n.attr("type"),
		Status:      n.attr("status"),
		StatusText:  n.attr("status_text"),
		Target:      n.attr("target"),
		TargetDesc:  n.attr("target_desc"),
		Client:      n.attr("source_client"),
		LastChanged: n.attr("lastchanged_timestamp"),
		Section:     sc.section,
		Bucket:      sc.bucket,
		Project:     sc.project,
		ProjectID:   sc.projectID,
	}

	// The enclosing tm:target names the target when the request element does
	// not carry one itself.
	if r.Target == "" {
		r.Target = sc.target
	}
	if r.TargetDesc == "" {
		r.TargetDesc = sc.targetDesc
	}

	for _, c := range n.Nodes {
		switch strings.ToLower(c.XMLName.Local) {
		case "task":
			r.Tasks = append(r.Tasks, newCTSRequest(c, sc))
		case "abap_object":
			r.Objects = append(r.Objects, newCTSObject(c))
		default:
			// Wrappers such as tm:abap_objects or tm:tasks: look one level in
			// rather than requiring the element to exist.
			for _, g := range c.Nodes {
				switch strings.ToLower(g.XMLName.Local) {
				case "task":
					r.Tasks = append(r.Tasks, newCTSRequest(g, sc))
				case "abap_object":
					r.Objects = append(r.Objects, newCTSObject(g))
				}
			}
		}
	}

	return r
}

func newCTSObject(n ctsNode) ctsObject {
	return ctsObject{
		PgmID:    n.attr("pgmid"),
		Type:     n.attr("type"),
		Name:     n.attr("name"),
		WBType:   n.attr("wbtype"),
		Info:     firstNonEmpty(n.attr("obj_info"), n.attr("obj_desc")),
		Position: n.attr("position"),
	}
}

// fillFromStructure supplies type and status from the position a request was
// found at, for servers that carry them as levels rather than as attributes.
func (r *ctsRequest) fillFromStructure() {
	if r.Type == "" {
		switch r.Section {
		case "workbench":
			r.Type = "K"
		case "customizing":
			r.Type = "W"
		}
	}
	if r.Status == "" {
		switch r.Bucket {
		case "modifiable":
			r.Status = "D"
		case "released":
			r.Status = "R"
		}
	}
	if r.StatusText == "" {
		switch r.Status {
		case "D":
			r.StatusText = "Modifiable"
		case "R":
			r.StatusText = "Released"
		case "N":
			r.StatusText = "Released (import started)"
		}
	}
}

// isReleased reports whether the server placed this request in a released
// bucket. Only the structure counts: a status attribute alone is not enough,
// because a released request reachable from a modifiable listing is still
// something the caller asked to see.
func (r ctsRequest) isReleased() bool {
	return r.Bucket == "released"
}
