# Upstream Sync Automation

## Overview

This directory contains tools to automate syncing your fork with the upstream `oisee/vibing-steampunk` repository.

## Quick Start

### Manual Sync (Recommended)

```bash
# Sync and review changes
./scripts/sync-upstream.sh

# Auto-merge and push (use with caution)
./scripts/sync-upstream.sh --auto-merge --push
```

### Automated Sync (GitHub Actions)

The workflow at `.github/workflows/sync-upstream.yml` runs daily at 2 AM UTC and:

1. ✅ Fetches upstream changes
2. ✅ Attempts auto-merge with conflict resolution
3. ✅ Fixes import paths (`oisee` → `vinchacho`)
4. ✅ Runs `go mod tidy`
5. ✅ Builds and tests
6. ✅ Creates a PR for review

**Manual trigger:**
```bash
gh workflow run sync-upstream.yml
```

Or via GitHub UI: Actions → Sync with Upstream → Run workflow

## What Gets Automated

| Task | Manual Script | GitHub Action |
|------|---------------|---------------|
| Fetch upstream | ✅ | ✅ |
| Auto-merge (no conflicts) | ✅ | ✅ |
| Fix import paths | ✅ | ✅ |
| Update dependencies | ✅ | ✅ |
| Build verification | ✅ | ✅ |
| Run tests | ✅ | ✅ |
| Create PR | ❌ | ✅ |
| Update CLAUDE.md | ⚠️ Template | ⚠️ Reminder |
| Push directly | Optional | ❌ (PR only) |

## Configuration

Edit `.github/sync-config.json` to customize:

```json
{
  "upstream": {
    "repo": "oisee/vibing-steampunk",
    "branch": "main"
  },
  "fork": {
    "org": "vinchacho",
    "repo": "vibing-steampunk"
  },
  "automation": {
    "schedule": "0 2 * * *",
    "auto_merge_trivial": true,
    "create_pr_on_conflict": true
  }
}
```

## Conflict Resolution

The automation handles the most common conflict (import paths in `cmd/vsp/main.go`):

1. Detects conflicts in `main.go`
2. Replaces all `oisee/vibing-steampunk` → `vinchacho/vibing-steampunk`
3. Ensures new imports from upstream are included
4. Commits the resolution

For complex conflicts, the GitHub Action will:
- Stop and create a draft PR
- Comment with conflict details
- Wait for manual resolution

## Manual Steps

Some tasks still need human review:

1. **CLAUDE.md Updates** - The script generates a template, but you should:
   - Review new features from upstream
   - Update "Last Session Reference" section
   - Add new reports to documentation list
   - Update project status table

2. **Breaking Changes** - Review upstream PRs for:
   - API changes
   - Configuration changes
   - Deprecated features

3. **Version Bumps** - Update version references in docs when upstream releases

## Testing Locally

Before pushing:

```bash
# Dry run (shows what would happen)
git fetch upstream
git log --oneline main..upstream/main

# Run the sync script
./scripts/sync-upstream.sh

# Review changes
git log --oneline -5
git diff HEAD~3

# If satisfied, push
git push origin main
```

## Troubleshooting

### "Merge conflict could not be auto-resolved"

```bash
# Check which files have conflicts
git status

# Manual resolution
# 1. Edit conflicted files
# 2. git add <files>
# 3. git merge --continue
```

### "Some tests failed"

The automation continues even if tests fail, but creates a warning:

```bash
# Review test failures
go test ./... -v

# Fix issues and commit
git add .
git commit -m "fix: resolve test failures from upstream merge"
```

### Import Path Issues

If you see `oisee` references after merge:

```bash
# Find remaining references
grep -r "github.com/oisee" --include="*.go" .

# Fix automatically
find . -name "*.go" -exec sed -i 's|github.com/oisee/vibing-steampunk|github.com/vinchacho/vibing-steampunk|g' {} +
```

## Future Improvements

- [ ] Auto-update CLAUDE.md with AI (use Claude API)
- [ ] Auto-extract new features from upstream commits
- [ ] Auto-update version numbers in docs
- [ ] Slack/Discord notifications for sync PRs
- [ ] Automated testing against SAP system
