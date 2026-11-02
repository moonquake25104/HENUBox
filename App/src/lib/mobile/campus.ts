import type { SemesterSettings } from "@/lib/api/academic"

export const TERM_WEEKS = 18
export const CAMPUSES = ["郑州", "金明", "明伦"] as const
export type Campus = typeof CAMPUSES[number]
const zhengzhou = ["08:30-09:15", "09:25-10:10", "10:20-11:05", "11:15-12:00", "12:00-12:45", "13:30-14:15", "14:25-15:10", "15:20-16:05", "16:15-17:00", "17:00-17:45", "18:10-18:55", "19:05-19:50", "19:55-20:40"]
const kaifeng = ["08:00-08:45", "08:55-09:40", "10:00-10:45", "10:55-11:40", "11:45-12:30", "14:05-14:50", "15:00-15:45", "15:55-16:40", "17:00-17:45", "17:55-18:40", "19:10-19:55", "20:05-20:50", "21:00-21:45"]
export function campusPeriods(campus: Campus) {
  return (campus === "郑州" ? zhengzhou : kaifeng).map((time, i) => ({
    index: i + 1, start_time: time.split("-")[0], end_time: time.split("-")[1], configured: true,
  }))
}
export function selectedCampus(settings: SemesterSettings): Campus | null {
  if (settings.campus && CAMPUSES.includes(settings.campus)) return settings.campus
  // Older installs did not store a campus. Recognise only an exact preset.
  return CAMPUSES.find(c => settings.periods.every((p, i) => {
    const preset = campusPeriods(c)[i]
    return preset && p.configured && p.start_time === preset.start_time && p.end_time === preset.end_time
  }) && settings.periods.length === 13) ?? null
}
export function semesterSettings(settings: SemesterSettings, campus: Campus, anchor?: string): SemesterSettings {
  const match = /^(\d{4})S([12])$/.exec(settings.semester)
  if (!match) throw new Error("INVALID_SEMESTER")
  const year = Number(match[1]), autumn = match[2] === "2"
  const start = anchor || settings.week_anchor.start_date || (settings.semester === "2026S2" ? "2026-08-31" : null)
  if (!start) throw new Error("WEEK_ANCHOR_REQUIRED")
  return { ...settings, campus, semester_label: `${autumn ? year : year - 1}-${autumn ? year + 1 : year}学年${autumn ? "第一" : "第二"}学期`,
    total_weeks: TERM_WEEKS, period_count: 13, periods: campusPeriods(campus),
    week_anchor: { start_date: start, source: "USER", confirmed: true } }
}
