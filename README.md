# kaneo-cli

[![Build status](https://github.com/foae/kaneo-cli/actions/workflows/ci.yml/badge.svg?branch=main)](https://github.com/foae/kaneo-cli/actions/workflows/ci.yml)
[![Latest stable version](https://img.shields.io/github/v/release/foae/kaneo-cli)](https://github.com/foae/kaneo-cli/releases/latest)

Unofficial, JSON-first command-line client for [Kaneo](https://kaneo.app). Manage workspaces, projects, tasks, comments and labels from your terminal or scripts. Not affiliated with the Kaneo project.

## Install

**macOS / Linux** — detect your OS and architecture, download the latest stable binary, and verify its checksum:

```sh
curl -fsSL https://raw.githubusercontent.com/foae/kaneo-cli/main/install.sh | sh
```

Installs to `~/.local/bin`, without sudo. [Inspect the script](install.sh) or see [installer options](docs/installation.md#automatic-installation). If `kaneo-cli` is not found, add its installation directory to your `PATH`.

**Homebrew** — macOS / Linux:

```sh
brew tap foae/kaneo-cli https://github.com/foae/kaneo-cli
brew install foae/kaneo-cli/kaneo-cli
```

**Manual installation** — download from [Releases](https://github.com/foae/kaneo-cli/releases):

| System | Architectures | Installation / downloads |
| --- | --- | --- |
| macOS | Apple Silicon (ARM64), Intel (amd64) | [Guide](docs/installation.md#macos) · [Archives](https://github.com/foae/kaneo-cli/releases/latest) |
| Linux | ARM64, amd64 | [Guide](docs/installation.md#linux) · [Archives, deb / rpm](https://github.com/foae/kaneo-cli/releases/latest) |
| Windows | ARM64, amd64 | [Guide](docs/installation.md#windows) · [ZIP archives](https://github.com/foae/kaneo-cli/releases/latest) |

Also available: [Go install](docs/installation.md#go-install) · [Container](docs/installation.md#container) · [Agent skill](docs/installation.md#agent-skill).

## Quick start

```sh
kaneo-cli instance get-status
kaneo-cli auth login
kaneo-cli auth get-session
kaneo-cli --help
```

Uses Kaneo Cloud by default; login prints browser authorization instructions. For a self-hosted instance:

```sh
kaneo-cli --profile work --api-url https://kaneo.example.com/api auth login
```

Keep passing `--profile work` for that instance. See [authentication and automation](docs/authentication.md) for API keys, profiles and credential storage. If the OS keyring is unavailable, credentials fall back to a warned, permission-restricted **unencrypted file**.

API JSON goes to stdout; diagnostics go to stderr. Destructive commands require `--yes`. Explore `kaneo-cli <group> <action> --help` or the [command reference](docs/api/operations.md).

## Learn more

- [CLI behavior, input and output](docs/cli-contract.md)
- [API compatibility](docs/api/README.md) — 162 pinned operations mapped; external integrations are fixture-tested, not live-provider verified.
- [Verification and platform limitations](docs/verification.md)
- [Contributing](CONTRIBUTING.md) — Go 1.27+ and just; start with `just`, `just build`, `just check`.
- [Architecture](docs/architecture.md) · [Release process](docs/releases.md)

[MIT licensed](LICENSE); vendored upstream material retains its own attribution.
