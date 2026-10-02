package adt

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/url"
	"regexp"
	"strings"
)

// --- source summary: what a read returns instead of the source -------------
//
// A summary is the facts about a source text an agent needs to tell whether
// it already has it: lines, bytes and a sha256. It is computed from the text
// the read just fetched, so it costs no SAP round trip beyond that read; it
// only spares the caller the body.
//
// SourceSHA256 is NOT normalised: it is the lower-case hex SHA-256 of the
// exact bytes of the text as the read returns it (CRLF stays CRLF, a final
// newline stays). That is the text an agent receives from a normal read, so
// `printf '%s' "$text" | sha256sum` over what it got gives the same digest.
//
// It is a different value from the two other digests vsp hands out, on
// purpose:
//
//   - SourceHash ("sha256:<hex>", for expected_source_hash on a write) folds
//     CRLF to LF and drops trailing newlines, because a write must not be
//     refused over a line-ending difference ADT itself introduces. A summary
//     reports it too, as sourceHash, so an agent can go from a summary
//     straight to a guarded write.
//   - git_delete_objects' expect sha256 (git_versions.go) is computed on SAP
//     by ZADT_VSP over the object's whole abapGit serialisation: the SHA-256
//     of the lines "<file>=<sha256 of the file>" for every file (XML
//     metadata included), sorted, joined by LF. It covers more than one
//     source text and is never equal to a source's sha256. For an abapGit
//     file whose bytes are exactly this text, this sha256 is the hash on
//     that file's manifest line, nothing more.

// SourceSummary describes a source text without carrying it.
type SourceSummary struct {
	ObjectType string `json:"objectType"`
	Name       string `json:"name"`
	Parent     string `json:"parent,omitempty"`
	Include    string `json:"include,omitempty"`
	Method     string `json:"method,omitempty"`
	// URI is the ADT source the text was read from, when it follows from
	// the arguments alone (a FUNC without its group has none: finding the
	// group is the read's business, not the summary's).
	URI   string `json:"uri,omitempty"`
	Lines int    `json:"lines"`
	Bytes int    `json:"bytes"`
	// SHA256 is SourceSHA256 of the text: exact bytes, no normalisation.
	SHA256 string `json:"sha256"`
	// SourceHash is SourceHash of the text, for expected_source_hash.
	SourceHash string `json:"sourceHash"`
	// Unchanged is set only when the caller passed a digest to compare:
	// whether it is this text's sha256.
	Unchanged *bool `json:"unchanged,omitempty"`
}

// SourceSHA256 is the lower-case hex SHA-256 of the exact bytes of source,
// with no normalisation of any kind.
func SourceSHA256(source string) string {
	sum := sha256.Sum256([]byte(source))
	return hex.EncodeToString(sum[:])
}

// SourceLineCount counts the lines of source the way an editor does: the
// number of LF, plus one for a last line without one. "" has no lines; a
// CRLF ends one line.
func SourceLineCount(source string) int {
	if source == "" {
		return 0
	}
	n := strings.Count(source, "\n")
	if !strings.HasSuffix(source, "\n") {
		n++
	}
	return n
}

// SummarizeSource is the summary of source, read for objectType and name
// with opts.
func SummarizeSource(objectType, name string, opts *GetSourceOptions, source string) SourceSummary {
	if opts == nil {
		opts = &GetSourceOptions{}
	}
	objectType = strings.ToUpper(strings.TrimSpace(objectType))
	name = strings.ToUpper(strings.TrimSpace(name))
	return SourceSummary{
		ObjectType: objectType,
		Name:       name,
		Parent:     strings.ToUpper(opts.Parent),
		Include:    opts.Include,
		Method:     strings.ToUpper(opts.Method),
		URI:        SourceReadURI(objectType, name, opts),
		Lines:      SourceLineCount(source),
		Bytes:      len(source),
		SHA256:     SourceSHA256(source),
		SourceHash: SourceHash(source),
	}
}

// SourceReadURI is the ADT URI GetSource reads for these arguments, or ""
// when it does not follow from them alone.
func SourceReadURI(objectType, name string, opts *GetSourceOptions) string {
	if opts == nil {
		opts = &GetSourceOptions{}
	}
	n := url.PathEscape(strings.ToUpper(name))
	switch strings.ToUpper(objectType) {
	case "PROG":
		return "/sap/bc/adt/programs/programs/" + n + "/source/main"
	case "CLAS":
		if opts.Include != "" && opts.Method == "" {
			return GetClassIncludeSourceURL(name, ClassIncludeType(opts.Include))
		}
		return "/sap/bc/adt/oo/classes/" + n + "/source/main"
	case "INTF":
		return "/sap/bc/adt/oo/interfaces/" + n + "/source/main"
	case "FUNC":
		if opts.Parent == "" {
			return ""
		}
		return "/sap/bc/adt/functions/groups/" + url.PathEscape(strings.ToUpper(opts.Parent)) + "/fmodules/" + n + "/source/main"
	case "INCL":
		return "/sap/bc/adt/programs/includes/" + n + "/source/main"
	case "DDLS":
		return "/sap/bc/adt/ddic/ddl/sources/" + n + "/source/main"
	case "VIEW":
		return "/sap/bc/adt/ddic/views/" + n + "/source/main"
	case "BDEF":
		return "/sap/bc/adt/bo/behaviordefinitions/" + n + "/source/main"
	case "SRVD":
		return "/sap/bc/adt/ddic/srvd/sources/" + n + "/source/main"
	}
	return ""
}

var sourceSHA256Re = regexp.MustCompile(`^[0-9a-fA-F]{64}$`)

// ParseIfNoneMatch checks a caller's if_none_match: the sha256 of a summary
// (64 hex digits, any case; surrounding quotes and space are dropped, as an
// HTTP ETag would carry them). A sourceHash ("sha256:...") is refused rather
// than compared: it is normalised, so comparing it with the exact-bytes
// digest would quietly never match.
func ParseIfNoneMatch(v string) (string, error) {
	v = strings.Trim(strings.TrimSpace(v), `"`)
	if strings.HasPrefix(strings.ToLower(v), "sha256:") {
		return "", fmt.Errorf("if_none_match %q is a sourceHash (for expected_source_hash on a write); pass the summary's sha256 (64 hex digits, no prefix)", v)
	}
	if !sourceSHA256Re.MatchString(v) {
		return "", fmt.Errorf("if_none_match %q is not a sha256 (64 hex digits): read it with summary=true", v)
	}
	return strings.ToLower(v), nil
}

// SourceUnchangedText is the short answer to a read whose if_none_match is
// the source's sha256.
func SourceUnchangedText(s SourceSummary) string {
	return fmt.Sprintf("unchanged (sha256 %s): %s %s, %d lines, %d bytes; the source was not returned", s.SHA256, s.ObjectType, s.Name, s.Lines, s.Bytes)
}
