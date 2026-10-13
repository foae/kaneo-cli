# API-key rate-limit header documentation disagrees with runtime

Environment: upstream Kaneo v2.46.2, commit `46ef6f78da1f0c9630d88c56f1686ebfc8de7beb`, disposable Linux amd64 instance, image `ghcr.io/usekaneo/kaneo@sha256:f44be9aa22546f95c59076bee86678bffcaf5e5d4b960c58a107ad1a05cd1f90`.

## Evidence

The pinned [OpenAPI component](https://github.com/usekaneo/kaneo/blob/46ef6f78da1f0c9630d88c56f1686ebfc8de7beb/apps/docs/openapi.json), `components.responses.ApiKeyRateLimited.description`, says: "The X-RateLimit headers are sent only when the rate limit was hit."

The [authentication guide](https://github.com/usekaneo/kaneo/blob/46ef6f78da1f0c9630d88c56f1686ebfc8de7beb/apps/docs/api-reference/authentication.mdx) describes the same headers on successful API-key requests. [Runtime source](https://github.com/usekaneo/kaneo/blob/46ef6f78da1f0c9630d88c56f1686ebfc8de7beb/apps/api/src/utils/rate-limit-headers.ts) returns window metadata for valid keys and window-limit denials. Quota-only denials do not necessarily carry window metadata, and Retry-After is absent when no refill is scheduled.

Live observation on 2026-10-13: authenticated `GET /api/workspace` returned HTTP 200 with `X-RateLimit-Limit: 100` and `X-RateLimit-Remaining: 67`, plus `X-RateLimit-Reset`. No credential is retained in this report.

## Impact and suggested fix

Clients cannot infer the presence of these headers solely from success/failure or HTTP 429. Correct the shared OpenAPI description to describe valid-key window metadata and distinguish window exhaustion, usage quotas and independent authentication limits. Do not alter runtime behavior to match the incorrect prose.

The CLI keeps the exact upstream snapshots and records this discrepancy in provenance. No upstream issue or fix has been submitted.
