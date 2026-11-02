-- name: GetNotificationConfig :one
SELECT * FROM notification_config
WHERE id = 1;

-- name: UpdateNotificationConfig :exec
UPDATE notification_config
SET enabled = @enabled,
    exam_offsets = @exam_offsets,
    course_offsets = @course_offsets,
    updated_at = now()
WHERE id = 1;
