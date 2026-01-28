// ABOUTME: Debug listener API for ABAP debugger.
// ABOUTME: Manages waiting for debuggees that hit breakpoints.

package adt

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// --- Debug Listener API ---

// DebuggerListen starts a debug listener that waits for a debuggee to hit a breakpoint.
// This is a BLOCKING call that uses long-polling. It will return when:
// - A debuggee is caught (returns Debuggee info)
// - Timeout occurs (returns TimedOut=true)
// - A conflict is detected (returns Conflict info)
// - Context is cancelled
//
// Default timeout is 240 seconds. For longer waits, call this in a loop.
func (c *Client) DebuggerListen(ctx context.Context, opts *ListenOptions) (*ListenResult, error) {
	if opts == nil {
		opts = &ListenOptions{}
	}

	// Apply defaults
	opts = applyListenDefaults(opts)

	query := url.Values{}
	query.Set("debuggingMode", string(opts.DebuggingMode))
	query.Set("terminalId", opts.TerminalID)
	query.Set("ideId", opts.IdeID)
	query.Set("timeout", fmt.Sprintf("%d", opts.TimeoutSeconds))

	if opts.User != "" {
		query.Set("requestUser", opts.User)
	}
	if opts.CheckConflict {
		query.Set("checkConflict", "true")
	}
	if opts.NotifyOnConflict {
		query.Set("isNotifiedOnConflict", "true")
	}

	// Long-polling request - use extended timeout via context
	httpTimeout := time.Duration(opts.TimeoutSeconds+30) * time.Second
	listenCtx, cancel := context.WithTimeout(ctx, httpTimeout)
	defer cancel()

	resp, err := c.transport.Request(listenCtx, "/sap/bc/adt/debugger/listeners", &RequestOptions{
		Method: http.MethodPost,
		Accept: "application/vnd.sap.as+xml",
		Query:  query,
	})
	if err != nil {
		// Check for conflict error
		if strings.Contains(err.Error(), "conflict") {
			return &ListenResult{
				Conflict: &ListenerConflict{
					ConflictText: err.Error(),
				},
			}, nil
		}
		return nil, fmt.Errorf("debugger listen failed: %w", err)
	}

	// Empty response = timeout
	if len(resp.Body) == 0 {
		return &ListenResult{TimedOut: true}, nil
	}

	// Parse debuggee response
	debuggee, err := parseDebuggeeResponse(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("parsing debuggee response: %w", err)
	}

	return &ListenResult{Debuggee: debuggee}, nil
}

// DebuggerCheckListener checks if there are active debug listeners.
// Returns nil if no listeners are active.
func (c *Client) DebuggerCheckListener(ctx context.Context, opts *ListenOptions) (*ListenerConflict, error) {
	if opts == nil {
		opts = &ListenOptions{}
	}

	// Apply defaults (except timeout which isn't used for check)
	opts = applyListenDefaults(opts)

	query := url.Values{}
	query.Set("debuggingMode", string(opts.DebuggingMode))
	query.Set("terminalId", opts.TerminalID)
	query.Set("ideId", opts.IdeID)
	query.Set("checkConflict", "true")

	if opts.User != "" {
		query.Set("requestUser", opts.User)
	}

	_, err := c.transport.Request(ctx, "/sap/bc/adt/debugger/listeners", &RequestOptions{
		Method: http.MethodGet,
		Accept: "application/xml",
		Query:  query,
	})
	if err != nil {
		// 404 = no listeners active
		if strings.Contains(err.Error(), "404") {
			return nil, nil
		}
		// Conflict detected
		if strings.Contains(err.Error(), "conflict") || strings.Contains(err.Error(), "409") {
			return &ListenerConflict{ConflictText: err.Error()}, nil
		}
		return nil, fmt.Errorf("check listener failed: %w", err)
	}

	return nil, nil
}

// DebuggerStopListener stops an active debug listener.
func (c *Client) DebuggerStopListener(ctx context.Context, opts *ListenOptions) error {
	if opts == nil {
		opts = &ListenOptions{}
	}

	// Apply defaults
	opts = applyListenDefaults(opts)

	query := url.Values{}
	query.Set("debuggingMode", string(opts.DebuggingMode))
	query.Set("terminalId", opts.TerminalID)
	query.Set("ideId", opts.IdeID)

	if opts.User != "" {
		query.Set("requestUser", opts.User)
	}

	_, err := c.transport.Request(ctx, "/sap/bc/adt/debugger/listeners", &RequestOptions{
		Method: http.MethodDelete,
		Query:  query,
	})
	if err != nil {
		return fmt.Errorf("stop listener failed: %w", err)
	}

	return nil
}

// applyListenDefaults applies default values to ListenOptions.
func applyListenDefaults(opts *ListenOptions) *ListenOptions {
	result := *opts // Copy to avoid mutating original
	if result.DebuggingMode == "" {
		result.DebuggingMode = DebuggingModeUser
	}
	if result.IdeID == "" {
		result.IdeID = "vsp"
	}
	if result.TerminalID == "" {
		result.TerminalID = getTerminalID()
	}
	if result.TimeoutSeconds == 0 {
		result.TimeoutSeconds = 240
	}
	return &result
}
