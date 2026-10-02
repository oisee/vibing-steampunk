package adt

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strings"
)

// A BAdI implementation -- an ENHO of tool type BADI_IMPL -- is created the way
// the editor's wizard does it, in two requests: a POST of an empty container
// that names only the enhancement spot, then the implementation itself written
// into it (lock, PUT, unlock). A POST that carries the implementation straight
// away is refused with an empty "I::000", and leaves a catalog entry behind.

const (
	enhoxhbCollection  = "/sap/bc/adt/enhancements/enhoxhb"
	enhoxhbContentType = "application/vnd.sap.adt.enh.enhoxhb.v4+xml"
)

// BadiImplementationOptions describes a BAdI implementation to create.
type BadiImplementationOptions struct {
	// Name is the enhancement implementation (the ENHO).
	Name        string
	Description string
	Package     string
	Transport   string
	// Spot is the enhancement spot the BAdI belongs to.
	Spot string
	// BadiDefinition is the BAdI; it defaults to the spot's name, which is
	// what it is for every spot with a single BAdI.
	BadiDefinition string
	// ImplementingClass must exist and implement the BAdI's interface.
	ImplementingClass string
	// Implementation is the BAdI implementation's name inside the ENHO; it
	// defaults to Name.
	Implementation string
	ShortText      string
	// Inactive creates the implementation switched off: the ENHO is active,
	// the implementation is not called.
	Inactive bool
}

// CreateBadiImplementation creates an ENHO with one BAdI implementation and
// returns its URL. The ENHO is left inactive; activate it to put it in force.
func (c *Client) CreateBadiImplementation(ctx context.Context, opts BadiImplementationOptions) (string, error) {
	opts.Name = strings.ToUpper(strings.TrimSpace(opts.Name))
	opts.Package = strings.ToUpper(strings.TrimSpace(opts.Package))
	opts.Spot = strings.ToUpper(strings.TrimSpace(opts.Spot))
	opts.BadiDefinition = strings.ToUpper(strings.TrimSpace(opts.BadiDefinition))
	opts.ImplementingClass = strings.ToUpper(strings.TrimSpace(opts.ImplementingClass))
	opts.Implementation = strings.ToUpper(strings.TrimSpace(opts.Implementation))
	if opts.Name == "" || opts.Package == "" || opts.Spot == "" || opts.ImplementingClass == "" {
		return "", fmt.Errorf("name, package, the enhancement spot and the implementing class are required")
	}
	if opts.Description == "" {
		return "", fmt.Errorf("a description is required")
	}
	if opts.BadiDefinition == "" {
		opts.BadiDefinition = opts.Spot
	}
	if opts.Implementation == "" {
		opts.Implementation = opts.Name
	}
	objectURL := enhoxhbCollection + "/" + url.PathEscape(strings.ToLower(opts.Name))

	transport, err := c.enhancementCreateGates(ctx, "CreateBadiImplementation", opts.Package, opts.Transport, objectURL)
	if err != nil {
		return "", err
	}
	opts.Transport = transport
	// A transportable package with no request and transportable edits off:
	// the POST would still land in a request SAP picks, which the lock then
	// names and the write refuses -- after the container exists. Refused
	// here instead, before anything is written.
	if opts.Transport == "" && !strings.HasPrefix(opts.Package, "$") && !c.config.Safety.AllowTransportableEdits {
		return "", fmt.Errorf("CreateBadiImplementation in package %s is blocked: it is not a local ($) package, and editing transportable "+
			"objects is disabled (use --allow-transportable-edits; name a transport unless the transport choice picks one)", opts.Package)
	}
	// The implementation is written by a PUT into the container: an update.
	// Refused here, before the POST, so a refusal leaves no empty ENHO.
	if err = c.checkSafety(OpUpdate, "CreateBadiImplementation"); err != nil {
		return "", err
	}

	params := url.Values{}
	if opts.Transport != "" {
		params.Set("corrNr", opts.Transport)
	}
	if _, err = c.transport.Request(ctx, enhoxhbCollection, &RequestOptions{
		Method:      http.MethodPost,
		Query:       params,
		Body:        []byte(badiImplementationBody(opts, c.config.Language, false)),
		ContentType: enhoxhbContentType,
		Accept:      enhoxhbContentType,
	}); err != nil {
		return "", fmt.Errorf("creating %s: %w", opts.Name, err)
	}

	// From here on the ENHO exists, and this call created it: a step that
	// fails takes it away again rather than leaving an empty container
	// behind, which would also block the next attempt under the same name.
	lock, err := c.LockObject(ctx, objectURL, "MODIFY", opts.Transport)
	if err != nil {
		return c.undoBadiContainer(ctx, objectURL, opts, fmt.Errorf("locking it to add the implementation failed: %w", err))
	}
	// With no request chosen at creation, the PUT goes with the one the lock
	// names, as every other write under a lock does.
	writeTransport, err := c.resolveWriteTransport(opts.Transport, lock.CorrNr, "CreateBadiImplementation")
	if err != nil {
		if uerr := c.releaseLockAfterFailure(ctx, objectURL, lock.LockHandle); uerr != nil {
			return objectURL, fmt.Errorf("created %s, but %w; %s", opts.Name, err, strandedLockAdvice(objectURL, uerr))
		}
		// Not deleted: the refusal is about writing to the request the
		// container landed in, and a DELETE is a write to that same request.
		return objectURL, fmt.Errorf("created %s in %s, but %w; it is an empty ENHO without its BAdI implementation: "+
			"delete it in SE80, or allow the request and add the implementation in SE19 or Eclipse", opts.Name, lock.CorrNr, err)
	}
	opts.Transport = writeTransport
	put := url.Values{}
	put.Set("lockHandle", lock.LockHandle)
	if opts.Transport != "" {
		put.Set("corrNr", opts.Transport)
	}
	_, err = c.transport.Request(ctx, objectURL, &RequestOptions{
		Method:      http.MethodPut,
		Query:       put,
		Body:        []byte(badiImplementationBody(opts, c.config.Language, true)),
		ContentType: enhoxhbContentType,
		Accept:      enhoxhbContentType,
		Stateful:    true,
	})
	if err != nil {
		// Released on a context of its own: the PUT may have failed because
		// ctx was cancelled, and the lock must not stay behind.
		if uerr := c.releaseLockAfterFailure(ctx, objectURL, lock.LockHandle); uerr != nil {
			return objectURL, fmt.Errorf("created %s, but adding the implementation failed: %w%s; %s", opts.Name, err, dialogHint(err), strandedLockAdvice(objectURL, uerr))
		}
		return c.undoBadiContainer(ctx, objectURL, opts, fmt.Errorf("adding the implementation failed: %w%s", err, dialogHint(err)))
	}
	if uerr := c.UnlockObject(ctx, objectURL, lock.LockHandle); uerr != nil {
		return objectURL, fmt.Errorf("created %s with its implementation, but %s", opts.Name, strandedLockAdvice(objectURL, uerr))
	}
	return objectURL, nil
}

// undoBadiContainer deletes the ENHO this call created when adding the
// implementation to it failed. It returns no URL when the container is gone
// again, and the URL with what is left to do by hand when it is not. The
// delete runs on a context of its own: the step may have failed because ctx
// was cancelled. A container in a transportable package whose request is not
// known is kept: see below.
func (c *Client) undoBadiContainer(ctx context.Context, objectURL string, opts BadiImplementationOptions, stepErr error) (string, error) {
	if opts.Transport == "" && !strings.HasPrefix(opts.Package, "$") {
		// A transportable package and no request known: SAP may have put the
		// container in a request of its choosing during the POST. A DELETE
		// sent without that request would write to it unchecked by
		// --allowed-transports, so the container is kept.
		return objectURL, &PartialCreateError{
			ObjectURL: objectURL,
			Package:   opts.Package,
			OriginalErr: fmt.Errorf("created %s, but %w; the empty container was kept, not deleted: "+
				"the request SAP recorded it in is not known, and a delete must not bypass --allowed-transports", opts.Name, stepErr),
			CleanupActions: []string{"not deleted: the transport request the container was recorded in could not be established"},
			ManualSteps: []string{
				fmt.Sprintf("%s is an empty ENHO without its BAdI implementation in package %s: add the implementation in SE19 or Eclipse, "+
					"or delete it in SE80 and remove its R3TR ENHO %s entry from the request SE09 shows it in", opts.Name, opts.Package, opts.Name),
			},
		}
	}
	cleanupCtx, cancel := failureCleanupContext(ctx)
	defer cancel()
	pce := c.cleanupPartialObject(cleanupCtx, objectURL, opts.Package, opts.Transport)
	pce.OriginalErr = fmt.Errorf("created %s, but %w", opts.Name, stepErr)
	if pce.CleanupOK {
		if opts.Transport != "" {
			// Created and deleted within one open request: the request keeps
			// an R3TR ENHO entry for an object that no longer exists.
			pce.ManualSteps = append(pce.ManualSteps, fmt.Sprintf(
				"%s keeps an entry R3TR ENHO %s for the deleted container; remove it if unwanted (remove_transport_object)", opts.Transport, opts.Name))
		}
		return "", pce
	}
	pce.ManualSteps = append([]string{
		fmt.Sprintf("%s is an empty ENHO without its BAdI implementation: add the implementation in SE19 or Eclipse, or delete it", opts.Name),
	}, pce.ManualSteps...)
	return objectURL, pce
}

// dialogHint explains SAP's answer when the server side tried to show a
// dialog. Over ADT there is no window to show it in, so the request fails
// with "Sending of dynpro SAPLSPO1 ... not possible" -- a POPUP_TO_CONFIRM.
func dialogHint(err error) string {
	if err == nil {
		return ""
	}
	msg := err.Error()
	if !strings.Contains(msg, "Sending of dynpro") && !strings.Contains(msg, "SAPLSPO1") {
		return ""
	}
	return " (SAP asked a question in a dialog, which cannot be answered over ADT. One known case: the BAdI is marked " +
		"SAP-internal and the ENHO's name is in a namespace of your own rather than Y/Z; SAP then asks whether to create " +
		"the implementation as an SAP advance delivery. Answer that question in SE19 or Eclipse.)"
}

// badiImplementationBody is the BADI_IMPL document: the container the POST
// creates, naming the spot, and -- with the implementation -- what the PUT
// writes into it.
func badiImplementationBody(opts BadiImplementationOptions, language string, withImplementation bool) string {
	lang := ""
	if language != "" {
		lang = fmt.Sprintf(` adtcore:language=%q adtcore:masterLanguage=%q`, xmlAttr(language), xmlAttr(language))
	}
	impl := "<enho:badiImplementations/>"
	if withImplementation {
		active := "true"
		if opts.Inactive {
			active = "false"
		}
		impl = fmt.Sprintf(`<enho:badiImplementations>
        <enho:badiImplementation enho:name="%s" enho:shortText="%s" enho:example="false" enho:default="false" enho:active="%s" enho:customizingLock="">
          <enho:enhancementSpot adtcore:type="ENHS/XSB" adtcore:name="%s"/>
          <enho:badiDefinition adtcore:type="ENHS/XB" adtcore:name="%s"/>
          <enho:implementingClass adtcore:type="CLAS/OC" adtcore:name="%s"/>
        </enho:badiImplementation>
      </enho:badiImplementations>`,
			xmlAttr(opts.Implementation), xmlAttr(opts.ShortText), active,
			xmlAttr(opts.Spot), xmlAttr(opts.BadiDefinition), xmlAttr(opts.ImplementingClass))
	}
	return fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?>
<enho:objectData xmlns:enho="http://www.sap.com/adt/enhancements/enho" xmlns:adtcore="http://www.sap.com/adt/core" xmlns:enhcore="http://www.sap.com/abapsource/enhancementscore" adtcore:name="%s" adtcore:type="ENHO/XHB" adtcore:description="%s"%s>
  <adtcore:packageRef adtcore:name="%s"/>
  <enho:contentCommon enho:toolType="BADI_IMPL">
    <enho:usages>
      <enhcore:referencedObject enhcore:element_usage="EXTO" enhcore:program_id="R3TR">
        <enhcore:objectReference adtcore:name="%s" adtcore:type="ENHS/XS"/>
        <enhcore:mainObjectReference/>
      </enhcore:referencedObject>
    </enho:usages>
  </enho:contentCommon>
  <enho:contentSpecific>
    <enho:badiTechnology>
      %s
    </enho:badiTechnology>
  </enho:contentSpecific>
</enho:objectData>`,
		xmlAttr(opts.Name), xmlAttr(opts.Description), lang, xmlAttr(opts.Package), xmlAttr(opts.Spot), impl)
}
