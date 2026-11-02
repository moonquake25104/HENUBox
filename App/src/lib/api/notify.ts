import type { NotificationConfig, UpdateNotificationConfig } from "@/types/notification"
import { apiFetch } from "./client"

export function getNotifyConfig(): Promise<NotificationConfig> {
  return apiFetch<NotificationConfig>("/api/notify/config")
}

export function updateNotifyConfig(req: UpdateNotificationConfig): Promise<NotificationConfig> {
  return apiFetch<NotificationConfig>("/api/notify/config", {
    method: "PATCH",
    body: JSON.stringify(req),
  })
}
