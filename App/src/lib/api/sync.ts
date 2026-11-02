import type { SyncConfig, SyncResult, UpdateSyncConfigRequest } from "@/types/sync"
import { apiFetch } from "./client"

export function getSyncConfig(): Promise<SyncConfig> {
  return apiFetch<SyncConfig>("/api/sync/config")
}

export function updateSyncConfig(config: UpdateSyncConfigRequest): Promise<SyncConfig> {
  return apiFetch<SyncConfig>("/api/sync/config", {
    method: "PATCH",
    body: JSON.stringify(config),
  })
}

export function testSyncConnection(): Promise<boolean> {
  return apiFetch<boolean>("/api/sync/test", { method: "POST" })
}

export function syncNow(): Promise<SyncResult> {
  return apiFetch<SyncResult>("/api/sync/now", { method: "POST" })
}
