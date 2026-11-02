import { useSyncExternalStore } from "react"
const KEY = "xique-focus-goal-minutes-v1", EVENT = "xique-focus-goal-change"
let sessionGoal: number | null = null
function snapshot() {
  if (sessionGoal !== null) return sessionGoal
  try { const n = Number(localStorage.getItem(KEY)); return Number.isInteger(n) && n >= 0 && n <= 720 ? n : 0 } catch { return 0 }
}
function subscribe(callback: () => void) { window.addEventListener(EVENT, callback); window.addEventListener("storage", callback); return () => { window.removeEventListener(EVENT, callback); window.removeEventListener("storage", callback) } }
export function setFocusGoal(minutes: number) {
  if (!Number.isInteger(minutes) || minutes < 0 || minutes > 720) throw Error("GOAL_RANGE")
  try { localStorage.setItem(KEY, String(minutes)); sessionGoal = null } catch { sessionGoal = minutes }
  window.dispatchEvent(new Event(EVENT))
}
export function useFocusGoal() { return { minutes: useSyncExternalStore(subscribe, snapshot), setGoal: setFocusGoal } }
