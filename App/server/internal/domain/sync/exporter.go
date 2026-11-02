package sync

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"kairos/server/internal/store"
)

// ChinaTZ is the fixed +08:00 zone used for the frontend's local date/time
// strings (remind_at, reminder_time, due_date) inside the snapshot.
var ChinaTZ = time.FixedZone("CST", 8*3600)

// ExportAll reads every entity (including tombstones) from the store and
// serializes it into a v2 snapshot. AI tables are never exported.
func ExportAll(ctx context.Context, q *store.Queries) (*SyncData, error) {
	if err := EnsureDeviceIDs(ctx, q); err != nil {
		return nil, err
	}
	cfg, err := q.GetSyncConfig(ctx)
	if err != nil {
		return nil, err
	}
	tasks, err := q.ListAllTasksForSync(ctx)
	if err != nil {
		return nil, err
	}
	courses, err := q.ListAllCoursesForSync(ctx)
	if err != nil {
		return nil, err
	}
	exams, err := q.ListAllExamsForSync(ctx)
	if err != nil {
		return nil, err
	}
	sessions, err := q.ListAllPomodoroSessionsForSync(ctx)
	if err != nil {
		return nil, err
	}
	phases, err := q.ListAllTermPhases(ctx)
	if err != nil {
		return nil, err
	}

	return &SyncData{
		SchemaVersion:    2,
		DatasetID:        store.TextString(cfg.DatasetID),
		DeviceID:         store.TextString(cfg.DeviceID),
		Tasks:            tasksFromStore(tasks),
		Courses:          coursesFromStore(courses),
		Exams:            examsFromStore(exams),
		PomodoroSessions: sessionsFromStore(sessions),
		TermPhases:       termPhasesFromStore(phases),
		ExportedAt:       time.Now().UTC().Format("2006-01-02T15:04:05Z"),
	}, nil
}

// ImportAll merges a remote snapshot into the local database inside a single
// transaction. q must be bound to a transaction. Matching key is sync_id
// (v1 fallback: local id); the winner is decided by effective_timestamp =
// deleted_at ?? updated_at; tombstones propagate via deleted_at.
func ImportAll(ctx context.Context, q *store.Queries, data *SyncData) (SyncStats, error) {
	tasksMerged, err := mergeTasks(ctx, q, data.Tasks)
	if err != nil {
		return SyncStats{}, err
	}
	taskIDMap, err := buildEntityIDMap(ctx, taskIDPairs(data.Tasks), func(ctx context.Context, syncID string) (int64, error) {
		return q.SyncFindTaskIDBySyncID(ctx, store.TextOf(syncID))
	})
	if err != nil {
		return SyncStats{}, err
	}
	coursesMerged, err := mergeCourses(ctx, q, data.Courses)
	if err != nil {
		return SyncStats{}, err
	}
	courseIDMap, err := buildEntityIDMap(ctx, courseIDPairs(data.Courses), func(ctx context.Context, syncID string) (int64, error) {
		return q.SyncFindCourseIDBySyncID(ctx, store.TextOf(syncID))
	})
	if err != nil {
		return SyncStats{}, err
	}
	examsMerged, err := mergeExams(ctx, q, data.Exams, courseIDMap)
	if err != nil {
		return SyncStats{}, err
	}
	sessionsMerged, err := mergeSessions(ctx, q, data.PomodoroSessions, taskIDMap)
	if err != nil {
		return SyncStats{}, err
	}
	termPhasesMerged, err := mergeTermPhases(ctx, q, data.TermPhases)
	if err != nil {
		return SyncStats{}, err
	}

	return SyncStats{
		TasksMerged:      tasksMerged,
		CoursesMerged:    coursesMerged,
		ExamsMerged:      examsMerged,
		SessionsMerged:   sessionsMerged,
		TermPhasesMerged: termPhasesMerged,
		Conflicts: saturatingSub(len(data.Tasks), tasksMerged) +
			saturatingSub(len(data.Courses), coursesMerged) +
			saturatingSub(len(data.Exams), examsMerged) +
			saturatingSub(len(data.PomodoroSessions), sessionsMerged) +
			saturatingSub(len(data.TermPhases), termPhasesMerged),
	}, nil
}

// EnsureDeviceIDs backfills device_id/dataset_id on first use, mirroring the
// Rust get_sync_config default row creation.
func EnsureDeviceIDs(ctx context.Context, q *store.Queries) error {
	_, err := q.EnsureSyncDeviceIds(ctx, store.EnsureSyncDeviceIdsParams{
		DeviceID:  store.TextOf(newSyncID()),
		DatasetID: store.TextOf(newSyncID()),
	})
	return err
}

// ─── merge helpers ─────────────────────────────────────────────────────────

type localMeta struct {
	ID        int64
	SyncID    string
	UpdatedAt string
	DeletedAt *string
}

func (m *localMeta) effective() string {
	return effectiveTimestamp(m.UpdatedAt, m.DeletedAt)
}

// effectiveTimestamp prefers the tombstone: deleted_at wins over updated_at so
// a delete from another device can cover a locally active entity.
func effectiveTimestamp(updatedAt string, deletedAt *string) string {
	if deletedAt != nil && *deletedAt != "" {
		return *deletedAt
	}
	return updatedAt
}

// resolveMerge applies the remote update when it is newer; otherwise it
// backfills the local sync_id when it differs. Returns true when a change was
// applied (update or sync_id backfill), which counts toward merged.
func resolveMerge(ctx context.Context, q *store.Queries, table string, local *localMeta, remoteSyncID string, remoteIsNewer bool, doUpdate func() error) (bool, error) {
	if remoteIsNewer {
		if err := doUpdate(); err != nil {
			return false, err
		}
		return true, nil
	}
	if local.SyncID != remoteSyncID {
		if err := updateLocalSyncID(ctx, q, table, local.ID, remoteSyncID); err != nil {
			return false, err
		}
		return true, nil
	}
	return false, nil
}

func updateLocalSyncID(ctx context.Context, q *store.Queries, table string, localID int64, syncID string) error {
	switch table {
	case "tasks":
		return q.SyncUpdateTaskSyncID(ctx, store.SyncUpdateTaskSyncIDParams{ID: localID, SyncID: store.TextOf(syncID)})
	case "courses":
		return q.SyncUpdateCourseSyncID(ctx, store.SyncUpdateCourseSyncIDParams{ID: localID, SyncID: store.TextOf(syncID)})
	case "exams":
		return q.SyncUpdateExamSyncID(ctx, store.SyncUpdateExamSyncIDParams{ID: localID, SyncID: store.TextOf(syncID)})
	case "pomodoro_sessions":
		return q.SyncUpdateSessionSyncID(ctx, store.SyncUpdateSessionSyncIDParams{ID: localID, SyncID: store.TextOf(syncID)})
	}
	return fmt.Errorf("unknown sync table %q", table)
}

type idSyncPair struct {
	remoteID int64
	syncID   string
}

func taskIDPairs(tasks []Task) []idSyncPair {
	pairs := make([]idSyncPair, 0, len(tasks))
	for _, t := range tasks {
		pairs = append(pairs, idSyncPair{remoteID: t.ID, syncID: normalizeTask(t).SyncID})
	}
	return pairs
}

func courseIDPairs(courses []Course) []idSyncPair {
	pairs := make([]idSyncPair, 0, len(courses))
	for _, c := range courses {
		pairs = append(pairs, idSyncPair{remoteID: c.ID, syncID: normalizeCourse(c).SyncID})
	}
	return pairs
}

func buildEntityIDMap(ctx context.Context, pairs []idSyncPair, lookup func(context.Context, string) (int64, error)) (map[int64]int64, error) {
	m := make(map[int64]int64, len(pairs))
	for _, p := range pairs {
		localID, err := lookup(ctx, p.syncID)
		if errors.Is(err, pgx.ErrNoRows) {
			continue
		}
		if err != nil {
			return nil, err
		}
		m[p.remoteID] = localID
	}
	return m, nil
}

func saturatingSub(total, merged int) int {
	if merged > total {
		return 0
	}
	return total - merged
}

// ─── tasks ─────────────────────────────────────────────────────────────────

func mergeTasks(ctx context.Context, q *store.Queries, remote []Task) (int, error) {
	merged := 0
	for _, t := range remote {
		isLegacy := t.SyncID == ""
		remote := normalizeTask(t)
		local, err := findTaskMeta(ctx, q, remote.SyncID, remote.ID, isLegacy)
		if err != nil {
			return 0, err
		}
		if local == nil {
			if err := q.SyncInsertTask(ctx, taskInsertParams(remote)); err != nil {
				return 0, err
			}
			merged++
			continue
		}
		remoteEff := effectiveTimestamp(remote.UpdatedAt, remote.DeletedAt)
		newer := remoteEff > local.effective()
		applied, err := resolveMerge(ctx, q, "tasks", local, remote.SyncID, newer, func() error {
			return q.SyncUpdateTask(ctx, taskUpdateParams(remote, local.ID))
		})
		if err != nil {
			return 0, err
		}
		if applied {
			merged++
		} else if remoteEff == local.effective() && local.DeletedAt == nil && remote.DeletedAt == nil {
			// LWW tie at second precision: merge the daily-task fields with a
			// non-default-wins rule so daily markers converge across devices.
			if err := mergeDailyFields(ctx, q, local.ID, &remote); err != nil {
				return 0, err
			}
		}
	}
	return merged, nil
}

func findTaskMeta(ctx context.Context, q *store.Queries, syncID string, remoteID int64, allowIDFallback bool) (*localMeta, error) {
	row, err := q.SyncFindTaskMeta(ctx, store.TextOf(syncID))
	if err == nil {
		return taskMetaFromRow(row), nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return nil, err
	}
	if !allowIDFallback {
		return nil, nil
	}
	byID, err := q.SyncFindTaskMetaByID(ctx, remoteID)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return taskMetaFromByIDRow(byID), nil
}

func taskMetaFromRow(row store.SyncFindTaskMetaRow) *localMeta {
	m := &localMeta{ID: row.ID, SyncID: store.TextString(row.SyncID), UpdatedAt: store.TSString(row.UpdatedAt)}
	if row.DeletedAt.Valid {
		s := store.TSString(row.DeletedAt)
		m.DeletedAt = &s
	}
	return m
}

func taskMetaFromByIDRow(row store.SyncFindTaskMetaByIDRow) *localMeta {
	m := &localMeta{ID: row.ID, SyncID: store.TextString(row.SyncID), UpdatedAt: store.TSString(row.UpdatedAt)}
	if row.DeletedAt.Valid {
		s := store.TSString(row.DeletedAt)
		m.DeletedAt = &s
	}
	return m
}

func mergeDailyFields(ctx context.Context, q *store.Queries, localID int64, remote *Task) error {
	fields, err := q.SyncGetTaskDailyFields(ctx, localID)
	if err != nil {
		return err
	}
	newIsDaily := fields.IsDaily || remote.IsDaily
	newLastCompleted := fields.LastCompletedDate
	if !newLastCompleted.Valid && remote.LastCompletedDate != nil {
		newLastCompleted = store.DateOf(*remote.LastCompletedDate)
	}
	newReminder := fields.ReminderTime
	if !newReminder.Valid && remote.ReminderTime != nil {
		newReminder = store.ClockOf(*remote.ReminderTime)
	}
	if newIsDaily == fields.IsDaily &&
		store.DateString(newLastCompleted) == store.DateString(fields.LastCompletedDate) &&
		store.ClockString(newReminder) == store.ClockString(fields.ReminderTime) {
		return nil
	}
	return q.SyncUpdateTaskDailyFields(ctx, store.SyncUpdateTaskDailyFieldsParams{
		IsDaily:           newIsDaily,
		LastCompletedDate: newLastCompleted,
		ReminderTime:      newReminder,
		ID:                localID,
	})
}

// ─── courses ───────────────────────────────────────────────────────────────

func mergeCourses(ctx context.Context, q *store.Queries, remote []Course) (int, error) {
	merged := 0
	for _, c := range remote {
		isLegacy := c.SyncID == ""
		remote := normalizeCourse(c)
		local, err := findCourseMeta(ctx, q, remote.SyncID, remote.ID, isLegacy)
		if err != nil {
			return 0, err
		}
		if local == nil {
			if err := q.SyncInsertCourse(ctx, courseInsertParams(remote)); err != nil {
				return 0, err
			}
			merged++
			continue
		}
		newer := effectiveTimestamp(remote.UpdatedAt, remote.DeletedAt) > local.effective()
		applied, err := resolveMerge(ctx, q, "courses", local, remote.SyncID, newer, func() error {
			return q.SyncUpdateCourse(ctx, courseUpdateParams(remote, local.ID))
		})
		if err != nil {
			return 0, err
		}
		if applied {
			merged++
		}
	}
	return merged, nil
}

func findCourseMeta(ctx context.Context, q *store.Queries, syncID string, remoteID int64, allowIDFallback bool) (*localMeta, error) {
	row, err := q.SyncFindCourseMeta(ctx, store.TextOf(syncID))
	if err == nil {
		return courseMetaFromRow(row), nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return nil, err
	}
	if !allowIDFallback {
		return nil, nil
	}
	byID, err := q.SyncFindCourseMetaByID(ctx, remoteID)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return courseMetaFromByIDRow(byID), nil
}

func courseMetaFromRow(row store.SyncFindCourseMetaRow) *localMeta {
	m := &localMeta{ID: row.ID, SyncID: store.TextString(row.SyncID), UpdatedAt: store.TSString(row.UpdatedAt)}
	if row.DeletedAt.Valid {
		s := store.TSString(row.DeletedAt)
		m.DeletedAt = &s
	}
	return m
}

func courseMetaFromByIDRow(row store.SyncFindCourseMetaByIDRow) *localMeta {
	m := &localMeta{ID: row.ID, SyncID: store.TextString(row.SyncID), UpdatedAt: store.TSString(row.UpdatedAt)}
	if row.DeletedAt.Valid {
		s := store.TSString(row.DeletedAt)
		m.DeletedAt = &s
	}
	return m
}

// ─── exams ─────────────────────────────────────────────────────────────────

func mergeExams(ctx context.Context, q *store.Queries, remote []Exam, courseIDMap map[int64]int64) (int, error) {
	merged := 0
	for _, e := range remote {
		isLegacy := e.SyncID == ""
		remote := normalizeExam(e)
		courseID := mapRemoteEntityID(courseIDMap, remote.CourseID)
		local, err := findExamMeta(ctx, q, remote.SyncID, remote.ID, isLegacy)
		if err != nil {
			return 0, err
		}
		if local == nil {
			if err := q.SyncInsertExam(ctx, examInsertParams(remote, courseID)); err != nil {
				return 0, err
			}
			merged++
			continue
		}
		newer := effectiveTimestamp(remote.UpdatedAt, remote.DeletedAt) > local.effective()
		applied, err := resolveMerge(ctx, q, "exams", local, remote.SyncID, newer, func() error {
			return q.SyncUpdateExam(ctx, examUpdateParams(remote, courseID, local.ID))
		})
		if err != nil {
			return 0, err
		}
		if applied {
			merged++
		}
	}
	return merged, nil
}

func findExamMeta(ctx context.Context, q *store.Queries, syncID string, remoteID int64, allowIDFallback bool) (*localMeta, error) {
	row, err := q.SyncFindExamMeta(ctx, store.TextOf(syncID))
	if err == nil {
		return examMetaFromRow(row), nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return nil, err
	}
	if !allowIDFallback {
		return nil, nil
	}
	byID, err := q.SyncFindExamMetaByID(ctx, remoteID)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return examMetaFromByIDRow(byID), nil
}

func examMetaFromRow(row store.SyncFindExamMetaRow) *localMeta {
	m := &localMeta{ID: row.ID, SyncID: store.TextString(row.SyncID), UpdatedAt: store.TSString(row.UpdatedAt)}
	if row.DeletedAt.Valid {
		s := store.TSString(row.DeletedAt)
		m.DeletedAt = &s
	}
	return m
}

func examMetaFromByIDRow(row store.SyncFindExamMetaByIDRow) *localMeta {
	m := &localMeta{ID: row.ID, SyncID: store.TextString(row.SyncID), UpdatedAt: store.TSString(row.UpdatedAt)}
	if row.DeletedAt.Valid {
		s := store.TSString(row.DeletedAt)
		m.DeletedAt = &s
	}
	return m
}

// ─── pomodoro sessions ─────────────────────────────────────────────────────

func mergeSessions(ctx context.Context, q *store.Queries, remote []PomodoroSession, taskIDMap map[int64]int64) (int, error) {
	merged := 0
	for _, s := range remote {
		isLegacy := s.SyncID == ""
		remote := normalizeSession(s)
		taskID := mapRemoteEntityID(taskIDMap, remote.TaskID)
		local, err := findSessionMeta(ctx, q, remote.SyncID, remote.ID, isLegacy)
		if err != nil {
			return 0, err
		}
		if local == nil {
			if err := q.SyncInsertSession(ctx, sessionInsertParams(remote, taskID)); err != nil {
				return 0, err
			}
			merged++
			continue
		}
		newer := sessionEffectiveTimestamp(&remote) > local.effective()
		applied, err := resolveMerge(ctx, q, "pomodoro_sessions", local, remote.SyncID, newer, func() error {
			return q.SyncUpdateSession(ctx, sessionUpdateParams(remote, taskID, local.ID))
		})
		if err != nil {
			return 0, err
		}
		if applied {
			merged++
		}
	}
	return merged, nil
}

func findSessionMeta(ctx context.Context, q *store.Queries, syncID string, remoteID int64, allowIDFallback bool) (*localMeta, error) {
	row, err := q.SyncFindSessionMeta(ctx, store.TextOf(syncID))
	if err == nil {
		return sessionMetaFromRow(row), nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return nil, err
	}
	if !allowIDFallback {
		return nil, nil
	}
	byID, err := q.SyncFindSessionMetaByID(ctx, remoteID)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return sessionMetaFromByIDRow(byID), nil
}

func sessionMetaFromRow(row store.SyncFindSessionMetaRow) *localMeta {
	m := &localMeta{ID: row.ID, SyncID: store.TextString(row.SyncID)}
	if row.EndedAt.Valid {
		m.UpdatedAt = store.TSString(row.EndedAt)
	}
	if row.DeletedAt.Valid {
		s := store.TSString(row.DeletedAt)
		m.DeletedAt = &s
	}
	return m
}

func sessionMetaFromByIDRow(row store.SyncFindSessionMetaByIDRow) *localMeta {
	m := &localMeta{ID: row.ID, SyncID: store.TextString(row.SyncID)}
	if row.EndedAt.Valid {
		m.UpdatedAt = store.TSString(row.EndedAt)
	}
	if row.DeletedAt.Valid {
		s := store.TSString(row.DeletedAt)
		m.DeletedAt = &s
	}
	return m
}

// sessionEffectiveTimestamp: deleted_at → ended_at → started_at. A tombstone
// wins so a delete can cover an active session; otherwise the ended time beats
// an in-progress session; worst case falls back to the start time.
func sessionEffectiveTimestamp(s *PomodoroSession) string {
	if s.DeletedAt != nil && *s.DeletedAt != "" {
		return *s.DeletedAt
	}
	if s.EndedAt != nil && *s.EndedAt != "" {
		return *s.EndedAt
	}
	return s.StartedAt
}

// ─── term phases ───────────────────────────────────────────────────────────

func mergeTermPhases(ctx context.Context, q *store.Queries, remote []TermPhase) (int, error) {
	merged := 0
	for _, p := range remote {
		phase := normalizeTermPhase(p)
		row, err := q.SyncFindTermPhaseMeta(ctx, store.TextOf(phase.SyncID))
		if errors.Is(err, pgx.ErrNoRows) {
			if err := q.SyncInsertTermPhase(ctx, termPhaseInsertParams(phase)); err != nil {
				return 0, err
			}
			merged++
			continue
		}
		if err != nil {
			return 0, err
		}
		remoteTS := effectiveTimestamp(phase.UpdatedAt, phase.DeletedAt)
		localTS := effectiveTimestamp(store.TSString(row.UpdatedAt), tsPtr(row.DeletedAt))
		if remoteTS <= localTS {
			continue
		}
		if err := q.SyncUpdateTermPhase(ctx, termPhaseUpdateParams(phase, row.ID)); err != nil {
			return 0, err
		}
		merged++
	}
	return merged, nil
}

// ─── normalization (v1 compatibility) ──────────────────────────────────────

func normalizeTask(t Task) Task {
	if t.SyncID == "" {
		t.SyncID = legacySyncID("task", t.ID)
	}
	return t
}

func normalizeCourse(c Course) Course {
	if c.SyncID == "" {
		c.SyncID = legacySyncID("course", c.ID)
	}
	return c
}

func normalizeExam(e Exam) Exam {
	if e.SyncID == "" {
		e.SyncID = legacySyncID("exam", e.ID)
	}
	return e
}

func normalizeSession(s PomodoroSession) PomodoroSession {
	if s.SyncID == "" {
		s.SyncID = legacySyncID("pomodoro-session", s.ID)
	}
	return s
}

func normalizeTermPhase(p TermPhase) TermPhase {
	if p.SyncID == "" {
		p.SyncID = legacySyncID("term-phase", p.ID)
	}
	return p
}

func legacySyncID(entity string, id int64) string {
	return fmt.Sprintf("legacy-%s-%d", entity, id)
}

func mapRemoteEntityID(m map[int64]int64, remoteID *int64) *int64 {
	if remoteID == nil {
		return nil
	}
	if localID, ok := m[*remoteID]; ok {
		return &localID
	}
	return nil
}

func tsPtr(t pgtype.Timestamptz) *string {
	if !t.Valid {
		return nil
	}
	s := store.TSString(t)
	return &s
}
