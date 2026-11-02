import type {
  PomodoroConfig,
  PomodoroPhase,
  PomodoroState,
  ResolvePomodoroInterruptionRequest,
} from "@/types/pomodoro"
import type {
  CreatePomodoroProfileRequest,
  PomodoroProfile,
  UpdatePomodoroProfileRequest,
} from "@/types/notification"
import { apiFetch } from "./client"
import type { StatusResponse } from "./client"

export interface FinishPhaseRequest {
  phase: PomodoroPhase
  completed_at?: string
  task_id?: number
}

export function getState(): Promise<PomodoroState> {
  return apiFetch<PomodoroState>("/api/pomodoro/state")
}

export function start(): Promise<PomodoroState> {
  return apiFetch<PomodoroState>("/api/pomodoro/start", { method: "POST" })
}

export function pause(): Promise<PomodoroState> {
  return apiFetch<PomodoroState>("/api/pomodoro/pause", { method: "POST" })
}

export function reset(): Promise<PomodoroState> {
  return apiFetch<PomodoroState>("/api/pomodoro/reset", { method: "POST" })
}

export function interrupt(
  action: ResolvePomodoroInterruptionRequest["action"],
): Promise<PomodoroState> {
  return apiFetch<PomodoroState>("/api/pomodoro/interrupt", {
    method: "POST",
    body: JSON.stringify({ action }),
  })
}

export function getConfig(): Promise<PomodoroConfig> {
  return apiFetch<PomodoroConfig>("/api/pomodoro/config")
}

export function updateConfig(config: PomodoroConfig): Promise<PomodoroState> {
  return apiFetch<PomodoroState>("/api/pomodoro/config", {
    method: "PATCH",
    body: JSON.stringify(config),
  })
}

export function listProfiles(): Promise<PomodoroProfile[]> {
  return apiFetch<PomodoroProfile[]>("/api/pomodoro/profiles")
}

export function createProfile(req: CreatePomodoroProfileRequest): Promise<PomodoroProfile> {
  return apiFetch<PomodoroProfile>("/api/pomodoro/profiles", {
    method: "POST",
    body: JSON.stringify(req),
  })
}

export function updateProfile(
  id: number,
  req: UpdatePomodoroProfileRequest,
): Promise<PomodoroProfile> {
  return apiFetch<PomodoroProfile>(`/api/pomodoro/profiles/${id}`, {
    method: "PATCH",
    body: JSON.stringify(req),
  })
}

export function deleteProfile(id: number): Promise<StatusResponse> {
  return apiFetch<StatusResponse>(`/api/pomodoro/profiles/${id}`, { method: "DELETE" })
}

export function finishPhase(payload: FinishPhaseRequest): Promise<PomodoroState> {
  return apiFetch<PomodoroState>("/api/pomodoro/finish-phase", {
    method: "POST",
    body: JSON.stringify(payload),
  })
}
