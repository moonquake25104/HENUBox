package handlers

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"

	"kairos/server/internal/httpapi/dto"
	"kairos/server/internal/store"
)

// NotifyRecomputer is the subset of notify.Scheduler the HTTP layer drives
// after mutations. A nil value disables recomputation.
type NotifyRecomputer interface {
	RecomputeExams(ctx context.Context)
	RecomputeTasks(ctx context.Context)
	RecomputeCourses(ctx context.Context)
	RecomputeAI(ctx context.Context)
}

// Notify implements the /api/notify endpoints.
type Notify struct {
	Q         *store.Queries
	Log       *slog.Logger
	Scheduler NotifyRecomputer
}

// NewNotify builds the notification-config handler.
func NewNotify(q *store.Queries, log *slog.Logger, sched NotifyRecomputer) *Notify {
	return &Notify{Q: q, Log: log, Scheduler: sched}
}

// GetConfig handles GET /api/notify/config.
func (h *Notify) GetConfig(w http.ResponseWriter, r *http.Request) {
	cfg, err := h.Q.GetNotificationConfig(r.Context())
	if err != nil {
		h.Log.Error("get notification config", "error", err)
		writeError(w, http.StatusInternalServerError, "internal server error")
		return
	}
	writeJSON(w, http.StatusOK, dto.FromNotificationConfig(cfg))
}

// UpdateConfig handles PATCH /api/notify/config with merge semantics.
func (h *Notify) UpdateConfig(w http.ResponseWriter, r *http.Request) {
	var req dto.UpdateNotificationConfigRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	ctx := r.Context()
	cur, err := h.Q.GetNotificationConfig(ctx)
	if err != nil {
		h.Log.Error("get notification config", "error", err)
		writeError(w, http.StatusInternalServerError, "internal server error")
		return
	}

	enabled := cur.Enabled
	if req.Enabled != nil {
		enabled = *req.Enabled
	}
	offsets := cur.ExamOffsets
	if req.ExamOffsetsJSON != nil {
		// 数据库列是 jsonb：非法 JSON 绝不能写进 Postgres（会直接报错）。
		// Rust 侧存 TEXT 可惰性回退，迁移到 jsonb 后必须在写入前校验为正整数
		// 数组（允许空数组 []），这是本 handler 相对原实现的必要适配。
		if !validExamOffsets(*req.ExamOffsetsJSON) {
			writeError(w, http.StatusBadRequest, "exam_offsets_json 必须是正整数组成的 JSON 数组")
			return
		}
		offsets = []byte(*req.ExamOffsetsJSON)
	}
	courseOffsets := cur.CourseOffsets
	if req.CourseOffsetsJSON != nil {
		if !validOffsets(*req.CourseOffsetsJSON) {
			writeError(w, http.StatusBadRequest, "course_offsets_json 必须是正整数组成的 JSON 数组")
			return
		}
		courseOffsets = []byte(*req.CourseOffsetsJSON)
	}

	// android_channel_created 仅用于前端类型兼容，不入库。
	if err := h.Q.UpdateNotificationConfig(ctx, store.UpdateNotificationConfigParams{
		Enabled:       enabled,
		ExamOffsets:   offsets,
		CourseOffsets: courseOffsets,
	}); err != nil {
		h.Log.Error("update notification config", "error", err)
		writeError(w, http.StatusInternalServerError, "internal server error")
		return
	}
	updated, err := h.Q.GetNotificationConfig(ctx)
	if err != nil {
		h.Log.Error("get notification config after update", "error", err)
		writeError(w, http.StatusInternalServerError, "internal server error")
		return
	}
	if h.Scheduler != nil && (req.Enabled != nil || req.ExamOffsetsJSON != nil || req.CourseOffsetsJSON != nil) {
		h.Scheduler.RecomputeExams(ctx)
		h.Scheduler.RecomputeCourses(ctx)
	}
	writeJSON(w, http.StatusOK, dto.FromNotificationConfig(updated))
}

func validExamOffsets(raw string) bool {
	return validOffsets(raw)
}

func validOffsets(raw string) bool {
	var offsets []int
	if err := json.Unmarshal([]byte(raw), &offsets); err != nil {
		return false
	}
	if offsets == nil {
		return false
	}
	for _, offset := range offsets {
		if offset <= 0 {
			return false
		}
	}
	return true
}
