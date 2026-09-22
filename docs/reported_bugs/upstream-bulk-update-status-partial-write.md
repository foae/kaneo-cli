# Bulk status update writes part of the batch before failing with HTTP 400

Upstream defect, `usekaneo/kaneo`. Not fixed in `foae/kaneo-cli`.

## Environment

- kaneo-cli 1.5.0, commit `565dbba70322b355e44645f65c4494e76e478b67`
- Pinned API snapshot SHA-256 `a5f29855e3f25c703bf665fd17703cc79b672bd4e24f9f5fbd8f0c1b8e44db9e`
- Disposable Kaneo image `ghcr.io/usekaneo/kaneo@sha256:a85a23996c36166cfcebcb4ee161b40cc62c7b924faf06353684a84d5e84162c`, brought up from `integration/compose.yaml` as Compose project `kaneo-cli-acceptance` on `http://localhost:15173/api`. MinIO was not started; no upload scenario was exercised.
- Upstream source read at `usekaneo/kaneo` commit `ca70c72c4ed5585d0e4ffc3baf692def6f44d185`
- Observed 2026-09-22, Linux amd64

## Defect

`PATCH /task/bulk` with `operation: "updateStatus"` validates and writes per project inside a single loop. A status that is valid in one project and invalid in another returns HTTP 400 *after* the write for the valid project has already been committed. The caller sees a failure and has no way to tell that part of the batch was applied.

Source: `apps/api/src/task/controllers/bulk-update-tasks.ts:96-122` — `assertValidTaskStatus(value, projectId)` and the `db.update(...)` both sit inside `for (const projectId of projectIds)`. The validation of a later project therefore runs after an earlier project's update.

## Reproduction

Two projects in one workspace. Project ACC has an extra column `blocked`; project SEC does not. One task in each: TA in ACC, TB in SEC.

```
$ curl -s -w 'http=%{http_code} ct=%{content_type}\n' -X PATCH http://localhost:15173/api/task/bulk \
    -H 'Authorization: Bearer [REDACTED]' -H 'Content-Type: application/json' \
    -d '{"operation":"updateStatus","taskIds":["<TA>","<TB>"],"value":"blocked"}'
http=400 ct=text/plain;charset=UTF-8
Invalid status "blocked". Valid statuses for this project: to-do, in-progress, in-review, done, planned, archived

$ kaneo-cli task get --id <TA> | jq -c '{status}'
{"status":"blocked"}
$ kaneo-cli task get --id <TB> | jq -c '{status}'
{"status":"to-do"}
```

TA was updated; TB was not; the request reported failure.

## Impact

A 400 from a bulk operation is not safe to treat as "nothing happened". A client that retries after the error, or that reports the batch as unapplied, will be wrong about the tasks in the project that succeeded. There is no partial-success information in the response body.

## Suggested fix

Validate every distinct project's status before performing any write, or wrap the whole operation in one transaction so a later validation failure rolls the earlier writes back.
