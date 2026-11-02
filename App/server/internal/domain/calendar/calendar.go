// Package calendar ports the Rust schedule.rs + commands/briefing.rs
// aggregation logic (src-tauri/src/schedule.rs) into pure, testable Go
// functions. It builds the week-schedule (courses+exams), the calendar-week
// (courses+exams+due-dated tasks) and the today-briefing overview.
//
// The functions are pure: they take store models and return plain structs,
// so they can be unit-tested without a database. Handlers load the store rows
// and call these functions.
package calendar

import (
	"encoding/json"
	"fmt"
	"math"
	"sort"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	"kairos/server/internal/store"
)

// ChinaTZ is the fixed +08:00 timezone used for all date/time display.
var ChinaTZ = time.FixedZone("CST", 8*3600)

// ExamFallbackColor is used for exams not bound to a course.
const ExamFallbackColor = "#DC2626"

// TaskColor is the fixed color for unfinished tasks in the calendar view.
const TaskColor = "#D97706"

// TaskCompletedColor is the fixed color for completed tasks in the calendar view.
const TaskCompletedColor = "#14B8A6"

// WeekScheduleItem mirrors the frontend WeekScheduleItem.
type WeekScheduleItem struct {
	Kind        string
	ID          int64
	Title       string
	DayOfWeek   int64
	StartTime   string
	EndTime     string
	Location    string
	Teacher     string
	Color       string
	Notes       string
	WeekPattern string
	CourseID    *int64
}

// WeekScheduleResponse mirrors the frontend WeekScheduleResponse.
type WeekScheduleResponse struct {
	WeekIndex         int64
	Semester          string
	SemesterStartDate string
	WeekStartDate     string
	WeekEndDate       string
	PhaseType         string
	CoursesVisible    bool
	Items             []WeekScheduleItem
}

// CalendarEvent mirrors the frontend CalendarEvent.
type CalendarEvent struct {
	Kind       string
	ID         int64
	Title      string
	DayOfWeek  int64
	StartTime  string
	EndTime    string
	Location   string
	Color      string
	Tags       []string
	SourceLink string
}

// CalendarWeekResponse mirrors the frontend CalendarWeekResponse.
type CalendarWeekResponse struct {
	WeekIndex         int64
	Semester          string
	SemesterStartDate string
	WeekStartDate     string
	WeekEndDate       string
	PhaseType         string
	CoursesVisible    bool
	Events            []CalendarEvent
}

// MatchesWeekPattern reports whether a course week pattern includes the given
// teaching week index. Ports schedule.rs matches_week_pattern + segment_matches.
func MatchesWeekPattern(weekPattern string, weekIndex int64) bool {
	normalized := strings.ReplaceAll(weekPattern, " ", "")
	if normalized == "" {
		return true
	}
	for _, segment := range strings.FieldsFunc(normalized, func(r rune) bool {
		return r == ',' || r == '，' || r == ';' || r == '；' || r == '、' || r == '/'
	}) {
		if segment == "" {
			continue
		}
		if segmentMatches(segment, weekIndex) {
			return true
		}
	}
	return false
}

func segmentMatches(segment string, weekIndex int64) bool {
	rangePart, tail := segment, ""
	if idx := strings.Index(segment, "周"); idx >= 0 {
		rangePart = segment[:idx]
		tail = segment[idx+1:]
	}
	rangePart = strings.TrimPrefix(rangePart, "第")

	var start, end int64
	var bare string
	if dash := strings.Index(rangePart, "-"); dash >= 0 {
		s, err1 := parseInt64(rangePart[:dash])
		e, err2 := parseInt64(rangePart[dash+1:])
		if err1 != nil || err2 != nil {
			return false
		}
		start, end = s, e
	} else if single, err := parseInt64(rangePart); err == nil {
		start, end = single, single
	} else if rangePart == "单" || rangePart == "双" || rangePart == "全" {
		bare = rangePart
		start, end = 1, math.MaxInt64
	} else {
		return false
	}

	if weekIndex < start || weekIndex > end {
		return false
	}
	if bare == "单" {
		return weekIndex%2 == 1
	}
	if bare == "双" {
		return weekIndex%2 == 0
	}
	if strings.Contains(tail, "单") {
		return weekIndex%2 == 1
	}
	if strings.Contains(tail, "双") {
		return weekIndex%2 == 0
	}
	return true
}

func parseInt64(s string) (int64, error) {
	var v int64
	_, err := fmt.Sscanf(s, "%d", &v)
	if err != nil {
		return 0, err
	}
	return v, nil
}

// CurrentWeekIndexFromStart computes the teaching week index for a date given
// the semester anchor date. Ports schedule.rs current_week_index_from_start.
func CurrentWeekIndexFromStart(semesterStartDate string, date time.Time) (int64, error) {
	anchor, err := time.ParseInLocation("2006-01-02", semesterStartDate, ChinaTZ)
	if err != nil {
		return 0, fmt.Errorf("无法解析学期开始日期: %s", semesterStartDate)
	}
	return computeWeekIndex(anchor, date), nil
}

func computeWeekIndex(anchor, date time.Time) int64 {
	anchorWeekStart := midnightOf(anchor.AddDate(0, 0, -daysFromMonday(anchor)))
	weekStart := midnightOf(date.AddDate(0, 0, -daysFromMonday(date)))
	diffDays := int64(weekStart.Sub(anchorWeekStart).Hours() / 24)
	return floorDiv(diffDays, 7) + 1
}

func daysFromMonday(t time.Time) int {
	return (int(t.Weekday()) + 6) % 7
}

func floorDiv(a, b int64) int64 {
	q := a / b
	if a%b != 0 && ((a < 0) != (b < 0)) {
		q--
	}
	return q
}

func midnightOf(t time.Time) time.Time {
	return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, ChinaTZ)
}

func mondayOf(t time.Time) time.Time {
	return midnightOf(t.AddDate(0, 0, -daysFromMonday(t)))
}

func parseDate(s string) (time.Time, error) {
	return time.ParseInLocation("2006-01-02", s, ChinaTZ)
}

func formatDate(t time.Time) string {
	return t.Format("2006-01-02")
}

// resolveSemesterStartDate picks the anchor date: the requested value wins,
// otherwise the first course with a non-empty semester_start_date.
func resolveSemesterStartDate(courses []store.Course, requested *string) (time.Time, error) {
	candidate := ""
	if requested != nil && strings.TrimSpace(*requested) != "" {
		candidate = strings.TrimSpace(*requested)
	} else {
		for _, c := range courses {
			if d := store.DateString(c.SemesterStartDate); strings.TrimSpace(d) != "" {
				candidate = strings.TrimSpace(d)
				break
			}
		}
	}
	if candidate == "" {
		return time.Time{}, fmt.Errorf("缺少学期开始日期，无法计算周视图。")
	}
	return parseDate(candidate)
}

// BuildWeekSchedule ports schedule.rs build_week_schedule.
func BuildWeekSchedule(
	courses []store.Course,
	exams []store.Exam,
	semester string,
	weekIndex int64,
	requestedSemesterStartDate *string,
	phaseType string,
	coursesVisible bool,
) (WeekScheduleResponse, error) {
	if weekIndex < 1 {
		return WeekScheduleResponse{}, fmt.Errorf("week_index 必须大于等于 1")
	}

	semesterCourses := filterCoursesBySemester(courses, semester)
	semesterExams := filterExamsBySemester(exams, semester)

	anchor, err := resolveSemesterStartDate(semesterCourses, requestedSemesterStartDate)
	if err != nil {
		return WeekScheduleResponse{}, err
	}
	rawStart := anchor.AddDate(0, 0, int((weekIndex-1)*7))
	weekStart := mondayOf(rawStart)
	weekEnd := weekStart.AddDate(0, 0, 6)

	courseColors := map[int64]string{}
	for _, c := range courses {
		courseColors[c.ID] = c.Color
	}

	items := []WeekScheduleItem{}
	if coursesVisible {
		for _, course := range semesterCourses {
			if !MatchesWeekPattern(course.WeekPattern, weekIndex) {
				continue
			}
			id := course.ID
			items = append(items, WeekScheduleItem{
				Kind:        "course",
				ID:          course.ID,
				Title:       course.Name,
				DayOfWeek:   int64(course.DayOfWeek),
				StartTime:   store.ClockString(course.StartTime),
				EndTime:     store.ClockString(course.EndTime),
				Location:    course.Location,
				Teacher:     course.Teacher,
				Color:       course.Color,
				Notes:       "",
				WeekPattern: course.WeekPattern,
				CourseID:    &id,
			})
		}
	}

	for _, exam := range semesterExams {
		start, end, examDate, color, err := parseExamTimeAndColor(exam, courseColors)
		if err != nil {
			return WeekScheduleResponse{}, err
		}
		if examDate.Before(weekStart) || examDate.After(weekEnd) {
			continue
		}
		items = append(items, WeekScheduleItem{
			Kind:        "exam",
			ID:          exam.ID,
			Title:       exam.CourseName,
			DayOfWeek:   int64(examDate.Weekday()+6)%7 + 1,
			StartTime:   start.Format("15:04"),
			EndTime:     end.Format("15:04"),
			Location:    exam.Location,
			Teacher:     "",
			Color:       color,
			Notes:       exam.Notes,
			WeekPattern: "",
			CourseID:    int64PtrOrNil(exam.CourseID),
		})
	}

	sort.SliceStable(items, func(i, j int) bool {
		if items[i].DayOfWeek != items[j].DayOfWeek {
			return items[i].DayOfWeek < items[j].DayOfWeek
		}
		if items[i].StartTime != items[j].StartTime {
			return items[i].StartTime < items[j].StartTime
		}
		return items[i].Kind < items[j].Kind
	})

	return WeekScheduleResponse{
		WeekIndex:         weekIndex,
		Semester:          semester,
		SemesterStartDate: formatDate(anchor),
		WeekStartDate:     formatDate(weekStart),
		WeekEndDate:       formatDate(weekEnd),
		PhaseType:         phaseType,
		CoursesVisible:    coursesVisible,
		Items:             items,
	}, nil
}

// BuildCalendarWeek ports schedule.rs build_calendar_week.
func BuildCalendarWeek(
	courses []store.Course,
	exams []store.Exam,
	tasks []store.Task,
	semester string,
	weekIndex int64,
	requestedSemesterStartDate *string,
	requestedWeekStartDate *string,
	phaseType string,
	coursesVisible bool,
) (CalendarWeekResponse, error) {
	if weekIndex < 1 {
		return CalendarWeekResponse{}, fmt.Errorf("week_index 必须大于等于 1")
	}

	semesterCourses := filterCoursesBySemester(courses, semester)

	anchor, err := resolveCalendarStartDate(semesterCourses, requestedSemesterStartDate, requestedWeekStartDate)
	if err != nil {
		return CalendarWeekResponse{}, err
	}

	var weekStart time.Time
	if requestedWeekStartDate != nil && strings.TrimSpace(*requestedWeekStartDate) != "" {
		weekStart = normalizeWeekStart(*requestedWeekStartDate)
	} else {
		rawStart := anchor.AddDate(0, 0, int((weekIndex-1)*7))
		weekStart = mondayOf(rawStart)
	}
	weekEnd := weekStart.AddDate(0, 0, 6)
	effectiveWeekIndex := computeWeekIndex(anchor, weekStart)

	courseColors := map[int64]string{}
	for _, c := range courses {
		courseColors[c.ID] = c.Color
	}

	events := []CalendarEvent{}
	if coursesVisible {
		for _, course := range semesterCourses {
			if effectiveWeekIndex < 1 || !MatchesWeekPattern(course.WeekPattern, effectiveWeekIndex) {
				continue
			}
			events = append(events, CalendarEvent{
				Kind:       "course",
				ID:         course.ID,
				Title:      course.Name,
				DayOfWeek:  int64(course.DayOfWeek),
				StartTime:  store.ClockString(course.StartTime),
				EndTime:    store.ClockString(course.EndTime),
				Location:   course.Location,
				Color:      course.Color,
				Tags:       []string{},
				SourceLink: "courses",
			})
		}
	}

	today := time.Now().In(ChinaTZ)

	for _, exam := range exams {
		start, end, examDate, color, err := parseExamTimeAndColor(exam, courseColors)
		if err != nil {
			return CalendarWeekResponse{}, err
		}
		if examDate.Before(weekStart) || examDate.After(weekEnd) {
			continue
		}
		tags := []string{"考试"}
		if exam.Notes != "" {
			tags = append(tags, exam.Notes)
		}
		events = append(events, CalendarEvent{
			Kind:       "exam",
			ID:         exam.ID,
			Title:      exam.CourseName,
			DayOfWeek:  int64(examDate.Weekday()+6)%7 + 1,
			StartTime:  start.Format("15:04"),
			EndTime:    end.Format("15:04"),
			Location:   exam.Location,
			Color:      color,
			Tags:       tags,
			SourceLink: "exams",
		})
	}

	for _, task := range tasks {
		if task.IsDaily {
			createdDate := parseCreatedAtDate(task.CreatedAt)
			for offset := 0; offset < 7; offset++ {
				day := weekStart.AddDate(0, 0, offset)
				if createdDate != nil && day.Before(*createdDate) {
					continue
				}
				isDone := false
				if d := store.DateString(task.LastCompletedDate); d != "" {
					if completed, err := parseDate(d); err == nil {
						isDone = !day.After(completed)
					}
				}
				color := TaskColor
				if isDone {
					color = TaskCompletedColor
				}
				tags := []string{"每日"}
				if isDone {
					tags = append(tags, "完成")
				}
				if sameDay(day, today) {
					tags = append(tags, "截止")
				}
				tags = append(tags, parseTaskTags(task.Tags)...)
				events = append(events, CalendarEvent{
					Kind:       "task",
					ID:         task.ID,
					Title:      task.Title,
					DayOfWeek:  int64(day.Weekday()+6)%7 + 1,
					StartTime:  "00:00",
					EndTime:    "00:00",
					Location:   "",
					Color:      color,
					Tags:       tags,
					SourceLink: "todo",
				})
			}
			continue
		}

		dueDateStr := strings.TrimSpace(store.DateString(task.DueDate))
		if dueDateStr == "" {
			continue
		}
		dueDate, err := parseDate(dueDateStr)
		if err != nil {
			continue
		}
		if dueDate.Before(weekStart) || dueDate.After(weekEnd) {
			continue
		}

		isDone := task.Status == "done"
		color := TaskColor
		if isDone {
			color = TaskCompletedColor
		}
		tags := []string{}
		if isDone {
			tags = append(tags, "完成")
		}
		if !isDone && sameDay(dueDate, today) {
			tags = append(tags, "截止")
		}
		tags = append(tags, parseTaskTags(task.Tags)...)
		events = append(events, CalendarEvent{
			Kind:       "task",
			ID:         task.ID,
			Title:      task.Title,
			DayOfWeek:  int64(dueDate.Weekday()+6)%7 + 1,
			StartTime:  "00:00",
			EndTime:    "00:00",
			Location:   "",
			Color:      color,
			Tags:       tags,
			SourceLink: "todo",
		})
	}

	sort.SliceStable(events, func(i, j int) bool {
		if events[i].DayOfWeek != events[j].DayOfWeek {
			return events[i].DayOfWeek < events[j].DayOfWeek
		}
		if events[i].StartTime != events[j].StartTime {
			return events[i].StartTime < events[j].StartTime
		}
		return events[i].Kind < events[j].Kind
	})

	return CalendarWeekResponse{
		WeekIndex:         effectiveWeekIndex,
		Semester:          semester,
		SemesterStartDate: formatDate(anchor),
		WeekStartDate:     formatDate(weekStart),
		WeekEndDate:       formatDate(weekEnd),
		PhaseType:         phaseType,
		CoursesVisible:    coursesVisible,
		Events:            events,
	}, nil
}

func resolveCalendarStartDate(courses []store.Course, requested, requestedWeekStartDate *string) (time.Time, error) {
	hasRequested := requested != nil && strings.TrimSpace(*requested) != ""
	date, err := resolveSemesterStartDate(courses, requested)
	if err == nil {
		return date, nil
	}
	if hasRequested || anyCourseHasStartDate(courses) {
		return time.Time{}, err
	}
	if requestedWeekStartDate != nil && strings.TrimSpace(*requestedWeekStartDate) != "" {
		return normalizeWeekStart(*requestedWeekStartDate), nil
	}
	today := time.Now().In(ChinaTZ)
	return mondayOf(today), nil
}

func anyCourseHasStartDate(courses []store.Course) bool {
	for _, c := range courses {
		if strings.TrimSpace(store.DateString(c.SemesterStartDate)) != "" {
			return true
		}
	}
	return false
}

func normalizeWeekStart(value string) time.Time {
	date, err := parseDate(strings.TrimSpace(value))
	if err != nil {
		return time.Time{}
	}
	return mondayOf(date)
}

func parseExamTimeAndColor(exam store.Exam, courseColors map[int64]string) (time.Time, time.Time, time.Time, string, error) {
	start, err := parseRFC3339(exam.ExamDatetime)
	if err != nil {
		return time.Time{}, time.Time{}, time.Time{}, "", fmt.Errorf("无法解析考试开始时间: %s", store.TSString(exam.ExamDatetime))
	}
	end := start
	if exam.ExamEndDatetime.Valid {
		if e, err := parseRFC3339(exam.ExamEndDatetime); err == nil {
			end = e
		}
	}
	examDate := start
	color := ExamFallbackColor
	if exam.CourseID.Valid {
		if c, ok := courseColors[exam.CourseID.Int64]; ok {
			color = c
		}
	}
	return start, end, examDate, color, nil
}

func parseRFC3339(t pgtype.Timestamptz) (time.Time, error) {
	if !t.Valid {
		return time.Time{}, fmt.Errorf("invalid timestamp")
	}
	return t.Time.In(ChinaTZ), nil
}

func parseCreatedAtDate(t pgtype.Timestamptz) *time.Time {
	if !t.Valid {
		return nil
	}
	dt := t.Time.In(ChinaTZ)
	midnight := midnightOf(dt)
	return &midnight
}

func parseTaskTags(tags []byte) []string {
	out := []string{}
	if len(tags) == 0 || string(tags) == "[]" {
		return out
	}
	var parsed []string
	if err := json.Unmarshal(tags, &parsed); err != nil {
		return out
	}
	for _, t := range parsed {
		if t != "" {
			out = append(out, t)
		}
	}
	return out
}

func sameDay(a, b time.Time) bool {
	return a.Year() == b.Year() && a.YearDay() == b.YearDay()
}

func isoWeekday(t time.Time) int64 {
	return int64(t.Weekday()+6)%7 + 1
}

func filterCoursesBySemester(courses []store.Course, semester string) []store.Course {
	if semester == "" {
		return courses
	}
	out := []store.Course{}
	for _, c := range courses {
		if c.Semester == semester {
			out = append(out, c)
		}
	}
	return out
}

func filterExamsBySemester(exams []store.Exam, semester string) []store.Exam {
	if semester == "" {
		return exams
	}
	out := []store.Exam{}
	for _, e := range exams {
		if e.Semester == semester {
			out = append(out, e)
		}
	}
	return out
}

func int64PtrOrNil(v pgtype.Int8) *int64 {
	if !v.Valid {
		return nil
	}
	return &v.Int64
}
