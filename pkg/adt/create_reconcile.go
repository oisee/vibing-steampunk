package adt

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"
)

// tryCleanupOrphanLock attempts to clear an orphan lock left behind by a failed creation.
// SAP ADT sometimes creates ENQUEUE locks during object creation that aren't released on failure.
// This function tries to acquire and immediately release such locks.
func (c *Client) tryCleanupOrphanLock(ctx context.Context, objectURL string) {
	// Try to acquire the lock - this may succeed if it's our own orphan lock
	lock, err := c.LockObject(ctx, objectURL, "MODIFY")
	if err != nil {
		// Lock acquisition failed - lock might be held by another user or doesn't exist
		return
	}
	// Successfully acquired - release it immediately, detached from ctx:
	// this runs on the path of a create that failed, often because a call
	// budget ran out.
	_ = c.releaseLockAfterFailure(ctx, objectURL, lock.LockHandle)
}

// isLockConflictError checks if an error is a lock conflict (HTTP 403 "is currently editing")
func isLockConflictError(err error) bool {
	if err == nil {
		return false
	}
	errStr := err.Error()
	return strings.Contains(errStr, "403") && strings.Contains(errStr, "currently editing")
}

// PartialCreateError is returned by CreateObject when the SAP backend
// failed mid-flight but had already persisted the new object before the
// HTTP failure surfaced. CleanupOK is true when the best-effort
// compensation that follows the failure (lock recovery, delete) brought
// SAP back to a clean state; when false, ManualSteps lists the residual
// recovery actions the operator must run by hand.
//
// The original transport error is wrapped via Unwrap so callers using
// errors.Is / errors.As keep working unchanged. Error() leads with the
// partial-create class so log scrapers can distinguish this case from a
// plain pre-persistence failure.
type PartialCreateError struct {
	ObjectURL      string
	Package        string
	Transport      string
	OriginalErr    error
	CleanupActions []string
	CleanupOK      bool
	ManualSteps    []string
	// LeftInPlace means no cleanup was attempted: the object exists, but
	// the caller could not show it was the one that created it.
	LeftInPlace bool
}

func (e *PartialCreateError) Error() string {
	status := "cleanup attempted"
	switch {
	case e.CleanupOK:
		status = "cleanup ok"
	case e.LeftInPlace:
		status = "object left in place, not deleted"
	}
	if e.LeftInPlace {
		// The object exists, but nothing shows this request created it.
		return fmt.Sprintf("create failed and its outcome is unknown: the object exists, but it may not be this request's (%s): %s [object=%s package=%s transport=%s]",
			status, e.OriginalErr, e.ObjectURL, e.Package, e.Transport)
	}
	return fmt.Sprintf("create failed after partial persistence (%s): %s [object=%s package=%s transport=%s]",
		status, e.OriginalErr, e.ObjectURL, e.Package, e.Transport)
}

func (e *PartialCreateError) Unwrap() error { return e.OriginalErr }

// objectExistsByURL probes whether an ADT object URL points at an
// object SAP currently knows about. Used by reconcileFailedCreate to
// disambiguate "request failed and SAP has nothing" from "request failed
// but SAP already created the object". A 200 means yes, 404 means no,
// any other outcome (5xx, network, auth) is treated as inconclusive and
// returned as an error so the caller does not falsely classify a partial
// create as clean.
func (c *Client) objectExistsByURL(ctx context.Context, objectURL string) (bool, error) {
	if objectURL == "" {
		return false, fmt.Errorf("empty object URL")
	}
	_, err := c.transport.Request(ctx, objectURL, &RequestOptions{
		Method: http.MethodGet,
		Accept: "application/*",
	})
	if err == nil {
		return true, nil
	}
	var apiErr *APIError
	if errors.As(err, &apiErr) {
		if apiErr.StatusCode == http.StatusNotFound {
			return false, nil
		}
	}
	return false, err
}

// reconcileFailedCreate handles the post-failure recovery sequence for
// CreateObject. If the original error came from a request that landed
// before SAP committed anything (404 on probe), it returns the original
// error unchanged. If SAP committed the object before responding (200
// on probe), it runs best-effort compensating cleanup — lock release
// and delete — and wraps the original error in a PartialCreateError
// so the caller sees both the failure and what we did about it.
//
// The cleanup is intentionally best-effort: every step swallows its own
// error and records what it tried in CleanupActions. The original
// create error is never lost — it is always the OriginalErr of the
// returned PartialCreateError. Manual recovery hints are only added
// when our best-effort attempt could not finish.
// partialCreateProbeTimeout bounds the read that checks whether an
// interrupted throwaway create was committed anyway.
const partialCreateProbeTimeout = 5 * time.Second

func (c *Client) reconcileFailedCreate(ctx context.Context, opts CreateObjectOptions, createErr error) error {
	// An already-exists response proves the object predates this create attempt.
	// It is not partial persistence owned by this request, so reconciliation
	// must never lock or delete it.
	if isAlreadyExistsError(createErr) {
		return createErr
	}

	objectURL := GetObjectURL(opts.ObjectType, opts.Name, opts.ParentName)
	if objectURL == "" {
		// Object type we cannot URL-encode → no probe possible.
		return createErr
	}

	probeCtx := ctx
	if opts.leavePartialObject {
		// A throwaway object's create is most often cut short by the
		// caller's own cancellation (Ctrl-C), after SAP may already have
		// committed it. The probe is a read, so it may outlive that
		// cancellation; with the caller's context it never leaves the
		// process, and a program left behind would go unreported.
		//
		// It is bounded well below the cleanup timeout: it runs after the
		// caller's own deadline, and a call given a time budget should not
		// overrun it by much just to say what it left behind.
		detached, cancelDetached := failureCleanupContext(ctx)
		defer cancelDetached()
		var cancel context.CancelFunc
		probeCtx, cancel = context.WithTimeout(detached, partialCreateProbeTimeout)
		defer cancel()
	}
	exists, probeErr := c.objectExistsByURL(probeCtx, objectURL)
	if probeErr != nil {
		if opts.leavePartialObject {
			return fmt.Errorf("%w (whether %s was created anyway could not be checked: %v)", createErr, opts.Name, probeErr)
		}
		// Probe inconclusive (5xx, network, auth). Returning the
		// original error keeps the existing failure semantics so we do
		// not regress callers who already handle plain create errors.
		return createErr
	}
	if !exists {
		if opts.leavePartialObject && ctx.Err() != nil {
			// Seen live: SAP goes on with a create whose client hung up,
			// and can commit it after this probe came back 404.
			return fmt.Errorf("%w (the create was interrupted and SAP may still complete it: look for %s in %s)", createErr, opts.Name, opts.PackageName)
		}
		// SAP did not persist anything — original error is final.
		return createErr
	}

	if opts.leavePartialObject {
		return &PartialCreateError{
			ObjectURL:   objectURL,
			Package:     opts.PackageName,
			Transport:   opts.Transport,
			OriginalErr: createErr,
			LeftInPlace: true,
			CleanupActions: []string{
				"not deleted: the create failed, yet an object of this name exists, and nothing shows this call created it",
			},
			ManualSteps: []string{
				fmt.Sprintf("look at %s (created by, created on); if it is an empty program this call left behind, delete it in SE80 or with vsp", opts.Name),
			},
		}
	}

	pce := c.cleanupPartialObject(ctx, objectURL, opts.PackageName, opts.Transport)
	pce.OriginalErr = createErr
	return pce
}

func isAlreadyExistsError(err error) bool {
	if err == nil {
		return false
	}
	var apiErr *APIError
	if !errors.As(err, &apiErr) || (apiErr.StatusCode != http.StatusBadRequest && apiErr.StatusCode != http.StatusConflict) {
		return false
	}
	message := strings.ToLower(apiErr.Message)
	return strings.Contains(message, "exceptionresourcealreadyexists") ||
		strings.Contains(message, "already exists") ||
		strings.Contains(message, "does already exist")
}

// cleanupPartialObject runs the best-effort compensating cleanup for a
// zombie object: orphan-lock release, then acquire a fresh session-
// scoped lock, delete, and release that lock. The result is a
// *PartialCreateError that records what was attempted and whether the
// object is now gone (CleanupOK); a lock it could not release is named in
// ManualSteps, with the stranded-lock advice, even when the object is gone.
// Callers that invoke it from an explicit recovery path — e.g. the
// MCP `recover_failed_create` tool — leave OriginalErr nil; callers
// that invoke it after a failed CreateObject attach the original
// transport error on top.
//
// Factored out of reconcileFailedCreate so the same cleanup sequence
// serves both the automatic post-failure path and the explicit
// operator-driven recovery path.
func (c *Client) cleanupPartialObject(ctx context.Context, objectURL, pkg, transport string) *PartialCreateError {
	pce := &PartialCreateError{
		ObjectURL: objectURL,
		Package:   pkg,
		Transport: transport,
	}

	// Step 0: run DeleteObject's gate here, before any lock. Inside the lock
	// window its package lookup is a stateless request that retires the
	// session the handle belongs to, and the DELETE comes back 423
	// (issue #238). Gating first also means an object outside the allowlist
	// is refused before it is ever locked, rather than locked and then refused.
	ctx, gateErr := c.PrepareDelete(ctx, objectURL, transport)
	if gateErr != nil {
		pce.CleanupActions = append(pce.CleanupActions,
			fmt.Sprintf("delete refused by the mutation gate: %v", gateErr))
		pce.ManualSteps = []string{
			"check that the object's package is covered by --allowed-packages",
			"otherwise delete the object manually via SE80",
		}
		return pce
	}

	// Step 1: orphan lock cleanup (cheap; reuses the existing helper).
	c.tryCleanupOrphanLock(ctx, objectURL)
	pce.CleanupActions = append(pce.CleanupActions, "tried orphan-lock cleanup")

	// Step 2: acquire a fresh lock owned by us, then delete the
	// half-created object. If we cannot acquire a lock the cleanup
	// stops here and we surface manual recovery steps; we never try
	// to delete without a lock because that would 403 anyway.
	lock, lockErr := c.LockObject(ctx, objectURL, "MODIFY", transport)
	if lockErr != nil {
		pce.CleanupActions = append(pce.CleanupActions,
			fmt.Sprintf("could not acquire lock for delete: %v", lockErr))
		pce.ManualSteps = []string{
			"check SM12 for stale locks owned by your user and release them",
			"if transport-bound, check SE09 for the object and remove it from the transport",
			"manually delete the object via SE80 once locks are clear",
		}
		return pce
	}

	delErr := c.DeleteObject(ctx, objectURL, lock.LockHandle, transport)
	if delErr != nil {
		// Delete failed despite holding a lock — release the lock
		// so we do not add to the leak, then surface manual steps.
		pce.CleanupActions = append(pce.CleanupActions,
			fmt.Sprintf("delete failed: %v", delErr))
		pce.ManualSteps = []string{
			"manually delete the object via SE80",
			"if transport-bound, remove from transport via SE09 first",
		}
		if uerr := c.releaseLockAfterFailure(ctx, objectURL, lock.LockHandle); uerr != nil {
			pce.CleanupActions = append(pce.CleanupActions, "could not release the delete lock")
			pce.ManualSteps = append(pce.ManualSteps, strandedLockAdvice(objectURL, uerr))
		}
		return pce
	}

	pce.CleanupActions = append(pce.CleanupActions, "deleted partially-created object")
	pce.CleanupOK = true

	// The DELETE does not release the ENQUEUE the LOCK took, except behind a
	// session-holding proxy, where DeleteObject retires the stateful context
	// and the lock goes with it (the rule deleteGated follows). Released on a
	// detached, bounded context: cleanup runs after a failure, often one
	// caused by the caller's context running out. The object is gone either
	// way; a lock left behind is reported, not hidden.
	if deleteReleasesLock(c) {
		return pce
	}
	if uerr := c.releaseLockAfterFailure(ctx, objectURL, lock.LockHandle); uerr != nil {
		pce.CleanupActions = append(pce.CleanupActions, "could not release the delete lock")
		pce.ManualSteps = append(pce.ManualSteps, strandedLockAdvice(objectURL, uerr))
		return pce
	}
	pce.CleanupActions = append(pce.CleanupActions, "released the delete lock")
	return pce
}

// RecoverFailedCreate is the operator-facing recovery primitive. It
// lets the caller clean up a zombie object left behind by an earlier
// failed CreateObject without needing a lock handle from the original
// (now-lost) session. The caller passes the object identity — type,
// name, parent for nested FMs, and optionally the transport it was
// attached to — and we probe whether SAP currently thinks the object
// exists:
//
//   - if the object does not exist, we return a PartialCreateError
//     with CleanupOK=true and a "nothing to clean" note. This is the
//     happy idempotent case: re-running recovery on an already-clean
//     object is a no-op.
//   - if the object exists, we run the same best-effort compensating
//     cleanup used by reconcileFailedCreate after a failed create:
//     orphan-lock release, fresh lock acquisition, DeleteObject,
//     structured result with ManualSteps on partial success.
//
// Callers MUST validate object ownership themselves before invoking
// this. The reconcile path is safe because the object was just created
// by the current user; the operator-driven path must not nuke an
// object that another user is legitimately editing. The MCP handler
// enforces this by gating behind SAP_ENABLE_LOCK_ADMIN and by
// refusing to touch objects whose package is not in the allowed list.
func (c *Client) RecoverFailedCreate(ctx context.Context, opts CreateObjectOptions) *PartialCreateError {
	objectURL := GetObjectURL(opts.ObjectType, opts.Name, opts.ParentName)
	if objectURL == "" {
		return &PartialCreateError{
			OriginalErr:    fmt.Errorf("unsupported object type %q for recovery", opts.ObjectType),
			CleanupActions: []string{"could not derive object URL"},
		}
	}

	exists, probeErr := c.objectExistsByURL(ctx, objectURL)
	if probeErr != nil {
		return &PartialCreateError{
			ObjectURL:      objectURL,
			Package:        opts.PackageName,
			Transport:      opts.Transport,
			OriginalErr:    fmt.Errorf("existence probe failed: %w", probeErr),
			CleanupActions: []string{"existence probe inconclusive — aborted without cleanup"},
			ManualSteps: []string{
				"retry the recovery once network / auth is stable",
				"check SM12 and SE09 manually if the retry also fails",
			},
		}
	}
	if !exists {
		// Idempotent no-op: nothing to clean. Return CleanupOK=true
		// with an explanatory note so the operator knows the action
		// was acknowledged and is safe to retry.
		return &PartialCreateError{
			ObjectURL:      objectURL,
			Package:        opts.PackageName,
			Transport:      opts.Transport,
			CleanupActions: []string{"object does not exist — nothing to recover"},
			CleanupOK:      true,
		}
	}

	return c.cleanupPartialObject(ctx, objectURL, opts.PackageName, opts.Transport)
}

// packageExists checks if a package exists in the system.
// Returns true if package exists or if the check is inconclusive (API errors).
// Only returns false when GetPackage succeeds but returns an empty/invalid result.
// Uses GetPackage (nodestructure API) which passes the package name as a query
// parameter, avoiding URL path encoding issues with $ in local package names.
func (c *Client) packageExists(ctx context.Context, packageName string) bool {
	pkg, err := c.GetPackage(ctx, packageName)
	if err != nil {
		// Only SAP's own answer means the package is missing: a 404, or a
		// "not found" in SAP's message. Any other failure (CSRF, auth, network,
		// a call budget running out) is optimistic: the create that follows
		// reports the real error. Both are read from the APIError, never from
		// err.Error(): the URL in it can carry "404" in a port number
		// (127.0.0.1:40457), which once turned a timeout into "package does
		// not exist".
		var apiErr *APIError
		if !errors.As(err, &apiErr) {
			return true
		}
		return apiErr.StatusCode != http.StatusNotFound &&
			!strings.Contains(strings.ToLower(apiErr.Message), "not found")
	}
	// GetPackage succeeded but returned no objects and no sub-packages —
	// still a valid (possibly empty) package
	return pkg != nil
}
