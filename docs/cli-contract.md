# CLI contract

This is the binding contract for the command surface. Help, version, `profile` management, `auth login`/`logout`/`get-session`, all GET read operations, and all JSON mutation operations (including the presigned task-image upload and the base64 avatar upload) are implemented. Two browser-navigation endpoints, `auth get-device-authorization-page` and `mcp start-authorization`, are deliberately deferred pending a browser interaction contract.

## Names and input

`kaneo-cli <group> <verb[-object]>`: `instance get-status`, `auth get-session`, `org accept-invitation`, `project list`, `task update-status`. Omit the object when the group already identifies it. Use `profile` for local configuration; do not collide with the server's `config` operations. The reviewed machine-readable inventory is authoritative for individual mappings; aliases are not additional coverage.

Use `--profile NAME`, `--api-url URL`, `--timeout DURATION` and `--yes` consistently once implemented. The URL is the full API base URL, including `/api`; do not append it twice. Explicitly configured HTTP is allowed for self-hosted instances, with a stderr warning before transmitting credentials; never downgrade HTTPS automatically. Validate malformed URLs before network access.

Document operation-specific named flags for scalar path/query parameters. Complex bodies accept `--body-file PATH` or `--body-file -` for stdin, are validated before any network access and are passed through unchanged so unknown schema-permitted fields and numeric precision survive. Two file operations construct their request from a local file instead: `task create-image-upload` requests a presigned URL, streams the bytes to storage with a client that never carries the API credential, and prints the API response; `user upload-avatar` sends a bounded base64 body. A flag is present only when explicitly supplied: omitted, JSON null, false, zero and empty string are distinct. Reject conflicting body/field inputs rather than silently picking one. Never accept secrets in positional arguments or print shell commands containing them.

No implicit prompts, even on a TTY. Device login is an explicit interactive action; describe its behavior on a non-TTY and provide a no-browser path. Destructive actions (deletes, revocations, removals, resets and equivalent irreversible state changes) require `--yes` before making any request, independent of terminal type. This includes local credential removal via `auth logout` and `profile delete`. Bulk mutation must report partial failure rather than successful exit for incomplete work.

## Output

- Successful JSON: API response directly on stdout, no envelope; emit a trailing newline. JSON null remains `null`.
- No-content responses: empty stdout, not `{}` or `null`.
- Preserve API pagination shape. No invented `--all` behavior or automatic aggregation; follow actual documented pagination parameters and links.
- Binary downloads: require an explicit destination. Never corrupt stdout with binary bytes by default; refuse overwriting an existing file without an explicit overwrite choice.
- Browser/HTML/redirect endpoints: document the browser or URL contract per operation; never claim their HTML is JSON. Do not print secret-bearing redirect URLs.
- Diagnostics and one structured error object go to stderr: `{"error":{"code":"invalid_arguments","message":"..."}}`. The foundation uses `unknown_command`, `invalid_arguments` and `process_failure`; add stable API error codes plus safe HTTP status or operation ID when relevant, never raw credentials/server dumps.
- `version` and `--version` return the same JSON object with `version`, `commit`, `date`. Help remains human-readable.

For automation, avoid progress bars, ANSI escapes, timestamps, and unsolicited stdout. Future human formatting must be explicit, leaving JSON the default. Preserve the documented output contract throughout 0.x; breaking changes still require release notes and the configured breaking bump.

## Exit codes

| Code | Meaning |
| --- | --- |
| 0 | Success |
| 1 | Other/local failure |
| 2 | Usage or input error |
| 3 | Authentication or authorization failure |
| 4 | Transport or timeout failure |
| 5 | Other API failure |
| 130 | Interrupted |

Do not infer failure solely from a nullable successful body: `auth get-session` can validly return null. Map documented protocol errors deliberately. Signal cancellation must stop pending requests and device polling, not merely hide their output.
