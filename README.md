# kaneo-cli

[![Build status](https://github.com/foae/kaneo-cli/actions/workflows/ci.yml/badge.svg?branch=main)](https://github.com/foae/kaneo-cli/actions/workflows/ci.yml)
[![Latest stable version](https://img.shields.io/github/v/release/foae/kaneo-cli)](https://github.com/foae/kaneo-cli/releases/latest)

Unofficial, JSON-first command-line client and [agent skill](#agent-skill) for [Kaneo](https://kaneo.app). Manage workspaces, projects, tasks, comments and labels from your terminal, scripts or AI coding agent. Not affiliated with the Kaneo project.

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

Also available: [Go install](docs/installation.md#go-install) · [Container](docs/installation.md#container).

## Agent skill

Teach any skill-capable coding agent to find Kaneo work, create tasks, update status and priority, and add comments using the portable [Kaneo CLI skill](skills/kaneo-cli/SKILL.md). It is model- and harness-agnostic.

Paste this prompt into your coding agent:

```text
Go to the GitHub repository and install kaneo-cli if it isn't already installed, together with its agent skill: https://github.com/foae/kaneo-cli
```

Then log in using the commands below. For manual skill installation, version pinning, updates, or the optional Claude Code plugin, see the [skill installation guide](docs/installation.md#agent-skill).

## Quick start

```sh
kaneo-cli auth login
kaneo-cli instance get-status
kaneo-cli auth get-session
kaneo-cli --help
```

Uses Kaneo Cloud by default; login prints browser authorization instructions. The default is the API base `https://cloud.kaneo.app/api`, not the Cloud dashboard URL. For a self-hosted instance, supply its API base (normally ending in `/api`):

```sh
kaneo-cli --api-url https://kaneo.example.com/api auth login
```

The CLI validates and normalizes ordinary URL spelling (for example, case, a default port, and one trailing slash), but never discovers, probes, or appends `/api` to a dashboard or proxy URL. An implicit Cloud default emits one stderr warning before API use; logging in persists that destination, so later commands do not receive that warning. Use `kaneo-cli profile get` to inspect the effective destination and timeout offline before logging in or changing anything.

Login saves the instance URL for subsequent commands. See [authentication and automation](docs/authentication.md) for API keys, multiple instances and credential storage. If the OS keyring is unavailable, credentials fall back to a warned, permission-restricted **unencrypted file**.

API JSON goes to stdout; diagnostics go to stderr. Destructive commands require `--yes`. Explore `kaneo-cli <group> <action> --help` or the [command reference](docs/api/operations.md).

## Learn more

- [CLI behavior, input and output](docs/cli-contract.md)
- [API compatibility](docs/api/README.md) — 162 pinned operations mapped; external integrations are fixture-tested, not live-provider verified.
- [Verification and platform limitations](docs/verification.md)
- [Contributing](CONTRIBUTING.md) — Go 1.27+ and just; start with `just`, `just build`, `just check`.
- [Architecture](docs/architecture.md) · [Release process](docs/releases.md)

[MIT licensed](LICENSE); vendored upstream material retains its own attribution.
