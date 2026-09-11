package adt

import (
	"context"
	"fmt"
	"net/http"
	"strings"
)

// RawGet performs an authenticated read-only GET against an arbitrary ADT
// path and returns the status, headers and body untouched.
//
// It exists so that object types vsp does not model yet can be inspected
// against a live system: the ADT discovery document names every collection
// with its accepted content types, and reading one instance of an unmodelled
// type reveals the XML root element and namespace that a create request has
// to send. Both are needed before a new type can be added to the objectTypes
// registry in crud.go, and neither is documented by SAP.
//
// The method is deliberately GET only. It performs no mutation-policy check
// because it cannot mutate: any attempt to pass a different verb is rejected
// rather than forwarded. Writes belong in typed operations that validate
// their payload and handle activation, not in a generic escape hatch.
func (c *Client) RawGet(ctx context.Context, path string, accept string) (*Response, error) {
	if !strings.HasPrefix(path, "/") {
		return nil, fmt.Errorf("path must be absolute and begin with /, got %q", path)
	}
	if strings.Contains(path, "://") {
		return nil, fmt.Errorf("path must be a path on the configured system, not a full URL: %q", path)
	}

	return c.transport.Request(ctx, path, &RequestOptions{
		Method: http.MethodGet,
		Accept: accept,
	})
}
