package store

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
)

func mustCreateTask(t *testing.T, q *Queries, title, status, priority string) Task {
	t.Helper()
	return mustCreateTaskFull(t, q, CreateTaskParams{
		SyncID:      "",
		Title:       title,
		Description: "A test task",
		Status:      status,
		Priority:    priority,
		Tags:        []byte("[]"),
	})
}

func mustCreateTaskFull(t *testing.T, q *Queries, arg CreateTaskParams) Task {
	t.Helper()
	task, err := q.CreateTask(context.Background(), arg)
	if err != nil {
		t.Fatalf("create task: %v", err)
	}
	return task
}

func TestTaskCreateAndGet(t *testing.T) {
	q, _ := newTestStore(t)

	task := mustCreateTask(t, q, "Test Task", "todo", "medium")
	if task.ID <= 0 {
		t.Fatalf("expected positive id, got %d", task.ID)
	}
	got, err := q.GetTask(context.Background(), task.ID)
	if err != nil {
		t.Fatalf("get task: %v", err)
	}
	if got.Title != "Test Task" || got.Status != "todo" || got.Priority != "medium" {
		t.Fatalf("unexpected task: %+v", got)
	}
	if !got.SyncID.Valid || got.SyncID.String == "" {
		t.Fatalf("expected backfilled sync_id, got %+v", got.SyncID)
	}
	if !got.CreatedAt.Valid || !got.UpdatedAt.Valid {
		t.Fatalf("expected created/updated timestamps, got %+v", got)
	}
}

func TestTaskGetMissingReturnsNoRows(t *testing.T) {
	q, _ := newTestStore(t)

	if _, err := q.GetTask(context.Background(), 999); err == nil {
		t.Fatal("expected error for missing task")
	}
}

func TestTaskUpdateRoundtrip(t *testing.T) {
	q, _ := newTestStore(t)

	task := mustCreateTask(t, q, "original", "todo", "medium")
	updated, err := q.UpdateTask(context.Background(), UpdateTaskParams{
		Title:        "Updated Title",
		Description:  "Updated description",
		Status:       "in_progress",
		Priority:     "high",
		DueDate:      dateOf("2024-12-31"),
		Tags:         []byte(`["urgent"]`),
		IsDaily:      false,
		RemindAt:     pgtype.Timestamptz{},
		ReminderTime: pgtype.Time{},
		ID:           task.ID,
	})
	if err != nil {
		t.Fatalf("update task: %v", err)
	}
	if updated.Title != "Updated Title" || updated.Status != "in_progress" || updated.Priority != "high" {
		t.Fatalf("unexpected updated task: %+v", updated)
	}
	got, err := q.GetTask(context.Background(), task.ID)
	if err != nil {
		t.Fatalf("get updated task: %v", err)
	}
	if got.Title != "Updated Title" || got.Status != "in_progress" || got.Priority != "high" {
		t.Fatalf("get returned stale task: %+v", got)
	}
	if !sameDate(got.DueDate, "2024-12-31") {
		t.Fatalf("due date not persisted: %+v", got.DueDate)
	}
}

func TestTaskRemindAtRoundtrip(t *testing.T) {
	q, _ := newTestStore(t)

	task := mustCreateTaskFull(t, q, CreateTaskParams{
		SyncID:      "",
		Title:       "Remind me",
		Description: "",
		Status:      "todo",
		Priority:    "medium",
		DueDate:     pgtype.Date{},
		Tags:        []byte("[]"),
		RemindAt:    tsOf("2026-08-05T06:00:00Z"),
	})
	if !sameTS(task.RemindAt, "2026-08-05T06:00:00Z") {
		t.Fatalf("remind_at not persisted: %+v", task.RemindAt)
	}

	cleared, err := q.UpdateTask(context.Background(), UpdateTaskParams{
		Title:        "Remind me",
		Description:  "",
		Status:       "todo",
		Priority:     "medium",
		Tags:         []byte("[]"),
		IsDaily:      false,
		RemindAt:     pgtype.Timestamptz{},
		ReminderTime: pgtype.Time{},
		ID:           task.ID,
	})
	if err != nil {
		t.Fatalf("clear remind_at: %v", err)
	}
	if cleared.RemindAt.Valid {
		t.Fatalf("expected remind_at cleared, got %+v", cleared.RemindAt)
	}
}

func TestTaskSoftDelete(t *testing.T) {
	q, _ := newTestStore(t)

	keeper := mustCreateTask(t, q, "Keeper", "todo", "medium")
	gone := mustCreateTask(t, q, "Doomed", "todo", "medium")

	if err := q.SoftDeleteTask(context.Background(), gone.ID); err != nil {
		t.Fatalf("soft delete task: %v", err)
	}

	if _, err := q.GetTask(context.Background(), gone.ID); err == nil {
		t.Fatal("deleted task should not be fetchable")
	}

	all, err := q.ListTasks(context.Background(), ListTasksParams{})
	if err != nil {
		t.Fatalf("list tasks: %v", err)
	}
	if len(all) != 1 || all[0].ID != keeper.ID {
		t.Fatalf("deleted task still listed: %+v", all)
	}

	syncAll, err := q.ListAllTasksForSync(context.Background())
	if err != nil {
		t.Fatalf("list all tasks for sync: %v", err)
	}
	if len(syncAll) != 2 {
		t.Fatalf("sync export should include tombstones, got %d", len(syncAll))
	}
	for _, task := range syncAll {
		if task.ID == gone.ID && !task.DeletedAt.Valid {
			t.Fatalf("expected deleted_at tombstone on soft-deleted task: %+v", task)
		}
	}
}

func TestTaskFiltersAndSortWhitelist(t *testing.T) {
	q, _ := newTestStore(t)

	mustCreateTaskFull(t, q, CreateTaskParams{
		SyncID: "", Title: "High priority task", Description: "", Status: "todo",
		Priority: "high", Tags: []byte("[]"),
	})
	mustCreateTaskFull(t, q, CreateTaskParams{
		SyncID: "", Title: "Low priority task", Description: "", Status: "todo",
		Priority: "low", Tags: []byte("[]"),
	})
	mustCreateTaskFull(t, q, CreateTaskParams{
		SyncID: "", Title: "Done task", Description: "", Status: "done",
		Priority: "medium", Tags: []byte("[]"),
	})

	all, err := q.ListTasks(context.Background(), ListTasksParams{})
	if err != nil {
		t.Fatalf("list all: %v", err)
	}
	if len(all) != 3 {
		t.Fatalf("expected 3 tasks, got %d", len(all))
	}

	high, err := q.ListTasks(context.Background(), ListTasksParams{PriorityFilter: textOf("high")})
	if err != nil {
		t.Fatalf("filter by priority: %v", err)
	}
	if len(high) != 1 || high[0].Title != "High priority task" {
		t.Fatalf("priority filter mismatch: %+v", high)
	}

	done, err := q.ListTasks(context.Background(), ListTasksParams{StatusFilter: textOf("done")})
	if err != nil {
		t.Fatalf("filter by status: %v", err)
	}
	if len(done) != 1 || done[0].Title != "Done task" {
		t.Fatalf("status filter mismatch: %+v", done)
	}

	none, err := q.ListTasks(context.Background(), ListTasksParams{StatusFilter: textOf("in_progress")})
	if err != nil {
		t.Fatalf("filter no match: %v", err)
	}
	if len(none) != 0 {
		t.Fatalf("expected empty result, got %+v", none)
	}
}

func TestTaskSortWhitelist(t *testing.T) {
	q, _ := newTestStore(t)

	mustCreateTask(t, q, "A", "todo", "low")
	mustCreateTask(t, q, "Z", "todo", "high")

	asc, err := q.ListTasksWithSort(context.Background(), ListTasksParams{}, "title", "ASC")
	if err != nil {
		t.Fatalf("sort title asc: %v", err)
	}
	if len(asc) != 2 || asc[0].Title != "A" || asc[1].Title != "Z" {
		t.Fatalf("title asc sort mismatch: %+v", asc)
	}

	desc, err := q.ListTasksWithSort(context.Background(), ListTasksParams{}, "title", "DESC")
	if err != nil {
		t.Fatalf("sort title desc: %v", err)
	}
	if desc[0].Title != "Z" || desc[1].Title != "A" {
		t.Fatalf("title desc sort mismatch: %+v", desc)
	}

	// Invalid sort column falls back to created_at DESC (newest first = Z).
	fallback, err := q.ListTasksWithSort(context.Background(), ListTasksParams{}, "malicious;drop", "upside")
	if err != nil {
		t.Fatalf("sort fallback: %v", err)
	}
	if len(fallback) != 2 || fallback[0].Title != "Z" {
		t.Fatalf("expected created_at DESC fallback, got %+v", fallback)
	}
}

func TestTaskSyncIDBackfillAndPartialUnique(t *testing.T) {
	q, _ := newTestStore(t)

	a := mustCreateTask(t, q, "A", "todo", "medium")
	b := mustCreateTask(t, q, "B", "todo", "medium")

	if !a.SyncID.Valid || !b.SyncID.Valid {
		t.Fatalf("sync_id should be backfilled on insert: %+v %+v", a.SyncID, b.SyncID)
	}
	if a.SyncID.String == b.SyncID.String {
		t.Fatalf("distinct inserts must get distinct sync_ids, both %q", a.SyncID.String)
	}

	if _, err := q.CreateTask(context.Background(), CreateTaskParams{
		SyncID: "fixed-uuid", Title: "dup1", Description: "", Status: "todo",
		Priority: "medium", Tags: []byte("[]"),
	}); err != nil {
		t.Fatalf("create with explicit sync_id: %v", err)
	}
	if _, err := q.CreateTask(context.Background(), CreateTaskParams{
		SyncID: "fixed-uuid", Title: "dup2", Description: "", Status: "todo",
		Priority: "medium", Tags: []byte("[]"),
	}); err == nil {
		t.Fatal("duplicate non-empty sync_id should violate the partial unique index")
	} else if !strings.Contains(err.Error(), "duplicate key") {
		t.Fatalf("unexpected error for duplicate sync_id: %v", err)
	}
}

func TestTaskDailyCompletedAndUncompleted(t *testing.T) {
	q, _ := newTestStore(t)

	daily := mustCreateTaskFull(t, q, CreateTaskParams{
		SyncID: "", Title: "Daily habit", Description: "", Status: "todo",
		Priority: "medium", IsDaily: true, Tags: []byte("[]"),
	})

	if err := q.SetTaskDailyCompleted(context.Background(), SetTaskDailyCompletedParams{
		Completed: true, CompletedDate: dateOf("2026-06-01"), ID: daily.ID,
	}); err != nil {
		t.Fatalf("complete daily: %v", err)
	}
	got, err := q.GetTask(context.Background(), daily.ID)
	if err != nil {
		t.Fatalf("get completed daily: %v", err)
	}
	if got.Status != "done" || !sameDate(got.LastCompletedDate, "2026-06-01") {
		t.Fatalf("daily completion not recorded: %+v", got)
	}

	if err := q.SetTaskDailyCompleted(context.Background(), SetTaskDailyCompletedParams{
		Completed: false, ID: daily.ID,
	}); err != nil {
		t.Fatalf("uncomplete daily: %v", err)
	}
	got, err = q.GetTask(context.Background(), daily.ID)
	if err != nil {
		t.Fatalf("get uncompleted daily: %v", err)
	}
	if got.Status != "todo" || got.LastCompletedDate.Valid {
		t.Fatalf("daily uncompletion not recorded: %+v", got)
	}

	oneShot := mustCreateTask(t, q, "One-shot", "todo", "medium")
	if err := q.SetTaskDailyCompleted(context.Background(), SetTaskDailyCompletedParams{
		Completed: true, CompletedDate: dateOf("2026-06-01"), ID: oneShot.ID,
	}); err != nil {
		t.Fatalf("complete non-daily task: %v", err)
	}
	got, err = q.GetTask(context.Background(), oneShot.ID)
	if err != nil {
		t.Fatalf("get one-shot task: %v", err)
	}
	if got.Status != "todo" || got.LastCompletedDate.Valid {
		t.Fatalf("non-daily task must not be touched: %+v", got)
	}
}

func TestListTasksReturnsEmptySlice(t *testing.T) {
	q, _ := newTestStore(t)

	all, err := q.ListTasks(context.Background(), ListTasksParams{})
	if err != nil {
		t.Fatalf("list empty: %v", err)
	}
	if all == nil || len(all) != 0 {
		t.Fatalf("expected empty non-nil slice, got %#v", all)
	}
}

func sameDate(d pgtype.Date, want string) bool {
	return d.Valid && d.Time.Format("2006-01-02") == want
}

func sameTS(ts pgtype.Timestamptz, want string) bool {
	return ts.Valid && ts.Time.UTC().Format(time.RFC3339) == want
}
