package vsp

import (
	"testing"

	"github.com/oisee/vibing-steampunk/internal/mcp"
	"github.com/oisee/vibing-steampunk/pkg/config"
)

// The MCP server takes cts_project and transport_target from the default
// system in .vsp.json, as the README says, unless a flag or the environment
// set them first -- the rule transport_attribute already follows.
func TestApplyDefaultSystemSettings_FilesRequestsUnderTheConfiguredProject(t *testing.T) {
	t.Setenv("VSP_DEV_TRANSPORT_ATTRIBUTE", "")
	t.Setenv("VSP_TRANSPORT_ATTRIBUTE", "")
	systems := &config.SystemsConfig{
		Default: "dev",
		Systems: map[string]config.SystemConfig{
			"dev": {URL: "https://dev.example:44300", CTSProject: "PROJ_A", TransportTarget: "QAS.100", TransportAttribute: "SAP_CTS_PROJECT"},
		},
	}

	c := &mcp.Config{}
	applyDefaultSystemSettings(c, systems)
	if c.CTSProject != "PROJ_A" || c.TransportTarget != "QAS.100" || c.TransportAttribute != "SAP_CTS_PROJECT" {
		t.Errorf("from .vsp.json: got project %q, target %q, attribute %q", c.CTSProject, c.TransportTarget, c.TransportAttribute)
	}

	c = &mcp.Config{CTSProject: "FROM_FLAG", TransportTarget: "PRD.100", TransportAttribute: "ZATTR"}
	applyDefaultSystemSettings(c, systems)
	if c.CTSProject != "FROM_FLAG" || c.TransportTarget != "PRD.100" || c.TransportAttribute != "ZATTR" {
		t.Errorf("a flag or env value must win: got project %q, target %q, attribute %q", c.CTSProject, c.TransportTarget, c.TransportAttribute)
	}

	c = &mcp.Config{}
	applyDefaultSystemSettings(c, &config.SystemsConfig{Systems: systems.Systems})
	if c.CTSProject != "" || c.TransportTarget != "" {
		t.Errorf("no default system, nothing to take: got project %q, target %q", c.CTSProject, c.TransportTarget)
	}
}
