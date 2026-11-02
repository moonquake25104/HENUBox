package httpapi

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"golang.org/x/crypto/bcrypt"

	"kairos/server/internal/domain/pomodoro"
	"kairos/server/internal/httpapi/dto"
	"kairos/server/internal/store"
	"kairos/server/internal/store/migrate"
)

func newPomodoroTestRouter(t *testing.T, notifier pomodoro.Notifier) (http.Handler, string, *store.Queries, *pgx.Conn) {
	t.Helper()
	ctx := context.Background()
	conn, err := pgx.Connect(ctx, testDatabaseURL())
	if err != nil {
		t.Fatalf("connect to test database: %v", err)
	}
	schema := "httpapi_pomodoro_" + randomHex()
	if _, err := conn.Exec(ctx, "CREATE SCHEMA "+schema); err != nil {
		t.Fatalf("create schema %s: %v", schema, err)
	}
	if _, err := conn.Exec(ctx, "SET search_path TO "+schema); err != nil {
		t.Fatalf("set search_path to %s: %v", schema, err)
	}
	if err := migrate.Up(ctx, conn); err != nil {
		t.Fatalf("apply migrations to schema %s: %v", schema, err)
	}
	t.Cleanup(func() {
		defer conn.Close(context.Background())
		if _, err := conn.Exec(context.Background(), "DROP SCHEMA IF EXISTS "+schema+" CASCADE"); err != nil {
			t.Errorf("drop schema %s: %v", schema, err)
		}
	})

	hash, err := bcrypt.GenerateFromPassword([]byte("correct-horse"), bcrypt.MinCost)
	if err != nil {
		t.Fatalf("hash password: %v", err)
	}
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	h := New(Options{
		Log:              log,
		Username:         "kairos",
		PasswordHash:     string(hash),
		JWTSecret:        []byte(testSecret),
		JWTTTL:           time.Hour,
		Store:            store.New(conn),
		PomodoroNotifier: notifier,
	})
	return h, loginToken(t, h), store.New(conn), conn
}

func decodeState(t *testing.T, rec *httptest.ResponseRecorder) pomodoro.State {
	t.Helper()
	var st pomodoro.State
	decodeBody(t, rec, &st)
	return st
}

// setRuntime rewrites the runtime row directly so tests can simulate elapsed
// wall time without waiting for real seconds.
func setRuntime(t *testing.T, conn *pgx.Conn, phase string, remaining, total int32, running bool, lastSeen time.Time, dateKey string) {
	t.Helper()
	_, err := conn.Exec(context.Background(),
		`UPDATE pomodoro_runtime_state
		 SET phase = $1, remaining_seconds = $2, total_seconds = $3, is_running = $4,
		     last_seen_at = $5, date_key = $6, interrupted = false
		 WHERE id = 1`,
		phase, remaining, total, running, lastSeen.UTC(), dateKey)
	if err != nil {
		t.Fatalf("set runtime state: %v", err)
	}
}

func todayKey() string {
	return time.Now().In(pomodoro.ChinaTZ).Format("2006-01-02")
}

func TestPomodoroStateInitialDefaults(t *testing.T) {
	h, token, _, _ := newPomodoroTestRouter(t, nil)

	rec := doJSON(t, h, http.MethodGet, "/api/pomodoro/state", nil, token)
	if rec.Code != http.StatusOK {
		t.Fatalf("state status = %d, want 200; body=%s", rec.Code, rec.Body.String())
	}
	st := decodeState(t, rec)
	if st.Phase != "work" || st.RemainingSeconds != 1500 || st.TotalSeconds != 1500 ||
		st.IsRunning || st.CompletedSessions != 0 || st.Interrupted {
		t.Fatalf("unexpected initial state: %+v", st)
	}
}

func TestPomodoroStartPauseReset(t *testing.T) {
	h, token, q, _ := newPomodoroTestRouter(t, nil)

	rec := doJSON(t, h, http.MethodPost, "/api/pomodoro/start", nil, token)
	if rec.Code != http.StatusOK {
		t.Fatalf("start status = %d, want 200; body=%s", rec.Code, rec.Body.String())
	}
	st := decodeState(t, rec)
	if !st.IsRunning || st.Phase != "work" || st.RemainingSeconds != 1500 {
		t.Fatalf("unexpected started state: %+v", st)
	}

	sessions, err := q.ListPomodoroSessions(context.Background(), store.ListPomodoroSessionsParams{LimitValue: 10, OffsetValue: 0})
	if err != nil {
		t.Fatalf("list sessions: %v", err)
	}
	if len(sessions) != 1 || sessions[0].SessionType != "work" || sessions[0].EndedAt.Valid {
		t.Fatalf("expected one open work session, got %+v", sessions)
	}

	rec = doJSON(t, h, http.MethodPost, "/api/pomodoro/pause", nil, token)
	if rec.Code != http.StatusOK {
		t.Fatalf("pause status = %d, want 200", rec.Code)
	}
	st = decodeState(t, rec)
	if st.IsRunning || st.RemainingSeconds != 1500 {
		t.Fatalf("unexpected paused state: %+v", st)
	}

	rec = doJSON(t, h, http.MethodPost, "/api/pomodoro/start", nil, token)
	st = decodeState(t, rec)
	if !st.IsRunning {
		t.Fatalf("resume should run: %+v", st)
	}

	rec = doJSON(t, h, http.MethodPost, "/api/pomodoro/reset", nil, token)
	if rec.Code != http.StatusOK {
		t.Fatalf("reset status = %d, want 200", rec.Code)
	}
	st = decodeState(t, rec)
	if st.IsRunning || st.RemainingSeconds != 1500 {
		t.Fatalf("unexpected reset state: %+v", st)
	}
	sessions, err = q.ListPomodoroSessions(context.Background(), store.ListPomodoroSessionsParams{LimitValue: 10, OffsetValue: 0})
	if err != nil {
		t.Fatalf("list sessions: %v", err)
	}
	if len(sessions) != 0 {
		t.Fatalf("reset should soft-delete the open session, got %+v", sessions)
	}
}

func TestPomodoroFinishPhaseWorkAndBreak(t *testing.T) {
	h, token, q, conn := newPomodoroTestRouter(t, nil)

	doJSON(t, h, http.MethodPost, "/api/pomodoro/start", nil, token)
	setRuntime(t, conn, "work", 0, 1500, true, time.Now(), todayKey())

	rec := doJSON(t, h, http.MethodPost, "/api/pomodoro/finish-phase", dto.FinishPhaseRequest{Phase: "work"}, token)
	if rec.Code != http.StatusOK {
		t.Fatalf("finish work status = %d, want 200; body=%s", rec.Code, rec.Body.String())
	}
	st := decodeState(t, rec)
	if st.Phase != "short_break" || st.IsRunning || st.CompletedSessions != 1 || st.RemainingSeconds != 300 {
		t.Fatalf("unexpected state after work finish: %+v", st)
	}

	sessions, err := q.ListPomodoroSessions(context.Background(), store.ListPomodoroSessionsParams{LimitValue: 10, OffsetValue: 0})
	if err != nil {
		t.Fatalf("list sessions: %v", err)
	}
	if len(sessions) != 1 || !sessions[0].EndedAt.Valid {
		t.Fatalf("work session should be closed, got %+v", sessions)
	}

	// Run the short break to zero, then finish it.
	setRuntime(t, conn, "short_break", 0, 300, true, time.Now(), todayKey())
	rec = doJSON(t, h, http.MethodPost, "/api/pomodoro/finish-phase", dto.FinishPhaseRequest{Phase: "short_break"}, token)
	if rec.Code != http.StatusOK {
		t.Fatalf("finish break status = %d, want 200; body=%s", rec.Code, rec.Body.String())
	}
	st = decodeState(t, rec)
	if st.Phase != "work" || st.IsRunning || st.RemainingSeconds != 1500 || st.CompletedSessions != 1 {
		t.Fatalf("unexpected state after break finish: %+v", st)
	}

	// A paused work phase with no open session cannot be finished.
	rec = doJSON(t, h, http.MethodPost, "/api/pomodoro/finish-phase", dto.FinishPhaseRequest{Phase: "work"}, token)
	if rec.Code != http.StatusConflict {
		t.Fatalf("finish without session status = %d, want 409; body=%s", rec.Code, rec.Body.String())
	}
}

func TestPomodoroFinishPhaseLongBreakAfterThreshold(t *testing.T) {
	h, token, _, conn := newPomodoroTestRouter(t, nil)

	rec := doJSON(t, h, http.MethodPatch, "/api/pomodoro/config", dto.PomodoroConfig{
		WorkSeconds: 1500, ShortBreakSeconds: 300, LongBreakSeconds: 900,
		SessionsBeforeLongBreak: 2, AutoStartNextPhase: false,
	}, token)
	if rec.Code != http.StatusOK {
		t.Fatalf("update config status = %d, want 200; body=%s", rec.Code, rec.Body.String())
	}

	finishWork := func() pomodoro.State {
		doJSON(t, h, http.MethodPost, "/api/pomodoro/start", nil, token)
		setRuntime(t, conn, "work", 0, 1500, true, time.Now(), todayKey())
		rec := doJSON(t, h, http.MethodPost, "/api/pomodoro/finish-phase", dto.FinishPhaseRequest{Phase: "work"}, token)
		if rec.Code != http.StatusOK {
			t.Fatalf("finish work status = %d, want 200; body=%s", rec.Code, rec.Body.String())
		}
		return decodeState(t, rec)
	}

	first := finishWork()
	if first.Phase != "short_break" || first.CompletedSessions != 1 {
		t.Fatalf("first finish: %+v", first)
	}
	setRuntime(t, conn, "short_break", 0, 300, true, time.Now(), todayKey())
	rec = doJSON(t, h, http.MethodPost, "/api/pomodoro/finish-phase", dto.FinishPhaseRequest{Phase: "short_break"}, token)
	if rec.Code != http.StatusOK {
		t.Fatalf("finish break status = %d, want 200", rec.Code)
	}

	second := finishWork()
	if second.Phase != "long_break" || second.CompletedSessions != 2 || second.RemainingSeconds != 900 {
		t.Fatalf("second finish should hit long_break: %+v", second)
	}
}

func TestPomodoroFinishPhaseValidation(t *testing.T) {
	h, token, _, _ := newPomodoroTestRouter(t, nil)

	// Fresh paused work phase with full remaining: not finished.
	rec := doJSON(t, h, http.MethodPost, "/api/pomodoro/finish-phase", dto.FinishPhaseRequest{Phase: "work"}, token)
	if rec.Code != http.StatusConflict {
		t.Fatalf("early finish status = %d, want 409; body=%s", rec.Code, rec.Body.String())
	}

	// Phase mismatch.
	rec = doJSON(t, h, http.MethodPost, "/api/pomodoro/finish-phase", dto.FinishPhaseRequest{Phase: "short_break"}, token)
	if rec.Code != http.StatusConflict {
		t.Fatalf("mismatch status = %d, want 409; body=%s", rec.Code, rec.Body.String())
	}

	// Invalid phase value.
	rec = doJSON(t, h, http.MethodPost, "/api/pomodoro/finish-phase", dto.FinishPhaseRequest{Phase: "nap"}, token)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("invalid phase status = %d, want 400; body=%s", rec.Code, rec.Body.String())
	}
}

func TestPomodoroInterruptionFlow(t *testing.T) {
	h, token, q, conn := newPomodoroTestRouter(t, nil)

	doJSON(t, h, http.MethodPost, "/api/pomodoro/start", nil, token)
	sessions, _ := q.ListPomodoroSessions(context.Background(), store.ListPomodoroSessionsParams{LimitValue: 10, OffsetValue: 0})
	sessionID := sessions[0].ID

	// Simulate the page being away for 600s of a running 800s-remaining phase.
	setRuntime(t, conn, "work", 800, 1500, true, time.Now().Add(-600*time.Second), todayKey())

	rec := doJSON(t, h, http.MethodGet, "/api/pomodoro/state", nil, token)
	if rec.Code != http.StatusOK {
		t.Fatalf("state status = %d, want 200", rec.Code)
	}
	st := decodeState(t, rec)
	if !st.Interrupted || st.IsRunning || st.RemainingSeconds != 200 {
		t.Fatalf("expected interrupted with 200s remaining, got %+v", st)
	}
	if st.InterruptedSessionID == nil || *st.InterruptedSessionID != sessionID {
		t.Fatalf("interrupted_session_id = %v, want %d", st.InterruptedSessionID, sessionID)
	}

	// continue keeps the decayed remaining, paused.
	rec = doJSON(t, h, http.MethodPost, "/api/pomodoro/interrupt", dto.ResolveInterruptionRequest{Action: "continue"}, token)
	if rec.Code != http.StatusOK {
		t.Fatalf("continue status = %d, want 200; body=%s", rec.Code, rec.Body.String())
	}
	st = decodeState(t, rec)
	if st.Interrupted || st.IsRunning || st.RemainingSeconds != 200 {
		t.Fatalf("unexpected continue state: %+v", st)
	}

	// discard resets the phase and soft-deletes the session.
	setRuntime(t, conn, "work", 800, 1500, true, time.Now().Add(-600*time.Second), todayKey())
	rec = doJSON(t, h, http.MethodPost, "/api/pomodoro/interrupt", dto.ResolveInterruptionRequest{Action: "discard"}, token)
	if rec.Code != http.StatusOK {
		t.Fatalf("discard status = %d, want 200; body=%s", rec.Code, rec.Body.String())
	}
	st = decodeState(t, rec)
	if st.Interrupted || st.IsRunning || st.RemainingSeconds != 1500 {
		t.Fatalf("unexpected discard state: %+v", st)
	}
	sessions, _ = q.ListPomodoroSessions(context.Background(), store.ListPomodoroSessionsParams{LimitValue: 10, OffsetValue: 0})
	if len(sessions) != 0 {
		t.Fatalf("discard should soft-delete the session, got %+v", sessions)
	}

	// complete backfills the session and advances the phase.
	doJSON(t, h, http.MethodPost, "/api/pomodoro/start", nil, token)
	setRuntime(t, conn, "work", 800, 1500, true, time.Now().Add(-600*time.Second), todayKey())
	rec = doJSON(t, h, http.MethodPost, "/api/pomodoro/interrupt", dto.ResolveInterruptionRequest{Action: "complete"}, token)
	if rec.Code != http.StatusOK {
		t.Fatalf("complete status = %d, want 200; body=%s", rec.Code, rec.Body.String())
	}
	st = decodeState(t, rec)
	if st.Phase != "short_break" || st.IsRunning || st.CompletedSessions != 1 {
		t.Fatalf("unexpected complete state: %+v", st)
	}
	sessions, _ = q.ListPomodoroSessions(context.Background(), store.ListPomodoroSessionsParams{LimitValue: 10, OffsetValue: 0})
	if len(sessions) != 1 || !sessions[0].EndedAt.Valid {
		t.Fatalf("complete should close the session, got %+v", sessions)
	}

	// complete without an active session fails.
	rec = doJSON(t, h, http.MethodPost, "/api/pomodoro/interrupt", dto.ResolveInterruptionRequest{Action: "complete"}, token)
	if rec.Code != http.StatusConflict {
		t.Fatalf("complete without session status = %d, want 409; body=%s", rec.Code, rec.Body.String())
	}

	// Unknown action.
	rec = doJSON(t, h, http.MethodPost, "/api/pomodoro/interrupt", dto.ResolveInterruptionRequest{Action: "explode"}, token)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("unknown action status = %d, want 400; body=%s", rec.Code, rec.Body.String())
	}
}

func TestPomodoroDailyRollover(t *testing.T) {
	h, token, _, conn := newPomodoroTestRouter(t, nil)

	doJSON(t, h, http.MethodPost, "/api/pomodoro/start", nil, token)
	yesterday := time.Now().In(pomodoro.ChinaTZ).AddDate(0, 0, -1).Format("2006-01-02")
	setRuntime(t, conn, "work", 800, 1500, true, time.Now().Add(-600*time.Second), yesterday)

	rec := doJSON(t, h, http.MethodGet, "/api/pomodoro/state", nil, token)
	if rec.Code != http.StatusOK {
		t.Fatalf("state status = %d, want 200", rec.Code)
	}
	st := decodeState(t, rec)
	if st.Phase != "work" || st.IsRunning || st.Interrupted || st.RemainingSeconds != 1500 || st.CompletedSessions != 0 {
		t.Fatalf("expected fresh state after day rollover, got %+v", st)
	}
}

func TestPomodoroConfigGetUpdate(t *testing.T) {
	h, token, _, _ := newPomodoroTestRouter(t, nil)

	rec := doJSON(t, h, http.MethodGet, "/api/pomodoro/config", nil, token)
	if rec.Code != http.StatusOK {
		t.Fatalf("get config status = %d, want 200", rec.Code)
	}
	var cfg dto.PomodoroConfig
	decodeBody(t, rec, &cfg)
	if cfg.WorkSeconds != 1500 || cfg.ShortBreakSeconds != 300 || cfg.LongBreakSeconds != 900 ||
		cfg.SessionsBeforeLongBreak != 4 || cfg.AutoStartNextPhase {
		t.Fatalf("unexpected default config: %+v", cfg)
	}

	rec = doJSON(t, h, http.MethodPatch, "/api/pomodoro/config", dto.PomodoroConfig{
		WorkSeconds: 1800, ShortBreakSeconds: 600, LongBreakSeconds: 1200,
		SessionsBeforeLongBreak: 3, AutoStartNextPhase: true,
	}, token)
	if rec.Code != http.StatusOK {
		t.Fatalf("update config status = %d, want 200; body=%s", rec.Code, rec.Body.String())
	}
	st := decodeState(t, rec)
	if st.Phase != "work" || st.IsRunning || st.RemainingSeconds != 1800 || st.TotalSeconds != 1800 {
		t.Fatalf("config update should reset to fresh work: %+v", st)
	}

	rec = doJSON(t, h, http.MethodGet, "/api/pomodoro/config", nil, token)
	decodeBody(t, rec, &cfg)
	if cfg.WorkSeconds != 1800 || cfg.ShortBreakSeconds != 600 || cfg.LongBreakSeconds != 1200 ||
		cfg.SessionsBeforeLongBreak != 3 || !cfg.AutoStartNextPhase {
		t.Fatalf("config not persisted: %+v", cfg)
	}

	rec = doJSON(t, h, http.MethodPatch, "/api/pomodoro/config", dto.PomodoroConfig{
		WorkSeconds: 30, ShortBreakSeconds: 600, LongBreakSeconds: 1200,
		SessionsBeforeLongBreak: 3, AutoStartNextPhase: false,
	}, token)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("invalid config status = %d, want 400; body=%s", rec.Code, rec.Body.String())
	}
}

func TestPomodoroProfilesCRUDAndBuiltinProtection(t *testing.T) {
	h, token, _, _ := newPomodoroTestRouter(t, nil)

	rec := doJSON(t, h, http.MethodGet, "/api/pomodoro/profiles", nil, token)
	if rec.Code != http.StatusOK {
		t.Fatalf("list profiles status = %d, want 200", rec.Code)
	}
	var profiles []dto.PomodoroProfile
	decodeBody(t, rec, &profiles)
	if len(profiles) != 3 {
		t.Fatalf("expected 3 builtin profiles, got %d", len(profiles))
	}
	var defaultProfile dto.PomodoroProfile
	for _, p := range profiles {
		if p.Name == "default" {
			defaultProfile = p
		}
		if !p.IsBuiltin {
			t.Fatalf("builtin profile %q should be builtin", p.Name)
		}
	}

	rec = doJSON(t, h, http.MethodPost, "/api/pomodoro/profiles", dto.CreatePomodoroProfileRequest{
		Name: "custom", WorkSeconds: 1800, ShortBreakSeconds: 300,
		LongBreakSeconds: 1200, SessionsBeforeLongBreak: 4,
	}, token)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create profile status = %d, want 201; body=%s", rec.Code, rec.Body.String())
	}
	var created dto.PomodoroProfile
	decodeBody(t, rec, &created)
	if created.IsBuiltin {
		t.Fatalf("custom profile must not be builtin: %+v", created)
	}

	rec = doJSON(t, h, http.MethodPatch, "/api/pomodoro/profiles/"+itoa(created.ID), dto.UpdatePomodoroProfileRequest{
		Name: "custom 2", WorkSeconds: 2400, ShortBreakSeconds: 600,
		LongBreakSeconds: 1800, SessionsBeforeLongBreak: 3,
	}, token)
	if rec.Code != http.StatusOK {
		t.Fatalf("update profile status = %d, want 200; body=%s", rec.Code, rec.Body.String())
	}
	var updated dto.PomodoroProfile
	decodeBody(t, rec, &updated)
	if updated.Name != "custom 2" || updated.WorkSeconds != 2400 {
		t.Fatalf("profile not updated: %+v", updated)
	}

	rec = doJSON(t, h, http.MethodDelete, "/api/pomodoro/profiles/"+itoa(created.ID), nil, token)
	if rec.Code != http.StatusOK {
		t.Fatalf("delete profile status = %d, want 200", rec.Code)
	}

	// Builtin protection.
	rec = doJSON(t, h, http.MethodDelete, "/api/pomodoro/profiles/"+itoa(defaultProfile.ID), nil, token)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("delete builtin status = %d, want 400; body=%s", rec.Code, rec.Body.String())
	}
	rec = doJSON(t, h, http.MethodPatch, "/api/pomodoro/profiles/"+itoa(defaultProfile.ID), dto.UpdatePomodoroProfileRequest{
		Name: "hacked", WorkSeconds: 1800, ShortBreakSeconds: 300,
		LongBreakSeconds: 1200, SessionsBeforeLongBreak: 4,
	}, token)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("update builtin status = %d, want 400; body=%s", rec.Code, rec.Body.String())
	}

	// Duplicate name.
	rec = doJSON(t, h, http.MethodPost, "/api/pomodoro/profiles", dto.CreatePomodoroProfileRequest{
		Name: "default", WorkSeconds: 1800, ShortBreakSeconds: 300,
		LongBreakSeconds: 1200, SessionsBeforeLongBreak: 4,
	}, token)
	if rec.Code != http.StatusConflict {
		t.Fatalf("duplicate name status = %d, want 409; body=%s", rec.Code, rec.Body.String())
	}
}

type recordingNotifier struct {
	events chan pomodoro.PhaseEndedEvent
}

func (n *recordingNotifier) PhaseEnded(ev pomodoro.PhaseEndedEvent) {
	n.events <- ev
}

func TestPomodoroFinishPhaseNotifierSeam(t *testing.T) {
	notifier := &recordingNotifier{events: make(chan pomodoro.PhaseEndedEvent, 1)}
	h, token, _, conn := newPomodoroTestRouter(t, notifier)

	doJSON(t, h, http.MethodPost, "/api/pomodoro/start", nil, token)
	setRuntime(t, conn, "work", 0, 1500, true, time.Now(), todayKey())

	rec := doJSON(t, h, http.MethodPost, "/api/pomodoro/finish-phase", dto.FinishPhaseRequest{Phase: "work"}, token)
	if rec.Code != http.StatusOK {
		t.Fatalf("finish work status = %d, want 200; body=%s", rec.Code, rec.Body.String())
	}

	select {
	case ev := <-notifier.events:
		if ev.Phase != pomodoro.PhaseWork || ev.SessionID == nil {
			t.Fatalf("unexpected notifier event: %+v", ev)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("notifier was not called")
	}
}

func TestPomodoroRoutesRequireAuth(t *testing.T) {
	h, _, _, _ := newPomodoroTestRouter(t, nil)

	for _, path := range []string{
		"/api/pomodoro/state",
		"/api/pomodoro/config",
		"/api/pomodoro/profiles",
	} {
		rec := doJSON(t, h, http.MethodGet, path, nil, "")
		if rec.Code != http.StatusUnauthorized {
			t.Fatalf("%s without token status = %d, want 401", path, rec.Code)
		}
	}
}
