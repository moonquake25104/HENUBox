import type { TodayBriefingResponse } from "@/types/briefing"
import { apiFetch } from "./client"

export function getTodayBriefing(): Promise<TodayBriefingResponse> {
  return apiFetch<TodayBriefingResponse>("/api/briefing/today")
}
