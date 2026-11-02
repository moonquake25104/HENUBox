-- 009: academic (HENU) phase A.
--
-- Adds only what phase A needs: semester settings, version counters, previews
-- and an (empty until phase B) import-batch table used to reject settings
-- changes that would require reprojecting committed source courses.
--
-- This migration does not touch courses/exams/tasks or any existing data.

CREATE TABLE academic_versions (
    owner               text NOT NULL,
    semester            text NOT NULL,
    settings_revision   bigint NOT NULL DEFAULT 0,
    timetable_revision  bigint NOT NULL DEFAULT 0,
    aggregate_revision  bigint NOT NULL DEFAULT 0,
    updated_at          timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (owner, semester),
    CONSTRAINT academic_versions_non_negative CHECK (
        settings_revision >= 0 AND timetable_revision >= 0 AND aggregate_revision >= 0
    )
);

CREATE TABLE academic_semester_settings (
    owner                  text NOT NULL,
    semester               text NOT NULL,
    revision               bigint NOT NULL,
    semester_label         text,
    time_zone              text NOT NULL DEFAULT 'Asia/Shanghai',
    week_anchor_date       date,
    week_anchor_source     text NOT NULL DEFAULT 'UNKNOWN',
    week_anchor_confirmed  boolean NOT NULL DEFAULT false,
    week_start_day         integer NOT NULL DEFAULT 1,
    total_weeks            integer,
    period_count           integer NOT NULL DEFAULT 13,
    periods                jsonb NOT NULL,
    created_at             timestamptz NOT NULL DEFAULT now(),
    updated_at             timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (owner, semester),
    CONSTRAINT academic_settings_period_count CHECK (period_count BETWEEN 1 AND 20),
    CONSTRAINT academic_settings_total_weeks CHECK (total_weeks IS NULL OR total_weeks BETWEEN 1 AND 60),
    CONSTRAINT academic_settings_week_start CHECK (week_start_day IN (1, 7)),
    CONSTRAINT academic_settings_anchor_source CHECK (
        week_anchor_source IN ('UNKNOWN', 'USER', 'CALENDAR', 'INFERRED')
    ),
    CONSTRAINT academic_settings_anchor_confirmed CHECK (
        week_anchor_confirmed = false OR (week_anchor_date IS NOT NULL AND week_anchor_source <> 'UNKNOWN')
    )
);

-- Phase B writes committed batches here. Phase A only reads it to refuse
-- settings changes that would invalidate an existing source projection.
CREATE TABLE academic_import_batches (
    id           uuid PRIMARY KEY,
    owner        text NOT NULL,
    semester     text NOT NULL,
    preview_id   uuid,
    status       text NOT NULL,
    committed_at timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT academic_batches_status CHECK (status IN ('COMMITTED')),
    CONSTRAINT academic_batches_unique_preview UNIQUE (owner, preview_id)
);

CREATE TABLE academic_previews (
    id                       uuid PRIMARY KEY,
    owner                    text NOT NULL,
    semester                 text NOT NULL,
    source                   text NOT NULL DEFAULT 'henu',
    acquisition              text NOT NULL DEFAULT 'FILE_IMPORT',
    source_semester_label    text,
    parser_version           text NOT NULL,
    status                   text NOT NULL,
    can_commit               boolean NOT NULL DEFAULT false,
    payload                  jsonb NOT NULL,
    summary                  jsonb NOT NULL,
    issues                   jsonb NOT NULL,
    file_sha256              text NOT NULL,
    file_bytes               bigint NOT NULL,
    raw_path                 text,
    base_settings_revision   bigint NOT NULL,
    base_timetable_revision  bigint NOT NULL,
    base_aggregate_revision  bigint NOT NULL,
    received_at              timestamptz NOT NULL,
    created_at               timestamptz NOT NULL,
    expires_at               timestamptz NOT NULL,
    committed_at             timestamptz,
    commit_id                uuid,
    CONSTRAINT academic_previews_status CHECK (
        status IN ('INVALID', 'NEEDS_REVIEW', 'NEEDS_CONFIGURATION', 'READY', 'COMMITTED')
    ),
    CONSTRAINT academic_previews_source CHECK (source IN ('henu')),
    CONSTRAINT academic_previews_acquisition CHECK (acquisition IN ('FILE_IMPORT', 'DIRECT_FETCH')),
    CONSTRAINT academic_previews_expiry CHECK (expires_at > created_at)
);

CREATE INDEX idx_academic_previews_owner_semester
    ON academic_previews (owner, semester, created_at DESC);

CREATE INDEX idx_academic_previews_expiry
    ON academic_previews (expires_at)
    WHERE committed_at IS NULL;
