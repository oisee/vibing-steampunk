package adt

import (
	"context"
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/oisee/vibing-steampunk/pkg/itf"
)

// SAP's documentation — SE61 — is two tables. DOKIL is the index: an object
// class (ID: DE for a data element, RE a report, FU a function module, CL a
// class, TB a table, NA a message, HY an IMG activity, TX a free text, ...),
// the object name, the language, the version. DOKTL is the text, one ITF
// line per row. The IMG hangs off the same tables: an activity's text is HY
// with the object "SIMG" + activity, and the tree that leads to it is
// TNODEIMG (nodes), TNODEIMGT (their texts) and TNODEIMGR (what a node
// points at: an activity, a document, a status).

// Documentation is one SE61 text.
type Documentation struct {
	ID       string     `json:"id"`
	Object   string     `json:"object"`
	Language string     `json:"language"`
	Version  int        `json:"version"`
	Lines    []itf.Line `json:"lines,omitempty"`
	Markdown string     `json:"markdown"`
}

// DocumentationClasses names the DOKIL IDs worth knowing.
var DocumentationClasses = map[string]string{
	"DE": "data element", "RE": "report", "FU": "function module", "CL": "class", "IF": "interface",
	"TB": "table", "NA": "message", "HY": "IMG activity", "TX": "general text", "IM": "IMG chapter",
	"DT": "domain", "TT": "transaction", "SD": "screen", "CA": "CA?", "UO": "IMG object", "IO": "IMG object",
}

// Documentation reads one text, latest version, in the language given (a
// SAP key or ISO code), with INCLUDE commands resolved.
func (c *Client) Documentation(ctx context.Context, id, object, lang string) (*Documentation, error) {
	lang = spras(lang)
	id, object = strings.ToUpper(strings.TrimSpace(id)), strings.ToUpper(strings.TrimSpace(object))
	lines, version, err := c.docLines(ctx, id, object, lang)
	if err != nil {
		return nil, err
	}
	doc := &Documentation{ID: id, Object: object, Language: lang, Version: version, Lines: lines}
	doc.Markdown = itf.ToMarkdown(lines, func(obj, incID string) ([]itf.Line, error) {
		l, _, err := c.docLines(ctx, incID, obj, lang)
		return l, err
	})
	return doc, nil
}

func (c *Client) docLines(ctx context.Context, id, object, lang string) ([]itf.Line, int, error) {
	res, err := c.RunQuery(ctx, fmt.Sprintf("SELECT dokversion FROM doktl WHERE id = '%s' AND object = '%s' AND langu = '%s' ORDER BY dokversion DESCENDING", sqlQuote(id), sqlQuote(object), sqlQuote(lang)), 1)
	if err != nil {
		return nil, 0, fmt.Errorf("reading documentation %s %s: %w", id, object, err)
	}
	if res == nil || len(res.Rows) == 0 {
		return nil, 0, fmt.Errorf("no %s documentation for %s in language %s", id, object, lang)
	}
	version, _ := strconv.Atoi(cell(res.Rows[0], "DOKVERSION"))
	res, err = c.RunQuery(ctx, fmt.Sprintf("SELECT line, dokformat, doktext FROM doktl WHERE id = '%s' AND object = '%s' AND langu = '%s' AND dokversion = '%04d' ORDER BY line", sqlQuote(id), sqlQuote(object), sqlQuote(lang), version), 20000)
	if err != nil {
		return nil, 0, fmt.Errorf("reading documentation %s %s: %w", id, object, err)
	}
	var lines []itf.Line
	for _, row := range res.Rows {
		lines = append(lines, itf.Line{Format: cell(row, "DOKFORMAT"), Text: strings.TrimRight(cellRaw(row, "DOKTEXT"), " ")})
	}
	return lines, version, nil
}

// cellRaw is cell without the trim, for text whose leading blanks matter.
func cellRaw(row map[string]interface{}, column string) string {
	value, ok := row[column]
	if !ok {
		value = row[strings.ToLower(column)]
	}
	if value == nil {
		return ""
	}
	return fmt.Sprint(value)
}

// DocumentationIndex lists the texts that exist for an object, in every
// class and language.
func (c *Client) DocumentationIndex(ctx context.Context, object string) ([]Documentation, error) {
	res, err := c.RunQuery(ctx, fmt.Sprintf("SELECT id, object, langu, version, txtlines FROM dokil WHERE object = '%s' ORDER BY id, langu", sqlQuote(strings.ToUpper(object))), 200)
	if err != nil {
		return nil, fmt.Errorf("reading the documentation index: %w", err)
	}
	var out []Documentation
	if res == nil {
		return out, nil
	}
	for _, row := range res.Rows {
		d := Documentation{ID: cell(row, "ID"), Object: cell(row, "OBJECT"), Language: cell(row, "LANGU")}
		d.Version, _ = strconv.Atoi(cell(row, "VERSION"))
		out = append(out, d)
	}
	return out, nil
}

// IMGActivity is one customizing activity: what it is, how it is reached,
// and its documentation.
type IMGActivity struct {
	Activity      string   `json:"activity"`
	Text          string   `json:"text,omitempty"`
	Transaction   string   `json:"transaction,omitempty"`
	DocObject     string   `json:"docObject,omitempty"`
	Paths         []string `json:"paths,omitempty"`
	Documentation string   `json:"documentation,omitempty"`
}

// IMGActivity reads an activity by its technical name (CUS_IMGACH).
func (c *Client) IMGActivity(ctx context.Context, activity, lang string) (*IMGActivity, error) {
	activity = strings.ToUpper(strings.TrimSpace(activity))
	res, err := c.RunQuery(ctx, fmt.Sprintf("SELECT activity, tcode, docu_id FROM cus_imgach WHERE activity = '%s'", sqlQuote(activity)), 1)
	if err != nil {
		return nil, fmt.Errorf("reading IMG activity %s: %w", activity, err)
	}
	if res == nil || len(res.Rows) == 0 {
		return nil, fmt.Errorf("IMG activity %s does not exist", activity)
	}
	a := &IMGActivity{Activity: activity, Transaction: cell(res.Rows[0], "TCODE"), DocObject: cell(res.Rows[0], "DOCU_ID")}
	if t, terr := c.RunQuery(ctx, fmt.Sprintf("SELECT text FROM cus_imgact WHERE activity = '%s' AND spras = '%s'", sqlQuote(activity), sqlQuote(spras(lang))), 1); terr == nil && t != nil && len(t.Rows) > 0 {
		a.Text = cell(t.Rows[0], "TEXT")
	}
	if a.Paths, err = c.imgPaths(ctx, activity, lang); err != nil {
		return a, err
	}
	if a.DocObject != "" {
		if doc, derr := c.Documentation(ctx, "HY", a.DocObject, lang); derr == nil {
			a.Documentation = doc.Markdown
		} else {
			a.Documentation = "_" + derr.Error() + "_"
		}
	}
	return a, nil
}

// IMGNode is one hit of an IMG search: a node's text, the path to it and
// what it points at.
type IMGNode struct {
	NodeID    string `json:"nodeId"`
	Text      string `json:"text"`
	Path      string `json:"path,omitempty"`
	RefType   string `json:"refType,omitempty"`
	RefObject string `json:"refObject,omitempty"`
	// Transaction is the activity's transaction when the node is one.
	Transaction string `json:"transaction,omitempty"`
}

// IMGSearch finds customizing activities and IMG folders whose text matches
// (a LIKE pattern, * as the wildcard) and says where each sits and what it
// opens. Activities are named in CUS_IMGACT, folders in TNODEIMGT.
func (c *Client) IMGSearch(ctx context.Context, pattern, lang string, limit int) ([]IMGNode, error) {
	lang = spras(lang)
	if limit <= 0 {
		limit = 40
	}
	like := strings.ReplaceAll(strings.TrimSpace(pattern), "*", "%")
	if !strings.Contains(like, "%") {
		like = "%" + like + "%"
	}
	var nodes []IMGNode

	acts, err := c.RunQuery(ctx, fmt.Sprintf("SELECT a~activity, a~text, h~tcode FROM cus_imgact AS a LEFT OUTER JOIN cus_imgach AS h ON h~activity = a~activity WHERE a~spras = '%s' AND a~text LIKE '%s' ORDER BY a~text", sqlQuote(lang), sqlQuote(like)), limit)
	if err != nil {
		return nil, fmt.Errorf("searching IMG activities: %w", err)
	}
	if acts != nil {
		for _, row := range acts.Rows {
			activity := cell(row, "ACTIVITY")
			n := IMGNode{Text: cell(row, "TEXT"), RefType: "ACTI", RefObject: activity, Transaction: cell(row, "TCODE")}
			if paths, perr := c.imgPaths(ctx, activity, lang); perr == nil && len(paths) > 0 {
				n.Path = paths[0]
				if len(paths) > 1 {
					n.Path += fmt.Sprintf(" (+%d more)", len(paths)-1)
				}
			}
			nodes = append(nodes, n)
		}
	}
	if len(nodes) >= limit {
		return nodes, nil
	}

	res, err := c.RunQuery(ctx, fmt.Sprintf("SELECT node_id, text FROM tnodeimgt WHERE spras = '%s' AND text LIKE '%s' ORDER BY text", sqlQuote(lang), sqlQuote(like)), limit-len(nodes))
	if err != nil {
		return nil, fmt.Errorf("searching IMG folders: %w", err)
	}
	if res == nil {
		return nodes, nil
	}
	for _, row := range res.Rows {
		n := IMGNode{NodeID: cell(row, "NODE_ID"), Text: cell(row, "TEXT")}
		if refs, rerr := c.RunQuery(ctx, fmt.Sprintf("SELECT ref_type, ref_object FROM tnodeimgr WHERE node_id = '%s'", sqlQuote(n.NodeID)), 5); rerr == nil && refs != nil {
			for _, r := range refs.Rows {
				t := cell(r, "REF_TYPE")
				if t == "COBJ" || t == "ACTI" || n.RefType == "" {
					n.RefType, n.RefObject = t, cell(r, "REF_OBJECT")
				}
			}
		}
		if path, perr := c.imgPathOf(ctx, n.NodeID, lang); perr == nil {
			n.Path = path
		}
		nodes = append(nodes, n)
	}
	return nodes, nil
}

// imgPaths finds every node pointing at an activity and renders the path to
// each.
func (c *Client) imgPaths(ctx context.Context, activity, lang string) ([]string, error) {
	res, err := c.RunQuery(ctx, fmt.Sprintf("SELECT node_id FROM tnodeimgr WHERE ref_object = '%s' AND ( ref_type = 'COBJ' OR ref_type = 'ACTI' )", sqlQuote(activity)), 20)
	if err != nil {
		return nil, fmt.Errorf("reading IMG references: %w", err)
	}
	var paths []string
	if res == nil {
		return paths, nil
	}
	for _, row := range res.Rows {
		if p, perr := c.imgPathOf(ctx, cell(row, "NODE_ID"), spras(lang)); perr == nil && p != "" {
			paths = append(paths, p)
		}
	}
	sort.Strings(paths)
	return paths, nil
}

// imgPathOf walks TNODEIMG upwards and joins the node texts, root first.
func (c *Client) imgPathOf(ctx context.Context, nodeID, lang string) (string, error) {
	var texts []string
	seen := map[string]bool{}
	for depth := 0; nodeID != "" && depth < 12 && !seen[nodeID]; depth++ {
		seen[nodeID] = true
		res, err := c.RunQuery(ctx, fmt.Sprintf("SELECT n~parent_id, t~text FROM tnodeimg AS n LEFT OUTER JOIN tnodeimgt AS t ON t~node_id = n~node_id AND t~spras = '%s' WHERE n~node_id = '%s'", sqlQuote(lang), sqlQuote(nodeID)), 1)
		if err != nil {
			return "", err
		}
		if res == nil || len(res.Rows) == 0 {
			break
		}
		if t := cell(res.Rows[0], "TEXT"); t != "" {
			texts = append([]string{t}, texts...)
		}
		nodeID = cell(res.Rows[0], "PARENT_ID")
	}
	return strings.Join(texts, " > "), nil
}
