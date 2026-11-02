import { matchesWeek } from "./offline-timetable"

export function weekRanges(weeks: number[]): string {
  const values = [...new Set(weeks)].sort((a, b) => a - b)
  const ranges: string[] = []
  for (let i = 0; i < values.length; i++) {
    const start = values[i]
    let end = start
    while (values[i + 1] === end + 1) end = values[++i]
    ranges.push(start === end ? `${start}` : `${start}-${end}`)
  }
  return ranges.join("、")
}
export function formatTeachingWeeks(pattern: string): string {
  if (!pattern.trim()) return "1-18周"
  // Preserve odd/even rules, compact only contiguous members.
  const values = Array.from({ length: 18 }, (_, i) => i + 1).filter(w => matchesWeek(pattern, w))
  if (!values.length) return pattern
  if (values.length > 2 && values.every((w, i) => !i || w === values[i - 1] + 2))
    return `${values[0]}-${values.at(-1)}周（${values[0] % 2 ? "单" : "双"}周）`
  return `${weekRanges(values)}周`
}
