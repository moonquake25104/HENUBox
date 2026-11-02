-- 012: phase B2 — preview lineage, resolutions and plan snapshot.
--
-- Preview families share the original 24h deadline: rebase creates a new row
-- that points at its parent and root, it never extends the expiry.

ALTER TABLE academic_previews
    ADD COLUMN parent_preview_id uuid,
    ADD COLUMN root_preview_id   uuid,
    ADD COLUMN resolutions       jsonb NOT NULL DEFAULT '[]'::jsonb,
    ADD COLUMN plan              jsonb;

CREATE INDEX idx_academic_previews_parent
    ON academic_previews (parent_preview_id);
