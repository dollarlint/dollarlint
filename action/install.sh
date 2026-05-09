#!/usr/bin/env bash
set -euo pipefail

repository="${DOLLARLINT_REPOSITORY:-dollarlint/dollarlint}"
requested_version="${DOLLARLINT_VERSION:-}"

fail() {
  echo "::error::$*" >&2
  exit 1
}

curl_download() {
  local output="$1"
  local url="$2"
  local args=(-fsSL --retry 3 --retry-delay 1)

  if [ -n "${GITHUB_TOKEN:-}" ] && [ "${url#https://api.github.com/}" != "$url" ]; then
    args+=(
      -H "Authorization: Bearer ${GITHUB_TOKEN}"
      -H "Accept: application/vnd.github+json"
      -H "X-GitHub-Api-Version: 2022-11-28"
    )
  fi

  curl "${args[@]}" -o "$output" "$url"
}

resolve_latest_tag() {
  local tmp_json="$1"
  curl_download "$tmp_json" "https://api.github.com/repos/${repository}/releases/latest"
  sed -n 's/.*"tag_name"[[:space:]]*:[[:space:]]*"\([^"]*\)".*/\1/p' "$tmp_json" | head -n 1
}

resolve_requested_version() {
  if [ -n "$requested_version" ]; then
    printf '%s\n' "$requested_version"
    return
  fi

  local action_ref="${DOLLARLINT_ACTION_REF:-${GITHUB_ACTION_REF:-}}"
  case "$action_ref" in
    v[0-9]*.[0-9]*.[0-9]* | [0-9]*.[0-9]*.[0-9]*)
      printf '%s\n' "$action_ref"
      ;;
    *)
      printf '%s\n' "latest"
      ;;
  esac
}

detect_platform() {
  case "$(uname -s)" in
    Linux)
      os="linux"
      archive_ext="tar.gz"
      binary_name="dollarlint"
      ;;
    Darwin)
      os="darwin"
      archive_ext="tar.gz"
      binary_name="dollarlint"
      ;;
    MINGW* | MSYS* | CYGWIN* | Windows_NT)
      os="windows"
      archive_ext="zip"
      binary_name="dollarlint.exe"
      ;;
    *)
      fail "unsupported runner OS: $(uname -s)"
      ;;
  esac

  case "$(uname -m)" in
    x86_64 | amd64)
      arch="amd64"
      ;;
    arm64 | aarch64)
      arch="arm64"
      ;;
    *)
      fail "unsupported runner architecture: $(uname -m)"
      ;;
  esac
}

sha256_file() {
  local file="$1"
  if command -v sha256sum >/dev/null 2>&1; then
    sha256sum "$file" | awk '{ print $1 }'
    return
  fi
  if command -v shasum >/dev/null 2>&1; then
    shasum -a 256 "$file" | awk '{ print $1 }'
    return
  fi
  fail "sha256sum or shasum is required to verify the release archive"
}

tmp_dir="$(mktemp -d)"
trap 'rm -rf "$tmp_dir"' EXIT

version_request="$(resolve_requested_version)"
if [ "$version_request" = "latest" ]; then
  tag="$(resolve_latest_tag "${tmp_dir}/latest.json")"
  [ -n "$tag" ] || fail "could not resolve the latest DollarLint release"
else
  case "$version_request" in
    v*) tag="$version_request" ;;
    *) tag="v${version_request}" ;;
  esac
fi

version="${tag#v}"
detect_platform

archive="dollarlint_${version}_${os}_${arch}.${archive_ext}"
release_url="https://github.com/${repository}/releases/download/${tag}"

echo "Installing DollarLint ${tag} for ${os}/${arch}"
curl_download "${tmp_dir}/${archive}" "${release_url}/${archive}"
curl_download "${tmp_dir}/checksums.txt" "${release_url}/checksums.txt"

expected_checksum="$(awk -v archive="$archive" '$2 == archive { print $1 }' "${tmp_dir}/checksums.txt" | head -n 1)"
[ -n "$expected_checksum" ] || fail "checksums.txt does not contain ${archive}"

actual_checksum="$(sha256_file "${tmp_dir}/${archive}")"
if [ "$actual_checksum" != "$expected_checksum" ]; then
  fail "checksum mismatch for ${archive}"
fi

mkdir -p "${tmp_dir}/extract"
case "$archive_ext" in
  zip)
    unzip -q "${tmp_dir}/${archive}" -d "${tmp_dir}/extract"
    ;;
  tar.gz)
    tar -xzf "${tmp_dir}/${archive}" -C "${tmp_dir}/extract"
    ;;
esac

binary_path="$(find "${tmp_dir}/extract" -type f -name "$binary_name" | head -n 1)"
[ -n "$binary_path" ] || fail "release archive did not contain ${binary_name}"

install_dir="${RUNNER_TEMP:-${tmp_dir}}/dollarlint-${version}/bin"
mkdir -p "$install_dir"
cp "$binary_path" "${install_dir}/${binary_name}"
chmod +x "${install_dir}/${binary_name}"

if [ -n "${GITHUB_PATH:-}" ]; then
  echo "$install_dir" >> "$GITHUB_PATH"
fi

if [ -n "${GITHUB_OUTPUT:-}" ]; then
  {
    echo "version=${version}"
    echo "binary=${install_dir}/${binary_name}"
  } >> "$GITHUB_OUTPUT"
fi

"${install_dir}/${binary_name}" version
