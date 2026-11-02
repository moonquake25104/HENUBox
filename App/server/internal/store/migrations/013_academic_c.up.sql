-- 013: phase C — local interactive authorization and manual direct fetch.
--
-- Additive only: existing academic tables keep their rows; the two new columns
-- carry defaults that backfill file imports as FILE_IMPORT with a null fetch
-- time, never a fabricated one.

-- Real acquisition metadata for committed batches backs the timetable
-- source_sync block; previews gain the same column so a rebase can preserve it.
ALTER TABLE academic_import_batches
    ADD COLUMN acquisition         text NOT NULL DEFAULT 'FILE_IMPORT',
    ADD COLUMN source_obtained_at  timestamptz;

ALTER TABLE academic_previews
    ADD COLUMN source_obtained_at  timestamptz;

-- One row per owner: the school is fixed to henu for this table. The session
-- ciphertext (cookies + internal subject data) is an AES-256-GCM envelope
-- keyed outside the database; plaintext credentials never land here.
CREATE TABLE henu_connections (
    owner                text PRIMARY KEY,
    school               text NOT NULL DEFAULT 'henu',
    generation           bigint NOT NULL DEFAULT 0,
    session_ciphertext   bytea,
    state                text NOT NULL DEFAULT 'DISCONNECTED',
    term_year            text,
    term_term            text,
    term_label           text,
    verified_at          timestamptz,
    last_error_code      text,
    last_error_message   text,
    last_error_at        timestamptz,
    created_at           timestamptz NOT NULL DEFAULT now(),
    updated_at           timestamptz NOT NULL DEFAULT now()
);

-- One interactive login attempt. Late helper output after cancel/timeout is
-- rejected via the generation + terminal-state checks on the API side.
CREATE TABLE henu_login_sessions (
    id              uuid PRIMARY KEY,
    owner           text NOT NULL,
    generation      bigint NOT NULL DEFAULT 0,
    state           text NOT NULL,
    idempotency_key text NOT NULL,
    created_at      timestamptz NOT NULL DEFAULT now(),
    expires_at      timestamptz NOT NULL,
    started_at      timestamptz,
    finished_at     timestamptz,
    error_code      text,
    error_message   text,
    error_at        timestamptz,
    CONSTRAINT henu_login_sessions_owner_key UNIQUE (owner, idempotency_key)
);

CREATE INDEX idx_henu_login_sessions_owner ON henu_login_sessions (owner, created_at DESC);

-- One manual check-update task per key. source_sync.last_checked_at /
-- last_error for a semester are derived from the newest finished row here.
CREATE TABLE henu_sync_jobs (
    id                         uuid PRIMARY KEY,
    owner                      text NOT NULL,
    semester                   text NOT NULL,
    generation                 bigint NOT NULL DEFAULT 0,
    expected_aggregate_revision bigint NOT NULL,
    idempotency_key            text NOT NULL,
    request_sha256             text NOT NULL,
    state                      text NOT NULL,
    created_at                 timestamptz NOT NULL DEFAULT now(),
    started_at                 timestamptz,
    finished_at                timestamptz,
    preview_id                 uuid,
    fetched_at                 timestamptz,
    error_code                 text,
    error_message              text,
    error_at                   timestamptz,
    CONSTRAINT henu_sync_jobs_owner_key UNIQUE (owner, idempotency_key)
);

CREATE INDEX idx_henu_sync_jobs_owner_semester ON henu_sync_jobs (owner, semester, created_at DESC);
