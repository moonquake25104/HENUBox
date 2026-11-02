// Package pomodoro ports the Rust pomodoro timer engine
// (src-tauri/src/timer.rs) plus the web-backend orchestration that Rust kept
// in its command layer and tick thread (src-tauri/src/lib.rs,
// src-tauri/src/commands/pomodoro.rs).
//
// Architecture note: the Rust desktop app ran a 1-second background tick
// thread that owned the countdown. The web backend deliberately has no such
// goroutine: the frontend drives the clock locally and reports a finished
// phase via POST /api/pomodoro/finish-phase. Everything the backend must know
// is persisted in pomodoro_runtime_state so that a phase/remaining/is_running
// can be recomputed from elapsed wall time on demand.
package pomodoro

import "time"

// Phase is one of the three timer phases, serialized as the snake_case values
// the frontend src/types/pomodoro.ts expects.
type Phase string

const (
	PhaseWork       Phase = "work"
	PhaseShortBreak Phase = "short_break"
	PhaseLongBreak  Phase = "long_break"
)

// ParsePhase converts a wire string into a Phase.
func ParsePhase(s string) (Phase, bool) {
	switch Phase(s) {
	case PhaseWork, PhaseShortBreak, PhaseLongBreak:
		return Phase(s), true
	default:
		return "", false
	}
}

// String returns the wire representation of the phase.
func (p Phase) String() string { return string(p) }

// Config is the active pomodoro configuration singleton (id = 1), without the
// database id, mirroring the Rust commands::pomodoro::PomodoroConfigData.
type Config struct {
	WorkSeconds             int32
	ShortBreakSeconds       int32
	LongBreakSeconds        int32
	SessionsBeforeLongBreak int32
	AutoStartNextPhase      bool
}

// SecondsFromConfig clamps a signed duration to a non-negative count.
func SecondsFromConfig(v int64) int32 {
	if v < 0 {
		return 0
	}
	return int32(v)
}

// DurationFor returns the configured total length (seconds) of a phase.
func (c Config) DurationFor(p Phase) int32 {
	switch p {
	case PhaseShortBreak:
		return SecondsFromConfig(int64(c.ShortBreakSeconds))
	case PhaseLongBreak:
		return SecondsFromConfig(int64(c.LongBreakSeconds))
	default:
		return SecondsFromConfig(int64(c.WorkSeconds))
	}
}

// PhaseTransition reports one finished phase and the phase the engine switched
// into, mirroring timer::PomodoroPhaseTransition.
type PhaseTransition struct {
	EndedPhase     Phase
	NextPhase      Phase
	EndedSessionID *int64
}

// RestoredState is the persisted snapshot used to rebuild an engine after a
// server restart / page reload, mirroring timer::RestoredPomodoroState.
type RestoredState struct {
	Phase             Phase
	RemainingSeconds  int32
	TotalSeconds      int32
	WasRunning        bool
	CompletedSessions int32
	ActiveSessionID   *int64
	LastSeenAt        *time.Time
}

// Engine is the pure pomodoro state machine. It performs no I/O and spawns no
// goroutine: callers drive it (see Service for the persisted orchestration).
type Engine struct {
	Phase                Phase
	RemainingSeconds     int32
	TotalSeconds         int32
	IsRunning            bool
	CompletedSessions    int32
	ActiveSessionID      *int64
	Interrupted          bool
	InterruptedSessionID *int64
	LastSeenAt           *time.Time
	config               Config
}

// New creates an engine initialised in the Work phase with the given config.
func New(config Config) *Engine {
	work := config.DurationFor(PhaseWork)
	return &Engine{
		Phase:            PhaseWork,
		RemainingSeconds: work,
		TotalSeconds:     work,
		config:           config,
	}
}

// Restore rebuilds an engine from a persisted snapshot. When the timer was
// running at persist time the engine marks itself interrupted and forces
// is_running = false so the frontend can prompt the user instead of silently
// counting time spent offline.
func Restore(config Config, state RestoredState) *Engine {
	interrupted := state.WasRunning
	interruptedSessionID := (*int64)(nil)
	if interrupted {
		interruptedSessionID = state.ActiveSessionID
	}
	return &Engine{
		Phase:                state.Phase,
		RemainingSeconds:     state.RemainingSeconds,
		TotalSeconds:         state.TotalSeconds,
		IsRunning:            false,
		CompletedSessions:    state.CompletedSessions,
		ActiveSessionID:      state.ActiveSessionID,
		Interrupted:          interrupted,
		InterruptedSessionID: interruptedSessionID,
		LastSeenAt:           state.LastSeenAt,
		config:               config,
	}
}

// Config returns the active configuration.
func (e *Engine) Config() Config { return e.config }

// SetInterrupted updates the interruption flag, clearing the associated
// session id when the flag is cleared.
func (e *Engine) SetInterrupted(interrupted bool) {
	e.Interrupted = interrupted
	if !interrupted {
		e.InterruptedSessionID = nil
	}
}

// Start resumes the countdown.
func (e *Engine) Start() { e.IsRunning = true }

// Pause stops the countdown without resetting progress.
func (e *Engine) Pause() { e.IsRunning = false }

// Reset restores the current phase to its full configured duration and stops
// the timer. It does not change the phase or the completed-session counter.
func (e *Engine) Reset() {
	e.IsRunning = false
	e.RemainingSeconds = e.TotalSeconds
	e.ActiveSessionID = nil
}

// Tick advances the timer by one second, mirroring engine.tick(). It returns
// a transition when the current phase just ended and the engine auto-switched
// to the next phase, otherwise nil.
func (e *Engine) Tick() *PhaseTransition {
	if !e.IsRunning {
		return nil
	}
	if e.RemainingSeconds == 0 {
		return nil
	}
	e.RemainingSeconds--
	if e.RemainingSeconds != 0 {
		return nil
	}
	endedPhase := e.Phase
	endedSessionID := e.ActiveSessionID
	next := e.advancePhase(e.config.AutoStartNextPhase)
	return &PhaseTransition{
		EndedPhase:     endedPhase,
		NextPhase:      next,
		EndedSessionID: endedSessionID,
	}
}

// CompletePhasePaused finishes the current phase and switches to the next one,
// keeping the timer paused (used by interruption resolution).
func (e *Engine) CompletePhasePaused() Phase { return e.advancePhase(false) }

// UpdateConfig replaces the configuration and resets the engine to the start
// of a fresh, paused Work phase.
func (e *Engine) UpdateConfig(config Config) {
	e.config = config
	e.Phase = PhaseWork
	e.IsRunning = false
	e.CompletedSessions = 0
	e.ActiveSessionID = nil
	e.Interrupted = false
	e.InterruptedSessionID = nil
	e.LastSeenAt = nil
	work := config.DurationFor(PhaseWork)
	e.RemainingSeconds = work
	e.TotalSeconds = work
}

func (e *Engine) advancePhase(runNext bool) Phase {
	switch e.Phase {
	case PhaseWork:
		e.CompletedSessions++
		threshold := max(e.config.SessionsBeforeLongBreak, int32(1))
		if e.CompletedSessions%threshold == 0 {
			e.Phase = PhaseLongBreak
		} else {
			e.Phase = PhaseShortBreak
		}
		e.TotalSeconds = e.config.DurationFor(e.Phase)
	case PhaseShortBreak, PhaseLongBreak:
		e.Phase = PhaseWork
		e.TotalSeconds = e.config.DurationFor(e.Phase)
	}

	e.RemainingSeconds = e.TotalSeconds
	e.IsRunning = runNext
	e.ActiveSessionID = nil
	return e.Phase
}
