-- VNC credentials per server (auto-configured via Redfish on first console open).
-- vnc_password is AES-256-GCM encrypted, same scheme as the iDRAC password.
-- vnc_port defaults to 5901 (5900 is Dell's proprietary KVM protocol).
ALTER TABLE servers ADD COLUMN vnc_password TEXT;
ALTER TABLE servers ADD COLUMN vnc_port     INTEGER NOT NULL DEFAULT 5901;
