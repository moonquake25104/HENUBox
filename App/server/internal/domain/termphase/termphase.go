// Package termphase ports the Rust term-phase logic (src-tauri/src/term_phase.rs):
// deriving the current phase status from the semester context, explicit
// term_phase rows and the fallback inference, including the downgrade of
// teaching weeks that have no active courses.
package termphase

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"kairos/server/internal/store"
)

const (
	// PhaseUnknown is returned when no semester context exists.
	PhaseUnknown = "unknown"
	// PhaseTeaching is the regular teaching period.
	PhaseTeaching = "teaching"
	// PhaseExam is the exam period.
	PhaseExam = "exam"
	// PhaseBreak is a holiday/break period.
	PhaseBreak = "break"

	// DefaultSource is the manual semester context source.
	DefaultSource = "manual"

	fallbackTotalWeeks = 20
)

// CurrentPhaseStatus mirrors the Rust CurrentPhaseStatus and the frontend
// CurrentPhaseStatus type.
type CurrentPhaseStatus struct {
	Source                   string  `json:"source"`
	TermLabel                *string `json:"term_label"`
	PhaseType                string  `json:"phase_type"`
	CurrentWeek              *int64  `json:"current_week"`
	StartWeek                *int64  `json:"start_week"`
	EndWeek                  *int64  `json:"end_week"`
	CoursesVisible           bool    `json:"courses_visible"`
	ExamNotificationsEnabled bool    `json:"exam_notifications_enabled"`
	PomodoroProfile          string  `json:"pomodoro_profile"`
	Inferred                 bool    `json:"inferred"`
}

// Store is the subset of the sqlc store the term-phase logic needs.
type Store interface {
	GetLatestSemesterContextBySource(ctx context.Context, source string) (store.SemesterContext, error)
	GetSemesterContext(ctx context.Context, arg store.GetSemesterContextParams) (store.SemesterContext, error)
	GetTermPhaseByWeek(ctx context.Context, arg store.GetTermPhaseByWeekParams) (store.TermPhase, error)
	ListTermPhases(ctx context.Context, termLabel string) ([]store.TermPhase, error)
	CreateTermPhase(ctx context.Context, arg store.CreateTermPhaseParams) (store.TermPhase, error)
	HasCoursesForSemester(ctx context.Context, semester string) (bool, error)
}

// GetCurrentPhaseStatus derives the phase status for today (+08:00).
func GetCurrentPhaseStatus(ctx context.Context, q Store, source string) (CurrentPhaseStatus, error) {
	today := time.Now().In(chinaTZ())
	return GetPhaseStatusForDate(ctx, q, source, today)
}

// GetPhaseStatusForDate derives the phase status for an arbitrary date.
func GetPhaseStatusForDate(ctx context.Context, q Store, source string, today time.Time) (CurrentPhaseStatus, error) {
	context, err := q.GetLatestSemesterContextBySource(ctx, source)
	if errors.Is(err, pgx.ErrNoRows) {
		return UnknownStatus(source), nil
	}
	if err != nil {
		return CurrentPhaseStatus{}, err
	}

	currentWeek, err := CurrentWeekFromContext(store.DateString(context.StartDate), today)
	if err != nil {
		return CurrentPhaseStatus{}, err
	}

	phase, err := q.GetTermPhaseByWeek(ctx, store.GetTermPhaseByWeekParams{
		TermLabel: context.TermLabel,
		WeekIndex: int32(currentWeek),
	})
	if err == nil {
		return DowngradeTeachingIfNoCourses(ctx, q, source, currentWeek, phase), nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return CurrentPhaseStatus{}, err
	}

	return DowngradeInferredTeachingIfNoCourses(
		ctx, q, InferredStatus(source, context, currentWeek), context.TermLabel,
	), nil
}

// GetPhaseStatusForTermWeek derives the phase status for a specific term week.
func GetPhaseStatusForTermWeek(ctx context.Context, q Store, source, termLabel string, weekIndex int64) (CurrentPhaseStatus, error) {
	if weekIndex < 1 {
		return CurrentPhaseStatus{}, fmt.Errorf("week_index 必须大于等于 1")
	}

	phase, err := q.GetTermPhaseByWeek(ctx, store.GetTermPhaseByWeekParams{
		TermLabel: termLabel,
		WeekIndex: int32(weekIndex),
	})
	if err == nil {
		return DowngradeTeachingIfNoCourses(ctx, q, source, weekIndex, phase), nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return CurrentPhaseStatus{}, err
	}

	context, err := findSemesterContext(ctx, q, source, termLabel)
	if err != nil {
		return CurrentPhaseStatus{}, err
	}
	totalWeeks := int64(fallbackTotalWeeks)
	if context != nil && context.TotalWeeks.Valid {
		totalWeeks = int64(context.TotalWeeks.Int32)
	}
	label := termLabel
	return DowngradeInferredTeachingIfNoCourses(
		ctx, q, InferredStatusFromParts(source, &label, weekIndex, totalWeeks), termLabel,
	), nil
}

// CurrentWeekFromContext computes the current teaching week from the semester
// anchor date, counting whole weeks since the anchor's Monday (min 1).
func CurrentWeekFromContext(startDate string, today time.Time) (int64, error) {
	anchor, err := time.ParseInLocation("2006-01-02", startDate, chinaTZ())
	if err != nil {
		return 0, fmt.Errorf("无法解析学期开始日期: %s", startDate)
	}
	weekStart := today.AddDate(0, 0, -daysFromMonday(today))
	anchorWeekStart := anchor.AddDate(0, 0, -daysFromMonday(anchor))
	diffDays := int64(weekStart.Sub(anchorWeekStart).Hours() / 24)
	week := floorDiv(diffDays, 7) + 1
	if week < 1 {
		week = 1
	}
	return week, nil
}

// UnknownStatus is the status returned when no semester context exists.
func UnknownStatus(source string) CurrentPhaseStatus {
	return CurrentPhaseStatus{
		Source:                   source,
		TermLabel:                nil,
		PhaseType:                PhaseUnknown,
		CurrentWeek:              nil,
		StartWeek:                nil,
		EndWeek:                  nil,
		CoursesVisible:           true,
		ExamNotificationsEnabled: true,
		PomodoroProfile:          "default",
		Inferred:                 true,
	}
}

// StatusFromPhase builds a status from an explicit term_phase row.
func StatusFromPhase(source string, currentWeek int64, phase store.TermPhase) CurrentPhaseStatus {
	termLabel := phase.TermLabel
	return CurrentPhaseStatus{
		Source:                   source,
		TermLabel:                &termLabel,
		PhaseType:                phase.PhaseType,
		CurrentWeek:              &currentWeek,
		StartWeek:                int64Ptr(int64(phase.StartWeek)),
		EndWeek:                  int64Ptr(int64(phase.EndWeek)),
		CoursesVisible:           phase.AffectsCourses,
		ExamNotificationsEnabled: phase.AffectsExamNotifications,
		PomodoroProfile:          phase.PomodoroProfile,
		Inferred:                 false,
	}
}

// InferredStatus builds the fallback status from the semester context.
func InferredStatus(source string, context store.SemesterContext, currentWeek int64) CurrentPhaseStatus {
	totalWeeks := int64(fallbackTotalWeeks)
	if context.TotalWeeks.Valid {
		totalWeeks = int64(context.TotalWeeks.Int32)
	}
	label := context.TermLabel
	return InferredStatusFromParts(source, &label, currentWeek, totalWeeks)
}

// InferredStatusFromParts builds the fallback status from raw parts.
func InferredStatusFromParts(source string, termLabel *string, currentWeek, totalWeeks int64) CurrentPhaseStatus {
	isBreak := currentWeek > totalWeeks
	phaseType := PhaseTeaching
	if isBreak {
		phaseType = PhaseBreak
	}
	profile := "default"
	if isBreak {
		profile = "relaxed"
	}
	return CurrentPhaseStatus{
		Source:                   source,
		TermLabel:                termLabel,
		PhaseType:                phaseType,
		CurrentWeek:              &currentWeek,
		StartWeek:                nil,
		EndWeek:                  nil,
		CoursesVisible:           !isBreak,
		ExamNotificationsEnabled: !isBreak,
		PomodoroProfile:          profile,
		Inferred:                 true,
	}
}

// HasCourses reports whether the term has any active courses. Query failures
// conservatively count as "has courses" so a DB error never turns a real
// teaching week into a holiday.
func HasCourses(ctx context.Context, q Store, termLabel string) bool {
	has, err := q.HasCoursesForSemester(ctx, termLabel)
	if err != nil {
		return true
	}
	return has
}

// DowngradeTeachingIfNoCourses resets an explicit teaching phase to break when
// the term has no active courses.
func DowngradeTeachingIfNoCourses(ctx context.Context, q Store, source string, currentWeek int64, phase store.TermPhase) CurrentPhaseStatus {
	if phase.PhaseType != PhaseTeaching {
		return StatusFromPhase(source, currentWeek, phase)
	}
	if HasCourses(ctx, q, phase.TermLabel) {
		return StatusFromPhase(source, currentWeek, phase)
	}
	return MarkAsBreak(StatusFromPhase(source, currentWeek, phase))
}

// DowngradeInferredTeachingIfNoCourses resets an inferred teaching status to
// break when the term has no active courses.
func DowngradeInferredTeachingIfNoCourses(ctx context.Context, q Store, status CurrentPhaseStatus, termLabel string) CurrentPhaseStatus {
	if status.PhaseType != PhaseTeaching || termLabel == "" {
		return status
	}
	if HasCourses(ctx, q, termLabel) {
		return status
	}
	return MarkAsBreak(status)
}

// MarkAsBreak rewrites a status into a break status.
func MarkAsBreak(status CurrentPhaseStatus) CurrentPhaseStatus {
	status.PhaseType = PhaseBreak
	status.CurrentWeek = nil
	status.StartWeek = nil
	status.EndWeek = nil
	status.CoursesVisible = false
	status.ExamNotificationsEnabled = false
	status.PomodoroProfile = "relaxed"
	status.Inferred = true
	return status
}

// EnsureDefaultPhases seeds the default teaching+exam phases for a term,
// idempotently (no-op when phases already exist or totalWeeks < 1).
func EnsureDefaultPhases(ctx context.Context, q Store, termLabel string, totalWeeks int64) (int, error) {
	if totalWeeks < 1 {
		return 0, nil
	}
	phases, err := q.ListTermPhases(ctx, termLabel)
	if err != nil {
		return 0, err
	}
	if len(phases) > 0 {
		return 0, nil
	}

	examStart := max(totalWeeks-1, 1)
	teachingEnd := max(examStart-1, 1)

	created := 0
	if teachingEnd >= 1 {
		if _, err := q.CreateTermPhase(ctx, store.CreateTermPhaseParams{
			SyncID:                   "",
			TermLabel:                termLabel,
			PhaseType:                PhaseTeaching,
			StartWeek:                1,
			EndWeek:                  int32(teachingEnd),
			AffectsCourses:           true,
			AffectsExamNotifications: true,
			PomodoroProfile:          "default",
			NotificationRules:        []byte("{}"),
			SortOrder:                0,
		}); err != nil {
			return created, err
		}
		created++
	}

	if _, err := q.CreateTermPhase(ctx, store.CreateTermPhaseParams{
		SyncID:                   "",
		TermLabel:                termLabel,
		PhaseType:                PhaseExam,
		StartWeek:                int32(examStart),
		EndWeek:                  int32(totalWeeks),
		AffectsCourses:           true,
		AffectsExamNotifications: true,
		PomodoroProfile:          "intense",
		NotificationRules:        []byte("{}"),
		SortOrder:                1,
	}); err != nil {
		return created, err
	}
	created++
	return created, nil
}

func findSemesterContext(ctx context.Context, q Store, source, semester string) (*store.SemesterContext, error) {
	termLabel := strings.TrimSpace(semester)
	if termLabel == "" {
		c, err := q.GetLatestSemesterContextBySource(ctx, source)
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		if err != nil {
			return nil, err
		}
		return &c, nil
	}
	c, err := q.GetSemesterContext(ctx, store.GetSemesterContextParams{Source: source, TermLabel: termLabel})
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &c, nil
}

func daysFromMonday(t time.Time) int {
	return (int(t.Weekday()) + 6) % 7
}

func floorDiv(a, b int64) int64 {
	q := a / b
	if (a%b != 0) && ((a < 0) != (b < 0)) {
		q--
	}
	return q
}

func int64Ptr(v int64) *int64 {
	return &v
}

func chinaTZ() *time.Location {
	return time.FixedZone("CST", 8*3600)
}
