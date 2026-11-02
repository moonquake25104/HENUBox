package handlers

import (
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"kairos/server/internal/domain/ai"
	"kairos/server/internal/domain/calendar"
	"kairos/server/internal/httpapi/dto"
	"kairos/server/internal/store"
)

// AI implements the /api/ai endpoints.
type AI struct {
	Q         *store.Queries
	Log       *slog.Logger
	DataDir   string
	Scheduler NotifyRecomputer
	gen       *ai.Generator
}

// NewAI builds the AI handler with its own domain generator.
func NewAI(q *store.Queries, log *slog.Logger, dataDir string) *AI {
	return NewAIWithGenerator(q, log, dataDir, nil)
}

// NewAIWithGenerator builds the AI handler with a shared generator. When gen is
// nil a fresh one is created; the router injects the scheduler's instance so the
// per-date cache and in-flight guard stay coherent across handlers.
func NewAIWithGenerator(q *store.Queries, log *slog.Logger, dataDir string, gen *ai.Generator) *AI {
	if gen == nil {
		gen = ai.NewGenerator(q, dataDir)
	}
	return &AI{Q: q, Log: log, DataDir: dataDir, gen: gen}
}

// GetConfig handles GET /api/ai/config.
func (h *AI) GetConfig(w http.ResponseWriter, r *http.Request) {
	cfg, err := h.Q.GetAiConfig(r.Context())
	if err != nil {
		h.Log.Error("get ai config", "error", err)
		writeError(w, http.StatusInternalServerError, "internal server error")
		return
	}
	writeJSON(w, http.StatusOK, dto.FromAiConfig(cfg))
}

// UpdateConfig handles PATCH /api/ai/config.
func (h *AI) UpdateConfig(w http.ResponseWriter, r *http.Request) {
	var req dto.UpdateAiConfigRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	ctx := r.Context()
	cur, err := h.Q.GetAiConfig(ctx)
	if err != nil {
		h.Log.Error("get ai config", "error", err)
		writeError(w, http.StatusInternalServerError, "internal server error")
		return
	}

	params := store.UpdateAiConfigParams{
		Enabled:         cur.Enabled,
		BaseUrl:         cur.BaseUrl,
		Model:           cur.Model,
		ApiKeyEncrypted: cur.ApiKeyEncrypted,
		SyncEnabled:     cur.SyncEnabled,
	}
	if req.Enabled != nil {
		params.Enabled = *req.Enabled
	}
	if req.BaseURL != nil {
		params.BaseUrl = *req.BaseURL
	}
	if req.Model != nil {
		params.Model = *req.Model
	}
	if req.SyncEnabled != nil {
		params.SyncEnabled = *req.SyncEnabled
	}
	if req.APIKey != nil {
		encrypted, err := h.encryptAPIKey(*req.APIKey)
		if err != nil {
			h.Log.Error("encrypt ai api key", "error", err)
			writeError(w, http.StatusInternalServerError, "internal server error")
			return
		}
		params.ApiKeyEncrypted = encrypted
	}

	if err := h.Q.UpdateAiConfig(ctx, params); err != nil {
		h.Log.Error("update ai config", "error", err)
		writeError(w, http.StatusInternalServerError, "internal server error")
		return
	}
	updated, err := h.Q.GetAiConfig(ctx)
	if err != nil {
		h.Log.Error("get ai config after update", "error", err)
		writeError(w, http.StatusInternalServerError, "internal server error")
		return
	}
	if h.Scheduler != nil {
		h.Scheduler.RecomputeAI(ctx)
	}
	writeJSON(w, http.StatusOK, dto.FromAiConfig(updated))
}

func (h *AI) encryptAPIKey(plaintext string) (string, error) {
	if strings.TrimSpace(plaintext) == "" {
		return "", nil
	}
	key, err := ai.EnsureKeyFile(h.DataDir)
	if err != nil {
		return "", err
	}
	return ai.EncryptAPIKey(plaintext, key)
}

// GetMorningBrief handles GET /api/ai/morning-brief?date=. A missing brief is
// returned as JSON null (the frontend shows the generate button).
func (h *AI) GetMorningBrief(w http.ResponseWriter, r *http.Request) {
	date, ok := resolveBriefDate(w, r)
	if !ok {
		return
	}
	brief, err := h.Q.GetMorningBrief(r.Context(), store.DateOf(date))
	if errors.Is(err, pgx.ErrNoRows) {
		writeJSON(w, http.StatusOK, nil)
		return
	}
	if err != nil {
		h.Log.Error("get morning brief", "error", err)
		writeError(w, http.StatusInternalServerError, "internal server error")
		return
	}
	writeJSON(w, http.StatusOK, dto.FromMorningBrief(brief))
}

// GenerateMorningBrief handles POST /api/ai/morning-brief/generate. The default
// path streams SSE deltas; ?sync=true returns the full brief as JSON.
func (h *AI) GenerateMorningBrief(w http.ResponseWriter, r *http.Request) {
	date := strings.TrimSpace(r.URL.Query().Get("date"))
	force := r.URL.Query().Get("force") == "true"

	if r.URL.Query().Get("sync") == "true" {
		brief, err := h.gen.GenerateBrief(r.Context(), date, force, nil)
		if err != nil {
			h.writeGenerateError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, dto.FromMorningBrief(*brief))
		return
	}

	flusher, ok := w.(http.Flusher)
	if !ok {
		writeError(w, http.StatusInternalServerError, "streaming unsupported")
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")
	w.WriteHeader(http.StatusOK)

	emitted := false
	onDelta := func(delta string) {
		emitted = true
		writeSSE(w, dto.AiStreamChunk{Delta: delta})
		flusher.Flush()
	}

	brief, err := h.gen.GenerateBrief(r.Context(), date, force, onDelta)
	if err != nil {
		h.Log.Error("generate morning brief stream", "error", err)
		writeSSE(w, map[string]string{"error": ai.UserMessage(err)})
		writeSSEDone(w)
		flusher.Flush()
		return
	}
	if !emitted {
		writeSSE(w, dto.AiStreamChunk{Delta: brief.Markdown})
		flusher.Flush()
	}
	writeSSEDone(w)
	flusher.Flush()
}

// GetRecoveryKey handles GET /api/ai/sync-recovery-key.
func (h *AI) GetRecoveryKey(w http.ResponseWriter, r *http.Request) {
	dek, err := ai.LoadDEK(h.DataDir)
	if err != nil {
		h.Log.Error("load ai recovery key", "error", err)
		writeError(w, http.StatusInternalServerError, "internal server error")
		return
	}
	if dek == nil {
		writeJSON(w, http.StatusOK, dto.RecoveryKeyResponse{RecoveryKey: nil})
		return
	}
	key := ai.RecoveryKeyHex(*dek)
	writeJSON(w, http.StatusOK, dto.RecoveryKeyResponse{RecoveryKey: &key})
}

// SetRecoveryKey handles POST /api/ai/sync-recovery-key.
func (h *AI) SetRecoveryKey(w http.ResponseWriter, r *http.Request) {
	var req dto.SetRecoveryKeyRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	dek, err := ai.RecoveryKeyFromHex(req.RecoveryKey)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if err := ai.SaveDEK(h.DataDir, &dek); err != nil {
		h.Log.Error("save ai recovery key", "error", err)
		writeError(w, http.StatusInternalServerError, "internal server error")
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (h *AI) writeGenerateError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, ai.ErrInvalidDate):
		writeError(w, http.StatusBadRequest, err.Error())
	case errors.Is(err, ai.ErrGenerating):
		writeError(w, http.StatusConflict, err.Error())
	default:
		h.Log.Error("generate morning brief", "error", err)
		writeError(w, http.StatusInternalServerError, "internal server error")
	}
}

func resolveBriefDate(w http.ResponseWriter, r *http.Request) (string, bool) {
	date := strings.TrimSpace(r.URL.Query().Get("date"))
	if date == "" {
		return ai.TodayChina(), true
	}
	if _, err := time.ParseInLocation("2006-01-02", date, calendar.ChinaTZ); err != nil {
		writeError(w, http.StatusBadRequest, ai.ErrInvalidDate.Error())
		return "", false
	}
	return date, true
}

func writeSSE(w http.ResponseWriter, payload any) {
	raw, err := json.Marshal(payload)
	if err != nil {
		return
	}
	_, _ = fmt.Fprintf(w, "data: %s\n\n", raw)
}

func writeSSEDone(w http.ResponseWriter) {
	_, _ = fmt.Fprint(w, "data: [DONE]\n\n")
}
