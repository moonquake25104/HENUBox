package store

import (
	"context"
	"time"
)

// SourceSessionRow is one academic session joined with its source revision,
// override and legacy link. It is the only read path used by the merge rules.
type SourceSessionRow struct {
	SessionID        string
	AcademicCourseID string
	SourceEntryID    string
	Active           bool
	UpstreamKey      *string
	ContentSHA256    string
	SourceValues     []byte
	SetFields        []string
	Name             *string
	Teacher          *string
	Location         *string
	Color            *string
	Hidden           *bool
	LegacyCourseID   *int64
	Semester         string
}

// GetSourceSessionByCourseID returns the academic session linked to a legacy
// course, if any.
func (q *Queries) GetSourceSessionByCourseID(ctx context.Context, courseID int64) (SourceSessionRow, error) {
	var row SourceSessionRow
	err := q.db.QueryRow(ctx, `
		SELECT s.id::text, s.academic_course_id::text, s.source_entry_id::text, s.active,
		       e.upstream_key, r.content_sha256, r.source_values,
		       COALESCE(o.set_fields, '{}'::text[]),
		       o.name, o.teacher, o.location, o.color, o.hidden,
		       l.course_id, c.semester
		FROM academic_legacy_links l
		JOIN academic_sessions s ON s.id = l.session_id
		JOIN academic_source_entries e ON e.id = s.source_entry_id
		JOIN academic_source_revisions r ON r.id = e.current_revision_id
		LEFT JOIN academic_session_overrides o ON o.session_id = s.id
		JOIN (SELECT id, semester FROM academic_courses) c ON c.id = s.academic_course_id
		WHERE l.course_id = $1`, courseID).
		Scan(&row.SessionID, &row.AcademicCourseID, &row.SourceEntryID, &row.Active,
			&row.UpstreamKey, &row.ContentSHA256, &row.SourceValues, &row.SetFields,
			&row.Name, &row.Teacher, &row.Location, &row.Color, &row.Hidden,
			&row.LegacyCourseID, &row.Semester)
	return row, err
}

// GetSourceSessionByID is GetSourceSessionByCourseID keyed by session id.
func (q *Queries) GetSourceSessionByID(ctx context.Context, sessionID string) (SourceSessionRow, error) {
	var row SourceSessionRow
	err := q.db.QueryRow(ctx, `
		SELECT s.id::text, s.academic_course_id::text, s.source_entry_id::text, s.active,
		       e.upstream_key, r.content_sha256, r.source_values,
		       COALESCE(o.set_fields, '{}'::text[]),
		       o.name, o.teacher, o.location, o.color, o.hidden,
		       l.course_id, c.semester
		FROM academic_sessions s
		JOIN academic_source_entries e ON e.id = s.source_entry_id
		JOIN academic_source_revisions r ON r.id = e.current_revision_id
		LEFT JOIN academic_session_overrides o ON o.session_id = s.id
		LEFT JOIN academic_legacy_links l ON l.session_id = s.id
		JOIN academic_courses c ON c.id = s.academic_course_id
		WHERE s.id = $1::uuid`, sessionID).
		Scan(&row.SessionID, &row.AcademicCourseID, &row.SourceEntryID, &row.Active,
			&row.UpstreamKey, &row.ContentSHA256, &row.SourceValues, &row.SetFields,
			&row.Name, &row.Teacher, &row.Location, &row.Color, &row.Hidden,
			&row.LegacyCourseID, &row.Semester)
	return row, err
}

// GetSourceSessionForOwner loads a source session only when it belongs to the
// given owner (and the henu school). Used by all owner-facing write paths.
func (q *Queries) GetSourceSessionForOwner(ctx context.Context, owner, sessionID string) (SourceSessionRow, error) {
	var row SourceSessionRow
	err := q.db.QueryRow(ctx, `
		SELECT s.id::text, s.academic_course_id::text, s.source_entry_id::text, s.active,
		       e.upstream_key, r.content_sha256, r.source_values,
		       COALESCE(o.set_fields, '{}'::text[]),
		       o.name, o.teacher, o.location, o.color, o.hidden,
		       l.course_id, c.semester
		FROM academic_sessions s
		JOIN academic_source_entries e ON e.id = s.source_entry_id
		JOIN academic_source_revisions r ON r.id = e.current_revision_id
		LEFT JOIN academic_session_overrides o ON o.session_id = s.id
		LEFT JOIN academic_legacy_links l ON l.session_id = s.id
		JOIN academic_courses c ON c.id = s.academic_course_id
		WHERE s.id = $2::uuid AND c.owner = $1 AND c.school = 'henu'`, owner, sessionID).
		Scan(&row.SessionID, &row.AcademicCourseID, &row.SourceEntryID, &row.Active,
			&row.UpstreamKey, &row.ContentSHA256, &row.SourceValues, &row.SetFields,
			&row.Name, &row.Teacher, &row.Location, &row.Color, &row.Hidden,
			&row.LegacyCourseID, &row.Semester)
	return row, err
}

// UpsertSessionOverride replaces the explicit override set for a session.
func (q *Queries) UpsertSessionOverride(ctx context.Context, sessionID string, setFields []string, name, teacher, location, color *string, hidden *bool) error {
	_, err := q.db.Exec(ctx, `
		INSERT INTO academic_session_overrides (session_id, set_fields, name, teacher, location, color, hidden, updated_at)
		VALUES ($1::uuid, $2, $3, $4, $5, $6, $7, now())
		ON CONFLICT (session_id) DO UPDATE
		SET set_fields = EXCLUDED.set_fields,
		    name = EXCLUDED.name, teacher = EXCLUDED.teacher,
		    location = EXCLUDED.location, color = EXCLUDED.color,
		    hidden = EXCLUDED.hidden, updated_at = now()`,
		sessionID, setFields, name, teacher, location, color, hidden)
	return err
}

// DeleteSessionOverrides removes every override for a session.
func (q *Queries) DeleteSessionOverrides(ctx context.Context, sessionID string) error {
	_, err := q.db.Exec(ctx, `DELETE FROM academic_session_overrides WHERE session_id = $1::uuid`, sessionID)
	return err
}

// SessionOverrideState is the minimal override state used by projection.
type SessionOverrideState struct {
	SetFields []string
	Name      *string
	Teacher   *string
	Location  *string
	Color     *string
	Hidden    *bool
}

// UpdateLegacyProjection writes the effective values into the legacy course row.
// course. Timestamps are passed as text in "HH:MM" / "YYYY-MM-DD".
func (q *Queries) TouchCourseProjection(ctx context.Context, courseID int64, name string, dayOfWeek int32, startClock, endClock, weekPattern, semesterStartDate, location, teacher, color, semester string, hidden bool) error {
	_, err := q.db.Exec(ctx, `
		UPDATE courses
		SET name = $2, day_of_week = $3,
		    start_time = $4::time, end_time = $5::time,
		    week_pattern = $6, semester_start_date = NULLIF($7,'')::date,
		    location = $8, teacher = $9, color = $10, semester = $11,
		    deleted_at = CASE WHEN $12 THEN COALESCE(deleted_at, now()) ELSE NULL END,
		    updated_at = now()
		WHERE id = $1`, courseID, name, dayOfWeek, startClock, endClock, weekPattern,
		semesterStartDate, location, teacher, color, semester, hidden)
	return err
}

// InsertAcademicCourse stores one logical course (used by commit and tests).
func (q *Queries) InsertAcademicCourse(ctx context.Context, id, owner, semester, school, name string, courseCode, classCode, credits *string, batchID *string) error {
	_, err := q.db.Exec(ctx, `
		INSERT INTO academic_courses (id, owner, semester, school, upstream_course_code,
		    upstream_teaching_class_code, name, credits, latest_batch_id)
		VALUES ($1::uuid,$2,$3,$4,$5,$6,$7,$8,$9::uuid)`,
		id, owner, semester, school, courseCode, classCode, name, credits, batchID)
	return err
}

// InsertSourceEntry stores one upstream source identity.
func (q *Queries) InsertSourceEntry(ctx context.Context, id, owner, semester, school, courseID string, upstreamKey *string) error {
	_, err := q.db.Exec(ctx, `
		INSERT INTO academic_source_entries (id, owner, semester, school, academic_course_id, upstream_key)
		VALUES ($1::uuid,$2,$3,$4,$5::uuid,$6)`,
		id, owner, semester, school, courseID, upstreamKey)
	return err
}

// InsertSourceRevision appends an immutable revision and points the entry at it.
func (q *Queries) InsertSourceRevision(ctx context.Context, id, entryID string, batchID *string, sha string, values []byte) error {
	if _, err := q.db.Exec(ctx, `
		INSERT INTO academic_source_revisions (id, source_entry_id, import_batch_id, content_sha256, source_values)
		VALUES ($1::uuid,$2::uuid,$3::uuid,$4,$5::jsonb)`, id, entryID, batchID, sha, string(values)); err != nil {
		return err
	}
	_, err := q.db.Exec(ctx, `
		UPDATE academic_source_entries SET current_revision_id = $2::uuid, updated_at = now()
		WHERE id = $1::uuid`, entryID, id)
	return err
}

// InsertAcademicSession links a source entry to an academic course.
func (q *Queries) InsertAcademicSession(ctx context.Context, id, courseID, entryID string) error {
	_, err := q.db.Exec(ctx, `
		INSERT INTO academic_sessions (id, academic_course_id, source_entry_id)
		VALUES ($1::uuid,$2::uuid,$3::uuid)`, id, courseID, entryID)
	return err
}

// UpsertLegacyLink associates one session with one legacy course row.
func (q *Queries) UpsertLegacyLink(ctx context.Context, sessionID string, courseID int64) error {
	_, err := q.db.Exec(ctx, `
		INSERT INTO academic_legacy_links (session_id, course_id)
		VALUES ($1::uuid,$2)
		ON CONFLICT (session_id) DO UPDATE SET course_id = EXCLUDED.course_id`, sessionID, courseID)
	return err
}

// AcademicSessionState is the committed source state used for diffing.
type AcademicSessionState struct {
	SessionID      string
	CourseID       string
	SourceEntryID  string
	CourseName     string
	CourseCode     *string
	ClassCode      *string
	Credits        *string
	Active         bool
	UpstreamKey    *string
	SourceValues   []byte
	LegacyCourseID *int64
	RetiredAt      *time.Time
}

// ListAcademicState returns every committed session (active or retired) for
// owner+semester. Retired rows are needed to ask for explicit LINK on reappear.
func (q *Queries) ListAcademicState(ctx context.Context, owner, semester string) ([]AcademicSessionState, error) {
	rows, err := q.db.Query(ctx, `
		SELECT s.id::text, c.id::text, s.source_entry_id::text, c.name, c.upstream_course_code,
		       c.upstream_teaching_class_code, c.credits, s.active, e.upstream_key,
		       r.source_values, l.course_id, s.retired_at
		FROM academic_sessions s
		JOIN academic_courses c ON c.id = s.academic_course_id
		JOIN academic_source_entries e ON e.id = s.source_entry_id
		JOIN academic_source_revisions r ON r.id = e.current_revision_id
		LEFT JOIN academic_legacy_links l ON l.session_id = s.id
		WHERE c.owner = $1 AND c.semester = $2 AND c.school = 'henu'
		ORDER BY c.name, s.created_at`, owner, semester)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []AcademicSessionState
	for rows.Next() {
		var s AcademicSessionState
		if err := rows.Scan(&s.SessionID, &s.CourseID, &s.SourceEntryID, &s.CourseName, &s.CourseCode,
			&s.ClassCode, &s.Credits, &s.Active, &s.UpstreamKey, &s.SourceValues,
			&s.LegacyCourseID, &s.RetiredAt); err != nil {
			return nil, err
		}
		out = append(out, s)
	}
	return out, rows.Err()
}

// CountAcademicSourceSessions reports whether this deployment already has any
// academic source links (active, retired or hidden).
func (q *Queries) CountAcademicSourceSessions(ctx context.Context) (int64, error) {
	var n int64
	err := q.db.QueryRow(ctx, `SELECT count(*) FROM academic_sessions`).Scan(&n)
	return n, err
}

// GetSemesterSettingsRaw returns the stored periods JSON for projection.
func (q *Queries) GetSemesterSettingsRaw(ctx context.Context, owner, semester string) ([]byte, error) {
	var periods []byte
	err := q.db.QueryRow(ctx, `
		SELECT periods FROM academic_semester_settings WHERE owner = $1 AND semester = $2`,
		owner, semester).Scan(&periods)
	return periods, err
}

// LegacyCourseSemesters returns the distinct non-empty semesters of legacy
// courses (used for lock ordering before bulk date updates).
func (q *Queries) LegacyCourseSemesters(ctx context.Context) ([]string, error) {
	rows, err := q.db.Query(ctx, `SELECT DISTINCT semester FROM courses WHERE deleted_at IS NULL`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var s string
		if err := rows.Scan(&s); err != nil {
			return nil, err
		}
		out = append(out, s)
	}
	return out, rows.Err()
}

// AcademicImportBatch is the minimal batch row used by commit/replay.
type AcademicImportBatch struct {
	ID             string
	Owner          string
	Semester       string
	PreviewID      *string
	Status         string
	CommittedAt    time.Time
	Receipt        []byte
	NoChanges      bool
	IdempotencyKey *string
}
