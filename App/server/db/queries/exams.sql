-- name: CreateExam :one
INSERT INTO exams (
    sync_id,
    course_name,
    exam_datetime,
    exam_end_datetime,
    location,
    notes,
    course_id,
    semester
) VALUES (
    COALESCE(NULLIF(@sync_id::text, ''), gen_random_uuid()::text),
    @course_name,
    @exam_datetime,
    @exam_end_datetime,
    @location,
    @notes,
    @course_id,
    @semester
)
RETURNING *;

-- name: GetExam :one
SELECT * FROM exams
WHERE id = @id AND deleted_at IS NULL;

-- name: ListExams :many
SELECT * FROM exams
WHERE deleted_at IS NULL
ORDER BY exam_datetime ASC, id ASC;

-- name: ListAllExamsForSync :many
SELECT * FROM exams
ORDER BY id ASC;

-- name: UpdateExam :one
UPDATE exams
SET course_name = @course_name,
    exam_datetime = @exam_datetime,
    exam_end_datetime = @exam_end_datetime,
    location = @location,
    notes = @notes,
    course_id = @course_id,
    semester = @semester,
    updated_at = now()
WHERE id = @id AND deleted_at IS NULL
RETURNING *;

-- name: SoftDeleteExam :exec
UPDATE exams
SET deleted_at = now(), updated_at = now()
WHERE id = @id AND deleted_at IS NULL;

-- name: CountExamImportDuplicates :one
SELECT count(*)
FROM exams
WHERE deleted_at IS NULL
  AND semester = @semester
  AND course_name = @course_name
  AND exam_datetime = @exam_datetime
  AND exam_end_datetime IS NOT DISTINCT FROM @exam_end_datetime
  AND location = @location;
