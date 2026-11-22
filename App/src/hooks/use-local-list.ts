import { useCallback, useEffect, useState } from "react"

const changeEvent = "henubox:local-list-changed"

export function useLocalList<T extends { id: string }>(key: string, isItem: (value: unknown) => value is T, limit: number) {
  const read = useCallback(() => {
    try {
      const raw = localStorage.getItem(key)
      const saved: unknown = raw ? JSON.parse(raw) : []
      if (Array.isArray(saved) && saved.every(isItem)) return { items: saved, unreadable: false }
    } catch { /* Preserve existing data when storage is inaccessible. */ }
    return { items: [] as T[], unreadable: true }
  }, [key, isItem])
  const [{ items, unreadable }, setData] = useState(read)
  useEffect(() => {
    const changed = (event: Event) => {
      if ((event as CustomEvent<string>).detail === key) setData(read())
    }
    window.addEventListener(changeEvent, changed)
    return () => window.removeEventListener(changeEvent, changed)
  }, [key, read])
  const [error, setError] = useState("")
  function write(next: T[]) {
    if (unreadable) return false
    if (next.length > limit) {
      setError(`最多保存 ${limit} 条，请先删除一条旧记录。`)
      return false
    }
    try {
      localStorage.setItem(key, JSON.stringify(next))
      window.dispatchEvent(new CustomEvent(changeEvent, { detail: key }))
      setError("")
      return true
    } catch {
      setError("暂时无法保存在本机，请重试。")
      return false
    }
  }
  return {
    items,
    error: unreadable ? "本机数据暂时无法读取。原记录已保留。" : error,
    save: (item: T) => write(items.some(value => value.id === item.id) ? items.map(value => value.id === item.id ? item : value) : [item, ...items]),
    remove: (id: string) => write(items.filter(value => value.id !== id)),
  }
}
