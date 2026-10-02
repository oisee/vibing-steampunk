package adt

import (
	"context"
	"fmt"
	"net/http"
	"strings"
)

// AMC applications (object type SAMC, transaction SAMC) define ABAP Messaging
// Channels: who may send on a channel, and who may bind an APC WebSocket to
// it. ADT edits them as "blue" objects under
// /sap/bc/adt/uc_object_type_group/samc, the definition being an asXML
// document at source/main.

const amcCollection = "/sap/bc/adt/uc_object_type_group/samc"

// AMCApplicationURL is the ADT URI of an AMC application.
func AMCApplicationURL(name string) string {
	return amcCollection + "/" + strings.ToLower(name)
}

// UpsertAMCApplication creates the AMC application name in package pkg when it
// does not exist, writes its definition (the asXML document of channels and
// authorities) and activates it.
func (c *Client) UpsertAMCApplication(ctx context.Context, name, description, pkg, definition string) error {
	name = strings.ToUpper(strings.TrimSpace(name))
	objectURL := AMCApplicationURL(name)
	if err := c.checkSafety(OpUpdate, "UpsertAMCApplication"); err != nil {
		return err
	}
	if err := c.checkPackageSafety(pkg); err != nil {
		return err
	}

	_, err := c.transport.Request(ctx, objectURL, &RequestOptions{
		Method: http.MethodGet,
		Accept: "application/vnd.sap.adt.blues.v1+xml",
	})
	switch {
	case err == nil:
	case IsNotFoundError(err):
		if err = c.checkSafety(OpCreate, "UpsertAMCApplication"); err != nil {
			return err
		}
		body := fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?>
<blue:blueSource xmlns:blue="http://www.sap.com/wbobj/blue" xmlns:adtcore="http://www.sap.com/adt/core" adtcore:description="%s" adtcore:name="%s" adtcore:type="SAMC" adtcore:masterLanguage="EN">
  <adtcore:packageRef adtcore:name="%s"/>
</blue:blueSource>`, escapeXML(description), escapeXML(name), escapeXML(pkg))
		if _, err = c.transport.Request(ctx, amcCollection, &RequestOptions{
			Method:      http.MethodPost,
			Body:        []byte(body),
			ContentType: "application/vnd.sap.adt.blues.v1+xml",
		}); err != nil {
			return fmt.Errorf("creating AMC application %s: %w", name, err)
		}
	default:
		return fmt.Errorf("reading AMC application %s: %w", name, err)
	}

	lock, err := c.LockObject(ctx, objectURL, "MODIFY")
	if err != nil {
		return fmt.Errorf("locking AMC application %s: %w", name, err)
	}
	_, werr := c.transport.Request(ctx, objectURL+"/source/main", &RequestOptions{
		Method:      http.MethodPut,
		Query:       map[string][]string{"lockHandle": {lock.LockHandle}},
		Body:        []byte(definition),
		ContentType: "application/vnd.sap.adt.blueasxml.v1+xml",
		Stateful:    true,
	})
	held := c.holdLock(objectURL, lock.LockHandle)
	if werr != nil {
		// Released on a detached, bounded context: the write may have failed
		// because ctx ended. A lock that cannot be released is reported.
		if advice := held.release(ctx); advice != "" {
			return fmt.Errorf("writing AMC application %s: %w — %s", name, werr, advice)
		}
		return fmt.Errorf("writing AMC application %s: %w", name, werr)
	}
	if uerr := held.unlock(ctx); uerr != nil {
		// ctx may have ended with the PUT; retry detached before giving up.
		if advice := held.release(ctx); advice != "" {
			return fmt.Errorf("unlocking AMC application %s: %w — %s", name, uerr, advice)
		}
	}
	res, err := c.Activate(ctx, objectURL, name)
	if err != nil {
		return fmt.Errorf("activating AMC application %s: %w", name, err)
	}
	if aerr := ActivationResultError(res); aerr != nil {
		return fmt.Errorf("activating AMC application %s: %w", name, aerr)
	}
	return nil
}
