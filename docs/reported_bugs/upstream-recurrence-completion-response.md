# Completing a recurring task returns stale recurrence metadata

Target: `usekaneo/kaneo`. No upstream issue has been filed.

## Environment and evidence

Live observation on 2026-11-03 against Kaneo v2.49.0, source `e0ebc61748654f4e33e713dcb441be726ec1cfc4`, disposable Linux amd64 image `ghcr.io/usekaneo/kaneo@sha256:cd1826b1aba3ce736015cadf6f6c975d923aac00f4b6aaa7aedb29bc722dae49`.

1. Create a task in an open column with due date `2026-11-03T12:00:00.000Z` and recurrence `{"frequency":"daily","interval":1,"timeZone":"UTC"}`.
2. Complete it with `PUT /task/status/{id}`, body `{"status":"done"}`.
3. The HTTP 200 task has `status: "done"` but still contains the recurrence rule.
4. A subsequent `GET /task/tasks/{projectId}` shows the completed task with `recurrence: null` and a successor in the first open column with the rule and due date `2026-11-04T12:00:00.000Z`.

The CLI exercised these paths through `task create`, `task update-status` and `task list`; no credentials or private data are included here. The successor and persisted state were correct. Consumers treating the mutation response as current state can incorrectly show the completed task as recurring.

## Source and suggested fix

[`update-task-status.ts` at the pinned revision](https://github.com/usekaneo/kaneo/blob/e0ebc61748654f4e33e713dcb441be726ec1cfc4/apps/api/src/task/controllers/update-task-status.ts) captures `updatedTask`, publishes the mutation, then returns that captured object. Review whether completion should await recurrence processing and return refreshed metadata, or explicitly document that the response precedes recurrence side effects.

The CLI preserves the server response without adding a hidden follow-up request. Callers needing post-completion recurrence state should re-read the task or board.
