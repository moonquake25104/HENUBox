package calendar

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	"kairos/server/internal/domain/termphase"
	"kairos/server/internal/store"
)

// NextCourse mirrors the frontend NextCourse.
type NextCourse struct {
	Title     string
	StartTime string
	EndTime   string
	Location  string
}

// TodayCourses mirrors the frontend TodayCourses.
type TodayCourses struct {
	TodayCount    int64
	CurrentCourse *NextCourse
	NextCourse    *NextCourse
}

// TaskSpotlight mirrors the frontend TaskSpotlight.
type TaskSpotlight struct {
	ID       int64
	Title    string
	Priority string
	DueDate  *string
}

// TodayTasks mirrors the frontend TodayTasks.
type TodayTasks struct {
	OverdueCount         int64
	DueTodayCount        int64
	Spotlight            []TaskSpotlight
	DailyUnfinishedCount int64
	DailySpotlight       []TaskSpotlight
}

// UpcomingExam mirrors the frontend UpcomingExam.
type UpcomingExam struct {
	ID           int64
	CourseName   string
	ExamDatetime string
	DaysUntil    int64
	Location     string
}

// PomodoroBriefing mirrors the frontend PomodoroBriefing.
type PomodoroBriefing struct {
	IsRunning         bool
	Phase             string
	RemainingSeconds  int64
	CompletedSessions int64
}

// PhaseBriefing mirrors the frontend PhaseBriefing.
type PhaseBriefing struct {
	PhaseType                string
	TermLabel                *string
	CurrentWeek              *int64
	CoursesVisible           bool
	ExamNotificationsEnabled bool
	PomodoroProfile          string
}

// TodayBriefingResponse mirrors the frontend TodayBriefingResponse.
type TodayBriefingResponse struct {
	Date         string
	WeekdayLabel string
	Courses      TodayCourses
	Tasks        TodayTasks
	Exam         *UpcomingExam
	Pomodoro     PomodoroBriefing
	Phase        PhaseBriefing
}

// WeekdayLabel returns the Chinese weekday label for a time.
func WeekdayLabel(t time.Time) string {
	switch t.Weekday() {
	case time.Monday:
		return "周一"
	case time.Tuesday:
		return "周二"
	case time.Wednesday:
		return "周三"
	case time.Thursday:
		return "周四"
	case time.Friday:
		return "周五"
	case time.Saturday:
		return "周六"
	default:
		return "周日"
	}
}

// BuildTodayCourses ports commands/briefing.rs build_today_courses.
func BuildTodayCourses(
	courses []store.Course,
	semesterContexts []store.SemesterContext,
	today time.Time,
	nowTime time.Time,
	phaseStatus termphase.CurrentPhaseStatus,
) TodayCourses {
	if !phaseStatus.CoursesVisible || phaseStatus.PhaseType == termphase.PhaseUnknown {
		return TodayCourses{}
	}

	todayWeekday := isoWeekday(today)
	contextStartDates := semesterContextStartDates(semesterContexts)

	todayCourses := []store.Course{}
	for _, c := range courses {
		if int64(c.DayOfWeek) != todayWeekday {
			continue
		}
		weekIndex, err := courseWeekIndex(c, contextStartDates, today)
		if err != nil {
			continue
		}
		if MatchesWeekPattern(c.WeekPattern, weekIndex) {
			todayCourses = append(todayCourses, c)
		}
	}

	todayCount := int64(len(todayCourses))

	var currentCourse *NextCourse
	for _, c := range todayCourses {
		start, err1 := clockOnDate(c.StartTime, today)
		end, err2 := clockOnDate(c.EndTime, today)
		if err1 != nil || err2 != nil {
			continue
		}
		if !start.After(nowTime) && nowTime.Before(end) {
			if currentCourse == nil || start.Before(clockOnDateOrZero(c.StartTime, today)) {
				currentCourse = courseBrief(c)
			}
		}
	}

	var nextCourse *NextCourse
	for _, c := range todayCourses {
		start, err := clockOnDate(c.StartTime, today)
		if err != nil {
			continue
		}
		if start.After(nowTime) {
			if nextCourse == nil || start.Before(clockOnDateOrZero(c.StartTime, today)) {
				nextCourse = courseBrief(c)
			}
		}
	}

	return TodayCourses{
		TodayCount:    todayCount,
		CurrentCourse: currentCourse,
		NextCourse:    nextCourse,
	}
}

func clockOnDate(t pgtype.Time, date time.Time) (time.Time, error) {
	if !t.Valid {
		return time.Time{}, fmt.Errorf("invalid clock")
	}
	hour := t.Microseconds / 3600e6
	minute := (t.Microseconds % 3600e6) / 60e6
	return time.Date(date.Year(), date.Month(), date.Day(), int(hour), int(minute), 0, 0, ChinaTZ), nil
}

func clockOnDateOrZero(t pgtype.Time, date time.Time) time.Time {
	v, err := clockOnDate(t, date)
	if err != nil {
		return time.Time{}
	}
	return v
}

func courseBrief(c store.Course) *NextCourse {
	return &NextCourse{
		Title:     c.Name,
		StartTime: store.ClockString(c.StartTime),
		EndTime:   store.ClockString(c.EndTime),
		Location:  c.Location,
	}
}

func semesterContextStartDates(contexts []store.SemesterContext) map[string]string {
	out := map[string]string{}
	for _, c := range contexts {
		if c.Source != termphase.DefaultSource {
			continue
		}
		if d := store.DateString(c.StartDate); strings.TrimSpace(d) != "" {
			out[c.TermLabel] = strings.TrimSpace(d)
		}
	}
	return out
}

func courseWeekIndex(c store.Course, contextStartDates map[string]string, today time.Time) (int64, error) {
	if startDate, ok := contextStartDates[c.Semester]; ok {
		if weekIndex, err := CurrentWeekIndexFromStart(startDate, today); err == nil {
			return weekIndex, nil
		}
	}
	return CurrentWeekIndexFromStart(store.DateString(c.SemesterStartDate), today)
}

func priorityScore(priority string) int64 {
	switch priority {
	case "high":
		return 3
	case "medium":
		return 2
	case "low":
		return 1
	default:
		return 0
	}
}

func taskDueRank(task store.Task, today time.Time) int64 {
	dueDateStr := strings.TrimSpace(store.DateString(task.DueDate))
	if dueDateStr == "" {
		return 0
	}
	dueDate, err := parseDate(dueDateStr)
	if err != nil {
		return 0
	}
	if dueDate.Before(today) {
		return 2
	}
	if sameDay(dueDate, today) {
		return 1
	}
	return 0
}

// BuildTodayTasks ports commands/briefing.rs build_today_tasks.
func BuildTodayTasks(tasks []store.Task, today time.Time) TodayTasks {
	var overdueCount, dueTodayCount int64
	spotlightCandidates := []store.Task{}

	for _, task := range tasks {
		if task.Status == "done" || task.DeletedAt.Valid {
			continue
		}
		dueDateStr := strings.TrimSpace(store.DateString(task.DueDate))
		if dueDateStr == "" {
			continue
		}
		due, err := parseDate(dueDateStr)
		if err != nil {
			continue
		}
		if due.Before(today) {
			overdueCount++
			spotlightCandidates = append(spotlightCandidates, task)
		} else if sameDay(due, today) {
			dueTodayCount++
			spotlightCandidates = append(spotlightCandidates, task)
		}
	}

	sort.SliceStable(spotlightCandidates, func(i, j int) bool {
		a, b := spotlightCandidates[i], spotlightCandidates[j]
		aRank, bRank := taskDueRank(a, today), taskDueRank(b, today)
		if aRank != bRank {
			return aRank > bRank
		}
		aScore, bScore := priorityScore(a.Priority), priorityScore(b.Priority)
		if aScore != bScore {
			return aScore > bScore
		}
		aDue := store.DateString(a.DueDate)
		if aDue == "" {
			aDue = "9999-99-99"
		}
		bDue := store.DateString(b.DueDate)
		if bDue == "" {
			bDue = "9999-99-99"
		}
		return aDue < bDue
	})

	spotlight := []TaskSpotlight{}
	for _, t := range spotlightCandidates {
		if len(spotlight) >= 3 {
			break
		}
		due := store.DateString(t.DueDate)
		spotlight = append(spotlight, TaskSpotlight{
			ID:       t.ID,
			Title:    t.Title,
			Priority: t.Priority,
			DueDate:  strPtrOrNil(due),
		})
	}

	todayStr := formatDate(today)
	daily := []store.Task{}
	for _, t := range tasks {
		if t.IsDaily && !t.DeletedAt.Valid && store.DateString(t.LastCompletedDate) != todayStr {
			daily = append(daily, t)
		}
	}
	sort.SliceStable(daily, func(i, j int) bool {
		a, b := daily[i], daily[j]
		aScore, bScore := priorityScore(a.Priority), priorityScore(b.Priority)
		if aScore != bScore {
			return aScore > bScore
		}
		return a.Title < b.Title
	})

	dailySpotlight := []TaskSpotlight{}
	for _, t := range daily {
		if len(dailySpotlight) >= 3 {
			break
		}
		dailySpotlight = append(dailySpotlight, TaskSpotlight{
			ID:       t.ID,
			Title:    t.Title,
			Priority: t.Priority,
			DueDate:  nil,
		})
	}

	return TodayTasks{
		OverdueCount:         overdueCount,
		DueTodayCount:        dueTodayCount,
		Spotlight:            spotlight,
		DailyUnfinishedCount: int64(len(daily)),
		DailySpotlight:       dailySpotlight,
	}
}

// BuildUpcomingExam ports commands/briefing.rs build_upcoming_exam.
func BuildUpcomingExam(exams []store.Exam, now time.Time) *UpcomingExam {
	upcoming := []store.Exam{}
	for _, exam := range exams {
		start, err := parseRFC3339(exam.ExamDatetime)
		if err != nil {
			continue
		}
		if start.After(now) {
			upcoming = append(upcoming, exam)
		}
	}
	sort.SliceStable(upcoming, func(i, j int) bool {
		a, _ := parseRFC3339(upcoming[i].ExamDatetime)
		b, _ := parseRFC3339(upcoming[j].ExamDatetime)
		return a.Before(b)
	})
	if len(upcoming) == 0 {
		return nil
	}
	exam := upcoming[0]
	start, _ := parseRFC3339(exam.ExamDatetime)
	examDate := midnightOf(start)
	todayDate := midnightOf(now)
	daysUntil := int64(examDate.Sub(todayDate).Hours() / 24)
	return &UpcomingExam{
		ID:           exam.ID,
		CourseName:   exam.CourseName,
		ExamDatetime: store.TSString(exam.ExamDatetime),
		DaysUntil:    daysUntil,
		Location:     exam.Location,
	}
}

// BuildTodayBriefing ports commands/briefing.rs collect_today_briefing.
func BuildTodayBriefing(
	courses []store.Course,
	semesterContexts []store.SemesterContext,
	tasks []store.Task,
	exams []store.Exam,
	phaseStatus termphase.CurrentPhaseStatus,
	pomodoro PomodoroBriefing,
	now time.Time,
) TodayBriefingResponse {
	today := now
	date := formatDate(today)
	weekdayLabel := WeekdayLabel(today)

	coursesBriefing := BuildTodayCourses(courses, semesterContexts, today, now, phaseStatus)
	tasksBriefing := BuildTodayTasks(tasks, today)
	exam := BuildUpcomingExam(exams, now)

	return TodayBriefingResponse{
		Date:         date,
		WeekdayLabel: weekdayLabel,
		Courses:      coursesBriefing,
		Tasks:        tasksBriefing,
		Exam:         exam,
		Pomodoro:     pomodoro,
		Phase:        phaseBriefing(phaseStatus),
	}
}

func phaseBriefing(status termphase.CurrentPhaseStatus) PhaseBriefing {
	return PhaseBriefing{
		PhaseType:                status.PhaseType,
		TermLabel:                status.TermLabel,
		CurrentWeek:              status.CurrentWeek,
		CoursesVisible:           status.CoursesVisible,
		ExamNotificationsEnabled: status.ExamNotificationsEnabled,
		PomodoroProfile:          status.PomodoroProfile,
	}
}

func strPtrOrNil(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}
