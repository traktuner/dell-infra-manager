-- SMTP / email notification configuration. Single-row table (id always = 1).
CREATE TABLE IF NOT EXISTS notification_settings (
    id                  INTEGER PRIMARY KEY CHECK (id = 1),
    enabled             BOOLEAN NOT NULL DEFAULT FALSE,
    smtp_host           TEXT    NOT NULL DEFAULT '',
    smtp_port           INTEGER NOT NULL DEFAULT 587,
    smtp_username       TEXT    NOT NULL DEFAULT '',
    smtp_password       TEXT    NOT NULL DEFAULT '',  -- AES-GCM encrypted (same scheme as iDRAC creds)
    smtp_from           TEXT    NOT NULL DEFAULT '',
    smtp_tls            TEXT    NOT NULL DEFAULT 'starttls',  -- 'none' | 'starttls' | 'tls'
    recipients          TEXT    NOT NULL DEFAULT '[]',         -- JSON array of email addresses
    on_server_offline   BOOLEAN NOT NULL DEFAULT TRUE,
    on_health_critical  BOOLEAN NOT NULL DEFAULT TRUE,
    on_job_failed       BOOLEAN NOT NULL DEFAULT TRUE,
    on_firmware_updates BOOLEAN NOT NULL DEFAULT FALSE,
    updated_at          DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);

INSERT OR IGNORE INTO notification_settings (id) VALUES (1);

-- Per-event-per-server cooldown so a flapping server doesn't email on every poll.
CREATE TABLE IF NOT EXISTS notification_dedup (
    server_id  TEXT     NOT NULL,
    event      TEXT     NOT NULL,
    sent_at    DATETIME NOT NULL,
    PRIMARY KEY (server_id, event)
);
