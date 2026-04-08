package adt

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"
)

func TestGctsListRepositories(t *testing.T) {
	mock := &mockTransportClient{
		responses: map[string]*http.Response{
			"/sap/bc/cts_abapvcs/repository": newTestResponse(
				`{"result":[{"rid":"repo1","name":"test-repo","url":"https://git.example.com/repo.git","branch":"main","status":"READY","role":"SOURCE"}]}`,
			),
		},
	}

	cfg := NewConfig("https://sap.example.com:44300", "user", "pass")
	cfg.Safety.EnableTransports = true
	transport := NewTransportWithClient(cfg, mock)
	client := NewClientWithTransport(cfg, transport)

	repos, err := client.GctsListRepositories(context.Background())
	if err != nil {
		t.Fatalf("GctsListRepositories failed: %v", err)
	}

	if len(repos) != 1 {
		t.Fatalf("Expected 1 repository, got %d", len(repos))
	}
	if repos[0].Rid != "repo1" {
		t.Errorf("Rid = %v, want repo1", repos[0].Rid)
	}
	if repos[0].Name != "test-repo" {
		t.Errorf("Name = %v, want test-repo", repos[0].Name)
	}
	if repos[0].Status != "READY" {
		t.Errorf("Status = %v, want READY", repos[0].Status)
	}
	if repos[0].Role != "SOURCE" {
		t.Errorf("Role = %v, want SOURCE", repos[0].Role)
	}
}

func TestGctsGetRepository(t *testing.T) {
	mock := &mockTransportClient{
		responses: map[string]*http.Response{
			"/sap/bc/cts_abapvcs/repository/repo1": newTestResponse(
				`{"result":{"rid":"repo1","name":"test-repo","url":"https://git.example.com/repo.git","branch":"main","status":"READY","role":"SOURCE","config":[{"key":"VCS_TARGET_DIR","value":"src/"}]}}`,
			),
		},
	}

	cfg := NewConfig("https://sap.example.com:44300", "user", "pass")
	cfg.Safety.EnableTransports = true
	transport := NewTransportWithClient(cfg, mock)
	client := NewClientWithTransport(cfg, transport)

	repo, err := client.GctsGetRepository(context.Background(), "repo1")
	if err != nil {
		t.Fatalf("GctsGetRepository failed: %v", err)
	}

	if repo.Rid != "repo1" {
		t.Errorf("Rid = %v, want repo1", repo.Rid)
	}
	if len(repo.Config) != 1 {
		t.Fatalf("Expected 1 config entry, got %d", len(repo.Config))
	}
	if repo.Config[0].Key != "VCS_TARGET_DIR" {
		t.Errorf("Config key = %v, want VCS_TARGET_DIR", repo.Config[0].Key)
	}
	if repo.Config[0].Value != "src/" {
		t.Errorf("Config value = %v, want src/", repo.Config[0].Value)
	}
}

func TestGctsListRepositories_ErrorLog(t *testing.T) {
	mock := &mockTransportClient{
		responses: map[string]*http.Response{
			"/sap/bc/cts_abapvcs/repository": newTestResponse(
				`{"result":null,"errorLog":[{"severity":"error","message":"Repository not found"},{"severity":"error","message":"Check configuration"}]}`,
			),
		},
	}

	cfg := NewConfig("https://sap.example.com:44300", "user", "pass")
	cfg.Safety.EnableTransports = true
	transport := NewTransportWithClient(cfg, mock)
	client := NewClientWithTransport(cfg, transport)

	_, err := client.GctsListRepositories(context.Background())
	if err == nil {
		t.Fatal("Expected error from errorLog, got nil")
	}

	if !strings.Contains(err.Error(), "Repository not found") {
		t.Errorf("Error should contain 'Repository not found', got: %v", err)
	}
	if !strings.Contains(err.Error(), "Check configuration") {
		t.Errorf("Error should contain 'Check configuration', got: %v", err)
	}
}

func TestGctsListRepositories_SafetyBlocked(t *testing.T) {
	// Without EnableTransports, all gCTS operations should be blocked
	mock := &mockTransportClient{
		responses: map[string]*http.Response{
			"/sap/bc/cts_abapvcs/repository": newTestResponse(`{"result":[]}`),
		},
	}

	cfg := NewConfig("https://sap.example.com:44300", "user", "pass")
	// EnableTransports is false by default
	transport := NewTransportWithClient(cfg, mock)
	client := NewClientWithTransport(cfg, transport)

	_, err := client.GctsListRepositories(context.Background())
	if err == nil {
		t.Fatal("Expected safety error when EnableTransports is false, got nil")
	}

	if !strings.Contains(err.Error(), "transport") && !strings.Contains(err.Error(), "blocked") &&
		!strings.Contains(err.Error(), "not enabled") && !strings.Contains(err.Error(), "not allowed") {
		t.Logf("Safety error message: %v", err)
	}
}

func TestGctsCreateRepository_SafetyBlocked(t *testing.T) {
	mock := &mockTransportClient{
		responses: map[string]*http.Response{},
	}

	cfg := NewConfig("https://sap.example.com:44300", "user", "pass")
	// EnableTransports is false by default - should block create
	transport := NewTransportWithClient(cfg, mock)
	client := NewClientWithTransport(cfg, transport)

	_, err := client.GctsCreateRepository(context.Background(), GctsCreateOptions{
		Rid:  "new-repo",
		Name: "new-repo",
		URL:  "https://git.example.com/new.git",
	})
	if err == nil {
		t.Fatal("Expected safety error when EnableTransports is false, got nil")
	}
}

func TestGctsDeleteRepository_SafetyBlocked(t *testing.T) {
	mock := &mockTransportClient{
		responses: map[string]*http.Response{},
	}

	cfg := NewConfig("https://sap.example.com:44300", "user", "pass")
	// EnableTransports is false by default - should block delete
	transport := NewTransportWithClient(cfg, mock)
	client := NewClientWithTransport(cfg, transport)

	err := client.GctsDeleteRepository(context.Background(), "repo1")
	if err == nil {
		t.Fatal("Expected safety error when EnableTransports is false, got nil")
	}
}

func TestGctsGetHistory(t *testing.T) {
	mock := &mockTransportClient{
		responses: map[string]*http.Response{
			"getHistory": newTestResponse(
				`{"result":[{"id":"abc123","message":"initial commit","author":"user","date":"2025-01-01"},{"id":"def456","message":"second commit","author":"user","date":"2025-01-02"}]}`,
			),
		},
	}

	cfg := NewConfig("https://sap.example.com:44300", "user", "pass")
	cfg.Safety.EnableTransports = true
	transport := NewTransportWithClient(cfg, mock)
	client := NewClientWithTransport(cfg, transport)

	history, err := client.GctsGetHistory(context.Background(), "repo1")
	if err != nil {
		t.Fatalf("GctsGetHistory failed: %v", err)
	}

	if len(history) != 2 {
		t.Fatalf("Expected 2 commits, got %d", len(history))
	}
	if history[0].ID != "abc123" {
		t.Errorf("Commit ID = %v, want abc123", history[0].ID)
	}
	if history[0].Message != "initial commit" {
		t.Errorf("Message = %v, want 'initial commit'", history[0].Message)
	}
	if history[1].ID != "def456" {
		t.Errorf("Second commit ID = %v, want def456", history[1].ID)
	}
}

func TestGctsListBranches(t *testing.T) {
	mock := &mockTransportClient{
		responses: map[string]*http.Response{
			"branches": newTestResponse(
				`{"result":[{"name":"main","type":"branch","isActive":true},{"name":"develop","type":"branch","isActive":false}]}`,
			),
		},
	}

	cfg := NewConfig("https://sap.example.com:44300", "user", "pass")
	cfg.Safety.EnableTransports = true
	transport := NewTransportWithClient(cfg, mock)
	client := NewClientWithTransport(cfg, transport)

	branches, err := client.GctsListBranches(context.Background(), "repo1")
	if err != nil {
		t.Fatalf("GctsListBranches failed: %v", err)
	}

	if len(branches) != 2 {
		t.Fatalf("Expected 2 branches, got %d", len(branches))
	}
	if branches[0].Name != "main" {
		t.Errorf("Branch name = %v, want main", branches[0].Name)
	}
	if !branches[0].IsActive {
		t.Error("Expected main branch to be active")
	}
	if branches[1].IsActive {
		t.Error("Expected develop branch to be inactive")
	}
}

func TestGctsListRepositories_EmptyResult(t *testing.T) {
	mock := &mockTransportClient{
		responses: map[string]*http.Response{
			"/sap/bc/cts_abapvcs/repository": {
				StatusCode: http.StatusOK,
				Body:       io.NopCloser(strings.NewReader(`{"result":[]}`)),
				Header:     http.Header{"X-CSRF-Token": []string{"test-token"}},
			},
		},
	}

	cfg := NewConfig("https://sap.example.com:44300", "user", "pass")
	cfg.Safety.EnableTransports = true
	transport := NewTransportWithClient(cfg, mock)
	client := NewClientWithTransport(cfg, transport)

	repos, err := client.GctsListRepositories(context.Background())
	if err != nil {
		t.Fatalf("GctsListRepositories failed: %v", err)
	}

	if len(repos) != 0 {
		t.Errorf("Expected 0 repositories, got %d", len(repos))
	}
}
