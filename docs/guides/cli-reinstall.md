---
page_title: "Reinstalling a server"
subcategory: "Guides"
description: |-
  Wipe a server and install a fresh OS image, seeding it with a cloud-init
  config or a first-boot bash script, using the piensa CLI.
---

# Reinstalling a server

`piensa reinstall` wipes the server and installs a fresh OS image,
optionally seeding it with a cloud-init config or a bash script that runs
on first boot. It is **destructive** — use `--dry-run` first, then confirm
or pass `--yes`.

The reinstall runs from the CLI, not the provider. This guide covers the
workflow.

## 1. Get the server ID

```sh
piensa list
```

## 2. Pick an image in the server's datacenter

```sh
piensa images <server-id>
```

`--image` accepts the exact image ID, the panel alias (`"Debian 13"`), a
short alias, or an unambiguous substring. Ambiguous input is rejected with
the matching candidates.

## 3. Dry-run

```sh
piensa reinstall <server-id> --image "Debian 13" --cloud-init init.yaml --dry-run
```

`--cloud-init` and `--script` are mutually exclusive.

## 4. Apply

With a cloud-init file — e.g. bootstrap Tailscale on first boot:

```yaml
# init.yaml
#cloud-config
runcmd:
  - curl -fsSL https://tailscale.com/install.sh | sh
  - tailscale up --auth-key tskey-auth-00000000-aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa --hostname my-vps
```

The auth key above is a placeholder — generate your own and treat it as a
secret.

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

Cloud-init and scripts are supported only on disk-image installs
(`IMAGE`); ISO and user-image paths are not implemented. Configuration is
limited to 16 KB. Omit `--password` to let the server generate one — the
CLI prints the fresh root password when the reinstall is initiated.