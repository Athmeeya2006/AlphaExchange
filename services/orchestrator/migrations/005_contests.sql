-- Contest lifecycle tables (P96).
CREATE TABLE IF NOT EXISTS contests (
    id                       TEXT PRIMARY KEY,
    name                     TEXT        NOT NULL,
    start_time               TIMESTAMPTZ,
    end_time                 TIMESTAMPTZ,
    max_tests_per_contestant INT  NOT NULL DEFAULT 5,
    max_duration_seconds     INT  NOT NULL DEFAULT 300,
    max_bot_count            INT  NOT NULL DEFAULT 500,
    status                   TEXT NOT NULL DEFAULT 'scheduled',
    created_at               TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at               TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS contest_tests (
    contest_id TEXT NOT NULL REFERENCES contests(id),
    test_id    TEXT NOT NULL,
    PRIMARY KEY (contest_id, test_id)
);

CREATE TABLE IF NOT EXISTS contest_results (
    contest_id    TEXT NOT NULL REFERENCES contests(id),
    contestant_id TEXT NOT NULL,
    best_score    DOUBLE PRECISION,
    final_rank    INT,
    PRIMARY KEY (contest_id, contestant_id)
);

DROP TRIGGER IF EXISTS trg_contests_updated_at ON contests;
CREATE TRIGGER trg_contests_updated_at
    BEFORE UPDATE ON contests
    FOR EACH ROW EXECUTE FUNCTION set_updated_at();
