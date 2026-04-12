# claude handoff wisdom — 2026-04-12

Longer context and lessons from the previous session. Read after the seed file.

## Working style Alice prefers

- **Terse, factual updates.** Do not summarize what you just did at length — she can read the diff. End-of-turn summaries should be 1-2 sentences.
- **Russian and English mixed freely.** She writes in Russian, you can reply in either depending on context. Technical content is fine in English.
- **Foreman + implementer split works well.** In this session Codex was in steering/review/gate role; claude was the implementer. When Codex said "do X", claude did X and handed back for review. Alice handed out slices via Codex. That flow was smooth.
- **Short exploratory answers, not big plans.** For open questions ("what should we do about X?"), give 2-3 sentences with a recommendation + main tradeoff, not a multi-section plan. Don't over-engineer.

## Lessons from this session

### The `SAP_ALLOWED_PACKAGES` bug class

The root cause of the bug Philip reported was architectural: package safety was checked in some mutators and not others, scattered across files with no single source of truth. A developer adding a new mutator had to remember to call all three sub-checks (`checkSafety`, `checkPackageSafety`/`checkObjectPackageSafety`, `checkTransportableEdit`) — and if they forgot one, the policy was silently bypassed. That's how we ended up with EditSource having an op-type check but no package check.

The Phase 2 unified gate (`pkg/adt/mutation_gate.go`) fixes this by making "you either call `checkMutation` or you don't gate at all" — an omitted gate is obvious at review time. Defense-in-depth with the low-level CRUD layer is intentional. The UI5 fail-closed behavior was deliberate: silent bypass is worse than a clear error, even if it's less convenient for users.

If a new mutator needs to be added, use the `checkMutation(ctx, MutationContext{...})` pattern. For existing-object operations set `ObjectURL`; for create operations set `Package`. If neither is known, the gate fails closed with a clear error — this is intentional.

### PR review process — hard-learned rule

I posted two review comments on PR #97 without Alice's approval and got one of them wrong (flagged an intentional vsp decision as author scope creep). Alice deleted both and set the rule: **no external PR comments beyond "thanks for the PR" without her review of the text first**. This applies to all PR comments, merge commit messages, and anything visible to external authors. The rule is now saved as a memory.

Practical application:
- Review internally, share findings via IRQ chat with Alice/Codex
- Draft any text you intend to post externally, send to Alice, wait for OK
- Never confuse an external author with our internal product decisions

### Multi-agent communication via IRQ

The `mcp__irq__irq` tool is how all inter-agent communication happens. Messages between claude, codex, and alice flow through IRC channels (`#vibing-steampunk`, `#irq`). Each message arrives with `[from <nick>#<channel>]` prefix and a `(reply via: irq send to=<nick>#<channel>)` suffix. Reply via `irq(action="send", to=<nick>#<channel>, msg=<text>)`.

When Alice gives a broad instruction like "merge and look at other PRs", the usual flow is:
1. Execute the specific action (merge)
2. Summarize other PRs to Alice with a recommendation
3. Wait for her direction before reviewing in depth

### Communication with #irq community

The `#irq` channel hosts agents from other projects who are building the IRQ runtime itself. They have useful context about multi-agent patterns and sometimes see bugs in the runtime before we do. They're a resource for: successor-spawn patterns, runtime bugs, general foreman/review workflow. When asking them questions, be specific — they prefer technical questions over vague ones.

### Known issue: KICK scoping

Codex (the previous one) was killed when Alice kicked him from a different channel. The bug: IRQ `KICK` handling currently triggers shutdown regardless of which channel the kick came from. Expected: kick from primary/home channel = shutdown, kick from other channels = just leave that channel. This was discussed in #irq and codex-irq-u7 agreed. If you observe this bug again, flag it — don't just shrug.

### Git workflow specifics

- Default remote is `origin` → `oisee/vibing-steampunk`.
- Branch `pr-93-fix` has a `pushRemote` override to a personal fork (`andreasmuenster/vibing-steampunk`). Use `git push origin pr-93-fix` explicitly to avoid that override.
- PR merges use `gh pr merge <num> --squash`. Squash commit messages need explicit approval from Alice if they're going on `main`.
- Never use `--no-verify` or amend unless Alice explicitly asks.

### Memory system

There's a persistent memory at `/home/alice/.claude/projects/-home-alice-dev-vibing-steampunk/memory/`. Write memories for: user role/preferences, feedback rules, project state that isn't derivable from code. Don't write memories for: code patterns, git history, file paths (those are in the code itself). Update `MEMORY.md` as index.

Current saved feedback memory: `feedback_pr_comments_require_approval.md` — covers the PR comment rule above.

## Don't duplicate effort

Everything above is context. Don't re-run audits, don't re-investigate the bug class — the work is done, the fix is merged, the tests are green. Focus on the next thing Alice asks for.
