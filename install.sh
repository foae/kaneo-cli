#!/bin/sh
# Download a verified release binary without sudo or shell-profile changes.
set -eu

fail() { printf 'kaneo-cli: %s\n' "$*" >&2; exit 1; }

main() {
    [ "$#" -eq 0 ] || fail 'usage: sh install.sh (set INSTALL_DIR to choose a destination)'
    case "$(uname -s)" in
        Linux) os=linux ;;
        Darwin) os=darwin ;;
        *) fail 'unsupported operating system; see https://github.com/foae/kaneo-cli/releases' ;;
    esac
    case "$(uname -m)" in
        x86_64|amd64) arch=amd64 ;;
        arm64|aarch64) arch=arm64 ;;
        *) fail 'unsupported architecture; supported: amd64 and arm64' ;;
    esac
    for tool in curl tar mktemp; do
        command -v "$tool" >/dev/null 2>&1 || fail "required command not found: $tool"
    done
    if command -v sha256sum >/dev/null 2>&1; then
        checksum=sha256sum
    elif command -v shasum >/dev/null 2>&1; then
        checksum=shasum
    else
        fail 'sha256sum or shasum is required'
    fi
    install_dir=${INSTALL_DIR:-"$HOME/.local/bin"}
    case "$install_dir" in /*) ;; *) fail 'INSTALL_DIR must be an absolute path' ;; esac
    work=$(mktemp -d)
    staged=
    trap 'rm -rf "$work"; if [ -n "$staged" ]; then rm -f "$staged"; fi' 0
    trap 'exit 130' INT
    trap 'exit 143' TERM
    base=https://github.com/foae/kaneo-cli/releases
    release=$(curl --fail --silent --show-error --location --proto '=https' --proto-redir '=https' --connect-timeout 15 --max-time 120 --output /dev/null --write-out '%{url_effective}' "$base/latest")
    version=$(printf '%s\n' "$release" | sed -n 's|^https://github.com/foae/kaneo-cli/releases/tag/v\([0-9][0-9]*\.[0-9][0-9]*\.[0-9][0-9]*\)$|\1|p')
    [ -n "$version" ] || fail 'could not resolve the latest stable release'
    archive="kaneo-cli_${version}_${os}_${arch}.tar.gz"
    manifest="kaneo-cli_${version}_checksums.txt"
    for file in "$archive" "$manifest"; do
        curl --fail --silent --show-error --location --proto '=https' --proto-redir '=https' --connect-timeout 15 --max-time 120 --output "$work/$file" "$base/download/v$version/$file"
    done
    expected=
    while read -r hash name rest; do
        if [ "$name" = "$archive" ]; then
            [ -z "$expected" ] || fail 'duplicate checksum entry'
            [ -z "$rest" ] || fail 'invalid checksum entry'
            expected=$hash
        fi
    done < "$work/$manifest"
    [ "${#expected}" -eq 64 ] || fail 'missing or invalid archive checksum'
    case "$expected" in *[!0-9a-f]*) fail 'invalid archive checksum' ;; esac
    if [ "$checksum" = sha256sum ]; then
        actual=$(sha256sum "$work/$archive")
    else
        actual=$(shasum -a 256 "$work/$archive")
    fi
    [ "${actual%% *}" = "$expected" ] || fail 'archive checksum mismatch; nothing installed'
    tar -xzf "$work/$archive" -C "$work" kaneo-cli
    [ -f "$work/kaneo-cli" ] && [ ! -L "$work/kaneo-cli" ] || fail 'archive does not contain a regular binary'
    mkdir -p "$install_dir"
    [ ! -d "$install_dir/kaneo-cli" ] || fail 'destination is a directory'
    staged=$(mktemp "$install_dir/.kaneo-cli.XXXXXXXX")
    cat "$work/kaneo-cli" > "$staged"
    chmod 755 "$staged"
    mv -f "$staged" "$install_dir/kaneo-cli"
    staged=
    printf 'Installed kaneo-cli %s to %s/kaneo-cli\n' "$version" "$install_dir"
    printf 'If kaneo-cli is not found, add %s to your PATH.\n' "$install_dir"
}

# Wrapping execution avoids running an incomplete download piped into sh.
main "$@"
