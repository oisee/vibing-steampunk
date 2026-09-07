package adt

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"regexp"
	"sort"
	"strings"
)

// Writing a program's text pool. The text elements are their own ADT
// resource — /sap/bc/adt/textelements/programs/{name} — with its own lock
// (REPT, not the program's), and three documents under it: symbols (text
// symbols, I), selections (selection texts, S), headings (H). Each is plain
// text, one `KEY     =text` per line with the key padded to eight
// characters, the format the GET returns and the PUT expects. The lock, the
// PUT and the unlock go through one stateful session, as every write does.

// TextPoolKinds maps the classic READ TEXTPOOL id to the ADT document.
var TextPoolKinds = map[string]string{"I": "symbols", "S": "selections", "H": "headings"}

// WriteTextPool replaces the texts of one kind (I, S or H) in one language
// with the entries given: every key in entries gets its text, keys not
// named keep theirs. transport may be empty for local objects.
func (c *Client) WriteTextPool(ctx context.Context, program, lang, kind string, entries map[string]string, transport string) error {
	if err := c.checkSafety(OpUpdate, "WriteTextPool"); err != nil {
		return err
	}
	program = strings.ToUpper(strings.TrimSpace(program))
	kind = strings.ToUpper(strings.TrimSpace(kind))
	doc, ok := TextPoolKinds[kind]
	if !ok {
		return fmt.Errorf("text pool kind %q: want I (symbols), S (selection texts) or H (headings)", kind)
	}
	if len(entries) == 0 {
		return fmt.Errorf("no texts to write")
	}

	// What is there now, so that keys not named keep their text.
	current, err := c.GetTextPoolInLanguage(ctx, program, lang)
	if err != nil {
		return err
	}
	merged := map[string]string{}
	for _, e := range current {
		if e.ID == kind {
			merged[strings.ToUpper(strings.TrimSpace(e.Key))] = e.Text
		}
	}
	for k, v := range entries {
		merged[strings.ToUpper(strings.TrimSpace(k))] = v
	}

	resource := fmt.Sprintf("/sap/bc/adt/textelements/programs/%s", url.PathEscape(strings.ToLower(program)))
	lock, err := c.LockObject(ctx, resource, "MODIFY")
	if err != nil {
		return fmt.Errorf("locking the text pool of %s: %w", program, err)
	}
	defer func() { _ = c.UnlockObject(ctx, resource, lock.LockHandle) }()

	params := url.Values{}
	params.Set("lockHandle", lock.LockHandle)
	if transport = strings.TrimSpace(transport); transport != "" {
		params.Set("corrNr", transport)
	}
	vocabulary := "application/vnd.sap.adt.textelements." + doc + ".v1"
	_, err = c.transport.Request(ctx, resource+"/source/"+doc, &RequestOptions{
		Method:           http.MethodPut,
		Query:            params,
		Body:             []byte(FormatTextPool(merged)),
		ContentType:      vocabulary + "; charset=UTF-8",
		Accept:           vocabulary,
		OverrideLanguage: strings.ToUpper(lang),
		Stateful:         true,
	})
	if err != nil {
		return fmt.Errorf("writing the %s of %s: %w", doc, program, err)
	}
	return nil
}

// FormatTextPool renders entries the way the text element documents hold
// them: keys padded to eight characters, one per line, sorted.
func FormatTextPool(entries map[string]string) string {
	keys := make([]string, 0, len(entries))
	for k := range entries {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	var b strings.Builder
	for _, k := range keys {
		fmt.Fprintf(&b, "%-8s=%s\n", k, entries[k])
	}
	return b.String()
}

// The selection-text comment: a trailing comment on the line that declares
// a parameter, a select-option or a named selection-screen comment, of the
// form "~t: the text. The text lives beside the field it describes, in the
// source, where it is versioned with it; SyncSelectionTexts carries it to
// the text pool.
//
//	PARAMETERS p_devc TYPE tadir-devclass DEFAULT '$TMP'. "~t: Package to scan
//	SELECT-OPTIONS s_obj FOR tadir-obj_name.            "~t: Object names
//	SELECTION-SCREEN COMMENT 3(60) gv_hint FOR FIELD p_devc. "~t: Where to look
var (
	textComment  = regexp.MustCompile(`"\s*~+t\s*:\s*(.*?)\s*$`)
	declKeyword  = regexp.MustCompile(`(?i)^\s*(?:PARAMETERS?|SELECT-OPTIONS?|SELECTION-SCREEN\s+COMMENT\s+(?:/?\d+\(\d+\)|\S+))\s*:?\s*`)
	firstIdent   = regexp.MustCompile(`^([A-Za-z_][A-Za-z0-9_]*)`)
	commentLabel = regexp.MustCompile(`(?i)SELECTION-SCREEN\s+COMMENT\s+\S+\s+([A-Za-z_][A-Za-z0-9_]*)`)
)

// SelectionTextsFromSource collects the "~t: comments of a program: the
// first identifier of the line, after any declaring keyword, is the key —
// which is right for PARAMETERS, SELECT-OPTIONS, the chained lines that
// follow them, and a named SELECTION-SCREEN COMMENT. A "~t: on a line of
// its own has no key and is skipped.
func SelectionTextsFromSource(source string) map[string]string {
	out := map[string]string{}
	for _, line := range strings.Split(source, "\n") {
		m := textComment.FindStringSubmatch(line)
		if m == nil {
			continue
		}
		text := strings.TrimSpace(m[1])
		if text == "" {
			continue
		}
		code := line[:strings.LastIndex(line, m[0])]
		key := ""
		if cm := commentLabel.FindStringSubmatch(code); cm != nil {
			key = cm[1]
		} else {
			rest := declKeyword.ReplaceAllString(code, "")
			if im := firstIdent.FindStringSubmatch(strings.TrimSpace(rest)); im != nil {
				key = im[1]
			}
		}
		if key == "" {
			continue
		}
		out[strings.ToUpper(key)] = text
	}
	return out
}
