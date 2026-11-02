-- 007: add course reminder offsets to notification_config.
ALTER TABLE notification_config
    ADD COLUMN course_offsets jsonb NOT NULL DEFAULT '[30]';