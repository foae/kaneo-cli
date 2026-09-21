# kaneo-cli

Unofficial, JSON-first command-line client for [Kaneo](https://kaneo.app). Manage workspaces, projects, tasks, comments, labels and more from your terminal or scripts. Not affiliated with the Kaneo project.

[Latest release](https://github.com/foae/kaneo-cli/releases/latest) · [Command reference](docs/api/operations.md) · [Authentication](docs/authentication.md) · [Contributing](CONTRIBUTING.md)

## Install

Choose **Homebrew** on macOS/Linux, **Go install** if you already use Go, or a **prebuilt archive** on any supported platform. Homebrew and archives do not require Go or just.

### Homebrew — macOS and Linux

```sh
brew tap foae/kaneo-cli https://github.com/foae/kaneo-cli
brew install foae/kaneo-cli/kaneo-cli
kaneo-cli version
```

To upgrade:

```sh
brew update
brew upgrade foae/kaneo-cli/kaneo-cli
```

### Go install — macOS, Linux and Windows

Requires [Go](https://go.dev/doc/install) **1.27 or newer**. No repository checkout or just installation is needed.

```sh
go install github.com/foae/kaneo-cli/cmd/kaneo-cli@latest
kaneo-cli version
```

For a pinned version, replace `@latest` with `@v1.0.0`. Run the installation command again to upgrade.

If `kaneo-cli` is not found, add Go's binary directory to your `PATH`. By default:

- **macOS/Linux:** `$HOME/go/bin`; for the current shell, run `export PATH="$HOME/go/bin:$PATH"`. Add that line to your shell startup file to keep it.
- **Windows:** `%USERPROFILE%\go\bin`; add it to your user `Path` in Environment Variables, then reopen your terminal. For the current PowerShell session, run `$env:Path += ";$env:USERPROFILE\go\bin"`.

If you configured `GOBIN` or `GOPATH`, use your configured binary directory instead. Go-installed builds report the module version; commit/date metadata may be unavailable. Release archives include all three.

### Prebuilt archives — no build tools required

Download the archive matching your machine from [GitHub Releases](https://github.com/foae/kaneo-cli/releases/latest):

| Operating system | Architecture | Archive suffix | Homebrew |
| --- | --- | --- | --- |
| macOS | Apple Silicon / ARM64 | `darwin_arm64.tar.gz` | Yes |
| macOS | Intel / x86-64 | `darwin_amd64.tar.gz` | Yes |
| Linux | x86-64 | `linux_amd64.tar.gz` | Yes |
| Linux | ARM64 / AArch64 | `linux_arm64.tar.gz` | Yes |
| Windows | x86-64 | `windows_amd64.zip` | No |
| Windows | ARM64 | `windows_arm64.zip` | No |

These are the six release targets; no 32-bit binaries are published. `amd64` means x86-64, including both Intel and AMD processors.

1. Download the archive and the release's `kaneo-cli_<version>_checksums.txt` manifest.
2. Verify the archive's SHA-256 against its entry in the manifest. Use `sha256sum <archive>` on Linux, `shasum -a 256 <archive>` on macOS, or `Get-FileHash <archive> -Algorithm SHA256` in PowerShell. Archives and the manifest also have [GitHub build attestations](https://docs.github.com/en/actions/security-for-github-actions/using-artifact-attestations/verifying-the-provenance-of-an-artifact).
3. Extract the archive. Put `kaneo-cli` (Windows: `kaneo-cli.exe`) in a directory on your `PATH`. On macOS/Linux, `~/.local/bin` is one option; create it and add it to `PATH` if needed. On Windows, choose a user-owned directory and add it to your user `Path`.
4. Open a new terminal and run `kaneo-cli version`.

To upgrade a manual installation, repeat these steps with the new release and replace the executable.

### Why GitHub Releases, not GitHub Packages?

The downloadable binaries are **release assets**, not entries in the repository's Packages sidebar. [GitHub Packages](https://docs.github.com/en/packages/learn-github-packages/introduction-to-github-packages#supported-clients-and-formats) does not provide a Go module registry. `go install` resolves this repository's tagged Go module, while Homebrew downloads the release archives. No container image is currently published.

## Quick start

### Kaneo Cloud

The default API URL is `https://cloud.kaneo.app/api`. Check connectivity without logging in, then authenticate:

```sh
kaneo-cli instance get-status
kaneo-cli auth login
kaneo-cli auth get-session
```

`auth login` starts device authorization and prints instructions to follow in a browser. It stores the credential only after successful authorization.

### Self-hosted Kaneo

Use the full API URL, including `/api`. A named profile keeps its URL and credentials separate from your Cloud profile:

```sh
kaneo-cli --profile work --api-url https://kaneo.example.com/api auth login
kaneo-cli --profile work auth get-session
```

Replace the example URL with your instance's URL. Keep passing `--profile work` to use that profile.

### API keys and automation

Instead of device login, read an API key from a protected file:

```sh
kaneo-cli auth login --api-key-file /path/to/protected-api-key
```

Use `--api-key-file -` to read from stdin. Automation can supply `KANEO_TOKEN` through its secret manager for a single invocation without persisting it. Do not put credentials in command arguments or shell history.

Credentials use the OS keyring first: macOS Keychain, Windows Credential Manager or Linux Secret Service. If unavailable, the CLI warns and uses a permission-restricted **unencrypted file**. See [credential storage and profiles](docs/authentication.md) before using a shared or headless machine.

### Discover commands

```sh
kaneo-cli --help
kaneo-cli project list --help
kaneo-cli task create --help
kaneo-cli task update-status --help
```

Commands follow `kaneo-cli <group> <action>`. Use each command's help for required IDs, flags and input. Successful API JSON goes to stdout; diagnostics and errors go to stderr. Destructive commands require explicit `--yes` and never prompt implicitly. See the [CLI contract](docs/cli-contract.md) for JSON input, file transfers and exit codes.

**Update notices (v1.1.0+):** help (`help`, `--help`, `-h`, including command help) and version (`version`, `--version`) check GitHub's latest stable release. An outdated stable build prints one short upgrade notice to stderr; stdout and exit status are unchanged. The unauthenticated check has a one-second timeout, stays silent on failure and writes no local state. Development and prerelease builds skip the check; ordinary API commands never check.

## Compatibility

Get the [latest stable release](https://github.com/foae/kaneo-cli/releases/latest). All **162 pinned API operations** have canonical command mappings, including browser URL handoffs. This is coverage of the [pinned API baseline](docs/api/README.md), not a promise of compatibility with every Kaneo server version.

- GitHub, Gitea, Slack, Discord, Mattermost and Telegram integrations are fixture-tested, **not live-provider verified**; live-provider acceptance is explicitly deferred for v1.0.0.
- Native credential checks passed on Linux, macOS and Windows. Homebrew installation was exercised on Linux amd64; macOS Homebrew and ARM installation remain unverified. Cross-builds alone are not native runtime validation.
- Browser-navigation commands print a URL; they do not complete authorization on your behalf.

See [acceptance evidence and limitations](docs/verification.md) for exact revisions, environments and exercised behavior.

## Build and develop

Repository development requires **Go 1.27+** and **[just 1.58.0](https://github.com/casey/just/releases/tag/1.58.0)**, the version pinned in CI. Install both, then:

```sh
git clone https://github.com/foae/kaneo-cli.git
cd kaneo-cli
just
just build
just check
```

`just` lists the available tasks. `just build` writes the host executable to `dist/kaneo-cli_<os>_<arch>` (plus `.exe` on Windows); it does not install it on your `PATH`.

| Task | Command |
| --- | --- |
| Build the host executable | `just build` |
| Shared checks: formatting, vet, lint, tests, module tidiness, inventory | `just check` |
| Go tests only | `just test` |
| Static analysis | `just lint` |
| Format Go source | `just fmt` |
| Race detector | `just race` |
| Vulnerability scan | `just vuln` |
| Cross-build all six release targets | `just cross` |
| Build release archives locally, without publishing | `just snapshot` |
| Opt in to Git hooks | `just hooks` |

Use `just` for repository workflows locally and in CI. See [verification](docs/verification.md) for tool prerequisites and the disposable integration environment, and [CONTRIBUTING.md](CONTRIBUTING.md) for the contribution workflow. Hooks are never installed automatically.

## Documentation

- [Command behavior and output](docs/cli-contract.md)
- [Operation inventory](docs/api/operations.md)
- [Authentication and profiles](docs/authentication.md)
- [Architecture](docs/architecture.md)
- [API baseline and provenance](docs/api/README.md)
- [Verification and acceptance evidence](docs/verification.md)
- [Release procedure](docs/releases.md)

[MIT licensed](LICENSE); vendored upstream material retains its own attribution.
