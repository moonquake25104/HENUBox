package dto

import (
	"kairos/server/internal/store"
)

// Exam mirrors the frontend src/types/exam.ts Exam entity.
type Exam struct {
	ID              int64   `json:"id"`
	SyncID          string  `json:"sync_id"`
	CourseName      string  `json:"course_name"`
	ExamDatetime    string  `json:"exam_datetime"`
	ExamEndDatetime string  `json:"exam_end_datetime"`
	Location        string  `json:"location"`
	Notes           string  `json:"notes"`
	CourseID        *int64  `json:"course_id"`
	Semester        string  `json:"semester"`
	CreatedAt       string  `json:"created_at"`
	UpdatedAt       string  `json:"updated_at"`
	DeletedAt       *string `json:"deleted_at"`
}

// CreateExamRequest is the body of POST /api/exams.
type CreateExamRequest struct {
	CourseName      string  `json:"course_name"`
	ExamDatetime    string  `json:"exam_datetime"`
	ExamEndDatetime *string `json:"exam_end_datetime"`
	Location        *string `json:"location"`
	Notes           *string `json:"notes"`
	CourseID        *int64  `json:"course_id"`
	Semester        *string `json:"semester"`
}

// UpdateExamRequest is the body of PATCH /api/exams/{id}; absent fields keep
// the current value (matching the Rust update_exam command semantics).
type UpdateExamRequest struct {
	CourseName      *string `json:"course_name"`
	ExamDatetime    *string `json:"exam_datetime"`
	ExamEndDatetime *string `json:"exam_end_datetime"`
	Location        *string `json:"location"`
	Notes           *string `json:"notes"`
	CourseID        *int64  `json:"course_id"`
	Semester        *string `json:"semester"`
}

// ImportExamsRequest is the body of POST /api/exams/import-text.
type ImportExamsRequest struct {
	Text     string `json:"text"`
	Semester string `json:"semester"`
}

// FromExam converts a store exam into the API representation.
func FromExam(e store.Exam) Exam {
	return Exam{
		ID:              e.ID,
		SyncID:          store.TextString(e.SyncID),
		CourseName:      e.CourseName,
		ExamDatetime:    store.TSString(e.ExamDatetime),
		ExamEndDatetime: store.TSString(e.ExamEndDatetime),
		Location:        e.Location,
		Notes:           e.Notes,
		CourseID:        Int8Ptr(e.CourseID),
		Semester:        e.Semester,
		CreatedAt:       store.TSString(e.CreatedAt),
		UpdatedAt:       store.TSString(e.UpdatedAt),
		DeletedAt:       tsPtr(e.DeletedAt),
	}
}
