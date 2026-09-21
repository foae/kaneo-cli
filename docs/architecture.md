# Architecture

## Current layout

- `cmd/kaneo-cli`: process boundary, IO wiring and exit status.
- `internal/cli`: Cobra command construction, profiles, authentication, API operations and file transfers.
- `internal/client`: HTTP transport, URL handling and safe error classification.
- `internal/config`: profile persistence and invocation configuration.
- `internal/auth`: device authorization and platform credential storage.
- `internal/buildinfo`: release ldflags with `runtime/debug.ReadBuildInfo` fallback for source and `go install` builds. Never invent a release version for an untagged working tree.
- `api/`: pinned upstream inputs and reviewed mapping/coverage metadata.
- `internal/cmd/`: repository development tooling, not shipped commands.
- `docs/`: implementation contracts and generated operation references.

Module: `github.com/foae/kaneo-cli`. One executable, no exported SDK. Release builds use `CGO_ENABLED=0`. Dependencies for credential storage must retain six-target cross-compilation support; native runtime functionality still requires native evidence.

## Grow only when needed

Extend existing `internal/client`, `internal/config`, `internal/auth` and command-domain files for working behavior; do not add empty scaffolding. Domain commands translate CLI input into the documented wire request and pass the response through, except for explicitly documented secret-safe transfer output. Transport owns timeouts, cancellation, bounded error handling and credential isolation. Configuration resolves a profile once per invocation. Auth owns device polling and credential lifecycle. Avoid a generic framework that hides operation-specific behavior.

Use an injected `http.Client` with explicit timeouts. Preserve response numbers and unknown fields: do not deserialize arbitrary API JSON through `map[string]any` and round integers through float64. Treat non-JSON success bodies according to the operation contract. Bound error-body reads and redact before rendering. Stream file transfers; don't load entire uploads/downloads in memory. Close bodies on every path.

Do not follow API redirects to another origin. Origins include scheme, hostname and effective port. Reject userinfo in URLs. Device verification URLs are sensitive untrusted server input; validate scheme/origin policy before launching a browser. The browser-navigation handoff contract being added prints only the initial navigation URL to stdout and a user-facing handoff diagnostic to stderr; it does not launch a browser, make an HTTP request, load credentials, poll, approve, or generate an authorization-code URL. Presigned uploads use a separate unauthenticated client and never receive the API bearer token.

No blanket retries. Device polling follows its documented protocol. A read retry policy, if later introduced, requires explicit bounded behavior and tests; mutations are not retried without documented idempotency support. An absent endpoint on an older server produces an actionable error, not a fake empty response.

## API compatibility

The compatibility target is the pinned documentation baseline, not every historic server version. OpenAPI `info.version` is not a Kaneo release version. Image digests, upstream source revisions and spec hashes are different provenance identifiers; do not conflate them. Refreshes are explicit reviewable changes, never build-time downloads.
