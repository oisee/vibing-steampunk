package mcp

import (
	"context"
	"fmt"

	"github.com/oisee/vibing-steampunk/pkg/adt"
)

// withObjectLock runs fn against a lock handle for objectURL.
//
// A lock handle is bound to the ADT session that issued it. When the handle is
// minted by one MCP tool call and consumed by a later one, everything the model
// does in between — a search, a read, a grep — is a stateless request that
// retires that session, and the write then fails with 423 InvalidLockHandle.
// That window cannot be closed from inside the process, because the process is
// not what holds it open: the model is (issue #169).
//
// So the handle stops being something a caller has to carry. When supplied is
// empty this acquires a lock, runs fn, and releases it inside the same call,
// which is the shape every high-level workflow in pkg/adt already uses. A
// caller that still passes a handle keeps the old behaviour, because breaking
// them buys nothing.
func (s *Server) withObjectLock(ctx context.Context, objectURL, supplied string, fn func(lockHandle string) error) error {
	if supplied != "" {
		return fn(supplied)
	}

	lock, err := s.adtClient.LockObject(ctx, objectURL, "MODIFY")
	if err != nil {
		return fmt.Errorf("locking %s: %w", objectURL, err)
	}

	released := false
	defer func() {
		if released {
			return
		}
		// The compensating release is the only place a stranded ENQUEUE can be
		// noticed, so its failure is reported rather than dropped (#166).
		if unlockErr := s.adtClient.UnlockObject(ctx, objectURL, lock.LockHandle); unlockErr != nil {
			fmt.Fprintf(adt.LogOutput, "[WARN] releasing the lock on %s failed: %v\n", objectURL, unlockErr)
		}
	}()

	if err := fn(lock.LockHandle); err != nil {
		return err
	}

	released = true
	return s.adtClient.UnlockObject(ctx, objectURL, lock.LockHandle)
}

// withObjectLockConsumed is withObjectLock for an operation that consumes the
// handle rather than returning it — DELETE being the one that matters. There is
// nothing to release on success, and an UNLOCK sent anyway would fail against
// an object that no longer exists. On failure the lock is still ours, so it is
// released.
func (s *Server) withObjectLockConsumed(ctx context.Context, objectURL, supplied string, fn func(lockHandle string) error) error {
	if supplied != "" {
		return fn(supplied)
	}

	lock, err := s.adtClient.LockObject(ctx, objectURL, "MODIFY")
	if err != nil {
		return fmt.Errorf("locking %s: %w", objectURL, err)
	}

	if err := fn(lock.LockHandle); err != nil {
		if unlockErr := s.adtClient.UnlockObject(ctx, objectURL, lock.LockHandle); unlockErr != nil {
			fmt.Fprintf(adt.LogOutput, "[WARN] releasing the lock on %s failed: %v\n", objectURL, unlockErr)
		}
		return err
	}
	return nil
}
