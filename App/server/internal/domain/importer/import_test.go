package importer

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"os"
	"testing"

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
	schema := "importer_test_" + randomHex()
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

func sampleCourse() CourseCandidate {
	return CourseCandidate{
		Name:              "自动控制原理",
		DayOfWeek:         3,
		StartTime:         "10:00",
		EndTime:           "11:40",
		WeekPattern:       "1-17周全周",
		SemesterStartDate: "2026-02-24",
		Location:          "秦岭堂A114",
		Teacher:           "李红信",
		Color:             "#3B82F6",
		Semester:          "2026S1",
	}
}

func TestImportNewCoursesSkipsDuplicates(t *testing.T) {
	q := newTestStore(t)
	ctx := context.Background()
	course := sampleCourse()

	first, err := ImportNewCourses(ctx, q, []CourseCandidate{course}, "2026S1")
	if err != nil {
		t.Fatalf("first import: %v", err)
	}
	if first.Parsed != 1 || first.Imported != 1 || first.Skipped != 0 {
		t.Fatalf("first import counts = %+v, want parsed=1 imported=1 skipped=0", first)
	}

	second, err := ImportNewCourses(ctx, q, []CourseCandidate{course}, "2026S1")
	if err != nil {
		t.Fatalf("second import: %v", err)
	}
	if second.Parsed != 1 || second.Imported != 0 || second.Skipped != 1 {
		t.Fatalf("second import counts = %+v, want parsed=1 imported=0 skipped=1", second)
	}

	courses, err := q.ListCourses(ctx, store.TextOf("2026S1"))
	if err != nil {
		t.Fatalf("list courses: %v", err)
	}
	if len(courses) != 1 {
		t.Fatalf("expected 1 course after dedup, got %d", len(courses))
	}
}

func TestImportNewCoursesDistinctKeysBothImported(t *testing.T) {
	q := newTestStore(t)
	ctx := context.Background()

	a := sampleCourse()
	b := sampleCourse()
	b.DayOfWeek = 4

	result, err := ImportNewCourses(ctx, q, []CourseCandidate{a, b}, "2026S1")
	if err != nil {
		t.Fatalf("import: %v", err)
	}
	if result.Imported != 2 || result.Skipped != 0 {
		t.Fatalf("counts = %+v, want imported=2 skipped=0", result)
	}
}

func sampleExam() ExamCandidate {
	return ExamCandidate{
		CourseName:      "自动控制原理",
		ExamDatetime:    "2026-07-06T08:00:00Z",
		ExamEndDatetime: "2026-07-06T10:00:00Z",
		Location:        "天山堂A409",
		Notes:           "正常考试",
		CourseID:        nil,
		Semester:        "2026S1",
	}
}

func TestImportNewExamsSkipsDuplicates(t *testing.T) {
	q := newTestStore(t)
	ctx := context.Background()
	exam := sampleExam()

	first, err := ImportNewExams(ctx, q, []ExamCandidate{exam})
	if err != nil {
		t.Fatalf("first import: %v", err)
	}
	if first.Parsed != 1 || first.Imported != 1 || first.Skipped != 0 {
		t.Fatalf("first import counts = %+v, want parsed=1 imported=1 skipped=0", first)
	}

	second, err := ImportNewExams(ctx, q, []ExamCandidate{exam})
	if err != nil {
		t.Fatalf("second import: %v", err)
	}
	if second.Parsed != 1 || second.Imported != 0 || second.Skipped != 1 {
		t.Fatalf("second import counts = %+v, want parsed=1 imported=0 skipped=1", second)
	}

	exams, err := q.ListExams(ctx)
	if err != nil {
		t.Fatalf("list exams: %v", err)
	}
	if len(exams) != 1 {
		t.Fatalf("expected 1 exam after dedup, got %d", len(exams))
	}
}
