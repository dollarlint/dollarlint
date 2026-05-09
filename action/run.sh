#!/usr/bin/env bash
set -euo pipefail

fail() {
  echo "::error::$*" >&2
  exit 1
}

is_true() {
  case "$(printf '%s' "$1" | tr '[:upper:]' '[:lower:]')" in
    true | 1 | yes | y | on)
      return 0
      ;;
    *)
      return 1
      ;;
  esac
}

write_output() {
  if [ -n "${GITHUB_OUTPUT:-}" ]; then
    echo "$1" >> "$GITHUB_OUTPUT"
  fi
}

target="${DOLLARLINT_PATH:-.}"
working_directory="${DOLLARLINT_WORKING_DIRECTORY:-}"
config="${DOLLARLINT_CONFIG:-}"
format="${DOLLARLINT_FORMAT:-text}"
output="${DOLLARLINT_OUTPUT:-}"
upload_sarif="${DOLLARLINT_UPLOAD_SARIF:-false}"
extra_args="${DOLLARLINT_ARGS:-}"

if ! command -v dollarlint >/dev/null 2>&1; then
  fail "dollarlint is not on PATH"
fi

if [ -n "$working_directory" ]; then
  cd "$working_directory"
fi

if is_true "$upload_sarif"; then
  format="sarif"
  if [ -z "$output" ]; then
    output="${RUNNER_TEMP:-.}/dollarlint.sarif"
  fi
fi

args=(validate)

if [ -n "$config" ]; then
  args+=(--config "$config")
fi

if [ -n "$format" ]; then
  args+=(--format "$format")
fi

if [ -n "$output" ]; then
  args+=(--output "$output")
fi

if [ -n "$extra_args" ]; then
  while IFS= read -r arg || [ -n "$arg" ]; do
    [ -n "$arg" ] || continue
    args+=("$arg")
  done <<< "$extra_args"
fi

args+=("$target")

echo "Running: dollarlint ${args[*]}"
set +e
dollarlint "${args[@]}"
exit_code=$?
set -e

write_output "exit_code=${exit_code}"

if is_true "$upload_sarif" && [ -n "$output" ]; then
  sarif_file="$output"
  case "$sarif_file" in
    /* | [A-Za-z]:/* | [A-Za-z]:\\*)
      ;;
    *)
      sarif_file="${PWD}/${sarif_file}"
      ;;
  esac

  if [ -f "$sarif_file" ]; then
    write_output "sarif_file=${sarif_file}"
  fi
fi
