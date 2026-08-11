---
page_title: "Authentication"
subcategory: "Guides"
description: |-
  How the provider and CLI authenticate with the PiensaSolutions panel,
  and how to derive the 2FA code from a TOTP secret.
---

# Authentication

The PiensaSolutions panel is authenticated in four steps (all handled
inside the provider/CLI; there is no public API):

1. `POST` your NIF, password, and a 2FA code to the secure gateway, which
   returns session cookies.
2. A signed request to the proxy endpoint discovers your VPS server IDs.
3. A redirect chain per server yields an XSRF token.
4. That token is sent as the `X-TOKEN` header on every `api/corevps/v1`
   call. Its lifetime is about **one hour**.

## CLI login (recommended)

Log in interactively with the CLI, which stores the session in
`~/.config/piensa/config.json` (mode `0600`):

```sh
piensa login
```

Or non-interactively (e.g. in CI) via flags or environment variables:

```sh
export PIENSA_NIF=12345678Z
export PIENSA_PASSWORD='…'
export PIENSA_TOTP_SECRET='…'
piensa login
```

The provider reuses this cached session when you don't configure
credentials, so a single `piensa login` covers both tools until the token
expires (~1 hour).

## TOTP secret

The 2FA code is derived from your TOTP secret (the base32 token you
enrolled with your authenticator app), using the standard TOTP algorithm
implemented in `pkg/client/auth.go`. Point the provider at it directly:

```terraform
provider "piensasolutions" {
  nif         = var.piensa_nif
  password    = var.piensa_password
  totp_secret = var.piensa_totp_secret
}
```

All three are required together; when set, the provider logs in itself and
refreshes the disk cache, so repeated `tofu plan` runs within the token TTL
don't reauthenticate. Treat `totp_secret` and `password` as secrets — use
Terraform variables, never literals in configuration.

## Gotcha: action logs need a fresher token

`GET /pss-core/logs` runs on a different backend that enforces token
freshness more strictly than other endpoints. A token that works for
`list`/`shutdown` can return `401` specifically on `logs`. Re-login
(`piensa login`) and retry before debugging anything else.