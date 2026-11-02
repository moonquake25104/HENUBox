-- name: GetAiConfig :one
SELECT * FROM ai_config
WHERE id = 1;

-- name: UpdateAiConfig :exec
UPDATE ai_config
SET enabled = @enabled,
    base_url = @base_url,
    model = @model,
    api_key_encrypted = @api_key_encrypted,
    sync_enabled = @sync_enabled,
    updated_at = now()
WHERE id = 1;

-- name: ApplySyncedAiConfig :exec
UPDATE ai_config
SET enabled = @enabled,
    base_url = @base_url,
    model = @model,
    api_key_encrypted = @api_key_encrypted,
    updated_at = @updated_at
WHERE id = 1;

-- name: GetMorningBrief :one
SELECT * FROM ai_morning_brief
WHERE date = @date;

-- name: UpsertMorningBrief :one
INSERT INTO ai_morning_brief (
    date,
    markdown,
    source,
    model,
    generated_at
) VALUES (
    @date,
    @markdown,
    @source,
    @model,
    @generated_at
)
ON CONFLICT (date) DO UPDATE SET
    markdown = EXCLUDED.markdown,
    source = EXCLUDED.source,
    model = EXCLUDED.model,
    generated_at = EXCLUDED.generated_at,
    updated_at = now()
RETURNING *;
