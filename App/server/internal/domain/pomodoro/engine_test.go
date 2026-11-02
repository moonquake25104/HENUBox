package pomodoro

import (
	"testing"
	"time"
)

func defaultConfig() Config {
	return Config{
		WorkSeconds:             3,
		ShortBreakSeconds:       1,
		LongBreakSeconds:        2,
		SessionsBeforeLongBreak: 2,
		AutoStartNextPhase:      true,
	}
}

func tickTimes(e *Engine, count int) *PhaseTransition {
	var last *PhaseTransition
	for range count {
		last = e.Tick()
	}
	return last
}

func startedEngine() *Engine {
	e := New(defaultConfig())
	e.Start()
	return e
}

func int64Ptr(v int64) *int64 { return &v }

func assertWorkToShortBreak(t *testing.T, got *PhaseTransition, endedSessionID *int64) {
	t.Helper()
	if got == nil {
		t.Fatal("expected a transition, got nil")
	}
	if got.EndedPhase != PhaseWork || got.NextPhase != PhaseShortBreak {
		t.Fatalf("transition = %+v, want work->short_break", *got)
	}
	if (got.EndedSessionID == nil) != (endedSessionID == nil) {
		t.Fatalf("ended_session_id = %v, want %v", got.EndedSessionID, endedSessionID)
	}
	if got.EndedSessionID != nil && *got.EndedSessionID != *endedSessionID {
		t.Fatalf("ended_session_id = %d, want %d", *got.EndedSessionID, *endedSessionID)
	}
}

func TestNewEngineStartsInWorkPaused(t *testing.T) {
	e := New(defaultConfig())
	if e.Phase != PhaseWork {
		t.Fatalf("phase = %s, want work", e.Phase)
	}
	if e.IsRunning {
		t.Fatal("engine should start paused")
	}
	if e.RemainingSeconds != 3 || e.TotalSeconds != 3 {
		t.Fatalf("remaining/total = %d/%d, want 3/3", e.RemainingSeconds, e.TotalSeconds)
	}
	if e.CompletedSessions != 0 {
		t.Fatalf("completed_sessions = %d, want 0", e.CompletedSessions)
	}
}

func TestTickWhenPausedDoesNothing(t *testing.T) {
	e := New(defaultConfig())
	if got := e.Tick(); got != nil {
		t.Fatalf("tick while paused = %+v, want nil", got)
	}
	if e.RemainingSeconds != 3 {
		t.Fatalf("remaining = %d, want 3", e.RemainingSeconds)
	}
}

func TestStartAndTickDecrements(t *testing.T) {
	e := startedEngine()
	if !e.IsRunning {
		t.Fatal("engine should be running")
	}
	e.Tick()
	if e.RemainingSeconds != 2 {
		t.Fatalf("remaining = %d, want 2", e.RemainingSeconds)
	}
	e.Tick()
	if e.RemainingSeconds != 1 {
		t.Fatalf("remaining = %d, want 1", e.RemainingSeconds)
	}
}

func TestTickReturnsPhaseOnExpiry(t *testing.T) {
	e := startedEngine()
	got := tickTimes(e, 3)
	assertWorkToShortBreak(t, got, nil)
	if e.Phase != PhaseShortBreak {
		t.Fatalf("phase = %s, want short_break", e.Phase)
	}
	if e.RemainingSeconds != 1 {
		t.Fatalf("remaining = %d, want 1", e.RemainingSeconds)
	}
	if e.CompletedSessions != 1 {
		t.Fatalf("completed_sessions = %d, want 1", e.CompletedSessions)
	}
	if !e.IsRunning {
		t.Fatal("engine should keep running with auto_start")
	}
}

func TestLongBreakAfterNSessions(t *testing.T) {
	e := startedEngine()
	tickTimes(e, 3)
	if e.Phase != PhaseShortBreak || e.CompletedSessions != 1 {
		t.Fatalf("after 1st work: phase=%s completed=%d", e.Phase, e.CompletedSessions)
	}
	tickTimes(e, 1)
	if e.Phase != PhaseWork || e.RemainingSeconds != 3 {
		t.Fatalf("after break: phase=%s remaining=%d", e.Phase, e.RemainingSeconds)
	}
	tickTimes(e, 3)
	if e.Phase != PhaseLongBreak || e.CompletedSessions != 2 || e.RemainingSeconds != 2 {
		t.Fatalf("after 2nd work: phase=%s completed=%d remaining=%d", e.Phase, e.CompletedSessions, e.RemainingSeconds)
	}
}

func TestBreakSwitchesBackToWork(t *testing.T) {
	e := startedEngine()
	tickTimes(e, 3)
	if e.Phase != PhaseShortBreak {
		t.Fatalf("phase = %s, want short_break", e.Phase)
	}
	tickTimes(e, 1)
	if e.Phase != PhaseWork || e.RemainingSeconds != 3 {
		t.Fatalf("phase=%s remaining=%d, want work/3", e.Phase, e.RemainingSeconds)
	}
}

func TestPauseStopsTicking(t *testing.T) {
	e := startedEngine()
	tickTimes(e, 1)
	if e.RemainingSeconds != 2 {
		t.Fatalf("remaining = %d, want 2", e.RemainingSeconds)
	}
	e.Pause()
	tickTimes(e, 2)
	if e.RemainingSeconds != 2 {
		t.Fatalf("remaining = %d, want 2", e.RemainingSeconds)
	}
	if e.IsRunning {
		t.Fatal("engine should be paused")
	}
}

func TestResetRestoresFullDuration(t *testing.T) {
	e := startedEngine()
	tickTimes(e, 2)
	if e.RemainingSeconds != 1 {
		t.Fatalf("remaining = %d, want 1", e.RemainingSeconds)
	}
	e.Reset()
	if e.RemainingSeconds != 3 || e.IsRunning {
		t.Fatalf("after reset: remaining=%d running=%v, want 3/false", e.RemainingSeconds, e.IsRunning)
	}
}

func TestUpdateConfigResetsToWork(t *testing.T) {
	e := startedEngine()
	e.Tick()
	newCfg := Config{
		WorkSeconds:             10,
		ShortBreakSeconds:       2,
		LongBreakSeconds:        5,
		SessionsBeforeLongBreak: 3,
		AutoStartNextPhase:      true,
	}
	e.UpdateConfig(newCfg)
	if e.Phase != PhaseWork || e.RemainingSeconds != 10 || e.TotalSeconds != 10 || e.IsRunning || e.CompletedSessions != 0 {
		t.Fatalf("after update_config: phase=%s remaining=%d total=%d running=%v completed=%d",
			e.Phase, e.RemainingSeconds, e.TotalSeconds, e.IsRunning, e.CompletedSessions)
	}
}

func TestNegativeConfigValuesClampedToZero(t *testing.T) {
	cfg := Config{WorkSeconds: -5, ShortBreakSeconds: -1, LongBreakSeconds: -3, SessionsBeforeLongBreak: 2}
	e := New(cfg)
	if e.RemainingSeconds != 0 {
		t.Fatalf("remaining = %d, want 0", e.RemainingSeconds)
	}
}

func TestEverySessionLongBreakWhenThresholdOne(t *testing.T) {
	cfg := Config{WorkSeconds: 2, ShortBreakSeconds: 1, LongBreakSeconds: 3, SessionsBeforeLongBreak: 1, AutoStartNextPhase: true}
	e := New(cfg)
	e.Start()
	tickTimes(e, 2)
	if e.Phase != PhaseLongBreak || e.RemainingSeconds != 3 || e.CompletedSessions != 1 {
		t.Fatalf("phase=%s remaining=%d completed=%d", e.Phase, e.RemainingSeconds, e.CompletedSessions)
	}
}

func TestResetDuringBreakRestoresBreakDuration(t *testing.T) {
	e := startedEngine()
	tickTimes(e, 3)
	if e.Phase != PhaseShortBreak || e.RemainingSeconds != 1 {
		t.Fatalf("phase=%s remaining=%d", e.Phase, e.RemainingSeconds)
	}
	e.Reset()
	if e.Phase != PhaseShortBreak || e.RemainingSeconds != 1 || e.IsRunning {
		t.Fatalf("after reset during break: phase=%s remaining=%d running=%v", e.Phase, e.RemainingSeconds, e.IsRunning)
	}
}

func TestMultipleCyclesTrackCorrectCount(t *testing.T) {
	e := startedEngine()
	for range 50 {
		e.Tick()
	}
	if e.CompletedSessions < 6 {
		t.Fatalf("completed_sessions = %d, want >= 6", e.CompletedSessions)
	}
}

func TestTickAtZeroNoDoubleAdvance(t *testing.T) {
	e := startedEngine()
	tickTimes(e, 3)
	if e.Phase != PhaseShortBreak || e.RemainingSeconds != 1 {
		t.Fatalf("phase=%s remaining=%d", e.Phase, e.RemainingSeconds)
	}
	tickTimes(e, 3)
	if e.CompletedSessions != 1 {
		t.Fatalf("completed_sessions = %d, want 1", e.CompletedSessions)
	}
}

func TestPauseDuringBreakAndResume(t *testing.T) {
	e := startedEngine()
	tickTimes(e, 3)
	if e.Phase != PhaseShortBreak || e.RemainingSeconds != 1 {
		t.Fatalf("phase=%s remaining=%d", e.Phase, e.RemainingSeconds)
	}
	e.Pause()
	tickTimes(e, 1)
	if e.RemainingSeconds != 1 || e.IsRunning {
		t.Fatalf("remaining=%d running=%v", e.RemainingSeconds, e.IsRunning)
	}
	e.Start()
	tickTimes(e, 1)
	if e.Phase != PhaseWork || e.RemainingSeconds != 3 {
		t.Fatalf("phase=%s remaining=%d", e.Phase, e.RemainingSeconds)
	}
}

func TestGetStateReflectsCurrentPhase(t *testing.T) {
	e := startedEngine()
	st := SnapshotOf(e)
	if st.Phase != "work" || st.RemainingSeconds != 3 || st.TotalSeconds != 3 || !st.IsRunning || st.CompletedSessions != 0 {
		t.Fatalf("unexpected state: %+v", st)
	}
	if st.Interrupted || st.InterruptedSessionID != nil {
		t.Fatalf("expected no interruption, got %+v", st)
	}
}

func TestRestoreFromPausedState(t *testing.T) {
	lastSeen := time.Date(2026, 6, 28, 10, 0, 0, 0, time.UTC)
	e := Restore(defaultConfig(), RestoredState{
		Phase:             PhaseShortBreak,
		RemainingSeconds:  120,
		TotalSeconds:      300,
		WasRunning:        false,
		CompletedSessions: 3,
		LastSeenAt:        &lastSeen,
	})
	if e.Phase != PhaseShortBreak || e.RemainingSeconds != 120 || e.TotalSeconds != 300 || e.IsRunning || e.CompletedSessions != 3 {
		t.Fatalf("unexpected engine: %+v", e)
	}
	if e.Interrupted {
		t.Fatal("paused state should not be interrupted")
	}
	if e.InterruptedSessionID != nil {
		t.Fatalf("interrupted_session_id = %v, want nil", e.InterruptedSessionID)
	}
}

func TestRestoreFromRunningSetsInterrupted(t *testing.T) {
	lastSeen := time.Date(2026, 6, 28, 10, 30, 0, 0, time.UTC)
	e := Restore(defaultConfig(), RestoredState{
		Phase:             PhaseWork,
		RemainingSeconds:  800,
		TotalSeconds:      1500,
		WasRunning:        true,
		CompletedSessions: 2,
		ActiveSessionID:   int64Ptr(42),
		LastSeenAt:        &lastSeen,
	})
	if e.Phase != PhaseWork || e.RemainingSeconds != 800 || e.IsRunning {
		t.Fatalf("phase=%s remaining=%d running=%v", e.Phase, e.RemainingSeconds, e.IsRunning)
	}
	if !e.Interrupted || e.InterruptedSessionID == nil || *e.InterruptedSessionID != 42 {
		t.Fatalf("interrupted=%v interrupted_session_id=%v", e.Interrupted, e.InterruptedSessionID)
	}
	if e.CompletedSessions != 2 {
		t.Fatalf("completed_sessions = %d, want 2", e.CompletedSessions)
	}
}

func TestRestoreCompletedSessionsPreserved(t *testing.T) {
	e := Restore(defaultConfig(), RestoredState{
		Phase: PhaseWork, RemainingSeconds: 1500, TotalSeconds: 1500,
		CompletedSessions: 5,
	})
	if e.CompletedSessions != 5 {
		t.Fatalf("completed_sessions = %d, want 5", e.CompletedSessions)
	}
}

func TestSetInterruptedClearsSessionID(t *testing.T) {
	e := New(defaultConfig())
	e.Interrupted = true
	e.InterruptedSessionID = int64Ptr(42)
	e.SetInterrupted(false)
	if e.Interrupted || e.InterruptedSessionID != nil {
		t.Fatalf("interrupted=%v id=%v", e.Interrupted, e.InterruptedSessionID)
	}
}

func TestTickTransitionPreservesEndingSessionID(t *testing.T) {
	e := New(defaultConfig())
	e.ActiveSessionID = int64Ptr(99)
	e.Start()
	got := tickTimes(e, 3)
	assertWorkToShortBreak(t, got, int64Ptr(99))
	if e.ActiveSessionID != nil {
		t.Fatalf("active_session_id = %v, want nil", e.ActiveSessionID)
	}
}

func TestCompletePhasePausedSwitchesWithoutRunning(t *testing.T) {
	e := New(defaultConfig())
	e.ActiveSessionID = int64Ptr(7)
	next := e.CompletePhasePaused()
	if next != PhaseShortBreak {
		t.Fatalf("next = %s, want short_break", next)
	}
	if e.CompletedSessions != 1 || e.Phase != PhaseShortBreak || e.IsRunning || e.ActiveSessionID != nil {
		t.Fatalf("unexpected engine: %+v", e)
	}
}

func configWithAutoStart(auto bool) Config {
	cfg := defaultConfig()
	cfg.AutoStartNextPhase = auto
	return cfg
}

func TestAutoStartDisabledPausesAfterWorkEnds(t *testing.T) {
	e := New(configWithAutoStart(false))
	e.Start()
	got := tickTimes(e, 3)
	assertWorkToShortBreak(t, got, nil)
	if e.Phase != PhaseShortBreak || e.IsRunning || e.RemainingSeconds != 1 {
		t.Fatalf("phase=%s running=%v remaining=%d", e.Phase, e.IsRunning, e.RemainingSeconds)
	}
}

func TestAutoStartDisabledPausesBreakToWork(t *testing.T) {
	e := New(configWithAutoStart(false))
	e.Start()
	tickTimes(e, 3)
	if e.IsRunning {
		t.Fatal("work->short_break should pause when auto_start disabled")
	}
	e.Start()
	tickTimes(e, 1)
	if e.Phase != PhaseWork || e.IsRunning || e.RemainingSeconds != 3 {
		t.Fatalf("phase=%s running=%v remaining=%d", e.Phase, e.IsRunning, e.RemainingSeconds)
	}
}

func TestAutoStartEnabledRunsAfterPhaseEnds(t *testing.T) {
	e := New(configWithAutoStart(true))
	e.Start()
	tickTimes(e, 3)
	if e.Phase != PhaseShortBreak || !e.IsRunning {
		t.Fatalf("phase=%s running=%v, want short_break/running", e.Phase, e.IsRunning)
	}
}
