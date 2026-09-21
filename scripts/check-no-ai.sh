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
#   scripts/check-no-ai.sh                  # staged-diff scan  (pre-commit hook)
#   scripts/check-no-ai.sh COMMIT_MSG_FILE  # message + staged  (commit-msg hook)
#   scripts/check-no-ai.sh --ci [RANGE]     # CI mode; RANGE defaults to
#                                           # origin/main..HEAD, else HEAD~1..HEAD
#
# The commit message is only available to the commit-msg hook, which git calls
# with the message file as $1. A pre-commit hook receives no arguments at all,
# so it must call this script with none — see scripts/git-hooks/{pre-commit,
# commit-msg}. Both hooks are required: without commit-msg the message half of
# this rule never ran (a message like "test WorkBuddy leak" was accepted).
#
# Design notes / false-positive guards — each one is load-bearing here:
#   * "cursor" is excluded from the CONTENT scan because it is a CSS
#     property: frontend/src/App.svelte and frontend/public/style.css all
#     contain `cursor: pointer` / `cursor: col-resize`. It stays in the
#     COMMIT-MESSAGE scan, where a CSS rule can never appear.
#   * "tencent" is excluded from the CONTENT scan — CHANGELOG.zh.md:511 (and
#     the zh-TW / ja / ko siblings) legitimately list 腾讯 DNSPod among the
#     public resolvers probed by the DNS-leak test. It stays in the
#     COMMIT-MESSAGE scan.
#   * .gitignore / .git/info/exclude are excluded: ignore rules for
#     assistant-tool dirs are permitted (they are NOT published artifacts).
#     See .gitignore line 44.
#   * CONTRIBUTING.md is excluded: it states this very rule, so it names the
#     guard tooling. Without this exemption the guard blocks the commit that
#     documents the guard.
#   * This script and its hook/workflow files are excluded (self-reference).
#   * The standalone-token scan neutralises the guard's own filenames before
#     matching (see SELF_REF). This repo is worse than most here: BOTH
#     "check-no-ai.sh" and ".github/workflows/no-ai-scan.yml" contain "-ai"
#     between non-word characters, so "no-ai" satisfies `\bAI\b` on its own.
#     Naming either file in a commit message — e.g. the commit that adds the
#     commit-msg hook — would otherwise be rejected. Assistant-tool names are
#     a separate rule and stay fully enforced.
#   * Every scan uses `grep -c` + `>/dev/null`, never `grep -q`. `-q` exits at
#     the first match, so when the producer is a large multi-KB diff the writer
#     blocks forever once the pipe buffer fills -- MSYS2/Git-Bash does not
#     deliver SIGPIPE reliably, and the guard hangs mid-commit. `-c` reads the
#     whole input, so it cannot deadlock; its exit status is still 0 on a match.
#     Same reason `sed -n '1p'` replaced `head -1`.
#
# Verify after any edit to this list:
#   bash scripts/check-no-ai.sh --ci HEAD~5..HEAD   # a range wide enough to matter

set -eu

AI_TOKEN='\bAI\b'
TOOLS_MSG='(workbuddy|codebuddy|tencent|trae|cursor|claude|copilot|windsurf|codeium|aider)'
TOOLS_CONTENT='(workbuddy|codebuddy|trae|claude|copilot|windsurf|codeium|aider)'
# Neutralises the "no-ai" literal that both guard filenames contain, so naming
# them in a message/doc is not a leak. See the design note above. Spelled one
# character class per letter so it stays case-insensitive under any POSIX sed
# (the GNU-only `I` flag is not portable).
SELF_REF='s/[Nn][Oo]-[Aa][Ii]/publish-hygiene/g'
# Files allowed to mention assistant-tool names / the AI token: self-referential
# guard files, ignore files, and the doc that states the rule.
# NOTE: a `case "$f" in $SKIP)` pattern built from a variable does not honour the
# `|` alternation in every sh, and a bare `scripts/git-hooks/` (no wildcard) cannot
# match its own subfiles — so we use an explicit function with literal patterns.
skip_file() {
  case "$1" in
    scripts/check-no-ai.sh) return 0 ;;
    scripts/git-hooks/*) return 0 ;;
    scripts/setup-hooks.sh) return 0 ;;
    .github/workflows/no-ai-scan.yml) return 0 ;;
    CONTRIBUTING.md) return 0 ;;
    .gitignore) return 0 ;;
    .git/info/exclude) return 0 ;;
  esac
  return 1
}

# Commits whose *message* is exempt from the scan, for the same reason
# CONTRIBUTING.md is exempt from the file scan: the commit that introduced this
# guard may write the rule out in prose and trip its own check, and being
# immutable history it cannot hide anything new -- but a `--ci` range reaching
# back past it would be red forever.
#
# No entries here, and unlike the sibling project that is not an oversight:
# `git log --all` over this repo's history finds zero commits whose message
# mentions a guarded tool name or a standalone "AI" token, so nothing needs
# exempting. Keep it that way -- if a future commit does need an exemption, add
# its abbreviated SHA here rather than widening the CI range.
skip_commit() {
  case "$1" in
    __never__) return 0 ;;
  esac
  return 1
}

err=0
report() { echo "publish-hygiene: $1" >&2; err=1; }

# Reads text on stdin and prints it with any self-reference to the guard's own
# filenames neutralised, so naming the guard in a message/doc is not a leak.
# See SELF_REF.
strip_self_ref() { sed -e "$SELF_REF"; }

scan_msg_file() {
  f="$1"
  if grep -Eic "$TOOLS_MSG" "$f" >/dev/null; then
    report "commit message references an assistant tool: $(grep -Eio "$TOOLS_MSG" "$f" | sed -n '1p')"
  fi
  if strip_self_ref < "$f" | grep -Eic "$AI_TOKEN" >/dev/null; then
    report "commit message contains the standalone token 'AI' (forbidden)"
  fi
}

scan_staged() {
  for f in $(git diff --cached --name-only --diff-filter=ACM); do
    if skip_file "$f"; then continue; fi
    diff=$(git diff --cached -- "$f" || true)
    if echo "$diff" | grep -Eic "$TOOLS_CONTENT" >/dev/null; then
      report "staged change to '$f' leaks an assistant-tool name"
    fi
    case "$f" in
      *.md|*.markdown)
        if echo "$diff" | strip_self_ref | grep -Eic "$AI_TOKEN" >/dev/null; then
          report "staged doc '$f' contains the standalone token 'AI'"
        fi ;;
    esac
  done
}

scan_range() {
  range="$1"
  echo "publish-hygiene: scanning $range" >&2
  for c in $(git rev-list "$range" 2>/dev/null || true); do
    if skip_commit "$c"; then continue; fi
    short=$(git rev-parse --short "$c")
    body=$(git log -1 --format=%B "$c")
    hit=$(echo "$body" | grep -Eio "$TOOLS_MSG" | sed -n '1p' || true)
    if [ -n "$hit" ]; then
      report "commit $short references an assistant tool: $hit"
    fi
    if echo "$body" | strip_self_ref | grep -Eic "$AI_TOKEN" >/dev/null; then
      report "commit $short message contains the standalone token 'AI'"
    fi
  done
  for f in $(git diff --name-only "$range" --diff-filter=ACM 2>/dev/null || true); do
    if skip_file "$f"; then continue; fi
    diff=$(git diff "$range" -- "$f" 2>/dev/null || true)
    if echo "$diff" | grep -Eic "$TOOLS_CONTENT" >/dev/null; then
      report "change to '$f' in range leaks an assistant-tool name"
    fi
    case "$f" in
      *.md|*.markdown)
        if echo "$diff" | strip_self_ref | grep -Eic "$AI_TOKEN" >/dev/null; then
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
      elif git rev-parse --verify -q 'HEAD~1^{commit}' >/dev/null 2>&1; then
        # No range given and origin/main is level with HEAD: check the tip only.
        # Deliberately NOT a fixed lookback window -- that re-scans history which
        # predates the guard and can go red on the guard's own commit message.
        range="HEAD~1..HEAD"
      else
        range="HEAD"   # root commit: `git log` walks nothing but this commit
      fi
    fi
    scan_range "$range"
    ;;
  "")
    echo "publish-hygiene: staged-diff scan (commit message is checked by the commit-msg hook)" >&2
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
