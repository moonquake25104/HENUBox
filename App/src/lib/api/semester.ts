import type {
  CreateTermPhaseRequest,
  CurrentPhaseStatus,
  SemesterContext,
  TermPhase,
  UpdateTermPhaseRequest,
} from "@/types/notification"
import { apiFetch, buildQuery } from "./client"
import type { StatusResponse } from "./client"

export function listSemesterContexts(): Promise<SemesterContext[]> {
  return apiFetch<SemesterContext[]>("/api/semesters")
}

export function listTermPhases(termLabel: string): Promise<TermPhase[]> {
  const query = buildQuery({ term_label: termLabel })
  return apiFetch<TermPhase[]>(`/api/term-phases${query}`)
}

export function getCurrentPhaseStatus(source: string): Promise<CurrentPhaseStatus> {
  const query = buildQuery({ source })
  return apiFetch<CurrentPhaseStatus>(`/api/term-phases/current-status${query}`)
}

export function createTermPhase(req: CreateTermPhaseRequest): Promise<TermPhase> {
  return apiFetch<TermPhase>("/api/term-phases", {
    method: "POST",
    body: JSON.stringify(req),
  })
}

export function updateTermPhase(id: number, req: UpdateTermPhaseRequest): Promise<TermPhase> {
  return apiFetch<TermPhase>(`/api/term-phases/${id}`, {
    method: "PATCH",
    body: JSON.stringify(req),
  })
}

export function deleteTermPhase(id: number): Promise<StatusResponse> {
  return apiFetch<StatusResponse>(`/api/term-phases/${id}`, { method: "DELETE" })
}
