# Releases

Releases are deliberately disabled. `release/readiness.json` is the mechanical gate; while `enabled` is `false`, the reusable release workflow's `publish` job cannot create a tag, publish assets, attest artifacts, or open a Formula pull request.

## Activation

Activation is a reviewed pull request that changes only the reviewed readiness decision after linking local acceptance evidence for every entry in `release/readiness.json`:

1. complete documented public API and current generated operation inventory;
2. every authentication flow, including invalid, expired, and revoked credentials;
3. profile selection, persistence, isolation, and migration;
4. destructive/sensitive-operation confirmations, error behavior, redaction, and non-interactive safety; and
5. a local release-plan and six-archive snapshot inspection, including checksums, embedded build information, and installation smoke behavior.

A maintainer must also review the repository prerequisites below. There is no manual dispatch and no tag, `workflow_run`, or `pull_request_target` trigger: trusted `main` CI calls the reusable workflow only after its exact-SHA `verify` and `cross` jobs succeed. The release workflow checks out that SHA and asserts it again before a remote mutation. Releases are serialized with a non-cancelling `kaneo-cli-release` concurrency group.

## Repository prerequisites

Before enabling releases, configure the `foae/kaneo-cli` repository to:

- allow GitHub Actions `GITHUB_TOKEN` read/write workflow permissions;
- allow Actions to create pull requests;
- retain `contents: write`, `pull-requests: write`, `id-token: write`, and `attestations: write` on the trusted CI release caller;
- permit the repository's public-attestation plan, or explicitly accept that GitHub will reject the attestation step on an unsupported private/internal plan; and
- keep `main` branch protection in force. The Formula update is a normal `chore(homebrew)` pull request and must not bypass review or required checks.

The workflow has no secret other than the caller's `GITHUB_TOKEN`; do not add long-lived publishing credentials. Action revisions are pinned in [the release workflow](../.github/workflows/release.yml). GoReleaser is fixed at v2.18.0. `svu` is installed at v3.3.0 and its committed policy is `.svu.yml`.

## Version policy

The first releasable Conventional Commit creates `v0.1.0`. `fix:` creates a patch release; `feat:` and a `BREAKING CHANGE:` footer or `!` create a minor release while major is zero. `docs:`, `chore:`, and commits without one of those release signals do not release. `.svu.yml` sets `always: false` and `v0: true`; the workflow compares `svu next` with `svu next --v0` before accepting a non-first version. Automatic releases never create `v1.0.0`.

Inspect a checkout without changing GitHub state:

```sh
go run ./internal/cmd/release plan
go run ./internal/cmd/release plan --json
```

A local package-only check is:

```sh
goreleaser release --snapshot --clean --config .goreleaser.yaml
```

Snapshot mode produces no uploads or releases. `/dist/` is ignored so snapshot output does not create an untracked-file dirty failure. The configured release produces six static (`CGO_ENABLED=0`) archives: Linux and macOS `amd64`/`arm64` tarballs and Windows `amd64`/`arm64` zip files, each carrying `LICENSE`, plus a SHA-256 checksum manifest. `Version`, `Commit`, and `Date` are injected into `internal/buildinfo` with ldflags.

## Publication and recovery

For a fresh version, the workflow first verifies no tag or release exists, then creates an annotated tag at the verified commit, pushes it, publishes with GoReleaser, and attests the explicitly listed archive/checksum paths. It never depends on a tag event.

A same-tag retry is intentionally conservative. `release/recover.sh <tag> <expected-commit>` checks the remote tag's peeled commit and refuses a different SHA. If a GitHub release exists, it downloads the published checksum manifest and every expected archive, then verifies each SHA-256. A complete, matching release is reported as `complete` and no asset is overwritten. A missing tag/release combination, missing checksum, missing archive, or checksum mismatch is ambiguous and fails closed; the workflow does not move tags or retry publication.

For an ambiguous partial publication, stop automatic retries. A maintainer must preserve the tag, record the failed run, compare the verified commit and locally rebuilt archive SHA-256 values with the published checksum manifest, and upload only an independently verified missing asset through the GitHub release UI or a reviewed, one-off command. Do not use overwrite/clobber options. If the tag points at a wrong SHA or any existing asset differs, abandon that version and prepare a new reviewed release; never retag it.

`complete` confirms the six assets against the published manifest, not completion of attestation or the Formula PR. A rerun does not repair those later steps. If either failed, a maintainer must finish that step in a reviewed recovery run using the verified existing assets; never republish or replace them.

## Homebrew Formula

After a fresh release, `release/formula.sh` downloads the already-published immutable checksum manifest and renders `Formula/kaneo-cli.rb` for macOS and Linux `amd64`/`arm64`. The workflow pushes that generated file to a new `chore/homebrew-vX.Y.Z` branch and opens a normal PR against `main`; it does not update the protected branch directly. The tap URL is `https://github.com/foae/kaneo-cli`, and, after the Formula PR merges, installation is:

Review the bot-created Formula PR and run local checks before merging. CI runs after the merge to `main`, not on PR creation or reopening. Do not configure these post-merge jobs as required PR checks; they cannot report on PR heads. Publication needs no long-lived credentials.

```sh
brew tap foae/kaneo-cli https://github.com/foae/kaneo-cli
brew install foae/kaneo-cli/kaneo-cli
```

There is deliberately no checked-in Formula before an immutable release exists.
