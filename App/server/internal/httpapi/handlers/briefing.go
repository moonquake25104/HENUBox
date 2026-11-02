package handlers

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"kairos/server/internal/domain/calendar"
	"kairos/server/internal/domain/termphase"
	"kairos/server/internal/httpapi/dto"
	"kairos/server/internal/store"
)

// Briefing implements the /api/briefing endpoints.
type Briefing struct {
	Q   *store.Queries
	Log *slog.Logger
}

// Today handles GET /api/briefing/today.
func (h *Briefing) Today(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	now := time.Now().In(calendar.ChinaTZ)

	courses, err := h.Q.ListCourses(ctx, pgtype.Text{})
	if err != nil {
		h.Log.Error("list courses for briefing", "error", err)
		writeError(w, http.StatusInternalServerError, "internal server error")
		return
	}
	contexts, err := h.Q.ListSemesterContexts(ctx)
	if err != nil {
		h.Log.Error("list semester contexts for briefing", "error", err)
		writeError(w, http.StatusInternalServerError, "internal server error")
		return
	}
	tasks, err := h.Q.ListTasks(ctx, store.ListTasksParams{})
	if err != nil {
		h.Log.Error("list tasks for briefing", "error", err)
		writeError(w, http.StatusInternalServerError, "internal server error")
		return
	}
	exams, err := h.Q.ListExams(ctx)
	if err != nil {
		h.Log.Error("list exams for briefing", "error", err)
		writeError(w, http.StatusInternalServerError, "internal server error")
		return
	}
	phaseStatus, err := termphase.GetCurrentPhaseStatus(ctx, h.Q, termphase.DefaultSource)
	if err != nil {
		h.Log.Error("get current phase status for briefing", "error", err)
		writeError(w, http.StatusInternalServerError, "internal server error")
		return
	}

	pomodoro := buildPomodoroBriefing(ctx, h, now)
	resp := calendar.BuildTodayBriefing(courses, contexts, tasks, exams, phaseStatus, pomodoro, now)
	writeJSON(w, http.StatusOK, dto.FromTodayBriefingResponse(resp))
}

func buildPomodoroBriefing(ctx context.Context, h *Briefing, now time.Time) calendar.PomodoroBriefing {
	completed, err := h.Q.CountCompletedWorkSessions(ctx, store.CountCompletedWorkSessionsParams{
		WindowStart: pgtype.Timestamptz{Time: startOfDayUTC(now), Valid: true},
		WindowEnd:   pgtype.Timestamptz{Time: startOfDayUTC(now).AddDate(0, 0, 1), Valid: true},
	})
	if err != nil {
		h.Log.Error("count completed work sessions for briefing", "error", err)
		completed = 0
	}

	state, err := h.Q.GetRuntimeState(ctx)
	if errors.Is(err, pgx.ErrNoRows) {
		return calendar.PomodoroBriefing{Phase: "work", CompletedSessions: completed}
	}
	if err != nil {
		h.Log.Error("get runtime state for briefing", "error", err)
		return calendar.PomodoroBriefing{Phase: "work", CompletedSessions: completed}
	}
	return calendar.PomodoroBriefing{
		IsRunning:         state.IsRunning,
		Phase:             state.Phase,
		RemainingSeconds:  int64(state.RemainingSeconds),
		CompletedSessions: completed,
	}
}

func startOfDayUTC(t time.Time) time.Time {
	start := time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, calendar.ChinaTZ)
	return start.UTC()
}
