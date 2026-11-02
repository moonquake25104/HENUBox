package store

import (
	"context"
	"time"
)

// AcademicVersions are the three monotonic counters for one owner+semester.
type AcademicVersions struct {
	SettingsRevision  int64
	TimetableRevision int64
	AggregateRevision int64
}

// AcademicSettingsRow is the persisted semester configuration.
type AcademicSettingsRow struct {
	Semester            string
	Revision            int64
	SemesterLabel       *string
	TimeZone            string
	WeekAnchorDate      *time.Time
	WeekAnchorSource    string
	WeekAnchorConfirmed bool
	WeekStartDay        int32
	TotalWeeks          *int32
	PeriodCount         int32
	Periods             []byte
	UpdatedAt           time.Time
}

// AcademicPreviewRow is one persisted import preview.
type AcademicPreviewRow struct {
	ID                    string
	Owner                 string
	Semester              string
	Source                string
	Acquisition           string
	SourceSemesterLabel   *string
	SourceObtainedAt      *time.Time
	ParserVersion         string
	Status                string
	CanCommit             bool
	Payload               []byte
	Summary               []byte
	Issues                []byte
	FileSHA256            string
	FileBytes             int64
	RawPath               *string
	BaseSettingsRevision  int64
	BaseTimetableRevision int64
	BaseAggregateRevision int64
	ReceivedAt            time.Time
	CreatedAt             time.Time
	ExpiresAt             time.Time
	CommittedAt           *time.Time
	CommitID              *string
	ParentPreviewID       *string
	RootPreviewID         *string
	Resolutions           []byte
}

// EnsureAcademicVersions creates the counter row for owner+semester if needed.
func (q *Queries) EnsureAcademicVersions(ctx context.Context, owner, semester string) error {
	_, err := q.db.Exec(ctx, `
		INSERT INTO academic_versions (owner, semester)
		VALUES ($1, $2)
		ON CONFLICT (owner, semester) DO NOTHING`, owner, semester)
	return err
}

// GetAcademicVersions reads the counters, creating them when missing.
func (q *Queries) GetAcademicVersions(ctx context.Context, owner, semester string) (AcademicVersions, error) {
	if err := q.EnsureAcademicVersions(ctx, owner, semester); err != nil {
		return AcademicVersions{}, err
	}
	var v AcademicVersions
	err := q.db.QueryRow(ctx, `
		SELECT settings_revision, timetable_revision, aggregate_revision
		FROM academic_versions WHERE owner = $1 AND semester = $2`, owner, semester).
		Scan(&v.SettingsRevision, &v.TimetableRevision, &v.AggregateRevision)
	return v, err
}

// BumpAcademicVersions adds deltas and returns the new counters.
func (q *Queries) BumpAcademicVersions(ctx context.Context, owner, semester string, settingsDelta, timetableDelta, aggregateDelta int64) (AcademicVersions, error) {
	if err := q.EnsureAcademicVersions(ctx, owner, semester); err != nil {
		return AcademicVersions{}, err
	}
	var v AcademicVersions
	err := q.db.QueryRow(ctx, `
		UPDATE academic_versions
		SET settings_revision  = settings_revision + $3,
		    timetable_revision = timetable_revision + $4,
		    aggregate_revision = aggregate_revision + $5,
		    updated_at = now()
		WHERE owner = $1 AND semester = $2
		RETURNING settings_revision, timetable_revision, aggregate_revision`,
		owner, semester, settingsDelta, timetableDelta, aggregateDelta).
		Scan(&v.SettingsRevision, &v.TimetableRevision, &v.AggregateRevision)
	return v, err
}

// GetAcademicSettings returns ErrNoRows when the semester is not configured.
func (q *Queries) GetAcademicSettings(ctx context.Context, owner, semester string) (AcademicSettingsRow, error) {
	var row AcademicSettingsRow
	err := q.db.QueryRow(ctx, `
		SELECT semester, revision, semester_label, time_zone,
		       week_anchor_date, week_anchor_source, week_anchor_confirmed,
		       week_start_day, total_weeks, period_count, periods, updated_at
		FROM academic_semester_settings
		WHERE owner = $1 AND semester = $2`, owner, semester).
		Scan(&row.Semester, &row.Revision, &row.SemesterLabel, &row.TimeZone,
			&row.WeekAnchorDate, &row.WeekAnchorSource, &row.WeekAnchorConfirmed,
			&row.WeekStartDay, &row.TotalWeeks, &row.PeriodCount, &row.Periods, &row.UpdatedAt)
	return row, err
}

// InsertAcademicSettings stores the first revision of a semester configuration.
func (q *Queries) InsertAcademicSettings(ctx context.Context, owner string, row AcademicSettingsRow, anchorDate *time.Time) error {
	_, err := q.db.Exec(ctx, `
		INSERT INTO academic_semester_settings (
			owner, semester, revision, semester_label, time_zone,
			week_anchor_date, week_anchor_source, week_anchor_confirmed,
			week_start_day, total_weeks, period_count, periods
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12)`,
		owner, row.Semester, row.Revision, row.SemesterLabel, row.TimeZone,
		anchorDate, row.WeekAnchorSource, row.WeekAnchorConfirmed,
		row.WeekStartDay, row.TotalWeeks, row.PeriodCount, row.Periods)
	return err
}

// UpdateAcademicSettings replaces a configuration when the expected revision
// still matches. It returns false when the revision moved on.
func (q *Queries) UpdateAcademicSettings(ctx context.Context, owner string, row AcademicSettingsRow, expectedRevision int64, anchorDate *time.Time) (bool, error) {
	tag, err := q.db.Exec(ctx, `
		UPDATE academic_semester_settings
		SET revision = $3, semester_label = $4, time_zone = $5,
		    week_anchor_date = $6, week_anchor_source = $7, week_anchor_confirmed = $8,
		    week_start_day = $9, total_weeks = $10, period_count = $11, periods = $12,
		    updated_at = now()
		WHERE owner = $1 AND semester = $2 AND revision = $13`,
		owner, row.Semester, row.Revision, row.SemesterLabel, row.TimeZone,
		anchorDate, row.WeekAnchorSource, row.WeekAnchorConfirmed,
		row.WeekStartDay, row.TotalWeeks, row.PeriodCount, row.Periods, expectedRevision)
	if err != nil {
		return false, err
	}
	return tag.RowsAffected() == 1, nil
}

// CountCommittedBatches reports whether a source projection exists for the
// semester. Phase A never creates one; phase B will.
func (q *Queries) CountCommittedBatches(ctx context.Context, owner, semester string) (int64, error) {
	var n int64
	err := q.db.QueryRow(ctx, `
		SELECT count(*) FROM academic_import_batches
		WHERE owner = $1 AND semester = $2 AND status = 'COMMITTED'`, owner, semester).Scan(&n)
	return n, err
}

// InsertAcademicPreview persists one preview.
func (q *Queries) InsertAcademicPreview(ctx context.Context, row AcademicPreviewRow) error {
	resolutions := string(row.Resolutions)
	if resolutions == "" {
		resolutions = "[]"
	}
	_, err := q.db.Exec(ctx, `
		INSERT INTO academic_previews (
			id, owner, semester, source, acquisition, source_semester_label,
			parser_version, status, can_commit, payload, summary, issues,
			file_sha256, file_bytes, raw_path,
			base_settings_revision, base_timetable_revision, base_aggregate_revision,
			received_at, created_at, expires_at, parent_preview_id, root_preview_id, resolutions,
			source_obtained_at
		) VALUES ($1::uuid,$2,$3,$4,$5,$6,$7,$8,$9,$10::jsonb,$11::jsonb,$12::jsonb,
		          $13,$14,$15,$16,$17,$18,$19,$20,$21,$22::uuid,$23::uuid,$24::jsonb,$25)`,
		row.ID, row.Owner, row.Semester, row.Source, row.Acquisition, row.SourceSemesterLabel,
		row.ParserVersion, row.Status, row.CanCommit, string(row.Payload), string(row.Summary), string(row.Issues),
		row.FileSHA256, row.FileBytes, row.RawPath,
		row.BaseSettingsRevision, row.BaseTimetableRevision, row.BaseAggregateRevision,
		row.ReceivedAt, row.CreatedAt, row.ExpiresAt, row.ParentPreviewID, row.RootPreviewID, resolutions,
		row.SourceObtainedAt)
	return err
}

// GetAcademicPreview loads a preview owned by the caller.
func (q *Queries) GetAcademicPreview(ctx context.Context, owner, id string) (AcademicPreviewRow, error) {
	var row AcademicPreviewRow
	err := q.db.QueryRow(ctx, `
		SELECT id::text, owner, semester, source, acquisition, source_semester_label,
		       parser_version, status, can_commit, payload, summary, issues,
		       file_sha256, file_bytes, raw_path,
		       base_settings_revision, base_timetable_revision, base_aggregate_revision,
		       received_at, created_at, expires_at, committed_at, commit_id::text,
		       parent_preview_id::text, root_preview_id::text, resolutions,
		       source_obtained_at
		FROM academic_previews
		WHERE owner = $1 AND id = $2::uuid`, owner, id).
		Scan(&row.ID, &row.Owner, &row.Semester, &row.Source, &row.Acquisition, &row.SourceSemesterLabel,
			&row.ParserVersion, &row.Status, &row.CanCommit, &row.Payload, &row.Summary, &row.Issues,
			&row.FileSHA256, &row.FileBytes, &row.RawPath,
			&row.BaseSettingsRevision, &row.BaseTimetableRevision, &row.BaseAggregateRevision,
			&row.ReceivedAt, &row.CreatedAt, &row.ExpiresAt, &row.CommittedAt, &row.CommitID,
			&row.ParentPreviewID, &row.RootPreviewID, &row.Resolutions,
			&row.SourceObtainedAt)
	return row, err
}

// LockAcademicVersions reads the counters with a row lock so a caller can read
// settings and versions from one consistent snapshot.
func (q *Queries) LockAcademicVersions(ctx context.Context, owner, semester string) (AcademicVersions, error) {
	if err := q.EnsureAcademicVersions(ctx, owner, semester); err != nil {
		return AcademicVersions{}, err
	}
	var v AcademicVersions
	err := q.db.QueryRow(ctx, `
		SELECT settings_revision, timetable_revision, aggregate_revision
		FROM academic_versions
		WHERE owner = $1 AND semester = $2
		FOR UPDATE`, owner, semester).
		Scan(&v.SettingsRevision, &v.TimetableRevision, &v.AggregateRevision)
	return v, err
}

// ExpiredPreview is the minimal data needed to tombstone and reclaim one
// expired preview.
type ExpiredPreview struct {
	ID       string
	Owner    string
	Semester string
	RawPath  *string
}

// ListExpiredPreviews returns uncommitted previews past their deadline.
func (q *Queries) ListExpiredPreviews(ctx context.Context, now time.Time, limit int) ([]ExpiredPreview, error) {
	rows, err := q.db.Query(ctx, `
		SELECT id::text, owner, semester, raw_path
		FROM academic_previews
		WHERE committed_at IS NULL AND expires_at <= $1
		ORDER BY expires_at
		LIMIT $2`, now, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []ExpiredPreview
	for rows.Next() {
		var p ExpiredPreview
		if err := rows.Scan(&p.ID, &p.Owner, &p.Semester, &p.RawPath); err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

// InsertTombstone records that a preview existed and expired, keeping the
// default 30-day retention window.
func (q *Queries) InsertTombstone(ctx context.Context, id, owner, semester string, expiredAt time.Time) error {
	return q.InsertTombstoneUntil(ctx, id, owner, semester, expiredAt, expiredAt.Add(30*24*time.Hour))
}

// InsertTombstoneUntil is InsertTombstone with an explicit retention deadline.
func (q *Queries) InsertTombstoneUntil(ctx context.Context, id, owner, semester string, expiredAt, purgeAfter time.Time) error {
	_, err := q.db.Exec(ctx, `
		INSERT INTO academic_preview_tombstones (id, owner, semester, expired_at, purge_after)
		VALUES ($1::uuid, $2, $3, $4, $5)
		ON CONFLICT (id) DO NOTHING`, id, owner, semester, expiredAt, purgeAfter)
	return err
}

// GetTombstone reports whether an expired preview is known to this owner.
func (q *Queries) GetTombstone(ctx context.Context, owner, id string) (bool, error) {
	var exists bool
	err := q.db.QueryRow(ctx, `
		SELECT EXISTS (SELECT 1 FROM academic_preview_tombstones WHERE id = $1::uuid AND owner = $2)`,
		id, owner).Scan(&exists)
	return exists, err
}

// DeleteAcademicPreview removes the preview row (and therefore its payload).
func (q *Queries) DeleteAcademicPreview(ctx context.Context, id string) error {
	_, err := q.db.Exec(ctx, `DELETE FROM academic_previews WHERE id = $1::uuid`, id)
	return err
}

// EnqueuePreviewFile schedules removal of one raw upload.
func (q *Queries) EnqueuePreviewFile(ctx context.Context, previewID, owner, path string) error {
	_, err := q.db.Exec(ctx, `
		INSERT INTO academic_preview_files (preview_id, owner, path)
		VALUES ($1::uuid, $2, $3)
		ON CONFLICT (preview_id) DO NOTHING`, previewID, owner, path)
	return err
}

// PreviewFileTask is one pending raw-file deletion.
type PreviewFileTask struct {
	PreviewID string
	Owner     string
	Path      string
	Attempts  int32
}

// ListDuePreviewFiles returns file deletions that are due.
func (q *Queries) ListDuePreviewFiles(ctx context.Context, now time.Time, limit int) ([]PreviewFileTask, error) {
	rows, err := q.db.Query(ctx, `
		SELECT preview_id::text, owner, path, attempts
		FROM academic_preview_files
		WHERE next_attempt_at <= $1
		ORDER BY next_attempt_at
		LIMIT $2`, now, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []PreviewFileTask
	for rows.Next() {
		var t PreviewFileTask
		if err := rows.Scan(&t.PreviewID, &t.Owner, &t.Path, &t.Attempts); err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	return out, rows.Err()
}

// CompletePreviewFile removes a finished cleanup task.
func (q *Queries) CompletePreviewFile(ctx context.Context, previewID string) error {
	_, err := q.db.Exec(ctx, `DELETE FROM academic_preview_files WHERE preview_id = $1::uuid`, previewID)
	return err
}

// FailPreviewFile records a failed deletion so it can be retried later.
func (q *Queries) FailPreviewFile(ctx context.Context, previewID, reason string, next time.Time) error {
	_, err := q.db.Exec(ctx, `
		UPDATE academic_preview_files
		SET attempts = attempts + 1, last_error = $2, next_attempt_at = $3
		WHERE preview_id = $1::uuid`, previewID, truncate(reason, 300), next)
	return err
}

// DeleteExpiredTombstones removes tombstones whose retention window is over.
// It never touches academic_preview_files: a pending file delete outlives the
// tombstone so an expired 410 can still become 404 without leaking the upload.
func (q *Queries) DeleteExpiredTombstones(ctx context.Context, now time.Time, limit int) (int64, error) {
	tag, err := q.db.Exec(ctx, `
		DELETE FROM academic_preview_tombstones
		WHERE id IN (
			SELECT id FROM academic_preview_tombstones
			WHERE purge_after <= $1
			ORDER BY purge_after
			LIMIT $2
		)`, now, limit)
	if err != nil {
		return 0, err
	}
	return tag.RowsAffected(), nil
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n]
}
