# 2026-04-15 Session Wisdom — Security Scrub + Lock-Handle Fix

Two unrelated threads landed in one session. Both worth remembering.

---

## 1. Security scrub — forward-only when password is rotated

A developer-driven audit caught tracked docs and test fixtures that
had accumulated environment-specific identifiers (SAP usernames,
internal hostnames, customer namespaces, transport IDs, one real
password) over dozens of feature commits. ~32 tracked files across
`reports/`, `contexts/`, `pkg/adt/{debugger,integration}_test.go`,
`cmd/vsp/{debug,devops}.go`, plus the `amdp-breakpoint-test-results-*.log`
artifact.

### Decision: Phase A only, no Phase B

- **Phase A (landed):** rotate the leaked credential out of band,
  scrub the tracked tree, rebuild any unpushed local feature commits
  via `git format-patch` + sed + `git am` (so leaks disappear from
  their own blobs too), expand `.gitignore` to cover operational
  scratch (`_bugs/`, `.local/`, `CR_*.md`, `cookie-*.txt`,
  `quickjs/`, `AGENTS.md`, `amdp-breakpoint-test-results-*.log`),
  add a sanitize policy section to `CLAUDE.md`, push additively
  as `chore(security): redact identifiers`.

- **Phase B (skipped):** `git filter-repo` history rewrite +
  force-push + tag re-sign + GitHub cache purge ticket.

### Why skip Phase B

1. **Password was rotated out of band** — the credential-class risk
   is already mitigated. Everything left is disclosure-class.
2. **Force-push on a tag-bearing repo is loud.** Every release SHA
   changes, every existing clone breaks, every watcher gets a
   force-push notification. Streisand effect on a cleanup meant to
   be quiet.
3. **GitHub cache keeps old commits for ~90 days** regardless of
   filter-repo. The benefit is smaller than the ergonomic cost.
4. **A `chore(security): redact` commit reads as mature hygiene**,
   not as an incident.

### Non-obvious gotcha — `.gitignore` coverage

Before this session, `.local/` was NOT in `.gitignore`. The user
had been assuming it was; a single `git add .` would have leaked
operational notes. Always **actually verify** ignore rules with
`git check-ignore -v <path>` — don't trust that a convention
("sensitive stuff goes in `.local/`") is matched by an actual rule.

### Also non-obvious — `git add -A` pulls everything

When committing a scrub, use `git add -u` (tracked-modifications
only), never `git add -A` (all files). The latter will pick up
every untracked path that is not explicitly ignored, which on a
repo with pending `.gitignore` fixes will drag in the very files
you are trying to protect.

### Sanitize policy — see CLAUDE.md

The authoritative redaction rules are codified in `CLAUDE.md`
under `## Security → Sanitize policy for tracked docs, tests, and
examples`. That file names the identifier families that must
never appear in tracked content, the synthetic placeholders to
use instead (`TESTUSER`, `dev.example.local`, `devsys{-adt}`,
`prodsys-{a,b}.example`, `trialsys.example`, `TR-EXAMPLE`,
`CR-EXAMPLE`, `ZDEMO_*` / `ZCL_DEMO_*` / `ZIF_DEMO_*` /
`$ZDEMO`), and the pre-agreed synthetic fixtures that remain
legal in the public tree (`$ZHIRTEST*`, `ZCL_HIRT*`,
`ZCUSTOM_DEVELOPMENT`). If in doubt, that file wins.

---

## 2. Lock-handle bug class (issues #88 / #91 / #92 / #98) — closed

Four open issues all reporting the same symptom: `LockObject`
returns a valid lock handle, the subsequent `PUT`/`POST` is
rejected a few seconds later with `HTTP 423
ExceptionResourceInvalidLockHandle`. Fixed in one commit
(`22517d4`) with two related defenses.

### Fix 1 — `Stateful: true` on class-include write paths

`CreateTestInclude` and `UpdateClassInclude` in
`pkg/adt/crud.go` both consume a lock handle but neither set
`Stateful: true` on their `RequestOptions`. Without that flag the
transport sends the write in stateless mode and SAP routes it
through a fresh HTTP session that has no record of the lock.

`EditSourceWithOptions` routes test-class and
definition/implementation include edits through
`UpdateClassInclude`, so **editing a class include reproduces the
bug 100% of the time** on systems where the TCP connection is
recycled between lock and write.

**The invariant — write it on the door:** any function in
`pkg/adt/crud.go` or a sibling file that takes a `lockHandle
string` parameter MUST set `Stateful: true` on the `RequestOptions`
passed to `transport.Request`. Regression-tested by the
`TestXxx_UsesStatefulSession` family in `crud_reconcile_test.go`
— inspect `X-sap-adt-sessiontype` on the captured request via
`headerCaptureMock`.

Full affected-function list as of this commit: `LockObject`,
`UnlockObject`, `UpdateSource`, `DeleteObject`, `CreateTestInclude`,
`UpdateClassInclude`. All six now set the flag.

### Fix 2 — `MODIFICATION_SUPPORT=NoModification` guard at `LockObject`

BTP / ABAP Cloud systems can return a **successful** LOCK response
whose `MODIFICATION_SUPPORT` field is `"NoModification"`. The
lock handle is valid, but any subsequent write fails with the same
confusing `423` seconds later — SAP is telling you upfront that
the object is read-only via ADT for this user/system.

The field was already parsed into `LockResult.ModificationSupport`
but never checked. `LockObject` now fails at LOCK time with a
clear, actionable error when `accessMode == "MODIFY"` and SAP
signals `NoModification`, naming the common causes: hyperfocused
mode, missing developer/edit role, BTP objects outside the
customer namespace, SAP-delivered objects.

Scoped to `MODIFY` locks — `READ` locks still succeed on
read-only objects, no downstream write to fail.

### Regression coverage (four new tests)

- `TestUpdateClassInclude_UsesStatefulSession`
- `TestCreateTestInclude_UsesStatefulSession`
- `TestLockObject_RejectsNoModification`
- `TestLockObject_AllowsNoModificationOnReadLock`

---

## 3. Drifted external PR — cherry-pick, don't wait on rebase

One open PR was a small 4-file review follow-up that had never
been rebased — 63 commits of unrelated drift (~24k additions),
`CONFLICTING` status after the security scrub. Rather than block
on a rebase request, the fix was to cherry-pick the intended
final commit directly onto current `main`. `git cherry-pick`
preserves the original `Author:` and only changes the
`Committer:`, so attribution stays intact. The PR was closed as
superseded with a friendly comment explaining the chain of
custody.

**Rule of thumb:** this is only acceptable when the PR diff is
clearly unreviewable due to drift (not merely large), and when
you have commit access on the target branch. Post the
explanatory comment first so the contributor sees the reasoning.

---

## 4. What to watch next session

- **Issues #88 / #91 / #92 / #98** are fix-landed but NOT closed
  — waiting for reporters to confirm on their systems via a
  `main` build. Check back in a few days; close on confirmation.
- **#90 + #99** (BTP OAuth2) are the next logical security /
  access-control thread after #91.
- **#55** (RunReport in APC) is an architectural limit; decide
  whether to close with a docs comment or keep as tracking
  issue.
- **#45 / #46** are low-effort, batch together when convenient.
- **Pre-commit / CI scanner** for the sanitize policy is still on
  the TODO list — right now the policy is a CLAUDE.md convention
  without an enforcement hook. The grep signature for the
  structural patterns is in CLAUDE.md; the names-based
  signature lives at `.local/scripts/check-identifiers.sh` (to
  keep the concrete list out of tracked files).
