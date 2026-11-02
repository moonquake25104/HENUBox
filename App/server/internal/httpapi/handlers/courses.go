package handlers

import (
	"context"
	"errors"
	"log/slog"
	"net/http"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"kairos/server/internal/domain/academic"
	"kairos/server/internal/domain/importer"
	"kairos/server/internal/httpapi/dto"
	"kairos/server/internal/httpapi/middleware"
	"kairos/server/internal/store"
)

// Courses implements the /api/courses endpoints. All mutations go through the
// academic transaction service so version counters and source protection are
// applied uniformly (phase B1 gate).
type Courses struct {
	Q         *store.Queries
	Log       *slog.Logger
	Scheduler NotifyRecomputer
	Academic  *academic.Service
}

func (h *Courses) owner(r *http.Request) string {
	return middleware.UsernameFrom(r.Context())
}

// List handles GET /api/courses?semester=.
func (h *Courses) List(w http.ResponseWriter, r *http.Request) {
	semester := pgtype.Text{}
	if v := r.URL.Query().Get("semester"); v != "" {
		semester = store.TextOf(v)
	}
	courses, err := h.Q.ListCourses(r.Context(), semester)
	if err != nil {
		h.Log.Error("list courses", "error", err)
		writeError(w, http.StatusInternalServerError, "internal server error")
		return
	}
	out := make([]dto.Course, 0, len(courses))
	for _, c := range courses {
		out = append(out, dto.FromCourse(c))
	}
	writeJSON(w, http.StatusOK, out)
}

// Create handles POST /api/courses.
func (h *Courses) Create(w http.ResponseWriter, r *http.Request) {
	var req dto.CreateCourseRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	if req.Name == "" {
		writeError(w, http.StatusBadRequest, "name is required")
		return
	}
	if req.DayOfWeek < 1 || req.DayOfWeek > 7 {
		writeError(w, http.StatusBadRequest, "day_of_week must be between 1 and 7")
		return
	}
	startTime, err := dto.ParseClock(&req.StartTime)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	endTime, err := dto.ParseClock(&req.EndTime)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	semesterStartDate, err := dto.ParseDate(req.SemesterStartDate)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	semester := strOr(req.Semester, "")
	var created store.Course
	err = h.Academic.Tx(r.Context(), h.owner(r), []string{semester}, academic.TxOptions{BumpTimetable: true}, func(q *store.Queries) (bool, error) {
		c, err := q.CreateCourse(r.Context(), store.CreateCourseParams{
			SyncID:            "",
			Name:              req.Name,
			DayOfWeek:         req.DayOfWeek,
			StartTime:         startTime,
			EndTime:           endTime,
			WeekPattern:       strOr(req.WeekPattern, ""),
			SemesterStartDate: semesterStartDate,
			Location:          strOr(req.Location, ""),
			Teacher:           strOr(req.Teacher, ""),
			Color:             strOr(req.Color, "#7C8CC0"),
			Semester:          semester,
		})
		if err != nil {
			return false, err
		}
		created = c
		return true, nil
	})
	if err != nil {
		h.writeMutationError(w, err)
		return
	}
	h.recomputeCourses(r.Context())
	writeJSON(w, http.StatusCreated, dto.FromCourse(created))
}

// Update handles PATCH /api/courses/{id}.
func (h *Courses) Update(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	var req dto.UpdateCourseRequest
	if !decodeJSON(w, r, &req) {
		return
	}

	probe, err := h.Q.GetCourse(r.Context(), id)
	if err != nil {
		writeStoreError(w, err)
		return
	}
	newSemester := probe.Semester
	if req.Semester != nil {
		newSemester = *req.Semester
	}

	var updated store.Course
	err = h.Academic.Tx(r.Context(), h.owner(r), []string{probe.Semester, newSemester}, academic.TxOptions{BumpTimetable: true}, func(q *store.Queries) (bool, error) {
		existing, err := q.GetCourse(r.Context(), id)
		if err != nil {
			return false, err
		}
		if existing.Semester != probe.Semester {
			return false, errSemesterMoved
		}
		if src, linked, err := lookupSource(r.Context(), q, id); err != nil {
			return false, err
		} else if linked {
			c, changed, err := h.applySourcePatch(r.Context(), h.owner(r), q, src, existing, req)
			if err != nil {
				return false, err
			}
			updated = c
			return changed, nil
		}
		c, changed, err := applyLegacyPatch(r.Context(), q, existing, req)
		if err != nil {
			return false, err
		}
		updated = c
		return changed, nil
	})
	if err != nil {
		h.writeMutationError(w, err)
		return
	}
	h.recomputeCourses(r.Context())
	writeJSON(w, http.StatusOK, dto.FromCourse(updated))
}

// Delete handles DELETE /api/courses/{id} (soft delete; source rows hidden).
func (h *Courses) Delete(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	probe, err := h.Q.GetCourse(r.Context(), id)
	if err != nil {
		writeStoreError(w, err)
		return
	}
	err = h.Academic.Tx(r.Context(), h.owner(r), []string{probe.Semester}, academic.TxOptions{BumpTimetable: true}, func(q *store.Queries) (bool, error) {
		src, linked, err := lookupSource(r.Context(), q, id)
		if err != nil {
			return false, err
		}
		if linked {
			if src.Hidden != nil && *src.Hidden {
				return false, nil // already hidden
			}
			hidden := true
			if err := q.UpsertSessionOverride(r.Context(), src.SessionID, withField(src.SetFields, "hidden"),
				src.Name, src.Teacher, src.Location, src.Color, &hidden); err != nil {
				return false, err
			}
			updated, err := q.GetSourceSessionByID(r.Context(), src.SessionID)
			if err != nil {
				return false, err
			}
			if _, err := academic.ProjectSession(r.Context(), q, h.owner(r), updated); err != nil {
				return false, err
			}
			return true, nil
		}
		if err := q.SoftDeleteCourse(r.Context(), id); err != nil {
			return false, err
		}
		return true, nil
	})
	if err != nil {
		h.writeMutationError(w, err)
		return
	}
	h.recomputeCourses(r.Context())
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

// ImportText handles POST /api/courses/import-text.
func (h *Courses) ImportText(w http.ResponseWriter, r *http.Request) {
	var req dto.ImportCoursesRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	candidates, err := importer.ParseCourseImportText(req.Text, req.Semester, req.SemesterStartDate)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	var result importer.ImportTextResult
	err = h.Academic.Tx(r.Context(), h.owner(r), []string{req.Semester}, academic.TxOptions{BumpTimetable: true}, func(q *store.Queries) (bool, error) {
		res, err := importer.ImportNewCourses(r.Context(), q, candidates, req.Semester)
		if err != nil {
			return false, err
		}
		result = res
		return res.Imported > 0, nil
	})
	if err != nil {
		h.writeMutationError(w, err)
		return
	}
	h.recomputeCourses(r.Context())
	writeJSON(w, http.StatusOK, dto.ImportTextResult{
		Parsed:   result.Parsed,
		Imported: result.Imported,
		Skipped:  result.Skipped,
		Message:  result.Message,
	})
}

// ResetSemesterDates handles POST /api/courses/reset-semester-dates.
func (h *Courses) ResetSemesterDates(w http.ResponseWriter, r *http.Request) {
	var req dto.ResetSemesterDatesRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	date, err := dto.ParseDate(&req.Date)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	semesters, err := h.Q.LegacyCourseSemesters(r.Context())
	if err != nil {
		h.Log.Error("list semesters", "error", err)
		writeError(w, http.StatusInternalServerError, "internal server error")
		return
	}
	var updated int64
	err = h.Academic.Tx(r.Context(), h.owner(r), semesters, academic.TxOptions{BumpTimetable: true}, func(q *store.Queries) (bool, error) {
		linked, err := q.CountAcademicSourceSessions(r.Context())
		if err != nil {
			return false, err
		}
		if linked > 0 {
			return false, &academicErr{status: http.StatusConflict, code: "SOURCE_STRUCTURE_EDIT_UNSUPPORTED",
				message: "已有来源课，不能整体重置学期锚点"}
		}
		n, err := q.UpdateAllCoursesSemesterStartDate(r.Context(), date)
		if err != nil {
			return false, err
		}
		if _, err := q.UpdateAllSemesterContextsStartDate(r.Context(), date); err != nil {
			return false, err
		}
		updated = n
		return n > 0, nil
	})
	if err != nil {
		h.writeMutationError(w, err)
		return
	}
	h.recomputeCourses(r.Context())
	writeJSON(w, http.StatusOK, dto.ResetSemesterDatesResponse{Updated: updated})
}

func (h *Courses) recomputeCourses(ctx context.Context) {
	if h.Scheduler != nil {
		h.Scheduler.RecomputeCourses(ctx)
	}
}

// writeMutationError maps academic validation errors to the HTTP envelope.
func (h *Courses) writeMutationError(w http.ResponseWriter, err error) {
	h.Log.Error("course mutation failed", "error", err)
	var ae *academicErr
	if errors.As(err, &ae) {
		writeErrorCode(w, ae.status, ae.message, ae.code, ae.details)
		return
	}
	var ve *academic.ValidationError
	if errors.As(err, &ve) {
		status := http.StatusConflict
		if ve.Code == "INVALID_ARGUMENT" {
			status = http.StatusBadRequest
		}
		writeErrorCode(w, status, ve.Message, ve.Code, ve.Details)
		return
	}
	if errors.Is(err, errSemesterMoved) {
		writeErrorCode(w, http.StatusConflict, "课程所属学期已变化，请重试", "REVISION_CONFLICT", nil)
		return
	}
	if errors.Is(err, academic.ErrUnsupportedDatedSession) {
		writeErrorCode(w, http.StatusUnprocessableEntity, "该安排含真实日期，旧课程模型无法表达", "UNSUPPORTED_DATED_SESSION", nil)
		return
	}
	writeStoreError(w, err)
}

type academicErr struct {
	status  int
	code    string
	message string
	details any
}

func (e *academicErr) Error() string { return e.message }

var errSemesterMoved = errors.New("course semester moved during update")

func lookupSource(ctx context.Context, q *store.Queries, courseID int64) (store.SourceSessionRow, bool, error) {
	row, err := q.GetSourceSessionByCourseID(ctx, courseID)
	if errors.Is(err, pgx.ErrNoRows) {
		return store.SourceSessionRow{}, false, nil
	}
	if err != nil {
		return store.SourceSessionRow{}, false, err
	}
	return row, true, nil
}

func withField(fields []string, field string) []string {
	for _, f := range fields {
		if f == field {
			return fields
		}
	}
	return append(append([]string{}, fields...), field)
}

func strOr(p *string, fallback string) string {
	if p == nil {
		return fallback
	}
	return *p
}
