# Installation

Use a prebuilt binary; no Go or just installation is required. Releases support macOS, Linux and Windows on amd64 (Intel/AMD x86-64) and arm64 (Apple Silicon/AArch64). No 32-bit binaries are published.

## Automatic installation

On macOS or Linux:

```sh
curl -fsSL https://raw.githubusercontent.com/foae/kaneo-cli/main/install.sh | sh
```

The [installer](../install.sh) detects OS and architecture, resolves the latest stable GitHub release, downloads its archive and SHA-256 manifest, verifies the archive, and installs only the binary to `~/.local/bin`. It requires curl, tar, and sha256sum or shasum. It does not run sudo, modify shell profiles, or configure credentials. Checksums detect download corruption; they are not independent proof of publisher identity.

To inspect before running or choose an existing writable location:

```sh
curl -fsSL https://raw.githubusercontent.com/foae/kaneo-cli/main/install.sh -o install.sh
less install.sh
INSTALL_DIR="$HOME/.local/bin" sh install.sh
```

`INSTALL_DIR` must be absolute. Running the installer again upgrades the binary, replacing any existing `kaneo-cli` at that destination only after verification. Remove that binary to uninstall; profiles and credentials are untouched. On an emulated shell, architecture detection follows `uname -m`; use a manual native archive if needed.

If `kaneo-cli` is not found, add its installation directory to your `PATH`.

## Manual archives

Download the appropriate archive and `kaneo-cli_<version>_checksums.txt` from [Releases](https://github.com/foae/kaneo-cli/releases). Match its filename and SHA-256 to the manifest before extracting. Artifacts also have [GitHub build attestations](https://docs.github.com/en/actions/security-for-github-actions/using-artifact-attestations/verifying-the-provenance-of-an-artifact).

Repeat with a newer archive to upgrade. Remove the installed executable to uninstall; this does not delete credentials or profiles.

### macOS

Choose `darwin_arm64.tar.gz` for Apple Silicon or `darwin_amd64.tar.gz` for Intel (filenames include the release version).

1. Run `shasum -a 256 <archive>` and compare with the manifest.
2. Extract with `tar -xzf <archive>` in an empty directory.
3. Place `kaneo-cli` in a directory on your `PATH`, such as `~/.local/bin`.
4. Run `kaneo-cli version`.

### Linux

Choose `linux_amd64.tar.gz` or `linux_arm64.tar.gz` (filenames include the release version).

1. Run `sha256sum <archive>` and compare with the manifest.
2. Extract with `tar -xzf <archive>` in an empty directory.
3. Place `kaneo-cli` in a directory on your `PATH`, such as `~/.local/bin`.
4. Run `kaneo-cli version`.

Alternatively, choose the matching `.deb` or `.rpm` release asset, verify its checksum, then install with `sudo apt install ./<download>.deb` or `sudo dnf install ./<download>.rpm`. Packages install `/usr/bin/kaneo-cli`, require CA certificates, and do not configure repositories or services. Install a newer package to upgrade; remove with `sudo apt remove kaneo-cli` or `sudo dnf remove kaneo-cli`.

### Windows

Choose `windows_amd64.zip` for x86-64 or `windows_arm64.zip` for ARM64 (filenames include the release version).

1. In PowerShell, run `Get-FileHash <archive> -Algorithm SHA256` and compare with the manifest.
2. Extract with `Expand-Archive <archive> <empty-directory>`.
3. Place `kaneo-cli.exe` in a user-owned directory on your `Path`.
4. Open a new terminal and run `kaneo-cli version`.

## Homebrew

On macOS or Linux:

```sh
brew tap foae/kaneo-cli https://github.com/foae/kaneo-cli
brew install foae/kaneo-cli/kaneo-cli
```

Upgrade with `brew update` followed by `brew upgrade foae/kaneo-cli/kaneo-cli`. Uninstall with `brew uninstall kaneo-cli`. The tap follows reviewed Formula updates and can lag GitHub Releases.

## Go install

Requires [Go 1.27+](https://go.dev/doc/install), but no repository checkout or just:

```sh
go install github.com/foae/kaneo-cli/cmd/kaneo-cli@latest
```

Replace `@latest` with a release tag to pin a version. Repeat to upgrade. The binary goes to `GOBIN`, or the `bin` directory under `GOPATH` (by default `~/go/bin`). Go-installed builds report module versions; commit/date metadata may be unavailable.

## Container

The [container package](https://github.com/foae/kaneo-cli/pkgs/container/kaneo-cli) supports Linux amd64 and arm64. Choose a published `vX.Y.Z` tag from [Releases](https://github.com/foae/kaneo-cli/releases); no `latest` image alias is published:

```sh
docker run --rm ghcr.io/foae/kaneo-cli:<release-tag> version
docker run --rm ghcr.io/foae/kaneo-cli:<release-tag> instance get-status
```

The image runs as UID/GID `65532:65532`, includes CA certificates, and uses the CLI as its entrypoint. Pin the published digest for reproducibility. Supply invocation-only `KANEO_TOKEN` through a secret manager (`docker run --rm -e KANEO_TOKEN …`), never as a token literal. Persistent profiles require an explicitly mounted writable directory owned by UID 65532; containers generally have no OS keyring. Host `localhost` is not the container's localhost.

## Agent skill

The portable [Kaneo CLI skill](../skills/kaneo-cli/SKILL.md) teaches commands and safety rules using the [Agent Skills format](https://agentskills.io/specification). It is model- and harness-agnostic; it does not install the executable or grant server access.

### Portable installation

1. Open the [latest release](https://github.com/foae/kaneo-cli/releases/latest), record its tag, and download **Source code (zip)** from that release.
2. Extract it and copy the complete `skills/kaneo-cli/` directory, including `SKILL.md` and `LICENSE`, into your agent's documented skill directory. There is no universal install path across agents; use its own skill installation mechanism.
3. Install the CLI from the same release and configure authentication separately. Reload skills using your agent's documented mechanism.

The repository release tag versions both the CLI and skill. The installed `SKILL.md` frontmatter records it as `metadata.version` (the tag without the `v`); compare that value with the [latest release](https://github.com/foae/kaneo-cli/releases/latest). To see exactly what changed, open `https://github.com/foae/kaneo-cli/compare/vOLD...vNEW` and filter **Files changed** to `skills/kaneo-cli/`, or run `git diff vOLD vNEW -- skills/kaneo-cli/` in a clone. A copy without `metadata.version` predates v1.8.0 and has an unknown version. To update, explicitly choose a newer release and replace the whole skill directory from its source archive. Do not infer the installed skill version from `kaneo-cli version`; the CLI does not inspect or update agent skill installations.

### Optional Claude Code plugin

The same skill can be installed through Claude Code's plugin interface. Replace `vX.Y.Z` with the tag recorded from the release:

```text
/plugin marketplace add https://github.com/foae/kaneo-cli.git#vX.Y.Z
/plugin install kaneo-cli@kaneo-cli
```

The plugin has no separate version counter; Claude uses the source commit as its cache key. Removing a marketplace also uninstalls its plugins. To change a pinned release, remove the marketplace, add it again at the new tag, and reinstall the plugin:

```text
/plugin marketplace remove kaneo-cli
/plugin marketplace add https://github.com/foae/kaneo-cli.git#vX.Y.Z
/plugin install kaneo-cli@kaneo-cli
```

Omitting `#vX.Y.Z` tracks the repository default branch instead of releases. See [Claude marketplace installation documentation](https://code.claude.com/docs/en/discover-plugins#add-marketplaces) for ref syntax and update behavior. Configure the CLI separately in either case.
