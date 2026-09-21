# kaneo-cli

Unofficial, JSON-first Go CLI for [Kaneo](https://kaneo.app). Not affiliated with the Kaneo project.

**Status: v1.0.0 release preparation.** All 162 pinned operations have command mappings, including browser-navigation URL handoffs. Runtime profiles, credential storage (OS keyring first, warned plaintext fallback), API-key input, device login, JSON operations and file transfers are implemented. Coverage is not full compatibility: external integrations are fixture-tested but not live-provider verified. Publication remains gated on release acceptance. See [acceptance evidence](docs/verification.md).

## Run from source

Requires Go 1.27 or newer; the build recipe also requires [just](https://github.com/casey/just) 1.58.0:

```sh
go run ./cmd/kaneo-cli --help
go run ./cmd/kaneo-cli instance get-status
just build
```

For development, install [just](https://github.com/casey/just) 1.58.0 and run `just` to list tasks. Routine verification uses `just check`, `just race`, `just vuln`, and `just cross`; `just test` runs Go tests only. See [verification](docs/verification.md) for prerequisites and direct Go alternatives.

The command interface uses a group and an action:

```text
kaneo-cli instance get-status
kaneo-cli auth get-session
kaneo-cli auth login
kaneo-cli org accept-invitation
kaneo-cli task update-status
```

Commands such as `task get`, `project list`, `task create` and `task update-status` are executable today. Releases will target Linux, macOS and Windows on amd64 and arm64, with Homebrew for Linux/macOS.

## Start here

- Implementing: [AGENTS.md](AGENTS.md), then [work packets](docs/implementation.md).
- Command behavior: [CLI contract](docs/cli-contract.md) and [operation inventory](docs/api/operations.md).
- Authentication: [profiles and credential security](docs/authentication.md).
- Contributing: [CONTRIBUTING.md](CONTRIBUTING.md).
- Shipping: [release procedure](docs/releases.md).
- API baseline: [provenance and refresh](docs/api/README.md).

[MIT licensed](LICENSE); vendored upstream material retains its own attribution.
