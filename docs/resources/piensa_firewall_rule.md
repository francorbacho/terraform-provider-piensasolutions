---
page_title: "Resource: piensa_firewall_rule"
description: |-
  A port/protocol allow rule on a PiensaSolutions VPS server.
---

# piensa_firewall_rule

A port/protocol allow rule on a PiensaSolutions VPS server. Creating the
resource opens the port in the server's first firewall policy; deleting it
denies the rule.

`protocol` accepts `TCP`, `UDP`, or `ICMP`. For `ICMP`, set `port` to `0`.
Rules are identified by the triple `server_id:port:protocol`.

## Example Usage

```terraform
resource "piensa_server" "vps1" {}

resource "piensa_firewall_rule" "ssh" {
  server_id   = piensa_server.vps1.id
  protocol    = "TCP"
  port        = 22
  description = "ssh"
}
```

To manage rules for several servers from a single map:

```terraform
locals {
  server_ids = {
    vps1 = piensa_server.vps1.id
    vps2 = piensa_server.vps2.id
  }

  rules = {
    "vps1:ssh"  = { server = "vps1", port = 22,  protocol = "TCP",  desc = "ssh" }
    "vps1:web"  = { server = "vps1", port = 443, protocol = "TCP",  desc = "web" }
    "vps2:icmp" = { server = "vps2", port = 0,   protocol = "ICMP", desc = null }
  }
}

resource "piensa_firewall_rule" "this" {
  for_each    = local.rules
  server_id   = local.server_ids[each.value.server]
  protocol    = each.value.protocol
  port        = each.value.port
  description = each.value.desc
}
```

## Schema

### Required

- `port` (Number) Port to open. Use `0` with `ICMP`.
- `protocol` (String) `TCP`, `UDP`, or `ICMP`.
- `server_id` (String) Server UUID the rule belongs to.

### Optional

- `description` (String) Free-form label shown in the panel.

### Read-Only

- `action` (String) Always `ALLOW` for rules managed through this resource.
- `allowed_ip` (String) Source IP restriction, if any.

## Import

Import is supported using the following syntax, with the ID formatted as
`<server_id>:<port>:<protocol>`:

```shell
$ terraform import piensa_firewall_rule.ssh "<server_id>:22:TCP"
```