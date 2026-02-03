# Upstream Sync Quick Reference

## TL;DR

Your fork now automatically syncs with upstream `oisee/vibing-steampunk` daily.

```bash
# Manual sync (review changes first - RECOMMENDED)
./scripts/sync-upstream.sh

# Auto-merge and push (if you're confident)
./scripts/sync-upstream.sh --auto-merge --push

# Trigger GitHub Action manually
gh workflow run sync-upstream.yml
```

## How It Works

### GitHub Action (Automatic)
- **Schedule**: Daily at 2 AM UTC
- **What it does**:
  1. Checks for upstream commits
  2. Merges if no conflicts
  3. Fixes import paths (`oisee` → `vinchacho`)
  4. Updates dependencies
  5. Runs build + tests
  6. Creates PR for your review

### Manual Script (On-Demand)
- **When to use**: When you want immediate sync
- **Benefits**: Local control, see what happens in real-time
- **Output**: Generates CLAUDE.md update template

## What Gets Automated ✅

| Task | Status |
|------|--------|
| Detect upstream changes | ✅ Automated |
| Merge (no conflicts) | ✅ Automated |
| Fix import paths | ✅ Automated |
| Update dependencies | ✅ Automated |
| Build verification | ✅ Automated |
| Run tests | ✅ Automated |
| Create PR for review | ✅ Automated |

## What Needs Manual Review ⚠️

| Task | Why Manual |
|------|------------|
| CLAUDE.md updates | Need to review features, write session notes |
| Breaking changes | Need to understand impact |
| Complex conflicts | Need human judgment |
| Version number updates | Need to coordinate with docs |

## Typical Workflow

### Option 1: Let GitHub Action Handle It (Recommended)
1. Wait for daily sync (or trigger manually)
2. Review the PR created by GitHub Actions
3. Check changes, test locally if needed
4. Merge the PR

### Option 2: Manual Sync
1. Run `./scripts/sync-upstream.sh`
2. Review changes with `git log --oneline -10`
3. Update CLAUDE.md using provided template
4. Push: `git push origin main`

## Example: Manual Sync Session

```bash
$ ./scripts/sync-upstream.sh
🔄 Syncing with upstream...
📥 Fetching from upstream...
📊 Found 5 new commits from upstream

Recent upstream commits:
abc1234 feat: add new feature X
def5678 fix: resolve bug Y
...

Proceed with merge? (y/n) y
🔀 Merging upstream/main...
✅ Merge successful (no conflicts)
🔧 Fixing import paths globally...
✅ Import paths fixed
📦 Updating dependencies...
✅ Dependencies updated
🔨 Building to verify...
✅ Build successful
🧪 Running tests...
✅ All tests passed

📋 Generated CLAUDE.md update:
----------------------------------------
## Last Session Reference (2026-02-03)
...
----------------------------------------

✅ Sync complete!
   • Merged: 5 commits
   • Build: ✅
   • Tests: ✅

💡 Run 'git push origin main' to push changes
```

## Troubleshooting

### "Merge conflict could not be auto-resolved"
→ See [scripts/README.md](scripts/README.md#troubleshooting) for resolution steps

### "Some tests failed"
→ Check test output, fix issues, commit fixes

### "Import paths still reference oisee"
```bash
find . -name "*.go" -exec sed -i 's|github.com/oisee/vibing-steampunk|github.com/vinchacho/vibing-steampunk|g' {} +
```

## Configuration

Edit `.github/sync-config.json` to customize behavior:
- Change sync schedule
- Adjust auto-merge behavior
- Add custom replacement patterns

## Documentation

- **Full Guide**: [scripts/README.md](scripts/README.md)
- **GitHub Action**: [.github/workflows/sync-upstream.yml](.github/workflows/sync-upstream.yml)
- **Sync Script**: [scripts/sync-upstream.sh](scripts/sync-upstream.sh)
- **Config**: [.github/sync-config.json](.github/sync-config.json)

---

**Next time you need to sync**: Just run `./scripts/sync-upstream.sh` or wait for the daily GitHub Action! 🚀
