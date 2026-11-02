package handlers

import (
	"errors"
	"net/http"

	webpush "github.com/SherClockHolmes/webpush-go"
	"github.com/jackc/pgx/v5"
	"kairos/server/internal/domain/push"
	"kairos/server/internal/httpapi/middleware"
)

type Push struct{ Service *push.Service }

func (h *Push) Config(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, 200, map[string]any{"enabled": true, "public_key": h.Service.Keys.Public, "course_reminders": false})
}
func (h *Push) Subscribe(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, 8192)
	var sub webpush.Subscription
	if !decodeJSON(w, r, &sub) {
		return
	}
	if !push.ValidSubscription(sub) {
		writeError(w, 400, "订阅无效或推送服务暂未支持，请检查浏览器订阅结果")
		return
	}
	s, err := h.Service.Q.SavePushSubscription(r.Context(), middleware.UsernameFrom(r.Context()), sub.Endpoint, sub.Keys.P256dh, sub.Keys.Auth)
	if err != nil {
		writeError(w, 500, "无法保存设备订阅")
		return
	}
	writeJSON(w, 200, s)
}
func (h *Push) Device(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	user := middleware.UsernameFrom(r.Context())
	sub, err := h.Service.Q.GetPushSubscription(r.Context(), user, id)
	if err != nil {
		writeStoreError(w, err)
		return
	}
	if r.Method == http.MethodDelete {
		if err := h.Service.Q.DisablePushSubscription(r.Context(), user, id); err != nil {
			writeStoreError(w, err)
			return
		}
		writeJSON(w, 200, map[string]bool{"disabled": true})
		return
	}
	writeJSON(w, 200, sub)
}
func (h *Push) Test(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, 1024)
	var body struct {
		SubscriptionID int64 `json:"subscription_id"`
		Delay          int   `json:"delay_seconds"`
	}
	if !decodeJSON(w, r, &body) {
		return
	}
	if body.Delay != 0 && body.Delay != 60 && body.Delay != 900 {
		writeError(w, 400, "测试延时只能为 0、60 或 900 秒")
		return
	}
	user := middleware.UsernameFrom(r.Context())
	sub, err := h.Service.Q.GetPushSubscription(r.Context(), user, body.SubscriptionID)
	if err != nil {
		writeStoreError(w, err)
		return
	}
	if !sub.Active {
		writeError(w, 409, "设备已停用，请重新启用手机提醒")
		return
	}
	job, err := h.Service.Queue(r.Context(), user, body.SubscriptionID, body.Delay)
	if errors.Is(err, pgx.ErrNoRows) {
		writeError(w, 429, "请等待 30 秒后再试，每天最多 20 条测试且同时最多 3 条待发送")
		return
	}
	if err != nil {
		writeStoreError(w, err)
		return
	}
	writeJSON(w, 202, job)
}
func (h *Push) Tests(w http.ResponseWriter, r *http.Request) {
	list, err := h.Service.Q.ListPushTests(r.Context(), middleware.UsernameFrom(r.Context()))
	if err != nil {
		writeStoreError(w, err)
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, 200, list)
}
func (h *Push) Confirm(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	if err := h.Service.Q.ConfirmPushTest(r.Context(), middleware.UsernameFrom(r.Context()), id); err != nil {
		writeStoreError(w, err)
		return
	}
	writeJSON(w, 200, map[string]bool{"recorded": true})
}
