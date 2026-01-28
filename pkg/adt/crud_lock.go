// ABOUTME: Lock and unlock operations for ABAP objects.
// ABOUTME: Provides LockObject, UnlockObject, and lock result parsing.

package adt

import (
	"context"
	"encoding/xml"
	"fmt"
	"net/http"
	"net/url"
)

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
func (c *Client) LockObject(ctx context.Context, objectURL string, accessMode string) (*LockResult, error) {
	// Safety check - only check for MODIFY locks, READ locks are safe
	if accessMode == "" || accessMode == "MODIFY" {
		if err := c.checkSafety(OpLock, "LockObject"); err != nil {
			return nil, err
		}
	}

	if accessMode == "" {
		accessMode = "MODIFY"
	}

	params := url.Values{}
	params.Set("_action", "LOCK")
	params.Set("accessMode", accessMode)

	resp, err := c.transport.Request(ctx, objectURL, &RequestOptions{
		Method: http.MethodPost,
		Query:  params,
		Accept: "application/vnd.sap.as+xml;charset=UTF-8;dataname=com.sap.adt.lock.result",
	})
	if err != nil {
		return nil, fmt.Errorf("locking object: %w", err)
	}

	return parseLockResult(resp.Body)
}

func parseLockResult(data []byte) (*LockResult, error) {
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

// UnlockObject releases an edit lock on an ABAP object.
func (c *Client) UnlockObject(ctx context.Context, objectURL string, lockHandle string) error {
	params := url.Values{}
	params.Set("_action", "UNLOCK")
	params.Set("lockHandle", lockHandle)

	_, err := c.transport.Request(ctx, objectURL, &RequestOptions{
		Method: http.MethodPost,
		Query:  params,
	})
	if err != nil {
		return fmt.Errorf("unlocking object: %w", err)
	}

	return nil
}
