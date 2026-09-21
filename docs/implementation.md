# Implementation work packets

These packets define the dependency order and acceptance criteria, not a claim that all criteria have passed. Packets 1–4 and follow-up work have evidence recorded in [verification](verification.md): all 162 pinned mappings are implemented, including the two browser-navigation URL handoffs. The v1.0.0 release decision explicitly defers external-provider live acceptance; see [releases](releases.md) for the approved boundary. Complete prerequisites before dependent work; within a packet, disjoint command groups may be parallelized after shared contracts are settled.

## 1. Runtime, configuration and auth

Inputs: architecture, CLI contract, authentication guide, pinned device supplement, inventory. Implement shared HTTP transport, timeout/cancellation and exit/error translation; named profiles, OS keyring plus warned plaintext fallback; API-key input and device login. Introduce only packages needed by working behavior.

Acceptance: run the binary against fixture servers to prove IO separation, JSON/no-content semantics, precise numbers, request presence semantics, redirect isolation and cancellation. Exercise keyring/fallback on real target operating systems, not only mocks. Prove device pending/slow-down/denied/expired transitions. Record upstream ambiguities with provenance before proceeding.

## 2. Read-only API coverage

Inputs: packet 1, each pinned operation and its referenced schemas, generated per-group inventory. Implement all read operations with documented path/query encoding, security exceptions, pagination and JSON/binary/browser handling. Public operations must not require credentials merely because a global security default exists.

Acceptance: operation-level requests match the wire contract and consumer-visible output; real disposable instance reads work for public and authenticated operations, absent objects and authorization failures. Mark each operation implemented only after the command actually exists and evidence is recorded. Keep supplemental device flow separate.

## 3. Mutations, uploads and integrations

Inputs: packets 1–2 and operation-specific evidence. Implement remaining operations, including deletion/revocation safeguards, JSON body presence, multipart and presigned transfer flows. `--yes` checks run before requests. Uploaded bytes and downloaded files are streamed and verified. Never send API credentials to object storage.

Acceptance: isolated create/read/update/delete lifecycles, permissions failures, invalid inputs, cancellation and partial-failure behavior. External integrations require fixture protocol coverage; real-service evidence must be clearly distinguished and is explicitly deferred for v1.0.0. Do not label mocked Slack/GitHub/etc. verification end-to-end. Browser operations use a deliberate URL handoff: print only the initial navigation URL and a diagnostic that the user must open it, without launching a browser, requesting approval, polling, or fabricating an authorization-code URL.

## 4. Full acceptance and release readiness

Inputs: all earlier packets, release readiness checklist, [verification](verification.md). Reconcile every operation against the pinned baseline: no duplicates, omissions or 'implemented' rows backed only by a stub. Run disposable instance acceptance with the pinned image digest and record actual server version separately. Verify both auth modes, named profiles, destructive safety, six-target builds, native OS credential handling and installation/version provenance.

Acceptance: maintainers can reproduce all evidence; discrepancies are resolved or explicitly approved contract changes, not silently waived endpoints. Only then propose release activation for v1.0.0 through reviewed configuration. Do not activate publication or treat command registration, help, or version output as compatibility evidence.

## Every packet handoff

Report changed commands/operation IDs, exact checks run, target OS/architecture, API hash/image digest, evidence paths, and remaining discrepancies. Keep documentation status accurate. When upstream documentation and implementation disagree materially, stop that affected operation and ask for the contract decision with evidence; continue only unrelated safe work.
