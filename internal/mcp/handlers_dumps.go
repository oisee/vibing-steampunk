// Package mcp provides the MCP server implementation for ABAP ADT tools.
// handlers_dumps.go contains handlers for runtime errors (short dumps / RABAX)
// and for the application log. The two belong in one file because they answer
// one question between them: the dump says what died, the log says what the
// code thought it was doing at the time, and correlating them is the whole
// point of both.
package mcp

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/oisee/vibing-steampunk/pkg/adt"
)

// dumpAnalysisTypes maps every "analyze" type answered here to its handler.
//
// A map rather than a switch because a map can be enumerated and a switch
// cannot. Everything in this file exists because the CLI grew a post-mortem
// surface that the MCP server never learned about; the test that walks these
// keys against the help text is what stops the same drift starting again.
func (s *Server) dumpAnalysisTypes() map[string]func(context.Context, mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	return map[string]func(context.Context, mcp.CallToolRequest) (*mcp.CallToolResult, error){
		"list_dumps":      s.handleListDumps,
		"group_dumps":     s.handleGroupDumps,
		"get_dump":        s.handleGetDump,
		"explain_dump":    s.handleExplainDump,
		"similar_dumps":   s.handleSimilarDumps,
		"dump_impact":     s.handleDumpImpact,
		"application_log": s.handleApplicationLog,
		"cluster_read":    s.handleClusterRead,
		"spool_list":      s.handleSpoolList,
		"spool_read":      s.handleSpoolRead,
		"job_list":        s.handleJobList,
		"job_log":         s.handleJobLog,
		"variants":        s.handleVariants,
		"fm_test_data":    s.handleFunctionTestData,
		"documentation":   s.handleDocumentation,
		"img_search":      s.handleIMGSearch,
		"img_activity":    s.handleIMGActivity,
	}
}

// routeDumpsAction routes "analyze" with dump-related and application-log types.
func (s *Server) routeDumpsAction(ctx context.Context, action, objectType, objectName string, params map[string]any) (*mcp.CallToolResult, bool, error) {
	if action != "analyze" {
		return nil, false, nil
	}
	handler, known := s.dumpAnalysisTypes()[getStringParam(params, "type")]
	if !known {
		return nil, false, nil
	}
	return s.callHandler(ctx, handler, params)
}

// The caveats below are printed by the CLI on stderr, where a person reads them
// and calibrates. An agent has no stderr, so they travel in the payload — and
// they have to, because every one of them is the difference between a ranking
// and a verdict.
const (
	noteCorrelationIsNotCause = "Ranked by the argument for each, not by nearness in time. A match is a candidate, not a cause."
	noteNoCallGraph           = "One rung is missing: this system serves no call graph, so \"written by something a stack frame calls\" was never asked."
	noteRungIsNotAVerdict     = "A rung is an argument, not a verdict. Rung 4 is the same class of failure; it is not the same bug."
	noteImpactIsNotBlame      = "Who can reach the bug, not who caused it. Object level: the where-used list resolves a method to its class, so a caller here reaches the class and not necessarily the failing method."
	noteImpactUnanswerable    = "No unit of this dump has a where-used list that can answer. This is not a finding of zero callers."
	noteNoLogBodies           = "Headers only. The messages live in the BALDAT cluster and are decoded on request: pass messages=true, or read one log with type=cluster_read, table=BALDAT, layout=applog."
	noteLogTextsMissing       = "Message texts could not be read from T100; class, number and variables are still here."
)

// --- Listing and grouping ---

type dumpListResult struct {
	Dumps []adt.Dump `json:"dumps"`
	Count int        `json:"count"`
	Notes []string   `json:"notes,omitempty"`
}

func (s *Server) handleListDumps(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	filter, notes, err := dumpFilterFrom(request.GetArguments())
	if err != nil {
		return newToolResultError(err.Error()), nil
	}
	// The MCP default was whatever pkg/adt used for the CLI, which is a
	// hundred. A hundred dump rows is ten thousand tokens that stay in the
	// context for the rest of the session, and the question people actually ask
	// this is "what broke recently".
	//
	// Asking for one more than we mean to show costs nothing — the feed is
	// fetched and parsed whole either way — and it is what lets the answer tell
	// "twenty, and that is all there are" from "twenty of more".
	capped := filter.Limit == 0
	if capped {
		filter.Limit = defaultDumps + 1
	}
	dumps, err := s.adtClient.Dumps(ctx, filter)
	if err != nil {
		return newToolResultError(fmt.Sprintf("Failed to get dumps: %v", err)), nil
	}
	if capped && len(dumps) > defaultDumps {
		dumps = dumps[:defaultDumps]
		notes = append(notes, truncationNoteUnknownTotal(defaultDumps, "max_results",
			"narrow the window with since/until"))
	}
	return newToolResultJSON(dumpListResult{Dumps: dumps, Count: len(dumps), Notes: notes}), nil
}

type dumpGroupResult struct {
	Groups []adt.DumpGroup `json:"groups"`
	// Dumps is how many dumps the groups were built from, which is the number
	// that says whether the grouping saw the whole picture or just the first
	// page of it.
	Dumps int      `json:"dumps"`
	Notes []string `json:"notes,omitempty"`
}

func (s *Server) handleGroupDumps(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	filter, notes, err := dumpFilterFrom(request.GetArguments())
	if err != nil {
		return newToolResultError(err.Error()), nil
	}
	dumps, err := s.adtClient.Dumps(ctx, filter)
	if err != nil {
		return newToolResultError(fmt.Sprintf("Failed to get dumps: %v", err)), nil
	}
	return newToolResultJSON(dumpGroupResult{
		Groups: adt.GroupDumps(dumps),
		Dumps:  len(dumps),
		Notes:  notes,
	}), nil
}

// --- One dump ---

type dumpDetailResult struct {
	Dump   adt.Dump        `json:"dump"`
	Detail *adt.DumpDetail `json:"detail,omitempty"`
	Notes  []string        `json:"notes,omitempty"`
}

func (s *Server) handleGetDump(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	args := request.GetArguments()
	dump, notes, err := s.resolveDump(ctx, args)
	if err != nil {
		return newToolResultError(err.Error()), nil
	}

	detail, err := s.adtClient.DumpDetail(ctx, dump.ID)
	switch {
	case errors.Is(err, adt.ErrDumpDetailUnavailable):
		// Not a failure and not reported as one: the feed entry is still a true
		// and complete answer to "what dumped", and a release that serves the
		// feed without the detail resource is a fact about the release.
		notes = append(notes, err.Error())
	case err != nil:
		return newToolResultError(fmt.Sprintf("Failed to get dump: %v", err)), nil
	}
	return newToolResultJSON(dumpDetailResult{Dump: dump, Detail: detail, Notes: notes}), nil
}

type dumpExplainResult struct {
	Dump    adt.Dump        `json:"dump"`
	Stack   []adt.DumpFrame `json:"stack,omitempty"`
	Matches []adt.LogMatch  `json:"matches"`
	Notes   []string        `json:"notes,omitempty"`
}

func (s *Server) handleExplainDump(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	args := request.GetArguments()
	dump, notes, err := s.resolveDump(ctx, args)
	if err != nil {
		return newToolResultError(err.Error()), nil
	}

	matches, err := s.adtClient.CorrelateDump(ctx, dump, toleranceFrom(args), correlationLimitFrom(args))
	if err != nil {
		return newToolResultError(fmt.Sprintf("Failed to correlate dump: %v", err)), nil
	}

	// Read again for the payload; the correlation used the stack for ranking
	// but does not hand it back, and the stack is half of what makes the
	// ranking arguable by whoever reads it.
	stack, stackErr := s.adtClient.DumpStack(ctx, dump.ID)
	switch {
	case errors.Is(stackErr, adt.ErrDumpDetailUnavailable):
		notes = append(notes, stackErr.Error())
	case stackErr != nil:
		notes = append(notes, fmt.Sprintf("the call stack could not be read: %v", stackErr))
	}

	notes = append(notes, noteCorrelationIsNotCause)
	// Asked only when there is a ranking to qualify, and asked at all because a
	// rung that silently never fires reads as a rung that found nothing.
	if len(matches) > 0 && s.adtClient.CalleesUnavailable(ctx) {
		notes = append(notes, noteNoCallGraph)
	}

	return newToolResultJSON(dumpExplainResult{Dump: dump, Stack: stack, Matches: matches, Notes: notes}), nil
}

type dumpSimilarResult struct {
	Dump    adt.Dump          `json:"dump"`
	Detail  *adt.DumpDetail   `json:"detail,omitempty"`
	Rungs   []adt.RungSummary `json:"rungs"`
	Matches []adt.DumpMatch   `json:"matches"`
	Notes   []string          `json:"notes,omitempty"`
}

// handleSimilarDumps answers "is this new, and how often does it happen".
//
// The shape follows the CLI: the two rungs that need only the feed always run,
// and the two that need the dump detail are paid for out of a budget. A system
// that refuses details still answers, on a shorter ladder, rather than failing.
func (s *Server) handleSimilarDumps(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	args := request.GetArguments()
	filter, notes, err := dumpFilterFrom(args)
	if err != nil {
		return newToolResultError(err.Error()), nil
	}
	dumps, err := s.adtClient.Dumps(ctx, filter)
	if err != nil {
		return newToolResultError(fmt.Sprintf("Failed to get dumps: %v", err)), nil
	}
	subject, found := adt.FindDump(dumps, dumpSelectorFrom(args))
	if !found {
		return newToolResultError(noSuchDump(dumpSelectorFrom(args))), nil
	}

	budget := deepBudgetFrom(args)
	details := map[string]*adt.DumpDetail{}

	if budget > 0 {
		// The subject's own detail is read first and outside the budget: with
		// no line and no component for the dump being asked about, no candidate
		// can reach rung 1 or rung 3 however many are read.
		detail, derr := s.adtClient.DumpDetail(ctx, subject.ID)
		switch {
		case errors.Is(derr, adt.ErrDumpDetailUnavailable):
			notes = append(notes, derr.Error()+" Rungs 1 and 3 need it, so this ranks on rungs 2 and 4 only.")
			budget = 0
		case derr != nil:
			notes = append(notes, fmt.Sprintf("the detail of the dump itself could not be read (%v), so rungs 1 and 3 are unavailable", derr))
			budget = 0
		default:
			details[subject.ID] = detail
		}
	}

	read := 0
	for _, candidate := range adt.DeepenOrder(subject, dumps) {
		if read >= budget {
			break
		}
		read++
		detail, derr := s.adtClient.DumpDetail(ctx, candidate.ID)
		if derr != nil {
			// One unreadable candidate cannot climb past rung 2, and that is
			// all it costs. The budget is still spent, because a system
			// answering slowly with errors is where an unbounded retry hurts
			// most.
			continue
		}
		details[candidate.ID] = detail
	}

	signatures := make([]adt.DumpSignature, 0, len(dumps))
	for _, d := range dumps {
		signatures = append(signatures, adt.SignatureOf(d, details[d.ID]))
	}
	matches := adt.RankSimilarDumps(adt.SignatureOf(subject, details[subject.ID]), signatures)

	notes = append(notes, noteRungIsNotAVerdict)
	return newToolResultJSON(dumpSimilarResult{
		Dump:    subject,
		Detail:  details[subject.ID],
		Rungs:   adt.SummarizeSimilar(matches),
		Matches: matches,
		Notes:   notes,
	}), nil
}

type dumpImpactResult struct {
	*adt.DumpImpactResult
	Notes []string `json:"notes,omitempty"`
}

func (s *Server) handleDumpImpact(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	args := request.GetArguments()
	dump, notes, err := s.resolveDump(ctx, args)
	if err != nil {
		return newToolResultError(err.Error()), nil
	}

	opts := adt.DumpImpactOptions{}
	if n, ok := firstNumber(args, "frames", "impact_frames", "max_units"); ok && n > 0 {
		opts.MaxUnits = int(n)
	}
	if n, ok := firstNumber(args, "top", "impact_top", "limit"); ok && n > 0 {
		opts.Limit = int(n)
	}

	result, err := s.adtClient.DumpImpact(ctx, dump, opts)
	if err != nil {
		return newToolResultError(fmt.Sprintf("Failed to compute dump impact: %v", err)), nil
	}
	if result.StackUnavailable {
		notes = append(notes, "the call stack could not be read, so only the dump's own program was asked about")
	}
	if !result.Answerable() {
		notes = append(notes, noteImpactUnanswerable)
	} else if unanswered := unansweredUnits(result); len(unanswered) > 0 {
		// Answerable() is all-or-nothing: one unit that came back is enough to
		// make it true. But "exposed" is the union over units, so a unit that
		// failed subtracts callers from the headline list without subtracting
		// anything from the reader's confidence in it.
		notes = append(notes, fmt.Sprintf(
			"%d of %d units could not be asked about, so the exposure below is a floor, not a total: %s",
			len(unanswered), len(result.Units), strings.Join(unanswered, "; ")))
	}
	notes = append(notes, noteImpactIsNotBlame)

	return newToolResultJSON(dumpImpactResult{DumpImpactResult: result, Notes: notes}), nil
}

// unansweredUnits names the units whose where-used list is missing from the
// answer, with the reason each is missing.
func unansweredUnits(result *adt.DumpImpactResult) []string {
	var out []string
	for _, u := range result.Units {
		switch {
		case u.Err != "":
			out = append(out, u.Object+" ("+u.Err+")")
		case u.Note != "":
			out = append(out, u.Object+" ("+u.Note+")")
		}
	}
	return out
}

// --- Application log ---

type appLogResult struct {
	Entries []adt.AppLogEntry `json:"entries"`
	Count   int               `json:"count"`
	Notes   []string          `json:"notes,omitempty"`
}

func (s *Server) handleApplicationLog(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	filter, err := appLogFilterFrom(request.GetArguments())
	if err != nil {
		return newToolResultError(err.Error()), nil
	}
	entries, err := s.adtClient.ApplicationLog(ctx, filter)
	if err != nil {
		return newToolResultError(fmt.Sprintf("Failed to read the application log: %v", err)), nil
	}
	notes := []string{noteNoLogBodies}
	if withMessages, _ := getBoolParam(request.GetArguments(), "messages"); withMessages {
		notes = nil
		if err := s.adtClient.AttachAppLogMessages(ctx, s.adtClient.Language(), entries); err != nil {
			if !strings.Contains(err.Error(), "message texts") {
				return newToolResultError(fmt.Sprintf("Failed to read the log messages: %v", err)), nil
			}
			notes = append(notes, noteLogTextsMissing)
		}
	}
	return newToolResultJSON(appLogResult{
		Entries: entries,
		Count:   len(entries),
		Notes:   notes,
	}), nil
}

// --- Parameters ---

// resolveDump picks the one dump a post-mortem type is about.
//
// Ids carry the instance name, the user and a counter, so nobody quotes one
// whole; "latest" or the timestamp prefix from a listing is what an agent
// actually has. A dump_id that is already a full id skips the listing, because
// an id from an older conversation would otherwise fall outside the window and
// look like it had ceased to exist.
func (s *Server) resolveDump(ctx context.Context, args map[string]any) (adt.Dump, []string, error) {
	which := dumpSelectorFrom(args)
	filter, notes, err := dumpFilterFrom(args)
	if err != nil {
		return adt.Dump{}, nil, err
	}

	full := strings.Contains(which, "/runtime/dump")

	dumps, listErr := s.adtClient.Dumps(ctx, filter)
	if listErr != nil && !full {
		return adt.Dump{}, nil, fmt.Errorf("Failed to get dumps: %w", listErr)
	}
	if listErr == nil {
		if dump, found := adt.FindDump(dumps, which); found {
			return dump, notes, nil
		}
		if !full {
			return adt.Dump{}, nil, errors.New(noSuchDump(which))
		}
	}

	// A full id that the listing does not carry. This is the case the shortcut
	// existed for — an id quoted from an older conversation falls outside the
	// window and would otherwise look as though it had ceased to exist.
	//
	// What the shortcut got wrong was returning nothing but the id. Three
	// post-mortem types then worked with a Dump that had no time, no program
	// and no error type, and explain_dump failed outright with "this dump
	// carries no timestamp" — of a dump whose timestamp is the first fourteen
	// characters of the id it was handed.
	dump := adt.Dump{ID: which, At: adt.DumpTimeFromID(which)}
	if dump.At.IsZero() {
		notes = append(notes, "this id carries no timestamp and the dump is not in the current listing, "+
			"so anything that needs a time — correlation, similarity by recency — cannot run on it")
	} else {
		notes = append(notes, "this dump is not in the current listing; its time was read from the id, "+
			"and the program and error type are unknown, which narrows what can be correlated")
	}
	return dump, notes, nil
}

func noSuchDump(which string) string {
	return fmt.Sprintf("no dump in this range has an id containing %q — pass \"latest\", part of an id from "+
		"analyze type=list_dumps, or widen the window with since/until", which)
}

func dumpSelectorFrom(args map[string]any) string {
	if which := firstString(args, "dump_id", "dump", "which"); which != "" {
		return which
	}
	return "latest"
}

// dumpFilterFrom builds the listing filter, and returns notes for anything the
// caller asked for that this resource cannot do.
func dumpFilterFrom(args map[string]any) (adt.DumpFilter, []string, error) {
	filter := adt.DumpFilter{
		Program: firstString(args, "program"),
		// exception_type is what the old MCP path called it. The name is kept
		// as an alias rather than broken, but error_type is what SAP labels the
		// category and what the CLI calls it.
		ErrorType: firstString(args, "error_type", "exception_type"),
		User:      firstString(args, "user"),
	}
	if n, ok := firstNumber(args, "max_results", "top", "limit"); ok && n > 0 {
		filter.Limit = int(n)
	}

	var err error
	if filter.From, err = boundFrom(args, false, "from", "since", "date_from"); err != nil {
		return adt.DumpFilter{}, nil, err
	}
	if filter.To, err = boundFrom(args, true, "to", "until", "date_to"); err != nil {
		return adt.DumpFilter{}, nil, err
	}

	var notes []string
	if pkg := firstString(args, "package"); pkg != "" {
		// Said out loud rather than dropped. The old path accepted a package
		// filter and built it into an OData $filter that the dumps feed ignores
		// entirely — verified on 7.58, where a filter naming a package that
		// exists nowhere returns every dump on the system. Silently ignoring it
		// again would preserve the same lie in a new place.
		notes = append(notes, fmt.Sprintf("package %q was ignored: the runtime-error feed offers no package filter, "+
			"and the one the old MCP path sent was never applied by the server. Filter by program instead.", pkg))
	}
	return filter, notes, nil
}

func appLogFilterFrom(args map[string]any) (adt.AppLogFilter, error) {
	filter := adt.AppLogFilter{
		Program:   firstString(args, "program"),
		User:      firstString(args, "user"),
		Object:    firstString(args, "object", "log_object"),
		SubObject: firstString(args, "subobject", "sub_object"),
	}
	if n, ok := firstNumber(args, "max_results", "top", "limit"); ok && n > 0 {
		filter.Limit = int(n)
	}

	var err error
	if filter.From, err = boundFrom(args, false, "from", "since", "date_from"); err != nil {
		return adt.AppLogFilter{}, err
	}
	if filter.To, err = boundFrom(args, true, "to", "until", "date_to"); err != nil {
		return adt.AppLogFilter{}, err
	}
	return filter, nil
}

// toleranceFrom reads the correlation window.
//
// Accepts a duration string because that is what the CLI takes, and a number of
// minutes because a JSON caller reaches for a number. Both are unambiguous;
// refusing one of them would only be a puzzle to solve at call time.
func toleranceFrom(args map[string]any) time.Duration {
	if raw := firstString(args, "tolerance"); raw != "" {
		if d, err := time.ParseDuration(strings.TrimSpace(raw)); err == nil && d > 0 {
			return d
		}
	}
	if n, ok := firstNumber(args, "tolerance_minutes"); ok && n > 0 {
		return time.Duration(n * float64(time.Minute))
	}
	return 5 * time.Minute
}

func correlationLimitFrom(args map[string]any) int {
	if n, ok := firstNumber(args, "matches", "match_limit"); ok && n > 0 {
		return int(n)
	}
	return 20
}

// deepBudgetFrom is how many candidate dumps similar_dumps may read in full.
//
// Ten where the CLI takes twenty-five, and lower on purpose: each read is a
// whole formatted dump — 45 KB to nearly a megabyte — and here they all happen
// inside one tool call that a client is timing. Rungs 2 and 4 need none of
// them, so a budget of 0 is a real answer and not a broken one.
func deepBudgetFrom(args map[string]any) int {
	if n, ok := firstNumber(args, "deep", "detail_budget"); ok {
		if n < 0 {
			return 0
		}
		return int(n)
	}
	return 10
}

// boundFrom reads a date bound under any of its accepted names.
//
// The complaint names the spelling the caller used, not the canonical one: an
// agent that passed date_from and is told "from is unreadable" will go looking
// for a parameter it never sent.
func boundFrom(args map[string]any, endOfDay bool, names ...string) (time.Time, error) {
	for _, name := range names {
		raw := firstString(args, name)
		if raw == "" {
			continue
		}
		when, err := parseDumpDate(raw, endOfDay)
		if err != nil {
			return time.Time{}, fmt.Errorf("%s: %w", name, err)
		}
		return when, nil
	}
	return time.Time{}, nil
}

// parseDumpDate accepts the three spellings a date arrives in here: the CLI's
// YYYY-MM-DD, the YYYYMMDD the old MCP path documented, and a full timestamp
// for a caller that wants an hour rather than a day.
//
// A date-only upper bound is stretched to the end of that day. Read literally
// it means midnight, which silently excludes the whole day the caller named —
// the kind of off-by-one that looks like missing data rather than a bug.
func parseDumpDate(raw string, endOfDay bool) (time.Time, error) {
	raw = strings.TrimSpace(raw)
	for _, layout := range []string{"2006-01-02", "20060102"} {
		if when, err := time.Parse(layout, raw); err == nil {
			if endOfDay {
				return when.Add(24*time.Hour - time.Second), nil
			}
			return when, nil
		}
	}
	for _, layout := range []string{time.RFC3339, "2006-01-02 15:04:05", "20060102150405"} {
		if when, err := time.Parse(layout, raw); err == nil {
			return when, nil
		}
	}
	return time.Time{}, fmt.Errorf("wants a date as YYYY-MM-DD (or a full timestamp), got %q", raw)
}

// firstString returns the first of the given keys that holds a non-empty
// string. Several names per parameter is how the old spellings keep working
// without a second copy of every handler.
func firstString(args map[string]any, names ...string) string {
	for _, name := range names {
		if v, ok := args[name].(string); ok {
			if v = strings.TrimSpace(v); v != "" {
				return v
			}
		}
	}
	return ""
}

// firstNumber returns the first of the given keys that holds a number. JSON
// numbers arrive as float64, but a client that sends a bare integer or a
// quoted one should not be told its filter is unsupported.
func firstNumber(args map[string]any, names ...string) (float64, bool) {
	for _, name := range names {
		switch v := args[name].(type) {
		case float64:
			return v, true
		case int:
			return float64(v), true
		case string:
			var parsed float64
			if _, err := fmt.Sscanf(strings.TrimSpace(v), "%g", &parsed); err == nil {
				return parsed, true
			}
		}
	}
	return 0, false
}
