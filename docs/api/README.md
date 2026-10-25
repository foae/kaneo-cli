# Vendored API reference

`api/openapi.json` is an exact snapshot of upstream release tag `v2.47.0`, fetched on 2026-10-25. Its SHA-256, upstream source relationship, and the complete upstream MIT notice are recorded in [`api/provenance.json`](../../api/provenance.json).

The relationship is verified rather than inferred from this repository's license: upstream [`usekaneo/kaneo`](https://github.com/usekaneo/kaneo) `apps/docs/docs.json` selects `openapi.json` as its API Reference. The pinned bytes match both the live official document and `apps/docs/openapi.json` at release commit `85a32200e7c5bfb8e9c2c6bb208a37297e0ff5a5`. All 221 operations retain canonical command mappings; the 2.47.0 refresh changed no command names. The authentication guide describes `X-RateLimit-*` headers on successful requests while the spec incorrectly restricts them to exceeded limits: upstream source sends them for valid API-key requests as well as window-limit failures. See the provenance supplement and [verification evidence](../verification.md#api-baseline-refresh-to-kaneo-2470-2026-10-25).

## Inventory

[`api/commands.json`](../../api/commands.json) is the reviewed source-to-command map. Each OpenAPI method/path has exactly one `group action` pair and status. Canonical groups are singular; `org` is the deliberate abbreviation for Organization Management. A status is `implemented` only when the command exists and has recorded evidence; every other entry is `planned` and remains a design reference.

Generate the derived inventory and this reference with:

```sh
go run ./internal/cmd/specinventory
```

Check that all generated files are current without network access:

```sh
go run ./internal/cmd/specinventory --check
```

[`operations.md`](operations.md) provides the per-operation table and generated request sections. [`api/operations.json`](../../api/operations.json) contains the full discoverable reference for each operation: source method/path and ID, command mapping/status, inherited or local security, query/path parameters, request-body schemas, and responses. [`internal/cli/body_help_generated.go`](../../internal/cli/body_help_generated.go) is generated from the same pinned schemas and supplies the static JSON-body help for CLI mutations; schema references resolve during generation, never at command runtime.

## Device authorization supplement

The official [authentication guide](https://kaneo.app/docs/api-reference/authentication) documents a device authorization flow (the RFC 8628 device-code grant) for interactive clients. Its provenance, factual summary, and unresolved differences from the OpenAPI snapshot are intentionally separate in `api/provenance.json` under `supplements.device_authorization`; it does not contribute to the OpenAPI operation count. In particular, the guide documents `POST /api/auth/device/code` and `POST /api/auth/device/token`, while the snapshot only describes `GET /auth/device`.

The exact guide is pinned at [`api/authentication.md`](../../api/authentication.md), with its own retrieval date and SHA-256 in provenance. It supplies the JSON code-request and token-poll examples without pretending to be a complete schema.

## Explicit refresh procedure

Never fetch upstream during ordinary generation, checks or builds.

1. Download the official OpenAPI JSON and authentication Markdown URLs recorded in provenance to temporary files. Check HTTP success and content type; reject an HTML error/challenge page rather than replacing the snapshot.
2. Review semantic differences: added/removed methods, security overrides, required fields, response shapes, protocol examples and behavioral changes.
3. Resolve the upstream source commit and compare exact bytes with the official spec. Recheck the documentation configuration/source relationship and upstream license; do not claim a source revision without matching evidence.
4. Replace reviewed snapshots and update their SHA-256, retrieval timestamps, source URLs/commits and complete attribution in `api/provenance.json`.
5. Update `api/commands.json` for every changed operation. Review public command naming explicitly; do not silently rename existing commands or treat a new operation as implemented.
6. Run the generator, inspect the generated inventory/reference diff, then run `just check`. Commit inputs and derived outputs together.
7. Reconcile affected implementation packets, compatibility evidence and release readiness. Updating the spec alone does not prove server compatibility.
