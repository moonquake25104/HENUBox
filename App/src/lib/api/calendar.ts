import type { CalendarWeekCmd, CalendarWeekResponse } from "@/types/schedule"
import { apiFetch, buildQuery } from "./client"

export function getCalendarWeek(cmd: CalendarWeekCmd): Promise<CalendarWeekResponse> {
  const query = buildQuery({
    semester: cmd.semester,
    week_index: cmd.week_index,
    semester_start_date: cmd.semester_start_date,
    week_start_date: cmd.week_start_date,
  })
  return apiFetch<CalendarWeekResponse>(`/api/calendar/week-events${query}`)
}
