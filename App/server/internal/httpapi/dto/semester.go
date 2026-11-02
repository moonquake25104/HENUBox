package dto

import (
	"github.com/jackc/pgx/v5/pgtype"

	"kairos/server/internal/store"
)

// SemesterContext mirrors the frontend src/types/notification.ts SemesterContext.
type SemesterContext struct {
	ID           int64   `json:"id"`
	Source       string  `json:"source"`
	AcademicYear *string `json:"academic_year"`
	Term         *string `json:"term"`
	TermLabel    string  `json:"term_label"`
	StartDate    string  `json:"start_date"`
	CurrentWeek  *int32  `json:"current_week"`
	TotalWeeks   *int32  `json:"total_weeks"`
	RefreshedAt  string  `json:"refreshed_at"`
	CreatedAt    string  `json:"created_at"`
	UpdatedAt    string  `json:"updated_at"`
}

// TermPhase mirrors the frontend src/types/notification.ts TermPhase.
type TermPhase struct {
	ID                       int64   `json:"id"`
	SyncID                   string  `json:"sync_id"`
	TermLabel                string  `json:"term_label"`
	PhaseType                string  `json:"phase_type"`
	StartWeek                int32   `json:"start_week"`
	EndWeek                  int32   `json:"end_week"`
	AffectsCourses           bool    `json:"affects_courses"`
	AffectsExamNotifications bool    `json:"affects_exam_notifications"`
	PomodoroProfile          string  `json:"pomodoro_profile"`
	NotificationRulesJSON    string  `json:"notification_rules_json"`
	SortOrder                int32   `json:"sort_order"`
	DeletedAt                *string `json:"deleted_at"`
	CreatedAt                string  `json:"created_at"`
	UpdatedAt                string  `json:"updated_at"`
}

// CreateTermPhaseRequest is the body of POST /api/term-phases.
type CreateTermPhaseRequest struct {
	TermLabel                string `json:"term_label"`
	PhaseType                string `json:"phase_type"`
	StartWeek                int32  `json:"start_week"`
	EndWeek                  int32  `json:"end_week"`
	AffectsCourses           bool   `json:"affects_courses"`
	AffectsExamNotifications bool   `json:"affects_exam_notifications"`
	PomodoroProfile          string `json:"pomodoro_profile"`
	NotificationRulesJSON    string `json:"notification_rules_json"`
	SortOrder                int32  `json:"sort_order"`
}

// UpdateTermPhaseRequest is the body of PATCH /api/term-phases/{id}.
type UpdateTermPhaseRequest struct {
	TermLabel                string `json:"term_label"`
	PhaseType                string `json:"phase_type"`
	StartWeek                int32  `json:"start_week"`
	EndWeek                  int32  `json:"end_week"`
	AffectsCourses           bool   `json:"affects_courses"`
	AffectsExamNotifications bool   `json:"affects_exam_notifications"`
	PomodoroProfile          string `json:"pomodoro_profile"`
	NotificationRulesJSON    string `json:"notification_rules_json"`
	SortOrder                int32  `json:"sort_order"`
}

// FromSemesterContext converts a store semester context into the API representation.
func FromSemesterContext(c store.SemesterContext) SemesterContext {
	return SemesterContext{
		ID:           c.ID,
		Source:       c.Source,
		AcademicYear: textPtr(c.AcademicYear),
		Term:         textPtr(c.Term),
		TermLabel:    c.TermLabel,
		StartDate:    store.DateString(c.StartDate),
		CurrentWeek:  Int4Ptr(c.CurrentWeek),
		TotalWeeks:   Int4Ptr(c.TotalWeeks),
		RefreshedAt:  store.TSString(c.RefreshedAt),
		CreatedAt:    store.TSString(c.CreatedAt),
		UpdatedAt:    store.TSString(c.UpdatedAt),
	}
}

// FromTermPhase converts a store term phase into the API representation.
func FromTermPhase(p store.TermPhase) TermPhase {
	return TermPhase{
		ID:                       p.ID,
		SyncID:                   store.TextString(p.SyncID),
		TermLabel:                p.TermLabel,
		PhaseType:                p.PhaseType,
		StartWeek:                p.StartWeek,
		EndWeek:                  p.EndWeek,
		AffectsCourses:           p.AffectsCourses,
		AffectsExamNotifications: p.AffectsExamNotifications,
		PomodoroProfile:          p.PomodoroProfile,
		NotificationRulesJSON:    string(p.NotificationRules),
		SortOrder:                p.SortOrder,
		DeletedAt:                tsPtr(p.DeletedAt),
		CreatedAt:                store.TSString(p.CreatedAt),
		UpdatedAt:                store.TSString(p.UpdatedAt),
	}
}

func textPtr(t pgtype.Text) *string {
	if !t.Valid {
		return nil
	}
	return StrPtr(store.TextString(t))
}
