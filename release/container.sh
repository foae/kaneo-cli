#!/usr/bin/env bash
set -euo pipefail
# Build only after GoReleaser has produced the binaries; publication is separate.
mode=${1:?usage: container.sh smoke|publish tag commit}
tag=${2:?missing tag}
commit=${3:?missing commit}
[[ $mode == smoke || $mode == publish ]] || exit 64
[[ $tag =~ ^v[0-9]+\.[0-9]+\.[0-9]+$ && $commit =~ ^[0-9a-f]{40}$ ]] || exit 64
if [[ $mode == publish ]]; then
  : "${GITHUB_OUTPUT:?publish requires a GitHub output file}"
fi
context=$(mktemp -d)
trap 'rm -rf "$context"' EXIT
cp Dockerfile LICENSE "$context/"
for arch in amd64 arm64; do
  mkdir -p "$context/linux/$arch"
  candidates=(dist/kaneo-cli_linux_"${arch}"_*/kaneo-cli)
  if [[ ${#candidates[@]} != 1 || ! -f ${candidates[0]} ]]; then
    printf 'expected exactly one release binary for %s\n' "$arch" >&2
    exit 1
  fi
  cp "${candidates[0]}" "$context/linux/$arch/kaneo-cli"
done
image=ghcr.io/foae/kaneo-cli
args=(--file "$context/Dockerfile" --label "org.opencontainers.image.source=https://github.com/foae/kaneo-cli" --label "org.opencontainers.image.revision=$commit" --label "org.opencontainers.image.version=${tag#v}" --label "org.opencontainers.image.licenses=MIT" --provenance=false)
if [[ $mode == smoke ]]; then
  docker buildx build "${args[@]}" --platform linux/amd64 --tag kaneo-cli-release-smoke --load "$context"
  docker run --rm --network none kaneo-cli-release-smoke version
  docker buildx build "${args[@]}" --platform linux/arm64 --output "type=oci,dest=$context/arm64.tar" "$context"
else
  go run ./internal/cmd/registry absent "$tag"
  docker buildx build "${args[@]}" --platform linux/amd64,linux/arm64 --tag "$image:$tag" --metadata-file "$context/build.json" --push "$context"
  digest=$(jq -er '.["containerimage.digest"]' "$context/build.json")
  [[ $digest =~ ^sha256:[0-9a-f]{64}$ ]]
  printf 'digest=%s\n' "$digest" >>"${GITHUB_OUTPUT:?missing GitHub output file}"
fi
