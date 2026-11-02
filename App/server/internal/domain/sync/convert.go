package sync

import (
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	"kairos/server/internal/store"
)

// ─── export: store rows → snapshot DTOs ────────────────────────────────────

func tasksFromStore(rows []store.Task) []Task {
	out := make([]Task, 0, len(rows))
	for _, t := range rows {
		out = append(out, Task{
			ID:                t.ID,
			SyncID:            store.TextString(t.SyncID),
			Title:             t.Title,
			Description:       t.Description,
			Status:            t.Status,
			Priority:          t.Priority,
			DueDate:           dateStringPtr(t.DueDate),
			Tags:              string(t.Tags),
			CreatedAt:         store.TSString(t.CreatedAt),
			UpdatedAt:         store.TSString(t.UpdatedAt),
			IsDaily:           t.IsDaily,
			LastCompletedDate: dateStringPtr(t.LastCompletedDate),
			ReminderTime:      clockStringPtr(t.ReminderTime),
			RemindAt:          remindAtStringPtr(t.RemindAt),
			DeletedAt:         tsStringPtr(t.DeletedAt),
		})
	}
	return out
}

func coursesFromStore(rows []store.Course) []Course {
	out := make([]Course, 0, len(rows))
	for _, c := range rows {
		out = append(out, Course{
			ID:                c.ID,
			SyncID:            store.TextString(c.SyncID),
			Name:              c.Name,
			DayOfWeek:         int64(c.DayOfWeek),
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
			DeletedAt:         tsStringPtr(c.DeletedAt),
		})
	}
	return out
}

func examsFromStore(rows []store.Exam) []Exam {
	out := make([]Exam, 0, len(rows))
	for _, e := range rows {
		out = append(out, Exam{
			ID:              e.ID,
			SyncID:          store.TextString(e.SyncID),
			CourseName:      e.CourseName,
			ExamDatetime:    store.TSString(e.ExamDatetime),
			ExamEndDatetime: examEndString(e.ExamEndDatetime),
			Location:        e.Location,
			Notes:           e.Notes,
			CourseID:        int8Ptr(e.CourseID),
			Semester:        e.Semester,
			CreatedAt:       store.TSString(e.CreatedAt),
			UpdatedAt:       store.TSString(e.UpdatedAt),
			DeletedAt:       tsStringPtr(e.DeletedAt),
		})
	}
	return out
}

func sessionsFromStore(rows []store.PomodoroSession) []PomodoroSession {
	out := make([]PomodoroSession, 0, len(rows))
	for _, s := range rows {
		out = append(out, PomodoroSession{
			ID:          s.ID,
			SyncID:      store.TextString(s.SyncID),
			StartedAt:   store.TSString(s.StartedAt),
			EndedAt:     tsStringPtr(s.EndedAt),
			SessionType: s.SessionType,
			TaskID:      int8Ptr(s.TaskID),
			DeletedAt:   tsStringPtr(s.DeletedAt),
		})
	}
	return out
}

func termPhasesFromStore(rows []store.TermPhase) []TermPhase {
	out := make([]TermPhase, 0, len(rows))
	for _, p := range rows {
		out = append(out, TermPhase{
			ID:                       p.ID,
			SyncID:                   store.TextString(p.SyncID),
			TermLabel:                p.TermLabel,
			PhaseType:                p.PhaseType,
			StartWeek:                int64(p.StartWeek),
			EndWeek:                  int64(p.EndWeek),
			AffectsCourses:           p.AffectsCourses,
			AffectsExamNotifications: p.AffectsExamNotifications,
			PomodoroProfile:          p.PomodoroProfile,
			NotificationRulesJSON:    string(p.NotificationRules),
			SortOrder:                int64(p.SortOrder),
			DeletedAt:                tsStringPtr(p.DeletedAt),
			CreatedAt:                store.TSString(p.CreatedAt),
			UpdatedAt:                store.TSString(p.UpdatedAt),
		})
	}
	return out
}

// ─── import: snapshot DTOs → store params ──────────────────────────────────

func taskInsertParams(t Task) store.SyncInsertTaskParams {
	return store.SyncInsertTaskParams{
		SyncID:            store.TextOf(t.SyncID),
		Title:             t.Title,
		Description:       t.Description,
		Status:            t.Status,
		Priority:          t.Priority,
		DueDate:           dateOfPtr(t.DueDate),
		Tags:              []byte(t.Tags),
		CreatedAt:         tsOf(t.CreatedAt),
		UpdatedAt:         tsOf(t.UpdatedAt),
		IsDaily:           t.IsDaily,
		LastCompletedDate: dateOfPtr(t.LastCompletedDate),
		ReminderTime:      clockOfPtr(t.ReminderTime),
		RemindAt:          remindAtOfPtr(t.RemindAt),
		DeletedAt:         tsOfPtr(t.DeletedAt),
	}
}

func taskUpdateParams(t Task, id int64) store.SyncUpdateTaskParams {
	p := taskInsertParams(t)
	return store.SyncUpdateTaskParams{
		SyncID:            p.SyncID,
		Title:             p.Title,
		Description:       p.Description,
		Status:            p.Status,
		Priority:          p.Priority,
		DueDate:           p.DueDate,
		Tags:              p.Tags,
		CreatedAt:         p.CreatedAt,
		UpdatedAt:         p.UpdatedAt,
		IsDaily:           p.IsDaily,
		LastCompletedDate: p.LastCompletedDate,
		ReminderTime:      p.ReminderTime,
		RemindAt:          p.RemindAt,
		DeletedAt:         p.DeletedAt,
		ID:                id,
	}
}

func courseInsertParams(c Course) store.SyncInsertCourseParams {
	return store.SyncInsertCourseParams{
		SyncID:            store.TextOf(c.SyncID),
		Name:              c.Name,
		DayOfWeek:         int32(c.DayOfWeek),
		StartTime:         clockOf(c.StartTime),
		EndTime:           clockOf(c.EndTime),
		WeekPattern:       c.WeekPattern,
		SemesterStartDate: dateOf(c.SemesterStartDate),
		Location:          c.Location,
		Teacher:           c.Teacher,
		Color:             c.Color,
		Semester:          c.Semester,
		CreatedAt:         tsOf(c.CreatedAt),
		UpdatedAt:         tsOf(c.UpdatedAt),
		DeletedAt:         tsOfPtr(c.DeletedAt),
	}
}

func courseUpdateParams(c Course, id int64) store.SyncUpdateCourseParams {
	p := courseInsertParams(c)
	return store.SyncUpdateCourseParams{
		SyncID:            p.SyncID,
		Name:              p.Name,
		DayOfWeek:         p.DayOfWeek,
		StartTime:         p.StartTime,
		EndTime:           p.EndTime,
		WeekPattern:       p.WeekPattern,
		SemesterStartDate: p.SemesterStartDate,
		Location:          p.Location,
		Teacher:           p.Teacher,
		Color:             p.Color,
		Semester:          p.Semester,
		CreatedAt:         p.CreatedAt,
		UpdatedAt:         p.UpdatedAt,
		DeletedAt:         p.DeletedAt,
		ID:                id,
	}
}

func examInsertParams(e Exam, courseID *int64) store.SyncInsertExamParams {
	return store.SyncInsertExamParams{
		SyncID:          store.TextOf(e.SyncID),
		CourseName:      e.CourseName,
		ExamDatetime:    tsOf(e.ExamDatetime),
		ExamEndDatetime: examEndOf(e.ExamEndDatetime),
		Location:        e.Location,
		Notes:           e.Notes,
		CourseID:        int8OfPtr(courseID),
		Semester:        e.Semester,
		CreatedAt:       tsOf(e.CreatedAt),
		UpdatedAt:       tsOf(e.UpdatedAt),
		DeletedAt:       tsOfPtr(e.DeletedAt),
	}
}

func examUpdateParams(e Exam, courseID *int64, id int64) store.SyncUpdateExamParams {
	p := examInsertParams(e, courseID)
	return store.SyncUpdateExamParams{
		SyncID:          p.SyncID,
		CourseName:      p.CourseName,
		ExamDatetime:    p.ExamDatetime,
		ExamEndDatetime: p.ExamEndDatetime,
		Location:        p.Location,
		Notes:           p.Notes,
		CourseID:        p.CourseID,
		Semester:        p.Semester,
		CreatedAt:       p.CreatedAt,
		UpdatedAt:       p.UpdatedAt,
		DeletedAt:       p.DeletedAt,
		ID:              id,
	}
}

func sessionInsertParams(s PomodoroSession, taskID *int64) store.SyncInsertSessionParams {
	return store.SyncInsertSessionParams{
		SyncID:      store.TextOf(s.SyncID),
		StartedAt:   tsOf(s.StartedAt),
		EndedAt:     tsOfPtr(s.EndedAt),
		SessionType: s.SessionType,
		TaskID:      int8OfPtr(taskID),
		DeletedAt:   tsOfPtr(s.DeletedAt),
	}
}

func sessionUpdateParams(s PomodoroSession, taskID *int64, id int64) store.SyncUpdateSessionParams {
	p := sessionInsertParams(s, taskID)
	return store.SyncUpdateSessionParams{
		SyncID:      p.SyncID,
		StartedAt:   p.StartedAt,
		EndedAt:     p.EndedAt,
		SessionType: p.SessionType,
		TaskID:      p.TaskID,
		DeletedAt:   p.DeletedAt,
		ID:          id,
	}
}

func termPhaseInsertParams(p TermPhase) store.SyncInsertTermPhaseParams {
	return store.SyncInsertTermPhaseParams{
		SyncID:                   store.TextOf(p.SyncID),
		TermLabel:                p.TermLabel,
		PhaseType:                p.PhaseType,
		StartWeek:                int32(p.StartWeek),
		EndWeek:                  int32(p.EndWeek),
		AffectsCourses:           p.AffectsCourses,
		AffectsExamNotifications: p.AffectsExamNotifications,
		PomodoroProfile:          p.PomodoroProfile,
		NotificationRules:        []byte(p.NotificationRulesJSON),
		SortOrder:                int32(p.SortOrder),
		DeletedAt:                tsOfPtr(p.DeletedAt),
		CreatedAt:                tsOf(p.CreatedAt),
		UpdatedAt:                tsOf(p.UpdatedAt),
	}
}

func termPhaseUpdateParams(p TermPhase, id int64) store.SyncUpdateTermPhaseParams {
	ins := termPhaseInsertParams(p)
	return store.SyncUpdateTermPhaseParams{
		TermLabel:                ins.TermLabel,
		PhaseType:                ins.PhaseType,
		StartWeek:                ins.StartWeek,
		EndWeek:                  ins.EndWeek,
		AffectsCourses:           ins.AffectsCourses,
		AffectsExamNotifications: ins.AffectsExamNotifications,
		PomodoroProfile:          ins.PomodoroProfile,
		NotificationRules:        ins.NotificationRules,
		SortOrder:                ins.SortOrder,
		DeletedAt:                ins.DeletedAt,
		CreatedAt:                ins.CreatedAt,
		UpdatedAt:                ins.UpdatedAt,
		ID:                       id,
	}
}

// ─── scalar conversions ─────────────────────────────────────────────────────

func dateStringPtr(d pgtype.Date) *string {
	if !d.Valid {
		return nil
	}
	s := store.DateString(d)
	return &s
}

func clockStringPtr(t pgtype.Time) *string {
	if !t.Valid {
		return nil
	}
	s := store.ClockString(t)
	return &s
}

func remindAtStringPtr(t pgtype.Timestamptz) *string {
	if !t.Valid {
		return nil
	}
	s := t.Time.In(ChinaTZ).Format("2006-01-02 15:04")
	return &s
}

func tsStringPtr(t pgtype.Timestamptz) *string {
	if !t.Valid {
		return nil
	}
	s := store.TSString(t)
	return &s
}

func examEndString(t pgtype.Timestamptz) string {
	if !t.Valid {
		return ""
	}
	return store.TSString(t)
}

func dateOfPtr(s *string) pgtype.Date {
	if s == nil || *s == "" {
		return pgtype.Date{}
	}
	return store.DateOf(*s)
}

func clockOfPtr(s *string) pgtype.Time {
	if s == nil || *s == "" {
		return pgtype.Time{}
	}
	return store.ClockOf(*s)
}

func remindAtOfPtr(s *string) pgtype.Timestamptz {
	if s == nil || *s == "" {
		return pgtype.Timestamptz{}
	}
	t, err := time.ParseInLocation("2006-01-02 15:04", *s, ChinaTZ)
	if err != nil {
		return pgtype.Timestamptz{}
	}
	return pgtype.Timestamptz{Time: t, Valid: true}
}

func tsOfPtr(s *string) pgtype.Timestamptz {
	if s == nil || *s == "" {
		return pgtype.Timestamptz{}
	}
	return store.TSOf(*s)
}

func tsOf(s string) pgtype.Timestamptz {
	if s == "" {
		return pgtype.Timestamptz{}
	}
	return store.TSOf(s)
}

func dateOf(s string) pgtype.Date {
	if s == "" {
		return pgtype.Date{}
	}
	return store.DateOf(s)
}

func clockOf(s string) pgtype.Time {
	if s == "" {
		return pgtype.Time{}
	}
	return store.ClockOf(s)
}

func examEndOf(s string) pgtype.Timestamptz {
	if s == "" {
		return pgtype.Timestamptz{}
	}
	return store.TSOf(s)
}

func int8Ptr(v pgtype.Int8) *int64 {
	if !v.Valid {
		return nil
	}
	return &v.Int64
}

func int8OfPtr(v *int64) pgtype.Int8 {
	if v == nil {
		return pgtype.Int8{}
	}
	return pgtype.Int8{Int64: *v, Valid: true}
}
