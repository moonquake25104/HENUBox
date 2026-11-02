package httpapi

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"strconv"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"golang.org/x/crypto/bcrypt"

	"kairos/server/internal/httpapi/dto"
	"kairos/server/internal/store"
	"kairos/server/internal/store/migrate"
)

func testDatabaseURL() string {
	if v := os.Getenv("TEST_DATABASE_URL"); v != "" {
		return v
	}
	return "postgres://kairos:kairos_dev@localhost:5432/kairos_dev"
}

func newCoreTestRouter(t *testing.T) (http.Handler, string) {
	return newCoreTestRouterConfigured(t, nil)
}

func newCoreTestRouterConfigured(t *testing.T, configure func(*Options)) (http.Handler, string) {
	t.Helper()
	ctx := context.Background()
	conn, err := pgx.Connect(ctx, testDatabaseURL())
	if err != nil {
		t.Fatalf("connect to test database: %v", err)
	}
	schema := "httpapi_test_" + randomHex()
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

	hash, err := bcrypt.GenerateFromPassword([]byte("correct-horse"), bcrypt.MinCost)
	if err != nil {
		t.Fatalf("hash password: %v", err)
	}
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	opts := Options{
		Log:          log,
		Username:     "kairos",
		PasswordHash: string(hash),
		JWTSecret:    []byte(testSecret),
		JWTTTL:       time.Hour,
		Store:        store.New(conn),
		Pool:         conn,
	}
	if configure != nil {
		configure(&opts)
	}
	h := New(opts)
	return h, loginToken(t, h)
}

func randomHex() string {
	var b [8]byte
	if _, err := rand.Read(b[:]); err != nil {
		panic(err)
	}
	return hex.EncodeToString(b[:])
}

func loginToken(t *testing.T, h http.Handler) string {
	t.Helper()
	rec := doJSON(t, h, http.MethodPost, "/api/auth/login",
		dto.LoginRequest{Username: "kairos", Password: "correct-horse"}, "")
	if rec.Code != http.StatusOK {
		t.Fatalf("login status = %d, want 200; body=%s", rec.Code, rec.Body.String())
	}
	var resp dto.LoginResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode login response: %v", err)
	}
	return resp.Token
}

func decodeBody[T any](t *testing.T, rec *httptest.ResponseRecorder, out *T) {
	t.Helper()
	if err := json.Unmarshal(rec.Body.Bytes(), out); err != nil {
		t.Fatalf("decode body: %v; body=%s", err, rec.Body.String())
	}
}

func TestTasksCRUDFilterSortAndSoftDelete(t *testing.T) {
	h, token := newCoreTestRouter(t)

	// Create three tasks.
	create := func(title, status, priority string) dto.Task {
		rec := doJSON(t, h, http.MethodPost, "/api/tasks", dto.CreateTaskRequest{
			Title: title, Status: &status, Priority: &priority,
		}, token)
		if rec.Code != http.StatusCreated {
			t.Fatalf("create %s status = %d, want 201; body=%s", title, rec.Code, rec.Body.String())
		}
		var task dto.Task
		decodeBody(t, rec, &task)
		return task
	}
	high := create("High priority task", "todo", "high")
	low := create("Low priority task", "todo", "low")
	done := create("Done task", "done", "medium")

	// List all.
	rec := doJSON(t, h, http.MethodGet, "/api/tasks", nil, token)
	if rec.Code != http.StatusOK {
		t.Fatalf("list status = %d, want 200", rec.Code)
	}
	var all []dto.Task
	decodeBody(t, rec, &all)
	if len(all) != 3 {
		t.Fatalf("expected 3 tasks, got %d", len(all))
	}

	// Filter by priority.
	rec = doJSON(t, h, http.MethodGet, "/api/tasks?priority_filter=high", nil, token)
	var filtered []dto.Task
	decodeBody(t, rec, &filtered)
	if len(filtered) != 1 || filtered[0].Title != "High priority task" {
		t.Fatalf("priority filter mismatch: %+v", filtered)
	}

	// Filter by status.
	rec = doJSON(t, h, http.MethodGet, "/api/tasks?status_filter=done", nil, token)
	var doneTasks []dto.Task
	decodeBody(t, rec, &doneTasks)
	if len(doneTasks) != 1 || doneTasks[0].Title != "Done task" {
		t.Fatalf("status filter mismatch: %+v", doneTasks)
	}

	// Sort by title ASC.
	rec = doJSON(t, h, http.MethodGet, "/api/tasks?sort_by=title&sort_order=ASC", nil, token)
	var sorted []dto.Task
	decodeBody(t, rec, &sorted)
	if len(sorted) != 3 || sorted[0].Title != "Done task" || sorted[2].Title != "Low priority task" {
		t.Fatalf("title sort mismatch: %+v", sorted)
	}

	// Update.
	rec = doJSON(t, h, http.MethodPatch, "/api/tasks/"+itoa(high.ID), dto.UpdateTaskRequest{
		Status: strPtr("in_progress"),
	}, token)
	if rec.Code != http.StatusOK {
		t.Fatalf("update status = %d, want 200; body=%s", rec.Code, rec.Body.String())
	}
	var updated dto.Task
	decodeBody(t, rec, &updated)
	if updated.Status != "in_progress" {
		t.Fatalf("updated status = %q, want in_progress", updated.Status)
	}

	// Soft delete.
	rec = doJSON(t, h, http.MethodDelete, "/api/tasks/"+itoa(low.ID), nil, token)
	if rec.Code != http.StatusOK {
		t.Fatalf("delete status = %d, want 200", rec.Code)
	}
	rec = doJSON(t, h, http.MethodGet, "/api/tasks", nil, token)
	decodeBody(t, rec, &all)
	if len(all) != 2 {
		t.Fatalf("expected 2 tasks after soft delete, got %d", len(all))
	}

	// Get deleted task -> 404.
	rec = doJSON(t, h, http.MethodPatch, "/api/tasks/"+itoa(low.ID), dto.UpdateTaskRequest{Title: strPtr("x")}, token)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("update deleted task status = %d, want 404", rec.Code)
	}

	// Complete a non-daily task -> 400.
	rec = doJSON(t, h, http.MethodPost, "/api/tasks/"+itoa(done.ID)+"/complete", nil, token)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("complete non-daily status = %d, want 400", rec.Code)
	}
}

func TestTaskDailyCompleteUncomplete(t *testing.T) {
	h, token := newCoreTestRouter(t)

	isDaily := true
	rec := doJSON(t, h, http.MethodPost, "/api/tasks", dto.CreateTaskRequest{
		Title: "Daily habit", IsDaily: &isDaily,
	}, token)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create daily status = %d, want 201; body=%s", rec.Code, rec.Body.String())
	}
	var task dto.Task
	decodeBody(t, rec, &task)
	if !task.IsDaily {
		t.Fatal("expected is_daily=true")
	}

	rec = doJSON(t, h, http.MethodPost, "/api/tasks/"+itoa(task.ID)+"/complete", nil, token)
	if rec.Code != http.StatusOK {
		t.Fatalf("complete status = %d, want 200; body=%s", rec.Code, rec.Body.String())
	}
	rec = doJSON(t, h, http.MethodGet, "/api/tasks", nil, token)
	var all []dto.Task
	decodeBody(t, rec, &all)
	if len(all) != 1 || all[0].Status != "done" || all[0].LastCompletedDate == nil {
		t.Fatalf("daily completion not recorded: %+v", all)
	}

	rec = doJSON(t, h, http.MethodPost, "/api/tasks/"+itoa(task.ID)+"/uncomplete", nil, token)
	if rec.Code != http.StatusOK {
		t.Fatalf("uncomplete status = %d, want 200", rec.Code)
	}
	rec = doJSON(t, h, http.MethodGet, "/api/tasks", nil, token)
	decodeBody(t, rec, &all)
	if len(all) != 1 || all[0].Status != "todo" || all[0].LastCompletedDate != nil {
		t.Fatalf("daily uncompletion not recorded: %+v", all)
	}
}

func TestTaskRemindAtRoundtrip(t *testing.T) {
	h, token := newCoreTestRouter(t)

	remindAt := "2026-08-05 14:00"
	rec := doJSON(t, h, http.MethodPost, "/api/tasks", dto.CreateTaskRequest{
		Title: "Remind me", RemindAt: &remindAt,
	}, token)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create status = %d, want 201; body=%s", rec.Code, rec.Body.String())
	}
	var task dto.Task
	decodeBody(t, rec, &task)
	if task.RemindAt == nil || *task.RemindAt != "2026-08-05 14:00" {
		t.Fatalf("remind_at roundtrip mismatch: %+v", task.RemindAt)
	}

	// Marking done clears the one-shot reminder.
	rec = doJSON(t, h, http.MethodPatch, "/api/tasks/"+itoa(task.ID), dto.UpdateTaskRequest{
		Status: strPtr("done"),
	}, token)
	if rec.Code != http.StatusOK {
		t.Fatalf("update status = %d, want 200", rec.Code)
	}
	var updated dto.Task
	decodeBody(t, rec, &updated)
	if updated.RemindAt != nil {
		t.Fatalf("remind_at should be cleared on completion, got %+v", updated.RemindAt)
	}
}

func TestCoursesCRUDFilterAndSoftDelete(t *testing.T) {
	h, token := newCoreTestRouter(t)

	create := func(name string, day int32, semester string) dto.Course {
		rec := doJSON(t, h, http.MethodPost, "/api/courses", dto.CreateCourseRequest{
			Name: name, DayOfWeek: day, StartTime: "08:00", EndTime: "09:30",
			WeekPattern: strPtr("1-16"), SemesterStartDate: strPtr("2026-02-24"),
			Semester: &semester,
		}, token)
		if rec.Code != http.StatusCreated {
			t.Fatalf("create %s status = %d, want 201; body=%s", name, rec.Code, rec.Body.String())
		}
		var course dto.Course
		decodeBody(t, rec, &course)
		return course
	}
	a := create("Course A", 1, "2024S1")
	create("Course B", 2, "2024S2")

	rec := doJSON(t, h, http.MethodGet, "/api/courses?semester=2024S1", nil, token)
	if rec.Code != http.StatusOK {
		t.Fatalf("list status = %d, want 200", rec.Code)
	}
	var s1 []dto.Course
	decodeBody(t, rec, &s1)
	if len(s1) != 1 || s1[0].Name != "Course A" {
		t.Fatalf("semester filter mismatch: %+v", s1)
	}

	// Update.
	rec = doJSON(t, h, http.MethodPatch, "/api/courses/"+itoa(a.ID), dto.UpdateCourseRequest{
		Color: strPtr("#EF4444"),
	}, token)
	if rec.Code != http.StatusOK {
		t.Fatalf("update status = %d, want 200; body=%s", rec.Code, rec.Body.String())
	}
	var updated dto.Course
	decodeBody(t, rec, &updated)
	if updated.Color != "#EF4444" {
		t.Fatalf("updated color = %q, want #EF4444", updated.Color)
	}

	// Soft delete.
	rec = doJSON(t, h, http.MethodDelete, "/api/courses/"+itoa(a.ID), nil, token)
	if rec.Code != http.StatusOK {
		t.Fatalf("delete status = %d, want 200", rec.Code)
	}
	rec = doJSON(t, h, http.MethodGet, "/api/courses", nil, token)
	var all []dto.Course
	decodeBody(t, rec, &all)
	if len(all) != 1 {
		t.Fatalf("expected 1 course after soft delete, got %d", len(all))
	}
}

func TestCoursesImportTextDedup(t *testing.T) {
	h, token := newCoreTestRouter(t)

	text := "课程号\t课程\n序号\t课程名称\t任课教师\t学 分\t选课属性\t考核方式\t考试\n性质\t是否\n缓考\t上课时间、地点\t教材\t教学记录\t过程性成绩\n2043056\t3\t自动控制原理\t李红信\n3\t必修\t未确定\t正常考试\t非缓考\n1-17周全周\t星期三\t上午34节\t秦岭堂A114\n1-17周单周\t星期二\t晚9-10节\t天山堂A312\n \t查看\t查看"

	body := dto.ImportCoursesRequest{Text: text, Semester: "2026S1", SemesterStartDate: "2026-02-24"}
	rec := doJSON(t, h, http.MethodPost, "/api/courses/import-text", body, token)
	if rec.Code != http.StatusOK {
		t.Fatalf("import status = %d, want 200; body=%s", rec.Code, rec.Body.String())
	}
	var first dto.ImportTextResult
	decodeBody(t, rec, &first)
	if first.Parsed != 2 || first.Imported != 2 || first.Skipped != 0 {
		t.Fatalf("first import = %+v, want parsed=2 imported=2 skipped=0", first)
	}

	rec = doJSON(t, h, http.MethodPost, "/api/courses/import-text", body, token)
	var second dto.ImportTextResult
	decodeBody(t, rec, &second)
	if second.Parsed != 2 || second.Imported != 0 || second.Skipped != 2 {
		t.Fatalf("second import = %+v, want parsed=2 imported=0 skipped=2", second)
	}

	rec = doJSON(t, h, http.MethodGet, "/api/courses?semester=2026S1", nil, token)
	var courses []dto.Course
	decodeBody(t, rec, &courses)
	if len(courses) != 2 {
		t.Fatalf("expected 2 courses after dedup, got %d", len(courses))
	}
}

func TestCoursesResetSemesterDates(t *testing.T) {
	h, token := newCoreTestRouter(t)

	rec := doJSON(t, h, http.MethodPost, "/api/courses", dto.CreateCourseRequest{
		Name: "Math", DayOfWeek: 1, StartTime: "08:00", EndTime: "09:30",
		WeekPattern: strPtr("1-16"), SemesterStartDate: strPtr("2026-02-24"),
		Semester: strPtr("2026S1"),
	}, token)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create course status = %d, want 201", rec.Code)
	}

	// Seed a semester context so the reset also updates it.
	rec = doJSON(t, h, http.MethodPost, "/api/term-phases", dto.CreateTermPhaseRequest{
		TermLabel: "2026S1", PhaseType: "teaching", StartWeek: 1, EndWeek: 16,
		AffectsCourses: true, AffectsExamNotifications: true,
		PomodoroProfile: "default", NotificationRulesJSON: "{}", SortOrder: 0,
	}, token)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create phase status = %d, want 201; body=%s", rec.Code, rec.Body.String())
	}

	rec = doJSON(t, h, http.MethodPost, "/api/courses/reset-semester-dates", dto.ResetSemesterDatesRequest{Date: "2026-03-02"}, token)
	if rec.Code != http.StatusOK {
		t.Fatalf("reset status = %d, want 200; body=%s", rec.Code, rec.Body.String())
	}
	var resp dto.ResetSemesterDatesResponse
	decodeBody(t, rec, &resp)
	if resp.Updated != 1 {
		t.Fatalf("reset updated = %d, want 1", resp.Updated)
	}

	rec = doJSON(t, h, http.MethodGet, "/api/courses", nil, token)
	var courses []dto.Course
	decodeBody(t, rec, &courses)
	if len(courses) != 1 || courses[0].SemesterStartDate != "2026-03-02" {
		t.Fatalf("course semester_start_date not reset: %+v", courses)
	}
}

func TestExamsCRUDAndImportTextDedup(t *testing.T) {
	h, token := newCoreTestRouter(t)

	rec := doJSON(t, h, http.MethodPost, "/api/exams", dto.CreateExamRequest{
		CourseName: "Calculus Final", ExamDatetime: "2024-12-15T09:00:00Z",
		ExamEndDatetime: strPtr("2024-12-15T11:00:00Z"), Location: strPtr("Hall A"),
		Semester: strPtr("2024S1"),
	}, token)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create exam status = %d, want 201; body=%s", rec.Code, rec.Body.String())
	}
	var exam dto.Exam
	decodeBody(t, rec, &exam)
	if exam.ExamDatetime != "2024-12-15T09:00:00Z" || exam.ExamEndDatetime != "2024-12-15T11:00:00Z" {
		t.Fatalf("exam datetimes mismatch: %+v", exam)
	}

	rec = doJSON(t, h, http.MethodPatch, "/api/exams/"+itoa(exam.ID), dto.UpdateExamRequest{
		Location: strPtr("Hall B"), Notes: strPtr("Bring calculator"),
	}, token)
	if rec.Code != http.StatusOK {
		t.Fatalf("update exam status = %d, want 200; body=%s", rec.Code, rec.Body.String())
	}
	var updated dto.Exam
	decodeBody(t, rec, &updated)
	if updated.Location != "Hall B" || updated.Notes != "Bring calculator" {
		t.Fatalf("exam not updated: %+v", updated)
	}

	// Import text with dedup.
	text := "课程号\t课程名称\t考试时间\t考试地点\t考试性质\n2043056\t自动控制原理\t2026-07-06 16:00--18:00\t天山堂A409\t正常考试"
	rec = doJSON(t, h, http.MethodPost, "/api/exams/import-text", dto.ImportExamsRequest{Text: text, Semester: "2026S1"}, token)
	if rec.Code != http.StatusOK {
		t.Fatalf("import exams status = %d, want 200; body=%s", rec.Code, rec.Body.String())
	}
	var first dto.ImportTextResult
	decodeBody(t, rec, &first)
	if first.Parsed != 1 || first.Imported != 1 || first.Skipped != 0 {
		t.Fatalf("first import = %+v, want parsed=1 imported=1 skipped=0", first)
	}

	rec = doJSON(t, h, http.MethodPost, "/api/exams/import-text", dto.ImportExamsRequest{Text: text, Semester: "2026S1"}, token)
	var second dto.ImportTextResult
	decodeBody(t, rec, &second)
	if second.Parsed != 1 || second.Imported != 0 || second.Skipped != 1 {
		t.Fatalf("second import = %+v, want parsed=1 imported=0 skipped=1", second)
	}

	// Soft delete.
	rec = doJSON(t, h, http.MethodDelete, "/api/exams/"+itoa(exam.ID), nil, token)
	if rec.Code != http.StatusOK {
		t.Fatalf("delete exam status = %d, want 200", rec.Code)
	}
	rec = doJSON(t, h, http.MethodGet, "/api/exams", nil, token)
	var exams []dto.Exam
	decodeBody(t, rec, &exams)
	if len(exams) != 1 {
		t.Fatalf("expected 1 exam after soft delete, got %d", len(exams))
	}
}

func TestTermPhasesCRUDAndCurrentStatus(t *testing.T) {
	h, token := newCoreTestRouter(t)

	// Seed a semester context via the term-phase flow is not exposed; use the
	// store directly through the API by creating a phase first.
	rec := doJSON(t, h, http.MethodPost, "/api/term-phases", dto.CreateTermPhaseRequest{
		TermLabel: "2026S1", PhaseType: "teaching", StartWeek: 1, EndWeek: 16,
		AffectsCourses: true, AffectsExamNotifications: true,
		PomodoroProfile: "default", NotificationRulesJSON: "{}", SortOrder: 0,
	}, token)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create phase status = %d, want 201; body=%s", rec.Code, rec.Body.String())
	}
	var phase dto.TermPhase
	decodeBody(t, rec, &phase)
	if phase.PhaseType != "teaching" || phase.NotificationRulesJSON != "{}" {
		t.Fatalf("unexpected phase: %+v", phase)
	}

	// List by term_label.
	rec = doJSON(t, h, http.MethodGet, "/api/term-phases?term_label=2026S1", nil, token)
	if rec.Code != http.StatusOK {
		t.Fatalf("list phases status = %d, want 200", rec.Code)
	}
	var phases []dto.TermPhase
	decodeBody(t, rec, &phases)
	if len(phases) != 1 {
		t.Fatalf("expected 1 phase, got %d", len(phases))
	}

	// Update.
	rec = doJSON(t, h, http.MethodPatch, "/api/term-phases/"+itoa(phase.ID), dto.UpdateTermPhaseRequest{
		TermLabel: "2026S1", PhaseType: "break", StartWeek: 1, EndWeek: 16,
		AffectsCourses: false, AffectsExamNotifications: false,
		PomodoroProfile: "relaxed", NotificationRulesJSON: "{}", SortOrder: 1,
	}, token)
	if rec.Code != http.StatusOK {
		t.Fatalf("update phase status = %d, want 200; body=%s", rec.Code, rec.Body.String())
	}
	var updated dto.TermPhase
	decodeBody(t, rec, &updated)
	if updated.PhaseType != "break" || updated.AffectsCourses {
		t.Fatalf("phase not updated: %+v", updated)
	}

	// Current status without a semester context -> unknown.
	rec = doJSON(t, h, http.MethodGet, "/api/term-phases/current-status", nil, token)
	if rec.Code != http.StatusOK {
		t.Fatalf("current status status = %d, want 200; body=%s", rec.Code, rec.Body.String())
	}
	var status struct {
		PhaseType string `json:"phase_type"`
		Inferred  bool   `json:"inferred"`
	}
	decodeBody(t, rec, &status)
	if status.PhaseType != "unknown" {
		t.Fatalf("phase_type = %q, want unknown", status.PhaseType)
	}

	// Soft delete.
	rec = doJSON(t, h, http.MethodDelete, "/api/term-phases/"+itoa(phase.ID), nil, token)
	if rec.Code != http.StatusOK {
		t.Fatalf("delete phase status = %d, want 200", rec.Code)
	}
	rec = doJSON(t, h, http.MethodGet, "/api/term-phases?term_label=2026S1", nil, token)
	decodeBody(t, rec, &phases)
	if len(phases) != 0 {
		t.Fatalf("expected 0 phases after soft delete, got %d", len(phases))
	}
}

func TestSemestersList(t *testing.T) {
	h, token := newCoreTestRouter(t)

	rec := doJSON(t, h, http.MethodGet, "/api/semesters", nil, token)
	if rec.Code != http.StatusOK {
		t.Fatalf("list semesters status = %d, want 200", rec.Code)
	}
	var contexts []dto.SemesterContext
	decodeBody(t, rec, &contexts)
	if len(contexts) != 0 {
		t.Fatalf("expected empty semester contexts, got %d", len(contexts))
	}
}

func TestCoreRoutesRequireAuth(t *testing.T) {
	h, _ := newCoreTestRouter(t)

	rec := doJSON(t, h, http.MethodGet, "/api/tasks", nil, "")
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("tasks without token status = %d, want 401", rec.Code)
	}
	rec = doJSON(t, h, http.MethodGet, "/api/courses", nil, "")
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("courses without token status = %d, want 401", rec.Code)
	}
	rec = doJSON(t, h, http.MethodGet, "/api/exams", nil, "")
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("exams without token status = %d, want 401", rec.Code)
	}
	rec = doJSON(t, h, http.MethodGet, "/api/term-phases/current-status", nil, "")
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("term-phases without token status = %d, want 401", rec.Code)
	}
}

func strPtr(s string) *string { return &s }

func itoa(v int64) string {
	return strconv.FormatInt(v, 10)
}
