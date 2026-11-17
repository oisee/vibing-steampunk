package adt

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/url"
	"strings"
)

// Print forms -- SAPscript forms, Smart Forms, Adobe forms and their interfaces
// -- are no ADT objects: ADT neither reads nor writes them. ZADT_VSP's form
// service does, through the APIs the form editors use. It runs in the APC
// session, where the workbench activation cannot run (it waits for asynchronous
// tasks); Adobe objects therefore come back inactive and are activated here,
// over ADT, which can.

// Form object types.
const (
	FormTypeSmartForm = "SSFO" // Smart Form
	FormTypeSAPscript = "FORM" // SAPscript form
	FormTypeAdobeForm = "SFPF" // Adobe form (without layout), or one layout with a language
	FormTypeAdobeIntf = "SFPI" // Adobe form interface
)

// formBridge is the part of ZADT_VSP's WebSocket client the form service needs.
type formBridge interface {
	FormRequest(ctx context.Context, action string, params map[string]any) (json.RawMessage, error)
}

// NormalizeFormType maps a form type and its common names to SSFO, FORM, SFPF
// or SFPI; ok is false for anything else.
func NormalizeFormType(t string) (string, bool) {
	switch strings.ToUpper(strings.TrimSpace(t)) {
	case "SSFO", "SMARTFORM", "SMARTFORMS", "SMART_FORM":
		return FormTypeSmartForm, true
	case "FORM", "SAPSCRIPT":
		return FormTypeSAPscript, true
	case "SFPF", "ADOBE", "ADOBE_FORM", "PDF_FORM":
		return FormTypeAdobeForm, true
	case "SFPI", "ADOBE_INTERFACE", "FORM_INTERFACE":
		return FormTypeAdobeIntf, true
	}
	return "", false
}

// FormInfo is what the system knows about a form.
type FormInfo struct {
	Type           string   `json:"type"`
	Name           string   `json:"name"`
	Exists         bool     `json:"exists"`
	Package        string   `json:"package,omitempty"`
	MasterLanguage string   `json:"masterLanguage,omitempty"`
	Languages      []string `json:"languages,omitempty"` // SAPscript texts, Adobe layouts
	Inactive       bool     `json:"inactive"`
}

// FormContent is a form as a document.
type FormContent struct {
	Type           string `json:"type"`
	Name           string `json:"name"`
	Language       string `json:"language,omitempty"` // the layout's language, for an Adobe layout
	MasterLanguage string `json:"masterLanguage,omitempty"`
	MimeType       string `json:"mimeType"`
	Content        string `json:"content"`
}

// FormWriteOptions describes a write.
type FormWriteOptions struct {
	Type      string
	Name      string
	Content   string // the document, as ReadForm returns it
	Language  string // Adobe form: write this language's layout (XDP) instead of the form
	Transport string
	Package   string // to create a Smart Form or SAPscript form
	TestRun   bool   // check everything, save nothing
}

// FormWriteResult is what a write did.
type FormWriteResult struct {
	Type       string            `json:"type"`
	Name       string            `json:"name"`
	Language   string            `json:"language,omitempty"`
	TestRun    bool              `json:"testRun,omitempty"`
	Saved      bool              `json:"saved"`
	Created    bool              `json:"created,omitempty"`
	Activated  bool              `json:"activated,omitempty"`
	Activation *ActivationResult `json:"activation,omitempty"`
	Restored   bool              `json:"restored,omitempty"` // a failed activation was undone
	Message    string            `json:"message,omitempty"`
}

// FormObjectURL is the ADT workbench URI of an Adobe form or interface, the one
// the inactive-objects list carries and the activation accepts.
func FormObjectURL(formType, name string) string {
	kind := "sfpf5f"
	if formType == FormTypeAdobeIntf {
		kind = "sfpi5i"
	}
	return "/sap/bc/adt/vit/wb/object_type/" + kind + "/object_name/" + url.PathEscape(strings.ToUpper(name))
}

func formRequestParams(formType, name, language string) (p map[string]any, t, normalized string, err error) {
	t, ok := NormalizeFormType(formType)
	if !ok {
		return nil, "", "", fmt.Errorf("form type %q is not one of SSFO, FORM, SFPF, SFPI", formType)
	}
	name = strings.ToUpper(strings.TrimSpace(name))
	if name == "" {
		return nil, "", "", fmt.Errorf("a form name is required")
	}
	p = map[string]any{"type": t, "name": name}
	if language = strings.ToUpper(strings.TrimSpace(language)); language != "" {
		p["language"] = language
	}
	return p, t, name, nil
}

// GetFormInfo reports whether a form exists, its package, original language,
// languages and whether it has an inactive version.
func (c *Client) GetFormInfo(ctx context.Context, ws formBridge, formType, name string) (*FormInfo, error) {
	p, _, _, err := formRequestParams(formType, name, "")
	if err != nil {
		return nil, err
	}
	if err = c.checkSafety(OpRead, "GetFormInfo"); err != nil {
		return nil, err
	}
	return formInfo(ctx, ws, p)
}

func formInfo(ctx context.Context, ws formBridge, p map[string]any) (*FormInfo, error) {
	data, err := ws.FormRequest(ctx, "info", map[string]any{"type": p["type"], "name": p["name"]})
	if err != nil {
		return nil, err
	}
	var info FormInfo
	if err := json.Unmarshal(data, &info); err != nil {
		return nil, fmt.Errorf("form service answer: %w", err)
	}
	return &info, nil
}

// ReadForm reads a form as a document. For an Adobe form, a language reads that
// language's layout (XDP) instead of the form.
func (c *Client) ReadForm(ctx context.Context, ws formBridge, formType, name, language string) (*FormContent, error) {
	p, t, formName, err := formRequestParams(formType, name, language)
	if err != nil {
		return nil, err
	}
	if err = c.checkSafety(OpRead, "ReadForm"); err != nil {
		return nil, err
	}
	data, err := ws.FormRequest(ctx, "read", p)
	if err != nil {
		return nil, err
	}
	var raw struct {
		MasterLanguage string `json:"masterLanguage"`
		MimeType       string `json:"mimeType"`
		ContentBase64  string `json:"contentBase64"`
	}
	if err = json.Unmarshal(data, &raw); err != nil {
		return nil, fmt.Errorf("form service answer: %w", err)
	}
	content, err := base64.StdEncoding.DecodeString(raw.ContentBase64)
	if err != nil {
		return nil, fmt.Errorf("form service content: %w", err)
	}
	lang, _ := p["language"].(string)
	if t != FormTypeAdobeForm {
		lang = "" // only an Adobe layout is one language's
	}
	return &FormContent{Type: t, Name: formName, Language: lang,
		MasterLanguage: raw.MasterLanguage, MimeType: raw.MimeType, Content: string(content)}, nil
}

type formWriteAnswer struct {
	Saved              bool   `json:"saved"`
	Created            bool   `json:"created"`
	ActivationRequired bool   `json:"activationRequired"`
	BackupBase64       string `json:"backupBase64"`
}

// WriteForm writes a form from a document. The package the form lives in --
// or, to create one, the package given -- goes through the mutation gate before
// anything is sent, and a transportable package needs a request. An Adobe form
// or interface comes back inactive and is activated here; when an Adobe form
// does not activate, the form as it was is written back and activated, so the
// system is left with the old form rather than none.
func (c *Client) WriteForm(ctx context.Context, ws formBridge, opts FormWriteOptions) (*FormWriteResult, error) {
	p, t, formName, err := formRequestParams(opts.Type, opts.Name, opts.Language)
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(opts.Content) == "" {
		return nil, fmt.Errorf("the form content is required")
	}
	transport := strings.ToUpper(strings.TrimSpace(opts.Transport))

	info, err := formInfo(ctx, ws, p)
	if err != nil {
		return nil, err
	}
	m := MutationContext{Op: OpUpdate, OpName: "WriteForm", Package: info.Package, Transport: transport}
	if !info.Exists {
		if t == FormTypeAdobeForm || t == FormTypeAdobeIntf {
			return nil, fmt.Errorf("%s %s does not exist; Adobe forms and interfaces are changed, not created", t, p["name"])
		}
		if strings.TrimSpace(opts.Package) == "" {
			return nil, fmt.Errorf("%s %s does not exist; a package is required to create it", t, p["name"])
		}
		m.Op = OpCreate
		m.Package = strings.ToUpper(strings.TrimSpace(opts.Package))
	}
	if err = c.checkMutation(ctx, m); err != nil {
		return nil, err
	}
	if transport != "" {
		if err = c.config.Safety.CheckTransport(transport, "WriteForm", true); err != nil {
			return nil, err
		}
	}

	p["contentBase64"] = base64.StdEncoding.EncodeToString([]byte(opts.Content))
	if transport != "" {
		p["transport"] = transport
	}
	if m.Op == OpCreate {
		p["package"] = m.Package
	}
	if opts.TestRun {
		p["testRun"] = "X"
	}
	ans, err := formWrite(ctx, ws, p)
	if err != nil {
		return nil, err
	}

	res := &FormWriteResult{Type: t, Name: formName, TestRun: opts.TestRun, Saved: ans.Saved, Created: ans.Created}
	if lang, ok := p["language"].(string); ok && t == FormTypeAdobeForm {
		res.Language = lang
	}
	if !ans.Saved || !ans.ActivationRequired {
		return res, nil
	}

	activation, inactive, actErr := c.activateForm(ctx, ws, t, res.Name)
	res.Activation = activation
	if actErr == nil && !inactive {
		res.Activated = true
		return res, nil
	}
	reason := activationFailure(activation, actErr)
	if ans.BackupBase64 == "" {
		// A layout or an interface: the active version is the old one, only
		// the new one waits inactive.
		return res, fmt.Errorf("%s %s was saved but did not activate: %s; the inactive version stays, the active one is unchanged", t, res.Name, reason)
	}

	// An Adobe form was replaced: without the old form back, it would have no
	// active version at all.
	restore := map[string]any{"type": t, "name": res.Name, "contentBase64": ans.BackupBase64}
	if transport != "" {
		restore["transport"] = transport
	}
	if _, err := formWrite(ctx, ws, restore); err != nil {
		return res, fmt.Errorf("%s %s did not activate (%s), and writing the old form back failed: %w", t, res.Name, reason, err)
	}
	if _, inactive, err := c.activateForm(ctx, ws, t, res.Name); err != nil || inactive {
		return res, fmt.Errorf("%s %s did not activate (%s); the old form was written back but did not activate either", t, res.Name, reason)
	}
	res.Restored = true
	return res, fmt.Errorf("%s %s did not activate: %s; the old form was put back and is active", t, res.Name, reason)
}

func formWrite(ctx context.Context, ws formBridge, p map[string]any) (*formWriteAnswer, error) {
	data, err := ws.FormRequest(ctx, "write", p)
	if err != nil {
		return nil, err
	}
	var ans formWriteAnswer
	if err := json.Unmarshal(data, &ans); err != nil {
		return nil, fmt.Errorf("form service answer: %w", err)
	}
	return &ans, nil
}

// activateForm activates an Adobe form or interface over ADT and asks the form
// service whether an inactive version is left. That answer, not the
// activation's, decides: SAP has answered success for objects it left inactive.
func (c *Client) activateForm(ctx context.Context, ws formBridge, formType, name string) (*ActivationResult, bool, error) {
	result, err := c.Activate(ctx, FormObjectURL(formType, name), name)
	if err != nil {
		return result, true, err
	}
	info, err := formInfo(ctx, ws, map[string]any{"type": formType, "name": name})
	if err != nil {
		return result, true, err
	}
	return result, info.Inactive, nil
}

func activationFailure(r *ActivationResult, err error) string {
	if err != nil {
		return err.Error()
	}
	if r != nil {
		if lines := r.ProblemLines(); len(lines) > 0 {
			return strings.Join(lines, "; ")
		}
	}
	return "SAP left it inactive without giving a reason"
}
