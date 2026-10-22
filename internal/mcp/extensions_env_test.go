package mcp

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mark3labs/mcp-go/mcp"

	"github.com/oisee/vibing-steampunk/pkg/adt"
	"github.com/oisee/vibing-steampunk/pkg/mcpext"
)

// envProbe is an extension that hands its Env to the test and keeps track of
// its lifecycle.
type envProbe struct {
	staticExtension
	version string
	started int
	closed  int
	env     mcpext.Env
}

func (p *envProbe) Version() string { return p.version }

func (p *envProbe) Start(_ context.Context, env mcpext.Env) error {
	p.started++
	p.env = env
	return nil
}

func (p *envProbe) Close(context.Context) error {
	p.closed++
	return errors.New("closing failed on purpose")
}

func newEnvProbe() *envProbe {
	return &envProbe{staticExtension: staticExtension{name: "envprobe", actions: []mcpext.Action{{
		Action: "read", Type: "PROBEX", Class: mcpext.Read, Op: adt.OpRead,
		Handler: func(context.Context, mcpext.Env, string, map[string]any) (*mcp.CallToolResult, error) {
			return mcp.NewToolResultText("ok"), nil
		}}}}, version: "1.2.3"}
}

// withSystemsFile puts a .vsp.json naming the server's own system "own" in a
// fresh working directory, with the given extension settings.
func withSystemsFile(t *testing.T, url, settings string) {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("HOME", t.TempDir())
	t.Setenv("USERPROFILE", t.TempDir())
	t.Chdir(dir)
	conf := fmt.Sprintf(`{"systems": {"own": {"url": %q, "client": "001", "extensions": %s}}}`, url, settings)
	if err := os.WriteFile(filepath.Join(dir, ".vsp.json"), []byte(conf), 0o600); err != nil {
		t.Fatal(err)
	}
}

func TestExtensionEnvKnowsItsSystemAndSettings(t *testing.T) {
	probe := newEnvProbe()
	sap := newFakeSAP(t)
	withSystemsFile(t, sap.srv.URL, `{"envprobe": {"allow_import": true, "level": "full"}, "other": {"allow_import": "no"}}`)
	s := NewServer(&Config{BaseURL: sap.srv.URL, Username: "u", Password: "p", Client: "001", Language: "EN",
		Mode: "hyperfocused", Extensions: []mcpext.Extension{probe}})
	if err := s.StartExtensions(context.Background()); err != nil {
		t.Fatal(err)
	}
	if probe.started != 1 || probe.env == nil {
		t.Fatalf("Start was called %d times", probe.started)
	}
	sys := probe.env.System()
	if sys.Name != "own" || sys.Client != "001" || sys.User != "u" || sys.Language != "EN" {
		t.Errorf("System() = %+v", sys)
	}
	if v, ok := probe.env.Setting("allow_import"); !ok || v != true {
		t.Errorf("Setting(allow_import) = %v, %t; want the probe's own true, not another extension's", v, ok)
	}
	if _, ok := probe.env.Setting("missing"); ok {
		t.Error("a setting that is not there was reported")
	}
	if err := s.CloseExtensions(context.Background()); err == nil || !strings.Contains(err.Error(), "envprobe") || probe.closed != 1 {
		t.Errorf("CloseExtensions: %v (closed %d times)", err, probe.closed)
	}
}

func TestExtensionAsyncTaskIsReportedByGetAsyncResult(t *testing.T) {
	probe := newEnvProbe()
	s := newExtensionServer(t, false, probe)
	if err := s.StartExtensions(context.Background()); err != nil {
		t.Fatal(err)
	}
	release := make(chan struct{})
	id, err := probe.env.StartAsync("probe_job", func(context.Context) (any, error) {
		<-release
		return map[string]string{"imported": "TR-1"}, nil
	})
	if err != nil || !strings.HasPrefix(id, "probe_job_") {
		t.Fatalf("StartAsync = %q, %v", id, err)
	}
	running := resultText(mustCall(t, s.handleGetAsyncResult, map[string]any{"task_id": id}))
	if !strings.Contains(running, "running") {
		t.Errorf("before it finished: %s", running)
	}
	close(release)
	done := resultText(mustCall(t, s.handleGetAsyncResult, map[string]any{"task_id": id, "wait": true}))
	if !strings.Contains(done, "completed") || !strings.Contains(done, "TR-1") {
		t.Errorf("after it finished: %s", done)
	}

	failing, _ := probe.env.StartAsync("probe_job", func(context.Context) (any, error) { panic("boom") })
	failed := resultText(mustCall(t, s.handleGetAsyncResult, map[string]any{"task_id": failing, "wait": true}))
	if !strings.Contains(failed, "error") || !strings.Contains(failed, "boom") {
		t.Errorf("a panicking task: %s", failed)
	}

	// A failed task keeps the result it returned with its error.
	partial, _ := probe.env.StartAsync("probe_job", func(context.Context) (any, error) {
		return map[string]string{"steps": "tp rc 8"}, errors.New("import failed")
	})
	got := resultText(mustCall(t, s.handleGetAsyncResult, map[string]any{"task_id": partial, "wait_seconds": float64(5)}))
	if !strings.Contains(got, "import failed") || !strings.Contains(got, "tp rc 8") {
		t.Errorf("a failed task with a result: %s", got)
	}
}

// A wait that ends before the task is an answer, not an error: the task, still
// running.
func TestGetAsyncResultWaitEndsWithTheRunningTask(t *testing.T) {
	probe := newEnvProbe()
	s := newExtensionServer(t, false, probe)
	if err := s.StartExtensions(context.Background()); err != nil {
		t.Fatal(err)
	}
	release := make(chan struct{})
	defer close(release)
	id, _ := probe.env.StartAsync("probe_job", func(context.Context) (any, error) { <-release; return nil, nil })
	res := mustCall(t, s.handleGetAsyncResult, map[string]any{"task_id": id, "wait_seconds": float64(1)})
	if res.IsError || !strings.Contains(resultText(res), "running") {
		t.Errorf("after the wait: %q (error %t)", resultText(res), res.IsError)
	}
}

func TestExtensionVersionInInfo(t *testing.T) {
	s := newExtensionServer(t, false, newEnvProbe())
	if got := s.extensionsInfoLine(); !strings.Contains(got, "envprobe 1.2.3") {
		t.Errorf("info line %q", got)
	}
	if got := newExtensionServer(t, false).extensionsInfoLine(); got != "" {
		t.Errorf("a server without extensions has an info line: %q", got)
	}
}

func mustCall(t *testing.T, h func(context.Context, mcp.CallToolRequest) (*mcp.CallToolResult, error), args map[string]any) *mcp.CallToolResult {
	t.Helper()
	res, err := h(context.Background(), newRequest(args))
	if err != nil {
		t.Fatal(err)
	}
	return res
}
