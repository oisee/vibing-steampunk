package adt

import (
	"context"
	"errors"
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
	_, err := c.deleteObject(ctx, objectURL, lockHandle, transport)
	return err
}

// deleteObject is DeleteObject that also says whether the lock went with the
// DELETE: lockGone is true only behind a session-holding proxy whose context
// retirement was confirmed. Everywhere else the ENQUEUE outlives the DELETE
// and the caller must UNLOCK (releaseLockAfterDelete).
func (c *Client) deleteObject(ctx context.Context, objectURL string, lockHandle string, transport string) (lockGone bool, err error) {
	// Unified mutation policy gate (op type + package + transport)
	if err := c.checkMutation(ctx, MutationContext{
		Op:         OpDelete,
		OpName:     "DeleteObject",
		ObjectURL:  objectURL,
		Transport:  transport,
		LockHandle: lockHandle,
	}); err != nil {
		return false, err
	}

	params := url.Values{}
	params.Set("lockHandle", lockHandle)
	if transport != "" {
		params.Set("corrNr", transport)
	}

	if _, err := c.transport.Request(ctx, objectURL, &RequestOptions{
		Method:   http.MethodDelete,
		Query:    params,
		Stateful: true, // Lock handles are session-specific — must match the session that acquired the lock (issue #88)
	}); err != nil {
		return false, fmt.Errorf("deleting object: %w", err)
	}

	// A delete consumes the handle without an UNLOCK ever being sent, which is
	// how a lock-window counter ends up permanently non-zero.
	c.noteLockClosed(lockHandle)

	// The DELETE consumed the handle, but the ENQUEUE the LOCK took lives on
	// with the stateful context. Behind a session-holding proxy that context
	// outlives the chain, and SM12 keeps showing a lock on an object that no
	// longer exists. Retire the context the way UnlockObject does.
	if c.transport.config == nil || !c.transport.config.ProxyContextIDGuard {
		return false, nil
	}
	return c.transport.retireProxyContext(ctx) == nil, nil
}

// releaseLockAfterDelete releases the lock a successful deleteObject left,
// on a detached, bounded context, and returns an error only for a lock that
// may still be held. lockGone (the proxy context was retired, and the
// ENQUEUE with it) means there is nothing to release.
//
// Behind the proxy, a retirement that was not confirmed may still have
// happened; the UNLOCK then lands in a context that is gone or holds no
// lock, and SAP's "session gone" or "invalid lock handle" answer means just
// that, not a stranded lock.
func (c *Client) releaseLockAfterDelete(ctx context.Context, objectURL, lockHandle string, lockGone bool) error {
	if lockGone {
		return nil
	}
	err := c.releaseLockAfterFailure(ctx, objectURL, lockHandle)
	if err == nil {
		return nil
	}
	var apiErr *APIError
	if c.transport.config != nil && c.transport.config.ProxyContextIDGuard &&
		errors.As(err, &apiErr) && (apiErr.IsSessionExpired() || isInvalidLockHandle(apiErr)) {
		return nil
	}
	return err
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
