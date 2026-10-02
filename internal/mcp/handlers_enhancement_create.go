package mcp

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"strings"

	"github.com/mark3labs/mcp-go/mcp"

	"github.com/oisee/vibing-steampunk/pkg/adt"
)

// enhancedObjectURL resolves the object to enhance from the call's
// parameters: object_url as given, or a function module (its group is looked
// up in TFDIR), a function group, a program or a class by name.
func (s *Server) enhancedObjectURL(ctx context.Context, args map[string]any) (string, error) {
	if u := strings.TrimSpace(getStringParam(args, "object_url")); u != "" {
		return u, nil
	}
	esc := func(n string) string { return url.PathEscape(strings.ToLower(strings.TrimSpace(n))) }
	if fm := strings.ToUpper(strings.TrimSpace(getStringParam(args, "function_module"))); fm != "" {
		sql := fmt.Sprintf("SELECT PNAME FROM TFDIR WHERE FUNCNAME = '%s'", strings.ReplaceAll(fm, "'", "''"))
		res, err := s.adtClient.GetTableContents(ctx, "TFDIR", 1, sql)
		if err != nil {
			return "", fmt.Errorf("looking up the function group of %s: %w", fm, err)
		}
		if res == nil || len(res.Rows) == 0 {
			return "", fmt.Errorf("function module %s does not exist", fm)
		}
		return "/sap/bc/adt/functions/groups/" + esc(functionGroupOf(fmt.Sprint(res.Rows[0]["PNAME"]))), nil
	}
	if g := getStringParam(args, "function_group"); g != "" {
		return "/sap/bc/adt/functions/groups/" + esc(g), nil
	}
	if p := getStringParam(args, "program"); p != "" {
		return "/sap/bc/adt/programs/programs/" + esc(p), nil
	}
	if c := getStringParam(args, "class"); c != "" {
		return "/sap/bc/adt/oo/classes/" + esc(c), nil
	}
	return "", fmt.Errorf("the object to enhance is required: object_url, function_module, function_group, program or class")
}

// functionGroupOf turns a function pool (SAPLGROUP, /NS/SAPLGROUP) back into
// its group.
func functionGroupOf(pool string) string {
	pool = strings.ToUpper(strings.TrimSpace(pool))
	if strings.HasPrefix(pool, "/") {
		if i := strings.Index(pool[1:], "/"); i >= 0 {
			return pool[:i+2] + strings.TrimPrefix(pool[i+2:], "SAPL")
		}
	}
	return strings.TrimPrefix(pool, "SAPL")
}

// handleEnhancementOptions lists where an object can be enhanced:
// SAP(action="read", target="ENHANCEMENT_OPTIONS", params={"function_module": "BAPI_X"}).
// "filter" keeps the options whose name or description contains it; for a
// function module it defaults to the module, since its group lists them all.
func (s *Server) handleEnhancementOptions(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	args := request.GetArguments()
	objectURL, err := s.enhancedObjectURL(ctx, args)
	if err != nil {
		return newToolResultError(err.Error()), nil
	}
	options, err := s.adtClient.EnhancementOptions(ctx, objectURL)
	if err != nil {
		return newToolResultError(err.Error()), nil
	}
	filter := strings.ToUpper(getStringParam(args, "filter"))
	if filter == "" {
		filter = strings.ToUpper(strings.TrimSpace(getStringParam(args, "function_module")))
	}
	if filter != "" {
		kept := options[:0]
		for _, o := range options {
			if strings.Contains(strings.ToUpper(o.FullName), filter) || strings.Contains(strings.ToUpper(o.Description), filter) {
				kept = append(kept, o)
			}
		}
		options = kept
	}
	return newToolResultJSON(map[string]any{"object_url": objectURL, "options": options}), nil
}

// handleCreateSourceCodePlugin creates an ENHO implementing one enhancement
// option, and with "source" also writes and activates its code:
// SAP(action="create", target="ENHO", params={"name": "ZENH_DEMO", "description": "...",
//
//	"package": "ZPKG", "function_module": "BAPI_X", "option": "\\FU:BAPI_X\\SE:BEGIN\\EI"}).
func (s *Server) handleCreateSourceCodePlugin(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	// Refused before the enhanced object is resolved and its options read:
	// on a read-only server those reads lead to nothing but the refusal the
	// client gives later, and a "no such option" answer would hide it.
	if err := s.adtClient.Safety().CheckOperation(adt.OpCreate, "CreateSourceCodePlugin"); err != nil {
		return newToolResultError(err.Error()), nil
	}
	args := request.GetArguments()
	objectURL, err := s.enhancedObjectURL(ctx, args)
	if err != nil {
		return newToolResultError(err.Error()), nil
	}
	opts := adt.SourceCodePluginOptions{
		Name:        getStringParam(args, "name"),
		Description: getStringParam(args, "description"),
		Package:     firstNonEmptyParam(args, "package", "package_name"),
		Transport:   getStringParam(args, "transport"),
		ObjectURL:   objectURL,
		Option:      getStringParam(args, "option"),
		Source:      getStringParam(args, "source"),
	}
	// The mode decides a static or dynamic plug-in; take it from the option
	// itself rather than make the caller repeat it.
	// Without the list neither the option's existence nor its mode is known,
	// and a wrong mode makes a plug-in of the wrong kind: stop there.
	options, oerr := s.adtClient.EnhancementOptions(ctx, objectURL)
	if oerr != nil {
		return newToolResultError(fmt.Sprintf("cannot check the enhancement option: %v", oerr)), nil
	}
	found := false
	for _, o := range options {
		if o.FullName == opts.Option {
			opts.Mode, found = o.Mode, true
			break
		}
	}
	if !found {
		return newToolResultError(fmt.Sprintf("%s has no enhancement option %s; list them with SAP(action=\"read\", target=\"ENHANCEMENT_OPTIONS\")", objectURL, opts.Option)), nil
	}
	enhoURL, err := s.adtClient.CreateSourceCodePlugin(ctx, opts)
	if err != nil {
		if enhoURL != "" {
			return enhancementPartial(map[string]any{"object_url": enhoURL, "enhanced_object": objectURL, "option": opts.Option}, err.Error()), nil
		}
		return newToolResultError(err.Error()), nil
	}
	out := map[string]any{"object_url": enhoURL, "enhanced_object": objectURL, "option": opts.Option}
	if opts.Source == "" {
		out["message"] = "created inactive with an empty ENHANCEMENT block; write the code to " + enhoURL + "/source/main"
		return newToolResultJSON(out), nil
	}
	return s.activateEnhancement(ctx, out, enhoURL, opts.Name, "created and written"), nil
}

// activateEnhancement activates a new ENHO and reports it. SAP refuses an
// activation in the answer, not with an HTTP error, so Success is checked
// too: an inactive ENHO is not a completed create.
func (s *Server) activateEnhancement(ctx context.Context, out map[string]any, enhoURL, name, done string) *mcp.CallToolResult {
	activation, err := s.adtClient.Activate(ctx, enhoURL, strings.ToUpper(name))
	out["activation"] = activation
	switch {
	case err != nil:
		return enhancementPartial(out, fmt.Sprintf("%s, but activation failed: %v", done, err))
	case activation != nil && !activation.Success:
		return enhancementPartial(out, fmt.Sprintf("%s, but SAP did not activate it: %s", done, strings.Join(activation.ProblemLines(), "; ")))
	}
	return newToolResultJSON(out)
}

// enhancementPartial is a result for an ENHO that exists but is not complete:
// what was done, and the error, marked as one.
func enhancementPartial(out map[string]any, msg string) *mcp.CallToolResult {
	out["error"] = msg
	res := newToolResultJSON(out)
	res.IsError = true
	return res
}

// handleCreateBadiImplementation creates an ENHO with one BAdI implementation
// and activates it:
// SAP(action="create", target="BADI_IMPL", params={"name": "ZENH_DEMO", "description": "...",
//
//	"package": "ZPKG", "spot": "BADI_X", "class": "ZCL_DEMO_BADI"}).
//
// "badi" names the BAdI when the spot holds more than one; "active": false
// creates the implementation switched off; "activate": false leaves the ENHO
// inactive.
func (s *Server) handleCreateBadiImplementation(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	args := request.GetArguments()
	opts := adt.BadiImplementationOptions{
		Name:              getStringParam(args, "name"),
		Description:       getStringParam(args, "description"),
		Package:           firstNonEmptyParam(args, "package", "package_name"),
		Transport:         getStringParam(args, "transport"),
		Spot:              firstNonEmptyParam(args, "spot", "enhancement_spot"),
		BadiDefinition:    firstNonEmptyParam(args, "badi", "badi_definition"),
		ImplementingClass: firstNonEmptyParam(args, "class", "implementing_class"),
		Implementation:    getStringParam(args, "implementation"),
		ShortText:         getStringParam(args, "short_text"),
	}
	if active, ok := getBoolParam(args, "active"); ok && !active {
		opts.Inactive = true
	}
	enhoURL, err := s.adtClient.CreateBadiImplementation(ctx, opts)
	if err != nil {
		// The ENHO was created but the implementation could not be added:
		// say whether the empty container was taken away again, and what is
		// left to do if not.
		var pce *adt.PartialCreateError
		if errors.As(err, &pce) {
			out := map[string]any{"cleanup_actions": pce.CleanupActions}
			if enhoURL != "" {
				out["object_url"] = enhoURL
			}
			if len(pce.ManualSteps) > 0 {
				out["manual_steps"] = pce.ManualSteps
			}
			return enhancementPartial(out, err.Error()), nil
		}
		if enhoURL != "" {
			return enhancementPartial(map[string]any{"object_url": enhoURL}, err.Error()), nil
		}
		return newToolResultError(err.Error()), nil
	}
	out := map[string]any{"object_url": enhoURL, "spot": strings.ToUpper(opts.Spot), "class": strings.ToUpper(opts.ImplementingClass)}
	if activate, ok := getBoolParam(args, "activate"); ok && !activate {
		out["message"] = "created inactive; activate " + enhoURL + " to put the implementation in force"
		return newToolResultJSON(out), nil
	}
	return s.activateEnhancement(ctx, out, enhoURL, opts.Name, "created"), nil
}

func firstNonEmptyParam(args map[string]any, keys ...string) string {
	for _, k := range keys {
		if v := strings.TrimSpace(getStringParam(args, k)); v != "" {
			return v
		}
	}
	return ""
}
