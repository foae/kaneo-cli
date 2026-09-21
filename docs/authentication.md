# Authentication and named profiles

**Implemented locally:** named profiles, keyring-first credential storage with a warned plaintext fallback, API-key input, the RFC 8628 device flow and local logout. The pinned API baseline does not expose a server-side revocation endpoint, so `auth logout` only removes the local credential. Source: [official authentication guide](https://kaneo.app/docs/api-reference/authentication), pinned alongside the API baseline. Kaneo documents API keys and RFC 8628 device authorization; `kaneo-cli` is a documented default client ID. Self-hosted administrators can restrict allowed client IDs.

## Profiles and precedence

Each named profile binds a full API base URL to a credential reference and safe defaults. Resolve the profile from `--profile`, then a nonempty inherited `KANEO_PROFILE`, then the persisted default selection. Resolve nonsecret settings from command flags, then nonempty inherited corresponding environment variables (`KANEO_API_URL`, `KANEO_TIMEOUT`), then the selected profile, then documented defaults. Empty environment variables, and variables that are not inherited by the CLI process, are unset; they do not override a profile or default. The defaults are profile `default`, API base URL `https://cloud.kaneo.app/api` and a 30s request timeout. Never silently use a different profile's credentials.

### Inspecting the effective profile offline

Run `kaneo-cli profile get [NAME]` before login or a mutation to inspect the canonical effective API URL, timeout, their sources, and the credential backend metadata bound to that effective URL. It reads local configuration only: it makes no server request, creates no profile, and does not access the keyring. With `NAME`, that name selects the profile before `--profile` or `KANEO_PROFILE`; settings themselves still resolve flags, environment, profile, then defaults, and `sources.profile` is `argument`. A fresh implicit `default` may be inspected without stored configuration; an explicitly named missing profile is an error. `profile list` and `profile set` instead show and modify stored settings; their `default` boolean reports only the persisted default selection.

When API use (or API-key login storage) relies on the implicit Cloud default, the CLI emits one stderr warning for that invocation. It does not warn for an explicit Cloud URL, profile inspection, help, or version. Login persists its resolved destination, so later profile-backed use does not receive the implicit-default warning.

Local `profile list`, `profile use`, `profile set`, `profile delete`, and `auth logout` do not resolve request URL or timeout overrides, so malformed inherited values cannot block configuration repair or credential removal. `profile set` still validates the URL and timeout flags it writes. `profile get` deliberately validates effective request settings. Logout removes all recorded credential bindings for the selected profile, not another profile's credentials.

`KANEO_TOKEN` is an invocation-only credential override; do not persist it. Persistent API keys are accepted via stdin or a protected file (`auth login --api-key-file PATH`, or `-` for stdin), not a token argument in shell history. Device login explicitly stores the resulting token in the selected profile. An explicit login also binds the profile's stored API URL to the URL the credential was obtained from. `auth logout` and `profile delete` remove stored credentials and require `--yes`. Public operations (`instance get-status`, `config get`) do not load or transmit a stored credential; `auth get-session` does. Document the source of effective settings without revealing values of secrets.

Use `os.UserConfigDir` for configuration, atomic updates and platform-appropriate locking for concurrent changes. Store no credential in the nonsecret configuration file. Do not write config or create directories just to run help/version. Keep profile names out of filesystem path construction. Local profile deletion removes only that profile's associated credential; no server-side revocation is exposed by the pinned API.

Bind credentials to both profile and normalized API URL. The CLI accepts only HTTP(S) API bases, trims surrounding whitespace, lowercases scheme and host, removes a default port and one trailing slash, and preserves the base path. It neither probes URLs nor changes a dashboard or proxy URL into an API URL: supply the API base yourself (normally including `/api`) and never append `/api` twice. Changing an origin or API base path requires explicit reauthentication/rebinding; do not transplant old tokens. Redirects must never forward credentials across origins. Logout removes local credentials reliably from the actual active backend; don't leave a plaintext fallback behind after keyring migration.

## Storage policy chosen for this project

Try the OS keyring first (macOS Keychain, Windows Credential Manager, Linux Secret Service). If unavailable, fall back to an **unencrypted credential file** and disclose the active backend in safe status output. Warn once per profile/API URL when storing a fallback credential, or on first use of an existing fallback credential that has not warned yet. Remember the warning across invocations; switching back to the keyring or deleting the credential resets it. This is not encrypted-at-rest security.

Credential-bearing HTTP requests warn once per profile/origin, including invocation-only tokens and device-flow secrets. These warning records live in nonsecret configuration, not credentials; different profiles or destinations warn independently. Logout and profile deletion reset that profile's warnings. If configuration cannot be written, warnings remain visible rather than silently suppressed. Remembering a warning does not make HTTP safe: use HTTPS to protect credentials in transit, even on a LAN.

On Unix require owner-only directory/file permissions (0700/0600), safe atomic replacement, and refusal of symlinks or unsafe ownership. On Windows enforce a current-user restricted DACL: `chmod(0600)` alone is insufficient. If secure file restrictions cannot be established, fail rather than write a broadly readable token. Never place secrets in logs, diagnostics, test snapshots or crash reports. Include adversarial file replacement and concurrent update cases in tests.

Headless Linux frequently lacks an unlocked Secret Service session. Treat keyring-unavailable, access-denied and corrupt-store cases deliberately; don't overwrite inaccessible credentials or hide a backend error as 'not logged in'. Choose a library only after inspecting its platform implementations and proving CGO-free cross-builds and native behavior. Switching backends must not resurrect stale credentials.

## Device flow

Implement the exact pinned guide's code-request and token-polling endpoints, request encodings and fields. OpenAPI's browser-facing `/auth/device` operation is **not** the complete protocol. Supplementary protocol operations remain distinguishable from OpenAPI coverage.

Display safe verification instructions on stderr. Do not reveal device codes or access tokens in debug/error output. Respect server polling interval, expiry, authorization-pending, slow-down, access-denied and expired-token outcomes; cancel promptly. Never poll indefinitely or retry a denied request as a new login. Store credentials only after documented success. Do not assume a refresh-token flow exists.

## Required evidence

API-key authenticated request; approved/denied/expired/cancelled device login; unavailable keyring fallback and warning; Unix permissions and Windows ACLs; profile isolation; URL changes; cross-origin redirect refusal; concurrent writes; logout cleanup. Mocked keyring tests do not prove native keyring behavior. Keep native-platform evidence explicit and pending until exercised.
