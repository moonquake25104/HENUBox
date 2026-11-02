import { useSyncExternalStore } from "react"

export type UiStyle = "classic" | "glass" | "android"
const KEY = "xique-ui-style-v1"
function snapshot(): UiStyle { const style = document.documentElement.dataset.uiStyle; return style === "glass" || style === "android" ? style : "classic" }
function subscribe(callback: () => void) {
  const observer = new MutationObserver(callback)
  observer.observe(document.documentElement, { attributes: true, attributeFilter: ["data-ui-style"] })
  return () => observer.disconnect()
}
export function setUiStyle(value: UiStyle) {
  document.documentElement.dataset.uiStyle = value
  try { localStorage.setItem(KEY, value) } catch { /* Style still works for this session. */ }
}
export function useUiStyle() { return { style: useSyncExternalStore(subscribe, snapshot), setStyle: setUiStyle } }
try { const saved = localStorage.getItem(KEY); setUiStyle(saved === "glass" || saved === "android" ? saved : "classic") } catch { setUiStyle("classic") }
