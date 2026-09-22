# Reported bugs

This folder registers defects found in, or affecting, this project that are **not** fixed here. Each file is a self-contained report: environment, evidence, source references and a suggested fix, written so it can be pasted into an issue tracker without further editing. Nothing in this folder has been fixed in this repository, and no upstream issue or pull request has been opened for any of it; the maintainer decides later whether to file anything.

Upstream reports target [usekaneo/kaneo](https://github.com/usekaneo/kaneo). Local reports target this repository, `foae/kaneo-cli`, and describe behavior of the CLI itself rather than of the server.

Claims are marked with their evidence class: live observation against a disposable instance, or a read of pinned upstream source. Where a report was not reproduced live, it says so in its own text.

## Upstream (usekaneo/kaneo)

- [upstream-bulk-update-status-partial-write.md](upstream-bulk-update-status-partial-write.md) — `PATCH /task/bulk` with `operation: "updateStatus"` commits part of a batch before returning HTTP 400 for a later project.
- [upstream-unbucketed-task-status.md](upstream-unbucketed-task-status.md) — GitHub/Gitea issue import can write a task status matching no column slug and no reserved value, leaving the task in no board bucket.
- [upstream-openapi-reserved-status-documentation.md](upstream-openapi-reserved-status-documentation.md) — the pinned OpenAPI never documents that `planned` and `archived` are reserved task statuses.

## Local (foae/kaneo-cli)

- [local-login-rebinds-profile-api-url.md](local-login-rebinds-profile-api-url.md) — `auth login` rebinds a profile's stored API URL to an ambient `KANEO_API_URL`, filing the credential under the wrong URL key.
