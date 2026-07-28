#!/usr/bin/env bash
# Run once per clone: points git at the repo-tracked hooks in .githooks/.
set -euo pipefail
cd "$(git rev-parse --show-toplevel)"
git config core.hooksPath .githooks
echo "core.hooksPath set to .githooks — pre-commit secret scan is active."
