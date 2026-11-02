import { useEffect, useState } from "react"
import { Capacitor, registerPlugin } from "@capacitor/core"
const key = "henubox-intro-seen-v1"
const bridge = registerPlugin<{ getIntroNotice(): Promise<{ acknowledged: boolean }>; acknowledgeIntroNotice(): Promise<{ acknowledged: boolean }> }>("XiqueAppearance")
function localSeen() { try { return localStorage.getItem(key) === "1" } catch { return false } }
export function useIntroNotice() {
  const [open, setOpen] = useState(false), [busy, setBusy] = useState(false), [error, setError] = useState("")
  useEffect(() => {
    let live = true
    const read = async () => {
      let seen = localSeen()
      if (Capacitor.isPluginAvailable("XiqueAppearance")) try { const result = await bridge.getIntroNotice(); if (typeof result.acknowledged === "boolean") seen = result.acknowledged } catch { /* Old shell/browser uses its local marker. */ }
      if (live) setOpen(!seen)
    }
    void read(); return () => { live = false }
  }, [])
  const acknowledge = async () => {
    if (busy) return
    setBusy(true); setError("")
    try {
      let nativeSaved = false
      if (Capacitor.isPluginAvailable("XiqueAppearance")) {
        try { nativeSaved = (await bridge.acknowledgeIntroNotice()).acknowledged === true }
        catch (e) { if ((e as { code?: string })?.code === "STORAGE_UNAVAILABLE") throw e }
      }
      try { localStorage.setItem(key, "1") } catch { if (!nativeSaved) throw Error("STORAGE_UNAVAILABLE") }
      setOpen(false)
    } catch { setError("确认状态暂时未能保存，请重试。") } finally { setBusy(false) }
  }
  return { open, busy, error, acknowledge }
}
