package handlers

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"kairos/server/internal/domain/calendar"
	"kairos/server/internal/domain/termphase"
	"kairos/server/internal/httpapi/dto"
	"kairos/server/internal/store"
)

// Calendar implements the /api/calendar endpoints.
type Calendar struct {
	Q   *store.Queries
	Log *slog.Logger
}

// Week handles GET /api/calendar/week?semester=&week_index=&semester_start_date=.
func (h *Calendar) Week(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	semester := strings.TrimSpace(q.Get("semester"))
	weekIndex, err := strconv.ParseInt(q.Get("week_index"), 10, 64)
	if err != nil || weekIndex < 1 {
		writeError(w, http.StatusBadRequest, "week_index must be >= 1")
		return
	}
	var requestedStart *string
	if v := strings.TrimSpace(q.Get("semester_start_date")); v != "" {
		requestedStart = &v
	}

	courses, err := h.Q.ListCourses(r.Context(), semesterFilter(semester))
	if err != nil {
		h.Log.Error("list courses for week schedule", "error", err)
		writeError(w, http.StatusInternalServerError, "internal server error")
		return
	}
	exams, err := h.Q.ListExams(r.Context())
	if err != nil {
		h.Log.Error("list exams for week schedule", "error", err)
		writeError(w, http.StatusInternalServerError, "internal server error")
		return
	}
	resolvedStart, err := effectiveSemesterStartDate(r.Context(), h.Q, semester, requestedStart)
	if err != nil {
		h.Log.Error("resolve semester start date", "error", err)
		writeError(w, http.StatusInternalServerError, "internal server error")
		return
	}
	phaseStatus, err := termphase.GetPhaseStatusForTermWeek(r.Context(), h.Q, termphase.DefaultSource, semester, weekIndex)
	if err != nil {
		h.Log.Error("get phase status for term week", "error", err)
		writeError(w, http.StatusInternalServerError, "internal server error")
		return
	}

	resp, err := calendar.BuildWeekSchedule(
		courses, exams, semester, weekIndex, resolvedStart,
		phaseStatus.PhaseType, phaseStatus.CoursesVisible,
	)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, dto.FromWeekScheduleResponse(resp))
}

// WeekEvents handles GET /api/calendar/week-events?semester=&week_start_date=&week_index=.
// It mirrors the Week schedule query but aggregates courses, exams and tasks
// into calendar events (including todos) for the integrated calendar view.
func (h *Calendar) WeekEvents(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	semester := strings.TrimSpace(q.Get("semester"))
	weekIndex, err := strconv.ParseInt(q.Get("week_index"), 10, 64)
	if err != nil || weekIndex < 1 {
		writeError(w, http.StatusBadRequest, "week_index must be >= 1")
		return
	}
	var requestedStart *string
	if v := strings.TrimSpace(q.Get("semester_start_date")); v != "" {
		requestedStart = &v
	}
	var requestedWeekStart *string
	if v := strings.TrimSpace(q.Get("week_start_date")); v != "" {
		requestedWeekStart = &v
	}

	courses, err := h.Q.ListCourses(r.Context(), semesterFilter(semester))
	if err != nil {
		h.Log.Error("list courses for calendar week", "error", err)
		writeError(w, http.StatusInternalServerError, "internal server error")
		return
	}
	exams, err := h.Q.ListExams(r.Context())
	if err != nil {
		h.Log.Error("list exams for calendar week", "error", err)
		writeError(w, http.StatusInternalServerError, "internal server error")
		return
	}
	tasks, err := h.Q.ListTasks(r.Context(), store.ListTasksParams{})
	if err != nil {
		h.Log.Error("list tasks for calendar week", "error", err)
		writeError(w, http.StatusInternalServerError, "internal server error")
		return
	}
	resolvedStart, err := effectiveSemesterStartDate(r.Context(), h.Q, semester, requestedStart)
	if err != nil {
		h.Log.Error("resolve semester start date", "error", err)
		writeError(w, http.StatusInternalServerError, "internal server error")
		return
	}
	phaseStatus, err := termphase.GetPhaseStatusForTermWeek(r.Context(), h.Q, termphase.DefaultSource, semester, weekIndex)
	if err != nil {
		h.Log.Error("get phase status for term week", "error", err)
		writeError(w, http.StatusInternalServerError, "internal server error")
		return
	}

	resp, err := calendar.BuildCalendarWeek(
		courses, exams, tasks, semester, weekIndex, resolvedStart, requestedWeekStart,
		phaseStatus.PhaseType, phaseStatus.CoursesVisible,
	)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, dto.FromCalendarWeekResponse(resp))
}

// Day handles GET /api/calendar/day?date=&semester=.
func (h *Calendar) Day(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	dateStr := strings.TrimSpace(q.Get("date"))
	if dateStr == "" {
		writeError(w, http.StatusBadRequest, "date is required")
		return
	}
	date, err := time.ParseInLocation("2006-01-02", dateStr, calendar.ChinaTZ)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid date, want YYYY-MM-DD")
		return
	}
	semester := strings.TrimSpace(q.Get("semester"))

	courses, err := h.Q.ListCourses(r.Context(), semesterFilter(semester))
	if err != nil {
		h.Log.Error("list courses for calendar day", "error", err)
		writeError(w, http.StatusInternalServerError, "internal server error")
		return
	}
	exams, err := h.Q.ListExams(r.Context())
	if err != nil {
		h.Log.Error("list exams for calendar day", "error", err)
		writeError(w, http.StatusInternalServerError, "internal server error")
		return
	}
	tasks, err := h.Q.ListTasks(r.Context(), store.ListTasksParams{})
	if err != nil {
		h.Log.Error("list tasks for calendar day", "error", err)
		writeError(w, http.StatusInternalServerError, "internal server error")
		return
	}
	resolvedStart, err := effectiveSemesterStartDate(r.Context(), h.Q, semester, nil)
	if err != nil {
		h.Log.Error("resolve semester start date", "error", err)
		writeError(w, http.StatusInternalServerError, "internal server error")
		return
	}

	weekStartDate := mondayOf(date)
	weekIndex := int64(1)
	if resolvedStart != nil {
		if idx, err := calendar.CurrentWeekIndexFromStart(*resolvedStart, date); err == nil {
			weekIndex = idx
		}
	}
	phaseStatus, err := termphase.GetPhaseStatusForTermWeek(r.Context(), h.Q, termphase.DefaultSource, semester, weekIndex)
	if err != nil {
		h.Log.Error("get phase status for term week", "error", err)
		writeError(w, http.StatusInternalServerError, "internal server error")
		return
	}

	resp, err := calendar.BuildCalendarWeek(
		courses, exams, tasks, semester, 1, resolvedStart, &weekStartDate,
		phaseStatus.PhaseType, phaseStatus.CoursesVisible,
	)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, dto.FromCalendarWeekResponse(resp))
}

func semesterFilter(semester string) pgtype.Text {
	if semester == "" {
		return pgtype.Text{}
	}
	return store.TextOf(semester)
}

func effectiveSemesterStartDate(ctx context.Context, q *store.Queries, semester string, requested *string) (*string, error) {
	if requested != nil && strings.TrimSpace(*requested) != "" {
		return requested, nil
	}
	if strings.TrimSpace(semester) == "" {
		c, err := q.GetLatestSemesterContextBySource(ctx, termphase.DefaultSource)
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		if err != nil {
			return nil, err
		}
		return strPtr(store.DateString(c.StartDate)), nil
	}
	c, err := q.GetSemesterContext(ctx, store.GetSemesterContextParams{
		Source:    termphase.DefaultSource,
		TermLabel: semester,
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return strPtr(store.DateString(c.StartDate)), nil
}

func mondayOf(t time.Time) string {
	daysFromMonday := (int(t.Weekday()) + 6) % 7
	return t.AddDate(0, 0, -daysFromMonday).Format("2006-01-02")
}

func strPtr(s string) *string { return &s }
