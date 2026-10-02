package adt

import (
	"sync"
	"time"
)

// lockWindowMaxAge bounds how long a recorded lock can suppress the keep-alive
// ping. Nothing here can prove a lock was released — a delete consumes the
// handle, a failed unlock leaves it held by a session that may already be gone,
// and a caller can drop a handle on the floor. So an entry that is never closed
// expires instead of suppressing the ping forever. Thirty minutes sits under
// SAP's own ADT session timeout (typically ~60), which is the point past which
// a lock this client is still tracking cannot be alive anyway.
const lockWindowMaxAge = 30 * time.Minute

// lockWindow records the lock handles this client believes are outstanding.
//
// Its only consumer is the keep-alive ticker. A keep-alive ping is a request
// like any other, and under the stateless default (config.go SessionType) an
// unflagged request carries an explicit X-sap-adt-sessiontype: stateless that
// retires the ADT session — including the session a lock handle is bound to.
// A ping that lands between LOCK and the write therefore kills the handle and
// the write returns 423, which is issue #168: a keep-alive that kills the
// session it exists to keep alive.
//
// Suppressing a tick is free. Sending one at the wrong moment costs a write.
// So this errs toward suppression: it clears an entry only on a *successful*
// unlock or delete, and lets anything else age out.
type lockWindow struct {
	mu   sync.Mutex
	open map[string]time.Time
}

// noteLockOpened records a handle as outstanding. Called after a LOCK that
// returned a usable handle.
func (c *Client) noteLockOpened(handle string) {
	if handle == "" {
		return
	}
	c.locks.mu.Lock()
	defer c.locks.mu.Unlock()
	if c.locks.open == nil {
		c.locks.open = make(map[string]time.Time)
	}
	c.locks.open[handle] = time.Now()
}

// noteLockClosed forgets a handle. Called after a successful UNLOCK, and after
// a successful DELETE — a delete consumes the handle without unlocking it,
// which is the case that leaves a naive counter permanently non-zero.
func (c *Client) noteLockClosed(handle string) {
	if handle == "" {
		return
	}
	c.locks.mu.Lock()
	defer c.locks.mu.Unlock()
	delete(c.locks.open, handle)
}

// lockOutstanding reports whether this client is inside a lock window, pruning
// entries old enough that the lock they name cannot still be alive.
func (c *Client) lockOutstanding() bool {
	return c.locks.outstanding()
}

// outstanding reports a lock opened within lockWindowMaxAge -- the keep-alive
// policy, which may err toward pinging after a while.
func (w *lockWindow) outstanding() bool {
	w.mu.Lock()
	defer w.mu.Unlock()
	cutoff := time.Now().Add(-lockWindowMaxAge)
	for _, opened := range w.open {
		if !opened.Before(cutoff) {
			return true
		}
	}
	return false
}

// lockRecordMaxAge is how long an entry nobody closed is kept for present.
// Stateful traffic keeps an SAP context alive past its idle timeout, so the
// transport cannot use the keep-alive's thirty minutes; a working day can.
const lockRecordMaxAge = 12 * time.Hour

// present reports any lock this client has not seen released, whatever its
// age short of lockRecordMaxAge. The transport's isolation uses it: a
// stateless request that carried the context id would end a context whose
// handle may still be valid, and that costs a write, not a ping.
func (w *lockWindow) present() bool {
	w.mu.Lock()
	defer w.mu.Unlock()
	cutoff := time.Now().Add(-lockRecordMaxAge)
	for handle, opened := range w.open {
		if opened.Before(cutoff) {
			delete(w.open, handle)
		}
	}
	return len(w.open) > 0
}
