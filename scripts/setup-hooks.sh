#!/bin/sh
# Enable the publish-hygiene pre-commit hook repository-locally.
# Run once after cloning / after the hook files appear:
#   sh scripts/setup-hooks.sh
git config core.hooksPath scripts/git-hooks
echo "core.hooksPath -> scripts/git-hooks (publish-hygiene pre-commit active)"
