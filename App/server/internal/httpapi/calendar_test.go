package httpapi

import (
	"net/http"
	"testing"

	"kairos/server/internal/httpapi/dto"
)

func TestCalendarWeekAggregatesCourseAndExam(t *testing.T) {
	h, token := newCoreTestRouter(t)

	rec := doJSON(t, h, http.MethodPost, "/api/courses", dto.CreateCourseRequest{
		Name: "自动控制原理", DayOfWeek: 3, StartTime: "10:00", EndTime: "11:40",
		WeekPattern: strPtr("1-16周单周"), SemesterStartDate: strPtr("2026-02-24"),
		Semester: strPtr("2026S1"),
	}, token)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create course status = %d, want 201; body=%s", rec.Code, rec.Body.String())
	}

	rec = doJSON(t, h, http.MethodPost, "/api/exams", dto.CreateExamRequest{
		CourseName: "自动控制原理", ExamDatetime: "2026-02-25T00:00:00Z",
		ExamEndDatetime: strPtr("2026-02-25T02:00:00Z"), Location: strPtr("天山堂A409"),
		Semester: strPtr("2026S1"),
	}, token)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create exam status = %d, want 201; body=%s", rec.Code, rec.Body.String())
	}

	rec = doJSON(t, h, http.MethodGet,
		"/api/calendar/week?semester=2026S1&week_index=1&semester_start_date=2026-02-24", nil, token)
	if rec.Code != http.StatusOK {
		t.Fatalf("week status = %d, want 200; body=%s", rec.Code, rec.Body.String())
	}
	var resp dto.WeekScheduleResponse
	decodeBody(t, rec, &resp)
	if resp.WeekStartDate != "2026-02-23" {
		t.Fatalf("week_start_date = %q, want 2026-02-23", resp.WeekStartDate)
	}
	if len(resp.Items) != 2 {
		t.Fatalf("items len = %d, want 2", len(resp.Items))
	}
	if resp.Items[0].Kind != "exam" || resp.Items[1].Kind != "course" {
		t.Fatalf("unexpected items order: %+v", resp.Items)
	}
	if resp.Items[1].CourseID == nil || *resp.Items[1].CourseID != resp.Items[1].ID {
		t.Fatalf("course item course_id should equal its id: %+v", resp.Items[1])
	}
}

func TestCalendarWeekRejectsInvalidWeekIndex(t *testing.T) {
	h, token := newCoreTestRouter(t)
	rec := doJSON(t, h, http.MethodGet, "/api/calendar/week?semester=2026S1&week_index=0", nil, token)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("week status = %d, want 400", rec.Code)
	}
}

func TestCalendarWeekEventsAggregatesCourseAndTask(t *testing.T) {
	h, token := newCoreTestRouter(t)

	rec := doJSON(t, h, http.MethodPost, "/api/courses", dto.CreateCourseRequest{
		Name: "自动控制原理", DayOfWeek: 3, StartTime: "10:00", EndTime: "11:40",
		WeekPattern: strPtr("1-16周单周"), SemesterStartDate: strPtr("2026-02-24"),
		Semester: strPtr("2026S1"),
	}, token)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create course status = %d, want 201; body=%s", rec.Code, rec.Body.String())
	}

	rec = doJSON(t, h, http.MethodPost, "/api/tasks", dto.CreateTaskRequest{
		Title: "提交作业", DueDate: strPtr("2026-02-24"),
	}, token)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create task status = %d, want 201; body=%s", rec.Code, rec.Body.String())
	}

	rec = doJSON(t, h, http.MethodGet,
		"/api/calendar/week-events?semester=2026S1&week_index=1&semester_start_date=2026-02-24&week_start_date=2026-02-23", nil, token)
	if rec.Code != http.StatusOK {
		t.Fatalf("week-events status = %d, want 200; body=%s", rec.Code, rec.Body.String())
	}
	var resp dto.CalendarWeekResponse
	decodeBody(t, rec, &resp)
	if resp.WeekStartDate != "2026-02-23" {
		t.Fatalf("week_start_date = %q, want 2026-02-23", resp.WeekStartDate)
	}
	var courseCount, taskCount int
	for _, e := range resp.Events {
		switch e.Kind {
		case "course":
			courseCount++
		case "task":
			taskCount++
		}
	}
	if courseCount != 1 {
		t.Fatalf("course events = %d, want 1; events=%+v", courseCount, resp.Events)
	}
	if taskCount != 1 {
		t.Fatalf("task events = %d, want 1; events=%+v", taskCount, resp.Events)
	}
}

func TestCalendarDayIncludesDueTask(t *testing.T) {
	h, token := newCoreTestRouter(t)

	rec := doJSON(t, h, http.MethodPost, "/api/tasks", dto.CreateTaskRequest{
		Title: "提交作业", DueDate: strPtr("2026-02-24"),
	}, token)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create task status = %d, want 201; body=%s", rec.Code, rec.Body.String())
	}

	rec = doJSON(t, h, http.MethodGet, "/api/calendar/day?date=2026-02-24&semester=2026S1", nil, token)
	if rec.Code != http.StatusOK {
		t.Fatalf("day status = %d, want 200; body=%s", rec.Code, rec.Body.String())
	}
	var resp dto.CalendarWeekResponse
	decodeBody(t, rec, &resp)
	if resp.WeekStartDate != "2026-02-23" {
		t.Fatalf("week_start_date = %q, want 2026-02-23", resp.WeekStartDate)
	}
	found := false
	for _, e := range resp.Events {
		if e.Kind == "task" && e.Title == "提交作业" {
			found = true
			if e.SourceLink != "todo" {
				t.Fatalf("task source_link = %q, want todo", e.SourceLink)
			}
		}
	}
	if !found {
		t.Fatalf("expected task event in day response, got %+v", resp.Events)
	}
}

func TestCalendarDayRejectsInvalidDate(t *testing.T) {
	h, token := newCoreTestRouter(t)
	rec := doJSON(t, h, http.MethodGet, "/api/calendar/day?date=not-a-date", nil, token)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("day status = %d, want 400", rec.Code)
	}
}

func TestBriefingToday(t *testing.T) {
	h, token := newCoreTestRouter(t)

	rec := doJSON(t, h, http.MethodGet, "/api/briefing/today", nil, token)
	if rec.Code != http.StatusOK {
		t.Fatalf("briefing status = %d, want 200; body=%s", rec.Code, rec.Body.String())
	}
	var resp dto.TodayBriefingResponse
	decodeBody(t, rec, &resp)
	if resp.Date == "" {
		t.Fatal("briefing date should not be empty")
	}
	if resp.WeekdayLabel == "" {
		t.Fatal("briefing weekday_label should not be empty")
	}
	if resp.Phase.PhaseType == "" {
		t.Fatal("briefing phase_type should not be empty")
	}
	if resp.Pomodoro.Phase == "" {
		t.Fatal("briefing pomodoro phase should not be empty")
	}
}

func TestCalendarAndBriefingRequireAuth(t *testing.T) {
	h, _ := newCoreTestRouter(t)

	rec := doJSON(t, h, http.MethodGet, "/api/calendar/week?semester=2026S1&week_index=1", nil, "")
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("calendar/week without token status = %d, want 401", rec.Code)
	}
	rec = doJSON(t, h, http.MethodGet, "/api/calendar/day?date=2026-02-24", nil, "")
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("calendar/day without token status = %d, want 401", rec.Code)
	}
	rec = doJSON(t, h, http.MethodGet, "/api/briefing/today", nil, "")
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("briefing/today without token status = %d, want 401", rec.Code)
	}
}
