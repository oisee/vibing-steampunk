package adt

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"
)

// Answers of the VIT resource for TRANT, taken from a 7.50 system.
const (
	trantSE38    = `<?xml version="1.0" encoding="utf-8"?><adtcore:mainObject adtcore:responsible="SAP" adtcore:masterLanguage="DE" adtcore:masterSystem="SAP" adtcore:name="SE38" adtcore:type="TRAN/T" adtcore:version="active" adtcore:description="ABAP-редактор" adtcore:language="RU" xmlns:adtcore="http://www.sap.com/adt/core"><adtcore:packageRef adtcore:uri="/sap/bc/adt/vit/wb/object_type/devck/object_name/SEDT" adtcore:type="DEVC/K" adtcore:name="SEDT"/></adtcore:mainObject>`
	trantUnknown = `<?xml version="1.0" encoding="utf-8"?><adtcore:mainObject adtcore:name="ZNO_SUCH_TX" adtcore:type="TRAN/T" adtcore:version="active" adtcore:language="RU" xmlns:adtcore="http://www.sap.com/adt/core"/>`
	// 7.40 has no VIT at all.
	vitNoHandler = "No application class found for URI: /sap/bc/adt/vit/wb/object_type/TRANT/object_name/S"
)

// transactionDoer answers the VIT resource with a fixed status and body,
// and data preview per table: columns and rows, or a failure status.
type transactionDoer struct {
	vitStatus int
	vitBody   string
	tables    map[string]tableAnswer

	vitPath   string
	vitAccept string
	reads     []string
}

type tableAnswer struct {
	columns []string
	rows    [][]string
	status  int
}

func (d *transactionDoer) Do(req *http.Request) (*http.Response, error) {
	resp := func(status int, body string) (*http.Response, error) {
		return &http.Response{StatusCode: status, Body: io.NopCloser(strings.NewReader(body)), Header: http.Header{"X-Csrf-Token": []string{"t"}}}, nil
	}
	switch {
	case strings.HasPrefix(req.URL.Path, "/sap/bc/adt/vit/"):
		d.vitPath, d.vitAccept = req.URL.EscapedPath(), req.Header.Get("Accept")
		return resp(d.vitStatus, d.vitBody)
	case req.URL.Path == "/sap/bc/adt/datapreview/ddic":
		table := req.URL.Query().Get("ddicEntityName")
		d.reads = append(d.reads, table)
		a, ok := d.tables[table]
		if !ok {
			return resp(http.StatusOK, dataPreviewBody([]string{"X"}, nil))
		}
		if a.status != 0 {
			return resp(a.status, "synthetic data preview failure")
		}
		return resp(http.StatusOK, dataPreviewBody(a.columns, a.rows))
	}
	return resp(http.StatusOK, "")
}

func tstcRow(tcode, program, screen, cinfo string) tableAnswer {
	return tableAnswer{columns: []string{"TCODE", "PGMNA", "DYPNO", "CINFO"}, rows: [][]string{{tcode, program, screen, cinfo}}}
}

func tstcpRow(tcode, param string) tableAnswer {
	return tableAnswer{columns: []string{"TCODE", "PARAM"}, rows: [][]string{{tcode, param}}}
}

func TestGetTransaction(t *testing.T) {
	tests := []struct {
		name      string
		doer      *transactionDoer
		tcode     string
		want      Transaction
		wantErr   string
		wantReads string
	}{
		{
			name:      "report transaction, header from VIT",
			doer:      &transactionDoer{vitStatus: 200, vitBody: trantSE38, tables: map[string]tableAnswer{"TSTC": tstcRow("SE38", "RSABAPPROGRAM", "1000", "84")}},
			tcode:     "se38",
			want:      Transaction{Name: "SE38", Description: "ABAP-редактор", Program: "RSABAPPROGRAM", Package: "SEDT", Responsible: "SAP", Screen: "1000", Kind: "report", CINFO: "84"},
			wantReads: "TSTC",
		},
		{
			name:      "dialog transaction",
			doer:      &transactionDoer{vitStatus: 200, vitBody: trantSE38, tables: map[string]tableAnswer{"TSTC": tstcRow("SE93", "SAPLSEUK", "0390", "04")}},
			tcode:     "SE93",
			want:      Transaction{Name: "SE93", Description: "ABAP-редактор", Program: "SAPLSEUK", Package: "SEDT", Responsible: "SAP", Screen: "0390", Kind: "dialog", CINFO: "04"},
			wantReads: "TSTC",
		},
		{
			name: "parameter transaction without a program",
			doer: &transactionDoer{vitStatus: 200, vitBody: trantSE38, tables: map[string]tableAnswer{
				"TSTC":  tstcRow("/ATL/6111_CODE", "", "0000", "02"),
				"TSTCP": tstcpRow("/ATL/6111_CODE", "/*SM30 VIEWNAME=/ATL/V_6111_CODE;UPDATE=X;"),
			}},
			tcode:     "/ATL/6111_CODE",
			want:      Transaction{Name: "/ATL/6111_CODE", Description: "ABAP-редактор", Package: "SEDT", Responsible: "SAP", Kind: "parameter", CINFO: "02", Parameter: "/*SM30 VIEWNAME=/ATL/V_6111_CODE;UPDATE=X;", CalledTransaction: "SM30"},
			wantReads: "TSTC,TSTCP",
		},
		{
			name: "parameter transaction with a program",
			doer: &transactionDoer{vitStatus: 200, vitBody: trantSE38, tables: map[string]tableAnswer{
				"TSTC":  tstcRow("F-01", "SAPMF05A", "0100", "02"),
				"TSTCP": tstcpRow("F-01", "/NFBM1 BKPF-BLART=AB;"),
			}},
			tcode:     "F-01",
			want:      Transaction{Name: "F-01", Description: "ABAP-редактор", Program: "SAPMF05A", Package: "SEDT", Responsible: "SAP", Screen: "0100", Kind: "parameter", CINFO: "02", Parameter: "/NFBM1 BKPF-BLART=AB;", CalledTransaction: "FBM1"},
			wantReads: "TSTC,TSTCP",
		},
		{
			name: "variant transaction",
			doer: &transactionDoer{vitStatus: 200, vitBody: trantSE38, tables: map[string]tableAnswer{
				"TSTC":  tstcRow("CO04N", "", "0000", "02"),
				"TSTCP": tstcpRow("CO04N", "@@COHVOMPRINT CO04"),
			}},
			tcode:     "CO04N",
			want:      Transaction{Name: "CO04N", Description: "ABAP-редактор", Package: "SEDT", Responsible: "SAP", Kind: "variant", CINFO: "02", Parameter: "@@COHVOMPRINT CO04", Variant: "COHVOMPRINT", CalledTransaction: "CO04"},
			wantReads: "TSTC,TSTCP",
		},
		{
			name: "OO transaction",
			doer: &transactionDoer{vitStatus: 200, vitBody: trantSE38, tables: map[string]tableAnswer{
				"TSTC":  tstcRow("/FMP/MP_COMPARE", "", "0000", "08"),
				"TSTCP": tstcpRow("/FMP/MP_COMPARE", `\CLASS=/FMP/CL_MP_COMPARE_CON\METHOD=START_PRICE_COMPARISON`),
			}},
			tcode:     "/FMP/MP_COMPARE",
			want:      Transaction{Name: "/FMP/MP_COMPARE", Description: "ABAP-редактор", Package: "SEDT", Responsible: "SAP", Kind: "oo", CINFO: "08", Parameter: `\CLASS=/FMP/CL_MP_COMPARE_CON\METHOD=START_PRICE_COMPARISON`, Class: "/FMP/CL_MP_COMPARE_CON", Method: "START_PRICE_COMPARISON"},
			wantReads: "TSTC,TSTCP",
		},
		{
			name:      "area menu",
			doer:      &transactionDoer{vitStatus: 200, vitBody: trantSE38, tables: map[string]tableAnswer{"TSTC": tstcRow("AC00", "MENUAC00", "1000", "01")}},
			tcode:     "AC00",
			want:      Transaction{Name: "AC00", Description: "ABAP-редактор", Program: "MENUAC00", Package: "SEDT", Responsible: "SAP", Screen: "1000", Kind: "area_menu", CINFO: "01"},
			wantReads: "TSTC",
		},
		{
			name:      "unknown transaction: VIT answers 200 but TSTC has no row",
			doer:      &transactionDoer{vitStatus: 200, vitBody: trantUnknown},
			tcode:     "ZNO_SUCH_TX",
			wantErr:   "transaction ZNO_SUCH_TX does not exist",
			wantReads: "TSTC",
		},
		{
			name: "no VIT (7.40): header from TSTCT and TADIR",
			doer: &transactionDoer{vitStatus: 404, vitBody: vitNoHandler, tables: map[string]tableAnswer{
				"TSTC":  tstcRow("SE38", "RSABAPPROGRAM", "1000", "84"),
				"TSTCT": {columns: []string{"SPRSL", "TCODE", "TTEXT"}, rows: [][]string{{"D", "SE38", "ABAP Editor (de)"}, {"E", "SE38", "ABAP Editor"}}},
				"TADIR": {columns: []string{"OBJ_NAME", "DEVCLASS", "AUTHOR"}, rows: [][]string{{"SE38", "SEDT", "SAP"}}},
			}},
			tcode:     "SE38",
			want:      Transaction{Name: "SE38", Description: "ABAP Editor", Program: "RSABAPPROGRAM", Package: "SEDT", Responsible: "SAP", Screen: "1000", Kind: "report", CINFO: "84"},
			wantReads: "TSTC,TSTCT,TADIR",
		},
		{
			name: "no VIT, TADIR fails: what could be read, and a note",
			doer: &transactionDoer{vitStatus: 404, vitBody: vitNoHandler, tables: map[string]tableAnswer{
				"TSTC":  tstcRow("SE38", "RSABAPPROGRAM", "1000", "84"),
				"TADIR": {status: 500},
			}},
			tcode:     "SE38",
			want:      Transaction{Name: "SE38", Program: "RSABAPPROGRAM", Screen: "1000", Kind: "report", CINFO: "84", Note: "header not read: TADIR: "},
			wantReads: "TSTC,TSTCT,TADIR",
		},
		{
			name:      "TSTC fails: header with a note",
			doer:      &transactionDoer{vitStatus: 200, vitBody: trantSE38, tables: map[string]tableAnswer{"TSTC": {status: 500}}},
			tcode:     "SE38",
			want:      Transaction{Name: "SE38", Description: "ABAP-редактор", Package: "SEDT", Responsible: "SAP", Note: "what the transaction starts is not known, TSTC: "},
			wantReads: "TSTC",
		},
		{
			name:      "TSTC fails and no header: error",
			doer:      &transactionDoer{vitStatus: 404, vitBody: vitNoHandler, tables: map[string]tableAnswer{"TSTC": {status: 500}}},
			tcode:     "SE38",
			wantErr:   "getting transaction SE38: TSTC:",
			wantReads: "TSTC",
		},
		{
			name:      "TSTC fails and VIT knows nothing: error",
			doer:      &transactionDoer{vitStatus: 200, vitBody: trantUnknown, tables: map[string]tableAnswer{"TSTC": {status: 500}}},
			tcode:     "ZNO_SUCH_TX",
			wantErr:   "getting transaction ZNO_SUCH_TX: TSTC:",
			wantReads: "TSTC",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := NewConfig("https://sap.example.com", "u", "p")
			client := NewClientWithTransport(cfg, NewTransportWithClient(cfg, tt.doer))
			got, err := client.GetTransaction(context.Background(), tt.tcode)
			if reads := strings.Join(tt.doer.reads, ","); reads != tt.wantReads {
				t.Errorf("tables read = %s, want %s", reads, tt.wantReads)
			}
			if tt.doer.vitAccept != "application/vnd.sap.adt.basic.object.properties+xml" {
				t.Errorf("VIT Accept = %q", tt.doer.vitAccept)
			}
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("err = %v, want it to contain %q", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("err = %v", err)
			}
			// Notes carry the underlying error; compare only the lead.
			if tt.want.Note != "" {
				if !strings.HasPrefix(got.Note, tt.want.Note) {
					t.Errorf("Note = %q, want prefix %q", got.Note, tt.want.Note)
				}
				got.Note = tt.want.Note
			}
			if *got != tt.want {
				t.Errorf("got  %+v\nwant %+v", *got, tt.want)
			}
		})
	}
}

func TestGetTransaction_Path(t *testing.T) {
	d := &transactionDoer{vitStatus: 200, vitBody: trantSE38, tables: map[string]tableAnswer{"TSTC": tstcRow("/FMP/MP_COMPARE", "", "0000", "08")}}
	cfg := NewConfig("https://sap.example.com", "u", "p")
	client := NewClientWithTransport(cfg, NewTransportWithClient(cfg, d))
	if _, err := client.GetTransaction(context.Background(), "/fmp/mp_compare"); err != nil {
		t.Fatal(err)
	}
	if want := "/sap/bc/adt/vit/wb/object_type/TRANT/object_name/%2FFMP%2FMP_COMPARE"; d.vitPath != want {
		t.Errorf("path = %s, want %s", d.vitPath, want)
	}
}
