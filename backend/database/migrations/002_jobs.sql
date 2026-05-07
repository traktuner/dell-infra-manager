CREATE TABLE IF NOT EXISTS jobs (
    id              TEXT PRIMARY KEY,
    server_id       TEXT NOT NULL REFERENCES servers(id) ON DELETE CASCADE,
    type            TEXT NOT NULL,
    status          TEXT NOT NULL DEFAULT 'queued',
    payload         TEXT NOT NULL DEFAULT '{}',
    result          TEXT,
    idrac_job_id    TEXT,
    created_at      DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    started_at      DATETIME,
    finished_at     DATETIME
);

CREATE INDEX IF NOT EXISTS idx_jobs_server_id ON jobs(server_id);
CREATE INDEX IF NOT EXISTS idx_jobs_status ON jobs(status);
