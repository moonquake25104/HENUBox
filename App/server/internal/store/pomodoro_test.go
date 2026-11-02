package store

import (
	"context"
	"testing"
)

func TestPomodoroConfigDefaultsAndUpdate(t *testing.T) {
	q, _ := newTestStore(t)

	cfg, err := q.GetPomodoroConfig(context.Background())
	if err != nil {
		t.Fatalf("get config: %v", err)
	}
	if cfg.ID != 1 || cfg.WorkSeconds != 1500 || cfg.ShortBreakSeconds != 300 ||
		cfg.LongBreakSeconds != 900 || cfg.SessionsBeforeLongBreak != 4 || cfg.AutoStartNextPhase {
		t.Fatalf("unexpected default config: %+v", cfg)
	}

	if err := q.UpdatePomodoroConfig(context.Background(), UpdatePomodoroConfigParams{
		WorkSeconds: 1800, ShortBreakSeconds: 600, LongBreakSeconds: 1200,
		SessionsBeforeLongBreak: 3, AutoStartNextPhase: true,
	}); err != nil {
		t.Fatalf("update config: %v", err)
	}

	cfg, err = q.GetPomodoroConfig(context.Background())
	if err != nil {
		t.Fatalf("get updated config: %v", err)
	}
	if cfg.WorkSeconds != 1800 || cfg.ShortBreakSeconds != 600 || cfg.LongBreakSeconds != 1200 ||
		cfg.SessionsBeforeLongBreak != 3 || !cfg.AutoStartNextPhase {
		t.Fatalf("config not updated: %+v", cfg)
	}
}

func TestPomodoroBuiltinProfilesPresent(t *testing.T) {
	q, _ := newTestStore(t)

	profiles, err := q.ListPomodoroProfiles(context.Background())
	if err != nil {
		t.Fatalf("list profiles: %v", err)
	}
	names := map[string]bool{}
	for _, p := range profiles {
		names[p.Name] = true
		if !p.IsBuiltin {
			t.Fatalf("builtin profile %q should have is_builtin=true", p.Name)
		}
	}
	for _, want := range []string{"default", "intense", "relaxed"} {
		if !names[want] {
			t.Fatalf("missing builtin profile %q; got %v", want, names)
		}
	}
}

func TestPomodoroProfileCreateUpdateDelete(t *testing.T) {
	q, _ := newTestStore(t)

	created, err := q.CreatePomodoroProfile(context.Background(), CreatePomodoroProfileParams{
		Name: "custom", WorkSeconds: 1800, ShortBreakSeconds: 300,
		LongBreakSeconds: 1200, SessionsBeforeLongBreak: 4,
	})
	if err != nil {
		t.Fatalf("create profile: %v", err)
	}
	if created.IsBuiltin {
		t.Fatalf("custom profile must not be builtin: %+v", created)
	}

	if err := q.UpdatePomodoroProfile(context.Background(), UpdatePomodoroProfileParams{
		Name: "custom 2", WorkSeconds: 2400, ShortBreakSeconds: 600,
		LongBreakSeconds: 1800, SessionsBeforeLongBreak: 3, ID: created.ID,
	}); err != nil {
		t.Fatalf("update profile: %v", err)
	}
	got, err := q.GetPomodoroProfile(context.Background(), created.ID)
	if err != nil {
		t.Fatalf("get profile: %v", err)
	}
	if got.Name != "custom 2" || got.WorkSeconds != 2400 {
		t.Fatalf("profile not updated: %+v", got)
	}

	if err := q.DeletePomodoroProfile(context.Background(), created.ID); err != nil {
		t.Fatalf("delete profile: %v", err)
	}
	if _, err := q.GetPomodoroProfile(context.Background(), created.ID); err == nil {
		t.Fatal("deleted profile should not be fetchable")
	}
}

func TestPomodoroBuiltinProfileDeleteIgnored(t *testing.T) {
	q, _ := newTestStore(t)

	profiles, err := q.ListPomodoroProfiles(context.Background())
	if err != nil {
		t.Fatalf("list profiles: %v", err)
	}
	var def *PomodoroProfile
	for i := range profiles {
		if profiles[i].Name == "default" {
			def = &profiles[i]
			break
		}
	}
	if def == nil {
		t.Fatal("default profile missing")
	}

	if err := q.DeletePomodoroProfile(context.Background(), def.ID); err != nil {
		t.Fatalf("delete builtin: %v", err)
	}
	got, err := q.GetPomodoroProfile(context.Background(), def.ID)
	if err != nil {
		t.Fatalf("builtin should survive delete: %v", err)
	}
	if !got.IsBuiltin {
		t.Fatalf("default should still be builtin: %+v", got)
	}
}

func TestPomodoroSessionCreateEndAndList(t *testing.T) {
	q, _ := newTestStore(t)

	sess, err := q.CreatePomodoroSession(context.Background(), CreatePomodoroSessionParams{
		SyncID: "", StartedAt: tsOf("2024-06-01T10:00:00Z"),
		SessionType: "work", TaskID: pgtype_Int8Invalid(),
	})
	if err != nil {
		t.Fatalf("create session: %v", err)
	}
	if !sess.SyncID.Valid || sess.SyncID.String == "" {
		t.Fatalf("expected backfilled sync_id: %+v", sess.SyncID)
	}

	if err := q.UpdatePomodoroSessionEnd(context.Background(), UpdatePomodoroSessionEndParams{
		EndedAt: tsOf("2024-06-01T10:25:00Z"), ID: sess.ID,
	}); err != nil {
		t.Fatalf("end session: %v", err)
	}

	sessions, err := q.ListPomodoroSessions(context.Background(), ListPomodoroSessionsParams{
		LimitValue: 10, OffsetValue: 0,
	})
	if err != nil {
		t.Fatalf("list sessions: %v", err)
	}
	if len(sessions) != 1 || sessions[0].ID != sess.ID || sessions[0].SessionType != "work" {
		t.Fatalf("unexpected sessions: %+v", sessions)
	}
	if !sameTS(sessions[0].EndedAt, "2024-06-01T10:25:00Z") {
		t.Fatalf("ended_at not persisted: %+v", sessions[0].EndedAt)
	}
}

func TestPomodoroSessionPagination(t *testing.T) {
	q, _ := newTestStore(t)

	for i := 0; i < 5; i++ {
		if _, err := q.CreatePomodoroSession(context.Background(), CreatePomodoroSessionParams{
			SyncID: "", StartedAt: tsOf("2024-06-01T10:0" + string(rune('0'+i)) + ":00Z"),
			SessionType: "work", TaskID: pgtype_Int8Invalid(),
		}); err != nil {
			t.Fatalf("create session %d: %v", i, err)
		}
	}

	page1, err := q.ListPomodoroSessions(context.Background(), ListPomodoroSessionsParams{
		LimitValue: 3, OffsetValue: 0,
	})
	if err != nil {
		t.Fatalf("page1: %v", err)
	}
	if len(page1) != 3 {
		t.Fatalf("expected 3 on page1, got %d", len(page1))
	}
	page2, err := q.ListPomodoroSessions(context.Background(), ListPomodoroSessionsParams{
		LimitValue: 3, OffsetValue: 3,
	})
	if err != nil {
		t.Fatalf("page2: %v", err)
	}
	if len(page2) != 2 {
		t.Fatalf("expected 2 on page2, got %d", len(page2))
	}
}

func TestPomodoroSessionSoftDelete(t *testing.T) {
	q, _ := newTestStore(t)

	sess, err := q.CreatePomodoroSession(context.Background(), CreatePomodoroSessionParams{
		SyncID: "", StartedAt: tsOf("2024-06-01T10:00:00Z"),
		SessionType: "work", TaskID: pgtype_Int8Invalid(),
	})
	if err != nil {
		t.Fatalf("create session: %v", err)
	}

	if err := q.SoftDeletePomodoroSession(context.Background(), sess.ID); err != nil {
		t.Fatalf("soft delete session: %v", err)
	}

	sessions, err := q.ListPomodoroSessions(context.Background(), ListPomodoroSessionsParams{
		LimitValue: 10, OffsetValue: 0,
	})
	if err != nil {
		t.Fatalf("list sessions: %v", err)
	}
	if len(sessions) != 0 {
		t.Fatalf("deleted session still listed: %+v", sessions)
	}
}

func TestPomodoroCountCompletedWorkSessions(t *testing.T) {
	q, _ := newTestStore(t)

	window := CountCompletedWorkSessionsParams{
		WindowStart: tsOf("2024-06-01T00:00:00Z"),
		WindowEnd:   tsOf("2024-06-02T00:00:00Z"),
	}
	count, err := q.CountCompletedWorkSessions(context.Background(), window)
	if err != nil {
		t.Fatalf("count empty: %v", err)
	}
	if count != 0 {
		t.Fatalf("expected 0, got %d", count)
	}

	open, err := q.CreatePomodoroSession(context.Background(), CreatePomodoroSessionParams{
		SyncID: "", StartedAt: tsOf("2024-06-01T10:00:00Z"),
		SessionType: "work", TaskID: pgtype_Int8Invalid(),
	})
	if err != nil {
		t.Fatalf("create open session: %v", err)
	}
	count, err = q.CountCompletedWorkSessions(context.Background(), window)
	if err != nil {
		t.Fatalf("count with open session: %v", err)
	}
	if count != 0 {
		t.Fatalf("unended session should not count, got %d", count)
	}

	if err := q.UpdatePomodoroSessionEnd(context.Background(), UpdatePomodoroSessionEndParams{
		EndedAt: tsOf("2024-06-01T10:25:00Z"), ID: open.ID,
	}); err != nil {
		t.Fatalf("end session: %v", err)
	}
	count, err = q.CountCompletedWorkSessions(context.Background(), window)
	if err != nil {
		t.Fatalf("count after end: %v", err)
	}
	if count != 1 {
		t.Fatalf("expected 1, got %d", count)
	}

	if err := q.SoftDeletePomodoroSession(context.Background(), open.ID); err != nil {
		t.Fatalf("soft delete session: %v", err)
	}
	count, err = q.CountCompletedWorkSessions(context.Background(), window)
	if err != nil {
		t.Fatalf("count after delete: %v", err)
	}
	if count != 0 {
		t.Fatalf("soft-deleted session should not count, got %d", count)
	}
}

func TestPomodoroFindLatestOpenWorkSession(t *testing.T) {
	q, _ := newTestStore(t)

	if _, err := q.FindLatestOpenWorkSession(context.Background()); err == nil {
		t.Fatal("expected no rows when no open session")
	}

	ended, err := q.CreatePomodoroSession(context.Background(), CreatePomodoroSessionParams{
		SyncID: "", StartedAt: tsOf("2024-06-01T10:00:00Z"),
		SessionType: "work", TaskID: pgtype_Int8Invalid(),
	})
	if err != nil {
		t.Fatalf("create ended session: %v", err)
	}
	if err := q.UpdatePomodoroSessionEnd(context.Background(), UpdatePomodoroSessionEndParams{
		EndedAt: tsOf("2024-06-01T10:25:00Z"), ID: ended.ID,
	}); err != nil {
		t.Fatalf("end session: %v", err)
	}
	if _, err := q.FindLatestOpenWorkSession(context.Background()); err == nil {
		t.Fatal("ended session should not be returned as open")
	}

	open, err := q.CreatePomodoroSession(context.Background(), CreatePomodoroSessionParams{
		SyncID: "", StartedAt: tsOf("2024-06-01T11:00:00Z"),
		SessionType: "work", TaskID: pgtype_Int8Invalid(),
	})
	if err != nil {
		t.Fatalf("create open session: %v", err)
	}
	id, err := q.FindLatestOpenWorkSession(context.Background())
	if err != nil {
		t.Fatalf("find open session: %v", err)
	}
	if id != open.ID {
		t.Fatalf("expected open session %d, got %d", open.ID, id)
	}
}

func TestPomodoroRuntimeStateGetOrCreateAndUpdate(t *testing.T) {
	q, _ := newTestStore(t)

	state, err := q.CreateRuntimeState(context.Background(), dateOf("2026-06-01"))
	if err != nil {
		t.Fatalf("create runtime state: %v", err)
	}
	if state.ID != 1 || state.Phase != "work" || state.RemainingSeconds != 1500 ||
		state.TotalSeconds != 1500 || state.IsRunning || state.Interrupted {
		t.Fatalf("unexpected default runtime state: %+v", state)
	}

	if err := q.UpdateRuntimeState(context.Background(), UpdateRuntimeStateParams{
		Phase: "short_break", RemainingSeconds: 120, TotalSeconds: 300,
		IsRunning: true, ActiveSessionID: pgtype_Int8Invalid(),
		DateKey: dateOf("2026-06-01"), Interrupted: true,
	}); err != nil {
		t.Fatalf("update runtime state: %v", err)
	}

	got, err := q.GetRuntimeState(context.Background())
	if err != nil {
		t.Fatalf("get runtime state: %v", err)
	}
	if got.Phase != "short_break" || got.RemainingSeconds != 120 || got.TotalSeconds != 300 ||
		!got.IsRunning || !got.Interrupted {
		t.Fatalf("runtime state not updated: %+v", got)
	}

	if err := q.ClearRuntimeInterruption(context.Background()); err != nil {
		t.Fatalf("clear interruption: %v", err)
	}
	got, err = q.GetRuntimeState(context.Background())
	if err != nil {
		t.Fatalf("get after clear: %v", err)
	}
	if got.Interrupted {
		t.Fatalf("interruption not cleared: %+v", got)
	}
}
