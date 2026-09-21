#!/usr/bin/env bash
set -euo pipefail

# Classify a same-tag release attempt without changing GitHub state.
# stdout is exactly one of fresh or complete; all other states fail closed.

usage() {
  printf 'usage: %s <tag> <expected-commit>\n' "${0##*/}" >&2
  exit 64
}

[[ $# == 2 ]] || usage
tag=$1
expected_sha=$2
repository=${GITHUB_REPOSITORY:-foae/kaneo-cli}
version=${tag#v}
checksum_name="kaneo-cli_${version}_checksums.txt"

[[ $tag =~ ^v[0-9]+\.[0-9]+\.[0-9]+$ ]] || { printf '%s\n' "invalid release tag: $tag" >&2; exit 1; }
[[ $expected_sha =~ ^[0-9a-f]{40}$ ]] || { printf '%s\n' 'expected commit must be a lowercase SHA-1 Git object ID' >&2; exit 1; }
command -v gh >/dev/null || { printf '%s\n' 'gh is required to inspect release state' >&2; exit 1; }
distribution=false
git cat-file -e "${expected_sha}^{commit}" || { printf '%s\n' 'verified commit is unavailable locally; fetch it before recovery' >&2; exit 1; }
distribution_path=$(git ls-tree "$expected_sha" -- release/container.sh)
if [[ -n $distribution_path ]]; then
  distribution=true
fi

remote_refs=$(git ls-remote --tags origin "refs/tags/$tag" "refs/tags/$tag^{}")
direct_sha=$(awk -v ref="refs/tags/$tag" '$2 == ref { print $1 }' <<<"$remote_refs")
peeled_sha=$(awk -v ref="refs/tags/$tag^{}" '$2 == ref { print $1 }' <<<"$remote_refs")
tag_sha=${peeled_sha:-$direct_sha}

if [[ -n $tag_sha && ${tag_sha,,} != "${expected_sha,,}" ]]; then
  printf '%s\n' "refusing recovery: $tag points to $tag_sha, not verified commit $expected_sha" >&2
  exit 1
fi

if ! response=$(gh api --include "repos/$repository/releases/tags/$tag" 2>/dev/null); then
  if [[ ! $response =~ ^HTTP/[0-9.]+[[:space:]]404([[:space:]]|$) ]]; then
    printf '%s\n' 'cannot establish release absence; refusing publication' >&2
    exit 1
  fi
  if [[ -n $tag_sha ]]; then
    printf '%s\n' "manual recovery required: $tag exists at the verified commit but has no GitHub release" >&2
    exit 1
  fi
  if [[ $distribution == true ]]; then
    go run ./internal/cmd/registry absent "$tag"
  fi
  printf '%s\n' fresh
  exit 0
fi

if [[ -z $tag_sha ]]; then
  printf '%s\n' "manual recovery required: GitHub release $tag exists without its immutable Git tag" >&2
  exit 1
fi

workdir=$(mktemp -d)
trap 'rm -rf "$workdir"' EXIT
gh release download "$tag" --repo "$repository" --pattern "$checksum_name" --dir "$workdir"
checksum_file="$workdir/$checksum_name"
[[ -s $checksum_file ]] || { printf '%s\n' "manual recovery required: $tag has no checksum asset" >&2; exit 1; }

assets=()
for platform in darwin_amd64 darwin_arm64 linux_amd64 linux_arm64 windows_amd64 windows_arm64; do
  extension=tar.gz
  [[ $platform == windows_* ]] && extension=zip
  assets+=("kaneo-cli_${version}_${platform}.${extension}")
done
if [[ $distribution == true ]]; then
  for arch in amd64 arm64; do
    assets+=("kaneo-cli_${version}_linux_${arch}.deb" "kaneo-cli_${version}_linux_${arch}.rpm")
  done
fi
for archive in "${assets[@]}"; do
  expected=$(awk -v name="$archive" '$2 == name || $2 == "*" name { print $1 }' "$checksum_file")
  [[ $expected =~ ^[0-9a-fA-F]{64}$ ]] || { printf '%s\n' "manual recovery required: checksum missing for $archive" >&2; exit 1; }
  gh release download "$tag" --repo "$repository" --pattern "$archive" --dir "$workdir"
  actual=$(sha256sum "$workdir/$archive" | awk '{print $1}')
  [[ ${actual,,} == "${expected,,}" ]] || { printf '%s\n' "manual recovery required: checksum mismatch for $archive" >&2; exit 1; }
done

if [[ $distribution == true ]]; then
  digest=$(gh release view "$tag" --repo "$repository" --json body --jq .body | tr -d '\r' | sed -n 's/^Container digest: \(sha256:[0-9a-f]\{64\}\)$/\1/p')
  [[ $digest =~ ^sha256:[0-9a-f]{64}$ ]] || { printf '%s\n' 'manual recovery required: missing or ambiguous container digest record' >&2; exit 1; }
  go run ./internal/cmd/registry verify "$tag" "$expected_sha" "$digest" >/dev/null
fi

printf '%s\n' complete
