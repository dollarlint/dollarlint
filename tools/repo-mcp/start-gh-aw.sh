#!/bin/sh
set -eu

: "${GITHUB_WORKSPACE:?GITHUB_WORKSPACE must be set}"

if [ -z "${PATH:-}" ]; then
  PATH="/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin"
fi

case ":$PATH:" in
  *:/usr/local/go/bin:*) ;;
  *) PATH="/usr/local/go/bin:$PATH" ;;
esac
export PATH

export DOLLARLINT_REPO_ROOT="${DOLLARLINT_REPO_ROOT:-$GITHUB_WORKSPACE}"

git config --global --add safe.directory "$GITHUB_WORKSPACE" || true
cd "$GITHUB_WORKSPACE"
exec go run ./tools/repo-mcp
