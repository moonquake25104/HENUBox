import { registerPlugin } from "@capacitor/core"
import type { Period } from "@/lib/api/academic"

export type ClassroomState = "FREE" | "OCCUPIED" | "UNKNOWN"
export interface QueryCache {
  state: "LIVE" | "FRESH" | "STALE"
  reason: string | null
}
export interface Campus {
  id: string
  name: string
  buildings: { id: string; name: string }[]
}
export interface ClassroomCatalog {
  connection_generation: number
  term: { year: string; term_code: string; label: string }
  fetched_at: string
  cache: QueryCache
  campuses: Campus[]
}
export interface ClassroomRoom {
  id: string
  name: string
  type: string | null
  seats: number | null
  floor: null
  cells: { period: number; status: ClassroomState }[]
}
export interface ClassroomQuery {
  workspace_id: string
  semester: string
  date: string
  campus_id: string
  building_id: string
  refresh?: boolean
}
export interface ClassroomDay {
  connection_generation: number
  semester: string
  date: string
  week: number
  weekday: number
  campus: { id: string; name: string }
  building: { id: string; name: string }
  period_count: 13
  source: "HENU_PUBLIC_QUERY"
  policy: { retained_as_occupied: true }
  fetched_at: string
  cache: QueryCache
  coverage: { roster_rooms: number; complete_rooms: number; unknown_cells: number }
  issues: { code: string; room_id: string | null }[]
  rooms: ClassroomRoom[]
}
interface ClassroomApi {
  getBuildings(input: { workspace_id: string }): Promise<ClassroomCatalog>
  getDay(input: ClassroomQuery): Promise<ClassroomDay>
}
export const classroomBridge = registerPlugin<ClassroomApi>("XiqueClassroom")
export const PERIODS = Array.from({ length: 13 }, (_, i) => i + 1)
export const STATUS = {
  FREE: { label: "空闲", className: "is-free" },
  OCCUPIED: { label: "有安排", className: "is-class" },
  UNKNOWN: { label: "未知", className: "is-unknown" },
} as const

function invalid(): never {
  throw Object.assign(new Error("Invalid classroom response"), { code: "UNRECOGNIZED_DOCUMENT" })
}
function object(value: unknown): value is Record<string, unknown> {
  return !!value && typeof value === "object" && !Array.isArray(value)
}
function text(value: unknown): value is string {
  return typeof value === "string" && value.length > 0 && value.length <= 512
}
function integer(value: unknown): value is number {
  return Number.isSafeInteger(value) && Number(value) >= 0
}
function cache(value: unknown): value is QueryCache {
  return (
    object(value) &&
    ["LIVE", "FRESH", "STALE"].includes(String(value.state)) &&
    (value.reason === null || typeof value.reason === "string")
  )
}
export function validateCatalog(value: unknown): ClassroomCatalog {
  if (
    !object(value) ||
    !integer(value.connection_generation) ||
    !object(value.term) ||
    !text(value.term.year) ||
    !text(value.term.term_code) ||
    !text(value.term.label) ||
    typeof value.fetched_at !== "string" ||
    !Number.isFinite(Date.parse(value.fetched_at)) ||
    !cache(value.cache) ||
    !Array.isArray(value.campuses) ||
    value.campuses.length > 100
  )
    invalid()
  const campusIds = new Set<string>()
  for (const campus of value.campuses) {
    if (
      !object(campus) ||
      !text(campus.id) ||
      !text(campus.name) ||
      campusIds.has(campus.id) ||
      !Array.isArray(campus.buildings) ||
      campus.buildings.length > 1000
    )
      invalid()
    campusIds.add(campus.id)
    const ids = new Set<string>()
    for (const b of campus.buildings) {
      if (!object(b) || !text(b.id) || !text(b.name) || ids.has(b.id)) invalid()
      ids.add(b.id)
    }
  }
  return value as unknown as ClassroomCatalog
}
export function sameQuery(day: ClassroomDay, query: ClassroomQuery) {
  return (
    day.semester === query.semester &&
    day.date === query.date &&
    day.campus.id === query.campus_id &&
    day.building.id === query.building_id
  )
}
export function validateDay(value: unknown, query: ClassroomQuery): ClassroomDay {
  if (
    !object(value) ||
    !integer(value.connection_generation) ||
    value.source !== "HENU_PUBLIC_QUERY" ||
    value.period_count !== 13 ||
    !object(value.policy) ||
    value.policy.retained_as_occupied !== true ||
    !text(value.semester) ||
    !text(value.date) ||
    !integer(value.week) ||
    !integer(value.weekday) ||
    Number(value.week) < 1 ||
    Number(value.weekday) < 1 ||
    Number(value.weekday) > 7 ||
    !object(value.campus) ||
    !text(value.campus.id) ||
    !text(value.campus.name) ||
    !object(value.building) ||
    !text(value.building.id) ||
    !text(value.building.name) ||
    !cache(value.cache) ||
    typeof value.fetched_at !== "string" ||
    !Number.isFinite(Date.parse(value.fetched_at)) ||
    !Array.isArray(value.issues) ||
    !Array.isArray(value.rooms) ||
    value.rooms.length > 2000 ||
    !object(value.coverage)
  )
    invalid()
  const ids = new Set<string>()
  let unknown = 0,
    complete = 0
  for (const room of value.rooms) {
    if (
      !object(room) ||
      !text(room.id) ||
      !text(room.name) ||
      ids.has(room.id) ||
      room.floor !== null ||
      !(room.type === null || text(room.type)) ||
      !(room.seats === null || integer(room.seats)) ||
      !Array.isArray(room.cells) ||
      room.cells.length !== 13
    )
      invalid()
    ids.add(room.id)
    const periods = new Set<number>()
    let roomUnknown = 0
    for (const cell of room.cells) {
      if (
        !object(cell) ||
        !integer(cell.period) ||
        cell.period < 1 ||
        cell.period > 13 ||
        periods.has(cell.period) ||
        !["FREE", "OCCUPIED", "UNKNOWN"].includes(String(cell.status))
      )
        invalid()
      periods.add(cell.period)
      if (cell.status === "UNKNOWN") roomUnknown++
    }
    unknown += roomUnknown
    if (!roomUnknown) complete++
  }
  if (
    value.coverage.roster_rooms !== value.rooms.length ||
    value.coverage.complete_rooms !== complete ||
    value.coverage.unknown_cells !== unknown
  )
    invalid()
  const day = value as unknown as ClassroomDay
  if (!sameQuery(day, query)) invalid()
  return day
}
export function roomStatus(room: ClassroomRoom, start: number, end: number): ClassroomState {
  const cells = PERIODS.filter((p) => p >= start && p <= end).map(
    (p) => room.cells.find((c) => c.period === p)?.status ?? "UNKNOWN",
  )
  if (cells.includes("OCCUPIED")) return "OCCUPIED"
  return cells.length && cells.every((s) => s === "FREE") ? "FREE" : "UNKNOWN"
}
export function groupRooms(rooms: ClassroomRoom[], start: number, end: number) {
  const rank = { FREE: 0, OCCUPIED: 1, UNKNOWN: 2 }
  return [...new Set(rooms.map((r) => r.type || "类型未标明"))].map((type) => ({
    type,
    rooms: rooms
      .filter((r) => (r.type || "类型未标明") === type)
      .sort(
        (a, b) =>
          rank[roomStatus(a, start, end)] - rank[roomStatus(b, start, end)] ||
          a.name.localeCompare(b.name, "zh-CN", { numeric: true }),
      ),
  }))
}
export function currentPeriod(periods: Period[], clock: string): number | null {
  const ready = [...periods]
    .filter((p) => p.configured && p.start_time && p.end_time)
    .sort((a, b) => a.index - b.index)
  return (
    ready.find((p) => p.start_time! <= clock && clock < p.end_time!)?.index ??
    ready.find((p) => p.start_time! > clock)?.index ??
    null
  )
}
export function errorCode(error: unknown) {
  return object(error) && typeof error.code === "string" ? error.code : "UNAVAILABLE"
}
export function classroomError(error: unknown, connected: boolean) {
  const code = errorCode(error)
  if (code === "UPSTREAM_AUTH_REQUIRED" || code === "CONNECTION_CHANGED")
    return {
      title: "学校连接需要更新",
      detail: "使用已有的河大连接重新登录，课表和教室查询共用一次授权。",
      action: "connect",
    }
  if (code === "NOT_CONFIGURED")
    return connected
      ? {
          title: "先确认学期设置",
          detail: "请在学校课表中核对学期、第一周日期与总周数。",
          action: "settings",
        }
      : {
          title: "连接河南大学",
          detail: "使用课表已有的学校连接，无需另建账号。",
          action: "connect",
        }
  if (code === "DATE_OUTSIDE_TERM" || code === "UNSUPPORTED_TERM")
    return {
      title: "这一天不在可查询学期内",
      detail: "目前支持已连接的学校学期，请更换日期或核对学期设置。",
      action: "settings",
    }
  if (code === "UNIMPLEMENTED" || code === "UNSUPPORTED_DEVICE")
    return {
      title: "当前应用暂不支持查询",
      detail: "请更新HENUBox后再试，已保存的课表仍可查看。",
      action: "none",
    }
  if (code === "UPSTREAM_TLS_ERROR")
    return {
      title: "学校安全连接失败",
      detail: "暂时无法验证学校连接，请稍后重试。",
      action: "retry",
    }
  if (code === "UNRECOGNIZED_DOCUMENT")
    return {
      title: "学校返回的数据暂时无法识别",
      detail: "请重试；缺少的信息不会显示为空闲。",
      action: "retry",
    }
  return {
    title: "暂时无法查询",
    detail: "请检查网络后重试。教室查询需要联网，本机课表仍可离线查看。",
    action: "retry",
  }
}
