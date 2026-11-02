/** Calendar dates are normalized to China time, independent of browser locale. */
export function chinaDate() {
  return new Intl.DateTimeFormat("en-CA", {
    timeZone: "Asia/Shanghai",
    year: "numeric",
    month: "2-digit",
    day: "2-digit",
  }).format(new Date())
}
export function chinaClock() {
  return new Intl.DateTimeFormat("en-GB", {
    timeZone: "Asia/Shanghai",
    hour: "2-digit",
    minute: "2-digit",
    hour12: false,
  }).format(new Date())
}
export function datePlus(date: string, days: number) {
  const value = new Date(`${date}T00:00:00Z`)
  value.setUTCDate(value.getUTCDate() + days)
  return value.toISOString().slice(0, 10)
}
export function monday(date: string) {
  const value = new Date(`${date}T00:00:00Z`)
  return datePlus(date, -((value.getUTCDay() + 6) % 7))
}
export function weekNumber(anchor: string, date = chinaDate()) {
  const days = (Date.parse(monday(date)) - Date.parse(monday(anchor))) / 86400000
  return Math.max(1, Math.floor(days / 7) + 1)
}
export function currentSemester() {
  const date = chinaDate()
  const year = Number(date.slice(0, 4)),
    month = Number(date.slice(5, 7))
  return month === 1 ? `${year - 1}S2` : `${year}S${month < 8 ? 1 : 2}`
}
export function defaultAnchor() {
  const date = chinaDate(),
    year = Number(date.slice(0, 4)),
    month = Number(date.slice(5, 7))
  return month < 2 ? `${year - 1}-09-01` : month < 8 ? `${year}-03-01` : `${year}-09-01`
}
export function semesterLabel(value: string) {
  if (value === "LOCAL-VALIDATION") return "本地验证课表"
  const match = /^(\d{4})S([12])$/.exec(value)
  return match ? `${match[1]} 年${match[2] === "1" ? "春季" : "秋季"}学期` : value || "未归档课程"
}
export const weekdays = ["周一", "周二", "周三", "周四", "周五", "周六", "周日"]
export function minutes(time: string) {
  const [h, m] = time.split(":").map(Number)
  return h * 60 + m
}
