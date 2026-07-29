# AGENTS.md

Handoff notes for coding agents working on this repo. This is a Go CLI
(`piensa`) plus a Terraform provider for managing PiensaSolutions VPS
servers, built entirely by **reverse-engineering the vendor's web panel** —
there is no public API documentation. Read this before touching
`pkg/client` or adding a new server action.

## Project layout

```
cmd/piensa/                          CLI (cobra). Single main.go, ~770 lines.
cmd/terraform-provider-piensasolutions/  Terraform provider entrypoint (thin).
pkg/client/                          HTTP client for the PiensaSolutions APIs.
pkg/config/                          ~/.config/piensa/config.json read/write.
pkg/models/                          Shared plain structs (Server, Image, LogEntry, ...).
pkg/provider/                        Terraform provider schema/resources.
```

New CLI subcommands go in `cmd/piensa/main.go` (it's a monolith by
convention here, not split per-command). New API wrapper functions go in
`pkg/client/<noun>.go` (e.g. `images.go`, `logs.go`, `servers.go`). Add a
matching `<noun>_test.go` in the same PR — see Testing below.

## The core workflow: reverse-engineering from HAR files

There is no PiensaSolutions API reference. All endpoints in this codebase
were discovered by capturing a HAR (HTTP Archive) of the real web panel
(`cloudpanel.piensasolutions.com` / `front-cloudpanel.piensasolutions.com`)
while performing an action, then grepping it. `*.har` is gitignored — the
user drops them in the repo root when they want a new feature reverse
engineered. They're large (10-30MB); don't `cat`/`Read` them directly.

Pattern that works:
```python
import json
with open('some.har') as f:
    har = json.load(f)
for e in har['log']['entries']:
    url = e['request']['url']
    # skip static assets and third-party noise: mypurecloud, kameleoon,
    # googletagmanager, ionos, uicdn, *.js/*.css/*.woff/*.png/etc.
    ...
```
Then inspect `request['postData']['text']` and
`response['content']['text']` on the interesting entries (usually PUT/POST
to `front-cloudpanel.piensasolutions.com/api/corevps/v1/...`).

**Do not trust the frontend JS's method names.** The bundled JS is
minified but un-obfuscated enough to grep (`grep -o '.\{100\}shutdown.\{200\}'
reflashing.har`), and it lies. Example: the DAO has a literal
`shutdown(e){this.doAction("shutdown",e)}` method, but the *actual captured
network request* when the panel's "shut down" button was clicked hit
`PUT .../servers/{id}/suspend`, never `/shutdown`. The tell was the
permissions enum (`SERVERS:["SHOW","CREATE",...,"SHUTDOWN",...]` — no
separate `SUSPEND` entry exists), and confirmed by grepping
`shutdownDisabled(){...!this.permissions.canSuspend}` — the "shutdown"
button's enabled-state literally reuses the suspend permission. When in
doubt, trust the captured network traffic over the JS source.

## Critical invariant: verb aliasing

The panel has **no working `/start` or `/shutdown` server action**. Real
action verbs, encoded in `actionCommandSpecs` in `cmd/piensa/main.go`:

| CLI command | REST action sent   | Why |
|---|---|---|
| `restart`   | `reboot`   | pre-existing, confirmed via JS `restart(){doAction("reboot")}` |
| `start`     | `resume`   | `start` action 409s on a suspended server; `resume` succeeds immediately (verified live) |
| `shutdown`  | `suspend`  | no `/shutdown` endpoint exists; see above |
| `suspend`   | `suspend`  | matches |
| `resume`    | `resume`   | matches |

`TestActionCommandMapping` in `cmd/piensa/main_test.go` pins this table
down. If you "simplify" `makeActionCmd` calls back to literal verbs, this
test will fail — that's the point, don't change the test to match, fix
the mapping back to the table above (or re-verify against a live server if
you think the platform changed).

`reinstall` is the one action that *is* named correctly on the wire
(`PUT .../servers/{id}/reinstall`).

## Reinstall / reflash details (`pkg/client/servers.go`, `images.go`)

```
PUT /pss/servers/{id}/reinstall
{
  "password": null | "<pw>",              // null -> server auto-generates one,
                                            // returned as properties.first_password
  "image_type": "IMAGE" | "ISO" | "USER_IMAGE",
  "image_id": "<uuid>",                    // IMAGE/USER_IMAGE
  "image_alias": "ubuntu:24.04_iso",       // ISO only, uses image.Alias with " "->":"
  "cloud_config_content_type": "yaml" | "sh" | null,
  "cloud_config": "<text>" | null
}
```
- Only `IMAGE` type (from `GET /pss/images?depth=3`, `type=="HDD"`) supports
  `cloud_config`. This CLI only implements the `IMAGE` path — ISO
  (`/pss/dvds`) and `USER_IMAGE` (`/pss/user-images`) reinstalls are not
  wired up, on purpose, since they can't carry cloud-init/scripts anyway.
- `cloud_config_content_type` is `"yaml"` for cloud-init (`#cloud-config`)
  or `"sh"` for a bash script (`#!/bin/bash`) — these are the only two the
  panel's own UI offers. Max size is **16KB** (`maxCloudConfigBytes` in
  main.go, taken from the frontend's `maxKb:16`).
- Images/DVDs are scoped by `datacenter_id` — always filter by the target
  server's own `datacenter_id` (see `findImage` in main.go) or you'll
  offer/select an image the datacenter doesn't have.
- `findImage` resolves in this order: exact ID → exact `alias` → exact
  entry in `image_aliases[]` → unambiguous substring match on alias/name.
  Ambiguous substrings error out rather than guessing.

## Auth flow (`pkg/client/auth.go`)

Multi-hop, not a simple bearer token:
1. `POST secure.piensasolutions.com/public-gateway.php` with form fields
   `DAFLOGIN`/`DAFPASS`/`DAFCODE` (NIF/password/2FA code) → session cookies
   `tkn` + `pvtKey`.
2. HMAC-signed requests (`SecureClient`, `X-HASH`/`X-MICROTIME` headers
   derived from `pvtKey`) to `secure.piensasolutions.com/proxy.php` →
   `service/list` to discover VPS `idsco` IDs.
3. `service/{idsco}/panellink` → follow a redirect chain through
   `loginuser.php` → extract an XSRF token from a `piensasolutions*` cookie.
4. **That XSRF token is the `X-TOKEN` header** used for every
   `front-cloudpanel.piensasolutions.com/api/corevps/v1/...` call for the
   rest of the CLI's life. It lasts **~1 hour**.

`client.GenerateTOTP` (`pkg/client/auth.go`, stdlib `crypto/hmac`+
`encoding/base32`, no external deps) derives a 6-digit code from a base32
TOTP secret — it's the one place in the repo that does this; both
`piensa login` and the Terraform provider's `totp_secret` config call it.
Don't add an `otp`/`pyotp` dependency, the existing code proves stdlib is
enough. `piensa login` resolves `--nif`/`--password`/`--2fa` from
`PIENSA_NIF`/`PIENSA_PASSWORD`/`PIENSA_TOTP_SECRET` env vars when the
flags are empty (`resolveLoginCredentials` in main.go), which is what
makes `sops exec-env secrets.yaml "piensa login"` work with zero flags
— note the flag name is `--totp-secret`, not `--2fa`, when you have a
secret rather than a literal code; an explicit `--2fa` always wins.

**Gotcha:** `GET /pss-core/logs` (the `piensa logs` backing endpoint) is on
a *different* backend service than the rest of `/api/corevps/v1/...` and
enforces token freshness more strictly. A token that's still happily
serving `piensa list`/`piensa shutdown` can 401 specifically on
`/pss-core/logs`. If you see an inexplicable 401/403 only on one endpoint,
suspect this before suspecting missing headers/cookies — re-login and
retry first.

**Gotcha:** `checkStatus()` in `client.go` maps *any* HTTP 409 to a generic
`"server is busy, try again in a few seconds"`. This can mask a genuinely
different root cause. Concretely: `piensa start` on a suspended server
409s not because of a concurrency lock but because `start` is simply the
wrong action for that state transition (see the verb table above) — the
error message doesn't distinguish these, so if a 409 doesn't clear after
waiting, consider "wrong verb" before "still busy."

## Config (`pkg/config/config.go`)

- Default path `~/.config/piensa/config.json` (mode 0600), override with
  `PIENSA_CONFIG_PATH` env var — **use this override in tests/experiments**
  so you never clobber the real logged-in session.
- `models.Config.Accounts` is a slice but `mergeAccount()` in main.go only
  ever writes into `Accounts[0]` — logging in with a different NIF does
  *not* create a second account, it merges servers into the same one. Real
  multi-account support isn't implemented despite the type shape.
- `config.FindAccountByServerID` matches by `strings.HasPrefix`, so short
  8-char IDs (as printed by `piensa list`) work as CLI arguments everywhere.

## Testing

No test framework existed before commit `24d5955` — this repo's tests are
all `net/http/httptest` based, no mocking library. The pattern:

```go
srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
    // assert method/path/query/body here
    fmt.Fprint(w, sampleJSON)
}))
defer srv.Close()

origBase := client.FrontPanelBase
client.FrontPanelBase = srv.URL
defer func() { client.FrontPanelBase = origBase }()

c := client.New("test-token")
// call the function under test
```

This works because `CloudPanelBase`/`FrontPanelBase`/`SecurePanelBase`/
`GatewayURL` in `pkg/client/client.go` are **package-level `var`s, not
`const`s**, specifically so tests can redirect them. Keep them that way.

- `pkg/client/*_test.go` use `package client_test` (black-box, exported API
  only).
- `cmd/piensa/main_test.go` uses `package main` (needs access to unexported
  `findImage`/`findServer`/`actionCommandSpecs`).
- When adding a JSON-parsing test, use real (trimmed) response bodies
  copied from a HAR capture, not hand-invented shapes — the wire format has
  surprises a hand-written fixture would miss (e.g. `details` comes back as
  either a JSON object *or* an empty array `[]` depending on the action;
  `finished` is JSON `null` while an action is `IN_PROGRESS`). Both are
  covered in `pkg/client/logs_test.go`.

Run everything with:
```
go build ./...
go vet ./...
go test ./...
```
There is no CI test gate (`.forgejo/workflows/release.yml` only builds and
packages binaries on tag push) — `go test ./...` is not run automatically
anywhere, so you must run it yourself before calling work done.

`gofmt -l .` currently flags several files
(`cmd/piensa/main.go`, `pkg/client/client.go`, `pkg/client/discovery.go`,
`pkg/client/firewall.go`, `pkg/config/config.go`, `pkg/models/types.go`)
with pre-existing struct-field-alignment drift unrelated to any of the work
in this document. Don't "fix" it as a drive-by in an unrelated change; only
keep gofmt clean in the files/hunks you actually touch (check with
`gofmt -d <file>` and confirm the diff is only your own new code before
worrying about it).

