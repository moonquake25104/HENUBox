import type { Period } from "@/lib/api/academic"

export interface TimeSlot {
  index: number | null
  start: number
  end: number
  /** Presentation-only rest band; it is not a school period. */
  break?: "lunch" | "evening"
  /** Relative layout height; ordinary periods are 1. */
  weight?: number
}
interface CourseTime {
  start_time: string
  end_time: string
}
export function clockMinutes(value: string) {
  if (!/^([01]\d|2[0-3]):[0-5]\d$/.test(value)) return NaN
  return Number(value.slice(0, 2)) * 60 + Number(value.slice(3))
}
export function clockLabel(value: number) {
  return `${String(Math.floor(value / 60)).padStart(2, "0")}:${String(value % 60).padStart(2, "0")}`
}
export function configuredSlots(periods: Period[], count: number): TimeSlot[] {
  const rows = [...periods].sort((a, b) => a.index - b.index)
  if (
    !rows.length ||
    rows.length !== count ||
    rows.some(
      (row, i) =>
        !row.configured ||
        row.index !== i + 1 ||
        !row.start_time ||
        !row.end_time ||
        !Number.isFinite(clockMinutes(row.start_time)) ||
        !Number.isFinite(clockMinutes(row.end_time)) ||
        clockMinutes(row.start_time) >= clockMinutes(row.end_time) ||
        (i > 0 && clockMinutes(rows[i - 1].end_time!) > clockMinutes(row.start_time)),
    )
  )
    return []
  return rows.map((row) => ({
    index: row.index,
    start: clockMinutes(row.start_time!),
    end: clockMinutes(row.end_time!),
  }))
}
export function weekSlots(configured: TimeSlot[], courses: CourseTime[]): TimeSlot[] {
  const boundaries = courses
    .flatMap((item) => [clockMinutes(item.start_time), clockMinutes(item.end_time)])
    .filter(Number.isFinite)
  if (!configured.length) {
    // Unconfigured/manual semesters use observed course boundaries, never guessed school times.
    const times = [...new Set(boundaries)].sort((a, b) => a - b)
    return times
      .slice(0, -1)
      .flatMap((start, i) =>
        courses.some(
          (item) =>
            clockMinutes(item.start_time) < times[i + 1] && clockMinutes(item.end_time) > start,
        )
          ? [{ index: null, start, end: times[i + 1] }]
          : [],
      )
  }
  const slots = [...configured]
  const gaps = [
    { start: Math.min(configured[0].start, ...boundaries), end: configured[0].start },
    ...configured.slice(0, -1).map((row, i) => ({ start: row.end, end: configured[i + 1].start })),
    { start: configured.at(-1)!.end, end: Math.max(configured.at(-1)!.end, ...boundaries) },
  ]
  for (const gap of gaps)
    if (
      gap.start < gap.end &&
      boundaries.some(
        (time) =>
          (time > gap.start && time < gap.end) ||
          (time === gap.start && time < configured[0].start) ||
          (time === gap.end && time > configured.at(-1)!.end),
      )
    )
      slots.push({ index: null, ...gap })
  return slots.sort((a, b) => a.start - b.start)
}
/** Insert the two visible rest bands without changing any actual course time. */
export function addBreaks(slots: TimeSlot[]): TimeSlot[] {
  const result = [...slots]
  const add = (after: number, label: "lunch" | "evening") => {
    const before = slots.find((slot) => slot.index === after)
    const next = slots.find((slot) => slot.index === after + 1)
    if (!before || !next || before.end >= next.start) return
    if (result.some((slot) => slot.start === before.end && slot.end === next.start)) return
    result.push({ index: null, start: before.end, end: next.start, break: label, weight: 0.4 })
  }
  add(5, "lunch")
  add(10, "evening")
  return result.sort((a, b) => a.start - b.start)
}

export function slotTotal(slots: TimeSlot[]) {
  return slots.reduce((total, slot) => total + (slot.weight ?? 1), 0)
}

export function slotPosition(slots: TimeSlot[], time: number) {
  let offset = 0
  for (const slot of slots) {
    const weight = slot.weight ?? 1
    if (time < slot.start) return offset
    if (time <= slot.end) return offset + ((time - slot.start) / (slot.end - slot.start)) * weight
    offset += weight
  }
  return offset
}
