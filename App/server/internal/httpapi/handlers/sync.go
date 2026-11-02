package handlers

import (
	"errors"
	"log/slog"
	"net/http"

	"kairos/server/internal/domain/sync"
	"kairos/server/internal/httpapi/dto"
	"kairos/server/internal/store"
)

// Sync implements the /api/sync endpoints.
type Sync struct {
	Q   *store.Queries
	Log *slog.Logger
	svc *sync.Service
}

// NewSync builds the sync handler with its domain service.
func NewSync(q *store.Queries, pool sync.TxBeginner, log *slog.Logger, dataDir, owner string) *Sync {
	return &Sync{
		Q:   q,
		Log: log,
		svc: &sync.Service{Q: q, Pool: pool, DataDir: dataDir, Owner: owner},
	}
}

// GetConfig handles GET /api/sync/config.
func (h *Sync) GetConfig(w http.ResponseWriter, r *http.Request) {
	if err := sync.EnsureDeviceIDs(r.Context(), h.Q); err != nil {
		h.Log.Error("ensure sync device ids", "error", err)
		writeError(w, http.StatusInternalServerError, "internal server error")
		return
	}
	cfg, err := h.Q.GetSyncConfig(r.Context())
	if err != nil {
		h.Log.Error("get sync config", "error", err)
		writeError(w, http.StatusInternalServerError, "internal server error")
		return
	}
	writeJSON(w, http.StatusOK, dto.FromSyncConfig(cfg))
}

// UpdateConfig handles PATCH /api/sync/config.
func (h *Sync) UpdateConfig(w http.ResponseWriter, r *http.Request) {
	var req dto.UpdateSyncConfigRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	cfg, err := h.Q.GetSyncConfig(r.Context())
	if err != nil {
		h.Log.Error("get sync config", "error", err)
		writeError(w, http.StatusInternalServerError, "internal server error")
		return
	}
	password := cfg.Password
	if req.Password != nil {
		password = *req.Password
	}
	if err := h.Q.UpdateSyncConfigCredentials(r.Context(), store.UpdateSyncConfigCredentialsParams{
		ServerUrl: req.ServerURL,
		Username:  req.Username,
		Password:  password,
		AutoSync:  req.AutoSync,
	}); err != nil {
		h.Log.Error("update sync config", "error", err)
		writeError(w, http.StatusInternalServerError, "internal server error")
		return
	}
	updated, err := h.Q.GetSyncConfig(r.Context())
	if err != nil {
		h.Log.Error("get sync config", "error", err)
		writeError(w, http.StatusInternalServerError, "internal server error")
		return
	}
	writeJSON(w, http.StatusOK, dto.FromSyncConfig(updated))
}

// TestConnection handles POST /api/sync/test.
func (h *Sync) TestConnection(w http.ResponseWriter, r *http.Request) {
	ok, err := h.svc.TestConnection(r.Context())
	if err != nil {
		if errors.Is(err, sync.ErrNotConfigured) {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		h.Log.Error("test sync connection", "error", err)
		writeJSON(w, http.StatusOK, false)
		return
	}
	writeJSON(w, http.StatusOK, ok)
}

// SyncNow handles POST /api/sync/now.
func (h *Sync) SyncNow(w http.ResponseWriter, r *http.Request) {
	result, err := h.svc.SyncNow(r.Context())
	if err != nil {
		if errors.Is(err, sync.ErrNotConfigured) {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		if errors.Is(err, sync.ErrAcademicSyncUnsupported) {
			writeErrorCode(w, http.StatusConflict, err.Error(), "ACADEMIC_SYNC_UNSUPPORTED", nil)
			return
		}
		h.Log.Error("sync now", "error", err)
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, result)
}

// GetRecoveryKey handles GET /api/sync/ai-recovery-key.
func (h *Sync) GetRecoveryKey(w http.ResponseWriter, r *http.Request) {
	key, err := h.svc.GetRecoveryKey(r.Context())
	if err != nil {
		h.Log.Error("get ai recovery key", "error", err)
		writeError(w, http.StatusInternalServerError, "internal server error")
		return
	}
	writeJSON(w, http.StatusOK, dto.RecoveryKeyResponse{RecoveryKey: key})
}

// SetRecoveryKey handles POST /api/sync/ai-recovery-key.
func (h *Sync) SetRecoveryKey(w http.ResponseWriter, r *http.Request) {
	var req dto.SetRecoveryKeyRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	if err := h.svc.SetRecoveryKey(r.Context(), req.RecoveryKey); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}
