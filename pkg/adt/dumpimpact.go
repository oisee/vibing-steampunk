package adt

import (
	"context"
	"encoding/xml"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"sync"
)

// A dump says what failed. This file answers the other half of the question:
// who else runs the code that failed.
//
// It is deliberately not a rung on the ladder in correlate.go, and the reason
// is worth writing down because the ladder is right there and inviting. That
// ladder ranks application log entries by how well each argues it explains
// *this* failure. A caller that took part in this failure is on the dump's own
// stack, so scoreOnStack already has it. A caller that is not on the stack did
// not run — by construction — so nothing it ever wrote can be evidence for this
// dump, and giving it a rung would dress a coincidence up as structure, which
// is the one thing correlate.go was written to avoid.
//
// Blast radius also has no timestamp. It is a static fact about who *could*
// reach the broken code, true yesterday and true next month, and it does not
// belong in a list sorted against a five-minute window. So it gets its own
// answer, under its own flag.

// The other reason this file exists rather than a call to GetCallersOf: the
// call-graph resource those wrappers use, /sap/bc/adt/cai/callgraph, answers
// 404 "No suitable resource found" on 7.58 — in both directions, checked with
// a CSRF token in hand so it is the resource that is missing and not the
// request. Building on it would have produced a command that reports "nobody
// calls this" on every system, which is the worst possible failure mode for an
// impact query: silent, plausible and wrong.
//
// What does answer is the where-used list behind SE84,
// /sap/bc/adt/repository/informationsystem/usageReferences, which FindReferences
// already speaks. It is also the better source — it grades each reference and
// says which package the caller lives in.

// ExposedCaller is one object that can reach the failing code by a path other
// than the one this dump took.
type ExposedCaller struct {
	Name string `json:"name"`
	// Type is SAP's own code — CLAS/OC, FUGR/FF, PROG/P — kept as it arrives
	// rather than flattened, because the second half distinguishes a function
	// module from its group and that distinction is the useful part.
	Type string `json:"type,omitempty"`
	// URI is the caller's own ADT path, taken from the row SAP sent (the
	// container, or the row itself for a module or a program) rather than
	// rebuilt from the name — a namespaced object or a function module is not
	// addressable by any rule this side could apply.
	URI       string `json:"uri,omitempty"`
	Package   string `json:"package,omitempty"`
	Component string `json:"component,omitempty"` // the method or routine holding the reference
	IsTest    bool   `json:"is_test"`
	// Distance counts units between the failing statement and this caller: 0
	// means it calls the unit that died, 1 means it calls that unit's caller,
	// and so on outward along the dump's stack.
	Distance int    `json:"distance"`
	Via      string `json:"via"` // the unit it reaches
}

// ImpactUnit is one compiled unit taken from the dump, with who calls it.
type ImpactUnit struct {
	Object   string          `json:"object"`
	Type     string          `json:"type"`
	URI      string          `json:"uri"`
	Distance int             `json:"distance"`
	Frame    *DumpFrame      `json:"frame,omitempty"`
	Callers  []ExposedCaller `json:"callers,omitempty"`
	// Total is how many direct callers the system reported, before any cap.
	Total int `json:"total"`
	// Err records a unit whose where-used list could not be read. An impact
	// answer that quietly drops a unit is worse than one that says which unit
	// it could not ask about.
	Err string `json:"error,omitempty"`
	// Unresolved names the program includes among this unit's callers that
	// could not be resolved to their main program, and Gap says so in a
	// sentence. They are kept apart from Note on purpose: Note means the unit
	// could not be asked at all, while this unit was asked and answered, only
	// with some callers reported as an include rather than as its program.
	Unresolved []Unsearched `json:"unresolved,omitempty"`
	Gap        string       `json:"gap,omitempty"`
	// Note records a unit the query reached but cannot answer for, which is a
	// different and more dangerous thing than an error: it comes back 200 with
	// an empty list, and an empty list reads as "nobody calls this".
	Note string `json:"note,omitempty"`
}

// DumpImpactResult is the blast radius of one dump.
type DumpImpactResult struct {
	Dump  Dump         `json:"dump"`
	Units []ImpactUnit `json:"units"`
	// Exposed is the ranked, deduplicated answer: callers that are *not* on
	// this dump's stack, nearest the failing statement first.
	Exposed []ExposedCaller `json:"exposed"`
	// OnPath is the callers that are on the stack. They are the route this
	// dump actually took, so they are not additional exposure — they are kept
	// apart rather than dropped because seeing the known path confirms the
	// query aimed at the right object.
	OnPath []ExposedCaller `json:"on_path,omitempty"`
	// StackUnavailable says the release served the dump but not its stack, so
	// only the dump's own program could be asked about.
	StackUnavailable bool `json:"stack_unavailable,omitempty"`
}

// Answerable reports whether any unit produced a where-used list this query can
// stand behind. It exists so the caller can tell "nothing else calls this code"
// apart from "nothing here could be asked", which look identical in the numbers
// and mean opposite things.
func (r *DumpImpactResult) Answerable() bool {
	for _, u := range r.Units {
		if u.Err == "" && u.Note == "" {
			return true
		}
	}
	return false
}

// DumpImpactOptions tunes how far out and how much.
type DumpImpactOptions struct {
	// MaxUnits is how many units to walk outward from the failing statement.
	// Small on purpose: the further out you go the more the answer becomes
	// "everything reaches everything", and a caller of frame nine is exposed to
	// this bug only in the sense that it is exposed to the whole system.
	MaxUnits int
	// Limit caps the callers reported per unit. The true count is kept in
	// ImpactUnit.Total either way.
	Limit int
}

// DumpImpact answers "who else is exposed to this bug".
func (c *Client) DumpImpact(ctx context.Context, dump Dump, opts DumpImpactOptions) (*DumpImpactResult, error) {
	if opts.MaxUnits <= 0 {
		opts.MaxUnits = 3
	}
	if opts.Limit <= 0 {
		opts.Limit = 25
	}

	result := &DumpImpactResult{Dump: dump}

	var stack []DumpFrame
	if dump.ID != "" {
		frames, err := c.DumpStack(ctx, dump.ID)
		switch {
		case errors.Is(err, ErrDumpDetailUnavailable):
			// 7.50 serves the feed and not the detail resource. The dump still
			// names its own program, so the query narrows rather than fails.
			result.StackUnavailable = true
		case err != nil:
			result.StackUnavailable = true
		default:
			stack = frames
		}
	}

	units := impactUnits(dump, stack, opts.MaxUnits)
	if len(units) == 0 {
		return nil, fmt.Errorf("this dump names no program, so there is nothing to ask a where-used list about")
	}

	includes := c.newIncludeResolver()
	for i := range units {
		if note := unanswerable(units[i]); note != "" {
			units[i].Note = note
			continue
		}
		callers, unresolved, err := c.callersOf(ctx, units[i].URI, units[i].Object, includes)
		if err != nil {
			if ctxErr := ctx.Err(); ctxErr != nil {
				// Out of time is not one unit's problem: every unit after this
				// would fail the same way, and a partial answer would read as
				// a whole one.
				return nil, fmt.Errorf("dump impact stopped at %s: %w", units[i].Object, ctxErr)
			}
			units[i].Err = err.Error()
			continue
		}
		units[i].Unresolved = unresolved
		units[i].Gap = UnresolvedIncludesNote(unresolved)
		units[i].Total = len(callers)
		for j := range callers {
			callers[j].Distance = units[i].Distance
			callers[j].Via = units[i].Object
		}
		if len(callers) > opts.Limit {
			callers = callers[:opts.Limit]
		}
		units[i].Callers = callers
	}

	result.Units = units
	result.Exposed, result.OnPath = rankExposure(units, dump, stack)
	return result, nil
}

// impactUnits picks the objects to ask about, nearest the failure first.
//
// The dump's own program leads even when the stack is readable, because the
// innermost frame is not always the unit that failed: an RFC refused at the
// door dumps with %_RFC_START on the stack and names the module it could not
// reach only in its header. Where the two agree — the usual case — the
// deduplication makes it a no-op.
func impactUnits(dump Dump, stack []DumpFrame, max int) []ImpactUnit {
	var units []ImpactUnit
	seen := map[string]bool{}

	add := func(u repoUnit, frame *DumpFrame) {
		if u.URI == "" || seen[u.URI] || len(units) >= max {
			return
		}
		seen[u.URI] = true
		units = append(units, ImpactUnit{
			Object:   u.Object,
			Type:     u.Type,
			URI:      u.URI,
			Distance: len(units),
			Frame:    frame,
		})
	}

	if u, ok := unitForFrame(DumpFrame{Program: dump.Program}); ok {
		add(u, nil)
	}
	for i := range stack {
		if u, ok := unitForFrame(stack[i]); ok {
			frame := stack[i]
			add(u, &frame)
		}
	}
	return units
}

// unanswerable says why a unit's where-used list would come back empty for a
// reason that has nothing to do with how many callers it has.
//
// Found live, and it is the worst kind of wrong answer: asking about a function
// group returns 200 with zero results and a description reading "SBAL_DB -
// SAPLSBAL_DB (Include)". The group URI resolves to the group's main include,
// and nothing references a main include — so the list is empty by construction
// whether the group has one caller or a thousand. Callers live on the modules.
// A dump frame that names its module gets asked about the module and never
// lands here; a frame that only names the group has nothing askable, and saying
// so is the only honest option.
func unanswerable(unit ImpactUnit) string {
	if unit.Type == "FUGR" {
		return "a function group's where-used list resolves to its main include and comes back empty whatever the truth is; the callers are on the modules (vsp graph FUNC <module> --direction callers)"
	}
	return ""
}

// repoUnit is what a dump frame points at in the repository: an addressable
// object, not a compiled program name.
type repoUnit struct {
	Object string
	Type   string
	URI    string
}

// unitForFrame maps one dump stack frame to the object a where-used list can be
// asked about.
//
// The trap is that a dump names *compiled programs*, and only some of those are
// repository objects. A class pool arrives as ZCL_X========CP and a function
// group as SAPLZFOO; neither is addressable under /programs/programs, and
// asking for one there is a 404 that reads exactly like "nobody calls this".
// (correlate.go's programURI had the second half of that bug: it unwrapped
// class pools and sent function groups to the program path.)
//
// A FUNCTION frame is the good case. It names the module, the module has its
// own where-used list, and that list is much narrower than the whole group's —
// which is the difference between "who calls BAL_DB_SEARCH" and "who calls
// anything in SBAL_DB".
func unitForFrame(frame DumpFrame) (repoUnit, bool) {
	program := strings.TrimSpace(frame.Program)
	if program == "" {
		return repoUnit{}, false
	}

	// A class or interface pool: the name, padded with '=', then a two-letter
	// pool suffix. IP and IU are the interface ones.
	if i := strings.Index(program, "="); i > 0 {
		name := strings.TrimRight(program[:i], "=")
		suffix := strings.TrimLeft(program[i:], "=")
		if name == "" {
			return repoUnit{}, false
		}
		if strings.HasPrefix(suffix, "IP") || strings.HasPrefix(suffix, "IU") {
			return repoUnit{name, "INTF", "/sap/bc/adt/oo/interfaces/" + adtSegment(name)}, true
		}
		return repoUnit{name, "CLAS", "/sap/bc/adt/oo/classes/" + adtSegment(name)}, true
	}

	if group, ok := functionGroupOf(program, frame.Include); ok {
		base := "/sap/bc/adt/functions/groups/" + adtSegment(group)
		if module := functionModuleOf(frame); module != "" {
			return repoUnit{module, "FUNC", base + "/fmodules/" + adtSegment(module)}, true
		}
		return repoUnit{group, "FUGR", base}, true
	}

	return repoUnit{program, "PROG", "/sap/bc/adt/programs/programs/" + adtSegment(program)}, true
}

// functionGroupOf recovers the group behind a function pool. The main pool is
// SAPL<group>; the pieces are L<group>U01, L<group>F02, L<group>TOP and so on,
// and a dump frame often names one of those rather than the pool.
func functionGroupOf(program, include string) (string, bool) {
	if group, ok := groupFromPool(program); ok {
		return group, true
	}
	return groupFromPool(include)
}

func groupFromPool(name string) (string, bool) {
	name = strings.TrimSpace(strings.ToUpper(name))
	// In a namespace the pool prefix comes after it: /NS/SAPLGROUP and
	// /NS/LGROUPU01 belong to /NS/GROUP, so the checks below read the part
	// after the namespace and the namespace goes back in front of the group.
	ns, name := splitNamespace(name)
	if strings.HasPrefix(name, "SAPL") && len(name) > 4 {
		return ns + name[4:], true
	}
	// L<group><section>: the section is a letter and two more characters, or
	// the literal TOP. Anything shorter is not a function pool include.
	if len(name) > 4 && name[0] == 'L' {
		if strings.HasSuffix(name, "TOP") {
			return ns + name[1:len(name)-3], true
		}
		tail := name[len(name)-3:]
		if isPoolSection(tail) {
			return ns + name[1:len(name)-3], true
		}
	}
	return "", false
}

// splitNamespace separates a repository name into its namespace and the rest:
// /DEMO/LOG is "/DEMO/" and "LOG", ZDEMO_LOG is "" and "ZDEMO_LOG". Function
// pool names put SAPL and L after the namespace, not before it, so whatever
// adds or strips those prefixes has to work on the second half.
func splitNamespace(name string) (namespace, rest string) {
	if strings.HasPrefix(name, "/") {
		if i := strings.Index(name[1:], "/"); i >= 0 {
			return name[:i+2], name[i+2:]
		}
	}
	return "", name
}

// isPoolSection recognises the U01/F02/I03/E01 suffix of a function pool
// include: one letter then two digits.
func isPoolSection(tail string) bool {
	if len(tail) != 3 {
		return false
	}
	if tail[0] < 'A' || tail[0] > 'Z' {
		return false
	}
	for _, ch := range tail[1:] {
		if ch < '0' || ch > '9' {
			return false
		}
	}
	return true
}

// functionModuleOf returns the module a FUNCTION frame names. A method name
// carries => or ~ and is not a module, so those are refused rather than turned
// into a URI that would 404.
func functionModuleOf(frame DumpFrame) string {
	if !strings.EqualFold(strings.TrimSpace(frame.Type), "FUNCTION") {
		return ""
	}
	name := strings.TrimSpace(frame.Name)
	if name == "" || strings.Contains(name, "=>") || strings.Contains(name, "~") {
		return ""
	}
	return name
}

// adtSegment lowercases a name for an ADT path and escapes it, which matters
// for namespaced objects: /SDF/GET_APP_LOG has to arrive as %2Fsdf%2Fget_app_log.
func adtSegment(name string) string {
	return url.PathEscape(strings.ToLower(strings.TrimSpace(name)))
}

// WhereUsed answers "who calls this" for one ADT object, over the where-used
// list SE84 uses. It is the same filtering the dump impact query relies on, and
// it is exported because "who calls this" is not a question only a dump asks.
//
// The name the object goes by is taken from its own URI, which is what lets the
// self-references be dropped.
//
// The second result names the program includes that could not be resolved to
// their main program; an empty one means every include was.
func (c *Client) WhereUsed(ctx context.Context, objectURI string) ([]ExposedCaller, []Unsearched, error) {
	return c.callersOf(ctx, objectURI, objectNameFromURI(objectURI), c.newIncludeResolver())
}

// callersOf is the one route from a where-used list to callers, shared by
// WhereUsed and DumpImpact so that graph, explain, the MCP tools and dumps
// --impact cannot disagree about who calls what.
//
// The Unsearched list names the program includes that could not be resolved
// to their main program. They are still in the caller list, as themselves; the
// list says the answer is less complete than it looks.
func (c *Client) callersOf(ctx context.Context, objectURI, target string, includes *includeResolver) ([]ExposedCaller, []Unsearched, error) {
	refs, err := c.FindReferences(ctx, objectURI, 0, 0)
	if err != nil {
		return nil, nil, err
	}
	return includes.resolve(ctx, exposedCallers(refs, target), target)
}

// objectNameFromURI recovers the object's own name from its ADT path. The
// escaping matters: a namespaced object arrives as %2Fdemo%2Fzreport and would
// otherwise never match itself.
func objectNameFromURI(objectURI string) string {
	path := objectURI
	if i := strings.IndexAny(path, "#?"); i >= 0 {
		path = path[:i]
	}
	path = strings.TrimRight(path, "/")
	i := strings.LastIndex(path, "/")
	if i < 0 {
		return ""
	}
	segment := path[i+1:]
	if unescaped, err := url.PathUnescape(segment); err == nil {
		segment = unescaped
	}
	return strings.ToUpper(segment)
}

// exposedCallers turns a where-used response into callers.
//
// Two filters carry the whole result, and both are easy to leave out and be
// badly wrong:
//
// The list is flat but two-level. A row with no usageInformation is a container
// — the object or package the rows under it belong to — and only the rows
// beneath it are references. Counting containers as callers doubles the answer.
//
// Grade separates a real reference from the target's own parts.
// gradeComponent rows are the object describing itself: every method of the
// class you asked about is listed as a component of it. Only gradeDirect is
// somebody else's code reaching in, so only gradeDirect is blast radius.
//
// One more thing gets dropped, found by running this against a live system:
// packages come back as containers of their own, with the package interfaces
// listed under them as direct references. A package naming an object in its
// interface is visibility, not a call — it cannot reach the broken code and it
// cannot be paged about — so package interfaces are not callers.
//
// What is dropped is decided by the row, never by its container. A package is
// also the container of every object that has no other parent, and that is
// every standalone program: on a live 8.16 developer edition, 11 of the 59
// direct references to BAPI_USER_GET_DETAIL were programs and program
// includes filed under a package, and judging them by their parent dropped all
// of them (#281). Which object a row stands for is callerOf's question.
//
// The answer still has program includes in it. Saying which program an
// include belongs to takes a request per include, so that is resolveIncludes'
// job, on the Client, and not this function's.
func exposedCallers(refs []UsageReference, target string) []ExposedCaller {
	containers := map[string]UsageReference{}
	for _, r := range refs {
		if r.UsageInformation == "" && r.URI != "" {
			containers[r.URI] = r
		}
	}

	var out []ExposedCaller
	for _, r := range refs {
		if !strings.Contains(r.UsageInformation, "gradeDirect") {
			continue
		}
		if isPackaging(r.Type) || isPackageAddress(r.URI) {
			continue
		}
		caller := callerOf(r, containers[r.ParentURI])
		caller.IsTest = strings.Contains(strings.ToLower(r.UsageInformation), "test")
		out = append(out, caller)
	}
	return mergeCallers(out, target)
}

// callerOf decides which object a reference row stands for.
//
// Usually that is the container: a row under a class is one of its methods,
// and the class is the unit that can be paged about. Three kinds of row are
// their own caller instead:
//
//   - a row filed under a package. The package is only where the object lives;
//     a standalone program or a program include has no other parent.
//   - a row with no container at all, for the same reason.
//   - a function module. It sits under its group, but it has its own address
//     and its own where-used list, and "BAPI_X calls this" is the answer while
//     "something in function group SU_USER calls this" is a search left to do.
//     A dump frame names the module too, so this is also what lets a caller on
//     the dump's own stack be recognised as one.
func callerOf(r, owner UsageReference) ExposedCaller {
	if strings.TrimSpace(owner.Name) == "" || isPackaging(owner.Type) || isFunctionModule(r.Type) {
		pkg := r.PackageName
		if isPackaging(owner.Type) {
			pkg = firstNonEmpty(pkg, owner.Name)
		}
		return ExposedCaller{
			Name:    strings.TrimSpace(r.Name),
			Type:    strings.TrimSpace(r.Type),
			URI:     addressOf(r.URI),
			Package: strings.TrimSpace(firstNonEmpty(pkg, owner.PackageName)),
		}
	}
	return ExposedCaller{
		Name:      strings.TrimSpace(owner.Name),
		Type:      strings.TrimSpace(owner.Type),
		URI:       strings.TrimSpace(owner.URI),
		Package:   strings.TrimSpace(firstNonEmpty(owner.PackageName, r.PackageName)),
		Component: strings.TrimSpace(r.Name),
	}
}

// mergeCallers folds rows naming one object into one caller and puts the
// answer in its reading order. It drops the target itself, which is not
// exposure: a class listing a reference to itself is the class.
func mergeCallers(in []ExposedCaller, target string) []ExposedCaller {
	// An index rather than a pointer: appending to out reallocates, and a
	// pointer into the old backing array would silently update nothing.
	byName := map[string]int{}
	var out []ExposedCaller
	for _, caller := range in {
		if caller.Name == "" || equalFoldTrim(caller.Name, target) {
			continue
		}
		key := trimUpper(caller.Name)
		if at, seen := byName[key]; seen {
			// One object can reference the target from several routines; the
			// object is the unit of exposure, so the extra rows only add to the
			// component list rather than becoming separate callers.
			for _, part := range strings.Split(caller.Component, ", ") {
				if part != "" && !containsPart(out[at].Component, part) {
					if out[at].Component != "" {
						out[at].Component += ", "
					}
					out[at].Component += part
				}
			}
			// Productive use anywhere makes the object productive exposure.
			out[at].IsTest = out[at].IsTest && caller.IsTest
			continue
		}
		out = append(out, caller)
		byName[key] = len(out) - 1
	}

	sort.SliceStable(out, func(i, j int) bool {
		// Productive callers before tests: a test that exercises the broken
		// code is real exposure but not the one anybody is paged about.
		if out[i].IsTest != out[j].IsTest {
			return !out[i].IsTest
		}
		return out[i].Name < out[j].Name
	})
	return out
}

func containsPart(list, part string) bool {
	for _, p := range strings.Split(list, ", ") {
		if p == part {
			return true
		}
	}
	return false
}

// maxIncludeLookups bounds the requests one answer makes to resolve includes.
// Each include costs a round trip, and a where-used list of a hub can name
// hundreds. Past the bound an include is reported as itself, which is still
// a true caller but not the program a dump stack names, so every include left
// over is listed in the answer's unresolved gap rather than passed off as
// resolved.
const maxIncludeLookups = 50

// includeLookupWorkers is how many mainprograms requests run at once. Small:
// these share the user's ICM session budget with everything else vsp does.
const includeLookupWorkers = 4

// includeLookup is the answer for one include: its main programs, or why
// there are none.
type includeLookup struct {
	mains []SearchResult
	err   error
}

// includeResolver resolves program includes to their main programs for one
// answer. It is shared across the units of a dump impact query, so an
// include that two units both name is asked about once, and the lookup cap
// counts across the whole answer rather than per unit.
type includeResolver struct {
	c       *Client
	mu      sync.Mutex
	cache   map[string]includeLookup
	lookups int
}

func (c *Client) newIncludeResolver() *includeResolver {
	return &includeResolver{c: c, cache: map[string]includeLookup{}}
}

// resolve replaces each program include in a caller list with the program it
// belongs to.
//
// An include is not a program anybody runs, and a dump stack names the main
// program, not the include. Reported as itself, RSCUA_USER_COMPARE_INIT would
// never match the RSCUA_USER_COMPARE frame on a stack, and a reader would be
// left to find the program by hand. The include's name is kept as the
// component, since that is where the reference actually sits.
//
// An include that could not be resolved — the lookup failed, or the cap was
// reached — stays in the list as itself, because dropping it would turn
// "could not ask" into "does not call". It is also returned as Unsearched, so
// the answer says it is incomplete rather than reading as whole. A cancelled
// or expired context is not a gap but a failed answer, and is returned as the
// error.
func (r *includeResolver) resolve(ctx context.Context, callers []ExposedCaller, target string) ([]ExposedCaller, []Unsearched, error) {
	// Decide which includes this call will ask about, under the shared cap,
	// before any request goes out — so which ones are left over does not
	// depend on which request finished first.
	var ask []string
	planned := map[string]bool{}
	r.mu.Lock()
	for _, caller := range callers {
		if !isProgramInclude(caller.Type) || caller.URI == "" {
			continue
		}
		if _, done := r.cache[caller.URI]; done || planned[caller.URI] {
			continue
		}
		if r.lookups >= maxIncludeLookups {
			continue
		}
		r.lookups++
		planned[caller.URI] = true
		ask = append(ask, caller.URI)
	}
	r.mu.Unlock()

	results := make([]includeLookup, len(ask))
	jobs := make(chan int)
	var wg sync.WaitGroup
	for w := 0; w < includeLookupWorkers && w < len(ask); w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := range jobs {
				if err := ctx.Err(); err != nil {
					results[i] = includeLookup{err: err}
					continue
				}
				mains, err := r.c.includeMainPrograms(ctx, ask[i])
				results[i] = includeLookup{mains: mains, err: err}
			}
		}()
	}
	for i := range ask {
		jobs <- i
	}
	close(jobs)
	wg.Wait()

	if err := ctx.Err(); err != nil {
		return nil, nil, fmt.Errorf("resolving includes to their programs: %w", err)
	}

	r.mu.Lock()
	for i, uri := range ask {
		r.cache[uri] = results[i]
	}
	cache := make(map[string]includeLookup, len(r.cache))
	for k, v := range r.cache {
		cache[k] = v
	}
	r.mu.Unlock()

	var out []ExposedCaller
	var gaps []Unsearched
	for _, caller := range callers {
		if !isProgramInclude(caller.Type) || caller.URI == "" {
			out = append(out, caller)
			continue
		}
		res, asked := cache[caller.URI]
		switch {
		case !asked:
			gaps = append(gaps, Unsearched{
				Object: caller.Name,
				Reason: fmt.Sprintf("not resolved to its main program: the cap of %d include lookups per answer was reached", maxIncludeLookups),
			})
			out = append(out, caller)
			continue
		case res.err != nil:
			gaps = append(gaps, Unsearched{Object: caller.Name, Reason: "not resolved to its main program: " + res.err.Error()})
			out = append(out, caller)
			continue
		case len(res.mains) == 0:
			// Answered, and the answer is that no program includes it. That is
			// a fact about the include, not a gap in this answer.
			out = append(out, caller)
			continue
		}
		for _, m := range res.mains {
			out = append(out, ExposedCaller{
				Name:      strings.TrimSpace(m.Name),
				Type:      strings.TrimSpace(m.Type),
				URI:       strings.TrimSpace(m.URI),
				Package:   firstNonEmpty(strings.TrimSpace(m.PackageName), caller.Package),
				Component: firstNonEmpty(caller.Component, caller.Name),
				IsTest:    caller.IsTest,
			})
		}
	}
	sort.SliceStable(gaps, func(i, j int) bool { return gaps[i].Object < gaps[j].Object })
	return mergeCallers(out, target), gaps, nil
}

// includeMainProgramsAccept is the only content type the mainprograms
// resource accepts: plain application/xml is answered 406. A refused lookup
// only leaves the include unresolved, so getting this wrong would never show
// as an error.
const includeMainProgramsAccept = "application/vnd.sap.adt.programs.includes.mainprograms+xml"

// includeMainPrograms asks ADT which programs an include is part of. The
// answer is an adtcore:objectReferences list, checked live on an 8.16
// developer edition: RSCUA_USER_COMPARE_INIT answers RSCUA_USER_COMPARE
// (PROG/P), and a function group's include answers the group (FUGR/F).
func (c *Client) includeMainPrograms(ctx context.Context, includeURI string) ([]SearchResult, error) {
	resp, err := c.transport.Request(ctx, strings.TrimRight(includeURI, "/")+"/mainprograms", &RequestOptions{
		Method: http.MethodGet,
		Accept: includeMainProgramsAccept,
	})
	if err != nil {
		return nil, err
	}
	xmlStr := strings.ReplaceAll(string(resp.Body), "adtcore:", "")
	var refs SearchResults
	if err := xml.Unmarshal([]byte(xmlStr), &refs); err != nil {
		return nil, fmt.Errorf("parsing the main programs of %s: %w", includeURI, err)
	}
	var out []SearchResult
	for _, r := range refs.Results {
		if strings.TrimSpace(r.Name) != "" {
			out = append(out, r)
		}
	}
	return out, nil
}

// rankExposure flattens the per-unit answers into one ranked list and splits
// off the callers that are on the dump's own stack.
//
// Those are not additional exposure — they are the route this dump already
// took, and the stack printed above them says so. They are kept rather than
// dropped because seeing the known path in the answer is what confirms the
// query aimed at the object that actually failed.
func rankExposure(units []ImpactUnit, dump Dump, stack []DumpFrame) (exposed, onPath []ExposedCaller) {
	onStack := map[string]bool{}
	for _, name := range StackPrograms(stack) {
		if u, ok := unitForFrame(DumpFrame{Program: name}); ok {
			onStack[trimUpper(u.Object)] = true
		}
	}
	// A FUNCTION frame also names its module, and a module is a caller in its
	// own right, so the module counts as on the stack beside its group.
	for _, frame := range stack {
		if u, ok := unitForFrame(frame); ok {
			onStack[trimUpper(u.Object)] = true
		}
	}
	if u, ok := unitForFrame(DumpFrame{Program: dump.Program}); ok {
		onStack[trimUpper(u.Object)] = true
	}

	// Non-nil so the JSON says "no exposure" with [] rather than null, which a
	// consumer has to special-case and a person reads as "not computed".
	exposed = []ExposedCaller{}

	seen := map[string]bool{}
	for _, unit := range units {
		for _, caller := range unit.Callers {
			key := trimUpper(caller.Name)
			if seen[key] {
				// Nearest the failure wins, and units are walked outward, so
				// the first sighting is already the shallowest.
				continue
			}
			seen[key] = true
			if onStack[key] {
				onPath = append(onPath, caller)
				continue
			}
			exposed = append(exposed, caller)
		}
	}
	return exposed, onPath
}

// isPackaging recognises the rows that describe where an object is allowed to
// be seen from rather than who reaches it: packages (DEVC) and the package
// interfaces (PINF) that list their contents. SAPMSSY1's only direct reference
// on a live system is one of these, and reporting it would have said a kernel
// dispatcher has exactly one caller, which is a package.
func isPackaging(adtType string) bool {
	t := strings.ToUpper(strings.TrimSpace(adtType))
	return strings.HasPrefix(t, "DEVC") || strings.HasPrefix(t, "PINF")
}

// isPackageAddress catches a package interface row that arrives without a
// type: its URI is the package's own, with the interface in the fragment.
func isPackageAddress(uri string) bool {
	path := strings.ToLower(addressOf(uri))
	return strings.HasPrefix(path, "/sap/bc/adt/packages/") ||
		strings.Contains(path, "/object_type/pinf")
}

// addressOf drops the fragment from a row URI. A row's fragment points at a
// position in the source, which is not part of the object's address.
func addressOf(uri string) string {
	if i := strings.Index(uri, "#"); i >= 0 {
		uri = uri[:i]
	}
	return strings.TrimSpace(uri)
}

func isFunctionModule(adtType string) bool {
	return strings.EqualFold(strings.TrimSpace(adtType), "FUGR/FF")
}

// UnresolvedIncludesNote says which program includes in a caller list are
// listed as themselves because their main program could not be read. Empty
// when there are none. Every surface that prints a caller list prints this
// beside it, so a capped or failed lookup never reads as a complete answer.
func UnresolvedIncludesNote(unresolved []Unsearched) string {
	if len(unresolved) == 0 {
		return ""
	}
	var b strings.Builder
	fmt.Fprintf(&b, "%d program includes are listed as themselves because their main program could not be read, "+
		"so this is not a complete answer and a dump stack naming that program will not match them:", len(unresolved))
	const named = 5
	for i, u := range unresolved {
		if i >= named {
			fmt.Fprintf(&b, "\n  … and %d more", len(unresolved)-named)
			break
		}
		fmt.Fprintf(&b, "\n  %s: %s", u.Object, oneLine(u.Reason))
	}
	return b.String()
}

func isProgramInclude(adtType string) bool {
	return strings.EqualFold(strings.TrimSpace(adtType), "PROG/I")
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}
