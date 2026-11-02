import { apiFetch } from "./client"

export interface HenuError {
  code: string
  message: string
  occurred_at: string | null
}
export interface HenuConnectionState {
  school: string
  connection_state:
    "UNAVAILABLE" | "DISCONNECTED" | "CONNECTED" | "AUTH_REQUIRED" | "TEMPORARY_FAILURE"
  capabilities: { file_import: boolean; direct_fetch: boolean; interactive_login: boolean }
  active_login_id?: string | null
  active_job_id?: string | null
  last_verified_at?: string | null
  last_checked_at?: string | null
  current_term?: { upstream_year: string; upstream_term: string; label: string } | null
  last_error?: HenuError | null
}
export interface HenuLogin {
  login_id: string
  state: "WAITING_FOR_USER" | "VERIFYING" | "SUCCEEDED" | "FAILED" | "CANCELLED" | "EXPIRED"
  created_at: string
  expires_at: string
  finished_at: string | null
  error: HenuError | null
}
export interface HenuJob {
  job_id: string
  semester: string
  state: "QUEUED" | "RUNNING" | "SUCCEEDED" | "FAILED" | "CANCELLED"
  created_at: string
  started_at?: string | null
  finished_at?: string | null
  preview_id?: string | null
  error?: HenuError | null
}
export const loginActive = (value: HenuLogin | null) =>
  value?.state === "WAITING_FOR_USER" || value?.state === "VERIFYING"
export const jobActive = (value: HenuJob | null) =>
  value?.state === "QUEUED" || value?.state === "RUNNING"
const root = "/api/academic/henu"
export const getHenuConnection = () => apiFetch<HenuConnectionState>(`${root}/connection`)
export const disconnectHenu = () => apiFetch<void>(`${root}/connection`, { method: "DELETE" })
export const startHenuLogin = (key: string) =>
  apiFetch<HenuLogin>(`${root}/login-sessions`, {
    method: "POST",
    body: JSON.stringify({ idempotency_key: key }),
  })
export const getHenuLogin = (id: string) =>
  apiFetch<HenuLogin>(`${root}/login-sessions/${encodeURIComponent(id)}`)
export const cancelHenuLogin = (id: string) =>
  apiFetch<void>(`${root}/login-sessions/${encodeURIComponent(id)}`, { method: "DELETE" })
export interface HenuSyncRequest {
  semester: string
  expected_aggregate_revision: number
  idempotency_key: string
}
export const startHenuJob = (body: HenuSyncRequest) =>
  apiFetch<HenuJob>(`${root}/sync-jobs`, { method: "POST", body: JSON.stringify(body) })
export const getHenuJob = (id: string) =>
  apiFetch<HenuJob>(`${root}/sync-jobs/${encodeURIComponent(id)}`)
