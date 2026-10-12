package adt

import (
	"context"
	"encoding/xml"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
)

// --- Transaction Operations ---

// Transaction represents an SAP transaction.
//
// The header (Description, Package, Responsible) comes from the VIT resource
// where the release has one (7.50 and later) and from TSTCT and TADIR where it
// has not (7.40). What the transaction starts comes from TSTC and TSTCP, which
// every release has: ADT does not give it at all.
//
// Kind is set only where the rows say it plainly: "report" (CINFO bit 80),
// "area_menu" (bit 01), "oo" (a \CLASS= parameter), "parameter" (a /* or /N
// parameter, CalledTransaction then names the transaction it calls),
// "variant" (@@VARIANT TCODE), "dialog" (a program and nothing else). The
// other CINFO bits are not read; CINFO keeps the raw byte.
type Transaction struct {
	Name              string
	Description       string
	Program           string
	Package           string `json:",omitempty"`
	Responsible       string `json:",omitempty"`
	Screen            string `json:",omitempty"`
	Kind              string `json:",omitempty"`
	CINFO             string `json:",omitempty"`
	Parameter         string `json:",omitempty"`
	CalledTransaction string `json:",omitempty"`
	Variant           string `json:",omitempty"`
	Class             string `json:",omitempty"`
	Method            string `json:",omitempty"`
	// Note is a caveat about the result: a part that could not be read.
	Note string `json:",omitempty"`
}

// transactionHeader is the VIT answer for TRANT. An unknown transaction
// comes back 200 all the same, with the name and nothing else.
type transactionHeader struct {
	Name        string `xml:"name,attr"`
	Description string `xml:"description,attr"`
	Responsible string `xml:"responsible,attr"`
	PackageRef  struct {
		Name string `xml:"name,attr"`
	} `xml:"packageRef"`
}

// GetTransaction retrieves information about a transaction.
func (c *Client) GetTransaction(ctx context.Context, tcode string) (*Transaction, error) {
	tcode = strings.ToUpper(strings.TrimSpace(tcode))
	if tcode == "" {
		return nil, fmt.Errorf("empty transaction name")
	}
	tran := &Transaction{Name: tcode}
	var notes []string

	// VIT knows the transaction as TRANT (type and subtype), not TRAN, and
	// answers only in its own content type.
	resp, err := c.transport.Request(ctx, "/sap/bc/adt/vit/wb/object_type/TRANT/object_name/"+url.PathEscape(tcode), &RequestOptions{
		Method: http.MethodGet,
		Accept: "application/vnd.sap.adt.basic.object.properties+xml",
	})
	headerRead := false
	if err == nil {
		var h transactionHeader
		if perr := xml.Unmarshal(resp.Body, &h); perr != nil {
			notes = append(notes, fmt.Sprintf("header not parsed: %v", perr))
		} else {
			tran.Description, tran.Responsible, tran.Package = h.Description, h.Responsible, h.PackageRef.Name
			headerRead = true
		}
	} else if !IsRouterNotFound(err) {
		notes = append(notes, fmt.Sprintf("header resource: %v", err))
	}

	tstc, err := c.GetTableContents(ctx, "TSTC", 1,
		"SELECT TCODE, PGMNA, DYPNO, CINFO FROM TSTC WHERE TCODE = '"+sqlQuote(tcode)+"'")
	if err != nil {
		if !headerRead || tran.Description == "" && tran.Package == "" {
			return nil, fmt.Errorf("getting transaction %s: TSTC: %w", tcode, err)
		}
		notes = append(notes, fmt.Sprintf("what the transaction starts is not known, TSTC: %v", err))
		tran.Note = strings.Join(notes, "; ")
		return tran, nil
	}
	if len(tstc.Rows) == 0 {
		return nil, fmt.Errorf("transaction %s does not exist", tcode)
	}
	row := tstc.Rows[0]
	tran.Program = rowString(row, "PGMNA")
	if screen := rowString(row, "DYPNO"); strings.Trim(screen, "0") != "" {
		tran.Screen = screen
	}
	tran.CINFO = strings.ToUpper(rowString(row, "CINFO"))
	cinfo, _ := strconv.ParseUint(tran.CINFO, 16, 8)

	if tran.Program == "" || cinfo&0x0A != 0 {
		tstcp, err := c.GetTableContents(ctx, "TSTCP", 1,
			"SELECT TCODE, PARAM FROM TSTCP WHERE TCODE = '"+sqlQuote(tcode)+"'")
		if err != nil {
			notes = append(notes, fmt.Sprintf("parameter not read, TSTCP: %v", err))
		} else if len(tstcp.Rows) > 0 {
			tran.Parameter = rowString(tstcp.Rows[0], "PARAM")
		}
	}
	classifyTransaction(tran, cinfo)

	if !headerRead {
		if err := c.transactionHeaderFromTables(ctx, tran); err != nil {
			notes = append(notes, err.Error())
		}
	}
	tran.Note = strings.Join(notes, "; ")
	return tran, nil
}

// transactionHeaderFromTables fills the header where the release has no VIT
// resource: the text in the session language (else any), package and
// responsible from TADIR.
func (c *Client) transactionHeaderFromTables(ctx context.Context, tran *Transaction) error {
	var failed []string
	texts, err := c.GetTableContents(ctx, "TSTCT", 50,
		"SELECT SPRSL, TCODE, TTEXT FROM TSTCT WHERE TCODE = '"+sqlQuote(tran.Name)+"'")
	if err != nil {
		failed = append(failed, fmt.Sprintf("TSTCT: %v", err))
	} else {
		lang := sapLanguageKey(c.config.Language)
		for _, r := range texts.Rows {
			if tran.Description == "" || rowString(r, "SPRSL") == lang {
				tran.Description = rowString(r, "TTEXT")
			}
		}
	}
	dir, err := c.GetTableContents(ctx, "TADIR", 1,
		"SELECT OBJ_NAME, DEVCLASS, AUTHOR FROM TADIR WHERE PGMID = 'R3TR' AND OBJECT = 'TRAN' AND OBJ_NAME = '"+sqlQuote(tran.Name)+"'")
	if err != nil {
		failed = append(failed, fmt.Sprintf("TADIR: %v", err))
	} else if len(dir.Rows) > 0 {
		tran.Package = rowString(dir.Rows[0], "DEVCLASS")
		tran.Responsible = rowString(dir.Rows[0], "AUTHOR")
	}
	if len(failed) > 0 {
		return fmt.Errorf("header not read: %s", strings.Join(failed, "; "))
	}
	return nil
}

// classifyTransaction sets Kind and what the parameter names, from the
// parameter's shape first and the plain CINFO bits after it.
func classifyTransaction(tran *Transaction, cinfo uint64) {
	p := tran.Parameter
	switch {
	case strings.HasPrefix(p, `\`) && strings.Contains(p, "CLASS="):
		tran.Kind = "oo"
		for _, part := range strings.Split(p, `\`) {
			key, value, ok := strings.Cut(part, "=")
			if !ok {
				continue
			}
			switch strings.ToUpper(key) {
			case "CLASS":
				tran.Class = value
			case "METHOD":
				tran.Method = value
			case "PROGRAM":
				if tran.Program == "" {
					tran.Program = value
				}
			}
		}
		return
	case strings.HasPrefix(p, "@@"):
		fields := strings.Fields(p[2:])
		if len(fields) == 2 {
			tran.Kind = "variant"
			tran.Variant, tran.CalledTransaction = fields[0], fields[1]
		}
		return
	case strings.HasPrefix(p, "/*"), strings.HasPrefix(strings.ToUpper(p), "/N"):
		if fields := strings.Fields(p[2:]); len(fields) > 0 {
			tran.Kind = "parameter"
			tran.CalledTransaction = fields[0]
		}
		return
	case p != "":
		return
	}
	switch {
	case cinfo&0x01 != 0:
		tran.Kind = "area_menu"
	case cinfo&0x80 != 0:
		tran.Kind = "report"
	case tran.Program != "":
		tran.Kind = "dialog"
	}
}
