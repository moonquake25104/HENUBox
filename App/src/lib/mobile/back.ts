import { Capacitor, registerPlugin, type PluginListenerHandle } from "@capacitor/core"
import { useEffect, useLayoutEffect, useRef } from "react"

// The App plugin is already included by the Android shell. Keep frontend
// dependencies separate from the shell rather than importing its node_modules.
interface AndroidApp {
  addListener(event: "backButton", listener: () => void): Promise<PluginListenerHandle>
  minimizeApp(): Promise<void>
}
const app = registerPlugin<AndroidApp>("App")
const layers: { token: symbol; priority: number; back: () => void }[] = []

/** Register visible layers. Dialogs precede full-screen overlays; newest wins ties. */
export function useMobileBack(enabled: boolean, back: () => void, priority = 10) {
  const latest = useRef(back)
  useLayoutEffect(() => {
    latest.current = back
  }, [back])
  useEffect(() => {
    if (!enabled) return
    const token = Symbol("mobile-back-layer")
    layers.push({ token, priority, back: () => latest.current() })
    return () => {
      const index = layers.findIndex((layer) => layer.token === token)
      if (index !== -1) layers.splice(index, 1)
    }
  }, [enabled, priority])
}

/** One native listener for the whole mobile app, removed even if setup finishes late. */
export function useAndroidBackNavigation(returnHome: () => boolean) {
  const latest = useRef(returnHome)
  useLayoutEffect(() => {
    latest.current = returnHome
  }, [returnHome])
  useEffect(() => {
    if (!Capacitor.isPluginAvailable("App")) return
    let disposed = false
    let handle: PluginListenerHandle | undefined
    void app
      .addListener("backButton", () => {
        if (disposed) return
        const layer = layers.reduce<(typeof layers)[number] | undefined>(
          (top, next) => (!top || next.priority >= top.priority ? next : top),
          undefined,
        )
        if (layer) layer.back()
        else if (!latest.current()) void app.minimizeApp().catch(() => {})
      })
      .then((value) => {
        if (disposed) void value.remove().catch(() => {})
        else handle = value
      })
      .catch(() => {})
    return () => {
      disposed = true
      if (handle) void handle.remove().catch(() => {})
    }
  }, [])
}
