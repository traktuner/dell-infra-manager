#!/usr/bin/env bash
#
# install-proxmox.sh — create a lightweight LXC container that runs
# dell-infra-manager. Run on any Proxmox PVE host. No assumptions about
# storage layout, network bridge naming, or CPU architecture.
#
# Quick start:
#   bash <(curl -fsSL https://raw.githubusercontent.com/traktuner/dell-infra-manager/master/appliance/install-proxmox.sh)
#
# Customise via env vars:
#   CT_HOSTNAME=dell-mgr IP=192.168.1.50/24 GATEWAY=192.168.1.1 \
#     bash <(curl -fsSL …)
#
# Everything is auto-detected by default; overrides are optional.

set -euo pipefail

# Bash inherits $HOSTNAME from the host's environment — using HOSTNAME as our
# variable name would mean the container always inherits the PVE host's name.
# Clear it once and use a dedicated variable for the container hostname.
unset HOSTNAME

# ── Defaults (everything overridable via env) ────────────────────────────────
CTID=${CTID:-}                              # blank → auto-pick next free
CT_HOSTNAME=${CT_HOSTNAME:-dell-infra-manager}
RAM_MB=${RAM_MB:-256}                       # 256 is plenty; 128 also works
SWAP_MB=${SWAP_MB:-128}
CORES=${CORES:-1}
DISK_GB=${DISK_GB:-2}                       # 2 GB is comfortable for the catalog
STORAGE=${STORAGE:-}                        # blank → auto-detect first rootdir-capable
TEMPLATE_STORAGE=${TEMPLATE_STORAGE:-}      # blank → auto-detect first vztmpl-capable
BRIDGE=${BRIDGE:-}                          # blank → auto-detect first PVE bridge (usually vmbr0)
IP=${IP:-dhcp}                              # or e.g. 192.168.1.50/24
GATEWAY=${GATEWAY:-}                        # required only if IP is static
ALPINE_VER=${ALPINE_VER:-3.23}
ONBOOT=${ONBOOT:-1}                         # 1 = autostart on Proxmox boot
UNPRIVILEGED=${UNPRIVILEGED:-1}

REPO=${REPO:-traktuner/dell-infra-manager}
APP_VERSION=${APP_VERSION:-latest}

# Detect the Proxmox host's architecture so we pull the matching binary.
# Proxmox runs on x86_64 in 99 % of cases but ARM PVE exists.
case "$(uname -m)" in
  x86_64|amd64)        BIN_ARCH=amd64 ;;
  aarch64|arm64)       BIN_ARCH=arm64 ;;
  *) echo "✗ Unsupported host architecture: $(uname -m)"; exit 1 ;;
esac

# GitHub release URL conventions:
#   latest:    /releases/latest/download/<asset>
#   pinned:    /releases/download/<tag>/<asset>
if [[ "$APP_VERSION" == "latest" ]]; then
  BINARY_URL=${BINARY_URL:-https://github.com/${REPO}/releases/latest/download/dell-infra-manager-linux-${BIN_ARCH}}
else
  BINARY_URL=${BINARY_URL:-https://github.com/${REPO}/releases/download/${APP_VERSION}/dell-infra-manager-linux-${BIN_ARCH}}
fi

# ── Sanity ───────────────────────────────────────────────────────────────────
[[ $EUID -eq 0 ]] || { echo "✗ Run as root on the Proxmox host"; exit 1; }
command -v pct      >/dev/null || { echo "✗ pct not found — not a Proxmox host?"; exit 1; }
command -v pvesh    >/dev/null || { echo "✗ pvesh not found — not a Proxmox host?"; exit 1; }
command -v pvesm    >/dev/null || { echo "✗ pvesm not found — not a Proxmox host?"; exit 1; }

# ── Storage auto-detect ──────────────────────────────────────────────────────
# Different Proxmox installs default to different storages: 'local-lvm' for the
# stock ext4+LVM setup, 'local-zfs' for ZFS-on-root, plain 'local' (dir) on
# minimal installs, plus whatever pools the user added. We pick the first
# *active* storage that supports the right content type unless the caller
# overrode STORAGE / TEMPLATE_STORAGE.
pick_storage() {
  # $1 = required content type (rootdir | vztmpl)
  pvesm status -content "$1" 2>/dev/null \
    | awk 'NR>1 && $3=="active" {print $1; exit}'
}

if [[ -z "$STORAGE" ]]; then
  STORAGE=$(pick_storage rootdir)
  [[ -n "$STORAGE" ]] || {
    echo "✗ No active container-capable storage found. Run 'pvesm status' and"
    echo "  set STORAGE=<name> manually. Available storages:"
    pvesm status
    exit 1
  }
fi
if [[ -z "$TEMPLATE_STORAGE" ]]; then
  TEMPLATE_STORAGE=$(pick_storage vztmpl)
  [[ -n "$TEMPLATE_STORAGE" ]] || {
    echo "✗ No template-capable storage found. Set TEMPLATE_STORAGE=<name>."
    exit 1
  }
fi

# ── Bridge auto-detect ───────────────────────────────────────────────────────
# vmbr0 is conventional but not guaranteed (some installs use vmbr1, OVS
# bridges, etc.). Pick the first bridge if no override.
if [[ -z "$BRIDGE" ]]; then
  BRIDGE=$(ip -br link show type bridge 2>/dev/null | awk 'NR==1 {print $1}')
  [[ -n "$BRIDGE" ]] || {
    echo "✗ No network bridge found. Configure one in Proxmox or set BRIDGE=<name>."
    exit 1
  }
fi

# ── Container ID ─────────────────────────────────────────────────────────────
if [[ -z "$CTID" ]]; then
  CTID=$(pvesh get /cluster/nextid)
fi

if pct status "$CTID" >/dev/null 2>&1; then
  echo "✗ CTID $CTID already in use. Set CTID=<other-id> and re-run."
  exit 1
fi

echo "→ Will create CTID $CTID ($CT_HOSTNAME)"
echo "  storage:    $STORAGE  (template: $TEMPLATE_STORAGE)"
echo "  resources:  ${RAM_MB} MB RAM · ${CORES} core(s) · ${DISK_GB} GB disk"
echo "  network:    bridge=$BRIDGE · ip=$IP"
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
  --hostname    "$CT_HOSTNAME" \
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

# ── Update script (in-container) ─────────────────────────────────────────────
# Installs /usr/local/bin/dell-infra-manager-update so the user can run a
# single command (from PVE host: `pct exec <CTID> -- dell-infra-manager-update`)
# to pull the newest binary, replace it atomically, restart, and rollback on
# failure. /data is never touched — DB, master key, and catalog cache survive.
#
# The URL points to /releases/latest so updates are always to the newest tag,
# regardless of which version the user originally installed.
LATEST_URL="https://github.com/${REPO}/releases/latest/download/dell-infra-manager-linux-${BIN_ARCH}"
pct exec "$CTID" -- sh -c "cat > /usr/local/bin/dell-infra-manager-update <<'UPDATE'
#!/bin/sh
# Update dell-infra-manager to the latest release. Atomic, with rollback.
# Override URL via env: \\\$BINARY_URL.
set -eu

URL=\"\${BINARY_URL:-${LATEST_URL}}\"
BIN=/opt/dell-infra-manager/dell-infra-manager
NEW=/opt/dell-infra-manager/dell-infra-manager.new
PREV=/opt/dell-infra-manager/dell-infra-manager.previous

echo \"→ Fetching: \$URL\"
wget -qO \"\$NEW\" \"\$URL\"

# Sanity-check the download — must be ELF and reasonably sized.
SIZE=\$(stat -c %s \"\$NEW\" 2>/dev/null || echo 0)
if [ \"\$SIZE\" -lt 5000000 ]; then
  echo \"✗ Downloaded file is only \$SIZE bytes — likely a 404 HTML page.\"
  rm -f \"\$NEW\"
  exit 1
fi
MAGIC=\$(head -c 4 \"\$NEW\" | od -An -c | tr -d ' \\n')
if [ \"\$MAGIC\" != '177ELF' ]; then
  echo \"✗ Downloaded file is not a Linux binary (head=\$MAGIC).\"
  rm -f \"\$NEW\"
  exit 1
fi

OLD_SIZE=\$(stat -c %s \"\$BIN\" 2>/dev/null || echo 0)
echo \"  current: \$OLD_SIZE bytes  →  new: \$SIZE bytes\"

if [ \"\$OLD_SIZE\" = \"\$SIZE\" ]; then
  if cmp -s \"\$BIN\" \"\$NEW\"; then
    echo \"✓ Already on the latest version. No change.\"
    rm -f \"\$NEW\"
    exit 0
  fi
fi

echo \"→ Restarting service with new binary…\"
cp -p \"\$BIN\" \"\$PREV\"
chmod +x \"\$NEW\"
mv \"\$NEW\" \"\$BIN\"
rc-service dell-infra-manager restart >/dev/null

# Health check — rollback if app doesn't come up within 15 s.
for i in \$(seq 1 15); do
  if wget -q --spider http://localhost:8080/api/v1/dashboard 2>/dev/null; then
    echo \"✓ Update OK. Previous binary kept at \$PREV (delete when satisfied).\"
    exit 0
  fi
  sleep 1
done

echo \"✗ Service didn't come up after update — rolling back…\"
mv \"\$PREV\" \"\$BIN\"
rc-service dell-infra-manager restart >/dev/null
echo \"  Rolled back. Run with --debug or check /var/log/dell-infra-manager.log.\"
exit 1
UPDATE
chmod +x /usr/local/bin/dell-infra-manager-update
"

# ── Console banner (/etc/issue) ──────────────────────────────────────────────
# Shows connection info on the LXC console (pct console / pct enter login).
# Re-rendered at every boot via /etc/local.d after the network is up, so the
# IP is always current.
pct exec "$CTID" -- sh -c "cat > /usr/local/bin/dell-infra-manager-banner <<'BANNER'
#!/bin/sh
# Generates /etc/issue with current connection info.
IP=\$(ip -4 -o addr show eth0 2>/dev/null | awk '{split(\$4,a,\"/\"); print a[1]; exit}')
[ -z \"\$IP\" ] && IP='(no network yet)'
HOST=\$(hostname)
ALPINE=\$(cat /etc/alpine-release 2>/dev/null || echo '?')

{
  printf '\n'
  printf '   \033[1;36mDell iDRAC Manager\033[0m   ·   Alpine %s\n' \"\$ALPINE\"
  printf '   ────────────────────────────────────────────\n'
  printf '\n'
  printf '   Web UI:    \033[1;32mhttp://%s:8080\033[0m\n' \"\$IP\"
  printf '\n'
  printf '   Hostname:  %s\n' \"\$HOST\"
  printf '   IP:        %s\n' \"\$IP\"
  printf '   Logs:      tail -f /var/log/dell-infra-manager.log\n'
  printf '   Service:   rc-service dell-infra-manager status|restart\n'
  printf '\n'
} > /etc/issue
BANNER
chmod +x /usr/local/bin/dell-infra-manager-banner

# Run at every boot, after network is up. /etc/local.d/*.start is invoked by
# the OpenRC 'local' service which depends on net.
mkdir -p /etc/local.d
ln -sf /usr/local/bin/dell-infra-manager-banner /etc/local.d/zz-banner.start
rc-update add local default >/dev/null 2>&1 || true

# Render once now so /etc/issue is correct before the first pct console.
/usr/local/bin/dell-infra-manager-banner
"

# ── Done ─────────────────────────────────────────────────────────────────────
sleep 2
IP_ADDR=$(pct exec "$CTID" -- ip -4 -o addr show eth0 | awk '{split($4,a,"/"); print a[1]}' || true)

cat <<EOF

✓ Dell iDRAC Manager appliance is up.

  CTID:        $CTID
  Hostname:    $CT_HOSTNAME
  IP:          ${IP_ADDR:-<DHCP pending — check pct exec $CTID -- ip a>}
  Web UI:      http://${IP_ADDR:-<container-ip>}:8080

Common operations:
  Logs:        pct exec $CTID -- tail -f /var/log/dell-infra-manager.log
  Restart:     pct exec $CTID -- rc-service dell-infra-manager restart
  Shell:       pct enter $CTID
  Update:      pct exec $CTID -- dell-infra-manager-update

Persistent data lives in /data inside the container — DB, master.key,
catalog cache, all settings. Updates only swap the binary; /data is never
touched, even if an update fails (the script auto-rolls-back).

Backup:      pct exec $CTID -- tar -C /data -czf - . > dell-infra-manager-data.tgz
EOF
