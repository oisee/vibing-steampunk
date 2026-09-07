package adt

import (
	"context"
	"net/http"
	"net/url"
)

// RawRequest sends one ADT request as given — method, path, query, headers,
// body — through the client's transport, with its session, CSRF token and
// cache, and returns the response as it came. It is the door for a resource
// this package has no method for yet: exploration, and the odd one-off.
func (c *Client) RawRequest(ctx context.Context, method, path string, query url.Values, accept, contentType string, body []byte, stateful bool) (*Response, error) {
	if method != http.MethodGet && method != http.MethodHead {
		if err := c.checkSafety(OpUpdate, "RawRequest "+method); err != nil {
			return nil, err
		}
	}
	return c.transport.Request(ctx, path, &RequestOptions{
		Method:      method,
		Query:       query,
		Accept:      accept,
		ContentType: contentType,
		Body:        body,
		Stateful:    stateful,
	})
}
