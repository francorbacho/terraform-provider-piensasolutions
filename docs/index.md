---
page_title: "Provider: piensasolutions"
description: |-
  Manage PiensaSolutions VPS servers and firewall rules. The API is not
  publicly documented; all endpoints were reverse-engineered from the
  vendor's web panel.
---

# piensasolutions Provider

The `piensasolutions` provider lets you manage
[PiensaSolutions](https://www.piensasolutions.com) VPS servers — reading
server state and driving firewall (port/protocol allow) rules. It pairs
with the companion `piensa` CLI, which handles power actions, reinstalls,
and action logs.

The vendor does not publish an API; every endpoint was reverse-engineered
from the web panel's captured traffic.

## Example Usage

The provider authenticates with your NIF/password/TOTP secret directly,
or reuses the session cached on disk by `piensa login` when no credentials
are configured.

```terraform
terraform {
  required_providers {
    piensasolutions = {
      source  = "francorbacho/piensasolutions"
      version = "~> 0.2"
    }
  }
}

provider "piensasolutions" {
  # Optional: omit to reuse the cached `piensa login` session.
  # nif         = var.piensa_nif
  # password    = var.piensa_password
  # totp_secret = var.piensa_totp_secret
}
```

The cached session lasts about an hour (the XSRF token TTL); when
credentials are configured, the provider logs in automatically and
refreshes the cache.

## Schema

### Optional

- `nif` (String) The account NIF (tax identifier). Required together with
  `password` and `totp_secret` to log in directly; omit to reuse a cached
  `piensa login` session.
- `password` (String, Sensitive) The account password.
- `totp_secret` (String, Sensitive) The base32 TOTP secret used to derive
  the 2FA code (see the [authentication guide](guides/authentication)).