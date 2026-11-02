-- name: HasCoursesForSemester :one
SELECT EXISTS(
    SELECT 1 FROM courses WHERE semester = @semester AND deleted_at IS NULL
) AS exists;