// Package itf renders SAPscript ITF — the format of every documentation
// text in DOKTL: data element docs, report and function docs, IMG activity
// docs, the general texts they include — as Markdown.
//
// ITF is a line per record with a two-character paragraph format in front:
// AS for a standard paragraph, U1..U4 for headings, B1/B2 for bullets, N1
// for numbered items, "/" for a line break, "=" for a continuation, "/:"
// for a command such as INCLUDE, "/*" for a comment, and an empty format
// for "the paragraph above continues". Inside a line, <ex>, <zh>, <zk> and
// <ds:...> are character formats, and &NAME& on a line of its own is a
// chapter marker the viewer turns into a heading.
package itf

import (
	"regexp"
	"strconv"
	"strings"
)

// Line is one DOKTL record.
type Line struct {
	Format string `json:"format"`
	Text   string `json:"text"`
}

// Includer resolves an INCLUDE command to the lines of the included text;
// nil disables includes, which then appear as a note.
type Includer func(object, id string) ([]Line, error)

// ToMarkdown renders the lines. include may be nil.
func ToMarkdown(lines []Line, include Includer) string {
	r := &renderer{include: include, seen: map[string]bool{}}
	r.render(lines, 0)
	r.flush()
	return strings.TrimRight(r.out.String(), "\n") + "\n"
}

type renderer struct {
	out     strings.Builder
	para    []string // words of the paragraph being built
	prefix  string   // list marker or heading prefix for the open paragraph
	include Includer
	seen    map[string]bool
	number  int
}

func (r *renderer) flush() {
	if len(r.para) == 0 {
		r.prefix = ""
		return
	}
	var sb strings.Builder
	for i, w := range r.para {
		if i > 0 && !strings.HasSuffix(r.para[i-1], "\n") {
			sb.WriteByte(' ')
		}
		sb.WriteString(w)
	}
	text := inline(sb.String())
	if r.prefix != "" {
		r.out.WriteString(r.prefix + text + "\n")
		if strings.HasPrefix(r.prefix, "#") {
			r.out.WriteString("\n")
		}
	} else {
		r.out.WriteString(text + "\n\n")
	}
	r.para, r.prefix = nil, ""
}

func (r *renderer) open(prefix string, text string) {
	r.flush()
	r.prefix = prefix
	if t := strings.TrimSpace(text); t != "" {
		r.para = append(r.para, t)
	}
}

func (r *renderer) render(lines []Line, depth int) {
	for _, l := range lines {
		format := strings.TrimSpace(l.Format)
		text := l.Text
		if m := chapter(text); m != "" && (format == "" || strings.HasPrefix(format, "U") || format == "AS" || format == "*") {
			r.flush()
			r.out.WriteString("## " + m + "\n\n")
			continue
		}
		switch {
		case format == "":
			// Continuation of the paragraph above.
			if t := strings.TrimSpace(text); t != "" {
				if r.prefix == "" && len(r.para) == 0 {
					r.prefix = ""
				}
				r.para = append(r.para, t)
			}
		case format == "=":
			// Continuation with no blank between.
			if len(r.para) > 0 {
				r.para[len(r.para)-1] += strings.TrimSpace(text)
			} else {
				r.para = append(r.para, strings.TrimSpace(text))
			}
		case format == "/":
			// Line break inside the paragraph.
			if len(r.para) > 0 {
				r.para[len(r.para)-1] += "  \n"
			}
			if t := strings.TrimSpace(text); t != "" {
				r.para = append(r.para, t)
			}
		case format == "/:":
			r.command(l.Text, depth)
		case format == "/*", format == "/(", format == "/=", format == "PE":
			// Comments, protection and page controls carry nothing to read.
		case format == "LZ":
			r.flush()
		case strings.HasPrefix(format, "U") && len(format) == 2:
			level := 3
			if n, err := strconv.Atoi(format[1:]); err == nil && n >= 1 && n <= 4 {
				level = n + 1
			}
			r.open(strings.Repeat("#", level)+" ", text)
		case format == "HT", format == "HU", format == "TX":
			r.open("### ", text)
		case format == "B1", format == "BL", format == "B0", format == "L1", format == "LL":
			r.open("- ", text)
		case format == "B2", format == "B3":
			r.open("  - ", text)
		case format == "N1", format == "N2":
			r.number++
			r.open(strconv.Itoa(r.number)+". ", text)
		case strings.HasPrefix(format, "K") && len(format) == 2:
			r.open("", "<zh>"+strings.TrimSpace(text)+"</>")
		case strings.HasPrefix(format, "M") && len(format) == 2, strings.HasPrefix(format, "T") && len(format) == 2, strings.HasPrefix(format, "E") && len(format) == 2:
			r.open("    ", text)
		case strings.HasPrefix(format, ">"):
			r.open("| ", strings.ReplaceAll(text, "  ", " | "))
		default:
			// AS, *, AL and everything not named: a paragraph.
			r.number = 0
			r.open("", text)
		}
	}
}

var includeCmd = regexp.MustCompile(`(?i)^\s*INCLUDE\s+(\S+)\s+OBJECT\s+(\S+)\s+ID\s+(\S+)`)

func (r *renderer) command(text string, depth int) {
	m := includeCmd.FindStringSubmatch(text)
	if m == nil {
		return
	}
	object, id := strings.ToUpper(m[1]), strings.ToUpper(strings.TrimRight(m[3], "."))
	key := id + " " + object
	r.flush()
	if r.include == nil || depth > 4 || r.seen[key] {
		r.out.WriteString("_[included text " + key + "]_\n\n")
		return
	}
	r.seen[key] = true
	lines, err := r.include(object, id)
	if err != nil {
		r.out.WriteString("_[included text " + key + " not read: " + err.Error() + "]_\n\n")
		return
	}
	r.render(lines, depth+1)
	r.flush()
}

var chapterMarker = regexp.MustCompile(`^&([A-Z_0-9 ]+)&$`)

// chapter turns a marker line like &DEFINITION& into its heading text.
func chapter(text string) string {
	m := chapterMarker.FindStringSubmatch(strings.TrimSpace(text))
	if m == nil {
		return ""
	}
	words := strings.Fields(strings.ReplaceAll(m[1], "_", " "))
	for i, w := range words {
		words[i] = strings.ToUpper(w[:1]) + strings.ToLower(w[1:])
	}
	return strings.Join(words, " ")
}

var (
	tagOpen  = regexp.MustCompile(`<([A-Za-z]{2})(?::([^>]*))?>`)
	tagClose = "</>"
)

// inline turns the character formats into Markdown: <ex> code, <zh> bold,
// <zk> italic, <ds:ID.OBJECT> links kept as their text with the target after
// it, <(> and <)> the literal brackets.
func inline(s string) string {
	s = strings.ReplaceAll(s, "<(>", "\x00lt\x00")
	s = strings.ReplaceAll(s, "<)>", "\x00gt\x00")
	var out strings.Builder
	var stack []string
	for len(s) > 0 {
		if strings.HasPrefix(s, tagClose) {
			if n := len(stack); n > 0 {
				out.WriteString(closeFor(stack[n-1]))
				stack = stack[:n-1]
			}
			s = s[len(tagClose):]
			continue
		}
		if m := tagOpen.FindStringSubmatchIndex(s); m != nil && m[0] == 0 {
			tag := strings.ToUpper(s[m[2]:m[3]])
			target := ""
			if m[4] >= 0 {
				target = s[m[4]:m[5]]
			}
			stack = append(stack, tag+"\x00"+target)
			out.WriteString(openFor(tag))
			s = s[m[1]:]
			continue
		}
		out.WriteByte(s[0])
		s = s[1:]
	}
	for i := len(stack) - 1; i >= 0; i-- {
		out.WriteString(closeFor(stack[i]))
	}
	res := out.String()
	res = strings.ReplaceAll(res, "\x00lt\x00", "<")
	res = strings.ReplaceAll(res, "\x00gt\x00", ">")
	return res
}

func openFor(tag string) string {
	switch tag {
	case "EX", "LS":
		return "`"
	case "ZH", "KY":
		return "**"
	case "ZK", "EM":
		return "*"
	}
	return ""
}

func closeFor(entry string) string {
	tag, target, _ := strings.Cut(entry, "\x00")
	switch tag {
	case "EX", "LS":
		return "`"
	case "ZH", "KY":
		return "**"
	case "ZK", "EM":
		return "*"
	case "DS":
		if target != "" {
			return " (" + strings.ReplaceAll(target, ".", " ") + ")"
		}
	}
	return ""
}
