package termphase

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"kairos/server/internal/store"
	"kairos/server/internal/store/migrate"
)

func testDatabaseURL() string {
	if v := os.Getenv("TEST_DATABASE_URL"); v != "" {
		return v
	}
	return "postgres://kairos:kairos_dev@localhost:5432/kairos_dev"
}

func newTestStore(t *testing.T) *store.Queries {
	t.Helper()
	ctx := context.Background()
	conn, err := pgx.Connect(ctx, testDatabaseURL())
	if err != nil {
		t.Fatalf("connect to test database: %v", err)
	}
	schema := "termphase_test_" + randomHex()
	if _, err := conn.Exec(ctx, "CREATE SCHEMA "+schema); err != nil {
		t.Fatalf("create schema %s: %v", schema, err)
	}
	if _, err := conn.Exec(ctx, "SET search_path TO "+schema); err != nil {
		t.Fatalf("set search_path to %s: %v", schema, err)
	}
	if err := migrate.Up(ctx, conn); err != nil {
		t.Fatalf("apply migrations to schema %s: %v", schema, err)
	}
	t.Cleanup(func() {
		defer conn.Close(context.Background())
		if _, err := conn.Exec(context.Background(), "DROP SCHEMA IF EXISTS "+schema+" CASCADE"); err != nil {
			t.Errorf("drop schema %s: %v", schema, err)
		}
	})
	return store.New(conn)
}

func randomHex() string {
	var b [8]byte
	if _, err := rand.Read(b[:]); err != nil {
		panic(err)
	}
	return hex.EncodeToString(b[:])
}

func dateOf(s string) time.Time {
	t, err := time.ParseInLocation("2006-01-02", s, chinaTZ())
	if err != nil {
		panic(err)
	}
	return t
}

func upsertContext(t *testing.T, q *store.Queries, termLabel, startDate string, totalWeeks *int32) {
	t.Helper()
	total := store.Int4Of(0)
	if totalWeeks != nil {
		total = store.Int4Of(*totalWeeks)
	} else {
		total = store.Int4Of(0)
		total.Valid = false
	}
	_, err := q.UpsertSemesterContext(context.Background(), store.UpsertSemesterContextParams{
		Source:       DefaultSource,
		AcademicYear: store.TextOf("2026"),
		Term:         store.TextOf("1"),
		TermLabel:    termLabel,
		StartDate:    store.DateOf(startDate),
		CurrentWeek:  store.Int4Of(1),
		TotalWeeks:   total,
	})
	if err != nil {
		t.Fatalf("upsert context: %v", err)
	}
}

func insertCourse(t *testing.T, q *store.Queries, semester string) {
	t.Helper()
	_, err := q.CreateCourse(context.Background(), store.CreateCourseParams{
		SyncID:            "",
		Name:              "测试课程",
		DayOfWeek:         1,
		StartTime:         store.ClockOf("08:00"),
		EndTime:           store.ClockOf("09:40"),
		WeekPattern:       "1-17周全周",
		SemesterStartDate: store.DateOf("2026-02-24"),
		Location:          "",
		Teacher:           "",
		Color:             "#3B82F6",
		Semester:          semester,
	})
	if err != nil {
		t.Fatalf("insert course: %v", err)
	}
}

func createPhase(t *testing.T, q *store.Queries, phaseType string, startWeek, endWeek int32) {
	t.Helper()
	profile := "default"
	if phaseType == PhaseExam {
		profile = "intense"
	}
	_, err := q.CreateTermPhase(context.Background(), store.CreateTermPhaseParams{
		SyncID:                   "",
		TermLabel:                "2026S1",
		PhaseType:                phaseType,
		StartWeek:                startWeek,
		EndWeek:                  endWeek,
		AffectsCourses:           phaseType != PhaseBreak,
		AffectsExamNotifications: phaseType != PhaseBreak,
		PomodoroProfile:          profile,
		NotificationRules:        []byte("{}"),
		SortOrder:                startWeek,
	})
	if err != nil {
		t.Fatalf("create phase: %v", err)
	}
}

func TestUnknownWithoutSemesterContext(t *testing.T) {
	q := newTestStore(t)

	status, err := GetPhaseStatusForDate(context.Background(), q, DefaultSource, dateOf("2026-02-24"))
	if err != nil {
		t.Fatalf("phase status: %v", err)
	}
	if status.PhaseType != PhaseUnknown {
		t.Fatalf("phase_type = %q, want unknown", status.PhaseType)
	}
	if status.CurrentWeek != nil {
		t.Fatalf("current_week should be nil, got %v", *status.CurrentWeek)
	}
}

func TestFallbackTeachingAndBreak(t *testing.T) {
	q := newTestStore(t)
	ctx := context.Background()
	weeks := int32(16)
	upsertContext(t, q, "2026S1", "2026-02-24", &weeks)
	insertCourse(t, q, "2026S1")

	teaching, err := GetPhaseStatusForDate(ctx, q, DefaultSource, dateOf("2026-02-26"))
	if err != nil {
		t.Fatalf("teaching status: %v", err)
	}
	if teaching.PhaseType != PhaseTeaching {
		t.Fatalf("phase_type = %q, want teaching", teaching.PhaseType)
	}
	if !teaching.CoursesVisible {
		t.Fatal("courses_visible should be true")
	}

	breakStatus, err := GetPhaseStatusForDate(ctx, q, DefaultSource, dateOf("2026-07-20"))
	if err != nil {
		t.Fatalf("break status: %v", err)
	}
	if breakStatus.PhaseType != PhaseBreak {
		t.Fatalf("phase_type = %q, want break", breakStatus.PhaseType)
	}
	if breakStatus.ExamNotificationsEnabled {
		t.Fatal("exam_notifications_enabled should be false in break")
	}
}

func TestExplicitPhaseOverridesFallback(t *testing.T) {
	q := newTestStore(t)
	ctx := context.Background()
	weeks := int32(16)
	upsertContext(t, q, "2026S1", "2026-02-24", &weeks)
	createPhase(t, q, PhaseExam, 1, 1)
	insertCourse(t, q, "2026S1")

	status, err := GetPhaseStatusForDate(ctx, q, DefaultSource, dateOf("2026-02-26"))
	if err != nil {
		t.Fatalf("phase status: %v", err)
	}
	if status.PhaseType != PhaseExam {
		t.Fatalf("phase_type = %q, want exam", status.PhaseType)
	}
	if status.PomodoroProfile != "intense" {
		t.Fatalf("pomodoro_profile = %q, want intense", status.PomodoroProfile)
	}
	if status.Inferred {
		t.Fatal("explicit phase must not be inferred")
	}
}

func TestExplicitTeachingWithoutCoursesDowngradesToBreak(t *testing.T) {
	q := newTestStore(t)
	ctx := context.Background()
	weeks := int32(16)
	upsertContext(t, q, "2026S1", "2026-02-24", &weeks)
	createPhase(t, q, PhaseTeaching, 1, 16)

	status, err := GetPhaseStatusForDate(ctx, q, DefaultSource, dateOf("2026-02-26"))
	if err != nil {
		t.Fatalf("phase status: %v", err)
	}
	if status.PhaseType != PhaseBreak {
		t.Fatalf("phase_type = %q, want break", status.PhaseType)
	}
	if status.CoursesVisible {
		t.Fatal("courses_visible should be false")
	}
	if status.ExamNotificationsEnabled {
		t.Fatal("exam_notifications_enabled should be false")
	}
	if status.PomodoroProfile != "relaxed" {
		t.Fatalf("pomodoro_profile = %q, want relaxed", status.PomodoroProfile)
	}
	if !status.Inferred {
		t.Fatal("downgraded status should be inferred")
	}
}

func TestInferredTeachingWithoutCoursesDowngradesToBreak(t *testing.T) {
	q := newTestStore(t)
	ctx := context.Background()
	weeks := int32(16)
	upsertContext(t, q, "2026S1", "2026-02-24", &weeks)

	status, err := GetPhaseStatusForDate(ctx, q, DefaultSource, dateOf("2026-02-26"))
	if err != nil {
		t.Fatalf("phase status: %v", err)
	}
	if status.PhaseType != PhaseBreak {
		t.Fatalf("phase_type = %q, want break", status.PhaseType)
	}
	if status.CoursesVisible {
		t.Fatal("courses_visible should be false")
	}
}

func TestTermWeekTeachingWithoutCoursesDowngradesToBreak(t *testing.T) {
	q := newTestStore(t)
	ctx := context.Background()
	weeks := int32(16)
	upsertContext(t, q, "2026S1", "2026-02-24", &weeks)
	createPhase(t, q, PhaseTeaching, 1, 16)

	status, err := GetPhaseStatusForTermWeek(ctx, q, DefaultSource, "2026S1", 2)
	if err != nil {
		t.Fatalf("phase status: %v", err)
	}
	if status.PhaseType != PhaseBreak {
		t.Fatalf("phase_type = %q, want break", status.PhaseType)
	}
	if status.CoursesVisible {
		t.Fatal("courses_visible should be false")
	}
}

func TestTermWeekRejectsNonPositiveWeek(t *testing.T) {
	q := newTestStore(t)
	if _, err := GetPhaseStatusForTermWeek(context.Background(), q, DefaultSource, "2026S1", 0); err == nil {
		t.Fatal("expected error for week_index < 1")
	}
}

func TestEnsureDefaultPhasesIsIdempotent(t *testing.T) {
	q := newTestStore(t)
	ctx := context.Background()

	created, err := EnsureDefaultPhases(ctx, q, "2026S1", 16)
	if err != nil {
		t.Fatalf("ensure defaults: %v", err)
	}
	if created != 2 {
		t.Fatalf("first ensure created %d phases, want 2", created)
	}

	created, err = EnsureDefaultPhases(ctx, q, "2026S1", 16)
	if err != nil {
		t.Fatalf("ensure again: %v", err)
	}
	if created != 0 {
		t.Fatalf("second ensure created %d phases, want 0", created)
	}

	phases, err := q.ListTermPhases(ctx, "2026S1")
	if err != nil {
		t.Fatalf("list phases: %v", err)
	}
	if len(phases) != 2 {
		t.Fatalf("expected 2 phases, got %d", len(phases))
	}
	if phases[0].PhaseType != PhaseTeaching || phases[1].PhaseType != PhaseExam {
		t.Fatalf("unexpected phases: %+v", phases)
	}
}

func TestCurrentWeekFromContext(t *testing.T) {
	cases := []struct {
		startDate string
		today     string
		want      int64
	}{
		{"2026-02-24", "2026-02-24", 1},
		{"2026-02-24", "2026-02-26", 1},
		{"2026-02-24", "2026-03-02", 2},
		{"2026-02-24", "2026-07-20", 22},
		{"2026-02-24", "2026-02-01", 1},
	}
	for _, tc := range cases {
		got, err := CurrentWeekFromContext(tc.startDate, dateOf(tc.today))
		if err != nil {
			t.Fatalf("current week for %s: %v", tc.today, err)
		}
		if got != tc.want {
			t.Errorf("CurrentWeekFromContext(%q, %q) = %d, want %d", tc.startDate, tc.today, got, tc.want)
		}
	}
}
