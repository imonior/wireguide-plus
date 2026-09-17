#!/bin/sh
# check-no-ai.sh — publish-hygiene guard.
#
# Blocks commits/PRs that leak assistant-tool (AI-IDE) names or the
# standalone token "AI" into (a) commit messages and (b) published files.
#
# Cross-platform & tool-agnostic: runs under any POSIX sh (Git Bash on
# Windows, /bin/sh on macOS/Linux). No external deps.
#
# Usage:
#   scripts/check-no-ai.sh [COMMIT_MSG_FILE]   # pre-commit mode
#   scripts/check-no-ai.sh --ci [RANGE]         # CI mode (RANGE default origin/main..HEAD)
#
# Design notes / false-positive guards:
#   * "cursor" is excluded from the CONTENT scan because it is a CSS property.
#   * "tencent" is excluded from the CONTENT scan — the legacy
#     "Tencent DNSPod" public-resolver description in CHANGELOG is allowed.
#   * .gitignore / .git/info/exclude are excluded: ignore rules for
#     assistant-tool dirs are permitted (they are NOT published artifacts).
#   * This script and its hook/workflow files are excluded (self-reference).

set -eu

AI_TOKEN='\bAI\b'
TOOLS_MSG='(workbuddy|codebuddy|tencent|trae|cursor|claude|copilot|windsurf|codeium|aider)'
TOOLS_CONTENT='(workbuddy|codebuddy|trae|claude|copilot|windsurf|codeium|aider)'
# Files allowed to mention assistant-tool names / the AI token: self-referential
# guard files and ignore files (which may list assistant-tool directories to keep
# them out of the repo — that's a hygiene rule, not a leak).
# NOTE: a `case "$f" in $SKIP)` pattern built from a variable does not honour the
# `|` alternation in every sh, and a bare `scripts/git-hooks/` (no wildcard) cannot
# match its own subfiles — so we use an explicit function with literal patterns.
skip_file() {
  case "$1" in
    scripts/check-no-ai.sh) return 0 ;;
    scripts/git-hooks/*) return 0 ;;
    .github/workflows/no-ai-scan.yml) return 0 ;;
    CONTRIBUTING.md) return 0 ;;
    .gitignore) return 0 ;;
    .git/info/exclude) return 0 ;;
  esac
  return 1
}

err=0
report() { echo "publish-hygiene: $1" >&2; err=1; }

scan_msg_file() {
  f="$1"
  if grep -Eiq "$TOOLS_MSG" "$f"; then
    report "commit message references an assistant tool: $(grep -Eio "$TOOLS_MSG" "$f" | head -1)"
  fi
  if grep -Eiq "$AI_TOKEN" "$f"; then
    report "commit message contains the standalone token 'AI' (forbidden)"
  fi
}

scan_staged() {
  for f in $(git diff --cached --name-only --diff-filter=ACM); do
    if skip_file "$f"; then continue; fi
    diff=$(git diff --cached -- "$f" || true)
    if echo "$diff" | grep -Eiq "$TOOLS_CONTENT"; then
      report "staged change to '$f' leaks an assistant-tool name"
    fi
    case "$f" in
      *.md|*.markdown)
        if echo "$diff" | grep -Eiq "$AI_TOKEN"; then
          report "staged doc '$f' contains the standalone token 'AI'"
        fi ;;
    esac
  done
}

scan_range() {
  range="$1"
  echo "publish-hygiene: scanning commit messages in $range" >&2
  if git log --format=%B "$range" 2>/dev/null | grep -Eiq "$TOOLS_MSG"; then
    report "a commit message in range references an assistant tool"
  fi
  if git log --format=%B "$range" 2>/dev/null | grep -Eiq "$AI_TOKEN"; then
    report "a commit message in range contains the standalone token 'AI'"
  fi
  for f in $(git diff --name-only "$range" --diff-filter=ACM 2>/dev/null); do
    if skip_file "$f"; then continue; fi
    diff=$(git diff "$range" -- "$f" 2>/dev/null || true)
    if echo "$diff" | grep -Eiq "$TOOLS_CONTENT"; then
      report "change to '$f' in range leaks an assistant-tool name"
    fi
    case "$f" in
      *.md|*.markdown)
        if echo "$diff" | grep -Eiq "$AI_TOKEN"; then
          report "doc '$f' in range contains the standalone token 'AI'"
        fi ;;
    esac
  done
}

case "${1:-}" in
  --ci)
    shift
    range="${1:-}"
    if [ -z "$range" ]; then
      if git rev-parse origin/main >/dev/null 2>&1 && [ "$(git rev-parse origin/main)" != "$(git rev-parse HEAD)" ]; then
        range="origin/main..HEAD"
      else
        range="HEAD~20..HEAD"
      fi
    fi
    scan_range "$range"
    ;;
  "")
    echo "publish-hygiene: no commit message file passed; scanning staged diff only" >&2
    scan_staged
    ;;
  *)
    scan_msg_file "$1"
    scan_staged
    ;;
esac

if [ "$err" -ne 0 ]; then
  echo "publish-hygiene: BLOCKED. Remove assistant-tool references / the 'AI' token from the commit message and changes." >&2
  exit 1
fi
echo "publish-hygiene: OK"
