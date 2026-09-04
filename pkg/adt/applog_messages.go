package adt

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/oisee/vibing-steampunk/pkg/datacluster"
)

// The messages of an application log live in BALDAT as one data cluster per
// log handle. Inside, the BAL layer keeps them in buckets named T_abcd: the
// first digit is the width class of the four message variables (1 = 20
// characters, 2 = 50), the second says whether a context structure travels
// with the message, the third and fourth whether parameters and a callback
// do. T_MHDR is the directory — message number to bucket — and T_PAR1/T_PAR2
// hold parameters by message number. Field names are not in the cluster;
// the layout below is BAL_S_MSG's, confirmed against the BAL API's output for
// the same logs.

// AppLogMessage is one message of an application log.
type AppLogMessage struct {
	// Number is the message's position in the log, from 000001.
	Number string `json:"number"`
	// Type is the message type: A, E, W, I, S or X.
	Type string `json:"type"`
	// ID is the message class, No the message number in it.
	ID string `json:"id"`
	No string `json:"no"`
	// Text is the message rendered from T100 with the variables substituted;
	// empty when the class or number was not found.
	Text string `json:"text,omitempty"`
	V1   string `json:"v1,omitempty"`
	V2   string `json:"v2,omitempty"`
	V3   string `json:"v3,omitempty"`
	V4   string `json:"v4,omitempty"`
	// DetailLevel (1–9) is the nesting the log viewer draws as a tree.
	DetailLevel string `json:"detailLevel,omitempty"`
	// ProblemClass is 1 (very important) to 4 (additional information).
	ProblemClass string `json:"problemClass,omitempty"`
	Sort         string `json:"sort,omitempty"`
	// Timestamp is the UTC time stamp with microseconds, as written.
	Timestamp string `json:"timestamp,omitempty"`
	// Count is how many times this message was collected when the log was
	// written with message aggregation.
	Count int64 `json:"count,omitempty"`
	// Context is the structure the writer attached, by DDIC name and raw value.
	Context *AppLogContext `json:"context,omitempty"`
	Params  []AppLogParam  `json:"params,omitempty"`
}

// AppLogContext is the context structure a message carries.
type AppLogContext struct {
	Table string `json:"table"`
	Value string `json:"value"`
}

// AppLogParam is one named parameter of a message.
type AppLogParam struct {
	Name  string `json:"name"`
	Value string `json:"value"`
}

// DecodeAppLogMessages reads the messages out of one BALDAT cluster.
func DecodeAppLogMessages(blob []byte) ([]AppLogMessage, error) {
	c, err := datacluster.Parse(blob)
	if err != nil {
		return nil, err
	}
	return appLogMessagesFromCluster(c)
}

func appLogMessagesFromCluster(c *datacluster.Cluster) ([]AppLogMessage, error) {
	var msgs []AppLogMessage
	params := map[string][]AppLogParam{}
	for _, obj := range c.Objects {
		switch {
		case strings.HasPrefix(obj.Name, "T_PAR"):
			for _, row := range obj.Rows {
				if len(row) < 4 {
					continue
				}
				n := str(row[0])
				params[n] = append(params[n], AppLogParam{Name: str(row[2]), Value: str(row[3])})
			}
		case len(obj.Name) == 6 && strings.HasPrefix(obj.Name, "T_") && obj.Name[2] >= '0' && obj.Name[2] <= '9':
			for _, row := range obj.Rows {
				m, err := appLogMessageFromRow(obj.Name, row)
				if err != nil {
					return nil, fmt.Errorf("%s: %w", obj.Name, err)
				}
				msgs = append(msgs, m)
			}
		}
	}
	for i := range msgs {
		msgs[i].Params = params[msgs[i].Number]
	}
	sort.SliceStable(msgs, func(i, j int) bool { return msgs[i].Number < msgs[j].Number })
	return msgs, nil
}

// appLogMessageFromRow lays BAL_S_MSG over a bucket row. The head is the
// message number and the four variables; the tail is the eight fixed fields
// from type to count; what lies between depends on the bucket.
func appLogMessageFromRow(bucket string, row []any) (AppLogMessage, error) {
	const head, tail = 5, 8
	if len(row) < head+tail {
		return AppLogMessage{}, fmt.Errorf("message row has %d fields, BAL_S_MSG needs at least %d", len(row), head+tail)
	}
	t := row[len(row)-tail:]
	m := AppLogMessage{
		Number: str(row[0]), V1: str(row[1]), V2: str(row[2]), V3: str(row[3]), V4: str(row[4]),
		Type: str(t[0]), ID: str(t[1]), No: str(t[2]),
		DetailLevel: str(t[3]), ProblemClass: str(t[4]), Sort: str(t[5]), Timestamp: str(t[6]),
	}
	if n, ok := t[7].(int64); ok {
		m.Count = n
	}
	extra := row[head : len(row)-tail]
	if bucket[3] == '1' && len(extra) >= 2 {
		m.Context = &AppLogContext{Table: str(extra[0]), Value: str(extra[1])}
	}
	return m, nil
}

func str(v any) string {
	if v == nil {
		return ""
	}
	if s, ok := v.(string); ok {
		return s
	}
	return fmt.Sprint(v)
}

// AppLogMessages reads the messages of the given logs from BALDAT, keyed by
// log handle. A long log is stored as several blocks — BLOCK is part of the
// key, each block its own cluster — and they are joined here in message
// order. Logs whose cluster is missing are absent from the map; a cluster
// that fails to decode is an error, because a wrong reading is worse than
// none.
func (c *Client) AppLogMessages(ctx context.Context, handles []string) (map[string][]AppLogMessage, error) {
	out := map[string][]AppLogMessage{}
	const batch = 40
	for start := 0; start < len(handles); start += batch {
		end := start + batch
		if end > len(handles) {
			end = len(handles)
		}
		quoted := make([]string, 0, end-start)
		for _, h := range handles[start:end] {
			if h == "" {
				continue
			}
			quoted = append(quoted, "'"+sqlQuote(h)+"'")
		}
		if len(quoted) == 0 {
			continue
		}
		// One literal per line: the data preview wraps the statement into
		// 255-character ABAP lines, and a literal cut by the wrap is an error.
		where := "relid = 'AL' AND log_handle IN (\n" + strings.Join(quoted, ",\n") + " )"
		// A log of a few messages is three 512-byte fragments; one of several
		// thousand is blocks of 150 messages at some fifteen fragments each.
		// The limit is a guard against a runaway, not a budget, and hitting
		// it is reported rather than returning part of a log as the whole.
		const fragmentsPerLog = 4000
		res, err := c.ReadClusterRecords(ctx, "BALDAT", where, len(quoted)*fragmentsPerLog)
		if err != nil {
			return nil, err
		}
		if res.Truncated {
			return nil, fmt.Errorf("the messages of %d logs span more than %d BALDAT rows; ask for fewer logs at a time", len(quoted), len(quoted)*fragmentsPerLog)
		}
		for _, rec := range res.Records {
			handle := ""
			for _, kv := range rec.Key {
				if kv.Column == "LOG_HANDLE" {
					handle = kv.Value
				}
			}
			msgs, err := DecodeAppLogMessages(rec.Blob)
			if err != nil {
				return nil, fmt.Errorf("log %s: %w", handle, err)
			}
			out[handle] = append(out[handle], msgs...)
		}
	}
	for handle := range out {
		sort.SliceStable(out[handle], func(i, j int) bool { return out[handle][i].Number < out[handle][j].Number })
	}
	return out, nil
}

// spras maps an ISO 639-1 code to SAP's one-character language key, which
// is what T100 is keyed by. A one-character input is taken as already
// converted; an unknown code falls back to English.
func spras(lang string) string {
	lang = strings.ToUpper(strings.TrimSpace(lang))
	if len(lang) == 1 {
		return lang
	}
	table := map[string]string{
		"EN": "E", "DE": "D", "FR": "F", "ES": "S", "IT": "I", "PT": "P", "NL": "N", "RU": "R",
		"JA": "J", "ZH": "1", "ZF": "M", "KO": "3", "PL": "L", "CS": "C", "SK": "Q", "TR": "T",
		"SV": "V", "DA": "K", "FI": "U", "NO": "O", "HU": "H", "EL": "G", "UK": "8", "AR": "A",
		"HE": "B", "TH": "2", "RO": "4", "HR": "6", "SL": "5", "BG": "W", "LT": "X", "LV": "Y",
		"ET": "9", "SR": "0", "CA": "c", "ID": "i", "MS": "7", "VI": "v", "KK": "k", "AF": "a",
	}
	if k, ok := table[lang]; ok {
		return k
	}
	return "E"
}

// AppLogTexts fills Text on every message from T100 in the given language
// (ISO code or SAP key), substituting the variables. Missing texts are left
// empty; a failed lookup is returned but the messages are still usable
// without it.
func (c *Client) AppLogTexts(ctx context.Context, lang string, msgs []AppLogMessage) error {
	lang = spras(lang)
	byClass := map[string]map[string]bool{}
	for _, m := range msgs {
		if m.ID == "" {
			continue
		}
		if byClass[m.ID] == nil {
			byClass[m.ID] = map[string]bool{}
		}
		byClass[m.ID][m.No] = true
	}
	texts := map[string]string{}
	for class, nos := range byClass {
		list := make([]string, 0, len(nos))
		for n := range nos {
			list = append(list, "'"+sqlQuote(n)+"'")
		}
		sort.Strings(list)
		query := fmt.Sprintf("SELECT msgnr, text FROM t100 WHERE sprsl = '%s' AND arbgb = '%s' AND msgnr IN (\n%s )",
			sqlQuote(lang), sqlQuote(class), strings.Join(list, ",\n"))
		res, err := c.RunQuery(ctx, query, len(list))
		if err != nil {
			return fmt.Errorf("reading message texts for %s: %w", class, err)
		}
		if res == nil {
			continue
		}
		for _, row := range res.Rows {
			texts[class+"\x00"+cell(row, "MSGNR")] = cell(row, "TEXT")
		}
	}
	for i := range msgs {
		if t, ok := texts[msgs[i].ID+"\x00"+msgs[i].No]; ok {
			msgs[i].Text = renderMessage(t, msgs[i].V1, msgs[i].V2, msgs[i].V3, msgs[i].V4)
		}
	}
	return nil
}

// renderMessage substitutes &1..&4 and bare & placeholders the way MESSAGE
// ... INTO does. &V1&-style and escaped && are handled as ABAP does.
func renderMessage(text string, vars ...string) string {
	var b strings.Builder
	next := 0
	for i := 0; i < len(text); i++ {
		ch := text[i]
		if ch != '&' {
			b.WriteByte(ch)
			continue
		}
		if i+1 < len(text) && text[i+1] == '&' {
			b.WriteByte('&')
			i++
			continue
		}
		if i+1 < len(text) && text[i+1] >= '1' && text[i+1] <= '4' {
			b.WriteString(vars[text[i+1]-'1'])
			i++
			continue
		}
		if next < len(vars) {
			b.WriteString(vars[next])
			next++
		}
	}
	return strings.TrimRight(b.String(), " ")
}
