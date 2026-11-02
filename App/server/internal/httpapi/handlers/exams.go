package handlers

import (
	"log/slog"
	"net/http"

	"github.com/jackc/pgx/v5/pgtype"

	"kairos/server/internal/domain/importer"
	"kairos/server/internal/httpapi/dto"
	"kairos/server/internal/store"
)

// Exams implements the /api/exams endpoints.
type Exams struct {
	Q         *store.Queries
	Log       *slog.Logger
	Scheduler NotifyRecomputer
}

// List handles GET /api/exams.
func (h *Exams) List(w http.ResponseWriter, r *http.Request) {
	exams, err := h.Q.ListExams(r.Context())
	if err != nil {
		h.Log.Error("list exams", "error", err)
		writeError(w, http.StatusInternalServerError, "internal server error")
		return
	}
	out := make([]dto.Exam, 0, len(exams))
	for _, e := range exams {
		out = append(out, dto.FromExam(e))
	}
	writeJSON(w, http.StatusOK, out)
}

// Create handles POST /api/exams.
func (h *Exams) Create(w http.ResponseWriter, r *http.Request) {
	var req dto.CreateExamRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	if req.CourseName == "" {
		writeError(w, http.StatusBadRequest, "course_name is required")
		return
	}
	examDatetime, err := dto.ParseTS(&req.ExamDatetime)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if !examDatetime.Valid {
		writeError(w, http.StatusBadRequest, "exam_datetime is required")
		return
	}
	examEndDatetime, err := dto.ParseTS(req.ExamEndDatetime)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	exam, err := h.Q.CreateExam(r.Context(), store.CreateExamParams{
		SyncID:          "",
		CourseName:      req.CourseName,
		ExamDatetime:    examDatetime,
		ExamEndDatetime: examEndDatetime,
		Location:        strOr(req.Location, ""),
		Notes:           strOr(req.Notes, ""),
		CourseID:        int8Or(req.CourseID),
		Semester:        strOr(req.Semester, ""),
	})
	if err != nil {
		h.Log.Error("create exam", "error", err)
		writeError(w, http.StatusInternalServerError, "internal server error")
		return
	}
	if h.Scheduler != nil {
		h.Scheduler.RecomputeExams(r.Context())
	}
	writeJSON(w, http.StatusCreated, dto.FromExam(exam))
}

// Update handles PATCH /api/exams/{id}.
func (h *Exams) Update(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	var req dto.UpdateExamRequest
	if !decodeJSON(w, r, &req) {
		return
	}

	existing, err := h.Q.GetExam(r.Context(), id)
	if err != nil {
		writeStoreError(w, err)
		return
	}

	courseName := existing.CourseName
	if req.CourseName != nil {
		courseName = *req.CourseName
	}
	if courseName == "" {
		writeError(w, http.StatusBadRequest, "course_name is required")
		return
	}
	examDatetime := existing.ExamDatetime
	if req.ExamDatetime != nil {
		parsed, err := dto.ParseTS(req.ExamDatetime)
		if err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		if !parsed.Valid {
			writeError(w, http.StatusBadRequest, "exam_datetime is required")
			return
		}
		examDatetime = parsed
	}
	examEndDatetime := existing.ExamEndDatetime
	if req.ExamEndDatetime != nil {
		parsed, err := dto.ParseTS(req.ExamEndDatetime)
		if err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		examEndDatetime = parsed
	}
	courseID := existing.CourseID
	if req.CourseID != nil {
		courseID = store.Int8Of(*req.CourseID)
	}

	updated, err := h.Q.UpdateExam(r.Context(), store.UpdateExamParams{
		CourseName:      courseName,
		ExamDatetime:    examDatetime,
		ExamEndDatetime: examEndDatetime,
		Location:        strOr(req.Location, existing.Location),
		Notes:           strOr(req.Notes, existing.Notes),
		CourseID:        courseID,
		Semester:        strOr(req.Semester, existing.Semester),
		ID:              id,
	})
	if err != nil {
		writeStoreError(w, err)
		return
	}
	if h.Scheduler != nil {
		h.Scheduler.RecomputeExams(r.Context())
	}
	writeJSON(w, http.StatusOK, dto.FromExam(updated))
}

// Delete handles DELETE /api/exams/{id} (soft delete).
func (h *Exams) Delete(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	if err := h.Q.SoftDeleteExam(r.Context(), id); err != nil {
		writeStoreError(w, err)
		return
	}
	if h.Scheduler != nil {
		h.Scheduler.RecomputeExams(r.Context())
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

// ImportText handles POST /api/exams/import-text.
func (h *Exams) ImportText(w http.ResponseWriter, r *http.Request) {
	var req dto.ImportExamsRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	candidates, err := importer.ParseExamImportText(req.Text, req.Semester)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	result, err := importer.ImportNewExams(r.Context(), h.Q, candidates)
	if err != nil {
		h.Log.Error("import exams", "error", err)
		writeError(w, http.StatusInternalServerError, "internal server error")
		return
	}
	if h.Scheduler != nil {
		h.Scheduler.RecomputeExams(r.Context())
	}
	writeJSON(w, http.StatusOK, dto.ImportTextResult{
		Parsed:   result.Parsed,
		Imported: result.Imported,
		Skipped:  result.Skipped,
		Message:  result.Message,
	})
}

func int8Or(p *int64) pgtype.Int8 {
	if p == nil {
		return pgtype.Int8{}
	}
	return store.Int8Of(*p)
}
