-- name: GetSyncConfig :one
SELECT * FROM sync_config
WHERE id = 1;

-- name: UpdateSyncConfigCredentials :exec
UPDATE sync_config
SET server_url = @server_url,
    username = @username,
    password = @password,
    auto_sync = @auto_sync
WHERE id = 1;

-- name: UpdateSyncLastSyncAt :exec
UPDATE sync_config
SET last_sync_at = @last_sync_at
WHERE id = 1;

-- name: UpdateSyncRemoteEtag :exec
UPDATE sync_config
SET remote_etag = @remote_etag
WHERE id = 1;

-- name: UpdateSyncAiSettingsRemoteEtag :exec
UPDATE sync_config
SET ai_settings_remote_etag = @ai_settings_remote_etag
WHERE id = 1;

-- name: EnsureSyncDeviceIds :execrows
UPDATE sync_config
SET device_id = CASE WHEN device_id IS NULL OR device_id = '' THEN @device_id ELSE device_id END,
    dataset_id = CASE WHEN dataset_id IS NULL OR dataset_id = '' THEN @dataset_id ELSE dataset_id END
WHERE id = 1;

-- ── v2 snapshot merge queries (port of src-tauri/src/sync/exporter.rs) ──────

-- name: SyncFindTaskMeta :one
SELECT id, sync_id, updated_at, deleted_at
FROM tasks
WHERE sync_id = @sync_id;

-- name: SyncFindTaskMetaByID :one
SELECT id, sync_id, updated_at, deleted_at
FROM tasks
WHERE id = @id;

-- name: SyncInsertTask :exec
INSERT INTO tasks (
    sync_id, title, description, status, priority, due_date, tags,
    created_at, updated_at, is_daily, last_completed_date, reminder_time, remind_at, deleted_at
) VALUES (
    @sync_id, @title, @description, @status, @priority, @due_date, @tags,
    @created_at, @updated_at, @is_daily, @last_completed_date, @reminder_time, @remind_at, @deleted_at
);

-- name: SyncUpdateTask :exec
UPDATE tasks
SET title = @title,
    description = @description,
    status = @status,
    priority = @priority,
    due_date = @due_date,
    tags = @tags,
    created_at = @created_at,
    updated_at = @updated_at,
    is_daily = @is_daily,
    last_completed_date = @last_completed_date,
    reminder_time = @reminder_time,
    remind_at = @remind_at,
    deleted_at = @deleted_at,
    sync_id = @sync_id
WHERE id = @id;

-- name: SyncUpdateTaskSyncID :exec
UPDATE tasks
SET sync_id = @sync_id
WHERE id = @id;

-- name: SyncFindTaskIDBySyncID :one
SELECT id FROM tasks WHERE sync_id = @sync_id;

-- name: SyncGetTaskDailyFields :one
SELECT is_daily, last_completed_date, reminder_time
FROM tasks
WHERE id = @id;

-- name: SyncUpdateTaskDailyFields :exec
UPDATE tasks
SET is_daily = @is_daily,
    last_completed_date = @last_completed_date,
    reminder_time = @reminder_time
WHERE id = @id;

-- name: SyncFindCourseMeta :one
SELECT id, sync_id, updated_at, deleted_at
FROM courses
WHERE sync_id = @sync_id;

-- name: SyncFindCourseMetaByID :one
SELECT id, sync_id, updated_at, deleted_at
FROM courses
WHERE id = @id;

-- name: SyncInsertCourse :exec
INSERT INTO courses (
    sync_id, name, day_of_week, start_time, end_time, week_pattern, semester_start_date,
    location, teacher, color, semester, created_at, updated_at, deleted_at
) VALUES (
    @sync_id, @name, @day_of_week, @start_time, @end_time, @week_pattern, @semester_start_date,
    @location, @teacher, @color, @semester, @created_at, @updated_at, @deleted_at
);

-- name: SyncUpdateCourse :exec
UPDATE courses
SET name = @name,
    day_of_week = @day_of_week,
    start_time = @start_time,
    end_time = @end_time,
    week_pattern = @week_pattern,
    semester_start_date = @semester_start_date,
    location = @location,
    teacher = @teacher,
    color = @color,
    semester = @semester,
    created_at = @created_at,
    updated_at = @updated_at,
    deleted_at = @deleted_at,
    sync_id = @sync_id
WHERE id = @id;

-- name: SyncUpdateCourseSyncID :exec
UPDATE courses
SET sync_id = @sync_id
WHERE id = @id;

-- name: SyncFindCourseIDBySyncID :one
SELECT id FROM courses WHERE sync_id = @sync_id;

-- name: SyncFindExamMeta :one
SELECT id, sync_id, updated_at, deleted_at
FROM exams
WHERE sync_id = @sync_id;

-- name: SyncFindExamMetaByID :one
SELECT id, sync_id, updated_at, deleted_at
FROM exams
WHERE id = @id;

-- name: SyncInsertExam :exec
INSERT INTO exams (
    sync_id, course_name, exam_datetime, exam_end_datetime, location, notes, course_id, semester,
    created_at, updated_at, deleted_at
) VALUES (
    @sync_id, @course_name, @exam_datetime, @exam_end_datetime, @location, @notes, @course_id, @semester,
    @created_at, @updated_at, @deleted_at
);

-- name: SyncUpdateExam :exec
UPDATE exams
SET course_name = @course_name,
    exam_datetime = @exam_datetime,
    exam_end_datetime = @exam_end_datetime,
    location = @location,
    notes = @notes,
    course_id = @course_id,
    semester = @semester,
    created_at = @created_at,
    updated_at = @updated_at,
    deleted_at = @deleted_at,
    sync_id = @sync_id
WHERE id = @id;

-- name: SyncUpdateExamSyncID :exec
UPDATE exams
SET sync_id = @sync_id
WHERE id = @id;

-- name: SyncFindSessionMeta :one
SELECT id, sync_id, ended_at, deleted_at
FROM pomodoro_sessions
WHERE sync_id = @sync_id;

-- name: SyncFindSessionMetaByID :one
SELECT id, sync_id, ended_at, deleted_at
FROM pomodoro_sessions
WHERE id = @id;

-- name: SyncInsertSession :exec
INSERT INTO pomodoro_sessions (sync_id, started_at, ended_at, session_type, task_id, deleted_at)
VALUES (@sync_id, @started_at, @ended_at, @session_type, @task_id, @deleted_at);

-- name: SyncUpdateSession :exec
UPDATE pomodoro_sessions
SET started_at = @started_at,
    ended_at = @ended_at,
    session_type = @session_type,
    task_id = @task_id,
    deleted_at = @deleted_at,
    sync_id = @sync_id
WHERE id = @id;

-- name: SyncUpdateSessionSyncID :exec
UPDATE pomodoro_sessions
SET sync_id = @sync_id
WHERE id = @id;

-- name: SyncFindTermPhaseMeta :one
SELECT id, sync_id, updated_at, deleted_at
FROM term_phases
WHERE sync_id = @sync_id;

-- name: SyncInsertTermPhase :exec
INSERT INTO term_phases (
    sync_id, term_label, phase_type, start_week, end_week, affects_courses,
    affects_exam_notifications, pomodoro_profile, notification_rules, sort_order,
    deleted_at, created_at, updated_at
) VALUES (
    @sync_id, @term_label, @phase_type, @start_week, @end_week, @affects_courses,
    @affects_exam_notifications, @pomodoro_profile, @notification_rules, @sort_order,
    @deleted_at, @created_at, @updated_at
);

-- name: SyncUpdateTermPhase :exec
UPDATE term_phases
SET term_label = @term_label,
    phase_type = @phase_type,
    start_week = @start_week,
    end_week = @end_week,
    affects_courses = @affects_courses,
    affects_exam_notifications = @affects_exam_notifications,
    pomodoro_profile = @pomodoro_profile,
    notification_rules = @notification_rules,
    sort_order = @sort_order,
    deleted_at = @deleted_at,
    created_at = @created_at,
    updated_at = @updated_at
WHERE id = @id;
