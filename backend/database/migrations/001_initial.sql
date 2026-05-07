CREATE TABLE IF NOT EXISTS servers (
    id          TEXT PRIMARY KEY,
    name        TEXT NOT NULL,
    hostname    TEXT NOT NULL UNIQUE,
    port        INTEGER NOT NULL DEFAULT 443,
    username    TEXT NOT NULL,
    password    TEXT NOT NULL,
    tls_verify  BOOLEAN NOT NULL DEFAULT FALSE,
    tags        TEXT NOT NULL DEFAULT '[]',
    created_at  DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at  DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS server_cache (
    server_id           TEXT PRIMARY KEY REFERENCES servers(id) ON DELETE CASCADE,
    system_json         TEXT,
    thermal_json        TEXT,
    power_json          TEXT,
    firmware_json       TEXT,
    storage_json        TEXT,
    bios_json           TEXT,
    bios_registry_json  TEXT,
    virtualmedia_json   TEXT,
    last_seen           DATETIME,
    status              TEXT NOT NULL DEFAULT 'unknown'
);
