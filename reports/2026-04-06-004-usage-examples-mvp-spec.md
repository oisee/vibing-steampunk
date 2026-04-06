# Usage Examples MVP Spec

Date: 2026-04-06
Status: Proposed implementation spec
Priority: Flagship next slice

## Goal

Build a tool that answers a different user question than `grep`, `where-used`, or `call graph`:

> Show me real usage examples of this thing in our codebase.

The output should favor concrete caller snippets with enough context to copy the pattern, not exhaustive reference lists.

## User Jobs

Primary jobs:

- understand how to call a function module or method correctly
- see what parameters real callers pass in practice
- find canonical usage patterns in custom code
- inspect how a report is `SUBMIT`-ed in real systems
- inspect how a legacy `FORM` is invoked via `PERFORM`
- help an LLM ground itself in real examples instead of inventing usage

Secondary jobs:

- estimate migration/refactor blast radius by looking at real call shapes
- find "best" examples instead of all examples
- spot deprecated-but-still-used APIs

## Non-Goals For MVP

Do not do these in v1:

- perfect parsing of all parameter blocks
- full path explanation across multiple hops
- transactions (`TCODE`) as first-class targets
- dynamic call resolution as exact truth
- graph persistence or graph DB integration
- generic query language

## Target Types In v1

Supported in MVP:

- function module
- class static or instance method
- interface method
- `FORM` in program
- submit target program

Deferred to v1.1 or later:

- transaction code via `TSTC`
- BAdI-specific flows
- DDIC/CDS usage examples

## UX Surfaces

### CLI

Examples:

```bash
vsp examples FUNC Z_MY_FM
vsp examples CLAS ZCL_API --method GET_DATA
vsp examples INTF ZIF_SERVICE --method EXECUTE
vsp examples PROG ZREPORT --form BUILD_OUTPUT
vsp examples PROG ZBATCH_RUN --submit
```

Recommended flags:

- `--top 5` default 5
- `--format text|json`
- `--package ZPKG` optional ranking bias, not hard filter
- `--custom-only` optional, prefer `Z*`/`Y*` callers only

### MCP

Examples:

```json
SAP(action="analyze", params={"type":"usage_examples","object_type":"FUNC","object_name":"Z_MY_FM"})
SAP(action="analyze", params={"type":"usage_examples","object_type":"CLAS","object_name":"ZCL_API","method":"GET_DATA"})
SAP(action="analyze", params={"type":"usage_examples","object_type":"INTF","object_name":"ZIF_SERVICE","method":"EXECUTE"})
SAP(action="analyze", params={"type":"usage_examples","object_type":"PROG","object_name":"ZREPORT","form":"BUILD_OUTPUT"})
SAP(action="analyze", params={"type":"usage_examples","object_type":"PROG","object_name":"ZBATCH_RUN","submit":true})
```

MCP should be the main structured surface. CLI is the fast human surface.

## Result Schema

```json
{
  "target": {
    "object_type": "CLAS",
    "object_name": "ZCL_API",
    "method": "GET_DATA"
  },
  "total_candidates": 14,
  "examples": [
    {
      "rank": 1,
      "caller": {
        "object_type": "CLAS",
        "object_name": "ZCL_ORDER_APP",
        "node_id": "CLAS:ZCL_ORDER_APP",
        "package": "ZORDER"
      },
      "match_kind": "METHOD_CALL",
      "source_kind": "ADT_CALL_GRAPH",
      "confidence": "HIGH",
      "why": "Exact method call found in custom class",
      "location": {
        "line": 142
      },
      "snippet": "lo_api->get_data( EXPORTING iv_id = lv_id IMPORTING es_data = ls_data )."
    }
  ]
}
```

Required fields per example:

- caller object
- match kind
- provenance/source kind
- confidence
- line or location when known
- short snippet
- one-line explanation of why this example ranked well

## Acquisition Pipeline

### 1. Normalize Target

Resolve input into a canonical target descriptor:

- FM name
- class + method
- interface + method
- program + form
- program submit target

This step should fail loudly on ambiguous input.

### 2. Discover Candidate Callers

Use the strongest source available per target type.

Preferred source order:

1. ADT call graph / callers API when it gives direct high-confidence callers
2. `CROSS` / `WBCROSSGT` reverse lookup as backbone fallback
3. parser-based local/procedural augmentation for gaps such as `PERFORM IN PROGRAM`

Target-specific guidance:

- FM:
  - ADT callers if available
  - fallback `CROSS TYPE=FU`
- class/interface method:
  - ADT callers first
  - fallback parser scan inside candidate classes/programs
- `FORM`:
  - parser-first is acceptable here
  - `PERFORM ... IN PROGRAM` must be supported in v1
- submit target program:
  - `CROSS TYPE=PR`
  - parser confirmation for concrete `SUBMIT`

### 3. Fetch Caller Source

For top candidate callers only:

- fetch typed source with known object type
- hard cap source fetch count in v1, for example 10 or 15

This is the main latency control.

### 4. Extract Call-Site Snippets

Parser responsibilities in v1:

- find exact line or local block where the target is used
- return a compact snippet with 2-5 lines of context
- detect call shape

Call shapes to recognize in v1:

- `CALL FUNCTION 'Z_FM'`
- `lo_obj->method( ... )`
- `zcl_cls=>method( ... )`
- `CALL METHOD ...`
- `PERFORM form IN PROGRAM prog`
- `SUBMIT prog ...`
- direct interface-method style that resolves through known method call syntax

Do not attempt perfect semantic extraction of every argument block in v1.
If needed, just return a high-quality context snippet around the call site.

### 5. Rank And Trim

Default output should be top 5 examples.

Proposed ranking order:

1. exact target match over heuristic match
2. confirmed source snippet over graph-only candidate
3. custom code over standard SAP code
4. same package or nearby package over distant package
5. cleaner/smaller caller over generated or glue-heavy caller
6. more recent transport history as a weak tie-breaker

### 6. Return Structured Result

Return both:

- machine-friendly JSON structure
- human-friendly text rendering in CLI

## Confidence Model

Three levels are enough for MVP:

- `HIGH`
  - exact source snippet confirms exact target usage
- `MEDIUM`
  - strong graph/reference evidence, but snippet extraction is weaker or indirect
- `LOW`
  - heuristic match only, or dynamic/unresolved pattern

Important rule:

- dynamic calls may appear in snippets, but must not be advertised as exact target usages unless there is separate evidence

## Target-Type Notes

### Function Module

Best initial case. Clear syntax. Good flagship path.

### Class Method

Works well if ADT callers are available. Parser gives snippet confirmation.

### Interface Method

Need to distinguish:

- direct calls through interface-typed refs
- implementation-class calls

For v1, keep it honest:

- show exact interface-method call sites first
- do not over-promise full implementation graph semantics

### FORM

Must be in v1.

Reason:

- legacy ABAP depends on it heavily
- parser already gives this path a realistic foundation
- skipping `FORM` would make the feature feel modern-only

### SUBMIT

Good v1 target because syntax is concrete and highly useful operationally.

## Risks

### Latency

Fetching source for many callers can get slow.

Mitigations:

- top-N cap
- source-fetch cap
- rank candidates before fetch

### Noise

Popular APIs may have too many callers.

Mitigations:

- prefer custom callers
- prefer exact snippet-confirmed matches
- trim aggressively

### Parser Overreach

Trying to fully parse argument semantics in v1 will slow delivery.

Mitigation:

- return snippet + context first
- deepen parsing later only where it clearly improves value

## Passive Historical-Impact Data Exhaust

Do not build the feature now, but start collecting cheap signals where possible.

Recommended minimal step:

- when `co_change` runs, log resolved transport IDs and target object into a lightweight local cache/log

Constraints:

- zero user-facing behavior change
- best-effort only
- no new persistence architecture yet

This is a seed for later `historical impact`, not a user feature.

## Suggested Implementation Phases

### Phase 1

- MCP `usage_examples`
- CLI `vsp examples`
- support FM, class method, interface method, `FORM`, submit target program
- text + json output
- top 5 examples
- snippet extraction, not full argument semantics

### Phase 1.1

- better ranking
- package-bias and custom-only filters
- improved interface handling
- better submit/form snippet shaping

### Phase 2

- transaction support
- mermaid/html export if useful
- better historical ranking
- optional richer parameter extraction

## Recommended Build Order

1. function module path end-to-end
2. class method path
3. `FORM` path
4. submit target path
5. interface method path

Reason:

- FM proves the surface fast
- class methods cover modern custom code
- `FORM` ensures legacy credibility early
- submit is concrete and practical
- interface method semantics are slightly trickier, so keep it after the path is proven

## Final Recommendation

Build `usage examples` next as the flagship VSP slice.

Keep MVP strict:

- concrete examples
- snippet-first
- confidence-aware
- not too smart

If the first version reliably answers "show me how this is really used", it will already be more valuable than a much more ambitious but noisier design.
