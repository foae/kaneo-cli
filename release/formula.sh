#!/usr/bin/env bash
set -euo pipefail

# Generate a Formula only from assets and checksums already published on GitHub.
# It never releases, uploads, tags, or pushes.

usage() {
  printf 'usage: %s <tag> [output]\n' "${0##*/}" >&2
  exit 64
}

[[ $# -ge 1 && $# -le 2 ]] || usage
tag=$1
[[ $tag =~ ^v[0-9]+\.[0-9]+\.[0-9]+$ ]] || { printf '%s\n' "invalid release tag: $tag" >&2; exit 1; }
version=${tag#v}
output=${2:-Formula/kaneo-cli.rb}
repository=${GITHUB_REPOSITORY:-foae/kaneo-cli}
checksum_name="kaneo-cli_${version}_checksums.txt"
base_url="https://github.com/${repository}/releases/download/${tag}"
workdir=$(mktemp -d)
trap 'rm -rf "$workdir"' EXIT

command -v gh >/dev/null || { printf '%s\n' 'gh is required to read published release assets' >&2; exit 1; }
gh release view "$tag" --repo "$repository" >/dev/null
gh release download "$tag" --repo "$repository" --pattern "$checksum_name" --dir "$workdir"
checksum_file="$workdir/$checksum_name"
[[ -s $checksum_file ]] || { printf '%s\n' "missing published checksum file: $checksum_name" >&2; exit 1; }

sha_for() {
  local name=$1
  local sha
  sha=$(awk -v name="$name" '$2 == name || $2 == "*" name { print $1 }' "$checksum_file")
  [[ $sha =~ ^[0-9a-fA-F]{64}$ ]] || { printf '%s\n' "missing SHA-256 for published asset: $name" >&2; exit 1; }
  printf '%s\n' "${sha,,}"
}

rendered=$(<release/homebrew/kaneo-cli.rb.tmpl)
for platform in darwin_amd64 darwin_arm64 linux_amd64 linux_arm64; do
  archive="kaneo-cli_${version}_${platform}.tar.gz"
  sha=$(sha_for "$archive")
  key=${platform^^}
  rendered=${rendered//"{{${key}_URL}}"/"${base_url}/${archive}"}
  rendered=${rendered//"{{${key}_SHA256}}"/"$sha"}
done
rendered=${rendered//\{\{VERSION\}\}/${tag#v}}

case $rendered in
  *'{{'*|*'}}'*) printf '%s\n' 'formula template has unresolved placeholders' >&2; exit 1 ;;
esac
mkdir -p "$(dirname "$output")"
printf '%s\n' "$rendered" >"$output"
