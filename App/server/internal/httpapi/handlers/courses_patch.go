package handlers

import (
	"context"
	"net/http"

	"kairos/server/internal/domain/academic"
	"kairos/server/internal/httpapi/dto"
	"kairos/server/internal/store"
)

// applyLegacyPatch keeps the original behaviour for manual courses: absent
// fields stay unchanged, present fields are written as before.
func applyLegacyPatch(ctx context.Context, q *store.Queries, existing store.Course, req dto.UpdateCourseRequest) (store.Course, bool, error) {
	name := existing.Name
	if req.Name != nil {
		name = *req.Name
	}
	if name == "" {
		return store.Course{}, false, &academicErr{status: http.StatusBadRequest, code: "INVALID_ARGUMENT", message: "name is required"}
	}
	dayOfWeek := existing.DayOfWeek
	if req.DayOfWeek != nil {
		dayOfWeek = *req.DayOfWeek
	}
	if dayOfWeek < 1 || dayOfWeek > 7 {
		return store.Course{}, false, &academicErr{status: http.StatusBadRequest, code: "INVALID_ARGUMENT", message: "day_of_week must be between 1 and 7"}
	}
	startTime := existing.StartTime
	if req.StartTime != nil {
		parsed, err := dto.ParseClock(req.StartTime)
		if err != nil {
			return store.Course{}, false, &academicErr{status: http.StatusBadRequest, code: "INVALID_ARGUMENT", message: err.Error()}
		}
		startTime = parsed
	}
	endTime := existing.EndTime
	if req.EndTime != nil {
		parsed, err := dto.ParseClock(req.EndTime)
		if err != nil {
			return store.Course{}, false, &academicErr{status: http.StatusBadRequest, code: "INVALID_ARGUMENT", message: err.Error()}
		}
		endTime = parsed
	}
	semesterStartDate := existing.SemesterStartDate
	if req.SemesterStartDate != nil {
		parsed, err := dto.ParseDate(req.SemesterStartDate)
		if err != nil {
			return store.Course{}, false, &academicErr{status: http.StatusBadRequest, code: "INVALID_ARGUMENT", message: err.Error()}
		}
		semesterStartDate = parsed
	}

	update := store.UpdateCourseParams{
		Name:              name,
		DayOfWeek:         dayOfWeek,
		StartTime:         startTime,
		EndTime:           endTime,
		WeekPattern:       strOr(req.WeekPattern, existing.WeekPattern),
		SemesterStartDate: semesterStartDate,
		Location:          strOr(req.Location, existing.Location),
		Teacher:           strOr(req.Teacher, existing.Teacher),
		Color:             strOr(req.Color, existing.Color),
		Semester:          strOr(req.Semester, existing.Semester),
		ID:                existing.ID,
	}
	changed := update.Name != existing.Name ||
		update.DayOfWeek != existing.DayOfWeek ||
		store.ClockString(update.StartTime) != store.ClockString(existing.StartTime) ||
		store.ClockString(update.EndTime) != store.ClockString(existing.EndTime) ||
		update.WeekPattern != existing.WeekPattern ||
		store.DateString(update.SemesterStartDate) != store.DateString(existing.SemesterStartDate) ||
		update.Location != existing.Location ||
		update.Teacher != existing.Teacher ||
		update.Color != existing.Color ||
		update.Semester != existing.Semester
	if !changed {
		return existing, false, nil
	}
	updated, err := q.UpdateCourse(ctx, update)
	if err != nil {
		return store.Course{}, false, err
	}
	return updated, true, nil
}

// applySourcePatch writes personal overrides for a source-bound course, then
// reprojects the legacy row. Structural fields cannot be edited.
func (h *Courses) applySourcePatch(ctx context.Context, owner string, q *store.Queries, src store.SourceSessionRow, existing store.Course, req dto.UpdateCourseRequest) (store.Course, bool, error) {
	source, err := academic.SourceValuesFromJSON(src.SourceValues)
	if err != nil {
		return store.Course{}, false, err
	}

	if req.DayOfWeek != nil && *req.DayOfWeek != int32(source.Weekday) {
		return store.Course{}, false, structuralEdit("day_of_week")
	}
	if req.StartTime != nil {
		parsed, err := dto.ParseClock(req.StartTime)
		if err != nil {
			return store.Course{}, false, &academicErr{status: http.StatusBadRequest, code: "INVALID_ARGUMENT", message: err.Error()}
		}
		if store.ClockString(parsed) != store.ClockString(existing.StartTime) {
			return store.Course{}, false, structuralEdit("start_time")
		}
	}
	if req.EndTime != nil {
		parsed, err := dto.ParseClock(req.EndTime)
		if err != nil {
			return store.Course{}, false, &academicErr{status: http.StatusBadRequest, code: "INVALID_ARGUMENT", message: err.Error()}
		}
		if store.ClockString(parsed) != store.ClockString(existing.EndTime) {
			return store.Course{}, false, structuralEdit("end_time")
		}
	}
	if req.WeekPattern != nil && *req.WeekPattern != existing.WeekPattern {
		return store.Course{}, false, structuralEdit("week_pattern")
	}
	if req.SemesterStartDate != nil && *req.SemesterStartDate != store.DateString(existing.SemesterStartDate) {
		return store.Course{}, false, structuralEdit("semester_start_date")
	}
	if req.Semester != nil && *req.Semester != existing.Semester {
		return store.Course{}, false, structuralEdit("semester")
	}
	if req.Name != nil && *req.Name == "" {
		return store.Course{}, false, &academicErr{status: http.StatusBadRequest, code: "INVALID_ARGUMENT", message: "name is required"}
	}

	ov := academic.OverridesFromRow(src.SetFields, src.Name, src.Teacher, src.Location, src.Color, src.Hidden)
	changed := false

	if req.Name != nil {
		if *req.Name == source.Name {
			ov.SetFields = dropField(ov.SetFields, "name")
			ov.Name = nil
		} else {
			ov.SetFields = withField(ov.SetFields, "name")
			ov.Name = req.Name
		}
		changed = true
	}
	setOptional := func(field string, provided *string, sourceValue *string, target **string) {
		if provided == nil {
			return
		}
		if sameOptional(provided, sourceValue) {
			ov.SetFields = dropField(ov.SetFields, field)
			*target = nil
		} else {
			ov.SetFields = withField(ov.SetFields, field)
			*target = provided
		}
		changed = true
	}
	setOptional("teacher", req.Teacher, source.Teacher, &ov.Teacher)
	setOptional("location", req.Location, source.Location, &ov.Location)
	setOptional("color", req.Color, source.Color, &ov.Color)

	if !changed {
		return existing, false, nil
	}
	if err := q.UpsertSessionOverride(ctx, src.SessionID, ov.SetFields, ov.Name, ov.Teacher, ov.Location, ov.Color, ov.Hidden); err != nil {
		return store.Course{}, false, err
	}
	updatedSrc, err := q.GetSourceSessionByID(ctx, src.SessionID)
	if err != nil {
		return store.Course{}, false, err
	}
	if _, err := academic.ProjectSession(ctx, q, owner, updatedSrc); err != nil {
		return store.Course{}, false, err
	}
	course, err := q.GetCourse(ctx, existing.ID)
	if err != nil {
		return store.Course{}, false, err
	}
	return course, true, nil
}

func structuralEdit(field string) error {
	return &academicErr{
		status:  http.StatusConflict,
		code:    "SOURCE_STRUCTURE_EDIT_UNSUPPORTED",
		message: "来源课程的结构字段不能个人修改：" + field,
		details: map[string]any{"field": field},
	}
}

func sameOptional(a, b *string) bool {
	av, bv := "", ""
	if a != nil {
		av = *a
	}
	if b != nil {
		bv = *b
	}
	return av == bv
}

func hasField(fields []string, f string) bool {
	for _, x := range fields {
		if x == f {
			return true
		}
	}
	return false
}

func dropField(fields []string, f string) []string {
	out := make([]string, 0, len(fields))
	for _, x := range fields {
		if x != f {
			out = append(out, x)
		}
	}
	return out
}
