package dto

import (
	"github.com/jackc/pgx/v5/pgtype"

	"kairos/server/internal/store"
)

// Task mirrors the frontend src/types/task.ts Task entity.
type Task struct {
	ID                int64   `json:"id"`
	SyncID            string  `json:"sync_id"`
	Title             string  `json:"title"`
	Description       string  `json:"description"`
	Status            string  `json:"status"`
	Priority          string  `json:"priority"`
	DueDate           *string `json:"due_date"`
	Tags              string  `json:"tags"`
	CreatedAt         string  `json:"created_at"`
	UpdatedAt         string  `json:"updated_at"`
	IsDaily           bool    `json:"is_daily"`
	LastCompletedDate *string `json:"last_completed_date"`
	ReminderTime      *string `json:"reminder_time"`
	RemindAt          *string `json:"remind_at"`
	DeletedAt         *string `json:"deleted_at"`
}

// CreateTaskRequest is the body of POST /api/tasks.
type CreateTaskRequest struct {
	Title        string  `json:"title"`
	Description  *string `json:"description"`
	Status       *string `json:"status"`
	Priority     *string `json:"priority"`
	DueDate      *string `json:"due_date"`
	Tags         *string `json:"tags"`
	IsDaily      *bool   `json:"is_daily"`
	ReminderTime *string `json:"reminder_time"`
	RemindAt     *string `json:"remind_at"`
}

// UpdateTaskRequest is the body of PATCH /api/tasks/{id}; absent fields keep
// the current value (matching the Rust update_task command semantics).
type UpdateTaskRequest struct {
	Title        *string `json:"title"`
	Description  *string `json:"description"`
	Status       *string `json:"status"`
	Priority     *string `json:"priority"`
	DueDate      *string `json:"due_date"`
	Tags         *string `json:"tags"`
	IsDaily      *bool   `json:"is_daily"`
	ReminderTime *string `json:"reminder_time"`
	RemindAt     *string `json:"remind_at"`
}

// FromTask converts a store task into the API representation.
func FromTask(t store.Task) Task {
	return Task{
		ID:                t.ID,
		SyncID:            store.TextString(t.SyncID),
		Title:             t.Title,
		Description:       t.Description,
		Status:            t.Status,
		Priority:          t.Priority,
		DueDate:           datePtr(t.DueDate),
		Tags:              string(t.Tags),
		CreatedAt:         store.TSString(t.CreatedAt),
		UpdatedAt:         store.TSString(t.UpdatedAt),
		IsDaily:           t.IsDaily,
		LastCompletedDate: datePtr(t.LastCompletedDate),
		ReminderTime:      clockPtr(t.ReminderTime),
		RemindAt:          remindAtPtr(t.RemindAt),
		DeletedAt:         tsPtr(t.DeletedAt),
	}
}

func datePtr(d pgtype.Date) *string {
	if !d.Valid {
		return nil
	}
	return StrPtr(store.DateString(d))
}

func clockPtr(t pgtype.Time) *string {
	if !t.Valid {
		return nil
	}
	return StrPtr(store.ClockString(t))
}

func remindAtPtr(t pgtype.Timestamptz) *string {
	if !t.Valid {
		return nil
	}
	return StrPtr(RemindAtString(t))
}

func tsPtr(t pgtype.Timestamptz) *string {
	if !t.Valid {
		return nil
	}
	return StrPtr(store.TSString(t))
}
