# Session Feedback Log

**Date:** 2026-03-02
**Session ID:** 001
**Objective:** Sync fork with upstream oisee/vibing-steampunk (8 commits behind), resolve conflicts, fix references
**Duration:** ~30 minutes

---

## Entries

### Entry 1 [auto]
- **Type:** Bug
- **Severity:** Medium
- **Component:** upstream (oisee/vibing-steampunk)
- **Description:** Upstream commit `8d2c343` added `internal/mcp/handlers_deploy.go` which calls `deps.GetDependencyZIP()`, but this function was never implemented in `embedded/deps/embed.go`. The upstream repo itself fails to compile.
- **Workaround:** Added a stub `GetDependencyZIP()` that returns nil (no ZIPs are embedded yet). This matches the existing `Available: false` pattern in `GetAvailableDependencies()`.
- **Action:** Pending — consider reporting upstream

### Entry 2 [auto]
- **Type:** Issue
- **Severity:** Medium
- **Component:** scripts/sync-upstream.sh
- **Description:** The sync script only auto-resolves conflicts in `cmd/vsp/main.go`. This session had conflicts in `CLAUDE.md` (3 conflict regions) and `README.md` (1 conflict region) that required manual resolution. The script exits with an error for any non-main.go conflicts.
- **Workaround:** Manually resolved all 4 conflict regions: kept fork's richer content for CLAUDE.md (disabled groups, codebase structure, test counts), merged both sides for README.md (vinchacho URL + upstream's reviewer guide link).
- **Action:** Pending — enhance sync script to handle CLAUDE.md/README.md conflicts

### Entry 3 [auto]
- **Type:** Lesson
- **Severity:** Low
- **Component:** upstream sync workflow
- **Description:** When resolving fork vs upstream conflicts, the fork (HEAD) typically has more complete/accurate content since it's been actively maintained with additional features and higher test counts. Strategy: keep HEAD for data that the fork extends (test counts, feature lists, detailed structure), but incorporate new content from upstream (new links, new docs references).
- **Action:** Pending — consider adding to CLAUDE.md

### Entry 4 [auto]
- **Type:** Gap
- **Severity:** Low
- **Component:** docs
- **Description:** New files from upstream (`docs/cli-agents/`, `docs/reviewer-guide.md`, `docs/architecture.md`, `articles/`) all arrive with `oisee` references that need manual find-and-replace to `vinchacho`. The sync script only fixes `.go` import paths, not markdown URLs.
- **Workaround:** Manual `replace_all` edits across 5 doc files (reviewer-guide + 4 cli-agents language variants). Articles left as-is since they reference the upstream author's other repos.
- **Action:** Pending — enhance sync script to also fix markdown URLs

### Entry 5 [auto]
- **Type:** Lesson
- **Severity:** Low
- **Component:** docs
- **Description:** Upstream articles (`articles/`) reference the original author's other GitHub repos (`oisee/zork-abap`, `oisee/vivid-vibes`) which only exist under `oisee`. These should NOT be changed to `vinchacho`. Only operational docs (guides, READMEs) should be updated.
- **Action:** Pending — document this distinction

---

## Action Items

| # | Type | Description | Destination | Status |
|---|------|-------------|-------------|--------|
| 1 | Bug | Upstream build broken: `GetDependencyZIP` missing in `embed.go` | GitHub Issue (upstream) | [ ] Pending |
| 2 | Gap | Sync script doesn't handle CLAUDE.md/README.md conflicts | Backlog | [ ] Pending |
| 3 | Lesson | Fork keeps richer content in conflicts; incorporate upstream additions | CLAUDE.md candidate | [ ] Pending |
| 4 | Gap | Sync script doesn't fix `oisee` references in markdown files | Backlog | [ ] Pending |
| 5 | Lesson | Don't change `oisee` refs in articles (author's other repos) | Backlog | [ ] Pending |

## Session Metrics

- **Tools used:** 8 unique (Bash, Read, Edit, Write, Grep, Glob, TaskCreate, TaskUpdate)
- **Estimated tool calls:** ~45
- **Errors encountered:** 2 (sync script conflict failure, `--no-edit` flag with `git merge --continue`)
- **Workarounds found:** 3 (manual conflict resolution, GetDependencyZIP stub, manual oisee→vinchacho in docs)
- **Key tools:** Bash (git, go build, go test), Edit (conflict resolution, reference fixes), Read (file inspection)

## Patterns & Observations

The upstream sync workflow works well for Go code (import path fixing is automated) but leaves a gap for documentation — markdown URLs still reference `oisee` after merge. The sync script's conflict resolution is too narrow (only `cmd/vsp/main.go`), which means most merges with doc changes will require manual intervention. The stash→merge→pop pattern preserved all 22 local files cleanly. The upstream repo itself had a build-breaking commit, which our fork now fixes.
