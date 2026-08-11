---
page_title: "Resource: piensa_server"
description: |-
  Read-only view of an existing PiensaSolutions VPS server.
---

# piensa_server

Read-only view of an existing PiensaSolutions VPS server — its power
state, resources, and IP addresses.

Servers are created and destroyed in the vendor panel, not through
Terraform, so this resource is **import-only**: `create` and `delete` are
no-ops. Bind it to an existing server with `terraform import`, then use
its attributes to drive other resources such as `piensa_server`-keyed
firewall rules.

## Import

Import is supported using the following syntax:

```shell
$ terraform import piensa_server.vps1 <server_id>
```

`<server_id>` is the full server UUID shown by `piensa list`; an
unambiguous prefix is also accepted.

## Schema

### Read-Only

- `cpu` (Number) Number of virtual CPUs.
- `disk` (Number) Disk size.
- `ip_addresses` (List of String) Public IP addresses.
- `name` (String) Server name set in the panel.
- `os_name` (String) Installed operating system.
- `power_state` (String) Power state reported by the hypervisor.
- `ram` (Number) Memory in GB.
- `state` (String) Overall panel state (e.g. active, suspended).
- `id` (String) The server UUID.