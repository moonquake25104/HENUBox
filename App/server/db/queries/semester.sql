-- name: UpsertSemesterContext :one
INSERT INTO semester_context (
    source,
    academic_year,
    term,
    term_label,
    start_date,
    current_week,
    total_weeks,
    refreshed_at
) VALUES (
    @source,
    @academic_year,
    @term,
    @term_label,
    @start_date,
    @current_week,
    @total_weeks,
    now()
)
ON CONFLICT (source, term_label) DO UPDATE SET
    academic_year = EXCLUDED.academic_year,
    term = EXCLUDED.term,
    start_date = EXCLUDED.start_date,
    current_week = EXCLUDED.current_week,
    total_weeks = EXCLUDED.total_weeks,
    refreshed_at = EXCLUDED.refreshed_at,
    updated_at = now()
RETURNING *;

-- name: GetSemesterContext :one
SELECT * FROM semester_context
WHERE source = @source AND term_label = @term_label;

-- name: GetLatestSemesterContextBySource :one
SELECT * FROM semester_context
WHERE source = @source
ORDER BY refreshed_at DESC, id DESC
LIMIT 1;

-- name: ListSemesterContexts :many
SELECT * FROM semester_context
ORDER BY source ASC, term_label ASC;

-- name: UpdateAllSemesterContextsStartDate :execrows
UPDATE semester_context
SET start_date = @start_date, updated_at = now();

-- name: CreateTermPhase :one
INSERT INTO term_phases (
    sync_id,
    term_label,
    phase_type,
    start_week,
    end_week,
    affects_courses,
    affects_exam_notifications,
    pomodoro_profile,
    notification_rules,
    sort_order
) VALUES (
    COALESCE(NULLIF(@sync_id::text, ''), gen_random_uuid()::text),
    @term_label,
    @phase_type,
    @start_week,
    @end_week,
    @affects_courses,
    @affects_exam_notifications,
    @pomodoro_profile,
    @notification_rules,
    @sort_order
)
RETURNING *;

-- name: GetTermPhase :one
SELECT * FROM term_phases
WHERE id = @id;

-- name: ListTermPhases :many
SELECT * FROM term_phases
WHERE term_label = @term_label AND deleted_at IS NULL
ORDER BY sort_order ASC, start_week ASC, id ASC;

-- name: ListAllTermPhases :many
SELECT * FROM term_phases
ORDER BY id ASC;

-- name: GetTermPhaseByWeek :one
SELECT * FROM term_phases
WHERE term_label = @term_label
  AND deleted_at IS NULL
  AND start_week <= @week_index
  AND end_week >= @week_index
ORDER BY sort_order ASC, start_week DESC, id DESC
LIMIT 1;

-- name: UpdateTermPhase :one
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
    updated_at = now()
WHERE id = @id AND deleted_at IS NULL
RETURNING *;

-- name: SoftDeleteTermPhase :exec
UPDATE term_phases
SET deleted_at = now(), updated_at = now()
WHERE id = @id AND deleted_at IS NULL;
