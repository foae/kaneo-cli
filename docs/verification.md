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

The justfile is a convenience interface, not a second implementation of verification. CI runs only on pushes to `main`, not on pull requests: its Linux/macOS/Windows verification matrix invokes `just check`, followed by `just cross`. Release preparation uses `just snapshot`. Race detection and vulnerability scanning remain optional local commands (`just race` and `just vuln`), not hosted jobs. Run local checks before requesting review or merging. Hooks call Go directly so installed hooks do not require just. Changes to recipes must preserve native Windows compatibility and nonzero failure propagation. Keep lifecycle logic and tool versions in Go, and update the documented just version alongside its CI pin.

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

Git hooks are opt-in via `just hooks`; they call shared Go checks directly and are not a substitute for CI. Never silently install hooks or overwrite an existing custom hook configuration.

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

The packet evidence below distinguishes implemented behavior from accepted compatibility. Browser operations, recorded API discrepancies, native credential stores and live external-service scenarios still have outstanding acceptance requirements. Publishing stays disabled until the evidence meets [release readiness](releases.md).

## Packet 1 evidence (2026-09-20)

Implemented: named profiles (`profile set|use|list|get|delete`), keyring-first credential storage with a warned plaintext fallback, API-key login (`auth login --api-key-file`), RFC 8628 device login (`auth login`), local logout (`auth logout`) and the public/session reads `instance get-status`, `config get` and `auth get-session`. Those three operations are marked `implemented` in `api/commands.json`; `release/readiness.json` remains disabled and no native keyring claim is made.

Environment: Linux amd64, Go 1.27.1, just 1.58.0. API snapshot SHA-256 `a5f29855e3f25c703bf665fd17703cc79b672bd4e24f9f5fbd8f0c1b8e44db9e`. Disposable Kaneo image `ghcr.io/usekaneo/kaneo@sha256:a85a23996c36166cfcebcb4ee161b40cc62c7b924faf06353684a84d5e84162c` (the compose pin); observed server version `2.25.0`. The instance was torn down with its volumes after testing.

Checks, all passing: `just check` (formatting, vet, pinned golangci-lint, tests, module tidiness, offline inventory), `just race`, `just cross` (six CGO-free targets) and `just vuln` (no vulnerabilities). Test suites cover the transport, configuration, credential store and CLI against fixture servers.

Binary scenarios exercised against a local fixture server and the disposable instance:

- IO separation: API JSON on stdout, diagnostics and warnings on stderr; no-content responses leave stdout empty; JSON `null` is preserved.
- Exit statuses 0/2/3/4/5 and interruption (130) with one structured error object carrying stable code, HTTP status and operation ID.
- Credentialed cross-origin redirects are refused; unauthenticated redirects are followed; plain-HTTP credential warnings appear once per invocation.
- Timeout and context cancellation stop requests.
- Profile isolation: two profiles send their own credentials; a changed origin is never sent another URL's credential.
- `KANEO_TOKEN` is invocation-only and is not persisted.
- Credential lifecycle: an available keyring backend (mocked), the unavailable-keyring fallback with a warning (the real headless Linux path), an access-denied keyring that is not silently downgraded, migration that clears the stale plaintext fallback, and logout/profile deletion that remove secrets. Config directory/file modes are 0700/0600 and concurrent updates are exercised.
- Device flow (fixture): pending, slow-down, access-denied, expired-token, invalid-client, expiry and cancellation transitions, with no device code or token in output.
- Disposable instance: public instance status and config, a null session, an authenticated session with a real API key, a real device-code request with observed `authorization_pending` polling and Ctrl-C interruption (`interrupted`, no credential stored), and a real `invalid_client` rejection.

Pending native or external evidence, not claimed: native OS keyring success and Windows DACL enforcement (this host has no reachable Secret Service, so only the warned fallback ran), macOS Keychain and native macOS/Windows execution, and real device approval/denial (fixture-verified; the disposable flow was observed for pending, rejection and interruption only).

Post-review hardening (same date), after a seven-seat cross-model panel: non-timeout transport failures map to exit 4 with safe messages; device denial, expiry and invalid-client map to exit 3; unrecognized error bodies are no longer echoed; cross-origin redirects carrying a bearer token or a sensitive body (including 307/308 replays) are refused; public reads no longer load a credential and the `profile`/`logout` paths no longer depend on a reachable backend; logout requires `--yes`; the fallback credential file is permission-checked on read and restricted before any secret byte is written; device interval and expiry are bounded against overflow and hostile values; and a 2xx polling error payload continues polling. `just check`, `just race`, `just cross` and `just vuln` were re-run after these changes. Native keyring/DACL evidence remains pending.

## Packet 2 evidence (2026-09-21)

Commit `8341d6d`. Implemented all 52 remaining read-only GET operations (57 GET operations total; `auth get-session`, `config get` and `instance get-status` came from packet 1). Coverage statuses are marked `implemented` in `api/commands.json` and regenerated into the inventory. `read_ops_test.go` cross-checks the command table against `api/operations.json`, failing if any GET operation is uncovered or any command/path/parameter/security classification drifts from the pinned inventory.

Environment: Linux amd64, Go 1.27.1, just 1.58.0. API snapshot SHA-256 `a5f29855e3f25c703bf665fd17703cc79b672bd4e24f9f5fbd8f0c1b8e44db9e`. Disposable Kaneo image `ghcr.io/usekaneo/kaneo@sha256:a85a23996c36166cfcebcb4ee161b40cc62c7b924faf06353684a84d5e84162c`; observed server version `2.25.0`; stack torn down with volumes after testing.

Checks, all passing: `just check` (formatting, vet, pinned golangci-lint, tests, module tidiness, offline inventory), `just race`, `just cross` (six CGO-free targets) and `just vuln` (no vulnerabilities).

Real disposable-instance reads. The instance was bootstrapped over HTTP (Better Auth sign-up, an API key created with an explicit `Origin`, then a workspace, project, columns and task seeded directly through the API). Reads were exercised with a stored API key and, for organization endpoints, with a session token passed through the invocation-only `KANEO_TOKEN`:

- Public reads (`instance get-status`, `config get`, `auth get-session`, `mcp get-authorization-request`, `user download-avatar`, `asset download`) work with no credential and send no `Authorization` header; `auth get-session` returns `null` unauthenticated and a session when authenticated.
- Authenticated JSON reads succeeded for activity, column, comment, custom-field (four), external-link, label (task/workspace), notification, notification-preference, oauth, project (get/list), search, task (get/list/export), task-relation, time-entry, workflow-rule, workspace members, and the GitHub/Gitea/Slack/Discord/Mattermost/Telegram/webhook integration reads where an integration exists.
- Organization reads: `org list`, `org get-full`, `org list-members`, `org list-invitations`, `org list-roles`, `org list-teams`, `org get-active-member`, `org get-active-member-role`, `org list-user-teams` and `org list-team-members` succeed once the session has an active organization. Without an active organization the server returns HTTP 400 (`No active organization` / `Organization ID is required`), mapped to `invalid_request`/exit 5; with API-key-only auth that prerequisite cannot be set from a read command.
- Absent objects (`project get`, `task get`, `label get`, `time-entry get`, `invitation get`, `activity list-task`, binary `user download-avatar`/`asset download`) return HTTP 404, mapped to `not_found`/exit 5 with the operation ID and no credentials in output.
- An authenticated read with no bound credential returns HTTP 401, mapped to `authentication_failed`/exit 3. A missing required flag is rejected before any network access with `invalid_arguments`/exit 2.
- Binary fidelity: a seeded PNG avatar uploaded out of band downloaded byte-for-byte identically (matching SHA-256). `--output` is required, an existing destination is refused without `--force`, and `--output -` is the only path that writes to stdout. `asset download` real success still needs an uploaded task asset and is pending the packet 3 presigned flow; its safeguards are covered by fixture tests.

Upstream discrepancy (recorded, not silently resolved): `getOrganizationRole` (`org get-role`, `GET /auth/organization/get-role`) documents zero parameters in the pinned snapshot, but the server requires a `roleId` or `roleName` query parameter. Supplying either changes the response from `[query] Invalid input` to `Role not found`/success, confirming the requirement. The command keeps the documented (empty) parameter set and returns the server's `invalid_request`; the missing parameter is **not** invented. A contract decision is needed before this command can be used successfully.

Pending evidence, not claimed: successful `asset download` of a real task asset; `org list-user-invitations` real success (the server returns HTTP 403 until the account's email is verified, which the disposable environment cannot do without SMTP); the two deferred browser-navigation endpoints (`auth get-device-authorization-page`, `mcp start-authorization`), whose correct contract is a deliberate browser interaction rather than a JSON wrapper.

## Packet 3 evidence (2026-09-21)

Commit `b4b60ca`. Implemented all 105 remaining non-GET operations: 103 generic JSON mutations plus dedicated `task create-image-upload` (presigned) and `user upload-avatar` (base64) commands. 160 of 162 pinned operations are now implemented; the two GET browser-navigation endpoints (`auth get-device-authorization-page`, `mcp start-authorization`) remain deferred. `write_ops_test.go` cross-checks the mutation table against `api/operations.json` and fails if any operation is uncovered or a method/path/parameter/body/security classification drifts.

Design: a mutating operation reads its JSON body from `--body-file PATH` or `--body-file -`, validates it as a JSON object before any network access, and passes it through byte-for-byte so unknown fields and numeric precision survive. Mutations are never retried. A destructive operation (delete/remove/cancel/leave/reject/clear/detach, or `task bulk-update`) requires `--yes` before any credential or network work. `task create-image-upload` streams the file to the presigned URL with a client that never carries the API credential and rejects a URL with userinfo or a non-http(s) scheme.

Environment: Linux amd64, Go 1.27.1, just 1.58.0. API snapshot SHA-256 `a5f29855e3f25c703bf665fd17703cc79b672bd4e24f9f5fbd8f0c1b8e44db9e`. Disposable Kaneo image `ghcr.io/usekaneo/kaneo@sha256:a85a23996c36166cfcebcb4ee161b40cc62c7b924faf06353684a84d5e84162c`; observed server version `2.25.0`; stack torn down with its volumes after testing.

Checks, all passing: `just check`, `just race`, `just cross` (six CGO-free targets) and `just vuln`.

Real disposable-instance lifecycles (instance bootstrapped over HTTP, resources seeded through the built CLI, with a stored API key and, for organization endpoints, a session token):

- Project create/get/update/archive/unarchive/reorder/delete; column create/list/update/reorder/delete; task create/get/update/`update-title`/`update-description`/`update-priority`/`update-status`/`update-due-date`/`update-assignee`/bulk-update/move/delete.
- Label create/update/attach/list-task/list-workspace/delete; comment create/update/delete; time-entry create/update/get/list; custom-field create/set-value/reorder/list-project-values/list-task-values/delete; activity create/`create-comment`/`update-comment`/`delete-comment`; notification create/list/mark-read/mark-all-read/clear-all.
- Binary: `user upload-avatar` then `user download-avatar` byte-identical (SHA-256); `task create-image-upload` issued a presigned URL, streamed the bytes to MinIO with no `Authorization` header, `task finalize-image-upload` recorded the asset, and `asset download` returned the bytes identically.
- `mcp register-oauth-client` (documented public) sent no credential.
- Organization (`KANEO_TOKEN` session with an active organization): check-slug, create, update, get-full, list, create-team, update-team, remove-team, create-role, list-roles, add-team-member, list-team-members, remove-team-member, set-active, set-active-team, invite-member, list-invitations and delete.
- Destructive gates: `task delete`, `org delete`/`remove-team`/`remove-member`/`remove-team-member`/`delete-role`, `notification clear-all`, `label delete`, `task-relation delete` and `task bulk-update` were refused before any request (exit 2, `invalid_arguments`) without `--yes`, and completed with it.
- Permissions failure: a second account's stored credential listing the first account's workspace returned HTTP 403, mapped to `authorization_failed`/exit 3.
- Invalid input: a missing `--body-file`, an invalid JSON body, a non-object body and a missing required flag all exit 2 before network access; a schema-violating body is forwarded and the server's 400 maps to `invalid_request`/exit 5.

Defects fixed during acceptance: `auth login --profile NAME` failed when the profile did not exist (the resolver rejected the missing explicit profile before `Save` could create it); login now creates the explicit profile first, with a regression test. `asset download` now attaches a stored credential when one is present, so a private asset downloads while a public asset still works anonymously; also covered by a regression test.

Upstream or environmental limitations observed and recorded (not CLI defects):

- `detachLabelFromTask` returns HTTP 400 `Label is not assigned to a task` even immediately after a successful attach that sets the label's `taskId`; an upstream contract decision is needed.
- `updateNotificationPreferences` and `upsertNotificationPreferenceWorkspaceRule` return HTTP 400 `Email notifications require an account email address` in this environment; the command and body mapping are correct.
- Organization admin operations require an active organization; with API-key-only auth the session has none (HTTP 400 `No active organization`), so they were exercised with a session token after an explicit active-organization bootstrap.
- `org list-user-invitations` returns HTTP 403 until the account email is verified; the disposable environment has no SMTP.

Pending evidence, not claimed: external-service integration operations (GitHub/Gitea/Slack/Discord/Mattermost/Telegram/generic webhook create/update/delete/verify/import) — only fixture protocol coverage is claimed, never a mocked live integration; organization invitation accept/reject/cancel with real invitation tokens; the two deferred browser-navigation endpoints; native OS keyring and Windows DACL evidence.

## Packet 4 evidence (2026-09-21)

Commit `cfcd4ff`. Reconciliation and local release acceptance. Publication remains disabled; `release/readiness.json` is unchanged.

Reconciliation (`just check` plus `TestInventoryCoverageIsHonest` in `internal/cli/coverage_test.go`): the pinned inventory has 162 operations and `api/commands.json` has exactly 162 entries with no duplicates, none missing and none extra. 160 are `implemented`; the only two `planned` entries are the deliberately deferred browser endpoints (`auth get-device-authorization-page`, `mcp start-authorization`). Every `implemented` mapping resolves to a registered cobra command that has a `RunE`; a `planned` mapping that is not one of the two documented deferrals fails the check, so there is no `implemented` row backed only by a stub. `go run ./internal/cmd/specinventory --check` reports the generated inventory current.

Local release plan (`go run ./internal/cmd/release plan`): `enabled: false`, current tag none, next tag `v0.1.0`, reason "first releasable change; policy fixes the first release at v0.1.0", five required-evidence items.

Snapshot (`just snapshot`, GoReleaser v2.18.0, snapshot mode, no uploads): six CGO-free archives — Linux and macOS `amd64`/`arm64` tarballs and Windows `amd64`/`arm64` zips — each containing `LICENSE` and the binary. The SHA-256 checksum manifest verifies all six (`OK`). Embedded build metadata is `version=0.0.0-SNAPSHOT-<commit>`, the exact commit SHA and build date; the extracted Linux amd64 binary is a statically linked ELF, `version` and `--version` return the same JSON object, and help lists 36 command groups.

Reproducible checks: `just check`; `just snapshot` then `(cd dist && sha256sum -c kaneo-cli_*_checksums.txt)`; `go run ./internal/cmd/release plan`; `go test ./internal/cli -run TestInventoryCoverageIsHonest`.

Readiness assessment against `release/readiness.json` — publication is **not** proposed:

- `public-api`: **not met.** Two documented operations (the browser endpoints) are unimplemented, and `getOrganizationRole` has an unresolved parameter gap. The full public API is not complete.
- `authentication`: **partially met.** API-key and device-code flows are locally accepted, but real device approval/denial and expired/revoked credential behavior remain pending.
- `profiles`: met locally (selection, persistence, isolation, migration, precedence, concurrency); native OS keyring behavior is unverified.
- `safety`: met locally (`--yes` before network, redaction, non-interactive behavior).
- `local-release-acceptance`: snapshot, checksums, embedded metadata and plan done; installation smoke is Linux-only.

Blockers before activation, recorded rather than waived: implement or obtain a decision for the two browser-navigation endpoints; resolve the `getOrganizationRole` missing-parameter discrepancy; resolve the `detachLabelFromTask` 400; obtain real device approval/denial and expired/revoked-credential evidence; obtain native macOS/Windows keyring and DACL execution; and complete real external-service integration acceptance.

## Packet follow-up evidence (2026-09-21)

This follow-up supersedes the role, label-detachment and real device approval/denial blockers above. Environment: Linux amd64; unchanged API SHA-256 `a5f29855e3f25c703bf665fd17703cc79b672bd4e24f9f5fbd8f0c1b8e44db9e`, disposable image `ghcr.io/usekaneo/kaneo@sha256:a85a23996c36166cfcebcb4ee161b40cc62c7b924faf06353684a84d5e84162c` (server 2.25.0).

- `org get-role --role-id ID` and `--role-name NAME`, each with `--organization-id ID`, returned the created role through the built CLI. Parameters are an explicitly approved source-backed supplement in `api/provenance.json`, not a modification of the pinned OpenAPI. Exactly one nonempty role selector is required; organization ID is optional. Missing, conflicting and empty selectors produce usage exit 2.
- `label attach-to-task` returns a new attached-label ID, different from the original workspace-label ID. Passing the **returned ID** to `label detach-from-task --label-id ID --yes` succeeds; `label list-task` then returns an empty list. The earlier 400 was an acceptance-input error, not an upstream defect.
- Real browser approval issued a bearer token successfully used for API operations. Real browser denial produced HTTP 400 `access_denied` from device polling. Credentials remained in memory, not acceptance logs.
- Successful task-asset upload/finalize/download was already recorded in packet 3 above; packet 2's pending asset-download note is superseded.
- Both browser-navigation mappings are now implemented as explicit URL handoffs: `auth get-device-authorization-page` forces `ui=1`; `mcp start-authorization` encodes the documented OAuth query. Built-binary smoke checks emitted the expected URLs and stderr guidance without making HTTP requests or claiming completed authorization. All 162 mappings are now marked implemented; this is coverage, not full compatibility.
- Follow-up verification passed: `just check`, `just cross`, `just race`, and `just vuln` (no vulnerabilities found). The six cross-builds are build evidence only, not native runtime acceptance.
- Hosted [follow-up CI](https://github.com/foae/kaneo-cli/actions/runs/35576268169) passed shared checks on Linux, macOS and Windows, including the Windows owner-only DACL and lock-reopen regression. Race, vulnerability and cross-build jobs also passed on that pre-policy-change revision. This does not establish native keyring integration.
- The subsequent CI cost-policy change removes PR triggers and hosted race/vulnerability jobs; main pushes retain the three-OS shared checks, cross-build and disabled release gate. Reviewed dependency upgrades target Node.js 24. The integrated revision passed local `actionlint`, workflow trigger/dependency assertions, `just check` and `just cross` on Linux amd64; no additional hosted run was requested.

### Six-commit review

The Pro panel reviewed `9bdf000..9c048d6` before follow-up edits (review run `20260921-072012`). All six non-self seats completed: Fable, GLM, Opus 5, Kimi, Sol and Terra. Raw reviewer reports remain local review artifacts; the reconciled outcomes are recorded below.

Confirmed findings addressed in the follow-up include upload path escaping, bounded file/presign reads, storage redirect refusal and timeout classification, secret-bearing response output, mutation redirect isolation, partial-failure exit status, failed-login profile atomicity, binary-output preflight, query minimum lengths, complete JSON validation, inventory parameter checks and stale documentation/diagnostics. Regression checks cover the security and behavioral paths.

Review dispositions not requiring a code change:

- Optional-auth downloads fail closed when the configured credential backend errors; silently falling back to anonymous access is not the contract.
- Presigned storage headers are server-defined signing requirements, not the CLI's Kaneo bearer token; filtering `Authorization` without an upstream contract could invalidate signatures.
- Search `limit` is a string in the pinned schema, with no numeric constraint; the requested numeric validation would invent a restriction.
- OS failures opening body files retain process-error semantics; malformed readable inputs remain usage errors.
- A failed storage upload emits no success key. Empty group help was hypothetical: no missing registered group description was identified.
- The panel's required-parameter test gap was real, including Kimi's multi-anchor entry omitted by automated parsing; the manual reconciliation includes it.

The panel agreed most strongly on unsafe upload path construction. Independently raised findings were checked against the implementation rather than accepted by vote; fixes after the snapshot do not make an original finding invalid.

These observations do not establish expired/revoked credential handling, native macOS/Windows credential storage or external-service integration acceptance. Publication remains disabled.

## v1.0.0 acceptance (2026-09-21)

The maintainer approved releasing with external-provider live acceptance explicitly deferred. GitHub, Gitea, Slack, Discord, Mattermost and Telegram integrations remain implemented and fixture-tested, **not live-provider verified**. This supersedes the earlier requirement to provision external services before publication; it does not reinterpret fixture evidence as live evidence.

Credential lifecycle exercised through the built Linux amd64 CLI against a fresh disposable server using the unchanged API hash and Kaneo 2.25.0 image digest recorded above:

- A synthetic user created an API key; `org list` returned `[]` with exit 0.
- Deleting that key through the real API made the same command return exit 3, empty stdout and structured HTTP 401 `authentication_failed`.
- A synthetic invalid key returned exit 3, empty stdout and HTTP 403 `authorization_failed`.
- A second working key was expired by setting only its disposable database row's `expires_at` to one minute before the server time. The same command returned exit 3, empty stdout and HTTP 401 `authentication_failed` (`API Key has expired`). This exercises real server expiry enforcement, not waiting through a production expiry interval.
- Synthetic credentials stayed in process memory/environment; no credential values were included in evidence.

Release preparation checks:

- Stable release planner returned `v1.0.0` with publication still disabled. Regression checks cover real Git NUL/newline separators, subject-only release signals and breaking-change precedence.
- `just check`, `actionlint` and `just snapshot` passed on Linux amd64. All six snapshot archives passed SHA-256 and LICENSE/executable-content checks; the extracted Linux amd64 binary returned linked version, commit and build date. Cross-built archives are not native runtime evidence.
- The native keyring lifecycle test calls the OS backend directly, without plaintext fallback. Run `KANEO_NATIVE_KEYRING_TEST=1 go test ./internal/auth -run TestNativeKeyring -count=1 -v` in an unlocked credential-store session (PowerShell: set `$env:KANEO_NATIVE_KEYRING_TEST='1'` first). This workstation has no Secret Service provider, so its native run failed as expected; no local native-keyring success is claimed.
- Hosted native credential acceptance passed on revision `ad42403454ac2fd15cef64468cba1af0c85d4128`: [CI run 35590087700](https://github.com/foae/kaneo-cli/actions/runs/35590087700). All three Verify jobs passed, including the real store/get/delete lifecycle on Linux Secret Service (isolated D-Bus session and unlocked GNOME Keyring), macOS Keychain and Windows Credential Manager. These are native hosted runner checks, not claims of ARM runtime execution.

Release activation links these observations from `release/readiness.json`. External-provider live acceptance remains explicitly deferred. GitHub immutable releases are enabled.

Publication and installation acceptance:
- [CI run 35590594298](https://github.com/foae/kaneo-cli/actions/runs/35590594298) passed every job, publishing [v1.0.0](https://github.com/foae/kaneo-cli/releases/tag/v1.0.0) at `93bfbe2b7085986aad3a625c473af4f9e7d39f9e` and attesting the six archives plus checksum manifest. GitHub reports `immutable: true`, `draft: false`, `prerelease: false`.
- Downloaded all seven assets using `gh release download`; `sha256sum --check kaneo-cli_1.0.0_checksums.txt` passed for all six archives. `gh attestation verify kaneo-cli_1.0.0_checksums.txt --repo foae/kaneo-cli --signer-workflow foae/kaneo-cli/.github/workflows/release.yml --source-digest 93bfbe2b7085986aad3a625c473af4f9e7d39f9e --deny-self-hosted-runners` succeeded.
- Generated [Formula PR #9](https://github.com/foae/kaneo-cli/pull/9) matched a fresh rendering from the published checksums byte-for-byte. `just check` passed before merge.
- On Linux amd64 with Homebrew 7.0.4, `brew tap foae/kaneo-cli https://github.com/foae/kaneo-cli`, `brew install foae/kaneo-cli/kaneo-cli` and `brew test foae/kaneo-cli/kaneo-cli` passed. The installed `kaneo-cli version` returned version `1.0.0`, commit `93bfbe2b7085986aad3a625c473af4f9e7d39f9e` and date `2026-09-21T10:52:20Z`; `kaneo-cli instance get-status` returned `{"hasUsers":true,"hasAdmin":false}`. No native macOS Homebrew or ARM installation claim is made.

## Distribution acceptance (2026-09-21)

Local Linux amd64 checks for the distribution-introduction change:

- GoReleaser snapshot produced six archives, four Linux deb/rpm packages, and their checksums. Debian and RPM installation/removal smoke checks used disposable native-amd64 containers; package binaries returned version metadata and successfully called the public instance-status endpoint.
- The rebuilt Debian package also passed installation, HTTPS instance status and removal in `debian:bookworm-slim@sha256:3783cc01769c7b2b1b83a5c5ad96c815348e28ed7da68e2e3687004faa906251`, including the Debian copyright path.
- The digest-pinned distroless container executed the CLI on amd64, including HTTPS instance status. The arm64 container built successfully; this is not native arm64 execution. The base digest is recorded in `Dockerfile`.
- `just check`, `actionlint`, and ShellCheck passed. Registry regression checks reject existing tags, auth/registry failures and digest mismatches, and enforce pull-only verification scope.
- Recovery smoke fixtures exercised fresh, complete, legacy and CRLF notes; missing local commits, missing/duplicate digest records, registry failure and corrupt packages failed closed. The actual workflow version-step script rejected an already-tagged partial release and accepted a complete one. Ambiguous binary candidates failed before container construction.
- Skill create/status/priority/comment examples ran against a synthetic local HTTP fixture. Read-back preserved unrelated task fields. Claude plugin metadata passed local validation. This is not live-server skill acceptance or hosted marketplace installation evidence.
- A six-seat Pro review covered the working distribution change. Release metadata remains editable under GitHub's immutable-release contract; assets and tags do not. GHCR version tags remain a documented append-only policy, not atomic registry-enforced immutability.
- Review scores (valid/invalid findings): Fable 4/5 (8/2), GLM 4/5 (6/0), Kimi 3/5 (5/2), Opus 4/5 (12/3), Sol 5/5 (5/0), Terra 2/5 (1/1). All six completed. Findings were validated against code and platform documentation; shared roots and duplicate version findings are not independent defects. Review telemetry: `778`.

The CLI/API behavior and pinned API baseline are unchanged. Hosted package publication, anonymous image pulls, published-artifact attestations and the new release's installation checks must be recorded after publication; local builds do not establish them.

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
