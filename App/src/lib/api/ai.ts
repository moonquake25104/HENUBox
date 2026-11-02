import type { AiConfig, AiMorningBrief, UpdateAiConfigRequest } from "@/types/ai"
import { apiFetch, buildQuery } from "./client"
import type { StatusResponse } from "./client"

interface RecoveryKeyResponse {
  recovery_key: string | null
}

export function getAiConfig(): Promise<AiConfig> {
  return apiFetch<AiConfig>("/api/ai/config")
}

export function updateAiConfig(req: UpdateAiConfigRequest): Promise<AiConfig> {
  return apiFetch<AiConfig>("/api/ai/config", {
    method: "PATCH",
    body: JSON.stringify(req),
  })
}

export function getMorningBrief(date?: string): Promise<AiMorningBrief | null> {
  const query = buildQuery({ date })
  return apiFetch<AiMorningBrief | null>(`/api/ai/morning-brief${query}`)
}

export async function getAiRecoveryKey(): Promise<string | null> {
  const res = await apiFetch<RecoveryKeyResponse>("/api/ai/sync-recovery-key")
  return res.recovery_key
}

export function setAiRecoveryKey(hexKey: string): Promise<StatusResponse> {
  return apiFetch<StatusResponse>("/api/ai/sync-recovery-key", {
    method: "POST",
    body: JSON.stringify({ recovery_key: hexKey }),
  })
}
