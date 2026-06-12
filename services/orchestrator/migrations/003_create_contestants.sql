CREATE TABLE IF NOT EXISTS contestants (
    id         TEXT PRIMARY KEY,
    name       TEXT        NOT NULL,
    email      TEXT UNIQUE NOT NULL,
    api_key    TEXT UNIQUE NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_contestants_api_key ON contestants (api_key);
