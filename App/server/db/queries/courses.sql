-- name: CreateCourse :one
INSERT INTO courses (
    sync_id,
    name,
    day_of_week,
    start_time,
    end_time,
    week_pattern,
    semester_start_date,
    location,
    teacher,
    color,
    semester
) VALUES (
    COALESCE(NULLIF(@sync_id::text, ''), gen_random_uuid()::text),
    @name,
    @day_of_week,
    @start_time,
    @end_time,
    @week_pattern,
    @semester_start_date,
    @location,
    @teacher,
    @color,
    @semester
)
RETURNING *;

-- name: GetCourse :one
SELECT * FROM courses
WHERE id = @id AND deleted_at IS NULL;

-- name: ListCourses :many
SELECT * FROM courses
WHERE deleted_at IS NULL
  AND (sqlc.narg('semester')::text IS NULL OR semester = sqlc.narg('semester')::text)
ORDER BY day_of_week ASC, start_time ASC, id ASC;

-- name: ListAllCoursesForSync :many
SELECT * FROM courses
ORDER BY id ASC;

-- name: UpdateCourse :one
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
    updated_at = now()
WHERE id = @id AND deleted_at IS NULL
RETURNING *;

-- name: SoftDeleteCourse :exec
UPDATE courses
SET deleted_at = now(), updated_at = now()
WHERE id = @id AND deleted_at IS NULL;

-- name: UpdateAllCoursesSemesterStartDate :execrows
UPDATE courses
SET semester_start_date = @semester_start_date, updated_at = now()
WHERE deleted_at IS NULL;

-- name: CountCourseImportDuplicates :one
SELECT count(*)
FROM courses
WHERE deleted_at IS NULL
  AND semester = @semester
  AND name = @name
  AND day_of_week = @day_of_week
  AND start_time = @start_time
  AND end_time = @end_time
  AND week_pattern = @week_pattern
  AND semester_start_date IS NOT DISTINCT FROM @semester_start_date
  AND location = @location
  AND teacher = @teacher;
