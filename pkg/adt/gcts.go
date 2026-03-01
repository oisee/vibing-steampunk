package adt

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
)

// --- gCTS Types ---

// GctsRepository represents a gCTS repository.
type GctsRepository struct {
	Rid     string       `json:"rid"`
	Name    string       `json:"name"`
	URL     string       `json:"url"`
	Branch  string       `json:"branch"`
	Package string       `json:"currentPackage,omitempty"`
	Status  string       `json:"status"`
	Role    string       `json:"role"`
	Config  []GctsConfig `json:"config,omitempty"`
}

// GctsConfig represents a configuration entry for a gCTS repository.
type GctsConfig struct {
	Key   string `json:"key"`
	Value string `json:"value"`
}

// GctsBranch represents a branch in a gCTS repository.
type GctsBranch struct {
	Name     string `json:"name"`
	Type     string `json:"type"`
	IsActive bool   `json:"isActive"`
}

// GctsCommitEntry represents a commit in the gCTS history.
type GctsCommitEntry struct {
	ID      string `json:"id"`
	Message string `json:"message"`
	Author  string `json:"author"`
	Date    string `json:"date"`
}

// GctsCreateOptions holds options for creating a gCTS repository.
type GctsCreateOptions struct {
	Rid     string `json:"rid"`
	Name    string `json:"name"`
	URL     string `json:"url"`
	Branch  string `json:"branch,omitempty"`
	Package string `json:"vsid,omitempty"`
	Role    string `json:"role,omitempty"`
}

// GctsCommitOptions holds options for committing in a gCTS repository.
type GctsCommitOptions struct {
	Message string             `json:"message"`
	Objects []GctsCommitObject `json:"objects,omitempty"`
}

// GctsCommitObject represents an object to include in a gCTS commit.
type GctsCommitObject struct {
	Name    string `json:"name"`
	Package string `json:"package"`
	Type    string `json:"type"`
}

// GctsPullResult represents the result of a gCTS pull operation.
type GctsPullResult struct {
	FromCommit string `json:"fromCommit"`
	ToCommit   string `json:"toCommit"`
	Log        []struct {
		Severity string `json:"severity"`
		Message  string `json:"message"`
	} `json:"log,omitempty"`
}

// GctsCommitResult represents the result of a gCTS commit operation.
type GctsCommitResult struct {
	CommitID string `json:"id"`
	Message  string `json:"message"`
}

// gCTS JSON response wrappers
type gctsResultWrapper[T any] struct {
	Result   T              `json:"result"`
	ErrorLog []gctsErrorLog `json:"errorLog,omitempty"`
}

type gctsErrorLog struct {
	Severity string `json:"severity"`
	Message  string `json:"message"`
}

// --- gCTS Methods ---

const gctsBasePath = "/sap/bc/cts_abapvcs"

// GctsListRepositories lists all gCTS repositories.
func (c *Client) GctsListRepositories(ctx context.Context) ([]GctsRepository, error) {
	if err := c.checkSafety(OpTransport, "GctsListRepositories"); err != nil {
		return nil, err
	}

	resp, err := c.transport.Request(ctx, gctsBasePath+"/repository", &RequestOptions{
		Method: http.MethodGet,
		Accept: "application/json",
	})
	if err != nil {
		return nil, fmt.Errorf("gcts list repositories: %w", err)
	}

	var wrapper gctsResultWrapper[[]GctsRepository]
	if err := json.Unmarshal(resp.Body, &wrapper); err != nil {
		return nil, fmt.Errorf("gcts list repositories: parse response: %w", err)
	}

	if len(wrapper.ErrorLog) > 0 {
		return nil, gctsErrorFromLog(wrapper.ErrorLog)
	}

	return wrapper.Result, nil
}

// GctsGetRepository gets details of a specific gCTS repository.
func (c *Client) GctsGetRepository(ctx context.Context, rid string) (*GctsRepository, error) {
	if err := c.checkSafety(OpTransport, "GctsGetRepository"); err != nil {
		return nil, err
	}

	resp, err := c.transport.Request(ctx, fmt.Sprintf("%s/repository/%s", gctsBasePath, rid), &RequestOptions{
		Method: http.MethodGet,
		Accept: "application/json",
	})
	if err != nil {
		return nil, fmt.Errorf("gcts get repository: %w", err)
	}

	var wrapper gctsResultWrapper[GctsRepository]
	if err := json.Unmarshal(resp.Body, &wrapper); err != nil {
		return nil, fmt.Errorf("gcts get repository: parse response: %w", err)
	}

	if len(wrapper.ErrorLog) > 0 {
		return nil, gctsErrorFromLog(wrapper.ErrorLog)
	}

	return &wrapper.Result, nil
}

// GctsCreateRepository creates a new gCTS repository.
func (c *Client) GctsCreateRepository(ctx context.Context, opts GctsCreateOptions) (*GctsRepository, error) {
	if err := c.checkSafety(OpTransport, "GctsCreateRepository"); err != nil {
		return nil, err
	}

	body, err := json.Marshal(opts)
	if err != nil {
		return nil, fmt.Errorf("gcts create repository: marshal options: %w", err)
	}

	resp, err := c.transport.Request(ctx, gctsBasePath+"/repository", &RequestOptions{
		Method:      http.MethodPost,
		Body:        body,
		ContentType: "application/json",
		Accept:      "application/json",
	})
	if err != nil {
		return nil, fmt.Errorf("gcts create repository: %w", err)
	}

	var wrapper gctsResultWrapper[GctsRepository]
	if err := json.Unmarshal(resp.Body, &wrapper); err != nil {
		return nil, fmt.Errorf("gcts create repository: parse response: %w", err)
	}

	if len(wrapper.ErrorLog) > 0 {
		return nil, gctsErrorFromLog(wrapper.ErrorLog)
	}

	return &wrapper.Result, nil
}

// GctsDeleteRepository deletes a gCTS repository.
func (c *Client) GctsDeleteRepository(ctx context.Context, rid string) error {
	if err := c.checkSafety(OpTransport, "GctsDeleteRepository"); err != nil {
		return err
	}

	resp, err := c.transport.Request(ctx, fmt.Sprintf("%s/repository/%s", gctsBasePath, rid), &RequestOptions{
		Method: http.MethodDelete,
		Accept: "application/json",
	})
	if err != nil {
		return fmt.Errorf("gcts delete repository: %w", err)
	}

	// Check for error in response body
	if len(resp.Body) > 0 {
		var wrapper gctsResultWrapper[json.RawMessage]
		if err := json.Unmarshal(resp.Body, &wrapper); err == nil && len(wrapper.ErrorLog) > 0 {
			return gctsErrorFromLog(wrapper.ErrorLog)
		}
	}

	return nil
}

// GctsCloneRepository clones a gCTS repository on the SAP system.
func (c *Client) GctsCloneRepository(ctx context.Context, rid string) error {
	if err := c.checkSafety(OpTransport, "GctsCloneRepository"); err != nil {
		return err
	}

	resp, err := c.transport.Request(ctx, fmt.Sprintf("%s/repository/%s/clone", gctsBasePath, rid), &RequestOptions{
		Method: http.MethodPost,
		Accept: "application/json",
	})
	if err != nil {
		return fmt.Errorf("gcts clone repository: %w", err)
	}

	if len(resp.Body) > 0 {
		var wrapper gctsResultWrapper[json.RawMessage]
		if err := json.Unmarshal(resp.Body, &wrapper); err == nil && len(wrapper.ErrorLog) > 0 {
			return gctsErrorFromLog(wrapper.ErrorLog)
		}
	}

	return nil
}

// GctsPull pulls a specific commit into a gCTS repository.
func (c *Client) GctsPull(ctx context.Context, rid, commitID string) (*GctsPullResult, error) {
	if err := c.checkSafety(OpTransport, "GctsPull"); err != nil {
		return nil, err
	}

	query := url.Values{}
	if commitID != "" {
		query.Set("request", commitID)
	}

	resp, err := c.transport.Request(ctx, fmt.Sprintf("%s/repository/%s/pullByCommit", gctsBasePath, rid), &RequestOptions{
		Method: http.MethodGet,
		Query:  query,
		Accept: "application/json",
	})
	if err != nil {
		return nil, fmt.Errorf("gcts pull: %w", err)
	}

	var wrapper gctsResultWrapper[GctsPullResult]
	if err := json.Unmarshal(resp.Body, &wrapper); err != nil {
		return nil, fmt.Errorf("gcts pull: parse response: %w", err)
	}

	if len(wrapper.ErrorLog) > 0 {
		return nil, gctsErrorFromLog(wrapper.ErrorLog)
	}

	return &wrapper.Result, nil
}

// GctsCommit creates a commit in a gCTS repository.
func (c *Client) GctsCommit(ctx context.Context, rid string, opts GctsCommitOptions) (*GctsCommitResult, error) {
	if err := c.checkSafety(OpTransport, "GctsCommit"); err != nil {
		return nil, err
	}

	body, err := json.Marshal(opts)
	if err != nil {
		return nil, fmt.Errorf("gcts commit: marshal options: %w", err)
	}

	resp, err := c.transport.Request(ctx, fmt.Sprintf("%s/repository/%s/commit", gctsBasePath, rid), &RequestOptions{
		Method:      http.MethodPost,
		Body:        body,
		ContentType: "application/json",
		Accept:      "application/json",
	})
	if err != nil {
		return nil, fmt.Errorf("gcts commit: %w", err)
	}

	var wrapper gctsResultWrapper[GctsCommitResult]
	if err := json.Unmarshal(resp.Body, &wrapper); err != nil {
		return nil, fmt.Errorf("gcts commit: parse response: %w", err)
	}

	if len(wrapper.ErrorLog) > 0 {
		return nil, gctsErrorFromLog(wrapper.ErrorLog)
	}

	return &wrapper.Result, nil
}

// GctsListBranches lists branches in a gCTS repository.
func (c *Client) GctsListBranches(ctx context.Context, rid string) ([]GctsBranch, error) {
	if err := c.checkSafety(OpTransport, "GctsListBranches"); err != nil {
		return nil, err
	}

	resp, err := c.transport.Request(ctx, fmt.Sprintf("%s/repository/%s/branches", gctsBasePath, rid), &RequestOptions{
		Method: http.MethodGet,
		Accept: "application/json",
	})
	if err != nil {
		return nil, fmt.Errorf("gcts list branches: %w", err)
	}

	var wrapper gctsResultWrapper[[]GctsBranch]
	if err := json.Unmarshal(resp.Body, &wrapper); err != nil {
		return nil, fmt.Errorf("gcts list branches: parse response: %w", err)
	}

	if len(wrapper.ErrorLog) > 0 {
		return nil, gctsErrorFromLog(wrapper.ErrorLog)
	}

	return wrapper.Result, nil
}

// GctsSwitchBranch switches the active branch of a gCTS repository.
func (c *Client) GctsSwitchBranch(ctx context.Context, rid, branch string) error {
	if err := c.checkSafety(OpTransport, "GctsSwitchBranch"); err != nil {
		return err
	}

	resp, err := c.transport.Request(ctx, fmt.Sprintf("%s/repository/%s/switchBranch", gctsBasePath, rid), &RequestOptions{
		Method: http.MethodGet,
		Query:  url.Values{"branch": {branch}},
		Accept: "application/json",
	})
	if err != nil {
		return fmt.Errorf("gcts switch branch: %w", err)
	}

	if len(resp.Body) > 0 {
		var wrapper gctsResultWrapper[json.RawMessage]
		if err := json.Unmarshal(resp.Body, &wrapper); err == nil && len(wrapper.ErrorLog) > 0 {
			return gctsErrorFromLog(wrapper.ErrorLog)
		}
	}

	return nil
}

// GctsGetHistory retrieves the commit history of a gCTS repository.
func (c *Client) GctsGetHistory(ctx context.Context, rid string) ([]GctsCommitEntry, error) {
	if err := c.checkSafety(OpTransport, "GctsGetHistory"); err != nil {
		return nil, err
	}

	resp, err := c.transport.Request(ctx, fmt.Sprintf("%s/repository/%s/getHistory", gctsBasePath, rid), &RequestOptions{
		Method: http.MethodGet,
		Accept: "application/json",
	})
	if err != nil {
		return nil, fmt.Errorf("gcts get history: %w", err)
	}

	var wrapper gctsResultWrapper[[]GctsCommitEntry]
	if err := json.Unmarshal(resp.Body, &wrapper); err != nil {
		return nil, fmt.Errorf("gcts get history: parse response: %w", err)
	}

	if len(wrapper.ErrorLog) > 0 {
		return nil, gctsErrorFromLog(wrapper.ErrorLog)
	}

	return wrapper.Result, nil
}

// gctsErrorFromLog converts a gCTS error log into an error.
func gctsErrorFromLog(logs []gctsErrorLog) error {
	if len(logs) == 0 {
		return nil
	}
	msg := logs[0].Message
	for _, l := range logs[1:] {
		msg += "; " + l.Message
	}
	return fmt.Errorf("gCTS error: %s", msg)
}
