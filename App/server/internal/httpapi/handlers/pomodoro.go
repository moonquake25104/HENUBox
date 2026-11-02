package handlers

import (
	"errors"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"kairos/server/internal/domain/pomodoro"
	"kairos/server/internal/httpapi/dto"
	"kairos/server/internal/store"
)

// Pomodoro implements the /api/pomodoro endpoints.
type Pomodoro struct {
	Q        *store.Queries
	Log      *slog.Logger
	Notifier pomodoro.Notifier
	svc      *pomodoro.Service
}

// NewPomodoro builds the handler with its domain service.
func NewPomodoro(q *store.Queries, log *slog.Logger, notifier pomodoro.Notifier) *Pomodoro {
	return &Pomodoro{
		Q:        q,
		Log:      log,
		Notifier: notifier,
		svc:      &pomodoro.Service{Q: q, Notifier: notifier},
	}
}

// State handles GET /api/pomodoro/state.
func (h *Pomodoro) State(w http.ResponseWriter, r *http.Request) {
	st, err := h.svc.State(r.Context(), time.Now())
	if err != nil {
		h.Log.Error("get pomodoro state", "error", err)
		writeError(w, http.StatusInternalServerError, "internal server error")
		return
	}
	writeJSON(w, http.StatusOK, st)
}

// Start handles POST /api/pomodoro/start.
func (h *Pomodoro) Start(w http.ResponseWriter, r *http.Request) {
	st, err := h.svc.Start(r.Context(), time.Now())
	if err != nil {
		h.Log.Error("start pomodoro", "error", err)
		writeError(w, http.StatusInternalServerError, "internal server error")
		return
	}
	writeJSON(w, http.StatusOK, st)
}

// Pause handles POST /api/pomodoro/pause.
func (h *Pomodoro) Pause(w http.ResponseWriter, r *http.Request) {
	st, err := h.svc.Pause(r.Context(), time.Now())
	if err != nil {
		h.Log.Error("pause pomodoro", "error", err)
		writeError(w, http.StatusInternalServerError, "internal server error")
		return
	}
	writeJSON(w, http.StatusOK, st)
}

// Reset handles POST /api/pomodoro/reset.
func (h *Pomodoro) Reset(w http.ResponseWriter, r *http.Request) {
	st, err := h.svc.Reset(r.Context(), time.Now())
	if err != nil {
		h.Log.Error("reset pomodoro", "error", err)
		writeError(w, http.StatusInternalServerError, "internal server error")
		return
	}
	writeJSON(w, http.StatusOK, st)
}

// Interrupt handles POST /api/pomodoro/interrupt (resolve_pomodoro_interruption).
func (h *Pomodoro) Interrupt(w http.ResponseWriter, r *http.Request) {
	var req dto.ResolveInterruptionRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	st, err := h.svc.ResolveInterruption(r.Context(), time.Now(), req.Action)
	if err != nil {
		h.writeServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, st)
}

// FinishPhase handles POST /api/pomodoro/finish-phase.
func (h *Pomodoro) FinishPhase(w http.ResponseWriter, r *http.Request) {
	var req dto.FinishPhaseRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	phase, ok := pomodoro.ParsePhase(strings.TrimSpace(req.Phase))
	if !ok {
		writeError(w, http.StatusBadRequest, "invalid phase")
		return
	}
	var completedAt *time.Time
	if req.CompletedAt != nil {
		t, err := time.Parse(time.RFC3339, *req.CompletedAt)
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid completed_at")
			return
		}
		completedAt = &t
	}
	st, err := h.svc.FinishPhase(r.Context(), time.Now(), phase, completedAt, req.TaskID)
	if err != nil {
		h.writeServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, st)
}

// GetConfig handles GET /api/pomodoro/config.
func (h *Pomodoro) GetConfig(w http.ResponseWriter, r *http.Request) {
	cfg, err := h.Q.GetPomodoroConfig(r.Context())
	if err != nil {
		h.Log.Error("get pomodoro config", "error", err)
		writeError(w, http.StatusInternalServerError, "internal server error")
		return
	}
	writeJSON(w, http.StatusOK, dto.FromPomodoroConfig(cfg))
}

// UpdateConfig handles PATCH /api/pomodoro/config.
func (h *Pomodoro) UpdateConfig(w http.ResponseWriter, r *http.Request) {
	var req dto.PomodoroConfig
	if !decodeJSON(w, r, &req) {
		return
	}
	if err := validatePomodoroDurations(req.WorkSeconds, req.ShortBreakSeconds, req.LongBreakSeconds, req.SessionsBeforeLongBreak); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	st, err := h.svc.UpdateConfig(r.Context(), time.Now(), req.ToConfig())
	if err != nil {
		h.Log.Error("update pomodoro config", "error", err)
		writeError(w, http.StatusInternalServerError, "internal server error")
		return
	}
	writeJSON(w, http.StatusOK, st)
}

// ListProfiles handles GET /api/pomodoro/profiles.
func (h *Pomodoro) ListProfiles(w http.ResponseWriter, r *http.Request) {
	profiles, err := h.Q.ListPomodoroProfiles(r.Context())
	if err != nil {
		h.Log.Error("list pomodoro profiles", "error", err)
		writeError(w, http.StatusInternalServerError, "internal server error")
		return
	}
	out := make([]dto.PomodoroProfile, 0, len(profiles))
	for _, p := range profiles {
		out = append(out, dto.FromPomodoroProfile(p))
	}
	writeJSON(w, http.StatusOK, out)
}

// CreateProfile handles POST /api/pomodoro/profiles.
func (h *Pomodoro) CreateProfile(w http.ResponseWriter, r *http.Request) {
	var req dto.CreatePomodoroProfileRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	name := strings.TrimSpace(req.Name)
	if name == "" {
		writeError(w, http.StatusBadRequest, "name is required")
		return
	}
	if err := validatePomodoroDurations(req.WorkSeconds, req.ShortBreakSeconds, req.LongBreakSeconds, req.SessionsBeforeLongBreak); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if h.profileNameTaken(r, name) {
		writeError(w, http.StatusConflict, "profile name already exists")
		return
	}
	profile, err := h.Q.CreatePomodoroProfile(r.Context(), store.CreatePomodoroProfileParams{
		Name:                    name,
		WorkSeconds:             req.WorkSeconds,
		ShortBreakSeconds:       req.ShortBreakSeconds,
		LongBreakSeconds:        req.LongBreakSeconds,
		SessionsBeforeLongBreak: req.SessionsBeforeLongBreak,
	})
	if err != nil {
		h.Log.Error("create pomodoro profile", "error", err)
		writeError(w, http.StatusInternalServerError, "internal server error")
		return
	}
	writeJSON(w, http.StatusCreated, dto.FromPomodoroProfile(profile))
}

// UpdateProfile handles PATCH /api/pomodoro/profiles/{id}.
func (h *Pomodoro) UpdateProfile(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	var req dto.UpdatePomodoroProfileRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	name := strings.TrimSpace(req.Name)
	if name == "" {
		writeError(w, http.StatusBadRequest, "name is required")
		return
	}
	if err := validatePomodoroDurations(req.WorkSeconds, req.ShortBreakSeconds, req.LongBreakSeconds, req.SessionsBeforeLongBreak); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	existing, err := h.Q.GetPomodoroProfile(r.Context(), id)
	if err != nil {
		writeStoreError(w, err)
		return
	}
	if existing.IsBuiltin {
		writeError(w, http.StatusBadRequest, "内置配置档不可修改")
		return
	}
	if name != existing.Name && h.profileNameTaken(r, name) {
		writeError(w, http.StatusConflict, "profile name already exists")
		return
	}
	if err := h.Q.UpdatePomodoroProfile(r.Context(), store.UpdatePomodoroProfileParams{
		Name:                    name,
		WorkSeconds:             req.WorkSeconds,
		ShortBreakSeconds:       req.ShortBreakSeconds,
		LongBreakSeconds:        req.LongBreakSeconds,
		SessionsBeforeLongBreak: req.SessionsBeforeLongBreak,
		ID:                      id,
	}); err != nil {
		h.Log.Error("update pomodoro profile", "error", err)
		writeError(w, http.StatusInternalServerError, "internal server error")
		return
	}
	updated, err := h.Q.GetPomodoroProfile(r.Context(), id)
	if err != nil {
		writeStoreError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, dto.FromPomodoroProfile(updated))
}

// DeleteProfile handles DELETE /api/pomodoro/profiles/{id}.
func (h *Pomodoro) DeleteProfile(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	existing, err := h.Q.GetPomodoroProfile(r.Context(), id)
	if err != nil {
		writeStoreError(w, err)
		return
	}
	if existing.IsBuiltin {
		writeError(w, http.StatusBadRequest, "内置配置档不可删除")
		return
	}
	if err := h.Q.DeletePomodoroProfile(r.Context(), id); err != nil {
		h.Log.Error("delete pomodoro profile", "error", err)
		writeError(w, http.StatusInternalServerError, "internal server error")
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (h *Pomodoro) writeServiceError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, pomodoro.ErrPhaseMismatch),
		errors.Is(err, pomodoro.ErrNoActiveWorkSession),
		errors.Is(err, pomodoro.ErrPhaseNotFinished):
		writeError(w, http.StatusConflict, err.Error())
	case errors.Is(err, pomodoro.ErrUnknownAction),
		errors.Is(err, pomodoro.ErrResumeAtZero):
		writeError(w, http.StatusBadRequest, err.Error())
	default:
		h.Log.Error("pomodoro service", "error", err)
		writeError(w, http.StatusInternalServerError, "internal server error")
	}
}

func (h *Pomodoro) profileNameTaken(r *http.Request, name string) bool {
	profiles, err := h.Q.ListPomodoroProfiles(r.Context())
	if err != nil {
		return false
	}
	for _, p := range profiles {
		if p.Name == name {
			return true
		}
	}
	return false
}

func validatePomodoroDurations(work, short, long, sessions int32) error {
	if work < 60 {
		return errMsg("work_seconds must be >= 60")
	}
	if short < 60 {
		return errMsg("short_break_seconds must be >= 60")
	}
	if long < 60 {
		return errMsg("long_break_seconds must be >= 60")
	}
	if sessions < 1 {
		return errMsg("sessions_before_long_break must be >= 1")
	}
	return nil
}
