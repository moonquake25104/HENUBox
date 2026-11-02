package store

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5/pgtype"
)

func TestCourseCreateAndGet(t *testing.T) {
	q, _ := newTestStore(t)

	course, err := q.CreateCourse(context.Background(), CreateCourseParams{
		SyncID: "", Name: "Math 101", DayOfWeek: 1,
		StartTime: clockOf("08:00"), EndTime: clockOf("09:30"),
		WeekPattern: "1-16", SemesterStartDate: dateOf("2026-02-24"),
		Location: "Room 101", Teacher: "Prof. Smith",
		Color: "#3B82F6", Semester: "2024S1",
	})
	if err != nil {
		t.Fatalf("create course: %v", err)
	}
	if !course.SyncID.Valid || course.SyncID.String == "" {
		t.Fatalf("expected backfilled sync_id: %+v", course.SyncID)
	}

	got, err := q.GetCourse(context.Background(), course.ID)
	if err != nil {
		t.Fatalf("get course: %v", err)
	}
	if got.Name != "Math 101" || got.DayOfWeek != 1 || got.WeekPattern != "1-16" || got.Color != "#3B82F6" {
		t.Fatalf("unexpected course: %+v", got)
	}
	if got.StartTime.Microseconds != clockOf("08:00").Microseconds {
		t.Fatalf("start_time not persisted: %+v", got.StartTime)
	}
}

func TestCourseUpdateAndSoftDelete(t *testing.T) {
	q, _ := newTestStore(t)

	course, err := q.CreateCourse(context.Background(), CreateCourseParams{
		SyncID: "", Name: "Physics", DayOfWeek: 1,
		StartTime: clockOf("08:00"), EndTime: clockOf("09:30"),
		WeekPattern: "1-16", SemesterStartDate: dateOf("2026-02-24"),
		Location: "", Teacher: "", Color: "#3B82F6", Semester: "2024S1",
	})
	if err != nil {
		t.Fatalf("create course: %v", err)
	}

	updated, err := q.UpdateCourse(context.Background(), UpdateCourseParams{
		Name: "Physics 201", DayOfWeek: 3,
		StartTime: clockOf("10:00"), EndTime: clockOf("11:30"),
		WeekPattern: "2-18双", SemesterStartDate: dateOf("2026-02-24"),
		Location: "Lab B", Teacher: "Dr. Jones", Color: "#EF4444",
		Semester: "2024S2", ID: course.ID,
	})
	if err != nil {
		t.Fatalf("update course: %v", err)
	}
	if updated.Name != "Physics 201" || updated.DayOfWeek != 3 || updated.Color != "#EF4444" {
		t.Fatalf("course not updated: %+v", updated)
	}

	if err := q.SoftDeleteCourse(context.Background(), course.ID); err != nil {
		t.Fatalf("soft delete course: %v", err)
	}
	if _, err := q.GetCourse(context.Background(), course.ID); err == nil {
		t.Fatal("deleted course should not be fetchable")
	}
}

func TestCourseListBySemester(t *testing.T) {
	q, _ := newTestStore(t)

	base := CreateCourseParams{
		SyncID: "", StartTime: clockOf("08:00"), EndTime: clockOf("09:00"),
		WeekPattern: "1-16", SemesterStartDate: dateOf("2026-02-24"),
		Location: "", Teacher: "", Color: "",
	}
	a := base
	a.Name, a.DayOfWeek, a.Semester = "Course A", 1, "2024S1"
	b := base
	b.Name, b.DayOfWeek, b.Semester = "Course B", 2, "2024S2"

	if _, err := q.CreateCourse(context.Background(), a); err != nil {
		t.Fatalf("create A: %v", err)
	}
	if _, err := q.CreateCourse(context.Background(), b); err != nil {
		t.Fatalf("create B: %v", err)
	}

	all, err := q.ListCourses(context.Background(), pgtype.Text{})
	if err != nil {
		t.Fatalf("list all: %v", err)
	}
	if len(all) != 2 {
		t.Fatalf("expected 2 courses, got %d", len(all))
	}

	s1, err := q.ListCourses(context.Background(), textOf("2024S1"))
	if err != nil {
		t.Fatalf("list s1: %v", err)
	}
	if len(s1) != 1 || s1[0].Name != "Course A" {
		t.Fatalf("semester filter mismatch: %+v", s1)
	}
}

func TestCourseImportDedup(t *testing.T) {
	q, _ := newTestStore(t)

	arg := CountCourseImportDuplicatesParams{
		Semester: "2024S1", Name: "Math", DayOfWeek: 1,
		StartTime: clockOf("08:00"), EndTime: clockOf("09:00"),
		WeekPattern: "1-16", SemesterStartDate: dateOf("2026-02-24"),
		Location: "Room 1", Teacher: "Prof",
	}
	count, err := q.CountCourseImportDuplicates(context.Background(), arg)
	if err != nil {
		t.Fatalf("count dup empty: %v", err)
	}
	if count != 0 {
		t.Fatalf("expected 0 duplicates, got %d", count)
	}

	if _, err := q.CreateCourse(context.Background(), CreateCourseParams{
		SyncID: "", Name: "Math", DayOfWeek: 1,
		StartTime: clockOf("08:00"), EndTime: clockOf("09:00"),
		WeekPattern: "1-16", SemesterStartDate: dateOf("2026-02-24"),
		Location: "Room 1", Teacher: "Prof", Color: "", Semester: "2024S1",
	}); err != nil {
		t.Fatalf("create course: %v", err)
	}

	count, err = q.CountCourseImportDuplicates(context.Background(), arg)
	if err != nil {
		t.Fatalf("count dup: %v", err)
	}
	if count != 1 {
		t.Fatalf("expected 1 duplicate, got %d", count)
	}
}

func TestExamCreateUpdateAndList(t *testing.T) {
	q, _ := newTestStore(t)

	exam, err := q.CreateExam(context.Background(), CreateExamParams{
		SyncID: "", CourseName: "Calculus Final",
		ExamDatetime: tsOf("2024-12-15T09:00:00Z"), ExamEndDatetime: tsOf("2024-12-15T11:00:00Z"),
		Location: "Hall A", Notes: "", CourseID: pgtype_Int8Invalid(), Semester: "2024S1",
	})
	if err != nil {
		t.Fatalf("create exam: %v", err)
	}
	if !exam.SyncID.Valid || exam.SyncID.String == "" {
		t.Fatalf("expected backfilled sync_id: %+v", exam.SyncID)
	}

	got, err := q.GetExam(context.Background(), exam.ID)
	if err != nil {
		t.Fatalf("get exam: %v", err)
	}
	if got.CourseName != "Calculus Final" || got.Location != "Hall A" {
		t.Fatalf("unexpected exam: %+v", got)
	}
	if !sameTS(got.ExamDatetime, "2024-12-15T09:00:00Z") {
		t.Fatalf("exam_datetime not persisted: %+v", got.ExamDatetime)
	}

	updated, err := q.UpdateExam(context.Background(), UpdateExamParams{
		CourseName: "Calculus Final (Updated)", ExamDatetime: tsOf("2024-12-20T14:00:00Z"),
		ExamEndDatetime: tsOf("2024-12-20T16:00:00Z"), Location: "Hall B",
		Notes: "Bring calculator", CourseID: pgtype_Int8Invalid(),
		Semester: "2024S1", ID: exam.ID,
	})
	if err != nil {
		t.Fatalf("update exam: %v", err)
	}
	if updated.Location != "Hall B" || updated.Notes != "Bring calculator" {
		t.Fatalf("exam not updated: %+v", updated)
	}

	if err := q.SoftDeleteExam(context.Background(), exam.ID); err != nil {
		t.Fatalf("soft delete exam: %v", err)
	}
	if _, err := q.GetExam(context.Background(), exam.ID); err == nil {
		t.Fatal("deleted exam should not be fetchable")
	}
}

func TestExamListOrderedByDatetime(t *testing.T) {
	q, _ := newTestStore(t)

	if _, err := q.CreateExam(context.Background(), CreateExamParams{
		SyncID: "", CourseName: "Exam A", ExamDatetime: tsOf("2024-12-15T09:00:00Z"),
		ExamEndDatetime: pgtype_TimestamptzInvalid(), Location: "", Notes: "",
		CourseID: pgtype_Int8Invalid(), Semester: "2024S1",
	}); err != nil {
		t.Fatalf("create A: %v", err)
	}
	if _, err := q.CreateExam(context.Background(), CreateExamParams{
		SyncID: "", CourseName: "Exam B", ExamDatetime: tsOf("2024-12-10T09:00:00Z"),
		ExamEndDatetime: pgtype_TimestamptzInvalid(), Location: "", Notes: "",
		CourseID: pgtype_Int8Invalid(), Semester: "2024S1",
	}); err != nil {
		t.Fatalf("create B: %v", err)
	}

	exams, err := q.ListExams(context.Background())
	if err != nil {
		t.Fatalf("list exams: %v", err)
	}
	if len(exams) != 2 || exams[0].CourseName != "Exam B" || exams[1].CourseName != "Exam A" {
		t.Fatalf("exams not ordered by datetime: %+v", exams)
	}
}

func TestExamImportDedup(t *testing.T) {
	q, _ := newTestStore(t)

	arg := CountExamImportDuplicatesParams{
		Semester: "2024S1", CourseName: "Math", ExamDatetime: tsOf("2024-12-15T09:00:00Z"),
		ExamEndDatetime: tsOf("2024-12-15T11:00:00Z"), Location: "Hall A",
	}
	count, err := q.CountExamImportDuplicates(context.Background(), arg)
	if err != nil {
		t.Fatalf("count dup empty: %v", err)
	}
	if count != 0 {
		t.Fatalf("expected 0 duplicates, got %d", count)
	}

	if _, err := q.CreateExam(context.Background(), CreateExamParams{
		SyncID: "", CourseName: "Math", ExamDatetime: tsOf("2024-12-15T09:00:00Z"),
		ExamEndDatetime: tsOf("2024-12-15T11:00:00Z"), Location: "Hall A",
		Notes: "", CourseID: pgtype_Int8Invalid(), Semester: "2024S1",
	}); err != nil {
		t.Fatalf("create exam: %v", err)
	}

	count, err = q.CountExamImportDuplicates(context.Background(), arg)
	if err != nil {
		t.Fatalf("count dup: %v", err)
	}
	if count != 1 {
		t.Fatalf("expected 1 duplicate, got %d", count)
	}
}
