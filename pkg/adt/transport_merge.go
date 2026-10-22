package adt

import (
	"context"
	"fmt"
	"strings"
)

// Merging two requests, and moving one object between them, is SE09's
// Utilities → Reorganize and nothing in ADT: the organizer's resources add
// objects, change owners and release, but none removes an entry. The
// function modules behind SE09 — TR_MERGE_REQUESTS, TR_APPEND_TO_COMM_OBJS_KEYS,
// TRINT_DELETE_COMM_OBJECT_KEYS — are not remote-enabled either. They are
// reachable through ZADT_VSP's CALL FUNCTION bridge, with their dialogs
// switched off, which is what these two do. Both need ZADT_VSP on the
// system and a WebSocket to it.

// TransportMergeResult is what a merge did.
type TransportMergeResult struct {
	From    string `json:"from"`
	To      string `json:"to"`
	Merged  bool   `json:"merged"`
	Message string `json:"message,omitempty"`
	// Objects are the target's objects after the merge.
	Objects []TransportObjectV2 `json:"objects,omitempty"`
	// SourceGone says the source request no longer exists, as a merge
	// leaves it.
	SourceGone bool `json:"sourceGone"`
}

// MergeTransports moves everything in from — its tasks and their objects —
// into to and deletes from, the way SE09's Merge Requests does. Both must
// be modifiable, of the same kind, and the caller must own them.
func (c *Client) MergeTransports(ctx context.Context, ws *DebugWebSocketClient, from, to string) (*TransportMergeResult, error) {
	from, to = strings.ToUpper(strings.TrimSpace(from)), strings.ToUpper(strings.TrimSpace(to))
	if from == "" || to == "" {
		return nil, fmt.Errorf("a source and a target request are required")
	}
	if from == to {
		return nil, fmt.Errorf("%s cannot be merged into itself", from)
	}
	for _, n := range []string{from, to} {
		if err := c.config.Safety.CheckTransport(n, "MergeTransports", true); err != nil {
			return nil, err
		}
	}
	if ws == nil || !ws.IsConnected() {
		return nil, fmt.Errorf("merging requests needs ZADT_VSP's function bridge (a WebSocket to the system)")
	}
	res, err := ws.CallRFC(ctx, "TR_MERGE_REQUESTS", map[string]any{
		"IS_REQUEST_FROM":   map[string]any{"H": map[string]any{"TRKORR": from}},
		"IS_REQUEST_TO":     map[string]any{"H": map[string]any{"TRKORR": to}},
		"IV_REQUEST_CHOICE": "", // no dialog; an empty value turns the default 'X' off
		"IV_WITH_DIALOG":    "",
	})
	if err != nil {
		return nil, fmt.Errorf("TR_MERGE_REQUESTS: %w", err)
	}
	out := &TransportMergeResult{From: from, To: to, Message: res.Message}
	if res.Subrc != 0 {
		return out, fmt.Errorf("TR_MERGE_REQUESTS %s into %s: sy-subrc %d%s", from, to, res.Subrc, withMessage(res.Message))
	}
	out.Merged = true
	if details, derr := c.GetTransport(ctx, to); derr == nil {
		out.Objects = details.Objects
	}
	if _, derr := c.GetTransport(ctx, from); derr != nil && IsNotFoundError(derr) {
		out.SourceGone = true
	}
	return out, nil
}

func withMessage(msg string) string {
	if strings.TrimSpace(msg) == "" {
		return ""
	}
	return ": " + strings.TrimSpace(msg)
}

// TransportObjectKey names one entry of a request: E071's PGMID, OBJECT,
// OBJ_NAME.
type TransportObjectKey struct {
	PgmID  string `json:"pgmid"`
	Object string `json:"object"`
	Name   string `json:"name"`
}

func (k TransportObjectKey) String() string {
	return fmt.Sprintf("%s %s %s", k.PgmID, k.Object, k.Name)
}

// paddedNameTypes are the LIMU types whose OBJ_NAME is two names in fixed
// columns: the owner padded to 30 characters, then the part -- a class and
// its method (METH), a Web Dynpro component and its controller (WDYC) or
// view (WDYV). E071 holds "ZCL_DEMO" + 22 blanks + "RUN".
var paddedNameTypes = map[string]bool{"METH": true, "WDYC": true, "WDYV": true}

// ParseTransportObject reads "PROG ZDEMO", "R3TR PROG ZDEMO" or
// "LIMU METH ZCL_DEMO RUN". Without a PGMID it is R3TR.
//
// The name is the rest of the string as given, not re-joined from its
// words: OBJ_NAME may hold blanks that matter. For a two-part LIMU name
// (METH, WDYC, WDYV) given as two words, the owner is padded to its 30
// columns, so "LIMU METH ZCL_DEMO RUN" names the entry E071 holds.
func ParseTransportObject(s string) (TransportObjectKey, error) {
	upper := strings.ToUpper(strings.TrimSpace(s))
	parts := strings.Fields(upper)
	switch {
	case len(parts) == 2:
		return TransportObjectKey{PgmID: "R3TR", Object: parts[0], Name: parts[1]}, nil
	case len(parts) >= 3 && (parts[0] == "R3TR" || parts[0] == "LIMU" || parts[0] == "LANG" || parts[0] == "CORR"):
		k := TransportObjectKey{PgmID: parts[0], Object: parts[1], Name: restAfterFields(upper, 2)}
		if k.PgmID == "LIMU" && paddedNameTypes[k.Object] && len(parts) == 4 && len(parts[2]) <= 30 {
			k.Name = fmt.Sprintf("%-30s%s", parts[2], parts[3])
		}
		return k, nil
	}
	return TransportObjectKey{}, fmt.Errorf("object %q: want TYPE NAME (PROG ZDEMO) or PGMID TYPE NAME (R3TR PROG ZDEMO)", s)
}

// restAfterFields is s from its (n+1)th word on, its inner blanks kept.
func restAfterFields(s string, n int) string {
	for i := 0; i < n; i++ {
		s = strings.TrimLeft(s, " \t")
		if j := strings.IndexAny(s, " \t"); j >= 0 {
			s = s[j:]
		} else {
			return ""
		}
	}
	return strings.TrimSpace(s)
}

// sameObjectName compares OBJ_NAMEs as SAP does for an entry, blanks inside
// included -- but a name read back from ADT may come with its blanks
// collapsed, so a two-part name also matches word for word.
func sameObjectName(a, b string) bool {
	if strings.EqualFold(a, b) {
		return true
	}
	return strings.EqualFold(strings.Join(strings.Fields(a), " "), strings.Join(strings.Fields(b), " "))
}

// TransportMoveResult is what a move did.
type TransportMoveResult struct {
	Object TransportObjectKey `json:"object"`
	From   string             `json:"from"`
	To     string             `json:"to"`
	// FromTask and ToTask are the task (or request) the entry left and
	// the one it went into.
	FromTask string `json:"fromTask"`
	ToTask   string `json:"toTask"`
	Moved    bool   `json:"moved"`
	Message  string `json:"message,omitempty"`
}

// MoveTransportObject takes one entry out of from and puts it into to:
// appended to the caller's modifiable task of the target (the request
// itself when there is none), then deleted from the task of the source
// that holds it. An entry that is not in the source is refused before
// anything is written.
func (c *Client) MoveTransportObject(ctx context.Context, ws *DebugWebSocketClient, key TransportObjectKey, from, to string) (*TransportMoveResult, error) {
	from, to = strings.ToUpper(strings.TrimSpace(from)), strings.ToUpper(strings.TrimSpace(to))
	if from == "" || to == "" {
		return nil, fmt.Errorf("a source and a target request are required")
	}
	if from == to {
		return nil, fmt.Errorf("source and target are the same request %s", from)
	}
	for _, n := range []string{from, to} {
		if err := c.config.Safety.CheckTransport(n, "MoveTransportObject", true); err != nil {
			return nil, err
		}
	}
	if ws == nil || !ws.IsConnected() {
		return nil, fmt.Errorf("moving an object between requests needs ZADT_VSP's function bridge (a WebSocket to the system)")
	}
	out := &TransportMoveResult{Object: key, From: from, To: to}

	source, err := c.transportTree(ctx, from)
	if err != nil {
		return out, fmt.Errorf("reading %s: %w", from, err)
	}
	out.FromTask = holderIn(source, from, key)
	if out.FromTask == "" {
		return out, fmt.Errorf("%s is not in %s", key, from)
	}
	target, err := c.transportTree(ctx, to)
	if err != nil {
		return out, fmt.Errorf("reading %s: %w", to, err)
	}
	out.ToTask = targetTask(target, to, strings.ToUpper(c.config.Username))
	if _, err = classifyTask(ctx, ws, c.requestHeader, target, out.ToTask); err != nil {
		return out, err
	}

	entry := map[string]any{"PGMID": key.PgmID, "OBJECT": key.Object, "OBJ_NAME": key.Name}
	res, err := ws.CallRFC(ctx, "TR_APPEND_TO_COMM_OBJS_KEYS", map[string]any{
		"WI_TRKORR":             out.ToTask,
		"WT_E071":               []any{entry},
		"IV_DIALOG":             "",
		"WI_SUPPRESS_KEY_CHECK": "X",
	})
	if err != nil {
		return out, fmt.Errorf("TR_APPEND_TO_COMM_OBJS_KEYS: %w", err)
	}
	if res.Subrc != 0 {
		out.Message = res.Message
		return out, fmt.Errorf("adding %s to %s: sy-subrc %d%s", key, out.ToTask, res.Subrc, withMessage(res.Message))
	}
	res, err = ws.CallRFC(ctx, "TRINT_DELETE_COMM_OBJECT_KEYS", map[string]any{
		"CS_REQUEST":     map[string]any{"H": map[string]any{"TRKORR": out.FromTask}},
		"IS_E071_DELETE": entry,
		"IV_DIALOG_FLAG": "",
	})
	if err != nil {
		return out, fmt.Errorf("added to %s; TRINT_DELETE_COMM_OBJECT_KEYS: %w", out.ToTask, err)
	}
	if res.Subrc != 0 {
		out.Message = res.Message
		return out, fmt.Errorf("added to %s, but removing %s from %s: sy-subrc %d%s", out.ToTask, key, out.FromTask, res.Subrc, withMessage(res.Message))
	}
	out.Moved = true
	return out, nil
}

// holderOf is the task of the request that carries the entry, the request
// itself when the entry sits at its level, "" when it is not there.
func holderOf(details *TransportDetails, key TransportObjectKey) string {
	for _, t := range details.Tasks {
		if holds(t.Objects, key) {
			return t.Number
		}
	}
	if holds(details.Objects, key) {
		return details.Number
	}
	return ""
}

func holds(objects []TransportObjectV2, key TransportObjectKey) bool {
	for _, o := range objects {
		if sameObjectName(o.Name, key.Name) && strings.EqualFold(o.Type, key.Object) && (o.PgmID == "" || strings.EqualFold(o.PgmID, key.PgmID)) {
			return true
		}
	}
	return false
}

// namedTask is number when it names one of the request's tasks rather than
// the request itself -- SE09 takes a task wherever a request goes, and
// transportTree reads the request of a task, so the task would be lost.
func namedTask(details *TransportDetails, number string) string {
	number = strings.TrimSpace(number)
	if number == "" || strings.EqualFold(number, details.Number) {
		return ""
	}
	for _, t := range details.Tasks {
		if strings.EqualFold(t.Number, number) {
			return t.Number
		}
	}
	return ""
}

// targetTask is where an entry goes: the task the caller named, or else the
// user's task in the request.
func targetTask(details *TransportDetails, named, user string) string {
	if t := namedTask(details, named); t != "" {
		return t
	}
	return taskFor(details, user)
}

// holderIn is the task an entry is taken from. When the caller named a task,
// only that one; the same entry in another task of the request is not it.
func holderIn(details *TransportDetails, named string, key TransportObjectKey) string {
	t := namedTask(details, named)
	if t == "" {
		return holderOf(details, key)
	}
	for _, task := range details.Tasks {
		if task.Number == t && holds(task.Objects, key) {
			return t
		}
	}
	return ""
}

// taskFor is the user's modifiable task in the request, or the request
// when there is none.
func taskFor(details *TransportDetails, user string) string {
	for _, t := range details.Tasks {
		if strings.EqualFold(t.Owner, user) && (t.Status == "" || t.Status == "D") {
			return t.Number
		}
	}
	return details.Number
}
