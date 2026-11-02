package store

import (
	"context"
	"strings"
)

// taskSortColumns is the whitelist of ORDER BY columns accepted by
// ListTasksWithSort; anything else falls back to created_at.
var taskSortColumns = map[string]string{
	"title":      "title",
	"status":     "status",
	"priority":   "priority",
	"due_date":   "due_date",
	"created_at": "created_at",
	"updated_at": "updated_at",
}

const listTasksBaseSQL = `SELECT id, sync_id, title, description, status, priority, due_date, tags, remind_at, reminder_time, is_daily, last_completed_date, deleted_at, created_at, updated_at
FROM tasks
WHERE deleted_at IS NULL
  AND ($1::text IS NULL OR status = $1)
  AND ($2::text IS NULL OR priority = $2)`

// ListTasksWithSort lists non-deleted tasks, optionally filtered by status
// and priority, ordered by a whitelisted column. sortBy is validated against
// taskSortColumns (fallback: created_at) and sortOrder is coerced to ASC or
// DESC (fallback: DESC), mirroring the Rust get_all_tasks semantics.
func (q *Queries) ListTasksWithSort(ctx context.Context, arg ListTasksParams, sortBy, sortOrder string) ([]Task, error) {
	col, ok := taskSortColumns[sortBy]
	if !ok {
		col = "created_at"
	}
	order := "DESC"
	if strings.EqualFold(sortOrder, "ASC") {
		order = "ASC"
	}
	sql := listTasksBaseSQL + " ORDER BY " + col + " " + order + ", id " + order
	rows, err := q.db.Query(ctx, sql, arg.StatusFilter, arg.PriorityFilter)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var items []Task
	for rows.Next() {
		var i Task
		if err := rows.Scan(
			&i.ID,
			&i.SyncID,
			&i.Title,
			&i.Description,
			&i.Status,
			&i.Priority,
			&i.DueDate,
			&i.Tags,
			&i.RemindAt,
			&i.ReminderTime,
			&i.IsDaily,
			&i.LastCompletedDate,
			&i.DeletedAt,
			&i.CreatedAt,
			&i.UpdatedAt,
		); err != nil {
			return nil, err
		}
		items = append(items, i)
	}
	return items, rows.Err()
}
