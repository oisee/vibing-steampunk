package adt

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"
	"sync"
)

// SourceHash is the stable fingerprint used to protect a source update from
// overwriting a version read by another user or agent. ADT commonly changes
// CRLF/LF and trailing blank lines when materialising source, so those
// transport-only differences are deliberately not part of the fingerprint.
func SourceHash(source string) string {
	canonical := strings.ReplaceAll(source, "\r\n", "\n")
	canonical = strings.TrimRight(canonical, "\n")
	sum := sha256.Sum256([]byte(canonical))
	return "sha256:" + hex.EncodeToString(sum[:])
}

// SourceDriftError means the source changed after the caller read it. No
// write was attempted; callers should re-read, reconcile the change, and try
// again with the new source hash.
type SourceDriftError struct {
	ExpectedSourceHash string
	ActualSourceHash   string
}

func (e *SourceDriftError) Error() string {
	return fmt.Sprintf("SOURCE_DRIFT: expected source hash %s, but the locked object is now %s", e.ExpectedSourceHash, e.ActualSourceHash)
}

type sourceHashExpectation struct {
	expected string
	mu       sync.Mutex
	used     bool
}

type sourceHashExpectationKey struct{}

func withExpectedSourceHash(ctx context.Context, expected string) context.Context {
	expected = strings.TrimSpace(expected)
	if expected == "" {
		return ctx
	}
	return context.WithValue(ctx, sourceHashExpectationKey{}, &sourceHashExpectation{expected: expected})
}

// verifyExpectedSourceHash is called only after a MODIFY lock has been
// acquired. The expectation is consumed by the first source write in a
// workflow so a class's optional test-include update cannot compare its own
// source with the main object's hash.
func (c *Client) verifyExpectedSourceHash(ctx context.Context, sourceURL string) error {
	expectation, _ := ctx.Value(sourceHashExpectationKey{}).(*sourceHashExpectation)
	if expectation == nil {
		return nil
	}

	expectation.mu.Lock()
	defer expectation.mu.Unlock()
	if expectation.used {
		return nil
	}

	resp, err := c.transport.Request(ctx, sourceURL, &RequestOptions{
		Method:   "GET",
		Accept:   "text/plain",
		Stateful: true,
	})
	if err != nil {
		return fmt.Errorf("reading locked source for version check: %w", err)
	}

	actual := SourceHash(string(resp.Body))
	if actual != expectation.expected {
		return &SourceDriftError{
			ExpectedSourceHash: expectation.expected,
			ActualSourceHash:   actual,
		}
	}
	expectation.used = true
	return nil
}
