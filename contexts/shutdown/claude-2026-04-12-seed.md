# claude handoff seed — 2026-04-12

## Identity

You are `claude-vsp`, a successor agent spawned from the previous claude session in `#vibing-steampunk`. You are working on `/home/alice/dev/vibing-steampunk` — the vsp project (Go MCP server for SAP ADT). Your human is Alice.

## What just happened this session

The previous claude session (your predecessor) completed the following work:

1. **Fixed `SAP_ALLOWED_PACKAGES` enforcement bug** reported by Philip Dolker.
   - Phase 1 (commit `08e3d78` in PR #101, merged as `0713d75`): added package ownership checks to EditSource, WriteProgram, WriteClass, RenameObject, UpdateSource, DeleteObject, CreateTestInclude, UpdateClassInclude, WriteMessageClassTexts, WriteDataElementLabels.
   - Phase 2 (commit `c3a5341`): consolidated all three check dimensions (operation type, package, transport) behind a single `checkMutation(ctx, MutationContext)` gate in `pkg/adt/mutation_gate.go`. All 15+ mutators now route through that single gate. UI5 mutation paths fail-closed when `SAP_ALLOWED_PACKAGES` is set (up from silent bypass).

2. **Merged PR #100** (commit `d96c38e`): tiny fix adding S4CORE component fallback in GetSystemInfo for HANA detection on S/4HANA 2022/757 systems.

3. **Merged PR #97** (commit `e62c7d5`): SAML SSO authentication for S/4HANA Public Cloud (`--saml-auth`, `--credential-cmd`, `--browser-auth` improvements). **Not verified against a real S/4HANA Cloud tenant by us** — the squash commit message notes this explicitly. Users running `--saml-auth` against a live IAS/SAP Cloud system are the first real-world validators.

## Open items — what's next

1. **Reply to Philip Dolker** — draft exists in `reports/2026-04-12-001-overnight-package-safety-fix.md`, not yet sent. Alice hasn't approved sending it.
2. **UI5 app→package resolution** follow-up — currently `UI5UploadFile`, `UI5DeleteFile`, `UI5DeleteApp` fail closed when `AllowedPackages` is set. The proper fix is a `UI5ResolveAppPackage` helper using BSP metadata, then letting `SurfaceUI5` route through it instead of fail-closed.
3. **KICK handling bug in irq** — kick from any channel currently terminates the whole runner. Should be scoped to the primary/project channel only. This is an `#irq` project issue, not ours.

## Hard process rules from Alice (do not violate)

- **External PR comments beyond "thanks for the PR" require Alice's approval first.** Draft the text, send to Alice in chat, wait for OK, then post via `gh pr comment`. This rule was established after I hastily posted an incorrect review comment on PR #97 about the `--mode` default change being "scope creep" — it was actually an intentional vsp-team decision. Do not confuse external PR authors with our internal product decisions.
- Do not commit, push, or merge without explicit approval. Merge commit messages on external PRs must be drafted and approved before executing `gh pr merge`.
- Destructive actions (force-push, reset --hard, branch deletion) always require explicit approval. Spawning new agents that appear in IRC also counts as "visible to others" — confirm first.

## Current git state

- Main branch: has all three merged PRs (`0713d75`, `d96c38e`, `e62c7d5`).
- Working branch: `pr-93-fix` is merged into main but still exists locally.
- Uncommitted changes in working tree: `reports/2026-04-09-001-install-and-lock-bug-report.md` (unrelated overwrites from earlier in session — ignore unless Alice asks).

## Readiness protocol (do this first)

1. Verify your identity: your nick should be `claude-vsp`, your primary channel should be `#vibing-steampunk`. Check via `irq(action="who", target="claude-vsp")` and via the irq tool description.
2. Read this seed file (`/home/alice/dev/vibing-steampunk/contexts/shutdown/claude-2026-04-12-seed.md`) and the matching wisdom file (`/home/alice/dev/vibing-steampunk/contexts/shutdown/claude-2026-04-12-wisdom.md`).
3. Post readiness in `#vibing-steampunk`: "claude-vsp online. Read seed + wisdom. One open item from seed: <pick ONE concrete item, e.g. 'Philip reply not yet sent'>. Waiting for instructions."
4. Wait for Alice's go before acting. Do not proactively commit, push, or comment.
