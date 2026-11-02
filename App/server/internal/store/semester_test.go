package store

import (
	"context"
	"strings"
	"testing"
)

func TestSemesterContextUpsertAndFind(t *testing.T) {
	q, _ := newTestStore(t)

	arg := UpsertSemesterContextParams{
		Source:       "manual",
		AcademicYear: textOf("2026"),
		Term:         textOf("1"),
		TermLabel:    "2026S1",
		StartDate:    dateOf("2026-02-24"),
		CurrentWeek:  int4Of(3),
		TotalWeeks:   int4Of(16),
	}
	first, err := q.UpsertSemesterContext(context.Background(), arg)
	if err != nil {
		t.Fatalf("upsert semester context: %v", err)
	}
	if first.ID <= 0 {
		t.Fatalf("expected positive id, got %d", first.ID)
	}

	got, err := q.GetSemesterContext(context.Background(), GetSemesterContextParams{
		Source: "manual", TermLabel: "2026S1",
	})
	if err != nil {
		t.Fatalf("get semester context: %v", err)
	}
	if got.ID != first.ID || !sameDate(got.StartDate, "2026-02-24") {
		t.Fatalf("unexpected context: %+v", got)
	}
	if got.CurrentWeek.Int32 != 3 || got.TotalWeeks.Int32 != 16 {
		t.Fatalf("week fields not persisted: %+v", got)
	}
}

func TestSemesterContextUpsertIsIdempotent(t *testing.T) {
	q, _ := newTestStore(t)

	arg := UpsertSemesterContextParams{
		Source: "manual", TermLabel: "2026S1",
		StartDate: dateOf("2026-02-24"), CurrentWeek: int4Of(3), TotalWeeks: int4Of(16),
	}
	first, err := q.UpsertSemesterContext(context.Background(), arg)
	if err != nil {
		t.Fatalf("first upsert: %v", err)
	}

	arg.StartDate = dateOf("2026-03-02")
	arg.CurrentWeek = int4Of(4)
	arg.TotalWeeks = pgtype_Int4Invalid()
	second, err := q.UpsertSemesterContext(context.Background(), arg)
	if err != nil {
		t.Fatalf("second upsert: %v", err)
	}
	if first.ID != second.ID {
		t.Fatalf("upsert must preserve id: first=%d second=%d", first.ID, second.ID)
	}

	got, err := q.GetSemesterContext(context.Background(), GetSemesterContextParams{
		Source: "manual", TermLabel: "2026S1",
	})
	if err != nil {
		t.Fatalf("get after upsert: %v", err)
	}
	if !sameDate(got.StartDate, "2026-03-02") || got.CurrentWeek.Int32 != 4 || got.TotalWeeks.Valid {
		t.Fatalf("upsert did not replace fields: %+v", got)
	}
}

func TestSemesterContextWeekConstraints(t *testing.T) {
	q, _ := newTestStore(t)

	bad := UpsertSemesterContextParams{
		Source: "manual", TermLabel: "2026S1",
		StartDate: dateOf("2026-02-24"), CurrentWeek: int4Of(0),
	}
	if _, err := q.UpsertSemesterContext(context.Background(), bad); err == nil {
		t.Fatal("current_week=0 should violate CHECK")
	}

	bad2 := UpsertSemesterContextParams{
		Source: "manual", TermLabel: "2026S1",
		StartDate: dateOf("2026-02-24"), CurrentWeek: int4Of(1), TotalWeeks: int4Of(0),
	}
	if _, err := q.UpsertSemesterContext(context.Background(), bad2); err == nil {
		t.Fatal("total_weeks=0 should violate CHECK")
	}
}

func TestSemesterContextLatestBySource(t *testing.T) {
	q, _ := newTestStore(t)

	old := UpsertSemesterContextParams{
		Source: "manual", TermLabel: "2026S1", StartDate: dateOf("2026-02-24"),
	}
	if _, err := q.UpsertSemesterContext(context.Background(), old); err != nil {
		t.Fatalf("upsert old: %v", err)
	}
	latest := UpsertSemesterContextParams{
		Source: "manual", TermLabel: "2026S2", StartDate: dateOf("2026-09-01"),
	}
	if _, err := q.UpsertSemesterContext(context.Background(), latest); err != nil {
		t.Fatalf("upsert latest: %v", err)
	}

	got, err := q.GetLatestSemesterContextBySource(context.Background(), "manual")
	if err != nil {
		t.Fatalf("get latest: %v", err)
	}
	if got.TermLabel != "2026S2" {
		t.Fatalf("expected latest term 2026S2, got %+v", got)
	}
}

func TestTermPhaseCreateAndList(t *testing.T) {
	q, _ := newTestStore(t)

	teaching, err := q.CreateTermPhase(context.Background(), CreateTermPhaseParams{
		SyncID: "", TermLabel: "2026S1", PhaseType: "teaching",
		StartWeek: 1, EndWeek: 16, AffectsCourses: true,
		AffectsExamNotifications: true, PomodoroProfile: "default",
		NotificationRules: []byte("{}"), SortOrder: 0,
	})
	if err != nil {
		t.Fatalf("create teaching phase: %v", err)
	}
	if !teaching.SyncID.Valid || teaching.SyncID.String == "" {
		t.Fatalf("expected backfilled sync_id: %+v", teaching.SyncID)
	}

	if _, err := q.CreateTermPhase(context.Background(), CreateTermPhaseParams{
		SyncID: "", TermLabel: "2026S1", PhaseType: "exam",
		StartWeek: 17, EndWeek: 18, AffectsCourses: true,
		AffectsExamNotifications: true, PomodoroProfile: "intense",
		NotificationRules: []byte("{}"), SortOrder: 1,
	}); err != nil {
		t.Fatalf("create exam phase: %v", err)
	}

	phases, err := q.ListTermPhases(context.Background(), "2026S1")
	if err != nil {
		t.Fatalf("list phases: %v", err)
	}
	if len(phases) != 2 || phases[0].PhaseType != "teaching" || phases[1].PhaseType != "exam" {
		t.Fatalf("unexpected phases: %+v", phases)
	}
}

func TestTermPhaseByWeekPrefersMatchingRange(t *testing.T) {
	q, _ := newTestStore(t)

	if _, err := q.CreateTermPhase(context.Background(), CreateTermPhaseParams{
		SyncID: "", TermLabel: "2026S1", PhaseType: "teaching",
		StartWeek: 1, EndWeek: 16, AffectsCourses: true,
		AffectsExamNotifications: true, PomodoroProfile: "default",
		NotificationRules: []byte("{}"), SortOrder: 0,
	}); err != nil {
		t.Fatalf("create teaching: %v", err)
	}
	if _, err := q.CreateTermPhase(context.Background(), CreateTermPhaseParams{
		SyncID: "", TermLabel: "2026S1", PhaseType: "exam",
		StartWeek: 17, EndWeek: 18, AffectsCourses: true,
		AffectsExamNotifications: true, PomodoroProfile: "intense",
		NotificationRules: []byte("{}"), SortOrder: 1,
	}); err != nil {
		t.Fatalf("create exam: %v", err)
	}

	phase, err := q.GetTermPhaseByWeek(context.Background(), GetTermPhaseByWeekParams{
		TermLabel: "2026S1", WeekIndex: 17,
	})
	if err != nil {
		t.Fatalf("get phase by week: %v", err)
	}
	if phase.PhaseType != "exam" {
		t.Fatalf("expected exam phase at week 17, got %+v", phase)
	}
}

func TestTermPhaseCheckRejectsBadPhaseAndRange(t *testing.T) {
	q, _ := newTestStore(t)

	badType := CreateTermPhaseParams{
		SyncID: "", TermLabel: "2026S1", PhaseType: "holiday",
		StartWeek: 1, EndWeek: 2, AffectsCourses: true,
		AffectsExamNotifications: true, PomodoroProfile: "default",
		NotificationRules: []byte("{}"), SortOrder: 0,
	}
	if _, err := q.CreateTermPhase(context.Background(), badType); err == nil {
		t.Fatal("phase_type 'holiday' should violate CHECK")
	} else if !strings.Contains(err.Error(), "check") {
		t.Fatalf("unexpected error for bad phase_type: %v", err)
	}

	badRange := CreateTermPhaseParams{
		SyncID: "", TermLabel: "2026S1", PhaseType: "break",
		StartWeek: 5, EndWeek: 4, AffectsCourses: false,
		AffectsExamNotifications: false, PomodoroProfile: "default",
		NotificationRules: []byte("{}"), SortOrder: 0,
	}
	if _, err := q.CreateTermPhase(context.Background(), badRange); err == nil {
		t.Fatal("end_week < start_week should violate CHECK")
	}
}

func TestTermPhaseSoftDelete(t *testing.T) {
	q, _ := newTestStore(t)

	phase, err := q.CreateTermPhase(context.Background(), CreateTermPhaseParams{
		SyncID: "", TermLabel: "2026S1", PhaseType: "teaching",
		StartWeek: 1, EndWeek: 16, AffectsCourses: true,
		AffectsExamNotifications: true, PomodoroProfile: "default",
		NotificationRules: []byte("{}"), SortOrder: 0,
	})
	if err != nil {
		t.Fatalf("create phase: %v", err)
	}

	if err := q.SoftDeleteTermPhase(context.Background(), phase.ID); err != nil {
		t.Fatalf("soft delete phase: %v", err)
	}

	phases, err := q.ListTermPhases(context.Background(), "2026S1")
	if err != nil {
		t.Fatalf("list phases: %v", err)
	}
	if len(phases) != 0 {
		t.Fatalf("deleted phase still listed: %+v", phases)
	}

	got, err := q.GetTermPhase(context.Background(), phase.ID)
	if err != nil {
		t.Fatalf("get tombstone: %v", err)
	}
	if !got.DeletedAt.Valid {
		t.Fatalf("expected deleted_at tombstone: %+v", got)
	}
}
