# `auth login` rebinds a profile's stored API URL to `KANEO_API_URL`

Local defect, this repository (`foae/kaneo-cli`). Not fixed here.

## Environment

- kaneo-cli 1.5.0, commit `565dbba70322b355e44645f65c4494e76e478b67`
- Pinned API snapshot SHA-256 `a5f29855e3f25c703bf665fd17703cc79b672bd4e24f9f5fbd8f0c1b8e44db9e`
- Disposable Kaneo image `ghcr.io/usekaneo/kaneo@sha256:a85a23996c36166cfcebcb4ee161b40cc62c7b924faf06353684a84d5e84162c`, brought up from `integration/compose.yaml` as Compose project `kaneo-cli-acceptance` on `http://localhost:15173/api`. MinIO was not started; no upload scenario was exercised.
- Upstream source read at `usekaneo/kaneo` commit `ca70c72c4ed5585d0e4ffc3baf692def6f44d185`
- Observed 2026-09-22, Linux amd64

## Defect

`auth login` binds the profile to the URL resolved for that invocation, and the documented precedence puts the `KANEO_API_URL` environment variable above the profile's own stored URL. An explicitly configured profile URL is therefore silently replaced by an ambient environment variable, and the credential is filed under the environment's URL.

## Reproduction

With `KANEO_API_URL` exported as some other instance:

```
$ kaneo-cli profile set acceptance --api-url http://localhost:15173/api
{"name":"acceptance","api_url":"http://localhost:15173/api","default":false,"authenticated":false}

$ kaneo-cli --profile acceptance auth login --api-key-file ./key
{"profile":"acceptance","api_url":"http://<KANEO_API_URL host>/api","method":"api_key","storage":"file"}
```

After this, the profile's stored `api_url` is the environment's value, and the API key for the first instance is stored under the second instance's URL key.

## Mechanism

Both halves are deliberate in isolation:

- `internal/config/resolve.go:86-99` ranks flag > environment > profile > default.
- `internal/auth/backend.go:72-76` then sets `profile.APIURL = apiURL`, with the comment "An explicit login also binds the profile's nonsecret URL to the credential it just stored."

## What is and is not at risk

`--api-key-file` login performs **no** network request (`internal/cli/auth.go:58-63`) — it only stores the secret — so no credential is transmitted to the wrong origin by this path. The defect is silent misconfiguration plus a credential filed under the wrong URL key: a later command using the profile targets the environment's instance, and the profile no longer records the URL the operator configured.

The device-login path does make network calls. Whether it is affected the same way was not tested [unverified].

## Options to weigh

Presented as options, not a decision:

- Have `auth login` prefer the selected profile's own stored URL over `KANEO_API_URL`.
- Refuse to rebind an existing profile's URL unless `--api-url` was given explicitly.
- Emit a warning when login is about to change a profile's stored URL.
