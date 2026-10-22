package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/mark3labs/mcp-go/mcp"

	"github.com/oisee/vibing-steampunk/pkg/adt"
)

// Print forms through ZADT_VSP's form service:
//
//	SAP(action="read", target="SSFO ZDEMO_FORM")
//	SAP(action="read", target="SFPF ZDEMO_PDF", params={"language": "ES"})   the XDP layout of ES
//	SAP(action="read", target="FORM ZDEMO_SCRIPT", params={"info": true})    package, languages, inactive
//	SAP(action="edit", target="SFPI ZDEMO_IF", params={"content": "<?xml …", "transport": "A4HK900001"})
//	SAP(action="create", target="FORM ZDEMO_SCRIPT", params={"file_path": "zdemo.xml", "package": "$TMP"})
//
// "file_path" writes a read to a local file, or takes a write's content from
// one; forms run to hundreds of kilobytes.
func (s *Server) routeFormAction(ctx context.Context, action, objectType, objectName string, params map[string]any) (*mcp.CallToolResult, bool, error) {
	formType, ok := formTargetType(objectType)
	if !ok {
		return nil, false, nil
	}
	args := map[string]any{}
	for k, v := range params {
		args[k] = v
	}
	args["type"] = formType
	args["name"] = objectName
	switch action {
	case "read":
		return s.callHandler(ctx, s.handleReadForm, args)
	case "edit", "create":
		args["create"] = action == "create"
		return s.callHandler(ctx, s.handleWriteForm, args)
	}
	return nil, false, nil
}

// formTargetType accepts the four form object types only. The aliases
// NormalizeFormType knows (SAPSCRIPT, ADOBE, ...) are left out on purpose:
// a SAP() target names an object type.
func formTargetType(objectType string) (string, bool) {
	switch objectType {
	case adt.FormTypeSmartForm, adt.FormTypeSAPscript, adt.FormTypeAdobeForm, adt.FormTypeAdobeIntf:
		return objectType, true
	}
	return "", false
}

func formLanguageParam(args map[string]any) string {
	for _, k := range []string{"language", "layout_language"} {
		if v := strings.TrimSpace(getStringParam(args, k)); v != "" {
			return v
		}
	}
	return ""
}

func (s *Server) handleReadForm(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	args := request.GetArguments()
	formType, name := getStringParam(args, "type"), getStringParam(args, "name")
	if name == "" {
		return newToolResultError(fmt.Sprintf(`a form name is required: SAP(action="read", target="%s ZDEMO")`, formType)), nil
	}
	if err := s.adtClient.Safety().CheckOperation(adt.OpRead, "ReadForm"); err != nil {
		return newToolResultError(err.Error()), nil
	}
	if err := s.ensureDebugWSClient(ctx); err != nil {
		return newToolResultError(fmt.Sprintf("reading a form needs ZADT_VSP's form service (a WebSocket to the system): %v", err)), nil
	}

	if info, _ := getBoolParam(args, "info"); info {
		res, err := s.adtClient.GetFormInfo(ctx, s.debugWSClient, formType, name)
		if err != nil {
			return newToolResultError(fmt.Sprintf("%s %s: %v", formType, name, err)), nil
		}
		return newToolResultJSON(res), nil
	}

	form, err := s.adtClient.ReadForm(ctx, s.debugWSClient, formType, name, formLanguageParam(args))
	if err != nil {
		return newToolResultError(fmt.Sprintf("reading %s %s: %v", formType, name, err)), nil
	}
	if path := getStringParam(args, "file_path"); path != "" {
		if err := os.WriteFile(path, []byte(form.Content), 0o644); err != nil {
			return newToolResultError(fmt.Sprintf("writing %s: %v", path, err)), nil
		}
		return newToolResultJSON(map[string]any{
			"type": form.Type, "name": form.Name, "language": form.Language,
			"masterLanguage": form.MasterLanguage, "mimeType": form.MimeType,
			"file": path, "bytes": len(form.Content),
		}), nil
	}
	return newToolResultJSON(form), nil
}

func (s *Server) handleWriteForm(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	args := request.GetArguments()
	opts := adt.FormWriteOptions{
		Type:      getStringParam(args, "type"),
		Name:      getStringParam(args, "name"),
		Language:  formLanguageParam(args),
		Transport: transportParam(args),
		Package:   getStringParam(args, "package"),
	}
	opts.TestRun, _ = getBoolParam(args, "test_run")
	if create, _ := getBoolParam(args, "create"); create && opts.Package == "" {
		return newToolResultError(fmt.Sprintf(`creating %s %s needs a package: params={"package": "$TMP", ...}`, opts.Type, opts.Name)), nil
	}
	if opts.Name == "" {
		return newToolResultError(fmt.Sprintf(`a form name is required: SAP(action="edit", target="%s ZDEMO", params={"content": "..."})`, opts.Type)), nil
	}

	opts.Content = getStringParam(args, "content")
	if opts.Content == "" {
		opts.Content = getStringParam(args, "source")
	}
	if path := getStringParam(args, "file_path"); opts.Content == "" && path != "" {
		data, err := os.ReadFile(path)
		if err != nil {
			return newToolResultError(fmt.Sprintf("reading %s: %v", path, err)), nil
		}
		opts.Content = string(data)
	}
	if opts.Content == "" {
		return newToolResultError(`the form content is required: params={"content": "<?xml ..."} or {"file_path": "..."}, as SAP(action="read") returns it`), nil
	}

	// Refused here, before a WebSocket is opened, under --read-only and the
	// other operation filters; the package and request are checked by
	// WriteForm once the form's package is known.
	if err := s.adtClient.Safety().CheckOperation(adt.OpUpdate, "WriteForm"); err != nil {
		return newToolResultError(err.Error()), nil
	}
	if opts.Transport != "" {
		if err := s.checkTransportWrite("WriteForm", opts.Transport); err != nil {
			return newToolResultError(err.Error()), nil
		}
	}
	if err := s.ensureDebugWSClient(ctx); err != nil {
		return newToolResultError(fmt.Sprintf("writing a form needs ZADT_VSP's form service (a WebSocket to the system): %v", err)), nil
	}

	res, err := s.adtClient.WriteForm(ctx, s.debugWSClient, opts)
	if err != nil {
		msg := fmt.Sprintf("writing %s %s: %v", opts.Type, opts.Name, err)
		if res != nil {
			out, _ := json.MarshalIndent(res, "", "  ")
			msg += "\n" + string(out)
		}
		return newToolResultError(msg), nil
	}
	return newToolResultJSON(res), nil
}
