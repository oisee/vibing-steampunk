package adt

import (
	"context"
	"encoding/xml"
	"fmt"
	"net/http"
	"net/url"
	"strings"
)

// Processing types of a function module, as ADT spells them in the
// fmodule:processingType attribute. "rfc" is the one that makes a module
// callable from outside the system — the SE37 checkbox "Remote-Enabled
// Module".
const (
	FunctionProcessingNormal = "normal"
	FunctionProcessingRFC    = "rfc"
	FunctionProcessingUpdate = "update"
)

// Namespaces used by the function-module resources.
const (
	nsADTCore  = "http://www.sap.com/adt/core"
	nsFModules = "http://www.sap.com/adt/functions/fmodules"
)

// FunctionModuleInfo is the metadata ADT keeps about a function module,
// separate from its source. ProcessingType is the interesting one: the
// signature lives in the source, but whether the module is remote-enabled
// does not.
type FunctionModuleInfo struct {
	Name           string `json:"name"`
	Description    string `json:"description"`
	Group          string `json:"group"`
	ProcessingType string `json:"processingType"`
	ReleaseState   string `json:"releaseState,omitempty"`
	RFCScope       string `json:"rfcScope,omitempty"`
	RFCVersion     string `json:"rfcVersion,omitempty"`
	Version        string `json:"version,omitempty"`
}

// IsRFCEnabled reports whether the module can be called over RFC.
func (i *FunctionModuleInfo) IsRFCEnabled() bool {
	return strings.EqualFold(i.ProcessingType, FunctionProcessingRFC)
}

// GetFunctionModule reads a function module's metadata (not its source).
func (c *Client) GetFunctionModule(ctx context.Context, group, name string) (*FunctionModuleInfo, error) {
	objectURL := GetObjectURL(ObjectTypeFunctionMod, name, group)

	resp, err := c.transport.Request(ctx, objectURL, &RequestOptions{
		Method: http.MethodGet,
		Accept: "application/vnd.sap.adt.functions.fmodules.v3+xml, application/vnd.sap.adt.functions.fmodules.v2+xml, application/xml",
	})
	if err != nil {
		return nil, fmt.Errorf("reading function module %s: %w", strings.ToUpper(name), err)
	}

	var doc struct {
		Name           string `xml:"http://www.sap.com/adt/core name,attr"`
		Description    string `xml:"http://www.sap.com/adt/core description,attr"`
		Version        string `xml:"http://www.sap.com/adt/core version,attr"`
		ProcessingType string `xml:"http://www.sap.com/adt/functions/fmodules processingType,attr"`
		ReleaseState   string `xml:"http://www.sap.com/adt/functions/fmodules releaseState,attr"`
		RFCScope       string `xml:"http://www.sap.com/adt/functions/fmodules rfcScope,attr"`
		RFCVersion     string `xml:"http://www.sap.com/adt/functions/fmodules rfcVersion,attr"`
		ContainerRef   struct {
			Name string `xml:"http://www.sap.com/adt/core name,attr"`
		} `xml:"http://www.sap.com/adt/core containerRef"`
	}
	if err := xml.Unmarshal(resp.Body, &doc); err != nil {
		return nil, fmt.Errorf("parsing function module %s: %w", strings.ToUpper(name), err)
	}

	info := &FunctionModuleInfo{
		Name:           doc.Name,
		Description:    doc.Description,
		Group:          doc.ContainerRef.Name,
		ProcessingType: doc.ProcessingType,
		ReleaseState:   doc.ReleaseState,
		RFCScope:       doc.RFCScope,
		RFCVersion:     doc.RFCVersion,
		Version:        doc.Version,
	}
	if info.Group == "" {
		info.Group = strings.ToUpper(group)
	}
	return info, nil
}

// SetFunctionModuleProcessingType switches a function module between normal
// and remote-enabled ("rfc").
//
// This is a separate call on purpose: creation ignores the attribute. POSTing
// a new module with fmodule:processingType="rfc" returns 201 and a module that
// comes back as "normal" — the flag only takes on a PUT of the module's
// metadata under a lock. Activation of the owning group is left to the caller,
// which usually has a source write to activate anyway.
func (c *Client) SetFunctionModuleProcessingType(ctx context.Context, group, name, processingType string) error {
	return c.writeFunctionModule(ctx, group, name, processingType, "", "")
}

// writeFunctionModule performs the metadata and/or source write of a function
// module inside a single lock: LOCK → PUT metadata → PUT source → UNLOCK.
// Either write is optional. Doing both under one lock keeps the ENQUEUE
// footprint to one acquire/release even for a full create-and-fill flow.
func (c *Client) writeFunctionModule(ctx context.Context, group, name, processingType, source, transport string) (retErr error) {
	objectURL := GetObjectURL(ObjectTypeFunctionMod, name, group)

	// Gate here, above the lock. Two reasons.
	//
	// Policy: the metadata PUT below is a mutation with no gate of its own, so
	// SetFunctionModuleProcessingType could flip a module to RFC-enabled in any
	// package, whitelist or not. The source write reaches UpdateSource, which
	// does gate — but only from inside the lock.
	//
	// Sessions: that inner package lookup is a stateless request, and issued
	// between the LOCK and either PUT it retires the session the lock handle
	// belongs to (issue #91). Running the same check up here and marking the
	// context keeps the check and removes the request from the window.
	ctx, err := c.gateAndMark(ctx, MutationContext{
		Op:        OpUpdate,
		OpName:    "WriteFunctionModule",
		ObjectURL: objectURL,
		Transport: transport,
	})
	if err != nil {
		return err
	}

	// The metadata PUT replaces the document, so start from what SAP has —
	// otherwise the description is silently blanked.
	info, err := c.GetFunctionModule(ctx, group, name)
	if err != nil {
		return err
	}

	lock, err := c.LockObject(ctx, objectURL, "MODIFY", transport)
	if err != nil {
		return fmt.Errorf("locking function module %s: %w", strings.ToUpper(name), err)
	}
	unlocked := false
	// unlockRetried marks a failed final unlock: the release below is then
	// its retry, detached from ctx.
	unlockRetried := false
	defer func() {
		if !unlocked {
			// The compensating unlock is the only place a leak can be
			// observed, so its failure is reported rather than dropped.
			if unlockErr := c.releaseLockAfterFailure(ctx, objectURL, lock.LockHandle); unlockErr != nil {
				advice := strandedLockAdvice(objectURL, unlockErr)
				if retErr != nil {
					retErr = fmt.Errorf("%w — %s", retErr, advice)
				} else {
					retErr = fmt.Errorf("%s", advice)
				}
			} else if unlockRetried && retErr != nil {
				retErr = fmt.Errorf("%w — the lock was released on a retry", retErr)
			}
		}
	}()

	// Reuse SAP's recording request when the caller did not name one, just
	// like the other source writers. Resolve it after cleanup is registered:
	// a request rejected by policy must still release the acquired lock.
	transport, err = c.resolveWriteTransport(transport, lock.CorrNr, "WriteFunctionModule")
	if err != nil {
		return err
	}

	if processingType != "" && !strings.EqualFold(processingType, info.ProcessingType) {
		params := url.Values{}
		params.Set("lockHandle", lock.LockHandle)
		if transport != "" {
			params.Set("corrNr", transport)
		}

		if _, err := c.transport.Request(ctx, objectURL, &RequestOptions{
			Method:      http.MethodPut,
			Query:       params,
			Body:        []byte(functionModuleMetadataXML(info, processingType)),
			ContentType: "application/vnd.sap.adt.functions.fmodules.v3+xml",
			Stateful:    true, // Must match the lock session (issue #88)
		}); err != nil {
			return fmt.Errorf("setting processingType=%s on %s: %w", processingType, strings.ToUpper(name), err)
		}
	}

	if source != "" {
		if err := c.UpdateSource(ctx, objectURL+"/source/main", source, lock.LockHandle, transport); err != nil {
			return err
		}
	}

	if err := c.UnlockObject(ctx, objectURL, lock.LockHandle); err != nil {
		// Left to the deferred release, which retries detached from ctx.
		unlockRetried = true
		return fmt.Errorf("unlocking function module %s: %w", strings.ToUpper(name), err)
	}
	unlocked = true
	return nil
}

// functionModuleMetadataXML renders the metadata document for a PUT, carrying
// over what SAP already knows and changing only the processing type. The
// rfcScope / rfcVersion pair is only meaningful for remote-enabled modules;
// SAP fills the defaults ("notClassified" / "any") when they are omitted.
func functionModuleMetadataXML(info *FunctionModuleInfo, processingType string) string {
	var attrs strings.Builder
	fmt.Fprintf(&attrs, ` fmodule:processingType=%q`, processingType)
	if info.ReleaseState != "" {
		fmt.Fprintf(&attrs, ` fmodule:releaseState=%q`, info.ReleaseState)
	}
	if strings.EqualFold(processingType, FunctionProcessingRFC) {
		scope := info.RFCScope
		if scope == "" {
			scope = "notClassified"
		}
		version := info.RFCVersion
		if version == "" {
			version = "any"
		}
		fmt.Fprintf(&attrs, ` fmodule:rfcScope=%q fmodule:rfcVersion=%q`, scope, version)
	}

	return fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?>
<fmodule:abapFunctionModule xmlns:fmodule=%q xmlns:adtcore=%q
  adtcore:description="%s"
  adtcore:name="%s"
  adtcore:type="FUGR/FF"%s>
  <adtcore:containerRef adtcore:name="%s" adtcore:type="FUGR/F"
    adtcore:uri="/sap/bc/adt/functions/groups/%s"/>
</fmodule:abapFunctionModule>`,
		nsFModules, nsADTCore,
		escapeXML(info.Description),
		strings.ToUpper(info.Name),
		attrs.String(),
		strings.ToUpper(info.Group),
		url.PathEscape(strings.ToLower(info.Group)))
}

// CreateFunctionModuleOptions describes a function module to create inside an
// existing function group.
type CreateFunctionModuleOptions struct {
	// Group is the existing function group that will hold the module.
	Group string `json:"group"`
	// Name of the function module.
	Name string `json:"name"`
	// Description is mandatory for creation.
	Description string `json:"description"`
	// PackageName is the package the module is created in — normally the
	// package of its function group.
	PackageName string `json:"packageName"`
	// Transport is required for objects outside a local package.
	Transport string `json:"transport,omitempty"`
	// RFCEnabled makes the module remote-enabled (SE37's "Remote-Enabled
	// Module"). Remote-enabled modules must pass every parameter by value.
	RFCEnabled bool `json:"rfcEnabled,omitempty"`
	// Source is the full module source, signature included:
	//
	//	FUNCTION zvsp_demo
	//	  IMPORTING VALUE(iv_n) TYPE i
	//	  EXPORTING VALUE(ev_result) TYPE i.
	//	  ev_result = iv_n * 2.
	//	ENDFUNCTION.
	//
	// ADT keeps a function module's interface in its source, so this is what
	// defines the parameters. Left empty, the module keeps SAP's skeleton.
	Source string `json:"source,omitempty"`
	// SkipActivation leaves the module inactive.
	SkipActivation bool `json:"skipActivation,omitempty"`
}

// CreateFunctionModuleResult reports what each step of the flow did.
type CreateFunctionModuleResult struct {
	Success       bool              `json:"success"`
	ObjectURL     string            `json:"objectUrl"`
	Group         string            `json:"group"`
	Name          string            `json:"name"`
	RFCEnabled    bool              `json:"rfcEnabled"`
	SourceWritten bool              `json:"sourceWritten"`
	Activation    *ActivationResult `json:"activation,omitempty"`
	Message       string            `json:"message"`
}

// CreateFunctionModule creates a function module in an existing function
// group and, unlike a bare CreateObject, finishes the job: it flips the
// remote-enabled flag, writes the source (which is where the signature
// lives), and activates.
//
// The flag needs its own PUT — see SetFunctionModuleProcessingType — so a
// module created with CreateObject alone is always "normal", whatever the
// creation document asked for. That is the whole reason this workflow exists.
func (c *Client) CreateFunctionModule(ctx context.Context, opts CreateFunctionModuleOptions) (*CreateFunctionModuleResult, error) {
	if opts.Group == "" {
		return nil, fmt.Errorf("function group is required to create a function module")
	}
	if opts.Name == "" {
		return nil, fmt.Errorf("function module name is required")
	}

	result := &CreateFunctionModuleResult{
		ObjectURL: GetObjectURL(ObjectTypeFunctionMod, opts.Name, opts.Group),
		Group:     strings.ToUpper(opts.Group),
		Name:      strings.ToUpper(opts.Name),
	}

	if err := c.CreateObject(ctx, CreateObjectOptions{
		ObjectType:  ObjectTypeFunctionMod,
		Name:        opts.Name,
		Description: opts.Description,
		PackageName: opts.PackageName,
		Transport:   opts.Transport,
		ParentName:  opts.Group,
	}); err != nil {
		result.Message = fmt.Sprintf("Failed to create function module: %v", err)
		return result, err
	}

	// The module was just created in opts.PackageName, which CreateObject
	// checked against the whitelist — so writeFunctionModule's gate need not
	// resolve it over the wire. That matters twice over here: a module created
	// moments ago may not be in the search index yet, and the lookup would sit
	// inside writeFunctionModule's lock window (issue #91).
	ctx = withMutationPackageChecked(ctx, result.ObjectURL)

	processingType := ""
	if opts.RFCEnabled {
		processingType = FunctionProcessingRFC
	}
	if processingType != "" || opts.Source != "" {
		if err := c.writeFunctionModule(ctx, opts.Group, opts.Name, processingType, opts.Source, opts.Transport); err != nil {
			result.Message = fmt.Sprintf("Function module created, but filling it failed: %v", err)
			return result, err
		}
		result.RFCEnabled = opts.RFCEnabled
		result.SourceWritten = opts.Source != ""
	}

	if opts.SkipActivation {
		result.Success = true
		result.Message = "Function module created (left inactive)"
		return result, nil
	}

	activation, err := c.Activate(ctx, result.ObjectURL, result.Name)
	result.Activation = activation
	if err != nil {
		result.Message = fmt.Sprintf("Function module written, but activation failed: %v", err)
		return result, err
	}
	if !activation.Success {
		result.Message = "Activation failed - check activation messages"
		return result, nil
	}

	result.Success = true
	if result.RFCEnabled {
		result.Message = "RFC-enabled function module created and activated"
	} else {
		result.Message = "Function module created and activated"
	}
	return result, nil
}
