import { Capacitor, registerPlugin } from "@capacitor/core"

export type WidgetOpacity = 100 | 80 | 65

const bridge = registerPlugin<{
  share(value: { text: string }): Promise<void>
  openLink(value: { url: string }): Promise<void>
  pinFocus(): Promise<{ requested: boolean }>
  pinTimetable(): Promise<{ requested: boolean }>
  pinWeekTimetable(value: { days: 5 | 7 }): Promise<{ requested: boolean }>
  getWidgetOpacity(): Promise<{ opacity: WidgetOpacity }>
  setWidgetOpacity(value: { opacity: WidgetOpacity }): Promise<void>
}>("XiqueCampusTools")

export const campusToolsAvailable = () => Capacitor.isPluginAvailable("XiqueCampusTools")
export const pinFocusWidget = () => bridge.pinFocus()
export const pinTimetableWidget = () => bridge.pinTimetable()
export const pinWeekTimetableWidget = (days: 5 | 7) => bridge.pinWeekTimetable({ days })
export const readWidgetOpacity = () => bridge.getWidgetOpacity()
export const saveWidgetOpacity = (opacity: WidgetOpacity) => bridge.setWidgetOpacity({ opacity })

export function campusLink(value: string) {
  try {
    const raw = value.trim().match(/https:\/\/[^\s，。！？、；<>"）]+/i)?.[0]
    if (!raw) return ""
    const url = new URL(raw)
    return url.protocol === "https:" && url.hostname && !url.username && !url.password && url.href.length <= 2048 ? url.href : ""
  } catch { return "" }
}

export async function shareCampusText(text: string) {
  if (campusToolsAvailable()) { await bridge.share({ text }); return false }
  if (navigator.share) { await navigator.share({ text }); return false }
  await navigator.clipboard.writeText(text)
  return true
}

export async function openCampusLink(value: string) {
  const url = campusLink(value)
  if (!url) throw Error("Invalid link")
  if (campusToolsAvailable()) { await bridge.openLink({ url }); return }
  window.open(url, "_blank", "noopener,noreferrer")
}

export function dateDistance(date: string, today: string) {
  return Math.round((Date.parse(date + "T00:00:00Z") - Date.parse(today + "T00:00:00Z")) / 86400000)
}

export function validPlanDate(value: string) {
  if (!/^\d{4}-\d{2}-\d{2}$/.test(value) || value < "2000-01-01" || value > "2100-12-31") return false
  const date = new Date(value + "T00:00:00Z")
  return Number.isFinite(date.getTime()) && date.toISOString().slice(0, 10) === value
}

export type Countdown = { id: string; title: string; duration: number; remaining: number; endsAt: number | null }
export const countdownRemaining = (item: Countdown, now: number) =>
  item.endsAt === null ? item.remaining : Math.min(item.duration, Math.max(0, item.endsAt - now))
