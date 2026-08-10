# terraform-provider-piensasolutions

CLI and Terraform provider for managing [PiensaSolutions](https://www.piensasolutions.com) VPS servers — power state, reinstalls, and firewall rules.

The API is not publicly documented; all endpoints were reverse-engineered from the vendor's web panel.

## Install

From a clone of this repository:

```sh
go build -o piensa ./cmd/piensa
```

Or download a release binary for your platform.

## Authentication

Log in interactively:

```sh
piensa login
```

Or non-interactively (e.g. in CI), via flags or environment variables:

```sh
export PIENSA_NIF=12345678Z
export PIENSA_PASSWORD='…'
export PIENSA_TOTP_SECRET='…'
piensa login
```

The session is cached in `~/.config/piensa/config.json` (mode 0600) and reused by
both the CLI and the Terraform provider until it expires (~1 hour).

## CLI

```sh
piensa list                          # list your servers
piensa fw show                       # show firewall rules for all servers
piensa fw allow <server> <port> [tcp|udp] -d "web"   # open a port
piensa fw deny  <server> <port> [tcp|udp]            # close a port
piensa images <server-id>            # list installable OS images
piensa reinstall <server-id> --image "Debian 13" --cloud-init init.yaml --yes
piensa logs <server-id>              # action history (power, restarts, reinstalls)
piensa shutdown <server-id>          # power off
piensa start <server-id>             # power on
piensa restart <server-id>           # reboot
```

Server IDs are full UUIDs; prefixes are also accepted.

## Terraform provider

Works with Terraform and OpenTofu. The provider authenticates with your
credentials directly, or reuses the session cached by `piensa login` when no
credentials are configured.

```hcl
terraform {
  required_providers {
    piensasolutions = {
      source  = "github.com/francorbacho/terraform-provider-piensasolutions"
      version = "~> 0.1"
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

### Resources

- `piensa_server` — read-only view of an existing server. Import-only (servers
  cannot be created or destroyed through the provider).
- `piensa_firewall_rule` — a port/protocol allow rule on a server.

### Example: manage firewall rules for two servers

Import the servers once, then drive all firewall rules from a single map.
Adding or removing an entry creates/denies the corresponding rule on the next
`apply`.

```hcl
resource "piensa_server" "vps1" {}
resource "piensa_server" "vps2" {}

locals {
  server_ids = {
    vps1 = piensa_server.vps1.id
    vps2 = piensa_server.vps2.id
  }

  rules = {
    "vps1:ssh"    = { server = "vps1", port = 22,   protocol = "TCP",  desc = "ssh" }
    "vps1:web"    = { server = "vps1", port = 443,  protocol = "TCP",  desc = "web" }
    "vps2:ssh"    = { server = "vps2", port = 22,   protocol = "TCP",  desc = "ssh" }
    "vps2:app"    = { server = "vps2", port = 8080, protocol = "TCP",  desc = "app" }
    "vps2:icmp"   = { server = "vps2", port = 0,    protocol = "ICMP", desc = null }
  }
}

resource "piensa_firewall_rule" "this" {
  for_each    = local.rules
  server_id   = local.server_ids[each.value.server]
  protocol    = each.value.protocol
  port        = each.value.port
  description = each.value.desc
}

# Bind the resources to your existing servers first:
#   terraform import piensa_server.vps1 <replace-with-server-id>
#   terraform import piensa_server.vps2 <replace-with-server-id>
```

Existing rules can also be adopted with `terraform import`:

```sh
terraform import 'piensa_firewall_rule.this["vps1:ssh"]' '<server-id>:22:TCP'
```

### Example: reinstall a server with a cloud-init config or init script

`piensa reinstall` wipes the server and installs a fresh OS image, optionally
seeding it with a cloud-init config or a bash script that runs on first boot.
It is destructive — use `--dry-run` first, then confirm or pass `--yes`.

1. **Get the server ID:**

   ```sh
   piensa list
   ```

2. **Pick an image** in the server's datacenter:

   ```sh
   piensa images <server-id>
   ```

   `--image` accepts the exact image ID, the panel alias (`"Debian 13"`), a
   short alias (`IF-debian-13-generic-amd64`), or an unambiguous substring.
   Ambiguous input is rejected with the matching candidates.

3. **Dry-run to see exactly what would be sent:**

   ```sh
   piensa reinstall <server-id> --image "Debian 13" --cloud-init init.yaml --dry-run
   ```

4. **Apply.** With a cloud-init file — e.g. bootstrap Tailscale on first boot:

   ```yaml
   # init.yaml
   #cloud-config
   runcmd:
     - curl -fsSL https://tailscale.com/install.sh | sh
     - tailscale up --auth-key tskey-auth-00000000-aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa --hostname my-vps
   ```

   The auth key above is a fake placeholder — generate your own with
   `tailscale up` or the admin console, and treat it as a secret.

   ```sh
   piensa reinstall <server-id> --image "Debian 13" --cloud-init init.yaml --yes
   ```

   Or with a bash init script:

   ```sh
   #!/bin/bash
   set -e
   apt-get update
   apt-get install -y nginx
   ```

   ```sh
   piensa reinstall <server-id> --image "Debian 13" --script init.sh --yes
   ```

   `--cloud-init` and `--script` are mutually exclusive. Omit `--password` to
   let the server generate one — the CLI prints the fresh root password when
   the reinstall is initiated.

## Development

Run the full check suite before opening a change:

```sh
go build ./... && go vet ./... && go test ./...
```

## License

MIT