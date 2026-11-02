package store

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

// SelectAcademicVersions reads counters without creating the row (safe inside a
// read-only snapshot transaction).
func (q *Queries) SelectAcademicVersions(ctx context.Context, owner, semester string) (AcademicVersions, error) {
	var v AcademicVersions
	err := q.db.QueryRow(ctx, `
		SELECT settings_revision, timetable_revision, aggregate_revision
		FROM academic_versions WHERE owner = $1 AND semester = $2`, owner, semester).
		Scan(&v.SettingsRevision, &v.TimetableRevision, &v.AggregateRevision)
	return v, err
}

// LatestCommitAt returns the most recent successful commit time.
func (q *Queries) LatestCommitAt(ctx context.Context, owner, semester string) (*time.Time, error) {
	var at pgtype.Timestamptz
	err := q.db.QueryRow(ctx, `
		SELECT max(committed_at) FROM academic_import_batches
		WHERE owner = $1 AND semester = $2 AND status = 'COMMITTED'`, owner, semester).Scan(&at)
	if err != nil {
		return nil, err
	}
	if !at.Valid {
		return nil, nil
	}
	return &at.Time, nil
}

// LatestPendingPreviewID returns the newest unexpired, uncommitted preview.
func (q *Queries) LatestPendingPreviewID(ctx context.Context, owner, semester string, now time.Time) (*string, error) {
	var id *string
	err := q.db.QueryRow(ctx, `
		SELECT id::text FROM academic_previews
		WHERE owner = $1 AND semester = $2 AND committed_at IS NULL AND expires_at > $3
		ORDER BY created_at DESC LIMIT 1`, owner, semester, now).Scan(&id)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	return id, nil
}

// ListLegacyLinkedCourseIDs returns legacy course IDs that have an academic link.
func (q *Queries) ListLegacyLinkedCourseIDs(ctx context.Context) (map[int64]bool, error) {
	rows, err := q.db.Query(ctx, `SELECT course_id FROM academic_legacy_links`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[int64]bool{}
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		out[id] = true
	}
	return out, rows.Err()
}

// ClearCommittedPreviewDetails drops payload details of committed previews
// after their retention window; the batch receipt stays intact.
func (q *Queries) ClearCommittedPreviewDetails(ctx context.Context, now time.Time, limit int) (int64, error) {
	tag, err := q.db.Exec(ctx, `
		UPDATE academic_previews
		SET payload = '{}'::jsonb, summary = '{}'::jsonb, issues = '[]'::jsonb
		WHERE id IN (
			SELECT id FROM academic_previews
			WHERE committed_at IS NOT NULL AND expires_at <= $1 AND payload <> '{}'::jsonb
			ORDER BY expires_at LIMIT $2
		)`, now, limit)
	if err != nil {
		return 0, err
	}
	return tag.RowsAffected(), nil
}

// GetAcademicPreviewForUpdate loads a preview with a row lock inside the commit
// transaction, so two commits of the same preview serialise.
func (q *Queries) GetAcademicPreviewForUpdate(ctx context.Context, owner, id string) (AcademicPreviewRow, error) {
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
		WHERE owner = $1 AND id = $2::uuid
		FOR UPDATE`, owner, id).
		Scan(&row.ID, &row.Owner, &row.Semester, &row.Source, &row.Acquisition, &row.SourceSemesterLabel,
			&row.ParserVersion, &row.Status, &row.CanCommit, &row.Payload, &row.Summary, &row.Issues,
			&row.FileSHA256, &row.FileBytes, &row.RawPath,
			&row.BaseSettingsRevision, &row.BaseTimetableRevision, &row.BaseAggregateRevision,
			&row.ReceivedAt, &row.CreatedAt, &row.ExpiresAt, &row.CommittedAt, &row.CommitID,
			&row.ParentPreviewID, &row.RootPreviewID, &row.Resolutions,
			&row.SourceObtainedAt)
	return row, err
}

// MarkPreviewCommitted records the commit on the preview row.
func (q *Queries) MarkPreviewCommitted(ctx context.Context, owner, previewID, commitID string, now time.Time) error {
	_, err := q.db.Exec(ctx, `
		UPDATE academic_previews
		SET status = 'COMMITTED', committed_at = $3, commit_id = $4::uuid, can_commit = false
		WHERE owner = $1 AND id = $2::uuid`, owner, previewID, now, commitID)
	return err
}

// InsertBatch stores a new import batch. acquisition/sourceObtainedAt record
// how the source bytes were really obtained (FILE_IMPORT with a null time for
// uploads; DIRECT_FETCH with the response time for school fetches).
func (q *Queries) InsertBatch(ctx context.Context, id, owner, semester string, previewID, rootPreviewID *string, parserVersion string, settingsRevision, aggregateRevision int64, counts, receipt, decisions []byte, noChanges bool, idempotencyKey *string, acquisition string, sourceObtainedAt *time.Time) error {
	_, err := q.db.Exec(ctx, `
		INSERT INTO academic_import_batches (
			id, owner, semester, preview_id, status, parser_version, config_settings_revision,
			aggregate_revision_after, counts, receipt, decisions, root_preview_id,
			no_changes, idempotency_key, acquisition, source_obtained_at
		) VALUES ($1::uuid,$2,$3,$4::uuid,'COMMITTED',$5,$6,$7,$8::jsonb,$9::jsonb,$10::jsonb,$11::uuid,$12,$13,$14,$15)`,
		id, owner, semester, previewID, parserVersion, settingsRevision, aggregateRevision,
		string(counts), string(receipt), string(decisions), rootPreviewID, noChanges, idempotencyKey,
		acquisition, sourceObtainedAt)
	return err
}

// InsertCommitRequest stores the idempotency record for one commit key.
func (q *Queries) InsertCommitRequest(ctx context.Context, id, owner, key, requestSHA, commitID string, receipt []byte, expiresAt time.Time) error {
	_, err := q.db.Exec(ctx, `
		INSERT INTO academic_commit_requests (id, owner, idempotency_key, request_sha256, commit_id, receipt, expires_at)
		VALUES ($1::uuid,$2,$3,$4,$5::uuid,$6::jsonb,$7)`,
		id, owner, key, requestSHA, commitID, string(receipt), expiresAt)
	return err
}

// ClearPreviewRawPath forgets the raw upload after it was removed, keeping the
// preview/receipt metadata intact.
func (q *Queries) ClearPreviewRawPath(ctx context.Context, owner, previewID string) error {
	_, err := q.db.Exec(ctx, `
		UPDATE academic_previews SET raw_path = NULL WHERE owner = $1 AND id = $2::uuid`, owner, previewID)
	return err
}

// UpdateBatchResult fills the effect summary and receipt after the projection
// succeeded inside the same transaction.
func (q *Queries) UpdateBatchResult(ctx context.Context, id string, counts, receipt, decisions []byte, noChanges bool) error {
	_, err := q.db.Exec(ctx, `
		UPDATE academic_import_batches
		SET counts = $2::jsonb, receipt = $3::jsonb, decisions = $4::jsonb, no_changes = $5
		WHERE id = $1::uuid`, id, string(counts), string(receipt), string(decisions), noChanges)
	return err
}

// GetCommitRequestByKey reads an idempotency record.
func (q *Queries) GetCommitRequestByKey(ctx context.Context, owner, key string) (string, string, []byte, error) {
	var commitID, sha string
	var receipt []byte
	err := q.db.QueryRow(ctx, `
		SELECT commit_id::text, request_sha256, receipt
		FROM academic_commit_requests WHERE owner = $1 AND idempotency_key = $2`, owner, key).
		Scan(&commitID, &sha, &receipt)
	return commitID, sha, receipt, err
}

// GetCommitByID reads a stored commit receipt.
func (q *Queries) GetCommitByID(ctx context.Context, owner, commitID string) ([]byte, time.Time, error) {
	var receipt []byte
	var at time.Time
	err := q.db.QueryRow(ctx, `
		SELECT receipt, committed_at FROM academic_import_batches
		WHERE owner = $1 AND id = $2::uuid`, owner, commitID).Scan(&receipt, &at)
	return receipt, at, err
}

// UpdateAcademicCourseMetadata refreshes the latest source metadata.
func (q *Queries) UpdateAcademicCourseMetadata(ctx context.Context, id, name string, credits, code, classCode *string, batchID string) error {
	_, err := q.db.Exec(ctx, `
		UPDATE academic_courses
		SET name = $2, credits = $3, upstream_course_code = $4,
		    upstream_teaching_class_code = $5, latest_batch_id = $6::uuid, updated_at = now()
		WHERE id = $1::uuid`, id, name, credits, code, classCode, batchID)
	return err
}

// SetSessionRetired flips a source arrangement to retired (or back to active).
func (q *Queries) SetSessionRetired(ctx context.Context, sessionID string, retired bool) error {
	_, err := q.db.Exec(ctx, `
		UPDATE academic_sessions
		SET active = NOT $2, retired_at = CASE WHEN $2 THEN now() ELSE NULL END, updated_at = now()
		WHERE id = $1::uuid`, sessionID, retired)
	return err
}

// EnableSourceEntry reactivates a previously retired source identity.
func (q *Queries) EnableSourceEntry(ctx context.Context, entryID string) error {
	_, err := q.db.Exec(ctx, `
		UPDATE academic_source_entries SET active = true, updated_at = now() WHERE id = $1::uuid`, entryID)
	return err
}
