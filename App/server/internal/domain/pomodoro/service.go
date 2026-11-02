package pomodoro

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"kairos/server/internal/store"
)

// ChinaTZ is the fixed +08:00 timezone the product counts "days" in. Dates
// are stored as UTC timestamps; the daily completed-session window and the
// runtime date_key rollover both use this zone.
var ChinaTZ = time.FixedZone("CST", 8*3600)

// finishGraceSeconds is how far from zero the server still accepts a
// finish-phase report. The frontend clock is authoritative for the countdown,
// so a report may legitimately arrive a few seconds before the server's own
// wall-clock decay reaches zero (network latency, jitter); anything further
// from the end is rejected as an invalid/forged duration.
const finishGraceSeconds = 5

// Store is the subset of the sqlc store the pomodoro service needs. *store.Queries
// satisfies it; tests can substitute a fake.
type Store interface {
	GetPomodoroConfig(ctx context.Context) (store.PomodoroConfig, error)
	UpdatePomodoroConfig(ctx context.Context, arg store.UpdatePomodoroConfigParams) error
	GetRuntimeState(ctx context.Context) (store.PomodoroRuntimeState, error)
	CreateRuntimeState(ctx context.Context, dateKey pgtype.Date) (store.PomodoroRuntimeState, error)
	UpdateRuntimeState(ctx context.Context, arg store.UpdateRuntimeStateParams) error
	CreatePomodoroSession(ctx context.Context, arg store.CreatePomodoroSessionParams) (store.PomodoroSession, error)
	GetPomodoroSession(ctx context.Context, id int64) (store.PomodoroSession, error)
	UpdatePomodoroSessionEnd(ctx context.Context, arg store.UpdatePomodoroSessionEndParams) error
	SetPomodoroSessionTask(ctx context.Context, arg store.SetPomodoroSessionTaskParams) error
	SoftDeletePomodoroSession(ctx context.Context, id int64) error
	FindLatestOpenWorkSession(ctx context.Context) (int64, error)
	CountCompletedWorkSessions(ctx context.Context, arg store.CountCompletedWorkSessionsParams) (int64, error)
}

// PhaseEndedEvent carries the details of a just-finished phase to the W9 email
// notifier. SessionID/StartedAt/TaskID are only set for work phases, which are
// the ones persisted as sessions.
type PhaseEndedEvent struct {
	Phase             Phase
	SessionID         *int64
	TaskID            *int64
	StartedAt         *time.Time
	EndedAt           time.Time
	CompletedSessions int32
}

// Notifier is the W9 email-hook seam. Leave it nil: finish-phase processing
// still works, it just does not notify.
type Notifier interface {
	PhaseEnded(ev PhaseEndedEvent)
}

// Service errors. Handlers map these to HTTP statuses.
var (
	ErrPhaseMismatch       = errors.New("reported phase does not match the running phase")
	ErrNoActiveWorkSession = errors.New("no active work session to close")
	ErrPhaseNotFinished    = errors.New("phase has not finished yet")
	ErrUnknownAction       = errors.New("unknown interruption action")
	ErrResumeAtZero        = errors.New("phase remaining time already elapsed")
)

// Service orchestrates the persisted pomodoro runtime. Each request loads the
// current engine from pomodoro_runtime_state, applies wall-clock decay while it
// was running, performs the mutation, and persists it back. No background
// goroutine ticks the countdown.
type Service struct {
	Q        Store
	Notifier Notifier
}

type loaded struct {
	eng         *Engine
	needPersist bool
}

// State mirrors the frontend src/types/pomodoro.ts PomodoroState.
type State struct {
	Phase                string  `json:"phase"`
	RemainingSeconds     int32   `json:"remaining_seconds"`
	TotalSeconds         int32   `json:"total_seconds"`
	IsRunning            bool    `json:"is_running"`
	CompletedSessions    int32   `json:"completed_sessions"`
	Interrupted          bool    `json:"interrupted"`
	InterruptedSessionID *int64  `json:"interrupted_session_id"`
	LastSeenAt           *string `json:"last_seen_at"`
}

// ConfigFromStore converts the singleton config row into the domain Config.
func ConfigFromStore(c store.PomodoroConfig) Config {
	return Config{
		WorkSeconds:             c.WorkSeconds,
		ShortBreakSeconds:       c.ShortBreakSeconds,
		LongBreakSeconds:        c.LongBreakSeconds,
		SessionsBeforeLongBreak: c.SessionsBeforeLongBreak,
		AutoStartNextPhase:      c.AutoStartNextPhase,
	}
}

// ConfigOf mirrors the Rust command-layer snapshot (no database id).
func ConfigOf(c Config) store.PomodoroConfig {
	return store.PomodoroConfig{
		ID:                      1,
		WorkSeconds:             c.WorkSeconds,
		ShortBreakSeconds:       c.ShortBreakSeconds,
		LongBreakSeconds:        c.LongBreakSeconds,
		SessionsBeforeLongBreak: c.SessionsBeforeLongBreak,
		AutoStartNextPhase:      c.AutoStartNextPhase,
	}
}

// State loads the persisted runtime, applies any wall-clock decay and returns
// a fresh snapshot. This is the page-mount calibration endpoint.
func (s *Service) State(ctx context.Context, now time.Time) (State, error) {
	ld, err := s.load(ctx, now)
	if err != nil {
		return State{}, err
	}
	if ld.needPersist {
		if err := s.persist(ctx, ld.eng, now); err != nil {
			return State{}, err
		}
	}
	return SnapshotOf(ld.eng), nil
}

// Start starts (or resumes) the countdown, mirroring start_pomodoro: it clears
// any interruption, opens a work session when a work phase begins, and records
// the new run baseline.
func (s *Service) Start(ctx context.Context, now time.Time) (State, error) {
	ld, err := s.load(ctx, now)
	if err != nil {
		return State{}, err
	}
	eng := ld.eng
	eng.SetInterrupted(false)

	if eng.IsRunning {
		// Idempotent re-start: keep the original baseline so elapsed time is
		// not silently forgiven.
		if ld.needPersist {
			if err := s.persist(ctx, eng, now); err != nil {
				return State{}, err
			}
		}
		return SnapshotOf(eng), nil
	}

	if eng.Phase == PhaseWork && eng.ActiveSessionID == nil {
		id, err := s.createWorkSession(ctx, now)
		if err != nil {
			return State{}, err
		}
		eng.ActiveSessionID = id
	}
	eng.Start()
	eng.LastSeenAt = &now
	if err := s.persist(ctx, eng, now); err != nil {
		return State{}, err
	}
	return SnapshotOf(eng), nil
}

// Pause stops the countdown without resetting progress, mirroring pause_pomodoro.
func (s *Service) Pause(ctx context.Context, now time.Time) (State, error) {
	ld, err := s.load(ctx, now)
	if err != nil {
		return State{}, err
	}
	ld.eng.SetInterrupted(false)
	ld.eng.Pause()
	if err := s.persist(ctx, ld.eng, now); err != nil {
		return State{}, err
	}
	return SnapshotOf(ld.eng), nil
}

// Reset stops the timer, soft-deletes any open work session and restores the
// current phase to its full duration, mirroring reset_pomodoro.
func (s *Service) Reset(ctx context.Context, now time.Time) (State, error) {
	ld, err := s.load(ctx, now)
	if err != nil {
		return State{}, err
	}
	eng := ld.eng
	if eng.ActiveSessionID != nil {
		if err := s.Q.SoftDeletePomodoroSession(ctx, *eng.ActiveSessionID); err != nil {
			return State{}, err
		}
	}
	eng.SetInterrupted(false)
	eng.Reset()
	eng.LastSeenAt = &now
	if err := s.persist(ctx, eng, now); err != nil {
		return State{}, err
	}
	return SnapshotOf(eng), nil
}

// ResolveInterruption handles the interrupt prompt, mirroring
// resolve_pomodoro_interruption with actions "continue", "discard" and
// "complete".
func (s *Service) ResolveInterruption(ctx context.Context, now time.Time, action string) (State, error) {
	ld, err := s.load(ctx, now)
	if err != nil {
		return State{}, err
	}
	eng := ld.eng

	switch action {
	case "continue":
		if eng.RemainingSeconds == 0 {
			return State{}, fmt.Errorf("%w: 剩余时间已耗尽，请选择丢弃或补记完成", ErrResumeAtZero)
		}
		eng.SetInterrupted(false)
		eng.Pause()
	case "discard":
		if eng.ActiveSessionID != nil {
			if err := s.Q.SoftDeletePomodoroSession(ctx, *eng.ActiveSessionID); err != nil {
				return State{}, err
			}
		}
		eng.SetInterrupted(false)
		eng.Reset()
	case "complete":
		if eng.Phase != PhaseWork || eng.ActiveSessionID == nil {
			return State{}, ErrNoActiveWorkSession
		}
		sid := *eng.ActiveSessionID
		if err := s.Q.UpdatePomodoroSessionEnd(ctx, store.UpdatePomodoroSessionEndParams{
			EndedAt: store.TSOf(now.Format(time.RFC3339)), ID: sid,
		}); err != nil {
			return State{}, err
		}
		eng.ActiveSessionID = nil
		eng.SetInterrupted(false)
		eng.CompletePhasePaused()
	default:
		return State{}, fmt.Errorf("%w: %q", ErrUnknownAction, action)
	}
	eng.LastSeenAt = &now
	if err := s.persist(ctx, eng, now); err != nil {
		return State{}, err
	}
	return SnapshotOf(eng), nil
}

// FinishPhase validates and persists a phase completed by the frontend's local
// clock. For a work phase it closes the open session, then advances the engine
// to the next phase (auto-running when configured) and fires the optional
// Notifier seam.
func (s *Service) FinishPhase(ctx context.Context, now time.Time, phase Phase, completedAt *time.Time, taskID *int64) (State, error) {
	ld, err := s.load(ctx, now)
	if err != nil {
		return State{}, err
	}
	eng := ld.eng

	if eng.Phase != phase {
		return State{}, fmt.Errorf("%w: engine=%s reported=%s", ErrPhaseMismatch, eng.Phase, phase)
	}
	if eng.RemainingSeconds > finishGraceSeconds {
		return State{}, fmt.Errorf("%w: remaining=%d", ErrPhaseNotFinished, eng.RemainingSeconds)
	}

	endAt := now
	if completedAt != nil {
		endAt = *completedAt
	}

	switch phase {
	case PhaseWork:
		sid := eng.ActiveSessionID
		if sid == nil {
			if orphan, err := s.Q.FindLatestOpenWorkSession(ctx); err == nil {
				if sess, err := s.Q.GetPomodoroSession(ctx, orphan); err == nil && withinDay(sess.StartedAt, now) {
					sid = &orphan
				}
			} else if !errors.Is(err, pgx.ErrNoRows) {
				return State{}, err
			}
		}
		if sid == nil {
			return State{}, ErrNoActiveWorkSession
		}
		session, err := s.Q.GetPomodoroSession(ctx, *sid)
		if err != nil {
			return State{}, err
		}
		if err := s.Q.UpdatePomodoroSessionEnd(ctx, store.UpdatePomodoroSessionEndParams{
			EndedAt: store.TSOf(endAt.Format(time.RFC3339)), ID: *sid,
		}); err != nil {
			return State{}, err
		}
		if taskID != nil && !session.TaskID.Valid {
			if err := s.Q.SetPomodoroSessionTask(ctx, store.SetPomodoroSessionTaskParams{
				TaskID: store.Int8Of(*taskID), ID: *sid,
			}); err != nil {
				return State{}, err
			}
		}
		eng.ActiveSessionID = nil
		eng.CompletePhasePaused()
		s.notify(PhaseEndedEvent{
			Phase:             phase,
			SessionID:         sid,
			TaskID:            taskID,
			StartedAt:         timestamptzPtr(session.StartedAt),
			EndedAt:           endAt,
			CompletedSessions: eng.CompletedSessions,
		})

	case PhaseShortBreak, PhaseLongBreak:
		eng.advancePhase(eng.config.AutoStartNextPhase)
		if eng.Phase == PhaseWork && eng.IsRunning && eng.ActiveSessionID == nil {
			id, err := s.createWorkSession(ctx, endAt)
			if err != nil {
				return State{}, err
			}
			eng.ActiveSessionID = id
		}
		s.notify(PhaseEndedEvent{
			Phase:             phase,
			EndedAt:           endAt,
			CompletedSessions: eng.CompletedSessions,
		})
	}

	eng.LastSeenAt = &now
	eng.SetInterrupted(false)
	if err := s.persist(ctx, eng, now); err != nil {
		return State{}, err
	}
	return SnapshotOf(eng), nil
}

// UpdateConfig replaces the singleton config and resets the engine to a fresh,
// paused Work phase (soft-deleting any open session first), mirroring
// update_pomodoro_config.
func (s *Service) UpdateConfig(ctx context.Context, now time.Time, cfg Config) (State, error) {
	ld, err := s.load(ctx, now)
	if err != nil {
		return State{}, err
	}
	eng := ld.eng
	if eng.ActiveSessionID != nil {
		if err := s.Q.SoftDeletePomodoroSession(ctx, *eng.ActiveSessionID); err != nil {
			return State{}, err
		}
	}
	if err := s.Q.UpdatePomodoroConfig(ctx, store.UpdatePomodoroConfigParams{
		WorkSeconds:             cfg.WorkSeconds,
		ShortBreakSeconds:       cfg.ShortBreakSeconds,
		LongBreakSeconds:        cfg.LongBreakSeconds,
		SessionsBeforeLongBreak: cfg.SessionsBeforeLongBreak,
		AutoStartNextPhase:      cfg.AutoStartNextPhase,
	}); err != nil {
		return State{}, err
	}
	eng.UpdateConfig(cfg)
	completed, err := s.Q.CountCompletedWorkSessions(ctx, s.dayWindowParams(now))
	if err != nil {
		return State{}, err
	}
	eng.CompletedSessions = int32(completed)
	eng.LastSeenAt = &now
	if err := s.persist(ctx, eng, now); err != nil {
		return State{}, err
	}
	return SnapshotOf(eng), nil
}

// load rebuilds the engine from the persisted runtime state. Semantics:
//   - a missing runtime row seeds a fresh, paused Work phase;
//   - a runtime row whose date_key is not today (UTC+8) is reset to that fresh
//     state — completed counts are derived from today's session history, so
//     the daily rollover happens automatically;
//   - a running phase is decayed by the wall time elapsed since last_seen_at
//     and parked in the interrupted state (paused), mirroring the Rust boot
//     restore: the frontend prompts the user to continue, discard or
//     backfill-complete the interrupted session.
func (s *Service) load(ctx context.Context, now time.Time) (*loaded, error) {
	cfgRow, err := s.Q.GetPomodoroConfig(ctx)
	if err != nil {
		return nil, err
	}
	cfg := ConfigFromStore(cfgRow)
	key := todayKey(now)

	rs, err := s.Q.GetRuntimeState(ctx)
	if errors.Is(err, pgx.ErrNoRows) {
		if _, err := s.Q.CreateRuntimeState(ctx, store.DateOf(key)); err != nil {
			return nil, err
		}
		rs, err = s.Q.GetRuntimeState(ctx)
		if err != nil {
			return nil, err
		}
	}
	if err != nil {
		return nil, err
	}

	eng := &Engine{
		Phase:            Phase(rs.Phase),
		RemainingSeconds: rs.RemainingSeconds,
		TotalSeconds:     rs.TotalSeconds,
		IsRunning:        rs.IsRunning,
		Interrupted:      rs.Interrupted,
		ActiveSessionID:  int8Ptr(rs.ActiveSessionID),
		LastSeenAt:       timestamptzPtr(rs.LastSeenAt),
		config:           cfg,
	}
	needPersist := false

	if dateKeyOf(rs.DateKey) != key {
		// Day rollover: today's completed count is derived from the session
		// history, so resetting to a fresh Work phase is all that is needed.
		eng = New(cfg)
		needPersist = true
	} else if eng.IsRunning {
		if eng.LastSeenAt != nil {
			elapsed := int(now.Sub(*eng.LastSeenAt).Seconds())
			if elapsed > 0 {
				eng.RemainingSeconds -= int32(elapsed)
				if eng.RemainingSeconds < 0 {
					eng.RemainingSeconds = 0
				}
				// The page was away while the timer was running: mirror the Rust
				// boot restore — pause and surface the interruption prompt so the
				// user can continue (from the decayed remaining), discard or
				// backfill-complete the session.
				eng.IsRunning = false
				eng.Interrupted = true
				eng.InterruptedSessionID = eng.ActiveSessionID
				needPersist = true
			}
		}
	}
	if eng.Interrupted {
		eng.InterruptedSessionID = eng.ActiveSessionID
	}

	completed, err := s.Q.CountCompletedWorkSessions(ctx, s.dayWindowParams(now))
	if err != nil {
		return nil, err
	}
	eng.CompletedSessions = int32(completed)

	return &loaded{eng: eng, needPersist: needPersist}, nil
}

func (s *Service) createWorkSession(ctx context.Context, at time.Time) (*int64, error) {
	session, err := s.Q.CreatePomodoroSession(ctx, store.CreatePomodoroSessionParams{
		SyncID:      "",
		StartedAt:   store.TSOf(at.Format(time.RFC3339)),
		SessionType: string(PhaseWork),
		TaskID:      pgtype.Int8{},
	})
	if err != nil {
		return nil, err
	}
	return &session.ID, nil
}

// persist writes the engine back to the runtime row, re-baselining last_seen_at
// to now (only meaningful while running).
func (s *Service) persist(ctx context.Context, eng *Engine, now time.Time) error {
	return s.Q.UpdateRuntimeState(ctx, store.UpdateRuntimeStateParams{
		Phase:            eng.Phase.String(),
		RemainingSeconds: eng.RemainingSeconds,
		TotalSeconds:     eng.TotalSeconds,
		IsRunning:        eng.IsRunning,
		ActiveSessionID:  int8Of(eng.ActiveSessionID),
		DateKey:          store.DateOf(todayKey(now)),
		Interrupted:      eng.Interrupted,
	})
}

func (s *Service) notify(ev PhaseEndedEvent) {
	if s.Notifier == nil {
		return
	}
	go func() {
		s.Notifier.PhaseEnded(ev)
	}()
}

func (s *Service) dayWindowParams(now time.Time) store.CountCompletedWorkSessionsParams {
	start, end := dayWindow(now)
	return store.CountCompletedWorkSessionsParams{
		WindowStart: pgtype.Timestamptz{Time: start, Valid: true},
		WindowEnd:   pgtype.Timestamptz{Time: end, Valid: true},
	}
}

// todayKey returns the local (UTC+8) calendar date of now as YYYY-MM-DD.
func todayKey(now time.Time) string {
	return now.In(ChinaTZ).Format("2006-01-02")
}

// dayWindow returns the [midnight, next midnight) UTC bounds of the UTC+8 day
// containing now.
func dayWindow(now time.Time) (time.Time, time.Time) {
	local := now.In(ChinaTZ)
	start := time.Date(local.Year(), local.Month(), local.Day(), 0, 0, 0, 0, ChinaTZ)
	return start.UTC(), start.AddDate(0, 0, 1).UTC()
}

// SnapshotOf serializes an engine for the frontend, aligning interrupted
// bookkeeping with the persisted active session.
func SnapshotOf(e *Engine) State {
	interruptedSessionID := e.InterruptedSessionID
	if e.Interrupted && interruptedSessionID == nil {
		interruptedSessionID = e.ActiveSessionID
	}
	st := State{
		Phase:                e.Phase.String(),
		RemainingSeconds:     e.RemainingSeconds,
		TotalSeconds:         e.TotalSeconds,
		IsRunning:            e.IsRunning,
		CompletedSessions:    e.CompletedSessions,
		Interrupted:          e.Interrupted,
		InterruptedSessionID: interruptedSessionID,
	}
	if e.LastSeenAt != nil {
		v := e.LastSeenAt.UTC().Format(time.RFC3339)
		st.LastSeenAt = &v
	}
	return st
}

func dateKeyOf(d pgtype.Date) string {
	if !d.Valid {
		return ""
	}
	return d.Time.In(ChinaTZ).Format("2006-01-02")
}

func withinDay(t pgtype.Timestamptz, now time.Time) bool {
	if !t.Valid {
		return false
	}
	start, end := dayWindow(now)
	return !t.Time.Before(start) && t.Time.Before(end)
}

func int8Ptr(v pgtype.Int8) *int64 {
	if !v.Valid {
		return nil
	}
	return &v.Int64
}

func int8Of(v *int64) pgtype.Int8 {
	if v == nil {
		return pgtype.Int8{}
	}
	return pgtype.Int8{Int64: *v, Valid: true}
}

func timestamptzPtr(t pgtype.Timestamptz) *time.Time {
	if !t.Valid {
		return nil
	}
	v := t.Time
	return &v
}
