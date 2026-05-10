#!/usr/bin/env bash
#
# install-proxmox.sh — create a lightweight LXC container that runs
# dell-infra-manager. Run this on your Proxmox PVE host.
#
# Quick start:
#   bash <(curl -fsSL https://raw.githubusercontent.com/traktuner/dell-infra-manager/main/appliance/install-proxmox.sh)
#
# Customise via env vars:
#   CTID=200 HOSTNAME=dell-mgr IP=192.168.1.50/24 GATEWAY=192.168.1.1 \
#     bash <(curl -fsSL …)
#
# All defaults below are sensible for a homelab; everything can be overridden.

set -euo pipefail

# ── Defaults ─────────────────────────────────────────────────────────────────
CTID=${CTID:-}                              # blank → auto-pick next free
HOSTNAME=${HOSTNAME:-dell-infra-manager}
RAM_MB=${RAM_MB:-256}                       # 256 is plenty; 128 also works
SWAP_MB=${SWAP_MB:-128}
CORES=${CORES:-1}
DISK_GB=${DISK_GB:-2}                       # 2 GB is comfortable for the catalog
STORAGE=${STORAGE:-local-lvm}
TEMPLATE_STORAGE=${TEMPLATE_STORAGE:-local}
BRIDGE=${BRIDGE:-vmbr0}
IP=${IP:-dhcp}                              # or e.g. 192.168.1.50/24
GATEWAY=${GATEWAY:-}                        # required only if IP is static
ALPINE_VER=${ALPINE_VER:-3.23}
ONBOOT=${ONBOOT:-1}                         # 1 = autostart on Proxmox boot
UNPRIVILEGED=${UNPRIVILEGED:-1}

REPO=${REPO:-traktuner/dell-infra-manager}
APP_VERSION=${APP_VERSION:-latest}

# GitHub release URL conventions:
#   latest:    /releases/latest/download/<asset>
#   pinned:    /releases/download/<tag>/<asset>
if [[ "$APP_VERSION" == "latest" ]]; then
  BINARY_URL=${BINARY_URL:-https://github.com/${REPO}/releases/latest/download/dell-infra-manager-linux-amd64}
else
  BINARY_URL=${BINARY_URL:-https://github.com/${REPO}/releases/download/${APP_VERSION}/dell-infra-manager-linux-amd64}
fi

# ── Sanity ───────────────────────────────────────────────────────────────────
[[ $EUID -eq 0 ]] || { echo "✗ Run as root on the Proxmox host"; exit 1; }
command -v pct      >/dev/null || { echo "✗ pct not found — not a Proxmox host?"; exit 1; }
command -v pvesh    >/dev/null || { echo "✗ pvesh not found — not a Proxmox host?"; exit 1; }

# ── Container ID ─────────────────────────────────────────────────────────────
if [[ -z "$CTID" ]]; then
  CTID=$(pvesh get /cluster/nextid)
fi

if pct status "$CTID" >/dev/null 2>&1; then
  echo "✗ CTID $CTID already in use. Set CTID=<other-id> and re-run."
  exit 1
fi

echo "→ Will create CTID $CTID ($HOSTNAME) on $STORAGE"
echo "  RAM ${RAM_MB} MB · ${CORES} core(s) · ${DISK_GB} GB disk · IP $IP"
echo ""

# ── Alpine template ──────────────────────────────────────────────────────────
echo "→ Resolving Alpine $ALPINE_VER template…"
pveam update >/dev/null 2>&1 || true
TEMPLATE_FILE=$(pveam available --section system 2>/dev/null \
  | awk -v v="$ALPINE_VER" '$2 ~ "alpine-"v && $2 ~ "default" {print $2}' \
  | sort -V | tail -1)
[[ -n "$TEMPLATE_FILE" ]] || { echo "✗ No Alpine $ALPINE_VER template found"; exit 1; }

if ! pveam list "$TEMPLATE_STORAGE" 2>/dev/null | grep -q "$TEMPLATE_FILE"; then
  echo "  Downloading $TEMPLATE_FILE…"
  pveam download "$TEMPLATE_STORAGE" "$TEMPLATE_FILE"
else
  echo "  $TEMPLATE_FILE already cached"
fi

# ── Network args ─────────────────────────────────────────────────────────────
NET_ARGS="name=eth0,bridge=${BRIDGE},ip=${IP}"
if [[ "$IP" != "dhcp" ]]; then
  [[ -n "$GATEWAY" ]] || { echo "✗ Static IP set ($IP) but GATEWAY is empty"; exit 1; }
  NET_ARGS="${NET_ARGS},gw=${GATEWAY}"
fi

# ── Create the container ─────────────────────────────────────────────────────
echo "→ Creating LXC…"
pct create "$CTID" "${TEMPLATE_STORAGE}:vztmpl/${TEMPLATE_FILE}" \
  --hostname    "$HOSTNAME" \
  --memory      "$RAM_MB" \
  --swap        "$SWAP_MB" \
  --cores       "$CORES" \
  --rootfs      "${STORAGE}:${DISK_GB}" \
  --net0        "$NET_ARGS" \
  --unprivileged "$UNPRIVILEGED" \
  --features    keyctl=1,nesting=1 \
  --onboot      "$ONBOOT" \
  --tags        "dell-infra-manager" \
  --description "Dell iDRAC Manager — lightweight Alpine appliance. Web UI on :8080." \
  >/dev/null

echo "→ Starting container…"
pct start "$CTID"

# Wait for network in the container.
for i in $(seq 1 30); do
  if pct exec "$CTID" -- sh -c 'wget -q --spider https://github.com 2>/dev/null'; then
    break
  fi
  sleep 1
done

# ── Install dependencies + binary ────────────────────────────────────────────
echo "→ Installing app inside container…"
pct exec "$CTID" -- sh -c "
  set -e
  apk add --no-cache ca-certificates tzdata wget >/dev/null
  mkdir -p /data /opt/dell-infra-manager
  wget -qO /opt/dell-infra-manager/dell-infra-manager '${BINARY_URL}'
  chmod +x /opt/dell-infra-manager/dell-infra-manager
"

# ── OpenRC service ───────────────────────────────────────────────────────────
pct exec "$CTID" -- sh -c "cat > /etc/init.d/dell-infra-manager <<'EOF'
#!/sbin/openrc-run
name=\"dell-infra-manager\"
description=\"Dell iDRAC Manager\"
command=\"/opt/dell-infra-manager/dell-infra-manager\"
command_background=\"yes\"
pidfile=\"/run/dell-infra-manager.pid\"
output_log=\"/var/log/dell-infra-manager.log\"
error_log=\"/var/log/dell-infra-manager.log\"
directory=\"/data\"
depend() {
  need net localmount
  after firewall
}
EOF
chmod +x /etc/init.d/dell-infra-manager
rc-update add dell-infra-manager default >/dev/null
rc-service dell-infra-manager start >/dev/null
"

# ── Done ─────────────────────────────────────────────────────────────────────
sleep 2
IP_ADDR=$(pct exec "$CTID" -- ip -4 -o addr show eth0 | awk '{split($4,a,"/"); print a[1]}' || true)

cat <<EOF

✓ Dell iDRAC Manager appliance is up.

  CTID:        $CTID
  Hostname:    $HOSTNAME
  IP:          ${IP_ADDR:-<DHCP pending — check pct exec $CTID -- ip a>}
  Web UI:      http://${IP_ADDR:-<container-ip>}:8080

Common operations:
  Logs:        pct exec $CTID -- tail -f /var/log/dell-infra-manager.log
  Restart:     pct exec $CTID -- rc-service dell-infra-manager restart
  Shell:       pct enter $CTID
  Update:      pct exec $CTID -- sh -c 'wget -qO /opt/dell-infra-manager/dell-infra-manager.new "$BINARY_URL" && \\
                 mv /opt/dell-infra-manager/dell-infra-manager.new /opt/dell-infra-manager/dell-infra-manager && \\
                 chmod +x /opt/dell-infra-manager/dell-infra-manager && \\
                 rc-service dell-infra-manager restart'

Persistent data lives in /data inside the container (DB, master.key, catalog cache).
Back it up with:  pct exec $CTID -- tar czf - /data > dell-infra-manager-data.tgz
EOF
