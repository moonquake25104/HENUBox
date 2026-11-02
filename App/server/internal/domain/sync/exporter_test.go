package sync

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgtype"
)

func TestExportEmptyDB(t *testing.T) {
	q, _ := newTestStore(t)
	data := exportAll(t, q)
	if data.SchemaVersion != 2 {
		t.Fatalf("schema_version = %d, want 2", data.SchemaVersion)
	}
	if data.DatasetID == "" || data.DeviceID == "" {
		t.Fatalf("dataset_id/device_id should be backfilled, got %q/%q", data.DatasetID, data.DeviceID)
	}
	if len(data.Tasks) != 0 || len(data.Courses) != 0 || len(data.Exams) != 0 ||
		len(data.PomodoroSessions) != 0 || len(data.TermPhases) != 0 {
		t.Fatal("empty db should export empty entity lists")
	}
}

func TestExportImportRoundtripTasks(t *testing.T) {
	q, conn := newTestStore(t)

	data := sampleSyncData()
	data.Tasks = []Task{sampleTask(1, "task-sync-1", "2024-01-01T00:00:00Z")}

	stats, err := importAll(t, conn, q, data)
	if err != nil {
		t.Fatalf("import: %v", err)
	}
	if stats.TasksMerged != 1 || stats.Conflicts != 0 {
		t.Fatalf("stats = %+v, want tasks_merged=1 conflicts=0", stats)
	}

	exported := exportAll(t, q)
	if len(exported.Tasks) != 1 {
		t.Fatalf("exported tasks = %d, want 1", len(exported.Tasks))
	}
	if exported.Tasks[0].Title != "Test Task" || exported.Tasks[0].Priority != "high" {
		t.Fatalf("roundtrip mismatch: %+v", exported.Tasks[0])
	}
}

func TestLWWNewerRemoteWins(t *testing.T) {
	q, conn := newTestStore(t)

	old := sampleTask(1, "task-sync-1", "2024-01-01T00:00:00Z")
	old.Title = "Old Title"
	old.Priority = "medium"
	data1 := sampleSyncData()
	data1.Tasks = []Task{old}
	if _, err := importAll(t, conn, q, data1); err != nil {
		t.Fatalf("initial import: %v", err)
	}

	newer := sampleTask(99, "task-sync-1", "2024-02-01T00:00:00Z")
	newer.Title = "Newer Title"
	newer.Status = "done"
	data2 := sampleSyncData()
	data2.Tasks = []Task{newer}
	stats, err := importAll(t, conn, q, data2)
	if err != nil {
		t.Fatalf("merge import: %v", err)
	}
	if stats.TasksMerged != 1 {
		t.Fatalf("tasks_merged = %d, want 1", stats.TasksMerged)
	}

	exported := exportAll(t, q)
	if len(exported.Tasks) != 1 {
		t.Fatalf("exported tasks = %d, want 1", len(exported.Tasks))
	}
	if exported.Tasks[0].Title != "Newer Title" || exported.Tasks[0].Status != "done" || exported.Tasks[0].Priority != "high" {
		t.Fatalf("newer remote should win: %+v", exported.Tasks[0])
	}
}

func TestLWWLocalNewerKeepsLocal(t *testing.T) {
	q, conn := newTestStore(t)

	local := sampleTask(1, "task-sync-1", "2024-03-01T00:00:00Z")
	local.Title = "Local Title"
	local.Status = "in_progress"
	data1 := sampleSyncData()
	data1.Tasks = []Task{local}
	if _, err := importAll(t, conn, q, data1); err != nil {
		t.Fatalf("initial import: %v", err)
	}

	olderRemote := sampleTask(99, "task-sync-1", "2024-02-01T00:00:00Z")
	olderRemote.Title = "Older Remote"
	olderRemote.Priority = "low"
	data2 := sampleSyncData()
	data2.Tasks = []Task{olderRemote}
	stats, err := importAll(t, conn, q, data2)
	if err != nil {
		t.Fatalf("merge import: %v", err)
	}
	if stats.TasksMerged != 0 || stats.Conflicts != 1 {
		t.Fatalf("stats = %+v, want tasks_merged=0 conflicts=1", stats)
	}

	exported := exportAll(t, q)
	if exported.Tasks[0].Title != "Local Title" || exported.Tasks[0].Status != "in_progress" || exported.Tasks[0].Priority != "high" {
		t.Fatalf("local should be kept: %+v", exported.Tasks[0])
	}
}

func TestLWWTieMergesDailyFieldsFromRemote(t *testing.T) {
	q, conn := newTestStore(t)

	local := sampleTask(1, "task-sync-1", "2024-06-01T10:00:00Z")
	local.IsDaily = false
	data1 := sampleSyncData()
	data1.Tasks = []Task{local}
	if _, err := importAll(t, conn, q, data1); err != nil {
		t.Fatalf("initial import: %v", err)
	}

	remote := sampleTask(99, "task-sync-1", "2024-06-01T10:00:00Z")
	remote.IsDaily = true
	remote.ReminderTime = strPtr("09:30")
	remote.LastCompletedDate = strPtr("2026-08-01")
	data2 := sampleSyncData()
	data2.Tasks = []Task{remote}
	stats, err := importAll(t, conn, q, data2)
	if err != nil {
		t.Fatalf("merge import: %v", err)
	}
	if stats.TasksMerged != 0 {
		t.Fatalf("tasks_merged = %d, want 0 on tie", stats.TasksMerged)
	}

	exported := exportAll(t, q)
	if !exported.Tasks[0].IsDaily {
		t.Fatal("tie should merge remote is_daily=true into local")
	}
	if exported.Tasks[0].ReminderTime == nil || *exported.Tasks[0].ReminderTime != "09:30" {
		t.Fatalf("tie should merge remote reminder_time, got %v", exported.Tasks[0].ReminderTime)
	}
	if exported.Tasks[0].LastCompletedDate == nil || *exported.Tasks[0].LastCompletedDate != "2026-08-01" {
		t.Fatalf("tie should merge remote last_completed_date, got %v", exported.Tasks[0].LastCompletedDate)
	}
}

func TestLWWTieMergesDailyFieldsFromLocal(t *testing.T) {
	q, conn := newTestStore(t)

	local := sampleTask(1, "task-sync-1", "2024-06-01T10:00:00Z")
	local.IsDaily = true
	local.ReminderTime = strPtr("09:30")
	local.LastCompletedDate = strPtr("2026-08-01")
	data1 := sampleSyncData()
	data1.Tasks = []Task{local}
	if _, err := importAll(t, conn, q, data1); err != nil {
		t.Fatalf("initial import: %v", err)
	}

	remote := sampleTask(99, "task-sync-1", "2024-06-01T10:00:00Z")
	remote.IsDaily = false
	data2 := sampleSyncData()
	data2.Tasks = []Task{remote}
	if _, err := importAll(t, conn, q, data2); err != nil {
		t.Fatalf("merge import: %v", err)
	}

	exported := exportAll(t, q)
	if !exported.Tasks[0].IsDaily {
		t.Fatal("local daily marker should survive a tie")
	}
	if exported.Tasks[0].ReminderTime == nil || *exported.Tasks[0].ReminderTime != "09:30" {
		t.Fatalf("local reminder_time should survive, got %v", exported.Tasks[0].ReminderTime)
	}
	if exported.Tasks[0].LastCompletedDate == nil || *exported.Tasks[0].LastCompletedDate != "2026-08-01" {
		t.Fatalf("local last_completed_date should survive, got %v", exported.Tasks[0].LastCompletedDate)
	}
}

func TestImportCoursesAndExams(t *testing.T) {
	q, conn := newTestStore(t)

	course := sampleCourse(1, "course-sync-1", "2024-01-01T00:00:00Z")
	exam := sampleExam(1, "exam-sync-1", "2024-01-01T00:00:00Z")
	data := sampleSyncData()
	data.Courses = []Course{course}
	data.Exams = []Exam{exam}

	stats, err := importAll(t, conn, q, data)
	if err != nil {
		t.Fatalf("import: %v", err)
	}
	if stats.CoursesMerged != 1 || stats.ExamsMerged != 1 {
		t.Fatalf("stats = %+v, want courses=1 exams=1", stats)
	}

	exported := exportAll(t, q)
	if len(exported.Courses) != 1 || exported.Courses[0].Name != "Math 101" || exported.Courses[0].WeekPattern != "1-16" {
		t.Fatalf("course roundtrip mismatch: %+v", exported.Courses)
	}
	if len(exported.Exams) != 1 || exported.Exams[0].CourseName != "Math 101" || exported.Exams[0].ExamEndDatetime != "2024-06-15T12:00:00Z" {
		t.Fatalf("exam roundtrip mismatch: %+v", exported.Exams)
	}
	if exported.Exams[0].CourseID == nil || *exported.Exams[0].CourseID != exported.Courses[0].ID {
		t.Fatalf("exam course_id should map to local course id, got %v want %d", exported.Exams[0].CourseID, exported.Courses[0].ID)
	}
}

func TestCourseEditMergesBySyncIDNotSQLiteID(t *testing.T) {
	q, conn := newTestStore(t)

	local := sampleCourse(1, "course-sync-1", "2024-01-01T00:00:00Z")
	local.StartTime = "08:00"
	initial := sampleSyncData()
	initial.Courses = []Course{local}
	if _, err := importAll(t, conn, q, initial); err != nil {
		t.Fatalf("initial import: %v", err)
	}

	remote := sampleCourse(42, "course-sync-1", "2024-02-01T00:00:00Z")
	remote.StartTime = "10:00"
	incoming := sampleSyncData()
	incoming.Courses = []Course{remote}
	stats, err := importAll(t, conn, q, incoming)
	if err != nil {
		t.Fatalf("merge import: %v", err)
	}
	if stats.CoursesMerged != 1 {
		t.Fatalf("courses_merged = %d, want 1", stats.CoursesMerged)
	}

	exported := exportAll(t, q)
	if len(exported.Courses) != 1 || exported.Courses[0].SyncID != "course-sync-1" || exported.Courses[0].StartTime != "10:00" {
		t.Fatalf("merge should match by sync_id: %+v", exported.Courses)
	}
}

func TestNewerTombstoneHidesCourseFromActiveQueries(t *testing.T) {
	q, conn := newTestStore(t)

	course := sampleCourse(1, "course-sync-1", "2024-01-01T00:00:00Z")
	initial := sampleSyncData()
	initial.Courses = []Course{course}
	if _, err := importAll(t, conn, q, initial); err != nil {
		t.Fatalf("initial import: %v", err)
	}

	tombstone := sampleCourse(99, "course-sync-1", "2024-01-01T00:00:00Z")
	tombstone.DeletedAt = strPtr("2024-03-01T00:00:00Z")
	incoming := sampleSyncData()
	incoming.Courses = []Course{tombstone}
	stats, err := importAll(t, conn, q, incoming)
	if err != nil {
		t.Fatalf("tombstone import: %v", err)
	}
	if stats.CoursesMerged != 1 {
		t.Fatalf("courses_merged = %d, want 1", stats.CoursesMerged)
	}

	active, err := q.ListCourses(context.Background(), pgtype.Text{})
	if err != nil {
		t.Fatalf("list active courses: %v", err)
	}
	if len(active) != 0 {
		t.Fatalf("active courses = %d, want 0 after tombstone", len(active))
	}

	exported := exportAll(t, q)
	if len(exported.Courses) != 1 || exported.Courses[0].DeletedAt == nil || *exported.Courses[0].DeletedAt != "2024-03-01T00:00:00Z" {
		t.Fatalf("tombstone should propagate: %+v", exported.Courses)
	}
}

func TestV1RemoteWithoutSyncIDImportsOnce(t *testing.T) {
	q, conn := newTestStore(t)

	v1Course := sampleCourse(7, "", "2024-01-01T00:00:00Z")
	v1Course.Name = "Legacy Course"
	v1 := sampleSyncData()
	v1.SchemaVersion = 1
	v1.Courses = []Course{v1Course}

	if _, err := importAll(t, conn, q, v1); err != nil {
		t.Fatalf("first v1 import: %v", err)
	}
	if _, err := importAll(t, conn, q, v1); err != nil {
		t.Fatalf("second v1 import: %v", err)
	}

	exported := exportAll(t, q)
	if len(exported.Courses) != 1 {
		t.Fatalf("exported courses = %d, want 1", len(exported.Courses))
	}
	if exported.Courses[0].SyncID != "legacy-course-7" || exported.Courses[0].Name != "Legacy Course" {
		t.Fatalf("v1 course should get legacy sync_id: %+v", exported.Courses[0])
	}
}

func TestV1RemoteMatchesExistingLocalRowByIDOnce(t *testing.T) {
	q, conn := newTestStore(t)
	ctx := context.Background()

	if _, err := conn.Exec(ctx, `INSERT INTO courses (
		id, sync_id, name, day_of_week, start_time, end_time, week_pattern,
		semester_start_date, location, teacher, color, semester, created_at, updated_at
	) VALUES (7, 'local-random-sync-id', 'Existing Course', 1, '08:00', '09:30', '1-16',
		'2026-02-24', 'Room 101', 'Prof. Smith', '#3B82F6', '2024S1',
		'2024-01-01T00:00:00Z', '2024-01-01T00:00:00Z')`); err != nil {
		t.Fatalf("seed local course: %v", err)
	}

	v1Course := sampleCourse(7, "", "2024-02-01T00:00:00Z")
	v1Course.Name = "Legacy Remote Update"
	v1 := sampleSyncData()
	v1.SchemaVersion = 1
	v1.Courses = []Course{v1Course}

	stats, err := importAll(t, conn, q, v1)
	if err != nil {
		t.Fatalf("v1 import: %v", err)
	}
	if stats.CoursesMerged != 1 {
		t.Fatalf("courses_merged = %d, want 1", stats.CoursesMerged)
	}

	exported := exportAll(t, q)
	if len(exported.Courses) != 1 || exported.Courses[0].ID != 7 || exported.Courses[0].SyncID != "legacy-course-7" || exported.Courses[0].Name != "Legacy Remote Update" {
		t.Fatalf("v1 remote should match local by id: %+v", exported.Courses)
	}
}

func TestExportExcludesAITables(t *testing.T) {
	q, conn := newTestStore(t)
	ctx := context.Background()

	if _, err := conn.Exec(ctx, `INSERT INTO ai_config
		(id, enabled, base_url, model, api_key_encrypted, sync_enabled, created_at, updated_at)
		VALUES (1, true, 'https://api.deepseek.com', 'deepseek-v4-flash', 'cipher', false,
		'2026-07-31T00:00:00Z', '2026-07-31T00:00:00Z')
		ON CONFLICT (id) DO NOTHING`); err != nil {
		t.Fatalf("seed ai_config: %v", err)
	}
	if _, err := conn.Exec(ctx, `INSERT INTO ai_morning_brief
		(date, markdown, source, model, generated_at, created_at, updated_at)
		VALUES ('2026-07-31', 'md', 'ai', 'deepseek-v4-flash', '2026-07-31T07:00:00Z',
		'2026-07-31T00:00:00Z', '2026-07-31T00:00:00Z')`); err != nil {
		t.Fatalf("seed ai_morning_brief: %v", err)
	}

	exported := exportAll(t, q)
	jsonBytes, err := json.Marshal(exported)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if strings.Contains(string(jsonBytes), "ai_config") {
		t.Fatal("exported JSON should not contain ai_config")
	}
	if strings.Contains(string(jsonBytes), "ai_morning_brief") {
		t.Fatal("exported JSON should not contain ai_morning_brief")
	}
}

func TestRemindAtSyncRoundtripAndClear(t *testing.T) {
	q, conn := newTestStore(t)

	task := sampleTask(1, "task-sync-1", "2024-01-01T00:00:00Z")
	task.RemindAt = strPtr("2026-08-05 14:00")
	data1 := sampleSyncData()
	data1.Tasks = []Task{task}
	if _, err := importAll(t, conn, q, data1); err != nil {
		t.Fatalf("import with remind_at: %v", err)
	}

	exported := exportAll(t, q)
	if exported.Tasks[0].RemindAt == nil || *exported.Tasks[0].RemindAt != "2026-08-05 14:00" {
		t.Fatalf("remind_at roundtrip mismatch: %v", exported.Tasks[0].RemindAt)
	}

	newer := sampleTask(99, "task-sync-1", "2024-02-01T00:00:00Z")
	newer.RemindAt = nil
	data2 := sampleSyncData()
	data2.Tasks = []Task{newer}
	stats, err := importAll(t, conn, q, data2)
	if err != nil {
		t.Fatalf("merge clear: %v", err)
	}
	if stats.TasksMerged != 1 {
		t.Fatalf("tasks_merged = %d, want 1", stats.TasksMerged)
	}
	exported = exportAll(t, q)
	if exported.Tasks[0].RemindAt != nil {
		t.Fatalf("remote clearing remind_at should overwrite local, got %v", exported.Tasks[0].RemindAt)
	}
}

func TestLWWTieDoesNotReviveRemindAt(t *testing.T) {
	q, conn := newTestStore(t)

	local := sampleTask(1, "task-sync-1", "2024-06-01T10:00:00Z")
	local.RemindAt = nil
	data1 := sampleSyncData()
	data1.Tasks = []Task{local}
	if _, err := importAll(t, conn, q, data1); err != nil {
		t.Fatalf("initial import: %v", err)
	}

	remote := sampleTask(99, "task-sync-1", "2024-06-01T10:00:00Z")
	remote.RemindAt = strPtr("2026-08-05 14:00")
	data2 := sampleSyncData()
	data2.Tasks = []Task{remote}
	stats, err := importAll(t, conn, q, data2)
	if err != nil {
		t.Fatalf("merge import: %v", err)
	}
	if stats.TasksMerged != 0 {
		t.Fatalf("tasks_merged = %d, want 0 on tie", stats.TasksMerged)
	}

	exported := exportAll(t, q)
	if exported.Tasks[0].RemindAt != nil {
		t.Fatalf("tie should not revive a cleared remind_at, got %v", exported.Tasks[0].RemindAt)
	}
}
