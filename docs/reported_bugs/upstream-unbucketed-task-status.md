# Issue import can write a task status that belongs to no board bucket

Upstream defect, `usekaneo/kaneo`. Not fixed in `foae/kaneo-cli`.

## Environment

- kaneo-cli 1.5.0, commit `565dbba70322b355e44645f65c4494e76e478b67`
- Pinned API snapshot SHA-256 `a5f29855e3f25c703bf665fd17703cc79b672bd4e24f9f5fbd8f0c1b8e44db9e`
- Disposable Kaneo image `ghcr.io/usekaneo/kaneo@sha256:a85a23996c36166cfcebcb4ee161b40cc62c7b924faf06353684a84d5e84162c`, brought up from `integration/compose.yaml` as Compose project `kaneo-cli-acceptance` on `http://localhost:15173/api`. MinIO was not started; no upload scenario was exercised.
- Upstream source read at `usekaneo/kaneo` commit `ca70c72c4ed5585d0e4ffc3baf692def6f44d185`
- Observed 2026-09-22, Linux amd64

## Not reproduced live

This defect was **not** reproduced against a running server. The disposable instance has no GitHub integration configured, so the import path could not be exercised. Everything below is a source read at the pinned upstream commit, except the CLI resolution behavior, which is a read of this repository's source.

## Defect

GitHub and Gitea issue import can write a task status that matches no column slug of the destination project and is neither of the reserved values `planned` and `archived`. The resulting task appears in no bucket of the board payload, and is therefore invisible in the UI and unreachable through this CLI's display-key resolution.

Source [verified by source]: `apps/api/src/github-integration/controllers/import-issues.ts:211` and `:241-249` take `extractIssueStatus(issue.labels)` and insert it as `status: status || "to-do"` with no validation. `apps/api/src/plugins/github/utils/extract-priority.ts:35-49` validates only the slug's *shape* (`/^[a-z0-9]+(?:-[a-z0-9]+)*$/`), never against the project's columns. The same pattern exists in `apps/api/src/gitea-integration/controllers/import-gitea-issues.ts:260`.

Consequence, quoted from `apps/api/src/task/controllers/get-tasks.ts:218-267`: columns are filtered by `task.status === column.slug`, and the two reserved buckets by `=== "archived"` / `=== "planned"`. A status matching none of those is returned in no bucket.

The webhook handlers under `plugins/github/webhooks/` and `plugins/gitea/webhooks/` appear to share the pattern but were not traced line by line [medium confidence].

## Effect on this CLI

`internal/cli/resolve.go:172` builds each page's candidate set from `archivedTasks + plannedTasks + columns[].tasks`. A task with an unbucketed status is in none of those, so `kaneo-cli task get --key` returns a usage error (exit 2, "no match") for a task that exists.

## Impact

An imported task can be silently unreachable: present in the database, absent from the board payload, invisible in the UI, and not resolvable by display key from any client that reads the board.

## Suggested fix

Run `assertValidTaskStatus` (or `coerceStatus`) on the imported status, as `apps/api/src/task/controllers/import-tasks.ts` already does.
