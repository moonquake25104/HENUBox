-- 010: phase A review fixes — expiry tombstones and a retryable file-reclaim queue.
--
-- 009 is already applied on the disposable QA schema, so this is a separate
-- migration rather than an edit. Additive only.

CREATE TABLE academic_preview_tombstones (
    id          uuid PRIMARY KEY,
    owner       text NOT NULL,
    semester    text NOT NULL,
    expired_at  timestamptz NOT NULL,
    purge_after timestamptz NOT NULL DEFAULT now() + interval '30 days'
);

CREATE INDEX idx_academic_preview_tombstones_owner
    ON academic_preview_tombstones (owner, expired_at);

-- One row per preview whose raw upload still needs to be removed from disk.
-- Rows survive process restarts so a failed delete can be retried.
CREATE TABLE academic_preview_files (
    preview_id      uuid PRIMARY KEY,
    owner           text NOT NULL,
    path            text NOT NULL,
    attempts        integer NOT NULL DEFAULT 0,
    last_error      text,
    next_attempt_at timestamptz NOT NULL DEFAULT now(),
    created_at      timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT academic_preview_files_attempts CHECK (attempts >= 0)
);

CREATE INDEX idx_academic_preview_files_due
    ON academic_preview_files (next_attempt_at);
