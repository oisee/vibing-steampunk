// ABOUTME: Delete operations for ABAP objects.
// ABOUTME: Provides DeleteObject for removing ABAP repository objects.

package adt

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
)

// DeleteObject deletes an ABAP object.
// objectURL is the ADT URL of the object (e.g., "/sap/bc/adt/programs/programs/ZTEST")
// lockHandle is required (from LockObject)
// transport is optional (for transportable objects)
func (c *Client) DeleteObject(ctx context.Context, objectURL string, lockHandle string, transport string) error {
	if err := c.checkSafety(OpDelete, "DeleteObject"); err != nil {
		return err
	}

	params := url.Values{}
	params.Set("lockHandle", lockHandle)
	if transport != "" {
		params.Set("corrNr", transport)
	}

	_, err := c.transport.Request(ctx, objectURL, &RequestOptions{
		Method: http.MethodDelete,
		Query:  params,
	})
	if err != nil {
		return fmt.Errorf("deleting object: %w", err)
	}

	return nil
}
