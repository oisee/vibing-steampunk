package adt

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"
)

// packageProbeDoer answers the package resource with a fixed status and
// body, and data preview on TDEVC with the given rows or a failure.
type packageProbeDoer struct {
	status      int
	body        string
	tdevcRows   []string
	tdevcStatus int
	tdevcSQL    string
	tdevcCalls  int
}

func (d *packageProbeDoer) Do(req *http.Request) (*http.Response, error) {
	resp := func(status int, body string) (*http.Response, error) {
		return &http.Response{StatusCode: status, Body: io.NopCloser(strings.NewReader(body)), Header: http.Header{"X-Csrf-Token": []string{"t"}}}, nil
	}
	switch {
	case strings.HasPrefix(req.URL.Path, "/sap/bc/adt/packages/"):
		return resp(d.status, d.body)
	case req.URL.Path == "/sap/bc/adt/datapreview/ddic":
		d.tdevcCalls++
		if req.Body != nil {
			b, _ := io.ReadAll(req.Body)
			d.tdevcSQL = string(b)
		}
		if d.tdevcStatus != 0 {
			return resp(d.tdevcStatus, "synthetic data preview failure")
		}
		rows := make([][]string, len(d.tdevcRows))
		for i, r := range d.tdevcRows {
			rows[i] = []string{r}
		}
		return resp(http.StatusOK, dataPreviewBody([]string{"DEVCLASS"}, rows))
	}
	return resp(http.StatusOK, "")
}

const routerNotFound = "Подходящий ресурс не найден"
const resourceNotFound = `<?xml version="1.0" encoding="utf-8"?><exc:exception xmlns:exc="http://www.sap.com/abapxml/types/communicationframework"><namespace id="com.sap.adt"/><type id="ExceptionResourceNotFound"/></exc:exception>`

func TestPackageExists(t *testing.T) {
	tests := []struct {
		name      string
		doer      *packageProbeDoer
		pkg       string
		want      bool
		wantErr   bool
		wantTDEVC int
	}{
		{name: "resource answers 200", doer: &packageProbeDoer{status: 200}, pkg: "$TMP", want: true},
		{name: "resource says not found", doer: &packageProbeDoer{status: 404, body: resourceNotFound}, pkg: "ZNONE"},
		{name: "no resource, row in TDEVC", doer: &packageProbeDoer{status: 404, body: routerNotFound, tdevcRows: []string{"$TMP"}}, pkg: "$tmp", want: true, wantTDEVC: 1},
		{name: "no resource, no row", doer: &packageProbeDoer{status: 404, body: routerNotFound}, pkg: "ZNONE", wantTDEVC: 1},
		{name: "no resource, TDEVC fails", doer: &packageProbeDoer{status: 404, body: routerNotFound, tdevcStatus: 500}, pkg: "ZNONE", wantErr: true, wantTDEVC: 1},
		{name: "server error", doer: &packageProbeDoer{status: 500, body: "boom"}, pkg: "ZNONE", wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := NewConfig("https://sap.example.com", "u", "p")
			client := NewClientWithTransport(cfg, NewTransportWithClient(cfg, tt.doer))
			got, err := client.PackageExists(context.Background(), tt.pkg)
			if (err != nil) != tt.wantErr {
				t.Fatalf("err = %v, wantErr %t", err, tt.wantErr)
			}
			if got != tt.want {
				t.Fatalf("exists = %t, want %t", got, tt.want)
			}
			if tt.doer.tdevcCalls != tt.wantTDEVC {
				t.Fatalf("TDEVC reads = %d, want %d", tt.doer.tdevcCalls, tt.wantTDEVC)
			}
		})
	}
}

func TestPackageExists_TDEVCQuery(t *testing.T) {
	for pkg, want := range map[string]string{
		"$tmp":    "DEVCLASS = '$TMP'",
		"/ns/pkg": "DEVCLASS = '/NS/PKG'",
		"Z'X":     "DEVCLASS = 'Z''X'",
	} {
		d := &packageProbeDoer{status: 404, body: routerNotFound}
		cfg := NewConfig("https://sap.example.com", "u", "p")
		client := NewClientWithTransport(cfg, NewTransportWithClient(cfg, d))
		if _, err := client.PackageExists(context.Background(), pkg); err != nil {
			t.Fatalf("%s: %v", pkg, err)
		}
		if !strings.Contains(d.tdevcSQL, want) {
			t.Errorf("%s: SQL %q, want %q", pkg, d.tdevcSQL, want)
		}
	}
}
