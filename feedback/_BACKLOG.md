# VSP Improvement Backlog

Aggregated action items from session feedback logs. Updated by `/session-wrap`.

## Open Items

| Date | Session | Type | Description | Severity | Component | Status |
|------|---------|------|-------------|----------|-----------|--------|
| 2026-03-02 | 001 | Bug | Upstream build broken: `GetDependencyZIP` missing in `embed.go` (commit 8d2c343) | Medium | upstream | Open |
| 2026-03-02 | 001 | Gap | Sync script doesn't auto-resolve CLAUDE.md/README.md conflicts | Medium | scripts | Open — needs GH issue |
| 2026-03-02 | 001 | Gap | Sync script doesn't fix `oisee` URLs in markdown files (only .go imports) | Low | scripts | Open — needs GH issue |

## Resolved Items

| Date | Session | Type | Description | Resolution | Resolved |
|------|---------|------|-------------|------------|----------|
| | | | *No items resolved yet* | | |

---

## How This Works

1. During a session, use `/feedback` to capture observations in real time
2. At session end, use `/session-wrap` to extract additional learnings and update this backlog
3. Items route to: **GitHub Issues** (bugs, high-severity gaps), **CLAUDE.md** (lessons), or stay here (low-priority backlog)
4. When an item is addressed (issue closed, feature shipped, doc updated), move it to "Resolved Items"
