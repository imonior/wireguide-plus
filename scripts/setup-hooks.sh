#!/bin/sh
# Enable the publish-hygiene git hooks repository-locally.
# Run once after cloning / after the hook files appear:
#   sh scripts/setup-hooks.sh
#
# Installs TWO hooks (both live in scripts/git-hooks/):
#   pre-commit  -- scans the staged diff
#   commit-msg  -- scans the proposed commit message (git passes the message
#                  file as $1; a pre-commit hook gets no arguments at all, so
#                  the message half cannot be done from there)
git config core.hooksPath scripts/git-hooks
echo "core.hooksPath -> scripts/git-hooks (publish-hygiene pre-commit + commit-msg hooks active)"
