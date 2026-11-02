import { useCallback, useEffect, useRef, useState } from "react"
import { focusAvailable, focusBridge, focusError, type FocusMode, type FocusSettings, type FocusState } from "@/lib/mobile/focus"

export function useFocus() {
  const [state, setState] = useState<FocusState | null>(null)
  const [error, setError] = useState("")
  const [busy, setBusy] = useState(false)
  const alive = useRef(true), locked = useRef(false), sequence = useRef(0)
  const current = useRef<FocusState | null>(null)
  const apply = useCallback((value: FocusState) => {
    if (value?.schema_version !== 1 || !Number.isSafeInteger(value.revision) || !Array.isArray(value.records)) throw Error("Invalid focus state")
    if (!alive.current || value.revision < (current.current?.revision ?? -1)) return
    current.current = value; setState(value)
  }, [])
  const refresh = useCallback(async () => {
    if (!focusAvailable() || locked.current) return
    const id = ++sequence.current
    try { const s = await focusBridge.getState(); if (id === sequence.current) apply(s) }
    catch (e) { if (alive.current && id === sequence.current) setError(focusError(e)) }
  }, [apply])
  useEffect(() => {
    alive.current = true
    void refresh()
    const timer = window.setInterval(() => { if (document.visibilityState === "visible") void refresh() }, 1000)
    const visible = () => { if (document.visibilityState === "visible") void refresh() }
    document.addEventListener("visibilitychange", visible)
    return () => { alive.current = false; window.clearInterval(timer); document.removeEventListener("visibilitychange", visible) }
  }, [refresh])
  const perform = async (operation: (s: FocusState) => Promise<FocusState>) => {
    if (locked.current || !current.current) return false
    locked.current = true; sequence.current++; setBusy(true); setError("")
    try { apply(await operation(current.current)); return true }
    catch (e) { if (alive.current) setError(focusError(e)); return false }
    finally { locked.current = false; if (alive.current) { setBusy(false); void refresh() } }
  }
  return { state, error, busy, refresh,
    command: (action: "start" | "pause" | "resume" | "finish" | "tag" | "archive", mode: FocusMode | "" = "", label = "") => perform(s => focusBridge.command({ action, mode, label, expected_revision: s.revision, request_id: crypto.randomUUID() })),
    saveSettings: (settings: FocusSettings) => perform(s => focusBridge.saveSettings({ expected_revision: s.revision, settings })),
  }
}
export type FocusController = ReturnType<typeof useFocus>
