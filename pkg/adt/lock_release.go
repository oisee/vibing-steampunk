package adt

import (
	"context"
	"fmt"
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

// failureCleanupContext keeps best-effort cleanup independent from the failed
// operation's cancellation while still bounding how long that cleanup may run.
// It deliberately keeps the caller's values, including mutation-policy marks.
func failureCleanupContext(ctx context.Context) (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.WithoutCancel(ctx), unlockAfterFailureTimeout)
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
