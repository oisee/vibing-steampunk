package adt

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

// A program's or a class's text pool: selection texts (S), text symbols
// (I), list headings (H). In ADT they are their own resource —
// /sap/bc/adt/textelements/programs/{name} or /classes/{name} — with their
// own lock object (REPT; a lock on the program is not a lock on its texts)
// and one plain-text document per kind: `KEY     =text`, the key padded to
// eight, with directive lines such as @MaxLength:60 or @DDICReference in
// front of an entry. The document is read whole, changed line by line, and
// written whole, so a directive on an entry nobody touched survives.
//
// Writing is diff-based and refuses what it cannot see: a selection text
// for a field the screen does not have, a key SAP will reject, a language
// that is not the object's master language unless the caller names it. It
// is a mutation like any other and goes through the package gate.

// TextPoolTarget names the object whose texts are meant.
type TextPoolTarget struct {
	// Type is PROG (the default) or CLAS.
	Type string
	Name string
}

func (t TextPoolTarget) normalized() (TextPoolTarget, error) {
	t.Type = strings.ToUpper(strings.TrimSpace(t.Type))
	t.Name = strings.ToUpper(strings.TrimSpace(t.Name))
	if t.Type == "" || t.Type == "PROGRAM" || t.Type == "REPORT" {
		t.Type = "PROG"
	}
	if t.Type == "CLASS" {
		t.Type = "CLAS"
	}
	if t.Type != "PROG" && t.Type != "CLAS" {
		return t, fmt.Errorf("text pools exist for programs and classes; %s is neither", t.Type)
	}
	if t.Name == "" {
		return t, fmt.Errorf("an object name is required")
	}
	return t, nil
}

// resource is the text elements resource, objectURL the object the gate
// resolves the package from, tadir the TADIR object type.
func (t TextPoolTarget) resource() string {
	if t.Type == "CLAS" {
		return "/sap/bc/adt/textelements/classes/" + url.PathEscape(strings.ToLower(t.Name))
	}
	return "/sap/bc/adt/textelements/programs/" + url.PathEscape(strings.ToLower(t.Name))
}

func (t TextPoolTarget) objectURL() string {
	if t.Type == "CLAS" {
		return "/sap/bc/adt/oo/classes/" + url.PathEscape(strings.ToLower(t.Name))
	}
	return "/sap/bc/adt/programs/programs/" + url.PathEscape(strings.ToLower(t.Name))
}

func (t TextPoolTarget) String() string { return t.Type + " " + t.Name }

// TextPoolKinds maps the classic READ TEXTPOOL id to the ADT document.
var TextPoolKinds = map[string]string{"I": "symbols", "S": "selections", "H": "headings"}

// Limits SAP enforces on the documents; a key or text past them is refused
// by the server for the whole document, so they are checked here first.
const (
	textKeyMaxLen       = 8
	selectionTextMaxLen = 30
)

var symbolKey = regexp.MustCompile(`^[0-9A-Z]{3}$`)

// textDocument is one text element document, lines kept in order.
type textDocument struct {
	entries []textEntry
}

type textEntry struct {
	directives []string // @... lines that precede the entry
	key        string
	text       string
}

func parseTextDocument(body string) *textDocument {
	d := &textDocument{}
	var pending []string
	for _, line := range strings.Split(body, "\n") {
		line = strings.TrimRight(line, "\r")
		if strings.TrimSpace(line) == "" {
			continue
		}
		if strings.HasPrefix(strings.TrimSpace(line), "@") {
			pending = append(pending, strings.TrimSpace(line))
			continue
		}
		eq := strings.Index(line, "=")
		if eq < 0 {
			continue
		}
		d.entries = append(d.entries, textEntry{directives: pending, key: strings.TrimSpace(line[:eq]), text: strings.TrimRight(line[eq+1:], "\r")})
		pending = nil
	}
	return d
}

func (d *textDocument) get(key string) (textEntry, bool) {
	for _, e := range d.entries {
		if strings.EqualFold(e.key, key) {
			return e, true
		}
	}
	return textEntry{}, false
}

// set changes an entry's text or appends one. An explicit text replaces a
// @DDICReference directive, which meant "take the DDIC label"; a symbol
// carries a @MaxLength the text must fit in, raised when it does not.
func (d *textDocument) set(kind, key, text string) {
	for i := range d.entries {
		if !strings.EqualFold(d.entries[i].key, key) {
			continue
		}
		d.entries[i].text = text
		var keep []string
		for _, dir := range d.entries[i].directives {
			if strings.HasPrefix(dir, "@DDICReference") {
				continue
			}
			if n, ok := strings.CutPrefix(dir, "@MaxLength:"); ok {
				if have, _ := strconv.Atoi(n); have < len([]rune(text)) {
					dir = fmt.Sprintf("@MaxLength:%d", len([]rune(text)))
				}
			}
			keep = append(keep, dir)
		}
		d.entries[i].directives = keep
		return
	}
	e := textEntry{key: key, text: text}
	if kind == "I" {
		n := len([]rune(text))
		if n < 40 {
			n = 40
		}
		e.directives = []string{fmt.Sprintf("@MaxLength:%d", n)}
	}
	d.entries = append(d.entries, e)
}

// String is the document as SAP wants it back. A selection text key is
// padded to eight; a symbol key and a heading key are not — SAP answers
// "Cannot parse the source code" to a padded symbol.
func (d *textDocument) String(kind string) string {
	var b strings.Builder
	for _, e := range d.entries {
		for _, dir := range e.directives {
			b.WriteString(dir + "\n")
		}
		if kind == "S" {
			fmt.Fprintf(&b, "%-8s=%s\n", e.key, e.text)
		} else {
			fmt.Fprintf(&b, "%s=%s\n", e.key, e.text)
		}
	}
	return b.String()
}

// headingKeys are the only keys a headings document has.
var headingKeys = []string{"listHeader", "columnHeader_1", "columnHeader_2", "columnHeader_3", "columnHeader_4"}

func headingKey(key string) (string, bool) {
	for _, k := range headingKeys {
		if strings.EqualFold(k, key) {
			return k, true
		}
	}
	return "", false
}

// TextPoolPlan is what a write would do, or did.
type TextPoolPlan struct {
	Target   string `json:"target"`
	Language string `json:"language"`
	// Master is the object's master language; a write to another language
	// is a translation and has to be asked for by name.
	Master    string     `json:"masterLanguage,omitempty"`
	Transport string     `json:"transport,omitempty"`
	Kinds     []KindPlan `json:"kinds"`
	Written   int        `json:"written"`
	Applied   bool       `json:"applied"`
	Activated bool       `json:"activated"`
	Notes     []string   `json:"notes,omitempty"`
}

// KindPlan is the plan for one document.
type KindPlan struct {
	Kind      string      `json:"kind"`
	Added     []KeyText   `json:"added,omitempty"`
	Changed   []KeyChange `json:"changed,omitempty"`
	Unchanged []string    `json:"unchanged,omitempty"`
	// Unknown are keys the document does not have — for selection texts, a
	// field the screen does not declare — and are not written.
	Unknown []string `json:"unknown,omitempty"`
	// Refused are keys or texts SAP would reject, with the reason.
	Refused []KeyReason `json:"refused,omitempty"`
	// Uncommented are the document's keys the caller said nothing about.
	Uncommented []string `json:"uncommented,omitempty"`
}

// KeyText is one text by key.
type KeyText struct {
	Key  string `json:"key"`
	Text string `json:"text"`
}

// KeyChange is one text that differs.
type KeyChange struct {
	Key string `json:"key"`
	Old string `json:"old"`
	New string `json:"new"`
}

// KeyReason says why a key was not written.
type KeyReason struct {
	Key    string `json:"key"`
	Reason string `json:"reason"`
}

// TextPoolOptions tune a write.
type TextPoolOptions struct {
	// DryRun computes the plan and writes nothing.
	DryRun bool
	// AnyLanguage allows writing a language other than the master language,
	// which is a translation; without it, that is refused.
	AnyLanguage bool
	// AllowUnknown writes selection texts for keys the screen does not
	// declare, which SAP accepts and SE38 never shows.
	AllowUnknown bool
}

// changes says whether the plan has anything to write.
func (p *TextPoolPlan) changes() int {
	n := 0
	for _, k := range p.Kinds {
		n += len(k.Added) + len(k.Changed)
	}
	return n
}

// TextPool reads every kind of one target in one language.
func (c *Client) TextPool(ctx context.Context, target TextPoolTarget, lang string) ([]TextPoolEntry, error) {
	t, err := target.normalized()
	if err != nil {
		return nil, err
	}
	if t.Type == "PROG" {
		return c.GetTextPoolInLanguage(ctx, t.Name, lang)
	}
	// A class has symbols only.
	doc, err := c.readTextDocument(ctx, t, "I", lang, false)
	if err != nil {
		return nil, err
	}
	var out []TextPoolEntry
	for _, e := range doc.entries {
		out = append(out, TextPoolEntry{ID: "I", Key: e.key, Text: e.text})
	}
	return out, nil
}

func (c *Client) readTextDocument(ctx context.Context, t TextPoolTarget, kind, lang string, stateful bool) (*textDocument, error) {
	doc := TextPoolKinds[kind]
	resp, err := c.transport.Request(ctx, t.resource()+"/source/"+doc, &RequestOptions{
		Method:           http.MethodGet,
		Accept:           "application/vnd.sap.adt.textelements." + doc + ".v1",
		OverrideLanguage: strings.ToUpper(lang),
		Stateful:         stateful,
	})
	if err != nil {
		if IsNotFoundError(err) {
			return &textDocument{}, nil
		}
		return nil, fmt.Errorf("reading the %s of %s: %w", doc, t, err)
	}
	return parseTextDocument(string(resp.Body)), nil
}

// MasterLanguage is the language the object was created in, from TADIR.
func (c *Client) MasterLanguage(ctx context.Context, target TextPoolTarget) (string, error) {
	t, err := target.normalized()
	if err != nil {
		return "", err
	}
	res, err := c.RunQuery(ctx, fmt.Sprintf("SELECT masterlang FROM tadir WHERE pgmid = 'R3TR' AND object = '%s' AND obj_name = '%s'", t.Type, sqlQuote(t.Name)), 1)
	if err != nil {
		return "", fmt.Errorf("reading the master language of %s: %w", t, err)
	}
	if res == nil || len(res.Rows) == 0 {
		return "", fmt.Errorf("%s is not in TADIR", t)
	}
	return cell(res.Rows[0], "MASTERLANG"), nil
}

// WriteTextPool writes texts of one or more kinds — S, I, H — to one
// target in one language and returns what it did. Keys not named keep
// their texts. Nothing is locked or written when nothing differs.
func (c *Client) WriteTextPool(ctx context.Context, target TextPoolTarget, lang string, texts map[string]map[string]string, transport string, opts TextPoolOptions) (*TextPoolPlan, error) {
	t, err := target.normalized()
	if err != nil {
		return nil, err
	}
	if err = c.checkMutation(ctx, MutationContext{Op: OpUpdate, OpName: "WriteTextPool", ObjectURL: t.objectURL(), Transport: transport}); err != nil {
		return nil, err
	}
	lang = strings.ToUpper(strings.TrimSpace(lang))
	if lang == "" {
		lang = strings.ToUpper(c.config.Language)
	}
	langKey := spras(lang)
	plan := &TextPoolPlan{Target: t.String(), Language: lang}

	master, err := c.MasterLanguage(ctx, t)
	if err != nil {
		return nil, err
	}
	plan.Master = master
	if master != "" && langKey != master && !opts.AnyLanguage {
		return plan, fmt.Errorf("%s's master language is %s; writing %s would be a translation — name the language explicitly to do that", t, master, lang)
	}

	kinds := make([]string, 0, len(texts))
	for k := range texts {
		k = strings.ToUpper(strings.TrimSpace(k))
		if _, ok := TextPoolKinds[k]; !ok {
			return nil, fmt.Errorf("text pool kind %q: want S (selection texts), I (text symbols) or H (headings)", k)
		}
		if t.Type == "CLAS" && k != "I" {
			return nil, fmt.Errorf("a class has text symbols only; %s has no %s", t, TextPoolKinds[k])
		}
		kinds = append(kinds, k)
	}
	sort.Strings(kinds)

	// The documents are read before the lock so a plan costs no lock, and
	// again under it for the write, so nothing changes in between unseen.
	docs := map[string]*textDocument{}
	for _, k := range kinds {
		d, rerr := c.readTextDocument(ctx, t, k, lang, false)
		if rerr != nil {
			return nil, rerr
		}
		docs[k] = d
		plan.Kinds = append(plan.Kinds, planKind(k, d, texts[k], opts))
	}
	if opts.DryRun || plan.changes() == 0 {
		return plan, nil
	}

	lock, err := c.LockObject(ctx, t.resource(), "MODIFY")
	if err != nil {
		return plan, fmt.Errorf("locking the text pool of %s: %w", t, err)
	}
	unlockCtx := context.WithoutCancel(ctx)
	unlocked := false
	unlock := func() {
		if !unlocked {
			unlocked = true
			_ = c.UnlockObject(unlockCtx, t.resource(), lock.LockHandle)
		}
	}
	defer unlock()
	if plan.Transport, err = c.resolveWriteTransport(transport, lock.CorrNr, "WriteTextPool"); err != nil {
		return plan, err
	}

	plan.Kinds = plan.Kinds[:0]
	for _, k := range kinds {
		d, err := c.readTextDocument(ctx, t, k, lang, true)
		if err != nil {
			return plan, err
		}
		kp := planKind(k, d, texts[k], opts)
		plan.Kinds = append(plan.Kinds, kp)
		if len(kp.Added)+len(kp.Changed) == 0 {
			continue
		}
		for _, a := range kp.Added {
			d.set(k, a.Key, a.Text)
		}
		for _, ch := range kp.Changed {
			d.set(k, ch.Key, ch.New)
		}
		params := url.Values{}
		params.Set("lockHandle", lock.LockHandle)
		if plan.Transport != "" {
			params.Set("corrNr", plan.Transport)
		}
		vocabulary := "application/vnd.sap.adt.textelements." + TextPoolKinds[k] + ".v1"
		if _, err := c.transport.Request(ctx, t.resource()+"/source/"+TextPoolKinds[k], &RequestOptions{
			Method:           http.MethodPut,
			Query:            params,
			Body:             []byte(d.String(k)),
			ContentType:      vocabulary + "; charset=UTF-8",
			Accept:           vocabulary,
			OverrideLanguage: lang,
			Stateful:         true,
		}); err != nil {
			return plan, fmt.Errorf("writing the %s of %s: %w", TextPoolKinds[k], t, err)
		}
		plan.Written += len(kp.Added) + len(kp.Changed)
	}
	plan.Applied = true
	if plan.Written == 0 {
		return plan, nil
	}
	// The PUT lands as an inactive version, and activating the program does
	// not carry it: texts written and left there read back as before from
	// the active version, and a later source edit may drop them. The text
	// elements are activated as their own object, after the unlock.
	unlock()
	if res, err := c.Activate(unlockCtx, t.resource(), t.Name); err != nil {
		plan.Notes = append(plan.Notes, "written but not activated: "+err.Error())
	} else if res != nil && !res.Success {
		plan.Notes = append(plan.Notes, "written but not activated: "+strings.Join(activationMessages(res), "; "))
	} else {
		plan.Activated = true
	}
	return plan, nil
}

func activationMessages(res *ActivationResult) []string {
	var out []string
	for _, m := range res.Messages {
		out = append(out, strings.TrimSpace(m.ShortText))
	}
	if len(out) == 0 {
		out = append(out, "activation reported no success and no message")
	}
	return out
}

// planKind compares wanted texts with a document.
func planKind(kind string, doc *textDocument, wanted map[string]string, opts TextPoolOptions) KindPlan {
	kp := KindPlan{Kind: kind}
	keys := make([]string, 0, len(wanted))
	byKey := make(map[string]string, len(wanted))
	for k, v := range wanted {
		k = strings.ToUpper(strings.TrimSpace(k))
		keys = append(keys, k)
		byKey[k] = v
	}
	sort.Strings(keys)
	seen := map[string]bool{}
	for _, k := range keys {
		text := strings.TrimRight(byKey[k], " ")
		seen[k] = true
		if k == "" {
			continue
		}
		switch kind {
		case "S":
			if len(k) > textKeyMaxLen {
				kp.Refused = append(kp.Refused, KeyReason{k, fmt.Sprintf("key longer than %d characters", textKeyMaxLen)})
				continue
			}
		case "I":
			if !symbolKey.MatchString(k) {
				kp.Refused = append(kp.Refused, KeyReason{k, "a text symbol key is three characters, digits or capitals (TEXT-001, TEXT-B01)"})
				continue
			}
		case "H":
			hk, ok := headingKey(k)
			if !ok {
				kp.Refused = append(kp.Refused, KeyReason{k, "a heading is listHeader or columnHeader_1..4"})
				continue
			}
			k = hk
		}
		if kind == "S" && len([]rune(text)) > selectionTextMaxLen {
			kp.Refused = append(kp.Refused, KeyReason{k, fmt.Sprintf("selection text longer than %d characters", selectionTextMaxLen)})
			continue
		}
		cur, exists := doc.get(k)
		switch {
		case !exists && kind != "I" && !opts.AllowUnknown:
			kp.Unknown = append(kp.Unknown, k)
		case !exists:
			kp.Added = append(kp.Added, KeyText{k, text})
		case cur.text == text:
			kp.Unchanged = append(kp.Unchanged, k)
		default:
			kp.Changed = append(kp.Changed, KeyChange{k, cur.text, text})
		}
	}
	for _, e := range doc.entries {
		if !seen[e.key] {
			kp.Uncommented = append(kp.Uncommented, e.key)
		}
	}
	return kp
}

// A hint, not a write. After a program is created or changed, the cheap
// question is whether its screen fields have texts: the selections document
// lists every field and says "?..." for one without, and the symbols
// document lists the TEXT-xxx a program defines. Neither needs the source
// parsed; the source is only scanned for TEXT-xxx tokens to say which
// symbols it uses that the pool does not define.

// TextPoolGaps names what is missing.
type TextPoolGaps struct {
	// Selections are screen fields with no selection text.
	Selections []string `json:"selections,omitempty"`
	// Symbols are TEXT-xxx keys the source uses and the pool lacks.
	Symbols []string `json:"symbols,omitempty"`
}

// Empty reports whether nothing is missing.
func (g TextPoolGaps) Empty() bool { return len(g.Selections) == 0 && len(g.Symbols) == 0 }

var textSymbolToken = regexp.MustCompile(`(?i)\bTEXT-([0-9A-Za-z]{3})\b`)

// TextPoolGaps reads the target's documents and compares them with the
// source given (may be empty, then only selection texts are checked).
func (c *Client) TextPoolGaps(ctx context.Context, target TextPoolTarget, lang, source string) (*TextPoolGaps, error) {
	t, err := target.normalized()
	if err != nil {
		return nil, err
	}
	gaps := &TextPoolGaps{}
	if t.Type == "PROG" {
		sel, err := c.readTextDocument(ctx, t, "S", lang, false)
		if err != nil {
			return nil, err
		}
		for _, e := range sel.entries {
			if strings.TrimSpace(e.text) == "" || strings.TrimSpace(e.text) == "?..." {
				gaps.Selections = append(gaps.Selections, e.key)
			}
		}
	}
	if source != "" {
		sym, err := c.readTextDocument(ctx, t, "I", lang, false)
		if err != nil {
			return nil, err
		}
		for _, key := range symbolsUsed(source) {
			if _, ok := sym.get(key); !ok {
				gaps.Symbols = append(gaps.Symbols, key)
			}
		}
		sort.Strings(gaps.Symbols)
	}
	return gaps, nil
}

// symbolsUsed lists the TEXT-xxx keys a source refers to, sorted, once
// each. Full-line comments and what follows a " are not code.
func symbolsUsed(source string) []string {
	seen := map[string]bool{}
	for _, line := range strings.Split(source, "\n") {
		code := line
		if i := strings.Index(code, "\""); i >= 0 {
			code = code[:i]
		}
		if strings.HasPrefix(strings.TrimSpace(code), "*") {
			continue
		}
		for _, m := range textSymbolToken.FindAllStringSubmatch(code, -1) {
			seen[strings.ToUpper(m[1])] = true
		}
	}
	keys := make([]string, 0, len(seen))
	for k := range seen {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}
