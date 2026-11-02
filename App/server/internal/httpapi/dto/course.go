package dto

import (
	"kairos/server/internal/store"
)

// Course mirrors the frontend src/types/course.ts Course entity.
type Course struct {
	ID                int64   `json:"id"`
	SyncID            string  `json:"sync_id"`
	Name              string  `json:"name"`
	DayOfWeek         int32   `json:"day_of_week"`
	StartTime         string  `json:"start_time"`
	EndTime           string  `json:"end_time"`
	WeekPattern       string  `json:"week_pattern"`
	SemesterStartDate string  `json:"semester_start_date"`
	Location          string  `json:"location"`
	Teacher           string  `json:"teacher"`
	Color             string  `json:"color"`
	Semester          string  `json:"semester"`
	CreatedAt         string  `json:"created_at"`
	UpdatedAt         string  `json:"updated_at"`
	DeletedAt         *string `json:"deleted_at"`
}

// CreateCourseRequest is the body of POST /api/courses.
type CreateCourseRequest struct {
	Name              string  `json:"name"`
	DayOfWeek         int32   `json:"day_of_week"`
	StartTime         string  `json:"start_time"`
	EndTime           string  `json:"end_time"`
	WeekPattern       *string `json:"week_pattern"`
	SemesterStartDate *string `json:"semester_start_date"`
	Location          *string `json:"location"`
	Teacher           *string `json:"teacher"`
	Color             *string `json:"color"`
	Semester          *string `json:"semester"`
}

// UpdateCourseRequest is the body of PATCH /api/courses/{id}; absent fields
// keep the current value (matching the Rust update_course command semantics).
type UpdateCourseRequest struct {
	Name              *string `json:"name"`
	DayOfWeek         *int32  `json:"day_of_week"`
	StartTime         *string `json:"start_time"`
	EndTime           *string `json:"end_time"`
	WeekPattern       *string `json:"week_pattern"`
	SemesterStartDate *string `json:"semester_start_date"`
	Location          *string `json:"location"`
	Teacher           *string `json:"teacher"`
	Color             *string `json:"color"`
	Semester          *string `json:"semester"`
}

// ImportCoursesRequest is the body of POST /api/courses/import-text.
type ImportCoursesRequest struct {
	Text              string `json:"text"`
	Semester          string `json:"semester"`
	SemesterStartDate string `json:"semester_start_date"`
}

// ResetSemesterDatesRequest is the body of POST /api/courses/reset-semester-dates.
type ResetSemesterDatesRequest struct {
	Date string `json:"date"`
}

// ResetSemesterDatesResponse reports how many courses were updated.
type ResetSemesterDatesResponse struct {
	Updated int64 `json:"updated"`
}

// ImportTextResult mirrors the frontend src/types/course-import.ts
// ImportTextResult returned by the text-import endpoints.
type ImportTextResult struct {
	Parsed   int    `json:"parsed"`
	Imported int    `json:"imported"`
	Skipped  int    `json:"skipped"`
	Message  string `json:"message"`
}

// FromCourse converts a store course into the API representation.
func FromCourse(c store.Course) Course {
	return Course{
		ID:                c.ID,
		SyncID:            store.TextString(c.SyncID),
		Name:              c.Name,
		DayOfWeek:         c.DayOfWeek,
		StartTime:         store.ClockString(c.StartTime),
		EndTime:           store.ClockString(c.EndTime),
		WeekPattern:       c.WeekPattern,
		SemesterStartDate: store.DateString(c.SemesterStartDate),
		Location:          c.Location,
		Teacher:           c.Teacher,
		Color:             c.Color,
		Semester:          c.Semester,
		CreatedAt:         store.TSString(c.CreatedAt),
		UpdatedAt:         store.TSString(c.UpdatedAt),
		DeletedAt:         tsPtr(c.DeletedAt),
	}
}
