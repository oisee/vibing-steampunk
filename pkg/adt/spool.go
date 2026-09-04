package adt

import (
	"context"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/oisee/vibing-steampunk/pkg/temse"
)

// The spool is three ordinary tables. TSP01 is the request: number, owner,
// title, when, what kind of document. TST01 is the TemSe object the request
// points at, with its code page and record type and where it is stored.
// TST03 is that object's content, in blocks, when it is stored in the
// database — the default; a system that keeps spool in files has the rows
// of TST01 and nothing in TST03, and then only the XBP interface over RFC
// can return the content. TBTCP ties a spool to the background job step
// that wrote it.

// SpoolRequest is one TSP01 row with what TST01 and TBTCP add to it.
type SpoolRequest struct {
	Number  int       `json:"number"`
	Client  string    `json:"client,omitempty"`
	Owner   string    `json:"owner"`
	Created time.Time `json:"created"`
	Title   string    `json:"title,omitempty"`
	// Suffixes are TSP01's three name parts (RQ0NAME, RQ1NAME, RQ2NAME):
	// usually the list kind, the device and the program that wrote it.
	Suffixes []string `json:"suffixes,omitempty"`
	// DocType is LIST for an ABAP list, OTF for SAPscript, RAW, or a PDF/
	// Smart Forms kind.
	DocType string `json:"docType,omitempty"`
	Device  string `json:"device,omitempty"`
	Pages   int    `json:"pages,omitempty"`
	Copies  int    `json:"copies,omitempty"`
	// TemSe is the object holding the content, and Storage where it is:
	// D in the database, F in a file.
	TemSe   string `json:"temse,omitempty"`
	Storage string `json:"storage,omitempty"`
	Lines   int    `json:"lines,omitempty"`
	Bytes   int    `json:"bytes,omitempty"`
	// Codepage is the content's SAP code page (4103 on Unicode systems).
	Codepage string `json:"codepage,omitempty"`
	// Job is the background job step that produced the request, when one did.
	Job *SpoolJobRef `json:"job,omitempty"`
}

// SpoolJobRef is the TBTCP row pointing at a spool.
type SpoolJobRef struct {
	Name    string `json:"name"`
	Count   string `json:"count"`
	Step    int    `json:"step"`
	Program string `json:"program,omitempty"`
	Variant string `json:"variant,omitempty"`
	User    string `json:"user,omitempty"`
}

// SpoolFilter narrows a listing. Everything is optional; Limit always applies.
type SpoolFilter struct {
	Owner   string
	Title   string // a LIKE pattern with * wildcards
	Program string // matches the third suffix, where the writer put the program
	Job     string // a job name; only spools written by that job's steps
	From    time.Time
	To      time.Time
	Limit   int
}

// SpoolRequests lists spool requests, newest first.
func (c *Client) SpoolRequests(ctx context.Context, filter SpoolFilter) ([]SpoolRequest, error) {
	limit := filter.Limit
	if limit <= 0 {
		limit = 50
	}
	var terms []string
	if v := strings.ToUpper(strings.TrimSpace(filter.Owner)); v != "" {
		terms = append(terms, "rqowner = '"+sqlQuote(v)+"'")
	}
	if v := strings.TrimSpace(filter.Title); v != "" {
		terms = append(terms, "rqtitle LIKE '"+sqlQuote(strings.ReplaceAll(v, "*", "%"))+"'")
	}
	if v := strings.ToUpper(strings.TrimSpace(filter.Program)); v != "" {
		if len(v) > 12 {
			v = v[:12] // RQ2NAME holds twelve characters of it
		}
		terms = append(terms, "rq2name = '"+sqlQuote(v)+"'")
	}
	if !filter.From.IsZero() {
		terms = append(terms, "rqcretime >= '"+filter.From.Format("20060102150405")+"'")
	}
	if !filter.To.IsZero() {
		terms = append(terms, "rqcretime <= '"+filter.To.Format("20060102150405")+"99'")
	}
	var byJob map[int]SpoolJobRef
	if job := strings.ToUpper(strings.TrimSpace(filter.Job)); job != "" {
		refs, err := c.spoolJobRefs(ctx, "jobname = '"+sqlQuote(job)+"' AND listident <> '0000000000'", 5000)
		if err != nil {
			return nil, err
		}
		if len(refs) == 0 {
			return nil, nil
		}
		byJob = refs
		ids := make([]string, 0, len(refs))
		for n := range refs {
			ids = append(ids, strconv.Itoa(n))
		}
		sort.Strings(ids)
		terms = append(terms, "rqident IN (\n"+strings.Join(ids, ",\n")+" )")
	}

	query := "SELECT rqident, rqclient, rqowner, rqcretime, rqtitle, rq0name, rq1name, rq2name, rqdoctype, rqdest, rqcopies, rqpjreq, rqo1name FROM tsp01"
	if len(terms) > 0 {
		query += " WHERE " + strings.Join(terms, " AND ")
	}
	query += " ORDER BY rqcretime DESCENDING"
	res, err := c.RunQuery(ctx, query, limit)
	if err != nil {
		return nil, fmt.Errorf("reading spool requests: %w", err)
	}
	if res == nil {
		return nil, nil
	}
	var out []SpoolRequest
	for _, row := range res.Rows {
		r := SpoolRequest{
			Client: cell(row, "RQCLIENT"), Owner: cell(row, "RQOWNER"), Title: cell(row, "RQTITLE"),
			DocType: cell(row, "RQDOCTYPE"), Device: cell(row, "RQDEST"), TemSe: cell(row, "RQO1NAME"),
			Created: parseSpoolTime(cell(row, "RQCRETIME")),
		}
		r.Number, _ = strconv.Atoi(cell(row, "RQIDENT"))
		r.Copies, _ = strconv.Atoi(cell(row, "RQCOPIES"))
		r.Pages, _ = strconv.Atoi(cell(row, "RQPJREQ"))
		for _, k := range []string{"RQ0NAME", "RQ1NAME", "RQ2NAME"} {
			r.Suffixes = append(r.Suffixes, cell(row, k))
		}
		out = append(out, r)
	}
	if len(out) == 0 {
		return out, nil
	}

	// TST01 for size, lines and storage; TBTCP for the job, unless the
	// listing was by job and the refs are already known.
	names := make([]string, 0, len(out))
	ids := make([]string, 0, len(out))
	for _, r := range out {
		if r.TemSe != "" {
			names = append(names, "'"+sqlQuote(r.TemSe)+"'")
		}
		ids = append(ids, strconv.Itoa(r.Number))
	}
	headers, err := c.temseHeaders(ctx, names)
	if err != nil {
		return nil, err
	}
	if byJob == nil {
		if byJob, err = c.spoolJobRefs(ctx, "listident IN (\n"+strings.Join(padSpoolIDs(ids), ",\n")+" )", len(ids)*2); err != nil {
			return nil, err
		}
	}
	for i := range out {
		if h, ok := headers[out[i].TemSe]; ok {
			out[i].Storage, out[i].Codepage, out[i].Lines, out[i].Bytes = h.Storage, h.Codepage, h.Rows, h.Size
		}
		if ref, ok := byJob[out[i].Number]; ok {
			ref := ref
			out[i].Job = &ref
		}
	}
	return out, nil
}

func padSpoolIDs(ids []string) []string {
	out := make([]string, len(ids))
	for i, id := range ids {
		out[i] = "'" + strings.Repeat("0", 10-len(id)) + id + "'"
	}
	return out
}

// spoolJobRefs reads TBTCP rows and keys them by spool number.
func (c *Client) spoolJobRefs(ctx context.Context, where string, limit int) (map[int]SpoolJobRef, error) {
	res, err := c.RunQuery(ctx, "SELECT jobname, jobcount, stepcount, progname, variant, authcknam, listident FROM tbtcp WHERE "+where, limit)
	if err != nil {
		return nil, fmt.Errorf("reading job steps: %w", err)
	}
	out := map[int]SpoolJobRef{}
	if res == nil {
		return out, nil
	}
	for _, row := range res.Rows {
		n, _ := strconv.Atoi(cell(row, "LISTIDENT"))
		if n == 0 {
			continue
		}
		step, _ := strconv.Atoi(cell(row, "STEPCOUNT"))
		out[n] = SpoolJobRef{Name: cell(row, "JOBNAME"), Count: cell(row, "JOBCOUNT"), Step: step,
			Program: cell(row, "PROGNAME"), Variant: cell(row, "VARIANT"), User: cell(row, "AUTHCKNAM")}
	}
	return out, nil
}

// TemSeHeader is what TST01 says about an object.
type TemSeHeader struct {
	Name       string `json:"name"`
	Type       string `json:"type,omitempty"`
	RecordType string `json:"recordType,omitempty"`
	Codepage   string `json:"codepage,omitempty"`
	Storage    string `json:"storage,omitempty"`
	Rows       int    `json:"rows,omitempty"`
	Size       int    `json:"size,omitempty"`
	LineLength int    `json:"lineLength,omitempty"`
	Parts      int    `json:"parts,omitempty"`
	Creator    string `json:"creator,omitempty"`
}

func (c *Client) temseHeaders(ctx context.Context, quotedNames []string) (map[string]TemSeHeader, error) {
	out := map[string]TemSeHeader{}
	if len(quotedNames) == 0 {
		return out, nil
	}
	res, err := c.RunQuery(ctx, "SELECT dname, dtype, drectyp, dcharcod, dstotyp, drows, dsize, dlinelen, dnoparts, dcreater FROM tst01 WHERE dname IN (\n"+strings.Join(quotedNames, ",\n")+" )", len(quotedNames)*2)
	if err != nil {
		return nil, fmt.Errorf("reading TemSe headers: %w", err)
	}
	if res == nil {
		return out, nil
	}
	for _, row := range res.Rows {
		h := TemSeHeader{Name: cell(row, "DNAME"), Type: cell(row, "DTYPE"), RecordType: cell(row, "DRECTYP"),
			Codepage: cell(row, "DCHARCOD"), Storage: cell(row, "DSTOTYP"), Creator: cell(row, "DCREATER")}
		h.Rows, _ = strconv.Atoi(cell(row, "DROWS"))
		h.Size, _ = strconv.Atoi(cell(row, "DSIZE"))
		h.LineLength, _ = strconv.Atoi(cell(row, "DLINELEN"))
		h.Parts, _ = strconv.Atoi(cell(row, "DNOPARTS"))
		out[h.Name] = h
	}
	return out, nil
}

// TemSeObject reads one TemSe object's header and content from TST01/TST03.
func (c *Client) TemSeObject(ctx context.Context, name string) (*TemSeHeader, []byte, error) {
	headers, err := c.temseHeaders(ctx, []string{"'" + sqlQuote(name) + "'"})
	if err != nil {
		return nil, nil, err
	}
	h, ok := headers[name]
	if !ok {
		return nil, nil, fmt.Errorf("TemSe object %s is not in TST01", name)
	}
	if h.Storage != "" && h.Storage != "D" {
		return &h, nil, fmt.Errorf("TemSe object %s is stored in %s, not the database; only XBP over RFC can read it", name, storageName(h.Storage))
	}
	res, err := c.RunQuery(ctx, "SELECT dpart, drowno, ddatalen, dcontent FROM tst03 WHERE dname = '"+sqlQuote(name)+"' ORDER BY dpart, drowno", 100000)
	if err != nil {
		return &h, nil, fmt.Errorf("reading TemSe content of %s: %w", name, err)
	}
	if res == nil || len(res.Rows) == 0 {
		return &h, nil, fmt.Errorf("TemSe object %s has no content rows in TST03", name)
	}
	var data []byte
	for _, row := range res.Rows {
		block, err := decodeHexCell(cell(row, "DCONTENT"))
		if err != nil {
			return &h, nil, fmt.Errorf("TemSe object %s: %w", name, err)
		}
		n, _ := strconv.Atoi(cell(row, "DDATALEN"))
		if n > 0 && n < len(block) {
			block = block[:n]
		}
		data = append(data, block...)
	}
	return &h, data, nil
}

func storageName(code string) string {
	switch code {
	case "F":
		return "a file on the application server"
	case "D":
		return "the database"
	}
	return "storage type " + code
}

// SpoolContent is one spool request with its content read and, for a list,
// decoded.
type SpoolContent struct {
	Request SpoolRequest `json:"request"`
	Header  *TemSeHeader `json:"temse,omitempty"`
	// List is the decoded ABAP list for DocType LIST.
	List *temse.List `json:"list,omitempty"`
	// Raw is the content of any other document type, as stored.
	Raw []byte `json:"raw,omitempty"`
}

// Spool reads one spool request and its content over ADT.
func (c *Client) Spool(ctx context.Context, number int) (*SpoolContent, error) {
	res, err := c.RunQuery(ctx, "SELECT rqident, rqclient, rqowner, rqcretime, rqtitle, rq0name, rq1name, rq2name, rqdoctype, rqdest, rqcopies, rqpjreq, rqo1name FROM tsp01 WHERE rqident = "+strconv.Itoa(number), 1)
	if err != nil {
		return nil, fmt.Errorf("reading spool request %d: %w", number, err)
	}
	if res == nil || len(res.Rows) == 0 {
		return nil, fmt.Errorf("spool request %d does not exist", number)
	}
	row := res.Rows[0]
	req := SpoolRequest{Number: number, Client: cell(row, "RQCLIENT"), Owner: cell(row, "RQOWNER"), Title: cell(row, "RQTITLE"),
		DocType: cell(row, "RQDOCTYPE"), Device: cell(row, "RQDEST"), TemSe: cell(row, "RQO1NAME"), Created: parseSpoolTime(cell(row, "RQCRETIME"))}
	req.Copies, _ = strconv.Atoi(cell(row, "RQCOPIES"))
	req.Pages, _ = strconv.Atoi(cell(row, "RQPJREQ"))
	for _, k := range []string{"RQ0NAME", "RQ1NAME", "RQ2NAME"} {
		req.Suffixes = append(req.Suffixes, cell(row, k))
	}
	if refs, err := c.spoolJobRefs(ctx, "listident = '"+fmt.Sprintf("%010d", number)+"'", 2); err == nil {
		if ref, ok := refs[number]; ok {
			req.Job = &ref
		}
	}
	out := &SpoolContent{Request: req}
	if req.TemSe == "" {
		return out, fmt.Errorf("spool request %d has no TemSe object", number)
	}
	h, data, err := c.TemSeObject(ctx, req.TemSe)
	out.Header = h
	if err != nil {
		return out, err
	}
	req.Storage, req.Codepage, req.Lines, req.Bytes = h.Storage, h.Codepage, h.Rows, h.Size
	out.Request = req
	if strings.EqualFold(req.DocType, "LIST") || strings.EqualFold(h.Type, "TEXT") {
		list, err := temse.DecodeList(data, h.Codepage)
		if err != nil {
			out.Raw = data
			return out, err
		}
		out.List = list
		return out, nil
	}
	out.Raw = data
	return out, nil
}

// parseSpoolTime reads TSP01's RQCRETIME: YYYYMMDDHHMMSS and two more digits.
func parseSpoolTime(s string) time.Time {
	s = strings.TrimSpace(s)
	if len(s) < 14 {
		return time.Time{}
	}
	t, err := time.Parse("20060102150405", s[:14])
	if err != nil {
		return time.Time{}
	}
	return t
}

func decodeHexCell(s string) ([]byte, error) {
	s = strings.TrimSpace(s)
	if len(s)%2 != 0 {
		return nil, fmt.Errorf("raw column has %d hex digits", len(s))
	}
	out := make([]byte, len(s)/2)
	for i := 0; i < len(out); i++ {
		v, err := strconv.ParseUint(s[2*i:2*i+2], 16, 8)
		if err != nil {
			return nil, fmt.Errorf("raw column is not hex at %d: %w", 2*i, err)
		}
		out[i] = byte(v)
	}
	return out, nil
}
