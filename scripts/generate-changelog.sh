#!/usr/bin/env bash
# scripts/generate-changelog.sh - Generate CHANGELOG.md entry (aligned with existing style)
# Format: [vX.Y.Z] + Clean descriptions + Optional callout

set -euo pipefail

NEW_TAG="${1:-}"
PREV_TAG="${2:-}"

if [[ -z "$NEW_TAG" ]]; then
  echo "Usage: $0 <new_tag> [prev_tag]" >&2
  exit 1
fi

# Auto-detect prev tag
if [[ -z "$PREV_TAG" ]]; then
  PREV_TAG=$(git tag --sort=-version:refname | grep -F "$NEW_TAG" -A 1 | tail -n 1 || echo "")
fi

VERSION="$NEW_TAG"  # Keep 'v' prefix: v0.4.5
DATE="$(date -u +'%Y-%m-%d')"

# Helper: clean commit message (remove conventional prefix + dedupe)
clean_msg() {
  sed -E 's/^(feat|fix|docs|chore|ci|refactor|test|build)(\([^)]+\))?:\s*//' | \
  sed 's/^/  - /' | \
  sort -u  # Deduplicate identical lines
}

# Generate header (aligned with your style)
echo "## [$VERSION] - $DATE"
echo ""

# Collect commits by category
ADDED=$(git log "$PREV_TAG".."$NEW_TAG" --pretty=format:"%s" --grep="^feat:" --no-merges | clean_msg || true)
CHANGED=$(git log "$PREV_TAG".."$NEW_TAG" --pretty=format:"%s" --grep="^refactor:" --no-merges | clean_msg || true)
FIXED=$(git log "$PREV_TAG".."$NEW_TAG" --pretty=format:"%s" --grep="^fix:" --no-merges | clean_msg || true)
INFRA=$(git log "$PREV_TAG".."$NEW_TAG" --pretty=format:"%s" --grep="^ci:\|^chore:\|^build:" --no-merges | clean_msg || true)
DOCS=$(git log "$PREV_TAG".."$NEW_TAG" --pretty=format:"%s" --grep="^docs:" --no-merges | clean_msg || true)

# Output sections (only if non-empty)
if [[ -n "$ADDED" ]]; then
  echo "### 🚀 Added"
  echo "$ADDED"
  echo ""
fi

if [[ -n "$CHANGED" ]]; then
  echo "### 🛠️ Changed"
  echo "$CHANGED"
  echo ""
fi

if [[ -n "$FIXED" ]]; then
  echo "### 🐛 Fixed"
  echo "$FIXED"
  echo ""
fi

if [[ -n "$INFRA" ]]; then
  echo "### 📦 Infrastructure"
  echo "$INFRA"
  echo ""
fi

if [[ -n "$DOCS" ]]; then
  echo "### 📖 Documentation"
  echo "$DOCS"
  echo ""
fi

# Fallback if no conventional commits found
if [[ -z "$ADDED$CHANGED$FIXED$INFRA$DOCS" ]]; then
  echo "### 📋 Changes"
  git log "$PREV_TAG".."$NEW_TAG" --pretty=format:"%s" --no-merges | head -10 | sed 's/^/  - /'
  echo ""
fi

# Optional: Add callout box (customizable per release)
# echo "> 💡 **Note**: This release focuses on DX improvements. No runtime behavior changes."

---
### 🔗 Quick Links
- 📄 [Full Changelog](https://github.com/$REPO/blob/main/CHANGELOG.md)
- 🐳 Docker: \`docker pull ghcr.io/$REPO:$VERSION\`
- 📦 Binaries: See Assets below

> 💡 **Tip**: Upgrade with \`docker compose pull && docker compose up -d\` for zero-downtime update.