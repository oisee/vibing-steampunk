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

// ParseTransportObject reads "PROG ZDEMO", "R3TR PROG ZDEMO" or
// "LIMU METH ZCL_DEMO  RUN". Without a PGMID it is R3TR.
func ParseTransportObject(s string) (TransportObjectKey, error) {
	parts := strings.Fields(strings.ToUpper(strings.TrimSpace(s)))
	switch {
	case len(parts) == 2:
		return TransportObjectKey{PgmID: "R3TR", Object: parts[0], Name: parts[1]}, nil
	case len(parts) >= 3 && (parts[0] == "R3TR" || parts[0] == "LIMU" || parts[0] == "LANG" || parts[0] == "CORR"):
		return TransportObjectKey{PgmID: parts[0], Object: parts[1], Name: strings.Join(parts[2:], " ")}, nil
	}
	return TransportObjectKey{}, fmt.Errorf("object %q: want TYPE NAME (PROG ZDEMO) or PGMID TYPE NAME (R3TR PROG ZDEMO)", s)
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

	source, err := c.GetTransport(ctx, from)
	if err != nil {
		return out, fmt.Errorf("reading %s: %w", from, err)
	}
	out.FromTask = holderOf(source, key)
	if out.FromTask == "" {
		return out, fmt.Errorf("%s is not in %s", key, from)
	}
	target, err := c.GetTransport(ctx, to)
	if err != nil {
		return out, fmt.Errorf("reading %s: %w", to, err)
	}
	out.ToTask = taskFor(target, strings.ToUpper(c.config.Username))

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
	has := func(objects []TransportObjectV2) bool {
		for _, o := range objects {
			if strings.EqualFold(o.Name, key.Name) && strings.EqualFold(o.Type, key.Object) && (o.PgmID == "" || strings.EqualFold(o.PgmID, key.PgmID)) {
				return true
			}
		}
		return false
	}
	for _, t := range details.Tasks {
		if has(t.Objects) {
			return t.Number
		}
	}
	if has(details.Objects) {
		return details.Number
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
