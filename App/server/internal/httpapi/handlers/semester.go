package handlers

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"strings"

	"kairos/server/internal/domain/termphase"
	"kairos/server/internal/httpapi/dto"
	"kairos/server/internal/store"
)

// Semester implements the /api/semesters and /api/term-phases endpoints.
type Semester struct {
	Q         *store.Queries
	Log       *slog.Logger
	Scheduler NotifyRecomputer
}

// ListSemesters handles GET /api/semesters.
func (h *Semester) ListSemesters(w http.ResponseWriter, r *http.Request) {
	contexts, err := h.Q.ListSemesterContexts(r.Context())
	if err != nil {
		h.Log.Error("list semester contexts", "error", err)
		writeError(w, http.StatusInternalServerError, "internal server error")
		return
	}
	out := make([]dto.SemesterContext, 0, len(contexts))
	for _, c := range contexts {
		out = append(out, dto.FromSemesterContext(c))
	}
	writeJSON(w, http.StatusOK, out)
}

// ListPhases handles GET /api/term-phases?term_label=.
func (h *Semester) ListPhases(w http.ResponseWriter, r *http.Request) {
	termLabel := strings.TrimSpace(r.URL.Query().Get("term_label"))
	phases, err := h.Q.ListTermPhases(r.Context(), termLabel)
	if err != nil {
		h.Log.Error("list term phases", "error", err)
		writeError(w, http.StatusInternalServerError, "internal server error")
		return
	}
	out := make([]dto.TermPhase, 0, len(phases))
	for _, p := range phases {
		out = append(out, dto.FromTermPhase(p))
	}
	writeJSON(w, http.StatusOK, out)
}

// CreatePhase handles POST /api/term-phases.
func (h *Semester) CreatePhase(w http.ResponseWriter, r *http.Request) {
	var req dto.CreateTermPhaseRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	if err := validatePhaseRequest(req.TermLabel, req.PhaseType, req.StartWeek, req.EndWeek, req.NotificationRulesJSON); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	phase, err := h.Q.CreateTermPhase(r.Context(), store.CreateTermPhaseParams{
		SyncID:                   "",
		TermLabel:                strings.TrimSpace(req.TermLabel),
		PhaseType:                req.PhaseType,
		StartWeek:                req.StartWeek,
		EndWeek:                  req.EndWeek,
		AffectsCourses:           req.AffectsCourses,
		AffectsExamNotifications: req.AffectsExamNotifications,
		PomodoroProfile:          normalizedProfile(req.PomodoroProfile),
		NotificationRules:        normalizedRules(req.NotificationRulesJSON),
		SortOrder:                req.SortOrder,
	})
	if err != nil {
		h.Log.Error("create term phase", "error", err)
		writeError(w, http.StatusInternalServerError, "internal server error")
		return
	}
	if h.Scheduler != nil {
		h.Scheduler.RecomputeExams(r.Context())
	}
	writeJSON(w, http.StatusCreated, dto.FromTermPhase(phase))
}

// UpdatePhase handles PATCH /api/term-phases/{id}.
func (h *Semester) UpdatePhase(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	var req dto.UpdateTermPhaseRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	if err := validatePhaseRequest(req.TermLabel, req.PhaseType, req.StartWeek, req.EndWeek, req.NotificationRulesJSON); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	phase, err := h.Q.UpdateTermPhase(r.Context(), store.UpdateTermPhaseParams{
		TermLabel:                strings.TrimSpace(req.TermLabel),
		PhaseType:                req.PhaseType,
		StartWeek:                req.StartWeek,
		EndWeek:                  req.EndWeek,
		AffectsCourses:           req.AffectsCourses,
		AffectsExamNotifications: req.AffectsExamNotifications,
		PomodoroProfile:          normalizedProfile(req.PomodoroProfile),
		NotificationRules:        normalizedRules(req.NotificationRulesJSON),
		SortOrder:                req.SortOrder,
		ID:                       id,
	})
	if err != nil {
		writeStoreError(w, err)
		return
	}
	if h.Scheduler != nil {
		h.Scheduler.RecomputeExams(r.Context())
	}
	writeJSON(w, http.StatusOK, dto.FromTermPhase(phase))
}

// DeletePhase handles DELETE /api/term-phases/{id} (soft delete).
func (h *Semester) DeletePhase(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	if err := h.Q.SoftDeleteTermPhase(r.Context(), id); err != nil {
		writeStoreError(w, err)
		return
	}
	if h.Scheduler != nil {
		h.Scheduler.RecomputeExams(r.Context())
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

// CurrentStatus handles GET /api/term-phases/current-status?source=.
func (h *Semester) CurrentStatus(w http.ResponseWriter, r *http.Request) {
	source := r.URL.Query().Get("source")
	if source == "" {
		source = termphase.DefaultSource
	}
	status, err := termphase.GetCurrentPhaseStatus(r.Context(), h.Q, source)
	if err != nil {
		h.Log.Error("get current phase status", "error", err)
		writeError(w, http.StatusInternalServerError, "internal server error")
		return
	}
	writeJSON(w, http.StatusOK, status)
}

func validatePhaseRequest(termLabel, phaseType string, startWeek, endWeek int32, rulesJSON string) error {
	if strings.TrimSpace(termLabel) == "" {
		return errMsg("term_label is required")
	}
	switch phaseType {
	case termphase.PhaseTeaching, termphase.PhaseExam, termphase.PhaseBreak:
	default:
		return errMsg("invalid phase_type")
	}
	if startWeek < 1 {
		return errMsg("start_week must be >= 1")
	}
	if endWeek < startWeek {
		return errMsg("end_week must be >= start_week")
	}
	if !json.Valid([]byte(rulesJSON)) {
		return errMsg("notification_rules_json must be a JSON object")
	}
	return nil
}

func normalizedProfile(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return "default"
	}
	return value
}

func normalizedRules(value string) []byte {
	value = strings.TrimSpace(value)
	if value == "" {
		return []byte("{}")
	}
	return []byte(value)
}

type apiError string

func (e apiError) Error() string { return string(e) }

func errMsg(s string) error { return apiError(s) }
