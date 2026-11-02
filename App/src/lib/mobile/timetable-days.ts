import { useSyncExternalStore } from "react"
import { Capacitor, registerPlugin } from "@capacitor/core"

type WeekDays = 5 | 7
const key = "henubox-timetable-days-v1"
const bridge = registerPlugin<{getTimetableDays(): Promise<{days: number}>; setTimetableDays(value: {days: WeekDays}): Promise<void>}>("XiqueAppearance")
let generation = 0, writes: Promise<void> = Promise.resolve()
function persistNative(days: WeekDays) {
  if (!Capacitor.isPluginAvailable("XiqueAppearance")) return
  writes = writes.catch(() => {}).then(() => bridge.setTimetableDays({ days })).catch(() => { /* Browser preference remains available if an older shell lacks this method. */ })
}
function snapshot(): WeekDays { return document.documentElement.dataset.timetableDays === "5" ? 5 : 7 }
function setDays(days: WeekDays) {
  if (days !== 5 && days !== 7) return
  generation++
  document.documentElement.dataset.timetableDays = String(days)
  try { localStorage.setItem(key, String(days)) } catch { /* Keep this session's choice if storage is unavailable. */ }
  persistNative(days)
}
function subscribe(change: () => void) {
  const observer = new MutationObserver(change)
  observer.observe(document.documentElement, { attributes: true, attributeFilter: ["data-timetable-days"] })
  const storage = (event: StorageEvent) => {
    if (event.key === key) document.documentElement.dataset.timetableDays = event.newValue === "5" ? "5" : "7"
  }
  window.addEventListener("storage", storage)
  return () => { observer.disconnect(); window.removeEventListener("storage", storage) }
}
export function useTimetableDays() { return { days: useSyncExternalStore(subscribe, snapshot), setDays } }
try { document.documentElement.dataset.timetableDays = localStorage.getItem(key) === "5" ? "5" : "7" } catch { document.documentElement.dataset.timetableDays = "7" }
if (Capacitor.isPluginAvailable("XiqueAppearance")) {
  const initialGeneration = generation
  void bridge.getTimetableDays().then(value => {
    if (generation !== initialGeneration) return
    if (value.days === 5 || value.days === 7) {
      document.documentElement.dataset.timetableDays = String(value.days)
      try { localStorage.setItem(key, String(value.days)) } catch { /* Native preference still restores the displayed choice. */ }
    } else if (value.days === 0) persistNative(snapshot())
  }).catch(() => { /* Older shells and browser previews use local storage. */ })
}
