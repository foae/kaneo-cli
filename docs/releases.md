# Releases

Publication is enabled by the reviewed v1.0.0 readiness decision in `release/readiness.json`; each criterion links its acceptance evidence. Setting `enabled` to `false` disables the reusable workflow's `publish` job: it cannot create a tag, publish assets, attest artifacts, or open a Formula pull request.

## Activation

Activation is a reviewed pull request that changes only the reviewed readiness decision after linking local acceptance evidence for every entry in `release/readiness.json`:

1. all canonical API mappings and current generated operation inventory, with documented local acceptance and explicit compatibility limitations;
2. every authentication flow, including invalid, expired, and revoked credentials;
3. profile selection, persistence, isolation, and migration;
4. destructive/sensitive-operation confirmations, error behavior, redaction, and non-interactive safety; and
5. a local release-plan and six-archive snapshot inspection, including checksums, embedded build information, and installation smoke behavior.

For v1.0.0, the maintainer explicitly approved deferring live external-provider acceptance. GitHub, Gitea, Slack, Discord, Mattermost and Telegram integrations are implemented and fixture-tested, but not live-verified against those providers. This does not waive authentication, credential isolation, safety, native credential-store checks or packaging checks. Operation coverage is not a claim of compatibility with every deployment.

A maintainer must also review the repository prerequisites below. There is no manual dispatch and no tag, `workflow_run`, or `pull_request_target` trigger: trusted `main` CI calls the reusable workflow only after its exact-SHA `verify` and `cross` jobs succeed. The release workflow checks out that SHA and asserts it again before a remote mutation. Releases are serialized with a non-cancelling `kaneo-cli-release` concurrency group.

## Repository prerequisites

Before enabling releases, configure the `foae/kaneo-cli` repository to:

- permit the workflow's explicit `GITHUB_TOKEN` write permissions (the repository default may remain read-only);
- allow Actions to create pull requests;
- retain `contents: write`, `pull-requests: write`, `packages: write`, `id-token: write`, and `attestations: write` on the trusted CI release caller;
- enable GitHub immutable releases so published assets and their tags cannot be replaced;
- permit the repository's public-attestation plan, or explicitly accept that GitHub will reject the attestation step on an unsupported private/internal plan; and
- keep `main` branch protection in force. The Formula update is a normal `chore(homebrew)` pull request and must not bypass review or required checks.

The workflow has no secret other than the caller's `GITHUB_TOKEN`; do not add long-lived publishing credentials. Action revisions are pinned in [the release workflow](../.github/workflows/release.yml). GoReleaser is fixed at v2.18.0. `svu` is installed at v3.3.0 and its committed policy is `.svu.yml`.

## Version policy

The first releasable Conventional Commit creates `v1.0.0`. Thereafter `fix:` creates a patch release, `feat:` a minor release, and a `BREAKING CHANGE:` footer or `!` a major release. `docs:`, `chore:`, and commits without one of those release signals do not release. `.svu.yml` sets `always: false` and `v0: false`; subsequent versions must agree with pinned `svu next`.

Inspect a checkout without changing GitHub state:

```sh
go run ./internal/cmd/release plan
go run ./internal/cmd/release plan --json
```

A local package-only check is:
```sh
just snapshot
```

Snapshot mode produces no uploads or releases. `/dist/` is ignored so snapshot output does not create an untracked-file dirty failure. The configured release produces six static (`CGO_ENABLED=0`) archives: Linux and macOS `amd64`/`arm64` tarballs and Windows `amd64`/`arm64` zip files, each carrying `LICENSE`, plus Linux `amd64`/`arm64` deb/rpm packages and a SHA-256 checksum manifest. `Version`, `Commit`, and `Date` are injected into `internal/buildinfo` with ldflags. Native packages install `/usr/bin/kaneo-cli`, include the license, depend on CA certificates, and contain no service or lifecycle scripts.

Container builds are separate from GoReleaser publication. After `just snapshot`, run `release/container.sh smoke vX.Y.Z <full-commit>` with the intended version and commit. Docker/buildx is required only for this explicit container check, not for `just snapshot`. It executes the amd64 binary and builds an arm64 OCI archive; it does not claim native arm64 execution. The image uses the digest-pinned distroless base in `Dockerfile`, UID/GID 65532, CA certificates and the CLI entrypoint.

## Publication and recovery

For a fresh version, the workflow verifies no tag, release or versioned GHCR image exists. Authenticated registry inspection requests scoped access using only `GITHUB_TOKEN`; authentication errors, forbidden responses and transport failures do not establish absence. Before creating a tag it builds snapshot archives/packages and smoke-builds the container. It then creates an annotated tag at the verified commit, pushes it, publishes with GoReleaser, and attests archives, native packages and checksums. It never depends on a tag event.

Only after GitHub artifacts are published does it push `ghcr.io/foae/kaneo-cli:vX.Y.Z` for Linux amd64/arm64. It verifies OCI revision/version/source/license labels, platforms and non-root runtime configuration before recording the image digest in release notes and attesting it. Release notes remain editable and are not an immutable asset; verify the image's provenance attestation as well as its digest. Preserve exactly one plain `Container digest: sha256:…` line for recovery; CRLF line endings are accepted. There is no `latest` alias. Versioned image tags are append-only policy, not registry-enforced immutability; consumers should pin digests. Publication rechecks absence immediately before pushing, but GHCR has no atomic create-only tag operation: the repository's serialized release workflow must be the only writer of version tags.

On first publication GitHub can create the package as private. The owner must make that package public in its GitHub package settings. The workflow checks anonymous registry access after attestation and fails if the documented public image is inaccessible; it does not silently accept authenticated-only success. No long-lived token or automatic settings change is used.

A same-tag retry is intentionally conservative. The workflow checks completeness even when HEAD is already tagged and the release planner proposes no new version. `release/recover.sh <tag> <expected-commit>` requires the expected commit locally, checks the remote tag's peeled commit and refuses a different SHA. It downloads and verifies every expected asset against the checksum manifest. For commits containing `release/container.sh`, completeness additionally requires all four native packages, the recorded container digest, and matching registry content. Earlier releases retain their six-archive contract. Missing or conflicting state fails closed; the workflow does not move tags, overwrite assets or retry publication.

For ambiguous partial publication, stop automatic retries. Preserve the tag and failed-run evidence. Repair only independently verified missing artifacts in an unpublished draft through a reviewed one-off action, never overwrite/clobber options. [Published immutable releases cannot accept additional assets](https://docs.github.com/en/repositories/releasing-projects-on-github/managing-releases-in-a-repository#editing-a-release); a published release missing an archive/package requires a new reviewed version. A GitHub release with a missing container requires a reviewed image publication from the exact tagged source and verified released binaries, followed by recording and attesting its digest. If a tag points at a wrong SHA or existing content conflicts, abandon that version; never retag it. There is no atomic rollback across GitHub Releases and GHCR.

`complete` confirms artifact integrity and container identity, not completion of attestations, anonymous access or the Formula PR. A rerun does not repair those later steps; finish them through reviewed recovery using existing verified artifacts.

The agent plugin has its own version in `.claude-plugin/plugin.json`, starting at `1.0.0`; bump it when its skill or plugin metadata changes, independently of CLI tags. README download links resolve to GitHub Releases rather than a hardcoded version. The [installer](../install.sh) resolves the latest stable tag once and downloads that version's archive and checksum manifest; keep its filename contract aligned with GoReleaser.

## Homebrew Formula

After a fresh release, `release/formula.sh` downloads the already-published immutable checksum manifest and renders `Formula/kaneo-cli.rb` for macOS and Linux `amd64`/`arm64`. The workflow pushes that generated file to a new `chore/homebrew-vX.Y.Z` branch and opens a normal PR against `main`; it does not update the protected branch directly. The tap URL is `https://github.com/foae/kaneo-cli`, and, after the Formula PR merges, installation is:

Review the bot-created Formula PR and run local checks before merging. CI runs after the merge to `main`, not on PR creation or reopening. Do not configure these post-merge jobs as required PR checks; they cannot report on PR heads. Publication needs no long-lived credentials.

```sh
brew tap foae/kaneo-cli https://github.com/foae/kaneo-cli
brew install foae/kaneo-cli/kaneo-cli
```

`Formula/kaneo-cli.rb` is generated from the published immutable release. Its v1.0.0 installation and test passed on Linux amd64; this does not claim native macOS Homebrew or ARM installation acceptance.
