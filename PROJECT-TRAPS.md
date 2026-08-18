# PROJECT TRAPS — dell-infra-manager

> **Every agent (Lumo/opencode, Claude, Codex) MUST read this before implementing in this repo.**
> Records this project's real, recurring failure modes so a first attempt stops re-introducing them.
> Each entry is a hard constraint. Append a bullet whenever a bug teaches a recurring trap
> (CONSTRAINT first, then why + the file it bit). Verify a path before citing it.
>
> Stack: Go (~42 `.go`) infra manager, with an install script and a small TS piece; shows connect
> info at boot; releases via GitHub Actions.

## Known traps

- **Never infer “up to date” from an empty catalog match.** Dell inventory display names are not
  stable identifiers, duplicate names are valid, and current Dell model names sit in nested
  `Model/Display` elements. Match `SoftwareId` to Dell `componentID`, parse those nested names, keep
  every unmatched row as `unknown`, and key UI rows by inventory ID. This bit
  `backend/redfish/firmware_compare.go` and `frontend/src/lib/components/FirmwareTable.svelte`.
- **Never compare iDRAC firmware history as installed firmware.** Dell FirmwareInventory includes
  `Previous-*` rollback entries and can expose the same running component as both `Current-*` and
  `Installed-*`. Exclude history and collapse only equivalent current/installed records before
  catalog comparison; otherwise an already-installed BIOS or iDRAC release appears as a warning.
  This applies to `backend/redfish/firmware.go` and `backend/redfish/firmware_compare.go`.
- **Never send `Immediate` for firmware or BIOS changes.** Dell documents that immediate firmware
  application can restart a server. The application permits only `OnReset`, and the backend must
  enforce this even if a client bypasses the dialogs. This applies to `backend/api/firmware.go`,
  `backend/redfish/firmware.go`, and `backend/api/bios.go`.
- **Never parse the full Dell catalog for each server or buffer a DUP in memory.** The LXC appliance
  has 256 MB RAM. Stream catalog decoding and firmware transfer, cache one validated catalog parse,
  and process one firmware upload at a time. This applies to `backend/redfish/catalog.go`,
  `backend/api/firmware.go`, and `backend/worker/firmware_job.go`.
- **Never treat a service restart as successful without exact-binary verification and rollback.**
  Appliance updates must verify `SHA256SUMS`, preserve the previous binary, and require `/healthz`
  to report the expected hash. The updater must never touch `/data` or call iDRAC. This applies to
  `backend/api/appliance.go` and `appliance/install-proxmox.sh`.
- **Do not return from a server poller after a permanent credential-decrypt error.** The watcher
  treats a returned poller as dead and restarts it every 10 seconds. Keep the poller dormant until
  its server row changes or the process stops. This applies to `backend/worker/pool.go`.
- **Never nest a card action button inside the card's navigation link.** Browsers and assistive
  technology handle nested interactive controls inconsistently, especially on touch devices. Use a
  non-interactive card with a separate full-card link layer and keep action buttons above that layer.
  This applies to `frontend/src/lib/components/ServerCard.svelte`.

## Working style here

- The user is a system engineer and relies on the harness to self-verify. Before "done": build/run
  the relevant check (not just `go build`), and name any runtime path you did not exercise.
