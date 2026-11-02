-- 001: singleton config tables (pomodoro_config, sync_config, notification_config, ai_config).
-- Seed each singleton row idempotently (ON CONFLICT DO NOTHING).

CREATE TABLE pomodoro_config (
    id                          bigint PRIMARY KEY DEFAULT 1 CHECK (id = 1),
    work_seconds                integer NOT NULL DEFAULT 1500,
    short_break_seconds         integer NOT NULL DEFAULT 300,
    long_break_seconds          integer NOT NULL DEFAULT 900,
    sessions_before_long_break  integer NOT NULL DEFAULT 4,
    auto_start_next_phase       boolean NOT NULL DEFAULT false
);

CREATE TABLE sync_config (
    id                      bigint PRIMARY KEY DEFAULT 1 CHECK (id = 1),
    server_url              text NOT NULL DEFAULT '',
    username                text NOT NULL DEFAULT '',
    password                text NOT NULL DEFAULT '',
    auto_sync               boolean NOT NULL DEFAULT false,
    last_sync_at            timestamptz,
    remote_etag             text,
    device_id               text,
    dataset_id              text,
    ai_settings_remote_etag text
);

CREATE TABLE notification_config (
    id           bigint PRIMARY KEY DEFAULT 1 CHECK (id = 1),
    enabled      boolean NOT NULL DEFAULT true,
    exam_offsets jsonb NOT NULL DEFAULT '[1440, 60]',
    created_at   timestamptz NOT NULL DEFAULT now(),
    updated_at   timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE ai_config (
    id                 bigint PRIMARY KEY DEFAULT 1 CHECK (id = 1),
    enabled            boolean NOT NULL DEFAULT false,
    base_url           text NOT NULL DEFAULT 'https://api.deepseek.com',
    model              text NOT NULL DEFAULT 'deepseek-v4-flash',
    api_key_encrypted  text NOT NULL DEFAULT '',
    sync_enabled       boolean NOT NULL DEFAULT false,
    created_at         timestamptz NOT NULL DEFAULT now(),
    updated_at         timestamptz NOT NULL DEFAULT now()
);

INSERT INTO pomodoro_config (id) VALUES (1) ON CONFLICT DO NOTHING;
INSERT INTO sync_config (id) VALUES (1) ON CONFLICT DO NOTHING;
INSERT INTO notification_config (id) VALUES (1) ON CONFLICT DO NOTHING;
INSERT INTO ai_config (id) VALUES (1) ON CONFLICT DO NOTHING;
