---
name: update-cli
description: Maintainer workflow for kaneo-cli. Refresh the pinned Kaneo OpenAPI baseline to the latest upstream release tag, find what was added, changed or removed, adapt the CLI, the consumer skill and tests, verify against a disposable instance, then open, review and merge the PR and finish the release including the Homebrew Formula bump. EXPLICIT INVOCATION ONLY. Run it when the user invokes /update-cli or names this skill. It pushes, merges and publishes a release.
license: MIT
disable-model-invocation: true
metadata:
  version: "1.0.0"
---

# Update kaneo-cli to the latest Kaneo API

Maintainer-only procedure for this repository. It is not part of the consumer skill in `skills/kaneo-cli/` and is not distributed with it. `metadata.version` above versions this procedure itself: bump it (semver) in the same PR as any change to this file.

Invoking this skill authorizes, for this one refresh: creating a branch, pushing it, opening the PR, merging it, the release that CI then publishes, and merging the bot's `chore(homebrew)` Formula PR. It authorizes nothing else, including repository settings, tag edits and manual releases.

Read `AGENTS.md`, then [API baseline](../../../docs/api/README.md) (its "Explicit refresh procedure" is authoritative), [Releases](../../../docs/releases.md) and [Verification](../../../docs/verification.md). This skill orders those steps and adds the decisions below. Where they disagree, the repository docs win; report the drift and fix this file in the same PR.

## Autonomy

Run end to end without asking, except to **stop and ask** on:

- **Breaking upstream changes:** an operation removed, a method/path renamed, a required field added to an existing request, a response shape or security requirement changed so existing CLI behavior or output breaks, or a change that needs a `!`/`BREAKING CHANGE` release.
- **Contract conflicts:** the official spec, upstream source, live instance behavior and docs disagree materially, or the live docs spec matches no release tag.
- **Findings you cannot fix cleanly:** a failing check, live-acceptance failure or review finding whose fix is risky, large or a judgment call.

Everything else is yours to decide, including **naming new commands**. Follow the existing conventions in `api/commands.json` (singular groups, `org` for Organization Management, verb-first kebab-case actions such as `get-public-description`, `find-description-matches`), then upstream Kaneo's own route/tag naming. Never rename an existing command. When you stop, bring the evidence: spec excerpts, file:line, the exact decision needed.

## 1. Decide whether there is anything to do

1. `git switch main && git pull --ff-only`; the tree must be clean.
2. Latest upstream release: `gh api repos/usekaneo/kaneo/releases/latest --jq .tag_name`. Pinned baseline: `api/provenance.json` (`openapi.upstream_source_commit`, `openapi.sha256`) and the tag named in `docs/api/README.md`.
3. Download `openapi.official_url` and the tag's `apps/docs/openapi.json` (raw URL at the tag's commit) to a temp dir. Check HTTP success and JSON content, never an HTML error page. Compare SHA-256:
   - tag spec equals pinned SHA → **already current**. Also check the authentication guide (`supplements.device_authorization.snapshot_url`) against its pinned SHA. If both match, report "already current at vX" and stop: no branch, no PR.
   - official spec differs from the tag spec → contract conflict (live docs ahead of or behind the release): stop and ask.
   - otherwise continue with that tag as the target.
4. Create branch `feat/api-baseline-vX.Y.Z`.

## 2. Semantic diff

Diff the pinned `api/openapi.json` against the target by `operationId` and method/path, not text. List:

- added / removed operations;
- per changed operation: parameters (required, pattern, min/max length, enum, numeric bounds), request-body schemas and media types, response codes and shapes (new status codes such as 202 continuation), security overrides (public vs. authenticated), descriptions that state behavior (pagination, limits, async work);
- changed shared `components/schemas` and which operations reference them.

Confirm each behavioral claim in the upstream source at the target commit (`apps/api/src/...`) when the schema is incomplete, as the 2.27.0 refresh did for pagination and continuation. Classify each change as additive, tightening, or breaking; breaking stops the run.

## 3. Update baseline and CLI

Follow the refresh procedure in `docs/api/README.md` steps 3–7:

- Replace `api/openapi.json` (and `api/authentication.md` if it changed) with the reviewed bytes; update `api/provenance.json` (SHA-256, `retrieved_at`, source commit/URLs, `operation_count_observed`, verified `main` commit) and the tag and date in `docs/api/README.md`.
- Map every new operation in `api/commands.json`, then `go run ./internal/cmd/specinventory` and inspect the generated diff.
- Implement every new operation in the existing data-driven style (read/write specs, browser/binary contracts per `AGENTS.md`); none may stay `planned` without an explicit reason in the docs. Apply tightened constraints before any request. Handle new response semantics (pagination, continuation, partial success) without silent retries.
- Move the Kaneo image digest in `integration/compose.yaml` to the target tag's image and update its audit comment.
- Update the consumer skill `skills/kaneo-cli/SKILL.md` wherever commands, flags, recipes or behavior changed, plus `docs/cli-contract.md` and any other affected docs.
- Update the operation count and evidence links in `release/readiness.json` (`public-api`).
- Record discrepancies (source URL, revision, endpoint, observed behavior, impact) per `AGENTS.md`; upstream defects left unfixed go in `docs/reported_bugs/`.

Delegate mechanical implementation to worker agents with disjoint files. Keep diff classification, naming and conflict judgments on the coordinator.

## 4. Tests

Tests defend behavior, not source text or mock forwarding. For every new or changed operation add fixture tests covering: request method/path/query/body, pre-request validation of tightened constraints (rejects with nothing sent), public vs. authenticated credential handling, stdout/stderr/exit code for success and each new error/continuation status, and pagination edges when paging changed. The inventory cross-check must keep passing without loosening.

## 5. Verify

All must pass. Record exactly what ran and on which platform.

1. `just check`, `just race`, `just vuln`, `just cross`.
2. **Disposable live instance (mandatory).** Use `integration/compose.yaml` exactly as `docs/verification.md` "Disposable real Kaneo" describes: remove leftover stacks first, prove the fresh stack with `docker compose ps`, bootstrap over HTTP, run the built binary against every new and changed operation that the instance can exercise, then `down --volumes --remove-orphans`. List anything that could not be exercised live (e.g. needs a GitHub App or SMTP) as fixture-only. Never record generated credentials.
3. Add a dated `## API baseline refresh to Kaneo X.Y.Z (YYYY-MM-DD)` section to `docs/verification.md` modeled on the 2.27.0 one: API SHA-256, image digest, observed server version, scenarios and outcomes, fixture-only items.
4. **Review (mandatory).** Run the `reviewer` agent on the full branch diff against `main`. Fix every confirmed finding, or stop if one cannot be fixed cleanly, then rerun step 1.

## 6. Version stamp, PR, merge

1. Commit with the Conventional Commit type the change warrants: `feat:` for new operations or flags, `fix:` for corrections only, and never a breaking release without asking (see Autonomy). Squash-merge uses the PR title, so the PR title must carry the same type.
2. `just stamp-skill` sets `metadata.version` in `skills/kaneo-cli/SKILL.md` to the planned release. Commit it. `go run ./internal/cmd/release plan` must report `release: true` and the intended next tag, and pass the stamp gate; `just check` again.
3. Push, and open the PR modeled on #28: summary of new/changed commands and flags, behavior changes, verification (checks, live acceptance, fixture-only, review), and any recorded discrepancies. End with the repository's attribution line.
4. CI runs only after merge to `main`, not on PRs. Merge with squash once the local checks and review are green, then pull `main`.

## 7. Release and Homebrew. A release is not done until brew serves it

1. Watch the `main` CI run (`gh run watch`; use `ci-triage` on failure). It tags `vX.Y.Z`, publishes archives, packages and the container image, then opens `chore(homebrew): update kaneo-cli vX.Y.Z`. A failure after publication follows the recovery section of `docs/releases.md`. Never re-tag, clobber or rerun blindly.
2. Find the Formula PR: `gh pr list --search "chore(homebrew) vX.Y.Z" --state open`. Verify every `sha256` in its `Formula/kaneo-cli.rb` against the release's `kaneo-cli_X.Y.Z_checksums.txt` by exact filename. Compare parsed values in a script, not by eyeballing shell output. All four must match; a mismatch stops the run.
3. Merge the Formula PR, then `brew update` and `brew info foae/kaneo-cli/kaneo-cli` must report the new stable version. If `kaneo-cli` is brew-installed here, `brew upgrade kaneo-cli` and confirm `kaneo-cli version`.

## 8. Report

Report: upstream tag and SHA, operations before → after, and added/changed/removed ones; new commands and flags; behavior changes users will notice; what was verified live vs. fixture-only; review findings and their fixes; PR numbers; release tag; Formula checksum result; `brew info` version. List anything skipped or deferred explicitly.
