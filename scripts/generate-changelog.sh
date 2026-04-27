#!/usr/bin/env bash
# scripts/generate-changelog.sh - Generate CHANGELOG.md entry from Git tags
# Format: Keep a Changelog (https://keepachangelog.com) + Semantic Versioning

set -euo pipefail

# Configuration
REPO="${GITHUB_REPOSITORY:-skylunna/luner}"
NEW_TAG="${1:-}"
PREV_TAG="${2:-}"

if [[ -z "$NEW_TAG" ]]; then
  echo "Usage: $0 <new_tag> [prev_tag]"
  echo "Example: $0 v0.4.4 v0.4.3"
  exit 1
fi

# Auto-detect previous tag if not provided
if [[ -z "$PREV_TAG" ]]; then
  PREV_TAG=$(git tag --sort=-version:refname | grep -F "$NEW_TAG" -A 1 | tail -n 1 || echo "")
  if [[ -z "$PREV_TAG" ]]; then
    # First release: get all commits
    PREV_TAG=""
  fi
fi

# Extract version and date
VERSION="${NEW_TAG#v}"  # Remove 'v' prefix
DATE="$(date -u +'%Y-%m-%d')"

# Generate header
echo ""
echo "## [$VERSION] - $DATE"
echo ""

# Parse commits by conventional commit type
echo "### 🚀 Added"
git log "$PREV_TAG".."$NEW_TAG" --pretty=format:"%s" --grep="^feat:" --invert-grep --grep="^fix:" --invert-grep --grep="^docs:" --invert-grep --grep="^chore:" --invert-grep --grep="^ci:" --invert-grep --grep="^test:" --invert-grep --grep="^refactor:" --invert-grep | grep -v "^Merge" | sed 's/^/- /' || echo "- *(No new features)*"
echo ""

echo "### 🛠️ Changed"
git log "$PREV_TAG".."$NEW_TAG" --pretty=format:"%s" --grep="^refactor:" | grep -v "^Merge" | sed 's/^/- /' || echo "- *(No changes)*"
echo ""

echo "### 🐛 Fixed"
git log "$PREV_TAG".."$NEW_TAG" --pretty=format:"%s" --grep="^fix:" | grep -v "^Merge" | sed 's/^/- /' || echo "- *(No bug fixes)*"
echo ""

echo "### 📦 Infrastructure"
git log "$PREV_TAG".."$NEW_TAG" --pretty=format:"%s" --grep="^ci:" --grep="^chore:" --grep="^build:" | grep -v "^Merge" | sed 's/^/- /' || echo "- *(No infra changes)*"
echo ""

echo "### 📖 Documentation"
git log "$PREV_TAG".."$NEW_TAG" --pretty=format:"%s" --grep="^docs:" | grep -v "^Merge" | sed 's/^/- /' || echo "- *(No docs updates)*"
echo ""

# Fallback: if no conventional commits found, list all
if ! git log "$PREV_TAG".."$NEW_TAG" --pretty=format:"%s" | grep -qE "^(feat|fix|docs|chore|ci|refactor):"; then
  echo "### 📋 All Changes"
  git log "$PREV_TAG".."$NEW_TAG" --pretty=format:"* %s (%h)" --no-merges | head -20
  echo ""
fi