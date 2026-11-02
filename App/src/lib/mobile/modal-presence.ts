import { useSyncExternalStore } from "react"
const dialogs = new Set<string>(), listeners = new Set<() => void>()
export function setModalPresence(id: string, open: boolean) {
  const before = dialogs.size > 0
  if (open) dialogs.add(id); else dialogs.delete(id)
  if (before !== (dialogs.size > 0)) listeners.forEach(fn => fn())
}
const subscribe = (fn: () => void) => { listeners.add(fn); return () => { listeners.delete(fn) } }
export function useModalPresence() { return useSyncExternalStore(subscribe, () => dialogs.size > 0, () => false) }
