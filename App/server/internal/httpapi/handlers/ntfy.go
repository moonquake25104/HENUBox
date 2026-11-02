package handlers

import (
	"errors"
	"kairos/server/internal/domain/ntfy"
	"kairos/server/internal/httpapi/middleware"
	"net/http"
)

type NativePush struct{ Client *ntfy.Client }

func (h *NativePush) allowed(w http.ResponseWriter, r *http.Request) bool {
	w.Header().Set("Cache-Control", "no-store")
	if h.Client == nil {
		writeError(w, 503, "本地 ntfy 通道尚未配置")
		return false
	}
	if middleware.UsernameFrom(r.Context()) != h.Client.Config.Owner {
		writeError(w, 403, "无权访问此通知通道")
		return false
	}
	return true
}
func (h *NativePush) Config(w http.ResponseWriter, r *http.Request) {
	if !h.allowed(w, r) {
		return
	}
	c := h.Client.Config
	writeJSON(w, 200, map[string]any{"enabled": true, "server_url": c.ServerURL, "topic": c.Topic, "username": c.ReaderUser, "apk_url": c.WebURL + "/downloads/ntfy-android.apk", "course_reminders": false})
}
func (h *NativePush) Setup(w http.ResponseWriter, r *http.Request) {
	if !h.allowed(w, r) {
		return
	}
	writeJSON(w, 200, map[string]string{"password": h.Client.Config.ReaderPassword})
}
func (h *NativePush) Test(w http.ResponseWriter, r *http.Request) {
	if !h.allowed(w, r) {
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, 1024)
	var req struct {
		Delay int `json:"delay_seconds"`
	}
	if !decodeJSON(w, r, &req) {
		return
	}
	if req.Delay != 0 && req.Delay != 60 && req.Delay != 900 {
		writeError(w, 400, "测试延时只能为 0、60 或 900 秒")
		return
	}
	receipt, err := h.Client.Send(r.Context(), req.Delay)
	if errors.Is(err, ntfy.ErrRate) {
		writeError(w, 429, err.Error())
		return
	}
	if err != nil {
		writeError(w, 502, err.Error())
		return
	}
	writeJSON(w, 202, receipt)
}
