package sync

import (
	"context"
	"errors"
	"testing"
	"time"

	"kairos/server/internal/store"
)

func seedAcademicSession(t *testing.T, q *store.Queries) {
	t.Helper()
	ctx := context.Background()
	const owner, semester = "kairos", "2026S2"
	courseUUID := "1a1a1a1a-0000-4000-8000-000000000001"
	entryUUID := "1a1a1a1a-0000-4000-8000-000000000002"
	revUUID := "1a1a1a1a-0000-4000-8000-000000000003"
	sessionUUID := "1a1a1a1a-0000-4000-8000-000000000004"

	if err := q.InsertAcademicCourse(ctx, courseUUID, owner, semester, "henu", "合成来源课", nil, nil, nil, nil); err != nil {
		t.Fatalf("academic course: %v", err)
	}
	if err := q.InsertSourceEntry(ctx, entryUUID, owner, semester, "henu", courseUUID, nil); err != nil {
		t.Fatalf("source entry: %v", err)
	}
	if err := q.InsertSourceRevision(ctx, revUUID, entryUUID, nil, "sha", []byte(`{"name":"合成来源课"}`)); err != nil {
		t.Fatalf("revision: %v", err)
	}
	if err := q.InsertAcademicSession(ctx, sessionUUID, courseUUID, entryUUID); err != nil {
		t.Fatalf("session: %v", err)
	}
}

func TestWouldChangeCourses(t *testing.T) {
	q, _ := newTestStore(t)
	ctx := context.Background()

	created, err := q.CreateCourse(ctx, store.CreateCourseParams{
		Name: "本地课", DayOfWeek: 1, StartTime: store.ClockOf("08:00"), EndTime: store.ClockOf("09:30"),
		Semester: "2026S2",
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}

	future := time.Now().Add(time.Hour).UTC().Format(time.RFC3339)
	past := time.Now().Add(-time.Hour).UTC().Format(time.RFC3339)

	changed, err := wouldChangeCourses(ctx, q, []Course{{ID: created.ID, Name: "改过", UpdatedAt: future}})
	if err != nil || !changed {
		t.Fatalf("newer remote should change: changed=%v err=%v", changed, err)
	}
	changed, err = wouldChangeCourses(ctx, q, []Course{{ID: created.ID, Name: "旧", UpdatedAt: past}})
	if err != nil || changed {
		t.Fatalf("older remote should not change: changed=%v err=%v", changed, err)
	}
	changed, err = wouldChangeCourses(ctx, q, []Course{{ID: 99999, Name: "新", UpdatedAt: future}})
	if err != nil || !changed {
		t.Fatalf("unknown course should change: changed=%v err=%v", changed, err)
	}
}

// TestImportAllAllowsNonCourseSyncWithAcademicSource: a snapshot that does not
// touch courses must still be importable when source links exist.
func TestImportAllAllowsNonCourseSyncWithAcademicSource(t *testing.T) {
	q, conn := newTestStore(t)
	ctx := context.Background()
	seedAcademicSession(t, q)

	svc := &Service{Q: q, Pool: conn, Owner: "kairos"}
	if _, err := svc.importAll(ctx, &SyncData{}); err != nil {
		t.Fatalf("empty sync should pass: %v", err)
	}
}

// TestImportAllRejectsCourseWritesWithAcademicSource pins the B1 sync gate: on
// a deployment with academic source links, a snapshot that would modify courses
// is rejected as a whole and leaves courses untouched.
func TestImportAllRejectsCourseWritesWithAcademicSource(t *testing.T) {
	q, conn := newTestStore(t)
	ctx := context.Background()
	seedAcademicSession(t, q)

	svc := &Service{Q: q, Pool: conn, Owner: "kairos"}
	future := time.Now().Add(time.Hour).UTC().Format(time.RFC3339)
	_, err := svc.importAll(ctx, &SyncData{Courses: []Course{{
		ID: 12345, Name: "同步想写入的课", DayOfWeek: 2,
		StartTime: "08:00", EndTime: "09:30", Semester: "2026S2", UpdatedAt: future,
	}}})
	if !errors.Is(err, ErrAcademicSyncUnsupported) {
		t.Fatalf("err = %v, want ErrAcademicSyncUnsupported", err)
	}

	courses, err := q.ListCourses(ctx, store.TextOf("2026S2"))
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(courses) != 0 {
		t.Fatalf("rejected sync must not write courses: %+v", courses)
	}
}
