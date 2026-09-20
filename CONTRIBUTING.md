# Contributing

Read [AGENTS.md](AGENTS.md) before implementation. The [work packets](docs/implementation.md) describe what remains; the foundation does not contact Kaneo.

Use Go 1.27+, Git, and [just](https://github.com/casey/just) 1.58.0 (the CI-pinned version). Run `just` to discover tasks, `just test` for Go tests only, and `just check` for shared verification. See [verification](docs/verification.md) for command details and direct Go alternatives. No Node runtime is required. Docker Compose is needed only for real-instance acceptance; GoReleaser and svu are pinned by release tooling.

Use `just lint` for pinned golangci-lint feedback; the shared `check` command, CI and opt-in hooks include it. The [lint policy](docs/verification.md#lint-policy) favors correctness over stylistic restrictions. Fix findings rather than weakening the configuration.

Keep changes focused. Include the operation IDs affected, behavior evidence, any upstream discrepancies, and platforms actually executed in your PR. Never commit credentials, local profiles, live HTTP recordings or generated binary artifacts. Regenerate the API inventory only from reviewed pinned inputs.

Use Conventional Commit squash titles: `feat:`, `fix:`, `perf:`, `docs:`, `test:`, `refactor:`, `build:`, `ci:`, `chore:`. Mark breaking changes with `!` or a `BREAKING CHANGE:` footer. These drive automatic releases after activation; see [release rules](docs/releases.md). Do not manually update a version string or move a published tag.

Local hooks are opt-in; CI remains authoritative. Formatting is explicit (`go fmt ./...`); CI checks formatting without silently changing files. Maintainers must configure branch protection and required checks in GitHub separately; adding workflow files does not activate those settings.
