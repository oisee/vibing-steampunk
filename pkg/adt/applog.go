package adt

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"
)

// The application log — SLG1 to anyone who has used it — is where ABAP code
// records what it did and why, and it is the first place to look when a program
// failed and nobody can reproduce it.
//
// SAP's own way in is the BAL_* function group, which cannot be called
// remotely: not over a gateway, not over SOAP-RFC, not over a WebSocket tunnel.
// The blocker is not the transport. But the log's tables are ordinary tables,
// and ADT's free SQL reads them, so the whole log is available over plain
// HTTPS with no Z code anywhere: the headers from BALHDR here, and the
// messages from the BALDAT data cluster in applog_messages.go.
//
// This exists so that nobody has to remember that. "Which program logged
// something around the time this dumped" is the question; BALHDR and the column
// name ALPROG are an implementation detail of answering it, and belong here
// rather than in the head of whoever is asking.

// AppLogEntry is one application log header: who logged, from where, when, and
// under which log object.
//
// The messages are not read with the header. For four months this comment
// said they could not be read at all, because BALDAT is a cluster table; they
// can, and Client.AppLogMessages does. Everything needed to correlate a log
// with a dump is in the header, so the messages stay optional and cost a
// second query only when asked for.
type AppLogEntry struct {
	LogNumber string `json:"logNumber"`
	// LogHandle is the key of the log's messages in BALDAT.
	LogHandle string    `json:"logHandle,omitempty"`
	Object    string    `json:"object"`
	SubObject string    `json:"subObject,omitempty"`
	External  string    `json:"externalId,omitempty"`
	Program   string    `json:"program,omitempty"`
	User      string    `json:"user,omitempty"`
	Mode      string    `json:"mode,omitempty"`
	At        time.Time `json:"at"`
	// MessageCount is what the header says the log holds, which is known
	// before the messages are read.
	MessageCount int `json:"messageCount,omitempty"`
	// Messages is filled by AppLogMessages when the caller asks for them.
	Messages []AppLogMessage `json:"messages,omitempty"`
}

// AppLogFilter narrows the search. Everything is optional, but a search with no
// bounds at all reads the whole log of a busy system, so Limit always applies.
type AppLogFilter struct {
	// Program matches ALPROG. This is the field that makes a log entry worth
	// ranking against a dump: an entry written by the program that died, or by
	// one on its call stack, is connected structurally rather than by the
	// coincidence of a nearby timestamp.
	Program string
	// User matches ALUSER.
	User string
	// Object and SubObject are the SLG0 log object and subobject.
	Object, SubObject string
	// From and To bound ALDATE/ALTIME. Zero means unbounded on that side.
	From, To time.Time
	// Limit caps the rows read; 100 when unset.
	Limit int
}

// ApplicationLog reads application log headers matching the filter, newest
// first.
func (c *Client) ApplicationLog(ctx context.Context, filter AppLogFilter) ([]AppLogEntry, error) {
	limit := filter.Limit
	if limit <= 0 {
		limit = 100
	}

	where := appLogWhere(filter)
	query := "SELECT lognumber, log_handle, object, subobject, extnumber, aldate, altime, aluser, alprog, almode, msg_cnt_al FROM balhdr"
	if where != "" {
		query += " WHERE " + where
	}
	query += " ORDER BY aldate DESCENDING, altime DESCENDING"

	res, err := c.RunQuery(ctx, query, limit)
	if err != nil {
		return nil, fmt.Errorf("reading the application log: %w", err)
	}
	if res == nil {
		return nil, nil
	}

	entries := make([]AppLogEntry, 0, len(res.Rows))
	for _, row := range res.Rows {
		count, _ := strconv.Atoi(cell(row, "MSG_CNT_AL"))
		entries = append(entries, AppLogEntry{
			LogNumber:    cell(row, "LOGNUMBER"),
			LogHandle:    cell(row, "LOG_HANDLE"),
			Object:       cell(row, "OBJECT"),
			SubObject:    cell(row, "SUBOBJECT"),
			External:     cell(row, "EXTNUMBER"),
			Program:      cell(row, "ALPROG"),
			User:         cell(row, "ALUSER"),
			Mode:         cell(row, "ALMODE"),
			At:           parseSAPStamp(cell(row, "ALDATE"), cell(row, "ALTIME")),
			MessageCount: count,
		})
	}
	return entries, nil
}

// appLogWhere builds the WHERE clause. Values are quoted and any quote inside
// them doubled, because this is free SQL and a log object name arrives from
// whatever the caller was given.
func appLogWhere(filter AppLogFilter) string {
	var terms []string
	add := func(column, value string) {
		if v := strings.TrimSpace(value); v != "" {
			terms = append(terms, fmt.Sprintf("%s = '%s'", column, sqlQuote(v)))
		}
	}
	add("alprog", filter.Program)
	add("aluser", strings.ToUpper(filter.User))
	add("object", strings.ToUpper(filter.Object))
	add("subobject", strings.ToUpper(filter.SubObject))

	// The date and the time are separate columns, so a window that spans
	// midnight cannot be expressed as one comparison. Bounding the days and
	// letting the caller's own comparison finish the job would be wrong in the
	// other direction — it would drop nothing but claim precision it does not
	// have — so the day bound is stated as exactly what it is.
	if !filter.From.IsZero() {
		terms = append(terms, fmt.Sprintf("aldate >= '%s'", filter.From.Format("20060102")))
	}
	if !filter.To.IsZero() {
		terms = append(terms, fmt.Sprintf("aldate <= '%s'", filter.To.Format("20060102")))
	}
	return strings.Join(terms, " AND ")
}

// sqlQuote doubles single quotes, the only escape Open SQL string literals have.
func sqlQuote(s string) string {
	return strings.ReplaceAll(s, "'", "''")
}

// parseSAPStamp turns SAP's separate YYYYMMDD and HHMMSS columns into one time.
// An unparseable pair yields the zero time rather than an error: a log entry
// with a strange date is still a log entry, and losing the row would hide it.
func parseSAPStamp(date, clock string) time.Time {
	date, clock = strings.TrimSpace(date), strings.TrimSpace(clock)
	if len(date) != 8 {
		return time.Time{}
	}
	if len(clock) != 6 {
		clock = "000000"
	}
	at, err := time.Parse("20060102150405", date+clock)
	if err != nil {
		return time.Time{}
	}
	return at
}

// cell reads one column from a row, whatever concrete type the query layer put
// there.
func cell(row map[string]interface{}, column string) string {
	value, ok := row[column]
	if !ok {
		value = row[strings.ToLower(column)]
	}
	switch v := value.(type) {
	case nil:
		return ""
	case string:
		return strings.TrimSpace(v)
	default:
		return strings.TrimSpace(fmt.Sprint(v))
	}
}

// AttachAppLogMessages reads the messages of every entry and stores them on
// it, with texts in the given language. Entries whose log has no cluster —
// deleted bodies, or a header written without messages — are left as they
// are.
func (c *Client) AttachAppLogMessages(ctx context.Context, lang string, entries []AppLogEntry) error {
	handles := make([]string, 0, len(entries))
	for _, e := range entries {
		if e.LogHandle != "" {
			handles = append(handles, e.LogHandle)
		}
	}
	byHandle, err := c.AppLogMessages(ctx, handles)
	if err != nil {
		return err
	}
	var all []AppLogMessage
	for i := range entries {
		entries[i].Messages = byHandle[entries[i].LogHandle]
		all = append(all, entries[i].Messages...)
	}
	if len(all) == 0 {
		return nil
	}
	if err := c.AppLogTexts(ctx, lang, all); err != nil {
		return err
	}
	// AppLogTexts wrote into the flat copy; carry the texts back.
	n := 0
	for i := range entries {
		for j := range entries[i].Messages {
			entries[i].Messages[j].Text = all[n].Text
			n++
		}
	}
	return nil
}
