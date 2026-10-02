package main

import (
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

// Hit is one finding. It never carries the matched value: the scanner's output
// goes to public CI logs, and a list of what must not be published is itself
// a thing that must not be published.
type Hit struct {
	File    string
	Line    int
	Class   string // e.g. "private-ip", or "identifier/host"
	View    string // which byte view it was found in: text, utf-16le, hex, base64, ...
	Generic bool   // a built-in pattern, as opposed to an entry of the identifier list
	Path    string // repository path, for allow-file path rules; "" for a commit message
	Commit  string // the commit that added the line, when it was found in a patch
	value   string // kept for allow-file matching only; never printed
}

// Identifier is one entry of the identifier list.
type Identifier struct {
	Class string
	Value string
}

// A view is one way of reading a file's bytes. Every view's text is single-byte
// (ASCII or Latin-1), so an offset into it maps back to a line of the file.
type view struct {
	how     string
	text    string
	line    func(int) int
	packed  []byte // the decoded bytes, for addresses packed into four bytes
	decoded bool   // a hex or base64 decoding, where short identifiers are noise
}

// Runs of text that are really bytes: a capture pasted into a source file.
// Short ones too: "SID=" plus a three-character SID is seven bytes, 14 hex
// digits or 12 base64 characters with padding.
var (
	hexRun = regexp.MustCompile(`[0-9a-fA-F]{8,}`)
	b64Run = regexp.MustCompile(`[A-Za-z0-9+/_-]{6,}={0,2}`)
)

// In a decoded (hex or base64) view most bytes are random, and a short
// identifier such as a three-character SID turns up there by chance next to
// any non-alphanumeric byte. So in those views an identifier must stand as a
// token: each neighbour is the edge of the decoded run or printable ASCII
// punctuation or whitespace ("SID=QX7;", "QX7 100"), never a control or high
// byte. The text and UTF-16LE views keep the plain alphanumeric boundary.
func tokenEdge(c byte) bool {
	return (c >= 0x20 && c < 0x7f && !isAlnum(c)) || c == '\t' || c == '\n' || c == '\r'
}

type lineIndex []int

func newLineIndex(data []byte) lineIndex {
	var nl lineIndex
	for i, b := range data {
		if b == '\n' {
			nl = append(nl, i)
		}
	}
	return nl
}

// at is the 1-based line of byte offset i.
func (nl lineIndex) at(i int) int { return sort.SearchInts(nl, i) + 1 }

// utf16ASCII reads b as UTF-16LE and keeps the ASCII code units, one byte each,
// so offsets stay countable; any other unit becomes a NUL, which no pattern
// matches and every identifier boundary accepts.
func utf16ASCII(b []byte) string {
	out := make([]byte, len(b)/2)
	for i := range out {
		lo, hi := b[2*i], b[2*i+1]
		if hi == 0 && lo < 0x80 {
			out[i] = lo
		}
	}
	return string(out)
}

func constLine(n int) func(int) int { return func(int) int { return n } }

// views decodes first, so the patterns can match second: the text itself, the
// text read as UTF-16LE at both alignments, and every hex run and base64 block
// decoded and read as ASCII and as UTF-16LE.
func views(data []byte) []view {
	lines := newLineIndex(data)
	out := []view{{how: "text", text: string(data), line: lines.at}}
	for align := 0; align < 2 && align < len(data); align++ {
		a := align
		out = append(out, view{how: "utf-16le", text: utf16ASCII(data[a:]),
			line: func(i int) int { return lines.at(a + 2*i) }})
	}
	// packed: whether to look for four-byte addresses in the decoded bytes.
	// Only in a run long enough to be a payload (a 32-hex GUID, a 40-character
	// base64 block, read from where it starts): a short run is a word or an
	// identifier, and its bytes produce 192.168 by coincidence.
	addDecoded := func(how string, b []byte, at int, packed bool) {
		if len(b) < 4 {
			return
		}
		ln := constLine(lines.at(at))
		v := view{how: how, text: string(b), line: ln, decoded: true}
		if packed {
			v.packed = b
		}
		out = append(out, v)
		for align := 0; align < 2 && align < len(b); align++ {
			out = append(out, view{how: how + "+utf-16le", text: utf16ASCII(b[align:]), line: ln, decoded: true})
		}
	}
	for _, loc := range hexRun.FindAllIndex(data, -1) {
		// Both nibble alignments: a run can start one digit early (a stray
		// digit, a length prefix), which shifts every byte after it.
		for shift := 0; shift < 2 && loc[1]-loc[0]-shift >= 8; shift++ {
			run := data[loc[0]+shift : loc[1]]
			run = run[:len(run)/2*2]
			b := make([]byte, len(run)/2)
			if _, err := hex.Decode(b, run); err == nil {
				// A 40- or 64-digit run is a SHA-1 or SHA-256 digest (a
				// commit, a checksum): hashed bytes, which carry no address.
				addDecoded("hex", b, loc[0], shift == 0 && len(run) >= 32 && len(run) != 40 && len(run) != 64)
			}
		}
	}
	for _, loc := range b64Run.FindAllIndex(data, -1) {
		orig := strings.TrimRight(string(data[loc[0]:loc[1]]), "=")
		payload := len(orig) >= 40 && strings.ContainsAny(orig, "0123456789+/")
		run := strings.NewReplacer("-", "+", "_", "/").Replace(orig)
		// A block can start mid-token (a path, "key=..."), so try every
		// alignment rather than trust where the run happened to begin.
		for shift := 0; shift < 4 && len(run)-shift >= 6; shift++ {
			// Decode the whole tail: two or three characters past the last
			// full quantum are one or two more bytes (the final "SA" of a
			// padded "...SA==" is the last letter of a SID). A single
			// leftover character carries no whole byte.
			s := run[shift:]
			if len(s)%4 == 1 {
				s = s[:len(s)-1]
			}
			if b, err := base64.RawStdEncoding.DecodeString(s); err == nil {
				// A long Go identifier (a test name) is a base64-shaped run too;
				// encoded bytes all but always include a digit, + or /.
				addDecoded("base64", b, loc[0], shift == 0 && payload)
			}
		}
	}
	return out
}

// genericPattern needs no configuration: the shapes of a session cookie, a
// basic-auth header, a CSRF token, a private address or a config password.
type genericPattern struct {
	class string
	re    *regexp.Regexp
	group int               // submatch holding the value
	real  func(string) bool // false for a placeholder, an example, a non-value
}

var genericPatterns = []genericPattern{
	{"private-ip", regexp.MustCompile(`\b(?:10\.\d{1,3}\.\d{1,3}\.\d{1,3}|172\.(?:1[6-9]|2\d|3[01])\.\d{1,3}\.\d{1,3}|192\.168\.\d{1,3}\.\d{1,3})\b`), 0, nil},
	{"sap-session-cookie", regexp.MustCompile(`(?i)\bSAP_SESSIONID_[A-Z0-9]{3}_\d{3}=([^;\s"'&<>\\,]{16,})`), 1, notPlaceholder},
	{"sap-sso-cookie", regexp.MustCompile(`(?i)\bMYSAPSSO2=([^;\s"'&<>\\,]{16,})`), 1, notPlaceholder},
	{"basic-auth", regexp.MustCompile(`(?i)\bAuthorization["']?\s*[:=]\s*["']?Basic\s+([A-Za-z0-9+/]{8,}={0,2})`), 1, realBasic},
	{"csrf-token", regexp.MustCompile(`(?i)\bx-csrf-token["']?\s*[:=]\s*["']?([A-Za-z0-9+/_=-]{16,})`), 1, notPlaceholder},
	{"config-password", regexp.MustCompile(`(?i)"(?:password|sap_password|passwd)"\s*:\s*"([^"\\]{4,})"`), 1, notPlaceholder},
}

// genericClasses are the classes an allow-file `match` rule may name.
var genericClasses = func() map[string]bool {
	m := map[string]bool{"packed-private-ip": true}
	for _, p := range genericPatterns {
		m[p.class] = true
	}
	return m
}()

// placeholderValues are whole values documentation and tests use. Only an
// exact value is a placeholder: a word inside a value proves nothing, and a
// real password such as "MySecret2026!" contains one often enough.
var placeholderValues = map[string]bool{
	"pass": true, "passwd": true, "password": true, "pwd": true, "secret": true,
	"user": true, "username": true, "admin": true, "token": true, "value": true,
	"changeme": true, "change-me": true, "change_me": true, "redacted": true,
	"your-password": true, "your_password": true, "yourpassword": true,
	"your-token": true, "your-session-id": true, "placeholder": true,
	"example": true, "dummy": true, "test": true, "fetch": true, "required": true,
	"abc123": true, "12345678": true, "foobar": true, "hunter2": true, "secret123": true,
}

// placeholderShape: a template or an elision rather than a value: <...>,
// ${VAR}, {{var}}, $VAR, %s / %v, runs of x, *, . or the ellipsis, and the
// <env>_password / YOUR_PASSWORD_HERE family of the example configs.
var placeholderShape = regexp.MustCompile(`^(?:<[^<>]*>|\$\{[^}]*\}|\{\{[^}]*\}\}|\$[A-Za-z_][A-Za-z0-9_]*|%[sv]|[xX*.…-]+|(?i:(?:your|my|dev|prod|qa|test)[_-]?password(?:[_-]here)?))$`)

// notPlaceholder is false for an exact placeholder value or shape.
func notPlaceholder(v string) bool {
	if placeholderValues[strings.ToLower(v)] || placeholderShape.MatchString(v) {
		return false
	}
	return true
}

// realBasic is true when the base64 decodes to user:password and is not one of
// the placeholder pairs in documentation.
func realBasic(v string) bool {
	b, err := base64.StdEncoding.DecodeString(v)
	if err != nil {
		b, err = base64.RawStdEncoding.DecodeString(strings.TrimRight(v, "="))
	}
	if err != nil || !strings.Contains(string(b), ":") {
		return false
	}
	user, pass, _ := strings.Cut(string(b), ":")
	common := map[string]bool{"user": true, "username": true, "admin": true, "developer": true, "foo": true, "bar": true, "pass": true, "password": true, "": true}
	l := func(s string) string { return strings.ToLower(s) }
	if common[l(user)] && common[l(pass)] {
		return false
	}
	return notPlaceholder(pass)
}

// validIP rejects an address with an octet over 255, and a dotted run that is
// longer than four parts (a version or an OID, not an address).
func validIP(text string, start, end int, ip string) bool {
	for _, o := range strings.Split(ip, ".") {
		if len(o) > 1 && o[0] == '0' {
			return false
		}
		n := 0
		for _, c := range o {
			n = n*10 + int(c-'0')
		}
		if n > 255 {
			return false
		}
	}
	if start >= 2 && text[start-1] == '.' && isDigit(text[start-2]) {
		return false
	}
	if end+1 < len(text) && text[end] == '.' && isDigit(text[end+1]) {
		return false
	}
	return true
}

func isDigit(c byte) bool { return c >= '0' && c <= '9' }
func isAlnum(c byte) bool {
	return isDigit(c) || (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z')
}

func asciiLower(s string) string {
	b := []byte(s)
	for i, c := range b {
		if c >= 'A' && c <= 'Z' {
			b[i] = c + 32
		}
	}
	return string(b)
}

// packedAddresses finds a private address that never spells itself out: four
// bytes, the way a client address travels in a session GUID's node field. Only
// the two-byte prefixes (192.168, 172.16-31) are matched; a 10.x prefix is one
// byte, which any random blob produces once per 256 bytes.
func packedAddresses(b []byte) []string {
	var found []string
	for i := 0; i+4 <= len(b); i++ {
		a, c, d := b[i], b[i+1], b[i+3]
		if ((a == 192 && c == 168) || (a == 172 && c >= 16 && c <= 31)) && d != 0 && d != 255 {
			found = append(found, fmt.Sprintf("%d.%d.%d.%d", a, c, b[i+2], d))
		}
	}
	return found
}

// scanBytes runs every pattern over every view of one file.
func scanBytes(file string, data []byte, ids []Identifier) []Hit {
	var hits []Hit
	seen := map[string]bool{}
	note := func(h Hit) {
		key := h.File + "\x00" + h.Class + "\x00" + h.value + "\x00" + strconv.Itoa(h.Line)
		if seen[key] {
			return
		}
		seen[key] = true
		hits = append(hits, h)
	}
	lowerIDs := make([]string, len(ids))
	for i, id := range ids {
		lowerIDs[i] = asciiLower(id.Value)
	}
	for _, v := range views(data) {
		for _, p := range genericPatterns {
			for _, m := range p.re.FindAllStringSubmatchIndex(v.text, -1) {
				val := v.text[m[2*p.group]:m[2*p.group+1]]
				if p.class == "private-ip" && !validIP(v.text, m[0], m[1], val) {
					continue
				}
				if p.real != nil && !p.real(val) {
					continue
				}
				note(Hit{File: file, Line: v.line(m[0]), Class: p.class, View: v.how, Generic: true, value: val})
			}
		}
		for _, addr := range packedAddresses(v.packed) {
			note(Hit{File: file, Line: v.line(0), Class: "packed-private-ip", View: v.how, Generic: true, value: addr})
		}
		if len(ids) == 0 {
			continue
		}
		hay := asciiLower(v.text)
		for i, id := range ids {
			needle := lowerIDs[i]
			for from := 0; ; {
				k := strings.Index(hay[from:], needle)
				if k < 0 {
					break
				}
				k += from
				end := k + len(needle)
				from = k + 1
				if v.decoded {
					if (k > 0 && !tokenEdge(hay[k-1])) || (end < len(hay) && !tokenEdge(hay[end])) {
						continue
					}
				} else if (k > 0 && isAlnum(hay[k-1])) || (end < len(hay) && isAlnum(hay[end])) {
					continue
				}
				note(Hit{File: file, Line: v.line(k), Class: "identifier/" + id.Class, View: v.how, value: id.Value})
			}
		}
	}
	return hits
}
