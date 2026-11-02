package dto

import (
	"kairos/server/internal/domain/pomodoro"
	"kairos/server/internal/store"
)

// PomodoroConfig mirrors the frontend src/types/pomodoro.ts PomodoroConfig
// (the Rust commands::pomodoro::PomodoroConfigData shape, without the db id).
type PomodoroConfig struct {
	WorkSeconds             int32 `json:"work_seconds"`
	ShortBreakSeconds       int32 `json:"short_break_seconds"`
	LongBreakSeconds        int32 `json:"long_break_seconds"`
	SessionsBeforeLongBreak int32 `json:"sessions_before_long_break"`
	AutoStartNextPhase      bool  `json:"auto_start_next_phase"`
}

// PomodoroProfile mirrors the frontend src/types/notification.ts PomodoroProfile.
type PomodoroProfile struct {
	ID                      int64  `json:"id"`
	Name                    string `json:"name"`
	WorkSeconds             int32  `json:"work_seconds"`
	ShortBreakSeconds       int32  `json:"short_break_seconds"`
	LongBreakSeconds        int32  `json:"long_break_seconds"`
	SessionsBeforeLongBreak int32  `json:"sessions_before_long_break"`
	IsBuiltin               bool   `json:"is_builtin"`
	CreatedAt               string `json:"created_at"`
	UpdatedAt               string `json:"updated_at"`
}

// CreatePomodoroProfileRequest is the body of POST /api/pomodoro/profiles.
type CreatePomodoroProfileRequest struct {
	Name                    string `json:"name"`
	WorkSeconds             int32  `json:"work_seconds"`
	ShortBreakSeconds       int32  `json:"short_break_seconds"`
	LongBreakSeconds        int32  `json:"long_break_seconds"`
	SessionsBeforeLongBreak int32  `json:"sessions_before_long_break"`
}

// UpdatePomodoroProfileRequest is the body of PATCH /api/pomodoro/profiles/{id}.
type UpdatePomodoroProfileRequest struct {
	Name                    string `json:"name"`
	WorkSeconds             int32  `json:"work_seconds"`
	ShortBreakSeconds       int32  `json:"short_break_seconds"`
	LongBreakSeconds        int32  `json:"long_break_seconds"`
	SessionsBeforeLongBreak int32  `json:"sessions_before_long_break"`
}

// ResolveInterruptionRequest is the body of POST /api/pomodoro/interrupt.
type ResolveInterruptionRequest struct {
	Action string `json:"action"`
}

// FinishPhaseRequest is the body of POST /api/pomodoro/finish-phase.
type FinishPhaseRequest struct {
	Phase       string  `json:"phase"`
	CompletedAt *string `json:"completed_at"`
	TaskID      *int64  `json:"task_id"`
}

// FromPomodoroConfig converts a store config row into the API representation.
func FromPomodoroConfig(c store.PomodoroConfig) PomodoroConfig {
	return PomodoroConfig{
		WorkSeconds:             c.WorkSeconds,
		ShortBreakSeconds:       c.ShortBreakSeconds,
		LongBreakSeconds:        c.LongBreakSeconds,
		SessionsBeforeLongBreak: c.SessionsBeforeLongBreak,
		AutoStartNextPhase:      c.AutoStartNextPhase,
	}
}

// FromPomodoroProfile converts a store profile row into the API representation.
func FromPomodoroProfile(p store.PomodoroProfile) PomodoroProfile {
	return PomodoroProfile{
		ID:                      p.ID,
		Name:                    p.Name,
		WorkSeconds:             p.WorkSeconds,
		ShortBreakSeconds:       p.ShortBreakSeconds,
		LongBreakSeconds:        p.LongBreakSeconds,
		SessionsBeforeLongBreak: p.SessionsBeforeLongBreak,
		IsBuiltin:               p.IsBuiltin,
		CreatedAt:               store.TSString(p.CreatedAt),
		UpdatedAt:               store.TSString(p.UpdatedAt),
	}
}

// ToConfig converts the API config shape into the domain Config.
func (c PomodoroConfig) ToConfig() pomodoro.Config {
	return pomodoro.Config{
		WorkSeconds:             c.WorkSeconds,
		ShortBreakSeconds:       c.ShortBreakSeconds,
		LongBreakSeconds:        c.LongBreakSeconds,
		SessionsBeforeLongBreak: c.SessionsBeforeLongBreak,
		AutoStartNextPhase:      c.AutoStartNextPhase,
	}
}
