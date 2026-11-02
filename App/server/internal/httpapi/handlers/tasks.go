package handlers

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	"kairos/server/internal/httpapi/dto"
	"kairos/server/internal/store"
)

// Tasks implements the /api/tasks endpoints.
type Tasks struct {
	Q         *store.Queries
	Log       *slog.Logger
	Scheduler NotifyRecomputer
}

// List handles GET /api/tasks?status_filter=&priority_filter=&sort_by=&sort_order=.
func (h *Tasks) List(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	statusFilter := pgtype.Text{}
	if v := q.Get("status_filter"); v != "" {
		statusFilter = store.TextOf(v)
	}
	priorityFilter := pgtype.Text{}
	if v := q.Get("priority_filter"); v != "" {
		priorityFilter = store.TextOf(v)
	}
	sortBy := q.Get("sort_by")
	if sortBy == "" {
		sortBy = "created_at"
	}
	sortOrder := q.Get("sort_order")
	if sortOrder == "" {
		sortOrder = "DESC"
	}

	tasks, err := h.Q.ListTasksWithSort(r.Context(), store.ListTasksParams{
		StatusFilter:   statusFilter,
		PriorityFilter: priorityFilter,
	}, sortBy, sortOrder)
	if err != nil {
		h.Log.Error("list tasks", "error", err)
		writeError(w, http.StatusInternalServerError, "internal server error")
		return
	}
	out := make([]dto.Task, 0, len(tasks))
	for _, t := range tasks {
		out = append(out, dto.FromTask(t))
	}
	writeJSON(w, http.StatusOK, out)
}

// Create handles POST /api/tasks.
func (h *Tasks) Create(w http.ResponseWriter, r *http.Request) {
	var req dto.CreateTaskRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	if req.Title == "" {
		writeError(w, http.StatusBadRequest, "title is required")
		return
	}

	status := "todo"
	if req.Status != nil {
		status = *req.Status
	}
	if !validTaskStatus(status) {
		writeError(w, http.StatusBadRequest, "invalid status")
		return
	}
	priority := "medium"
	if req.Priority != nil {
		priority = *req.Priority
	}
	if !validTaskPriority(priority) {
		writeError(w, http.StatusBadRequest, "invalid priority")
		return
	}
	description := ""
	if req.Description != nil {
		description = *req.Description
	}
	tags := []byte("[]")
	if req.Tags != nil {
		if !json.Valid([]byte(*req.Tags)) {
			writeError(w, http.StatusBadRequest, "tags must be a JSON array")
			return
		}
		tags = []byte(*req.Tags)
	}
	isDaily := false
	if req.IsDaily != nil {
		isDaily = *req.IsDaily
	}

	dueDate, err := dto.ParseDate(req.DueDate)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	reminderTime, err := dto.ParseClock(req.ReminderTime)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	remindAt, err := dto.ParseRemindAt(req.RemindAt)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	task, err := h.Q.CreateTask(r.Context(), store.CreateTaskParams{
		SyncID:            "",
		Title:             req.Title,
		Description:       description,
		Status:            status,
		Priority:          priority,
		DueDate:           dueDate,
		Tags:              tags,
		IsDaily:           isDaily,
		LastCompletedDate: pgtype.Date{},
		ReminderTime:      reminderTime,
		RemindAt:          remindAt,
	})
	if err != nil {
		h.Log.Error("create task", "error", err)
		writeError(w, http.StatusInternalServerError, "internal server error")
		return
	}
	if h.Scheduler != nil {
		h.Scheduler.RecomputeTasks(r.Context())
	}
	writeJSON(w, http.StatusCreated, dto.FromTask(task))
}

// Update handles PATCH /api/tasks/{id}.
func (h *Tasks) Update(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	var req dto.UpdateTaskRequest
	if !decodeJSON(w, r, &req) {
		return
	}

	existing, err := h.Q.GetTask(r.Context(), id)
	if err != nil {
		writeStoreError(w, err)
		return
	}

	status := existing.Status
	if req.Status != nil {
		status = *req.Status
	}
	if !validTaskStatus(status) {
		writeError(w, http.StatusBadRequest, "invalid status")
		return
	}
	priority := existing.Priority
	if req.Priority != nil {
		priority = *req.Priority
	}
	if !validTaskPriority(priority) {
		writeError(w, http.StatusBadRequest, "invalid priority")
		return
	}

	title := existing.Title
	if req.Title != nil {
		title = *req.Title
	}
	if title == "" {
		writeError(w, http.StatusBadRequest, "title is required")
		return
	}
	description := existing.Description
	if req.Description != nil {
		description = *req.Description
	}
	tags := existing.Tags
	if req.Tags != nil {
		if !json.Valid([]byte(*req.Tags)) {
			writeError(w, http.StatusBadRequest, "tags must be a JSON array")
			return
		}
		tags = []byte(*req.Tags)
	}
	isDaily := existing.IsDaily
	if req.IsDaily != nil {
		isDaily = *req.IsDaily
	}

	dueDate := existing.DueDate
	if req.DueDate != nil {
		parsed, err := dto.ParseDate(req.DueDate)
		if err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		dueDate = parsed
	}
	reminderTime := existing.ReminderTime
	if req.ReminderTime != nil {
		parsed, err := dto.ParseClock(req.ReminderTime)
		if err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		reminderTime = parsed
	}
	remindAt := existing.RemindAt
	if req.RemindAt != nil {
		parsed, err := dto.ParseRemindAt(req.RemindAt)
		if err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		remindAt = parsed
	}

	// A one-shot task being marked done drops its one-shot reminder.
	if existing.Status != "done" && status == "done" {
		remindAt = pgtype.Timestamptz{}
	}

	updated, err := h.Q.UpdateTask(r.Context(), store.UpdateTaskParams{
		Title:             title,
		Description:       description,
		Status:            status,
		Priority:          priority,
		DueDate:           dueDate,
		Tags:              tags,
		IsDaily:           isDaily,
		LastCompletedDate: existing.LastCompletedDate,
		ReminderTime:      reminderTime,
		RemindAt:          remindAt,
		ID:                id,
	})
	if err != nil {
		writeStoreError(w, err)
		return
	}
	if h.Scheduler != nil {
		h.Scheduler.RecomputeTasks(r.Context())
	}
	writeJSON(w, http.StatusOK, dto.FromTask(updated))
}

// Delete handles DELETE /api/tasks/{id} (soft delete).
func (h *Tasks) Delete(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	if err := h.Q.SoftDeleteTask(r.Context(), id); err != nil {
		writeStoreError(w, err)
		return
	}
	if h.Scheduler != nil {
		h.Scheduler.RecomputeTasks(r.Context())
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

// Complete handles POST /api/tasks/{id}/complete for daily tasks.
func (h *Tasks) Complete(w http.ResponseWriter, r *http.Request) {
	h.setDailyCompleted(w, r, true)
}

// Uncomplete handles POST /api/tasks/{id}/uncomplete for daily tasks.
func (h *Tasks) Uncomplete(w http.ResponseWriter, r *http.Request) {
	h.setDailyCompleted(w, r, false)
}

func (h *Tasks) setDailyCompleted(w http.ResponseWriter, r *http.Request, completed bool) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	existing, err := h.Q.GetTask(r.Context(), id)
	if err != nil {
		writeStoreError(w, err)
		return
	}
	if !existing.IsDaily {
		writeError(w, http.StatusBadRequest, "该任务不是每日任务")
		return
	}
	completedDate := pgtype.Date{}
	if completed {
		completedDate = store.DateOf(todayChina())
	}
	if err := h.Q.SetTaskDailyCompleted(r.Context(), store.SetTaskDailyCompletedParams{
		Completed:     completed,
		CompletedDate: completedDate,
		ID:            id,
	}); err != nil {
		h.Log.Error("set daily completed", "error", err)
		writeError(w, http.StatusInternalServerError, "internal server error")
		return
	}
	if h.Scheduler != nil {
		h.Scheduler.RecomputeTasks(r.Context())
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func validTaskStatus(s string) bool {
	return s == "todo" || s == "in_progress" || s == "done"
}

func validTaskPriority(p string) bool {
	return p == "high" || p == "medium" || p == "low"
}

// todayChina returns the current date in +08:00 as YYYY-MM-DD.
func todayChina() string {
	return time.Now().In(time.FixedZone("CST", 8*3600)).Format("2006-01-02")
}
