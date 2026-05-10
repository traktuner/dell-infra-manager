# dell-infra-manager

Self-hosted Dell PowerEdge / iDRAC9 management tool. Replaces Dell OpenManage
Enterprise for homelab and small-business setups — single static Go binary,
embedded SvelteKit UI, talks Redfish straight to iDRAC.

## Quick start — three options

### 1. Proxmox LXC appliance (recommended for resilience)

Run on your Proxmox PVE host shell:

```bash
bash <(curl -fsSL https://raw.githubusercontent.com/traktuner/dell-infra-manager/master/appliance/install-proxmox.sh)
```

Boots in ~5 seconds, ~80 MB on disk, ~50 MB RAM idle. Survives reboots of
your Docker host because it doesn't depend on it. See
[`appliance/README.md`](appliance/README.md).

### 2. Docker

```bash
docker run -d --name dell-infra-manager \
  -p 8080:8080 \
  -v /srv/dell-infra-manager:/data \
  ghcr.io/traktuner/dell-infra-manager:latest
```

### 3. Bare binary

Download from [Releases](https://github.com/traktuner/dell-infra-manager/releases)
and run from any directory containing a writable `data/` folder.

## Features

- Live Redfish polling (system / thermal / power / storage / firmware) per server
- KVM via noVNC — auto-enables iDRAC VNC over WebSocket-TCP-tunnel
- SSH Serial-Over-LAN console fallback for non-Enterprise iDRACs
- BIOS read + write with pending-changes tracking
- Firmware comparison against the official Dell catalog (`Catalog.xml.gz`),
  matching by Dell `componentID` ↔ Redfish `SoftwareId`
- Bulk power actions and bulk firmware updates across the fleet
- WebSocket push for thermal / job status / power-state changes

## Build from source

```bash
git clone https://github.com/traktuner/dell-infra-manager.git
cd dell-infra-manager
docker build -t dell-infra-manager .
```

For development:

```bash
cd backend  && go run .                      # backend on :8080
cd frontend && npm install && npm run dev    # Vite dev on :5173, proxies to :8080
```
