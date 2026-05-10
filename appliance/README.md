# Proxmox Appliance

Run dell-infra-manager as a tiny Alpine LXC container on Proxmox — completely
isolated from the Docker host that might also be one of your managed servers.

## Why LXC instead of a VM?

| | LXC (this) | VM | Dell OME (suse-based) |
|---|---|---|---|
| Boot time   | < 5 s    | ~30 s | minutes |
| Disk size   | ~80 MB   | ~1 GB | ~10 GB |
| RAM at idle | < 50 MB  | ~300 MB | 4 GB+ |
| Setup       | 1 minute | manual ISO install | manual install |

LXC shares the Proxmox host kernel, so the appliance has zero kernel boot
overhead. The actual app is a single static Go binary; everything else is
Alpine's `busybox` and OpenRC. Restoring after a host crash is just
`pct start 9999`.

## One-line install

Run on the **Proxmox PVE host shell** (not inside an existing VM):

```bash
bash <(curl -fsSL https://raw.githubusercontent.com/traktuner/dell-infra-manager/master/appliance/install-proxmox.sh)
```

The script:

1. Downloads the Alpine 3.23 LXC template (cached, ~3 MB).
2. Creates an unprivileged container with sane defaults (256 MB RAM, 1 core,
   2 GB disk, DHCP).
3. Pulls the latest dell-infra-manager static binary from GitHub Releases.
4. Sets up an OpenRC service that auto-starts on container boot.
5. Prints the IP address and Web UI URL.

You'll see something like:

```
✓ Dell iDRAC Manager appliance is up.

  CTID:        9001
  Hostname:    dell-infra-manager
  IP:          192.168.1.42
  Web UI:      http://192.168.1.42:8080
```

## Customising

All defaults can be overridden via env vars before the curl pipe:

```bash
CTID=200 \
CT_HOSTNAME=oobm \
IP=192.168.1.50/24 \
GATEWAY=192.168.1.1 \
RAM_MB=512 \
DISK_GB=4 \
STORAGE=local-zfs \
  bash <(curl -fsSL https://raw.githubusercontent.com/.../appliance/install-proxmox.sh)
```

Available variables and their defaults:

| Variable | Default | Notes |
|---|---|---|
| `CTID`             | next free CTID via `pvesh get /cluster/nextid` | container ID |
| `CT_HOSTNAME`      | `dell-infra-manager` | LXC hostname (NOT `HOSTNAME` — that's a bash builtin) |
| `RAM_MB`           | `256` | min ~128 MB works |
| `SWAP_MB`          | `128` | |
| `CORES`            | `1` | bumping doesn't help, app is mostly idle |
| `DISK_GB`          | `2` | catalog cache is ~80 MB; 2 GB is comfortable |
| `STORAGE`          | first active rootdir-capable | `local-lvm`, `local-zfs`, `local`, NFS pools, … |
| `TEMPLATE_STORAGE` | first active vztmpl-capable | usually `local` |
| `BRIDGE`           | first PVE bridge (usually `vmbr0`) | override e.g. `vmbr1`, `vmbr2` |
| `IP`               | `dhcp` | or e.g. `192.168.1.50/24` |
| `GATEWAY`          |  | required if `IP` is static |
| `ALPINE_VER`       | `3.23` | |
| `ONBOOT`           | `1` | start with Proxmox |
| `UNPRIVILEGED`     | `1` | rootless container |
| `BINARY_URL`       | latest GitHub Release for the host's CPU arch (`uname -m`) | override to pin a specific version |
| `APP_VERSION`      | `latest` | or e.g. `v0.1.0` for a pinned tag |

## Common operations

| | |
|---|---|
| Tail logs              | `pct exec <CTID> -- tail -f /var/log/dell-infra-manager.log` |
| Restart                | `pct exec <CTID> -- rc-service dell-infra-manager restart` |
| Shell into container   | `pct enter <CTID>` |
| Stop / Start           | `pct stop <CTID>` / `pct start <CTID>` |
| Destroy                | `pct destroy <CTID>` (⚠ wipes /data — back it up first) |

## Updating

Re-run the same command — it pulls the latest binary and restarts the service:

```bash
pct exec <CTID> -- sh -c '
  wget -qO /opt/dell-infra-manager/dell-infra-manager.new \
    https://github.com/traktuner/dell-infra-manager/releases/latest/download/dell-infra-manager-linux-amd64 &&
  mv /opt/dell-infra-manager/dell-infra-manager.new /opt/dell-infra-manager/dell-infra-manager &&
  chmod +x /opt/dell-infra-manager/dell-infra-manager &&
  rc-service dell-infra-manager restart
'
```

## Backup / restore

Persistent state lives in `/data` inside the container — DB, master.key,
catalog cache, configured servers (encrypted credentials).

**Backup:**

```bash
pct exec <CTID> -- tar -C /data -czf - . > dell-infra-manager-data-$(date +%F).tgz
```

**Restore on a fresh appliance:**

```bash
pct stop <CTID>
pct exec <CTID> -- sh -c 'rm -rf /data/*'
cat dell-infra-manager-data-XXX.tgz | pct exec <CTID> -- tar -C /data -xzf -
pct start <CTID>
```

## Don't put it on a managed host

The point of this appliance is independence from any single managed server.
Pick a Proxmox host that is **not** one of the iDRACs you're administering, or
that lives on a separate cluster — otherwise a hard reboot of that host kills
the tool you'd use to recover it.
