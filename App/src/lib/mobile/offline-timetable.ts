import { Capacitor, registerPlugin } from "@capacitor/core"
import type { Course } from "@/types/course"
import type { WeekScheduleResponse } from "@/types/schedule"

export interface TimetableSnapshot {
  schema: 1
  saved_at: string
  courses: Course[]
}
interface TimetablePlugin {
  read(input: { expected_generation: number }): Promise<{ snapshot: TimetableSnapshot | null }>
  write(input: { expected_generation: number; snapshot: TimetableSnapshot }): Promise<{ saved: boolean }>
}
const bridge = registerPlugin<TimetablePlugin>("XiqueTimetable")
let snapshot: TimetableSnapshot | null = null
let generation = -1
let epoch = 0

export const offlineTimetable = () => snapshot
export function forgetOfflineTimetable() {
  epoch++
  snapshot = null
  generation = -1
}
export function validSnapshot(value: unknown): value is TimetableSnapshot {
  if (!value || typeof value !== "object") return false
  const data = value as TimetableSnapshot
  return data.schema === 1 && typeof data.saved_at === "string" && Number.isFinite(Date.parse(data.saved_at)) &&
    Array.isArray(data.courses) && data.courses.length <= 10000 &&
    new Set(data.courses.map((c) => c?.id)).size === data.courses.length && data.courses.every((c) =>
      c && Number.isSafeInteger(c.id) && c.id > 0 &&
      ["sync_id", "name", "semester", "week_pattern", "semester_start_date", "teacher", "location", "color", "created_at", "updated_at"].every((key) => typeof c[key as keyof Course] === "string") &&
      [c.start_time, c.end_time].every((time) => typeof time === "string" && /^(?:[01]\d|2[0-3]):[0-5]\d$/.test(time)) &&
      c.end_time > c.start_time && Number.isInteger(c.day_of_week) && c.day_of_week >= 1 && c.day_of_week <= 7 &&
      /^\d{4}-\d{2}-\d{2}$/.test(c.semester_start_date) && Number.isFinite(Date.parse(c.semester_start_date)) && c.deleted_at === null)
}
export async function restoreOfflineTimetable(expectedGeneration: number) {
  forgetOfflineTimetable()
  const current = epoch
  if (!Capacitor.isPluginAvailable("XiqueTimetable")) throw new Error("本机课表组件未就绪，请更新安装包。")
  const result = await bridge.read({ expected_generation: expectedGeneration })
  if (epoch !== current) throw new Error("登录状态已变化，请重新操作。")
  if (result.snapshot !== null && !validSnapshot(result.snapshot)) throw new Error("本机课表未能完整读取，请重新同步。")
  snapshot = result.snapshot
  generation = expectedGeneration
}
export async function saveOfflineTimetable(courses: Course[]): Promise<TimetableSnapshot> {
  const expected = generation
  if (expected < 0) throw new Error("本机课表存储尚未就绪。")
  const value: TimetableSnapshot = { schema: 1, saved_at: new Date().toISOString(), courses }
  if (!validSnapshot(value)) throw new Error("课表数据不完整，已保留上次保存的课表。")
  const result = await bridge.write({ expected_generation: expected, snapshot: value })
  if (generation !== expected) throw new Error("登录状态已变化，请重新操作。")
  if (!result.saved) throw new Error("课表尚未保存到本机，请重试。")
  snapshot = value
  return value
}

/** Same segment and parity semantics as the Go calendar domain. */
export function matchesWeek(pattern: string, week: number): boolean {
  const text = pattern.replaceAll(" ", "")
  if (!text) return true
  return text.split(/[,，;；、/]/).filter(Boolean).some((segment) => {
    const pos = segment.indexOf("周")
    const range = (pos < 0 ? segment : segment.slice(0, pos)).replace(/^第/, "")
    const tail = pos < 0 ? "" : segment.slice(pos + 1)
    let start: number, end: number
    if (["单", "双", "全"].includes(range)) { start = 1; end = Number.MAX_SAFE_INTEGER }
    else {
      const match = /^(\d+)(?:-(\d+))?$/.exec(range)
      if (!match) return false
      start = Number(match[1]); end = Number(match[2] ?? match[1])
    }
    if (week < start || week > end) return false
    if (range === "单" || tail.includes("单")) return week % 2 === 1
    if (range === "双" || tail.includes("双")) return week % 2 === 0
    return true
  })
}
function plusDays(date: string, days: number) {
  const value = new Date(`${date}T00:00:00Z`)
  value.setUTCDate(value.getUTCDate() + days)
  return value.toISOString().slice(0, 10)
}
export function localWeek(courses: Course[], semester: string, anchor: string, week: number): WeekScheduleResponse {
  if (!Number.isSafeInteger(week) || week < 1 || week > 5200) throw new Error("教学周无效")
  const day = new Date(`${anchor}T00:00:00Z`).getUTCDay()
  const start = plusDays(anchor, (week - 1) * 7 - (day + 6) % 7)
  return {
    week_index: week, semester, semester_start_date: anchor, week_start_date: start,
    week_end_date: plusDays(start, 6), phase_type: "unknown", courses_visible: true,
    items: courses.filter((c) => c.semester === semester && c.deleted_at === null && matchesWeek(c.week_pattern, week))
      .map((c) => ({ kind: "course" as const, id: c.id, course_id: c.id, title: c.name, day_of_week: c.day_of_week,
        start_time: c.start_time, end_time: c.end_time, location: c.location, teacher: c.teacher, color: c.color,
        notes: "", week_pattern: c.week_pattern }))
      .sort((a, b) => a.day_of_week - b.day_of_week || a.start_time.localeCompare(b.start_time)),
  }
}
