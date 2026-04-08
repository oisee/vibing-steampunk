# Health MVP Spec

Date: 2026-04-06
Status: Proposed implementation spec
Priority: Next after Usage Examples

## Goal

Build a fast aggregated health snapshot for a package or single object.

This feature should answer:

> In what shape is this package or object right now?

It is not meant to replace deep tools like ATC, unit tests, or boundary analysis.
It should orchestrate them into one compact operational report.

## User Jobs

Primary jobs:

- get a quick quality snapshot before editing a package
- spot obvious red flags before transport or refactor work
- see whether a package is testable, boundary-clean, and actively maintained
- give AI assistants one health summary instead of making them call five tools manually

Secondary jobs:

- package review before release
- daily team dashboard
- CI/export-friendly JSON health snapshot

## MVP Scope

Supported scopes:

- package health
- single object health

Recommended CLI:

```bash
vsp health $ZDEV
vsp health $ZDEV --format json
vsp health CLAS ZCL_ORDER_SERVICE
```

Recommended MCP:

```json
SAP(action="analyze", params={"type":"health","package":"$ZDEV"})
SAP(action="analyze", params={"type":"health","object_type":"CLAS","object_name":"ZCL_ORDER_SERVICE"})
```

## Signals In v1

### 1. Unit Test Signal

Goal:

- say whether tests for the package/object pass, fail, or are absent

Source:

- existing unit-test support in `pkg/adt`

Output:

- status: `PASS`, `FAIL`, `NONE`, or `ERROR`
- total classes / methods when available
- short failure count

### 2. ATC Signal

Goal:

- summarize static findings without dumping raw ATC output by default

Source:

- existing ATC check support in `pkg/adt`

Output:

- status: `CLEAN`, `FINDINGS`, or `ERROR`
- finding count
- top categories / severities if cheaply available

### 3. Boundary Signal

Goal:

- show whether the package has forbidden custom dependencies

Source:

- existing boundary analysis in `pkg/graph`
- existing CLI/MCP boundary tooling

Output:

- status: `CLEAN`, `VIOLATIONS`, or `ERROR`
- violation count
- top violating packages / objects

### 4. Staleness Signal

Goal:

- answer whether the package/object has gone cold

Source:

- transport history / revision metadata

MVP heuristic:

- most recent change date
- maybe oldest/youngest object date range for package view

Output:

- status: `ACTIVE`, `AGING`, `STALE`, or `UNKNOWN`
- last changed date

### 5. Optional Lightweight Reachability Signal

Only include if cheap:

- no callers / low usage / isolated objects

This must stay best-effort in v1.
Do not delay the feature for this.

## Result Shape

```json
{
  "scope": {
    "kind": "package",
    "name": "$ZDEV"
  },
  "summary": {
    "status": "WARN",
    "headline": "ATC findings and boundary violations detected"
  },
  "signals": {
    "tests": {
      "status": "PASS",
      "details": {
        "classes": 4,
        "methods": 27
      }
    },
    "atc": {
      "status": "FINDINGS",
      "details": {
        "count": 12
      }
    },
    "boundaries": {
      "status": "VIOLATIONS",
      "details": {
        "count": 3
      }
    },
    "staleness": {
      "status": "AGING",
      "details": {
        "last_changed": "2026-02-14"
      }
    }
  }
}
```

Human CLI output should be even simpler:

- one short headline
- one line per signal
- optional brief "top issues" section

## Summary Logic

Simple rule-based summary is enough for MVP:

- `FAIL` tests => overall `BAD`
- boundary violations or serious ATC findings => overall `WARN`
- all clean/pass/recent => overall `GOOD`
- missing data should not automatically mark package as bad

Avoid fake scoring in v1.

Do not invent a meaningless 78/100 health score.

## Non-Goals For v1

Do not include yet:

- coverage percentage
- sophisticated dead-code analysis
- semantic diff
- historical trend charts
- package architecture diagrams
- ownership/team mapping
- replacement suggestions

## Implementation Approach

Thin orchestration layer only.

Suggested shape:

1. normalize scope
2. run existing signal collectors
3. map raw outputs to compact statuses
4. build one canonical result struct
5. format as text/json

No new platform/service abstraction unless duplication becomes obvious.

## Data Source Notes

### Package Scope

Need:

- package contents
- tests for package objects or package root
- ATC for package
- boundary scan for package
- latest change/transport signal

### Object Scope

Need:

- object tests if applicable
- ATC on object
- boundary scan on object or object package with object focus
- last change date for object

## Risks

### Latency

ATC + tests + package traversal can be slow.

Mitigations:

- package scope can use bounded/default behavior
- allow later flags like `--fast`
- keep output honest if some signals were skipped or timed out

### Missing Data

Not every package has tests or easy revision history.

Mitigation:

- use `NONE` / `UNKNOWN` statuses instead of pretending failure

### Signal Overload

If CLI dumps raw ATC and raw boundary details together, it stops being a snapshot.

Mitigation:

- keep default text output compact
- put full detail in JSON or optional verbose mode later

## Suggested Build Order

1. canonical result structs
2. package-scope health via existing signals
3. object-scope health
4. compact text formatter
5. JSON support
6. MCP + CLI exposure

## Final Recommendation

Keep `health` aggressively practical:

- one snapshot
- four core signals
- no fake scoring
- no platform rewrite

If it reliably answers "is this package in decent shape or not?", it will already be high-value and easy to use daily.
