-- name: CreateTask :one
INSERT INTO tasks (
    sync_id,
    title,
    description,
    status,
    priority,
    due_date,
    tags,
    is_daily,
    last_completed_date,
    reminder_time,
    remind_at
) VALUES (
    COALESCE(NULLIF(@sync_id::text, ''), gen_random_uuid()::text),
    @title,
    @description,
    @status,
    @priority,
    @due_date,
    @tags,
    @is_daily,
    @last_completed_date,
    @reminder_time,
    @remind_at
)
RETURNING *;

-- name: GetTask :one
SELECT * FROM tasks
WHERE id = @id AND deleted_at IS NULL;

-- name: ListTasks :many
SELECT * FROM tasks
WHERE deleted_at IS NULL
  AND (sqlc.narg('status_filter')::text IS NULL OR status = sqlc.narg('status_filter')::text)
  AND (sqlc.narg('priority_filter')::text IS NULL OR priority = sqlc.narg('priority_filter')::text)
ORDER BY created_at DESC, id DESC;

-- name: ListAllTasksForSync :many
SELECT * FROM tasks
ORDER BY id ASC;

-- name: UpdateTask :one
UPDATE tasks
SET title = @title,
    description = @description,
    status = @status,
    priority = @priority,
    due_date = @due_date,
    tags = @tags,
    is_daily = @is_daily,
    last_completed_date = @last_completed_date,
    reminder_time = @reminder_time,
    remind_at = @remind_at,
    updated_at = now()
WHERE id = @id AND deleted_at IS NULL
RETURNING *;

-- name: SoftDeleteTask :exec
UPDATE tasks
SET deleted_at = now(), updated_at = now()
WHERE id = @id AND deleted_at IS NULL;

-- name: SetTaskDailyCompleted :exec
UPDATE tasks
SET status = CASE WHEN @completed::boolean THEN 'done' ELSE 'todo' END,
    last_completed_date = CASE WHEN @completed::boolean THEN @completed_date::date ELSE NULL END,
    updated_at = now()
WHERE id = @id AND deleted_at IS NULL AND is_daily = true;
