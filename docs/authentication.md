# Authentication and named profiles

**Partially implemented:** named profiles, keyring-first credential storage with a warned plaintext fallback, API-key input, the RFC 8628 device flow and local logout. Server-side revocation is a separate, still planned operation. Source: [official authentication guide](https://kaneo.app/docs/api-reference/authentication), pinned alongside the API baseline. Kaneo documents API keys and RFC 8628 device authorization; `kaneo-cli` is a documented default client ID. Self-hosted administrators can restrict allowed client IDs.

## Profiles and precedence

Each named profile binds a full API base URL to a credential reference and safe defaults. Resolve profile name from `--profile`, then `KANEO_PROFILE`, then the explicitly selected default profile. Resolve nonsecret settings from command flags, then corresponding environment variables (`KANEO_API_URL`, `KANEO_TIMEOUT`), then selected profile, then documented defaults. The defaults are profile `default`, API base URL `https://cloud.kaneo.app/api` and a 30s request timeout. Never silently use a different profile's credentials.

`KANEO_TOKEN` is an invocation-only credential override; do not persist it. Persistent API keys are accepted via stdin or a protected file (`auth login --api-key-file PATH`, or `-` for stdin), not a token argument in shell history. Device login explicitly stores the resulting token in the selected profile. An explicit login also binds the profile's stored API URL to the URL the credential was obtained from. `auth logout` and `profile delete` remove stored credentials and require `--yes`. Public operations (`instance get-status`, `config get`) do not load or transmit a stored credential; `auth get-session` does. Document the source of effective settings without revealing values of secrets.

Use `os.UserConfigDir` for configuration, atomic updates and platform-appropriate locking for concurrent changes. Store no credential in the nonsecret configuration file. Do not write config or create directories just to run help/version. Keep profile names out of filesystem path construction. Local profile deletion removes only that profile's associated credential; server revocation is a separate explicit operation.

Bind credentials to both profile and normalized API URL. Changing an origin or API base path requires explicit reauthentication/rebinding; do not transplant old tokens. Redirects must never forward credentials across origins. Logout removes local credentials reliably from the actual active backend; don't leave a plaintext fallback behind after keyring migration.

## Storage policy chosen for this project

Try the OS keyring first (macOS Keychain, Windows Credential Manager, Linux Secret Service). If unavailable, fall back to an **unencrypted credential file**, emit a stderr warning, and disclose the active backend in safe status output. Warn when storing and when using fallback credentials; this is not encrypted-at-rest security.

On Unix require owner-only directory/file permissions (0700/0600), safe atomic replacement, and refusal of symlinks or unsafe ownership. On Windows enforce a current-user restricted DACL: `chmod(0600)` alone is insufficient. If secure file restrictions cannot be established, fail rather than write a broadly readable token. Never place secrets in logs, diagnostics, test snapshots or crash reports. Include adversarial file replacement and concurrent update cases in tests.

Headless Linux frequently lacks an unlocked Secret Service session. Treat keyring-unavailable, access-denied and corrupt-store cases deliberately; don't overwrite inaccessible credentials or hide a backend error as 'not logged in'. Choose a library only after inspecting its platform implementations and proving CGO-free cross-builds and native behavior. Switching backends must not resurrect stale credentials.

## Device flow

Implement the exact pinned guide's code-request and token-polling endpoints, request encodings and fields. OpenAPI's browser-facing `/auth/device` operation is **not** the complete protocol. Supplementary protocol operations remain distinguishable from OpenAPI coverage.

Display safe verification instructions on stderr. Do not reveal device codes or access tokens in debug/error output. Respect server polling interval, expiry, authorization-pending, slow-down, access-denied and expired-token outcomes; cancel promptly. Never poll indefinitely or retry a denied request as a new login. Store credentials only after documented success. Do not assume a refresh-token flow exists.

## Required evidence

API-key authenticated request; approved/denied/expired/cancelled device login; unavailable keyring fallback and warning; Unix permissions and Windows ACLs; profile isolation; URL changes; cross-origin redirect refusal; concurrent writes; logout cleanup. Mocked keyring tests do not prove native keyring behavior. Keep native-platform evidence explicit and pending until exercised.
