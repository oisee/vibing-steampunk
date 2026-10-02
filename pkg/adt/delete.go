package adt

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
)

// --- Delete Object Operations ---

// DeleteObject deletes an ABAP object.
// objectURL is the ADT URL of the object (e.g., "/sap/bc/adt/programs/programs/ZTEST")
// lockHandle is required (from LockObject)
// transport is optional (for transportable objects)
func (c *Client) DeleteObject(ctx context.Context, objectURL string, lockHandle string, transport string) error {
	// Unified mutation policy gate (op type + package + transport)
	if err := c.checkMutation(ctx, MutationContext{
		Op:         OpDelete,
		OpName:     "DeleteObject",
		ObjectURL:  objectURL,
		Transport:  transport,
		LockHandle: lockHandle,
	}); err != nil {
		return err
	}

	params := url.Values{}
	params.Set("lockHandle", lockHandle)
	if transport != "" {
		params.Set("corrNr", transport)
	}

	_, err := c.transport.Request(ctx, objectURL, &RequestOptions{
		Method:   http.MethodDelete,
		Query:    params,
		Stateful: true, // Lock handles are session-specific — must match the session that acquired the lock (issue #88)
	})
	if err != nil {
		return fmt.Errorf("deleting object: %w", err)
	}

	// A delete consumes the handle without an UNLOCK ever being sent, which is
	// how a lock-window counter ends up permanently non-zero.
	c.noteLockClosed(lockHandle)

	// The DELETE consumed the handle, but the ENQUEUE the LOCK took lives on
	// with the stateful context. Behind a session-holding proxy that context
	// outlives the chain, and SM12 keeps showing a lock on an object that no
	// longer exists. Retire the context the way UnlockObject does.
	c.transport.ReleaseProxyContext(ctx)

	return nil
}

// DeleteObjectGated deletes an object in one call, taking and releasing its
// own lock: DeleteObject's gate (read-only, operation, package whitelist,
// transportable edit) before the LOCK, then LOCK, DELETE and UNLOCK, with the
// UNLOCK sent after a successful DELETE too, because the DELETE does not
// release the ENQUEUE the LOCK took (see deleteGated). note is non-empty
// when the object was deleted but that UNLOCK failed.
func (c *Client) DeleteObjectGated(ctx context.Context, objectURL, transport string) (note string, err error) {
	note, _, err = c.deleteGated(ctx, objectURL, transport)
	return note, err
}
