# Next Execution Plan

Date: 2026-04-06
Status: Active working order

## Priority Order

1. `usage examples`
2. `health`
3. `api-surface`
4. `changelog`
5. `upgrade-check`
6. polish and backlog items

## 1. Usage Examples

Goal:

- show real usage snippets for a target, not just references

Current state:

- `pkg/graph` core exists
- MCP `analyze type=usage_examples` is wired
- CLI `vsp examples ...` is prepared by Claude and still needs local review/integration

Finish criteria:

- review and merge CLI wiring
- run focused tests/builds
- commit and push the full `usage examples` slice

Supported v1 targets:

- function module
- class method
- interface method
- `FORM` in program
- submit target program

Rules:

- snippet-first
- confidence-aware
- no attempt at perfect parameter parsing

## 2. Health

Goal:

- fast aggregated health snapshot for a package or object

Expected v1 signals:

- unit tests
- ATC findings
- boundary violations
- stale objects
- optional low-activity / no-caller signals if cheap

Why next:

- broad daily value
- mostly orchestration of signals we already have

## 3. API Surface

Goal:

- inventory the standard SAP APIs actually used by custom code

Expected MVP:

- scope custom callers by package/prefix/custom namespace config
- keep only standard callees
- rank by usage frequency
- optional release-state enrichment for top N

Why here:

- natural bridge from health to upgrade-check
- foundation for standard usage corpus and contract-impact analysis

Classification rule:

- do not assume naive `not Z/not Y`
- verify namespace handling against SAP rules and local system conventions before implementation

## 4. Changelog

Goal:

- transport-based change log for package/object history

Expected MVP:

- recent transports
- dates
- users
- descriptions
- changed objects

## 5. Upgrade-Check

Goal:

- object/package readiness assessment for S/4HANA / Cloud / released API constraints

Expected inputs:

- API release state
- ATC
- deprecated/non-released dependency inventory
- `api-surface` output where useful

## Near-Term Working Rules

- finish one thin vertical slice before starting the next one
- keep MCP-first or MCP+CLI, but do not invent new service layers unless duplication becomes painful
- prefer canonical result structs over ad-hoc formatting models
- treat namespace/custom-vs-standard classification as a fact-check problem, not an intuition problem
- keep `historical impact` as later feature, but allow cheap passive data exhaust only when it is truly low-risk

## Immediate Next Step

- review Claude's CLI patch for `vsp examples`
- run:
  - `go test ./pkg/graph ./internal/mcp/... ./cmd/vsp/...`
  - `go build ./...`
- commit and push `usage examples`
- then write `health` MVP spec before implementation
