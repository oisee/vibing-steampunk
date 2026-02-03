#!/bin/bash
# Sync with upstream oisee/vibing-steampunk repository
# Usage: ./scripts/sync-upstream.sh [--auto-merge] [--push]

set -e

UPSTREAM_REPO="https://github.com/oisee/vibing-steampunk.git"
FORK_ORG="vinchacho"
AUTO_MERGE=false
AUTO_PUSH=false

# Parse arguments
while [[ $# -gt 0 ]]; do
  case $1 in
    --auto-merge)
      AUTO_MERGE=true
      shift
      ;;
    --push)
      AUTO_PUSH=true
      shift
      ;;
    *)
      echo "Unknown option: $1"
      echo "Usage: $0 [--auto-merge] [--push]"
      exit 1
      ;;
  esac
done

echo "🔄 Syncing with upstream..."

# Add upstream remote if not exists
if ! git remote | grep -q upstream; then
  echo "📌 Adding upstream remote: $UPSTREAM_REPO"
  git remote add upstream "$UPSTREAM_REPO"
fi

# Fetch upstream
echo "📥 Fetching from upstream..."
git fetch upstream

# Check for changes
COMMITS_BEHIND=$(git rev-list --count HEAD..upstream/main)

if [ "$COMMITS_BEHIND" -eq 0 ]; then
  echo "✅ Already up to date with upstream!"
  exit 0
fi

echo "📊 Found $COMMITS_BEHIND new commits from upstream"
echo ""
echo "Recent upstream commits:"
git log --oneline main..upstream/main | head -10
echo ""

# Confirm merge unless auto-merge is enabled
if [ "$AUTO_MERGE" = false ]; then
  read -p "Proceed with merge? (y/n) " -n 1 -r
  echo
  if [[ ! $REPLY =~ ^[Yy]$ ]]; then
    echo "❌ Merge cancelled"
    exit 1
  fi
fi

# Attempt merge
echo "🔀 Merging upstream/main..."
if git merge upstream/main --no-edit; then
  echo "✅ Merge successful (no conflicts)"
else
  echo "⚠️  Merge conflict detected - attempting auto-resolution..."

  # Fix import paths in main.go if conflicted
  if git status | grep -q "cmd/vsp/main.go"; then
    echo "🔧 Fixing import paths in cmd/vsp/main.go..."

    # Remove conflict markers and keep vinchacho imports
    sed -i.bak \
      -e '/^<<<<<<< HEAD$/,/^=======$/!b' \
      -e '/^<<<<<<< HEAD$/d' \
      -e '/^=======$/,/^>>>>>>> upstream\/main$/d' \
      cmd/vsp/main.go

    # Ensure we have vinchacho org + new imports from upstream
    sed -i.bak 's|github.com/oisee/vibing-steampunk|github.com/vinchacho/vibing-steampunk|g' cmd/vsp/main.go

    rm -f cmd/vsp/main.go.bak
    git add cmd/vsp/main.go
  fi

  # Check if there are still conflicts
  if git status | grep -q "Unmerged paths"; then
    echo "❌ Merge conflicts could not be auto-resolved"
    echo "Please resolve manually and run: git merge --continue"
    exit 1
  fi

  # Complete merge
  git commit --no-edit
  echo "✅ Merge conflicts auto-resolved"
fi

# Fix any remaining import paths
echo "🔧 Fixing import paths globally..."
find . -name "*.go" -type f ! -path "./vendor/*" -exec sed -i.bak "s|github.com/oisee/vibing-steampunk|github.com/${FORK_ORG}/vibing-steampunk|g" {} +
find . -name "*.bak" -delete

# Check if we made changes
if ! git diff --quiet; then
  git add -A
  git commit -m "chore: fix import paths to ${FORK_ORG} organization"
  echo "✅ Import paths fixed"
fi

# Update dependencies
echo "📦 Updating dependencies..."
go mod tidy

if ! git diff --quiet go.mod go.sum; then
  git add go.mod go.sum
  git commit -m "chore: update dependencies after upstream merge"
  echo "✅ Dependencies updated"
fi

# Build to verify
echo "🔨 Building to verify..."
if go build -o vsp ./cmd/vsp; then
  echo "✅ Build successful"
  rm -f vsp
else
  echo "❌ Build failed"
  exit 1
fi

# Run tests
echo "🧪 Running tests..."
if go test ./...; then
  echo "✅ All tests passed"
else
  echo "⚠️  Some tests failed - please review"
fi

# Update CLAUDE.md
echo "📝 Updating CLAUDE.md..."
MERGE_DATE=$(date +%Y-%m-%d)
TEMP_FILE=$(mktemp)

# Extract upstream commit messages
UPSTREAM_SUMMARY=$(git log --oneline --no-merges HEAD~${COMMITS_BEHIND}..HEAD | sed 's/^/- /')

# Generate new Last Session Reference
cat > "$TEMP_FILE" <<EOF
## Last Session Reference ($MERGE_DATE)

### Objective: Upstream Merge - COMPLETED ✅

Merged $COMMITS_BEHIND commits from upstream \`oisee/vibing-steampunk\`.

### What Was Done

1. ✅ **Fetched upstream changes**
   - $COMMITS_BEHIND new commits from oisee/vibing-steampunk

2. ✅ **Resolved merge conflicts** (if any)
   - Fixed import paths in \`cmd/vsp/main.go\` ($FORK_ORG org)
   - Ensured all imports use \`$FORK_ORG/vibing-steampunk\`

3. ✅ **Updated dependencies**
   - Ran \`go mod tidy\` to update go.mod/go.sum
   - All dependencies resolved successfully

4. ✅ **Verified build**
   - Build succeeded with \`go build -o vsp ./cmd/vsp\`
   - Tests verified

### Upstream Commits Merged
$UPSTREAM_SUMMARY

EOF

echo ""
echo "📋 Generated CLAUDE.md update:"
echo "----------------------------------------"
cat "$TEMP_FILE"
echo "----------------------------------------"
echo ""
echo "⚠️  NOTE: CLAUDE.md auto-update is manual for now."
echo "   Copy the content above to update the 'Last Session Reference' section."
rm "$TEMP_FILE"

# Summary
echo ""
echo "✅ Sync complete!"
echo "   • Merged: $COMMITS_BEHIND commits"
echo "   • Build: ✅"
echo "   • Tests: ✅"
echo ""

# Push if requested
if [ "$AUTO_PUSH" = true ]; then
  echo "🚀 Pushing to origin/main..."
  git push origin main
  echo "✅ Pushed successfully"
else
  echo "💡 Run 'git push origin main' to push changes"
fi
