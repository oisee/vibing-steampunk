package adt

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"
)

// unlockAfterFailureTimeout bounds the compensating UNLOCK. It runs on a
// context detached from the caller's, so it needs a deadline of its own.
const unlockAfterFailureTimeout = 30 * time.Second

// releaseLockAfterFailure releases a lock taken for a mutation that has since
// failed.
//
// It differs from UnlockObject in one way that matters: it runs on a context
// detached from the caller's cancellation. Every compensating unlock in this
// package used to reuse the enclosing ctx, so when the mutation failed
// *because* the context was cancelled or hit its deadline — an MCP client
// timeout, a Ctrl-C, an HTTP deadline — the unlock never left the process. It
// failed at http.NewRequestWithContext before a byte was sent, and the ENQUEUE
// it was meant to release stayed on the object. A timeout inside a lock window
// was a guaranteed leak.
//
// The returned error is the caller's only evidence that an object was left
// locked; do not discard it. strandedLockAdvice turns it into something a user
// can act on.
func (c *Client) releaseLockAfterFailure(ctx context.Context, objectURL, lockHandle string) error {
	if lockHandle == "" {
		return nil
	}

	releaseCtx, cancel := failureCleanupContext(ctx)
	defer cancel()

	return c.UnlockObject(releaseCtx, objectURL, lockHandle)
}

// heldLock is a lock a write workflow holds from its LOCK to its own UNLOCK
// before activation. The workflow defers releaseOnReturn right after the LOCK
// and calls unlock where it means to release; any return in between, the
// ones caused by its context running out included, releases the lock on a
// detached, bounded context and says so when that fails too.
//
// The deferred release used to be c.UnlockObject(ctx, ...) on the workflow's
// own ctx, with its error dropped. Under a call budget that is the common
// failure: the PUT outlasts the budget, ctx is done, and the UNLOCK never
// leaves the process — the ENQUEUE stays on the object and nobody is told.
type heldLock struct {
	c         *Client
	objectURL string
	handle    string
	released  bool
	// mayBeReleased is set when an UNLOCK left the process and no answer came
	// back: SAP may have released the lock, so a later "invalid lock handle"
	// is that release, not a refusal.
	mayBeReleased bool
}

func (c *Client) holdLock(objectURL, handle string) *heldLock {
	return &heldLock{c: c, objectURL: objectURL, handle: handle}
}

// unlock is the workflow's own UNLOCK. When it fails the lock counts as still
// held, so releaseOnReturn tries again on a context of its own.
func (h *heldLock) unlock(ctx context.Context) error {
	sent := ctx.Err() == nil
	if err := h.c.UnlockObject(ctx, h.objectURL, h.handle); err != nil {
		h.noteOutcome(sent, err)
		return err
	}
	h.released = true
	return nil
}

// noteOutcome records an UNLOCK that failed without an answer from SAP: one
// sent (its context still live) that ended in a transport error or a timeout
// waiting for the response. An answer from SAP (an APIError) is definite.
func (h *heldLock) noteOutcome(sent bool, err error) {
	var apiErr *APIError
	if sent && !errors.As(err, &apiErr) {
		h.mayBeReleased = true
	}
}

// release releases the lock if the workflow did not, on a detached, bounded
// context, and returns the advice to give when that fails, or "".
//
// Only a definite refusal for a lock known to be held is "left LOCKED". After
// an UNLOCK whose answer was lost, SAP answering the retry with "invalid lock
// handle" means the first one worked; and a retry that gets no answer either
// leaves the lock in doubt, which is what the advice then says.
func (h *heldLock) release(ctx context.Context) string {
	if h.released {
		return ""
	}
	h.released = true
	releaseCtx, cancel := failureCleanupContext(ctx)
	defer cancel()
	err := h.c.UnlockObject(releaseCtx, h.objectURL, h.handle)
	if err == nil {
		return ""
	}
	h.noteOutcome(true, err)
	var apiErr *APIError
	if !errors.As(err, &apiErr) {
		return uncertainLockAdvice(h.objectURL, err)
	}
	if h.mayBeReleased {
		if isInvalidLockHandle(apiErr) {
			return ""
		}
		return uncertainLockAdvice(h.objectURL, err)
	}
	return strandedLockAdvice(h.objectURL, err)
}

// releaseOnReturn is release for a deferred call: it appends the advice, if
// any, to *message.
func (h *heldLock) releaseOnReturn(ctx context.Context, message *string) {
	if advice := h.release(ctx); advice != "" && message != nil {
		*message += " — " + advice
	}
}

// isInvalidLockHandle reports whether SAP refused an UNLOCK because it holds
// no lock under the handle: ExceptionResourceInvalidLockHandle and its kin,
// on a 400, 404 or 423.
func isInvalidLockHandle(e *APIError) bool {
	switch e.StatusCode {
	case http.StatusBadRequest, http.StatusNotFound, http.StatusLocked:
	default:
		return false
	}
	msg := strings.ToLower(e.Message)
	return strings.Contains(msg, "invalidlockhandle") ||
		strings.Contains(msg, "invalid lock handle") ||
		strings.Contains(msg, "not locked")
}

// uncertainLockAdvice is strandedLockAdvice for a lock whose release could
// not be confirmed either way.
func uncertainLockAdvice(objectURL string, err error) string {
	object := strings.TrimPrefix(objectURL, "/sap/bc/adt/")
	return fmt.Sprintf(
		"%s may still be locked: the release could not be confirmed (%v). "+
			"Check SM12 for an entry on your user and the object, and delete it if it is there.",
		object, err)
}

// failureCleanupContext keeps best-effort cleanup independent from the failed
// operation's cancellation while still bounding how long that cleanup may run.
// It deliberately keeps the caller's values, including mutation-policy marks.
func failureCleanupContext(ctx context.Context) (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.WithoutCancel(ctx), unlockAfterFailureTimeout)
}

// CleanupContext is failureCleanupContext for callers outside this package:
// a context for a compensating request (an UNLOCK) that still goes out when
// ctx has been cancelled or has run out, bounded by a deadline of its own. It
// keeps ctx's values, including mutation-policy marks.
func CleanupContext(ctx context.Context) (context.Context, context.CancelFunc) {
	return failureCleanupContext(ctx)
}

// strandedLockAdvice explains an unlock that failed, in the terms a user needs
// to act on it.
//
// The failure mode this exists for is issue #91: the mutation returned 423
// because the stateful ADT session that issued the lock handle was retired, and
// UNLOCK cannot work either, because `_action=UNLOCK` addresses the ENQUEUE
// only through that handle and that session. The lock is real, it belongs to
// the user's own session, no vsp command can clear it, and the next edit will
// fail with SAP's "is currently editing" message naming the user themselves —
// which reads exactly like a colleague holding the object. Every one of those
// facts has to be in the message or the user is left where the issue reporters
// were: at SM12, guessing.
func strandedLockAdvice(objectURL string, unlockErr error) string {
	object := strings.TrimPrefix(objectURL, "/sap/bc/adt/")

	return fmt.Sprintf(
		"%s was left LOCKED: releasing the lock failed (%v). "+
			"The lock is held by your own user, not a colleague. "+
			"If the write failed with 423/invalid lock handle, vsp cannot release it — "+
			"UNLOCK needs both the handle and the ADT session that issued it, and that session is gone. "+
			"It clears by itself when SAP reaps the abandoned session (ADT session timeout, typically ~60 min), "+
			"or immediately in SM12: filter on your user and the object, and delete the entry. "+
			"Until then the next edit will fail with \"is currently editing\" naming you.",
		object, unlockErr)
}
