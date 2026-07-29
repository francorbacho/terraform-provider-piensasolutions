# AGENTS.md

## Overview

Go CLI (`piensa`) and Terraform provider for managing PiensaSolutions VPS
servers. All API endpoints were reverse-engineered from the vendor's web
panel. There is no public API documentation.

## Rules

- NEVER change the verb aliasing table or the test that pins it
  (`TestActionCommandMapping`). See "Verb aliasing" below.
- NEVER trust the frontend JS method names over captured network traffic.
  They disagree. The wire is the source of truth.
- NEVER run `gofmt -w .` repo-wide. Several files have pre-existing
  formatting drift. Only format the files and hunks you change.
- NEVER `cat` or read `*.har` files directly. They are 10–30 MB.
  Parse them programmatically (see "Adding a new endpoint").
- NEVER split `cmd/piensa/main.go` into per-command files. It is a
  deliberate monolith. New cobra commands go in that file.
- NEVER invent hand-written JSON fixtures for tests. Copy real response
  bodies from HAR captures. The wire format has surprises (see "Testing").
- ALWAYS run `go build ./... && go vet ./... && go test ./...` before
  considering work done. There is no CI test gate.
- ALWAYS use the `PIENSA_CONFIG_PATH` env var to redirect config in tests
  and experiments so the real logged-in session is never modified.

## Project layout

| Path | Purpose |
|---|---|
| `cmd/piensa/main.go` | CLI. All cobra commands live here (~770 lines, monolith by design). |
| `cmd/terraform-provider-piensasolutions/` | Terraform provider entrypoint (thin wrapper). |
| `pkg/client/` | HTTP client for the PiensaSolutions APIs. One file per noun (`servers.go`, `images.go`, `logs.go`, …). |
| `pkg/config/` | `~/.config/piensa/config.json` read/write. |
| `pkg/models/` | Shared plain structs (`Server`, `Image`, `LogEntry`, …). |
| `pkg/provider/` | Terraform provider schema and resources. |

New API wrapper functions go in `pkg/client/<noun>.go` with a matching
`<noun>_test.go` in the same change.

## Verb aliasing (critical invariant)

The panel has no working `/start` or `/shutdown` endpoint. The CLI maps
user-facing verbs to the real wire verbs:

| CLI command | Wire action sent | Reason |
|---|---|---|
| `restart` | `reboot` | Confirmed via JS and live traffic. |
| `start` | `resume` | `start` returns 409 on a suspended server. `resume` succeeds. |
| `shutdown` | `suspend` | No `/shutdown` endpoint exists. The panel's "shutdown" button sends `suspend`. |
| `suspend` | `suspend` | Direct match. |
| `resume` | `resume` | Direct match. |
| `reinstall` | `reinstall` | Direct match (the one correctly-named action). |

This mapping lives in `actionCommandSpecs` in `cmd/piensa/main.go` and is
pinned by `TestActionCommandMapping` in `cmd/piensa/main_test.go`. If that
test fails, restore the mapping. Do not update the test to match new code.

Related: `checkStatus()` in `client.go` maps all HTTP 409 to a generic
"server is busy" message. A persistent 409 may mean wrong verb, not a
concurrency lock.

## Auth flow

Four-step chain, implemented in `pkg/client/auth.go`:

1. `POST secure.piensasolutions.com/public-gateway.php` with NIF, password,
   2FA code → session cookies (`tkn`, `pvtKey`).
2. HMAC-signed requests (`X-HASH` / `X-MICROTIME` from `pvtKey`) to
   `secure.piensasolutions.com/proxy.php` → discover VPS `idsco` IDs.
3. `service/{idsco}/panellink` → redirect chain → extract XSRF token from
   a `piensasolutions*` cookie.
4. That XSRF token becomes the `X-TOKEN` header for all
   `front-cloudpanel.piensasolutions.com/api/corevps/v1/...` calls.
   Lifetime: ~1 hour.

The CLI accepts a literal `--2fa <code>`, or a `--totp-secret`
(equivalently `PIENSA_NIF`/`PIENSA_PASSWORD`/`PIENSA_TOTP_SECRET` env
vars, all with the same names as `secrets.yaml` uses) to derive one.
`client.GenerateTOTP` in `pkg/client/auth.go` is the single implementation
of this (stdlib only: `crypto/hmac`, `encoding/base32`); the Terraform
provider's `totp_secret` config calls the same function. If you need TOTP
generation somewhere else, call it — do not reimplement it or add an
external OTP dependency.

**Gotcha:** `GET /pss-core/logs` is on a different backend and enforces
token freshness more strictly than other endpoints. A token that works for
`list`/`shutdown` can 401 specifically on logs. Re-login and retry before
debugging headers.

## Config

- Path: `~/.config/piensa/config.json` (mode 0600). Override with
  `PIENSA_CONFIG_PATH`.
- `models.Config.Accounts` is a slice, but `mergeAccount()` only writes to
  `Accounts[0]`. Multi-account is not implemented despite the type shape.
  Do not build features that assume multiple accounts work.
- `config.FindAccountByServerID` uses `strings.HasPrefix`, so short 8-char
  server IDs work as CLI arguments.

## Reinstall / reflash

Endpoint: `PUT /pss/servers/{id}/reinstall`

```json
{
  "password": null,
  "image_type": "IMAGE",
  "image_id": "<uuid>",
  "cloud_config_content_type": "yaml | sh | null",
  "cloud_config": "<text> | null"
}
```

- Only `image_type: "IMAGE"` (from `GET /pss/images?depth=3`,
  `type=="HDD"`) supports `cloud_config`. ISO and USER_IMAGE paths are
  intentionally not implemented.
- `cloud_config_content_type` is `"yaml"` for `#cloud-config` or `"sh"`
  for `#!/bin/bash`. Max size: 16 KB (`maxCloudConfigBytes` in main.go).
- Images are scoped by `datacenter_id`. Always filter by the target
  server's `datacenter_id` (see `findImage` in main.go).
- `findImage` resolution order: exact ID → exact alias → exact
  `image_aliases[]` entry → unambiguous substring on alias/name. Ambiguous
  substrings return an error.

## Testing

All tests use `net/http/httptest`. No mocking library.

Pattern:

```go
srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
    // assert method, path, query, body
    fmt.Fprint(w, sampleJSON)
}))
defer srv.Close()

origBase := client.FrontPanelBase
client.FrontPanelBase = srv.URL
defer func() { client.FrontPanelBase = origBase }()

c := client.New("test-token")
```

This works because `CloudPanelBase`, `FrontPanelBase`, `SecurePanelBase`,
and `GatewayURL` in `pkg/client/client.go` are package-level `var`s.
Do not change them to `const`.

- `pkg/client/*_test.go`: `package client_test` (black-box).
- `cmd/piensa/main_test.go`: `package main` (needs unexported
  `findImage`, `findServer`, `actionCommandSpecs`).

Wire-format surprises to preserve in fixtures:
- `details` is either a JSON object or an empty array `[]` depending on
  the action.
- `finished` is JSON `null` while an action is `IN_PROGRESS`.
Both cases are covered in `pkg/client/logs_test.go`.

## Adding a new endpoint

1. Get a HAR capture of the action from the real panel.
2. Parse it programmatically. Filter to PUT/POST/GET requests on
   `front-cloudpanel.piensasolutions.com/api/corevps/v1/...`. Skip static
   assets and third-party domains. Inspect `request.postData.text` and
   `response.content.text` on matching entries.
3. Cross-check against the JS bundle if useful, but trust captured traffic
   over JS when they disagree.
4. Add a typed wrapper in `pkg/client/<noun>.go`. Follow existing style:
   unexported response structs with json tags, exported function returning
   `[]models.X` or `map[string]interface{}`, `checkStatus(resp)` for
   errors.
5. Add `<noun>_test.go` using the httptest pattern above. Use real
   captured JSON for fixtures.
6. Wire a cobra command in `cmd/piensa/main.go`. Resolve the token via
   `config.FindAccountByServerID(cfg, inputID)`.
7. For destructive actions: add a confirmation prompt gated by `--yes`
   (see `reinstallCmd`). Consider a `--dry-run` flag.
8. Run `go build ./... && go vet ./... && go test ./...`.
