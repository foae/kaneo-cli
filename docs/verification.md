# Verification

## Explicit developer commands

Use Go 1.27+ and [just](https://github.com/casey/just) 1.58.0, the version pinned in CI. Install that version from the [upstream release](https://github.com/casey/just/releases/tag/1.58.0); check your installation with `just --version`. Recipes can be invoked from the repository root or a subdirectory and run at the root. Native Windows recipes use Windows PowerShell; Unix recipes use `sh`. No dotenv files are loaded implicitly.

Run `just` or `just --list` for the task menu; neither runs checks or starts services:

```sh
just check     # formatting, vet, lint, tests, tidy diff, inventory
just test      # Go tests only
just lint      # pinned golangci-lint, including test code
just fmt       # explicitly format Go files (writes files)
just build     # host executable
just cross     # six CGO-free release targets
just race      # race tests, supported native toolchain required
just vuln      # pinned govulncheck
just snapshot  # pinned GoReleaser, never publishes
just hooks     # explicitly opt in to Git hooks
```

The justfile is a convenience interface, not a second implementation of verification. CI's Linux/macOS/Windows verification matrix invokes `just check`, exercising the wrapper without repeating the full checks; hooks and the other CI jobs continue to call Go directly. Changes to recipes must preserve native Windows compatibility and nonzero failure propagation. Keep lifecycle logic and tool versions in Go, and update the documented just version alongside its CI pin.

`just test-end2end` is intentionally absent: the disposable environment below is not an automated CLI acceptance suite. Add that recipe only when shared checks and real built-CLI scenarios can run with automated bootstrap, bounded readiness, isolated credentials, owned resources and cleanup on failure/cancellation. Fixed ports require rejecting concurrent runs or a deliberate networking redesign.

Without just, these direct commands remain available from the repository root; they are implemented development tooling, not planned API commands:

```sh
go run ./internal/cmd/dev fmt       # explicitly format Go files
go run ./internal/cmd/dev check     # formatting, vet, lint, tests, tidy diff, inventory
go test ./...                      # Go tests only
go run ./internal/cmd/dev lint      # pinned golangci-lint, including test code
go run ./internal/cmd/dev build     # host executable
go run ./internal/cmd/dev cross     # six CGO-free release targets
go run ./internal/cmd/dev race      # race tests, supported native toolchain required
go run ./internal/cmd/dev vuln      # pinned govulncheck
go run ./internal/cmd/dev snapshot  # pinned GoReleaser, never publishes
```

The formatting check must fail on changes; use `fmt` deliberately to fix them. Tests should exercise observable behavior and real edge cases. Use fixture HTTP servers for wire-contract tests, temporary config directories for isolation and synthetic tokens only. Avoid snapshots that pin incidental prose or tests asserting source-code spelling.

### Lint policy

`lint` runs golangci-lint v2.13.2 through the version-pinned Go module in `internal/cmd/dev`, using `.golangci.yml`. No global installation is needed; the first run downloads/builds the tool, and later runs reuse Go's caches. `check` includes lint, so native CI and opt-in hooks enforce the same policy without a separate lint action. Neither command applies automatic fixes.

The explicit rule set is correctness-first: `errcheck`, `govet`, `ineffassign`, `staticcheck`, `unused`, `bodyclose`, `contextcheck`, `nilerr` and `nolintlint`. It covers unchecked errors, suspicious code, dead code, HTTP response cleanup and dropped inherited contexts. No complexity/line-length limits, mandatory error-wrapping style or blanket test exclusions. Formatting remains the existing `gofmt` check; dependency vulnerabilities remain the separate `vuln` command.

Fix the cause of findings. For a demonstrated false positive, use a narrowly scoped `//nolint:<linter> // concrete reason`; do not disable checks globally or discard errors merely to silence lint. Review the rule set and Go compatibility when updating the pinned tool.

Git hooks are opt-in via `go run ./internal/cmd/dev hooks`; they call shared checks and are not a substitute for CI. Never silently install hooks or overwrite an existing custom hook configuration.

CI uses GitHub-hosted runners only, read-only permissions for untrusted code, pinned actions and no privileged fork-PR execution. Native runtime coverage is precisely the workflow matrix, not whatever GOOS/GOARCH values cross-compile. Race detection requires a supported native C toolchain even though distributed binaries are CGO-free. Linux/macOS/Windows keyring behavior and Windows ACLs need additional real acceptance once implemented.

## Disposable real Kaneo

`integration/compose.yaml` pins Kaneo, PostgreSQL and MinIO by immutable image digest. This is a local test environment with deliberately public test credentials, **never a deployment template**. All exposed ports bind to loopback; PostgreSQL is not published. Use only disposable test data. Do not point these commands at an existing production Compose project.

Prerequisites: Docker Engine with Compose v2, free host ports 15173/19000/19001, and registry access. Always use the same explicit project name for startup and cleanup; unique names isolate volumes but not the fixed host ports.

```sh
docker compose -p kaneo-cli-acceptance -f integration/compose.yaml config --quiet
docker compose -p kaneo-cli-acceptance -f integration/compose.yaml up -d --wait --wait-timeout 180
curl --fail http://localhost:15173/api/instance/status
curl --fail http://localhost:19000/minio/health/ready
docker compose -p kaneo-cli-acceptance -f integration/compose.yaml exec -T -e MC_HOST_acceptance=http://acceptance:local-acceptance-only@localhost:19000 minio mc mb --ignore-existing acceptance/kaneo-uploads
```

On first start, instance status must report `hasUsers: false` and `hasAdmin: false`. Open `http://localhost:15173` and create a disposable account; the first signup is the instance administrator. Create a test workspace/project and an API key through Kaneo's UI. Use the configured `kaneo-cli` device client ID for device-login acceptance. No real email/social provider is configured; email/password sign-in is enabled for this isolated environment. Never record generated credentials in acceptance evidence.

MinIO shares Kaneo's network namespace deliberately: `http://localhost:19000` reaches the same storage service from Kaneo and from host CLI/browser clients, so presigned URLs remain valid without rewriting signed hosts. The upstream Compose example's internal `minio:9000` endpoint is not usable by an ordinary host browser. The pinned Quay image replaces the upstream example's inaccessible Docker Hub `minio/minio:latest` reference. For browser upload tests, configure MinIO CORS to allow only the local Kaneo origin if required by the actual browser response; CLI upload tests do not exercise browser CORS.

After testing, destroy **only this disposable project's** containers and volumes:

```sh
docker compose -p kaneo-cli-acceptance -f integration/compose.yaml down --volumes --remove-orphans
```

Always clean up after a failed test too. Capture safe status/error information before teardown, not credentials or raw auth logs. Do not run broad Docker prune commands.

## Acceptance evidence required from implementation agents

Record CLI commit, API snapshot hash, all image digests, actual Kaneo version if discoverable, OS/architecture, auth backend and exact scenario outcomes. A digest alone is not proof that the server matches the spec.

Required scenarios:

- Public instance/config reads and authentication exceptions; authenticated reads with API key and device token.
- Device approval, denial, expiry, slow-down, timeout and cancellation; no credentials in output.
- Two profiles/instances with different credentials; URL overrides and cross-origin redirects never leak credentials.
- Successful JSON, null and no-content output; large numbers; invalid input/401/403/404/API failure; stable exit statuses and stdout/stderr separation.
- Isolated workspace/project/task lifecycle and relationships, labels, comments, columns, notifications, time entries and every remaining inventory operation.
- Destructive operations refused before network access without `--yes`.
- Actual upload/download byte equality and error cleanup; binary output destination safeguards; no bearer token sent to storage.
- Native keyring success, unavailable-keyring warning/fallback, secure Unix mode/Windows DACL, concurrency and logout cleanup.
- Integration operations: fixture protocol coverage is distinct from real external-service acceptance. Document service credentials/prerequisites and evidence; never mark a mock as a live integration run.

The foundation can prove help/version, checks, inventory consistency, packaging and environment readiness only. Full API compatibility, authentication, profiles and all platform-specific credentials remain **pending implementation**. Publishing stays disabled until their evidence meets [release readiness](releases.md).

## Foundation evidence (2026-09-20)

Verified locally on Linux amd64 with Go 1.27.1:

- Help, both JSON version forms, linked provenance and invalid-input stdout/stderr/exit behavior.
- Shared checks (formatting, vet, tests, module tidiness and offline inventory), race detection and pinned vulnerability scan; no vulnerabilities reported.
- All six cross-builds and GoReleaser snapshot archives; archive contents, SHA-256 manifests and execution of the extracted Linux amd64 binary.
- Workflow syntax with actionlint, release scripts with ShellCheck, and execution of the disabled-gate, docs-only/no-release and pinned-tool-provenance shell steps.
- Release policy in a disposable Git repository: first `v0.1.0`, fix/feature/breaking bumps and non-release commits; later bumps agree with pinned svu and explicit `--v0`.
- Recovery using real local annotated Git tags and synthetic GitHub asset fixtures: fresh/complete states, permission failures, wrong commits, tag-only partial publication and corrupt assets; Formula rendering uses fixture checksums.
- Pinned OpenAPI and authentication document hashes; disposable Kaneo instance status and storage readiness/bucket creation, followed by teardown.

This is not evidence of GitHub publication, remote attestation, Homebrew installation, macOS/Windows execution, ARM execution or any API command. Those remain unexercised, and publishing is still disabled.
