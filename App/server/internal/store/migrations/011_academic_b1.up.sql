-- 011: phase B1 — source identity, immutable revisions, overrides, legacy links,
-- commit requests and extended import batches.
--
-- Additive only: existing courses/exams/tasks and A tables are untouched.

ALTER TABLE academic_import_batches
    ADD COLUMN source                text NOT NULL DEFAULT 'henu',
    ADD COLUMN parser_version        text,
    ADD COLUMN config_settings_revision bigint,
    ADD COLUMN aggregate_revision_after bigint,
    ADD COLUMN counts                jsonb NOT NULL DEFAULT '{}'::jsonb,
    ADD COLUMN receipt               jsonb NOT NULL DEFAULT '{}'::jsonb,
    ADD COLUMN decisions             jsonb NOT NULL DEFAULT '[]'::jsonb,
    ADD COLUMN root_preview_id       uuid,
    ADD COLUMN parent_preview_id     uuid,
    ADD COLUMN no_changes            boolean NOT NULL DEFAULT false,
    ADD COLUMN idempotency_key       text;

CREATE TABLE academic_courses (
    id                            uuid PRIMARY KEY,
    owner                         text NOT NULL,
    semester                      text NOT NULL,
    school                        text NOT NULL DEFAULT 'henu',
    upstream_course_code          text,
    upstream_teaching_class_code  text,
    name                          text NOT NULL,
    credits                       text,
    latest_batch_id               uuid REFERENCES academic_import_batches (id),
    created_at                    timestamptz NOT NULL DEFAULT now(),
    updated_at                    timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX idx_academic_courses_owner_semester
    ON academic_courses (owner, semester, school, name);

CREATE TABLE academic_source_entries (
    id                  uuid PRIMARY KEY,
    owner               text NOT NULL,
    semester            text NOT NULL,
    school              text NOT NULL DEFAULT 'henu',
    academic_course_id  uuid NOT NULL REFERENCES academic_courses (id),
    upstream_key        text,
    current_revision_id uuid,
    active              boolean NOT NULL DEFAULT true,
    created_at          timestamptz NOT NULL DEFAULT now(),
    updated_at          timestamptz NOT NULL DEFAULT now()
);

-- Stable upstream key is scoped by owner/school/semester; course codes alone are
-- not globally unique.
CREATE UNIQUE INDEX idx_academic_source_entries_upstream_key
    ON academic_source_entries (owner, school, semester, upstream_key)
    WHERE upstream_key IS NOT NULL;

CREATE INDEX idx_academic_source_entries_course
    ON academic_source_entries (academic_course_id);

CREATE TABLE academic_source_revisions (
    id              uuid PRIMARY KEY,
    source_entry_id uuid NOT NULL REFERENCES academic_source_entries (id),
    import_batch_id uuid REFERENCES academic_import_batches (id),
    content_sha256  text NOT NULL,
    source_values   jsonb NOT NULL,
    created_at      timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX idx_academic_source_revisions_entry
    ON academic_source_revisions (source_entry_id, created_at DESC);

-- Immutable history: revisions may be inserted, never rewritten or deleted.
-- The body is single-quoted because the migration splitter does not treat
-- dollar-quoted bodies as opaque.
CREATE OR REPLACE FUNCTION academic_source_revisions_immutable() RETURNS trigger
LANGUAGE plpgsql AS 'BEGIN RAISE EXCEPTION ''academic_source_revisions are immutable''; END;';

CREATE TRIGGER trg_academic_source_revisions_immutable
    BEFORE UPDATE OR DELETE ON academic_source_revisions
    FOR EACH ROW EXECUTE FUNCTION academic_source_revisions_immutable();

CREATE TABLE academic_sessions (
    id                uuid PRIMARY KEY,
    academic_course_id uuid NOT NULL REFERENCES academic_courses (id),
    source_entry_id   uuid NOT NULL REFERENCES academic_source_entries (id),
    active            boolean NOT NULL DEFAULT true,
    retired_at        timestamptz,
    created_at        timestamptz NOT NULL DEFAULT now(),
    updated_at        timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT academic_sessions_entry_unique UNIQUE (source_entry_id)
);

CREATE INDEX idx_academic_sessions_course ON academic_sessions (academic_course_id);

CREATE TABLE academic_session_overrides (
    session_id uuid PRIMARY KEY REFERENCES academic_sessions (id) ON DELETE CASCADE,
    set_fields text[] NOT NULL DEFAULT '{}',
    name       text,
    teacher    text,
    location   text,
    color      text,
    hidden     boolean,
    updated_at timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT academic_session_overrides_set_fields CHECK (
        set_fields <@ ARRAY['name','teacher','location','color','hidden']::text[]
    )
);

CREATE TABLE academic_legacy_links (
    session_id uuid PRIMARY KEY REFERENCES academic_sessions (id),
    course_id  bigint NOT NULL UNIQUE REFERENCES courses (id),
    created_at timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX idx_academic_legacy_links_course ON academic_legacy_links (course_id);

CREATE TABLE academic_commit_requests (
    id              uuid PRIMARY KEY,
    owner           text NOT NULL,
    idempotency_key text NOT NULL,
    request_sha256  text NOT NULL,
    commit_id       uuid NOT NULL,
    receipt         jsonb NOT NULL,
    created_at      timestamptz NOT NULL DEFAULT now(),
    expires_at      timestamptz NOT NULL,
    CONSTRAINT academic_commit_requests_unique_key UNIQUE (owner, idempotency_key)
);

CREATE INDEX idx_academic_commit_requests_commit ON academic_commit_requests (commit_id);
