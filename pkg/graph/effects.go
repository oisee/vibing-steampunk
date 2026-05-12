package graph

import (
	"strings"

	"github.com/oisee/vibing-steampunk/pkg/abaplint"
)

// EffectInfo describes the side effects of a code unit.
type EffectInfo struct {
	// Database
	ReadsDB  []string `json:"readsDB,omitempty"`  // tables read (SELECT)
	WritesDB []string `json:"writesDB,omitempty"` // tables written (INSERT/UPDATE/DELETE/MODIFY)

	// State
	ReadsState  bool `json:"readsState,omitempty"`  // reads me->attribute or class-data
	WritesState bool `json:"writesState,omitempty"` // writes me->attribute or class-data

	// LUW / Transactional
	HasCommit       bool `json:"hasCommit,omitempty"`       // COMMIT WORK
	HasRollback     bool `json:"hasRollback,omitempty"`     // ROLLBACK WORK
	UpdateTask      bool `json:"updateTask,omitempty"`      // CALL FUNCTION IN UPDATE TASK
	BackgroundTask  bool `json:"backgroundTask,omitempty"`  // CALL FUNCTION IN BACKGROUND TASK
	UpdateTaskLocal bool `json:"updateTaskLocal,omitempty"` // SET UPDATE TASK LOCAL

	// Async
	AsyncRFC       bool `json:"asyncRFC,omitempty"`       // CALL FUNCTION STARTING NEW TASK
	BackgroundJob  bool `json:"backgroundJob,omitempty"`  // SUBMIT VIA JOB
	SubmitAndReturn bool `json:"submitAndReturn,omitempty"` // SUBMIT AND RETURN

	// External
	SyncRFC  []string `json:"syncRFC,omitempty"`  // DESTINATION targets
	HTTPCall bool     `json:"httpCall,omitempty"` // cl_http_client usage
	APCPush  bool     `json:"apcPush,omitempty"`  // APC/WebSocket send

	// Control flow
	RaisesExc     bool `json:"raisesExc,omitempty"`     // RAISE EXCEPTION
	RaisesMessage bool `json:"raisesMessage,omitempty"` // MESSAGE TYPE E/A/X
	LeavesContext bool `json:"leavesContext,omitempty"` // LEAVE PROGRAM / LEAVE TO TRANSACTION

	// LUW classification (computed)
	LUWClass string `json:"luwClass,omitempty"` // safe, participant, owner, unsafe
}

// IsPure returns true if the code unit has no observable side effects.
func (e *EffectInfo) IsPure() bool {
	return len(e.ReadsDB) == 0 && len(e.WritesDB) == 0 &&
		!e.ReadsState && !e.WritesState &&
		!e.HasCommit && !e.HasRollback &&
		!e.UpdateTask && !e.BackgroundTask &&
		!e.AsyncRFC && !e.BackgroundJob &&
		len(e.SyncRFC) == 0 && !e.HTTPCall && !e.APCPush
}

// ClassifyLUW determines the LUW classification.
func (e *EffectInfo) ClassifyLUW() string {
	if e.HasCommit && e.UpdateTask {
		return "unsafe" // mixes own commit with deferred updates
	}
	if e.HasCommit || e.HasRollback {
		return "owner" // owns the transaction boundary
	}
	if e.UpdateTask || e.BackgroundTask {
		return "participant" // registers deferred work
	}
	return "safe" // no transactional impact
}

// ExtractEffects analyzes ABAP source for side effects.
func ExtractEffects(source string) *EffectInfo {
	lexer := &abaplint.Lexer{}
	tokens := lexer.Run(source)
	parser := &abaplint.StatementParser{}
	stmts := parser.Parse(tokens)
	matcher := abaplint.NewStatementMatcher()
	matcher.ClassifyStatements(stmts)

	info := &EffectInfo{}
	seenReadDB := map[string]bool{}
	seenWriteDB := map[string]bool{}
	seenRFC := map[string]bool{}

	for _, stmt := range stmts {
		toks := stmt.Tokens
		if len(toks) < 1 {
			continue
		}
		first := strings.ToUpper(toks[0].Str)

		switch stmt.Type {
		case "Select":
			// SELECT ... FROM table
			for i, t := range toks {
				if strings.EqualFold(t.Str, "FROM") && i+1 < len(toks) {
					tbl := strings.ToUpper(toks[i+1].Str)
					if isCustomName(tbl) && !seenReadDB[tbl] {
						info.ReadsDB = append(info.ReadsDB, tbl)
						seenReadDB[tbl] = true
					}
				}
			}

		case "InsertInternal":
			// INSERT INTO ztable or INSERT ztable FROM
			tbl := extractDBTable(toks, "INSERT")
			if tbl != "" && !seenWriteDB[tbl] {
				info.WritesDB = append(info.WritesDB, tbl)
				seenWriteDB[tbl] = true
			}

		case "DeleteInternal":
			// DELETE FROM ztable or DELETE ztable
			tbl := extractDBTable(toks, "DELETE")
			if tbl != "" && !seenWriteDB[tbl] {
				info.WritesDB = append(info.WritesDB, tbl)
				seenWriteDB[tbl] = true
			}
		}

		// UPDATE/MODIFY not classified by abaplint matcher — detect by first token
		if first == "UPDATE" && len(toks) >= 2 {
			tbl := strings.ToUpper(toks[1].Str)
			if isCustomName(tbl) && !seenWriteDB[tbl] {
				info.WritesDB = append(info.WritesDB, tbl)
				seenWriteDB[tbl] = true
			}
		}
		if first == "MODIFY" && len(toks) >= 2 {
			tbl := strings.ToUpper(toks[1].Str)
			if isCustomName(tbl) && !seenWriteDB[tbl] {
				info.WritesDB = append(info.WritesDB, tbl)
				seenWriteDB[tbl] = true
			}
		}

		// COMMIT / ROLLBACK
		if first == "COMMIT" {
			info.HasCommit = true
		}
		if first == "ROLLBACK" {
			info.HasRollback = true
		}

		// SET UPDATE TASK LOCAL
		if first == "SET" && hasTokenSequence(toks, "SET", "UPDATE", "TASK", "LOCAL") {
			info.UpdateTaskLocal = true
		}

		// CALL FUNCTION patterns
		if stmt.Type == "CallFunction" {
			if hasToken(toks, "DESTINATION") {
				dest := tokenAfter(toks, "DESTINATION")
				if dest != "" {
					cleaned := strings.Trim(dest, "'")
					if !seenRFC[cleaned] {
						info.SyncRFC = append(info.SyncRFC, cleaned)
						seenRFC[cleaned] = true
					}
				}
			}
			if hasTokenSequence(toks, "IN", "UPDATE", "TASK") {
				info.UpdateTask = true
			}
			if hasTokenSequence(toks, "IN", "BACKGROUND", "TASK") {
				info.BackgroundTask = true
			}
			if hasTokenSequence(toks, "STARTING", "NEW", "TASK") {
				info.AsyncRFC = true
			}
		}

		// SUBMIT patterns
		if stmt.Type == "Submit" {
			if hasTokenSequence(toks, "VIA", "JOB") {
				info.BackgroundJob = true
			}
			if hasTokenSequence(toks, "AND", "RETURN") {
				info.SubmitAndReturn = true
			}
		}

		// RAISE EXCEPTION
		if stmt.Type == "Raise" {
			info.RaisesExc = true
		}

		// MESSAGE TYPE E/A/X
		if first == "MESSAGE" {
			for _, t := range toks {
				if strings.EqualFold(t.Str, "TYPE") {
					break
				}
			}
			for i, t := range toks {
				if strings.EqualFold(t.Str, "TYPE") && i+1 < len(toks) {
					msgType := strings.ToUpper(strings.Trim(toks[i+1].Str, "'"))
					if msgType == "E" || msgType == "A" || msgType == "X" {
						info.RaisesMessage = true
					}
					break
				}
			}
		}

		// LEAVE TO TRANSACTION / LEAVE PROGRAM
		if stmt.Type == "LeaveToTransaction" {
			info.LeavesContext = true
		}
		if first == "LEAVE" && len(toks) >= 2 && strings.EqualFold(toks[1].Str, "PROGRAM") {
			info.LeavesContext = true
		}

		// State access: me-> patterns
		for _, t := range toks {
			s := t.Str
			if strings.Contains(s, "->") && (strings.HasPrefix(strings.ToLower(s), "me->") ||
				strings.HasPrefix(strings.ToLower(s), "self->")) {
				info.ReadsState = true
			}
		}

		// HTTP client
		for _, t := range toks {
			if strings.EqualFold(t.Str, "CL_HTTP_CLIENT") || strings.EqualFold(t.Str, "IF_HTTP_CLIENT") {
				info.HTTPCall = true
			}
		}

		// APC
		for _, t := range toks {
			if strings.Contains(strings.ToLower(t.Str), "apc") && strings.Contains(strings.ToLower(t.Str), "send") {
				info.APCPush = true
			}
			if strings.EqualFold(t.Str, "I_APC_WSP_MESSAGE") || strings.EqualFold(t.Str, "IF_APC_WSP_MESSAGE") {
				info.APCPush = true
			}
		}
	}

	info.LUWClass = info.ClassifyLUW()
	return info
}

// extractDBTable extracts the table name from INSERT/DELETE statements.
func extractDBTable(toks []abaplint.Token, keyword string) string {
	for i, t := range toks {
		if strings.EqualFold(t.Str, keyword) && i+1 < len(toks) {
			next := strings.ToUpper(toks[i+1].Str)
			if next == "INTO" || next == "FROM" {
				if i+2 < len(toks) {
					tbl := strings.ToUpper(toks[i+2].Str)
					if isCustomName(tbl) {
						return tbl
					}
				}
			} else if isCustomName(next) {
				return next
			}
		}
	}
	return ""
}

func hasToken(toks []abaplint.Token, keyword string) bool {
	for _, t := range toks {
		if strings.EqualFold(t.Str, keyword) {
			return true
		}
	}
	return false
}

func hasTokenSequence(toks []abaplint.Token, keywords ...string) bool {
	for i := 0; i <= len(toks)-len(keywords); i++ {
		match := true
		for j, kw := range keywords {
			if !strings.EqualFold(toks[i+j].Str, kw) {
				match = false
				break
			}
		}
		if match {
			return true
		}
	}
	return false
}

func tokenAfter(toks []abaplint.Token, keyword string) string {
	for i, t := range toks {
		if strings.EqualFold(t.Str, keyword) && i+1 < len(toks) {
			return toks[i+1].Str
		}
	}
	return ""
}
