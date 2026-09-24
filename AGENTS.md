# Agent entry point

This public repository is an **unofficial Kaneo CLI**. Read progressively: this file, the task-specific document below, then only the relevant source and upstream evidence.

## Non-negotiable boundaries

- Current executable coverage includes profiles, authentication, all 60 non-browser reads, 103 JSON mutations, avatar/presigned file transfers and the two browser-navigation URL handoffs: all 167 pinned operations. Consult the operation inventory and verification evidence for implemented coverage and outstanding blockers; registration alone is not acceptance. No fake successful API commands, no TODO handlers, no placeholder packages.
- The pinned official API baseline plus documented device authorization defines the implementation target. Do not invent fields, pagination, authentication requirements, or server behavior.
- Every pinned operation has exactly one canonical command mapping. Mark it implemented only with observable evidence. Browser and binary operations need deliberate contracts, not fabricated JSON wrappers.
- Preserve LICENSE and upstream notices. No public SDK promise: application packages belong in `internal/`.
- Never print credentials, cookies, authorization headers, device tokens, presigned URLs, or unredacted HTTP debug dumps. Tests use synthetic credentials.
- No implicit prompts. Destructive operations require `--yes`. Never silently retry mutations or send credentials across origins.
- Publishing remains disabled until the readiness requirements in the release documentation are met. Never publish, tag, push, or change repository settings unless authorized.
- A release is one deliverable: the binary release and its Homebrew Formula bump ship together. Publishing archives is not a finished release while the generated `chore(homebrew)` pull request is still open, because `brew` keeps resolving the previous version. Verify the Formula's checksums against the published manifest, merge it, and confirm the tap reports the new version before reporting a release done.
- Complete one dependency-ordered work packet at a time. Keep API response JSON on stdout and diagnostics on stderr; preserve omitted/null/false/zero distinctions.

## Task routing

| Task | Read next |
| --- | --- |
| Layout, dependencies, HTTP client | [Architecture](docs/architecture.md) |
| Commands, flags, output, errors | [CLI contract](docs/cli-contract.md) |
| API endpoint or spec refresh | [API baseline](docs/api/README.md), then that operation in [inventory](docs/api/operations.md) |
| Login, config, keyring | [Authentication](docs/authentication.md) |
| Implementation sequence | [Work packets](docs/implementation.md) |
| Checks, fixtures, real instance | [Verification](docs/verification.md) |
| Tags, distribution, recovery | [Releases](docs/releases.md) |
| Known defects left unfixed here | [Reported bugs](docs/reported_bugs/README.md) |
| Refresh to a newer Kaneo API and release | [update-cli skill](.agents/skills/update-cli/SKILL.md) |

## Working rules

Inspect existing patterns first. Prefer Go standard library and concrete types; add a dependency only for a demonstrated need. Keep command construction separate from process exit, and inject IO and HTTP dependencies for tests. Propagate context cancellation. No global mutable command state.

Before changing an endpoint, read its complete operation, referenced schemas, parameters, security overrides and request/response media types. Compare official prose and pinned upstream source when the schema is incomplete. Record discrepancies with source URL, revision, endpoint, observed behavior and impact; escalate materially conflicting contracts rather than silently choosing.

Run `just` to discover developer tasks, `just test` for Go tests only, and `just check`, `just race`, `just vuln`, and `just cross` for routine shared verification before handoff. Hooks are optional and never auto-installed. Keep recipes thin: orchestration and tool pins belong in the Go developer tooling, not duplicated in the justfile. Direct Go alternatives remain available in [verification](docs/verification.md). Run focused behavior checks during development. Tests must defend behavior, not source text or mock forwarding. Report exactly what ran, platform, API baseline and container digest; never call a cross-build runtime validation.

Run `just lint` for focused static feedback; it is also part of `check`. The pinned golangci-lint policy in `.golangci.yml` applies to production and test code. Fix causes, not symptoms; suppress only demonstrated false positives with a specific linter and concrete explanation. See [verification](docs/verification.md#lint-policy).

For delegated work, define disjoint file ownership, contracts, exact acceptance criteria and escalation rules. Require raw evidence; surprises outside the assignment must be reported rather than improvised around.
