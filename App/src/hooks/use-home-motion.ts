import { useEffect, useState, useSyncExternalStore } from "react"
import { Capacitor, registerPlugin, type PluginListenerHandle } from "@capacitor/core"
const app = registerPlugin<{ getState(): Promise<{ isActive: boolean }>; addListener(event: "appStateChange", listener: (state: { isActive: boolean }) => void): Promise<PluginListenerHandle> }>("App")
function environment() { return document.hidden || matchMedia("(prefers-reduced-motion: reduce)").matches }
function subscribe(callback: () => void) {
  const media = matchMedia("(prefers-reduced-motion: reduce)")
  document.addEventListener("visibilitychange", callback); media.addEventListener("change", callback)
  return () => { document.removeEventListener("visibilitychange", callback); media.removeEventListener("change", callback) }
}
export function useHomeMotion(active: boolean) {
  const stopped = useSyncExternalStore(subscribe, environment)
  const [foreground, setForeground] = useState(true)
  useEffect(() => {
    if (!Capacitor.isPluginAvailable("App")) return
    let live = true, changed = false, handle: PluginListenerHandle | undefined
    void app.addListener("appStateChange", state => { changed = true; if (live && typeof state.isActive === "boolean") setForeground(state.isActive) }).then(h => { if (live) handle = h; else void h.remove() }).catch(() => {})
    void app.getState().then(state => { if (live && !changed && typeof state.isActive === "boolean") setForeground(state.isActive) }).catch(() => {})
    return () => { live = false; void handle?.remove() }
  }, [])
  return { playing: active && !stopped && foreground }
}
