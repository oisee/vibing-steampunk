package main

import (
	"bufio"
	"fmt"
	"io"
	"regexp"
	"strings"
)

// An allowRule excuses hits that were looked at and judged not to be a leak.
// The allow-file is tracked, because it records a judgement worth reviewing,
// and it holds no secret: a rule names a path or the shape of a generic match,
// never an identifier from the list. Every rule needs a reason, so a later
// reader can tell a considered exception from one added to make CI green.
//
//	path  <glob>  <class>  <reason ...>
//	match <class> <regex>  <reason ...>
//
// A path rule's class is a hit class ("private-ip", "identifier/host"), or
// "generic" for every built-in pattern, or "*" for everything. A match rule's
// class must be a built-in (generic) class, or "generic", and its regex must
// match the whole matched value; it can never excuse an identifier, because
// that would need the identifier written down in a public file.
type allowRule struct {
	kind   string // "path" or "match"
	path   *regexp.Regexp
	class  string
	value  *regexp.Regexp
	reason string
	line   int
}

const minReasonLen = 10

func parseAllow(r io.Reader) ([]allowRule, error) {
	var rules []allowRule
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 64*1024), 1024*1024)
	for n := 1; sc.Scan(); n++ {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		f := strings.Fields(line)
		if len(f) < 3 {
			return nil, fmt.Errorf("line %d: want `path <glob> <class> <reason>` or `match <class> <regex> <reason>`", n)
		}
		reason := strings.TrimSpace(strings.Join(f[3:], " "))
		if len(reason) < minReasonLen {
			return nil, fmt.Errorf("line %d: no reason given (at least %d characters); an exception without a reason is refused", n, minReasonLen)
		}
		rule := allowRule{kind: f[0], reason: reason, line: n}
		switch f[0] {
		case "path":
			re, err := globRegexp(f[1])
			if err != nil {
				return nil, fmt.Errorf("line %d: %v", n, err)
			}
			rule.path, rule.class = re, f[2]
		case "match":
			rule.class = f[1]
			if rule.class != "generic" && !genericClasses[rule.class] {
				return nil, fmt.Errorf("line %d: a match rule may only name a built-in class or \"generic\", not %q: an identifier is excused by path only", n, rule.class)
			}
			re, err := regexp.Compile(`^(?:` + f[2] + `)$`)
			if err != nil {
				return nil, fmt.Errorf("line %d: %v", n, err)
			}
			rule.value = re
		default:
			return nil, fmt.Errorf("line %d: unknown rule %q (want path or match)", n, f[0])
		}
		rules = append(rules, rule)
	}
	return rules, sc.Err()
}

func classMatches(want string, h Hit) bool {
	return want == "*" || want == h.Class || (want == "generic" && h.Generic)
}

func allowed(rules []allowRule, h Hit) bool {
	for _, r := range rules {
		if !classMatches(r.class, h) {
			continue
		}
		switch r.kind {
		case "path":
			if r.path.MatchString(h.File) {
				return true
			}
		case "match":
			if h.Generic && r.value.MatchString(h.value) {
				return true
			}
		}
	}
	return false
}

// globRegexp turns a path glob into a regexp: `**` crosses directories, `*`
// and `?` do not.
func globRegexp(g string) (*regexp.Regexp, error) {
	var b strings.Builder
	b.WriteString("^")
	for i := 0; i < len(g); i++ {
		switch c := g[i]; c {
		case '*':
			if i+1 < len(g) && g[i+1] == '*' {
				i++
				if i+1 < len(g) && g[i+1] == '/' {
					i++
					b.WriteString("(?:.*/)?")
				} else {
					b.WriteString(".*")
				}
			} else {
				b.WriteString("[^/]*")
			}
		case '?':
			b.WriteString("[^/]")
		default:
			b.WriteString(regexp.QuoteMeta(string(c)))
		}
	}
	b.WriteString("$")
	return regexp.Compile(b.String())
}

var classPrefix = regexp.MustCompile(`^([a-z][a-z0-9-]{0,30}):\s*(.*)$`)

// parseIdentifiers reads the identifier list: one value per line, optionally
// `class: value` (host, ip, sid, user, ...). Blank lines and # comments are
// skipped. A value shorter than three characters would match everywhere, so it
// is refused rather than dropped in silence.
func parseIdentifiers(text string) ([]Identifier, error) {
	var ids []Identifier
	for n, raw := range strings.Split(text, "\n") {
		line := strings.TrimSpace(strings.TrimSuffix(raw, "\r"))
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		id := Identifier{Class: "identifier", Value: line}
		if m := classPrefix.FindStringSubmatch(line); m != nil && m[2] != "" {
			id = Identifier{Class: m[1], Value: strings.TrimSpace(m[2])}
		}
		if len(id.Value) < 3 {
			return nil, fmt.Errorf("identifier list line %d: a value under 3 characters would match everywhere", n+1)
		}
		ids = append(ids, id)
	}
	return ids, nil
}
