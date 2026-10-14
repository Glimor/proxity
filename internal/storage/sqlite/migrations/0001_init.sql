CREATE TABLE IF NOT EXISTS schema_migrations (
    name       TEXT PRIMARY KEY,
    applied_at TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS proxy_profiles (
    id                  TEXT PRIMARY KEY,
    name                TEXT NOT NULL,
    host                TEXT NOT NULL,
    port                INTEGER NOT NULL,
    username            TEXT NOT NULL DEFAULT '',
    password_ciphertext BLOB,
    enabled             INTEGER NOT NULL DEFAULT 1,
    timeout_ms          INTEGER NOT NULL DEFAULT 10000,
    retry_interval_ms   INTEGER NOT NULL DEFAULT 5000,
    max_retries         INTEGER NOT NULL DEFAULT 3,
    created_at          TEXT NOT NULL,
    updated_at          TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS rules (
    id              TEXT PRIMARY KEY,
    name            TEXT NOT NULL,
    kind            TEXT NOT NULL CHECK (kind IN ('exe', 'folder')),
    path            TEXT NOT NULL,
    path_fold       TEXT NOT NULL,
    action          TEXT NOT NULL CHECK (action IN ('proxy', 'direct', 'block')),
    proxy_id        TEXT NOT NULL DEFAULT '',
    enabled         INTEGER NOT NULL DEFAULT 1,
    kill_switch     INTEGER NOT NULL DEFAULT 0,
    include_subdirs INTEGER NOT NULL DEFAULT 1,
    priority        INTEGER NOT NULL DEFAULT 0,
    created_at      TEXT NOT NULL,
    updated_at      TEXT NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_rules_path_fold ON rules(path_fold);
CREATE INDEX IF NOT EXISTS idx_rules_order ON rules(kind, priority DESC);

CREATE TABLE IF NOT EXISTS settings (
    key   TEXT PRIMARY KEY,
    value TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS exe_daily_stats (
    day      TEXT NOT NULL,
    exe_path TEXT NOT NULL,
    proxy_id TEXT NOT NULL DEFAULT '',
    down     INTEGER NOT NULL DEFAULT 0,
    up       INTEGER NOT NULL DEFAULT 0,
    conns    INTEGER NOT NULL DEFAULT 0,
    seconds  INTEGER NOT NULL DEFAULT 0,
    PRIMARY KEY (day, exe_path, proxy_id)
);
CREATE INDEX IF NOT EXISTS idx_exe_daily_exe ON exe_daily_stats(exe_path);

CREATE TABLE IF NOT EXISTS proxy_daily_stats (
    day      TEXT NOT NULL,
    proxy_id TEXT NOT NULL,
    down     INTEGER NOT NULL DEFAULT 0,
    up       INTEGER NOT NULL DEFAULT 0,
    conns    INTEGER NOT NULL DEFAULT 0,
    seconds  INTEGER NOT NULL DEFAULT 0,
    PRIMARY KEY (day, proxy_id)
);

CREATE TABLE IF NOT EXISTS blocked_log (
    id       INTEGER PRIMARY KEY AUTOINCREMENT,
    ts       TEXT NOT NULL,
    exe_path TEXT NOT NULL,
    pid      INTEGER NOT NULL,
    dst      TEXT NOT NULL,
    port     INTEGER NOT NULL,
    proto    TEXT NOT NULL DEFAULT 'tcp',
    reason   TEXT NOT NULL DEFAULT ''
);
CREATE INDEX IF NOT EXISTS idx_blocked_log_ts ON blocked_log(ts);

CREATE TABLE IF NOT EXISTS session_stats (
    key   TEXT PRIMARY KEY,
    value TEXT NOT NULL
);
