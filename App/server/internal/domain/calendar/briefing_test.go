package calendar

import (
	"testing"
	"time"

	"kairos/server/internal/domain/termphase"
	"kairos/server/internal/store"
)

func sampleBriefingTask(id int64, title, dueDate, status, priority string) store.Task {
	return store.Task{
		ID:        id,
		SyncID:    store.TextOf("task-sync-" + itoa(id)),
		Title:     title,
		Status:    status,
		Priority:  priority,
		DueDate:   store.DateOf(dueDate),
		Tags:      []byte("[]"),
		CreatedAt: store.TSOf("2026-01-01T00:00:00Z"),
		UpdatedAt: store.TSOf("2026-01-01T00:00:00Z"),
	}
}

func taskNoDate(id int64, title string) store.Task {
	t := sampleBriefingTask(id, title, "", "todo", "medium")
	t.DueDate = store.DateOf("")
	return t
}

func sampleBriefingExam(id int64, courseName, examDatetime, examEndDatetime, location string) store.Exam {
	return store.Exam{
		ID:              id,
		SyncID:          store.TextOf("exam-" + itoa(id)),
		CourseName:      courseName,
		ExamDatetime:    store.TSOf(examDatetime),
		ExamEndDatetime: store.TSOf(examEndDatetime),
		Location:        location,
		Semester:        "2026S1",
	}
}

func sampleBriefingCourse(id int64, name string, dayOfWeek int32, startTime, endTime, semesterStartDate string) store.Course {
	return store.Course{
		ID:                id,
		SyncID:            store.TextOf("course-" + itoa(id)),
		Name:              name,
		DayOfWeek:         dayOfWeek,
		StartTime:         store.ClockOf(startTime),
		EndTime:           store.ClockOf(endTime),
		WeekPattern:       "1-17周全周",
		SemesterStartDate: store.DateOf(semesterStartDate),
		Location:          "教室A",
		Teacher:           "李老师",
		Color:             "#3B82F6",
		Semester:          "2026S1",
	}
}

func sampleContext(termLabel, startDate string) store.SemesterContext {
	return store.SemesterContext{
		ID:        1,
		Source:    termphase.DefaultSource,
		TermLabel: termLabel,
		StartDate: store.DateOf(startDate),
	}
}

func teachingPhase() termphase.CurrentPhaseStatus {
	week := int64(17)
	label := "2026S1"
	return termphase.CurrentPhaseStatus{
		Source:                   termphase.DefaultSource,
		TermLabel:                &label,
		PhaseType:                "teaching",
		CurrentWeek:              &week,
		CoursesVisible:           true,
		ExamNotificationsEnabled: true,
		PomodoroProfile:          "default",
		Inferred:                 true,
	}
}

func dateOf(s string) time.Time {
	t, err := time.ParseInLocation("2006-01-02", s, ChinaTZ)
	if err != nil {
		panic(err)
	}
	return t
}

func TestWeekdayLabel(t *testing.T) {
	if got := WeekdayLabel(time.Date(2026, 6, 22, 0, 0, 0, 0, ChinaTZ)); got != "周一" {
		t.Fatalf("WeekdayLabel = %q, want 周一", got)
	}
	if got := WeekdayLabel(time.Date(2026, 6, 28, 0, 0, 0, 0, ChinaTZ)); got != "周日" {
		t.Fatalf("WeekdayLabel = %q, want 周日", got)
	}
}

func TestBuildTodayTasksCounts(t *testing.T) {
	today := dateOf("2026-06-28")
	tasks := []store.Task{
		sampleBriefingTask(1, "逾期任务", "2026-06-27", "todo", "high"),
		sampleBriefingTask(2, "今日到期", "2026-06-28", "todo", "medium"),
		sampleBriefingTask(3, "已完成", "2026-06-28", "done", "low"),
		taskNoDate(4, "无截止日期"),
	}
	result := BuildTodayTasks(tasks, today)
	if result.OverdueCount != 1 {
		t.Fatalf("overdue_count = %d, want 1", result.OverdueCount)
	}
	if result.DueTodayCount != 1 {
		t.Fatalf("due_today_count = %d, want 1", result.DueTodayCount)
	}
	if len(result.Spotlight) != 2 {
		t.Fatalf("spotlight len = %d, want 2", len(result.Spotlight))
	}
}

func TestBuildTodayTasksSpotlightOrdering(t *testing.T) {
	today := dateOf("2026-06-28")
	tasks := []store.Task{
		sampleBriefingTask(1, "低优先级逾期", "2026-06-27", "todo", "low"),
		sampleBriefingTask(2, "高优先级今日", "2026-06-28", "todo", "high"),
		sampleBriefingTask(3, "中优先级逾期", "2026-06-26", "todo", "medium"),
	}
	result := BuildTodayTasks(tasks, today)
	if len(result.Spotlight) != 3 {
		t.Fatalf("spotlight len = %d, want 3", len(result.Spotlight))
	}
	if result.Spotlight[0].Title != "中优先级逾期" {
		t.Fatalf("spotlight[0] = %q, want 中优先级逾期", result.Spotlight[0].Title)
	}
	if result.Spotlight[1].Title != "低优先级逾期" {
		t.Fatalf("spotlight[1] = %q, want 低优先级逾期", result.Spotlight[1].Title)
	}
	if result.Spotlight[2].Title != "高优先级今日" {
		t.Fatalf("spotlight[2] = %q, want 高优先级今日", result.Spotlight[2].Title)
	}
}

func TestBuildTodayTasksSpotlightCappedAtThree(t *testing.T) {
	today := dateOf("2026-06-28")
	tasks := []store.Task{}
	for i := int64(1); i <= 5; i++ {
		tasks = append(tasks, sampleBriefingTask(i, "任务"+itoa(i), "2026-06-27", "todo", "low"))
	}
	result := BuildTodayTasks(tasks, today)
	if result.OverdueCount != 5 {
		t.Fatalf("overdue_count = %d, want 5", result.OverdueCount)
	}
	if len(result.Spotlight) != 3 {
		t.Fatalf("spotlight len = %d, want 3", len(result.Spotlight))
	}
}

func TestBuildUpcomingExam(t *testing.T) {
	now := time.Date(2026, 6, 28, 10, 0, 0, 0, ChinaTZ)
	exams := []store.Exam{
		sampleBriefingExam(1, "高数期末", "2026-07-01T00:00:00Z", "2026-07-01T02:00:00Z", "天山堂A409"),
		sampleBriefingExam(2, "已过去考试", "2026-06-27T00:00:00Z", "2026-06-27T02:00:00Z", "教室A"),
	}
	result := BuildUpcomingExam(exams, now)
	if result == nil {
		t.Fatal("expected an upcoming exam")
	}
	if result.CourseName != "高数期末" {
		t.Fatalf("course_name = %q, want 高数期末", result.CourseName)
	}
	if result.DaysUntil != 3 {
		t.Fatalf("days_until = %d, want 3", result.DaysUntil)
	}
}

func TestBuildUpcomingExamNoneWhenAllPast(t *testing.T) {
	now := time.Date(2026, 7, 1, 12, 0, 0, 0, ChinaTZ)
	exams := []store.Exam{
		sampleBriefingExam(1, "已过去考试", "2026-06-27T00:00:00Z", "2026-06-27T02:00:00Z", "教室A"),
	}
	if result := BuildUpcomingExam(exams, now); result != nil {
		t.Fatalf("expected no upcoming exam, got %+v", result)
	}
}

func TestBuildTodayCoursesNoMatchingDay(t *testing.T) {
	today := dateOf("2026-06-28")
	nowTime := time.Date(2026, 6, 28, 10, 0, 0, 0, ChinaTZ)
	courses := []store.Course{sampleBriefingCourse(1, "周一课程", 1, "10:00", "11:40", "2026-02-24")}
	result := BuildTodayCourses(courses, nil, today, nowTime, teachingPhase())
	if result.TodayCount != 0 {
		t.Fatalf("today_count = %d, want 0", result.TodayCount)
	}
	if result.CurrentCourse != nil || result.NextCourse != nil {
		t.Fatal("expected no current/next course")
	}
}

func TestBuildTodayCoursesNextCourse(t *testing.T) {
	today := dateOf("2026-06-19")
	nowTime := time.Date(2026, 6, 19, 9, 0, 0, 0, ChinaTZ)
	courses := []store.Course{
		sampleBriefingCourse(1, "上午课程", 5, "08:00", "09:40", "2026-02-24"),
		sampleBriefingCourse(2, "下午课程", 5, "14:00", "15:40", "2026-02-24"),
	}
	result := BuildTodayCourses(courses, nil, today, nowTime, teachingPhase())
	if result.TodayCount != 2 {
		t.Fatalf("today_count = %d, want 2", result.TodayCount)
	}
	if result.CurrentCourse == nil || result.CurrentCourse.Title != "上午课程" {
		t.Fatalf("current_course = %+v, want 上午课程", result.CurrentCourse)
	}
	if result.NextCourse == nil || result.NextCourse.Title != "下午课程" {
		t.Fatalf("next_course = %+v, want 下午课程", result.NextCourse)
	}
}

func TestBuildTodayCoursesUsesSemesterContextStartDate(t *testing.T) {
	today := dateOf("2026-06-19")
	nowTime := time.Date(2026, 6, 19, 7, 0, 0, 0, ChinaTZ)
	courses := []store.Course{sampleBriefingCourse(1, "上下文校准课程", 5, "08:00", "09:40", "2026-02-03")}
	contexts := []store.SemesterContext{sampleContext("2026S1", "2026-02-24")}

	result := BuildTodayCourses(courses, contexts, today, nowTime, teachingPhase())
	if result.TodayCount != 1 {
		t.Fatalf("today_count = %d, want 1", result.TodayCount)
	}
	if result.NextCourse == nil || result.NextCourse.Title != "上下文校准课程" {
		t.Fatalf("next_course = %+v, want 上下文校准课程", result.NextCourse)
	}
}

func TestBuildTodayCoursesFallsBackWhenContextMissing(t *testing.T) {
	today := dateOf("2026-06-19")
	nowTime := time.Date(2026, 6, 19, 7, 0, 0, 0, ChinaTZ)
	courses := []store.Course{sampleBriefingCourse(1, "课程字段锚点", 5, "08:00", "09:40", "2026-02-24")}

	result := BuildTodayCourses(courses, nil, today, nowTime, teachingPhase())
	if result.TodayCount != 1 {
		t.Fatalf("today_count = %d, want 1", result.TodayCount)
	}
}

func TestBuildTodayCoursesHidesCoursesWhenPhaseHidden(t *testing.T) {
	today := dateOf("2026-06-19")
	nowTime := time.Date(2026, 6, 19, 7, 0, 0, 0, ChinaTZ)
	phase := teachingPhase()
	phase.PhaseType = "break"
	phase.CoursesVisible = false
	courses := []store.Course{sampleBriefingCourse(1, "假期隐藏课程", 5, "08:00", "09:40", "2026-02-24")}

	result := BuildTodayCourses(courses, nil, today, nowTime, phase)
	if result.TodayCount != 0 {
		t.Fatalf("today_count = %d, want 0", result.TodayCount)
	}
	if result.NextCourse != nil {
		t.Fatal("expected no next course when phase hidden")
	}
}

func TestBuildTodayCoursesHidesCoursesWhenPhaseUnknown(t *testing.T) {
	today := dateOf("2026-07-31")
	nowTime := time.Date(2026, 7, 31, 7, 0, 0, 0, ChinaTZ)
	phase := teachingPhase()
	phase.PhaseType = termphase.PhaseUnknown
	phase.CoursesVisible = true
	courses := []store.Course{sampleBriefingCourse(1, "无上下文课程", 5, "08:00", "09:40", "2026-03-09")}

	result := BuildTodayCourses(courses, nil, today, nowTime, phase)
	if result.TodayCount != 0 {
		t.Fatalf("today_count = %d, want 0", result.TodayCount)
	}
	if result.CurrentCourse != nil || result.NextCourse != nil {
		t.Fatal("expected no courses when phase unknown")
	}
}

func TestBuildTodayTasksDailyUnfinished(t *testing.T) {
	today := dateOf("2026-08-01")
	dailyDone := sampleBriefingTask(1, "已完成的每日", "2026-08-01", "done", "medium")
	dailyDone.IsDaily = true
	dailyDone.LastCompletedDate = store.DateOf("2026-08-01")
	dailyYesterday := sampleBriefingTask(2, "昨日完成的每日", "2026-08-01", "done", "medium")
	dailyYesterday.IsDaily = true
	dailyYesterday.LastCompletedDate = store.DateOf("2026-07-31")
	normal := sampleBriefingTask(3, "普通任务", "2026-08-01", "todo", "medium")

	result := BuildTodayTasks([]store.Task{dailyDone, dailyYesterday, normal}, today)
	if result.DailyUnfinishedCount != 1 {
		t.Fatalf("daily_unfinished_count = %d, want 1", result.DailyUnfinishedCount)
	}
	if len(result.DailySpotlight) != 1 {
		t.Fatalf("daily_spotlight len = %d, want 1", len(result.DailySpotlight))
	}
	if result.DailySpotlight[0].Title != "昨日完成的每日" {
		t.Fatalf("daily_spotlight[0] = %q, want 昨日完成的每日", result.DailySpotlight[0].Title)
	}
}

func TestBuildTodayTasksDailySpotlightOrdering(t *testing.T) {
	today := dateOf("2026-08-01")
	low := sampleBriefingTask(1, "低优先级", "2026-08-01", "todo", "low")
	low.IsDaily = true
	high := sampleBriefingTask(2, "高优先级", "2026-08-01", "todo", "high")
	high.IsDaily = true

	result := BuildTodayTasks([]store.Task{low, high}, today)
	if result.DailyUnfinishedCount != 2 {
		t.Fatalf("daily_unfinished_count = %d, want 2", result.DailyUnfinishedCount)
	}
	if result.DailySpotlight[0].Title != "高优先级" {
		t.Fatalf("daily_spotlight[0] = %q, want 高优先级", result.DailySpotlight[0].Title)
	}
}

func TestPriorityScore(t *testing.T) {
	if got := priorityScore("high"); got != 3 {
		t.Fatalf("priorityScore(high) = %d, want 3", got)
	}
	if got := priorityScore("medium"); got != 2 {
		t.Fatalf("priorityScore(medium) = %d, want 2", got)
	}
	if got := priorityScore("low"); got != 1 {
		t.Fatalf("priorityScore(low) = %d, want 1", got)
	}
	if got := priorityScore("unknown"); got != 0 {
		t.Fatalf("priorityScore(unknown) = %d, want 0", got)
	}
}
