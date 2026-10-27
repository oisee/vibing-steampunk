package mcp

import "testing"

func TestDisabledGroupsParseGroupCodes(t *testing.T) {
	tests := []struct {
		name        string
		input       string
		wantPresent []string
		wantAbsent  []string
		wantDisable []string
		wantKeep    []string
	}{
		{
			name:        "multi-character gCTS code takes precedence",
			input:       "GC",
			wantPresent: []string{"GitTypes", "ListTransports"},
			wantDisable: []string{"GctsListRepositories", "GctsGetRepository"},
			wantKeep:    []string{"GitTypes", "ListTransports"},
		},
		{
			name:        "separated Git and CTS codes",
			input:       "G,C",
			wantAbsent:  []string{"GitTypes", "ListTransports"},
			wantDisable: []string{"GitTypes", "ListTransports", "CreateTransport", "GitExport"},
			wantKeep:    []string{"GctsListRepositories"},
		},
		{
			name:       "whitespace-separated codes",
			input:      "G C",
			wantAbsent: []string{"GitTypes", "ListTransports"},
		},
		{
			name:        "legacy packed single-character codes",
			input:       "5THD",
			wantDisable: []string{"UI5ListApps", "RunUnitTests", "AMDPDebuggerStart", "DebuggerListen"},
			wantAbsent: []string{
				"UI5ListApps", "RunUnitTests", "AMDPDebuggerStart", "DebuggerListen",
			},
		},
		{
			name:        "legacy UI5 alias",
			input:       "5/U",
			wantAbsent:  []string{"UI5ListApps", "UI5GetApp"},
			wantDisable: []string{"UI5ListApps", "UI5GetApp"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			disabled := disabledToolSet(tt.input)
			for _, name := range tt.wantDisable {
				if !disabled[name] {
					t.Errorf("DisabledGroups=%q did not select group containing %s", tt.input, name)
				}
			}
			for _, name := range tt.wantKeep {
				if disabled[name] {
					t.Errorf("DisabledGroups=%q unexpectedly selected group containing %s", tt.input, name)
				}
			}

			server := NewServer(&Config{
				BaseURL:        "https://example.invalid",
				Mode:           "expert",
				DisabledGroups: tt.input,
			})
			registered := make(map[string]bool)
			for _, name := range server.RegisteredTools() {
				registered[name] = true
			}
			for _, name := range tt.wantPresent {
				if !registered[name] {
					t.Errorf("DisabledGroups=%q hid %s; registered tools: %v", tt.input, name, server.RegisteredTools())
				}
			}
			for _, name := range tt.wantAbsent {
				if registered[name] {
					t.Errorf("DisabledGroups=%q left %s registered", tt.input, name)
				}
			}
		})
	}
}
