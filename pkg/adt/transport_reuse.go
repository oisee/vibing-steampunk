package adt

// resolveWriteTransport determines which transport request a write should bind to.
//
// When the caller supplied a transport, it is used unchanged. Otherwise, if the
// object is already captured in an open transport request of the current user, SAP
// returns that request in the LOCK response (LockResult.CorrNr). Reusing it lets an
// already-captured object be edited without the spurious
//
//	409 ExceptionResourceLockConflict
//	"Object ... is already locked in request <id> ... cannot be edited under a new request"
//
// that vsp otherwise triggers by binding the write to a fresh request. ADT / Eclipse
// reuse the lock's request automatically; this brings vsp in line (issue #144).
//
// The fallback re-runs the transportable-edit policy against the resolved request, so
// auto-reuse can never bypass the --allow-transportable-edits gate or the
// --allowed-transports whitelist: if a request is discovered but transportable edits
// are not permitted, this returns the same error the caller would have gotten had they
// passed the transport explicitly.
func (c *Client) resolveWriteTransport(supplied, lockCorrNr, opName string) (string, error) {
	if supplied != "" {
		return supplied, nil
	}
	if lockCorrNr == "" {
		return "", nil
	}
	if err := c.checkTransportableEdit(lockCorrNr, opName); err != nil {
		return "", err
	}
	return lockCorrNr, nil
}
