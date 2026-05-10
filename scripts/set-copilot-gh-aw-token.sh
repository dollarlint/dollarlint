#!/usr/bin/env bash
set -euo pipefail

usage() {
  cat <<'EOF'
Usage: scripts/set-copilot-gh-aw-token.sh [--repo OWNER/REPO] [--copilot | --safe-outputs]

Sets a fine-grained GitHub PAT as a gh-aw repository secret.

Modes:
  --copilot       Set COPILOT_GITHUB_TOKEN for the Copilot agent engine. This is the default.
  --safe-outputs  Set GH_AW_GITHUB_TOKEN so safe outputs can open PRs and Discussions.

For --safe-outputs, mint the PAT for this repository with Contents, Pull requests,
Issues, and Discussions set to read/write.
EOF
}

repo="dollarlint/dollarlint"
mode="copilot"

while [[ $# -gt 0 ]]; do
  case "$1" in
    --repo)
      if [[ $# -lt 2 ]]; then
        echo "error: --repo requires OWNER/REPO" >&2
        exit 1
      fi
      repo="$2"
      shift 2
      ;;
    --copilot)
      mode="copilot"
      shift
      ;;
    --safe-outputs)
      mode="safe-outputs"
      shift
      ;;
    -h | --help)
      usage
      exit 0
      ;;
    */*)
      repo="$1"
      shift
      ;;
    *)
      echo "error: unknown argument: $1" >&2
      usage >&2
      exit 1
      ;;
  esac
done

case "$mode" in
  copilot)
    secret_name="COPILOT_GITHUB_TOKEN"
    token_hint="Copilot agent engine"
    ;;
  safe-outputs)
    secret_name="GH_AW_GITHUB_TOKEN"
    token_hint="safe outputs PR/Discussion publishing"
    ;;
  *)
    echo "error: unknown mode: $mode" >&2
    exit 1
    ;;
esac

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

printf "Paste fine-grained GitHub PAT for %s (%s) on %s: " "$secret_name" "$token_hint" "$repo" > /dev/tty
IFS= read -r -s token < /dev/tty
printf "\n" > /dev/tty

token="${token//$'\r'/}"

if [[ -z "$token" ]]; then
  echo "error: token was empty" >&2
  exit 1
fi

if [[ "$token" != github_pat_* ]]; then
  echo "error: expected a fine-grained PAT starting with github_pat_" >&2
  if [[ "$mode" == "copilot" ]]; then
    echo "       GitHub Copilot CLI rejects gh auth OAuth tokens (gho_...) and classic PATs (ghp_...)." >&2
  else
    echo "       Use a repository-scoped fine-grained PAT instead of a gh auth OAuth token (gho_...)." >&2
  fi
  exit 1
fi

if [[ "$token" =~ [[:space:]] ]]; then
  echo "error: token contains whitespace; paste only the token value" >&2
  exit 1
fi

echo "Setting $secret_name for $repo..."
export "$secret_name=$token"
gh aw secrets set "$secret_name" \
  --repo "$repo" \
  --value-from-env "$secret_name"

unset token "$secret_name"

echo "Done. You can now run: gh aw run agentic-product-testing"
