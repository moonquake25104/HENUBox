-- name: GetPomodoroConfig :one
SELECT * FROM pomodoro_config
WHERE id = 1;

-- name: UpdatePomodoroConfig :exec
UPDATE pomodoro_config
SET work_seconds = @work_seconds,
    short_break_seconds = @short_break_seconds,
    long_break_seconds = @long_break_seconds,
    sessions_before_long_break = @sessions_before_long_break,
    auto_start_next_phase = @auto_start_next_phase
WHERE id = 1;

-- name: ListPomodoroProfiles :many
SELECT * FROM pomodoro_profiles
ORDER BY is_builtin DESC, name ASC;

-- name: GetPomodoroProfile :one
SELECT * FROM pomodoro_profiles
WHERE id = @id;

-- name: CreatePomodoroProfile :one
INSERT INTO pomodoro_profiles (
    name,
    work_seconds,
    short_break_seconds,
    long_break_seconds,
    sessions_before_long_break,
    is_builtin
) VALUES (
    @name,
    @work_seconds,
    @short_break_seconds,
    @long_break_seconds,
    @sessions_before_long_break,
    false
)
RETURNING *;

-- name: UpdatePomodoroProfile :exec
UPDATE pomodoro_profiles
SET name = @name,
    work_seconds = @work_seconds,
    short_break_seconds = @short_break_seconds,
    long_break_seconds = @long_break_seconds,
    sessions_before_long_break = @sessions_before_long_break,
    updated_at = now()
WHERE id = @id AND is_builtin = false;

-- name: DeletePomodoroProfile :exec
DELETE FROM pomodoro_profiles
WHERE id = @id AND is_builtin = false;

-- name: CreatePomodoroSession :one
INSERT INTO pomodoro_sessions (
    sync_id,
    started_at,
    session_type,
    task_id
) VALUES (
    COALESCE(NULLIF(@sync_id::text, ''), gen_random_uuid()::text),
    @started_at,
    @session_type,
    @task_id
)
RETURNING *;

-- name: GetPomodoroSession :one
SELECT * FROM pomodoro_sessions
WHERE id = @id;

-- name: SetPomodoroSessionTask :exec
UPDATE pomodoro_sessions
SET task_id = @task_id
WHERE id = @id
  AND task_id IS NULL;

-- name: ListPomodoroSessions :many
SELECT * FROM pomodoro_sessions
WHERE deleted_at IS NULL
ORDER BY started_at DESC, id DESC
LIMIT @limit_value OFFSET @offset_value;

-- name: ListAllPomodoroSessionsForSync :many
SELECT * FROM pomodoro_sessions
ORDER BY id ASC;

-- name: ListPomodoroSessionsByTask :many
SELECT * FROM pomodoro_sessions
WHERE task_id = @task_id AND deleted_at IS NULL
ORDER BY started_at DESC, id DESC;

-- name: UpdatePomodoroSessionEnd :exec
UPDATE pomodoro_sessions
SET ended_at = @ended_at
WHERE id = @id;

-- name: SoftDeletePomodoroSession :exec
UPDATE pomodoro_sessions
SET deleted_at = now()
WHERE id = @id;

-- name: FindLatestOpenWorkSession :one
SELECT id FROM pomodoro_sessions
WHERE session_type = 'work'
  AND ended_at IS NULL
  AND deleted_at IS NULL
ORDER BY started_at DESC, id DESC
LIMIT 1;

-- name: CountCompletedWorkSessions :one
SELECT count(*)
FROM pomodoro_sessions
WHERE session_type = 'work'
  AND ended_at IS NOT NULL
  AND deleted_at IS NULL
  AND started_at >= @window_start
  AND started_at < @window_end;

-- name: GetRuntimeState :one
SELECT * FROM pomodoro_runtime_state
WHERE id = 1;

-- name: CreateRuntimeState :one
INSERT INTO pomodoro_runtime_state (
    id,
    date_key,
    last_seen_at
) VALUES (
    1,
    @date_key,
    now()
)
RETURNING *;

-- name: UpdateRuntimeState :exec
UPDATE pomodoro_runtime_state
SET phase = @phase,
    remaining_seconds = @remaining_seconds,
    total_seconds = @total_seconds,
    is_running = @is_running,
    active_session_id = @active_session_id,
    date_key = @date_key,
    last_seen_at = now(),
    interrupted = @interrupted,
    updated_at = now()
WHERE id = 1;

-- name: ClearRuntimeInterruption :exec
UPDATE pomodoro_runtime_state
SET interrupted = false, updated_at = now()
WHERE id = 1;
