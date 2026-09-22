# OpenAPI never documents the reserved task statuses `planned` and `archived`

Upstream documentation defect, `usekaneo/kaneo`. Not fixed in `foae/kaneo-cli`.

## Environment

- kaneo-cli 1.5.0, commit `565dbba70322b355e44645f65c4494e76e478b67`
- Pinned API snapshot SHA-256 `a5f29855e3f25c703bf665fd17703cc79b672bd4e24f9f5fbd8f0c1b8e44db9e`
- Disposable Kaneo image `ghcr.io/usekaneo/kaneo@sha256:a85a23996c36166cfcebcb4ee161b40cc62c7b924faf06353684a84d5e84162c`, brought up from `integration/compose.yaml` as Compose project `kaneo-cli-acceptance` on `http://localhost:15173/api`. MinIO was not started; no upload scenario was exercised.
- Upstream source read at `usekaneo/kaneo` commit `ca70c72c4ed5585d0e4ffc3baf692def6f44d185`
- Observed 2026-09-22, Linux amd64

## Defect

`planned` and `archived` are reserved task statuses accepted alongside the project's column slugs, and they move a task into the Backlog or the Archive rather than into a column. The pinned OpenAPI never says so. Four concrete points, each with the current text quoted from `api/openapi.json`:

1. `PUT /task/status/{id}` description is `"Move a task to another column in the same project."` — incomplete: the same endpoint also moves a task out of the columns entirely, into the Backlog or the Archive.
2. The request schema for that endpoint is `{"status": {"type": "string"}}`, with no enum and no description, so nothing hints that two values are reserved.
3. `components.schemas.Board.plannedTasks` and `Board.archivedTasks` have no description at all.
4. The board GET description (`"Get a project's board: its columns, each with the tasks in it, plus the archived and planned buckets."`) is the only prose in the entire spec that admits the buckets exist, and it is on a read path.

## Server-side truth

Quoted from `apps/api/src/task/validate-task-fields.ts:144-177`: `export const VIRTUAL_STATUSES = ["planned", "archived"] as const;`, appended to the project's column slugs by `getValidTaskStatuses` and enforced by `assertValidTaskStatus`.

## Observation

The server already self-documents the valid values in its error text, while the spec does not:

```
$ curl -s -w 'http=%{http_code} ct=%{content_type}\n' -X PUT http://localhost:15173/api/task/status/<TASK> \
    -H 'Authorization: Bearer [REDACTED]' -H 'Content-Type: application/json' \
    -d '{"status":"totally-made-up"}'
http=400 ct=text/plain;charset=UTF-8
Invalid status "totally-made-up". Valid statuses for this project: to-do, in-progress, in-review, done, planned, archived
```

## Not a defect

`components.schemas.ProjectListItem` documents `"Always empty."` on `archivedTasks`, `plannedTasks` and `columns`. That is accurate: a `GET` of the project list really does return all three empty; only the board endpoint populates them. It is recorded here so it is not re-reported.

## Impact

A client generated from, or written against, the spec cannot learn that two status values exist with different semantics from every other value. Discovering them requires provoking a 400 or reading the server source.

## Suggested fix

Document `planned` and `archived` on the `PUT /task/status/{id}` request schema and in its description, and describe the `Board.plannedTasks` / `Board.archivedTasks` properties.
