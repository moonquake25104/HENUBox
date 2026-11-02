import { useCallback, useEffect, useRef, useState } from "react"
import { todoBridge, todoError, todosAvailable, type TodoState, type TodoDraft, type TodoItem } from "@/lib/mobile/todos"

export function useTodos() {
  const [state, setState] = useState<TodoState | null>(null), [loadError, setLoadError] = useState(""), [actionError, setActionError] = useState(""), [busy, setBusy] = useState(false)
  const live = useRef(true), locked = useRef(false), sequence = useRef(0), current = useRef<TodoState | null>(null)
  const apply = useCallback((s: TodoState) => { if (s.schema_version !== 1 || !Number.isSafeInteger(s.revision) || !Array.isArray(s.items)) throw Error("Invalid todo snapshot"); if (live.current && s.revision >= (current.current?.revision ?? -1)) { current.current = s; setState(s) } }, [])
  const refresh = useCallback(async () => {
    if (!todosAvailable() || locked.current) return
    const seq = ++sequence.current
    try { const s = await todoBridge.getState(); if (seq === sequence.current) { apply(s); if (live.current) setLoadError("") } } catch (e) { if (live.current && seq === sequence.current) setLoadError(todoError(e)) }
  }, [apply])
  useEffect(() => {
    live.current = true; void refresh()
    const visible = () => { if (!document.hidden) void refresh() }, timer = window.setInterval(visible, 3000)
    document.addEventListener("visibilitychange", visible)
    return () => { live.current = false; window.clearInterval(timer); document.removeEventListener("visibilitychange", visible) }
  }, [refresh])
  const perform = async (operation: () => Promise<TodoState>) => {
    if (locked.current || !todosAvailable()) return false
    locked.current = true; sequence.current++; setBusy(true); setActionError("")
    try { apply(await operation()); return true } catch (e) { if (live.current) setActionError(todoError(e)); return false }
    finally { locked.current = false; if (live.current) { setBusy(false); void refresh() } }
  }
  return { state, busy, available: todosAvailable(), error: actionError || loadError, refresh,
    save: (id: string, original: TodoItem | null, draft: TodoDraft) => perform(() => original ? todoBridge.update({ id, expected_revision: original.revision, ...draft }) : todoBridge.create({ id, ...draft })),
    complete: (item: TodoItem, completed: boolean) => perform(() => { const latest = current.current?.items.find(i => i.id === item.id) ?? item; return todoBridge.setCompleted({ id: item.id, expected_revision: latest.revision, completed }) }),
    remove: (item: TodoItem) => perform(() => todoBridge.remove({ id: item.id, expected_revision: item.revision })),
  }
}
export type TodoController = ReturnType<typeof useTodos>
