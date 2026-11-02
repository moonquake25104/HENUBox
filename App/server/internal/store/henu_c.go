package store

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
)

// HenuConnectionRow is one owner's direct-connection state. SessionCiphertext
// is the AES-256-GCM envelope; the key never lives in this database.
type HenuConnectionRow struct {
	Owner             string
	School            string
	Generation        int64
	SessionCiphertext []byte
	State             string
	TermYear          *string
	TermTerm          *string
	TermLabel         *string
	VerifiedAt        *time.Time
	LastErrorCode     *string
	LastErrorMessage  *string
	LastErrorAt       *time.Time
}

const henuConnectionColumns = `owner, school, generation, session_ciphertext, state,
	term_year, term_term, term_label, verified_at,
	last_error_code, last_error_message, last_error_at`

func scanHenuConnection(row pgx.Row) (HenuConnectionRow, error) {
	var r HenuConnectionRow
	err := row.Scan(&r.Owner, &r.School, &r.Generation, &r.SessionCiphertext, &r.State,
		&r.TermYear, &r.TermTerm, &r.TermLabel, &r.VerifiedAt,
		&r.LastErrorCode, &r.LastErrorMessage, &r.LastErrorAt)
	return r, err
}

// GetHenuConnection reads the connection row; pgx.ErrNoRows when never connected.
func (q *Queries) GetHenuConnection(ctx context.Context, owner string) (HenuConnectionRow, error) {
	return scanHenuConnection(q.db.QueryRow(ctx,
		`SELECT `+henuConnectionColumns+` FROM henu_connections WHERE owner = $1`, owner))
}

// GetHenuConnectionForUpdate locks the row inside a transaction.
func (q *Queries) GetHenuConnectionForUpdate(ctx context.Context, owner string) (HenuConnectionRow, error) {
	return scanHenuConnection(q.db.QueryRow(ctx,
		`SELECT `+henuConnectionColumns+` FROM henu_connections WHERE owner = $1 FOR UPDATE`, owner))
}

// UpsertHenuConnected writes a newly verified session in one statement. The
// generation always advances so in-flight fetches from the previous session
// become stale.
func (q *Queries) UpsertHenuConnected(ctx context.Context, owner string, generation int64, ciphertext []byte,
	termYear, termTerm, termLabel string, verifiedAt time.Time) error {
	_, err := q.db.Exec(ctx, `
		INSERT INTO henu_connections (
			owner, school, generation, session_ciphertext, state,
			term_year, term_term, term_label, verified_at,
			last_error_code, last_error_message, last_error_at, updated_at
		) VALUES ($1, 'henu', $2, $3, 'CONNECTED', $4, $5, $6, $7, NULL, NULL, NULL, now())
		ON CONFLICT (owner) DO UPDATE SET
			generation = EXCLUDED.generation,
			session_ciphertext = EXCLUDED.session_ciphertext,
			state = 'CONNECTED',
			term_year = EXCLUDED.term_year,
			term_term = EXCLUDED.term_term,
			term_label = EXCLUDED.term_label,
			verified_at = EXCLUDED.verified_at,
			last_error_code = NULL,
			last_error_message = NULL,
			last_error_at = NULL,
			updated_at = now()`,
		owner, generation, ciphertext, termYear, termTerm, termLabel, verifiedAt)
	return err
}

// SetHenuConnectionState records a post-verification state transition with an
// optional safe error (auth required / temporary failure).
func (q *Queries) SetHenuConnectionState(ctx context.Context, owner, state, errorCode, errorMessage string, at time.Time) error {
	_, err := q.db.Exec(ctx, `
		UPDATE henu_connections
		SET state = $2,
		    last_error_code = $3,
		    last_error_message = $4,
		    last_error_at = $5,
		    updated_at = now()
		WHERE owner = $1`, owner, state, errorCode, errorMessage, at)
	return err
}

// DisconnectHenu bumps the generation and clears the session ciphertext in the
// same statement; committed timetables and overrides are untouched.
func (q *Queries) DisconnectHenu(ctx context.Context, owner string, generation int64) error {
	_, err := q.db.Exec(ctx, `
		INSERT INTO henu_connections (owner, school, generation, state, updated_at)
		VALUES ($1, 'henu', $2, 'DISCONNECTED', now())
		ON CONFLICT (owner) DO UPDATE SET
			generation = $2,
			session_ciphertext = NULL,
			state = 'DISCONNECTED',
			term_year = NULL,
			term_term = NULL,
			term_label = NULL,
			verified_at = NULL,
			updated_at = now()`, owner, generation)
	return err
}

// InsertHenuLoginSession creates one interactive login attempt.
func (q *Queries) InsertHenuLoginSession(ctx context.Context, id, owner string, generation int64,
	idempotencyKey string, now time.Time, expiresAt time.Time) error {
	_, err := q.db.Exec(ctx, `
		INSERT INTO henu_login_sessions (id, owner, generation, state, idempotency_key, created_at, expires_at)
		VALUES ($1::uuid, $2, $3, 'WAITING_FOR_USER', $4, $5, $6)`,
		id, owner, generation, idempotencyKey, now, expiresAt)
	return err
}

// GetHenuLoginSession reads one login session for the owner.
func (q *Queries) GetHenuLoginSession(ctx context.Context, owner, id string) (HenuLoginSessionRow, error) {
	return scanHenuLoginSession(q.db.QueryRow(ctx, `
		SELECT id, owner, generation, state, idempotency_key, created_at, expires_at,
		       started_at, finished_at, error_code, error_message, error_at
		FROM henu_login_sessions WHERE owner = $1 AND id = $2::uuid`, owner, id))
}

// GetHenuLoginSessionByKey reads the idempotency record for a login key.
func (q *Queries) GetHenuLoginSessionByKey(ctx context.Context, owner, key string) (HenuLoginSessionRow, error) {
	return scanHenuLoginSession(q.db.QueryRow(ctx, `
		SELECT id, owner, generation, state, idempotency_key, created_at, expires_at,
		       started_at, finished_at, error_code, error_message, error_at
		FROM henu_login_sessions WHERE owner = $1 AND idempotency_key = $2`, owner, key))
}

// HenuLoginSessionRow is one interactive login attempt.
type HenuLoginSessionRow struct {
	ID             string
	Owner          string
	Generation     int64
	State          string
	IdempotencyKey string
	CreatedAt      time.Time
	ExpiresAt      time.Time
	StartedAt      *time.Time
	FinishedAt     *time.Time
	ErrorCode      *string
	ErrorMessage   *string
	ErrorAt        *time.Time
}

func scanHenuLoginSession(row pgx.Row) (HenuLoginSessionRow, error) {
	var r HenuLoginSessionRow
	err := row.Scan(&r.ID, &r.Owner, &r.Generation, &r.State, &r.IdempotencyKey,
		&r.CreatedAt, &r.ExpiresAt, &r.StartedAt, &r.FinishedAt,
		&r.ErrorCode, &r.ErrorMessage, &r.ErrorAt)
	return r, err
}

// ActiveHenuLoginStates are the non-terminal login states.
var ActiveHenuLoginStates = []string{"WAITING_FOR_USER", "VERIFYING"}

// GetActiveHenuLogin returns the owner's single active login attempt, if any.
func (q *Queries) GetActiveHenuLogin(ctx context.Context, owner string) (*HenuLoginSessionRow, error) {
	rows, err := q.db.Query(ctx, `
		SELECT id, owner, generation, state, idempotency_key, created_at, expires_at,
		       started_at, finished_at, error_code, error_message, error_at
		FROM henu_login_sessions
		WHERE owner = $1 AND state = ANY($2)
		ORDER BY created_at DESC LIMIT 1`, owner, ActiveHenuLoginStates)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	if !rows.Next() {
		return nil, pgx.ErrNoRows
	}
	r, err := scanHenuLoginSession(rows)
	if err != nil {
		return nil, err
	}
	return &r, nil
}

// TransitionHenuLogin applies a state change only from the expected current
// state, so a cancelled/expired task can never be overwritten by a late helper.
func (q *Queries) TransitionHenuLogin(ctx context.Context, owner, id, fromState, toState string,
	errorCode, errorMessage *string, startedAt, finishedAt *time.Time) (bool, error) {
	tag, err := q.db.Exec(ctx, `
		UPDATE henu_login_sessions
		SET state = $3,
		    started_at = coalesce(started_at, $6),
		    finished_at = coalesce($7, finished_at),
		    error_code = coalesce($4, error_code),
		    error_message = coalesce($5, error_message),
		    error_at = CASE WHEN $4 IS NOT NULL THEN now() ELSE error_at END
		WHERE owner = $1 AND id = $2::uuid AND state = $8`,
		owner, id, toState, errorCode, errorMessage, startedAt, finishedAt, fromState)
	if err != nil {
		return false, err
	}
	return tag.RowsAffected() > 0, nil
}

// InsertHenuSyncJob creates one check-update task.
func (q *Queries) InsertHenuSyncJob(ctx context.Context, id, owner, semester string, generation, expectedRevision int64,
	idempotencyKey, requestSHA string) error {
	_, err := q.db.Exec(ctx, `
		INSERT INTO henu_sync_jobs (
			id, owner, semester, generation, expected_aggregate_revision,
			idempotency_key, request_sha256, state
		) VALUES ($1::uuid, $2, $3, $4, $5, $6, $7, 'QUEUED')`,
		id, owner, semester, generation, expectedRevision, idempotencyKey, requestSHA)
	return err
}

// HenuSyncJobRow is one manual check-update task.
type HenuSyncJobRow struct {
	ID               string
	Owner            string
	Semester         string
	Generation       int64
	ExpectedRevision int64
	IdempotencyKey   string
	RequestSHA256    string
	State            string
	CreatedAt        time.Time
	StartedAt        *time.Time
	FinishedAt       *time.Time
	PreviewID        *string
	FetchedAt        *time.Time
	ErrorCode        *string
	ErrorMessage     *string
	ErrorAt          *time.Time
}

const henuSyncJobColumns = `id, owner, semester, generation, expected_aggregate_revision,
	idempotency_key, request_sha256, state, created_at, started_at, finished_at,
	preview_id, fetched_at, error_code, error_message, error_at`

func scanHenuSyncJob(row pgx.Row) (HenuSyncJobRow, error) {
	var r HenuSyncJobRow
	err := row.Scan(&r.ID, &r.Owner, &r.Semester, &r.Generation, &r.ExpectedRevision,
		&r.IdempotencyKey, &r.RequestSHA256, &r.State, &r.CreatedAt, &r.StartedAt, &r.FinishedAt,
		&r.PreviewID, &r.FetchedAt, &r.ErrorCode, &r.ErrorMessage, &r.ErrorAt)
	return r, err
}

// GetHenuSyncJob reads one job for the owner.
func (q *Queries) GetHenuSyncJob(ctx context.Context, owner, id string) (HenuSyncJobRow, error) {
	return scanHenuSyncJob(q.db.QueryRow(ctx,
		`SELECT `+henuSyncJobColumns+` FROM henu_sync_jobs WHERE owner = $1 AND id = $2::uuid`, owner, id))
}

// GetHenuSyncJobByKey reads the idempotency record for a sync key.
func (q *Queries) GetHenuSyncJobByKey(ctx context.Context, owner, key string) (HenuSyncJobRow, error) {
	return scanHenuSyncJob(q.db.QueryRow(ctx,
		`SELECT `+henuSyncJobColumns+` FROM henu_sync_jobs WHERE owner = $1 AND idempotency_key = $2`, owner, key))
}

// ActiveHenuSyncStates are the non-terminal sync states.
var ActiveHenuSyncStates = []string{"QUEUED", "RUNNING"}

// GetActiveHenuSyncJob returns the owner's single active fetch task, if any.
func (q *Queries) GetActiveHenuSyncJob(ctx context.Context, owner string) (*HenuSyncJobRow, error) {
	rows, err := q.db.Query(ctx, `
		SELECT `+henuSyncJobColumns+` FROM henu_sync_jobs
		WHERE owner = $1 AND state = ANY($2)
		ORDER BY created_at DESC LIMIT 1`, owner, ActiveHenuSyncStates)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	if !rows.Next() {
		return nil, pgx.ErrNoRows
	}
	r, err := scanHenuSyncJob(rows)
	if err != nil {
		return nil, err
	}
	return &r, nil
}

// TransitionHenuSyncJob applies a guarded state change; a cancelled or
// disconnected task can never be revived by a late executor.
func (q *Queries) TransitionHenuSyncJob(ctx context.Context, owner, id, fromState, toState string,
	errorCode, errorMessage *string, startedAt, finishedAt, fetchedAt *time.Time, previewID *string) (bool, error) {
	tag, err := q.db.Exec(ctx, `
		UPDATE henu_sync_jobs
		SET state = $3,
		    started_at = coalesce(started_at, $6),
		    finished_at = coalesce($7, finished_at),
		    fetched_at = coalesce($8, fetched_at),
		    preview_id = coalesce($9::uuid, preview_id),
		    error_code = coalesce($4, error_code),
		    error_message = coalesce($5, error_message),
		    error_at = CASE WHEN $4 IS NOT NULL THEN now() ELSE error_at END
		WHERE owner = $1 AND id = $2::uuid AND state = $10`,
		owner, id, toState, errorCode, errorMessage, startedAt, finishedAt, fetchedAt, previewID, fromState)
	if err != nil {
		return false, err
	}
	return tag.RowsAffected() > 0, nil
}

// LatestFinishedHenuSyncJob returns the newest finished job for a semester so
// the timetable source_sync block can report last_checked_at / last_error.
func (q *Queries) LatestFinishedHenuSyncJob(ctx context.Context, owner, semester string) (*HenuSyncJobRow, error) {
	row := q.db.QueryRow(ctx, `
		SELECT `+henuSyncJobColumns+` FROM henu_sync_jobs
		WHERE owner = $1 AND semester = $2 AND state IN ('SUCCEEDED','FAILED','CANCELLED')
		ORDER BY coalesce(finished_at, created_at) DESC LIMIT 1`, owner, semester)
	r, err := scanHenuSyncJob(row)
	if err != nil {
		return nil, err
	}
	return &r, nil
}

// LatestCommittedBatchAcquisition returns the real acquisition metadata of the
// newest committed batch; a file import reports FILE_IMPORT with a null time.
func (q *Queries) LatestCommittedBatchAcquisition(ctx context.Context, owner, semester string) (string, *time.Time, error) {
	var acquisition string
	var obtainedAt *time.Time
	err := q.db.QueryRow(ctx, `
		SELECT acquisition, source_obtained_at FROM academic_import_batches
		WHERE owner = $1 AND semester = $2 AND status = 'COMMITTED'
		ORDER BY committed_at DESC LIMIT 1`, owner, semester).
		Scan(&acquisition, &obtainedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", nil, pgx.ErrNoRows
	}
	return acquisition, obtainedAt, err
}

// LatestHenuSyncFinishedAt returns when the owner last completed a fetch check
// for any semester; nil when no check ever finished.
func (q *Queries) LatestHenuSyncFinishedAt(ctx context.Context, owner string) (*time.Time, error) {
	var at *time.Time
	err := q.db.QueryRow(ctx, `
		SELECT max(coalesce(finished_at, created_at)) FROM henu_sync_jobs WHERE owner = $1`, owner).
		Scan(&at)
	return at, err
}

// MarkNonterminalHenuLoginsFailed is the restart recovery for login tasks.
func (q *Queries) MarkNonterminalHenuLoginsFailed(ctx context.Context, errorCode string) error {
	_, err := q.db.Exec(ctx, `
		UPDATE henu_login_sessions
		SET state = 'FAILED', error_code = $1, error_message = $1,
		    finished_at = coalesce(finished_at, now()), error_at = coalesce(error_at, now())
		WHERE state = ANY($2)`, errorCode, ActiveHenuLoginStates)
	return err
}

// MarkNonterminalHenuSyncJobsFailed is the restart recovery for fetch tasks.
func (q *Queries) MarkNonterminalHenuSyncJobsFailed(ctx context.Context, errorCode string) error {
	_, err := q.db.Exec(ctx, `
		UPDATE henu_sync_jobs
		SET state = 'FAILED', error_code = $1, error_message = $1,
		    finished_at = coalesce(finished_at, now()), error_at = coalesce(error_at, now())
		WHERE state = ANY($2)`, errorCode, ActiveHenuSyncStates)
	return err
}

// CancelOwnerHenuTasks cancels every non-terminal login and fetch of one
// owner; used by DELETE /connection inside its short transaction.
func (q *Queries) CancelOwnerHenuTasks(ctx context.Context, owner string, at time.Time) error {
	if _, err := q.db.Exec(ctx, `
		UPDATE henu_login_sessions
		SET state = 'CANCELLED', finished_at = coalesce(finished_at, $2)
		WHERE owner = $1 AND state = ANY($3)`, owner, at, ActiveHenuLoginStates); err != nil {
		return err
	}
	_, err := q.db.Exec(ctx, `
		UPDATE henu_sync_jobs
		SET state = 'CANCELLED', finished_at = coalesce(finished_at, $2)
		WHERE owner = $1 AND state = ANY($3)`, owner, at, ActiveHenuSyncStates)
	return err
}
