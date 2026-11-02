package calendar

import (
	"strconv"
	"testing"
	"time"

	"kairos/server/internal/store"
)

func sampleCourse() store.Course {
	return store.Course{
		ID:                1,
		SyncID:            store.TextOf("course-sync-1"),
		Name:              "自动控制原理",
		DayOfWeek:         3,
		StartTime:         store.ClockOf("10:00"),
		EndTime:           store.ClockOf("11:40"),
		WeekPattern:       "1-16周单周",
		SemesterStartDate: store.DateOf("2026-02-24"),
		Location:          "秦岭堂A114",
		Teacher:           "李红信",
		Color:             "#3B82F6",
		Semester:          "2026S1",
		CreatedAt:         store.TSOf("2026-01-01T00:00:00Z"),
		UpdatedAt:         store.TSOf("2026-01-01T00:00:00Z"),
	}
}

func sampleExam() store.Exam {
	return store.Exam{
		ID:              10,
		SyncID:          store.TextOf("exam-sync-10"),
		CourseName:      "自动控制原理",
		ExamDatetime:    store.TSOf("2026-02-25T00:00:00Z"),
		ExamEndDatetime: store.TSOf("2026-02-25T02:00:00Z"),
		Location:        "天山堂A409",
		Notes:           "正常考试",
		CourseID:        store.Int8Of(1),
		Semester:        "2026S1",
		CreatedAt:       store.TSOf("2026-01-01T00:00:00Z"),
		UpdatedAt:       store.TSOf("2026-01-01T00:00:00Z"),
	}
}

func sampleTask(id int64, title, dueDate, status string) store.Task {
	return store.Task{
		ID:        id,
		SyncID:    store.TextOf("task-sync-" + itoa(id)),
		Title:     title,
		Status:    status,
		Priority:  "medium",
		DueDate:   store.DateOf(dueDate),
		Tags:      []byte("[]"),
		CreatedAt: store.TSOf("2026-01-01T00:00:00Z"),
		UpdatedAt: store.TSOf("2026-01-01T00:00:00Z"),
		IsDaily:   false,
	}
}

func sampleDailyTask(id int64, title string, lastCompleted *string) store.Task {
	t := sampleTask(id, title, "", "todo")
	t.DueDate = store.DateOf("")
	t.IsDaily = true
	if lastCompleted != nil {
		t.Status = "done"
		t.LastCompletedDate = store.DateOf(*lastCompleted)
	}
	return t
}

func itoa(v int64) string {
	return strconv.FormatInt(v, 10)
}

func TestMatchesWeekPattern(t *testing.T) {
	cases := []struct {
		pattern string
		week    int64
		want    bool
	}{
		{"1-16周全周", 8, true},
		{"1-16周单周", 9, true},
		{"1-16周单周", 10, false},
		{"2-18周双周", 10, true},
		{"", 5, true},
		{"1-16", 8, true},
		{"单周", 3, true},
		{"单周", 4, false},
		{"双周", 4, true},
		{"双周", 3, false},
		{"全周", 7, true},
		{"1-8周,10-16周", 12, true},
		{"1-8周,10-16周", 9, false},
		{"第1-16周", 5, true},
		{"1-16周单周", 0, false},
	}
	for _, tc := range cases {
		if got := MatchesWeekPattern(tc.pattern, tc.week); got != tc.want {
			t.Errorf("MatchesWeekPattern(%q, %d) = %v, want %v", tc.pattern, tc.week, got, tc.want)
		}
	}
}

func TestBuildWeekSchedule(t *testing.T) {
	resp, err := BuildWeekSchedule(
		[]store.Course{sampleCourse()},
		[]store.Exam{sampleExam()},
		"2026S1", 1, strPtr("2026-02-24"), "teaching", true,
	)
	if err != nil {
		t.Fatalf("build schedule: %v", err)
	}
	if resp.WeekStartDate != "2026-02-23" {
		t.Fatalf("week_start_date = %q, want 2026-02-23", resp.WeekStartDate)
	}
	if len(resp.Items) != 2 {
		t.Fatalf("items len = %d, want 2", len(resp.Items))
	}
	if resp.Items[0].Kind != "exam" {
		t.Fatalf("items[0].kind = %q, want exam", resp.Items[0].Kind)
	}
	if resp.Items[1].Kind != "course" {
		t.Fatalf("items[1].kind = %q, want course", resp.Items[1].Kind)
	}
}

func TestBuildWeekScheduleRejectsNonPositiveWeek(t *testing.T) {
	if _, err := BuildWeekSchedule(nil, nil, "2026S1", 0, nil, "teaching", true); err == nil {
		t.Fatal("expected error for week_index < 1")
	}
}

func TestBuildWeekScheduleHidesCoursesWhenNotVisible(t *testing.T) {
	resp, err := BuildWeekSchedule(
		[]store.Course{sampleCourse()},
		[]store.Exam{sampleExam()},
		"2026S1", 1, strPtr("2026-02-24"), "break", false,
	)
	if err != nil {
		t.Fatalf("build schedule: %v", err)
	}
	if len(resp.Items) != 1 || resp.Items[0].Kind != "exam" {
		t.Fatalf("expected only the exam when courses hidden, got %+v", resp.Items)
	}
	if resp.CoursesVisible {
		t.Fatal("courses_visible should be false")
	}
}

func TestBuildCalendarWeekCourseAndExam(t *testing.T) {
	resp, err := BuildCalendarWeek(
		[]store.Course{sampleCourse()},
		[]store.Exam{sampleExam()},
		nil, "2026S1", 1, strPtr("2026-02-24"), nil, "teaching", true,
	)
	if err != nil {
		t.Fatalf("build calendar week: %v", err)
	}
	if resp.WeekStartDate != "2026-02-23" {
		t.Fatalf("week_start_date = %q, want 2026-02-23", resp.WeekStartDate)
	}
	if len(resp.Events) != 2 {
		t.Fatalf("events len = %d, want 2", len(resp.Events))
	}
	if resp.Events[0].Kind != "exam" {
		t.Fatalf("events[0].kind = %q, want exam", resp.Events[0].Kind)
	}
	if resp.Events[1].Kind != "course" {
		t.Fatalf("events[1].kind = %q, want course", resp.Events[1].Kind)
	}
}

func TestBuildCalendarWeekIncludesExamByDateEvenWhenSemesterDiffers(t *testing.T) {
	exam := sampleExam()
	exam.Semester = ""
	resp, err := BuildCalendarWeek(
		[]store.Course{sampleCourse()},
		[]store.Exam{exam},
		nil, "2026S1", 1, strPtr("2026-02-24"), nil, "teaching", true,
	)
	if err != nil {
		t.Fatalf("build calendar week: %v", err)
	}
	examEvents := filterEvents(resp.Events, "exam")
	if len(examEvents) != 1 {
		t.Fatalf("exam events len = %d, want 1", len(examEvents))
	}
	if examEvents[0].Title != "自动控制原理" {
		t.Fatalf("exam title = %q, want 自动控制原理", examEvents[0].Title)
	}
}

func TestBuildCalendarWeekWithTasks(t *testing.T) {
	resp, err := BuildCalendarWeek(
		[]store.Course{sampleCourse()},
		nil,
		[]store.Task{sampleTask(100, "提交作业", "2026-02-24", "todo")},
		"2026S1", 1, strPtr("2026-02-24"), nil, "teaching", true,
	)
	if err != nil {
		t.Fatalf("build calendar week: %v", err)
	}
	if resp.WeekStartDate != "2026-02-23" {
		t.Fatalf("week_start_date = %q, want 2026-02-23", resp.WeekStartDate)
	}
	taskEvents := filterEvents(resp.Events, "task")
	if len(taskEvents) != 1 {
		t.Fatalf("task events len = %d, want 1", len(taskEvents))
	}
	if taskEvents[0].Title != "提交作业" {
		t.Fatalf("task title = %q, want 提交作业", taskEvents[0].Title)
	}
	if taskEvents[0].Color != TaskColor {
		t.Fatalf("task color = %q, want %q", taskEvents[0].Color, TaskColor)
	}
	if taskEvents[0].SourceLink != "todo" {
		t.Fatalf("task source_link = %q, want todo", taskEvents[0].SourceLink)
	}
}

func TestBuildCalendarWeekCompletedTask(t *testing.T) {
	resp, err := BuildCalendarWeek(
		nil, nil,
		[]store.Task{sampleTask(200, "已完成作业", "2026-02-25", "done")},
		"2026S1", 1, strPtr("2026-02-24"), nil, "teaching", true,
	)
	if err != nil {
		t.Fatalf("build calendar week: %v", err)
	}
	task := findEvent(resp.Events, "task")
	if task == nil {
		t.Fatal("expected a task event")
	}
	if task.Color != TaskCompletedColor {
		t.Fatalf("task color = %q, want %q", task.Color, TaskCompletedColor)
	}
	if !containsTag(task.Tags, "完成") {
		t.Fatalf("task tags = %v, want to contain 完成", task.Tags)
	}
}

func TestBuildCalendarWeekTaskOutsideWeek(t *testing.T) {
	resp, err := BuildCalendarWeek(
		nil, nil,
		[]store.Task{sampleTask(300, "远期任务", "2026-03-10", "todo")},
		"2026S1", 1, strPtr("2026-02-24"), nil, "teaching", true,
	)
	if err != nil {
		t.Fatalf("build calendar week: %v", err)
	}
	if len(filterEvents(resp.Events, "task")) != 0 {
		t.Fatal("expected no task events for a due date outside the week")
	}
}

func TestBuildCalendarWeekTaskNoDueDate(t *testing.T) {
	task := sampleTask(400, "无截止日期", "", "todo")
	task.DueDate = store.DateOf("")
	resp, err := BuildCalendarWeek(
		nil, nil, []store.Task{task},
		"2026S1", 1, strPtr("2026-02-24"), nil, "teaching", true,
	)
	if err != nil {
		t.Fatalf("build calendar week: %v", err)
	}
	if len(filterEvents(resp.Events, "task")) != 0 {
		t.Fatal("expected no task events without a due date")
	}
}

func TestBuildCalendarWeekWithoutSemesterAnchorUsesCurrentWeek(t *testing.T) {
	today := time.Now().In(ChinaTZ)
	monday := mondayOf(today)
	dueDate := formatDate(monday)

	resp, err := BuildCalendarWeek(
		nil, nil,
		[]store.Task{sampleTask(500, "本周待办", dueDate, "todo")},
		"", 1, nil, nil, "teaching", true,
	)
	if err != nil {
		t.Fatalf("build calendar week: %v", err)
	}
	if resp.WeekStartDate != dueDate {
		t.Fatalf("week_start_date = %q, want %q", resp.WeekStartDate, dueDate)
	}
	if len(filterEvents(resp.Events, "task")) != 1 {
		t.Fatal("expected 1 task event in the current week")
	}
}

func TestBuildCalendarWeekVacationWeekUsesRealDatesWithoutRepeatingCourses(t *testing.T) {
	weekStart := "2026-07-20"
	resp, err := BuildCalendarWeek(
		[]store.Course{sampleCourse()},
		nil,
		[]store.Task{sampleTask(600, "暑假读书计划", "2026-07-20", "todo")},
		"2026S1", 1, strPtr("2026-02-24"), &weekStart, "teaching", true,
	)
	if err != nil {
		t.Fatalf("build calendar week: %v", err)
	}
	if resp.WeekStartDate != "2026-07-20" {
		t.Fatalf("week_start_date = %q, want 2026-07-20", resp.WeekStartDate)
	}
	if resp.WeekIndex <= 16 {
		t.Fatalf("week_index = %d, want > 16", resp.WeekIndex)
	}
	if len(filterEvents(resp.Events, "course")) != 0 {
		t.Fatal("expected no course events in a vacation week")
	}
	taskEvents := filterEvents(resp.Events, "task")
	if len(taskEvents) != 1 || taskEvents[0].Title != "暑假读书计划" {
		t.Fatalf("unexpected task events: %+v", taskEvents)
	}
}

func TestCurrentWeekIndexFromStart(t *testing.T) {
	today := time.Date(2026, 2, 26, 0, 0, 0, 0, ChinaTZ)
	if got, err := CurrentWeekIndexFromStart("2026-02-24", today); err != nil || got != 1 {
		t.Fatalf("CurrentWeekIndexFromStart(2026-02-24, 2026-02-26) = %d, %v; want 1", got, err)
	}
	later := time.Date(2026, 3, 5, 0, 0, 0, 0, ChinaTZ)
	if got, err := CurrentWeekIndexFromStart("2026-02-24", later); err != nil || got != 2 {
		t.Fatalf("CurrentWeekIndexFromStart(2026-02-24, 2026-03-05) = %d, %v; want 2", got, err)
	}
}

func TestBuildCalendarWeekDailyTaskRepeats7Days(t *testing.T) {
	resp, err := BuildCalendarWeek(
		nil, nil,
		[]store.Task{sampleDailyTask(100, "背单词", nil)},
		"2026S1", 1, strPtr("2026-02-24"), nil, "teaching", true,
	)
	if err != nil {
		t.Fatalf("build calendar week: %v", err)
	}
	dailyEvents := filterEvents(resp.Events, "task")
	if len(dailyEvents) != 7 {
		t.Fatalf("daily events len = %d, want 7", len(dailyEvents))
	}
	for dow := int64(1); dow <= 7; dow++ {
		count := 0
		for _, e := range dailyEvents {
			if e.DayOfWeek == dow {
				count++
			}
		}
		if count != 1 {
			t.Errorf("day_of_week %d should appear exactly once, got %d", dow, count)
		}
	}
	for _, e := range dailyEvents {
		if !containsTag(e.Tags, "每日") {
			t.Errorf("daily event tags = %v, want to contain 每日", e.Tags)
		}
		if e.SourceLink != "todo" {
			t.Errorf("daily event source_link = %q, want todo", e.SourceLink)
		}
	}
}

func TestBuildCalendarWeekDailyTaskCompletion(t *testing.T) {
	lastCompleted := "2026-02-26"
	resp, err := BuildCalendarWeek(
		nil, nil,
		[]store.Task{sampleDailyTask(200, "背单词", &lastCompleted)},
		"2026S1", 1, strPtr("2026-02-24"), nil, "teaching", true,
	)
	if err != nil {
		t.Fatalf("build calendar week: %v", err)
	}
	dailyEvents := filterEvents(resp.Events, "task")
	if len(dailyEvents) != 7 {
		t.Fatalf("daily events len = %d, want 7", len(dailyEvents))
	}
	for _, e := range dailyEvents {
		expectDone := e.DayOfWeek <= 4
		isDone := containsTag(e.Tags, "完成")
		if isDone != expectDone {
			t.Errorf("day_of_week=%d done=%v, want %v", e.DayOfWeek, isDone, expectDone)
		}
		expectColor := TaskColor
		if expectDone {
			expectColor = TaskCompletedColor
		}
		if e.Color != expectColor {
			t.Errorf("day_of_week=%d color=%q, want %q", e.DayOfWeek, e.Color, expectColor)
		}
	}
}

func TestBuildCalendarWeekDailyTaskCreatedMidWeek(t *testing.T) {
	task := sampleDailyTask(300, "背单词", nil)
	task.CreatedAt = store.TSOf("2026-02-26T00:00:00Z")

	resp, err := BuildCalendarWeek(
		nil, nil, []store.Task{task},
		"2026S1", 1, strPtr("2026-02-24"), nil, "teaching", true,
	)
	if err != nil {
		t.Fatalf("build calendar week: %v", err)
	}
	dailyEvents := filterEvents(resp.Events, "task")
	if len(dailyEvents) != 4 {
		t.Fatalf("daily events len = %d, want 4", len(dailyEvents))
	}
	for _, e := range dailyEvents {
		if e.DayOfWeek < 4 {
			t.Errorf("day_of_week=%d should be >= 4", e.DayOfWeek)
		}
	}
}

func filterEvents(events []CalendarEvent, kind string) []CalendarEvent {
	out := []CalendarEvent{}
	for _, e := range events {
		if e.Kind == kind {
			out = append(out, e)
		}
	}
	return out
}

func findEvent(events []CalendarEvent, kind string) *CalendarEvent {
	for i := range events {
		if events[i].Kind == kind {
			return &events[i]
		}
	}
	return nil
}

func containsTag(tags []string, want string) bool {
	for _, t := range tags {
		if t == want {
			return true
		}
	}
	return false
}

func strPtr(s string) *string { return &s }
