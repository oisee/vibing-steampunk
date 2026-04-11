# Execution Semantics Steering Plan

**Date:** 2026-04-08
**Plan Type:** Steering plan
**Scope:** Code-unit contracts, execution semantics, local effects, and transitive effect summaries

---

## Goal

Build a reliable context-compression layer that can answer:

- what is this code unit’s callable contract?
- what does it do locally?
- what execution mode/boundary does it run under?
- what may happen transitively below it?

Without turning the graph into a noisy bundle of overloaded edges.

---

## Strategic Direction

The order of work should be:

1. unify contract model
2. add local execution/effect summaries
3. build context bundles for editing and review
4. add bounded transitive rollups

This is the correct order because local, explainable summaries are immediately useful and much cheaper to trust than deep inferred behavior.

---

## Milestones

### Milestone 1. Shared contract foundation

Objective:

- establish one internal `CodeUnitContract` model

Scope:

- method contract adapter from existing `MethodSignature`
- room for FM, FORM, report, transaction entry contracts later

Deliverables:

- internal contract structs
- adapter from current method-signature extraction
- short design note or package comments documenting intended expansion

Checkpoint:

- one existing consumer can use the shared contract model without changing user-facing behavior

Exit criteria:

- future readers no longer need to invent their own result shape

### Milestone 2. Local execution semantics and effect extraction

Objective:

- infer high-value local runtime facts from syntax and structure

Scope:

- methods first
- local detectors only

First detectors:

- DB read/write
- `COMMIT WORK` / `ROLLBACK WORK`
- update task
- async task
- RFC boundary
- `CALL TRANSACTION`
- `SUBMIT`
- dynamic calls
- state access categories

Deliverables:

- `ExecutionSemantics`
- `EffectProfile`
- confidence notes and explicit unknown handling

Checkpoint:

- a tool or test fixture can explain why a method is classified as DB-writer, transactional, async-boundary, or unknown

Exit criteria:

- local summaries are explainable and stable on representative fixtures

### Milestone 3. Method context bundle

Objective:

- produce a single high-signal context pack for editing a method

Scope:

- self contract
- self execution/effect summary
- sibling method contracts
- direct callers via examples
- direct callees with compact contract/effect badges
- lightweight metrics

Deliverables:

- `MethodContext` result model
- one query/handler/CLI path that returns it

Checkpoint:

- an agent can request one result and get enough context to edit a method without reading whole neighboring classes

Exit criteria:

- token cost is bounded and output is obviously more useful than raw call graph alone

### Milestone 4. Multi-entrypoint generalization

Objective:

- apply the same semantics to executable code units beyond methods

Scope:

- function modules
- FORM routines
- reports/program entrypoints
- transaction entrypoints
- APC callbacks

Deliverables:

- shared executable-unit classification
- initial support for at least one non-method kind

Checkpoint:

- same questions can be asked of a report or FM, not only a method

Exit criteria:

- execution/effect semantics are clearly code-unit generic

### Milestone 5. Bounded transitive effect rollups

Objective:

- answer “what may happen eventually below this unit?” without pretending certainty

Scope:

- depth-limited traversal
- custom-first traversal by default
- confidence-marked aggregation

Deliverables:

- `TransitiveEffectProfile`
- bounded traversal policy
- notes about dynamic-edge confidence loss

Checkpoint:

- tool can answer “eventually writes DB?”, “eventually commits?”, “eventually crosses RFC boundary?” with explainable bounded reasoning

Exit criteria:

- derived rollups are useful without flooding output or overstating certainty

---

## Milestone Ordering Rules

If there is pressure to reorder, keep these rules:

- do not do transitive propagation before local effect summaries exist
- do not add many new graph edge types for effect semantics
- do not expose a generic “pure” boolean before explicit effect categories are trustworthy
- do not optimize for breadth of code-unit kinds before one method-centric workflow is strong

---

## Checkpoints

### Checkpoint A. Contract model accepted

Questions:

- can current method-signature output map cleanly into `CodeUnitContract`?
- do future FM/FORM/report shapes fit without distortion?

Success signal:

- no new contract readers need bespoke output structs

### Checkpoint B. Local effect summaries trusted

Questions:

- are detected effects explainable from source?
- are unknowns represented honestly?
- are we avoiding false certainty on purity/LUW claims?

Success signal:

- users can inspect local effect output and see the exact statement classes that justify it

### Checkpoint C. Context pack is genuinely compressive

Questions:

- does `MethodContext` reduce prompt size versus raw source reading?
- are siblings, callers, and callees all bounded?
- are the chosen metrics actually decision-helpful?

Success signal:

- one result gives materially better edit context than source plus manual grep

### Checkpoint D. Non-method generalization holds

Questions:

- does the model still make sense for reports, transactions, and APC callbacks?
- are we distinguishing entry semantics from owned effects?

Success signal:

- same vocabulary works across code-unit kinds without awkward exceptions

### Checkpoint E. Transitive rollups remain honest

Questions:

- are traversal depth and confidence explicit?
- do dynamic calls degrade certainty properly?
- are “eventually” claims bounded and reproducible?

Success signal:

- transitive output reads like a careful summary, not a theorem we cannot prove

---

## Non-Goals

Not part of the first wave:

- full interprocedural proof of purity
- unrestricted whole-landscape effect closure
- exact runtime scheduling truth for all background/update/APC cases
- replacing the graph with a semantic engine
- broad UI work before the internal models are trusted

---

## First Concrete Slice

The best first implementation slice is:

1. add `CodeUnitContract`
2. add `ExecutionSemantics` and `EffectProfile`
3. populate them for methods only
4. add one `MethodContext` result shape

This gives the fastest user-visible value with the lowest semantic risk.

---

## Risks

### Risk 1. Semantic overclaiming

Failure mode:

- tool says “LUW-safe” or “pure” where static evidence is incomplete

Mitigation:

- use explicit categories
- mark confidence
- keep unknown as a first-class outcome

### Risk 2. Graph pollution

Failure mode:

- every effect becomes a new edge type

Mitigation:

- keep topology separate from effects and ranking signals

### Risk 3. Token bloat

Failure mode:

- context bundle grows into another large source dump

Mitigation:

- default caps on siblings, callers, callees, and depth

### Risk 4. Scope sprawl

Failure mode:

- trying to support methods, reports, transactions, FMs, and APC equally on day one

Mitigation:

- method-first workflow
- one non-method generalization only after method context proves out

---

## Acceptance Signals

This steering plan is succeeding when:

- agents ask for `MethodContext` instead of piecing context together manually
- effect summaries are short, explainable, and trusted
- transitive behavior is presented as bounded inference
- extension to reports/FMs/APC feels additive rather than architectural rework
