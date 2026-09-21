# kaneo-cli

Unofficial, JSON-first Go CLI for [Kaneo](https://kaneo.app). Not affiliated with the Kaneo project.

**Status: early implementation.** The runtime, local profiles, credential storage (OS keyring first, warned plaintext fallback), API-key input, the RFC 8628 device login flow and all read-only (GET) API operations work. Mutations, uploads and integrations remain planned; publishing is disabled until full agreed coverage passes acceptance. No release or Homebrew installation is available yet.

## Try the foundation

Requires Go 1.27 or newer:

```sh
go run ./cmd/kaneo-cli --help
go run ./cmd/kaneo-cli version
go run ./internal/cmd/dev build
```

For development, install [just](https://github.com/casey/just) 1.58.0 and run `just` to list tasks, `just test` for Go tests, or `just check` for full shared verification. See [verification](docs/verification.md) for prerequisites and direct Go alternatives.

The eventual command interface uses a group and an action:

```text
kaneo-cli instance get-status
kaneo-cli auth get-session
kaneo-cli auth login
kaneo-cli org accept-invitation
kaneo-cli task update-status
```

Read commands such as `task get`, `project list` and `org list` are executable today; `org accept-invitation` and `task update-status` remain planned. Releases will target Linux, macOS and Windows on amd64 and arm64, with Homebrew for Linux/macOS.

## Start here

- Implementing: [AGENTS.md](AGENTS.md), then [work packets](docs/implementation.md).
- Command behavior: [CLI contract](docs/cli-contract.md) and [operation inventory](docs/api/operations.md).
- Authentication: [profiles and credential security](docs/authentication.md).
- Contributing: [CONTRIBUTING.md](CONTRIBUTING.md).
- Shipping: [release procedure](docs/releases.md).
- API baseline: [provenance and refresh](docs/api/README.md).

[MIT licensed](LICENSE); vendored upstream material retains its own attribution.
