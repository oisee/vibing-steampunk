package vsp

import (
	"bytes"
	"strings"
	"testing"

	"github.com/oisee/vibing-steampunk/internal/mcp"
	"github.com/oisee/vibing-steampunk/pkg/config"
)

// The server warns at startup when -s / SAP_SYSTEM names an entry for another
// system than SAP_URL / SAP_CLIENT. A matching or URL-less entry is silent.
func TestWarnNamedSystemMismatch(t *testing.T) {
	systems := &config.SystemsConfig{Systems: map[string]config.SystemConfig{
		"prodsys-a": {URL: "https://prodsys-a.example:44300", Client: "100"},
		"gw":        {RFCHost: "gw.example.local"},
	}}
	cases := map[string]struct {
		name, url, client string
		warn              bool
	}{
		"other URL":    {"prodsys-a", "https://dev.example.local:44300", "100", true},
		"other client": {"prodsys-a", "https://prodsys-a.example:44300", "200", true},
		"same system":  {"prodsys-a", "https://prodsys-a.example:44300/", "100", false},
		"URL-less":     {"gw", "https://dev.example.local:44300", "100", false},
		"no name":      {"", "https://dev.example.local:44300", "100", false},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			var out bytes.Buffer
			warnNamedSystemMismatch(&out, &mcp.Config{SystemName: tc.name, BaseURL: tc.url, Client: tc.client}, systems)
			got := out.String()
			if tc.warn != strings.Contains(got, "[WARNING]") {
				t.Fatalf("warning = %q, want warning %v", got, tc.warn)
			}
			if tc.warn && (!strings.Contains(got, "https://prodsys-a.example:44300 client 100") || !strings.Contains(got, tc.url+" client "+tc.client)) {
				t.Errorf("the warning does not name both systems: %q", got)
			}
		})
	}
}
