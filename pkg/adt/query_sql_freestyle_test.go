package adt

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
)

// freestyleFake answers datapreview/freestyle the way SAP's handler
// (CL_ADT_DP_OPEN_SQL_HANDLER) reads the body, as far as line breaks go. Both
// versions were read from SAP's source, on SAP_BASIS 7.40 and on S/4HANA:
//
//   - 7.40 takes the body as one string, splits it at CR LF only and joins the
//     pieces with a blank. An LF left over ends up in the generated ABAP, and
//     SAP refuses with a message about a token the caller never wrote.
//   - Later releases read the body as plain text, where CR LF and LF are both
//     line breaks, and refuse a line longer than 255 characters.
//   - Both drop a line that starts with '*' and cut a line at '"': ABAP
//     comments.
type freestyleFake struct {
	*httptest.Server
	release string // "740" or "758"

	mu         sync.Mutex
	bodies     []string
	rows       []string
	statements []string // what SAP's handler made of each body
}

func newFreestyleFake(t *testing.T, release string) *freestyleFake {
	t.Helper()
	f := &freestyleFake{release: release}
	f.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("x-csrf-token", "AbCdEfGhIjKlMnOpQrStUv==")
		if r.URL.Path != "/sap/bc/adt/datapreview/freestyle" || r.Method != http.MethodPost {
			_, _ = w.Write([]byte("ok"))
			return
		}
		b, _ := io.ReadAll(r.Body)
		body := string(b)
		read := sapStatement740
		if f.release != "740" {
			read = sapStatement75x
		}
		stmt, err := read(body)
		f.mu.Lock()
		f.bodies = append(f.bodies, body)
		f.rows = append(f.rows, r.URL.Query().Get("rowNumber"))
		f.statements = append(f.statements, stmt)
		f.mu.Unlock()
		if err != nil {
			w.WriteHeader(http.StatusBadRequest)
			fmt.Fprintf(w, `<?xml version="1.0" encoding="utf-8"?><exc:exception xmlns:exc="http://www.sap.com/abapxml/types/communicationframework"><namespace id="http://www.sap.com/adt/wda/dataPreview"/><type id="ExceptionDataPreviewGeneral"/><message lang="EN">%s</message></exc:exception>`, xmlEscapeTest(err.Error()))
			return
		}
		_, _ = w.Write([]byte(previewXML([]string{"N"}, []string{"42"})))
	}))
	t.Cleanup(f.Close)
	return f
}

// sapStatement740 is GET_INSTANCE on 7.40: SPLIT AT cr_lf, drop comment lines,
// CONCATENATE ... SEPARATED BY space.
func sapStatement740(body string) (string, error) {
	stmt := joinStatementLines(strings.Split(body, "\r\n"))
	if strings.ContainsAny(stmt, "\r\n") {
		return "", errors.New(`"ROWS" may not be converted into a number.`)
	}
	return stmt, nil
}

// sapStatement75x is the plain-text handler and the 255 check of later
// releases.
func sapStatement75x(body string) (string, error) {
	lines := strings.Split(strings.ReplaceAll(body, "\r\n", "\n"), "\n")
	for n, l := range lines {
		if len(l) > 255 {
			return "", fmt.Errorf("Line %d is longer than 255 characters.", n+1)
		}
	}
	return joinStatementLines(lines), nil
}

func joinStatementLines(lines []string) string {
	var kept []string
	for _, l := range lines {
		if l == "" || l[0] == '*' {
			continue
		}
		if i := strings.IndexByte(l, '"'); i >= 0 {
			l = l[:i]
		}
		kept = append(kept, l)
	}
	return strings.TrimSpace(strings.Join(kept, " "))
}

func (f *freestyleFake) last() (body, rows, stmt string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	n := len(f.bodies) - 1
	if n < 0 {
		return "", "", ""
	}
	return f.bodies[n], f.rows[n], f.statements[n]
}

// The statement from the 7.40 report: 232 characters, past the 200 at which
// wrapSQL breaks a line.
const longJoinStatement = "SELECT PRODH,l~VKORG,VSTEL,VDATU,WADAT_IST,COUNT(*) AS N FROM LIPS AS p JOIN LIKP AS l ON l~VBELN = p~VBELN JOIN VBAK AS a ON a~VBELN = p~VGBEL WHERE AUDAT LIKE '202305%' AND LFART = 'LF' GROUP BY PRODH,l~VKORG,VSTEL,VDATU,WADAT_IST"

func TestRunQuery_LongStatementGoesOutWithCRLFBetweenLines(t *testing.T) {
	if len(longJoinStatement) != 232 {
		t.Fatalf("the statement is %d characters, the test wants 232", len(longJoinStatement))
	}
	// Exactly what goes on the wire: one break, CR LF, at the last blank
	// before 200 characters.
	const want = "SELECT PRODH,l~VKORG,VSTEL,VDATU,WADAT_IST,COUNT(*) AS N FROM LIPS AS p JOIN LIKP AS l ON l~VBELN = p~VBELN JOIN VBAK AS a ON a~VBELN = p~VGBEL WHERE AUDAT LIKE '202305%' AND LFART = 'LF' GROUP BY" +
		"\r\n" +
		"PRODH,l~VKORG,VSTEL,VDATU,WADAT_IST"

	for _, release := range []string{"740", "758"} {
		for _, top := range []int{0, 1000000} {
			f := newFreestyleFake(t, release)
			c := NewClient(f.URL, "TESTUSER", "secret")
			res, err := c.RunQuery(context.Background(), longJoinStatement, top)
			if err != nil {
				t.Fatalf("%s, top %d: %v", release, top, err)
			}
			body, rows, stmt := f.last()
			if body != want {
				t.Errorf("%s, top %d: body\n%q\nwant\n%q", release, top, body, want)
			}
			if stmt != longJoinStatement {
				t.Errorf("%s, top %d: SAP's handler read\n%q\nnot the statement", release, top, stmt)
			}
			wantRows := "100"
			if top > 0 {
				wantRows = "1000000"
			}
			if rows != wantRows {
				t.Errorf("%s, top %d: rowNumber %q, want %q", release, top, rows, wantRows)
			}
			if len(res.Rows) != 1 || res.Rows[0]["N"] != "42" {
				t.Errorf("%s, top %d: rows %v", release, top, res.Rows)
			}
		}
	}
}

// The fake has to fail the way 7.40 did, or the test above proves nothing:
// the same statement with a bare LF at the break is refused.
func TestFreestyleFake740RefusesABareLF(t *testing.T) {
	f := newFreestyleFake(t, "740")
	c := NewClient(f.URL, "TESTUSER", "secret")
	lf := strings.ReplaceAll(freestyleBody(longJoinStatement), "\r\n", "\n")
	_, err := c.transport.Request(context.Background(), "/sap/bc/adt/datapreview/freestyle", &RequestOptions{
		Method: http.MethodPost, Body: []byte(lf), ContentType: "text/plain",
	})
	if msg, ok := sapQueryMessage(err); !ok || !strings.Contains(msg, `"ROWS"`) {
		t.Fatalf("a bare LF went through the 7.40 fake: %v", err)
	}
	f758 := newFreestyleFake(t, "758")
	c = NewClient(f758.URL, "TESTUSER", "secret")
	if _, err := c.transport.Request(context.Background(), "/sap/bc/adt/datapreview/freestyle", &RequestOptions{
		Method: http.MethodPost, Body: []byte(lf), ContentType: "text/plain",
	}); err != nil {
		t.Fatalf("later releases take an LF: %v", err)
	}
}

// The caller's own line breaks, LF or CR LF, go out as CR LF.
func TestRunQuery_CallersLineBreaksGoOutAsCRLF(t *testing.T) {
	const want = "SELECT carrid, connid\r\nFROM spfli\r\nWHERE carrid = 'LH'"
	for _, in := range []string{
		"SELECT carrid, connid\nFROM spfli\nWHERE carrid = 'LH'",
		"SELECT carrid, connid\r\nFROM spfli\r\nWHERE carrid = 'LH'",
	} {
		for _, release := range []string{"740", "758"} {
			f := newFreestyleFake(t, release)
			c := NewClient(f.URL, "TESTUSER", "secret")
			if _, err := c.RunQuery(context.Background(), in, 10); err != nil {
				t.Fatalf("%s, %q: %v", release, in, err)
			}
			body, _, stmt := f.last()
			if body != want {
				t.Errorf("%s, %q: body %q, want %q", release, in, body, want)
			}
			if stmt != "SELECT carrid, connid FROM spfli WHERE carrid = 'LH'" {
				t.Errorf("%s, %q: SAP's handler read %q", release, in, stmt)
			}
		}
	}
}

// A line break inside a literal, LF or CR LF, goes out as one CR LF: SAP splits
// there as it always did, and no stray CR is left in the literal.
func TestFreestyleBody_LineBreakInsideALiteralIsOneCRLF(t *testing.T) {
	const want = "SELECT mtext FROM t000 WHERE mtext = 'A\r\nB'"
	for _, in := range []string{
		"SELECT mtext FROM t000 WHERE mtext = 'A\nB'",
		"SELECT mtext FROM t000 WHERE mtext = 'A\r\nB'",
	} {
		if got := freestyleBody(in); got != want {
			t.Errorf("%q: body %q, want %q", in, got, want)
		}
	}
}

// A wrapped line that starts with '*' is a comment line to the data preview,
// which would drop the rest of COUNT( * ). wrapSQL puts a blank in front.
func TestRunQuery_AWrappedLineNeverStartsWithAnAsterisk(t *testing.T) {
	var lits []string
	for i := 0; i < 16; i++ {
		lits = append(lits, fmt.Sprintf("'A%02d'", i))
	}
	in := "SELECT kunnr FROM vbak WHERE vkorg = '1000' AND auart IN ( " + strings.Join(lits, ", ") + " ) GROUP BY kunnr HAVING COUNT( * ) > 1"
	first := "SELECT kunnr FROM vbak WHERE vkorg = '1000' AND auart IN ( " + strings.Join(lits, ", ") + " ) GROUP BY kunnr HAVING COUNT("
	if len(first) != 200 {
		t.Fatalf("the first line is %d characters; the '*' must land on the break at 200", len(first))
	}
	want := first + "\r\n * ) > 1"

	for _, release := range []string{"740", "758"} {
		f := newFreestyleFake(t, release)
		c := NewClient(f.URL, "TESTUSER", "secret")
		if _, err := c.RunQuery(context.Background(), in, 10); err != nil {
			t.Fatalf("%s: %v", release, err)
		}
		body, _, stmt := f.last()
		if body != want {
			t.Errorf("%s: body\n%q\nwant\n%q", release, body, want)
		}
		if strings.Join(strings.Fields(stmt), " ") != in {
			t.Errorf("%s: SAP's handler read\n%q", release, stmt)
		}
	}
}
