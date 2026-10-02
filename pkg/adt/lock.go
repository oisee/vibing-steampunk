package adt

import (
	"bytes"
	"context"
	"encoding/xml"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
)

// --- Lock/Unlock Operations ---

// LockResult represents the result of locking an object.
type LockResult struct {
	LockHandle          string `json:"lockHandle"`
	CorrNr              string `json:"corrNr,omitempty"`
	CorrUser            string `json:"corrUser,omitempty"`
	CorrText            string `json:"corrText,omitempty"`
	IsLocal             bool   `json:"isLocal"`
	IsLinkUp            bool   `json:"isLinkUp"`
	ModificationSupport string `json:"modificationSupport,omitempty"`
}

// LockObject acquires an edit lock on an ABAP object.
// objectURL is the ADT URL of the object (e.g., "/sap/bc/adt/programs/programs/ZTEST")
// accessMode is typically "MODIFY" for editing
//
// corrNr is the transport request (or task) the edit goes under. ADT accepts
// it on the LOCK request itself, as the ADT API documents, and on-premise
// systems that bind the lock to a request expect it there rather than only on
// the write that follows. Without it the request is sent exactly as before.
//
// It is variadic so that a call site without a transport stays valid as
// written; only the first value is read.
func (c *Client) LockObject(ctx context.Context, objectURL string, accessMode string, corrNr ...string) (*LockResult, error) {
	transport := ""
	if len(corrNr) > 0 {
		transport = corrNr[0]
	}
	// Safety check - only a READ lock is safe. Every other mode (MODIFY, the
	// empty default which becomes MODIFY, or anything else SAP may accept) is
	// checked as a lock, and refused under --read-only: a write lock serves
	// no write there and only leaves an ENQUEUE entry in SM12. The mode is
	// compared case-insensitively.
	accessMode = strings.ToUpper(strings.TrimSpace(accessMode))
	if accessMode != "READ" {
		if err := c.checkSafety(OpLock, "LockObject"); err != nil {
			return nil, err
		}
		if c.config.Safety.ReadOnly && !c.config.Safety.DryRun {
			mode := accessMode
			if mode == "" {
				mode = "MODIFY"
			}
			return nil, fmt.Errorf("operation 'LockObject' (%s) is blocked: read-only mode enabled (only READ locks are allowed)", mode)
		}
	}

	// The transport goes out on the LOCK, so the transport policy is checked
	// here, before SAP sees it, and not only in the write that follows.
	if err := c.checkTransportableEdit(transport, "LockObject"); err != nil {
		return nil, err
	}

	if accessMode == "" {
		accessMode = "MODIFY"
	}

	params := url.Values{}
	params.Set("_action", "LOCK")
	params.Set("accessMode", accessMode)
	if transport != "" {
		params.Set("corrNr", transport)
	}

	// The window opens when the handle is recorded, after the response is in.
	// Until then, count the LOCK as a stateful request under way, so no
	// stateless request ends the context between the two (see Transport.do).
	if c.transport != nil {
		c.transport.contextInFlight.Add(1)
		defer c.transport.contextInFlight.Add(-1)
	}

	resp, err := c.transport.Request(ctx, objectURL, &RequestOptions{
		Method:   http.MethodPost,
		Query:    params,
		Accept:   "application/vnd.sap.as+xml;charset=UTF-8;dataname=com.sap.adt.lock.result",
		Stateful: true, // Lock handles are session-specific — force stateful (issue #88)
		// Behind a session-holding proxy, every chain starts in its own context:
		// one reused across chains loses the activation worklist entry.
		FreshContext: true,
	})
	if err != nil {
		return nil, fmt.Errorf("locking object: %w", err)
	}

	result, err := parseLockResult(resp.Body)
	if err != nil {
		return nil, err
	}

	// MODIFICATION_SUPPORT="NoModification" alone does NOT mean "you cannot
	// write". SAP returns it for local/customer objects that need no
	// modification recording: verified against A4H, where LOCK on a local
	// global class returns IS_LOCAL=X, MODIFICATION_SUPPORT=NoModification
	// AND a valid LOCK_HANDLE — and the PUT of .../source/main that follows
	// returns 200. Failing on the field alone (the original issue #91 guard)
	// made every local object unwritable, and because the guard returned
	// before unlocking, each attempt leaked the ENQUEUE it had just taken —
	// the object then really was blocked, by our own orphan lock.
	//
	// A LOCK without a handle is the genuinely unusable case: nothing to
	// write with, and nothing to release.
	if accessMode == "MODIFY" && result.LockHandle == "" {
		return nil, fmt.Errorf(
			"object %s is not modifiable via ADT on this system "+
				"(SAP returned a LOCK with no lock handle, modificationSupport=%q). "+
				"Common causes: read-only system class, missing developer/edit role, "+
				"BTP ABAP Environment object outside the customer namespace, "+
				"or hyperfocused mode locking the object as read-only",
			objectURL, result.ModificationSupport)
	}

	c.noteLockOpened(result.LockHandle)

	return result, nil
}

func parseLockResult(data []byte) (*LockResult, error) {
	// Parse the ABAP serialization XML format
	type lockData struct {
		LockHandle string `xml:"LOCK_HANDLE"`
		CorrNr     string `xml:"CORRNR"`
		CorrUser   string `xml:"CORRUSER"`
		CorrText   string `xml:"CORRTEXT"`
		IsLocal    string `xml:"IS_LOCAL"`
		IsLinkUp   string `xml:"IS_LINK_UP"`
		ModSupport string `xml:"MODIFICATION_SUPPORT"`
	}
	type values struct {
		Data lockData `xml:"DATA"`
	}
	type abapResponse struct {
		Values values `xml:"values"`
	}

	// An ADT error comes back as an exception document, not a lock result —
	// e.g. EU510 "User X is currently editing Y" when another session still
	// holds the ENQUEUE. xml.Unmarshal parses that into an empty LockResult,
	// which used to surface as a bogus modificationSupport="" / no handle
	// instead of the real conflict. Report what SAP actually said.
	if bytes.Contains(data, []byte("exc:exception")) {
		return nil, lockExceptionError(data)
	}

	var resp abapResponse
	if err := xml.Unmarshal(data, &resp); err != nil {
		return nil, fmt.Errorf("parsing lock response: %w", err)
	}

	return &LockResult{
		LockHandle:          resp.Values.Data.LockHandle,
		CorrNr:              resp.Values.Data.CorrNr,
		CorrUser:            resp.Values.Data.CorrUser,
		CorrText:            resp.Values.Data.CorrText,
		IsLocal:             resp.Values.Data.IsLocal == "X",
		IsLinkUp:            resp.Values.Data.IsLinkUp == "X",
		ModificationSupport: resp.Values.Data.ModSupport,
	}, nil
}

// lockExceptionError turns an ADT exception document returned by _action=LOCK
// into a Go error carrying SAP's own message (EU510 lock conflicts, missing
// authorization, unknown object).
func lockExceptionError(data []byte) error {
	type adtException struct {
		Type struct {
			ID string `xml:"id,attr"`
		} `xml:"type"`
		Message string `xml:"message"`
	}

	var exc adtException
	if err := xml.Unmarshal(data, &exc); err != nil || exc.Message == "" {
		return errors.New("locking object: SAP returned an ADT exception")
	}
	if exc.Type.ID != "" {
		return fmt.Errorf("locking object: %s: %s", exc.Type.ID, exc.Message)
	}
	return fmt.Errorf("locking object: %s", exc.Message)
}

// UnlockObject releases an edit lock on an ABAP object.
func (c *Client) UnlockObject(ctx context.Context, objectURL string, lockHandle string) error {
	params := url.Values{}
	params.Set("_action", "UNLOCK")
	params.Set("lockHandle", lockHandle)

	_, err := c.transport.Request(ctx, objectURL, &RequestOptions{
		Method:   http.MethodPost,
		Query:    params,
		Stateful: true, // Must match lock session (issue #88)
	})
	if err != nil {
		// Under cookie-file recovery, an expired session answering the
		// unlock means the session holding the lock is gone, and the enqueue
		// with it. Keeping the handle would refuse every later recovery until
		// the entry ages out.
		var apiErr *APIError
		if c.config != nil && c.config.ReauthReadOnly && errors.As(err, &apiErr) &&
			(apiErr.IsSessionExpired() || apiErr.StatusCode == http.StatusUnauthorized) {
			c.noteLockClosed(lockHandle)
		}
		return fmt.Errorf("unlocking object: %w", err)
	}

	// Only a *successful* unlock ends the window. A failed one may have left
	// the lock held, and suppressing a ping is the cheaper mistake.
	c.noteLockClosed(lockHandle)

	// The chain is done with its stateful context; behind a session-holding
	// proxy, retire it rather than leave it to the session timeout.
	c.transport.ReleaseProxyContext(ctx)

	return nil
}

// --- Update Source Operations ---

// UpdateSource writes source code to an ABAP object.
// objectSourceURL is the source URL (e.g., "/sap/bc/adt/programs/programs/ZTEST/source/main")
// lockHandle is required (from LockObject)
// transport is optional (for transportable objects)
func (c *Client) UpdateSource(ctx context.Context, objectSourceURL string, source string, lockHandle string, transport string) error {
	// Unified mutation policy gate (op type + package + transport)
	if err := c.checkMutation(ctx, MutationContext{
		Op:         OpUpdate,
		OpName:     "UpdateSource",
		ObjectURL:  objectSourceURL,
		Transport:  transport,
		LockHandle: lockHandle,
	}); err != nil {
		return err
	}
	// The caller may have read this object before preparing its replacement.
	// Read it again after taking the stateful MODIFY lock, so another editor
	// cannot be silently overwritten between that read and this PUT.
	if err := c.verifyExpectedSourceHash(ctx, objectSourceURL); err != nil {
		return err
	}

	params := url.Values{}
	params.Set("lockHandle", lockHandle)
	if transport != "" {
		params.Set("corrNr", transport)
	}

	// Determine content type based on source content
	contentType := "text/plain; charset=utf-8"
	if strings.HasPrefix(strings.TrimSpace(source), "<?xml") {
		contentType = "application/*"
	}

	_, err := c.transport.Request(ctx, objectSourceURL, &RequestOptions{
		Method:      http.MethodPut,
		Query:       params,
		Body:        []byte(source),
		ContentType: contentType,
		Stateful:    true, // Must match lock session (issue #88)
	})
	if err != nil {
		return fmt.Errorf("updating source: %w", err)
	}

	return nil
}
