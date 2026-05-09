#!/usr/bin/env bash
set -euo pipefail

repo="${1:-dollarlint/dollarlint}"
secret_name="COPILOT_GITHUB_TOKEN"

if ! command -v gh >/dev/null 2>&1; then
  echo "error: gh is not installed or not on PATH" >&2
  exit 1
fi

if ! gh aw --help >/dev/null 2>&1; then
  echo "error: gh-aw is not installed. Install it with: gh extension install github/gh-aw" >&2
  exit 1
fi

if [[ ! -r /dev/tty ]]; then
  echo "error: this script needs a terminal so it can read the token without echoing it" >&2
  exit 1
fi

printf "Paste fine-grained GitHub PAT for %s on %s: " "$secret_name" "$repo" > /dev/tty
IFS= read -r -s token < /dev/tty
printf "\n" > /dev/tty

token="${token//$'\r'/}"

if [[ -z "$token" ]]; then
  echo "error: token was empty" >&2
  exit 1
fi

if [[ "$token" != github_pat_* ]]; then
  echo "error: expected a fine-grained PAT starting with github_pat_" >&2
  echo "       GitHub Copilot CLI rejects gh auth OAuth tokens (gho_...) and classic PATs (ghp_...)." >&2
  exit 1
fi

if [[ "$token" =~ [[:space:]] ]]; then
  echo "error: token contains whitespace; paste only the token value" >&2
  exit 1
fi

echo "Setting $secret_name for $repo..."
COPILOT_GITHUB_TOKEN="$token" gh aw secrets set "$secret_name" \
  --repo "$repo" \
  --value-from-env COPILOT_GITHUB_TOKEN

unset token COPILOT_GITHUB_TOKEN

echo "Done. You can now run: gh aw run weekly-real-world-testing"
