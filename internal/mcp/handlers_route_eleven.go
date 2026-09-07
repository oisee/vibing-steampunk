package mcp

// The last eleven, routed.
//
// Measured before written: of 147 tools registered in expert mode, the
// universal SAP() tool could reach 126. Of the twenty-one it could not, ten are
// gCTS — `registerGCTSTools` is defined and called from nowhere, so they are
// registered by no mode and have been dead since they landed. The live
// remainder is **eleven**: seven translation tools, three revision-history
// tools, and the ABAP lint.
//
// Eleven is therefore the entire cost of retiring focused and expert modes, and
// this file is that cost paid. Nothing new is built here — every handler
// already existed and was already advertised as a tool. What changes is that an
// agent in the mode that ships can reach them.
//
// The routing tables are maps rather than switches for the same reason as
// analysisTypes: the surface can then be enumerated without calling into SAP,
// which is what lets `vsp sweep` check that every advertised action is claimed
// by some route.

import (
	"context"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

// i18nTypes are the translation operations, addressed as
// SAP(action="i18n", params={"op": "..."}).
func (s *Server) i18nTypes() map[string]server.ToolHandlerFunc {
	return map[string]server.ToolHandlerFunc{
		"texts":               s.handleGetObjectTextsInLanguage,
		"data_element_labels": s.handleGetDataElementLabels,
		"message_class_texts": s.handleGetMessageClassTexts,
		"text_pool":           s.handleTextsGet,
		"texts_get":           s.handleTextsGet,
		"texts_set":           s.handleTextsSet,
		"compare_languages":   s.handleCompareObjectLanguages,
		"write_labels":        s.handleWriteDataElementLabels,
		"write_message_texts": s.handleWriteMessageClassTexts,
		"write_text_pool":     s.handleTextsSet,
	}
}

// revisionTypes are the version-history operations, addressed as
// SAP(action="revisions", params={"op": "..."}).
func (s *Server) revisionTypes() map[string]server.ToolHandlerFunc {
	return map[string]server.ToolHandlerFunc{
		"list":    s.handleGetRevisions,
		"source":  s.handleGetRevisionSource,
		"compare": s.handleCompareVersions,
	}
}

// routeI18nAction routes action="i18n".
func (s *Server) routeI18nAction(ctx context.Context, action, objectType, objectName string, params map[string]any) (*mcp.CallToolResult, bool, error) {
	if action != "i18n" {
		return nil, false, nil
	}
	op := firstParam(params, "op")
	if objectName != "" && firstParam(params, "object_name") == "" {
		params["object_type"], params["object_name"] = objectType, objectName
	}
	handler, known := s.i18nTypes()[op]
	if !known {
		// The action is recognised, so it owns the answer. Falling through
		// would tell a caller that action="i18n" does not exist, which is the
		// defect that made query and grep look missing.
		return needParams("i18n", params, i18nOps(),
			`SAP(action="i18n", params={"op": "texts", "object_url": "/sap/bc/adt/oo/classes/zcl_demo", "language": "DE"})
  SAP(action="i18n", params={"op": "compare_languages", "object_url": "...", "languages": "EN,DE"})`), true, nil
	}
	return s.callHandler(ctx, handler, params)
}

// routeRevisionsAction routes action="revisions".
func (s *Server) routeRevisionsAction(ctx context.Context, action, objectType, objectName string, params map[string]any) (*mcp.CallToolResult, bool, error) {
	if action != "revisions" && action != "history" {
		return nil, false, nil
	}
	// `op` only. Not `type`, which these handlers read as the *object* type —
	// consuming it as the operation selector made the one call form the handler
	// accepts unroutable, and the error then printed `type="CLAS"` back at the
	// caller while demanding something else. `type` is the most overloaded key
	// in this codebase; it must never double as a selector.
	op := firstParam(params, "op")
	if op == "" {
		op = "list" // the question somebody asking for history means first
	}
	// `target="CLAS ZCL_DEMO"` is the form the tool description teaches for
	// everything else, and these routes were throwing it away — a caller who
	// followed the documentation got "type and name are required".
	if objectType != "" && objectName != "" {
		params = copyParams(params)
		if firstParam(params, "type") == "" {
			params["type"] = objectType
		}
		if firstParam(params, "name") == "" {
			params["name"] = objectName
		}
	}
	handler, known := s.revisionTypes()[op]
	if !known {
		return needParams("revisions", params, []string{"list", "source", "compare"},
			`SAP(action="revisions", params={"op": "list", "object_type": "CLAS", "object_name": "ZCL_DEMO"})`), true, nil
	}
	return s.callHandler(ctx, handler, params)
}

// routeLintAction routes action="lint" to the offline ABAP analyser.
//
// It is also reachable as analyze type=lint, because a caller looking for
// static analysis reaches for "analyze" first and finding nothing there is how
// a capability comes to look missing.
func (s *Server) routeLintAction(ctx context.Context, action, objectType, objectName string, params map[string]any) (*mcp.CallToolResult, bool, error) {
	types := s.lintTypes()
	if action == "analyze" {
		handler, ok := types[firstParam(params, "type")]
		if !ok {
			return nil, false, nil
		}
		return s.callHandler(ctx, handler, params)
	}
	handler, ok := types[action]
	if !ok {
		return nil, false, nil
	}
	return s.callHandler(ctx, handler, params)
}

// lintTypes is the lint router's table.
//
// It exists so that three things read from one place instead of three: the
// router, the advertised analyze-type list, and the reach check. Before it,
// `analyze type=lint` was written by hand into the advertised set, matched by
// hand in the router, and absent from AnalyzeTypes() — so the sweep reported it
// unreachable while the call worked, and the tool listed a surface its own
// documentation contradicted.
func (s *Server) lintTypes() map[string]server.ToolHandlerFunc {
	return map[string]server.ToolHandlerFunc{
		"lint": s.handleAnalyzeABAPCode,
	}
}

// i18nOps lists the operations, sorted, for the message a wrong one earns.
func i18nOps() []string {
	return []string{
		"texts", "data_element_labels", "message_class_texts", "text_pool",
		"compare_languages", "write_labels", "write_message_texts", "texts_get", "texts_set",
	}
}
