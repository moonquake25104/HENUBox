package importer

import (
	"context"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5/pgtype"

	"kairos/server/internal/store"
)

// Store is the subset of the sqlc store the importers need.
type Store interface {
	ListCourses(ctx context.Context, semester pgtype.Text) ([]store.Course, error)
	CreateCourse(ctx context.Context, arg store.CreateCourseParams) (store.Course, error)
	ListExams(ctx context.Context) ([]store.Exam, error)
	CreateExam(ctx context.Context, arg store.CreateExamParams) (store.Exam, error)
}

// ImportNewCourses persists parsed course candidates, skipping any that match
// an existing course on the dedup key (semester+name+day+time+weeks+date+location+teacher).
func ImportNewCourses(ctx context.Context, q Store, courses []CourseCandidate, semester string) (ImportTextResult, error) {
	existing, err := q.ListCourses(ctx, store.TextOf(semester))
	if err != nil {
		return ImportTextResult{}, err
	}
	seen := make(map[string]struct{}, len(existing))
	for _, c := range existing {
		seen[CourseImportKey(c)] = struct{}{}
	}

	imported, skipped := 0, 0
	for _, c := range courses {
		key := CourseRequestImportKey(c)
		if _, dup := seen[key]; dup {
			skipped++
			continue
		}
		seen[key] = struct{}{}
		if _, err := q.CreateCourse(ctx, courseToParams(c)); err != nil {
			return ImportTextResult{}, err
		}
		imported++
	}
	return FromCounts(len(courses), imported, skipped), nil
}

// ImportNewExams persists parsed exam candidates, skipping any that match an
// existing exam on the dedup key (semester+course_name+start+end+location).
func ImportNewExams(ctx context.Context, q Store, exams []ExamCandidate) (ImportTextResult, error) {
	existing, err := q.ListExams(ctx)
	if err != nil {
		return ImportTextResult{}, err
	}
	seen := make(map[string]struct{}, len(existing))
	for _, e := range existing {
		seen[ExamImportKey(e)] = struct{}{}
	}

	imported, skipped := 0, 0
	for _, e := range exams {
		key := ExamRequestImportKey(e)
		if _, dup := seen[key]; dup {
			skipped++
			continue
		}
		seen[key] = struct{}{}
		if _, err := q.CreateExam(ctx, examToParams(e)); err != nil {
			return ImportTextResult{}, err
		}
		imported++
	}
	return FromCounts(len(exams), imported, skipped), nil
}

// CourseImportKey builds the dedup key for a persisted course.
func CourseImportKey(c store.Course) string {
	return courseKeyParts(
		c.Semester,
		c.Name,
		int64(c.DayOfWeek),
		store.ClockString(c.StartTime),
		store.ClockString(c.EndTime),
		c.WeekPattern,
		store.DateString(c.SemesterStartDate),
		c.Location,
		c.Teacher,
	)
}

// CourseRequestImportKey builds the dedup key for a parsed course candidate.
func CourseRequestImportKey(c CourseCandidate) string {
	return courseKeyParts(
		c.Semester,
		c.Name,
		c.DayOfWeek,
		c.StartTime,
		c.EndTime,
		c.WeekPattern,
		c.SemesterStartDate,
		c.Location,
		c.Teacher,
	)
}

func courseKeyParts(semester, name string, dayOfWeek int64, startTime, endTime, weekPattern, semesterStartDate, location, teacher string) string {
	return strings.Join([]string{
		semester, name, fmt.Sprintf("%d", dayOfWeek), startTime, endTime,
		weekPattern, semesterStartDate, location, teacher,
	}, "\t")
}

// ExamImportKey builds the dedup key for a persisted exam.
func ExamImportKey(e store.Exam) string {
	return examKeyParts(
		e.Semester,
		e.CourseName,
		store.TSString(e.ExamDatetime),
		store.TSString(e.ExamEndDatetime),
		e.Location,
	)
}

// ExamRequestImportKey builds the dedup key for a parsed exam candidate.
func ExamRequestImportKey(e ExamCandidate) string {
	return examKeyParts(e.Semester, e.CourseName, e.ExamDatetime, e.ExamEndDatetime, e.Location)
}

func examKeyParts(semester, courseName, examDatetime, examEndDatetime, location string) string {
	return strings.Join([]string{semester, courseName, examDatetime, examEndDatetime, location}, "\t")
}

func courseToParams(c CourseCandidate) store.CreateCourseParams {
	return store.CreateCourseParams{
		SyncID:            "",
		Name:              c.Name,
		DayOfWeek:         int32(c.DayOfWeek),
		StartTime:         store.ClockOf(c.StartTime),
		EndTime:           store.ClockOf(c.EndTime),
		WeekPattern:       c.WeekPattern,
		SemesterStartDate: store.DateOf(c.SemesterStartDate),
		Location:          c.Location,
		Teacher:           c.Teacher,
		Color:             c.Color,
		Semester:          c.Semester,
	}
}

func examToParams(e ExamCandidate) store.CreateExamParams {
	courseID := pgtype.Int8{}
	if e.CourseID != nil {
		courseID = store.Int8Of(*e.CourseID)
	}
	return store.CreateExamParams{
		SyncID:          "",
		CourseName:      e.CourseName,
		ExamDatetime:    store.TSOf(e.ExamDatetime),
		ExamEndDatetime: store.TSOf(e.ExamEndDatetime),
		Location:        e.Location,
		Notes:           e.Notes,
		CourseID:        courseID,
		Semester:        e.Semester,
	}
}
