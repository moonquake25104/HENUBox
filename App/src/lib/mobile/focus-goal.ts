import { useSyncExternalStore } from "react"
const KEY = "xique-focus-goal-minutes-v1", EVENT = "xique-focus-goal-change"
let sessionGoal: number | null = null
function snapshot() {
  return savedFocusGoal() ?? 0
}
export function savedFocusGoal(): number | null {
  if (sessionGoal !== null) return sessionGoal
  try {
    const stored = localStorage.getItem(KEY)
    if (stored === null) return null
    const minutes = Number(stored)
    return Number.isInteger(minutes) && minutes >= 0 && minutes <= 720 ? minutes : null
  } catch { return null }
}
export function restoreFocusGoal(minutes: number): boolean {
  if (!Number.isInteger(minutes) || minutes < 0 || minutes > 720) throw Error("GOAL_RANGE")
  if (savedFocusGoal() !== null) return false
  localStorage.setItem(KEY, String(minutes))
  window.dispatchEvent(new Event(EVENT))
  return true
}
function subscribe(callback: () => void) { window.addEventListener(EVENT, callback); window.addEventListener("storage", callback); return () => { window.removeEventListener(EVENT, callback); window.removeEventListener("storage", callback) } }
export function setFocusGoal(minutes: number) {
  if (!Number.isInteger(minutes) || minutes < 0 || minutes > 720) throw Error("GOAL_RANGE")
  try { localStorage.setItem(KEY, String(minutes)); sessionGoal = null } catch { sessionGoal = minutes }
  window.dispatchEvent(new Event(EVENT))
}
export function useFocusGoal() { return { minutes: useSyncExternalStore(subscribe, snapshot), setGoal: setFocusGoal } }
