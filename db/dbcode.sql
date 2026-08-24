CREATE TABLE IF NOT EXISTS users (
    id             SERIAL PRIMARY KEY,
    username       TEXT UNIQUE NOT NULL,
    password_hash  TEXT NOT NULL,
    points         INTEGER NOT NULL DEFAULT 0
);

CREATE TABLE IF NOT EXISTS logs (
    id          BIGSERIAL PRIMARY KEY,
    username    TEXT        NOT NULL,
    level       TEXT        NOT NULL,
    message     TEXT        NOT NULL,
    source      TEXT        NOT NULL,
    logged_at   TIMESTAMPTZ NOT NULL,
    received_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- Logs are written in bulk and read newest-first, so the read path gets an
-- index while the write path stays a plain append.
CREATE INDEX IF NOT EXISTS logs_logged_at_idx ON logs (logged_at DESC);