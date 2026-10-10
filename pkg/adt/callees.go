package adt

import (
	"context"
	"fmt"
	"net/url"
	"sort"
	"strings"
)

// This file answers the down direction — what does this object reach — and it
// exists because the resource everything used to ask is not there.
//
// /sap/bc/adt/cai/callgraph is advertised in the discovery document of none of
// 7.50, 7.57 and 7.58 and answers 404 "No suitable resource found" in both
// directions, checked with a CSRF token in hand so it is the resource that is
// missing and not the request. Every wrapper built on it — GetCallGraph,
// GetCallersOf, GetCalleesOf — reported "this object calls nothing" on every
// system it was ever pointed at. They are gone; the two directions are now
// served by the two sources that do exist:
//
//	up   — the where-used list behind SE84, in WhereUsed (dumpimpact.go)
//	down — the cross-reference tables SAP fills at activation, here
//
// The down direction is weaker than the up one and the difference is worth
// stating plainly. The where-used list is a resource that grades its answers;
// CROSS and WBCROSSGT are tables, they are keyed by *include* rather than by
// object, and they record references rather than calls. What comes back is
// "the code of this object mentions these things", which is a superset of what
// it calls at run time and a subset of what it might call — a dynamic
// CALL METHOD (name) appears nowhere. Callee.Calls separates the rows that are
// an invocation from the rows that are a type or a data reference, because an
// agent asking "what does this call" should not have to guess which is which.

// Callee is one thing an object's code reaches.
type Callee struct {
	Name string `json:"name"`
	// Kind is what the reference is: "method", "function module", "type",
	// "data", "subroutine", "report", "transaction", "program". It is derived
	// from the table's own type code, and an unrecognised code is passed
	// through rather than guessed at.
	Kind string `json:"kind"`
	// Component is the method or field named, when the reference names one:
	// WBCROSSGT records ZCL_VSP_UTILS\ME:JSON_STR, and the half after the
	// backslash is the part that says which method.
	Component string `json:"component,omitempty"`
	// Calls says this reference is an invocation — a method call, a CALL
	// FUNCTION, a PERFORM, a SUBMIT — rather than a mention of a type or a
	// constant. Both are real dependencies; only one is a call.
	Calls bool `json:"calls"`
	// Source names the table the row came from, because they have different
	// blind spots: WBCROSSGT knows nothing of CALL FUNCTION, CROSS knows
	// nothing of classes, and WBCROSSGTI knows only about a version that is
	// not running.
	Source string `json:"source"`
	// Inactive marks a reference recorded against a version of the object that
	// has not been activated. It is never mixed into an ordinary answer: the
	// question "what does this object reference" is about the code that runs,
	// and a row from the inactive index describes code that does not. Asked
	// for explicitly it is useful — "what will change when this is activated" —
	// and then every such row says so, in the field and in Source.
	Inactive bool `json:"inactive,omitempty"`
}

// calleeTarget is an object translated into the terms the cross-reference
// tables are keyed by: includes, not objects.
type calleeTarget struct {
	Name string
	Type string // CLAS, INTF, PROG, INCL, FUGR, FUNC
	// Group is the function group a module lives in, known only after TFDIR
	// has been asked.
	Group string
}

// Callees answers "what does this object's code reach" for one ADT object.
//
// It is one hop by construction. The tables could be walked further, but depth
// buys breadth and breadth here is a liability: two hops out of any class in a
// real system reaches most of the system, and an answer that includes
// everything says nothing.
// The two tables are read independently and either can fail on its own. A
// half-read answer is the dangerous case: CROSS is the only place a call to a
// function module appears, so losing it turns "I could not read CROSS" into the
// confident and wrong "this object calls no function modules". The gaps are
// returned alongside the rows for the same reason Unsearched exists — a result
// that could not look everywhere says so, and names what it missed.
func (c *Client) Callees(ctx context.Context, objectURI string) ([]Callee, []Unsearched, error) {
	target, err := calleeTargetFromURI(objectURI)
	if err != nil {
		return nil, nil, err
	}
	predicate, err := c.includePredicate(ctx, target)
	if err != nil {
		return nil, nil, err
	}

	var out []Callee
	var failures []error

	// WBCROSSGT: the OO half — classes, interfaces, methods, global types and
	// data. DIRECT and INDIRECT are asked for because only DIRECT is this
	// object's own doing; an INDIRECT row is a type implied by a type it did
	// name, and reporting those makes every class look like it depends on half
	// of DDIC.
	var gaps []Unsearched
	wb, wbErr := c.RunQuery(ctx,
		"SELECT INCLUDE, OTYPE, NAME, DIRECT FROM WBCROSSGT WHERE "+predicate, calleeRowLimit)
	if wbErr != nil {
		failures = append(failures, fmt.Errorf("WBCROSSGT: %w", wbErr))
		gaps = append(gaps, Unsearched{Object: "WBCROSSGT", Reason: wbErr.Error()})
	} else if wb != nil {
		// A name too long for WBCROSSGT's NAME column is stored there as a
		// hash. Decode before the rows are read as names, and drop the ones
		// that could not be decoded rather than reporting forty hex characters
		// as the thing this object references — the gap says so instead.
		if lost := c.ResolveLongNames(ctx, wb.Rows); len(lost) > 0 {
			gaps = append(gaps, lost...)
			failed := make(map[string]bool, len(lost))
			for _, l := range lost {
				failed[l.Object] = true
			}
			kept := wb.Rows[:0]
			for _, row := range wb.Rows {
				if !failed[strings.ToUpper(rowString(row, "NAME"))] {
					kept = append(kept, row)
				}
			}
			wb.Rows = kept
		}
		out = append(out, wbCrossCallees(wb.Rows, target)...)
	}

	// CROSS: the procedural half. A class calling a function module shows up
	// here and nowhere else — checked live: ZCL_VSP_GIT_SERVICE's call to
	// SSFC_BASE64_DECODE is a CROSS row and has no WBCROSSGT row at all.
	cross, crossErr := c.RunQuery(ctx,
		"SELECT INCLUDE, TYPE, NAME, PROG FROM CROSS WHERE "+predicate, calleeRowLimit)
	if crossErr != nil {
		failures = append(failures, fmt.Errorf("CROSS: %w", crossErr))
		gaps = append(gaps, Unsearched{Object: "CROSS", Reason: crossErr.Error()})
	} else if cross != nil {
		out = append(out, crossCallees(cross.Rows, target)...)
	}

	// An empty answer has one more reading, and it is one this can settle
	// rather than list. WBCROSSGTI is the same index for objects that have
	// unactivated changes — "Index Global Types for Inactive Objects Where-Used
	// List" in SAP's own words — and per object the two are disjoint: a class
	// with only an active version has its rows in WBCROSSGT and none here, and
	// a program with unactivated changes has the reverse. So an object that
	// looks like it references nothing may simply have its references filed
	// against a version this reader does not look at, and saying which is worth
	// one query — asked only when the answer is empty, which is the only time
	// it changes anything.
	// A query that failed and a query that returned nothing look identical
	// once the rows are merged, and they mean opposite things. Only an empty
	// answer that no failure could explain is reported as an empty answer.
	if len(out) == 0 && len(failures) > 0 {
		return nil, gaps, fmt.Errorf("the cross-reference tables could not be read for %s (%s); "+
			"callees are read from CROSS and WBCROSSGT over free SQL, so this answers nothing "+
			"if free SQL is blocked or the user may not read those tables", objectURI, joinErrors(failures))
	}

	return mergeCallees(out), gaps, nil
}

// InactiveReferenceCount answers the one question an empty callee list cannot
// answer about itself.
//
// WBCROSSGTI is the same index for objects carrying unactivated changes — SAP
// calls it "Index Global Types for Inactive Objects Where-Used List" — and per
// object the two are disjoint: a class with only an active version has its rows
// in WBCROSSGT and none here; a program edited and not activated has the
// reverse. So "no rows" has a reading that used to be listed as a possibility
// and can instead be decided.
//
// It is only worth asking when the list is empty, which is why it is a separate
// call rather than a second query on every lookup. Its own failure returns
// zero: a probe that cannot run leaves the empty answer exactly as ambiguous as
// it already was, and a caveat about a caveat helps nobody.
func (c *Client) InactiveReferenceCount(ctx context.Context, objectURI string) int {
	rows, _, err := c.inactiveCallees(ctx, objectURI)
	if err != nil {
		return 0
	}
	return len(rows)
}

// InactiveCallees answers a different question from Callees, which is why it is
// a different call and not a flag with a default.
//
// Callees says what an object references. This says what the *unactivated* copy
// of it references — the answer to "what will change when somebody activates
// this", and to nothing else. Merging the two would produce a list describing
// behaviour no running code has, which is the invented answer this package
// spends most of its comments guarding against; keeping the inactive rows out
// of sight would repeat the older mistake of an empty list that cannot say why.
// So: separate call, and every row it returns carries Inactive and a Source of
// WBCROSSGTI, so a row cannot be quoted out of context by accident.
func (c *Client) InactiveCallees(ctx context.Context, objectURI string) ([]Callee, []Unsearched, error) {
	rows, target, err := c.inactiveCallees(ctx, objectURI)
	if err != nil {
		return nil, nil, err
	}
	out := wbCrossCallees(rows, target)
	for i := range out {
		out[i].Inactive = true
		out[i].Source = "WBCROSSGTI"
	}
	return mergeCallees(out), nil, nil
}

func (c *Client) inactiveCallees(ctx context.Context, objectURI string) ([]map[string]interface{}, calleeTarget, error) {
	target, err := calleeTargetFromURI(objectURI)
	if err != nil {
		return nil, calleeTarget{}, err
	}
	predicate, err := c.includePredicate(ctx, target)
	if err != nil {
		return nil, target, err
	}
	res, err := c.RunQuery(ctx,
		"SELECT INCLUDE, OTYPE, NAME, DIRECT FROM WBCROSSGTI WHERE "+predicate, calleeRowLimit)
	if err != nil {
		return nil, target, err
	}
	if res == nil {
		return nil, target, nil
	}
	// The same long-name decoding the active index needs: a name too long for
	// CHAR(120) is a hash here too, and a hash reported as an object name is
	// the worst class of wrong answer this package has produced.
	_ = c.ResolveLongNames(ctx, res.Rows)
	return res.Rows, target, nil
}

// calleeRowLimit caps each table query. A class with more references than this
// is a class whose callee list nobody was going to read anyway, and the cap is
// what keeps one query on a kernel include from pulling thousands of rows
// through the data preview resource.
//
// The cap applies before includeBelongsTo does, so a class sharing a prefix
// with a much larger sibling could in principle have its own rows crowded out
// of the first 500. Nothing observed has come close, and the alternative —
// enumerating a class's includes first — is an extra round trip on every
// query to guard against a case that has not happened.
const calleeRowLimit = 500

// calleeTargetFromURI reads an ADT URI back into the object it names.
//
// This is the reverse of unitForFrame, and it is deliberately strict: a URI
// shape nobody has taught it about produces an error naming what it does
// understand, rather than a query built on a guess that comes back empty and
// reads as "this object calls nothing".
func calleeTargetFromURI(objectURI string) (calleeTarget, error) {
	path := strings.TrimSpace(objectURI)
	if i := strings.IndexAny(path, "#?"); i >= 0 {
		path = path[:i]
	}
	path = strings.TrimRight(path, "/")
	lower := strings.ToLower(path)

	// A method's own URI carries the class in the path and the method in the
	// fragment that was just cut off; the tables are keyed by include, and a
	// class's includes are per-method only in name. Asking about the class is
	// the honest answer to a method URI, and it is what the caller wanted.
	switch {
	case strings.Contains(lower, "/functions/groups/") && strings.Contains(lower, "/fmodules/"):
		group := segmentAfter(path, "/functions/groups/")
		module := segmentAfter(path, "/fmodules/")
		if group == "" || module == "" {
			return calleeTarget{}, fmt.Errorf("this function module URI names no group or no module: %s", objectURI)
		}
		return calleeTarget{Name: module, Type: "FUNC", Group: group}, nil
	case strings.Contains(lower, "/functions/groups/"):
		return calleeTarget{Name: segmentAfter(path, "/functions/groups/"), Type: "FUGR"}, nil
	case strings.Contains(lower, "/oo/classes/"):
		return calleeTarget{Name: segmentAfter(path, "/oo/classes/"), Type: "CLAS"}, nil
	case strings.Contains(lower, "/oo/interfaces/"):
		return calleeTarget{Name: segmentAfter(path, "/oo/interfaces/"), Type: "INTF"}, nil
	case strings.Contains(lower, "/programs/includes/"):
		return calleeTarget{Name: segmentAfter(path, "/programs/includes/"), Type: "INCL"}, nil
	case strings.Contains(lower, "/programs/programs/"):
		return calleeTarget{Name: segmentAfter(path, "/programs/programs/"), Type: "PROG"}, nil
	}

	return calleeTarget{}, fmt.Errorf("what %q calls cannot be asked: the cross-reference tables are keyed by include, "+
		"and includes are known for classes, interfaces, programs, includes, function groups and function modules", objectURI)
}

// segmentAfter takes the first path segment following marker and unescapes it,
// which is what turns %2Fsdf%2Fget_app_log back into /SDF/GET_APP_LOG.
func segmentAfter(path, marker string) string {
	i := strings.Index(strings.ToLower(path), marker)
	if i < 0 {
		return ""
	}
	rest := path[i+len(marker):]
	if j := strings.Index(rest, "/"); j >= 0 {
		rest = rest[:j]
	}
	return strings.ToUpper(unescapeSegment(rest))
}

// includePredicate builds the WHERE clause that selects this object's own
// includes, and is where most of the ways to get a wrong answer live.
//
// A class's includes are the name padded with '=' plus a two-or-more letter
// section — ZCL_FOO=====CM001, =====CI, =====CU. LIKE 'ZCL_FOO%' catches all of
// them and also catches ZCL_FOO_HELPER's, so every row is checked against
// unitForFrame afterwards. A program is its own include. A function group is
// L<group>* (/NS/L<group>* in a namespace), and a function module is one
// specific include inside that, which only TFDIR knows the number of.
func (c *Client) includePredicate(ctx context.Context, target calleeTarget) (string, error) {
	name := strings.ToUpper(strings.TrimSpace(target.Name))
	if err := checkSQLLiteral(name); err != nil {
		return "", err
	}

	switch target.Type {
	case "CLAS", "INTF":
		return fmt.Sprintf("INCLUDE LIKE '%s%%'", name), nil
	case "PROG", "INCL":
		return fmt.Sprintf("INCLUDE = '%s'", name), nil
	case "FUGR":
		ns, group := splitNamespace(name)
		return fmt.Sprintf("INCLUDE LIKE '%sL%s%%'", ns, group), nil
	case "FUNC":
		include, err := c.functionModuleInclude(ctx, target)
		if err != nil {
			return "", err
		}
		return fmt.Sprintf("INCLUDE = '%s'", include), nil
	}
	return "", fmt.Errorf("no include is known for a %s", target.Type)
}

// functionModuleInclude resolves a module to the one include that holds its
// body: TFDIR keeps the group in PNAME and the section number in INCLUDE, and
// L<group>U<nn> is the include the tables are keyed by. Checked live:
// BAL_LOG_CREATE is SAPLSBAL section 15, so LSBALU15; /DMO/FLIGHT_TRAVEL_CREATE
// is /DMO/SAPLFLIGHT_TRAVEL_API section 3, so /DMO/LFLIGHT_TRAVEL_APIU03.
//
// Asking about the module rather than about its group is the whole point of
// the round trip — a group's includes are every module in it, and "what does
// this module call" answered with the group's entire reference list is a
// different question with a much worse answer.
func (c *Client) functionModuleInclude(ctx context.Context, target calleeTarget) (string, error) {
	name := strings.ToUpper(strings.TrimSpace(target.Name))
	if err := checkSQLLiteral(name); err != nil {
		return "", err
	}
	res, err := c.RunQuery(ctx,
		fmt.Sprintf("SELECT PNAME, INCLUDE FROM TFDIR WHERE FUNCNAME = '%s'", name), 1)
	if err != nil {
		return "", fmt.Errorf("asking TFDIR which include holds %s: %w", name, err)
	}
	if res == nil || len(res.Rows) == 0 {
		return "", fmt.Errorf("function module %s is not in TFDIR, so the include holding its body is unknown", name)
	}
	pool := strings.ToUpper(strings.TrimSpace(fmt.Sprintf("%v", res.Rows[0]["PNAME"])))
	section := strings.TrimSpace(fmt.Sprintf("%v", res.Rows[0]["INCLUDE"]))
	ns, rest := splitNamespace(pool)
	group := strings.TrimPrefix(rest, "SAPL")
	if group == "" || section == "" {
		return "", fmt.Errorf("TFDIR names no group or no section for %s", name)
	}
	include := poolIncludeFor(ns+group, section)
	if err := checkSQLLiteral(include); err != nil {
		return "", err
	}
	return include, nil
}

// poolIncludeFor assembles the L<group>U<nn> include name. The padding is the
// part worth having a name for: TFDIR keeps the section as a number, the
// include wants two digits, and LZDEMO_LOGU5 matches nothing at all. A
// namespace stays in front of the L: /DEMO/LOG's section 3 is /DEMO/LLOGU03.
func poolIncludeFor(group, section string) string {
	ns, group := splitNamespace(strings.ToUpper(strings.TrimSpace(group)))
	section = strings.TrimSpace(section)
	if len(section) < 2 {
		section = strings.Repeat("0", 2-len(section)) + section
	}
	return ns + "L" + group + "U" + section
}

// checkSQLLiteral refuses a name that would not survive being pasted into a
// WHERE clause. The name arrives from a URI a caller supplied, and RunQuery
// sends freestyle SQL, so a quote in it would end the literal and the rest
// would be executed. Repository names are upper case letters, digits,
// underscore and the slashes of a namespace; nothing else has any business
// here.
func checkSQLLiteral(name string) error {
	if name == "" {
		return fmt.Errorf("this object has no name to look up")
	}
	for _, ch := range name {
		switch {
		case ch >= 'A' && ch <= 'Z', ch >= '0' && ch <= '9', ch == '_', ch == '/', ch == '=':
		default:
			return fmt.Errorf("%q is not a repository name, so it is not put into a query", name)
		}
	}
	return nil
}

// wbCrossCallees turns WBCROSSGT rows into callees.
func wbCrossCallees(rows []map[string]interface{}, target calleeTarget) []Callee {
	var out []Callee
	for _, row := range rows {
		include := rowString(row, "INCLUDE")
		if !includeBelongsTo(include, target) {
			continue
		}
		// INDIRECT rows are types this object never named: SYST_DATUM arrives
		// because SY was named, and calling that a dependency of the code
		// would be true of every program ever written.
		if !strings.EqualFold(rowString(row, "DIRECT"), "X") {
			continue
		}
		name, component := splitCrossName(rowString(row, "NAME"))
		if name == "" || strings.EqualFold(name, target.Name) {
			// A class referring to its own attributes and methods is not a
			// callee; every class does it, in every method.
			continue
		}
		kind, calls := wbCrossKind(rowString(row, "OTYPE"))
		out = append(out, Callee{
			Name:      name,
			Kind:      kind,
			Component: component,
			Calls:     calls,
			Source:    "WBCROSSGT",
		})
	}
	return out
}

// crossCallees turns CROSS rows into callees.
func crossCallees(rows []map[string]interface{}, target calleeTarget) []Callee {
	var out []Callee
	for _, row := range rows {
		include := rowString(row, "INCLUDE")
		if !includeBelongsTo(include, target) {
			continue
		}
		name := strings.ToUpper(rowString(row, "NAME"))
		if name == "" {
			continue
		}
		kind, calls := crossKind(rowString(row, "TYPE"))
		component := ""
		// A PERFORM is recorded as the subroutine in NAME and the program that
		// owns it in PROG. The program is the object somebody can open; the
		// form is which part of it, so they swap round.
		if prog := strings.ToUpper(rowString(row, "PROG")); prog != "" {
			component, name = name, prog
		}
		if strings.EqualFold(name, target.Name) {
			continue
		}
		out = append(out, Callee{
			Name:      name,
			Kind:      kind,
			Component: component,
			Calls:     calls,
			Source:    "CROSS",
		})
	}
	return out
}

// includeBelongsTo guards the LIKE patterns. 'ZCL_FOO%' matches
// ZCL_FOO_HELPER's includes too, and a callee list quietly containing another
// class's dependencies is worse than a short one — so each include is mapped
// back to the object that owns it and compared.
func includeBelongsTo(include string, target calleeTarget) bool {
	include = strings.ToUpper(strings.TrimSpace(include))
	if include == "" {
		return false
	}
	switch target.Type {
	case "PROG", "INCL", "FUNC":
		// These were selected by an exact include, so there is nothing to
		// disambiguate.
		return true
	case "CLAS", "INTF":
		unit, ok := unitForFrame(DumpFrame{Program: include})
		return ok && strings.EqualFold(unit.Object, target.Name)
	case "FUGR":
		unit, ok := unitForFrame(DumpFrame{Program: include, Include: include})
		return ok && unit.Type == "FUGR" && strings.EqualFold(unit.Object, target.Name)
	}
	return false
}

// splitCrossName pulls the object out of a WBCROSSGT name.
//
// The format is the object, then components separated by backslashes with a
// two-letter tag: ZCL_VSP_UTILS\ME:JSON_STR is a method, and
// ZCL_VSP_UTILS\ME:BUILD_SUCCESS\DA:IV_DATA is one of that method's
// parameters. Only the first component is kept: the parameter of a method you
// call is not a separate thing you call.
func splitCrossName(raw string) (name, component string) {
	raw = strings.ToUpper(strings.TrimSpace(raw))
	if raw == "" {
		return "", ""
	}
	parts := strings.Split(raw, "\\")
	name = strings.TrimSpace(parts[0])
	if len(parts) > 1 {
		component = strings.TrimSpace(parts[1])
		// The tag says what kind of component it is, which Kind already
		// carries; what is wanted here is the name.
		if i := strings.Index(component, ":"); i >= 0 {
			component = component[i+1:]
		}
	}
	return name, component
}

// wbCrossKind reads WBCROSSGT's OTYPE.
func wbCrossKind(otype string) (kind string, calls bool) {
	switch strings.ToUpper(strings.TrimSpace(otype)) {
	case "ME":
		return "method", true
	case "TY":
		return "type", false
	case "DA":
		return "data", false
	case "":
		return "reference", false
	default:
		return strings.ToUpper(strings.TrimSpace(otype)), false
	}
}

// crossKind reads CROSS's TYPE.
//
// It is one character — checked against DD03L, CROSS-TYPE is C(1) — and the
// codes below are the ones seen on a live system. This is worth stating
// because two-letter codes 'FU', 'PR' and 'SU' appear in this codebase's
// history: they do not merely miss, the data preview resource rejects them
// with 400 "'FU' is not a valid value for C(1,0)", and the error was being
// swallowed as "no callers found".
func crossKind(t string) (kind string, calls bool) {
	switch strings.ToUpper(strings.TrimSpace(t)) {
	case CrossTypeFunctionModule:
		return "function module", true
	case CrossTypeReport:
		return "report", true
	case CrossTypeTransaction:
		return "transaction", true
	case CrossTypeSubroutine:
		return "subroutine", true
	case CrossTypeProgram:
		return "program", true
	case CrossTypeDialogModule:
		return "dialog module", true
	case "":
		return "reference", false
	default:
		return strings.ToUpper(strings.TrimSpace(t)), false
	}
}

// The CROSS type codes, as single characters. They are constants because they
// are the kind of thing that gets typed from memory as 'FU' and then fails in
// a way nobody sees.
const (
	CrossTypeFunctionModule = "F"
	CrossTypeReport         = "R" // SUBMIT
	CrossTypeTransaction    = "T" // CALL TRANSACTION
	CrossTypeSubroutine     = "U" // PERFORM; PROG holds the program that owns the form
	CrossTypeProgram        = "P"
	CrossTypeDialogModule   = "D"
)

// mergeCallees collapses the rows into one entry per object.
//
// One object is reached many times — a class whose three methods you call is
// three rows, plus a row for the type itself — and the object is the unit
// anybody acts on. A call outranks a type reference when both are present:
// "this class is a type I mention" is true whenever "I call its method" is,
// and the second is the one worth reporting.
func mergeCallees(in []Callee) []Callee {
	byName := map[string]int{}
	out := []Callee{}
	for _, callee := range in {
		key := trimUpper(callee.Name)
		at, seen := byName[key]
		if !seen {
			out = append(out, callee)
			byName[key] = len(out) - 1
			continue
		}
		if callee.Calls && !out[at].Calls {
			out[at].Kind = callee.Kind
			out[at].Calls = true
			out[at].Source = callee.Source
		}
		if callee.Component != "" && !strings.Contains(out[at].Component, callee.Component) {
			if out[at].Component == "" {
				out[at].Component = callee.Component
			} else {
				out[at].Component += ", " + callee.Component
			}
		}
	}

	sort.SliceStable(out, func(i, j int) bool {
		// Calls first: they are what "callees" was asked about, and a list
		// that opens with forty DDIC types buries them.
		if out[i].Calls != out[j].Calls {
			return out[i].Calls
		}
		return out[i].Name < out[j].Name
	})
	return out
}

// unescapeSegment undoes the escaping adtSegment applies, so that a namespaced
// object arriving as %2Fsdf%2Fget_app_log is looked up as /SDF/GET_APP_LOG.
func unescapeSegment(segment string) string {
	if unescaped, err := url.PathUnescape(segment); err == nil {
		return unescaped
	}
	return segment
}

func rowString(row map[string]interface{}, column string) string {
	v, ok := row[column]
	if !ok || v == nil {
		return ""
	}
	return strings.TrimSpace(fmt.Sprintf("%v", v))
}

func joinErrors(errs []error) string {
	parts := make([]string, 0, len(errs))
	for _, err := range errs {
		parts = append(parts, err.Error())
	}
	return strings.Join(parts, "; ")
}

// CallGraph is the one-hop graph of an object in either direction, kept in the
// shape the old CAI resource used so that what already consumes CallGraphNode
// keeps working.
//
// MaxDepth is accepted and ignored, and that is not laziness: neither source
// is recursive. The where-used list is one hop by nature and the
// cross-reference tables are keyed by include, so a second hop would be a
// second round of queries per child — dozens of them — for an answer that gets
// less useful the wider it gets. Depth was never real on the old resource
// either; it was a parameter on a request that 404'd.
func (c *Client) CallGraph(ctx context.Context, objectURI string, opts *CallGraphOptions) (*CallGraphNode, error) {
	if opts == nil {
		opts = &CallGraphOptions{Direction: "callees"}
	}
	direction := strings.ToLower(strings.TrimSpace(opts.Direction))
	if direction == "" {
		direction = "callees"
	}

	root := &CallGraphNode{
		URI:  objectURI,
		Name: objectNameFromURI(objectURI),
		Type: "OBJECT",
	}
	if target, err := calleeTargetFromURI(objectURI); err == nil {
		root.Type = target.Type
		root.Name = target.Name
	}

	switch direction {
	case "callers":
		callers, unresolved, err := c.WhereUsed(ctx, objectURI)
		if err != nil {
			return nil, err
		}
		root.Unsearched = unresolved
		for _, caller := range callers {
			if opts.MaxResults > 0 && len(root.Children) >= opts.MaxResults {
				break
			}
			root.Children = append(root.Children, CallGraphNode{
				URI:         caller.URI,
				Name:        caller.Name,
				Type:        caller.Type,
				Description: describeCaller(caller),
			})
		}
	case "callees":
		callees, gaps, err := c.Callees(ctx, objectURI)
		if err != nil {
			return nil, err
		}
		root.Unsearched = gaps
		for _, callee := range callees {
			if opts.MaxResults > 0 && len(root.Children) >= opts.MaxResults {
				break
			}
			root.Children = append(root.Children, CallGraphNode{
				Name:        callee.Name,
				Type:        callee.Kind,
				Description: callee.Component,
			})
		}
	default:
		return nil, fmt.Errorf("direction %q is not one this can answer: it is \"callers\" (the where-used list) or \"callees\" (the cross-reference tables)", opts.Direction)
	}

	return root, nil
}

func describeCaller(caller ExposedCaller) string {
	parts := []string{}
	if caller.Package != "" {
		parts = append(parts, caller.Package)
	}
	if caller.Component != "" {
		parts = append(parts, "in "+caller.Component)
	}
	return strings.Join(parts, " ")
}

// CalleeTargetNameFromURI reports the object name a callee lookup would read
// out of an object URI.
//
// Exported for the test that pins the namespace defect: the failure was that a
// URI built from /BOBF/CL_X carried no name at all, and the only way to see it
// without a live system is to read the name back out.
func CalleeTargetNameFromURI(objectURI string) (string, error) {
	target, err := calleeTargetFromURI(objectURI)
	if err != nil {
		return "", err
	}
	return target.Name, nil
}
