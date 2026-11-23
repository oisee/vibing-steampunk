package adt

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"
)

// optionsHTTP answers every OPTIONS request with status, as SAP does on
// systems that do not implement OPTIONS for ADT collections.
type optionsHTTP struct{ status int }

func (m *optionsHTTP) Do(req *http.Request) (*http.Response, error) {
	h := http.Header{}
	h.Set("X-CSRF-Token", "TOKEN")
	status, body := http.StatusOK, ""
	if req.Method == http.MethodOptions {
		status, body = m.status, "HTTP&#x20;method&#x20;OPTIONS&#x20;not&#x20;supported"
	}
	return &http.Response{StatusCode: status, Body: io.NopCloser(strings.NewReader(body)), Header: h}, nil
}

// S/4HANA 2023 answers OPTIONS on /ddic/ddl/sources, /filestore/ui5-bsp and
// /cts/transports with 400 "HTTP method OPTIONS not supported". The endpoint
// is there — it answered — but the probe read anything but 200/405 as absent,
// so RAP, UI5 and CTS were reported unavailable for the life of the process.
// Only a 404 means the endpoint is missing.
func TestFeatureProbe_MethodRejectedMeansEndpointExists(t *testing.T) {
	for _, tc := range []struct {
		status int
		want   bool
	}{
		{http.StatusBadRequest, true},
		{http.StatusMethodNotAllowed, true},
		{http.StatusNotFound, false},
	} {
		cfg := NewConfig("https://sap.example.com:44300", "dev", "secret")
		client := NewClientWithTransport(cfg, NewTransportWithClient(cfg, &optionsHTTP{status: tc.status}))
		p := NewFeatureProber(client, FeatureConfig{}, false)
		for _, id := range []FeatureID{FeatureRAP, FeatureUI5, FeatureTransport} {
			got := p.Probe(context.Background(), id)
			if got.Available != tc.want {
				t.Errorf("OPTIONS %d, %s: available = %v (%s), want %v", tc.status, id, got.Available, got.Message, tc.want)
			}
		}
	}
}
