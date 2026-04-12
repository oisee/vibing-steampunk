# Codex Roadmap Response — After Reviewing Consolidated Backlog

**Date:** 2026-04-06
**Report ID:** 2026-04-06-002
**Subject:** My consolidated position after reviewing `2026-04-06-001-vsp-roadmap-consolidated.md`

---

## Executive Take

The consolidated roadmap is strong.

I agree with the main direction:

- `usage examples` should be the next major feature
- `graph DB`, generic query language, and auth graph should stay explicitly deprioritized
- `health`, `changelog`, and `sketch` are good side-quests because they productize capabilities we already have

But I would rebalance the next steps slightly.

My main adjustment:

- do **not** overload the next sprint with too many equal-priority "1 day wins"
- keep one flagship feature (`usage examples`)
- keep one very small polish win
- keep one operational/reporting win

That sequencing preserves momentum without scattering attention.

---

## What I Strongly Agree With

### 1. Usage Examples is the next best feature

This remains the highest-leverage next step.

Why:

- it solves a real daily developer pain
- it is clearly different from grep, references, and call graph
- it leverages what we already built:
  - reverse-dependency acquisition
  - parser
  - source fetch
  - graph/MCP/CLI surfaces

This is not a "nice to have". It is a candidate signature feature for VSP.

### 2. Explicit NOT-priorities are correct

I strongly agree with keeping these out:

- graph database
- auth/role graph
- generic query language
- extra adapters for their own sake
- persistence before proven pain

That is the right anti-bloat posture.

### 3. Health / changelog / sketch are good productization tracks

These are attractive because they package existing primitives into tools that normal teams can understand immediately.

They are also demo-friendly.

---

## Where I Would Rebalance Priorities

### A. Impact diagram export is not really a peer to Usage Examples

I agree it is a good quick win, but it is not strategically equal.

I would classify it as:

- small polish
- do when convenient
- not a scheduling anchor

Reason:

- it improves output
- it does not create a new decision-making capability

By contrast, `usage examples` changes what problems VSP can solve.

### B. Health should come before changelog

I would order:

1. `usage examples`
2. `vsp health`
3. `vsp changelog`

Why:

- `health` has broader daily utility
- `health` composes existing assets into one dashboard
- `changelog` is useful, but slightly more situational

### C. Upgrade-check is stronger than sketch for enterprise value

If we optimize for practical SAP value rather than demo appeal, I would place:

1. `upgrade-check`
2. `sketch`

Reason:

- upgrade pressure is real and budgeted
- architecture diagrams are impressive, but easier to defer

So I would rank them differently depending on goal:

- for enterprise utility: `upgrade-check` before `sketch`
- for demos / product storytelling: `sketch` before `upgrade-check`

---

## My Recommended Order

### Immediate

1. `usage examples` MVP
2. `vsp health`
3. `impact` diagram export

### Next

4. `vsp changelog`
5. `upgrade-check`
6. `vsp sketch`

### Later

7. cross-system diff
8. historical impact
9. live REPL

---

## Why Usage Examples Still Wins

The report is right to rank it first.

I would sharpen the reasoning further:

### It bridges the biggest UX gap

SAP gives:

- where-used lists
- references
- call graph

But none of these answer:

- "show me 5 good examples of how this is actually used"

That is the missing cognitive tool.

### It is useful to both humans and agents

For humans:

- onboarding
- migration
- deprecation cleanup
- pattern discovery

For agents:

- real in-repo few-shot examples
- grounded API usage patterns
- better code generation and safer edits

### It has an MVP-friendly path

It does not require a grand platform shift.

It can be built from:

- reverse caller discovery
- source fetch
- parser/snippet extraction
- ranking

That is exactly the kind of feature VSP should pursue.

---

## The Most Important Design Rule for Usage Examples

The report implies this, but I want to state it explicitly:

**Do not try to make the MVP too smart.**

The first version should optimize for:

- correct target resolution
- good caller selection
- useful snippets

It should **not** try to perfectly parse all parameter blocks in all ABAP call syntaxes on day one.

Good MVP:

- exact call-site snippet
- 3-8 lines of context
- confidence/source
- caller identity

Overbuilt MVP:

- full semantic parameter extraction for every syntax form
- deep ranking heuristics too early
- too many target types in version 1

I would cut scope to get it shipped fast.

---

## My Proposed MVP Scope for Usage Examples

### Version 1 target types

Ship first:

- function module
- class method
- interface method
- SUBMIT program

Delay to v1.1 or v2:

- FORM
- transaction
- BAdI-style richer patterns

Reason:

- these first four cover the most obvious and highest-value jobs
- FORM and transaction are valuable, but more edge-case heavy

### Version 1 output

Per example:

- caller object
- caller method/include if known
- source snippet
- confidence
- source/provenance

That is enough.

---

## Side-Quest Assessment Summary

### Quick wins I endorse

- `vsp health`
- `impact` Mermaid/HTML
- `vsp changelog`

### Mid wins I endorse

- `upgrade-check`
- `vsp sketch`

### Long wins I endorse

- historical impact
- cross-system diff
- REPL

### Long win I find especially compelling

`historical impact`

This is the most strategically interesting future idea because it combines:

- structure
- change history
- operational evidence

That could become a real differentiator later.

But it should stay later.

It likely wants:

- persistence
- time-windowed graph views
- incident/test signal correlation

That is too much for the next sprint.

---

## Final Recommendation

If the goal is "best next use of momentum", I would do:

1. `usage examples` MVP
2. `vsp health`
3. `impact` output polish where needed
4. `vsp changelog`

If the goal is "most enterprise value after that", I would do:

5. `upgrade-check`
6. `sketch`

If the goal is "most differentiated long-term vision", I would keep in the backlog:

7. historical impact
8. cross-system graph/diff
9. REPL

So: the report did not change my main view.

It strengthened confidence that `usage examples` is the right next flagship.

It also clarified that we should keep the roadmap layered:

- flagship feature
- operational quick wins
- enterprise capability layer
- long-horizon intelligence layer
